package commandguard

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/settings/settingstest"
)

func TestShellCommandsThatWriteASettingsFileAreDenied(t *testing.T) {
	source := settingstest.Policy(settings.Keys...).Source()
	for _, c := range []struct{ tool, command string }{
		{"Bash", `echo '{}' > .agentics/hooks.json`},
		{"Bash", `echo '{}' >> ~/.agentics/hooks.json`},
		{"Bash", `printf x | tee .agentics/hooks.json`},
		{"Bash", `printf x | tee -a ~/.agentics/hooks.json`},
		{"Bash", `cp /tmp/x.json .agentics/hooks.json`},
		{"Bash", `cp -f /tmp/x.json ./.agentics/hooks.json`},
		{"Bash", `mv /tmp/x.json .agentics/hooks.json`},
		{"Bash", `mv .agentics/hooks.json /tmp/x.json`},
		{"Bash", `rm .agentics/hooks.json`},
		{"Bash", `rm -f ~/.agentics/hooks.json`},
		{"Bash", `sed -i 's/true/false/' .agentics/hooks.json`},
		{"Bash", `sed --in-place s/true/false/ .agentics/hooks.json`},
		{"Bash", `install -m 644 /tmp/x.json .agentics/hooks.json`},
		{"Bash", `ln -sf /tmp/x.json .agentics/hooks.json`},
		{"Bash", `true && echo '{}' > .agentics/hooks.json`},
		{"PowerShell", `Set-Content -Path .agentics\hooks.json -Value x`},
		{"PowerShell", `Set-Content -LiteralPath:.agentics/hooks.json x`},
		{"PowerShell", `'{}' | Out-File .agentics\hooks.json`},
		{"PowerShell", `Add-Content ~\.agentics\hooks.json x`},
		{"PowerShell", `sc .agentics/hooks.json x`},
		{"Bash", `pwsh -NoProfile -Command "sc .agentics/hooks.json x"`},
		{"Bash", `echo x > .agentics./hooks.json`},
		{"Bash", `rm "/repo/.agentics /hooks.json"`},
		{"PowerShell", `Set-Content -Path .agentics.\hooks.json -Value x`},
		{"PowerShell", `Set-Content -Path ".agentics \hooks.json." -Value x`},
		{"PowerShell", `Remove-Item .agentics\hooks.json`},
		{"PowerShell", `Copy-Item x.json .agentics\hooks.json`},
		{"PowerShell", `Out-File -Append .agentics\hooks.json`},
		{"PowerShell", `Remove-Item -Path:.agentics\hooks.json`},
		{"PowerShell", `Copy-Item -Destination .agentics\hooks.json -Path x.json`},
		{"Bash", `cp /tmp/hooks.json .agentics/`},
		{"Bash", `mv /tmp/hooks.json .agentics`},
		{"Bash", `cp -t .agentics /tmp/hooks.json`},
		{"Bash", `cp -vt .agentics /tmp/hooks.json`},
		{"Bash", `cp -T hooks.json .agentics`},
		{"Bash", `cp -S.txt x.json .agentics/hooks.json`},
		{"Bash", `install -gstaff x.json .agentics/hooks.json`},
		// Value-free letters from cp, install, and ln may precede -t in a cluster.
		{"Bash", `cp -Rt .agentics hooks.json`},
		{"Bash", `install -Dt .agentics hooks.json`},
		{"Bash", `ln -Ft .agentics ../hooks.json`},
		// A quoted -WhatIf is a value, not the dry-run switch.
		{"PowerShell", `Copy-Item x.json .agentics\hooks.json -Exclude '-WhatIf'`},
		{"PowerShell", `Move-Item x.json .agentics\hooks.json -Exclude '-WhatIf'`},
		{"PowerShell", `Set-Content -Path .agentics\hooks.json -Value '{"version":1}' -Exclude "-WhatIf"`},
	} {
		t.Run(c.tool+" "+c.command, func(t *testing.T) {
			got, _, _ := runGo(t, tableCase{tool: c.tool, command: c.command, settings: source})
			if got.Kind() != "deny" {
				t.Fatalf("kind = %s, reason %q", got.Kind(), got.Payload())
			}
			if !strings.Contains(got.Payload(), "hook_settings_edit - Do not edit the Agentics hook settings files.") {
				t.Fatalf("reason = %q", got.Payload())
			}
		})
	}
}

