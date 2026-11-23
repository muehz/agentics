package main

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// proofGitHubToolName spells a GitHub MCP tool the way each surface names it.
func proofGitHubToolName(surface surface.ID, tool string) string {
	switch surface {
	case "copilot":
		return "github." + tool
	case "codex":
		return "mcp__github/" + tool
	default:
		return "mcp__github__" + tool
	}
}

// TestPreToolOutcomeProof proves, through the real entry point on every
// surface, what the grouped pre-tool event now does: it names itself in a
// denial, the retired rules no longer gate, the kept rules still do, and no
// command-rule switch lifts the deploy gate. It also proves the retired
// post-tool event is an unknown hook.
func TestPreToolOutcomeProof(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			hosttest.ScrubGuardEnv(t)
			root := t.TempDir()
			t.Setenv("COMMAND_GUARD_LOG_DIR", t.TempDir())
			t.Setenv("RELEASE_GUARD_LOG_DIR", t.TempDir())
			outcome := func(payload, want string) string {
				t.Helper()
				code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "pre-tool")
				requireOutcome(t, surface, want, "", code, out, diagnostic)
				return out + diagnostic
			}
			shell := func(command, want string) string {
				t.Helper()
				return outcome(secretsShellStdin(surface, root, "Bash", command), want)
			}
			mcp := func(tool string, input map[string]any, want string) string {
				t.Helper()
				return outcome(string(hosttest.Stdin(surface, hosttest.Call{Tool: proofGitHubToolName(surface, tool), Input: input, Cwd: root, Native: true})), want)
			}
			repo := map[string]any{"owner": "o", "repo": "r"}

			// The grouped check names itself, and the deploy rule is part of it.
			for _, command := range []string{"rm -rf /", "kubectl apply -f x.yaml"} {
				if text := shell(command, "deny"); !strings.Contains(text, "[commands]") {
					t.Fatalf("%q: the denial does not carry the commands label: %q", command, text)
				}
			}
			if text := mcp("delete_repository", repo, "deny"); !strings.Contains(text, "[commands]") {
				t.Fatalf("delete_repository: the denial does not carry the commands label: %q", text)
			}

			// Removed rules no longer gate.
			for _, command := range []string{
				"git push --force origin main", "git push origin +main", "gh pr merge 1", "gh stack merge",
				"gh release create v1", "gh release edit v1 --draft=false",
			} {
				shell(command, "pass")
			}
			mcp("merge_pull_request", map[string]any{"owner": "o", "repo": "r", "pullNumber": 1}, "pass")

			// Kept rules still gate.
			for _, command := range []string{
				"rm -rf /", "git reset --hard origin/main", "git clean -fdx", "gh pr create --title x", "gh pr ready 1",
				"npm publish", "git commit --no-verify -m x", "cat .env",
			} {
				shell(command, "deny")
			}

			// The command rules' switches never lift the deploy gate.
			t.Setenv("SKIP_COMMAND_GUARD", "true")
			t.Setenv("COMMAND_GUARD_ALLOWLIST", "kubectl")
			shell("kubectl apply -f x.yaml", "deny")
			shell("rm -rf /", "pass")
		})
	}

	// The retired event is an unknown hook, a non-blocking usage error.
	code, _, stderr := invoke(t, "{}", "claude-code", "post-tool")
	if code != 1 || !strings.Contains(stderr, `unknown hook "post-tool"`) {
		t.Fatalf("post-tool: exit=%d stderr=%q, want exit 1 and the unknown-hook message", code, stderr)
	}
}
