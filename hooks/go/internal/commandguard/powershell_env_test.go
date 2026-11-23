package commandguard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func TestPowerShellEnvironmentNameCasing(t *testing.T) {
	for _, name := range []string{"GIT_CONFIG_KEY_0", "GIT_CONFIG_PARAMETERS"} {
		value := "CoRe.HoOkSpAtH"
		if name == "GIT_CONFIG_PARAMETERS" {
			value += "=x"
		}
		for _, variant := range []string{name, strings.ToLower(name), "Git_Config_" + strings.TrimPrefix(name, "GIT_CONFIG_")} {
			want := "pass"
			if variant == name || runtime.GOOS == "windows" {
				want = "deny"
			}
			for form, command := range map[string]string{
				"assignment": "$env:" + variant + " = '" + value + "'",
				"braced":     "${env:" + variant + "} = '" + value + "'",
				"item":       "Set-Item -LiteralPath Env:" + variant + " -Value '" + value + "'",
				"setter":     "[Environment]::SetEnvironmentVariable('" + variant + "','" + value + "','Process')",
			} {
				runHookBypassCases(t, []tableCase{{name: variant + "/" + form, tool: "PowerShell", command: command, want: want}})
				runHookBypassCases(t, []tableCase{{name: variant + "/" + form + "/unrelated-key", tool: "PowerShell", command: strings.ReplaceAll(command, "CoRe.HoOkSpAtH", "UsEr.NaMe"), want: "pass"}})
			}
		}
	}
}

func TestPowerShellEnvironmentNameCasingUnknownValues(t *testing.T) {
	unknown := "1 blocking threat(s) detected: command_parse_limit - " + powerShellEnvironmentSuggestion
	for _, name := range []string{"GIT_CONFIG_KEY_0", "GIT_CONFIG_PARAMETERS"} {
		for _, variant := range []string{name, strings.ToLower(name), "Git_Config_" + strings.TrimPrefix(name, "GIT_CONFIG_")} {
			want, reason := "pass", ""
			if variant == name || runtime.GOOS == "windows" {
				want, reason = "deny", unknown
			}
			for form, command := range map[string]string{
				"assignment": "$env:" + variant + " = $value",
				"braced":     "${env:" + variant + "} = $value",
				"item":       "Set-Item -LiteralPath Env:" + variant + " -Value $value",
				"append":     "$env:" + variant + " += 'core.hooksPath'",
			} {
				runHookBypassCases(t, []tableCase{{name: variant + "/" + form, tool: "PowerShell", command: command, want: want, wantReason: reason}})
			}
		}
	}
}

