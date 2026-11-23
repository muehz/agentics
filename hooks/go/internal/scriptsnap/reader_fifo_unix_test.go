//go:build unix

package scriptsnap

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func readWithin(t *testing.T, p string) shellwords.ScriptRead {
	t.Helper()
	done := make(chan shellwords.ScriptRead, 1)
	go func() { done <- New().ReadScript(p) }()
	select {
	case got := <-done:
		return got
	case <-time.After(time.Second):
		t.Fatal("reader blocked")
		return shellwords.ScriptRead{}
	}
}

func TestReaderFifoDoesNotBlock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fifo.sh")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	got := readWithin(t, p)
	if got.Limit != shellwords.RuleUnreadable || got.Detail != "not a regular file" {
		t.Fatalf("%+v", got)
	}
}

// A FIFO swapped in after the Lstat reaches the open call; only a non-blocking open returns.
func TestFifoSwappedInAfterLstatDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.sh")
	if err := os.WriteFile(p, []byte("echo one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	made := false
	setHooks(t, 10*time.Second, func() {
		if os.Remove(p) == nil {
			made = syscall.Mkfifo(p, 0o600) == nil
		}
	}, nil, nil)
	got := readWithin(t, p)
	if !made {
		t.Skip("mkfifo unavailable")
	}
	if got.Limit != shellwords.RuleUnreadable && got.Limit != shellwords.RuleChanging || got.Text != "" {
		t.Fatalf("%+v", got)
	}
}

// A symlink swapped in after the Lstat is refused at the open itself, before any kind check on its target.
func TestSymlinkSwappedInAfterLstatIsNotFollowed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.sh")
	target := filepath.Join(dir, "target.sh")
	for _, f := range []string{p, target} {
		if err := os.WriteFile(f, []byte("echo one\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	setHooks(t, 0, func() {
		if os.Remove(p) == nil {
			_ = os.Symlink(target, p)
		}
	}, nil, nil)
	got := readWithin(t, p)
	if got.Limit != shellwords.RuleUnreadable || got.Detail != "open failed" || got.Text != "" {
		t.Fatalf("%+v", got)
	}
}
