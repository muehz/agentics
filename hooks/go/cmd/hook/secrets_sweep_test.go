package main

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// recordingTB embeds the parent only to satisfy testing.TB's private method;
// every failure path is overridden so none reaches the parent from this
// goroutine.
type recordingTB struct {
	testing.TB
	failed bool
}

func (r *recordingTB) Helper()               {}
func (r *recordingTB) Log(...any)            {}
func (r *recordingTB) Logf(string, ...any)   {}
func (r *recordingTB) Fail()                 { r.failed = true }
func (r *recordingTB) Error(...any)          { r.failed = true }
func (r *recordingTB) Errorf(string, ...any) { r.failed = true }
func (r *recordingTB) FailNow()              { r.failed = true; runtime.Goexit() }
func (r *recordingTB) Fatalf(string, ...any) { r.failed = true; runtime.Goexit() }
func (r *recordingTB) Fatal(...any)          { r.failed = true; runtime.Goexit() }

// TestHardeningDenyNamesFinding pins #670: a deny row with a non-empty
// finding must fail requireOutcome unless the provider's deny reason
// actually names that finding; a helper that checks only the outcome kind
// and ignores the deny reason's finding name must not pass this case.
func TestHardeningDenyNamesFinding(t *testing.T) {
	deny := `{"permissionDecision":"deny","permissionDecisionReason":"1 potential secret(s) found: AWS_ACCESS_KEY"}`
	run := func(finding string) bool {
		rec := &recordingTB{TB: t}
		done := make(chan struct{})
		go func() {
			defer close(done)
			requireOutcome(rec, "copilot", "deny", finding, 0, deny, "")
		}()
		<-done
		return rec.failed
	}
	if !run("GENERIC_SECRET") {
		t.Fatal("deny row naming GENERIC_SECRET passed on an AWS_ACCESS_KEY deny")
	}
	if run("AWS_ACCESS_KEY") {
		t.Fatal("deny row naming the actual finding failed")
	}
}

// secretsShellStdin is the payload a surface's host writes for a shell call in
// root.
func secretsShellStdin(surface surface.ID, root, tool, command string) string {
	return string(hosttest.Stdin(surface, hosttest.Call{Tool: tool, Input: map[string]any{"command": command}, Cwd: root}))
}

// secretsWriteStdin is the payload a surface's host writes for a whole-file
// write of probe.txt.
func secretsWriteStdin(surface surface.ID, root, content string) string {
	return string(hosttest.Stdin(surface, hosttest.Call{Tool: "Write", Input: map[string]any{"file_path": "probe.txt", "content": content}, Cwd: root}))
}

// copilotNativeStdin is the payload Copilot's host writes for a tool call whose
// toolArgs are given as is: Copilot's own tool names and keys, or a string.
func copilotNativeStdin(tool string, args any, cwd string) string {
	return string(hosttest.Stdin("copilot", hosttest.Call{Tool: tool, Input: args, Cwd: cwd, Native: true}))
}

// copilotStringToolArgs is the payload for a Copilot call whose toolArgs is a
// JSON-encoded string (not an object), the shape #685 pins: the guards must
// decode it before they read commands and paths out of tool.input.value.
func copilotStringToolArgs(t *testing.T, tool string, args map[string]any, cwd string) string {
	t.Helper()
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return copilotNativeStdin(tool, string(encoded), cwd)
}

// TestCopilotStringToolArgs pins #685 at the hook seam: Copilot's toolArgs
// arrives as a JSON string when the client encodes it that way, and the
// guards must still see the decoded object under tool.input.value. A read
// naming a dotenv file denies, and an edit that adds a GENERIC_SECRET value
// denies naming that finding.
func TestCopilotStringToolArgs(t *testing.T) {
	t.Run("view denies dotenv", func(t *testing.T) {
		root := t.TempDir()
		payload := copilotStringToolArgs(t, "view", map[string]any{"path": ".env"}, root)
		code, out, diagnostic := invokeWithLegalArgs(t, payload, "copilot", "secrets-guardian")
		requireOutcome(t, "copilot", "deny", "dotenv", code, out, diagnostic)
	})

	t.Run("edit denies GENERIC_SECRET", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("SECRETS_GUARD_ALLOWLIST", "")
		value := "pass" + "word = " + "Zq8" + "Lm2Rt7Vx9Kp4"
		payload := copilotStringToolArgs(t, "edit", map[string]any{
			"path":    "probe.txt",
			"old_str": "",
			"new_str": value,
		}, root)
		code, out, diagnostic := invokeWithLegalArgs(t, payload, "copilot", "secrets-guardian")
		requireOutcome(t, "copilot", "deny", "GENERIC_SECRET", code, out, diagnostic)
	})
}

