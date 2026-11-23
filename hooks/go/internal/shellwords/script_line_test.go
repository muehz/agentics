package shellwords

import (
	"strings"
	"testing"
)

func lineOfCommand(t *testing.T, body, exe string) int {
	t.Helper()
	res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: body}})
	for _, inv := range res.Invocations {
		if len(inv.Words) > 0 && ExecutableName(inv.Words[0].Text) == exe && inv.Script != nil {
			return inv.Script.Line
		}
	}
	t.Fatalf("no %s child in %+v", exe, res.Invocations)
	return 0
}

func TestScriptLineNumbers(t *testing.T) {
	if got := lineOfCommand(t, "echo a\nrm -rf /\n", "rm"); got != 2 {
		t.Errorf("rm line = %d, want 2", got)
	}
	if got := lineOfCommand(t, "cat <<'EOF'\nrm -rf /\nEOF\nrm -rf /tmp/x\n", "rm"); got != 4 {
		t.Errorf("rm after heredoc line = %d, want 4", got)
	}
	res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: "echo hi\necho hi\necho hi\n"}})
	var lines []int
	for _, inv := range res.Invocations {
		if inv.Script != nil {
			lines = append(lines, inv.Script.Line)
		}
	}
	if len(lines) != 3 || lines[0] != 1 || lines[1] != 2 || lines[2] != 3 {
		t.Errorf("repeated lines mapped to %v", lines)
	}
}

// A quoted heredoc body that spells the very command that follows it must not take the command's line, and the body
// line itself is never reported as a command.
func TestScriptLineSkipsQuotedHeredocBodies(t *testing.T) {
	body := "cat <<'EOF'\nrm -rf /tmp/x\nEOF\nrm -rf /tmp/x\n"
	res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: body}})
	var rmLines []int
	for _, inv := range res.Invocations {
		if len(inv.Words) > 0 && ExecutableName(inv.Words[0].Text) == "rm" {
			rmLines = append(rmLines, inv.Script.Line)
		}
	}
	if len(rmLines) != 1 || rmLines[0] != 4 {
		t.Fatalf("rm children at lines %v, want exactly line 4", rmLines)
	}
	// A body whose first line is the same as a later real command, with a blank and indented lines in between.
	body = "echo start\ncat <<-'EOF'\n\t\trm -rf /tmp/y\n\nEOF\n  rm -rf /tmp/y\n"
	if got := lineOfCommand(t, body, "rm"); got != 6 {
		t.Errorf("rm after an indented heredoc body: line %d, want 6", got)
	}
}

func TestScriptReasonCarriesTheLine(t *testing.T) {
	got := scriptReasons(t, "echo a\necho b\neval \"$x\"\n", nil)
	if len(got) != 1 || got[0] != "script-opaque:eval in /ws/t.sh:3" {
		t.Fatalf("got %v", got)
	}
	got = scriptReasons(t, "echo a\nbash /ws/nope.sh\n", nil)
	if len(got) != 1 || got[0] != "script-unreadable in /ws/t.sh:2" {
		t.Fatalf("refused call: got %v", got)
	}
}

// A nested script's children keep the path and line they were labeled with when that script was expanded, and the
// enclosing pass neither relabels them nor gives two siblings the same line.
func TestScriptNestedChildKeepsItsOwnPathAndLine(t *testing.T) {
	extra := fakeReader{"/ws/inner.sh": {Text: "echo a\neval \"$x\"\necho b\ntrap 'x' EXIT\n"}}
	got := scriptReasons(t, "echo top\nbash /ws/inner.sh\neval \"$y\"\n", extra)
	want := []string{
		"script-opaque:eval in /ws/inner.sh:2 (via /ws/t.sh)",
		"script-opaque:trap in /ws/inner.sh:4 (via /ws/t.sh)",
		"script-opaque:eval in /ws/t.sh:3",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestScriptSiblingChildrenGetDifferentLines(t *testing.T) {
	got := scriptReasons(t, "eval \"$a\"\necho x\neval \"$a\"\n", nil)
	want := []string{"script-opaque:eval in /ws/t.sh:1", "script-opaque:eval in /ws/t.sh:3"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A child whose line cannot be located keeps the form without a line, and labeling twice changes nothing.
func TestScriptUnlocatedLineAndIdempotentMarking(t *testing.T) {
	if line, next := newScriptLines("echo a\n").locate(3, ""); line != 0 || next != 3 {
		t.Errorf("blank raw: line %d next %d, want 0 and 3", line, next)
	}
	if line, _ := newScriptLines("echo a\n").locate(0, "rm -rf /"); line != 0 {
		t.Errorf("absent raw: line %d, want 0", line)
	}
	word := func(s string) Word { return Word{Text: s, Raw: s, Literal: true} }
	origin := &ScriptOrigin{Path: "/ws/t.sh", Chain: []string{"/ws/o.sh"}}
	p := parser{}
	p.result.Invocations = []Invocation{
		{Words: []Word{word("eval"), word("x")}, Inspectable: true, Script: origin},
		{Words: []Word{word("git"), word("status")}, Reason: ruleRefusal(RuleUnreadable, nil), Script: origin},
	}
	for pass := 0; pass < 3; pass++ {
		p.markScriptOpacity(0)
		got := []string{p.result.Invocations[0].Reason.String(), p.result.Invocations[1].Reason.String()}
		want := []string{"script-opaque:eval in /ws/t.sh (via /ws/o.sh)", "script-opaque:unsupported-syntax in /ws/t.sh (via /ws/o.sh)"}
		if got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("pass %d: got %v, want %v", pass, got, want)
		}
	}
}

// linesOf returns the located line of every child of `bash /ws/t.sh` whose executable is exe, in order.
func linesOf(t *testing.T, body, exe string) []int {
	t.Helper()
	res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: body}})
	var lines []int
	for _, inv := range res.Invocations {
		if len(inv.Words) > 0 && ExecutableName(inv.Words[0].Text) == exe && inv.Script != nil {
			lines = append(lines, inv.Script.Line)
		}
	}
	return lines
}

