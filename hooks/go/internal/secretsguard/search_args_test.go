package secretsguard

import (
	"context"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

type searchProofContext struct {
	context.Context
	checks, cancelAt int
	cancel           context.CancelFunc
}

func (c *searchProofContext) Err() error {
	err := c.Context.Err()
	c.checks++
	if c.cancel != nil && c.checks == c.cancelAt {
		c.cancel()
	}
	return err
}

func TestSearchRoleFinalPublication(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	clearCredentialPathEnv(t)
	root := globFixture(t, "source.txt")
	const command = "rg --no-config '.env' source.txt"
	input := map[string]any{"command": command}
	event := buildEventAt(t, "PowerShell", input, root)
	document, err := event.Read()
	if err != nil {
		t.Fatal(err)
	}
	call, err := toolcall.Read(document)
	if err != nil {
		t.Fatal(err)
	}
	baseline := &hookspectest.CancelContext{Context: context.Background()}
	inspection := inspectShellReads(baseline, call, call.Inspect(baseline, nil))
	if inspection.Reason.Refused() || baseline.Calls == 0 {
		t.Fatalf("search baseline: checks=%d reason=%q", baseline.Calls, inspection.Reason)
	}
	invoke := func(ctx context.Context) hookspec.Outcome {
		return Run(hookspec.Request{Context: ctx, Event: event})
	}
	complete := &hookspectest.CancelContext{Context: context.Background()}
	if out := invoke(complete); out.Kind() != "pass" || complete.Calls != baseline.Calls {
		t.Fatalf("search control: outcome=%s checks=%d inspection=%d", out.Kind(), complete.Calls, baseline.Calls)
	}
	// This fixture's last inspection poll publishes its search argument proof.
	// Earlier cancellation would not exercise that final decision.
	canceled := &hookspectest.CancelContext{Context: context.Background(), At: baseline.Calls}
	out := invoke(canceled)
	if canceled.Calls < canceled.At || out.Kind() != "deny" || !strings.Contains(out.Payload(), "search argument inspection canceled") {
		t.Fatalf("search final publication: outcome=%s checks=%d checkpoint=%d reason=%q", out.Kind(), canceled.Calls, canceled.At, out.Payload())
	}
	retained, err := document.Tool.DecodeInput()
	if err != nil || retained["command"] != command {
		t.Fatal("search publication changed original command evidence")
	}
}

func TestSearchRoleLoopPolling(t *testing.T) {
	const units = 6
	decode := func(ctx context.Context, command string, dialect shellwords.Dialect) searchArguments {
		parsed := shellwords.Parse(command, dialect)
		if len(parsed.Invocations) != 1 {
			t.Fatalf("role fixture invocations=%d", len(parsed.Invocations))
		}
		words := parsed.Invocations[0].Words
		return searchReadArguments(ctx, words[0].Text, words[1:], dialect, false)
	}
	for _, tc := range []struct{ name, target, baseline string }{
		{"long-name", "--" + strings.Repeat("n", units*64*1024+5), "--glob=" + strings.Repeat("n", units*64*1024)},
		{"short-cluster", "-" + strings.Repeat("n", units*64*1024+1), "-e" + strings.Repeat("n", units*64*1024)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.target) != len(tc.baseline) {
				t.Fatal("unequal scalar work")
			}
			prefix, suffix := "rg --no-config --ignore-file=.env ", " smoke source.txt"
			baseline := &searchProofContext{Context: context.Background()}
			decode(baseline, prefix+tc.baseline+suffix, shellwords.PowerShell)
			probe := &searchProofContext{Context: context.Background()}
			decode(probe, prefix+tc.target+suffix, shellwords.PowerShell)
			if probe.checks < baseline.checks+units {
				t.Fatalf("search %s polling: target=%d scalar-baseline=%d minimum-extra=%d", tc.name, probe.checks, baseline.checks, units)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			probe = &searchProofContext{Context: ctx, cancelAt: baseline.checks + units, cancel: cancel}
			result := decode(probe, prefix+tc.target+suffix, shellwords.PowerShell)
			if ctx.Err() == nil || result.reason != shellwords.GuardSearchCanceled || result.patterns != nil || len(result.joinedPaths) == 0 || result.joinedPaths[0].text != ".env" || !result.joinedPaths[0].file {
				t.Fatalf("search %s cancellation: checks=%d reason=%q files=%d", tc.name, probe.checks, result.reason, len(result.joinedPaths))
			}
		})
	}
	t.Run("words", func(t *testing.T) {
		command := "grep --file=.env " + strings.TrimSpace(strings.Repeat("ordinary ", 4096))
		words := shellwords.Parse(command, shellwords.POSIX).Invocations[0].Words[1:]
		scalar := &searchProofContext{Context: context.Background()}
		for _, word := range words {
			searchLiteralArgument(scalar, word, shellwords.POSIX)
		}
		probe := &searchProofContext{Context: context.Background()}
		searchReadArguments(probe, "grep", words, shellwords.POSIX, false)
		if probe.checks-scalar.checks < 4096/256 {
			t.Fatalf("search words polling: total=%d scalar=%d minimum-extra=%d", probe.checks, scalar.checks, 4096/256)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		probe = &searchProofContext{Context: ctx, cancelAt: 20, cancel: cancel}
		result := searchReadArguments(probe, "grep", words, shellwords.POSIX, false)
		if ctx.Err() == nil || result.reason == shellwords.GuardNone || result.patterns != nil || len(result.joinedPaths) == 0 || !result.joinedPaths[0].file {
			t.Fatalf("search words cancellation: checks=%d reason=%q", probe.checks, result.reason)
		}
	})
}

func TestPowerShellSearchPrecisionDecoderCancellation(t *testing.T) {
	const chunks = 4
	for _, form := range []string{"pattern", "joined filter"} {
		t.Run(form, func(t *testing.T) {
			decode := func(ctx context.Context, value string) searchArguments {
				arg := "'" + value + "'"
				if form == "joined filter" {
					arg = "--glob=" + arg + " smoke"
				}
				parsed := shellwords.Parse("rg --no-config --ignore-file=.env "+arg+" source.txt", shellwords.PowerShell)
				return searchReadArguments(ctx, "rg", parsed.Invocations[0].Words[1:], shellwords.PowerShell, false)
			}
			short := &searchProofContext{Context: context.Background()}
			decode(short, "safe")
			long := &searchProofContext{Context: context.Background()}
			value := strings.Repeat("x", chunks*64*1024)
			if result := decode(long, value); result.reason != shellwords.GuardNone || result.patterns == nil {
				t.Fatalf("uncanceled data proof failed: %+v", result)
			}
			if long.checks < short.checks+chunks {
				t.Fatalf("long scalar lacks periodic checks: short=%d long=%d", short.checks, long.checks)
			}
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			probe := &searchProofContext{Context: base, cancelAt: short.checks + chunks, cancel: cancel}
			result := decode(probe, value)
			if base.Err() == nil || result.reason == shellwords.GuardNone || result.patterns != nil {
				t.Fatalf("mid-decoder cancellation published proof: canceled=%v reason=%q patterns=%v", base.Err(), result.reason, result.patterns)
			}
			if len(result.joinedPaths) == 0 || result.joinedPaths[0].text != ".env" || !result.joinedPaths[0].file {
				t.Fatal("cancellation discarded known file evidence")
			}
		})
	}
}

func TestPowerShellSearchPrecisionRunCancellation(t *testing.T) {
	root := globFixture(t, ".env", "source.txt")
	for _, command := range []string{`rg --no-config 'smoke' source.txt`, `rg --no-config --glob='*.txt' smoke source.txt`} {
		t.Run(command, func(t *testing.T) {
			count := &searchProofContext{Context: context.Background()}
			shellwords.ParseContext(count, command, shellwords.PowerShell)
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			probe := &searchProofContext{Context: base, cancelAt: count.checks, cancel: cancel}
			outcome := Run(hookspec.Request{Context: probe, Event: buildEventAt(t, "PowerShell", map[string]any{"command": command}, root)})
			if base.Err() == nil || outcome.Kind() != "deny" {
				t.Fatalf("canceled analysis returned %s; canceled=%v", outcome.Kind(), base.Err())
			}
		})
	}
}

func TestPowerShellSearchPrecision(t *testing.T) {
	root := globFixture(t, ".env", "source.txt")
	for _, command := range []string{
		`grep -n '.env' source.txt`,
		`rg --no-config -n smoke source.txt --glob '!.github/ci-cost/**'`,
		`rg --no-config --glob='.env' smoke source.txt`,
		`grep --exclude '.env' smoke source.txt`,
		`rg --no-config -n 'protected|\.git|\.env' source.txt`,
		`rg --no-config 'Function:.env' source.txt`,
		`rg --no-config 'Alias:.env' source.txt`,
		`rg --no-config '$env:PATH' source.txt`,
		`rg --no-config -e '.env' source.txt`,
		`rg --no-config --regexp='.env' source.txt`,
		`rg --no-config -- '.env' source.txt`,
	} {
		t.Run(command, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "PowerShell", map[string]any{"command": command}, root), nil)
			if kind != "pass" {
				t.Fatalf("kind=%s message=%s; want pass", kind, message)
			}
		})
	}
}

