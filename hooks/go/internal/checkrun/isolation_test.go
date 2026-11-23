package checkrun

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hooks"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/provider"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// requireRenderedDeny fails the test unless the surface's host reads the
// rendered reply as a deny, and returns the reason the host shows.
func requireRenderedDeny(t testing.TB, id surface.ID, rendered provider.Rendered) string {
	t.Helper()
	verdict := hosttest.Read(id, rendered.Exit, rendered.Stdout, rendered.Stderr)
	if verdict.Decision != hosttest.Deny {
		t.Fatalf("%s host reads %v, want Deny: exit=%d stdout=%q stderr=%q", id, verdict.Decision, rendered.Exit, rendered.Stdout, rendered.Stderr)
	}
	return verdict.Reason
}

// eventRequest is the base request the event plan hands every check.
func eventRequest(provider string, event hookspec.Event) hookspec.Request {
	return hookspec.Request{Provider: provider, Event: event, Scripts: hooks.NewScriptSnapshot()}
}

// budgetedPosture is the production posture with the given per-check budgets
// and cleanup grace.
func budgetedPosture(budgets map[string]time.Duration, grace time.Duration) posture {
	p := productionPosture()
	p.checkBudgets = budgets
	p.event.grace = grace
	return p
}

func TestEventIsolationIndependentWorkers(t *testing.T) {
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
	stages.Lookup = func(name string) (hookspec.Handler, bool) { handler, ok := handlers[name]; return handler, ok }
	checks := []hooks.EventCheck{
		{Name: "deny", Guard: true},
		{Name: "panic", Guard: true},
		{Name: "late", Guard: true},
		{Name: "partial", Guard: true},
		{Name: "healthy", Guard: true},
	}
	p := budgetedPosture(map[string]time.Duration{
		"deny": time.Second, "panic": time.Second, "late": 20 * time.Millisecond, "partial": 20 * time.Millisecond, "healthy": time.Second,
	}, 100*time.Millisecond)
	start := time.Now()
	parent, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan []checkResult, 1)
	go func() {
		done <- runChecks(parent, stages, p, eventRequest("codex", hookspec.Event{}), checks)
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
	reason := requireRenderedDeny(t, surface.Codex, rendered)
	for _, want := range []string{"first denial", "retained partial result", "healthy reminder", "cleanup unacknowledged"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("missing %q after sibling failure: %q", want, reason)
		}
	}
	combined := string(rendered.Stdout) + string(rendered.Stderr)
	if strings.Contains(combined, "private") || !strings.Contains(combined, "completed cleanup diagnostic\n") {
		t.Fatalf("unsafe or incomplete diagnostics: %q", combined)
	}
	if strings.Index(reason, "[deny]") > strings.Index(reason, "[partial]") || strings.Index(reason, "[partial]") > strings.Index(reason, "[healthy]") {
		t.Fatalf("completion timing changed stable check order: %q", reason)
	}
}

func TestEventIsolationPrivateDiagnostics(t *testing.T) {
	stages := Production()
	stages.Lookup = func(name string) (hookspec.Handler, bool) {
		return func(req hookspec.Request) hookspec.Outcome {
			_, _ = io.WriteString(req.Diag, name+" diagnostic\n")
			return contract.Context(name + " context")
		}, true
	}
	p := budgetedPosture(map[string]time.Duration{"first": time.Second, "second": time.Second}, time.Second)
	results := runChecks(context.Background(), stages, p, eventRequest("codex", hookspec.Event{}), []hooks.EventCheck{
		{Name: "first"},
		{Name: "second"},
	})
	for index, name := range []string{"first", "second"} {
		if string(results[index].diagnostic) != name+" diagnostic\n" {
			t.Fatalf("diagnostic buffer was shared: %+v", results)
		}
	}
}

func TestEventIsolationCanceledParent(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	stages := Production()
	stages.Lookup = func(string) (hookspec.Handler, bool) {
		return func(hookspec.Request) hookspec.Outcome { return contract.Pass() }, true
	}
	p := budgetedPosture(map[string]time.Duration{"guard": time.Second}, time.Second)
	for range 32 {
		result := runCheck(parent, stages, p, eventRequest("codex", hookspec.Event{}), hooks.EventCheck{Name: "guard", Guard: true})
		if !strings.Contains(result.fault, "deadline exceeded") || result.outcome.Kind() != contract.KindPass {
			t.Fatalf("expired context produced unannotated/lost result: %+v", result)
		}
		rendered := foldResults(stages, p.event, "codex", "PreToolUse", []checkResult{result})
		requireRenderedDeny(t, surface.Codex, rendered)
	}
}

