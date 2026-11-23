// Package surface names the coding-agent surfaces Agentics runs on. It is a leaf:
// it imports nothing from the module, so packages below provider can use it.
//
// Parse is the only way a string becomes an ID, and the set it accepts is
// closed: an unknown, differently cased, padded, or NUL-bearing name is
// refused, so a surface name that arrives on the command line cannot select
// the wrong rendering of a deny.
package surface

// ID names one surface. The zero ID names none and is never a member of All.
type ID string

// The four surfaces. Each constant is the name given on the command line.
const (
	Antigravity ID = "antigravity"
	ClaudeCode  ID = "claude-code"
	Copilot     ID = "copilot"
	Codex       ID = "codex"
)

// All returns every surface sorted by name. Each call returns a new slice.
func All() []ID {
	return []ID{Antigravity, ClaudeCode, Codex, Copilot}
}

// Parse returns the surface with exactly this name. It refuses every other
// string with the zero ID and false.
func Parse(name string) (ID, bool) {
	switch id := ID(name); id {
	case Antigravity, ClaudeCode, Copilot, Codex:
		return id, true
	}
	return "", false
}

// Label returns the name a message uses for the surface's host product.
func (id ID) Label() string {
	switch id {
	case Antigravity:
		return "Antigravity"
	case ClaudeCode:
		return "Claude"
	case Copilot:
		return "Copilot"
	case Codex:
		return "Codex"
	}
	return ""
}

// PreToolEvent returns the surface's wire name for the one event every hook
// serves, the moment before a tool runs. It is empty for the zero ID and for
// any ID that Parse refuses.
func (id ID) PreToolEvent() string {
	switch id {
	case Antigravity, ClaudeCode, Codex:
		return "PreToolUse"
	case Copilot:
		return "preToolUse"
	}
	return ""
}
