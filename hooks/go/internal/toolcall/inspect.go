package toolcall

import (
	"context"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// Inspection is one shell inspection of a call: the parse of the typed
// command, the parse a guard's policy reads, and the authority each policy
// invocation carries.
//
// Authority comes from the call and never from the masked text: the execution
// context is read from the call's arguments once, and the data view only
// changes which parse the authority is narrowed over.
//
// Policy differs from Source only for a PowerShell call whose data view was
// proven and whose reparse finished before cancellation. Whenever Policy is
// Source, DataProven is false and DataView is the command. Scripts and script
// calls are always read from Source, because the reparse of a data view
// expands no scripts.
//
// The value holds no memo: two Inspect calls parse twice.
type Inspection struct {
	// Source is the parse of the typed command, with scripts expanded when a
	// reader was given.
	Source shellwords.Result
	// Policy is the parse a guard judges invocations by: the parse of the
	// proven data view, or Source.
	Policy shellwords.Result
	// DataView is the masked text of a proven PowerShell data call, or the
	// command.
	DataView string
	// DataProven reports that Policy is the parse of a proven data view.
	DataProven bool

	execution shellwords.ExecutionContext
}

// Inspect parses the call's command for the guards. It reads the execution
// context, builds the script options from it and the proven working directory,
// parses the command with scripts, proves a PowerShell data view, and reparses
// that view, which is used only when the context is still live after the
// reparse. A nil reader expands no scripts.
//
// Only a command that is a JSON string is parsed, and an empty string counts.
// Without one, the parses are the zero Result, DataView is empty, DataProven is
// false, and the context is never consulted.
func (c Call) Inspect(ctx context.Context, scripts shellwords.ScriptReader) Inspection {
	execution := c.Execution()
	inspection := Inspection{execution: execution}
	command, ok := c.Command()
	if !ok {
		return inspection
	}
	source := shellwords.ParseScripts(ctx, command, execution.Dialect, c.ScriptOptions(scripts))
	inspection.Source, inspection.Policy, inspection.DataView = source, source, command
	if view, proven, err := shellwords.PowerShellFileData(ctx, command, execution); err == nil && proven {
		candidate := shellwords.ParseContext(ctx, view, execution.Dialect)
		if ctx.Err() == nil {
			inspection.Policy, inspection.DataView, inspection.DataProven = candidate, view, true
		}
	}
	return inspection
}

// Refusal reports why the guards will not vouch for Policy invocation index.
// An invocation that is not inspectable gives its own reason, even the zero
// one. An inspection role, such as a Bash syntax check, is refused as
// authority unavailable when the invocation has no new authority or the
// context is done; the context is consulted only after authority is granted.
// An index outside Policy is refused the same way, without consulting the
// context.
//
// Each guard keeps its own posture for a refused invocation: it decides
// whether the refusal denies, gates, or is reported.
func (i Inspection) Refusal(ctx context.Context, index int) (shellwords.Refusal, bool) {
	unavailable := shellwords.GuardRefusal(shellwords.GuardBashAuthorityUnavailable)
	if index < 0 || index >= len(i.Policy.Invocations) {
		return unavailable, true
	}
	invocation := i.Policy.Invocations[index]
	if !invocation.Inspectable {
		return invocation.Reason, true
	}
	if invocation.Inspection == shellwords.NoInspection {
		return shellwords.Refusal{}, false
	}
	if !i.Authority(index).AllowsNewProof() || ctx.Err() != nil {
		return unavailable, true
	}
	return shellwords.Refusal{}, false
}

// Authority returns the call's execution context narrowed to Policy
// invocation index. An index outside Policy has no authority.
func (i Inspection) Authority(index int) shellwords.ExecutionContext {
	return i.execution.ForInvocation(i.Policy, index)
}
