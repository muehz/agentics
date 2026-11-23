package shellwords

import "strings"

// otherShells run text the POSIX parser does not model, so a call to one from a script is never inspected.
var otherShells = map[string]bool{"zsh": true, "dash": true, "ksh": true, "fish": true, "csh": true, "tcsh": true}

func isShellExe(exe string) bool { return exe == "sh" || exe == "bash" || otherShells[exe] }

// isScriptCallExe reports whether exe starts a script-call form, whose refusals carry a named script- reason.
func isScriptCallExe(exe string) bool {
	return exe == "sh" || exe == "bash" || exe == "source" || exe == "."
}

// scriptCallRefusal reports whether inv is a refused script call with a named reason, such as script-unreadable.
// Only those keep their reason; another reason that merely starts with script- is not trusted to be content-free.
func scriptCallRefusal(inv *Invocation) bool {
	return !inv.Inspectable && strings.HasPrefix(inv.Reason, "script-") && len(inv.Words) > 0 &&
		isScriptCallExe(strings.ToLower(ExecutableName(inv.Words[0].Text)))
}

// shellReadsStdin reports whether a shell invocation may take its commands from standard input: the segment follows
// a pipe in its list, the parser marked it as reading a pipeline, or its raw text holds a <, <>, << or <<<
// redirection. InputRedirects records only < and <>, so the raw segment is lexed again for heredocs and here-strings.
func shellReadsStdin(inv *Invocation) bool {
	if inv.Before == Pipe || inv.PipelineInput || len(inv.InputRedirects) > 0 {
		return true
	}
	return rawRedirectsStdin(inv.Raw, inv.Dialect)
}

// scriptConstruct names the construct on an inspectable child that hides the commands it runs, or "" when there is
// none. A literal source of a .sh file never reaches here: it was expanded, or refused with a named reason.
//
// builtin runs the builtin its next word names, and `builtin -- x` and `builtin builtin x` run x as well, so each
// leading builtin, and a -- after it, is stripped before the command word is judged, as the cwd replay does.
func scriptConstruct(inv *Invocation) string {
	words := stripBuiltin(inv.Words)
	if len(words) == 0 {
		return ""
	}
	first := words[0]
	if !first.Literal {
		return "command-position-expansion"
	}
	switch exe := strings.ToLower(ExecutableName(first.Text)); {
	case exe == "eval":
		return "eval"
	case exe == "trap":
		return "trap"
	case exe == "source" || exe == ".":
		return "source"
	case otherShells[exe]:
		return "other-shell"
	}
	return ""
}

// legacyConstruct maps a refusal reason written for typed commands to a content-free construct name. Those reasons
// may quote the command, which in a script is file content, so the reason itself is never kept.
func legacyConstruct(inv *Invocation) string {
	exe := ""
	if len(inv.Words) > 0 {
		exe = strings.ToLower(ExecutableName(inv.Words[0].Text))
	}
	switch {
	case strings.HasSuffix(inv.Reason, " command string is not literal"):
		return "dynamic-c"
	case strings.HasSuffix(inv.Reason, " child executable is not literal"):
		return "command-position-expansion"
	case strings.HasPrefix(inv.Reason, "wrapper depth exceeds "):
		return "nesting-depth"
	case isShellExe(exe) && shellReadsStdin(inv):
		return "stdin-to-shell"
	case otherShells[exe]:
		return "other-shell"
	}
	return "unsupported-syntax"
}

// markScriptOpacity labels the children from index start on, each with the location of its own origin. Children of a
// nested script were labeled when that script was expanded and already end in their location, so a child that does
// is left alone: that is what keeps the enclosing pass from relabeling them, and the pass idempotent. A refused
// script call keeps its named reason with the location appended; every other refusal, and every construct that
// hides commands, becomes script-opaque:<construct> with the location and never the script text.
func (p *parser) markScriptOpacity(start int) {
	for i := start; i < len(p.result.Invocations); i++ {
		inv := &p.result.Invocations[i]
		if inv.Script == nil || strings.HasSuffix(inv.Reason, " in "+inv.Script.label()) {
			continue
		}
		switch {
		case scriptCallRefusal(inv):
			inv.Reason = ScriptReason(inv.Reason, inv.Script)
		case !inv.Inspectable:
			inv.Reason = ScriptReason("script-opaque:"+legacyConstruct(inv), inv.Script)
		default:
			rule := scriptConstruct(inv)
			if rule == "" && !p.judged[i] {
				rule = pinnedConstruct(inv, p.ownResets(inv))
			}
			if p.judged == nil {
				p.judged = map[int]bool{}
			}
			p.judged[i] = true
			if rule != "" {
				inv.Inspectable = false
				inv.Reason = ScriptReason("script-opaque:"+rule, inv.Script)
			}
		}
	}
}
