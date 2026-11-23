package conformance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuditReentry(t *testing.T) {
	if inventory := os.Getenv("AGX_REENTRY_INVENTORY"); inventory != "" {
		f, err := os.OpenFile(inventory, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintf(f, "helper depth=%s pid=%d\n", os.Getenv("AGX_REENTRY_DEPTH"), os.Getpid()); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if os.Getenv("AGX_HOOK_AUDIT_GATE") == "1" {
		return
	}
	if root := os.Getenv("AGX_REENTRY_ROOT"); root != "" {
		failures, err := RunModuleGates(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range failures {
			if f.Rule == "module-test" || f.Rule == "module-coverage-floor" {
				t.Fatalf("%s: %s", f.Rule, f.Message)
			}
		}
		return
	}
	root := cloneTinyRepo(t)
	module := filepath.Join(root, "hooks", "go")
	mainTest := filepath.Join(module, "cmd", "hook", "main_test.go")
	mainData, err := os.ReadFile(mainTest)
	if err != nil {
		t.Fatal(err)
	}
	mainSource := strings.Replace(string(mainData), "\"bytes\"", "\"bytes\"\n\"os\"\n\"fmt\"", 1)
	mainSource = strings.Replace(mainSource, "func TestRun(t *testing.T) {", `func TestRun(t *testing.T) {
 f, err := os.OpenFile(os.Getenv("AGX_REENTRY_INVENTORY"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); if err != nil { t.Fatal(err) }; defer f.Close()
 if _, err := fmt.Fprintf(f, "fixture TestRun pid=%d\n", os.Getpid()); err != nil { t.Fatal(err) }
`, 1)
	if err := os.WriteFile(mainTest, []byte(mainSource), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := `package main
import ("fmt"; "os"; "os/exec"; "testing"; "time")
func TestNestedGate(t *testing.T) {
 f, err := os.OpenFile(os.Getenv("AGX_REENTRY_INVENTORY"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); if err != nil { t.Fatal(err) }; defer f.Close()
 if _, err := fmt.Fprintf(f, "fixture TestNestedGate pid=%d\n", os.Getpid()); err != nil { t.Fatal(err) }
 if os.Getenv("AGX_REENTRY_FORCE_TIMEOUT") == "1" { time.Sleep(time.Minute) }
 if os.Getenv("AGX_REENTRY_DEPTH") == "1" { t.Fatal("REENTRY_DEPTH_SENTINEL: nested gate recursed") }
 cmd := exec.Command(os.Getenv("AGX_REENTRY_EXE"), "-test.run=^TestAuditReentry$", "-test.v")
 cmd.Env = append(os.Environ(), "AGX_REENTRY_DEPTH=1")
 out, err := cmd.CombinedOutput(); if err != nil { t.Fatalf("nested gate: %v\n%s", err, out) }
 if _, err := f.WriteString("nested TestAuditReentry completed\n"); err != nil { t.Fatal(err) }
}
`
	path := filepath.Join(module, "cmd", "hook", "reentry_test.go")
	if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("gofmt", "-w", path, mainTest).CombinedOutput(); err != nil {
		t.Fatalf("gofmt fixture: %v %s", err, out)
	}
	inv := filepath.Join(t.TempDir(), "inventory")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestAuditReentry$", "-test.v")
	cmd.Env = append(os.Environ(), "AGX_REENTRY_ROOT="+root, "AGX_REENTRY_EXE="+exe, "AGX_REENTRY_INVENTORY="+inv)
	out, err, timedOut := runReentryProcess(t, cmd, 3*time.Minute)
	if timedOut {
		t.Fatalf("gate exceeded deadline: %v\n%s", err, out)
	}
	data, readErr := os.ReadFile(inv)
	if readErr != nil {
		t.Fatal(readErr)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if err != nil {
		t.Fatalf("outer gate failed: %v\ninventory:\n%s\n%s", err, data, out)
	}
	if len(lines) != 5 || !strings.HasPrefix(lines[0], "helper depth= pid=") || !strings.HasPrefix(lines[1], "fixture TestRun pid=") || !strings.HasPrefix(lines[2], "fixture TestNestedGate pid=") || !strings.HasPrefix(lines[3], "helper depth=1 pid=") || lines[4] != "nested TestAuditReentry completed" {
		t.Fatalf("unexpected test inventory: %q", data)
	}
	forcedInv := filepath.Join(t.TempDir(), "timeout-inventory")
	forced := exec.Command(exe, "-test.run=^TestAuditReentry$", "-test.v")
	forced.Env = append(os.Environ(), "AGX_REENTRY_ROOT="+root, "AGX_REENTRY_EXE="+exe, "AGX_REENTRY_INVENTORY="+forcedInv, "AGX_REENTRY_FORCE_TIMEOUT=1")
	_, _, timedOut = runReentryProcess(t, forced, 10*time.Second)
	if !timedOut {
		t.Fatal("forced module gate did not reach its deadline")
	}
	forcedData, err := os.ReadFile(forcedInv)
	if err != nil || !strings.Contains(string(forcedData), "fixture TestNestedGate pid=") {
		t.Fatalf("forced timeout did not reach nested gate: %v inventory=%q", err, forcedData)
	}
}

// TestAuditReentryDeadline reproduces descendant-held output after a deadline.
func TestAuditReentryDeadline(t *testing.T) {
	if os.Getenv("AGX_REENTRY_SLEEP") == "1" {
		time.Sleep(3 * time.Second)
		return
	}
	if marker := os.Getenv("AGX_REENTRY_SPAWN"); marker != "" {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		child := exec.Command(exe, "-test.run=^TestAuditReentryDeadline$")
		child.Env = append(os.Environ(), "AGX_REENTRY_SLEEP=1")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(marker, []byte("started"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := child.Wait(); err != nil {
			t.Fatal(err)
		}
		return
	}
	marker := filepath.Join(t.TempDir(), "started")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestAuditReentryDeadline$")
	cmd.Env = append(os.Environ(), "AGX_REENTRY_SPAWN="+marker)
	start := time.Now()
	_, _, timedOut := runReentryProcess(t, cmd, 500*time.Millisecond)
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("descendant did not start: %v", err)
	}
	if !timedOut {
		t.Fatal("expected deadline")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("deadline failed to bound descendant-held output: %v", elapsed)
	}
}