// TestCopilotFileText pins #686: Copilot's create writes its body under
// file_text, which has no paired old text and is scanned in full. A file_text
// carrying a secret denies naming the rule; one without a secret passes.
func TestCopilotFileText(t *testing.T) {
	key := "AKIA" + strings.Repeat("Q", 16)
	t.Run("deny", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("SECRETS_GUARD_ALLOWLIST", "")
		payload := copilotNativeStdin("create", map[string]any{"path": "probe.txt", "file_text": "key = " + key}, root)
		code, out, diagnostic := invokeWithLegalArgs(t, payload, "copilot", "secrets-guardian")
		requireOutcome(t, "copilot", "deny", "AWS_ACCESS_KEY", code, out, diagnostic)
	})

	t.Run("pass", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("SECRETS_GUARD_ALLOWLIST", "")
		payload := copilotNativeStdin("create", map[string]any{"path": "probe.txt", "file_text": "no secret here"}, root)
		code, out, diagnostic := invokeWithLegalArgs(t, payload, "copilot", "secrets-guardian")
		requireOutcome(t, "copilot", "pass", "", code, out, diagnostic)
	})
}

// TestCopilotApplyPatch pins #686: Copilot's apply_patch shares Codex's patch
// grammar and gets the same added-line extraction and malformed-patch deny,
// across the three shapes Copilot CLI's own extractor reads (1.0.88): raw
// string toolArgs, an object's input, and an object's patch.
func TestCopilotApplyPatch(t *testing.T) {
	key := "AKIA" + strings.Repeat("Q", 16)
	addPatch := "*** Begin Patch\n*** Add File: probe.txt\n+key = " + key + "\n*** End Patch"
	removePatch := "*** Begin Patch\n*** Update File: probe.txt\n@@\n-key = " + key + "\n+placeholder\n*** End Patch"
	malformedPatch := "*** Begin Patch\n*** Add File: probe.txt"

	shapes := map[string]func(patch string) any{
		"raw string toolArgs": func(patch string) any { return patch },
		"object with input":   func(patch string) any { return map[string]any{"input": patch} },
		"object with patch":   func(patch string) any { return map[string]any{"patch": patch} },
	}

	for name, toolArgs := range shapes {
		t.Run(name, func(t *testing.T) {
			invoke := func(patch string) (int, string, string) {
				root := t.TempDir()
				t.Setenv("SECRETS_GUARD_ALLOWLIST", "")
				payload := copilotNativeStdin("apply_patch", toolArgs(patch), root)
				return invokeWithLegalArgs(t, payload, "copilot", "secrets-guardian")
			}

			t.Run("added secret line denies", func(t *testing.T) {
				code, out, diagnostic := invoke(addPatch)
				requireOutcome(t, "copilot", "deny", "AWS_ACCESS_KEY", code, out, diagnostic)
			})

			t.Run("removed secret line passes", func(t *testing.T) {
				code, out, diagnostic := invoke(removePatch)
				requireOutcome(t, "copilot", "pass", "", code, out, diagnostic)
			})

			t.Run("malformed patch denies", func(t *testing.T) {
				code, out, diagnostic := invoke(malformedPatch)
				requireOutcome(t, "copilot", "deny", "patch body is malformed", code, out, diagnostic)
			})
		})
	}
}

