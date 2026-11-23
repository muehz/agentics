package commandguard

import (
	"os"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/settings/settingstest"
)

func TestShellCommandsThatBypassTheGitHooksAreDenied(t *testing.T) {
	source := settingstest.Policy(settings.Keys...).Source()
	for _, c := range []struct{ tool, command string }{
		{"Bash", `rm .husky/pre-commit`},
		{"Bash", `rm -f .husky/pre-commit`},
		{"Bash", `rm -rf .husky`},
		{"Bash", `rm -r .husky/`},
		{"Bash", `rm .husky/_/husky.sh`},
		{"Bash", `rm .pre-commit-config.yaml`},
		{"Bash", `unlink .husky/pre-commit`},
		{"Bash", `shred -u .husky/pre-commit`},
		{"Bash", `mv .husky/pre-commit /tmp/pre-commit.bak`},
		{"Bash", `mv .husky old-husky`},
		{"Bash", `mv -t /tmp .husky/pre-commit`},
		{"Bash", `git rm .husky/pre-commit`},
		{"Bash", `git rm -r .husky`},
		{"Bash", `git rm --cached .pre-commit-config.yaml`},
		{"Bash", `git -C repo rm .pre-commit-config.yaml`},
		{"Bash", `: > .husky/pre-commit`},
		{"Bash", `npm test && rm .husky/pre-commit`},
		{"Bash", `rm "/repo/.husky/pre-commit"`},
		{"PowerShell", `Remove-Item .husky\pre-commit`},
		{"PowerShell", `Remove-Item -Recurse -Force .husky`},
		{"PowerShell", `Remove-Item -LiteralPath '.husky/pre-commit'`},
		{"PowerShell", `rm .pre-commit-config.yaml`},
		{"PowerShell", `Clear-Content .husky\pre-commit`},
		{"PowerShell", `Move-Item .husky\pre-commit $env:TEMP\x`},
		{"PowerShell", `Move-Item -Path .husky\pre-commit -Destination x`},
		{"PowerShell", `Remove-Item -r .husky`},
		{"PowerShell", `rm -r .husky\pre-commit`},
		{"PowerShell", `del .husky\pre-commit`},
		{"PowerShell", `erase .pre-commit-config.yaml`},
		{"PowerShell", `ri .husky\pre-commit`},
		{"PowerShell", `clc .husky\pre-commit`},
		{"PowerShell", `mi .husky\pre-commit backup.txt`},
		{"PowerShell", `move .husky\pre-commit backup.txt`},
		{"Bash", `mv .git/hooks/pre-commit /tmp/x`},
		{"Bash", `: > .git/hooks/pre-commit`},
		{"Bash", `del .husky/pre-commit`},
	} {
		t.Run(c.tool+" "+c.command, func(t *testing.T) {
			got, _, _ := runGo(t, tableCase{tool: c.tool, command: c.command, settings: source})
			if got.Kind() != "deny" {
				t.Fatalf("kind = %s, reason %q", got.Kind(), got.Payload())
			}
			if !strings.HasPrefix(got.Payload(), hookEditReason) {
				t.Fatalf("reason = %q", got.Payload())
			}
		})
	}
}

// A command that another rule judges at the same or a higher severity, or that adds a second threat, still carries
// C11's threat in guard.log even when a different rule names the deny reason.
func TestShellHookEditsThatAnotherRuleLeadsStillRecordTheHookThreat(t *testing.T) {
	for _, c := range []struct{ tool, command string }{
		{"Bash", `truncate -s 0 .husky/pre-commit`},
		{"Bash", `truncate -s 0 .git/hooks/pre-commit`},
		{"PowerShell", `Remove-Item -re .husky\pre-commit`},
	} {
		t.Run(c.tool+" "+c.command, func(t *testing.T) {
			got, _, logRoot := runGo(t, tableCase{tool: c.tool, command: c.command})
			if got.Kind() != "deny" {
				t.Fatalf("kind = %s, reason %q", got.Kind(), got.Payload())
			}
			log, err := os.ReadFile(guardLogPath(logRoot))
			if err != nil || !strings.Contains(string(log), `"category":"git_hook_bypass"`) {
				t.Fatalf("guard.log %q (%v) lacks the git_hook_bypass threat", log, err)
			}
		})
	}
}

func TestShellCommandsThatLeaveTheGitHooksWorkingPass(t *testing.T) {
	for _, c := range []struct{ tool, command string }{
		{"Bash", `cat .husky/pre-commit`},
		{"Bash", `ls .husky`},
		{"Bash", `echo 'npx lint-staged' >> .husky/pre-commit`},
		{"Bash", `echo 'npx lint-staged' > .husky/pre-commit`},
		{"Bash", `printf '#!/bin/sh\n' | tee .husky/pre-commit`},
		{"Bash", `cp /tmp/hook .husky/pre-commit`},
		{"Bash", `mv /tmp/hook .husky/pre-commit`},
		{"Bash", `mv /tmp/hook .husky/`},
		{"Bash", `mv -t .husky /tmp/hook`},
		{"Bash", `git rm -n .husky/pre-commit`},
		{"Bash", `git rm --dry-run .husky/pre-commit`},
		{"Bash", `git rm -rn .husky`},
		{"Bash", `git rm src/old.go`},
		{"Bash", `rm src/old.go`},
		{"Bash", `rm docs/husky.md`},
		{"Bash", `git status .husky`},
		{"Bash", `sed -i 's/a/b/' .husky/pre-commit`},
		{"Bash", `touch .husky/new-hook`},
		{"Bash", `chmod +x .husky/pre-commit`},
		{"Bash", `: >> .husky/pre-commit`},
		{"Bash", `: > notes.txt`},
		{"PowerShell", `Set-Content .husky\pre-commit x`},
		{"PowerShell", `Move-Item x.txt .husky\pre-commit`},
		{"PowerShell", `Remove-Item src\old.go`},
		{"PowerShell", `Get-Content .husky\pre-commit`},
	} {
		t.Run(c.tool+" "+c.command, func(t *testing.T) {
			got, _, _ := runGo(t, tableCase{tool: c.tool, command: c.command})
			if got.Kind() != "pass" {
				t.Fatalf("kind = %s, reason %q", got.Kind(), got.Payload())
			}
		})
	}
}

