package commandguard

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func TestDeletionUncertaintyPerInvocation(t *testing.T) {
	const inspection = "Use literal deletion operands and supported options so the guard can inspect every target."
	const protected = "Do not delete the .git directory. Use Git commands to manage repository state."
	for _, dialect := range []struct {
		name, tool, broad, targeted, suggestion string
		mode                                    shellwords.Dialect
	}{
		{"POSIX", "Bash", "rm -rf /[a-z]*", "rm -f build/[a-z]*", "Remove only specific, verified paths instead of the filesystem root. If the user still wants to delete the root, tell the user that agents cannot perform this action and ask them to run it manually in their own terminal.", shellwords.POSIX},
		{"PowerShell", "PowerShell", "Remove-Item -Recurse -Force $env:USERPROFILE", "Remove-Item -Recurse -Force $env:USERPROFILE/project", destructiveFileSuggestion, shellwords.PowerShell},
	} {
		t.Run(dialect.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, command                string
				broad, inspection, protected bool
			}{
				{"broad alone", dialect.broad, true, false, false},
				{"targeted alone", dialect.targeted, false, true, false},
				{"broad then targeted", dialect.broad + "; " + dialect.targeted, true, true, false},
				{"targeted then broad", dialect.targeted + "; " + dialect.broad, true, true, false},
				{"broad with protected operand", dialect.broad + " .git/config", true, false, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					got := recordCase(t, tableCase{tool: dialect.tool, command: tc.command})
					if got.Outcome.Kind() != "deny" {
						t.Fatalf("deletion outcome = %s, want deny", got.Outcome)
					}
					var log guardLogRecord
					if err := json.Unmarshal([]byte(got.GuardLog), &log); err != nil {
						t.Fatal(err)
					}
					want := map[string]string{}
					if tc.broad {
						want[dialect.suggestion] = "destructive_file_ops/critical"
					}
					if tc.inspection {
						want[inspection] = "command_parse_limit/high"
					}
					if tc.protected {
						want[protected] = "destructive_file_ops/critical"
						want[hookEditSuggestion] = "git_hook_bypass/high"
					}
					// Incomplete inspection retains legacy protected-path evidence
					// as well as the structured operand finding.
					if log.ThreatCount != len(log.Threats) || (!tc.protected && len(log.Threats) != len(want)) {
						t.Fatalf("deletion finding count = %d/%d, want %d: %s", len(log.Threats), log.ThreatCount, len(want), got.GuardLog)
					}
					protectedSeen := false
					for _, threat := range log.Threats {
						if expected, ok := want[threat.Suggestion]; !ok || threat.Category+"/"+threat.Severity != expected {
							t.Fatalf("unexpected deletion finding: %+v", threat)
						}
						if !tc.protected || threat.Suggestion != protected {
							delete(want, threat.Suggestion)
						} else if strings.Contains(threat.Match, ".git/config") {
							protectedSeen = true
						}
					}
					if tc.protected {
						delete(want, protected)
					}
					if len(want) != 0 {
						t.Fatalf("missing deletion findings: %v", want)
					}
					if tc.inspection && !strings.Contains(got.Diag, "command_parse_limit") {
						t.Fatalf("separate uncertain deletion lost inspection diagnostic: %s", got.Diag)
					}
					if tc.protected && !protectedSeen {
						t.Fatalf("protected operand evidence lost: %s", got.GuardLog)
					}
				})
			}
			t.Run("incomplete", func(t *testing.T) {
				parsed := shellwords.Parse(dialect.broad, dialect.mode)
				if checkProtectedDeletion(parsed.Invocations, func(rule, string, string) {}) {
					t.Fatal("broad uncertainty incorrectly established completeness")
				}
			})
		})
	}
}

func TestPowerShellDeletionPrecision(t *testing.T) {
	for _, tc := range []tableCase{
		{name: "scratch", tool: "PowerShell", command: `Remove-Item -LiteralPath '.git-commit-msg.txt'`, want: "pass"},
		{name: "protected child", tool: "PowerShell", command: `Remove-Item -LiteralPath '.git/config'`, want: "deny", wantReason: "2 blocking threat(s) detected: destructive_file_ops - Do not delete the .git directory. Use Git commands to manage repository state."},
		{name: "protected env", tool: "PowerShell", command: `Remove-Item -LiteralPath '.env.production'`, want: "deny", wantReason: "1 blocking threat(s) detected: destructive_file_ops - Move the .env file to a backup location instead of removing it."},
	} {
		t.Run(tc.name, func(t *testing.T) { assertCaseSemantics(t, tc, recordCase(t, tc)) })
	}
}

