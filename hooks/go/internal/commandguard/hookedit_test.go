package commandguard

import (
	"os"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/settings/settingstest"
)

const hookEditReason = "1 blocking threat(s) detected: git_hook_bypass - Do not edit, empty, move or delete the Git hook files"

func TestHookPathPredicates(t *testing.T) {
	for _, p := range []string{
		".git/config", "/repo/.git/config", `C:\repo\.GIT\Config`, "./.git/config",
		".git/hooks/pre-commit", "/repo/.git/hooks/pre-push", ".husky/pre-commit", "/repo/.husky/_/husky.sh",
		`.husky\pre-commit`, ".pre-commit-config.yaml", "/repo/.pre-commit-config.yaml", ".Pre-Commit-Config.YAML",
	} {
		if !isHookFilePath(p) {
			t.Errorf("isHookFilePath(%q) = false", p)
		}
	}
	for _, p := range []string{
		"", ".git/hooks", ".husky", "config", ".git/config.bak", "docs/.git-notes/config", ".git/hooks.sample",
		"husky/pre-commit", ".husky-notes/pre-commit", "pre-commit-config.yaml", ".pre-commit-config.yml", "/repo/.git/HEAD",
	} {
		if isHookFilePath(p) {
			t.Errorf("isHookFilePath(%q) = true", p)
		}
	}
	for _, p := range []string{".husky", "/repo/.husky/", ".git/hooks", `C:\repo\.git\hooks`} {
		if !isHookDirectory(p) {
			t.Errorf("isHookDirectory(%q) = false", p)
		}
	}
	for _, p := range []string{"", ".husky/pre-commit", ".git", ".git/hooks/pre-commit", "husky"} {
		if isHookDirectory(p) {
			t.Errorf("isHookDirectory(%q) = true", p)
		}
	}
}

func TestAddsHooksPath(t *testing.T) {
	for _, c := range []struct {
		name, text, old string
		want            bool
	}{
		{"assignment", "hooksPath = /tmp/h", "", true},
		{"no spaces", "hookspath=/tmp/h", "", true},
		{"header and key", "[core]\n\thooksPath = .husky/_\n", "", true},
		{"key on the header line", "[core] hooksPath = x", "", true},
		{"upper case key", "HOOKSPATH = x", "", true},
		{"quoted value", `hooksPath = "/tmp/h"`, "", true},
		{"leading byte order mark", "\xef\xbb\xbf[core] hooksPath = /tmp/h", "", true},
		{"crlf", "[core]\r\n\thooksPath = x\r\n", "", true},
		{"changed value", "\thooksPath = y\n", "\thooksPath = x\n", true},
		{"other key", "[core]\n\tfilemode = false\n", "", false},
		{"prefix of another key", "myhookspath = x", "", false},
		{"comments", "# hooksPath = x\n; hooksPath = y", "", false},
		{"removal", "[core]\n", "[core]\n\thooksPath = x\n", false},
		{"repeated from the old text", "\thooksPath = x\n\tfilemode = true\n", "\thooksPath = x\n\tfilemode = false\n", false},
		{"empty", "", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := addsHooksPath(c.text, c.old); got != c.want {
				t.Fatalf("addsHooksPath = %v, want %v", got, c.want)
			}
		})
	}
}

