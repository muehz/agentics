package shellwords

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
)

// waitForNoLeakedForks lets parses abandoned by an earlier test finish, so the cap tests do not depend on order.
func waitForNoLeakedForks(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for leakedForks.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d abandoned parses are still running", leakedForks.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// blockForkedParses makes every forked child parse wait until the test ends, or 5 s at most, so a missing deadline
// fails the timing assertions instead of hanging the run. It shortens the ceiling to ceiling and, when the test ends,
// releases the parses, waits until none is left running and restores both variables. It returns the number of forked
// parses started so far.
func blockForkedParses(t *testing.T, ceiling time.Duration) *atomic.Int32 {
	t.Helper()
	waitForNoLeakedForks(t)
	release := make(chan struct{})
	var started atomic.Int32
	oldHook, oldCeiling := scriptParseHook, scriptCeiling
	scriptParseHook = func(*parser) {
		started.Add(1)
		select {
		case <-release:
		case <-time.After(5 * time.Second):
		}
	}
	scriptCeiling = ceiling
	t.Cleanup(func() {
		close(release)
		waitForNoLeakedForks(t)
		scriptParseHook, scriptCeiling = oldHook, oldCeiling
	})
	return &started
}

// countingReader counts the reads that reach its files.
type countingReader struct {
	files fakeReader
	reads atomic.Int32
}

func (c *countingReader) ReadScript(path string) ScriptRead {
	c.reads.Add(1)
	return c.files.ReadScript(path)
}

func chainFixture(verb string, levels int) fakeReader {
	files := fakeReader{}
	names := []string{"a", "b", "c", "d", "e"}
	for i := 0; i < levels; i++ {
		text := "echo end\n"
		if i < levels-1 {
			text = verb + " /ws/" + names[i+1] + ".sh\n"
		}
		files["/ws/"+names[i]+".sh"] = ScriptRead{Text: text}
	}
	return files
}

func TestScriptDepthBoundIsThreeLevels(t *testing.T) {
	for _, verb := range []string{"bash", "source"} {
		res := parseScript("bash /ws/a.sh", chainFixture(verb, 3))
		if got := firstScriptReason(res); got != "" {
			t.Errorf("%s: three nested scripts refused: %q", verb, got)
		}
		if child := scriptChild(res); child == nil || child.Script.Path != "/ws/c.sh" {
			t.Errorf("%s: the third script was not expanded: %+v", verb, res.Invocations)
		}
		want := "script-depth in /ws/c.sh:1 (via /ws/a.sh > /ws/b.sh)"
		if got := firstScriptReason(parseScript("bash /ws/a.sh", chainFixture(verb, 4))); got != want {
			t.Errorf("%s: fourth script: want %q, got %q", verb, want, got)
		}
	}
	// A wrapper on the typed call adds wrapper depth but no script, so three nested scripts still pass.
	res := parseScript("env bash /ws/a.sh", chainFixture("bash", 3))
	if got := firstScriptReason(res); got != "" {
		t.Errorf("wrapped call with three scripts refused: %q", got)
	}
	if got := firstScriptReason(parseScript("env bash /ws/a.sh", chainFixture("bash", 4))); !strings.HasPrefix(got, "script-depth") {
		t.Errorf("wrapped call with four scripts: %q", got)
	}
}

func TestScriptIdentityCycle(t *testing.T) {
	// alias.sh is a hardlink of p.sh: another path, the same identity and text.
	alias := fakeReader{
		"/ws/p.sh":     {ID: "6", Text: "bash /ws/alias.sh\n"},
		"/ws/alias.sh": {ID: "6", Text: "bash /ws/alias.sh\n"},
	}
	if got, want := firstScriptReason(parseScript("bash /ws/p.sh", alias)), "script-cycle in /ws/p.sh:1"; got != want {
		t.Errorf("alias: want %q, got %q", want, got)
	}
	// The same file called twice in a row is no cycle: the identity leaves the chain with its script.
	twice := fakeReader{"/ws/p.sh": {ID: "6", Text: "echo p\n"}, "/ws/alias.sh": {ID: "6", Text: "echo p\n"}}
	if got := firstScriptReason(parseScript("bash /ws/p.sh && bash /ws/alias.sh", twice)); got != "" {
		t.Errorf("sequential alias: %q", got)
	}
	// Scripts without an identity never match each other, and neither do two different identities.
	for name, files := range map[string]fakeReader{
		"no identity": {"/ws/x.sh": {Text: "bash /ws/y.sh\n"}, "/ws/y.sh": {Text: "echo end\n"}},
		"distinct":    {"/ws/x.sh": {ID: "1", Text: "bash /ws/y.sh\n"}, "/ws/y.sh": {ID: "2", Text: "echo end\n"}},
	} {
		res := parseScript("bash /ws/x.sh", files)
		if got := firstScriptReason(res); got != "" {
			t.Errorf("%s: %q", name, got)
		}
		if child := scriptChild(res); child == nil || child.Script.Path != "/ws/y.sh" {
			t.Errorf("%s: nested script not expanded: %+v", name, res.Invocations)
		}
	}
}

// A script that names itself is refused from the path alone, before a second read: the snapshot counts every distinct
// read toward its file budget, and the identity check after a read is a second layer for aliases.
func TestScriptPathCycleIsRefusedWithoutAnotherRead(t *testing.T) {
	reader := &countingReader{files: fakeReader{"/ws/a.sh": {Text: "bash /ws/a.sh\n"}}}
	res := ParseScripts(context.Background(), "bash /ws/a.sh", POSIX, ScriptOptions{Reader: reader, Context: provenPOSIX, Cwd: "/ws"})
	if got, want := firstScriptReason(res), "script-cycle in /ws/a.sh:1"; got != want {
		t.Errorf("want %q, got %q", want, got)
	}
	if n := reader.reads.Load(); n != 1 {
		t.Errorf("the cycle cost %d reads, want 1", n)
	}
}

// Wrappers on the typed call count toward the same nesting bound as scripts, and a source adds no wrapper of its own, so
// the bound is checked where a script is expanded.
func TestScriptWrapperDepthCountsSourcedNests(t *testing.T) {
	files := fakeReader{"/ws/a.sh": {Text: "source /ws/b.sh\n"}, "/ws/b.sh": {Text: "echo end\n"}}
	if got := firstScriptReason(parseScript("env bash /ws/a.sh", files)); got != "" {
		t.Errorf("one wrapper: %q", got)
	}
	if got := firstScriptReason(parseScript("env env env bash /ws/a.sh", files)); !strings.HasPrefix(got, "script-depth in /ws/a.sh:1") {
		t.Errorf("three wrappers and a sourced nest: %q", got)
	}
}

func TestScriptPrescanCaps(t *testing.T) {
	over := map[string]string{
		"lines":           strings.Repeat("echo a\n", 10001),
		"unfinished line": strings.Repeat("echo a\n", 10000) + "echo a",
		"tokens":          strings.Repeat("a ", 100001) + "\n",
		"nesting":         strings.Repeat("(", 65) + "echo a" + strings.Repeat(")", 65) + "\n",
		"mixed nesting":   strings.Repeat("{[(", 22) + "\n",
		// Unmatched closers never bank depth for the openers after them.
		"closers first": strings.Repeat(")", 100) + strings.Repeat("(", 65) + "\n",
	}
	at := map[string]string{
		"lines":       strings.Repeat("echo a\n", 10000),
		"tokens":      strings.Repeat("a ", 100000) + "\n",
		"nesting":     strings.Repeat("(", 64) + "echo a" + strings.Repeat(")", 64) + "\n",
		"closed runs": strings.Repeat("(echo a)\n", 100),
	}
	for name, body := range over {
		if got := prescan(body); got != RuleBudget {
			t.Errorf("over %s: prescan %v", name, got)
		}
		res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: body}})
		if got := firstScriptReason(res); got != "script-budget" {
			t.Errorf("over %s: want script-budget, got %q", name, got)
		}
		if len(res.Scripts) != 0 {
			t.Errorf("over %s: a refused script was recorded: %v", name, res.Scripts)
		}
	}
	for name, body := range at {
		if got := prescan(body); got != RuleNone {
			t.Errorf("at %s: prescan %v", name, got)
		}
	}
	// The limits sit before the lexer, not instead of it: text at the line and nesting limits is still parsed.
	for _, name := range []string{"lines", "nesting"} {
		res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: at[name]}})
		if got := firstScriptReason(res); strings.HasPrefix(got, "script-budget") || len(res.Scripts) != 1 {
			t.Errorf("at %s: %q, scripts %d", name, got, len(res.Scripts))
		}
	}
}