// A redirect on a script call, on a wrapper, or on a group sends the output of
// the commands under it to the redirect's target, so the target is judged
// even though no command's own text names it, and even when the script runs
// no command at all and the redirect only truncates the file.
func TestRedirectsOnScriptCallsAndWrappersThatWriteASettingsFileAreDenied(t *testing.T) {
	settingsFile := ".agentics/" + "hooks.json"
	files := hookspectest.Scripts{
		"/ws/ok.sh":            "echo hello\n",
		"/ws/empty.sh":         "",
		"/ws/comment.sh":       "# nothing runs here\n",
		"/ws/outer.sh":         "bash /ws/ok.sh\n",
		"/ws/outer-empty.sh":   "bash /ws/empty.sh\n",
		"/ws/inner-write.sh":   "bash /ws/ok.sh > " + settingsFile + "\n",
		"/ws/inner-empty.sh":   "bash /ws/empty.sh >> " + settingsFile + "\n",
		"/ws/sources.sh":       "source /ws/ok.sh > " + settingsFile + "\n",
		"/ws/sources-empty.sh": "source /ws/empty.sh > " + settingsFile + "\n",
	}
	for _, c := range []struct{ command, shown string }{
		{"bash /ws/ok.sh > " + settingsFile, settingsFile},
		{"bash /ws/ok.sh >> " + settingsFile, settingsFile},
		{"bash /ws/ok.sh 1> ./" + settingsFile, "./" + settingsFile},
		{"bash /ws/ok.sh &> " + settingsFile, settingsFile},
		{"bash /ws/empty.sh > " + settingsFile, settingsFile},
		{"bash /ws/empty.sh >> " + settingsFile, settingsFile},
		{"bash /ws/comment.sh > " + settingsFile, settingsFile},
		{"bash /ws/outer.sh > " + settingsFile, settingsFile},
		{"bash /ws/outer-empty.sh > " + settingsFile, settingsFile},
		{"env bash /ws/empty.sh > " + settingsFile, settingsFile},
		{"bash -c 'bash /ws/empty.sh' > " + settingsFile, settingsFile},
		{"bash -c 'echo hello' > " + settingsFile, settingsFile},
		{"{ echo hello; } > " + settingsFile, settingsFile},
		{"( echo hello ) > " + settingsFile, settingsFile},
		{"echo hello; > " + settingsFile, settingsFile},
		// The redirect is the script's own text, so the target is replaced by the script's location.
		{"bash /ws/inner-write.sh", ""},
		{"bash /ws/inner-empty.sh", ""},
		{"bash /ws/sources.sh", ""},
		{"bash /ws/sources-empty.sh", ""},
	} {
		t.Run(c.command, func(t *testing.T) {
			out, _, log := runScriptCallWith(t, settingstest.Policy(settings.Keys...).Source(), c.command, files)
			if out.Kind() != "deny" || !strings.Contains(out.Payload(), "hook_settings_edit - Do not edit the Agentics hook settings files.") {
				t.Fatalf("got %s %q, want the settings edit deny", out.Kind(), out.Payload())
			}
			if c.shown != "" && !strings.Contains(log, `"match":"`+c.shown+`"`) {
				t.Errorf("guard.log does not show the typed target %q: %q", c.shown, log)
			}
			if c.shown == "" && (!strings.Contains(log, "script content redacted") || strings.Contains(log, `"match":"`+settingsFile)) {
				t.Errorf("guard.log shows a target taken from script text: %q", log)
			}
		})
	}
}

