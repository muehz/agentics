package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// nulCwdStdin is a shell call whose working directory holds a NUL, which the
// normalized event rejects.
func nulCwdStdin(surface surface.ID) string {
	return string(hosttest.Stdin(surface, hosttest.Call{
		Tool: "Bash", Input: map[string]any{"command": "git status --short"}, Cwd: "\x00",
	}))
}

func TestEventBoundaryCanonicalShellText(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			deps := productionHookDependencies()
			called := false
			deps = stubLookup(deps, func(req hookspec.Request) hookspec.Outcome {
				called = true
				event, err := req.Event.Read()
				if err != nil {
					t.Fatal(err)
				}
				want := `{"command":"echo <a> && echo b > out","nested":{"a":1.00,"z":2}}`
				if surface != "claude-code" {
					want = `{"command":"echo <a> && echo b > out","value":` + want + `}`
				}
				if string(event.Tool.InputRaw) != want {
					t.Fatalf("raw input = %s, want %s", event.Tool.InputRaw, want)
				}
				return contract.Pass()
			})
			// The raw input keeps its number spelling and key order, which the
			// event must canonicalize.
			payload := string(hosttest.Stdin(surface, hosttest.Call{
				Tool:   hosttest.NativeTool(surface, "Bash"),
				Input:  json.RawMessage(`{"nested":{"z":2,"a":1.00},"command":"echo <a> && echo b > out"}`),
				Cwd:    t.TempDir(),
				Native: true,
			}))
			code, _, stderr := invokeWithLegalArgsAndDependencies(t, deps, payload, surface, "command-guardian")
			if !called || code != 0 || stderr != "" {
				t.Fatalf("called=%v exit=%d stderr=%q", called, code, stderr)
			}
		})
	}
}

func TestEventBoundarySkipPrecedesNULValidation(t *testing.T) {
	for _, surface := range surface.All() {
		for _, tc := range []struct{ hook, flag string }{
			{"command-guardian", "SKIP_COMMAND_GUARD"},
			{"secrets-guardian", "SKIP_SECRETS_GUARD"},
		} {
			t.Run(string(surface)+"/"+tc.hook, func(t *testing.T) {
				payload := nulCwdStdin(surface)
				t.Setenv(tc.flag, "true")
				code, stdout, stderr := invokeWithLegalArgs(t, payload, surface, tc.hook)
				wantOut := ""
				switch surface {
				case "codex":
					wantOut = "{}\n"
				case "antigravity":
					wantOut = "{\"decision\":\"allow\"}\n"
				}
				if code != 0 || stdout != wantOut || stderr != "" {
					t.Fatalf("skipped NUL event: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
				}
				t.Setenv(tc.flag, "")
				code, stdout, stderr = invokeWithLegalArgs(t, payload, surface, tc.hook)

				// command-guardian denies an event-validation failure like any
				// other unreadable input; secrets-guardian keeps the error shape
				// (an operational fault, not a policy decision on the call).
				if tc.hook == "command-guardian" {
					if reason := requireDeny(t, surface, code, stdout, stderr); !strings.Contains(reason, "could not validate normalized input") {
						t.Fatalf("active NUL event: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
					}
					return
				}
				wantCode := 0
				if surface == "claude-code" {
					wantCode = 1
				}
				if code != wantCode || stdout != wantOut || !strings.Contains(stderr, "could not validate normalized input") {
					t.Fatalf("active NUL event: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
				}
			})
		}
	}
}

// The grouped pre-tool path reads the event before consulting any guard, so
// the skip switches that let a single guard pass a NUL event must not let the
// grouped invocation pass it.
func TestEventBoundaryGroupedNULDeniesDespiteSkips(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			payload := nulCwdStdin(surface)
			t.Setenv("SKIP_COMMAND_GUARD", "true")
			t.Setenv("SKIP_SECRETS_GUARD", "true")
			code, stdout, stderr := invokeWithLegalArgs(t, payload, surface, "pre-tool")
			if reason := requireDeny(t, surface, code, stdout, stderr); !strings.Contains(reason, "normalized event fields are invalid") {
				t.Fatalf("grouped NUL event with skips: reason=%q, want it to contain %q", reason, "normalized event fields are invalid")
			}
		})
	}
}