func TestPowerShellEnvironmentScanPolling(t *testing.T) {
	const units = 6
	large := strings.Repeat("a", units*64*1024)
	for _, form := range []string{"braced", "unbraced"} {
		t.Run(form, func(t *testing.T) {
			source := "$env:" + large + " = 'x'"
			if form == "braced" {
				source = "${env:" + large + "} = 'x'"
			}
			parsed := shellwords.Parse(source, shellwords.PowerShell)
			if len(parsed.Invocations) != 1 {
				t.Fatalf("environment fixture invocations=%d", len(parsed.Invocations))
			}
			baseline := &hookspectest.CancelContext{Context: context.Background()}
			powerShellDynamicName(baseline, large)
			probe := &hookspectest.CancelContext{Context: context.Background()}
			issue := powerShellEnvironmentWrite(probe, parsed.Invocations[0])
			if probe.Calls-baseline.Calls < units {
				t.Fatalf("environment %s polling: total=%d name-scan=%d minimum-extra=%d", form, probe.Calls, baseline.Calls, units)
			}
			if issue.kind != noGitHookIssue {
				t.Fatalf("environment %s ordinary kind=%d", form, issue.kind)
			}
			probe = &hookspectest.CancelContext{Context: context.Background(), At: 4}
			issue = powerShellEnvironmentWrite(probe, parsed.Invocations[0])
			if probe.Calls < 4 || issue.kind != unknownGitHookEffect || issue.match != powerShellEnvironmentMatch {
				t.Fatalf("environment %s canceled checks=%d kind=%d", form, probe.Calls, issue.kind)
			}
		})
	}
	t.Run("value", func(t *testing.T) {
		probe := &hookspectest.CancelContext{Context: context.Background()}
		issue := powerShellEnvironmentValue(probe, "GIT_CONFIG_PARAMETERS", large)
		if probe.Calls < units {
			t.Fatalf("environment value polling: got=%d minimum=%d", probe.Calls, units)
		}
		if issue.kind != noGitHookIssue {
			t.Fatalf("environment value kind=%d", issue.kind)
		}
		probe = &hookspectest.CancelContext{Context: context.Background(), At: 4}
		issue = powerShellEnvironmentValue(probe, "GIT_CONFIG_PARAMETERS", large)
		if probe.Calls < 4 || issue.kind != unknownGitHookEffect || issue.match != powerShellEnvironmentMatch {
			t.Fatalf("environment value canceled checks=%d kind=%d", probe.Calls, issue.kind)
		}
	})
	t.Run("name", func(t *testing.T) {
		probe := &hookspectest.CancelContext{Context: context.Background()}
		dynamic := powerShellDynamicName(probe, large)
		if probe.Calls < units {
			t.Fatalf("environment name polling: got=%d minimum=%d", probe.Calls, units)
		}
		if dynamic {
			t.Fatal("ordinary name classified dynamic")
		}
		probe = &hookspectest.CancelContext{Context: context.Background(), At: 4}
		if !powerShellDynamicName(probe, large) || probe.Calls < 4 {
			t.Fatalf("environment name canceled checks=%d", probe.Calls)
		}
	})
	t.Run("scalar", func(t *testing.T) {
		word := shellwords.Parse("Write-Output '"+large+"'", shellwords.PowerShell).Invocations[0].Words[1]
		probe := &hookspectest.CancelContext{Context: context.Background()}
		ok := powerShellScalar(probe, word)
		if probe.Calls < units {
			t.Fatalf("environment scalar polling: got=%d minimum=%d", probe.Calls, units)
		}
		if !ok {
			t.Fatal("ordinary scalar lost proof")
		}
		probe = &hookspectest.CancelContext{Context: context.Background(), At: 4}
		if powerShellScalar(probe, word) || probe.Calls < 4 {
			t.Fatalf("environment scalar canceled checks=%d", probe.Calls)
		}
	})
	for _, verb := range []string{"Set-Item", "Remove-Item"} {
		t.Run(verb+"-words", func(t *testing.T) {
			source := verb + " " + strings.Repeat("ordinary ", 4096) + "Env:OTHER"
			words := shellwords.Parse(source, shellwords.PowerShell).Invocations[0].Words[1:]
			inspect := powerShellSetItem
			if verb == "Remove-Item" {
				inspect = powerShellRemoveEnvironment
			}
			probe := &hookspectest.CancelContext{Context: context.Background()}
			issue := inspect(probe, words)
			if probe.Calls < 4096/256 {
				t.Fatalf("environment %s words polling: got=%d minimum=%d", verb, probe.Calls, 4096/256)
			}
			if issue.kind != unknownGitHookEffect {
				t.Fatalf("environment %s competing kind=%d", verb, issue.kind)
			}
			probe = &hookspectest.CancelContext{Context: context.Background(), At: 4}
			issue = inspect(probe, words)
			if probe.Calls < 4 || issue.kind != unknownGitHookEffect {
				t.Fatalf("environment %s words canceled checks=%d kind=%d", verb, probe.Calls, issue.kind)
			}
		})
	}
}

