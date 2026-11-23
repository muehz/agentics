package checkrun

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/commandguard"
	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hooks"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/settings/settingstest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func commandEvent(t *testing.T, tool, command string) hookspec.Event {
	t.Helper()
	// A real cwd keeps a developer's own .agentics/hooks.json in the checkout out
	// of the run; the release rules read their settings relative to it.
	cwd := t.TempDir()
	event, err := contract.NewEvent(contract.EventParts{
		Surface:   surface.ClaudeCode,
		Cwd:       &cwd,
		ToolName:  tool,
		ToolInput: map[string]any{"command": command},
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

// commandsCheck is the commands check as the selection hands it out, steps
// and all.
func commandsCheck(t *testing.T) hooks.EventCheck {
	t.Helper()
	cwd := t.TempDir()
	event, err := contract.NewEvent(contract.EventParts{Surface: surface.Codex, Cwd: &cwd, ToolName: "Bash", ToolInput: map[string]any{"command": "true"}})
	if err != nil {
		t.Fatal(err)
	}
	checks, err := hooks.SelectEventChecks("codex", event)
	if err != nil || len(checks) == 0 || checks[0].Name != "commands" || len(checks[0].Steps) != 2 {
		t.Fatalf("selection = %+v, %v", checks, err)
	}
	return checks[0]
}

// commandsHandler runs the commands check as the event plan does: its two
// steps, resolved through the production lookup and folded by the step
// runner. The check's own diagnostic text goes to the request's buffer, as a
// handler's would, and a fault the runner adds fails the test.
func commandsHandler(t *testing.T) hookspec.Handler {
	t.Helper()
	check := commandsCheck(t)
	run, ok := resolve(Production(), check)
	if !ok || len(run.steps) != 2 {
		t.Fatalf("the commands check did not resolve to its two steps: %+v", run)
	}
	return func(req hookspec.Request) hookspec.Outcome {
		result := invokeCheck(eventPlan, run, check, req)
		if result.fault != "" {
			t.Errorf("the commands check ended with the fault %q", result.fault)
		}
		if req.Diag != nil {
			_, _ = req.Diag.Write(result.diagnostic)
		}
		return result.outcome
	}
}

func runCommands(t *testing.T, tool, command string) hookspec.Outcome {
	t.Helper()
	return commandsHandler(t)(hookspec.Request{
		Provider: "claude-code", Hook: "commands",
		Event: commandEvent(t, tool, command),
	})
}

func TestCommandsDeniesACommandRuleAndADeployRule(t *testing.T) {
	isolateGuards(t)
	if out := runCommands(t, "Bash", "rm -rf /"); out.Kind() != contract.KindDeny {
		t.Fatalf("rm -rf / = %s %q, want deny", out.Kind(), out.Payload())
	}
	out := runCommands(t, "Bash", "kubectl apply -f x.yaml")
	if out.Kind() != contract.KindDeny || !strings.Contains(out.Payload(), "production-shaped deploy command") {
		t.Fatalf("kubectl apply = %s %q, want the deploy denial", out.Kind(), out.Payload())
	}
	if out := runCommands(t, "Bash", "git status"); out.Kind() != contract.KindPass {
		t.Fatalf("git status = %s %q, want pass", out.Kind(), out.Payload())
	}
}

func TestCommandsJoinsBothMessagesCommandRulesFirst(t *testing.T) {
	isolateGuards(t)
	out := runCommands(t, "Bash", "rm -rf / && kubectl apply -f x.yaml")
	if out.Kind() != contract.KindDeny {
		t.Fatalf("kind = %s, want deny", out.Kind())
	}
	command := strings.Index(out.Payload(), "blocking threat")
	deploy := strings.Index(out.Payload(), "production-shaped deploy command")
	if command < 0 || deploy < 0 || command > deploy {
		t.Fatalf("payload %q must name the command rule before the deploy rule", out.Payload())
	}
}

// Both rule sets of the commands check run in one goroutine on one request, so
// the request's source gives them one reading of the settings. The stub turns
// the deploy record off, so a release rule that read the files itself would find
// the defaults and deny the droppable deployment.
func TestCommandsShareOneSettingsSnapshot(t *testing.T) {
	isolateGuards(t)
	stub := settingstest.Policy(settings.RequireDeployRecord)
	out := commandsHandler(t)(hookspec.Request{
		Provider: "claude-code", Hook: "commands",
		Event:    commandEvent(t, "Bash", "gh pr create --title t --body b && kubectl apply -f x.yaml"),
		Settings: stub.Source(),
	})
	if calls := stub.Calls(); calls != 1 {
		t.Fatalf("the settings resolver ran %d times, want 1", calls)
	}
	if out.Kind() != contract.KindDeny || !strings.Contains(out.Payload(), "blocking threat") {
		t.Fatalf("outcome = %s %q, want the command rules' deny", out.Kind(), out.Payload())
	}
	for _, unwanted := range []string{"release-guardian", "production-shaped deploy command"} {
		if strings.Contains(out.Payload(), unwanted) {
			t.Fatalf("the release rules judged other settings than the stub's: %q", out.Payload())
		}
	}
}

func TestCommandsRuleSetsStayIndependent(t *testing.T) {
	// Each switch is set alone: the skip switch returns before the allowlist
	// and the mode are read, so setting them together would never exercise the
	// other two.
	for _, tc := range []struct{ name, key, value string }{
		{"skip", "SKIP_COMMAND_GUARD", "true"},
		{"allowlist", "COMMAND_GUARD_ALLOWLIST", "kubectl"},
		{"warn mode", "COMMAND_GUARD_MODE", "warn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateGuards(t)
			t.Setenv(tc.key, tc.value)
			if out := runCommands(t, "Bash", "kubectl apply -f x.yaml"); out.Kind() != contract.KindDeny {
				t.Fatalf("the deploy gate must ignore the command rules' %s: %s %q", tc.name, out.Kind(), out.Payload())
			}
			if tc.name == "skip" {
				if out := runCommands(t, "Bash", "rm -rf /"); out.Kind() != contract.KindPass {
					t.Fatalf("skip must lift the command rules: %s %q", out.Kind(), out.Payload())
				}
			}
		})
	}
}

func TestCommandsPassesNonShellTools(t *testing.T) {
	isolateGuards(t)
	if out := runCommands(t, "Read", "kubectl apply -f x.yaml"); out.Kind() != contract.KindPass {
		t.Fatalf("Read = %s %q, want pass", out.Kind(), out.Payload())
	}
}

// checkpointContext counts every Err call, which is how the guards poll for
// cancellation, and cancels itself at the cancelAt-th call.
type checkpointContext struct {
	context.Context
	checks, cancelAt int
	cancel           context.CancelFunc
}

func (c *checkpointContext) Err() error {
	c.checks++
	if c.cancel != nil && c.checks == c.cancelAt {
		c.cancel()
	}
	return c.Context.Err()
}

// cancelRun runs handler with a request context that cancels at its
// cancelAt-th cancellation check, or never when cancelAt is zero. It returns
// the outcome, the probe, whether the context ended up canceled and the
// diagnostic table the guard wrote.
func cancelRun(t *testing.T, handler hookspec.Handler, tool, command string, cancelAt int) (hookspec.Outcome, *checkpointContext, bool, string) {
	t.Helper()
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	probe := &checkpointContext{Context: base}
	if cancelAt > 0 {
		probe.cancelAt, probe.cancel = cancelAt, cancel
	}
	var diag bytes.Buffer
	outcome := handler(hookspec.Request{
		Provider: "claude-code", Hook: "commands",
		Event: commandEvent(t, tool, command), Context: probe, Diag: &diag,
	})
	return outcome, probe, base.Err() != nil, diag.String()
}

// TestCommandsCancellationReachesEachRuleSet proves that a context canceled
// partway through either rule set still denies with that rule set's own
// reason. The steps share one context, so the release-rule checkpoint is
// placed after the checks the command rules use.
func TestCommandsCancellationReachesEachRuleSet(t *testing.T) {
	isolateGuards(t)
	for _, name := range []string{"POSIXLY_CORRECT", "CDPATH", "BASHOPTS", "GLOBIGNORE"} {
		t.Setenv(name, "")
	}
	const padding = 6 * 64 * 1024
	data := "@'\ndon't; gh stack submit; cat .env; kubectl apply -f prod.yaml; 'literal\n" + strings.Repeat("x", padding) + "\n'@ | Set-Content -LiteralPath draft.txt" + strings.Repeat(" ", padding)
	for _, tc := range []struct {
		name, tool, command, stage string
		commandReason              string
		releaseReason              string
	}{
		{"bash-inspection", "Bash", "bash" + strings.Repeat(" ", padding) + "--help", "parse", "command_parse_limit", "canceled"},
		{"bash-consumer-publication", "Bash", "bash --help", "consumer", "", "Bash inspection authority unavailable"},
		{"powershell-environment", "PowerShell", "[Environment]::SetEnvironmentVariable('EXAMPLE','" + strings.Repeat("x", padding) + "')", "parse", "command_parse_limit", "canceled"},
		{"powershell-data-body", "PowerShell", data, "body", "unsafe_publication", "production-shaped deploy command"},
		{"powershell-data-suffix", "PowerShell", data, "suffix", "unsafe_publication", "production-shaped deploy command"},
		{"powershell-data-publication", "PowerShell", data, "publication", "unsafe_publication", "production-shaped deploy command"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The prefix is measured with the same parser the handlers use;
			// cancellation then occurs after entry into the next bounded scan.
			prefix := &checkpointContext{Context: context.Background()}
			dialect := shellwords.PowerShell
			if tc.tool == "Bash" {
				dialect = shellwords.POSIX
			}
			if tc.stage != "parse" {
				shellwords.ParseContext(prefix, tc.command, dialect)
			}
			if tc.stage == "suffix" {
				shellwords.PowerShellLiteralData(prefix, tc.command)
			}
			cancelAt := prefix.checks + 4
			switch tc.stage {
			case "consumer":
				cancelAt = prefix.checks + 1
			case "publication":
				execution := shellwords.ExecutionContext{Dialect: dialect, Proven: true}
				view, proven, err := shellwords.PowerShellFileData(prefix, tc.command, execution)
				if err != nil || !proven {
					t.Fatalf("publication fixture proven=%v err=%v", proven, err)
				}
				shellwords.ParseContext(prefix, view, execution.Dialect)
				cancelAt = prefix.checks + 1
			}

			commands := commandsHandler(t)
			if tc.commandReason != "" {
				outcome, probe, canceled, diag := cancelRun(t, commands, tc.tool, tc.command, cancelAt)
				if !canceled || probe.checks < cancelAt {
					t.Fatalf("command rules did not reach the checkpoint: checks=%d want=%d canceled=%v", probe.checks, cancelAt, canceled)
				}
				// The cancellation adds a fixed parse-limit threat, and a fixed
				// threat names the reason alone whenever one is present. The
				// publication threat is governed by a settings key, so the reason
				// omits it and only the diagnostic table, which lists every
				// threat, carries the original executable evidence.
				if outcome.Kind() != contract.KindDeny || !strings.Contains(outcome.Payload()+diag, tc.commandReason) {
					t.Fatalf("command rules lost %q: %s %q\n%s", tc.commandReason, outcome.Kind(), outcome.Payload(), diag)
				}
				if !strings.Contains(outcome.Payload(), "release-guardian") {
					t.Fatalf("the release rules did not run after the command rules were canceled: %q", outcome.Payload())
				}
			}

			// Measure the checks the command rules use on their own, then
			// cancel the composite that many checks later.
			_, commandOnly, _, _ := cancelRun(t, commandguard.Run, tc.tool, tc.command, 0)
			releaseAt := commandOnly.checks + cancelAt
			outcome, probe, canceled, _ := cancelRun(t, commands, tc.tool, tc.command, releaseAt)
			if !canceled || probe.checks < releaseAt {
				t.Fatalf("release rules did not reach the checkpoint: checks=%d want=%d canceled=%v", probe.checks, releaseAt, canceled)
			}
			if outcome.Kind() != contract.KindDeny || !strings.Contains(outcome.Payload(), tc.releaseReason) {
				t.Fatalf("release rules lost %q: %s %q", tc.releaseReason, outcome.Kind(), outcome.Payload())
			}
			if tc.stage == "parse" || tc.stage == "consumer" {
				if strings.Contains(outcome.Payload(), "command_parse_limit") {
					t.Fatalf("the cancellation landed in the command rules: %q", outcome.Payload())
				}
			}
		})
	}
}
