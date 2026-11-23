package commandguard

import (
	"strings"
	"testing"
)

func TestHeredocCommentCannotHideCommand(t *testing.T) {
	for _, tool := range []string{"Bash", "PowerShell", "mcp__shell__PowerShell"} {
		for _, prefix := range []string{"# cat <<EOF", "echo safe # tee <<'EOF'"} {
			t.Run(tool+"/"+prefix, func(t *testing.T) {
				tc := tableCase{tool: tool, command: prefix + "\nFormat-Volume -DriveLetter Z\n# EOF", want: "deny"}
				got := recordCase(t, tc)
				assertCaseSemantics(t, tc, got)
				if !strings.Contains(got.Outcome.Payload(), "Do not format a volume") {
					t.Fatalf("wrong denial: %s", got.Outcome)
				}
			})
		}
	}
}

func TestHeredocExemptionRequiresBash(t *testing.T) {
	for _, tool := range []string{"Bash", "PowerShell", "mcp__shell__PowerShell"} {
		t.Run(tool, func(t *testing.T) {
			want := "deny"
			if tool == "Bash" {
				want = "pass"
			}
			tc := tableCase{tool: tool, command: "cat <<'EOF'\nFormat-Volume -DriveLetter Z\nEOF", want: want}
			assertCaseSemantics(t, tc, recordCase(t, tc))
		})
	}
}
