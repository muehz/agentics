package hookutil

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
)

// TestAppendOwnedLogCreatesPrivateFile pins the happy path: a new directory
// is created 0700, a new log 0600, and successive appends accumulate lines.
func TestAppendOwnedLogCreatesPrivateFile(t *testing.T) {
	trusted := t.TempDir()
	dir := filepath.Join(trusted, ".agentics", "logs", "x")
	for _, line := range []string{"one\n", "two\n"} {
		if err := appendOwnedLog(trusted, dir, "guard.log", []byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, "guard.log"))
	if string(data) != "one\ntwo\n" {
		t.Fatalf("log = %q", data)
	}
	if runtime.GOOS == "windows" {
		return
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, "guard.log"): 0o600} {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != want {
			t.Errorf("%s mode %v, want %v", path, info.Mode().Perm(), want)
		}
	}
}

// TestAppendOwnedLogTightensExistingMode pins the retrofit case: a log that
// already exists with wider bits is tightened to 0600 on append.
func TestAppendOwnedLogTightensExistingMode(t *testing.T) {
	trusted := t.TempDir()
	dir := filepath.Join(trusted, "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "guard.log")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := appendOwnedLog(trusted, dir, "guard.log", []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "old\nnew\n" {
		t.Fatalf("log = %q", data)
	}
	if runtime.GOOS == "windows" {
		return
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

// TestAppendOwnedLogRefusesSymlinkLeaf pins the leaf check: a leaf that is a
// symlink is refused and the link target is left untouched.
func TestAppendOwnedLogRefusesSymlinkLeaf(t *testing.T) {
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

	if err := appendOwnedLog(trusted, dir, "scan.log", []byte("line\n")); !errors.Is(err, ErrLogAlias) {
		t.Fatalf("appendOwnedLog = %v, want ErrLogAlias", err)
	}
	data, _ := os.ReadFile(victim)
	if string(data) != "original\n" {
		t.Fatalf("victim changed: %q", data)
	}
}

// TestAppendOwnedLogRefusesSymlinkedComponent pins the component walk: a
// symlinked directory between trusted and dir is refused before any write.
func TestAppendOwnedLogRefusesSymlinkedComponent(t *testing.T) {
	trusted := t.TempDir()
	outside := t.TempDir()
	hookutiltest.SymlinkOrSkip(t, outside, filepath.Join(trusted, "logs"))
	dir := filepath.Join(trusted, "logs", "secrets")

	if err := appendOwnedLog(trusted, dir, "scan.log", []byte("line\n")); !errors.Is(err, ErrLogAlias) {
		t.Fatalf("appendOwnedLog = %v, want ErrLogAlias", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "secrets", "scan.log")); !os.IsNotExist(err) {
		t.Fatalf("a write through the parent link would have landed outside trusted (err=%v)", err)
	}
}

// TestAppendOwnedLogRefusesJunctionComponent is Windows-only: a junction
// component between trusted and dir is refused the same way a symlink is.
func TestAppendOwnedLogRefusesJunctionComponent(t *testing.T) {
	trusted := t.TempDir()
	outside := t.TempDir()
	hookutiltest.JunctionOrSkip(t, outside, filepath.Join(trusted, "logs"))
	dir := filepath.Join(trusted, "logs", "secrets")

	if err := appendOwnedLog(trusted, dir, "scan.log", []byte("line\n")); !errors.Is(err, ErrLogAlias) {
		t.Fatalf("appendOwnedLog = %v, want ErrLogAlias", err)
	}
}

// TestAppendOwnedLogOutsideTrustedAppends pins the untrusted-dir path: a
// dir outside trusted still appends, created and opened through its own
// os.Root rather than trusted's.
func TestAppendOwnedLogOutsideTrustedAppends(t *testing.T) {
	trusted := t.TempDir()
	outside := filepath.Join(t.TempDir(), "nested", "logs")

	if err := appendOwnedLog(trusted, outside, "scan.log", []byte("line\n")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outside, "scan.log"))
	if err != nil || string(data) != "line\n" {
		t.Fatalf("log = %q, err %v", data, err)
	}
}

// TestAppendOwnedLogRefusesInRootSymlinkLeafAfterCheck drives the post-open
// check directly: it swaps the leaf for a link to another file inside the
// root between ownedLogPath's refusal and root.OpenFile, using the
// afterOwnedCheck seam. os.Root would otherwise follow that link since its
// target stays inside the root; the post-open SameFile/IsRegular check must
// still refuse it, and the link target must be left unchanged.
func TestAppendOwnedLogRefusesInRootSymlinkLeafAfterCheck(t *testing.T) {
	trusted := t.TempDir()
	dir := filepath.Join(trusted, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "other.log")
	if err := os.WriteFile(target, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Join(dir, "guard.log")

	prev := afterOwnedCheck
	t.Cleanup(func() { afterOwnedCheck = prev })
	afterOwnedCheck = func() {
		hookutiltest.SymlinkOrSkip(t, target, leaf)
	}

	err := appendOwnedLog(trusted, dir, "guard.log", []byte("line\n"))
	if !errors.Is(err, ErrLogAlias) {
		t.Fatalf("appendOwnedLog = %v, want ErrLogAlias", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "keep\n" {
		t.Fatalf("target = %q, err %v, want unchanged", data, err)
	}
}
