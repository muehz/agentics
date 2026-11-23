package shellwords

import (
	"context"
	"strings"
	"testing"
	"time"
)

// scriptReasons parses `bash /ws/t.sh` with body as the script text and returns the reasons of every refused child.
func scriptReasons(t *testing.T, body string, extra fakeReader) []string {
	t.Helper()
	files := fakeReader{"/ws/t.sh": {Text: body}}
	for k, v := range extra {
		files[k] = v
	}
	res := parseScript("bash /ws/t.sh", files)
	var reasons []string
	for _, inv := range res.Invocations {
		if !inv.Inspectable {
			reasons = append(reasons, inv.Reason.String())
		}
	}
	return reasons
}

func TestScriptOpacityCore(t *testing.T) {
	cases := []struct{ body, want string }{
		{"eval \"$x\"\n", "script-opaque:eval in /ws/t.sh:1"},
		{"source \"$d/lib.sh\"\n", "script-opaque:source in /ws/t.sh:1"},
		{". \"$d/lib.sh\"\n", "script-opaque:source in /ws/t.sh:1"},
		{"source lib.txt\n", "script-opaque:source in /ws/t.sh:1"},
		{"source ./lib.txt\n", "script-unreadable in /ws/t.sh:1"},
		// bash looks a source operand without a slash up in PATH before the working directory, and sh never looks in
		// the working directory, so a bare name is not the file next to the script even when that file exists.
		{"source lib.sh\n", "script-opaque:source in /ws/t.sh:1"},
		{". lib.sh\n", "script-opaque:source in /ws/t.sh:1"},
		// Behind a wrapper, or spelled with a directory, source is a program of that name, not the builtin, so the
		// file is not read as if it ran in this shell.
		{"env source /ws/lib.sh\n", "script-opaque:source in /ws/t.sh:1"},
		{"/x/source /ws/lib.sh\n", "script-opaque:source in /ws/t.sh:1"},
		{"trap 'echo x' EXIT\n", "script-opaque:trap in /ws/t.sh:1"},
		{"$cmd arg\n", "script-opaque:command-position-expansion in /ws/t.sh:1"},
		{"\"$(pick)\" arg\n", "script-opaque:command-position-expansion in /ws/t.sh:1"},
		{"env $cmd arg\n", "script-opaque:command-position-expansion in /ws/t.sh:1"},
		{"bash -c \"$1\"\n", "script-opaque:dynamic-c in /ws/t.sh:1"},
		{"echo y | bash\n", "script-opaque:stdin-to-shell in /ws/t.sh:1"},
		{"bash <<< \"$x\"\n", "script-opaque:stdin-to-shell in /ws/t.sh:1"},
		{"bash < /ws/x\n", "script-opaque:stdin-to-shell in /ws/t.sh:1"},
		{"zsh -c 'x'\n", "script-opaque:other-shell in /ws/t.sh:1"},
		{"zsh x\n", "script-opaque:other-shell in /ws/t.sh:1"},
		{"fish x\n", "script-opaque:other-shell in /ws/t.sh:1"},
		{"csh x\n", "script-opaque:other-shell in /ws/t.sh:1"},
		{"env -x git status\n", "script-opaque:unsupported-syntax in /ws/t.sh:1"},
		// builtin runs the builtin it names, so the construct is judged on the word after it.
		{"builtin eval \"$x\"\n", "script-opaque:eval in /ws/t.sh:1"},
		{"builtin source ./lib.sh\n", "script-opaque:source in /ws/t.sh:1"},
		{"builtin . ./lib.sh\n", "script-opaque:source in /ws/t.sh:1"},
		{"builtin trap 'echo x' EXIT\n", "script-opaque:trap in /ws/t.sh:1"},
		{"builtin -- eval \"$x\"\n", "script-opaque:eval in /ws/t.sh:1"},
		{"builtin builtin eval \"$x\"\n", "script-opaque:eval in /ws/t.sh:1"},
		{"builtin $cmd arg\n", "script-opaque:command-position-expansion in /ws/t.sh:1"},
		// Wrappers nested past the bound are a nesting refusal, not a syntax one.
		{"env env env env echo x\n", "script-opaque:nesting-depth in /ws/t.sh:1"},
	}
	for _, tc := range cases {
		got := scriptReasons(t, tc.body, fakeReader{"/ws/lib.sh": {Text: "echo lib\n"}})
		found := false
		for _, r := range got {
			found = found || r == tc.want
		}
		if !found {
			t.Errorf("%q: want a child with reason %q, got %v", tc.body, tc.want, got)
		}
	}
}