func sameLines(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// The Raw of a continued command has its continuations rewritten, so it is not a substring of the file; the
// command still keeps the line it starts on.
func TestScriptLineOfAContinuedCommand(t *testing.T) {
	got := scriptReasons(t, "echo a\neval \\\n    \"$x\"\n", nil)
	if len(got) != 1 || got[0] != "script-opaque:eval in /ws/t.sh:2" {
		t.Fatalf("continued eval: got %v", got)
	}
	got = scriptReasons(t, "echo a\necho b\ntrap \\\n  'x' \\\n  EXIT\n", nil)
	if len(got) != 1 || got[0] != "script-opaque:trap in /ws/t.sh:3" {
		t.Fatalf("continued trap: got %v", got)
	}
}

// A quoted heredoc body is blanked for the line search even when its introduction holds a parenthesis or a
// backtick, which makes the heredoc view unreliable for any inert proof.
func TestScriptLineSkipsHeredocBodiesBehindAnUnreliableIntroduction(t *testing.T) {
	for name, body := range map[string]string{
		"function opening": "f() { cat <<'EOF'\nrm -rf /tmp/x\nEOF\n}\nrm -rf /tmp/x\n",
		"backtick":         "x=`date`; cat <<'EOF'\nrm -rf /tmp/x\nEOF\nrm -rf /tmp/x\n",
		"substitution":     "echo $(date) && cat <<'EOF'\nrm -rf /tmp/x\nEOF\nrm -rf /tmp/x\n",
	} {
		want := strings.Count(body[:strings.LastIndex(body, "rm -rf")], "\n") + 1
		if got := linesOf(t, body, "rm"); !sameLines(got, []int{want}) {
			t.Errorf("%s: rm at lines %v, want [%d]", name, got, want)
		}
	}
}

// A substitution child is found inside the line of the command that holds it, and the command after it still gets
// its own line.
func TestScriptLineOfSubstitutionChildren(t *testing.T) {
	if got := linesOf(t, "echo $(date)\ndate\n", "date"); !sameLines(got, []int{1, 2}) {
		t.Errorf("date children at %v, want [1 2]", got)
	}
	if got := linesOf(t, "echo $(date)\ndate\n", "echo"); !sameLines(got, []int{1}) {
		t.Errorf("echo at %v, want [1]", got)
	}
	if got := linesOf(t, "echo hi\necho hi\necho hi\n", "echo"); !sameLines(got, []int{1, 2, 3}) {
		t.Errorf("repeated lines at %v", got)
	}
}

// A comment that spells a command never takes the command's line, and a # inside quotes or in the middle of a word
// is not a comment.
func TestScriptLineIgnoresComments(t *testing.T) {
	for name, body := range map[string]string{
		"own line":  "# rm -rf /tmp/x later\nrm -rf /tmp/x\n",
		"trailing":  "echo a # rm -rf /tmp/x\nrm -rf /tmp/x\n",
		"indented":  "  \t# rm -rf /tmp/x\nrm -rf /tmp/x\n",
		"after and": "true && # rm -rf /tmp/x\nrm -rf /tmp/x\n",
	} {
		if got := linesOf(t, body, "rm"); !sameLines(got, []int{2}) {
			t.Errorf("%s: rm at lines %v, want [2]", name, got)
		}
	}
	if got := linesOf(t, "echo \"# not a comment\" x\necho '#' y\necho a#b z\necho ${#v} w\n", "echo"); !sameLines(got, []int{1, 2, 3, 4}) {
		t.Errorf("quoted and mid-word hashes: echo at %v, want [1 2 3 4]", got)
	}
}

// An unquoted heredoc body is not blanked, because its substitutions run, so a body line equal to a later command
// takes that command's line. The line is wrong but still names the script; this pins the conservative behavior.
func TestScriptLineUnquotedHeredocBodyResidual(t *testing.T) {
	if got := linesOf(t, "cat <<EOF\nrm -rf /tmp/x\nEOF\nrm -rf /tmp/x\n", "rm"); !sameLines(got, []int{2}) {
		t.Errorf("rm at lines %v, want the body line [2]", got)
	}
}

func TestScriptLabelFor(t *testing.T) {
	res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: "echo a\nrm -rf /\n"}})
	if label, ok := res.ScriptLabelFor("rm -rf /"); !ok || label != "/ws/t.sh:2" {
		t.Fatalf("got %q %v", label, ok)
	}
	if _, ok := res.ScriptLabelFor("git push"); ok {
		t.Fatal("label for text not in any script")
	}
	if label, _ := res.ScriptLabelFor("rm -rf /"); strings.Contains(label, "rm") {
		t.Fatalf("label leaks content: %q", label)
	}
}