func TestPowerShellDeletionAliasesAndOptions(t *testing.T) {
	for _, name := range []string{"Remove-Item", "rm", "ri", "del"} {
		for _, tc := range []tableCase{
			{name: "literal scratch", command: ` -LiteralPath '.git-commit-msg.txt'`, want: "pass"},
			{name: "path scratch", command: ` -Force -Path 'cache/file.txt'`, want: "pass"},
			{name: "literal wildcard", command: ` -LiteralPath 'cache/*.txt'`, want: "pass"},
			{name: "recursive literal wildcard", command: ` -Recurse -LiteralPath './*'`, want: "pass"},
			{name: "quoted punctuation", command: ` -LiteralPath 'cache/@item,$name.txt'`, want: "pass"},
			{name: "filter value", command: ` -Filter '.git' -LiteralPath 'cache'`, want: "pass"},
			{name: "include value", command: ` -Include '.env' -LiteralPath 'cache'`, want: "pass"},
			{name: "exclude value", command: ` -Exclude '.git' -LiteralPath 'cache'`, want: "pass"},
			{name: "credential value", command: ` -Credential 'synthetic' -LiteralPath 'cache'`, want: "pass"},
			{name: "stream value", command: ` -Stream 'synthetic' -LiteralPath 'cache'`, want: "pass"},
			{name: "protected child", command: ` -LiteralPath '.git/config'`, want: "deny"},
			{name: "multiple targets", command: ` scratch.txt .env.production other.txt`, want: "deny"},
			{name: "case and normalization", command: ` -LiteralPath 'cache/../.GiT/config'`, want: "deny"},
			{name: "dynamic target", command: ` -LiteralPath $target`, want: "deny"},
			{name: "path wildcard", command: ` -Path 'cache/*'`, want: "deny"},
			{name: "splat", command: ` @targets`, want: "deny"},
			{name: "array", command: ` scratch.txt,other.txt`, want: "deny"},
			{name: "dynamic filter", command: ` -Filter $filter -LiteralPath cache`, want: "deny"},
			{name: "unknown option", command: ` -Unknown value -LiteralPath cache`, want: "deny"},
			{name: "posix flags", command: ` -rf cache`, want: "deny"},
			{name: "quoted option is target", command: ` '-Filter' '.git' scratch.txt`, want: "deny"},
			{name: "recursive root", command: ` -Recurse -Force C:\`, want: "deny"},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				tc.tool = "PowerShell"
				tc.command = name + tc.command
				switch tc.name {
				case "protected child", "case and normalization":
					tc.wantReason = "2 blocking threat(s) detected: destructive_file_ops - Do not delete the .git directory. Use Git commands to manage repository state."
				case "quoted option is target":
					tc.wantReason = "1 blocking threat(s) detected: destructive_file_ops - Do not delete the .git directory. Use Git commands to manage repository state."
				case "multiple targets":
					tc.wantReason = "1 blocking threat(s) detected: destructive_file_ops - Move the .env file to a backup location instead of removing it."
				case "recursive root":
					tc.wantReason = "1 blocking threat(s) detected: destructive_file_ops - " + destructiveFileSuggestion
				default:
					if tc.want == "deny" {
						tc.wantReason = "1 blocking threat(s) detected: command_parse_limit - Use literal deletion operands and supported options so the guard can inspect every target."
					}
				}
				assertCaseSemantics(t, tc, recordCase(t, tc))
			})
		}
	}
}

func TestPowerShellDeletionCompleteness(t *testing.T) {
	for _, command := range []string{
		`custom-remove -LiteralPath '.git-commit-msg.txt'`,
		`Set-Alias rm custom-remove; rm -LiteralPath '.git-commit-msg.txt'`,
		`function ri { custom-remove }; ri -LiteralPath '.git-commit-msg.txt'`,
		`rm @targets`,
		`ri -Path 'cache/*'`,
		`del -Filter $filter -LiteralPath 'cache'`,
		`Remove-Item -LiteralPath cache,other`,
		`rm -LiteralPath cache -Unknown value`,
		`Remove-Item Env:OTHER`,
		`Remove-Item -LiteralPath Env:GIT_CONFIG_GLOBAL`,
	} {
		t.Run(command, func(t *testing.T) {
			parsed := shellwords.Parse(command, shellwords.PowerShell)
			if checkProtectedDeletion(parsed.Invocations, func(rule, string, string) {}) {
				t.Fatal("ambiguous execution context established protected-deletion completeness")
			}
		})
	}
}

func TestPowerShellDeletionUnsupportedPaths(t *testing.T) {
	paths := []string{
		`C:.git/config`, `.git:stream`, `cache:stream`,
		`Env::.git/config`, `Env::.env.production`, `Env::C:\`,
		`Repo:.git/config`, `Repo:.env.production`, `Repo:\`,
		`\\server\share\.git\config`, `\\server\share\.env.production`, `\\server\share\`,
		`\\?\C:\.git\config`, `\\?\C:\.env.production`, `\\?\C:\`,
		`\\.\C:\.git\config`, `\\.\C:\`,
	}
	for _, provider := range []string{"FileSystem::", `Microsoft.PowerShell.Core\FileSystem::`, "UnknownProvider::"} {
		for _, target := range []string{`.git/config`, `.env.production`, `C:\`} {
			paths = append(paths, provider+target)
		}
	}
	for _, name := range []string{"Remove-Item", "rm", "ri", "del"} {
		for _, path := range paths {
			t.Run(name+"/"+path, func(t *testing.T) {
				command := name + ` -Recurse -LiteralPath '` + path + `'`
				tc := tableCase{tool: "PowerShell", command: command, want: "deny"}
				got := recordCase(t, tc)
				if got.Outcome.Kind() != "deny" || got.Outcome.Payload() == "" {
					t.Fatalf("unsupported path was not denied: %v", got.Outcome)
				}
				if !got.LogExists || !strings.Contains(got.Diag, "command_parse_limit") {
					t.Fatalf("unsupported path inspection finding was not recorded: %s", got.Diag)
				}
				if path == `FileSystem::.git/config` {
					wantReason := "1 blocking threat(s) detected: command_parse_limit - Use literal deletion operands and supported options so the guard can inspect every target."
					if name == "rm" || name == "del" {
						wantReason = "2 blocking threat(s) detected: destructive_file_ops - Do not delete the .git directory. Use Git commands to manage repository state."
					}
					if got.Outcome.Payload() != wantReason {
						t.Fatalf("reason = %q, want %q", got.Outcome.Payload(), wantReason)
					}
				}
				parsed := shellwords.Parse(command, shellwords.PowerShell)
				inspectionDenial := false
				complete := checkProtectedDeletion(parsed.Invocations, func(r rule, match, suggestion string) {
					if r.category == "command_parse_limit" && r.severity == "high" && suggestion == "Use literal deletion operands and supported options so the guard can inspect every target." {
						inspectionDenial = true
					}
				})
				if !inspectionDenial {
					t.Fatal("unsupported path lacks inspection denial")
				}
				if complete {
					t.Fatal("unsupported path established protected-deletion completeness")
				}
			})
		}
	}
}

func TestProtectedDeletionPrecision(t *testing.T) {
	for _, tc := range []tableCase{
		{name: "scratch prefix", command: `rm -f .git-commit-msg.txt`, want: "pass"},
		{name: "later listing", command: `rm -f scratch.txt; ls .git`, want: "pass"},
		{name: "later clustered listing", command: `rm -f scratch.txt; ls -la .git`, want: "pass"},
		{name: "later listing with lone dash", command: `rm -f scratch.txt; ls - .git`, want: "pass"},
		{name: "later listing after end marker", command: `rm -f scratch.txt; ls -la -- -la .git`, want: "pass"},
		{name: "later search", command: `rm -f scratch.txt; grep -n '\.Env' source.txt`, want: "pass"},
		{name: "docker option", command: `docker run --rm image ls .git`, want: "pass"},
		{name: "docker bind mount", command: `docker run --rm --mount type=bind,source=/work/.git,target=/g,readonly image ls /g`, want: "pass"},
		{name: "unlink scratch", command: `unlink scratch.txt`, want: "pass"},
		{name: "del scratch", command: `del scratch.txt`, want: "pass"},
		{name: "env directory child", command: `rm .env.directory/scratch.txt`, want: "pass"},
	} {
		t.Run(tc.name, func(t *testing.T) { assertCaseSemantics(t, tc, recordCase(t, tc)) })
	}
	for _, tc := range []struct{ name, command, reason string }{
		{"git child", `rm -f .git/config`, "destructive_file_ops - Do not delete the .git directory."},
		{"env production", `rm .env.production`, "destructive_file_ops - Move the .env file"},
		{"env example", `rm .env.example`, "destructive_file_ops - Move the .env file"},
		{"unlink git", `unlink .git/config`, "destructive_file_ops - Do not delete the .git directory."},
		{"unlink end marker", `unlink -- .env`, "destructive_file_ops - Move the .env file"},
		{"del git", `del .git/config`, "destructive_file_ops - Do not delete the .git directory."},
		{"del unknown option", `del /q .git/config`, "destructive_file_ops - Do not delete the .git directory."},
		{"multiple targets", `rm -riv scratch.txt .env.production other`, "destructive_file_ops - Move the .env file"},
		{"escaped operand", `rm .g\it/config`, "destructive_file_ops - Do not delete the .git directory."},
		{"dot segments", `rm parent/../.git/./config`, "destructive_file_ops - Do not delete the .git directory."},
		{"case insensitive", `rm .EnV.Production`, "destructive_file_ops - Move the .env file"},
		{"broad root", `rm -rf /`, "destructive_file_ops - Remove only specific, verified paths instead of the filesystem root."},
		{"unknown flag", `rm --unknown .git/config`, "destructive_file_ops - Do not delete the .git directory."},
		{"opaque wrapper", `custom-wrapper rm .git/config`, "destructive_file_ops - Do not delete the .git directory."},
		{"python embedded deletion", `python -c "import os; os.system('rm .git/config')"`, "destructive_file_ops - Do not delete the .git directory."},
		{"ruby embedded deletion", `ruby -e "system('unlink .env.production')"`, "destructive_file_ops - Move the .env file"},
		{"option attached deletion", `custom-wrapper --command=rm .git/config`, "destructive_file_ops - Do not delete the .git directory."},
		{"docker embedded deletion", `docker run --rm image python -c "import os; os.system('rm .git/config')"`, "destructive_file_ops - Do not delete the .git directory."},
		{"docker mount embedded deletion", `docker run --rm --mount type=bind,source=/work/.git,target=/g,readonly image python -c "import os; os.system('rm /g/config')"`, "destructive_file_ops"},
		{"later embedded deletion", `rm scratch.txt; python -c "import os; os.system('rm .git/config')"`, "destructive_file_ops - Do not delete the .git directory."},
		{"unknown listing option", `ls --command="rm .git/config"`, "destructive_file_ops - Do not delete the .git directory."},
		{"flag listing joined with equals", `rm scratch.txt; ls --all=x .git`, "destructive_file_ops - Do not delete the .git directory."},
		{"cluster listing joined with equals", `rm scratch.txt; ls -l=x .git`, "destructive_file_ops - Do not delete the .git directory."},
		{"unknown listing cluster", `rm scratch.txt; ls -laZ .git`, "destructive_file_ops - Do not delete the .git directory."},
		{"value taking listing cluster", `rm scratch.txt; ls -law .git`, "destructive_file_ops - Do not delete the .git directory."},
		{"unknown search consumer", `rg --pre="rm .git/config" pattern source.txt`, "destructive_file_ops - Do not delete the .git directory."},
		{"docker entrypoint override", `docker run --rm --entrypoint=python image -c "import os; os.system('rm .git/config')"`, "destructive_file_ops - Do not delete the .git directory."},
		{"cmd code", `cmd /c "del .git/config"`, "destructive_file_ops - Do not delete the .git directory."},
		{"eval code", `eval 'rm .git/config'`, "destructive_file_ops - Do not delete the .git directory."},
		{"docker child", `docker run --rm image rm .git/config`, "destructive_file_ops - Do not delete the .git directory."},
		{"dynamic operand", `rm "$TARGET"`, "command_parse_limit"},
		{"mixed quoted glob", `rm "."*`, "command_parse_limit"},
		{"parse failure", `rm 'unterminated`, "command_parse_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := tableCase{name: tc.name, command: tc.command, want: "deny"}
			got := recordCase(t, c)
			assertCaseSemantics(t, c, got)
			if !strings.Contains(got.Outcome.Payload(), tc.reason) {
				t.Fatalf("reason = %q, want %q", got.Outcome.Payload(), tc.reason)
			}
		})
	}
	t.Run("missing command fallback", func(t *testing.T) {
		tc := tableCase{input: `{"script":"rm .git/config"}`, want: "deny"}
		got := recordCase(t, tc)
		assertCaseSemantics(t, tc, got)
		if !strings.Contains(got.Outcome.Payload(), "destructive_file_ops") {
			t.Fatal(got.Outcome.Payload())
		}
	})
}