// parseWithin parses command on another goroutine and fails the test, rather than hanging it, when the parse does not
// finish in time: an unbounded expansion never returns.
func parseWithin(t *testing.T, command string, files fakeReader) Result {
	t.Helper()
	done := make(chan Result, 1)
	go func() { done <- parseScript(command, files) }()
	select {
	case res := <-done:
		return res
	case <-time.After(10 * time.Second):
		t.Fatalf("%q: parse did not finish", command)
		return Result{}
	}
}

// foldingReader reads every path in lower case and reports the lower-case path, as a case-insensitive file system
// would name the file it opened.
type foldingReader struct{ files fakeReader }

func (f foldingReader) ReadScript(path string) ScriptRead {
	return f.files.ReadScript(strings.ToLower(path))
}

// A script that sources or calls itself, directly or through another, and a source chain deeper than the nesting
// bound, end in one refused child with a named reason.
func TestScriptCyclesAndDeepSourcesTerminate(t *testing.T) {
	deep := fakeReader{}
	// A chain holds at most three scripts, t.sh, s1.sh and s2.sh, so the source in s2.sh is past the bound.
	for i, next := range []string{"s1", "s2", "s3", "s4"} {
		name := "/ws/t.sh"
		if i > 0 {
			name = "/ws/s" + string(rune('0'+i)) + ".sh"
		}
		deep[name] = ScriptRead{Text: "source /ws/" + next + ".sh\n"}
	}
	deep["/ws/s4.sh"] = ScriptRead{Text: "echo end\n"}
	// A chain of script calls past the bound gets the same named reason.
	deepCall := fakeReader{}
	for name, read := range deep {
		deepCall[name] = ScriptRead{Text: strings.Replace(read.Text, "source", "bash", 1)}
	}
	cases := []struct {
		name   string
		files  fakeReader
		reason string
	}{
		{"self source", fakeReader{"/ws/t.sh": {Text: "source /ws/t.sh\n"}}, "script-cycle in /ws/t.sh:1"},
		{"self dot", fakeReader{"/ws/t.sh": {Text: ". ./t.sh\n"}}, "script-cycle in /ws/t.sh:1"},
		{"mutual source", fakeReader{"/ws/t.sh": {Text: "source /ws/b.sh\n"}, "/ws/b.sh": {Text: "source /ws/t.sh\n"}}, "script-cycle in /ws/b.sh:1 (via /ws/t.sh)"},
		{"self call", fakeReader{"/ws/t.sh": {Text: "bash /ws/t.sh\n"}}, "script-cycle in /ws/t.sh:1"},
		{"mutual call", fakeReader{"/ws/t.sh": {Text: "bash /ws/b.sh\n"}, "/ws/b.sh": {Text: "source /ws/t.sh\n"}}, "script-cycle in /ws/b.sh:1 (via /ws/t.sh)"},
		{"deep source", deep, "script-depth in /ws/s2.sh:1 (via /ws/t.sh > /ws/s1.sh)"},
		{"deep call", deepCall, "script-depth in /ws/s2.sh:1 (via /ws/t.sh > /ws/s1.sh)"},
	}
	for _, tc := range cases {
		res := parseWithin(t, "bash /ws/t.sh", tc.files)
		var reasons []string
		for _, inv := range res.Invocations {
			if !inv.Inspectable {
				reasons = append(reasons, inv.Reason.String())
			}
		}
		if len(reasons) != 1 || reasons[0] != tc.reason {
			t.Errorf("%s: want one refused child %q, got %v", tc.name, tc.reason, reasons)
		}
	}
	// A reader that names the file it read in its own spelling still closes the cycle.
	folded := foldingReader{fakeReader{"/ws/t.sh": {Text: "source /ws/T.sh\n"}}}
	done := make(chan Result, 1)
	go func() {
		done <- ParseScripts(context.Background(), "bash /ws/t.sh", POSIX, ScriptOptions{Reader: folded, Context: provenPOSIX, Cwd: "/ws"})
	}()
	select {
	case res := <-done:
		if got := firstScriptReason(res); got != "script-cycle in /ws/t.sh:1" {
			t.Errorf("respelled self source: got %q", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("respelled self source: parse did not finish")
	}
	// One level less than the bound still expands.
	deep["/ws/s2.sh"] = ScriptRead{Text: "echo end\n"}
	for _, inv := range parseWithin(t, "bash /ws/t.sh", deep).Invocations {
		if !inv.Inspectable || inv.Script == nil || inv.Script.Path != "/ws/s2.sh" {
			t.Errorf("source chain within the bound: got %+v", inv)
		}
	}
}

func TestScriptLegacyReasonNeverLeaksScriptText(t *testing.T) {
	got := scriptReasons(t, "env -x git status\n", nil)
	if len(got) != 1 || got[0] != "script-opaque:unsupported-syntax in /ws/t.sh:1" {
		t.Fatalf("got %v", got)
	}
	for _, word := range []string{"env", "-x", "git", "status", "wrapper"} {
		if strings.Contains(got[0], word) {
			t.Fatalf("reason %q carries script text %q", got[0], word)
		}
	}
}

// Only a refusal of a script call keeps its named reason; a named rule on any other executable is not trusted to be
// content-free and is flattened like the rest.
func TestScriptOpacityRelabelsOnlyScriptCallRefusals(t *testing.T) {
	p := parser{}
	origin := &ScriptOrigin{Path: "/ws/t.sh"}
	word := func(s string) Word { return Word{Text: s, Raw: s, Literal: true} }
	p.result.Invocations = []Invocation{
		{Words: []Word{word("bash"), word("/ws/x.sh")}, Reason: ruleRefusal(RuleUnreadable, nil), Script: origin},
		{Words: []Word{word("source"), word("/ws/x.sh")}, Reason: ruleRefusal(RuleTooLarge, nil), Script: origin},
		{Words: []Word{word("."), word("/ws/x.sh")}, Reason: ruleRefusal(RuleCwdUnproven, nil), Script: origin},
		{Words: []Word{word("git"), word("status")}, Reason: ruleRefusal(RuleUnreadable, nil), Script: origin},
	}
	p.markScriptOpacity(0)
	want := []string{
		"script-unreadable in /ws/t.sh",
		"script-too-large in /ws/t.sh",
		"script-cwd-unproven in /ws/t.sh",
		"script-opaque:unsupported-syntax in /ws/t.sh",
	}
	for i, inv := range p.result.Invocations {
		if inv.Reason.String() != want[i] {
			t.Errorf("invocation %d: reason %q, want %q", i, inv.Reason, want[i])
		}
	}
}

// The replay passes over a refused sh or bash script call, which runs in a shell of its own, but not over another
// refused command whose reason happens to start with script-: that one may be a cd the parser could not take apart.
func TestScriptCwdReplayTrustsOnlyScriptCallRefusals(t *testing.T) {
	word := func(s string) Word { return Word{Text: s, Raw: s, Literal: true} }
	cases := []struct {
		words  []Word
		reason Refusal
		proven bool
	}{
		{[]Word{word("bash"), word("/ws/x.sh")}, ruleRefusal(RuleUnreadable, &ScriptOrigin{Path: "/ws/t.sh"}), true},
		{[]Word{word("command"), word("-p"), word("cd"), word("sub")}, opaqueRefusal(constructUnsupportedSyntax, &ScriptOrigin{Path: "/ws/lib.sh"}), false},
	}
	for _, tc := range cases {
		p := parser{script: scriptState{opts: ScriptOptions{Cwd: "/ws"}}}
		p.result.Invocations = []Invocation{{Words: tc.words, Reason: tc.reason, Dialect: POSIX}}
		p.lists = []commandList{{segments: []segmentMark{{before: Start, first: 0, end: 1}, {before: Sequence, first: 1, end: -1}}}}
		if _, ok, byCD := p.cwdBefore(); ok != tc.proven || byCD == tc.proven {
			t.Errorf("%v with reason %q: proven %v byCD %v, want proven %v", tc.words, tc.reason, ok, byCD, tc.proven)
		}
	}
}

func TestScriptBenignContentIsNotMarked(t *testing.T) {
	for _, body := range []string{"echo hi\n", "git status\n", "if true; then echo y; fi\n", "x=1\necho \"$x\"\n", "# comment\n", "f() { echo a; }\nf\n"} {
		if got := scriptReasons(t, body, nil); len(got) != 0 {
			t.Errorf("%q: unexpected marks %v", body, got)
		}
	}
}

func TestScriptLiteralNestedCallsRecurse(t *testing.T) {
	extra := fakeReader{
		"/ws/inner.sh": {Text: "eval \"$x\"\n"},
		"/ws/lib.sh":   {Text: "echo lib\n"},
	}
	got := scriptReasons(t, "bash /ws/inner.sh\nsource /ws/lib.sh\n", extra)
	if len(got) != 1 || got[0] != "script-opaque:eval in /ws/inner.sh:1 (via /ws/t.sh)" {
		t.Fatalf("got %v", got)
	}
	files := fakeReader{"/ws/t.sh": {Text: "bash /ws/inner.sh\nsource /ws/lib.sh\n"}}
	for k, v := range extra {
		files[k] = v
	}
	res := parseScript("bash /ws/t.sh", files)
	var lib *Invocation
	for i := range res.Invocations {
		if inv := &res.Invocations[i]; len(inv.Words) > 0 && inv.Words[0].Text == "echo" {
			lib = inv
		}
	}
	if lib == nil || lib.Script == nil || lib.Script.Path != "/ws/lib.sh" || !lib.Inspectable {
		t.Fatalf("sourced child missing or untagged: %+v", res.Invocations)
	}
	// A sourced file runs in the same shell, so it adds no wrapper, and WrapperIDs must stay aligned with Wrappers.
	if len(lib.Wrappers) != 1 || len(lib.WrapperIDs) != len(lib.Wrappers) {
		t.Fatalf("sourced child wrappers %+v ids %v", lib.Wrappers, lib.WrapperIDs)
	}
	for _, body := range []string{"bash /ws/nope.sh\n", "source /ws/nope.sh\n", ". /ws/nope.sh\n"} {
		missing := scriptReasons(t, body, nil)
		if len(missing) != 1 || missing[0] != "script-unreadable in /ws/t.sh:1" {
			t.Errorf("%q: nested refusal not relabeled: %v", body, missing)
		}
	}
}

func TestScriptRelativeSourceResolvesInProvenCwd(t *testing.T) {
	for _, body := range []string{"source ./lib.sh\n", ". ./lib.sh\n", "cd sub && source ./lib.sh\n"} {
		files := fakeReader{"/ws/t.sh": {Text: body}, "/ws/lib.sh": {Text: "echo ws\n"}, "/ws/sub/lib.sh": {Text: "echo sub\n"}}
		res := parseScript("bash /ws/t.sh", files)
		want := "/ws/lib.sh"
		if strings.HasPrefix(body, "cd sub") {
			want = "/ws/sub/lib.sh"
		}
		found := false
		for _, inv := range res.Invocations {
			if !inv.Inspectable {
				t.Errorf("%q: refused child %q", body, inv.Reason)
			}
			found = found || (inv.Script != nil && inv.Script.Path == want && inv.Words[0].Text == "echo")
		}
		if !found {
			t.Errorf("%q: no child read from %s: %+v", body, want, res.Invocations)
		}
	}
}

// A sourced or evaluated file runs in this shell and may change its directory, so a relative call after it is not
// proven, while a relative call inside the sourced file still resolves from the directory the source ran in.
func TestScriptCallAfterSourceIsNotProven(t *testing.T) {
	files := fakeReader{
		"/ws/a.sh":      {Text: "cd sub\n"},
		"/ws/call.sh":   {Text: "bash ./ok.sh\n"},
		"/ws/ok.sh":     {Text: "echo ws\n"},
		"/ws/sub/ok.sh": {Text: "echo sub\n"},
	}
	cases := []struct{ body, wantPath, wantReason string }{
		{"source ./a.sh\nbash ./ok.sh\n", "", "script-cwd-unproven in /ws/t.sh:2"},
		{". ./a.sh\nbash ./ok.sh\n", "", "script-cwd-unproven in /ws/t.sh:2"},
		{"source /ws/nope.sh\nbash ./ok.sh\n", "", "script-cwd-unproven in /ws/t.sh:2"},
		{"source \"$d/x.sh\"\nbash ./ok.sh\n", "", "script-cwd-unproven in /ws/t.sh:2"},
		{"eval \"$x\"\nbash ./ok.sh\n", "", "script-cwd-unproven in /ws/t.sh:2"},
		{"source ./call.sh\n", "/ws/ok.sh", ""},
		{"cd sub && source ../call.sh\n", "/ws/sub/ok.sh", ""},
	}
	for _, tc := range cases {
		files["/ws/t.sh"] = ScriptRead{Text: tc.body}
		res := parseScriptOn(spellingHost, "bash /ws/t.sh", files)
		var reasons []string
		read := false
		for _, inv := range res.Invocations {
			if !inv.Inspectable {
				reasons = append(reasons, inv.Reason.String())
			}
			read = read || (inv.Script != nil && inv.Script.Path == tc.wantPath)
		}
		for _, rec := range res.Scripts {
			if tc.wantPath == "" && strings.HasSuffix(rec.Path, "/ok.sh") {
				t.Errorf("%q: read %s after a source", tc.body, rec.Path)
			}
		}
		if tc.wantReason != "" {
			found := false
			for _, r := range reasons {
				found = found || r == tc.wantReason
			}
			if !found {
				t.Errorf("%q: want reason %q, got %v", tc.body, tc.wantReason, reasons)
			}
			continue
		}
		if !read || len(reasons) != 0 {
			t.Errorf("%q: want a child read from %s, got reasons %v", tc.body, tc.wantPath, reasons)
		}
	}
}
