package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func marshalAntigravityPreTool(t *testing.T, cwd, toolName string, args map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"hookEventName": "PreToolUse",
		"cwd":           cwd,
		"toolCall": map[string]any{
			"name": toolName,
			"args": args,
		},
	})
	if err != nil {
		t.Fatalf("marshaling Antigravity payload: %v", err)
	}
	return string(raw)
}

func TestAntigravityTaskInputEndToEnd(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	t.Setenv("AGX_RELEASE_RECORD", "")

	denyCases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{
			name: "manage_task send_input python git reset",
			tool: "manage_task",
			args: map[string]any{"Action": "send_input", "TaskId": "task-1", "Input": "import os; os.system(\"git reset --hard\")\n"},
		},
		{
			name: "manage_task send_input python env read",
			tool: "manage_task",
			args: map[string]any{"Action": "send_input", "TaskId": "task-1", "Input": "open(\".env\").read()\n"},
		},
		{
			name: "manage_task send_input shell git reset",
			tool: "manage_task",
			args: map[string]any{"Action": "send_input", "TaskId": "task-1", "Input": "git reset --hard\n"},
		},
		{
			name: "manage_task send_input shell cat env",
			tool: "manage_task",
			args: map[string]any{"Action": "send_input", "TaskId": "task-1", "Input": "cat .env\n"},
		},
		{
			name: "manage_task send_input shell kubectl apply",
			tool: "manage_task",
			args: map[string]any{"Action": "send_input", "TaskId": "task-1", "Input": "kubectl apply -f production.yaml\n"},
		},
		{
			name: "manage_task send_input empty Input",
			tool: "manage_task",
			args: map[string]any{"Action": "send_input", "TaskId": "task-1", "Input": ""},
		},
		{
			name: "manage_task send_input non-string Input",
			tool: "manage_task",
			args: map[string]any{"Action": "send_input", "TaskId": "task-1", "Input": 123},
		},
		{
			name: "manage_task missing Action",
			tool: "manage_task",
			args: map[string]any{"TaskId": "task-1"},
		},
		{
			name: "manage_task unknown Action",
			tool: "manage_task",
			args: map[string]any{"Action": "write", "TaskId": "task-1"},
		},
		{
			name: "manage_task smuggled Input alongside status",
			tool: "manage_task",
			args: map[string]any{"Action": "status", "TaskId": "task-1", "Input": "git reset --hard\n"},
		},
		{
			name: "send_command_input shell Input",
			tool: "send_command_input",
			args: map[string]any{"CommandId": "cmd-1", "Input": "git reset --hard\n"},
		},
		{
			name: "send_command_input python Input",
			tool: "send_command_input",
			args: map[string]any{"CommandId": "cmd-1", "Input": "open(\".env\").read()\n"},
		},
		{
			name: "send_command_input Terminate true with non-empty Input",
			tool: "send_command_input",
			args: map[string]any{"CommandId": "cmd-1", "Terminate": true, "Input": "y\n"},
		},
		{
			name: "send_command_input Terminate false",
			tool: "send_command_input",
			args: map[string]any{"CommandId": "cmd-1", "Terminate": false, "Input": ""},
		},
	}

	for _, tc := range denyCases {
		t.Run("deny/"+tc.name, func(t *testing.T) {
			cwd := t.TempDir()
			t.Setenv("COMMAND_GUARD_LOG_DIR", filepath.Join(cwd, "cmd-logs"))
			payload := marshalAntigravityPreTool(t, cwd, tc.tool, tc.args)
			code, stdout, stderr := invoke(t, payload, "antigravity", "pre-tool")
			reason := requireDeny(t, surface.Antigravity, code, stdout, stderr)
			if !strings.Contains(reason, "command_parse_limit") {
				t.Errorf("reason = %q, want command_parse_limit", reason)
			}
			if stderr != "" {
				t.Errorf("unexpected stderr: %q", stderr)
			}
		})
	}

	allowCases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{
			name: "manage_task list",
			tool: "manage_task",
			args: map[string]any{"Action": "list"},
		},
		{
			name: "manage_task status",
			tool: "manage_task",
			args: map[string]any{"Action": "status", "TaskId": "task-1"},
		},
		{
			name: "manage_task kill",
			tool: "manage_task",
			args: map[string]any{"Action": "kill", "TaskId": "task-1"},
		},
		{
			name: "send_command_input pure terminate",
			tool: "send_command_input",
			args: map[string]any{"CommandId": "cmd-1", "Terminate": true},
		},
		{
			name: "send_command_input pure terminate empty Input",
			tool: "send_command_input",
			args: map[string]any{"CommandId": "cmd-1", "Terminate": true, "Input": ""},
		},
		{
			name: "send_command_input pure terminate null Input",
			tool: "send_command_input",
			args: map[string]any{"CommandId": "cmd-1", "Terminate": true, "Input": nil},
		},
	}

	for _, tc := range allowCases {
		t.Run("allow/"+tc.name, func(t *testing.T) {
			payload := marshalAntigravityPreTool(t, t.TempDir(), tc.tool, tc.args)
			code, stdout, stderr := invoke(t, payload, "antigravity", "pre-tool")
			if code != 0 || stdout != "{\"decision\":\"allow\"}\n" || stderr != "" {
				t.Fatalf("got exit=%d stdout=%q stderr=%q, want exit 0 allow JSON with empty stderr", code, stdout, stderr)
			}
		})
	}

	t.Run("single-check guards inspect lifted command directly", func(t *testing.T) {
		cwd := t.TempDir()
		secretPayload := marshalAntigravityPreTool(t, cwd, "manage_task", map[string]any{
			"Action": "send_input",
			"TaskId": "task-1",
			"Input":  "cat .env\n",
		})
		code, stdout, stderr := invoke(t, secretPayload, "antigravity", "secrets-guardian", "--mode", "block")
		requireDeny(t, surface.Antigravity, code, stdout, stderr)

		releasePayload := marshalAntigravityPreTool(t, cwd, "manage_task", map[string]any{
			"Action": "send_input",
			"TaskId": "task-1",
			"Input":  "kubectl apply -f production.yaml\n",
		})
		code, stdout, stderr = invoke(t, releasePayload, "antigravity", "release-guardian", "--mode", "block")
		requireDeny(t, surface.Antigravity, code, stdout, stderr)
	})

	t.Run("allowlist does not bypass pre-tool manage_task send_input", func(t *testing.T) {
		cwd := t.TempDir()
		t.Setenv("COMMAND_GUARD_ALLOWLIST", "import os, Bash")
		t.Setenv("COMMAND_GUARD_LOG_DIR", filepath.Join(cwd, "cmd-logs"))
		payload := marshalAntigravityPreTool(t, cwd, "manage_task", map[string]any{
			"Action": "send_input",
			"TaskId": "task-1",
			"Input":  "import os; os.system(\"git reset --hard\")\n",
		})
		code, stdout, stderr := invoke(t, payload, "antigravity", "pre-tool")
		requireDeny(t, surface.Antigravity, code, stdout, stderr)
	})

	t.Run("warn mode logs and renders allow on Antigravity", func(t *testing.T) {
		cwd := t.TempDir()
		logDir := filepath.Join(cwd, "cmd-logs")
		t.Setenv("COMMAND_GUARD_MODE", "warn")
		t.Setenv("COMMAND_GUARD_LOG_DIR", logDir)
		payload := marshalAntigravityPreTool(t, cwd, "manage_task", map[string]any{
			"Action": "send_input",
			"TaskId": "task-1",
			"Input":  "import os; os.system(\"git reset --hard\")\n",
		})
		code, stdout, stderr := invoke(t, payload, "antigravity", "pre-tool")
		if code != 0 || stdout != "{\"decision\":\"allow\"}\n" || stderr != "" {
			t.Fatalf("warn mode got exit=%d stdout=%q stderr=%q, want exit 0 allow JSON", code, stdout, stderr)
		}
		if _, err := os.Stat(filepath.Join(logDir, "guard.log")); err != nil {
			t.Fatalf("expected guard.log in warn mode: %v", err)
		}
	})
}