func TestPowerShellSearchPrecisionDenied(t *testing.T) {
	root := globFixture(t, ".env", "source.txt")
	for _, command := range []string{
		`rg --no-config -f .env source.txt`,
		`rg --no-config -f.env source.txt`,
		`grep --file=.env source.txt`,
		`rg --no-config --ignore-file=.env smoke source.txt`,
		`rg --no-config --ignore-file .env smoke source.txt`,
		`grep --exclude-from=.env smoke source.txt`,
		`grep --exclude-from .env smoke source.txt`,
		`rg --no-config smoke .env`,
		`rg -n '.env' source.txt`,
		`rg -- '.env' --no-config source.txt`,
		`rg -e --no-config .env`,
		`rg --no-config smoke -- .env`,
		`rg --no-config --unknown '.env' source.txt`,
		`rg --no-config --pre .env smoke source.txt`,
		`rg --no-config --pre=.env smoke source.txt`,
		`rg --no-config '.env' source.txt < .env`,
		`My.Module\rg --no-config '.env' source.txt`,
		`.\rg --no-config '.env' source.txt`,
		`Set-Alias rg other; rg --no-config '.env' source.txt`,
		`Set-Item Function:rg other; rg --no-config '.env' source.txt`,
		`Write-Output smoke; rg --no-config '.env' source.txt`,
		`rg --no-config '.env' $env:PATH`,
		`rg --no-config '.env' @paths`,
		`rg --no-config '.env' source.txt,other.txt`,
		`rg --no-config '.env' --% source.txt`,
	} {
		t.Run(command, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "PowerShell", map[string]any{"command": command}, root), nil)
			if kind != "deny" || !strings.Contains(message, "(dotenv, critical)") {
				t.Fatalf("kind=%s message=%s; want dotenv deny", kind, message)
			}
		})
	}
}

