package flow

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestNewRepoUsesGitPhysicalPathThroughLinkedTemp(t *testing.T) {
	// TempDir caches its parent, so create the link before its first call.
	root, err := os.MkdirTemp("", "flow-linked-temp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	realTemp := filepath.Join(root, "physical")
	if err := os.Mkdir(realTemp, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedTemp := filepath.Join(root, "linked")
	if err := os.Symlink(realTemp, linkedTemp); err != nil {
		if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
			t.Skipf("directory symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
	for _, name := range []string{"TMPDIR", "TEMP", "TMP"} {
		t.Setenv(name, linkedTemp)
	}

	repo := newRepo(t)
	physical, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	if repo != physical {
		t.Fatalf("fixture repository = %q, want physical path %q", repo, physical)
	}
	if got := filepath.FromSlash(gitOut(t, repo, "rev-parse", "--show-toplevel")); got != repo {
		t.Fatalf("Git repository = %q, fixture repository = %q", got, repo)
	}
}
