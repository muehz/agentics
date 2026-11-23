package commandguard

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestUnclassifiedTaskInputGuard(t *testing.T) {
	wantDeny := "1 blocking threat(s) detected: " + unclassifiedTaskInputCategory + " - " + unclassifiedTaskInputSuggestion
	wantWarn := "1 threat(s) detected: " + unclassifiedTaskInputCategory + " - " + unclassifiedTaskInputSuggestion

	t.Run("denies unclassified task input in block mode regardless of stdin content", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			input string
		}{
			{"python git reset", `{"command":"import os; os.system(\"git reset --hard\")\n","unclassified_task_input":true}`},
			{"python env read", `{"command":"open(\".env\").read()\n","unclassified_task_input":true}`},
			{"benign-looking stdin", `{"command":"print(1 + 1)\n","unclassified_task_input":true}`},
			{"empty stdin", `{"command":"","unclassified_task_input":true}`},
			{"missing command key", `{"unclassified_task_input":true}`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out, _, logRoot := runGo(t, tableCase{tool: "Bash", input: tc.input})
				if out.Kind() != "deny" {
					t.Fatalf("kind = %s, want deny: %s", out.Kind(), out.Payload())
				}
				if out.Payload() != wantDeny {
					t.Fatalf("reason = %q, want %q", out.Payload(), wantDeny)
				}
				data, err := os.ReadFile(guardLogPath(logRoot))
				if err != nil {
					t.Fatalf("reading guard.log: %v", err)
				}
				if !bytes.Contains(data, []byte(`"match":"`+unclassifiedTaskInputMatch+`"`)) {
					t.Errorf("guard.log = %s, want fixed match %q", data, unclassifiedTaskInputMatch)
				}
				if bytes.Contains(data, []byte("os.system")) || bytes.Contains(data, []byte(".env")) {
					t.Errorf("guard.log leaked raw stdin: %s", data)
				}
			})
		}
	})

	t.Run("allowlist cannot bypass unclassified task input", func(t *testing.T) {
		out, _, _ := runGo(t, tableCase{
			tool:  "Bash",
			input: `{"command":"import os; os.system(\"git reset --hard\")\n","unclassified_task_input":true}`,
			env:   map[string]string{"COMMAND_GUARD_ALLOWLIST": "import os, Bash"},
		})
		if out.Kind() != "deny" || out.Payload() != wantDeny {
			t.Fatalf("allowlist bypassed unclassified_task_input: kind=%s payload=%q", out.Kind(), out.Payload())
		}
	})

	t.Run("warn mode returns context and logs", func(t *testing.T) {
		out, _, logRoot := runGo(t, tableCase{
			tool:  "Bash",
			input: `{"command":"print(1)\n","unclassified_task_input":true}`,
			env:   map[string]string{"COMMAND_GUARD_MODE": "warn"},
		})
		if out.Kind() != "context" || out.Payload() != wantWarn {
			t.Fatalf("warn mode = %s %q, want context %q", out.Kind(), out.Payload(), wantWarn)
		}
		data, err := os.ReadFile(guardLogPath(logRoot))
		if err != nil {
			t.Fatalf("reading guard.log: %v", err)
		}
		if !bytes.Contains(data, []byte(`"mode":"warn"`)) {
			t.Errorf("guard.log = %s, want mode warn", data)
		}
	})

	t.Run("invalid mode blocks with warning prefix", func(t *testing.T) {
		out, _, _ := runGo(t, tableCase{
			tool:  "Bash",
			input: `{"command":"print(1)\n","unclassified_task_input":true}`,
			env:   map[string]string{"COMMAND_GUARD_MODE": "blcok"},
		})
		if out.Kind() != "deny" || !strings.HasPrefix(out.Payload(), unknownModeWarning+"\n\n"+wantDeny) {
			t.Fatalf("invalid mode = %s %q", out.Kind(), out.Payload())
		}
	})

	t.Run("SKIP_COMMAND_GUARD disables check", func(t *testing.T) {
		out, _, _ := runGo(t, tableCase{
			tool:  "Bash",
			input: `{"command":"print(1)\n","unclassified_task_input":true}`,
			env:   map[string]string{"SKIP_COMMAND_GUARD": "true"},
		})
		if out.Kind() != "pass" {
			t.Fatalf("SKIP_COMMAND_GUARD=true = %s %q, want pass", out.Kind(), out.Payload())
		}
	})

	t.Run("negative cases pass", func(t *testing.T) {
		for _, tc := range []tableCase{
			{name: "false flag on safe Bash", tool: "Bash", input: `{"command":"echo ok","unclassified_task_input":false}`},
			{name: "non-bool flag on safe Bash", tool: "Bash", input: `{"command":"echo ok","unclassified_task_input":"true"}`},
			{name: "manage_task list", tool: "manage_task", input: `{"value":{"Action":"list"}}`},
			{name: "manage_task status", tool: "manage_task", input: `{"value":{"Action":"status","TaskId":"task-1"}}`},
			{name: "manage_task kill", tool: "manage_task", input: `{"value":{"Action":"kill","TaskId":"task-1"}}`},
			{name: "send_command_input pure terminate", tool: "send_command_input", input: `{"value":{"CommandId":"cmd-1","Terminate":true}}`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out, _, _ := runGo(t, tc)
				if out.Kind() != "pass" {
					t.Fatalf("%s = %s %q, want pass", tc.name, out.Kind(), out.Payload())
				}
			})
		}
	})
}
