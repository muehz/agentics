package checkrun

import (
	"fmt"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/provider"
)

// accepted returns outcome when the contract accepts it, and otherwise the
// substitute cell's outcome and false. Every validity question the runner
// asks goes through here, so no path can render an outcome the contract has
// refused.
func accepted(outcome hookspec.Outcome, substitute cell) (hookspec.Outcome, bool) {
	if outcome.Valid() {
		return outcome, true
	}
	return substitute.outcome("", nil, nil), false
}

// stepSeverity ranks a step's outcome: deny, then error, then context, then
// pass and continue alike.
var stepSeverity = map[string]int{
	contract.KindPass: 0, contract.KindContinue: 0, contract.KindContext: 1, contract.KindError: 2, contract.KindDeny: 3,
}

// foldSteps runs a check's steps in order and folds their outcomes into one:
// the most severe kind wins and every message is kept in step order. A panic
// in one step becomes that step's error outcome, so it cannot keep a later
// step from answering.
func foldSteps(pl plan, steps []boundStep, req hookspec.Request) hookspec.Outcome {
	var messages []string
	worst := contract.KindPass
	for _, step := range steps {
		outcome, panicked := call(step.handler, req)
		if panicked != nil {
			outcome = pl.cells[faultStepPanic].outcome(step.label, nil, nil)
		}
		outcome, _ = accepted(outcome, pl.cells[faultStepInvalid])
		if stepSeverity[outcome.Kind()] > stepSeverity[worst] {
			worst = outcome.Kind()
		}
		if outcome.Payload() != "" {
			messages = append(messages, outcome.Payload())
		}
	}
	joined := strings.Join(messages, pl.join)
	switch worst {
	case contract.KindDeny:
		return contract.Deny(joined)
	case contract.KindError:
		return contract.Error(joined)
	case contract.KindContext:
		return contract.Context(joined)
	}
	return contract.Pass()
}

// labelled prefixes text with the check's label.
func labelled(pl plan, name, text string) string {
	return fmt.Sprintf(pl.label, name) + text
}

// foldResults folds the event's check results into one reply. A deny, or a
// guard's error or fault, denies; otherwise any message becomes context.
// Messages keep the checks' order whatever order they finished in. Errors,
// fault notices and the chatter of a check with an error or fault also go to
// stderr, after whatever the surface rendered there.
func foldResults(stages Stages, pl plan, surface, event string, results []checkResult) provider.Rendered {
	invalidCell, guardCell := pl.cells[faultInvalidOutcome], pl.cells[faultGuardError]
	var messages, diagnostics []string
	hasDeny := false
	for _, result := range results {
		outcome, ok := accepted(result.outcome, invalidCell)
		fault := result.fault
		if !ok {
			fault = invalidCell.fault
		}
		kind := outcome.Kind()
		if kind == contract.KindError && fault == "" {
			fault = guardCell.fault
		}
		if kind == contract.KindDeny || (result.check.Guard && guardCell.guardDenies && (kind == contract.KindError || fault != "")) {
			hasDeny = true
		}
		if outcome.Payload() != "" {
			if kind == contract.KindError {
				diagnostics = append(diagnostics, labelled(pl, result.check.Name, outcome.Payload()))
				if result.check.Guard {
					messages = append(messages, labelled(pl, result.check.Name, outcome.Payload()))
				}
			} else {
				messages = append(messages, labelled(pl, result.check.Name, outcome.Payload()))
			}
		}
		if fault != "" {
			notice := labelled(pl, result.check.Name, fault)
			diagnostics = append(diagnostics, notice)
			messages = append(messages, notice)
		}
		if len(result.diagnostic) != 0 && (pl.chatterOnValid || kind == contract.KindError || fault != "") {
			diagnostics = append(diagnostics, labelled(pl, result.check.Name, string(result.diagnostic)))
		}
	}
	joined := strings.Join(messages, pl.join)
	outcome := contract.Pass()
	switch {
	case hasDeny:
		outcome = contract.Deny(joined)
	case len(messages) != 0:
		outcome = contract.Context(joined)
	}
	rendered := stages.Render(surface, event, outcome, nil)
	if provider.GroupedContextOnStderr(surface) && !hasDeny && joined != "" {
		diagnostics = append([]string{joined}, diagnostics...)
	}
	if len(diagnostics) != 0 {
		if len(rendered.Stderr) != 0 && rendered.Stderr[len(rendered.Stderr)-1] != '\n' {
			rendered.Stderr = append(rendered.Stderr, '\n')
		}
		rendered.Stderr = append(rendered.Stderr, []byte(strings.Join(diagnostics, pl.join))...)
		if rendered.Stderr[len(rendered.Stderr)-1] != '\n' {
			rendered.Stderr = append(rendered.Stderr, '\n')
		}
	}
	return rendered
}
