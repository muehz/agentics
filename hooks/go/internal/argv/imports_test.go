package argv

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// modulePath is the prefix that marks an import as part of this module.
const modulePath = "github.com/muehz/agentics/hooks/go"

// TestArgvImportsNothingInternal keeps argv at the bottom of the import graph:
// any import of the module's own packages would let a cycle form once the
// guards and shellwords depend on it.
func TestArgvImportsNothingInternal(t *testing.T) {
	if got := readModulePath(t, filepath.Join("..", "..")); got != modulePath {
		t.Fatalf("go.mod declares %q, but the test checks imports against %q", got, modulePath)
	}
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("importing the package: %v", err)
	}
	if len(pkg.IgnoredGoFiles) != 0 {
		t.Errorf("files excluded by build constraints could hide an import: %v", pkg.IgnoredGoFiles)
	}
	for _, imp := range pkg.Imports {
		if imp == modulePath || strings.HasPrefix(imp, modulePath+"/") {
			t.Errorf("argv imports %s", imp)
		}
	}
}

// readModulePath returns the module path that go.mod declares.
func readModulePath(t *testing.T, moduleRoot string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	for line := range strings.Lines(string(data)) {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(path)
		}
	}
	t.Fatal("go.mod has no module line")
	return ""
}