// TestSecretsSweep699 pins #699 at the hook seam: a GENERIC_SECRET-keyed
// value that is a two-operand `||` reference chain passes the guardian on
// every surface, while the same key assigned a quoted literal still denies,
// naming GENERIC_SECRET, as does a chain with one quoted literal operand.
func TestSecretsSweep699(t *testing.T) {
	key := "GITHUB_MCP_SERVER_TOKEN"
	chainLine := key + ": " + "${{ secrets.A || secrets.B }}"
	literal := strings.Join([]string{"Rk4", "Pt8", "Nq2", "Wc6"}, "")
	literalLine := key + ": " + "'" + literal + "'"
	chainWithLiteralLine := key + ": " + "${{ secrets.A || '" + literal + "' }}"

	t.Run("guardian", func(t *testing.T) {
		for _, surface := range surface.All() {
			t.Run(string(surface), func(t *testing.T) {
				root := t.TempDir()
				t.Setenv("SECRETS_GUARD_MODE", "block")
				t.Setenv("SECRETS_GUARD_ALLOWLIST", "")

				payload := secretsWriteStdin(surface, root, chainLine)
				code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "secrets-guardian")
				requireOutcome(t, surface, "pass", "", code, out, diagnostic)

				payload = secretsWriteStdin(surface, root, literalLine)
				code, out, diagnostic = invokeWithLegalArgs(t, payload, surface, "secrets-guardian")
				requireOutcome(t, surface, "deny", "GENERIC_SECRET", code, out, diagnostic)

				// A quoted literal inside an otherwise-referential `||` chain
				// stays a finding: the chain exemption covers only a whole
				// reference, and one literal operand makes the value a
				// candidate.
				payload = secretsWriteStdin(surface, root, chainWithLiteralLine)
				code, out, diagnostic = invokeWithLegalArgs(t, payload, surface, "secrets-guardian")
				requireOutcome(t, surface, "deny", "GENERIC_SECRET", code, out, diagnostic)
			})
		}
	})
}