func TestPowerShellSearchPrecisionAuthority(t *testing.T) {
	root := globFixture(t, ".env", "source.txt")
	for _, tc := range []struct {
		name, tool string
		extra      map[string]any
	}{
		{"selector", "PowerShell", map[string]any{"shell": "powershell"}},
		{"environment", "PowerShell", map[string]any{"env": map[string]any{}}},
		{"metadata", "PowerShell", map[string]any{"metadata": map[string]any{}}},
	} {
		for _, command := range []string{`rg --no-config '.env' source.txt`, `rg --no-config --glob='.env' smoke source.txt`} {
			t.Run(tc.name+"/"+command, func(t *testing.T) {
				input := map[string]any{"command": command}
				for key, value := range tc.extra {
					input[key] = value
				}
				kind, message := run(t, buildEventAt(t, tc.tool, input, root), nil)
				if kind != "deny" || !strings.Contains(message, "(dotenv, critical)") {
					t.Fatalf("kind=%s message=%s; want dotenv deny", kind, message)
				}
			})
		}
	}
}

func TestPowerShellSearchPrecisionNoncanonicalInspection(t *testing.T) {
	for _, tool := range []string{"powershell", "mcp__shell__PowerShell"} {
		for _, command := range []string{`rg --no-config '.env' source.txt`, `rg --no-config --glob='.env' smoke source.txt`} {
			t.Run(tool+"/"+command, func(t *testing.T) {
				hosttest.ScrubGuardEnv(t)
				call := readCall(t, buildEventAt(t, tool, map[string]any{"command": command}, ""))
				ctx := context.Background()
				inspection := inspectShellReads(ctx, call, call.Inspect(ctx, nil))
				for _, path := range inspection.Paths {
					if path.Text == ".env" {
						return
					}
				}
				t.Fatal("unqualified tool gained a PowerShell data exemption")
			})
		}
	}
}

