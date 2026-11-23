package main

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestReleaseSubstitutionAndANSIOnProviders(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("AGX_RELEASE_RECORD", "")
			for _, tc := range []struct{ command, reason string }{
				{`terraform -chdir=$(pwd)/infra apply`, "non-literal"},
				{`kubectl $(printf apply) -f prod.yaml`, "non-literal"},
				{`k$()ubectl apply`, "script-opaque:command-position-expansion"},
				{`$'\x6bubectl' apply`, "kubectl apply is a production-shaped deploy command"},
				{`echo $'it\'s; $(kubectl apply)'`, ""},
				{"cat <<EOF\nkubectl apply $(date)\nEOF\n", ""},
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
