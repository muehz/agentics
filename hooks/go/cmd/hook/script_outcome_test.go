package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// TestScriptOutcomeProof drives the grouped pre-tool event end to end: a script
// call is judged by what the script contains, the verdict names the line but
// never the content, and a direct invocation behaves as it did before.
func TestScriptOutcomeProof(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	slash := filepath.ToSlash(dir)
	run := func(command string) (int, string) {
		code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin("claude-code", dir, command), "claude-code", "pre-tool")
		return code, stdout + stderr
	}
	writeScript(t, dir, "ok.sh", "#!/usr/bin/env bash\necho hello\n")
	if code, out := run("cd " + slash + " && bash ./ok.sh arg"); code != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("benign script call: exit %d output %q", code, out)
	}
	writeScript(t, dir, "ok.sh", "#!/usr/bin/env bash\necho hello\nrm -rf /\n")
	code, out := run("cd " + slash + " && bash ./ok.sh arg")
	if code != 2 || !strings.Contains(out, "ok.sh:3") || !strings.Contains(out, "destructive_file_ops") ||
		strings.Contains(out, "command_parse_limit") || strings.Contains(out, "script-opaque") || strings.Contains(out, "rm -rf") {
		t.Fatalf("dangerous script call: exit %d output %q", code, out)
	}
	writeScript(t, dir, "ok.sh", "#!/usr/bin/env bash\nsource \"$HOME/x.sh\"\n")
	code, out = run("cd " + slash + " && bash ./ok.sh arg")
	// The refusal names the construct, the script and the line, offers the script remedy rather than the generic
	// parse-limit text alone, and never repeats the operand the script wrote.
	if code != 2 || !strings.Contains(out, "script-opaque:source") || !strings.Contains(out, "ok.sh:2") ||
		!strings.Contains(out, "Rewrite the script line") || strings.Contains(out, "$HOME") || strings.Contains(out, "x.sh") {
		t.Fatalf("dynamic source: exit %d output %q", code, out)
	}
	writeScript(t, dir, "ok.sh", "#!/usr/bin/env bash\necho hello\nrm -rf /\n")
	if code, out := run("cd " + slash + " && ./ok.sh arg"); code != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("direct invocation changed: exit %d output %q", code, out)
	}
}

// TestScriptOutcomeProofRealHelper runs the repository's own flow helper
// through the guards, so a rule that blocks the helper the workflow depends on
// fails here rather than in a user's session.
func TestScriptOutcomeProofRealHelper(t *testing.T) {
	pinScriptGuardEnv(t)
	// The event's cwd is the real checkout, so a project-level settings file
	// there (an untracked .agentics/hooks.json) layers over the pinned HOME and can
	// change the verdict; the proof assumes none exists.
	root := filepath.ToSlash(hosttest.Checkout(t))
	payload := string(hosttest.Stdin("claude-code", hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "bash hooks/go/internal/shellwords/testdata/flow.sh status"}, Cwd: root}))
	code, stdout, stderr := invokeWithLegalArgs(t, payload, "claude-code", "pre-tool")
	if code != 0 || strings.TrimSpace(stdout+stderr) != "" {
		t.Fatalf("flow.sh is still blocked: exit %d\n%s%s", code, stdout, stderr)
	}
}