func TestSearchEnvironmentRoles(t *testing.T) {
	for _, tc := range []struct {
		name, command, config, want string
		extra                       map[string]any
	}{
		{"inline config", `RIPGREP_CONFIG_PATH=config.txt rg .env source.txt`, "", "deny", nil},
		{"host config", `rg .env source.txt`, "config.txt", "deny", nil},
		{"export config", `export RIPGREP_CONFIG_PATH=config.txt; rg .env source.txt`, "", "deny", nil},
		{"printf then export", `printf -v RIPGREP_CONFIG_PATH '%s' config.txt; export RIPGREP_CONFIG_PATH; rg .env source.txt`, "", "deny", nil},
		{"printf export no config", `printf -v RIPGREP_CONFIG_PATH '%s' config.txt; export RIPGREP_CONFIG_PATH; rg --no-config literal.key source.txt`, "", "pass", nil},
		{"empty declaration then writer", `export RIPGREP_CONFIG_PATH=; printf -v RIPGREP_CONFIG_PATH '%s' config.txt; rg .env source.txt`, "", "deny", nil},
		{"read target", `read RIPGREP_CONFIG_PATH; export RIPGREP_CONFIG_PATH; rg .env source.txt`, "", "deny", nil},
		{"bare declaration", `declare -x RIPGREP_CONFIG_PATH; rg .env source.txt`, "", "deny", nil},
		{"readonly name", `readonly RIPGREP_CONFIG_PATH; rg .env source.txt`, "", "deny", nil},
		{"bare config", `RIPGREP_CONFIG_PATH=config.txt; rg .env source.txt`, "", "deny", nil},
		{"tool env", `rg .env source.txt`, "", "deny", map[string]any{"env": map[string]any{"RIPGREP_CONFIG_PATH": "config.txt"}}},
		{"nested tool env", `rg .env source.txt`, "", "deny", map[string]any{"value": map[string]any{"env": map[string]any{"RIPGREP_CONFIG_PATH": "config.txt"}}}},
		{"tool environment", `rg .env source.txt`, "", "deny", map[string]any{"environment": map[string]any{"RIPGREP_CONFIG_PATH": "config.txt"}}},
		{"malformed tool env", `rg .env source.txt`, "", "deny", map[string]any{"env": "opaque"}},
		{"null tool env", `rg .env source.txt`, "", "deny", map[string]any{"env": nil}},
		{"null config entry", `rg .env source.txt`, "", "deny", map[string]any{"env": map[string]any{"RIPGREP_CONFIG_PATH": nil}}},
		{"variant container", `rg .env source.txt`, "", "deny", map[string]any{"ENV": map[string]any{"RIPGREP_CONFIG_PATH": "config.txt"}}},
		{"variant name", `rg .env source.txt`, "", "deny", map[string]any{"env": map[string]any{"ripgrep_config_path": "config.txt"}}},
		{"empty variant container", `rg .env source.txt`, "", "deny", map[string]any{"ENV": map[string]any{}}},
		{"null tool env with effective no config", `rg --no-config 'literal.key' source.txt`, "", "pass", map[string]any{"env": nil}},
		{"variant container with effective no config", `rg --no-config 'literal.key' source.txt`, "", "pass", map[string]any{"ENV": map[string]any{}}},
		{"wrapper", `env RIPGREP_CONFIG_PATH=config.txt rg .env source.txt`, "", "deny", nil},
		{"preceding opaque environment", `source config.sh; rg .env source.txt`, "", "deny", nil},
		{"inline posix", `POSIXLY_CORRECT=1 grep x -e .env`, "", "deny", nil},
		{"host posix", `grep x -e .env`, "", "deny", nil},
		{"normal implicit", `rg 'literal.key' source.txt`, "", "pass", nil},
		{"effective no config", `rg --no-config 'literal.key' source.txt`, "config.txt", "pass", nil},
		{"no config after separator is file", `rg -- .env --no-config`, "config.txt", "deny", nil},
		{"empty tool env", `rg 'literal.key' source.txt`, "", "pass", map[string]any{"env": map[string]any{"RIPGREP_CONFIG_PATH": ""}}},
		{"explicit config arity", `rg -e .env source.txt`, "config.txt", "deny", nil},
		{"explicit pattern no config", `rg --no-config -e 'literal.key' source.txt`, "config.txt", "pass", nil},
		{"grep before operand", `grep -e 'literal.key' source.txt`, "", "pass", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("POSIXLY_CORRECT", "1")
			input := map[string]any{"command": tc.command}
			for key, value := range tc.extra {
				input[key] = value
			}
			kind, message := run(t, buildEventAt(t, "Bash", input, globFixture(t, ".env", "source.txt")), map[string]string{"RIPGREP_CONFIG_PATH": tc.config})
			if kind != tc.want || tc.want == "deny" && !strings.Contains(message, "(dotenv, critical)") {
				t.Fatalf("kind=%s message=%s; want %s", kind, message, tc.want)
			}
		})
	}
}

