package toolcall_test

// This test lives in the external test package so that it reads real provider
// payloads: it drives them through provider.Normalize before reading the call.

import (
	"encoding/json"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/provider"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

// normalizedCall normalizes one provider payload and reads its call.
func normalizedCall(t *testing.T, surfaceName, hookEvent string, payload map[string]any) toolcall.Call {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	event, reject := provider.Normalize(surfaceName, hookEvent, raw)
	if reject != nil {
		t.Fatalf("normalize rejected: %+v", reject)
	}
	neutral, err := event.Read()
	if err != nil {
		t.Fatal(err)
	}
	call, err := toolcall.Read(neutral)
	if err != nil {
		t.Fatal(err)
	}
	return call
}

// inventedSurfaceCall reads a call on a surface name no adapter produces.
func inventedSurfaceCall(t *testing.T, surfaceName, tool string) toolcall.Call {
	t.Helper()
	call, err := toolcall.Read(contract.NeutralEvent{
		Surface: surfaceName,
		Tool:    contract.NeutralTool{Name: &tool, InputRaw: json.RawMessage(`{"command":"echo ok"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return call
}

func TestExecutionCopilotNative(t *testing.T) {
	proven := shellwords.ExecutionContext{Dialect: shellwords.PowerShell, Proven: true}
	competing := shellwords.ExecutionContext{Dialect: shellwords.PowerShell, Reason: "competing command evidence"}
	for _, tc := range []struct {
		name string
		args map[string]any
		want shellwords.ExecutionContext
	}{
		{"native cmd", map[string]any{"cmd": "echo ok"}, proven},
		{"fallback command", map[string]any{"command": "echo ok"}, proven},
		{"same evidence", map[string]any{"cmd": "echo ok", "command": "echo ok"}, proven},
		{"conflicting evidence", map[string]any{"cmd": "echo ok", "command": "other"}, competing},
		{"malformed competing command", map[string]any{"cmd": "echo ok", "command": nil}, competing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := normalizedCall(t, "copilot", "preToolUse", map[string]any{"toolName": "powershell", "toolArgs": tc.args})
			if got := call.Execution(); got != tc.want {
				t.Fatalf("real normalized surface=%s input=%v context=%+v want %+v", call.Surface(), call.Input(), got, tc.want)
			}
		})
	}
	want := shellwords.ExecutionContext{Dialect: shellwords.PowerShell, Reason: "unqualified provider"}
	if got := inventedSurfaceCall(t, "copilot-cli", "PowerShell").Execution(); got != want {
		t.Fatalf("invented neutral surface: context=%+v, want %+v", got, want)
	}
}

func TestExecutionAntigravityNative(t *testing.T) {
	proven := shellwords.ExecutionContext{Dialect: shellwords.POSIX, Proven: true}
	competing := shellwords.ExecutionContext{Dialect: shellwords.POSIX, Reason: "competing command evidence"}
	for _, tc := range []struct {
		name string
		args map[string]any
		want shellwords.ExecutionContext
	}{
		{"native CommandLine", map[string]any{"CommandLine": "echo ok"}, proven},
		{"fallback command", map[string]any{"command": "echo ok"}, proven},
		{"same evidence", map[string]any{"CommandLine": "echo ok", "command": "echo ok"}, proven},
		{"conflicting evidence", map[string]any{"CommandLine": "echo ok", "command": "other"}, competing},
		{"malformed competing command", map[string]any{"CommandLine": "echo ok", "command": nil}, competing},
		{"malformed competing CommandLine", map[string]any{"CommandLine": nil, "command": "echo ok"}, competing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := normalizedCall(t, "antigravity", "PreToolUse", map[string]any{
				"hookEventName": "PreToolUse",
				"toolCall": map[string]any{
					"name": "run_command",
					"args": tc.args,
				},
			})
			if got := call.Execution(); got != tc.want {
				t.Fatalf("real normalized surface=%s input=%v context=%+v want %+v", call.Surface(), call.Input(), got, tc.want)
			}
		})
	}
	want := shellwords.ExecutionContext{Dialect: shellwords.POSIX, Reason: "unqualified provider"}
	if got := inventedSurfaceCall(t, "agy", "Bash").Execution(); got != want {
		t.Fatalf("invented neutral surface agy: context=%+v, want %+v", got, want)
	}
}