func TestToolCallsThatBypassTheGitHooksAreDenied(t *testing.T) {
	for _, c := range []struct {
		name, tool string
		input      map[string]any
	}{
		{"write sets hooksPath", "Write", map[string]any{"file_path": ".git/config", "content": "[core]\n\thooksPath = /tmp/h\n"}},
		{"write with an absolute path", "Write", map[string]any{"file_path": "/repo/.git/config", "content": "hookspath=/tmp/h"}},
		{"edit adds hooksPath", "Edit", map[string]any{"file_path": ".git/config", "old_string": "\tfilemode = true\n", "new_string": "\tfilemode = true\n\thooksPath = /tmp/h\n"}},
		{"edit changes the value", "Edit", map[string]any{"file_path": ".git/config", "old_string": "\thooksPath = .husky/_\n", "new_string": "\thooksPath = /tmp/h\n"}},
		{"multiedit second replacement", "MultiEdit", map[string]any{"file_path": ".git/config", "edits": []any{
			map[string]any{"old_string": "a", "new_string": "b"},
			map[string]any{"old_string": "c", "new_string": "[core]\n\thooksPath = /tmp/h\n"},
		}}},
		{"codex patch files", "Edit", map[string]any{"command": "*** Begin Patch\n*** Update File: .git/config\n*** End Patch", "files": []any{
			map[string]any{"path": ".git/config", "content": "[core]\n\thooksPath = /tmp/h\n"},
		}}},
		{"copilot value envelope", "Edit", map[string]any{"value": map[string]any{"path": ".git/config", "old_str": "x", "new_str": "hooksPath = /tmp/h"}}},
		{"notebook edit", "NotebookEdit", map[string]any{"notebook_path": ".git/config", "new_source": "hooksPath = /tmp/h"}},
		{"empty write to a husky hook", "Write", map[string]any{"file_path": ".husky/pre-commit", "content": ""}},
		{"whitespace write to a git hook", "Write", map[string]any{"file_path": ".git/hooks/pre-commit", "content": " \n\t\n"}},
		{"empty write to the pre-commit config", "Write", map[string]any{"file_path": ".pre-commit-config.yaml", "content": ""}},
		{"empty write to git config", "Write", map[string]any{"file_path": ".git/config", "content": ""}},
		{"copilot create with empty file_text", "Write", map[string]any{"path": ".husky/pre-commit", "file_text": ""}},
		{"copilot empty write under the value envelope", "Write", map[string]any{"value": map[string]any{"path": ".husky/pre-commit", "file_text": ""}}},
		{"windows path", "Write", map[string]any{"file_path": `C:\repo\.husky\pre-commit`, "content": ""}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _, logRoot := runGo(t, tableCase{tool: c.tool, input: inputJSON(t, c.input), settings: settingstest.Policy(settings.Keys...).Source()})
			if got.Kind() != "deny" {
				t.Fatalf("kind = %s, reason %q", got.Kind(), got.Payload())
			}
			if !strings.HasPrefix(got.Payload(), hookEditReason) {
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

func TestToolCallsThatLeaveTheGitHooksWorkingPass(t *testing.T) {
	for _, c := range []struct {
		name, tool string
		input      map[string]any
	}{
		{"write a hook with content", "Write", map[string]any{"file_path": ".husky/pre-commit", "content": "npx lint-staged\n"}},
		{"create a new husky hook", "Write", map[string]any{"file_path": ".husky/post-merge", "content": "npm ci\n"}},
		{"edit empties a fragment", "Edit", map[string]any{"file_path": ".husky/pre-commit", "old_string": "npx a\n", "new_string": ""}},
		{"edit removes a hooksPath line", "Edit", map[string]any{"file_path": ".git/config", "old_string": "\thooksPath = x\n", "new_string": ""}},
		{"edit repeats an existing hooksPath line", "Edit", map[string]any{"file_path": ".git/config", "old_string": "\thooksPath = x\n\tfilemode = false\n", "new_string": "\thooksPath = x\n\tfilemode = true\n"}},
		{"multiedit empties a fragment", "MultiEdit", map[string]any{"file_path": ".husky/pre-commit", "edits": []any{map[string]any{"old_string": "a", "new_string": ""}}}},
		{"git config without hooksPath", "Write", map[string]any{"file_path": ".git/config", "content": "[core]\n\tfilemode = false\n"}},
		{"hooksPath in another file", "Write", map[string]any{"file_path": "notes.md", "content": "hooksPath = x"}},
		{"hooksPath in another git file", "Write", map[string]any{"file_path": ".git/info/exclude", "content": "hooksPath = x"}},
		{"a comment that names hooksPath", "Write", map[string]any{"file_path": ".git/config", "content": "# hooksPath = x\n"}},
		{"empty write elsewhere", "Write", map[string]any{"file_path": "empty.txt", "content": ""}},
		{"codex patch to a hook", "Edit", map[string]any{"files": []any{map[string]any{"path": ".husky/pre-commit", "content": "npx lint-staged\n"}}}},
		{"whitespace added by a patch", "Edit", map[string]any{"files": []any{map[string]any{"path": ".husky/pre-commit", "content": "\n"}}}},
		{"an Edit never matches the empty-write item", "Edit", map[string]any{"file_path": ".husky/pre-commit", "content": ""}},
		{"a read of a hook", "Read", map[string]any{"file_path": ".husky/pre-commit"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _, _ := runGo(t, tableCase{tool: c.tool, input: inputJSON(t, c.input)})
			if got.Kind() != "pass" {
				t.Fatalf("kind = %s, reason %q", got.Kind(), got.Payload())
			}
		})
	}
}

func TestHookEditFollowsTheCommandRuleControls(t *testing.T) {
	write := inputJSON(t, map[string]any{"file_path": ".git/config", "content": "hooksPath = /tmp/h"})
	for _, c := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"an allowlist entry cannot vouch for a write-tool form", map[string]string{"COMMAND_GUARD_ALLOWLIST": "hooksPath"}, "deny"},
		{"skip lifts the rule", map[string]string{"SKIP_COMMAND_GUARD": "true"}, "pass"},
		{"warn mode advises", map[string]string{"COMMAND_GUARD_MODE": "warn"}, "context"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _, _ := runGo(t, tableCase{tool: "Write", input: write, env: c.env, settings: settingstest.Policy(settings.Keys...).Source()})
			if got.Kind() != c.want {
				t.Fatalf("kind = %s, want %s, reason %q", got.Kind(), c.want, got.Payload())
			}
		})
	}
}
