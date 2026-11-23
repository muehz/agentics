package checkrun

import (
	"fmt"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
)

// fault names one way a hook invocation goes wrong. Each plan holds one cell
// per fault, so a fault one plan answers and the other lacks is still written
// down on both sides.
type fault int

const (
	// faultDispatchRejected: the surface refused the argv, or a replaced
	// dispatch stage named an event other than the trusted one.
	faultDispatchRejected fault = iota
	// faultUnreadableInput: stdin failed or exceeded the payload bound.
	faultUnreadableInput
	// faultMalformedPayload: the surface's normalize stage refused the payload.
	faultMalformedPayload
	// faultInputDeadline: reading and normalizing the payload outran the
	// input budget.
	faultInputDeadline
	// faultInputPanic: a stage panicked while the payload was read and
	// normalized under the input budget.
	faultInputPanic
	// faultInvalidEvent: the normalized event does not read back.
	faultInvalidEvent
	// faultSelection: no check list could be selected for the event.
	faultSelection
	// faultPipelinePanic: a stage panicked outside any check.
	faultPipelinePanic
	// faultLookupMiss: the registry resolved neither the hook nor every step
	// of the check.
	faultLookupMiss
	// faultHandlerPanic: a check's own handler panicked.
	faultHandlerPanic
	// faultInvalidOutcome: a check returned an outcome the contract refuses.
	faultInvalidOutcome
	// faultGuardError: a guard returned an error outcome.
	faultGuardError
	// faultCheckDeadline: a check finished only after its budget ran out.
	faultCheckDeadline
	// faultCleanupUnacknowledged: a check outlived its budget and the
	// cleanup grace.
	faultCleanupUnacknowledged
	// faultWorkerPanic: the code that runs a check panicked around it.
	faultWorkerPanic
	// faultStepPanic: one rule set of a check with steps panicked.
	faultStepPanic
	// faultStepInvalid: one rule set of a check with steps returned an
	// outcome the contract refuses.
	faultStepInvalid

	faultCount
)

// noFault marks a stage that finished without a fault.
const noFault fault = -1

// faultNames names each fault for the closure test and its failures.
var faultNames = [faultCount]string{
	faultDispatchRejected:      "dispatch-rejected",
	faultUnreadableInput:       "unreadable-input",
	faultMalformedPayload:      "malformed-payload",
	faultInputDeadline:         "input-deadline",
	faultInputPanic:            "input-panic",
	faultInvalidEvent:          "invalid-event",
	faultSelection:             "selection",
	faultPipelinePanic:         "pipeline-panic",
	faultLookupMiss:            "lookup-miss",
	faultHandlerPanic:          "handler-panic",
	faultInvalidOutcome:        "invalid-outcome",
	faultGuardError:            "guard-error",
	faultCheckDeadline:         "check-deadline",
	faultCleanupUnacknowledged: "cleanup-unacknowledged",
	faultWorkerPanic:           "worker-panic",
	faultStepPanic:             "step-panic",
	faultStepInvalid:           "step-invalid",
}

// response says how a cell answers its fault. The zero value is no answer at
// all, which the closure test refuses, so a cell left out of a plan cannot
// pass for a deliberate one.
type response int

const (
	responseUnset response = iota
	// responseAbsent: the plan has no such stage, so the fault cannot occur.
	responseAbsent
	// responseOutcome: the cell's own kind and message replace the outcome.
	responseOutcome
	// responseOwn: the check's own outcome stands, with the cell's fault
	// notice and chatter rule.
	responseOwn
	// responseRejection: the stage's own rendered rejection is the answer.
	responseRejection
	// responseUnanswered: the runner answers nothing and the entry point
	// reports the invocation.
	responseUnanswered
)

// cell is one plan's posture for one fault.
type cell struct {
	response response
	// kind is contract.KindError or contract.KindDeny for responseOutcome.
	kind string
	// message is the outcome's text. When formatted is set it is a format
	// over the subject (%[1]s, a hook name or a step label), the value
	// (%[2]v, a panic value or a read error) and the stack (%[3]s).
	message   string
	formatted bool
	// showsValue lets the value and the stack reach the message. Without it
	// only the subject is offered, so no panic value or stack can leak.
	showsValue bool
	// fault is the notice a check result carries, labelled in the reply.
	fault string
	// keepChatter keeps the diagnostic text the check wrote.
	keepChatter bool
	// guardDenies makes a guard's error or fault deny.
	guardDenies bool
}

// outcome builds the cell's outcome for one occurrence of its fault.
func (c cell) outcome(subject string, value any, stack []byte) hookspec.Outcome {
	text := c.message
	switch {
	case c.formatted && c.showsValue:
		text = fmt.Sprintf(c.message, subject, value, stack)
	case c.formatted:
		text = fmt.Sprintf(c.message, subject)
	}
	if c.kind == contract.KindDeny {
		return contract.Deny(text)
	}
	return contract.Error(text)
}

// plan is the whole posture of one way of running a hook.
type plan struct {
	name  string
	cells [faultCount]cell
	// inputBudget bounds reading and normalizing the payload, and
	// checkBudget each check. Zero means no budget: the request carries no
	// context.
	inputBudget, checkBudget time.Duration
	// grace is how long a check that outran its budget may take to return.
	grace time.Duration
	// label prefixes each check's message with the check's name, and join
	// separates the messages. An empty label means the plan has one check
	// and no fold.
	label, join string
	// chatterOnValid passes a check's chatter on with a valid outcome that
	// is neither an error nor a fault.
	chatterOnValid bool
}

// invalidOutcomeDiagnostic is what both plans report when a check's outcome
// is invalid, including a handler that returns its zero value.
const invalidOutcomeDiagnostic = "core produced an invalid neutral outcome"