func TestScriptDeadlineAndConcurrencyCap(t *testing.T) {
	const ceiling = 100 * time.Millisecond
	started := blockForkedParses(t, ceiling)
	files := fakeReader{"/ws/t.sh": {ID: "1", Text: "echo a\n"}}
	for i := 0; i < 4; i++ {
		start := time.Now()
		res := parseScript("bash /ws/t.sh", files)
		took := time.Since(start)
		// A missing deadline waits out the blocked parse, 5 s, well past this bound even under the race detector.
		if got := firstScriptReason(res); got != "script-budget" || took > ceiling+raceSlowdown*300*time.Millisecond {
			t.Fatalf("call %d: deadline not enforced: %q after %v", i, got, took)
		}
		if len(res.Scripts) != 0 || len(res.Invocations) != 1 {
			t.Fatalf("call %d: a timed-out script left state behind: %+v", i, res)
		}
	}
	if n := leakedForks.Load(); n != 4 {
		t.Fatalf("want %d abandoned parses, got %d", 4, n)
	}
	// Four timed-out parses are still alive, so the fifth call is refused without starting a parse. Its ceiling is
	// long, so a call that started one anyway would also miss the time bound.
	scriptCeiling = 2 * time.Second
	start := time.Now()
	got := firstScriptReason(parseScript("bash /ws/t.sh", files))
	if took := time.Since(start); got != "script-budget" || took > raceSlowdown*150*time.Millisecond {
		t.Fatalf("cap not enforced: %q after %v", got, took)
	}
	if n, m := started.Load(), leakedForks.Load(); n != 4 || m != 4 {
		t.Fatalf("the fifth call started a parse: %d started, %d abandoned", n, m)
	}
}

