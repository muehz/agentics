package secretsguard

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

// TestMCPReadDirection pins the remote half of the read direction.
//
// A `.env` committed to a repository is the same credential whether it is read
// from the working tree or fetched over the API, so the remote path is
// classified by the same table and denies the same way.
func TestMCPReadDirection(t *testing.T) {
	cases := []struct {
		name  string
		tool  string
		input map[string]any
		want  string
	}{
		{"remote dotenv read is blocked", "mcp__github__get_file_contents",
			map[string]any{"owner": "o", "repo": "r", "path": ".env"}, "deny"},
		{"remote ssh key read is blocked", "mcp__github__get_file_contents",
			map[string]any{"owner": "o", "repo": "r", "path": "deploy/id_rsa"}, "deny"},
		{"remote ordinary file passes", "mcp__github__get_file_contents",
			map[string]any{"owner": "o", "repo": "r", "path": "README.md"}, "pass"},
		// The template exemption is a property of the classifier, so it
		// reaches the remote path for free. Pinning it here keeps a future
		// refactor from re-implementing classification per transport.
		{"remote dotenv template passes", "mcp__github__get_file_contents",
			map[string]any{"owner": "o", "repo": "r", "path": ".env.example"}, "pass"},
		// A directory listing with no path names no file.
		{"remote read with no path passes", "mcp__github__get_file_contents",
			map[string]any{"owner": "o", "repo": "r"}, "pass"},
		// Host namespace spellings.
		{"dot namespace is inspected", "github.get_file_contents",
			map[string]any{"owner": "o", "repo": "r", "path": ".env"}, "deny"},
		{"unnamespaced is inspected", "get_file_contents",
			map[string]any{"owner": "o", "repo": "r", "path": ".env"}, "deny"},
		// A different tool whose name merely contains the gated one.
		{"longer neighbour is not inspected", "mcp__github__get_file_contents_batch",
			map[string]any{"owner": "o", "repo": "r", "path": ".env"}, "pass"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kind, _ := run(t, buildEvent(t, c.tool, c.input), nil)
			if kind != c.want {
				t.Fatalf("expected %s, got %s", c.want, kind)
			}
		})
	}
}

// TestMCPWriteDirection pins the remote half of the write direction.
//
// This half matters more than its local counterpart. A file pushed straight to
// a branch never touches the working tree, so no check of local files
// sees it. For these tools the guardian is the only check.
func TestMCPWriteDirection(t *testing.T) {
	t.Run("single file write with a secret is blocked", func(t *testing.T) {
		kind, message := run(t, buildEvent(t, "mcp__github__create_or_update_file", map[string]any{
			"owner": "o", "repo": "r", "branch": "b", "message": "m",
			"path":    "src/config.ts",
			"content": "export const key = \"" + fakeAWSKey + "\";\n",
		}), nil)
		if kind != "deny" {
			t.Fatalf("expected deny, got %s (%s)", kind, message)
		}
		if !strings.Contains(message, "src/config.ts") {
			t.Fatalf("message does not name the target path: %q", message)
		}
		// The matched value must never be reproduced in full: a deny reason is
		// replayed to the model.
		if strings.Contains(message, fakeAWSKey) {
			t.Fatalf("message reproduced the secret: %q", message)
		}
	})

	t.Run("single file write without a secret passes", func(t *testing.T) {
		kind, _ := run(t, buildEvent(t, "mcp__github__create_or_update_file", map[string]any{
			"owner": "o", "repo": "r", "branch": "b", "message": "m",
			"path": "README.md", "content": "# hello\n",
		}), nil)
		if kind != "pass" {
			t.Fatalf("expected pass, got %s", kind)
		}
	})

	t.Run("multi file push is scanned per file", func(t *testing.T) {
		kind, message := run(t, buildEvent(t, "mcp__github__push_files", map[string]any{
			"owner": "o", "repo": "r", "branch": "b", "message": "m",
			"files": []any{
				map[string]any{"path": "README.md", "content": "# hello\n"},
				map[string]any{"path": "deploy/env.sh", "content": "AWS_ACCESS_KEY_ID=" + fakeAWSKey + "\n"},
			},
		}), nil)
		if kind != "deny" {
			t.Fatalf("expected deny, got %s (%s)", kind, message)
		}
		// The offending file is named even though the call carries no
		// top-level path, so a reader knows which entry to fix.
		if !strings.Contains(message, "deploy/env.sh") {
			t.Fatalf("message does not name the offending file: %q", message)
		}
	})

	t.Run("multi file push of clean files passes", func(t *testing.T) {
		kind, _ := run(t, buildEvent(t, "mcp__github__push_files", map[string]any{
			"owner": "o", "repo": "r", "branch": "b", "message": "m",
			"files": []any{
				map[string]any{"path": "a.md", "content": "one\n"},
				map[string]any{"path": "b.md", "content": "two\n"},
			},
		}), nil)
		if kind != "pass" {
			t.Fatalf("expected pass, got %s", kind)
		}
	})

	// The placeholder filter skips example values, so documentation
	// showing an example key must not block a push.
	t.Run("placeholder in a pushed file passes", func(t *testing.T) {
		kind, _ := run(t, buildEvent(t, "mcp__github__push_files", map[string]any{
			"owner": "o", "repo": "r", "branch": "b", "message": "m",
			"files": []any{
				map[string]any{"path": "docs/setup.md", "content": "Set it like this: " + fakePlaceholderKey + "\n"},
			},
		}), nil)
		if kind != "pass" {
			t.Fatalf("expected pass, got %s", kind)
		}
	})

	// The write direction never classifies the PATH. Pushing a `.env` is
	// ordinary setup work; only its content is examined.
	t.Run("pushing a credential path with clean content passes", func(t *testing.T) {
		kind, _ := run(t, buildEvent(t, "mcp__github__create_or_update_file", map[string]any{
			"owner": "o", "repo": "r", "branch": "b", "message": "m",
			"path": ".env.example", "content": "AWS_ACCESS_KEY_ID=replace-me\n",
		}), nil)
		if kind != "pass" {
			t.Fatalf("expected pass, got %s", kind)
		}
	})

	// A per-file allowlist entry must exempt that file alone, not the call.
	t.Run("allowlist exempts one pushed file", func(t *testing.T) {
		env := map[string]string{"SECRETS_GUARD_ALLOWLIST": "testdata/"}
		kind, _ := run(t, buildEvent(t, "mcp__github__push_files", map[string]any{
			"owner": "o", "repo": "r", "branch": "b", "message": "m",
			"files": []any{
				map[string]any{"path": "testdata/fixture.env", "content": "AWS_ACCESS_KEY_ID=" + fakeAWSKey + "\n"},
			},
		}), env)
		if kind != "pass" {
			t.Fatalf("expected pass, got %s", kind)
		}

		kind, _ = run(t, buildEvent(t, "mcp__github__push_files", map[string]any{
			"owner": "o", "repo": "r", "branch": "b", "message": "m",
			"files": []any{
				map[string]any{"path": "testdata/fixture.env", "content": "AWS_ACCESS_KEY_ID=" + fakeAWSKey + "\n"},
				map[string]any{"path": "src/live.ts", "content": "const k = \"" + fakeGitHubPAT + "\";\n"},
			},
		}), env)
		if kind != "deny" {
			t.Fatalf("an allowlisted sibling must not exempt the whole call, got %s", kind)
		}
	})

	t.Run("warn mode reports instead of denying", func(t *testing.T) {
		kind, _ := run(t, buildEvent(t, "mcp__github__create_or_update_file", map[string]any{
			"owner": "o", "repo": "r", "branch": "b", "message": "m",
			"path": "src/config.ts", "content": "const k = \"" + fakeAWSKey + "\";\n",
		}), map[string]string{"SECRETS_GUARD_MODE": "warn"})
		if kind != "context" {
			t.Fatalf("expected context, got %s", kind)
		}
	})
}