// The positive siblings: the same calls aimed at another file, at a
// descriptor, or reading the settings file pass.
func TestRedirectsOnScriptCallsToOtherFilesPass(t *testing.T) {
	settingsFile := ".agentics/" + "hooks.json"
	files := hookspectest.Scripts{"/ws/ok.sh": "echo hello\n", "/ws/empty.sh": ""}
	for _, command := range []string{
		"bash /ws/ok.sh > out.txt",
		"bash /ws/ok.sh >> out.txt",
		"bash /ws/empty.sh > out.txt",
		"bash /ws/ok.sh 2>&1",
		"bash /ws/ok.sh < " + settingsFile,
		"bash /ws/empty.sh < " + settingsFile,
		"bash /ws/ok.sh > " + settingsFile + ".bak",
		"env bash /ws/empty.sh > out.txt",
	} {
		t.Run(command, func(t *testing.T) {
			if out, _, _ := runScriptCallWith(t, settingstest.Policy(settings.Keys...).Source(), command, files); out.Kind() != "pass" {
				t.Fatalf("got %s %q, want pass", out.Kind(), out.Payload())
			}
		})
	}
}

// The false-positive set: commands that only read the file, name it in prose,
// or touch a file with a similar name.
func TestShellCommandsThatOnlyReadOrMentionASettingsFilePass(t *testing.T) {
	for _, c := range []struct{ tool, command string }{
		{"Bash", `cat .agentics/hooks.json`},
		{"Bash", `jq . ~/.agentics/hooks.json`},
		{"Bash", `grep -n draft .agentics/hooks.json`},
		{"Bash", `cp .agentics/hooks.json /tmp/backup.json`},
		{"Bash", `diff .agentics/hooks.json other.json`},
		{"Bash", `echo ".agentics/hooks.json" > notes.txt`},
		{"Bash", `echo '.agentics/hooks.json' | tee notes.txt`},
		{"Bash", `git add .agentics/hooks.json`},
		{"Bash", `git commit -m "rm .agentics/hooks.json"`},
		{"Bash", `sed 's/a/b/' .agentics/hooks.json`},
		{"Bash", `ls .agentics`},
		{"Bash", `rm .agentics/hooks.json.bak`},
		{"Bash", `rm hooks.json`},
		{"Bash", `mv old.json .agentics/other.json`},
		// sc and ni are PowerShell aliases only; in a Bash invocation they are other programs.
		{"Bash", `sc .agentics/hooks.json x`},
		{"Bash", `sc config .agentics/hooks.json`},
		{"Bash", `ni .agentics/hooks.json`},
		{"Bash", `Set-Content .agentics/hooks.json x`},
		{"Bash", `Out-File .agentics/hooks.json`},
		{"PowerShell", `Get-Content .agentics\hooks.json`},
		{"PowerShell", `Set-Content notes.txt ".agentics/hooks.json"`},
		{"PowerShell", `Copy-Item .agentics\hooks.json backup.json`},
		{"PowerShell", `Copy-Item -Destination backup.json -Path .agentics\hooks.json`},
		// With one positional word and no -Destination the copy goes to the
		// current location.
		{"PowerShell", `Copy-Item .agentics\hooks.json`},
		// Remove-Item with -WhatIf is held by a separate rule (command_parse_limit),
		// so the dry run is pinned with cmdlets that rule does not parse.
		{"PowerShell", `Set-Content .agentics\hooks.json x -WhatIf`},
		{"PowerShell", `Copy-Item x.json .agentics\hooks.json -whatif`},
		{"Bash", `cat < .agentics/hooks.json`},
		{"Bash", `ln -s .agentics/hooks.json /tmp/link`},
		{"Bash", `cp -t /tmp .agentics/hooks.json`},
		{"Bash", `cp -Rt /tmp .agentics/hooks.json`},
		{"Bash", `install -Dt /tmp .agentics/hooks.json`},
		{"Bash", "cat <<'EOF' > notes.txt\necho x > .agentics/hooks.json\nEOF"},
	} {
		t.Run(c.tool+" "+c.command, func(t *testing.T) {
			got, _, _ := runGo(t, tableCase{tool: c.tool, command: c.command})
			if got.Kind() != "pass" {
				t.Fatalf("kind = %s, reason %q", got.Kind(), got.Payload())
			}
		})
	}
}
