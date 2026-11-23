package shellwords

import "strings"

// ExecutionContext separates protective syntax from authority to establish a
// new exemption. Values are call-local snapshots; no process state is changed.
type ExecutionContext struct {
	Dialect Dialect
	Proven  bool
	Reason  string
}

// AllowsNewProof reports whether the closed source contract establishes authority.
func (c ExecutionContext) AllowsNewProof() bool { return c.Proven && c.Reason == "" }

// ForInvocation retains the child's effective syntax while narrowing authority
// using the policy parse supplied by the caller. Callers may supply a separately
// proven data-masked parse; this function never re-reads or rewrites source.
// The current closed inert-predecessor subset is empty. Thus even harmless
// predecessors withhold new proof, as do visible shell-state writes. Inherited
// aliases and functions remain outside this lexical source contract.
func (c ExecutionContext) ForInvocation(parsed Result, index int) ExecutionContext {
	if index < 0 || index >= len(parsed.Invocations) {
		c.Proven = false
		c.Reason = "missing invocation"
		return c
	}
	inv := parsed.Invocations[index]
	c.Dialect = inv.Dialect
	if !c.AllowsNewProof() {
		return c
	}
	if index != 0 || len(parsed.BareAssignments) != 0 || len(parsed.EnvironmentEffects) != 0 ||
		!inv.Inspectable || !inv.Complete || inv.EnvironmentCall != nil || len(inv.Assignments) != 0 || inv.Parent >= 0 ||
		inv.Before != Start || len(inv.Words) == 0 {
		c.Proven = false
		c.Reason = "unknown preceding execution state"
		return c
	}
	verb := strings.ToLower(ExecutableName(inv.Words[0].Text))
	if inv.Dialect == PowerShell {
		verb = identityWriterName(verb)
	}
	switch verb {
	case "function", "filter", "set-alias", "new-alias", "remove-alias", "alias", "unalias", "source", ".", "eval", "invoke-expression", "iex", "set-item", "new-item", "clear-item", "rename-item", "remove-itemproperty", "set-itemproperty", "new-itemproperty", "copy-itemproperty", "move-itemproperty", "clear-itemproperty", "rename-itemproperty", "set-variable", "new-variable", "remove-variable", "clear-variable":
		c.Proven = false
		c.Reason = "visible command identity mutation"
	}
	// Only executable/write roles can change identity. A search operand such
	// as 'Function:rg' or '$env:PATH' is data, not an identity write.
	first := inv.Words[0]
	if !first.Literal || identityProviderPath(first.Text) {
		c.Proven = false
		c.Reason = "unknown command identity"
	}
	switch verb {
	case "set-content", "add-content", "clear-content", "out-file", "copy-item", "move-item", "remove-item", "rm", "ri", "del":
		if !identityWriteRolesKnown(inv.Words[1:], verb) {
			c.Proven = false
			c.Reason = "unknown identity write state"
		}
	}
	redirects, complete := inv.OwnRedirects()
	if !complete {
		c.Proven = false
		c.Reason = "unknown redirection state"
	}
	for _, redirect := range redirects {
		if strings.Contains(redirect.Operator, ">") && (!redirect.Target.Literal || identityProviderPath(redirect.Target.Text)) {
			c.Proven = false
			c.Reason = "visible command identity mutation"
		}
	}
	return c
}

func identityProviderPath(text string) bool {
	colon := strings.IndexByte(text, ':')
	if colon < 0 {
		return false
	}
	// Only an ordinary absolute drive path has a known filesystem shape.
	// Provider-qualified paths and custom drives can address shell identity;
	// inspecting installed providers or inherited drives is outside this proof.
	return !(colon == 1 && len(text) > 2 &&
		(text[0] >= 'A' && text[0] <= 'Z' || text[0] >= 'a' && text[0] <= 'z') &&
		(text[2] == '\\' || text[2] == '/') && !strings.Contains(text[2:], ":"))
}

// identityWriterName recognizes built-in writer aliases conservatively across
// PowerShell platforms. Recognition narrows proof; it does not assert that an
// inherited alias still resolves to its built-in command on this host.
func identityWriterName(verb string) string {
	switch verb {
	case "cpi", "copy", "cp":
		return "copy-item"
	case "mi", "move", "mv":
		return "move-item"
	case "ri", "rm", "del", "erase", "rd", "rmdir":
		return "remove-item"
	case "sc":
		return "set-content"
	case "ac":
		return "add-content"
	case "clc":
		return "clear-content"
	case "si":
		return "set-item"
	case "ni":
		return "new-item"
	case "cli":
		return "clear-item"
	case "ren", "rni":
		return "rename-item"
	case "sal":
		return "set-alias"
	case "nal":
		return "new-alias"
	case "sp":
		return "set-itemproperty"
	case "rp":
		return "remove-itemproperty"
	case "cpp":
		return "copy-itemproperty"
	case "mp":
		return "move-itemproperty"
	case "clp":
		return "clear-itemproperty"
	case "rnp":
		return "rename-itemproperty"
	case "set", "sv":
		return "set-variable"
	case "nv":
		return "new-variable"
	case "rv", "rmv":
		return "remove-variable"
	case "clv":
		return "clear-variable"
	}
	return verb
}

// identityWriteRolesKnown proves only that a bounded writer's literal path
// roles do not address shell identity. It does not authorize its data or sink.
func identityWriteRolesKnown(words []Word, verb string) bool {
	pathSeen := false
	for i := 0; i < len(words); i++ {
		word := words[i]
		pathRole := true
		if strings.HasPrefix(word.Text, "-") {
			option, inline, joined := strings.Cut(strings.ToLower(word.Text), ":")
			switch option {
			case "-path", "-literalpath", "-filepath", "-destination":
			case "-value", "-filter", "-include", "-exclude", "-credential", "-stream", "-encoding":
				pathRole = false
			case "-force", "-recurse", "-append", "-nonewline", "-noclobber", "-whatif", "-confirm":
				if joined {
					return false
				}
				continue
			default:
				return false
			}
			if joined {
				word.Text = inline
			} else {
				i++
				if i == len(words) {
					return false
				}
				word = words[i]
			}
		} else if pathSeen && (verb == "set-content" || verb == "add-content") {
			// Positional values follow the first path; their text is data.
			pathRole = false
		}
		if pathRole {
			if !word.Literal || identityProviderPath(word.Text) {
				return false
			}
			pathSeen = true
		}
	}
	return pathSeen
}
