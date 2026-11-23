package hosttest_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/provider"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// providerEvents is the event name each surface's Normalize serves for a
// pre-tool call: Copilot spells its events in camelCase.
var providerEvents = map[surface.ID]string{
	surface.Antigravity: "PreToolUse",
	surface.ClaudeCode:  "PreToolUse",
	surface.Copilot:     "preToolUse",
	surface.Codex:       "PreToolUse",
}

func mustPanic(t *testing.T, want string, run func()) {
	t.Helper()
	defer func() {
		t.Helper()
		got := recover()
		if got == nil {
			t.Fatalf("did not panic, want a panic naming %q", want)
		}
		if text, _ := got.(string); !strings.Contains(text, want) {
			t.Fatalf("panic = %v, want it to name %q", got, want)
		}
	}()
	run()
}

func TestNativeTool(t *testing.T) {
	want := map[string]map[surface.ID]string{
		"Bash":       {surface.Antigravity: "run_command", surface.ClaudeCode: "Bash", surface.Copilot: "bash", surface.Codex: "exec_command"},
		"PowerShell": {surface.Antigravity: "run_command", surface.ClaudeCode: "PowerShell", surface.Copilot: "powershell", surface.Codex: "exec_command"},
		"Read":       {surface.Antigravity: "view_file", surface.ClaudeCode: "Read", surface.Copilot: "view", surface.Codex: "Read"},
		"Write":      {surface.Antigravity: "write_to_file", surface.ClaudeCode: "Write", surface.Copilot: "create", surface.Codex: "Write"},
		"Edit":       {surface.Antigravity: "replace_file_content", surface.ClaudeCode: "Edit", surface.Copilot: "edit", surface.Codex: "Edit"},
		"MultiEdit":  {surface.Antigravity: "multi_replace_file_content", surface.ClaudeCode: "MultiEdit", surface.Copilot: "edit", surface.Codex: "MultiEdit"},
	}
	for neutral, row := range want {
		for _, surface := range surface.All() {
			if got := hosttest.NativeTool(surface, neutral); got != row[surface] {
				t.Errorf("NativeTool(%q, %q) = %q, want %q", surface, neutral, got, row[surface])
			}
		}
	}
	mustPanic(t, "Grep", func() { hosttest.NativeTool("claude-code", "Grep") })
	mustPanic(t, "cursor", func() { hosttest.NativeTool("cursor", "Bash") })
}

func decodeStdin(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("Stdin wrote %q, not one JSON object: %v", raw, err)
	}
	return document
}

func TestStdinWritesEachSurfaceEnvelope(t *testing.T) {
	const cwd = "/work"
	neutral := map[string]map[string]any{
		"Bash":       {"command": "ls"},
		"PowerShell": {"command": "Get-ChildItem"},
		"Read":       {"file_path": "a.txt"},
		"Write":      {"file_path": "a.txt", "content": "x"},
		"Edit":       {"file_path": "a.txt", "old_string": "x", "new_string": "y"},
		"MultiEdit":  {"file_path": "a.txt", "edits": []any{map[string]any{"old_string": "x", "new_string": "y"}}},
	}
	copilotArgs := map[string]map[string]any{
		"Bash":       {"cmd": "ls"},
		"PowerShell": {"cmd": "Get-ChildItem"},
		"Read":       {"path": "a.txt"},
		"Write":      {"file_path": "a.txt", "content": "x"},
		"Edit":       {"path": "a.txt", "old_str": "x", "new_str": "y"},
		"MultiEdit":  {"file_path": "a.txt", "edits": []any{map[string]any{"old_string": "x", "new_string": "y"}}},
	}
	antigravityArgs := map[string]map[string]any{
		"Bash":       {"CommandLine": "ls"},
		"PowerShell": {"CommandLine": "Get-ChildItem"},
		"Read":       {"AbsolutePath": "a.txt"},
		"Write":      {"TargetFile": "a.txt", "CodeContent": "x"},
		"Edit":       {"TargetFile": "a.txt", "TargetContent": "x", "ReplacementContent": "y"},
		"MultiEdit":  {"TargetFile": "a.txt", "ReplacementChunks": []any{map[string]any{"TargetContent": "x", "ReplacementContent": "y"}}},
	}
	for tool, input := range neutral {
		for _, surface := range surface.All() {
			t.Run(string(surface)+"/"+tool, func(t *testing.T) {
				withExtra := map[string]any{"unnamed_key": "kept"}
				for key, value := range input {
					withExtra[key] = value
				}
				got := decodeStdin(t, hosttest.Stdin(surface, hosttest.Call{Tool: tool, Input: withExtra, Cwd: cwd}))
				var want map[string]any
				switch surface {
				case "copilot":
					args := map[string]any{"unnamed_key": "kept"}
					for key, value := range copilotArgs[tool] {
						args[key] = value
					}
					want = map[string]any{"toolName": hosttest.NativeTool(surface, tool), "toolArgs": args, "cwd": cwd}
				case "antigravity":
					args := map[string]any{"unnamed_key": "kept"}
					for key, value := range antigravityArgs[tool] {
						args[key] = value
					}
					want = map[string]any{
						"hookEventName": "PreToolUse", "cwd": cwd,
						"toolCall": map[string]any{"name": hosttest.NativeTool(surface, tool), "args": args},
					}
				default:
					want = map[string]any{
						"hook_event_name": "PreToolUse", "tool_name": hosttest.NativeTool(surface, tool),
						"tool_input": withExtra, "cwd": cwd,
					}
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("Stdin = %#v\nwant %#v", got, want)
				}
				if _, renamed := withExtra["cmd"]; renamed {
					t.Fatal("Stdin rewrote the caller's input map")
				}
				if _, renamed := withExtra["CommandLine"]; renamed {
					t.Fatal("Stdin rewrote the caller's input map")
				}
			})
		}
	}
}