func TestScriptFinishedParsesNeverCountAgainstTheCap(t *testing.T) {
	waitForNoLeakedForks(t)
	files := fakeReader{"/ws/t.sh": {ID: "1", Text: "bash /ws/u.sh\n"}, "/ws/u.sh": {ID: "2", Text: "echo a\n"}}
	for i := 0; i < 20; i++ {
		if got := firstScriptReason(parseScript("bash /ws/t.sh", files)); got != "" {
			t.Fatalf("call %d refused: %q", i, got)
		}
	}
	if n := leakedForks.Load(); n != 0 {
		t.Fatalf("finished parses counted as abandoned: %d", n)
	}
}

func TestScriptDeadlineIsSharedAcrossCallsOfOneParse(t *testing.T) {
	files := fakeReader{"/ws/a.sh": {ID: "1", Text: "echo a\n"}, "/ws/b.sh": {ID: "2", Text: "echo b\n"}}
	command := "bash /ws/a.sh && bash /ws/b.sh"
	if res := parseScript(command, files); firstScriptReason(res) != "" || len(res.Scripts) != 2 {
		t.Fatalf("unblocked: both calls should be inspected: %+v", res)
	}
	const ceiling = 300 * time.Millisecond
	blockForkedParses(t, ceiling)
	reader := &countingReader{files: files}
	start := time.Now()
	res := ParseScripts(context.Background(), command, POSIX, ScriptOptions{Reader: reader, Context: provenPOSIX, Cwd: "/ws"})
	took := time.Since(start)
	if len(res.Invocations) != 2 {
		t.Fatalf("want two refused calls, got %+v", res.Invocations)
	}
	for i, inv := range res.Invocations {
		if inv.Inspectable || inv.Reason.String() != "script-budget" {
			t.Errorf("call %d: want script-budget, got %q", i, inv.Reason)
		}
	}
	// One ceiling for the whole parse: a ceiling for each call would take twice as long, and the second call, made
	// after the deadline, neither reads its file nor starts a parse.
	if took > ceiling+raceSlowdown*200*time.Millisecond {
		t.Errorf("two calls took %v, over one ceiling of %v", took, ceiling)
	}
	if n := leakedForks.Load(); n != 1 {
		t.Errorf("want one abandoned parse, got %d", n)
	}
	if n := reader.reads.Load(); n != 1 {
		t.Errorf("want one read, the first call's, got %d", n)
	}
}

