package checkrun

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hooks"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/provider"
	"github.com/muehz/agentics/hooks/go/internal/scriptsnap"
	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// isolateGuards keeps the developer's own settings, release record and log
// directories out of a run of the real guards.
func isolateGuards(t *testing.T) {
	t.Helper()
	hosttest.IsolateHome(t)
	hosttest.ScrubGuardEnv(t)
	t.Setenv("COMMAND_GUARD_LOG_DIR", t.TempDir())
	t.Setenv("RELEASE_GUARD_LOG_DIR", t.TempDir())
}

func shellStdin(t *testing.T, id surface.ID, command string) []byte {
	t.Helper()
	return hosttest.Stdin(id, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": command}, Cwd: t.TempDir()})
}

// countingReader records whether anything read it.
type countingReader struct {
	inner io.Reader
	read  atomic.Bool
}

func (r *countingReader) Read(p []byte) (int, error) {
	r.read.Store(true)
	return r.inner.Read(p)
}

func TestAnswerLeavesAnUnknownHookToTheEntryPoint(t *testing.T) {
	var later atomic.Int32
	stages := Stages{
		Lookup: func(string) (hookspec.Handler, bool) { return nil, false },
		Dispatch: func(string, string, []string) (string, *provider.Rendered) {
			later.Add(1)
			return "", nil
		},
		Normalize: func(string, string, []byte) (hookspec.Event, *provider.Rendered) {
			later.Add(1)
			return hookspec.Event{}, nil
		},
		Render: func(string, string, hookspec.Outcome, []byte) provider.Rendered {
			later.Add(1)
			return provider.Rendered{}
		},
	}
	reader := &countingReader{inner: strings.NewReader("{}")}
	rendered, answered := Answer(stages, surface.Codex, "no-such-hook", []string{"--mode", "block"}, reader)
	if answered || rendered.Exit != 0 || rendered.Stdout != nil || rendered.Stderr != nil {
		t.Fatalf("answered=%v rendered=%+v, want an unanswered zero reply", answered, rendered)
	}
	if later.Load() != 0 || reader.read.Load() {
		t.Fatalf("stages after the lookup ran %d times, stdin read=%v; want none", later.Load(), reader.read.Load())
	}
}

func TestProductionWiresTheRegistryAndTheAdapters(t *testing.T) {
	stages := Production()
	if _, ok := stages.Lookup("command-guardian"); !ok {
		t.Fatal("Production's lookup misses command-guardian")
	}
	if _, ok := stages.Lookup("commands"); ok {
		t.Fatal("commands is event-only and must not be a registry key")
	}
	for _, id := range surface.All() {
		got, reject := stages.Dispatch(string(id), "pre-tool", nil)
		want, _ := provider.Dispatch(string(id), "pre-tool", nil)
		if reject != nil || got != want {
			t.Fatalf("%s: Production dispatch = %q %+v, want %q", id, got, reject, want)
		}
		if _, malformed := stages.Normalize(string(id), want, []byte("{")); malformed == nil {
			t.Fatalf("%s: Production normalize accepted a truncated payload", id)
		}
		rendered := stages.Render(string(id), want, contract.Deny("no"), nil)
		if hosttest.Read(id, rendered.Exit, rendered.Stdout, rendered.Stderr).Decision != hosttest.Deny {
			t.Fatalf("%s: Production render of a deny is not a deny: %+v", id, rendered)
		}
	}
}

func TestAnswerDeniesADestructiveCommandOnBothPlans(t *testing.T) {
	isolateGuards(t)
	for _, id := range surface.All() {
		for _, verb := range []string{"pre-tool", "command-guardian"} {
			t.Run(string(id)+"/"+verb, func(t *testing.T) {
				var args []string
				if verb != "pre-tool" {
					args = []string{"--mode", "block"}
				}
				rendered, answered := Answer(Production(), id, verb, args, bytes.NewReader(shellStdin(t, id, "rm -rf /")))
				if !answered {
					t.Fatal("unanswered")
				}
				verdict := hosttest.Read(id, rendered.Exit, rendered.Stdout, rendered.Stderr)
				if verdict.Decision != hosttest.Deny || !strings.Contains(verdict.Reason, "destructive_file_ops") {
					t.Fatalf("host reads %v %q, want the destructive-command deny", verdict.Decision, verdict.Reason)
				}
			})
		}
	}
}

// TestRequestsCarryEachPlansFields pins what a handler is given: the legacy
// plan's request has no context, and each event check gets its own name, a
// context and the one script snapshot every check shares.
func TestRequestsCarryEachPlansFields(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	var legacy hookspec.Request
	stages := Production()
	stages.Lookup = func(string) (hookspec.Handler, bool) {
		return func(req hookspec.Request) hookspec.Outcome { legacy = req; return contract.Pass() }, true
	}
	if _, answered := Answer(stages, surface.Codex, "command-guardian", []string{"--mode", "block"}, bytes.NewReader(shellStdin(t, surface.Codex, "echo safe"))); !answered {
		t.Fatal("unanswered")
	}
	if legacy.Context != nil || legacy.Hook != "command-guardian" || legacy.Provider != "codex" || legacy.Scripts == nil || legacy.Diag == nil || !legacy.Event.Present() {
		t.Fatalf("legacy request = %+v", legacy)
	}

	requests := make(chan hookspec.Request, 2)
	stages.Lookup = func(string) (hookspec.Handler, bool) {
		return func(req hookspec.Request) hookspec.Outcome { requests <- req; return contract.Pass() }, true
	}
	if _, answered := Answer(stages, surface.Codex, "pre-tool", nil, bytes.NewReader(shellStdin(t, surface.Codex, "echo safe"))); !answered {
		t.Fatal("unanswered")
	}
	first, second := <-requests, <-requests
	if first.Hook == second.Hook || first.Context == nil || second.Context == nil || first.Scripts == nil || first.Scripts != second.Scripts {
		t.Fatalf("event requests = %+v and %+v", first, second)
	}
	if first.Diag == second.Diag {
		t.Fatal("two checks share one diagnostic buffer")
	}
}

// settingsStages returns production stages whose lookup records each request
// a handler is given and passes it.
func settingsStages(requests chan<- hookspec.Request) Stages {
	stages := Production()
	stages.Lookup = func(string) (hookspec.Handler, bool) {
		return func(req hookspec.Request) hookspec.Outcome { requests <- req; return contract.Pass() }, true
	}
	return stages
}

func TestTheHandlerReceivesASettingsSource(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	requests := make(chan hookspec.Request, 1)
	stdin := bytes.NewReader(shellStdin(t, surface.Codex, "echo safe"))
	if _, answered := Answer(settingsStages(requests), surface.Codex, "command-guardian", []string{"--mode", "block"}, stdin); !answered {
		t.Fatal("unanswered")
	}
	if got := <-requests; got.Settings == nil {
		t.Fatal("the single check's request carries no settings source")
	}
}

func TestEventChecksShareOneSettingsSource(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	requests := make(chan hookspec.Request, 2)
	stdin := bytes.NewReader(shellStdin(t, surface.Codex, "echo safe"))
	if _, answered := Answer(settingsStages(requests), surface.Codex, "pre-tool", nil, stdin); !answered {
		t.Fatal("unanswered")
	}
	first, second := <-requests, <-requests
	if first.Hook == second.Hook {
		t.Fatalf("both requests came from the check %q, want two checks", first.Hook)
	}
	if first.Settings == nil || first.Settings != second.Settings {
		t.Fatalf("settings sources = %p and %p, want one shared non-nil source", first.Settings, second.Settings)
	}
}

// TestEachInvocationBuildsItsOwnSettingsSource fails if the source moves to a
// package variable: a repaired settings file must take effect on the next
// invocation.
func TestEachInvocationBuildsItsOwnSettingsSource(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	for _, verb := range []string{"command-guardian", "pre-tool"} {
		t.Run(verb, func(t *testing.T) {
			var args []string
			want := 2
			if verb != "pre-tool" {
				args = []string{"--mode", "block"}
				want = 1
			}
			sources := make([]*settings.Source, 0, 2)
			for range 2 {
				requests := make(chan hookspec.Request, 2)
				stdin := bytes.NewReader(shellStdin(t, surface.Codex, "echo safe"))
				if _, answered := Answer(settingsStages(requests), surface.Codex, verb, args, stdin); !answered {
					t.Fatal("unanswered")
				}
				if len(requests) != want {
					t.Fatalf("%d requests, want %d", len(requests), want)
				}
				sources = append(sources, (<-requests).Settings)
			}
			if sources[0] == nil || sources[0] == sources[1] {
				t.Fatalf("sources = %p and %p, want two distinct non-nil sources", sources[0], sources[1])
			}
		})
	}
}

func TestReadPayloadKeepsTheSixtyFourMebibyteBound(t *testing.T) {
	payload, err := readPayload(io.LimitReader(zeroReader{}, maxPayloadBytes))
	if err != nil || int64(len(payload)) != maxPayloadBytes {
		t.Fatalf("an exact-size payload: %d bytes, %v", len(payload), err)
	}
	if _, err := readPayload(io.LimitReader(zeroReader{}, maxPayloadBytes+1)); err == nil || err.Error() != "hook payload exceeds 64 MiB limit" {
		t.Fatalf("an oversize payload: %v", err)
	}
	failure := errors.New("private input error")
	if _, err := readPayload(failingReader{failure}); !errors.Is(err, failure) {
		t.Fatalf("a failing reader: %v", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestReadEventBoundsReadingAndNormalizing(t *testing.T) {
	for _, mode := range []string{"read", "normalize"} {
		t.Run(mode, func(t *testing.T) {
			stages := Production()
			blocked := newPostureBlockingReader(t)
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			var reader io.Reader = blocked
			if mode == "normalize" {
				reader = strings.NewReader(`{}`)
				stages.Normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) {
					<-release
					return hookspec.Event{}, nil
				}
			}
			pl := eventPlan
			pl.inputBudget = 20 * time.Millisecond
			done := make(chan fault, 1)
			go func() {
				event, got := readEvent(stages, pl, "claude-code", "PreToolUse", reader)
				if event.Present() {
					got = noFault
				}
				done <- got
			}()
			select {
			case got := <-done:
				if got != faultInputDeadline {
					t.Fatalf("fault = %v, want the input deadline", got)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("reading the event outran its budget")
			}
			if mode == "read" {
				select {
				case <-blocked.closed:
				default:
					t.Fatal("the deadline did not close the invocation-owned reader")
				}
			}
		})
	}
}

func TestRunChecksIsolatesEachWorker(t *testing.T) {
	started := make(chan string, 5)
	releaseLate, lateDone, partialCanceled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		close(releaseLate)
		select {
		case <-lateDone:
		case <-time.After(time.Second):
			t.Error("late worker failed to finish after fixture release")
		}
	})
	handlers := map[string]hookspec.Handler{
		"deny": func(req hookspec.Request) hookspec.Outcome {
			started <- req.Hook
			return contract.Deny("first denial")
		},
		"panic": func(req hookspec.Request) hookspec.Outcome {
			started <- req.Hook
			panic("private panic value")
		},
		"late": func(req hookspec.Request) hookspec.Outcome {
			started <- req.Hook
			<-releaseLate
			_, _ = io.WriteString(req.Diag, "late private diagnostic")
			close(lateDone)
			return contract.Context("late private result")
		},
		"partial": func(req hookspec.Request) hookspec.Outcome {
			started <- req.Hook
			<-req.ExecutionContext().Done()
			close(partialCanceled)
			_, _ = io.WriteString(req.Diag, "completed cleanup diagnostic\n")
			return contract.Context("retained partial result; check incomplete")
		},
		"healthy": func(req hookspec.Request) hookspec.Outcome {
			started <- req.Hook
			<-partialCanceled
			if req.ExecutionContext().Err() != nil {
				return contract.Error("short sibling canceled healthy worker")
			}
			return contract.Context("healthy reminder")
		},
	}
	stages := Production()
	stages.Lookup = postureLookupByName(handlers)
	p := productionPosture()
	p.event.grace = 100 * time.Millisecond
	p.checkBudgets = map[string]time.Duration{
		"deny": time.Second, "panic": time.Second, "late": 20 * time.Millisecond, "partial": 20 * time.Millisecond, "healthy": time.Second,
	}
	checks := []hooks.EventCheck{
		{Name: "deny", Guard: true}, {Name: "panic", Guard: true}, {Name: "late", Guard: true},
		{Name: "partial", Guard: true}, {Name: "healthy", Guard: true},
	}
	start := time.Now()
	parent, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan []checkResult, 1)
	go func() {
		done <- runChecks(parent, stages, p, hookspec.Request{Provider: "codex", Scripts: scriptsnap.New()}, checks)
	}()
	var results []checkResult
	select {
	case results = <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("workers exceeded independent deadlines after %v", time.Since(start))
	}
	if len(started) != 5 || results[1].fault != "panic" || !results[2].cleanupPending {
		t.Fatalf("workers not isolated: starts=%d results=%+v", len(started), results)
	}
	if results[3].outcome.Kind() != contract.KindContext || results[3].cleanupPending || !strings.Contains(results[3].fault, "deadline exceeded") {
		t.Fatalf("partial completion lost during grace: %+v", results[3])
	}
	rendered := foldResults(stages, p.event, "codex", "PreToolUse", results)
	verdict := hosttest.Read(surface.Codex, rendered.Exit, rendered.Stdout, rendered.Stderr)
	if verdict.Decision != hosttest.Deny {
		t.Fatalf("host reads %v, want Deny: %+v", verdict.Decision, rendered)
	}
	for _, want := range []string{"first denial", "retained partial result", "healthy reminder", "cleanup unacknowledged"} {
		if !strings.Contains(verdict.Reason, want) {
			t.Fatalf("missing %q after sibling failure: %q", want, verdict.Reason)
		}
	}
	combined := string(rendered.Stdout) + string(rendered.Stderr)
	if strings.Contains(combined, "private") || !strings.Contains(combined, "completed cleanup diagnostic\n") {
		t.Fatalf("unsafe or incomplete diagnostics: %q", combined)
	}
	if strings.Index(verdict.Reason, "[deny]") > strings.Index(verdict.Reason, "[partial]") || strings.Index(verdict.Reason, "[partial]") > strings.Index(verdict.Reason, "[healthy]") {
		t.Fatalf("completion timing changed stable check order: %q", verdict.Reason)
	}
}

func TestRunCheckAnnotatesACanceledParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	stages := Production()
	stages.Lookup = postureStubLookup(posturePass)
	p := productionPosture()
	for range 32 {
		result := runCheck(parent, stages, p, hookspec.Request{Provider: "codex"}, hooks.EventCheck{Name: "guard", Guard: true})
		if result.fault != "deadline exceeded; incomplete check" || result.outcome.Kind() != contract.KindPass {
			t.Fatalf("expired context produced unannotated/lost result: %+v", result)
		}
		rendered := foldResults(stages, p.event, "codex", "PreToolUse", []checkResult{result})
		if hosttest.Read(surface.Codex, rendered.Exit, rendered.Stdout, rendered.Stderr).Decision != hosttest.Deny {
			t.Fatalf("an incomplete guard does not deny: %+v", rendered)
		}
	}
}