func TestStdinNativePassesToolAndInputThrough(t *testing.T) {
	patch := "*** Begin Patch\n*** Add File: a.txt\n+x\n*** End Patch"
	got := decodeStdin(t, hosttest.Stdin("copilot", hosttest.Call{Tool: "apply_patch", Input: patch, Cwd: "/w", Native: true}))
	want := map[string]any{"toolName": "apply_patch", "toolArgs": patch, "cwd": "/w"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("copilot native = %#v, want %#v", got, want)
	}
	input := map[string]any{"command": patch}
	got = decodeStdin(t, hosttest.Stdin("codex", hosttest.Call{Tool: "apply_patch", Input: input, Cwd: "/w", Native: true}))
	want = map[string]any{"hook_event_name": "PreToolUse", "tool_name": "apply_patch", "tool_input": input, "cwd": "/w"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("codex native = %#v, want %#v", got, want)
	}
	got = decodeStdin(t, hosttest.Stdin("copilot", hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "ls"}, Cwd: "/w", Native: true}))
	want = map[string]any{"toolName": "Bash", "toolArgs": map[string]any{"command": "ls"}, "cwd": "/w"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("native rewrote a name or key: %#v, want %#v", got, want)
	}
}

func TestStdinPanics(t *testing.T) {
	call := hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "probe-command"}}
	mustPanic(t, "cwd", func() { hosttest.Stdin("claude-code", call) })
	mustPanic(t, "probe-command", func() { hosttest.Stdin("codex", call) })
	call.Cwd = "/w"
	mustPanic(t, "cursor", func() { hosttest.Stdin("cursor", call) })
	mustPanic(t, "Grep", func() { hosttest.Stdin("copilot", hosttest.Call{Tool: "Grep", Input: map[string]any{}, Cwd: "/w"}) })
}

// omittedFields are the fixture fields Stdin does not write. The comparison
// below leaves out what they feed: the session and turn ids, the runtime
// roots (read from the environment), and the fields no check reads.
var omittedFields = []string{
	"session_id", "sessionId", "conversationId", "turn_id", "turnId", "transcript_path", "transcriptPath",
	"model", "modelName", "permission_mode", "tool_use_id", "timestamp", "workspacePaths",
	"unsupported_future_field", "unsupportedFutureField",
}

type comparedEvent struct {
	Surface   string
	Cwd       *string
	ToolName  *string
	ToolInput string
}