// payloadWithField is the surface's benign shell call with one extra top-level
// field, as the host writes it. hosttest.Stdin cannot add fields it does not
// know, so the field is spliced into the decoded payload.
func payloadWithField(t *testing.T, id surface.ID, field, value string) string {
	t.Helper()
	var payload map[string]any
	base := hosttest.Stdin(id, hosttest.Call{
		Tool: "Bash", Input: map[string]any{"command": "git status --short"}, Cwd: t.TempDir(),
	})
	if err := json.Unmarshal(base, &payload); err != nil {
		t.Fatal(err)
	}
	payload[field] = value
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func quietBody(id surface.ID) string {
	switch id {
	case surface.Codex:
		return "{}\n"
	case surface.Antigravity:
		return "{\"decision\":\"allow\"}\n"
	}
	return ""
}

// Codex permission_mode and Antigravity transcriptPath and modelName are read
// by no check, yet a NUL in them is rejected when the event is read, so a
// single guard's skip switch still wins and the grouped event still denies.
func TestEventBoundaryUnreadFieldNULSkipAndDeny(t *testing.T) {
	for _, tc := range []struct {
		id    surface.ID
		field string
	}{
		{surface.Codex, "permission_mode"},
		{surface.Antigravity, "transcriptPath"},
		{surface.Antigravity, "modelName"},
	} {
		t.Run(string(tc.id)+"/"+tc.field, func(t *testing.T) {
			payload := payloadWithField(t, tc.id, tc.field, "\x00")
			for _, hc := range []struct{ hook, flag string }{
				{"command-guardian", "SKIP_COMMAND_GUARD"},
				{"secrets-guardian", "SKIP_SECRETS_GUARD"},
			} {
				t.Run(hc.hook, func(t *testing.T) {
					wantOut := quietBody(tc.id)
					t.Setenv(hc.flag, "true")
					code, stdout, stderr := invokeWithLegalArgs(t, payload, tc.id, hc.hook)
					if code != 0 || stdout != wantOut || stderr != "" {
						t.Fatalf("skipped NUL event: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
					}
					t.Setenv(hc.flag, "")
					code, stdout, stderr = invokeWithLegalArgs(t, payload, tc.id, hc.hook)
					if hc.hook == "command-guardian" {
						if reason := requireDeny(t, tc.id, code, stdout, stderr); !strings.Contains(reason, "could not validate normalized input") {
							t.Fatalf("active NUL event: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
						}
						return
					}
					wantCode := 0
					if tc.id == surface.ClaudeCode {
						wantCode = 1
					}
					if code != wantCode || stdout != wantOut || !strings.Contains(stderr, "could not validate normalized input") {
						t.Fatalf("active NUL event: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
					}
				})
			}
			t.Run("pre-tool", func(t *testing.T) {
				t.Setenv("SKIP_COMMAND_GUARD", "true")
				t.Setenv("SKIP_SECRETS_GUARD", "true")
				code, stdout, stderr := invokeWithLegalArgs(t, payload, tc.id, "pre-tool")
				if reason := requireDeny(t, tc.id, code, stdout, stderr); !strings.Contains(reason, "normalized event fields are invalid") {
					t.Fatalf("grouped NUL event with skips: reason=%q, want it to contain %q", reason, "normalized event fields are invalid")
				}
			})
		})
	}

	// Strings the normalizers drop and no check reads are not part of the NUL
	// check: a NUL in them does not deny the single check.
	t.Run("unchecked", func(t *testing.T) {
		for _, tc := range []struct {
			id    surface.ID
			field string
		}{
			{surface.Codex, "model"},
			{surface.Codex, "transcript_path"},
			{surface.Codex, "tool_use_id"},
			{surface.Codex, "agent_id"},
			{surface.Codex, "agent_type"},
			{surface.ClaudeCode, "permission_mode"},
			{surface.ClaudeCode, "transcript_path"},
		} {
			t.Run(string(tc.id)+"/"+tc.field, func(t *testing.T) {
				payload := payloadWithField(t, tc.id, tc.field, "\x00")
				code, stdout, stderr := invokeWithLegalArgs(t, payload, tc.id, "command-guardian")
				if code != 0 || stdout != quietBody(tc.id) || stderr != "" {
					t.Fatalf("unchecked NUL field: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
				}
			})
		}
	})
}

func TestEventBoundaryInvalidModePrecedesNULDiagnostic(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			t.Setenv("SECRETS_GUARD_MODE", "blcok")
			payload := nulCwdStdin(surface)
			code, stdout, stderr := invokeWithLegalArgs(t, payload, surface, "secrets-guardian")
			wantCode, wantOut := 0, ""
			if surface == "claude-code" {
				wantCode = 1
			}
			switch surface {
			case "codex":
				wantOut = "{}\n"
			case "antigravity":
				wantOut = "{\"decision\":\"allow\"}\n"
			}
			const warning = "secrets-guardian: SECRETS_GUARD_MODE=\"blcok\" is not warn or block; using block\n"
			if code != wantCode || stdout != wantOut || stderr != warning {
				t.Fatalf("invalid mode + NUL: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}

func TestEventBoundaryUnusedOversizedInteger(t *testing.T) {
	for _, surface := range surface.All() {
		const hook = "secrets-guardian"
		t.Run(string(surface)+"/"+hook, func(t *testing.T) {
			payload := string(hosttest.Stdin(surface, hosttest.Call{
				Tool:   "UninspectedTool",
				Input:  json.RawMessage(`{"huge":1` + strings.Repeat("0", 400) + `}`),
				Cwd:    "/agentics-event-boundary-missing",
				Native: true,
			}))
			code, stdout, stderr := invokeWithLegalArgs(t, payload, surface, hook)
			wantOut := ""
			switch surface {
			case "codex":
				wantOut = "{}\n"
			case "antigravity":
				wantOut = "{\"decision\":\"allow\"}\n"
			}
			if code != 0 || stdout != wantOut || stderr != "" {
				t.Fatalf("unused huge integer: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
		})
	}
}
