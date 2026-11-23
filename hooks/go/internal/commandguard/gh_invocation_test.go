package commandguard

import (
	"strings"
	"testing"
)

// TestGhSpellingsTheRegexCannotSee: a quoted executable, a global flag, and
// a .exe suffix all reach gh, so the invocation path gates them with the same
// exemptions the regex rules grant.
func TestGhSpellingsTheRegexCannotSee(t *testing.T) {
	for _, c := range []struct{ command, want string }{
		{`"gh" pr ` + "create --title x", "deny"},
		{`'gh' pr ` + "create --title x", "deny"},
		{`"gh" pr ` + "create --draft --title x", "pass"},
		{`"gh" pr ` + "create -d", "pass"},
		{`"gh" pr ` + "create --draft=false", "deny"},
		{`gh -R o/r pr ` + "create --title x", "deny"},
		{`gh -R o/r pr ` + "create --draft", "pass"},
		{`gh.exe pr ` + "create", "deny"},
		{`"gh" pr ` + "create --help", "pass"},
		{`"gh" stack ` + "submit", "deny"},
		{`"gh" stack ` + "submit --auto", "pass"},
		{`"gh" stack ` + "submit --auto --open", "deny"},
		{`"gh" stack ` + "link --open", "deny"},
		{`"gh" stack ` + "link", "pass"},
		{`"gh" pr ` + "ready 1", "deny"},
	} {
		t.Run(c.command, func(t *testing.T) {
			category := ""
			if c.want == "deny" {
				category = "unsafe_publication"
			}
			assertGhOutcome(t, "Bash", c.command, c.want, category)
		})
	}
}

// TestGhOneThreatPerVerb: the regex rules and the invocation path judge the
// same gh verbs from two readings of one command; Run dedupes by the reported
// verb so one command never carries two threats for one verb.
func TestGhOneThreatPerVerb(t *testing.T) {
	for _, command := range []string{
		"gh pr " + "create", "gh stack " + "submit",
		"gh stack " + "link --open", "gh pr " + "ready 1",
		// Blanking the title leaves the executed head intact, so the regex
		// rule still owns this spelling and the invocation path stays silent.
		"gh pr " + `create --title "gh pr ` + `create"`,
		"gh pr " + "ready 1; gh -R o/r pr " + "ready 2",
		`"gh" pr ` + `create --title "gh pr ` + `create"`,
	} {
		t.Run(command, func(t *testing.T) {
			outcome, _, _ := runGo(t, tableCase{name: "one-threat", command: command})
			if !strings.HasPrefix(outcome.Payload(), "1 blocking threat(s) detected: unsafe_publication") {
				t.Fatalf("%q: payload %q, want exactly one unsafe_publication threat", command, outcome.Payload())
			}
		})
	}
}

// TestGhValueSpellingTheVerbDoesNotHideTheInvocation: a data value that
// spells the verb is blanked for the regex rules, so the invocation path must
// still judge a spelling those rules cannot see.
func TestGhValueSpellingTheVerbDoesNotHideTheInvocation(t *testing.T) {
	for _, c := range []struct{ command, want string }{
		{`"gh" pr ` + `create --title "gh pr ` + `create"`, "deny"},
		{`gh -R o/r pr ` + `create --title "gh pr ` + `create"`, "deny"},
		{`"gh" pr ` + `ready 1 --title "gh pr ` + `ready"`, "deny"},
		{`gh -R o/r stack ` + `submit --body "gh stack ` + `submit"`, "deny"},
		{`gh stack ` + "submit --auto -n --open", "deny"},
		{`gh pr ` + "create --draft --draft=false", "deny"},
		{`"gh" pr ` + `create --draft --title "gh pr ` + `create"`, "pass"},
		{`gh pr ` + `create --draft --title "gh pr ` + `create"`, "pass"},
	} {
		t.Run(c.command, func(t *testing.T) {
			category := ""
			if c.want == "deny" {
				category = "unsafe_publication"
			}
			assertGhOutcome(t, "Bash", c.command, c.want, category)
		})
	}
}