// A script parse adopted after one of its own nested parses was abandoned rewrites operands and locates call prefixes
// of invocations the abandoned parse still holds; the abandoned copy keeps the values it had.
func TestScriptPassesAfterAdoptionLeaveAnAbandonedCopyAlone(t *testing.T) {
	waitForNoLeakedForks(t)
	files := fakeReader{
		// Both operand rewrites, a proven constant and a proven temp, and a call prefix (line 5) to locate.
		"/ws/outer.sh": {Text: "D=./build\ntmp=$(mktemp)\nrm -rf \"$D\"\nrm -f \"$tmp\"\nPATH=/x bash /ws/mid.sh\nbash /ws/inner.sh\n"},
		"/ws/mid.sh":   {Text: "echo m\n"},
		"/ws/inner.sh": {Text: "echo i\n"},
	}
	release := make(chan struct{})
	abandoned := make(chan *parser, 1)
	var calls atomic.Int32
	oldHook := scriptParseHook
	t.Cleanup(func() {
		close(release)
		waitForNoLeakedForks(t)
		scriptParseHook = oldHook
	})
	scriptParseHook = func(child *parser) {
		switch calls.Add(1) {
		case 1:
			// outer.sh: its own nested parses get a short deadline, while the typed call keeps the long one, so the
			// parse of inner.sh is abandoned and outer.sh is still adopted.
			child.script.deadline = time.Now().Add(raceSlowdown * 100 * time.Millisecond)
		case 3:
			abandoned <- child
			select {
			case <-release:
			case <-time.After(5 * time.Second):
			}
		}
	}
	res := parseScript("bash /ws/outer.sh", files)
	var rewritten []string
	prefixLine := 0
	for _, inv := range res.Invocations {
		switch {
		case len(inv.Words) == 3 && inv.Words[0].Text == "rm" && inv.Words[2].Literal:
			rewritten = append(rewritten, inv.Words[2].Text)
		case inv.Words[0].Text == "script-construct":
			prefixLine = inv.Script.Line
		}
	}
	if want := []string{"./build", scriptTempPath}; !slices.Equal(rewritten, want) || prefixLine != 5 || calls.Load() != 3 {
		t.Fatalf("fixture: want operands %q rewritten and the prefix on line 5, got %q, line %d, %d parses",
			want, rewritten, prefixLine, calls.Load())
	}
	child := <-abandoned
	held := 0
	for _, inv := range child.result.Invocations {
		switch {
		case len(inv.Words) == 3 && inv.Words[0].Text == "rm":
			held++
			if !strings.HasPrefix(inv.Words[2].Raw, `"$`) {
				t.Errorf("the abandoned copy's operand was rewritten in place: %+v", inv.Words[2])
			}
		case inv.Words[0].Text == "script-construct":
			held++
			if inv.Script.Line != 0 {
				t.Errorf("the abandoned copy's origin was written in place: line %d", inv.Script.Line)
			}
		}
	}
	if held != 3 {
		t.Fatalf("the abandoned copy should hold both rm calls and the prefix stand-in: %+v", child.result.Invocations)
	}
}