func postureStubLookup(handler hookspec.Handler) func(string) (hookspec.Handler, bool) {
	return func(string) (hookspec.Handler, bool) { return handler, true }
}

func TestResolveFindsTheCheckOrEveryStep(t *testing.T) {
	steps := []hooks.Step{{Label: "first rules", Hook: "first"}, {Label: "second rules", Hook: "second"}}
	var looked []string
	lookup := func(handlers map[string]hookspec.Handler) func(string) (hookspec.Handler, bool) {
		return func(name string) (hookspec.Handler, bool) {
			looked = append(looked, name)
			handler, ok := handlers[name]
			return handler, ok
		}
	}
	for _, tc := range []struct {
		name     string
		handlers map[string]hookspec.Handler
		steps    []hooks.Step
		ok       bool
		steps2   int
		looked   string
	}{
		{"own name wins", map[string]hookspec.Handler{"check": posturePass, "first": posturePass, "second": posturePass}, steps, true, 0, "check"},
		{"own name nil", map[string]hookspec.Handler{"check": nil, "first": posturePass, "second": posturePass}, steps, false, 0, "check"},
		{"every step", map[string]hookspec.Handler{"first": posturePass, "second": posturePass}, steps, true, 2, "check first second"},
		{"no steps", map[string]hookspec.Handler{"first": posturePass}, nil, false, 0, "check"},
		{"missing first step", map[string]hookspec.Handler{"second": posturePass}, steps, false, 0, "check first"},
		{"nil second step", map[string]hookspec.Handler{"first": posturePass, "second": nil}, steps, false, 0, "check first second"},
		{"empty step hook", map[string]hookspec.Handler{"first": posturePass}, []hooks.Step{{Label: "empty", Hook: ""}}, false, 0, "check "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			looked = nil
			run, ok := resolve(Stages{Lookup: lookup(tc.handlers)}, hooks.EventCheck{Name: "check", Steps: tc.steps})
			if ok != tc.ok || len(run.steps) != tc.steps2 || strings.Join(looked, " ") != tc.looked {
				t.Fatalf("ok=%v steps=%d looked=%q, want ok=%v steps=%d looked=%q", ok, len(run.steps), strings.Join(looked, " "), tc.ok, tc.steps2, tc.looked)
			}
		})
	}
}

