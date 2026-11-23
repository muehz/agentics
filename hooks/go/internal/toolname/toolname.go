// Package toolname resolves the host-independent name of a tool call.
//
// A host that exposes an MCP server prefixes every tool it registers with a
// namespace, and the spelling of that namespace is the host's choice, not the
// server's: Claude Code registers github-mcp-server's create_pull_request as
// mcp__github__create_pull_request, and other hosts use a dot, a slash, or a
// colon for the same idea. The server-side name is the only part a rule can
// key on, so every hook that gates an MCP tool compares Base(name) against a
// bare server-side name.
package toolname

import "strings"

// separators are the namespace delimiters hosts use, longest first.
//
// A single underscore is deliberately absent. Server-side names are snake_case
// (create_pull_request, get_file_contents), so splitting on one underscore
// would reduce every one of them to its last word.
var separators = []string{"__", ".", "/", ":"}

// Base returns the last namespace component of a tool name.
//
// A local tool name carries no separator and is returned unchanged, so a
// caller may pass every tool name through this function without special-casing
// the non-MCP path.
//
// Matching the RESULT against a bare name — rather than testing the full name
// with strings.HasSuffix — is what keeps a neighbouring tool out of a gate.
// create_pull_request_with_copilot ends in no separator before
// create_pull_request, so it is its own base name and matches nothing the
// create_pull_request rule holds.
func Base(name string) string {
	cut := 0
	for _, sep := range separators {
		if i := strings.LastIndex(name, sep); i >= 0 && i+len(sep) > cut {
			cut = i + len(sep)
		}
	}
	if cut >= len(name) {
		// A name that is nothing but a namespace prefix names no tool. The
		// empty string matches no rule, which is the correct outcome: this
		// function must never invent a name a rule could gate on.
		return ""
	}
	return name[cut:]
}

// IsShell reports whether name is a host tool whose input carries a shell
// command line: Claude Code's Bash and, on Windows, its native PowerShell
// tool. Copilot's adapter already normalizes its powershell tool to PowerShell,
// so a guard that gates on this function preserves the shell dialect on both surfaces.
// The comparison uses Base so an MCP-namespaced spelling cannot slip past.
func IsShell(name string) bool {
	switch Base(name) {
	case "Bash", "PowerShell":
		return true
	}
	return false
}