// legacyPlan is the single check "<surface> <hook> --mode block". It keeps
// the surface's own error shape for every fault, so a fault never reads as a
// deny, and it shows the panic value and stack to the person reading stderr.
var legacyPlan = plan{
	name: "legacy",
	cells: [faultCount]cell{
		faultDispatchRejected: {response: responseRejection},
		faultUnreadableInput: {response: responseOutcome, kind: contract.KindError,
			message: "unable to read the hook payload: %[2]v", formatted: true, showsValue: true},
		faultMalformedPayload: {response: responseRejection},
		faultInputDeadline:    {response: responseAbsent},
		faultInputPanic:       {response: responseAbsent},
		faultInvalidEvent:     {response: responseAbsent},
		faultSelection:        {response: responseAbsent},
		faultPipelinePanic: {response: responseOutcome, kind: contract.KindError,
			message: "agx: internal error: %[2]v\n%[3]s", formatted: true, showsValue: true},
		faultLookupMiss: {response: responseUnanswered},
		faultHandlerPanic: {response: responseOutcome, kind: contract.KindError,
			message: "agx: internal error in %[1]s hook: %[2]v\n%[3]s", formatted: true, showsValue: true, keepChatter: true},
		// A broken core must not smuggle text into an error it did not
		// legitimately produce, so its chatter goes with the outcome.
		faultInvalidOutcome:        {response: responseOutcome, kind: contract.KindError, message: invalidOutcomeDiagnostic},
		faultGuardError:            {response: responseOwn, keepChatter: true},
		faultCheckDeadline:         {response: responseAbsent},
		faultCleanupUnacknowledged: {response: responseAbsent},
		faultWorkerPanic:           {response: responseAbsent},
		faultStepPanic:             {response: responseAbsent},
		faultStepInvalid:           {response: responseAbsent},
	},
	chatterOnValid: true,
}

// eventPlan is the grouped event "<surface> pre-tool". Every fault denies,
// because a guard that cannot answer must not let the tool call through, and
// no message carries a panic value or a stack, because the reply reaches the
// model.
var eventPlan = plan{
	name: "event",
	cells: [faultCount]cell{
		faultDispatchRejected: {response: responseOutcome, kind: contract.KindDeny, message: "agx: grouped event dispatch rejected the invocation"},
		faultUnreadableInput:  {response: responseOutcome, kind: contract.KindDeny, message: "agx: unable to read the bounded event payload"},
		faultMalformedPayload: {response: responseOutcome, kind: contract.KindDeny, message: "agx: malformed or mismatched event payload"},
		faultInputDeadline:    {response: responseOutcome, kind: contract.KindDeny, message: "agx: event payload receive/decode deadline exceeded"},
		faultInputPanic:       {response: responseOutcome, kind: contract.KindDeny, message: "agx: event input normalization panicked"},
		faultInvalidEvent:     {response: responseOutcome, kind: contract.KindDeny, message: "agx: normalized event fields are invalid"},
		faultSelection:        {response: responseOutcome, kind: contract.KindDeny, message: "agx: event selection failed"},
		faultPipelinePanic:    {response: responseOutcome, kind: contract.KindDeny, message: "agx: grouped event infrastructure panicked"},
		faultLookupMiss: {response: responseOutcome, kind: contract.KindError, message: "registered check is unavailable",
			fault: "missing handler", guardDenies: true},
		faultHandlerPanic: {response: responseOutcome, kind: contract.KindError, message: "check panicked",
			fault: "panic", keepChatter: true, guardDenies: true},
		faultInvalidOutcome: {response: responseOutcome, kind: contract.KindError, message: invalidOutcomeDiagnostic,
			fault: "invalid outcome", keepChatter: true, guardDenies: true},
		faultGuardError:    {response: responseOwn, fault: "check failed", keepChatter: true, guardDenies: true},
		faultCheckDeadline: {response: responseOwn, fault: "deadline exceeded; incomplete check", keepChatter: true, guardDenies: true},
		faultCleanupUnacknowledged: {response: responseOutcome, kind: contract.KindError,
			message: "check did not acknowledge cancellation within cleanup grace",
			fault:   "deadline exceeded; cleanup unacknowledged", guardDenies: true},
		faultWorkerPanic: {response: responseOutcome, kind: contract.KindError, message: "check infrastructure panicked",
			fault: "worker infrastructure panic", guardDenies: true},
		// A rule set's fault becomes the check's error outcome, which the
		// guard error cell then denies; it carries no notice of its own.
		faultStepPanic:   {response: responseOutcome, kind: contract.KindError, message: "%[1]s panicked", formatted: true},
		faultStepInvalid: {response: responseOutcome, kind: contract.KindError, message: "rule set returned an invalid outcome"},
	},
	inputBudget:    2 * time.Second,
	checkBudget:    10 * time.Second,
	grace:          time.Second,
	label:          "[%s] ",
	join:           "\n\n",
	chatterOnValid: false,
}

// posture is both plans as one runner reads them.
type posture struct {
	legacy, event plan
	// checkBudgets overrides the event plan's check budget by check name, so
	// a test can give two checks of one event different budgets. Production
	// leaves it empty.
	checkBudgets map[string]time.Duration
}

// productionPosture is the posture Answer runs with.
func productionPosture() posture {
	return posture{legacy: legacyPlan, event: eventPlan}
}

// budgetFor is the budget of the check named name on the event plan.
func (p posture) budgetFor(name string) time.Duration {
	if budget, ok := p.checkBudgets[name]; ok {
		return budget
	}
	return p.event.checkBudget
}
