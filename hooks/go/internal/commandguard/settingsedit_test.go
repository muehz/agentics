package commandguard

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/settings/settingstest"
)

func inputJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestIsSettingsPath(t *testing.T) {
	for _, path := range []string{
		".agentics/hooks.json", "./.agentics/hooks.json", "/repo/.agentics/hooks.json", "~/.agentics/hooks.json",
		`.agentics\hooks.json`, `C:\Users\me\repo\.agentics\hooks.json`, ".AGENTICS/Hooks.JSON", " .agentics/hooks.json ",
		".agentics/hooks.json.", ".agentics/hooks.json  ",
		"/repo/.agentics/./hooks.json", "/repo/.agentics//hooks.json", "/repo/.agentics/x/../hooks.json", ".agentics/../.agentics/hooks.json",
		// Windows drops the trailing dots and spaces of every component.
		`C:\repo\.agentics.\hooks.json`, ".agentics./hooks.json", ".agentics /hooks.json", ".agentics. ./hooks.json",
		"/repo/.agentics./hooks.json.", "/repo/.agentics /hooks.json ", "repo ./.agentics/hooks.json", "/repo/.agentics./hooks.json  ",
	} {
		if !isSettingsPath(path) {
			t.Errorf("isSettingsPath(%q) = false", path)
		}
	}
	for _, path := range []string{
		"", "hooks.json", ".agentics/other.json", ".agentics/hooks.json.bak", "x.agentics/hooks.json",
		"docs/.agentics-notes/hooks.json", ".agentics/hooks.jsonl", ".agentics", "agentics/hooks.json",
		"/repo/.agentics/../hooks.json",
		"/repo/.agentics./../hooks.json", ".agentics./other.json", "x.agentics./hooks.json", "../hooks.json",
	} {
		if isSettingsPath(path) {
			t.Errorf("isSettingsPath(%q) = true", path)
		}
	}
}