func normalizeCompared(t *testing.T, surface surface.ID, raw []byte) comparedEvent {
	t.Helper()
	event, rejected := provider.Normalize(string(surface), providerEvents[surface], raw)
	if rejected != nil {
		t.Fatalf("Normalize rejected %q: %s", raw, rejected.Stderr)
	}
	neutral, err := event.Read()
	if err != nil {
		t.Fatalf("event of %q is unreadable: %v", raw, err)
	}
	// The neutral tool document's result is null on every pre-tool event by
	// construction, so input and tool name are its observable part (the event
	// name is no longer carried).
	return comparedEvent{
		Surface: neutral.Surface, Cwd: neutral.Cwd,
		ToolName: neutral.Tool.Name, ToolInput: string(neutral.Tool.InputRaw),
	}
}

func TestStdinMatchesCommittedPayloads(t *testing.T) {
	root := hosttest.Checkout(t)
	for _, surface := range surface.All() {
		for _, fixture := range []string{"pre-tool-use.json", "pre-tool-use-dangerous.json"} {
			t.Run(string(surface)+"/"+fixture, func(t *testing.T) {
				raw, err := os.ReadFile(filepath.Join(root, "hooks", "go", "surfaces", string(surface), "payloads", fixture))
				if err != nil {
					t.Fatal(err)
				}
				var document map[string]any
				if err := json.Unmarshal(raw, &document); err != nil {
					t.Fatal(err)
				}
				input := map[string]any{}
				switch surface {
				case "copilot":
					command, _ := document["toolArgs"].(map[string]any)["cmd"].(string)
					input["command"] = command
				case "antigravity":
					args, _ := document["toolCall"].(map[string]any)["args"].(map[string]any)
					for k, v := range args {
						if k == "CommandLine" {
							input["command"] = v
						} else {
							input[k] = v
						}
					}
				default:
					command, _ := document["tool_input"].(map[string]any)["command"].(string)
					input["command"] = command
				}
				command, _ := input["command"].(string)
				cwd, _ := document["cwd"].(string)
				if command == "" || cwd == "" {
					t.Fatalf("fixture has no command or cwd: %s", raw)
				}
				stdin := hosttest.Stdin(surface, hosttest.Call{Tool: "Bash", Input: input, Cwd: cwd})
				written := decodeStdin(t, stdin)
				for _, field := range omittedFields {
					if _, ok := written[field]; ok {
						t.Fatalf("Stdin wrote omitted field %q: %s", field, stdin)
					}
				}
				got, want := normalizeCompared(t, surface, stdin), normalizeCompared(t, surface, raw)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("Stdin normalizes to %+v, fixture to %+v", got, want)
				}
			})
		}
	}
}

type readCase struct {
	name           string
	surface        surface.ID
	exit           int
	stdout, stderr string
	want           hosttest.Verdict
}

func allowV(stderr string) hosttest.Verdict {
	return hosttest.Verdict{Decision: hosttest.Allow, Stderr: stderr}
}

func unreadableV(stderr string) hosttest.Verdict {
	return hosttest.Verdict{Decision: hosttest.Unreadable, Stderr: stderr}
}

