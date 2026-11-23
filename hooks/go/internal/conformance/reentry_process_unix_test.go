//go:build !windows

package conformance

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func runReentryProcess(t *testing.T, cmd *exec.Cmd, deadline time.Duration) ([]byte, error, bool) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pgid := cmd.Process.Pid
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	var waitErr error
	timedOut := false
	select {
	case waitErr = <-done:
	case <-timer.C:
		timedOut = true
	}
	// Reap unexpected orphans even after the fixture leader exits normally.
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("kill fixture process group: %v", err)
	}
	if timedOut {
		select {
		case waitErr = <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("fixture leader did not terminate")
		}
	}
	limit := time.Now().Add(5 * time.Second)
	for {
		err := syscall.Kill(-pgid, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		// Darwin can report EPERM while killed group members await reaping.
		// Only ESRCH proves the group is gone; keep the bounded wait for EPERM.
		if err != nil && !errors.Is(err, syscall.EPERM) {
			t.Fatalf("check fixture process group: %v", err)
		}
		if time.Now().After(limit) {
			t.Fatalf("fixture process group %d still exists after cleanup", pgid)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Logf("fixture process group %d terminated (deadline=%t)", pgid, timedOut)
	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	return data, waitErr, timedOut
}
