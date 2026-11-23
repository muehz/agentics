package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// TestMain points HOME and USERPROFILE at an empty directory, so no test reads
// the developer's own ~/.agentics/hooks.json. A test that needs a home directory
// of its own sets both variables again with t.Setenv.
func TestMain(m *testing.M) {
	os.Exit(hosttest.MainWithIsolatedHome(m))
}

// settingsWriteStdin is the file-writing PreToolUse payload each surface's host
// writes. Copilot's create names its body file_text and Codex writes through
// apply_patch, whose target is the file header in the patch, so both are
// native calls.
func settingsWriteStdin(surface surface.ID, root, path string) string {
	var call hosttest.Call
	switch surface {
	case "antigravity", "claude-code":
		call = hosttest.Call{Tool: "Write", Input: map[string]any{"file_path": path, "content": "{}"}, Cwd: root}
	case "copilot":
		call = hosttest.Call{Tool: "create", Input: map[string]any{"path": path, "file_text": "{}"}, Cwd: root, Native: true}
	case "codex":
		call = hosttest.Call{
			Tool:   "apply_patch",
			Input:  map[string]any{"command": "*** Begin Patch\n*** Add File: " + path + "\n+{}\n*** End Patch"},
			Cwd:    root,
			Native: true,
		}
	default:
		panic("unknown surface " + surface)
	}
	return string(hosttest.Stdin(surface, call))
}

// TestWorkflowSettings is the outcome proof for the hook workflow settings. It
// drives the grouped pre-tool event, as a host does, across every surface with
// a temporary home directory and repository.
func TestWorkflowSettings(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			hosttest.ScrubGuardEnv(t)
			t.Setenv("AGX_RELEASE_RECORD", "")
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			repo := t.TempDir()
			if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			repoFile := filepath.Join(repo, ".agentics", "hooks.json")
			userFile := filepath.Join(home, ".agentics", "hooks.json")
			put := func(path, body string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			remove := func(path string) {
				t.Helper()
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
			}
			// call feeds one native payload through pre-tool and returns the
			// text a host would show the model.
			call := func(payload, want string) string {
				t.Helper()
				code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "pre-tool")
				requireOutcome(t, surface, want, "", code, out, diagnostic)
				return out + diagnostic
			}
			shell := func(command, want string) string {
				t.Helper()
				return call(string(hosttest.Stdin(surface, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": command}, Cwd: repo})), want)
			}

			const create = "gh pr create --title x"

			// 1. No settings file: the draft policy blocks, and names its key.
			if text := shell(create, "deny"); !strings.Contains(text, "policy.draft_prs") {
				t.Fatalf("the denial does not name its key: %q", text)
			}

			// 2. A repository file turns that one policy off.
			put(repoFile, `{"version":1,"policy":{"draft_prs":false}}`)
			shell(create, "pass")
			shell("npm publish", "deny")

			// 3. A repository value overrides the user value.
			put(userFile, `{"version":1,"policy":{"draft_prs":false}}`)
			put(repoFile, `{"version":1,"policy":{"draft_prs":true}}`)
			shell(create, "deny")

			// 4. A broken repository file keeps every default, even with a
			// user-level false, and the denial names the ignored file.
			put(repoFile, `{"version":1,"policy":{"merge":false}}`)
			if text := shell(create, "deny"); !strings.Contains(text, "ignored invalid") || !strings.Contains(text, "unknown-key") || !strings.Contains(text, filepath.Base(repo)) {
				t.Fatalf("the denial does not name the ignored file: %q", text)
			}

			// 5. A fixed safety rule denies with every policy off.
			put(userFile, `{"version":1,"policy":{"draft_prs":false,"block_package_publish":false,"require_deploy_record":false,"block_review_thread_changes":false,"block_security_alert_dismissal":false,"command_reformulation":false}}`)
			remove(repoFile)
			shell("rm -rf ~", "deny")
			shell("git clean -fdx", "deny")
			shell(create+" && rm -rf ~", "deny")

			// 6. The deploy gate follows its key, and a destructive invocation
			// anywhere in the command keeps it.
			shell("terraform apply plan.tfplan", "pass")
			shell("kubectl apply -f a.yaml && terraform destroy", "deny")

			// 7. An agent cannot write a settings file, and can write others.
			call(settingsWriteStdin(surface, repo, filepath.Join(repo, ".agentics", "hooks.json")), "deny")
			call(settingsWriteStdin(surface, repo, filepath.Join(repo, "notes.txt")), "pass")
			shell("echo '{}' > .agentics/hooks.json", "deny")
			shell("cat .agentics/hooks.json", "pass")
		})
	}
}
