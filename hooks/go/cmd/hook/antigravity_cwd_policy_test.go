package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func initTestRepoWithPolicy(t *testing.T, policyJSON string) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("creating .git dir: %v", err)
	}
	if policyJSON != "" {
		dir := filepath.Join(repo, ".agentics")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("creating .agentics dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "hooks.json"), []byte(policyJSON), 0o600); err != nil {
			t.Fatalf("writing .agentics/hooks.json: %v", err)
		}
	}
	return repo
}

func TestAntigravityMultiWorkspaceCommandCwdPolicy(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	t.Setenv("AGX_RELEASE_RECORD", "")
	// Isolate HOME/USERPROFILE so no user-level ~/.agentics/hooks.json affects policy.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	relaxed := initTestRepoWithPolicy(t, `{"version":1,"policy":{"require_deploy_record":false,"draft_prs":false}}`)
	protected := initTestRepoWithPolicy(t, "")

	for _, tc := range []struct {
		name      string
		payload   map[string]any
		wantBlock bool
	}{
		{
			name: "relaxed first workspace cannot weaken release policy for run_command.Cwd targeting protected repo",
			payload: map[string]any{
				"hookEventName":  "PreToolUse",
				"workspacePaths": []string{relaxed, protected},
				"toolCall": map[string]any{
					"name": "run_command",
					"args": map[string]any{"CommandLine": "kubectl apply -f production.yaml", "Cwd": protected},
				},
			},
			wantBlock: true,
		},
		{
			name: "reversed workspace order still blocks deploy in protected repo",
			payload: map[string]any{
				"hookEventName":  "PreToolUse",
				"workspacePaths": []string{protected, relaxed},
				"toolCall": map[string]any{
					"name": "run_command",
					"args": map[string]any{"CommandLine": "kubectl apply -f production.yaml", "Cwd": protected},
				},
			},
			wantBlock: true,
		},
		{
			name: "relaxed session cwd cannot weaken draft_prs policy for run_command.Cwd targeting protected repo",
			payload: map[string]any{
				"hookEventName": "PreToolUse",
				"cwd":           relaxed,
				"toolCall": map[string]any{
					"name": "run_command",
					"args": map[string]any{"CommandLine": "gh pr create --title test --body body", "Cwd": protected},
				},
			},
			wantBlock: true,
		},
		{
			name: "malformed non-string Cwd falls back to strict defaults instead of relaxed workspace",
			payload: map[string]any{
				"hookEventName":  "PreToolUse",
				"workspacePaths": []string{relaxed, protected},
				"toolCall": map[string]any{
					"name": "run_command",
					"args": map[string]any{"CommandLine": "kubectl apply -f production.yaml", "Cwd": 123},
				},
			},
			wantBlock: true,
		},
		{
			name: "run_command.Cwd targeting relaxed repo honors relaxed repo policy regardless of workspace order",
			payload: map[string]any{
				"hookEventName":  "PreToolUse",
				"workspacePaths": []string{protected, relaxed},
				"toolCall": map[string]any{
					"name": "run_command",
					"args": map[string]any{"CommandLine": "kubectl apply -f production.yaml", "Cwd": relaxed},
				},
			},
			wantBlock: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.payload)
			if err != nil {
				t.Fatalf("marshaling payload: %v", err)
			}
			code, stdout, stderr := invoke(t, string(raw), "antigravity", "pre-tool")
			if tc.wantBlock {
				_ = requireDeny(t, surface.Antigravity, code, stdout, stderr)
			} else if code != 0 || stdout != "{\"decision\":\"allow\"}\n" || stderr != "" {
				t.Fatalf("got exit=%d stdout=%q stderr=%q, want exit 0 allow JSON with empty stderr", code, stdout, stderr)
			}
		})
	}
}
