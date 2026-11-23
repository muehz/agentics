package hookutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
)

// TestOwnedLogPathRefusesSymlinkLeaf pins the leaf check: a leaf that is a
// symlink anywhere — inside trusted or not — is refused.
func TestOwnedLogPathRefusesSymlinkLeaf(t *testing.T) {
	trusted := t.TempDir()
	dir := filepath.Join(trusted, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hookutiltest.SymlinkOrSkip(t, victim, filepath.Join(dir, "scan.log"))

	if _, err := ownedLogPath(trusted, dir, "scan.log"); !errors.Is(err, ErrLogAlias) {
		t.Fatalf("ownedLogPath = %v, want ErrLogAlias", err)
	}
}

// TestOwnedLogPathRefusesSymlinkedComponentUnderTrusted pins the component
// walk: a symlinked directory between trusted and dir is refused even though
// the leaf itself would be fine.
func TestOwnedLogPathRefusesSymlinkedComponentUnderTrusted(t *testing.T) {
	trusted := t.TempDir()
	outside := t.TempDir()
	hookutiltest.SymlinkOrSkip(t, outside, filepath.Join(trusted, "logs"))
	dir := filepath.Join(trusted, "logs", "secrets")

	if _, err := ownedLogPath(trusted, dir, "scan.log"); !errors.Is(err, ErrLogAlias) {
		t.Fatalf("ownedLogPath = %v, want ErrLogAlias", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "secrets", "scan.log")); !os.IsNotExist(err) {
		t.Fatalf("a write through the parent link would have landed outside trusted (err=%v)", err)
	}
}

// TestOwnedLogPathKeepsUntrustedParentsUnchecked pins the trusted-boundary
// rule: a dir outside trusted keeps its own parents unchecked, even when one
// of them is a symlink.
func TestOwnedLogPathKeepsUntrustedParentsUnchecked(t *testing.T) {
	trusted := t.TempDir()
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	hookutiltest.SymlinkOrSkip(t, real, link)

	got, err := ownedLogPath(trusted, link, "scan.log")
	if err != nil {
		t.Fatalf("ownedLogPath = %v, want no error for an untrusted dir", err)
	}
	want := filepath.Join(link, "scan.log")
	if got != want {
		t.Errorf("ownedLogPath = %q, want %q", got, want)
	}
}

// TestOwnedLogPathAllowsMissingDir pins the not-yet-created case: MkdirAll
// will make a real directory, so a missing component under trusted passes.
func TestOwnedLogPathAllowsMissingDir(t *testing.T) {
	trusted := t.TempDir()
	dir := filepath.Join(trusted, "not", "yet", "created")

	got, err := ownedLogPath(trusted, dir, "scan.log")
	if err != nil {
		t.Fatalf("ownedLogPath = %v, want no error for a dir that does not exist yet", err)
	}
	want := filepath.Join(dir, "scan.log")
	if got != want {
		t.Errorf("ownedLogPath = %q, want %q", got, want)
	}
}

// TestOwnedLogPathRefusesJunctionComponentUnderTrusted is Windows-only: since
// Go 1.23 a junction/mount point reports ModeIrregular rather than
// ModeSymlink, so the component walk must check for both.
func TestOwnedLogPathRefusesJunctionComponentUnderTrusted(t *testing.T) {
	trusted := t.TempDir()
	outside := t.TempDir()
	hookutiltest.JunctionOrSkip(t, outside, filepath.Join(trusted, "logs"))
	dir := filepath.Join(trusted, "logs", "secrets")

	if _, err := ownedLogPath(trusted, dir, "scan.log"); !errors.Is(err, ErrLogAlias) {
		t.Fatalf("ownedLogPath = %v, want ErrLogAlias", err)
	}
}
