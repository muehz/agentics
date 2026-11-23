package shellwords

import "testing"

// A match that neither the typed call explains nor any search can place is still replaced, by the one inspected
// script's path when there is one and by a fixed name when there are several; only a call with no inspected script,
// or a match the typed call explains, is shown as written. Text that looks like a redaction label is judged like any
// other text, so a script cannot opt out of redaction by spelling one.
func TestRedactorFailsClosed(t *testing.T) {
	one := Result{Scripts: []ScriptRecord{{Path: "/ws/a.sh", Text: "echo a\n"}}}
	two := Result{Scripts: []ScriptRecord{{Path: "/ws/a.sh", Text: "echo a\n"}, {Path: "/ws/b.sh", Text: "echo b\n"}}}
	typed := "bash /ws/a.sh && git status"
	word := func(text string) Word { return Word{Text: text, Raw: text, Literal: true} }
	// A typed invocation whose words a rule spells back together ("rm" and a target) is explained without the typed
	// text holding that spelling; an invocation that came from a script explains nothing.
	typedRm := Result{Scripts: one.Scripts, Invocations: []Invocation{{Words: []Word{word("rm"), word("-rf"), word("/tmp/x")}}}}
	scriptRm := Result{Scripts: one.Scripts, Invocations: []Invocation{{Words: []Word{word("rm"), word("-rf"), word("/tmp/x")},
		Script: &ScriptOrigin{Path: "/ws/a.sh", Line: 1}}}}
	cases := []struct {
		name          string
		redactor      *Redactor
		match         string
		shown, suffix string
	}{
		{"no scripts", NewRedactor(Result{}, typed), "anything", "anything", ""},
		{"unplaceable, one script", NewRedactor(one, typed), "no such text", "script content redacted (/ws/a.sh)", "/ws/a.sh"},
		{"unplaceable, two scripts", NewRedactor(two, typed), "no such text", "script content redacted (inspected script)", "inspected script"},
		{"placeable", NewRedactor(two, typed), "echo b", "script content redacted (/ws/b.sh:1)", "/ws/b.sh:1"},
		{"typed text", NewRedactor(two, typed), "git status", "git status", ""},
		{"typed text with other spacing", NewRedactor(two, typed), "bash /ws/a.sh&&git  status", "bash /ws/a.sh&&git  status", ""},
		// The match comes from the JSON text of the tool input, so its quotes are escaped, while the typed command is not.
		{"escaped quotes of a typed text", NewRedactor(one, `echo "hi"`), `echo \"hi\"`, `echo \"hi\"`, ""},
		{"composed from the words of a typed invocation", NewRedactor(typedRm, typed), "rm /tmp/x", "rm /tmp/x", ""},
		{"words of a script invocation explain nothing", NewRedactor(scriptRm, typed), "rm /tmp/x", "script content redacted (/ws/a.sh:1)", "/ws/a.sh:1"},
		{"label spelling is not trusted", NewRedactor(two, typed), "script content redacted (/ws/b.sh:9)", "script content redacted (inspected script)", "inspected script"},
	}
	for _, tc := range cases {
		shown, label, redacted := tc.redactor.Redact(tc.match)
		if shown != tc.shown || label != tc.suffix || redacted != (tc.suffix != "") {
			t.Errorf("%s: got %q %q %v, want %q %q", tc.name, shown, label, redacted, tc.shown, tc.suffix)
		}
	}
}

// Every refusal the parser emits for these script calls has a remedy, so no new reason silently falls back to the
// generic text.
func TestScriptHintCoversParserRefusals(t *testing.T) {
	cases := map[string]string{
		"bash -x /ws/t.sh":    "",
		"bash /ws/t.sh":       "eval \"$X\"\n",
		"bash /ws/missing.sh": "",
	}
	for command, body := range cases {
		files := fakeReader{"/ws/t.sh": {Text: body}, "/ws/missing.sh": {Limit: RuleUnreadable}}
		for _, inv := range parseScript(command, files).Invocations {
			if !inv.Inspectable && inv.Reason.Hint() == "" {
				t.Errorf("%s: reason %q has no hint", command, inv.Reason)
			}
		}
	}
}