func TestPowerShellHookMoveSources(t *testing.T) {
	for _, c := range []struct {
		command string
		deny    bool
	}{
		{`Move-Item -LiteralPath /tmp/pre-commit .husky/ -Force`, false},
		{`Move-Item -Path /tmp/pre-commit .husky/pre-commit -Force`, false},
		{`Move-Item .husky/ -LiteralPath /tmp/pre-commit`, false},
		{`Move-Item -LiteralPath:/tmp/pre-commit .husky/`, false},
		{`Move-Item -Path:/tmp/pre-commit .husky/`, false},
		{`Move-Item -LiteralPath $source .husky/`, false},
		{`Move-Item $source .husky/`, false},
		{`Move-Item /tmp/pre-commit -Destination .husky/`, false},
		{`Move-Item -Destination .husky/ /tmp/pre-commit`, false},
		{`mi -Path /tmp/pre-commit .husky/`, false},
		{`move -LiteralPath /tmp/pre-commit .husky/`, false},
		{`mv -LiteralPath /tmp/pre-commit .husky/`, false},
		{`Move-Item -LiteralPath .husky/pre-commit /tmp/pre-commit -Force`, true},
		{`Move-Item /tmp/pre-commit -LiteralPath .husky/pre-commit`, true},
		{`Move-Item -Path:.husky/pre-commit /tmp/pre-commit`, true},
		{`Move-Item -LiteralPath .husky/pre-commit -Destination /tmp/pre-commit`, true},
		{`Move-Item -Destination /tmp/pre-commit .husky/pre-commit`, true},
		{`Move-Item .husky/pre-commit $destination`, true},
		{`Move-Item -Path .husky/pre-commit .git/hooks/pre-commit`, true},
		{`mi -LiteralPath .husky/pre-commit /tmp/pre-commit`, true},
		{`move -Path .husky/pre-commit /tmp/pre-commit`, true},
		{`mv -Path .husky/pre-commit /tmp/pre-commit`, true},
	} {
		t.Run(c.command, func(t *testing.T) {
			got, _, _ := runGo(t, tableCase{tool: "PowerShell", command: c.command})
			if c.deny {
				if got.Kind() != "deny" || !strings.HasPrefix(got.Payload(), hookEditReason) {
					t.Fatalf("kind = %s, reason %q, want the git_hook_bypass denial", got.Kind(), got.Payload())
				}
			} else if got.Kind() != "pass" {
				t.Fatalf("kind = %s, reason %q, want pass", got.Kind(), got.Payload())
			}
		})
	}
}

func TestADeletedGitPathKeepsItsDestructiveReasonAndRecordsTheHookThreat(t *testing.T) {
	for _, command := range []string{`rm .git/hooks/pre-commit`, `rm -r .git/hooks`, `unlink .git/config`} {
		t.Run(command, func(t *testing.T) {
			got, _, logRoot := runGo(t, tableCase{command: command})
			if got.Kind() != "deny" || !strings.Contains(got.Payload(), "destructive_file_ops") {
				t.Fatalf("kind = %s, reason %q, want the destructive_file_ops reason", got.Kind(), got.Payload())
			}
			log, err := os.ReadFile(guardLogPath(logRoot))
			if err != nil || !strings.Contains(string(log), `"category":"git_hook_bypass"`) {
				t.Fatalf("guard.log %q (%v) lacks the git_hook_bypass threat", log, err)
			}
		})
	}
}

func TestAnInspectedScriptCannotBypassTheGitHooks(t *testing.T) {
	for _, c := range []struct {
		name, command, body string
		deny                bool
	}{
		{"deletes a hook", "bash /ws/t.sh", "echo hi\nrm .husky/pre-commit\n", true},
		{"truncates a hook", "bash /ws/t.sh", ": > .husky/pre-commit\n", true},
		{"removes the directory", "bash /ws/t.sh", "git rm -r .husky\n", true},
		{"only appends to a hook", "bash /ws/t.sh", "echo 'npx lint-staged' >> .husky/pre-commit\n", false},
		{"a call redirected onto a hook", "bash /ws/t.sh > .husky/pre-commit", "echo hi\n", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, log := runScriptCall(t, c.command, hookspectest.Scripts{"/ws/t.sh": c.body})
			if (out.Kind() == "deny") != c.deny {
				t.Fatalf("kind = %s %q, want deny=%v", out.Kind(), out.Payload(), c.deny)
			}
			if c.deny && !strings.Contains(log, `"category":"git_hook_bypass"`) {
				t.Fatalf("guard.log %q lacks the git_hook_bypass threat", log)
			}
		})
	}
}