// TestShellWriteSecretHook pins #684 at the hook seam: secrets-guardian scans
// the whole shell command text in the write direction, after the
// read-direction checks pass, across all three surfaces and both shells. A
// real-shaped AWS key belongs here rather than in hardening_test.go's
// mixed-heredoc row, whose purpose is "heredoc data is not a credential read"
// and which uses the AWS_ACCESS_KEY EXAMPLE allowlist shape for its key.
func TestShellWriteSecretHook(t *testing.T) {
	key := "AKIA" + strings.Repeat("Q", 16)
	generic := "Tok3n" + strings.Repeat("Ab", 6)
	pat := "ghp_" + strings.Repeat("a", 36)

	denyCases := []struct {
		name, tool, command, finding string
	}{
		{"heredoc AWS key redirected to a file", "Bash",
			"cat <<EOF > out.env\nAWS_ACCESS_KEY_ID=" + key + "\nEOF", "AWS_ACCESS_KEY"},
		{"printf GENERIC_SECRET assignment", "Bash",
			"printf 'TOKEN=" + generic + `\n' > out.txt`, "GENERIC_SECRET"},
		{"sed -i replacement carrying a GitHub PAT", "Bash",
			"sed -i 's/OLD/" + pat + "/' config.txt", "GITHUB_PAT"},
		{"PowerShell Set-Content -Value carrying a secret", "PowerShell",
			"Set-Content -Path out.txt -Value '" + key + "'", "AWS_ACCESS_KEY"},
	}
	for _, tc := range denyCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, surface := range surface.All() {
				t.Run(string(surface), func(t *testing.T) {
					root := t.TempDir()
					t.Setenv("SECRETS_GUARD_MODE", "block")
					t.Setenv("SECRETS_GUARD_ALLOWLIST", "")
					payload := secretsShellStdin(surface, root, tc.tool, tc.command)
					code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "secrets-guardian")
					requireOutcome(t, surface, "deny", tc.finding, code, out, diagnostic)
					reason := out
					if surface == "claude-code" {
						reason = diagnostic
					}
					if !strings.Contains(reason, "shell command") {
						t.Errorf("%s deny reason=%q, want it to name the shell command", surface, reason)
					}
				})
			}
		})
	}

	t.Run("warn mode returns context not deny", func(t *testing.T) {
		command := "printf 'TOKEN=" + generic + `\n' > out.txt`
		for _, surface := range surface.All() {
			t.Run(string(surface), func(t *testing.T) {
				root := t.TempDir()
				t.Setenv("SECRETS_GUARD_MODE", "warn")
				t.Setenv("SECRETS_GUARD_ALLOWLIST", "")
				payload := secretsShellStdin(surface, root, "Bash", command)
				code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "secrets-guardian")
				if surface == "copilot" {
					if code != 0 || out != "" || diagnostic != "" {
						t.Fatalf("copilot warn result = exit %d stdout %q stderr %q, want empty pass", code, out, diagnostic)
					}
					return
				}
				if surface == "antigravity" {
					if code != 0 || out != "{\"decision\":\"allow\"}\n" || diagnostic != "" {
						t.Fatalf("antigravity warn result = exit %d stdout %q stderr %q, want allow JSON", code, out, diagnostic)
					}
					return
				}
				if code != 0 || diagnostic != "" || !strings.Contains(out, "additionalContext") || !strings.Contains(out, "GENERIC_SECRET") {
					t.Fatalf("%s warn result = exit %d stdout %q stderr %q, want contextual finding", surface, code, out, diagnostic)
				}
			})
		}
	})

	t.Run("advisory only never denies", func(t *testing.T) {
		ip := "10.1." + "2.3:8080"
		for _, surface := range surface.All() {
			t.Run(string(surface), func(t *testing.T) {
				root := t.TempDir()
				t.Setenv("SECRETS_GUARD_MODE", "block")
				t.Setenv("SECRETS_GUARD_ALLOWLIST", "")
				payload := secretsShellStdin(surface, root, "Bash", "echo 'connecting to "+ip+"'")
				code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "secrets-guardian")
				assertAdvisoryContext(t, surface, code, out, diagnostic)
			})
		}
	})

	// The reference-chain line is built from pieces, as TestSecretsSweep699
	// does: a contiguous "${{ a || b }}" literal in this source is itself a
	// GENERIC_SECRET-shaped match the checked-in secrets-guardian flags.
	chain := "TOKEN: " + "${{ secrets.A || secrets.B }}"
	passCases := []struct{ name, command string }{
		{"export from another variable", "export TOKEN=$OTHER"},
		{"secrets reference chain", "echo '" + chain + "'"},
		{"git commit message mentioning token parsing", `git commit -m "fix     token parsing"`},
	}
	for _, tc := range passCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, surface := range surface.All() {
				t.Run(string(surface), func(t *testing.T) {
					root := t.TempDir()
					t.Setenv("SECRETS_GUARD_MODE", "block")
					t.Setenv("SECRETS_GUARD_ALLOWLIST", "")
					payload := secretsShellStdin(surface, root, "Bash", tc.command)
					code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "secrets-guardian")
					requireOutcome(t, surface, "pass", "", code, out, diagnostic)
				})
			}
		})
	}

	t.Run("credential read still wins over the shell write scan", func(t *testing.T) {
		for _, surface := range surface.All() {
			t.Run(string(surface), func(t *testing.T) {
				root := t.TempDir()
				t.Setenv("SECRETS_GUARD_MODE", "block")
				t.Setenv("SECRETS_GUARD_ALLOWLIST", "")
				payload := secretsShellStdin(surface, root, "Bash", "cat .env")
				code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "secrets-guardian")
				requireOutcome(t, surface, "deny", "dotenv", code, out, diagnostic)
			})
		}
	})

	t.Run("path allowlist does not exempt the command-text scan", func(t *testing.T) {
		command := "printf 'TOKEN=" + generic + `\n' > out.txt`
		for _, surface := range surface.All() {
			t.Run(string(surface), func(t *testing.T) {
				root := t.TempDir()
				t.Setenv("SECRETS_GUARD_MODE", "block")
				t.Setenv("SECRETS_GUARD_ALLOWLIST", "out.txt")
				payload := secretsShellStdin(surface, root, "Bash", command)
				code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, "secrets-guardian")
				requireOutcome(t, surface, "deny", "GENERIC_SECRET", code, out, diagnostic)
			})
		}
	})
}