func TestReadTable(t *testing.T) {
	ccContext := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"note"}}`
	cxDeny := `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"why"}}`
	cxContext := ccContext
	cpDeny := `{"permissionDecision":"deny","permissionDecisionReason":"why"}`
	cases := []readCase{
		{"cc/deny keeps stderr verbatim", "claude-code", 2, "", "why\n", hosttest.Verdict{Decision: hosttest.Deny, Reason: "why\n", Stderr: "why\n"}},
		{"cc/exit 2 with stdout", "claude-code", 2, "x", "why\n", unreadableV("why\n")},
		{"cc/exit 2 with whitespace stdout", "claude-code", 2, "\n", "why\n", unreadableV("why\n")},
		{"cc/exit 2 blank stderr", "claude-code", 2, "", " \n\t", unreadableV(" \n\t")},
		{"cc/exit 2 empty stderr", "claude-code", 2, "", "", unreadableV("")},
		{"cc/silent", "claude-code", 0, "", "", allowV("")},
		{"cc/whitespace stdout", "claude-code", 0, " \n", "", allowV("")},
		{"cc/exit 0 stderr", "claude-code", 0, "", "reason", allowV("reason")},
		{"cc/context", "claude-code", 0, ccContext + "\n", "", hosttest.Verdict{Decision: hosttest.Allow, Context: "note"}},
		{"cc/empty context", "claude-code", 0, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":""}}`, "", unreadableV("")},
		{"cc/context wrong event", "claude-code", 0, `{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"note"}}`, "", unreadableV("")},
		{"cc/context with decision", "claude-code", 0, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"note","permissionDecision":"allow"}}`, "", unreadableV("")},
		{"cc/permission decision", "claude-code", 0, cxDeny, "", unreadableV("")},
		{"cc/empty object", "claude-code", 0, "{}", "", unreadableV("")},
		{"cc/context trailing", "claude-code", 0, ccContext + ccContext, "", unreadableV("")},
		{"cc/invalid json", "claude-code", 0, "{", "", unreadableV("")},
		{"cc/exit 1 diagnostic", "claude-code", 1, "", "fault\n", allowV("fault\n")},
		{"cc/exit 1 whitespace stdout", "claude-code", 1, "\n", "fault\n", allowV("fault\n")},
		{"cc/exit 1 blank stderr", "claude-code", 1, "", " \n", unreadableV(" \n")},
		{"cc/exit 1 empty stderr", "claude-code", 1, "", "", unreadableV("")},
		{"cc/exit 1 with stdout", "claude-code", 1, ccContext, "fault\n", unreadableV("fault\n")},
		{"cc/exit 3", "claude-code", 3, "", "x", unreadableV("x")},
		{"cc/exit -1", "claude-code", -1, "", "", unreadableV("")},

		{"cp/deny", "copilot", 0, cpDeny + "\n", "", hosttest.Verdict{Decision: hosttest.Deny, Reason: "why"}},
		{"cp/deny exit 1", "copilot", 1, cpDeny, "", unreadableV("")},
		{"cp/silent", "copilot", 0, "", "", allowV("")},
		{"cp/stderr only", "copilot", 0, "\n", "ctx\n", allowV("ctx\n")},
		{"cp/empty object", "copilot", 0, "{}", "", unreadableV("")},
		{"cp/allow", "copilot", 0, `{"permissionDecision":"allow"}`, "", unreadableV("")},
		{"cp/deny empty reason", "copilot", 0, `{"permissionDecision":"deny","permissionDecisionReason":""}`, "", unreadableV("")},
		{"cp/deny no reason", "copilot", 0, `{"permissionDecision":"deny"}`, "", unreadableV("")},
		{"cp/deny reason not string", "copilot", 0, `{"permissionDecision":"deny","permissionDecisionReason":1}`, "", unreadableV("")},
		{"cp/nested envelope", "copilot", 0, cxDeny, "", unreadableV("")},
		{"cp/invalid json", "copilot", 0, "{", "", unreadableV("")},

		{"cx/silent", "codex", 0, "{}\n", "", allowV("")},
		{"cx/fault body", "codex", 0, "{}\n", "fault\n", allowV("fault\n")},
		{"cx/deny", "codex", 0, cxDeny + "\n", "", hosttest.Verdict{Decision: hosttest.Deny, Reason: "why"}},
		{"cx/context", "codex", 0, cxContext, "", hosttest.Verdict{Decision: hosttest.Allow, Context: "note"}},
		{"cx/exit 1", "codex", 1, "{}", "", unreadableV("")},
		{"cx/empty stdout", "codex", 0, "", "", unreadableV("")},
		{"cx/other event", "codex", 0, strings.Replace(cxDeny, "PreToolUse", "PostToolUse", 1), "", unreadableV("")},
		{"cx/no event", "codex", 0, `{"hookSpecificOutput":{"permissionDecision":"deny","permissionDecisionReason":"why"}}`, "", unreadableV("")},
		{"cx/flat decision", "codex", 0, cpDeny, "", unreadableV("")},
		{"cx/deny empty reason", "codex", 0, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":""}}`, "", unreadableV("")},
		{"cx/allow decision", "codex", 0, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":"why"}}`, "", unreadableV("")},
		{"cx/empty context", "codex", 0, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":""}}`, "", unreadableV("")},
		{"cx/empty envelope", "codex", 0, `{"hookSpecificOutput":{"hookEventName":"PreToolUse"}}`, "", unreadableV("")},
		{"cx/invalid json", "codex", 0, "{", "", unreadableV("")},
		{"cx/second document", "codex", 0, "{}{}", "", unreadableV("")},

		// The old readers decode into structs, which match keys in any letter
		// case and read Copilot's hookSpecificOutput, and compare Codex's pass
		// body as text; a document they would read differently is not one
		// documented form.
		{"cp/deny beside an envelope", "copilot", 0, `{"permissionDecision":"deny","permissionDecisionReason":"why","hookSpecificOutput":{}}`, "", unreadableV("")},
		{"cp/deny beside a null envelope", "copilot", 0, `{"permissionDecision":"deny","permissionDecisionReason":"why","hookSpecificOutput":null}`, "", unreadableV("")},
		{"cp/case-folded decision", "copilot", 0, `{"permissionDecision":"deny","permissionDecisionReason":"why","PermissionDecision":"allow"}`, "", unreadableV("")},
		{"cp/case-folded reason", "copilot", 0, `{"permissionDecision":"deny","permissionDecisionReason":"why","permissionDecisionreason":""}`, "", unreadableV("")},
		{"cp/case-folded envelope", "copilot", 0, `{"permissionDecision":"deny","permissionDecisionReason":"why","HookSpecificOutput":{}}`, "", unreadableV("")},
		{"cx/case-folded envelope", "codex", 0, `{"HookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"note"}}`, "", unreadableV("")},
		{"cx/case-folded top-level decision", "codex", 0, strings.Replace(cxDeny, `"why"}}`, `"why"},"PERMISSIONDECISION":"allow"}`, 1), "", unreadableV("")},
		{"cx/case-folded inner decision", "codex", 0, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"why","permissiondecision":"allow"}}`, "", unreadableV("")},
		{"cx/case-folded inner context", "codex", 0, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","AdditionalContext":"note"}}`, "", unreadableV("")},
		{"cx/case-folded event name", "codex", 0, `{"hookSpecificOutput":{"hookeventname":"PostToolUse","hookEventName":"PreToolUse","additionalContext":"note"}}`, "", unreadableV("")},
		{"cx/spaced empty object", "codex", 0, "{ }", "", unreadableV("")},
		{"cx/split empty object", "codex", 0, "{\n}", "", unreadableV("")},
		{"cx/padded empty object", "codex", 0, " \n{}\r\n", "", allowV("")},

		{"ag/allow", "antigravity", 0, `{"decision":"allow"}` + "\n", "", allowV("")},
		{"ag/fault body", "antigravity", 0, `{"decision":"allow"}` + "\n", "fault\n", allowV("fault\n")},
		{"ag/deny", "antigravity", 0, `{"decision":"deny","reason":"why"}` + "\n", "", hosttest.Verdict{Decision: hosttest.Deny, Reason: "why"}},
		{"ag/exit 1", "antigravity", 1, `{"decision":"allow"}`, "", unreadableV("")},
		{"ag/empty stdout", "antigravity", 0, "", "", unreadableV("")},
		{"ag/empty object", "antigravity", 0, "{}", "", unreadableV("")},
		{"ag/allow with extra key", "antigravity", 0, `{"decision":"allow","reason":"why"}`, "", unreadableV("")},
		{"ag/deny empty reason", "antigravity", 0, `{"decision":"deny","reason":""}`, "", unreadableV("")},
		{"ag/deny missing reason", "antigravity", 0, `{"decision":"deny"}`, "", unreadableV("")},
		{"ag/deny extra key", "antigravity", 0, `{"decision":"deny","reason":"why","extra":1}`, "", unreadableV("")},
		{"ag/case-folded decision", "antigravity", 0, `{"Decision":"allow"}`, "", unreadableV("")},
		{"ag/case-folded reason", "antigravity", 0, `{"decision":"deny","Reason":"why"}`, "", unreadableV("")},
		{"ag/copilot decision", "antigravity", 0, cpDeny, "", unreadableV("")},
		{"ag/codex envelope", "antigravity", 0, cxDeny, "", unreadableV("")},
		{"ag/invalid json", "antigravity", 0, "{", "", unreadableV("")},
		{"ag/second document", "antigravity", 0, `{"decision":"allow"}{"decision":"allow"}`, "", unreadableV("")},
	}
	runReadCases(t, cases)
	mustPanic(t, "cursor", func() { hosttest.Read("cursor", 0, nil, nil) })
}

