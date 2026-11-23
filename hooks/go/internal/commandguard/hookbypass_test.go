package commandguard

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func TestGitConfigurationTraversalPolling(t *testing.T) {
	const count = 4096
	for _, tc := range []struct {
		name, source string
		dialect      shellwords.Dialect
	}{
		{"assignments", strings.TrimSpace(strings.Repeat("ORDINARY=value ", count)), shellwords.POSIX},
		{"effects", "env " + strings.Repeat("-u OTHER ", count) + "bash -c ''", shellwords.POSIX},
		{"invocations", strings.Repeat("echo ordinary;", count), shellwords.POSIX},
		{"item-words", "Set-Item " + strings.TrimSpace(strings.Repeat("ordinary ", count)), shellwords.PowerShell},
		{"git-words", "git " + strings.Repeat("--no-pager ", count) + "status", shellwords.POSIX},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed := shellwords.Parse(tc.source, tc.dialect)
			visited := 0
			switch tc.name {
			case "assignments":
				visited = len(parsed.BareAssignments)
			case "effects":
				visited = len(parsed.EnvironmentEffects)
			case "invocations":
				visited = len(parsed.Invocations)
			default:
				if len(parsed.Invocations) == 1 {
					visited = len(parsed.Invocations[0].Words) - 1
				}
			}
			if visited < count {
				t.Fatalf("privacy %s fixture units=%d", tc.name, visited)
			}
			probe := &hookspectest.CancelContext{Context: context.Background()}
			private := gitConfigurationDiagnosticSource(probe, parsed)
			if probe.Calls < count/256 {
				t.Fatalf("privacy %s polling: got=%d minimum=%d", tc.name, probe.Calls, count/256)
			}
			if private {
				t.Fatalf("privacy %s ordinary source marked sensitive", tc.name)
			}
			probe = &hookspectest.CancelContext{Context: context.Background(), At: 4}
			if !gitConfigurationDiagnosticSource(probe, parsed) || probe.Calls < 4 {
				t.Fatalf("privacy %s canceled checks=%d", tc.name, probe.Calls)
			}
		})
	}
	t.Run("policy-effects", func(t *testing.T) {
		source := "env " + strings.Repeat("ORDINARY=value ", count) + "command -v printf"
		effects := shellwords.Parse(source, shellwords.POSIX).EnvironmentEffects
		if len(effects) != count {
			t.Fatalf("policy fixture effects=%d", len(effects))
		}
		probe := &hookspectest.CancelContext{Context: context.Background()}
		issue := gitEnvironmentEffectsIssue(probe, effects)
		if probe.Calls < count/256 {
			t.Fatalf("policy effects polling: got=%d minimum=%d", probe.Calls, count/256)
		}
		if issue.kind != noGitHookIssue {
			t.Fatalf("policy ordinary effects kind=%d", issue.kind)
		}
		probe = &hookspectest.CancelContext{Context: context.Background(), At: 4}
		issue = gitEnvironmentEffectsIssue(probe, effects)
		if probe.Calls < 4 || issue.kind != unknownGitHookEffect || issue.match != gitMutationMatch {
			t.Fatalf("policy effects canceled checks=%d kind=%d", probe.Calls, issue.kind)
		}
	})
}