func TestGitConfigurationCrossThreatPrivacy(t *testing.T) {
	for _, tc := range []tableCase{
		{name: "alternate assignment", command: `GIT_CONFIG_GLOBAL='rm private-config-value-marker .git' git status`, want: "deny"},
		{name: "malformed env assignment", command: `env GIT_CONFIG_GLOBAL='rm private-config-value-marker .git`, want: "deny"},
		{name: "unknown env option", command: `env --unknown GIT_CONFIG_GLOBAL='rm private-config-value-marker .git' git status`, want: "deny"},
		{name: "malformed git config", command: `git config core.hooksPath 'rm private-config-value-marker .git`, want: "deny"},
		{name: "bare assignment", command: `GIT_CONFIG_SYSTEM='rm private-config-value-marker .git'`, want: "deny"},
		{name: "export assignment", command: `export GIT_CONFIG_GLOBAL='rm private-config-value-marker .git'`, want: "deny"},
		{name: "PowerShell assignment", tool: "PowerShell", command: `$env:GIT_CONFIG_GLOBAL = 'rm private-config-value-marker .git'`, want: "deny"},
		{name: "PowerShell item", tool: "PowerShell", command: `Set-Item Env:GIT_CONFIG_GLOBAL 'rm private-config-value-marker .git'`, want: "deny"},
		{name: "PowerShell call", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('GIT_CONFIG_GLOBAL', 'rm private-config-value-marker .git')`, want: "deny"},
		{name: "mapped alternate", input: `{"command":"git status","env":{"GIT_CONFIG_GLOBAL":"rm private-config-value-marker .git"}}`, want: "deny"},
		{name: "protected configuration", command: `git -c core.hooksPath='rm private-config-value-marker .git' status`, want: "deny"},
		{name: "confirmed priority", command: `GIT_CONFIG_GLOBAL='rm private-config-value-marker .git' git commit --no-verify`, want: "deny"},
		{name: "following protected operation", command: `GIT_CONFIG_GLOBAL='rm private-config-value-marker .git' git status; git clean -fdx`, want: "deny"},
		{name: "earlier confirmed operation", command: `git commit --no-verify; GIT_CONFIG_GLOBAL='rm private-config-value-marker .git' git status`, want: "deny"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := recordCase(t, tc)
			if got.Outcome.Kind() != "deny" || !got.LogExists || got.Diag == "" {
				t.Fatal("configuration-bearing request lost its denial or diagnostics")
			}
			if strings.Contains(got.Diag+string(got.GuardLog)+got.Outcome.Payload(), "private-config-value-marker") {
				t.Fatal("configuration value leaked through another threat")
			}
			var logged guardLogRecord
			if err := json.Unmarshal([]byte(got.GuardLog), &logged); err != nil {
				t.Fatal(err)
			}
			if logged.ThreatCount == 0 || logged.ThreatCount != len(logged.Threats) {
				t.Fatal("lost contributing findings")
			}
			categories := map[string]bool{}
			for _, finding := range logged.Threats {
				categories[finding.Category] = true
			}
			if tc.name == "confirmed priority" || tc.name == "earlier confirmed operation" {
				if !categories["git_hook_bypass"] {
					t.Fatal("lost confirmed hook bypass")
				}
			}
			if tc.name == "following protected operation" {
				if !categories["destructive_git_ops"] {
					t.Fatalf("lost following protected operation: %#v", categories)
				}
			}
			if tc.name == "alternate assignment" {
				if !categories["destructive_file_ops"] {
					t.Fatalf("lost generic finding: %#v", categories)
				}
			}
		})
	}
}

func TestAlternateGitEnvironmentPrivacy(t *testing.T) {
	for _, tc := range []tableCase{
		{name: "malformed assignment", tool: "PowerShell", command: `$env:GIT_CONFIG_GLOBAL = 'private-source-canary`, want: "deny"},
		{name: "malformed item", tool: "PowerShell", command: `Set-Item Env:GIT_CONFIG_GLOBAL -Value 'private-source-canary`, want: "deny"},
		{name: "malformed wrapper", command: `GIT_CONFIG_GLOBAL=private-source-canary env --unknown git status`, want: "deny"},
		{name: "dynamic git key", command: `git -c "$private_source_canary=x" status`, want: "deny"},
		{name: "protected dynamic git value", command: `git -c core.hooksPath="$private_source_canary" status`, want: "deny"},
		{name: "null unrelated", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('OTHER', $null, 'Process')`, want: "pass"},
		{name: "quoted null unrelated", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('OTHER', '$null', 'Process')`, want: "pass"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := recordCase(t, tc)
			assertCaseSemantics(t, tc, got)
			for _, marker := range []string{"private-source-canary", "private_source_canary"} {
				if strings.Contains(got.Diag+string(got.GuardLog)+got.Outcome.Payload(), marker) {
					t.Fatal("configuration value leaked")
				}
			}
		})
	}
}

