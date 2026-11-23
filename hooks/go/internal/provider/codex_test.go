package provider

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCodexPatchGrammarThroughNormalize(t *testing.T) {
	pinCodexRuntime(t)
	for _, tc := range []struct {
		name, body string
		valid      bool
		files      []any
	}{
		{"replacement with EOF", "*** Update File: app.txt\n@@\n context\n-old\n+new\n*** End of File", true, []any{map[string]any{"path": "app.txt", "content": "new\n"}}},
		{"added file trailing newline", "*** Add File: app.txt\n+new", true, []any{map[string]any{"path": "app.txt", "content": "new\n"}}},
		{"empty environment ID", "*** Environment ID: \n*** Add File: app.txt\n+new", false, nil},
		{"empty add path", "*** Add File: \n+new", false, nil},
		{"empty update path", "*** Update File: \n@@\n+new", false, nil},
		{"empty delete path", "*** Delete File: ", false, nil},
		{"empty hunk before add", "*** Update File: app.txt\n@@\n*** Add File: next.txt\n+new", false, nil},
		{"empty hunk before update", "*** Update File: app.txt\n@@\n*** Update File: next.txt\n@@\n+new", false, nil},
		{"empty hunk before delete", "*** Update File: app.txt\n@@\n*** Delete File: next.txt", false, nil},
		{"empty hunk before hunk", "*** Update File: app.txt\n@@\n@@\n+new", false, nil},
		{"empty hunk before EOF", "*** Update File: app.txt\n@@\n*** End of File", false, nil},
		{"empty hunk before end", "*** Update File: app.txt\n@@", false, nil},
		{"move after add", "*** Add File: app.txt\n*** Move to: next.txt\n+new", false, nil},
		{"empty move destination", "*** Update File: app.txt\n*** Move to: \n@@\n+new", false, nil},
		{"orphan content", "+new", false, nil},
		{"unknown directive", "*** Unexpected: app.txt", false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := "*** Begin Patch\n" + tc.body + "\n*** End Patch\n"
			payload, err := json.Marshal(map[string]any{
				"hook_event_name": "PreToolUse", "cwd": "/c", "tool_name": "apply_patch",
				"tool_input": map[string]any{"command": command, "path": "forged.txt", "file_path": "forged.txt", "files": []any{map[string]any{"path": "forged.txt", "content": "forged"}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			event, rejected := Normalize("codex", "PreToolUse", payload)
			if rejected != nil {
				t.Fatalf("Normalize rejected patch envelope: %s", rejected.Stderr)
			}
			doc := readEvent(t, event)
			if doc.Tool.Name == nil || *doc.Tool.Name != "Edit" {
				t.Fatalf("tool name = %v, want Edit", doc.Tool.Name)
			}
			input := eventInput(t, doc)
			want := map[string]any{"command": command, "value": map[string]any{"command": command}}
			if tc.valid {
				want["value"].(map[string]any)["files"] = tc.files
			} else {
				want["write_parse_error"] = true
			}
			if !reflect.DeepEqual(input, want) {
				t.Fatalf("normalized input = %#v, want %#v", input, want)
			}
		})
	}
}

// A valid patch that adds no text writes no file, and says so with an empty
// file list: never a null list, never an absent key, and never a parse error.
func TestCodexPatchWithoutAddedTextHasEmptyFileList(t *testing.T) {
	pinCodexRuntime(t)
	for _, tc := range []struct{ name, body string }{
		{"delete only", "*** Delete File: x"},
		{"move without hunk", "*** Update File: x\n*** Move to: y"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := "*** Begin Patch\n" + tc.body + "\n*** End Patch\n"
			payload, err := json.Marshal(map[string]any{
				"hook_event_name": "PreToolUse", "cwd": "/c", "tool_name": "apply_patch",
				"tool_input": map[string]any{"command": command},
			})
			if err != nil {
				t.Fatal(err)
			}
			event, rejected := Normalize("codex", "PreToolUse", payload)
			if rejected != nil {
				t.Fatalf("Normalize rejected patch envelope: %s", rejected.Stderr)
			}
			input := eventInput(t, readEvent(t, event))
			want := map[string]any{"command": command, "value": map[string]any{"command": command, "files": []any{}}}
			if !reflect.DeepEqual(input, want) {
				t.Fatalf("normalized input = %#v, want %#v", input, want)
			}
		})
	}
}

// Fixed runtime roots keep normalization independent of local installations.
const (
	codexPinnedPluginRoot = "/tmp/agentics-codex-golden-plugin-root"
	codexPinnedPluginData = "/tmp/agentics-codex-golden-plugin-data"
	codexPinnedVault      = "/tmp/agentics-codex-golden-vault"
)

func pinCodexRuntime(t *testing.T) {
	t.Helper()
	t.Setenv("PLUGIN_ROOT", codexPinnedPluginRoot)
	t.Setenv("PLUGIN_DATA", codexPinnedPluginData)
	t.Setenv("AGX_VAULT", codexPinnedVault)
}

// TestCodexNormalizeOptionalFields proves the minimal payload is accepted,
// that an absent optional field is null, and that optional metadata is validated.
func TestCodexNormalizeOptionalFields(t *testing.T) {
	pinCodexRuntime(t)
	t.Run("minimal PreToolUse payload", func(t *testing.T) {
		event, rendered := Normalize("codex", "PreToolUse",
			[]byte(`{"hook_event_name":"PreToolUse","cwd":"/c","tool_name":"exec_command","tool_input":{"command":"ls"}}`))
		if rendered != nil {
			t.Fatalf("rejected: %s", rendered.Stderr)
		}
		doc := readEvent(t, event)
		if doc.SessionID != nil || doc.TurnID != nil {
			t.Errorf("session/turn = %v/%v, want null/null", doc.SessionID, doc.TurnID)
		}
		if doc.Tool.Name == nil || *doc.Tool.Name != "Bash" {
			t.Errorf("tool.name = %v, want the exec_command->Bash mapping", doc.Tool.Name)
		}
		input := eventInput(t, doc)
		if input["command"] != "ls" {
			t.Errorf("command = %v, want ls", input["command"])
		}
		if _, has := input["tool_use_id"]; has {
			t.Errorf("tool_use_id = %v, want none on PreToolUse", input["tool_use_id"])
		}
	})
	t.Run("any permission_mode string passes through", func(t *testing.T) {
		event, rendered := Normalize("codex", "PreToolUse",
			[]byte(`{"hook_event_name":"PreToolUse","cwd":"/c","tool_name":"Bash","tool_input":{},"permission_mode":"workspace-write"}`))
		if rendered != nil {
			t.Fatalf("rejected: %s", rendered.Stderr)
		}
		readEvent(t, event)
	})
	t.Run("permission_mode retains deferred validation", func(t *testing.T) {
		payload := `{"hook_event_name":"PreToolUse","cwd":"/c","tool_name":"Bash","tool_input":{},"permission_mode":"\u0000"}`
		event, rendered := Normalize("codex", "PreToolUse", []byte(payload))
		if rendered != nil {
			t.Fatalf("NUL must reach handler validation: %s", rendered.Stderr)
		}
		if _, err := event.Read(); err == nil {
			t.Fatal("permission_mode bypassed NUL validation")
		}
	})
}