func TestGitConfigSourcePrecision(t *testing.T) {
	runHookBypassCases(t, []tableCase{
		{name: "alternate prefix", command: `GIT_CONFIG_GLOBAL=/private/config git status`, want: "deny", wantReason: "1 blocking threat(s) detected: command_parse_limit - The guard could not inspect alternate Git configuration. Use the established Git configuration sources so it can determine whether repository hooks are affected."},
		{name: "inherited alternate", command: `git status`, env: map[string]string{"GIT_CONFIG_GLOBAL": "/private/config"}, want: "pass"},
		{name: "unrelated prefix", command: `OTHER=/private/config git status`, want: "pass"},
	})
	for _, tc := range []tableCase{
		{name: "bare empty", command: `GIT_CONFIG_GLOBAL=`, want: "deny"},
		{name: "export null device", command: `export GIT_CONFIG_SYSTEM=/dev/null`, want: "deny"},
		{name: "export existing source", command: `export GIT_CONFIG_GLOBAL`, want: "deny"},
		{name: "wrapper alternate", command: `env GIT_CONFIG_GLOBAL=private-source-canary git status`, want: "deny"},
		{name: "root override", input: `{"command":"git status","env":{"GIT_CONFIG_GLOBAL":"private-source-canary"}}`, want: "deny"},
		{name: "value override", input: `{"command":"git status","value":{"command":"git status","environment":{"GIT_CONFIG_SYSTEM":""}}}`, want: "deny"},
		{name: "null override", input: `{"command":"git status","env":{"GIT_CONFIG_GLOBAL":null}}`, want: "deny"},
		{name: "competing unrelated overrides", input: `{"command":"git status","env":{"OTHER":"x"},"value":{"environment":{"OTHER":"y"}}}`, want: "pass"},
		{name: "competing alternate overrides", input: `{"command":"git status","env":{"OTHER":"x"},"value":{"environment":{"GIT_CONFIG_GLOBAL":null}}}`, want: "deny"},
		{name: "malformed unrelated override", input: `{"command":"git status","env":[]}`, want: "pass"},
		{name: "null container independent flag", input: `{"command":"git --no-pager grep gh -- p.sh","env":null}`, want: "pass"},
		{name: "malformed named selector", input: `{"command":"git status","env":{"GIT_CONFIG_GLOBAL":[]}}`, want: "deny"},
		{name: "nested unrelated envelope", input: `{"command":"git status","value":{"value":{"env":{"OTHER":"x"}}}}`, want: "pass"},
		{name: "nested envelope root selector", input: `{"command":"git status","env":{"GIT_CONFIG_SYSTEM":""},"value":{"value":{}}}`, want: "deny"},
		{name: "malformed envelope direct selector", input: `{"command":"git status","value":{"env":{"GIT_CONFIG_GLOBAL":""},"value":{}}}`, want: "deny"},
		{name: "unrelated override", input: `{"command":"git status","env":{"OTHER":"private-source-canary"}}`, want: "pass"},
		{name: "quoted example", command: `printf '%s' 'GIT_CONFIG_GLOBAL=private-source-canary'`, want: "pass"},
		{name: "PS assignment", tool: "PowerShell", command: `$env:GIT_CONFIG_GLOBAL = 'private-source-canary'`, want: "deny"},
		{name: "PS null assignment", tool: "PowerShell", command: `$env:GIT_CONFIG_SYSTEM = $null`, want: "deny"},
		{name: "PS set item", tool: "PowerShell", command: `Set-Item Env:GIT_CONFIG_GLOBAL 'private-source-canary'`, want: "deny"},
		{name: "PS static setter", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('GIT_CONFIG_GLOBAL', 'private-source-canary', 'Process')`, want: "deny"},
	} {
		if tc.want == "deny" {
			tc.wantReason = "1 blocking threat(s) detected: command_parse_limit - " + alternateGitConfigSuggestion
		}
		t.Run(tc.name, func(t *testing.T) {
			got := recordCase(t, tc)
			assertCaseSemantics(t, tc, got)
			if strings.Contains(got.Diag+string(got.GuardLog), "private-source-canary") {
				t.Fatal("alternate source value leaked")
			}
		})
	}
	for _, windows := range []bool{false, true} {
		if got := alternateGitConfigName("git_config_global", windows); got != windows {
			t.Fatalf("case classifier for windows=%v: %v", windows, got)
		}
	}
	tc := tableCase{name: "execution platform case", command: `git_config_global=private-source-canary git status`, want: "pass"}
	if runtime.GOOS == "windows" {
		tc.want = "deny"
		tc.wantReason = "1 blocking threat(s) detected: command_parse_limit - " + alternateGitConfigSuggestion
	}
	assertCaseSemantics(t, tc, recordCase(t, tc))
}

const hookBypassReason = "1 blocking threat(s) detected: git_hook_bypass - " + hookBypassSuggestion

const gitMutationUnknownReason = "1 blocking threat(s) detected: command_parse_limit - The guard could not inspect the Git configuration mutation. Use literal configuration keys and supported options so it can determine whether repository hooks are affected."

func TestGitConfigSourceLegacyEnvironment(t *testing.T) {
	runHookBypassCases(t, []tableCase{
		{name: "unquoted protected head", command: `$env:GIT_CONFIG_KEY_0 = 'core.hooksPath'`, want: "deny", wantReason: gitMutationUnknownReason},
		{name: "unquoted joined assignment", command: `$env:GIT_CONFIG_KEY_0='core.hooksPath'`, want: "deny", wantReason: gitMutationUnknownReason},
		{name: "quoted head", command: `'$env:GIT_CONFIG_KEY_0' = 'core.hooksPath'`, want: "pass"},
		{name: "escaped head", command: `\$env:GIT_CONFIG_KEY_0 = 'core.hooksPath'`, want: "pass"},
		{name: "quoted operator", command: `$env:GIT_CONFIG_KEY_0 '=' 'core.hooksPath'`, want: "pass"},
		{name: "unrelated value", command: `$env:GIT_CONFIG_KEY_0 = 'safe.directory'`, want: "pass"},
		{name: "unrelated name", command: `$env:OTHER = 'core.hooksPath'`, want: "pass"},
	})
}

func TestGitConfigSourceDynamicMutation(t *testing.T) {
	runHookBypassCases(t, []tableCase{
		{name: "config env dynamic key", command: `git --config-env="$KEY=VALUE" status`, want: "deny", wantReason: gitMutationUnknownReason},
		{name: "export dynamic key", command: `export GIT_CONFIG_KEY_0="$KEY"`, want: "deny", wantReason: gitMutationUnknownReason},
		{name: "dynamic parameters", command: `GIT_CONFIG_PARAMETERS="$PARAMS" git status`, want: "deny", wantReason: gitMutationUnknownReason},
		{name: "known key dynamic value", command: `git -c core.hooksPath="$VALUE" "$COMMAND"`, want: "deny"},
		{name: "later proven global", command: `git -c "$KEY=x" -c core.hooksPath=x status`, want: "deny"},
		{name: "later proven invocation", command: `git -c "$KEY=x" status; git commit --no-verify`, want: "deny"},
		{name: "later proven bare", command: `git -c "$KEY=x" status; GIT_CONFIG_KEY_0=core.hooksPath`, want: "deny"},
		{name: "alternate then proven", command: `GIT_CONFIG_GLOBAL=x git commit --no-verify`, want: "deny"},
		{name: "override then proven", input: `{"command":"git commit --no-verify","env":{"GIT_CONFIG_GLOBAL":"private-source-canary"}}`, want: "deny"},
		{name: "quoted literal key", command: `git -c '$KEY=x' status`, want: "pass"},
		{name: "unrelated dynamic value", command: `git config user.name "$VALUE"`, want: "pass"},
	})
}

func TestGitConfigSourceReset(t *testing.T) {
	for _, tc := range []tableCase{
		{name: "env unset", command: `env -u GIT_CONFIG_GLOBAL git status`, want: "deny"},
		{name: "env unset joined", command: `env --unset=GIT_CONFIG_SYSTEM git status`, want: "deny"},
		{name: "env empty unset", command: `env --unset= git status`, want: "deny"},
		{name: "env short joined", command: `env -uGIT_CONFIG_SYSTEM git status`, want: "deny"},
		{name: "env reset", command: `env -i git status`, want: "deny"},
		{name: "env reset unrelated child", command: `env --ignore-environment printf ok`, want: "deny"},
		{name: "env reset without child", command: `env -i`, want: "deny"},
		{name: "env nested", command: `env -u GIT_CONFIG_GLOBAL bash -c 'env -u OTHER git status'`, want: "deny"},
		{name: "env unrelated unset", command: `env -u OTHER git status`, want: "pass"},
		{name: "unset literal", command: `unset GIT_CONFIG_GLOBAL`, want: "deny"},
		{name: "unset variable", command: `unset -v GIT_CONFIG_SYSTEM`, want: "deny"},
		{name: "unexport literal", command: `export -n GIT_CONFIG_GLOBAL`, want: "deny"},
		{name: "declare unexport", command: `declare +x GIT_CONFIG_SYSTEM`, want: "deny"},
		{name: "typeset unexport", command: `typeset +x GIT_CONFIG_SYSTEM`, want: "deny"},
		{name: "unset dynamic", command: `unset "$NAME"`, want: "deny"},
		{name: "unset unrelated", command: `unset OTHER`, want: "pass"},
		{name: "unset function", command: `unset -f GIT_CONFIG_GLOBAL`, want: "pass"},
		{name: "unset competing roles", command: `unset -f -v GIT_CONFIG_GLOBAL`, want: "deny"},
		{name: "PS remove", tool: "PowerShell", command: `Remove-Item Env:GIT_CONFIG_GLOBAL`, want: "deny"},
		{name: "PS remove literal path", tool: "PowerShell", command: `Remove-Item -LiteralPath Env:GIT_CONFIG_SYSTEM`, want: "deny"},
		{name: "PS remove unrelated", tool: "PowerShell", command: `Remove-Item Env:OTHER`, want: "pass"},
		{name: "PS competing remove", tool: "PowerShell", command: `Remove-Item Env:GIT_CONFIG_GLOBAL -Path harmless`, want: "deny"},
		{name: "malformed alternate wrapper", command: `GIT_CONFIG_GLOBAL=private-source-canary env --unknown git status`, want: "deny"},
		{name: "PS static null", tool: "PowerShell", command: `[Environment]::SetEnvironmentVariable('GIT_CONFIG_GLOBAL', $null, 'Process')`, want: "deny"},
	} {
		if tc.want == "deny" {
			tc.wantReason = "1 blocking threat(s) detected: command_parse_limit - " + alternateGitConfigSuggestion
		}
		t.Run(tc.name, func(t *testing.T) { assertCaseSemantics(t, tc, recordCase(t, tc)) })
	}
}

func TestGitConfigSourceWithoutChildInvocation(t *testing.T) {
	for _, tc := range []tableCase{
		{name: "reset empty shell", command: `env -i bash -c ''`, want: "deny"},
		{name: "reset bare assignment", command: `env -i bash -c 'OTHER=value'`, want: "deny"},
		{name: "reset inspection", command: `env -i command -v git`, want: "deny"},
		{name: "alternate empty shell", command: `env GIT_CONFIG_GLOBAL=cfg bash -c ''`, want: "deny"},
		{name: "alternate bare assignment", command: `env GIT_CONFIG_GLOBAL=cfg bash -c 'OTHER=value'`, want: "deny"},
		{name: "confirmed empty shell", command: `env GIT_CONFIG_KEY_0=core.hooksPath bash -c ''`, want: "deny", wantReason: hookBypassReason},
		{name: "inner confirmed priority", command: `env -i bash -c 'GIT_CONFIG_KEY_0=core.hooksPath'`, want: "deny", wantReason: hookBypassReason},
		{name: "outer confirmed priority", command: `env GIT_CONFIG_KEY_0=core.hooksPath bash -c 'env -i command -v git'`, want: "deny", wantReason: hookBypassReason},
		{name: "unrelated assignment", command: `env OTHER=value bash -c ''`, want: "pass"},
	} {
		if tc.want == "deny" && tc.wantReason == "" {
			tc.wantReason = "1 blocking threat(s) detected: command_parse_limit - " + alternateGitConfigSuggestion
		}
		t.Run(tc.name, func(t *testing.T) { assertCaseSemantics(t, tc, recordCase(t, tc)) })
	}
}

// runHookBypassCases fills in the exact deny reason, so a deny produced by
// any other rule fails the case.
func runHookBypassCases(t *testing.T, cases []tableCase) {
	t.Helper()
	for _, tc := range cases {
		if tc.want == "deny" && tc.wantReason == "" {
			tc.wantReason = hookBypassReason
		}
		t.Run(tc.name, func(t *testing.T) { assertCaseSemantics(t, tc, recordCase(t, tc)) })
	}
}

// TestGitOptionUnreadablePosture pins that an option the readers cannot read
// never hides a later bypass, while the same commands without a bypass pass.
func TestGitOptionUnreadablePosture(t *testing.T) {
	runHookBypassCases(t, []tableCase{
		{name: "unknown short option before no-verify", command: `git commit -Z --no-verify -m x`, want: "deny"},
		{name: "unknown long option before no-verify", command: `git commit --bogus --no-verify -m x`, want: "deny"},
		{name: "unknown config option before hooksPath", command: `git config --bogus core.hooksPath x`, want: "deny"},
		{name: "unresolved config abbreviation before hooksPath", command: `git config --unset-a core.hooksPath`, want: "deny"},
		{name: "unknown send-email option before no-validate", command: `git send-email --bogus --no-validate x`, want: "deny"},
		{name: "plain commit", command: `git commit -m x`, want: "pass"},
		{name: "config read of hooksPath", command: `git config --get core.hooksPath`, want: "pass"},
	})
}

func TestGitOptionValuePrecision(t *testing.T) {
	runHookBypassCases(t, []tableCase{
		{name: "merge message value", command: `git merge -m --no-verify feature`, want: "pass"},
		{name: "commit abbreviated message", command: `git commit --mess -n`, want: "pass"},
		{name: "merge later bypass", command: `git merge -m --no-verify --no-verify feature`, want: "deny"},
		{name: "push dry run", command: `git push -n origin feature`, want: "pass"},
		{name: "commit real short bypass", command: `git commit -n -m x`, want: "deny"},
		{name: "commit ambiguous no-value competitor", command: `git commit --a -n`, want: "deny"},
		{name: "commit unknown option", command: `git commit --message-unknown -n`, want: "deny"},
		{name: "commit attached message leaves next flag", command: `git commit --mess=x -n`, want: "deny"},
		{name: "commit optional value leaves next flag", command: `git commit --gpg-sign -n`, want: "deny"},
		{name: "merge abbreviated message", command: `git merge --mess --no-verify feature`, want: "pass"},
		{name: "merge ambiguous strategy", command: `git merge --str --no-verify feature`, want: "deny"},
		{name: "pull value", command: `git pull --upload-p --no-verify origin feature`, want: "pass"},
		{name: "pull optional value", command: `git pull --rebase --no-verify origin feature`, want: "deny"},
		{name: "push value", command: `git push --repo --no-verify feature`, want: "pass"},
		{name: "push abbreviated value", command: `git push --push-o --no-verify origin feature`, want: "pass"},
		{name: "push attached value", command: `git push --repo=x --no-verify feature`, want: "deny"},
		{name: "rebase value", command: `git rebase --ont --no-verify main`, want: "pass"},
		{name: "rebase optional value", command: `git rebase --rebase-merges --no-verify main`, want: "deny"},
		{name: "am value", command: `git am --quoted-c --no-verify p.patch`, want: "pass"},
		{name: "am ambiguous value", command: `git am --i -n p.patch`, want: "deny"},
		{name: "am optional value", command: `git am --show-current-patch -n`, want: "deny"},
		{name: "value then terminator", command: `git merge --message x -- --no-verify`, want: "pass"},
		{name: "merge no-stat then message cluster", command: `git merge -nm --no-verify feature`, want: "pass"},
		{name: "commit unified value", command: `git commit -U -n -m x`, want: "pass"},
		{name: "pull strategy value", command: `git pull -s --no-verify origin feature`, want: "pass"},
		{name: "push option value", command: `git push -o --no-verify origin feature`, want: "pass"},
		{name: "rebase strategy value", command: `git rebase -s --no-verify main`, want: "pass"},
		{name: "am context value", command: `git am -C -n p.patch`, want: "pass"},
		{name: "am strip value", command: `git am -p -n p.patch`, want: "pass"},
		{name: "am short bypass before value", command: `git am -nC 1 p.patch`, want: "deny"},
		{name: "am short later bypass", command: `git am -C 1 -n p.patch`, want: "deny"},
		{name: "merge no-stat later bypass", command: `git merge -nm x --no-verify feature`, want: "deny"},
		{name: "commit unknown short before value", command: `git commit -Zm --no-verify`, want: "deny"},
		{name: "merge unknown short before value", command: `git merge -Zm --no-verify feature`, want: "deny"},
		{name: "am unknown short before value", command: `git am -ZC --no-verify p.patch`, want: "deny"},
		{name: "pull unknown short before value", command: `git pull -Zs --no-verify origin feature`, want: "deny"},
		{name: "push unknown short before value", command: `git push -Zo --no-verify origin feature`, want: "deny"},
		{name: "rebase unknown short before value", command: `git rebase -Zs --no-verify main`, want: "deny"},
	})
}

func TestHookBypassNoVerify(t *testing.T) {
	runHookBypassCases(t, []tableCase{
		{name: "commit", command: `git commit --no-verify -m x`, want: "deny"},
		{name: "push", command: `git push --no-verify origin feature`, want: "deny"},
		{name: "merge", command: `git merge --no-verify feature`, want: "deny"},
		{name: "pull", command: `git pull --no-verify`, want: "deny"},
		{name: "rebase", command: `git rebase --no-verify main`, want: "deny"},
		{name: "am", command: `git am --no-verify p.patch`, want: "deny"},
		{name: "abbreviation", command: `git commit --no-verif -m x`, want: "deny"},
		{name: "shortest abbreviation", command: `git push --no-v origin feature`, want: "deny"},
		{name: "global options before verb", command: `git -C repo --no-pager commit --no-verify -m x`, want: "deny"},
		{name: "bash -c wrapper", command: `bash -c 'git commit --no-verify -m x'`, want: "deny"},
		{name: "PowerShell", tool: "PowerShell", command: `git commit --no-verify -m x`, want: "deny"},
		{name: "commit -n", command: `git commit -n -m x`, want: "deny"},
		{name: "commit cluster", command: `git commit -anm x`, want: "deny"},
		{name: "am -n", command: `git am -n p.patch`, want: "deny"},
		{name: "warn mode gives context", command: `git commit --no-verify -m x`, env: map[string]string{"COMMAND_GUARD_MODE": "warn"}, want: "context"},
		{name: "push -n is dry run", command: `git push -n origin feature`, want: "pass"},
		{name: "commit -uno", command: `git commit -uno -m x`, want: "pass"},
		{name: "message mentions flag", command: `git commit -m "mentions --no-verify"`, want: "pass"},
		{name: "message is the flag", command: `git commit -m --no-verify`, want: "pass"},
		{name: "message starts with -n", command: `git commit -m "-n fixes"`, want: "pass"},
		{name: "attached message", command: `git commit -mn`, want: "pass"},
		{name: "after end of options", command: `git commit -m x -- --no-verify`, want: "pass"},
		{name: "grep pattern", command: `git grep -e --no-verify`, want: "pass"},
		{name: "plain commit", command: `git commit -m x`, want: "pass"},
		{name: "verify-signatures is not the flag", command: `git merge --no-verify-signatures feature`, want: "pass"},
		{name: "force push with bypass reports the bypass", command: `git push --force --no-verify origin main`, want: "deny", wantReason: "1 blocking threat(s) detected: git_hook_bypass - " + hookBypassSuggestion},
	})
}

func TestHookBypassHooksPathOverride(t *testing.T) {
	runHookBypassCases(t, []tableCase{
		{name: "global -c", command: `git -c core.hooksPath=/dev/null commit -m x`, want: "deny"},
		{name: "case-insensitive key", command: `git -c CORE.HOOKSPATH=/dev/null push origin feature`, want: "deny"},
		{name: "bare key", command: `git -c core.hooksPath commit -m x`, want: "deny"},
		{name: "any subcommand", command: `git -c core.hooksPath=/dev/null checkout feature`, want: "deny"},
		{name: "config-env joined", command: `git --config-env=core.hooksPath=HOOKS commit -m x`, want: "deny"},
		{name: "config-env separated", command: `git --config-env core.hooksPath=HOOKS commit -m x`, want: "deny"},
		{name: "dynamic key", command: `git -c "$KEY=/dev/null" commit -m x`, want: "deny", wantReason: gitMutationUnknownReason},
		{name: "dynamic subcommand", command: `git -c core.hooksPath=x "$V" -m x`, want: "deny"},
		{name: "prefix env key", command: `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.hooksPath GIT_CONFIG_VALUE_0=/dev/null git commit -m x`, want: "deny"},
		{name: "env wrapper parameters", command: `env GIT_CONFIG_PARAMETERS="'core.hooksPath'='/dev/null'" git commit -m x`, want: "deny"},
		{name: "export", command: `export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.hooksPath GIT_CONFIG_VALUE_0=/dev/null; git commit -m x`, want: "deny"},
		{name: "prefix on another tool", command: `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.hooksPath GIT_CONFIG_VALUE_0=x npm test`, want: "deny"},
		{name: "dynamic env key", command: `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=$K git commit -m x`, want: "deny", wantReason: gitMutationUnknownReason},
		{name: "PowerShell env write", tool: "PowerShell", command: `$env:GIT_CONFIG_KEY_0 = 'core.hooksPath'; git commit -m x`, want: "deny"},
		{name: "PowerShell joined env write", tool: "PowerShell", command: `$env:GIT_CONFIG_PARAMETERS="'core.hooksPath'='x'"; git commit -m x`, want: "deny"},
		{name: "PowerShell env write, equals attached to value", tool: "PowerShell", command: `$env:GIT_CONFIG_KEY_0 ='core.hooksPath'; git commit -m x`, want: "deny"},
		{name: "PowerShell env write, equals attached to name", tool: "PowerShell", command: `$env:GIT_CONFIG_KEY_0= 'core.hooksPath'; git commit -m x`, want: "deny"},
		{name: "attached -c", command: `git -ccore.hooksPath=/dev/null commit -m x`, want: "deny"},
		{name: "declare", command: `declare -x GIT_CONFIG_KEY_0=core.hooksPath; git commit -m x`, want: "deny"},
		{name: "bare env key", command: `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.hooksPath GIT_CONFIG_VALUE_0=/dev/null; git commit -m x`, want: "deny"},
		{name: "bare reassignment of exported key", command: `export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=old; GIT_CONFIG_KEY_0=core.hooksPath; git commit -m x`, want: "deny"},
		{name: "bare parameters", command: `GIT_CONFIG_PARAMETERS="'core.hooksPath'='/dev/null'"; git commit -m x`, want: "deny"},
		{name: "bare dynamic env key", command: `GIT_CONFIG_KEY_0=$K; git commit -m x`, want: "deny", wantReason: gitMutationUnknownReason},
		{name: "bare env key inside bash -c", command: `bash -c 'GIT_CONFIG_KEY_0=core.hooksPath; git commit -m x'`, want: "deny"},
		{name: "bare unrelated assignment", command: `FOO=bar; git commit -m x`, want: "pass"},
		{name: "bare other env key", command: `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0=/repo; git status`, want: "pass"},
		{name: "other config override", command: `git -c user.name=bot commit -m x`, want: "pass"},
		{name: "other dynamic value", command: `git -c user.name="$NAME" commit -m x`, want: "pass"},
		{name: "dynamic subcommand without override", command: `git "$V" -m x`, want: "pass"},
		{name: "other env key", command: `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0=/repo git status`, want: "pass"},
	})
}

func TestHookBypassConfigWrite(t *testing.T) {
	runHookBypassCases(t, []tableCase{
		{name: "legacy write", command: `git config core.hooksPath /dev/null`, want: "deny"},
		{name: "global write", command: `git config --global core.hooksPath /tmp/no-hooks`, want: "deny"},
		{name: "file scope", command: `git config -f .git/config core.hooksPath x`, want: "deny"},
		{name: "empty value", command: `git config core.hooksPath ""`, want: "deny"},
		{name: "unset", command: `git config --unset core.hooksPath`, want: "deny"},
		{name: "unset subcommand", command: `git config unset core.hooksPath`, want: "deny"},
		{name: "set subcommand", command: `git config set core.hooksPath x`, want: "deny"},
		{name: "case-insensitive key", command: `git config core.HooksPath x`, want: "deny"},
		{name: "replace-all", command: `git config --replace-all core.hooksPath x`, want: "deny"},
		{name: "dynamic key", command: `git config "$K" /dev/null`, want: "deny", wantReason: gitMutationUnknownReason},
		{name: "abbreviated unset-all", command: `git config --unset-a core.hooksPath`, want: "deny"},
		{name: "abbreviated replace-all", command: `git config --rep core.hooksPath x`, want: "deny"},
		{name: "abbreviated add", command: `git config --ad core.hooksPath x`, want: "deny"},
		{name: "abbreviated unset-all on unset subcommand", command: `git config unset --a core.hooksPath`, want: "deny"},
		{name: "abbreviated file option", command: `git config --fil x core.hooksPath y`, want: "deny"},
		{name: "abbreviated value option on set subcommand", command: `git config set --val v core.hooksPath x`, want: "deny"},
		{name: "short type option", command: `git config -t path core.hooksPath /tmp/hooks`, want: "deny"},
		{name: "short type option in a cluster", command: `git config -zt path core.hooksPath /tmp/hooks`, want: "deny"},
		{name: "abbreviated get-all", command: `git config --get-a core.hooksPath`, want: "pass"},
		{name: "abbreviated get-regexp", command: `git config --get-r core.hooksPath`, want: "pass"},
		{name: "ambiguous abbreviation Git rejects", command: `git config --uns core.hooksPath`, want: "pass"},
		{name: "unknown option Git rejects", command: `git config --unset-regexp '^core\.hooksPath$'`, want: "pass"},
		{name: "attached short type read", command: `git config -tpath core.hooksPath`, want: "pass"},
		{name: "get", command: `git config --get core.hooksPath`, want: "pass"},
		{name: "get subcommand", command: `git config get core.hooksPath`, want: "pass"},
		{name: "legacy read", command: `git config core.hooksPath`, want: "pass"},
		{name: "get with value pattern", command: `git config --get core.hooksPath husky`, want: "pass"},
		{name: "joined file read", command: `git config --file=cfg core.hooksPath`, want: "pass"},
		{name: "dynamic file, other key", command: `git config --file "$F" user.name bot`, want: "pass"},
		{name: "other key, dynamic value", command: `git config user.name "$NAME"`, want: "pass"},
		{name: "other key", command: `git config user.name bot`, want: "pass"},
	})
}

func TestGitConfigVerbPrecision(t *testing.T) {
	inspectionReason := "1 blocking threat(s) detected: command_parse_limit - " + gitHookInspectionSuggestion
	runHookBypassCases(t, []tableCase{
		{name: "legacy remove core", command: `git config --remove-section core`, want: "deny"},
		{name: "legacy rename core", command: `git config --rename-section core oldcore`, want: "deny"},
		{name: "modern remove core", command: `git config remove-section core`, want: "deny"},
		{name: "modern rename core", command: `git config rename-section core oldcore`, want: "deny"},
		{name: "case insensitive core", command: `git config remove-section CoRe`, want: "deny"},
		{name: "legacy remove abbreviation", command: `git config --remove-s core`, want: "deny"},
		{name: "legacy rename abbreviation", command: `git config --rename-s core oldcore`, want: "deny"},
		{name: "modern location value", command: `git config remove-section --file core core`, want: "deny"},
		{name: "legacy location value", command: `git config --file core --remove-section example`, want: "pass"},
		{name: "legacy short location value", command: `git config -f core --remove-section example`, want: "pass"},
		{name: "modern joined location value", command: `git config remove-section -fcore example`, want: "pass"},
		{name: "modern rename other section", command: `git config rename-section example core`, want: "pass"},
		{name: "modern remove other section", command: `git config remove-section example`, want: "pass"},
		{name: "legacy remove other section", command: `git config --remove-section example`, want: "pass"},
		{name: "similarly named section", command: `git config remove-section corex`, want: "pass"},
		{name: "core subsection", command: `git config remove-section core.example`, want: "pass"},
		{name: "get remains read only", command: `git config --get core.hooksPath`, want: "pass"},
		{name: "section text consumed by file", command: `git config --file --remove-section user.name`, want: "pass"},
		{name: "modern end options", command: `git config remove-section -- core`, want: "deny"},
		{name: "legacy end options", command: `git config --remove-section -- core`, want: "deny"},
		{name: "quoted variable is literal section", command: `git config remove-section '$SECTION'`, want: "pass"},
		{name: "dynamic old section", command: `git config remove-section "$SECTION"`, want: "deny", wantReason: inspectionReason},
		{name: "legacy dynamic old section", command: `git config --rename-section "$SECTION" oldcore`, want: "deny", wantReason: inspectionReason},
		{name: "unknown option role", command: `git config remove-section --unknown core`, want: "deny", wantReason: inspectionReason},
		{name: "unknown legacy option role", command: `git config --unknown --remove-section core`, want: "deny", wantReason: inspectionReason},
		{name: "ambiguous legacy section operation", command: `git config --r core`, want: "deny", wantReason: inspectionReason},
		{name: "missing section", command: `git config remove-section`, want: "deny", wantReason: inspectionReason},
		{name: "missing rename destination", command: `git config rename-section core`, want: "deny", wantReason: inspectionReason},
		{name: "extra section operand", command: `git config remove-section example core`, want: "deny", wantReason: inspectionReason},
		{name: "attached legacy action operand", command: `git config --remove-section=core`, want: "deny", wantReason: inspectionReason},
		{name: "send email no validate", command: `git send-email --no-validate p.patch`, want: "deny"},
		{name: "send email no validate abbreviation", command: `git send-email --no-val p.patch`, want: "deny"},
		{name: "send email shortest no validate abbreviation", command: `git send-email --no-v p.patch`, want: "deny"},
		{name: "send email undashed negation", command: `git send-email --novalidate p.patch`, want: "deny"},
		{name: "send email single dash option", command: `git send-email -no-validate p.patch`, want: "deny"},
		{name: "send email plus option", command: `git send-email +no-validate p.patch`, want: "deny"},
		{name: "send email case insensitive", command: `git send-email --NO-VALIDATE p.patch`, want: "deny"},
		{name: "send email recipient value", command: `git send-email --to --no-validate p.patch`, want: "pass"},
		{name: "send email exact subject value", command: `git send-email --subject --no-validate p.patch`, want: "pass"},
		{name: "send email abbreviated subject value", command: `git send-email --subj --no-validate p.patch`, want: "pass"},
		{name: "send email attached subject value", command: `git send-email --subject=--no-validate p.patch`, want: "pass"},
		{name: "send email later real flag", command: `git send-email --subject --no-validate --no-validate p.patch`, want: "deny"},
		{name: "send email attached subject leaves next flag", command: `git send-email --subject=x --no-validate p.patch`, want: "deny"},
		{name: "send email optional password leaves flag", command: `git send-email --smtp-pass --no-validate p.patch`, want: "deny"},
		{name: "send email optional domain leaves flag", command: `git send-email --smtp-domain --no-validate p.patch`, want: "deny"},
		{name: "send email optional debug leaves flag", command: `git send-email --smtp-debug --no-validate p.patch`, want: "deny"},
		{name: "send email integer cannot consume flag", command: `git send-email --batch-size --no-validate p.patch`, want: "deny"},
		{name: "send email ambiguous option cannot consume flag", command: `git send-email --smtp-s --no-validate p.patch`, want: "deny"},
		{name: "send email unknown option cannot consume flag", command: `git send-email --subject-unknown --no-validate p.patch`, want: "deny"},
		{name: "send email terminator", command: `git send-email -- --no-validate`, want: "pass"},
		{name: "send email normal validation", command: `git send-email --validate p.patch`, want: "pass"},
		{name: "send email identity parsed first", command: `git send-email --subject --identity --no-validate`, want: "pass"},
		{name: "send email identity value then real flag", command: `git send-email --identity --no-validate --no-validate p.patch`, want: "deny"},
		{name: "send email help parsed before subject", command: `git send-email --subject -h --no-validate`, want: "pass"},
		{name: "confirmed override before dynamic section", command: `git -c core.hooksPath=x config remove-section "$SECTION"`, want: "deny"},
		{name: "conflicting legacy section and read actions", command: `git config --get --remove-section core`, want: "deny", wantReason: inspectionReason},
		{name: "unsupported modern short flag", command: `git config remove-section -z core`, want: "deny", wantReason: inspectionReason},
	})
}

// TestGitLineRedaction pins which git lines hide every finding's match. A line
// the git grammar cannot read, or one that only prints, may still carry a
// configuration value in the words left over, so its findings are redacted;
// so is a `config` verb in any letter case. A line read to an ordinary verb,
// or one with no words left over, keeps the match shown. guard.log is read
// rather than the diagnostic table, because the table cuts a match short.
func TestGitLineRedaction(t *testing.T) {
	const redacted = "configuration-bearing command (match redacted)"
	matches := func(t *testing.T, got semanticRecord) []string {
		t.Helper()
		var log guardLogRecord
		if err := json.Unmarshal([]byte(got.GuardLog), &log); err != nil {
			t.Fatalf("guard.log is not a semantic record: %v", err)
		}
		if len(log.Threats) == 0 {
			t.Fatal("guard.log records no threat")
		}
		out := make([]string, 0, len(log.Threats))
		for _, threat := range log.Threats {
			out = append(out, threat.Match)
		}
		return out
	}
	alone := matches(t, recordCase(t, tableCase{command: "rm -rf /", want: "deny"}))
	if len(alone) != 1 || alone[0] == redacted {
		t.Fatalf("rm -rf / alone logs %q, want one shown match", alone)
	}
	shown := alone[0]
	// A bare -c without its value is a second blocking finding of its own, so
	// that row leads with the same root deletion but counts two threats.
	twoThreats := "2" + strings.TrimPrefix(denyRootReason, "1")
	for _, tc := range []struct {
		prefix string
		hidden bool
		reason string // empty: the reason is not pinned
	}{
		{"git --unknown status", true, denyRootReason},
		{"git CONFIG x y", true, denyRootReason},
		{"git Config x y", true, denyRootReason},
		{"git $VERB x", true, ""},
		{"git -h status", true, denyRootReason},
		{"git -c x=y status", true, denyRootReason},
		{"git -c", true, twoThreats},
		{"git config x y", true, denyRootReason},
		{"git --exec-path config x", true, denyRootReason},
		{"git -- config x", true, denyRootReason},
		{"git --version", false, denyRootReason},
		{"git --unknown", false, denyRootReason},
		{"git status", false, denyRootReason},
		{"git -C config status", false, denyRootReason},
	} {
		command := tc.prefix + "; rm -rf /"
		t.Run(command, func(t *testing.T) {
			got := recordCase(t, tableCase{command: command})
			if kind := got.Outcome.Kind(); kind != "deny" {
				t.Fatalf("outcome = %s, want deny (diag %q)", got.Outcome, got.Diag)
			}
			if reason := got.Outcome.Payload(); tc.reason != "" && reason != tc.reason {
				t.Fatalf("reason = %q, want %q", reason, tc.reason)
			}
			want := shown
			if tc.hidden {
				want = redacted
			}
			for _, match := range matches(t, got) {
				if match != want {
					t.Errorf("guard.log match = %q, want %q", match, want)
				}
			}
			if tc.hidden && (strings.Contains(got.Diag+got.GuardLog, "rm -rf /") || strings.Contains(got.Diag+got.GuardLog, shown)) {
				t.Errorf("redacted line leaks the command: diag %q, guard.log %q", got.Diag, got.GuardLog)
			}
		})
	}
}
