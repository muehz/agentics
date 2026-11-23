package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// TestFlowCallForm inspects provider-shaped commands without executing the binary
// or the command-looking prose supplied as its arguments.
func TestFlowCallForm(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	root := filepath.ToSlash(hosttest.Checkout(t))
	binary, state := hosttest.CommittedBinaryPath(filepath.Join(hosttest.Checkout(t), "hooks"))
	if state == hosttest.OffContract {
		t.Skipf("no hook binary is committed for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	bashBinary := filepath.ToSlash(binary)
	powerShellBinary := root + "/hooks/bin/agx-windows-amd64.exe"
	const logArgs = `log execute enter task-1 ok "git push --force; cat .env; terraform apply" "none" "execute go"`
	for _, surface := range surface.All() {
		for _, verb := range []struct{ name, args string }{
			{"status", "status"},
			{"log", logArgs},
		} {
			for _, form := range []struct {
				name, tool, command string
				deny                bool
			}{
				{"bash-literal", "Bash", `"` + bashBinary + `" flow ` + verb.args, false},
				{"bash-variable", "Bash", `AGX_BIN="` + bashBinary + `"; "$AGX_BIN" flow ` + verb.args, true},
				{"powershell-call", "PowerShell", `& "` + powerShellBinary + `" flow ` + verb.args, false},
				{"bash-computed", "Bash", `$(printf "%s" "` + bashBinary + `") flow ` + verb.args, true},
				{"powershell-computed", "PowerShell", `& $(Write-Output "` + powerShellBinary + `") flow ` + verb.args, true},
				{"bash-bare", "Bash", `agx flow ` + verb.args, false},
				{"powershell-bare", "PowerShell", `agx flow ` + verb.args, false},
				{"bash-launcher", "Bash", `"` + root + `/bin/agx" flow ` + verb.args, false},
				{"powershell-launcher", "PowerShell", `& "` + root + `/bin/agx.exe" flow ` + verb.args, false},
				{"bash-bare-computed", "Bash", `$(command -v agx) flow ` + verb.args, true},
			} {
				t.Run(string(surface)+"/"+form.name+"/"+verb.name, func(t *testing.T) {
					command := form.command
					// Codex and Antigravity deliver shell calls as Bash; a literal
					// interpreter wrapper is required to reach PowerShell syntax there.
					if (surface == "codex" || surface == "antigravity") && form.tool == "PowerShell" {
						command = "pwsh -Command '" + command + "'"
					}
					payload := secretsShellStdin(surface, dir, form.tool, command)
					code, stdout, stderr := invokeWithLegalArgs(t, payload, surface, "pre-tool")
					if !form.deny {
						requireSilent(t, surface, code, stdout, stderr)
						return
					}
					reason := requireDeny(t, surface, code, stdout, stderr)
					if !strings.Contains(reason, "[commands] 1 blocking threat(s) detected: command_parse_limit - ") ||
						!strings.Contains(reason, "script-opaque:command-position-expansion") {
						t.Fatalf("dynamic executable denial lost its distinctive reason: %q", reason)
					}
					if surface != "claude-code" && stderr != "" {
						t.Fatalf("JSON guard emitted unexpected stderr: %q", stderr)
					}
				})
			}
		}
	}
}
