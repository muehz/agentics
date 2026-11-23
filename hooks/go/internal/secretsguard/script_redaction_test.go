package secretsguard

import (
	"context"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// A credential path read inside a script is script content, so the message names the script line instead; a typed
// path is shown as written.
func TestRenderMessageNamesTheScriptLineForAScriptPath(t *testing.T) {
	const marker = "leakmarkerq7"
	origin := &shellwords.ScriptOrigin{Path: "/ws/t.sh", Line: 2}
	got := renderMessage([]finding{{"dotenv", sevCritical, "./" + marker + "/.env", origin}, {"dotenv", sevCritical, "typed/.env", nil}})
	if strings.Contains(got, marker) || !strings.Contains(got, "script content redacted (/ws/t.sh:2)") || !strings.Contains(got, "typed/.env") {
		t.Fatalf("message %q", got)
	}
}

func inspectScriptLine(t *testing.T, line string) shellInspection {
	t.Helper()
	hosttest.ScrubGuardEnv(t)
	call := readCall(t, buildEventAt(t, "Bash", map[string]any{"command": "bash /ws/t.sh"}, "/ws"))
	ctx := context.Background()
	return inspectShellReads(ctx, call, call.Inspect(ctx, hookspectest.Scripts{"/ws/t.sh": "echo a\n" + line + "\n"}))
}

// The parse carries each operand's script line to the finding.
func TestScriptOperandFindingCarriesItsOrigin(t *testing.T) {
	inspection := inspectScriptLine(t, "cat ./x/.env")
	findings, err := collectShell(inspection.Paths, nil, inspection.Overrides)
	if err != nil || len(findings) != 1 || findings[0].origin == nil || findings[0].origin.Location() != "/ws/t.sh:2" {
		t.Fatalf("reason %q findings %+v err %v", inspection.Reason, findings, err)
	}
}

// A glob from a script line that cannot be resolved names only the line: the resolver's error can name a path the
// glob reached.
func TestScriptGlobErrorNamesOnlyTheLine(t *testing.T) {
	inspection := inspectScriptLine(t, "cat ../leakmarkerq7*/.env")
	_, err := collectShell(inspection.Paths, nil, inspection.Overrides)
	if err == nil || err.Error() != "script content redacted (/ws/t.sh:2) (glob could not be resolved)" {
		t.Fatalf("reason %q err %v", inspection.Reason, err)
	}
}

// A bash syntax check inside a script is a nested script call and is refused by its form, so its operand never
// reaches the syntax-read path; if that changes, the path it then takes must carry the line's origin.
func TestScriptSyntaxCheckIsRefusedByForm(t *testing.T) {
	inspection := inspectScriptLine(t, "bash -n ./x/.env")
	if inspection.Reason.String() != "script-form-unsupported-option in /ws/t.sh:2" || len(inspection.Paths) != 0 {
		t.Fatalf("reason %q paths %+v", inspection.Reason, inspection.Paths)
	}
}

// The message leads with the remedy of a named rule and spells every other refusal as one literal. Each expected text
// is written out in full, so a change to any byte fails here.
func TestShellParseMessageCarriesTheScriptHint(t *testing.T) {
	const hintCallerIn = "Do not pass a protected target, a check-bypass flag or an unquoted pattern as an argument to a script that uses its parameters; quote a pattern, and run the command itself when it needs a protected target."
	for _, tc := range []struct {
		name    string
		refusal shellwords.Refusal
		want    string
	}{
		{"named rule", shellwords.CallerArgumentRefusal(shellwords.ScriptCall{Path: "/ws/t.sh"}),
			"secrets-guardian could not inspect a recognized execution path: " + hintCallerIn + " (script-caller-argument in /ws/t.sh)"},
		{"bash authority", shellwords.GuardRefusal(shellwords.GuardBashAuthorityUnavailable),
			"secrets-guardian could not inspect a recognized execution path (Bash inspection authority unavailable)"},
		{"bash syntax operand", shellwords.GuardRefusal(shellwords.GuardBashSyntaxOperandUnavailable),
			"secrets-guardian could not inspect a recognized execution path (Bash syntax file read operand unavailable)"},
		{"search canceled", shellwords.GuardRefusal(shellwords.GuardSearchCanceled),
			"secrets-guardian could not inspect a recognized execution path (search argument inspection canceled)"},
		{"bash canceled", shellwords.GuardRefusal(shellwords.GuardBashCanceled),
			"secrets-guardian could not inspect a recognized execution path (Bash inspection canceled)"},
		{"search brace expansion", shellwords.GuardRefusal(shellwords.GuardSearchBraceExpansion),
			"secrets-guardian could not inspect a recognized execution path (unsupported search brace expansion)"},
	} {
		if got := shellParseMessage(tc.refusal); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
