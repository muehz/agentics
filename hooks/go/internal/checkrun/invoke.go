package checkrun

import (
	"bytes"
	"context"
	"runtime/debug"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hooks"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
)

// handler is the registry's handler type under a local name.
type handler = hookspec.Handler

// checkResult is what one check produced: its outcome, the diagnostic text
// it wrote, and the fault notices the runner added.
type checkResult struct {
	check          hooks.EventCheck
	outcome        hookspec.Outcome
	diagnostic     []byte
	fault          string
	cleanupPending bool
}

// indexedResult carries a result back to its check's place in the list.
type indexedResult struct {
	index  int
	result checkResult
}

// boundStep is one rule set of a check, resolved to its handler.
type boundStep struct {
	label   string
	handler handler
}

// runnable is a resolved check: its own handler, or its steps in order.
type runnable struct {
	handler handler
	steps   []boundStep
}

// resolve finds what a check runs. The lookup of the check's own name comes
// first, so a test that stubs that name intercepts the whole check. On a miss
// the check's steps are resolved, every one before any runs, so a missing
// rule set leaves the check unavailable rather than half run.
func resolve(stages Stages, check hooks.EventCheck) (runnable, bool) {
	if own, ok := stages.Lookup(check.Name); ok {
		return runnable{handler: own}, own != nil
	}
	if len(check.Steps) == 0 {
		return runnable{}, false
	}
	steps := make([]boundStep, 0, len(check.Steps))
	for _, step := range check.Steps {
		resolved, ok := stages.Lookup(step.Hook)
		if !ok || resolved == nil {
			return runnable{}, false
		}
		steps = append(steps, boundStep{label: step.Label, handler: resolved})
	}
	return runnable{steps: steps}, true
}

// recovered is a panic that call caught, with the stack at the panic.
type recovered struct {
	value any
	stack []byte
}

// call runs one handler or one step. It is the only place a handler runs,
// so a panic in either becomes a value the caller turns into its plan's
// outcome instead of ending the goroutine. The recover covers only this
// goroutine; see hookspec's package comment.
func call(run handler, req hookspec.Request) (outcome hookspec.Outcome, panicked *recovered) {
	defer func() {
		if r := recover(); r != nil {
			panicked = &recovered{value: r, stack: debug.Stack()}
		}
	}()
	return run(req), nil
}

// invokeCheck runs a resolved check against req with a diagnostic buffer of
// its own, and applies the plan's panic and invalid-outcome cells. Steps share
// the request, its context and the buffer, and run in order whether or not
// the context has ended, so every rule set answers.
func invokeCheck(pl plan, run runnable, check hooks.EventCheck, req hookspec.Request) (result checkResult) {
	result.check = check
	var diagnostic bytes.Buffer
	req.Diag = &diagnostic
	keepChatter := true
	if run.steps != nil {
		result.outcome = foldSteps(pl, run.steps, req)
	} else {
		outcome, panicked := call(run.handler, req)
		result.outcome = outcome
		if panicked != nil {
			panicCell := pl.cells[faultHandlerPanic]
			result.outcome = panicCell.outcome(req.Hook, panicked.value, panicked.stack)
			result.fault = panicCell.fault
			keepChatter = panicCell.keepChatter
		}
	}
	invalidCell := pl.cells[faultInvalidOutcome]
	if outcome, ok := accepted(result.outcome, invalidCell); !ok {
		result.outcome = outcome
		result.fault = invalidCell.fault
		keepChatter = invalidCell.keepChatter
	}
	if keepChatter {
		result.diagnostic = append([]byte(nil), diagnostic.Bytes()...)
	}
	return result
}

// markCheckDeadline adds the deadline notice to a result that arrived after
// its check's budget ran out. The outcome stands: a partial verdict is kept.
func markCheckDeadline(pl plan, result checkResult) checkResult {
	if result.fault != "" {
		result.fault += "; "
	}
	result.fault += pl.cells[faultCheckDeadline].fault
	return result
}

// runCheck runs one check of the event plan under its own budget. A check
// that outruns the budget gets the cleanup grace to return; one that still
// has not is reported as pending and abandoned.
func runCheck(parent context.Context, stages Stages, p posture, base hookspec.Request, check hooks.EventCheck) checkResult {
	pl := p.event
	run, ok := resolve(stages, check)
	if !ok {
		missing := pl.cells[faultLookupMiss]
		return checkResult{check: check, outcome: missing.outcome(check.Name, nil, nil), fault: missing.fault}
	}
	ctx, cancel := context.WithTimeout(parent, p.budgetFor(check.Name))
	defer cancel()
	req := base
	req.Hook = check.Name
	req.Context = ctx
	done := make(chan checkResult, 1)
	go func() { done <- invokeCheck(pl, run, check, req) }()
	select {
	case result := <-done:
		if ctx.Err() != nil {
			return markCheckDeadline(pl, result)
		}
		return result
	case <-ctx.Done():
	}
	cancel()
	timer := time.NewTimer(pl.grace)
	defer timer.Stop()
	select {
	case result := <-done:
		return markCheckDeadline(pl, result)
	case <-timer.C:
		select {
		case result := <-done:
			return markCheckDeadline(pl, result)
		default:
		}
		pending := pl.cells[faultCleanupUnacknowledged]
		return checkResult{
			check: check, outcome: pending.outcome(check.Name, nil, nil),
			fault: pending.fault, cleanupPending: true,
		}
	}
}

// runChecks runs every check of one event concurrently and returns the
// results in the checks' order. Every check gets the same base request, so
// all of them read identical script bytes through its one snapshot.
func runChecks(parent context.Context, stages Stages, p posture, base hookspec.Request, checks []hooks.EventCheck) []checkResult {
	completed := make(chan indexedResult, len(checks))
	for index, check := range checks {
		go func(index int, check hooks.EventCheck) {
			var result checkResult
			defer func() {
				if recover() != nil {
					worker := p.event.cells[faultWorkerPanic]
					result = checkResult{check: check, outcome: worker.outcome(check.Name, nil, nil), fault: worker.fault}
				}
				completed <- indexedResult{index: index, result: result}
			}()
			result = runCheck(parent, stages, p, base, check)
		}(index, check)
	}
	results := make([]checkResult, len(checks))
	for range checks {
		result := <-completed
		results[result.index] = result.result
	}
	return results
}
