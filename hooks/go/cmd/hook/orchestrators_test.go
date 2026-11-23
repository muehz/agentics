package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestOrchestratorProviderCoverage(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("AGX_RELEASE_RECORD", "")
			t.Setenv("RELEASE_GUARD_LOG_DIR", filepath.Join(root, "logs"))
			for _, tc := range []struct{ command, category string }{
				{"terragrunt run --all -- apply", "terragrunt_deploy"},
				{"terragrunt stack run destroy", "terragrunt_deploy"},
				{"terragrunt run-all apply", "terragrunt_deploy"},
				{"helmfile -f prod.yaml sync", "helmfile_deploy"},
				{"helmfile --interactive destroy", "helmfile_deploy"},
				{"terragrunt run --all plan", ""}, {"helmfile diff", ""},
			} {
				for _, shell := range []string{"Bash", "PowerShell"} {
					tool, command := shell, tc.command
					if (surface == "codex" || surface == "antigravity") && shell == "PowerShell" {
						tool, command = "Bash", "pwsh -Command '"+command+"'"
					}
					payload := secretsShellStdin(surface, root, tool, command)
					code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "release-guardian")
					if tc.category == "" {
						requireSilent(t, surface, code, out, diagnostic)
						continue
					}
					requireDeny(t, surface, code, out, diagnostic)
					if !strings.Contains(out+diagnostic, tc.category) {
						t.Fatalf("%s: wrong denial: %s %s", command, out, diagnostic)
					}
				}
			}
			record := filepath.Join(root, "change.md")
			if err := os.WriteFile(record, []byte("# Change Record: orchestrator verification\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("AGX_RELEASE_RECORD", record)
			for _, executable := range []string{"terragrunt", "helmfile"} {
				payload := secretsShellStdin(surface, root, "Bash", executable+" apply")
				code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "release-guardian")
				requireSilent(t, surface, code, out, diagnostic)
				log, err := os.ReadFile(filepath.Join(root, "logs", "guard.log"))
				if err != nil || !strings.Contains(string(log), `"category":"`+executable+`_deploy"`) {
					t.Fatalf("audit missing for %s: %s (%v)", executable, log, err)
				}
			}
		})
	}
}
