package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

var hookEditSurfaces = surface.All()

// hookEditCall is one file-writing tool call. build returns the call one surface sends for it, and false when that
// surface's tools cannot express the form.
type hookEditCall struct {
	name  string
	want  string
	build func(surface, root string) (hosttest.Call, bool)
}

// hookEditPatch is a one-file Codex patch call. Codex edits files through apply_patch, which no neutral tool names.
func hookEditPatch(root, patch string) hosttest.Call {
	return hosttest.Call{Tool: "apply_patch", Input: map[string]any{"command": patch}, Cwd: root, Native: true}
}

// hookEditCodexUpdate sends a one-file patch in which removed lines carry a minus sign and added lines a plus sign.
func hookEditCodexUpdate(root, path string, removed, added []string) hosttest.Call {
	patch := "*** Begin Patch\n*** Update File: " + path + "\n@@\n"
	for _, line := range removed {
		patch += "-" + line + "\n"
	}
	for _, line := range added {
		patch += "+" + line + "\n"
	}
	patch += "*** End Patch"
	return hookEditPatch(root, patch)
}

func hookEditLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// hookEditWrite is a whole-file write. Codex edits files through apply_patch only, so it cannot express one. Copilot's
// create names its body file_text, which the neutral Write does not, so that call is native.
func hookEditWrite(path, content string) func(string, string) (hosttest.Call, bool) {
	return func(surface, root string) (hosttest.Call, bool) {
		switch surface {
		case "antigravity", "claude-code":
			return hosttest.Call{Tool: "Write", Input: map[string]any{"file_path": path, "content": content}, Cwd: root}, true
		case "copilot":
			return hosttest.Call{Tool: "create", Input: map[string]any{"path": path, "file_text": content}, Cwd: root, Native: true}, true
		}
		return hosttest.Call{}, false
	}
}

// hookEditAdd writes non-empty content on every surface: a Write, a create, and a patch that adds the lines.
func hookEditAdd(path, content string) func(string, string) (hosttest.Call, bool) {
	write := hookEditWrite(path, content)
	return func(surface, root string) (hosttest.Call, bool) {
		if surface == "codex" {
			return hookEditCodexUpdate(root, path, nil, hookEditLines(content)), true
		}
		return write(surface, root)
	}
}

// hookEditCodexAdd sends a patch that creates the file with the given lines. Claude Code and Copilot use the same
// whole-file write as hookEditAdd.
func hookEditCodexAdd(path, content string) func(string, string) (hosttest.Call, bool) {
	write := hookEditWrite(path, content)
	return func(surface, root string) (hosttest.Call, bool) {
		if surface != "codex" {
			return write(surface, root)
		}
		patch := "*** Begin Patch\n*** Add File: " + path + "\n"
		for _, line := range hookEditLines(content) {
			patch += "+" + line + "\n"
		}
		patch += "*** End Patch"
		return hookEditPatch(root, patch), true
	}
}

// hookEditReplace replaces oldText with newText. A Codex patch has no paired old text, so a form that depends on the
// pairing sets codex to false.
func hookEditReplace(path, oldText, newText string, codex bool) func(string, string) (hosttest.Call, bool) {
	return func(surface, root string) (hosttest.Call, bool) {
		if surface == "codex" {
			return hookEditCodexUpdate(root, path, hookEditLines(oldText), hookEditLines(newText)), codex
		}
		return hosttest.Call{Tool: "Edit", Input: map[string]any{"file_path": path, "old_string": oldText, "new_string": newText}, Cwd: root}, true
	}
}

// hookEditRead is a read of a path. Codex reads through its shell only.
func hookEditRead(path string) func(string, string) (hosttest.Call, bool) {
	return func(surface, root string) (hosttest.Call, bool) {
		if surface == "codex" {
			return hosttest.Call{}, false
		}
		return hosttest.Call{Tool: "Read", Input: map[string]any{"file_path": path}, Cwd: root}, true
	}
}