func TestWriteToolsCannotTargetASettingsFile(t *testing.T) {
	for _, c := range []struct {
		name, tool string
		input      map[string]any
	}{
		{"claude Write", "Write", map[string]any{"file_path": "/repo/.agentics/hooks.json", "content": "{}"}},
		{"claude Edit", "Edit", map[string]any{"file_path": "/repo/.agentics/hooks.json", "old_string": "a", "new_string": "b"}},
		{"claude MultiEdit", "MultiEdit", map[string]any{"file_path": "/repo/.agentics/hooks.json", "edits": []any{}}},
		{"claude NotebookEdit", "NotebookEdit", map[string]any{"notebook_path": "/repo/.agentics/hooks.json"}},
		{"windows separators", "Write", map[string]any{"file_path": `C:\Users\me\repo\.agentics\hooks.json`, "content": "{}"}},
		{"trailing dot on the directory", "Write", map[string]any{"file_path": `C:\repo\.agentics.\hooks.json`, "content": "{}"}},
		{"trailing space on the directory", "Edit", map[string]any{"file_path": "/repo/.agentics /hooks.json", "old_string": "a", "new_string": "b"}},
		{"trailing dot on the file", "Write", map[string]any{"file_path": "/repo/.agentics/hooks.json.", "content": "{}"}},
		{"trailing space on the file", "Write", map[string]any{"file_path": "/repo/.agentics/hooks.json ", "content": "{}"}},
		{"user file", "Write", map[string]any{"file_path": "~/.agentics/hooks.json", "content": "{}"}},
		{"mixed case", "Write", map[string]any{"file_path": "/repo/.AGENTICS/Hooks.JSON", "content": "{}"}},
		{"copilot path under value", "Edit", map[string]any{"value": map[string]any{"path": "/repo/.agentics/hooks.json"}}},
		{"copilot patch files", "Edit", map[string]any{"value": map[string]any{"files": []any{
			map[string]any{"path": "notes.md", "content": "x"},
			map[string]any{"path": "/repo/.agentics/hooks.json", "content": "{}"},
		}}}},
		{"copilot patch files under value", "Edit", map[string]any{"command": "*** Begin Patch\n*** Update File: x\n*** End Patch", "value": map[string]any{"files": []any{
			map[string]any{"path": ".agentics/hooks.json", "content": "{}"},
		}}}},
		{"a benign top-level list beside a value list", "Edit", map[string]any{
			"files": []any{map[string]any{"path": "notes.md", "content": "x"}},
			"value": map[string]any{"files": []any{map[string]any{"path": ".agentics/hooks.json", "content": "{}"}}},
		}},
		{"a benign top-level edit list beside a value list", "MultiEdit", map[string]any{
			"file_path": "notes.md",
			"edits":     []any{map[string]any{"file_path": "notes.md", "old_string": "a", "new_string": "b"}},
			"value":     map[string]any{"edits": []any{map[string]any{"file_path": ".agentics/hooks.json", "old_string": "a", "new_string": "b"}}},
		}},
		{"codex patch files", "Edit", map[string]any{"command": "*** Begin Patch\n*** Update File: x\n*** End Patch", "files": []any{
			map[string]any{"path": ".agentics/hooks.json", "content": "{}"},
		}}},
		{"item path in an edit list", "MultiEdit", map[string]any{"file_path": "notes.md", "edits": []any{
			map[string]any{"file_path": ".agentics/hooks.json", "old_string": "a", "new_string": "b"},
		}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _, logRoot := runGo(t, tableCase{tool: c.tool, input: inputJSON(t, c.input), settings: settingstest.Policy(settings.Keys...).Source()})
			if got.Kind() != "deny" {
				t.Fatalf("kind = %s, reason %q", got.Kind(), got.Payload())
			}
			if !strings.HasPrefix(got.Payload(), "1 blocking threat(s) detected: hook_settings_edit - Do not edit the Agentics hook settings files.") {
				t.Fatalf("reason = %q", got.Payload())
			}
			if strings.Contains(got.Payload(), "Setting:") {
				t.Fatalf("a fixed rule advertised a setting: %q", got.Payload())
			}
			if _, err := os.Stat(guardLogPath(logRoot)); err != nil {
				t.Fatalf("the denial left no guard.log: %v", err)
			}
		})
	}
}

func TestAnAllowlistedSubstringCannotVouchForASettingsWrite(t *testing.T) {
	got, _, _ := runGo(t, tableCase{
		tool:  "Write",
		input: inputJSON(t, map[string]any{"file_path": "/repo/.agentics/hooks.json", "content": "{}"}),
		env:   map[string]string{"COMMAND_GUARD_ALLOWLIST": "hooks.json"},
	})
	if got.Kind() != "deny" {
		t.Fatalf("kind = %s", got.Kind())
	}
}

func TestOtherWritesAndReadsOfSimilarPathsPass(t *testing.T) {
	for _, c := range []struct {
		name, tool string
		input      map[string]any
	}{
		{"another json file", "Write", map[string]any{"file_path": "/repo/.agentics/other.json", "content": "{}"}},
		{"a backup name", "Write", map[string]any{"file_path": "/repo/.agentics/hooks.json.bak", "content": "{}"}},
		{"no directory", "Write", map[string]any{"file_path": "/repo/hooks.json", "content": "{}"}},
		{"a similar directory", "Write", map[string]any{"file_path": "/repo/docs/.agentics-notes/hooks.json", "content": "{}"}},
		{"content that mentions the path", "Write", map[string]any{"file_path": "/repo/notes.md", "content": "edit .agentics/hooks.json to change this"}},
		{"a read", "Read", map[string]any{"file_path": "/repo/.agentics/hooks.json"}},
		{"multiedit content is not scanned", "MultiEdit", map[string]any{"file_path": "/repo/a.go", "edits": []any{
			map[string]any{"old_string": "a", "new_string": "rm -rf /"},
		}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _, _ := runGo(t, tableCase{tool: c.tool, input: inputJSON(t, c.input)})
			if got.Kind() != "pass" {
				t.Fatalf("kind = %s, reason %q", got.Kind(), got.Payload())
			}
		})
	}
}