// fillReferences gives every slice of v one element and every map one entry, recursively through structs and slice
// elements, so a field the fork forgets to clone is shared and visible to checkUnshared. Maps stay nil when nilMaps
// is set. Pointer, interface and func fields are left nil.
func fillReferences(v reflect.Value, nilMaps bool) {
	switch v.Kind() {
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			return
		}
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			fillReferences(reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem(), nilMaps)
		}
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 2)
		fillReferences(s.Index(0), nilMaps)
		v.Set(s)
	case reflect.Map:
		if !nilMaps {
			m := reflect.MakeMap(v.Type())
			m.SetMapIndex(reflect.Zero(v.Type().Key()), reflect.Zero(v.Type().Elem()))
			v.Set(m)
		}
	}
}

// sharedReadOnly are the reference fields a fork shares on purpose: none of them is written after a parse starts.
var sharedReadOnly = map[string]bool{"parser.ctx": true, "parser.script.opts.Reader": true, "parser.script.parseHook": true}

// checkUnshared fails when clone shares a slice or map with orig, or when a reference field is neither cloned nor
// listed as shared read-only. Slice elements are compared only for commandList, whose segments the cwd replay writes;
// an invocation's inner slices are shared by design.
func checkUnshared(t *testing.T, path string, orig, clone reflect.Value) {
	t.Helper()
	switch orig.Kind() {
	case reflect.Struct:
		if orig.Type() == reflect.TypeOf(time.Time{}) {
			return
		}
		for i := 0; i < orig.NumField(); i++ {
			checkUnshared(t, path+"."+orig.Type().Field(i).Name, orig.Field(i), clone.Field(i))
		}
	case reflect.Slice:
		if orig.Cap() > 0 && clone.Cap() > 0 && orig.Pointer() == clone.Pointer() {
			t.Errorf("%s: the fork shares the slice", path)
		}
		if orig.Type().Elem() == reflect.TypeOf(commandList{}) {
			for i := 0; i < min(orig.Len(), clone.Len()); i++ {
				checkUnshared(t, path+"[i]", orig.Index(i), clone.Index(i))
			}
		}
	case reflect.Map:
		switch {
		case orig.IsNil() && !clone.IsNil():
			t.Errorf("%s: a nil map became non-nil", path)
		case !orig.IsNil() && (clone.IsNil() || orig.Pointer() == clone.Pointer()):
			t.Errorf("%s: the fork shares the map", path)
		}
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		if !sharedReadOnly[path] {
			t.Errorf("%s: reference field not reviewed for the fork: clone it, or list it as shared read-only", path)
		}
	}
}

func TestForkSharesNoReferenceState(t *testing.T) {
	for _, nilMaps := range []bool{false, true} {
		var p parser
		fillReferences(reflect.ValueOf(&p).Elem(), nilMaps)
		if len(p.lists) != 1 || len(p.lists[0].segments) != 1 || (p.judged == nil) != nilMaps {
			t.Fatalf("fill missed a field: %+v", p)
		}
		f := p.fork()
		checkUnshared(t, "parser", reflect.ValueOf(&p).Elem(), reflect.ValueOf(f).Elem())
		if !reflect.DeepEqual(&p, f) {
			t.Errorf("nil maps %v: the fork differs from the parser it copies", nilMaps)
		}
	}
}

// snapshotResult copies the slices of r, so a later write into the parser's own arrays shows as a difference.
func snapshotResult(r Result) Result {
	return Result{Invocations: slices.Clone(r.Invocations), EnvironmentEffects: slices.Clone(r.EnvironmentEffects),
		BareAssignments: slices.Clone(r.BareAssignments), Scripts: slices.Clone(r.Scripts), ScriptCalls: slices.Clone(r.ScriptCalls)}
}

