package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestHeredocAndNuGetProviderCoverage(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("AGX_RELEASE_RECORD", "")
			t.Setenv("RELEASE_GUARD_LOG_DIR", filepath.Join(root, "logs"))
			for _, tc := range []struct{ hook, command, reason string }{
				{"release-guardian", "# cat <<EOF\nkubectl apply -f deployment.yaml\nEOF", "kubernetes_deploy"},
				{"release-guardian", "echo harmless;# cat <<EOF\nhelm upgrade app ./chart\nEOF", "helm_deploy"},
				{"release-guardian", "echo \\x # cat <<EOF\nterraform apply\nEOF", "terraform_deploy"},
				{"command-guardian", "# cat <<EOF\ndotnet nuget push package.nupkg\nEOF", "NuGet package publication"},
				{"command-guardian", `dotnet nu$(printf get) push package.nupkg`, "non-literal NuGet operation"},
				{"command-guardian", `nuget pu$(printf sh) package.nupkg`, "non-literal NuGet operation"},
				{"command-guardian", `nuget.exe pus package.nupkg`, "NuGet package publication"},
				{"release-guardian", "cat <<'EOF'\nkubectl apply -f deployment.yaml\nEOF", ""},
				{"release-guardian", "cat '#' <<'EOF'\nkubectl apply -f deployment.yaml\nEOF", ""},
				{"release-guardian", "cat \\# <<'EOF'\nkubectl apply -f deployment.yaml\nEOF", ""},
				{"release-guardian", "cat x# <<'EOF'\nkubectl apply -f deployment.yaml\nEOF", ""},
				{"release-guardian", "cat \\ # <<'EOF'\nkubectl apply -f deployment.yaml\nEOF", ""},
				{"release-guardian", "cat <<'EOF' # quoted body\nkubectl apply -f deployment.yaml\nEOF", ""},
				{"command-guardian", "cat <<'EOF'\ndotnet nuget push package.nupkg\nEOF", ""},
			} {
				t.Run(tc.command, func(t *testing.T) {
					payload := secretsShellStdin(surface, root, "Bash", tc.command)
					code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, tc.hook)
					if tc.reason == "" {
						requireSilent(t, surface, code, out, diagnostic)
						return
					}
					requireDeny(t, surface, code, out, diagnostic)
					if !strings.Contains(out+diagnostic, tc.reason) {
						t.Fatalf("wrong denial: %s %s", out, diagnostic)
					}
				})
			}
		})
	}
}