// The second case parses to no words, so its only invocation is the word-less one a canceled parse appends. The
// match the guard picked for it is not asserted: a canceled parse is configuration-bearing, and the report's outcome
// replaces every match with a fixed text before the diagnostics or the log are written.
func TestPowerShellEnvironmentCancellation(t *testing.T) {
	for _, tc := range []struct{ name, command, marker string }{
		{"environment write", `[Environment]::SetEnvironmentVariable('EXAMPLE','ordinary')`, "ordinary"},
		{"comment only", `# leakmarkerS8 note`, "leakmarkerS8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hosttest.ScrubGuardEnv(t)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			root := t.TempDir()
			event := buildEvent(t, "PowerShell", commandInput(t, tc.command), root)
			var diag bytes.Buffer
			out := Run(hookspec.Request{Context: ctx, Event: event, Diag: &diag})
			if out.Kind() != "deny" || !strings.Contains(out.Payload(), "command_parse_limit") {
				t.Fatalf("canceled inspection = %s %s", out.Kind(), out.Payload())
			}
			if strings.Contains(diag.String()+out.Payload(), tc.marker) {
				t.Fatal("command text leaked in cancellation diagnostic")
			}
			log, err := os.ReadFile(filepath.Join(guardLog.Dir(root), "guard.log"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(log), tc.marker) {
				t.Fatal("command text leaked in cancellation audit match")
			}
		})
	}
}

func TestPowerShellEnvironmentMalformedPrivacy(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	for _, tc := range []struct{ name, prefix, marker string }{
		{"plain assignment", `$env:GIT_CONFIG_KEY_0 = '`, "plain-private-marker"},
		{"braced assignment", `${env:GIT_CONFIG_KEY_0} = '`, "braced-private-marker"},
		{"Set-Item", `Set-Item Env:GIT_CONFIG_KEY_0 -Value '`, "cmdlet-private-marker"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			event := buildEvent(t, "PowerShell", commandInput(t, tc.prefix+tc.marker), root)
			var diag bytes.Buffer
			out := Run(hookspec.Request{Event: event, Diag: &diag})
			if out.Kind() != "deny" || !strings.Contains(out.Payload(), "command_parse_limit") {
				t.Fatalf("malformed environment write = %s %s", out.Kind(), out.Payload())
			}
			if strings.Contains(diag.String(), tc.marker) || strings.Contains(out.Payload(), tc.marker) {
				t.Error("environment value leaked in diagnostic")
			}
			log, err := os.ReadFile(filepath.Join(guardLog.Dir(root), "guard.log"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(log), tc.marker) {
				t.Error("environment value leaked in audit match")
			}
		})
	}
}