func TestScriptAbandonedForkDoesNotTouchTheCaller(t *testing.T) {
	waitForNoLeakedForks(t)
	files := fakeReader{
		"/ws/first.sh":  {ID: "1", Text: "true\n"},
		"/ws/slow.sh":   {ID: "2", Text: "true\ntrue\nPATH=/x true\ntrue\ntrue\n"},
		"/ws/pinned.sh": {ID: "3", Text: "PATH=/x true\n"},
	}
	if got := firstScriptReason(parseScript("bash /ws/pinned.sh", files)); !strings.HasPrefix(got, "script-opaque:") {
		t.Fatalf("the pinned-name fixture must be refused on its own: %q", got)
	}
	// One parser runs three typed commands so that the third one runs, with a fresh deadline, after the abandoned
	// copy has finished writing its own state.
	p := newParser(context.Background(), POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws"})
	p.script.parseHook = func(*parser) {}
	p.parseCommand("bash /ws/first.sh", POSIX, nil, true, Start, -1, 0)
	if p.judged == nil {
		t.Fatal("the first script left no judged map to share")
	}
	release := make(chan struct{})
	p.script.parseHook = func(*parser) {
		select {
		case <-release:
		case <-time.After(5 * time.Second):
		}
	}
	p.script.deadline = time.Now().Add(100 * time.Millisecond)
	p.parseCommand("bash /ws/slow.sh", POSIX, nil, true, Start, -1, 0)
	if last := p.result.Invocations[len(p.result.Invocations)-1]; last.Reason.String() != "script-budget" {
		t.Fatalf("the slow script was not abandoned: %+v", last)
	}
	before, judged, claimed := snapshotResult(p.result), maps.Clone(p.judged), maps.Clone(p.bareClaimed)
	close(release)
	waitForNoLeakedForks(t)
	if !reflect.DeepEqual(p.result, before) || !maps.Equal(p.judged, judged) || !maps.Equal(p.bareClaimed, claimed) {
		t.Fatalf("the abandoned parse changed the caller:\nbefore %+v\nafter  %+v", before, p.result)
	}
	p.script.parseHook = func(*parser) {}
	p.script.deadline = time.Now().Add(2 * time.Second)
	start := len(p.result.Invocations)
	p.parseCommand("bash /ws/pinned.sh", POSIX, nil, true, Start, -1, 0)
	refused := false
	for _, inv := range p.result.Invocations[start:] {
		refused = refused || inv.Reason.Rule() == RuleOpaque
	}
	if !refused {
		t.Fatalf("a pinned-name assignment after an abandoned parse was not refused: %+v", p.result.Invocations[start:])
	}
}

// A copy abandoned at the deadline that then writes every slice element and map it holds changes nothing the caller
// sees.
func TestScriptAbandonedForkWritesStayInTheFork(t *testing.T) {
	waitForNoLeakedForks(t)
	var p parser
	fillReferences(reflect.ValueOf(&p).Elem(), false)
	p.script.parseHook = func(*parser) {}
	p.script.deadline = time.Now().Add(50 * time.Millisecond)
	release, written := make(chan struct{}), make(chan struct{})
	adopted := p.parseForked(func(c *parser) {
		<-release
		c.result.Invocations[0].Reason = ruleRefusal(RuleCycle, nil)
		c.result.EnvironmentEffects[0].Order = 7
		c.result.BareAssignments[0].Text = "fork"
		c.result.Scripts[0].Path = "fork"
		c.wrapperIDs[0] = 7
		c.environmentResets[0].Order = 7
		c.script.chain[0] = "fork"
		c.script.ids[0] = "fork"
		c.bareClaimed[7] = true
		c.bareAt[0] = 7
		c.judged[7] = true
		c.pendingCalls[0].rule = constructEval
		c.lists[0].depth = 7
		c.lists[0].segments[0].end = 7
		close(written)
	})
	if adopted {
		t.Fatal("a copy that missed the deadline was adopted")
	}
	close(release)
	<-written
	waitForNoLeakedForks(t)
	for name, changed := range map[string]bool{
		"Invocations":        p.result.Invocations[0].Reason.Refused(),
		"EnvironmentEffects": p.result.EnvironmentEffects[0].Order != 0,
		"BareAssignments":    p.result.BareAssignments[0].Text != "",
		"Scripts":            p.result.Scripts[0].Path != "",
		"wrapperIDs":         p.wrapperIDs[0] != 0,
		"environmentResets":  p.environmentResets[0].Order != 0,
		"chain":              p.script.chain[0] != "",
		"ids":                p.script.ids[0] != "",
		"bareClaimed":        p.bareClaimed[7],
		"bareAt":             p.bareAt[0] != 0,
		"judged":             p.judged[7],
		"pendingCalls":       p.pendingCalls[0].rule != constructNone,
		"lists":              p.lists[0].depth != 0,
		"segments":           p.lists[0].segments[0].end != 0,
	} {
		if changed {
			t.Errorf("%s: the abandoned copy wrote into the caller's state", name)
		}
	}
}

func TestScriptHostComesFromTheOptions(t *testing.T) {
	files := fakeReader{
		"/ws/t.sh":     {Text: "bash 'sub\\x.sh'\ncd sub/.. && bash ./y.sh\n"},
		"/ws/sub/x.sh": {Text: "echo x\n"}, "/ws/y.sh": {Text: "echo y\n"},
	}
	// The fixture tells the hosts apart twice: a backslash in the script word separates on Windows and is a name
	// character elsewhere, and a .. in a cd is followed by spelling on Windows only. Every line is judged in the
	// forked parse of t.sh, which must follow the host its parse began with. A fork that dropped the host would judge
	// by the running host, so the case whose host differs from the running one is the case that fails on a given
	// runner: the linux case on Windows, the windows case elsewhere.
	cases := []struct {
		host    pathshape.Host
		reasons []string
		paths   []string // nil when the case does not pin them
	}{
		{pathshape.For("linux"), []string{"script-path-not-literal in /ws/t.sh:1", "script-cwd-unproven in /ws/t.sh:2"}, nil},
		{pathshape.For("windows"), nil, []string{"/ws/sub/x.sh", "/ws/t.sh", "/ws/y.sh"}}, // the cd is t.sh's own
	}
	sawForeign := false
	for _, c := range cases {
		sawForeign = sawForeign || c.host != pathshape.Current()
		res := parseScriptOn(c.host, "bash /ws/t.sh", files)
		var reasons, paths []string
		for _, inv := range res.Invocations {
			if !inv.Inspectable {
				reasons = append(reasons, inv.Reason.String())
			}
			if inv.Script != nil {
				paths = append(paths, inv.Script.Path)
			}
		}
		if !slices.Equal(reasons, c.reasons) {
			t.Errorf("host %v: the forked parse did not follow the host of its parse: want reasons %q, got %q", c.host, c.reasons, reasons)
		}
		if c.paths != nil && !slices.Equal(paths, c.paths) {
			t.Errorf("host %v: nested scripts: want %q, got %q", c.host, c.paths, paths)
		}
	}
	if !sawForeign {
		t.Fatal("no case names a host other than the running one")
	}
}

func TestScriptForkPanicIsRefused(t *testing.T) {
	waitForNoLeakedForks(t)
	oldHook := scriptParseHook
	t.Cleanup(func() { scriptParseHook = oldHook })
	files := fakeReader{"/ws/t.sh": {Text: "echo a\n"}}
	scriptParseHook = func(*parser) { panic("parser fault") }
	if res := parseScript("bash /ws/t.sh", files); firstScriptReason(res) != "script-budget" || len(res.Scripts) != 0 {
		t.Fatalf("a panic in the forked parse: %+v", res)
	}
	scriptParseHook = func(*parser) {}
	if got := firstScriptReason(parseScript("bash /ws/t.sh", files)); got != "" {
		t.Fatalf("the same call without the fault: %q", got)
	}
	if n := leakedForks.Load(); n != 0 {
		t.Fatalf("a failed parse counted as abandoned: %d", n)
	}
}