// TestMCPValueEnvelope pins reads through the envelope Copilot and Codex wrap
// a tool's arguments in.
//
// Without it this hook finds no path and no content for an MCP call on two of
// the three surfaces, and a guardrail that finds nothing passes.
func TestMCPValueEnvelope(t *testing.T) {
	t.Run("enveloped credential path is blocked", func(t *testing.T) {
		kind, _ := run(t, buildEvent(t, "mcp__github__get_file_contents", map[string]any{
			"value": map[string]any{"owner": "o", "repo": "r", "path": ".env"},
		}), nil)
		if kind != "deny" {
			t.Fatalf("expected deny, got %s", kind)
		}
	})

	t.Run("enveloped ordinary path passes", func(t *testing.T) {
		kind, _ := run(t, buildEvent(t, "mcp__github__get_file_contents", map[string]any{
			"value": map[string]any{"owner": "o", "repo": "r", "path": "README.md"},
		}), nil)
		if kind != "pass" {
			t.Fatalf("expected pass, got %s", kind)
		}
	})

	t.Run("enveloped multi file push is scanned", func(t *testing.T) {
		kind, message := run(t, buildEvent(t, "mcp__github__push_files", map[string]any{
			"value": map[string]any{
				"owner": "o", "repo": "r", "branch": "b", "message": "m",
				"files": []any{
					map[string]any{"path": "deploy/env.sh", "content": "AWS_ACCESS_KEY_ID=" + fakeAWSKey + "\n"},
				},
			},
		}), nil)
		if kind != "deny" {
			t.Fatalf("expected deny, got %s (%s)", kind, message)
		}
	})

	// A non-object envelope must not panic or match.
	t.Run("string envelope passes", func(t *testing.T) {
		kind, _ := run(t, buildEvent(t, "mcp__github__get_file_contents", map[string]any{
			"value": ".env",
		}), nil)
		if kind != "pass" {
			t.Fatalf("expected pass, got %s", kind)
		}
	})
}

// TestMCPToolsAreNotCrossWired guards the one hazard of normalizing tool
// names: a local tool must never be pulled into a gate it was not in, and a
// remote tool must land in exactly one direction.
func TestMCPToolsAreNotCrossWired(t *testing.T) {
	for name := range mcpReadTools {
		if mcpWriteTools[name] {
			t.Fatalf("%q is in both directions; the write branch would shadow the read", name)
		}
		if inspectedTools[name] || toolcall.IsFileWrite(name) {
			t.Fatalf("%q duplicates a local tool name", name)
		}
	}
	for name := range mcpWriteTools {
		if inspectedTools[name] || toolcall.IsFileWrite(name) {
			t.Fatalf("%q duplicates a local tool name", name)
		}
	}
}
