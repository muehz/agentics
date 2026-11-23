package main

import (
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// A here-string (<<<) introduces no heredoc body, so every command after it is still inspected. The rows are the
// reproduction from #1213: each command is denied by its guard alone, and must stay denied after a here-string and a
// newline.
func TestHereStringDoesNotHideLaterCommands(t *testing.T) {
	rows := []struct{ guard, command string }{
		{"command-guardian", "rm -rf /"},
		{"command-guardian", "git commit -m x --no-verify"},
		{"command-guardian", "git reset --hard HEAD~1"},
		{"secrets-guardian", "cat .env"},
	}
	preludes := []string{
		"cat <<< \"x\"\n",
		"cat <<< x\n",
		"cat <<<\"x\"\n",
		"read -r a <<< \"$1\" || true\n",
		"cat <<< \"x\" <<< \"y\"\n",
		"cat <<< $(echo x)\n",
	}
	for _, surface := range surface.All() {
		for _, row := range rows {
			alone := secretsShellStdin(surface, t.TempDir(), "Bash", row.command)
			code, stdout, stderr := invokeWithLegalArgs(t, alone, surface, row.guard)
			if verdict := hosttest.Read(surface, code, []byte(stdout), []byte(stderr)); verdict.Decision != hosttest.Deny {
				t.Fatalf("%s %s no longer denies %q on its own: host reads %v", surface, row.guard, row.command, verdict.Decision)
			}
			for _, prelude := range preludes {
				payload := secretsShellStdin(surface, t.TempDir(), "Bash", prelude+row.command)
				code, stdout, stderr := invokeWithLegalArgs(t, payload, surface, row.guard)
				if verdict := hosttest.Read(surface, code, []byte(stdout), []byte(stderr)); verdict.Decision != hosttest.Deny {
					t.Errorf("%s %s passes %q after %q: host reads %v", surface, row.guard, row.command, prelude, verdict.Decision)
				}
			}
		}
	}
}

// The same commands in the same line were already denied; the here-string must not change that, and a real heredoc
// must still hide its body from the guards while the line after it stays inspected.
func TestHereStringControlsStayDenied(t *testing.T) {
	commands := []string{
		"cat <<< \"x\"; rm -rf /",
		"cat <<< \"x\" && rm -rf /",
		"rm -rf /\ncat <<< \"x\"",
		"cat <<EOF\nx\nEOF\nrm -rf /",
		"cat <<< \"x\"\ncat <<EOF\nx\nEOF\nrm -rf /",
	}
	for _, command := range commands {
		payload := secretsShellStdin("claude-code", t.TempDir(), "Bash", command)
		code, stdout, stderr := invokeWithLegalArgs(t, payload, "claude-code", "command-guardian")
		if verdict := hosttest.Read("claude-code", code, []byte(stdout), []byte(stderr)); verdict.Decision != hosttest.Deny {
			t.Errorf("%q: host reads %v", command, verdict.Decision)
		}
	}
}

// Text a here-string is fed stays data: a heredoc-looking word inside it opens nothing, and the guards do not
// invent a command from it.
func TestHereStringBodyIsNotACommand(t *testing.T) {
	for _, command := range []string{
		"cat <<< \"rm -rf /\"",
		"cat <<< 'rm -rf /'\necho done",
		"grep -c x <<< \"git push --force origin main\"",
	} {
		payload := secretsShellStdin("claude-code", t.TempDir(), "Bash", command)
		code, stdout, stderr := invokeWithLegalArgs(t, payload, "claude-code", "command-guardian")
		requireSilent(t, "claude-code", code, stdout, stderr)
	}
}