func hookEditCalls() []hookEditCall {
	const config = ".git/config"
	return []hookEditCall{
		{"write sets hooksPath", "deny", hookEditAdd(config, "[core]\n\thooksPath = /tmp/h\n")},
		{"write sets hooksPath without a header", "deny", hookEditAdd(config, "hookspath=/tmp/h\n")},
		{"edit adds hooksPath", "deny", hookEditReplace(config, "\tfilemode = true\n", "\tfilemode = true\n\thooksPath = /tmp/h\n", true)},
		{"edit changes the hooksPath value", "deny", hookEditReplace(config, "\thooksPath = .husky/_\n", "\thooksPath = /tmp/h\n", true)},
		{"empty write to a husky hook", "deny", hookEditWrite(".husky/pre-commit", "")},
		{"whitespace write to a git hook", "deny", hookEditWrite(".git/hooks/pre-commit", " \n\t\n")},
		{"empty write to the pre-commit config", "deny", hookEditWrite(".pre-commit-config.yaml", "")},
		{"empty write to git config", "deny", hookEditWrite(config, "")},
		{"whitespace write to a husky hook", "deny", hookEditWrite(".husky/pre-commit", "\n  \n")},
		{"empty write to a git hook", "deny", hookEditWrite(".git/hooks/pre-commit", "")},
		{"whitespace write to the pre-commit config", "deny", hookEditWrite(".pre-commit-config.yaml", " \n")},
		{"whitespace write to git config", "deny", hookEditWrite(config, "\t\n")},
		{"write sets hooksPath with no spaces after a header", "deny", hookEditAdd(config, "[core]\n\thooksPath=/tmp/h\n")},
		{"edit adds hooksPath in another case", "deny", hookEditReplace(config, "\tfilemode = true\n", "\tfilemode = true\n\tHOOKSPATH = /tmp/h\n", true)},
		{"write sets hooksPath on the header line", "deny", hookEditAdd(config, "[core] hooksPath=/tmp/h\n")},
		{"write sets a quoted hooksPath", "deny", hookEditAdd(config, "[core]\n\thooksPath = \"/tmp/h\"\n")},
		{"patch creates git config with hooksPath", "deny", hookEditCodexAdd(config, "[core]\n\thooksPath = /tmp/h\n")},
		{"write a hook with content", "pass", hookEditAdd(".husky/pre-commit", "npx lint-staged\n")},
		{"edit the pre-commit config", "pass", hookEditReplace(".pre-commit-config.yaml", "rev: v1\n", "rev: v2\n", true)},
		{"edit a husky hook", "pass", hookEditReplace(".husky/pre-commit", "npx a\n", "npx b\n", true)},
		{"create a new husky hook", "pass", hookEditAdd(".husky/post-merge", "npm ci\n")},
		{"git config without hooksPath", "pass", hookEditAdd(config, "[core]\n\tfilemode = false\n")},
		{"hooksPath in another file", "pass", hookEditAdd("notes.md", "hooksPath = x\n")},
		{"edit empties a fragment of a hook", "pass", hookEditReplace(".husky/pre-commit", "npx a\n", "", true)},
		{"edit removes a hooksPath line", "pass", hookEditReplace(config, "\thooksPath = x\n", "", true)},
		{"edit repeats an existing hooksPath line", "pass", hookEditReplace(config, "\thooksPath = x\n\tfilemode = false\n", "\thooksPath = x\n\tfilemode = true\n", false)},
		{"empty write elsewhere", "pass", hookEditWrite("empty.txt", "")},
		{"a read of a husky hook", "pass", hookEditRead(".husky/pre-commit")},
		{"a read of a git hook", "pass", hookEditRead(".git/hooks/pre-commit")},
		{"a read of the pre-commit config", "pass", hookEditRead(".pre-commit-config.yaml")},
		{"a read of git config", "pass", hookEditRead(config)},
	}
}

func hookEditEnvironment(t *testing.T) string {
	t.Helper()
	hosttest.ScrubGuardEnv(t)
	logDir := t.TempDir()
	t.Setenv("COMMAND_GUARD_LOG_DIR", logDir)
	t.Setenv("RELEASE_GUARD_LOG_DIR", t.TempDir())
	return logDir
}