func runReadCases(t *testing.T, cases []readCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hosttest.Read(tc.surface, tc.exit, []byte(tc.stdout), []byte(tc.stderr))
			if got != tc.want {
				t.Fatalf("Read(%s, %d, %q, %q) = %+v, want %+v", tc.surface, tc.exit, tc.stdout, tc.stderr, got, tc.want)
			}
		})
	}
}

func TestVerdictSilent(t *testing.T) {
	for _, tc := range []struct {
		verdict hosttest.Verdict
		want    bool
	}{
		{hosttest.Verdict{Decision: hosttest.Allow}, true},
		{hosttest.Verdict{Decision: hosttest.Allow, Context: "c"}, false},
		{hosttest.Verdict{Decision: hosttest.Allow, Stderr: "s"}, false},
		{hosttest.Verdict{Decision: hosttest.Deny}, false},
		{hosttest.Verdict{Decision: hosttest.Unreadable}, false},
	} {
		if got := tc.verdict.Silent(); got != tc.want {
			t.Errorf("%+v.Silent() = %v, want %v", tc.verdict, got, tc.want)
		}
	}
	for _, stdout := range []string{"", "\n"} {
		for _, stderr := range []string{"", " ", "x\n"} {
			if hosttest.Read("claude-code", 1, []byte(stdout), []byte(stderr)).Silent() {
				t.Errorf("claude-code exit 1 stdout=%q stderr=%q read as silent", stdout, stderr)
			}
		}
	}
}

