package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDispatcher(t *testing.T, content string) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "hooks", "run-hook.cmd"), []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestASCIIPassesCleanDispatcher(t *testing.T) {
	repo := writeDispatcher(t, "@echo off\nREM plain ASCII only - no traps here\nexit /b 0\n")
	failures, err := runASCII(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 0 {
		t.Errorf("clean dispatcher flagged: %v", failures)
	}
}

func TestASCIIBitesOnMultibyte(t *testing.T) {
	// The exact byte sequence that broke windows-latest: a UTF-8 em-dash in a
	// REM line.
	repo := writeDispatcher(t, "@echo off\nREM the fast path — no bash needed\nexit /b 0\n")
	failures, err := runASCII(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 || failures[0].Rule != "run-hook-ascii" {
		t.Fatalf("failures = %v, want one run-hook-ascii", failures)
	}
	if failures[0].Path != "hooks/run-hook.cmd" {
		t.Errorf("path = %q, want forward-slash hooks/run-hook.cmd", failures[0].Path)
	}
}

func TestASCIIErrorsWhenDispatcherMissing(t *testing.T) {
	if _, err := runASCII(t.TempDir()); err == nil {
		t.Error("missing dispatcher must be an audit error, not a pass")
	}
}
