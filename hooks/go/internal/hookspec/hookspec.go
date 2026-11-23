// Package hookspec holds the contract shared by the agx entry point,
// the hook registry, the surface-adapter layer, and every hook core.
//
// The contract package owns the shared values; this package owns handler requests.
//
// Keep it free of hook-specific and provider-specific knowledge.
//
// Panics: internal/checkrun recovers a panic that escapes a handler, turns it
// into a contract error outcome, and renders that through the calling surface,
// so a crashed core fails the way each surface expects instead of exiting 2 with a
// goroutine dump — which a host would read as a deliberate block. recover
// covers only the panicking goroutine, so a handler that starts goroutines
// must recover inside each one.
package hookspec

import (
	"context"
	"io"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// Exit codes. They cross a process boundary into the hosts' hook protocols,
// so they are contract, not implementation detail.
//
// A handler never picks one. It returns an Outcome, and the provider adapter
// maps that outcome to the exit code its surface expects — the same outcome
// exits 2 on claude-code and 0 on copilot.
const (
	// ExitOK means the hook ran and raised no objection.
	ExitOK = 0

	// ExitUsage means the invocation itself was wrong: unknown provider,
	// unknown hook, or a missing argument. It is also the claude-code
	// adapter's exit for an adapter or core fault, which Claude Code treats
	// as a non-blocking error. It must never be confused with a deny, which
	// is why argv validation never exits 2.
	ExitUsage = 1

	// ExitDeny is a surface's blocking signal. Claude Code treats exit 2 from
	// a hook as blocking and replays stderr to the model, so the claude-code
	// adapter uses it for a deny (docs/surfaces/claude-code.md, Hook
	// protocol). Inside this binary only a provider's Render may produce
	// it: never argv validation, never a panic. hooks/run-hook.cmd is the one
	// other source, on purpose — a missing command-guardian binary exits 2 so
	// a guard that cannot run fails closed.
	ExitDeny = 2
)

// Event is the typed provider-neutral event produced by an adapter's Normalize.
type Event = contract.Event

// Outcome is the canonical typed result returned by every hook core.
type Outcome = contract.Outcome

// Request is everything a hook core is given.
//
// It carries no process streams. Cores write their progress chatter — the
// guardian's threat table — to Diag, and the provider's
// Render discards that on every outcome but error, where it takes precedence
// over the outcome's own message. The provider owns the process streams.
type Request struct {
	// Context cancels work owned by this invocation. Nil preserves legacy behavior.
	Context context.Context

	// Provider is the calling surface: claude-code, copilot, or codex.
	Provider string

	// Hook is the registry key this request resolved through.
	Hook string

	// Event is the normalized event the adapter produced from the surface's
	// raw payload. Event and its Read result are immutable; handlers may
	// decode fresh input maps. A core never parses provider-shaped JSON.
	Event Event

	// Diag is the capture buffer for the core's own progress chatter. It may
	// be nil; a core treats nil as io.Discard.
	Diag io.Writer

	// Scripts reads local script files for the guards. Nil disables script
	// inspection. One reader is shared by every check of a grouped event so
	// the checks inspect identical bytes.
	Scripts shellwords.ScriptReader

	// Settings resolves the workflow settings. One source is shared by every
	// check of a grouped event so the rule sets judge one snapshot; nil reads
	// the files on each call.
	Settings *settings.Source
}

// ExecutionContext returns the invocation context, with a legacy background default.
func (r Request) ExecutionContext() context.Context {
	if r.Context != nil {
		return r.Context
	}
	return context.Background()
}

// Handler runs one hook core and returns the outcome it reached.
//
// A handler owns its failure policy — a guardrail denies where a best-effort
// check reports — but not the exit code: rendering an outcome into a
// surface's response is the provider adapter's job.
type Handler func(Request) Outcome
