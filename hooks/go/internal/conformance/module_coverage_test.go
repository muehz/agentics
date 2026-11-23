package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestModuleGatesBiteCoverageFloor removes the tiny module's only test file.
// With -coverpkg every package is still instrumented, so the total drops to
// 0.0% and the floor must fire with the measured number in its message.
func TestModuleGatesBiteCoverageFloor(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	if err := os.Remove(filepath.Join(root, "hooks", "go", "cmd", "hook", "main_test.go")); err != nil {
		t.Fatal(err)
	}
	failures, err := checkTestsAndCoverage(filepath.Join(root, engineRel), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "module-coverage-floor")
	for _, f := range failures {
		if f.Rule == "module-coverage-floor" && !strings.Contains(f.Message, "statement coverage is 0.0%, under the 90% floor") {
			t.Fatalf("module-coverage-floor fired with %q; the message must carry the measured number and the floor", f.Message)
		}
	}
}
