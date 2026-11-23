package shellwords

import (
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/argv"
)

// isShellExe reports whether exe is a POSIX shell or a shell no parser here models. A PowerShell host is neither: its
// text is read by the PowerShell parser.
func isShellExe(exe string) bool {
	shell := argv.Lookup(exe).Shell
	return shell == argv.PosixShell || shell == argv.OtherShell
}

// isScriptCallExe reports whether exe starts a script-call form, whose refusals carry a named script- reason.
func isScriptCallExe(exe string) bool {
	return exe == "sh" || exe == "bash" || exe == "source" || exe == "."
}

// refusedScriptCall reports whether inv is a refused script call under a named rule, such as script-unreadable. Only
// those keep their reason; any other refusal on a script child is relabeled, because its text may quote file content.
func refusedScriptCall(inv *Invocation) bool {
	return !inv.Inspectable && inv.Reason.Rule() != RuleNone && len(inv.Words) > 0 &&
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

// scriptConstruct names the construct on an inspectable child that hides the commands it runs, or constructNone when there is
// none. A literal source of a .sh file never reaches here: it was expanded, or refused with a named reason.
//
// builtin runs the builtin its next word names, and `builtin -- x` and `builtin builtin x` run x as well, so each
// leading builtin, and a -- after it, is stripped before the command word is judged, as the cwd replay does.
func scriptConstruct(inv *Invocation) construct {
	words := stripBuiltin(inv.Words)
	if len(words) == 0 {
		return constructNone
	}
	first := words[0]
	if !first.Literal {
		return constructCommandPositionExpansion
	}
	switch exe := strings.ToLower(ExecutableName(first.Text)); {
	case exe == "eval":
		return constructEval
	case exe == "trap":
		return constructTrap
	case exe == "source" || exe == ".":
		return constructSource
	case argv.Lookup(exe).Shell == argv.OtherShell:
		return constructOtherShell
	}
	return constructNone
}

// formConstruct maps a refusal written for typed commands to a content-free construct name. Those refusals may quote
// the command, which in a script is file content, so the refusal itself is never kept.
func formConstruct(inv *Invocation) construct {
	exe := ""
	if len(inv.Words) > 0 {
		exe = strings.ToLower(ExecutableName(inv.Words[0].Text))
	}
	switch {
	case inv.Reason.form == formCommandString:
		return constructDynamicC
	case inv.Reason.form == formChildNotLiteral:
		return constructCommandPositionExpansion
	case inv.Reason.form == formWrapperDepth:
		return constructNestingDepth
	case isShellExe(exe) && shellReadsStdin(inv):
		return constructStdinToShell
	case argv.Lookup(exe).Shell == argv.OtherShell:
		return constructOtherShell
	}
	return constructUnsupportedSyntax
}

// markScriptOpacity labels the children from index start on, each with the location of its own origin. Children of a
// nested script were labeled when that script was expanded and already carry their location, so a child that does
// is left alone: that is what keeps the enclosing pass from relabeling them, and the pass idempotent. A refused
// script call keeps its named reason with the location appended; every other refusal, and every construct that
// hides commands, becomes script-opaque:<construct> with the location and never the script text.
func (p *parser) markScriptOpacity(start int) {
	for i := start; i < len(p.result.Invocations); i++ {
		inv := &p.result.Invocations[i]
		if inv.Script == nil || inv.Reason.Origin() != nil {
			continue
		}
		switch {
		case refusedScriptCall(inv):
			inv.Reason = inv.Reason.locatedAt(inv.Script)
		case !inv.Inspectable:
			inv.Reason = opaqueRefusal(formConstruct(inv), inv.Script)
		default:
			rule := scriptConstruct(inv)
			if rule == constructNone && !p.judged[i] {
				rule = pinnedConstruct(inv, p.ownResets(inv))
			}
			if p.judged == nil {
				p.judged = map[int]bool{}
			}
			p.judged[i] = true
			if rule != constructNone {
				inv.Inspectable = false
				inv.Reason = opaqueRefusal(rule, inv.Script)
			}
		}
	}
}
