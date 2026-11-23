package main

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// TestReleaseEnvironmentFilterOnProviders drives the AGX_RELEASE_ENVIRONMENTS
// filter (#578) across every surface: a deploy that carries a configured
// marker denies without a record, and one that does not carries no marker at
// all in its own invocation passes.
func TestReleaseEnvironmentFilterOnProviders(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("AGX_RELEASE_RECORD", "")
			t.Setenv("AGX_RELEASE_ENVIRONMENTS", "prod-sub,production.tfvars,customer-facing")
			for _, tc := range []struct{ command, want, reason string }{
				{"terraform apply -var-file=production.tfvars", "deny", "terraform apply is a production-shaped deploy command"},
				{"terraform apply -var-file=nonprod-production.tfvars", "pass", ""},
			} {
				t.Run(tc.command, func(t *testing.T) {
					payload := secretsShellStdin(surface, root, "Bash", tc.command)
					code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "release-guardian")
					requireOutcome(t, surface, tc.want, "", code, out, diagnostic)
					if tc.want == "deny" && tc.reason != "" && !strings.Contains(out+diagnostic, tc.reason) {
						t.Fatalf("wrong denial: stdout=%q stderr=%q; want %q", out, diagnostic, tc.reason)
					}
				})
			}
		})
	}
}
