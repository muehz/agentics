package provider

import (
	"testing"
)

// pinClaudeRuntime pins the three environment variables claudeNormalize
// reads, so a test never inherits a real plugin installation's values.
func pinClaudeRuntime(t *testing.T, pluginRoot, dataRoot, vault string) {
	t.Helper()
	t.Setenv("CLAUDE_PLUGIN_ROOT", pluginRoot)
	t.Setenv("CLAUDE_PLUGIN_DATA", dataRoot)
	t.Setenv("AGX_VAULT", vault)
}

// An unknown key inside tool_input is payload, not schema, and survives.
func TestClaudeNormalizeUnknownKeyInsideToolInputSurvives(t *testing.T) {
	pinClaudeRuntime(t, "", "", "")
	payload := []byte(`{
		"hook_event_name": "PreToolUse",
		"tool_name": "Bash",
		"tool_input": {"command": "true", "unsupported_future_field": {"kept": 1}}
	}`)
	event, malformed := Normalize("claude-code", "PreToolUse", payload)
	if malformed != nil {
		t.Fatalf("unexpected rejection: %q", malformed.Stderr)
	}
	doc := readEvent(t, event)
	input := eventInput(t, doc)
	if kept, ok := input["unsupported_future_field"].(map[string]any); !ok || kept["kept"] != float64(1) {
		t.Errorf("tool.input.unsupported_future_field = %v, want preserved", input["unsupported_future_field"])
	}
	if doc.SessionID != nil || doc.TurnID != nil || doc.Cwd != nil {
		t.Errorf("absent identifiers = %v/%v/%v, want nulls", doc.SessionID, doc.TurnID, doc.Cwd)
	}
}
