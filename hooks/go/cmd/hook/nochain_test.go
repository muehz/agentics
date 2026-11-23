package main

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec"
)

// TestChainPackageIsGone holds one contract. The hook binary has no chain
// verb, so "chain state" gets the same usage error as any unknown first word:
// exit code ExitUsage, nothing on stdout, and the unknown-provider message plus
// the usage line on stderr.
func TestChainPackageIsGone(t *testing.T) {
	code, stdout, stderr := invoke(t, "", "chain", "state")
	if code != hookspec.ExitUsage {
		t.Errorf("chain state exit = %d, want %d (ExitUsage)", code, hookspec.ExitUsage)
	}
	if stdout != "" {
		t.Errorf("chain state wrote to stdout: %q", stdout)
	}
	for _, want := range []string{`unknown provider "chain"`, usage} {
		if !strings.Contains(stderr, want) {
			t.Errorf("chain state stderr missing %q:\n%s", want, stderr)
		}
	}
}