func TestPowerShellEnvironmentLargeCancellation(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	for _, at := range []int{0, 4, 12} {
		t.Run(fmt.Sprint(at), func(t *testing.T) {
			ctx := &hookspectest.CancelContext{Context: context.Background(), At: at}
			root := t.TempDir()
			command := `[Environment]::SetEnvironmentVariable('EXAMPLE','local-value-marker` + strings.Repeat("x", 256*1024) + `')`
			event := buildEvent(t, "PowerShell", commandInput(t, command), root)
			out := Run(hookspec.Request{Context: ctx, Event: event})
			if at == 0 {
				if out.Kind() != "pass" {
					t.Fatalf("normal large call: %s %s", out.Kind(), out.Payload())
				}
				return
			}
			if out.Kind() != "deny" || !strings.Contains(out.Payload(), "command_parse_limit") {
				t.Fatalf("canceled large call: %s %s", out.Kind(), out.Payload())
			}
			log, err := os.ReadFile(filepath.Join(guardLog.Dir(root), "guard.log"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(log), "local-value-marker") {
				t.Fatal("environment value leaked in audit match")
			}
		})
	}
}

func TestPowerShellEnvironmentLargeValues(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	for _, prefix := range []string{`${env:GIT_CONFIG_PARAMETERS} = `, `Set-Item Env:GIT_CONFIG_PARAMETERS -Value `} {
		for _, at := range []int{0, 4, 12} {
			t.Run(prefix+fmt.Sprint(at), func(t *testing.T) {
				ctx := &hookspectest.CancelContext{Context: context.Background(), At: at}
				root := t.TempDir()
				command := prefix + `'local-value-marker` + strings.Repeat("x", 256*1024) + `'`
				event := buildEvent(t, "PowerShell", commandInput(t, command), root)
				out := Run(hookspec.Request{Context: ctx, Event: event})
				if at == 0 {
					if out.Kind() != "pass" {
						t.Fatalf("large literal value: %s %s", out.Kind(), out.Payload())
					}
				} else {
					if out.Kind() != "deny" || !strings.Contains(out.Payload(), "command_parse_limit") {
						t.Fatalf("canceled value: %s %s", out.Kind(), out.Payload())
					}
					log, err := os.ReadFile(filepath.Join(guardLog.Dir(root), "guard.log"))
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(log), "local-value-marker") {
						t.Fatal("environment value leaked in audit match")
					}
				}
			})
		}
	}
}

