// Package guardmode resolves the two switches a guard reads from its
// environment: the kill switch that stops the guard from running, and the mode
// variable that makes it advisory instead of blocking. It imports nothing from
// this module.
//
// Three invariants hold for every guard that uses it:
//
//   - Only the exact value "true" skips the guard. "TRUE", "1", "yes", and a
//     value with surrounding whitespace do not.
//   - A mode value the guard cannot use never downgrades it. Anything other
//     than "warn" or "block" resolves to block, with a warning that names the
//     value.
//   - The skip variable is read before the mode variable, so a skipped guard
//     never reports on a mode value it will not use.
//
// release-guardian has no skip switch and no warn mode, and does not use this
// package.
package guardmode

import "fmt"

// Mode is the effect a guard's findings have. Its zero value is Block, so a
// zero Resolution never downgrades a guard.
type Mode int

const (
	// Block makes a finding deny the tool call.
	Block Mode = iota
	// Warn reports a finding as context and lets the tool call proceed.
	Warn
)

// Blocks reports whether findings deny the call. Every value other than Warn
// blocks, so a corrupted Mode fails closed.
func (m Mode) Blocks() bool { return m != Warn }

// String returns "warn" for Warn and "block" for every other value, so the
// mode a guard logs is never a third word.
func (m Mode) String() string {
	if m == Warn {
		return "warn"
	}
	return "block"
}

// Control names a guard and the two environment variables that steer it.
type Control struct {
	// Guard prefixes the warning, for example "command-guardian".
	Guard string
	// SkipVar is the kill switch variable.
	SkipVar string
	// ModeVar is the mode override variable.
	ModeVar string
}

// Resolution is what a guard does for one invocation.
type Resolution struct {
	// Skipped is true when the kill switch is set: the guard does not run.
	Skipped bool
	// Mode is the effect of a finding. It is Block when Skipped is true.
	Mode Mode
	// Warning is non-empty when the mode variable held a value other than
	// "warn", "block", or empty. It carries no trailing newline.
	Warning string
}

// Resolve reads the skip variable and then the mode variable through getenv.
func (c Control) Resolve(getenv func(string) string) Resolution {
	if getenv(c.SkipVar) == "true" {
		return Resolution{Skipped: true, Mode: Block}
	}
	switch value := getenv(c.ModeVar); value {
	case "warn":
		return Resolution{Mode: Warn}
	case "block", "":
		return Resolution{Mode: Block}
	default:
		return Resolution{
			Mode:    Block,
			Warning: fmt.Sprintf("%s: %s=%q is not warn or block; using block", c.Guard, c.ModeVar, value),
		}
	}
}
