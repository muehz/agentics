package checkrun

import (
	"bytes"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hooks"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/provider"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestEventAggregationDenyPrecedence(t *testing.T) {
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			results := []checkResult{
				{check: hooks.EventCheck{Name: "first", Guard: true}, outcome: contract.Deny("duplicate"), diagnostic: []byte("first diagnostic\n")},
				{check: hooks.EventCheck{Name: "second", Guard: true}, outcome: contract.Context("healthy context")},
				{check: hooks.EventCheck{Name: "third", Guard: true}, outcome: contract.Context("duplicate")},
				{check: hooks.EventCheck{Name: "fourth", Guard: true}, outcome: contract.Error("guard failure"), diagnostic: []byte("line one\nline two\n")},
				{check: hooks.EventCheck{Name: "fifth", Guard: true}, outcome: contract.Pass(), diagnostic: []byte("pass diagnostic\n")},
			}
			stages := Production()
			renders := 0
			stages.Render = func(surface, event string, outcome hookspec.Outcome, diag []byte) provider.Rendered {
				renders++
				return provider.Render(surface, event, outcome, diag)
			}
			event := "PreToolUse"
			if surface == "copilot" {
				event = "preToolUse"
			}
			rendered := foldResults(stages, eventPlan, string(surface), event, results)
			reason := requireRenderedDeny(t, surface, rendered)
			joined := "[first] duplicate\n\n[second] healthy context\n\n[third] duplicate\n\n[fourth] guard failure\n\n[fourth] check failed"
			if !strings.HasPrefix(reason, joined) || strings.Count(reason, "duplicate") != 2 || renders != 1 {
				t.Fatalf("decision/messages lost: renders=%d reason=%q", renders, reason)
			}
			for _, diagnostic := range []string{"[fourth] line one\nline two\n"} {
				if !bytes.Contains(rendered.Stderr, []byte(diagnostic)) {
					t.Fatalf("lost completed diagnostic %q: %q", diagnostic, rendered.Stderr)
				}
			}
			if bytes.Contains(rendered.Stderr, []byte("first diagnostic")) || bytes.Contains(rendered.Stderr, []byte("pass diagnostic")) {
				t.Fatalf("ordinary core chatter leaked into the event: %q", rendered.Stderr)
			}
		})
	}
}

func TestEventRenderCopilotPreWarningPreservesPermissions(t *testing.T) {
	result := checkResult{check: hooks.EventCheck{Name: "commands", Guard: true}, outcome: contract.Context("warning stays visible")}
	rendered := foldResults(Production(), eventPlan, "copilot", "preToolUse", []checkResult{result})
	if rendered.Exit != 0 || len(rendered.Stdout) != 0 || string(rendered.Stderr) != "[commands] warning stays visible\n" {
		t.Fatalf("warning changed normal permission flow: %+v", rendered)
	}
}

func TestEventAggregationEmptyAndInvalidResults(t *testing.T) {
	for _, result := range []checkResult{
		{check: hooks.EventCheck{Name: "guard", Guard: true}, outcome: contract.Error("")},
		{check: hooks.EventCheck{Name: "guard", Guard: true}},
	} {
		rendered := foldResults(Production(), eventPlan, "codex", "PreToolUse", []checkResult{result})
		requireRenderedDeny(t, surface.Codex, rendered)
	}
}

func TestFoldStepsRows(t *testing.T) {
	panics := func(hookspec.Request) hookspec.Outcome { panic("boom") }
	returns := func(outcome hookspec.Outcome) hookspec.Handler {
		return func(hookspec.Request) hookspec.Outcome { return outcome }
	}
	for _, tc := range []struct {
		name     string
		handlers []hookspec.Handler
		kind     string
		payload  string
	}{
		{"a-panic-becomes-an-error", []hookspec.Handler{panics}, contract.KindError, "test rules panicked"},
		{"deny-beats-context-and-keeps-order", []hookspec.Handler{returns(contract.Context("first")), returns(contract.Deny("second")), returns(contract.Pass())}, contract.KindDeny, "first\n\nsecond"},
		{"two-passes-pass", []hookspec.Handler{returns(contract.Pass()), returns(contract.Pass())}, contract.KindPass, ""},
		{"an-invalid-outcome-is-an-error", []hookspec.Handler{returns(contract.Outcome{})}, contract.KindError, "rule set returned an invalid outcome"},
		{"error-ranks-above-context", []hookspec.Handler{returns(contract.Context("note")), returns(contract.Error("broke"))}, contract.KindError, "note\n\nbroke"},
		{"deny-ranks-above-error", []hookspec.Handler{returns(contract.Error("broke")), returns(contract.Deny("no"))}, contract.KindDeny, "broke\n\nno"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := make([]boundStep, len(tc.handlers))
			for i, run := range tc.handlers {
				steps[i] = boundStep{label: "test rules", handler: run}
			}
			out := foldSteps(eventPlan, steps, hookspec.Request{})
			if out.Kind() != tc.kind || out.Payload() != tc.payload {
				t.Fatalf("outcome = %s %q, want %s %q", out.Kind(), out.Payload(), tc.kind, tc.payload)
			}
		})
	}
}