// TestReadDeniedResultContracts carries the cases the cmd/hook deny readers
// pinned: a Claude Code deny needs exit 2 and a non-blank stderr, a JSON deny
// needs exit 0, and anything that is not one deny document is unreadable.
func TestReadDeniedResultContracts(t *testing.T) {
	cases := []readCase{
		{"copilot allow with deny note", "copilot", 0, `{"permissionDecision":"allow","note":"deny"}`, "", unreadableV("")},
		{"claude deny", "claude-code", 2, "", "reason", hosttest.Verdict{Decision: hosttest.Deny, Reason: "reason", Stderr: "reason"}},
		{"claude exit 0 is not a deny", "claude-code", 0, "", "reason", allowV("reason")},
		{"claude blank reason", "claude-code", 2, "", " \n\t", unreadableV(" \n\t")},
	}
	for _, surface := range []surface.ID{surface.Copilot, surface.Codex} {
		body := `{"permissionDecision":"deny","permissionDecisionReason":"why"}`
		wrongNesting := `{"hookSpecificOutput":{"permissionDecision":"deny","permissionDecisionReason":"why"}}`
		allow := `{"permissionDecision":"allow","note":"deny"}`
		if surface == "codex" {
			body = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"why"}}`
			wrongNesting = `{"permissionDecision":"deny","permissionDecisionReason":"why"}`
			allow = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"},"note":"deny"}`
		}
		cases = append(cases, readCase{string(surface) + "/deny", surface, 0, body, "", hosttest.Verdict{Decision: hosttest.Deny, Reason: "why"}})
		for _, tc := range []struct{ name, body string }{
			{"null", "null"},
			{"missing", `{"permissionDecisionReason":"why"}`},
			{"allow-with-deny-note", allow},
			{"wrong-nesting", wrongNesting},
			{"malformed", "{"},
			{"trailing", body + " trailing"},
			{"second-document", body + body},
			{"array", "[]"},
			{"wrong-field-type", `{"permissionDecision":false}`},
			{"nested-wrong-field-type", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":false}}`},
		} {
			cases = append(cases, readCase{string(surface) + "/" + tc.name, surface, 0, tc.body, "", unreadableV("")})
		}
		cases = append(cases, readCase{string(surface) + "/exit 1", surface, 1, body, "", unreadableV("")})
	}
	runReadCases(t, cases)
}