func TestDestructiveOptionBoundariesThroughRun(t *testing.T) {
	for _, tc := range []tableCase{
		{name: "chmod reference consumes numeric mode", command: `chmod --reference 777 file`, want: "pass"},
		{name: "chmod joined reference", command: `chmod --reference=777 file`, want: "pass"},
		{name: "chmod end marker world write", command: `chmod -- o=w file`, want: "deny"},
		{name: "chmod zero mode", command: `chmod 000 file`, want: "pass"},
		{name: "chmod empty symbolic permissions", command: `chmod o= file`, want: "pass"},
		{name: "literal broad removal path", tool: "PowerShell", command: `Remove-Item -Recurse -LiteralPath C:\`, want: "deny"},
		{name: "bounded removal path", tool: "PowerShell", command: `Remove-Item -Recurse -Path C:\cache`, want: "pass"},
		{name: "filter consumes broad target", tool: "PowerShell", command: `Remove-Item -Recurse -Filter C:\ -LiteralPath C:\cache`, want: "pass"},
		{name: "include consumes broad target", tool: "PowerShell", command: `Remove-Item -Recurse -Include C:\ -Path C:\cache`, want: "pass"},
		{name: "exclude consumes broad target", tool: "PowerShell", command: `Remove-Item -Recurse -Exclude C:\ -Path C:\cache`, want: "pass"},
		{name: "attached find optimization broad root", command: `find -O3 / -delete`, want: "deny"},
		{name: "attached find debug bounded root", command: `find -Dsearch build/cache -delete`, want: "context"},
	} {
		t.Run(tc.name, func(t *testing.T) { assertCaseSemantics(t, tc, recordCase(t, tc)) })
	}
}

func literalWordsForTest(values ...string) []shellwords.Word {
	out := make([]shellwords.Word, len(values))
	for i, value := range values {
		out[i] = shellwords.Word{Text: value, Raw: value, Literal: true}
	}
	return out
}

func TestDestructiveHardening(t *testing.T) {
	t.Run("find predicate before delete", func(t *testing.T) {
		_, broad, active := classifyFindDelete(literalWordsForTest("find", "/", "-type", "f", "-delete"))
		if !active || !broad {
			t.Fatal("broad find delete was not classified")
		}
	})
	for _, tc := range []struct {
		name, command string
		broad         bool
	}{
		{"logical option targeted", "find -L build/cache -delete", false},
		{"double dash targeted", "find -- build/cache -delete", false},
		{"debug option targeted", "find -D search build/cache -delete", false},
		{"logical option broad root", "find -L / -delete", true},
		{"double dash broad root", "find -- / -delete", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			words := shellwords.Parse(tc.command, shellwords.POSIX).Invocations
			if len(words) != 1 {
				t.Fatalf("invocations = %#v", words)
			}
			_, broad, active := classifyFindDelete(words[0].Words)
			if !active || broad != tc.broad {
				t.Fatalf("classification = active %v broad %v, want active true broad %v", active, broad, tc.broad)
			}
		})
	}
	for _, tc := range []struct {
		name, command string
		broad         bool
	}{
		{"POSIX root alias", "rm --force --recursive /synthetic/../*", true},
		{"relative current glob alias", "rm --force --recursive synthetic/../*", true},
		{"relative current alias", "rm --force --recursive synthetic/..", true},
		{"relative targeted alias", "rm --force --recursive synthetic/../cache", false},
		{"POSIX targeted parent", "rm --force --recursive /synthetic/../synthetic/cache", false},
		{"PowerShell drive root alias", `Remove-Item -Recurse -Force C:\synthetic\..\*`, true},
		{"PowerShell drive targeted parent", `Remove-Item -Recurse -Force C:\synthetic\..\Temp`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dialect := shellwords.POSIX
			if strings.HasPrefix(tc.command, "Remove-Item") {
				dialect = shellwords.PowerShell
			}
			invocations := shellwords.Parse(tc.command, dialect).Invocations
			if len(invocations) != 1 {
				t.Fatalf("invocations = %#v", invocations)
			}
			var got bool
			if dialect == shellwords.POSIX {
				_, got, _ = classifyRM(invocations[0].Words)
			} else {
				_, got = broadRemoveItem(invocations[0].Words)
			}
			if got != tc.broad {
				t.Fatalf("broad = %v, want %v", got, tc.broad)
			}
		})
	}
	for _, tc := range []struct {
		name, command string
		broad         bool
	}{
		{"PowerShell profile root", "Remove-Item -Recurse -Force $env:USERPROFILE", true},
		{"PowerShell braced profile glob", "Remove-Item -Recurse -Force ${env:USERPROFILE}/*", true},
		{"PowerShell drive root", `Remove-Item -Recurse -Force C:\`, true},
		{"PowerShell drive root glob", `Remove-Item -Recurse -Force C:\*`, true},
		{"PowerShell drive child", `Remove-Item -Recurse -Force C:\Temp`, false},
		{"PowerShell profile child", "Remove-Item -Recurse -Force $env:USERPROFILE/project", false},
		{"PowerShell unrelated variable", "Remove-Item -Recurse -Force $env:OTHER", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invocations := shellwords.Parse(tc.command, shellwords.PowerShell).Invocations
			if len(invocations) != 1 {
				t.Fatalf("invocations = %#v", invocations)
			}
			_, got := broadRemoveItem(invocations[0].Words)
			if got != tc.broad {
				t.Fatalf("broad = %v, want %v", got, tc.broad)
			}
		})
	}

	t.Run("targeted rm", func(t *testing.T) {
		_, broad, active := classifyRM(literalWordsForTest("rm", "-fr", "node_modules"))
		if !active || broad {
			t.Fatal("targeted rm classification changed")
		}
	})

	t.Run("all invocations are inspected after advisory cleanup", func(t *testing.T) {
		result := shellwords.Parse("rm -rf node_modules; rm --force --recursive /", shellwords.POSIX)
		var categories []string
		checkDestructiveCommands(result.Invocations, func(r rule, _, _ string) {
			categories = append(categories, r.category)
		})
		if len(categories) != 2 || categories[0] != "command_reformulation" || categories[1] != "destructive_file_ops" {
			t.Fatalf("categories = %#v, want targeted advisory followed by broad denial", categories)
		}
	})

	t.Run("find advisory does not hide later chmod denial", func(t *testing.T) {
		result := shellwords.Parse("find build/cache -delete; chmod 1777 build", shellwords.POSIX)
		var categories []string
		checkDestructiveCommands(result.Invocations, func(r rule, _, _ string) {
			categories = append(categories, r.category)
		})
		if len(categories) != 2 || categories[0] != "command_reformulation" || categories[1] != "permission_abuse" {
			t.Fatalf("categories = %#v, want targeted advisory followed by permission denial", categories)
		}
	})

	for _, tc := range []struct {
		name, command, category string
	}{
		{"quoted reordered flags", `rm "--force" "--recursive" "/"`, "destructive_file_ops"},
		{"long flags", "rm --recursive --force /", "destructive_file_ops"},
		{"executable path", "/bin/chmod o+w build", "permission_abuse"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := shellwords.Parse(tc.command, shellwords.POSIX)
			found := false
			checkDestructiveCommands(result.Invocations, func(r rule, _, _ string) {
				if r.category == tc.category {
					found = true
				}
			})
			if !found {
				t.Fatalf("%q did not produce %s", tc.command, tc.category)
			}
		})
	}

	for _, tc := range []struct {
		mode string
		want bool
	}{
		{"2", true}, {"77", true}, {"000777", true},
		{"755", false}, {"000755", false}, {"08", false}, {"00000000000077777", false},
	} {
		t.Run("numeric mode "+tc.mode, func(t *testing.T) {
			if got := numericWorldWrite(tc.mode); got != tc.want {
				t.Fatalf("numericWorldWrite(%q) = %v, want %v", tc.mode, got, tc.want)
			}
		})
	}
}

func TestBroadBracketGlobTargets(t *testing.T) {
	for _, target := range []string{"/[a-z]*", "$HOME/[a-z]*", "./[a-z]*", "../[a-z]*", "[a-z]*", "/[[:alpha:]]*", "/[!0-9]*", "C:/[a-z]*"} {
		t.Run(target, func(t *testing.T) {
			if !isBroadTarget(literalWordsForTest(target)[0]) {
				t.Fatal("broad bracket glob was treated as targeted cleanup")
			}
		})
	}
	for _, target := range []string{"node_modules", "build/[a-z]*", "/tmp/cache/[a-z]*", "$HOME/project/[a-z]*", "/[a-z]cache"} {
		t.Run("targeted "+target, func(t *testing.T) {
			if isBroadTarget(literalWordsForTest(target)[0]) {
				t.Fatal("targeted child was treated as broad cleanup")
			}
		})
	}
}