// requireHookEditOutcome asserts a pass, or a denial that names the git_hook_bypass category.
func requireHookEditOutcome(t *testing.T, surface surface.ID, want string, code int, stdout, stderr string) {
	t.Helper()
	if want == "pass" {
		requireSilent(t, surface, code, stdout, stderr)
		return
	}
	reason := requireDeny(t, surface, code, stdout, stderr)
	if !strings.Contains(reason, "git_hook_bypass") {
		t.Fatalf("%s denial reason=%q, want the git_hook_bypass category", surface, reason)
	}
	// C11 is a fixed rule, so its reason must not point at a setting that would relax it.
	for _, forbidden := range []string{".agentics/hooks.json", "Setting:"} {
		if strings.Contains(reason, forbidden) {
			t.Fatalf("%s denial reason=%q names a setting (%q)", surface, reason, forbidden)
		}
	}
}

func TestHookEditToolCalls(t *testing.T) {
	for _, tc := range hookEditCalls() {
		t.Run(tc.name, func(t *testing.T) {
			expressible := 0
			for _, surface := range hookEditSurfaces {
				if _, ok := tc.build(string(surface), ""); ok {
					expressible++
				}
			}
			if expressible < 2 {
				t.Fatalf("%s is expressible on %d surfaces, want at least 2", tc.name, expressible)
			}
			for _, surface := range hookEditSurfaces {
				t.Run(string(surface), func(t *testing.T) {
					root := t.TempDir()
					call, ok := tc.build(string(surface), root)
					if !ok {
						t.Skip("the " + surface + " tools cannot express this form")
					}
					hookEditEnvironment(t)
					code, out, diagnostic := invokeWithLegalArgs(t, string(hosttest.Stdin(surface, call)), surface, "pre-tool")
					requireHookEditOutcome(t, surface, tc.want, code, out, diagnostic)
				})
			}
		})
	}
}

func TestHookEditScriptContent(t *testing.T) {
	files := map[string]string{
		"kill.sh":   "echo hello\nrm .husky/pre-commit\n",
		"append.sh": "echo hello\necho 'npx lint-staged' >> .husky/pre-commit\n",
	}
	for _, tc := range []struct{ name, command, want string }{
		{"a script that deletes a hook", "bash kill.sh", "deny"},
		{"a script that only appends to a hook", "bash append.sh", "pass"},
	} {
		for _, surface := range hookEditSurfaces {
			for _, hook := range []string{"pre-tool", "command-guardian"} {
				t.Run(tc.name+"/"+string(surface)+"/"+hook, func(t *testing.T) {
					hookEditEnvironment(t)
					root := t.TempDir()
					for name, content := range files {
						writeHardeningFile(t, filepath.Join(root, name), content)
					}
					payload := string(hosttest.Stdin(surface, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": tc.command}, Cwd: root}))
					code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, hook)
					requireHookEditOutcome(t, surface, tc.want, code, out, diagnostic)
				})
			}
		}
	}
}

func TestHookEditEnvironmentControls(t *testing.T) {
	for _, surface := range hookEditSurfaces {
		t.Run(string(surface), func(t *testing.T) {
			root := t.TempDir()
			write, ok := hookEditAdd(".git/config", "[core]\n\thooksPath = /tmp/h\n")(string(surface), root)
			if !ok {
				t.Fatal("the write form must be expressible on every surface")
			}
			writePayload := string(hosttest.Stdin(surface, write))
			shellPayload := string(hosttest.Stdin(surface, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "rm .husky/pre-commit"}, Cwd: root}))
			var logDir string
			run := func(env map[string]string, payload string) (int, string, string) {
				logDir = hookEditEnvironment(t)
				for key, value := range env {
					t.Setenv(key, value)
				}
				return invokeWithLegalArgs(t, payload, surface, "pre-tool")
			}
			for _, payload := range []string{writePayload, shellPayload} {
				code, out, diagnostic := run(nil, payload)
				requireHookEditOutcome(t, surface, "deny", code, out, diagnostic)
			}
			code, out, diagnostic := run(map[string]string{"COMMAND_GUARD_ALLOWLIST": "hooksPath"}, writePayload)
			requireHookEditOutcome(t, surface, "deny", code, out, diagnostic)
			code, out, diagnostic = run(map[string]string{"COMMAND_GUARD_ALLOWLIST": ".husky"}, shellPayload)
			requireHookEditOutcome(t, surface, "pass", code, out, diagnostic)
			for _, payload := range []string{writePayload, shellPayload} {
				code, out, diagnostic = run(map[string]string{"SKIP_COMMAND_GUARD": "true"}, payload)
				requireHookEditOutcome(t, surface, "pass", code, out, diagnostic)
				code, out, diagnostic = run(map[string]string{"COMMAND_GUARD_MODE": "warn"}, payload)
				if code != 0 || hosttest.Read(surface, code, []byte(out), []byte(diagnostic)).Decision != hosttest.Allow {
					t.Fatalf("%s warn mode must exit 0 without a deny: exit=%d stdout=%q stderr=%q", surface, code, out, diagnostic)
				}
				// Warn mode advises rather than passes, so the detection is still logged.
				record, err := os.ReadFile(filepath.Join(logDir, "guard.log"))
				if err != nil || !strings.Contains(string(record), `"category":"git_hook_bypass"`) {
					t.Fatalf("%s warn mode left no git_hook_bypass record: %q (%v)", surface, record, err)
				}
			}
		})
	}
}

