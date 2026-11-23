package hookutiltest

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSymlinkOrSkipCreatesALink is the smoke test: on a host that allows a
// symlink, the link resolves back to the target.
func TestSymlinkOrSkipCreatesALink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	SymlinkOrSkip(t, target, link)

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat(%q): %v", link, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%q is not a symlink: %v", link, info.Mode())
	}
}

// TestJunctionOrSkipCreatesAJunction is the smoke test: on Windows with
// cmd.exe available, the junction resolves to the target's content and
// Lstat reports it as neither a plain file nor a plain directory (Go 1.23+
// reports ModeIrregular for a junction, not ModeSymlink).
func TestJunctionOrSkipCreatesAJunction(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "marker"), []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "junction")
	JunctionOrSkip(t, target, link)

	if _, err := os.Stat(filepath.Join(link, "marker")); err != nil {
		t.Fatalf("the junction did not resolve to the target's content: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat(%q): %v", link, err)
	}
	if info.Mode()&os.ModeIrregular == 0 {
		t.Fatalf("%q was not reported as irregular: %v", link, info.Mode())
	}
}