// TestUnresolvedStepRunsNoStep proves a check whose later step is missing is
// unavailable before its first step runs.
func TestUnresolvedStepRunsNoStep(t *testing.T) {
	var ran atomic.Int32
	stages := Production()
	stages.Lookup = postureLookupByName(map[string]hookspec.Handler{
		"first": func(hookspec.Request) hookspec.Outcome { ran.Add(1); return contract.Pass() },
	})
	check := hooks.EventCheck{Name: "check", Guard: true, Steps: []hooks.Step{{Label: "first rules", Hook: "first"}, {Label: "second rules", Hook: "second"}}}
	result := runCheck(context.Background(), stages, productionPosture(), hookspec.Request{Provider: "codex"}, check)
	if ran.Load() != 0 || result.fault != "missing handler" || result.outcome.Kind() != contract.KindError || result.outcome.Payload() != "registered check is unavailable" {
		t.Fatalf("ran=%d result=%+v", ran.Load(), result)
	}
}

// TestStepsShareOneRequestAndRunAfterTheDeadline proves the steps of one
// check see the check's name, one context and one buffer, and that a later
// step still answers once the budget has ended.
func TestStepsShareOneRequestAndRunAfterTheDeadline(t *testing.T) {
	var seen []hookspec.Request
	stages := Production()
	stages.Lookup = postureLookupByName(map[string]hookspec.Handler{
		"first": func(req hookspec.Request) hookspec.Outcome {
			seen = append(seen, req)
			<-req.ExecutionContext().Done()
			_, _ = io.WriteString(req.Diag, "first chatter\n")
			return contract.Context("first note")
		},
		"second": func(req hookspec.Request) hookspec.Outcome {
			seen = append(seen, req)
			_, _ = io.WriteString(req.Diag, "second chatter\n")
			return contract.Deny("second denial")
		},
	})
	p := productionPosture()
	p.event.checkBudget = 20 * time.Millisecond
	check := hooks.EventCheck{Name: "check", Guard: true, Steps: []hooks.Step{{Label: "first rules", Hook: "first"}, {Label: "second rules", Hook: "second"}}}
	result := runCheck(context.Background(), stages, p, hookspec.Request{Provider: "codex"}, check)
	if result.outcome.Kind() != contract.KindDeny || result.outcome.Payload() != "first note\n\nsecond denial" || result.fault != "deadline exceeded; incomplete check" {
		t.Fatalf("result = %+v", result)
	}
	if string(result.diagnostic) != "first chatter\nsecond chatter\n" {
		t.Fatalf("diagnostic = %q", result.diagnostic)
	}
	if len(seen) != 2 || seen[0].Hook != "check" || seen[1].Hook != "check" || seen[0].Context != seen[1].Context || seen[0].Diag != seen[1].Diag {
		t.Fatalf("the steps did not share one request: %+v", seen)
	}
}

