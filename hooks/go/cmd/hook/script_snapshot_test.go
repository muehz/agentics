package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hooks"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func capturingDependencies(seen map[string]shellwords.ScriptReader, mu *sync.Mutex) hookDependencies {
	deps := productionHookDependencies()
	deps.lookup = func(name string) (hookspec.Handler, bool) {
		return func(req hookspec.Request) hookspec.Outcome {
			mu.Lock()
			seen[name] = req.Scripts
			mu.Unlock()
			return contract.Pass()
		}, true
	}
	return deps
}

func TestGroupedChecksShareOneSnapshot(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]shellwords.ScriptReader{}
	payload := string(hosttest.Stdin("claude-code", hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "echo hi"}, Cwd: t.TempDir()}))
	code, _, stderr := invokeWithLegalArgsAndDependencies(t, capturingDependencies(seen, &mu), payload, "claude-code", "pre-tool")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if len(seen) != 2 {
		t.Fatalf("want 2 checks, got %v", seen)
	}
	first := seen["commands"]
	for name, reader := range seen {
		if reader == nil || reader != first {
			t.Fatalf("%s did not share the snapshot", name)
		}
	}
}

func TestSeparateEventsDoNotShareASnapshot(t *testing.T) {
	var mu sync.Mutex
	payload := string(hosttest.Stdin("claude-code", hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "echo hi"}, Cwd: t.TempDir()}))
	firstSeen := map[string]shellwords.ScriptReader{}
	secondSeen := map[string]shellwords.ScriptReader{}
	invokeWithLegalArgsAndDependencies(t, capturingDependencies(firstSeen, &mu), payload, "claude-code", "pre-tool")
	invokeWithLegalArgsAndDependencies(t, capturingDependencies(secondSeen, &mu), payload, "claude-code", "pre-tool")
	if firstSeen["commands"] == nil || firstSeen["commands"] == secondSeen["commands"] {
		t.Fatalf("a snapshot outlived its event")
	}
}

// Both construction sites must apply the credential path policy: a snapshot built without it would read a
// credential-named script that the other entry refuses.
func TestEverySnapshotConstructionSiteAppliesThePathPolicy(t *testing.T) {
	var mu sync.Mutex
	dir := t.TempDir()
	protected := filepath.ToSlash(filepath.Join(dir, ".env.sh"))
	if err := os.WriteFile(filepath.Join(dir, ".env.sh"), []byte("echo hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for entry, check := range map[string]string{"pre-tool": "commands", "command-guardian": "command-guardian"} {
		t.Run(entry, func(t *testing.T) {
			seen := map[string]shellwords.ScriptReader{}
			payload := string(hosttest.Stdin("claude-code", hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "echo hi"}, Cwd: dir}))
			code, _, stderr := invokeWithLegalArgsAndDependencies(t, capturingDependencies(seen, &mu), payload, "claude-code", entry)
			if code != 0 || seen[check] == nil {
				t.Fatalf("exit %d, reader %v, stderr %s", code, seen[check], stderr)
			}
			if got := seen[check].ReadScript(protected); got.Limit != shellwords.RulePathProtected || got.Text != "" {
				t.Fatalf("snapshot built without the path policy: %+v", got)
			}
		})
	}
	if got := hooks.NewScriptSnapshot().ReadScript(protected); got.Limit != shellwords.RulePathProtected {
		t.Fatalf("%+v", got)
	}
}

func TestLegacySingleCheckBuildsItsOwnSnapshot(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]shellwords.ScriptReader{}
	payload := string(hosttest.Stdin("claude-code", hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "echo hi"}, Cwd: t.TempDir()}))
	code, _, stderr := invokeWithLegalArgsAndDependencies(t, capturingDependencies(seen, &mu), payload, "claude-code", "command-guardian")
	if code != 0 || seen["command-guardian"] == nil {
		t.Fatalf("exit %d, reader %v, stderr %s", code, seen["command-guardian"], stderr)
	}
}
