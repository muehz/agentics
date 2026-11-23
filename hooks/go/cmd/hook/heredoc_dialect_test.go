package main

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestHeredocCommentProviderCoverage(t *testing.T) {
	for _, surface := range surface.All() {
		for _, tool := range []string{"Bash", "PowerShell"} {
			t.Run(string(surface)+"/"+tool, func(t *testing.T) {
				command := "# cat <<EOF\nFormat-Volume -DriveLetter Z\n# EOF"
				if (surface == "codex" || surface == "antigravity") && tool == "PowerShell" {
					tool, command = "Bash", "pwsh -Command '"+command+"'"
				}
				payload := secretsShellStdin(surface, t.TempDir(), tool, command)
				code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "command-guardian")
				requireDeny(t, surface, code, out, diagnostic)
				if !strings.Contains(out+diagnostic, "Do not format a volume") {
					t.Fatalf("wrong denial: %s %s", out, diagnostic)
				}
			})
		}
	}
}