// TestHookEditLedByAnotherRule covers shell forms that another rule leads the deny reason for (database_destruction on
// truncate, command_parse_limit on an abbreviated Remove-Item parameter). The reason therefore names the other rule, so
// the proof is the deny plus C11's record in guard.log.
func TestHookEditLedByAnotherRule(t *testing.T) {
	for _, tc := range []struct{ tool, command string }{
		{"Bash", "truncate -s 0 .husky/pre-commit"},
		{"Bash", "truncate -s 0 .git/hooks/pre-commit"},
		{"PowerShell", `Remove-Item -re .husky\pre-commit`},
	} {
		for _, surface := range hookEditSurfaces {
			t.Run(string(surface)+"/"+tc.command, func(t *testing.T) {
				logDir := hookEditEnvironment(t)
				// Codex and Antigravity have no PowerShell tool of their own, so this neutral-vocabulary call exercises the
				// adapter's passthrough, not native tool support.
				call := hosttest.Call{Tool: tc.tool, Input: map[string]any{"command": tc.command}, Cwd: t.TempDir(), Native: (surface == "codex" || surface == "antigravity") && tc.tool == "PowerShell"}
				code, out, diagnostic := invokeWithLegalArgs(t, string(hosttest.Stdin(surface, call)), surface, "pre-tool")
				requireDeny(t, surface, code, out, diagnostic)
				record, err := os.ReadFile(filepath.Join(logDir, "guard.log"))
				if err != nil || !strings.Contains(string(record), `"category":"git_hook_bypass"`) {
					t.Fatalf("guard.log %q (%v) lacks the git_hook_bypass threat", record, err)
				}
			})
		}
	}
}

func TestHookEditOverlap(t *testing.T) {
	for _, surface := range hookEditSurfaces {
		t.Run(string(surface), func(t *testing.T) {
			logDir := hookEditEnvironment(t)
			payload := string(hosttest.Stdin(surface, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "rm .git/hooks/pre-commit"}, Cwd: t.TempDir()}))
			code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "pre-tool")
			reason := requireDeny(t, surface, code, out, diagnostic)
			if !strings.Contains(reason, "destructive_file_ops") {
				t.Fatalf("%s reason=%q, want destructive_file_ops", surface, reason)
			}
			record, err := os.ReadFile(filepath.Join(logDir, "guard.log"))
			if err != nil || !strings.Contains(string(record), `"category":"git_hook_bypass"`) {
				t.Fatalf("guard.log %q (%v) lacks the git_hook_bypass threat", record, err)
			}
		})
	}
}