func TestSearchPatternData(t *testing.T) {
	t.Setenv("RIPGREP_CONFIG_PATH", "")
	root := globFixture(t, "source.txt", ".env")
	for _, command := range []string{
		`rg -n 'literal.key' source.txt`,
		`grep -n 'literal.key' source.txt`,
		`rg -e 'literal.key' source.txt`,
		`rg -eliteral.key source.txt`,
		`grep --regexp=literal.key source.txt`,
		`rg --regexp literal.key -e other.key source.txt`,
		`grep -nEi -- 'literal.key' source.txt`,
		`rg -n -C 2 'literal.key' source.txt`,
		`rg -r replacement --encoding utf-8 'literal.key' source.txt`,
		`rg -E utf-8 'literal.key' source.txt`,
		`grep -r -E 'literal.key' source.txt`,
		`rg '{literal,other}.key' source.txt`,
		`rg x *.txt`,
		`rg --glob='*.txt' 'literal.key' source.txt`,
		`rg --glob '*.txt' 'literal.key' source.txt`,
		`grep --include='*.txt' 'literal.key' source.txt`,
		`grep --include '*.txt' 'literal.key' source.txt`,
	} {
		t.Run(command, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": command}, root), nil)
			if kind != "pass" {
				t.Fatalf("kind=%s message=%s", kind, message)
			}
		})
	}
}

