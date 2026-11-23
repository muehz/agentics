package commandguard

import "testing"

// TestGhFlagExpressionsUnchanged pins the matcher text of every gh flag byte
// for byte, so a change to ghTruthyValues that alters the joined alternation
// fails here rather than silently changing which spellings the rules accept.
func TestGhFlagExpressionsUnchanged(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"ghHelpFlags", ghHelpFlags.String(), `(^|[[:space:]])(--help|-h)(([[:space:]]|"|$)|=(true|True|TRUE|t|T|1)([[:space:]]|"|$))`},
		{"ghDraftFlags", ghDraftFlags.String(), `(^|[[:space:]])(--draft|-d)(([[:space:]]|"|$)|=(true|True|TRUE|t|T|1)([[:space:]]|"|$))`},
		{"ghAutoFlag", ghAutoFlag.String(), `(^|[[:space:]])(--auto)(([[:space:]]|"|$)|=(true|True|TRUE|t|T|1)([[:space:]]|"|$))`},
		{"ghOpenFlag", ghOpenFlag.String(), `(^|[[:space:]])(--open)(([[:space:]]|"|$)|=(true|True|TRUE|t|T|1)([[:space:]]|"|$))`},
		{"ghUndoFlag", ghUndoFlag.String(), `(^|[[:space:]])(--undo)(([[:space:]]|"|$)|=(true|True|TRUE|t|T|1)([[:space:]]|"|$))`},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestPrReadyUndoConvertsToDraft: --undo converts a pull request back to a
// draft, the direction the rule exists to allow. Only an effective flag
// counts: after `--` it is positional, `=false` is not the flag, and a
// quoted spelling is data.
func TestPrReadyUndoConvertsToDraft(t *testing.T) {
	ready := "gh pr " + "ready"
	cases := []struct {
		command, want string
	}{
		{ready + " --undo", "pass"},
		{ready + " 123 --undo", "pass"},
		{ready + " --undo 123", "pass"},
		{ready + " --undo=true", "pass"},
		{ready, "deny"},
		{ready + " 123", "deny"},
		{ready + " -- --undo", "deny"},
		{ready + " --undo=false", "deny"},
		{ready + ` "--undo"`, "deny"},
		// Conflicting --undo occurrences deny even though ghSegmentHasEffectiveFlag
		// finds a truthy occurrence: pflag's last-wins semantics mean these
		// commands actually publish the pull request, and conflicting safety
		// flags do not grant exceptions
		// (hooks/README.md, Command-guardian overview).
		{ready + " --undo --undo=false", "deny"},
		{ready + " --undo=false --undo", "deny"},
		{ready + " --undo --undo=maybe", "deny"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			outcome, _, _ := runGo(t, tableCase{name: "ready", command: tc.command})
			if outcome.Kind() != tc.want {
				t.Fatalf("%s: kind = %q, want %q (%s)", tc.command, outcome.Kind(), tc.want, outcome.Payload())
			}
		})
	}
}

// TestPrReadyBehindAGlobalFlagStillGates: a global flag between gh and the
// subcommand (--repo/-R here) defeats the regex rule's adjacency
// requirement, so the invocation-walking path must own the ready rule for
// this spelling; the plain spelling still resolves through the regex rule
// alone (TestPrReadyUndoConvertsToDraft), so this does not double-report.
func TestPrReadyBehindAGlobalFlagStillGates(t *testing.T) {
	ready := "pr " + "ready"
	cases := []struct {
		command, want string
	}{
		{"gh -R o/r " + ready, "deny"},
		{"gh --repo o/r " + ready + " --undo", "pass"},
		{"gh -R o/r " + ready + " --undo=false", "deny"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			outcome, _, _ := runGo(t, tableCase{name: "ready-flag", command: tc.command})
			if outcome.Kind() != tc.want {
				t.Fatalf("%s: kind = %q, want %q (%s)", tc.command, outcome.Kind(), tc.want, outcome.Payload())
			}
		})
	}
}

// TestGhGlobalHelpBeforeSubcommandIsHelp: a global --help/-h that precedes
// the subcommand name is a help request for that subcommand (gh's cobra
// parsing resolves the target command first, then honors --help for it), so
// the invocation-walking path must treat it the same as a --help found among
// the subcommand's own args. Without this, a global flag like -R that
// forces the walking path (TestPrReadyBehindAGlobalFlagStillGates) hides the
// help exemption and denies a command that never mutates anything.
func TestGhGlobalHelpBeforeSubcommandIsHelp(t *testing.T) {
	cases := []struct {
		command, want string
	}{
		{"gh --help -R o/r pr ready", "pass"},
		{"gh -h -R o/r pr ready", "pass"},
		// The subcommand's own args still deny when no help flag is present
		// anywhere, global or local, so this isn't a blanket -R pass.
		{"gh -R o/r pr ready", "deny"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			outcome, _, _ := runGo(t, tableCase{name: "gh-global-help", command: tc.command})
			if outcome.Kind() != tc.want {
				t.Fatalf("%s: kind = %q, want %q (%s)", tc.command, outcome.Kind(), tc.want, outcome.Payload())
			}
		})
	}
}

// TestGhBooleanFlagsAcceptFullPflagVocabularyBehindAGlobalFlag: the
// invocation-walking path (forced here by -R, which defeats the regex
// rules' gh-then-subcommand adjacency) must accept every spelling pflag
// parses as true or false for a bool flag, not only lowercase true/false —
// the same vocabulary the regex-based `gh pr ready` rule already accepts
// (TestPrReadyUndoConvertsToDraft's "--undo=true" case, and ghUndoTruthyValue).
// Swept across the gh boolean flag the walking path parses for pr ready:
// --undo.
func TestGhBooleanFlagsAcceptFullPflagVocabularyBehindAGlobalFlag(t *testing.T) {
	cases := []struct {
		command, want string
	}{
		{"gh -R o/r pr ready --undo=TRUE", "pass"},
		{"gh -R o/r pr ready --undo=True", "pass"},
		{"gh -R o/r pr ready --undo=t", "pass"},
		{"gh -R o/r pr ready --undo=T", "pass"},
		{"gh -R o/r pr ready --undo=1", "pass"},
		{"gh -R o/r pr ready --undo=FALSE", "deny"},
		{"gh -R o/r pr ready --undo=0", "deny"},
		{"gh -R o/r pr ready --undo=maybe", "deny"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			outcome, _, _ := runGo(t, tableCase{name: "gh-bool-vocab", command: tc.command})
			if outcome.Kind() != tc.want {
				t.Fatalf("%s: kind = %q, want %q (%s)", tc.command, outcome.Kind(), tc.want, outcome.Payload())
			}
		})
	}
}

func TestHeredocStrippersReturnInputWithoutMarker(t *testing.T) {
	for _, text := range []string{"", `Bash{"command":"echo hi"}`, `cat file\nls`, "printf '<' '>'"} {
		if got := stripHeredocsFor(text, "", nil, true); got != text {
			t.Fatalf("stripHeredocsFor(%q, \"\", nil, true) = %q", text, got)
		}
	}
}