func TestScriptLabelForUsesTheChildAndJSONEscapes(t *testing.T) {
	res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: "echo a\necho a\nrm -rf \"/tmp/x\"\n"}})
	if label, ok := res.ScriptLabelFor(`rm -rf \"/tmp/x\"`); !ok || label != "/ws/t.sh:3" {
		t.Fatalf("JSON-escaped match: %q %v", label, ok)
	}
	if label, ok := res.ScriptLabelFor("rm -rf /tmp/x"); !ok || label != "/ws/t.sh:3" {
		t.Fatalf("unquoted words match: %q %v", label, ok)
	}
	// The scanned text holds a real newline as the two characters backslash and n.
	if label, ok := res.ScriptLabelFor(`echo a\nrm -rf \"/tmp/x\"`); !ok || label != "/ws/t.sh:1" {
		t.Fatalf("escaped newline match: %q %v", label, ok)
	}
}

// The label is built from the rendered path, so a control or bidirectional character in a script path never reaches
// a deny reason, and a nested child's own path is what names its line.
func TestScriptLabelForRendersAHostilePathSafely(t *testing.T) {
	hostile := "/ws/a\x1b[31m\U0000202e.sh"
	res := Result{
		Invocations: []Invocation{{Raw: "rm -rf /tmp/x", Script: &ScriptOrigin{Path: hostile, Line: 7}}},
		Scripts:     []ScriptRecord{{Path: hostile, Text: "echo a\nrm -rf /tmp/x\n"}},
	}
	for _, match := range []string{"rm -rf /tmp/x", "rm -rf /tmp/x\n"} {
		label, ok := res.ScriptLabelFor(match)
		if !ok || strings.ContainsAny(label, "\x1b\U0000202e") || !strings.HasPrefix(label, "/ws/a?[31m?.sh:") {
			t.Fatalf("%q: label %q %v", match, label, ok)
		}
	}
}

// A rule that builds its match from the executable and one operand ("rm /" for "rm -rf /") still finds the child.
func TestScriptLabelForPlacesAMatchComposedFromWords(t *testing.T) {
	res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: "echo a\n/bin/rm -rf \"/tmp/x y\"\n"}})
	for _, match := range []string{`rm "/tmp/x y"`, "rm /tmp/x y", "rm -rf"} {
		if label, ok := res.ScriptLabelFor(match); !ok || label != "/ws/t.sh:2" {
			t.Errorf("%q: got %q %v", match, label, ok)
		}
	}
	for _, match := range []string{"rm /etc", "echo b", "git push"} {
		if label, ok := res.ScriptLabelFor(match); ok {
			t.Errorf("%q: unexpected label %q", match, label)
		}
	}
	unlocated := Result{Invocations: []Invocation{{Words: []Word{{Text: "rm", Raw: "rm"}, {Text: "/", Raw: "/"}}, Script: &ScriptOrigin{Path: "/ws/t.sh"}}}}
	if label, ok := unlocated.ScriptLabelFor("rm /"); !ok || label != "/ws/t.sh" {
		t.Errorf("unlocated child: got %q %v", label, ok)
	}
}

func TestScriptLabelForNestedChildUsesItsOwnPathAndLine(t *testing.T) {
	files := fakeReader{
		"/ws/t.sh":     {Text: "echo top\nbash /ws/inner.sh\n"},
		"/ws/inner.sh": {Text: "echo a\necho b\nrm -rf /tmp/z\n"},
	}
	res := parseScript("bash /ws/t.sh", files)
	if label, ok := res.ScriptLabelFor("rm -rf /tmp/z"); !ok || label != "/ws/inner.sh:3" {
		t.Fatalf("got %q %v", label, ok)
	}
}