func TestSearchCredentialPaths(t *testing.T) {
	t.Setenv("RIPGREP_CONFIG_PATH", "")
	root := globFixture(t, "source.txt", ".env")
	for _, command := range []string{
		`rg 'literal.key' .env`, `grep -e literal.key .env`,
		`rg -f .env source.txt`, `rg -f.env source.txt`,
		`grep --file=.env source.txt`, `grep --file .env source.txt`,
		`rg x < .env`, `rg x .e[n]v`, `rg -g .env x source.txt`,
		`rg --glob=.env x source.txt`, `grep --include=.env x source.txt`,
		`rg --glob .env x source.txt`, `grep --include .env x source.txt`,
		`rg -e other -- .env`, `grep -nf.env source.txt`,
	} {
		t.Run(command, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": command}, root), nil)
			if kind != "deny" || !strings.Contains(message, "(dotenv, critical)") {
				t.Fatalf("kind=%s message=%s; want credential denial", kind, message)
			}
		})
	}
}

func TestSearchPreprocessorCredentialRead(t *testing.T) {
	root := globFixture(t, ".env", "source.txt")
	for _, tool := range []string{"Bash", "PowerShell"} {
		for _, option := range []string{"--pre=.env", "--pre .env"} {
			t.Run(tool+"/"+option, func(t *testing.T) {
				command := "rg --no-config " + option + " smoke source.txt"
				kind, message := run(t, buildEventAt(t, tool, map[string]any{"command": command}, root), nil)
				if kind != "deny" || !strings.Contains(message, ".env (dotenv, critical)") {
					t.Fatalf("kind=%s message=%s; want preprocessor credential denial", kind, message)
				}
			})
		}
	}
}

func TestSearchAmbiguousArguments(t *testing.T) {
	t.Setenv("RIPGREP_CONFIG_PATH", "")
	root := globFixture(t, "source.txt", ".env")
	for _, command := range []string{
		`rg --unknown 'literal.key' source.txt`,
		`grep --encoding utf-8 'literal.key' source.txt`,
		`rg -r 'literal.key' source.txt`,
		`rg -E 'literal.key' source.txt`,
		`rg -e other -- literal.key`,
		`rg --pre sh 'literal.key' source.txt`,
		`rg "$FLAGS" 'literal.key' source.txt`,
		`rg -e 'literal.key' *.txt`,
	} {
		t.Run(command, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": command}, root), nil)
			if kind != "deny" || !strings.Contains(message, "private_key_file") {
				t.Fatalf("kind=%s message=%s", kind, message)
			}
		})
	}
}

func TestSearchJoinedWildcardFile(t *testing.T) {
	t.Setenv("RIPGREP_CONFIG_PATH", "")
	for _, command := range []string{`rg -f.e[n]v source.txt`, `grep --file='.e[n]v' source.txt`} {
		t.Run(command, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": command}, t.TempDir()), nil)
			if kind != "deny" || !strings.Contains(message, "could not inspect glob") || !strings.Contains(message, "joined search") {
				t.Fatalf("kind=%s message=%s", kind, message)
			}
		})
	}
}

func TestSearchBraceExpansion(t *testing.T) {
	t.Setenv("RIPGREP_CONFIG_PATH", "")
	kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": `rg {pattern,.env} source.txt`}, t.TempDir()), nil)
	if kind != "deny" || !strings.Contains(message, "unsupported search brace expansion") {
		t.Fatalf("kind=%s message=%s", kind, message)
	}
}

func TestSearchPatternStillScansSecretValue(t *testing.T) {
	t.Setenv("RIPGREP_CONFIG_PATH", "")
	command := "rg -e '" + fakeAWSKey + "' source.txt"
	kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": command}, t.TempDir()), nil)
	if kind != "deny" || !strings.Contains(message, "AWS_ACCESS_KEY") || strings.Contains(message, fakeAWSKey) {
		t.Fatal("secret pattern must deny with redacted AWS rule")
	}
}