// TestLookupMissRunsTheSteps pins how a check finds what it runs: the
// production lookup misses the event-only name, so the check's steps run;
// a lookup stubbed under the check's own name intercepts the whole check;
// and a check with neither is unavailable.
func TestLookupMissRunsTheSteps(t *testing.T) {
	cwd := t.TempDir()
	event, err := contract.NewEvent(contract.EventParts{
		Surface:   surface.ClaudeCode,
		Cwd:       &cwd,
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "rm -rf /"},
	})
	if err != nil {
		t.Fatal(err)
	}
	checks, err := hooks.SelectEventChecks("claude-code", event)
	if err != nil || len(checks) == 0 || checks[0].Name != "commands" {
		t.Fatalf("selection = %+v, %v", checks, err)
	}
	if _, registered := hooks.Lookup("commands"); registered {
		t.Fatal("commands is event-only and must not be a registry key")
	}
	p := budgetedPosture(map[string]time.Duration{"commands": time.Second}, time.Second)

	// recording wraps whatever the production lookup resolves and records
	// each wrapped handler that runs.
	recording := func(ran *[]string, mu *sync.Mutex) func(string) (hookspec.Handler, bool) {
		return func(name string) (hookspec.Handler, bool) {
			handler, ok := hooks.Lookup(name)
			if !ok {
				return nil, false
			}
			return func(req hookspec.Request) hookspec.Outcome {
				mu.Lock()
				*ran = append(*ran, name)
				mu.Unlock()
				return handler(req)
			}, true
		}
	}

	t.Run("production lookup misses and the steps run", func(t *testing.T) {
		var mu sync.Mutex
		var ran []string
		stages := Production()
		stages.Lookup = recording(&ran, &mu)
		result := runCheck(context.Background(), stages, p, eventRequest("claude-code", event), checks[0])
		if result.fault != "" || result.outcome.Kind() != contract.KindDeny || !strings.Contains(result.outcome.Payload(), "destructive_file_ops") {
			t.Fatalf("production lookup did not reach the steps: %+v", result)
		}
		mu.Lock()
		defer mu.Unlock()
		if !slices.Equal(ran, []string{"command-guardian", "release-guardian"}) {
			t.Fatalf("steps ran = %v, want command-guardian then release-guardian once each", ran)
		}
	})

	t.Run("a lookup stubbed under the check's name intercepts it", func(t *testing.T) {
		var mu sync.Mutex
		var ran []string
		var stubbed atomic.Int32
		production := recording(&ran, &mu)
		stages := Production()
		stages.Lookup = func(name string) (hookspec.Handler, bool) {
			if name != "commands" {
				return production(name)
			}
			return func(hookspec.Request) hookspec.Outcome { stubbed.Add(1); return contract.Pass() }, true
		}
		if result := runCheck(context.Background(), stages, p, eventRequest("claude-code", event), checks[0]); result.fault != "" || result.outcome.Kind() != contract.KindPass {
			t.Fatalf("stubbed lookup result: %+v", result)
		}
		mu.Lock()
		defer mu.Unlock()
		if stubbed.Load() != 1 || len(ran) != 0 {
			t.Fatalf("stub calls=%d steps ran=%v, want 1 and none", stubbed.Load(), ran)
		}
	})

	t.Run("a check with no registry entry and no steps is unavailable", func(t *testing.T) {
		result := runCheck(context.Background(), Production(), p, eventRequest("claude-code", event), hooks.EventCheck{Name: "commands", Guard: true})
		if result.fault != "missing handler" || result.outcome.Kind() != contract.KindError {
			t.Fatalf("handlerless check result: %+v", result)
		}
	})
}

func TestEventFailurePostureWorkers(t *testing.T) {
	for _, surface := range surface.All() {
		for _, fault := range []string{"error", "empty error", "invalid", "panic", "missing", "lookup panic"} {
			verb, event := "pre-tool", "PreToolUse"
			if surface == "copilot" {
				event = strings.ToLower(event[:1]) + event[1:]
			}
			t.Run(string(surface)+"/"+verb+"/"+fault, func(t *testing.T) {
				stages := Production()
				stages.Lookup = func(string) (hookspec.Handler, bool) {
					if fault == "missing" {
						return nil, false
					}
					if fault == "lookup panic" {
						panic("private lookup panic")
					}
					return func(hookspec.Request) hookspec.Outcome {
						switch fault {
						case "error":
							return contract.Error("failure diagnostic")
						case "empty error":
							return contract.Error("")
						case "invalid":
							return hookspec.Outcome{}
						default:
							panic("private handler panic")
						}
					}, true
				}
				p := budgetedPosture(map[string]time.Duration{"check": time.Second}, time.Second)
				results := runChecks(context.Background(), stages, p, eventRequest(string(surface), hookspec.Event{}), []hooks.EventCheck{{Name: "check", Guard: true}})
				rendered := foldResults(stages, p.event, string(surface), event, results)
				requireRenderedDeny(t, surface, rendered)
				if bytes.Contains(append(rendered.Stdout, rendered.Stderr...), []byte("private")) {
					t.Fatalf("panic value leaked into output: %+v", rendered)
				}
			})
		}
	}
}