func TestFoldResultsTreatsAnInvalidResultAsAFault(t *testing.T) {
	rendered := foldResults(Production(), eventPlan, "codex", "PreToolUse", []checkResult{{check: hooks.EventCheck{Name: "check", Guard: true}}})
	verdict := hosttest.Read(surface.Codex, rendered.Exit, rendered.Stdout, rendered.Stderr)
	if verdict.Decision != hosttest.Deny || verdict.Reason != "[check] core produced an invalid neutral outcome\n\n[check] invalid outcome" {
		t.Fatalf("host reads %v %q", verdict.Decision, verdict.Reason)
	}
}

func TestFoldResultsKeepsANonGuardErrorOffTheReply(t *testing.T) {
	rendered := foldResults(Production(), eventPlan, "codex", "PreToolUse", []checkResult{{
		check: hooks.EventCheck{Name: "advice"}, outcome: contract.Error("advice failed"), diagnostic: []byte("advice chatter"),
	}})
	verdict := hosttest.Read(surface.Codex, rendered.Exit, rendered.Stdout, rendered.Stderr)
	if verdict.Decision == hosttest.Deny || verdict.Reason != "" {
		t.Fatalf("a non-guard error reads as %v %q", verdict.Decision, verdict.Reason)
	}
	if string(rendered.Stdout) != "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"additionalContext\":\"[advice] check failed\"}}\n" {
		t.Fatalf("stdout = %q", rendered.Stdout)
	}
	if string(rendered.Stderr) != "[advice] advice failed\n\n[advice] check failed\n\n[advice] advice chatter\n" {
		t.Fatalf("stderr = %q", rendered.Stderr)
	}
}

