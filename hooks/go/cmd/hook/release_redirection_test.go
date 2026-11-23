package main

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// Issue #443 item 6: leading redirections must not become the executable,
// and their targets must not become deployment verbs.
func TestReleaseLeadingRedirectionsOnProviders(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("AGX_RELEASE_RECORD", "")
			for _, tc := range []struct{ command, reason string }{
				{`> /tmp/log kubectl apply -f prod.yaml`, "kubectl apply is a production-shaped deploy command"},
				{`2>/dev/null kubectl apply -f prod.yaml`, "kubectl apply is a production-shaped deploy command"},
				{`2>&1 kubectl apply -f prod.yaml`, "kubectl apply is a production-shaped deploy command"},
				{`>>'/tmp/deploy log' kubectl apply -f prod.yaml`, "kubectl apply is a production-shaped deploy command"},
				{`0<input.yaml >output.log kubectl apply -f prod.yaml`, "kubectl apply is a production-shaped deploy command"},
				{`>$(printf log) kubectl apply -f prod.yaml`, "kubectl apply is a production-shaped deploy command"},
				{`>log env -- kubectl apply -f prod.yaml`, "kubectl apply is a production-shaped deploy command"},
				{`>apply kubectl get pods`, ""},
				{`2>/dev/null kubectl apply --dry-run=client -f prod.yaml`, ""},
				{`printf '%s' '> /tmp/log kubectl apply'`, ""},
			} {
				t.Run(tc.command, func(t *testing.T) {
					payload := secretsShellStdin(surface, root, "Bash", tc.command)
					code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "release-guardian")
					if tc.reason == "" {
						requireSilent(t, surface, code, out, diagnostic)
						return
					}
					requireDeny(t, surface, code, out, diagnostic)
					if !strings.Contains(out+diagnostic, tc.reason) {
						t.Fatalf("wrong denial: stdout=%q stderr=%q; want %q", out, diagnostic, tc.reason)
					}
				})
			}
		})
	}
}
