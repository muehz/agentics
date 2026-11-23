package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookcost"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestHookCostDispatcherRealLaunch(t *testing.T) {
	source, err := filepath.Abs(filepath.Join(hosttest.Checkout(t), "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	hooksDir := filepath.Join(t.TempDir(), "hooks")
	target := stageHookCostTarget(t, source, packagedHookNativeBinary(t, hooksDir))
	fixture, env := hookcost.Prepare(t, target)
	c := hookcost.Config{Provider: "claude-code", Hook: "command-guardian", Event: "PreToolUse",
		Argv: []string{"claude-code", "command-guardian", "--mode", "block"}, TimeoutSeconds: 10}
	probe := hookCostProbe(c, hookCostStdin(surface.ClaudeCode, "Bash", fixture))
	route := hookcost.Dispatcher(target, fixture, env, run)
	arm := hookcost.LaunchArm("dispatcher", "dispatcher-launch", route, probe, c)
	if err := arm.Preflight(); err != nil {
		t.Fatal(err)
	}
	if _, err := arm.Run(); err != nil {
		t.Fatal(err)
	}
	if got := route.Command(probe).WorkingDir; got != hooksDir {
		t.Fatalf("factory working directory lost: got %q, want %q", got, hooksDir)
	}
	// The Bash form of the dispatcher is the one a Unix host runs; on Windows
	// it is Git Bash.
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, append([]string{"run-hook.cmd"}, c.Argv...)...)
	cmd.Dir = hooksDir
	cmd.Env = append(append([]string{}, env...), "AGX_DEBUG=1")
	cmd.Stdin = strings.NewReader(probe.Stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("dispatcher preflight failed: %v\nstdout: %q\nstderr: %q", err, stdout.String(), stderr.String())
	}
	if err := hookcost.CheckResolution(stderr.String(), cmd.Dir, target.Binary); stdout.Len() != 0 || err != nil {
		t.Fatalf("dispatcher preflight did not resolve %q: stdout=%q stderr=%q error=%v", target.Binary, stdout.String(), stderr.String(), err)
	}
	if runtime.GOOS == "windows" {
		t.Log("Bash diagnostic probe is Git Bash on Windows, not native Unix runtime evidence")
	}
}