func TestCellOutcomeShowsOnlyWhatItDeclares(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    cell
		want hookspec.Outcome
	}{
		{"fixed", cell{kind: contract.KindDeny, message: "fixed"}, contract.Deny("fixed")},
		{"subject", cell{kind: contract.KindError, message: "%[1]s panicked", formatted: true}, contract.Error("rules panicked")},
		{"value", cell{kind: contract.KindError, message: "in %[1]s: %[2]v\n%[3]s", formatted: true, showsValue: true}, contract.Error("in rules: boom\nstack")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.outcome("rules", "boom", []byte("stack")); got != tc.want {
				t.Fatalf("outcome = %s %q, want %s %q", got.Kind(), got.Payload(), tc.want.Kind(), tc.want.Payload())
			}
		})
	}
}

func TestBudgetForPrefersTheOverride(t *testing.T) {
	p := productionPosture()
	if got := p.budgetFor("commands"); got != 10*time.Second {
		t.Fatalf("production budget = %v", got)
	}
	p.checkBudgets = map[string]time.Duration{"commands": time.Millisecond}
	if p.budgetFor("commands") != time.Millisecond || p.budgetFor("secrets-guardian") != 10*time.Second {
		t.Fatalf("override = %v, other = %v", p.budgetFor("commands"), p.budgetFor("secrets-guardian"))
	}
}