type recorded struct {
	Exit   int    `json:"exit"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

func readRecording(t *testing.T, root, file, key string) recorded {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "hooks", "go", "cmd", "hook", "testdata", "corpus", "recorded", file))
	if err != nil {
		t.Fatal(err)
	}
	var entries map[string]recorded
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&entries); err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	entry, ok := entries[key]
	if !ok {
		t.Fatalf("%s has no entry %q", file, key)
	}
	return entry
}

// TestReadRecordedResponses reads one recorded response of each kind per
// surface. No recording is unreadable on its own surface, so the unreadable
// row reads another surface's recorded bytes, which that host would not
// accept as a decision.
func TestReadRecordedResponses(t *testing.T) {
	root := hosttest.Checkout(t)
	const (
		deny    = "command-guardian/deny-destructive"
		silent  = "command-guardian/allowlist-empty-entries-are-skipped"
		context = "command-guardian/context-info-only-threat"
	)
	for _, tc := range []struct {
		name      string
		surface   surface.ID
		file, key string
		decision  hosttest.Decision
		check     func(hosttest.Verdict, recorded) bool
	}{
		{"claude-code/deny", "claude-code", "claude-code.pre-tool.json", deny, hosttest.Deny,
			func(v hosttest.Verdict, r recorded) bool { return v.Reason == r.Stderr && v.Reason != "" }},
		{"claude-code/silent", "claude-code", "claude-code.pre-tool.json", silent, hosttest.Allow,
			func(v hosttest.Verdict, r recorded) bool { return v.Silent() }},
		{"claude-code/context", "claude-code", "claude-code.pre-tool.json", context, hosttest.Allow,
			func(v hosttest.Verdict, r recorded) bool {
				return strings.HasPrefix(v.Context, "[commands] ") && v.Stderr == ""
			}},
		{"claude-code/fault", "claude-code", "claude-code.json", "claude-code/invalid-utf8", hosttest.Allow,
			func(v hosttest.Verdict, r recorded) bool { return v.Stderr == r.Stderr && !v.Silent() }},
		{"claude-code/unreadable", "claude-code", "codex.pre-tool.json", silent, hosttest.Unreadable,
			func(v hosttest.Verdict, r recorded) bool { return true }},
		{"copilot/deny", "copilot", "copilot.pre-tool.json", deny, hosttest.Deny,
			func(v hosttest.Verdict, r recorded) bool { return strings.HasPrefix(v.Reason, "[commands] ") }},
		{"copilot/silent", "copilot", "copilot.pre-tool.json", silent, hosttest.Allow,
			func(v hosttest.Verdict, r recorded) bool { return v.Silent() }},
		{"copilot/context", "copilot", "copilot.pre-tool.json", context, hosttest.Allow,
			func(v hosttest.Verdict, r recorded) bool {
				return v.Context == "" && v.Stderr == r.Stderr && !v.Silent()
			}},
		{"copilot/unreadable", "copilot", "codex.pre-tool.json", deny, hosttest.Unreadable,
			func(v hosttest.Verdict, r recorded) bool { return true }},
		{"codex/deny", "codex", "codex.pre-tool.json", deny, hosttest.Deny,
			func(v hosttest.Verdict, r recorded) bool { return strings.HasPrefix(v.Reason, "[commands] ") }},
		{"codex/silent", "codex", "codex.pre-tool.json", silent, hosttest.Allow,
			func(v hosttest.Verdict, r recorded) bool { return v.Silent() }},
		{"codex/context", "codex", "codex.pre-tool.json", context, hosttest.Allow,
			func(v hosttest.Verdict, r recorded) bool { return strings.HasPrefix(v.Context, "[commands] ") }},
		{"codex/unreadable", "codex", "copilot.pre-tool.json", deny, hosttest.Unreadable,
			func(v hosttest.Verdict, r recorded) bool { return true }},
		{"antigravity/deny", "antigravity", "antigravity.pre-tool.json", deny, hosttest.Deny,
			func(v hosttest.Verdict, r recorded) bool { return strings.HasPrefix(v.Reason, "[commands] ") }},
		{"antigravity/silent", "antigravity", "antigravity.pre-tool.json", silent, hosttest.Allow,
			func(v hosttest.Verdict, r recorded) bool { return v.Silent() }},
		{"antigravity/context", "antigravity", "antigravity.pre-tool.json", context, hosttest.Allow,
			func(v hosttest.Verdict, r recorded) bool { return v.Context == "" && v.Silent() }},
		{"antigravity/unreadable", "antigravity", "codex.pre-tool.json", deny, hosttest.Unreadable,
			func(v hosttest.Verdict, r recorded) bool { return true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := readRecording(t, root, tc.file, tc.key)
			got := hosttest.Read(tc.surface, r.Exit, []byte(r.Stdout), []byte(r.Stderr))
			if got.Decision != tc.decision || got.Stderr != r.Stderr || !tc.check(got, r) {
				t.Fatalf("Read(%s) of %s %s = %+v, want decision %v", tc.surface, tc.file, tc.key, got, tc.decision)
			}
		})
	}
}