func TestPowerShellEnvironmentPrecision(t *testing.T) {
	unknown := "1 blocking threat(s) detected: command_parse_limit - " + powerShellEnvironmentSuggestion
	runHookBypassCases(t, []tableCase{
		{name: "braced assignment", tool: "PowerShell", command: `${env:GIT_CONFIG_KEY_0} = 'core.hooksPath'`, want: "deny"},
		{name: "braced unrelated", tool: "PowerShell", command: `${env:EXAMPLE} = 'ordinary'`, want: "pass"},
		{name: "append", tool: "PowerShell", command: `$env:GIT_CONFIG_KEY_0 += 'core.hooksPath'`, want: "deny", wantReason: unknown},
		{name: "dynamic value", tool: "PowerShell", command: `$env:GIT_CONFIG_KEY_0 = $value`, want: "deny", wantReason: unknown},
		{name: "dynamic name", tool: "PowerShell", command: `${env:$name} = 'core.hooksPath'`, want: "deny", wantReason: unknown},
		{name: "no whitespace", tool: "PowerShell", command: `${env:GIT_CONFIG_KEY_0}='core.hooksPath'`, want: "deny"},
		{name: "read", tool: "PowerShell", command: `$env:GIT_CONFIG_KEY_0`, want: "pass"},
		{name: "unrelated append", tool: "PowerShell", command: `$env:EXAMPLE += 'ordinary'`, want: "pass"},
		{name: "literal dollar", tool: "PowerShell", command: `$env:GIT_CONFIG_KEY_0 = '$ordinary'`, want: "pass"},
		{name: "posix quoted example", command: `'${env:GIT_CONFIG_KEY_0}' = 'core.hooksPath'`, want: "pass"},
		{name: "literalpath", tool: "PowerShell", command: `Set-Item -LiteralPath Env:GIT_CONFIG_KEY_0 -Value 'core.hooksPath'`, want: "deny"},
		{name: "positional provider", tool: "PowerShell", command: `Set-Item Env:\GIT_CONFIG_KEY_0 'core.hooksPath'`, want: "deny"},
		{name: "unrelated cmdlet", tool: "PowerShell", command: `Set-Item Env:EXAMPLE -Value 'ordinary'`, want: "pass"},
		{name: "unrelated key value", tool: "PowerShell", command: `Set-Item Env:GIT_CONFIG_KEY_0 -Value 'user.name'`, want: "pass"},
		{name: "dynamic path", tool: "PowerShell", command: `Set-Item -Path $target -Value 'ordinary'`, want: "deny", wantReason: unknown},
		{name: "competing paths", tool: "PowerShell", command: `Set-Item -LiteralPath Env:EXAMPLE -Path Env:GIT_CONFIG_KEY_0 -Value 'core.hooksPath'`, want: "deny", wantReason: unknown},
		{name: "dynamic cmdlet value", tool: "PowerShell", command: `Set-Item Env:GIT_CONFIG_KEY_0 -Value $value`, want: "deny", wantReason: unknown},
		{name: "quoted cmdlet example", tool: "PowerShell", command: `'Set-Item Env:GIT_CONFIG_KEY_0 core.hooksPath'`, want: "pass"},
		{name: "system setter", tool: "PowerShell", command: `[System.Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0','core.hooksPath','Process')`, want: "deny"},
		{name: "short setter", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0','core.hooksPath')`, want: "deny"},
		{name: "user setter lexical only", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0','core.hooksPath','User')`, want: "deny"},
		{name: "machine setter lexical only", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0','core.hooksPath','Machine')`, want: "deny"},
		{name: "unrelated setter", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('EXAMPLE','ordinary')`, want: "pass"},
		{name: "getter", tool: "PowerShell", command: `[Environment]::GetEnvironmentVariable('GIT_CONFIG_KEY_0')`, want: "pass"},
		{name: "dynamic setter", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0',$value)`, want: "deny", wantReason: unknown},
		{name: "unknown target", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0','core.hooksPath',$target)`, want: "deny", wantReason: unknown},
		{name: "quoted setter", tool: "PowerShell", command: `Write-Output "[Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0','core.hooksPath')"`, want: "pass"},
		{name: "wrapped powershell", command: `pwsh -Command "[Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0','core.hooksPath')"`, want: "deny"},
		{name: "assignment expression setter", tool: "PowerShell", command: `$result = [Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0','core.hooksPath')`, want: "deny", wantReason: unknown},
		{name: "tight assignment expression setter", tool: "PowerShell", command: `$result=[Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0','core.hooksPath')`, want: "deny", wantReason: unknown},
		{name: "grouped setter", tool: "PowerShell", command: `([Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0','core.hooksPath'))`, want: "deny"},
		{name: "malformed setter", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0')`, want: "deny", wantReason: unknown},
		{name: "setter trailing expression", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('GIT_CONFIG_KEY_0','core.hooksPath') + $value`, want: "deny", wantReason: unknown},
		{name: "assignment array", tool: "PowerShell", command: `$env:GIT_CONFIG_KEY_0 = 'core.hooksPath','user.name'`, want: "deny", wantReason: unknown},
		{name: "cmdlet array", tool: "PowerShell", command: `Set-Item Env:GIT_CONFIG_KEY_0 -Value 'core.hooksPath','user.name'`, want: "deny", wantReason: unknown},
		{name: "literal comma value", tool: "PowerShell", command: `$env:GIT_CONFIG_KEY_0 = 'user.name,ordinary'`, want: "pass"},
		{name: "later real bypass", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('EXAMPLE','ordinary'); git commit --no-verify`, want: "deny"},
		{name: "nested posix literal data", tool: "PowerShell", command: `bash -c 'printf "%s" "[Environment]::SetEnvironmentVariable(ordinary)"'`, want: "pass"},
	})
}
