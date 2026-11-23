// Package checkrun runs one hook invocation, from the hook lookup to the
// rendered reply, under one posture table with two plans: the grouped event
// ("<surface> pre-tool") and the legacy single check ("<surface> <hook>
// --mode block").
//
// The two plans differ on purpose. The grouped event guards every tool call
// at once and reaches the model, so each of its faults denies and shows no
// panic value; the single check keeps the surface's own error shape, which no
// host reads as a deny. posture.go holds every difference as a cell or a plan
// field, so the other files hold only what both plans share. The eleven
// differences are:
//
//  1. A fault before any check (bad argv, unreadable stdin, a malformed
//     payload) keeps the surface's error shape on the legacy plan and denies
//     on the event plan.
//  2. A pipeline panic (dispatch, normalize, render, lookup) shows the panic
//     value and a stack on the legacy plan and fixed text on the event plan.
//  3. A handler panic shows a stack on the legacy plan and "check panicked"
//     on the event plan.
//  4. A guard's error outcome keeps the error shape on the legacy plan and
//     denies on the event plan, with or without a message.
//  5. An invalid outcome drops the check's chatter on the legacy plan and
//     keeps it, labelled, on the event plan.
//  6. Only the event plan labels each message "[<check>] " and joins them
//     with a blank line.
//  7. A lookup miss is answered by the entry point as a usage error on the
//     legacy plan and is a deny naming the unavailable check on the event
//     plan.
//  8. Only the event plan cross-checks the stages: the dispatched event
//     against the trusted one, the event's surface against the invocation,
//     and the normalized fields.
//  9. Only the event plan gives the input and each check a budget, each
//     check its own worker, and a cleanup grace.
//  10. Grouped context reaches stderr on copilot only, as the provider's
//     trait says.
//  11. A NUL in the event is denied by the grouped event even with every
//     skip switch set, while a single check's skip switch wins, because the
//     event plan reads the event back before any check runs.
//
// The entry point in cmd/hook keeps argv, stdin and emit: it parses the
// surface, hands the rest here, and writes the reply this package returns.
package checkrun

import (
	"context"
	"io"
	"runtime/debug"

	"github.com/muehz/agentics/hooks/go/internal/hooks"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/provider"
	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// preToolVerb is the hook name that runs the grouped event.
const preToolVerb = "pre-tool"

// Stages are the replaceable steps of an invocation. Production wires the
// registry and the provider adapters; a test replaces any of them.
type Stages struct {
	// Lookup resolves a hook name to its handler.
	Lookup func(string) (handler, bool)
	// Dispatch validates the argv and names the surface's event; a non-nil
	// rejection ends the invocation before stdin is read.
	Dispatch func(string, string, []string) (string, *provider.Rendered)
	// Normalize turns the raw payload into the neutral event, or renders the
	// surface's own rejection of it.
	Normalize func(string, string, []byte) (hookspec.Event, *provider.Rendered)
	// Render turns an outcome and the core's chatter into the surface's reply.
	Render func(string, string, hookspec.Outcome, []byte) provider.Rendered
}

// Production returns the stages the binary runs with. It is a fresh value
// on every call, so no caller shares a seam another one replaced.
func Production() Stages {
	return Stages{
		Lookup:    hooks.Lookup,
		Dispatch:  provider.Dispatch,
		Normalize: provider.Normalize,
		Render:    provider.Render,
	}
}

// Answer runs one invocation on surface id and returns the reply to emit.
// The boolean is false only when verb is neither "pre-tool" nor a hook that
// stages.Lookup resolves; then no later stage has run and stdin is unread,
// and the entry point reports the unknown hook itself.
func Answer(stages Stages, id surface.ID, verb string, args []string, stdin io.Reader) (provider.Rendered, bool) {
	return answer(context.Background(), stages, productionPosture(), id, verb, args, stdin)
}

// answer is Answer under a given parent context and posture, so a test can
// cancel the checks or shorten the budgets.
//
// A panic in any stage outside a check is caught here and rendered through
// provider.Render, never the replaceable render stage, so a render panic
// cannot recur while its own fault is reported.
func answer(parent context.Context, stages Stages, p posture, id surface.ID, verb string, args []string, stdin io.Reader) (rendered provider.Rendered, answered bool) {
	name := string(id)
	pl := p.legacy
	var event string
	if verb == preToolVerb {
		pl = p.event
		// The trusted event is resolved before a replaceable stage can fail,
		// so even a dispatch panic is rendered as the surface's tool-use event.
		event, _ = provider.Dispatch(name, verb, nil)
	}
	defer func() {
		if r := recover(); r != nil {
			outcome := pl.cells[faultPipelinePanic].outcome("", r, debug.Stack())
			rendered, answered = provider.Render(name, event, outcome, nil), true
		}
	}()
	if verb == preToolVerb {
		return runEvent(parent, stages, p, name, event, verb, args, stdin), true
	}
	return runLegacy(stages, p.legacy, name, verb, args, stdin, &event)
}

// runLegacy runs one registry hook as the legacy plan. It records the event
// dispatch names in *event, so a later panic renders as that event.
func runLegacy(stages Stages, pl plan, name, hook string, args []string, stdin io.Reader, event *string) (provider.Rendered, bool) {
	own, ok := stages.Lookup(hook)
	if !ok {
		return provider.Rendered{}, false
	}
	dispatched, reject := stages.Dispatch(name, hook, args)
	*event = dispatched
	if reject != nil {
		return *reject, true
	}
	// The payload is read only once dispatch has accepted the invocation, so
	// an argv typo never blocks waiting on a stream that will not arrive.
	payload, err := readPayload(stdin)
	if err != nil {
		return stages.Render(name, dispatched, pl.cells[faultUnreadableInput].outcome("", err, nil), nil), true
	}
	normalized, malformed := stages.Normalize(name, dispatched, payload)
	if malformed != nil {
		return *malformed, true
	}
	result := invokeCheck(pl, runnable{handler: own}, hooks.EventCheck{Name: hook}, hookspec.Request{
		Provider: name,
		Hook:     hook,
		Event:    normalized,
		Scripts:  hooks.NewScriptSnapshot(),
		Settings: settings.NewSource(settings.Resolve),
	})
	return stages.Render(name, dispatched, result.outcome, result.diagnostic), true
}

// runEvent runs the grouped event as the event plan.
func runEvent(parent context.Context, stages Stages, p posture, name, event, verb string, args []string, stdin io.Reader) provider.Rendered {
	pl := p.event
	mapped, reject := stages.Dispatch(name, verb, args)
	if reject != nil || mapped != event {
		return stages.Render(name, event, pl.cells[faultDispatchRejected].outcome("", nil, nil), nil)
	}
	normalized, inputFault := readEvent(stages, pl, name, event, stdin)
	if inputFault != noFault {
		return stages.Render(name, event, pl.cells[inputFault].outcome("", nil, nil), nil)
	}
	checks, err := hooks.SelectEventChecks(name, normalized)
	if err != nil {
		return stages.Render(name, event, pl.cells[faultSelection].outcome("", nil, nil), nil)
	}
	base := hookspec.Request{
		Provider: name,
		Event:    normalized,
		Scripts:  hooks.NewScriptSnapshot(),
		Settings: settings.NewSource(settings.Resolve),
	}
	return foldResults(stages, pl, name, event, runChecks(parent, stages, p, base, checks))
}
