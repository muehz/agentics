package shellwords

import "strings"

// leadingKeywords returns the reserved words that open a segment's text, in order. bash reads a word as a reserved
// word only when it is unquoted, spelled exactly, and stands where a command starts: first in the segment, or after
// another reserved word that leaves the command position open (if, then, elif, else, do, while, until, !, time and
// its -p). for, select and case take a name or a word next, so the run stops after them, and so it does after done,
// fi and esac. A word in quotes, in a comment (the splitter drops comments) or in an argument is never returned.
func leadingKeywords(raw string) []string {
	var out []string
	rest, afterTime := raw, false
	for {
		rest = strings.TrimLeft(rest, " \t\r\n")
		end := strings.IndexAny(rest, " \t\r\n;&|()<>")
		if end < 0 {
			end = len(rest)
		}
		w := rest[:end]
		rest = rest[end:]
		switch w {
		case "while", "until", "if":
			out = append(out, w)
		case "then", "elif", "else", "do", "!":
		case "time":
			afterTime = true
			continue
		case "-p":
			if !afterTime {
				return out
			}
		case "for", "select", "case", "done", "fi", "esac":
			return append(out, w)
		default:
			return out
		}
		afterTime = false
	}
}

// concurrencyJudge refuses typed writers that the position of a call does not order before it, segment by segment
// of one command list. A writer is refused, whatever it names, when it
//   - belongs to a substitution, process substitution or backtick in the call's own words, prefix or redirects (the
//     splitter marks every segment of one with the segment it belongs to), which finishes before the shell opens the
//     script;
//   - follows a call inside the same open loop, or in a list nested in a loop, so the call may run again after it;
//   - follows a call and an & in the same list, so it may run while the call does, until a plain `wait`;
//   - follows a call that reads a heredoc: the unquoted body is parsed on the lines after the call, where nothing ties
//     its substitutions to it.
//
// A command in the call's own pipeline runs while the call does. Such a command is refused when it writes files its
// words do not list (writesUnnamedFiles); one that names its targets is left to the write-target check, which runs on
// every typed command, so `| tee run.log` passes and `| tee ./ok.sh` does not. The pipeline is followed through its
// edges: from the call (or the group, compound command or command word whose substitution holds it) to each member, to
// a group or compound command that is a member, to commands in that member's substitutions, and on.
//
// A writer before a call is a predecessor and was refused before the script was read.
type concurrencyJudge struct {
	segs                  []rawSegment
	calls, owned, members []bool
	anyCall               bool
	// background is set by a & after a call and cleared by a plain wait; heredoc is never cleared.
	background, heredoc bool
	// waitPending carries a plain wait to the next segment, which clears background before it is judged. stateSeen
	// blocks that clearing once the list may have redefined wait.
	waitPending, stateSeen bool
	// inherited is true when the list is nested in a loop of an enclosing list.
	inherited bool
	// open holds the compound commands open at the segment: 'l' a loop, 'i' an if, 'c' a case.
	open  []byte
	loops int
	// callInLoop holds from a call inside a loop until the outermost loop closes; callInCompound likewise for any
	// compound command, whose closer then carries the pipeline the call's output enters.
	callInLoop, callInCompound bool
	// memberFrom is the depth of open at which a compound command that is a member of the call's pipeline began, 0
	// for none: every segment until it closes is a member.
	memberFrom int
	// closerTied marks the segment being judged as the closer of a compound command tied to the call's pipeline, and
	// pushed the depth of open after the first compound command the segment opened, 0 for none.
	closerTied bool
	pushed     int
	edges      map[int]bool
	groups     map[int]bool
}

func newConcurrencyJudge(segs []rawSegment, inherited bool) *concurrencyJudge {
	return &concurrencyJudge{segs: segs, calls: make([]bool, len(segs)), owned: make([]bool, len(segs)), members: make([]bool, len(segs)),
		inherited: inherited, edges: map[int]bool{}, groups: map[int]bool{}}
}

// enter reads the reserved words that open segment i, before it is parsed, and reports whether the segment runs in a
// loop, which a list nested in it inherits.
func (j *concurrencyJudge) enter(i int) bool {
	seg := j.segs[i]
	j.closerTied, j.pushed = false, 0
	if j.waitPending {
		// A & after the wait sets the flag again when this segment is judged, and plainWait refused a wait in a pipe.
		j.waitPending, j.background = false, false
	}
	for _, kw := range leadingKeywords(seg.raw) {
		switch kw {
		case "while", "until", "for", "select":
			j.push('l')
		case "if":
			j.push('i')
		case "case":
			j.push('c')
		case "done":
			j.pop('l')
		case "fi":
			j.pop('i')
		case "esac":
			j.pop('c')
		}
	}
	return j.inherited || j.loops > 0
}

func (j *concurrencyJudge) push(kind byte) {
	j.open = append(j.open, kind)
	if kind == 'l' {
		j.loops++
	}
	if j.pushed == 0 {
		j.pushed = len(j.open)
	}
}

// pop closes the innermost compound command when it is of the given kind. A closer of another kind is a case pattern
// such as `done)` after `;;`, which bash reads as a pattern, so it closes nothing. A pattern spelled for, while or
// until opens a loop that never closes, which only over-refuses.
func (j *concurrencyJudge) pop(kind byte) {
	n := len(j.open)
	if n == 0 || j.open[n-1] != kind {
		return
	}
	j.open = j.open[:n-1]
	if kind == 'l' {
		j.loops--
	}
	if j.callInCompound {
		j.closerTied = true
	}
	if j.memberFrom > 0 && len(j.open) < j.memberFrom {
		j.closerTied, j.memberFrom = true, 0
	}
	if len(j.open) == 0 {
		j.callInCompound = false
	}
	if j.loops == 0 && !j.inherited {
		j.callInLoop = false
	}
}

// tie adds the pipeline edges segment i feeds, and those of the command words whose substitutions hold it, to the
// call's pipeline, and remembers the groups it sits in.
func (j *concurrencyJudge) tie(i int) {
	for k := i; k >= 0 && k < len(j.segs); k = j.segs[k].owner {
		if e := j.segs[k].pipelineOutputEdge; e != 0 {
			j.edges[e] = true
		}
		if g := j.segs[k].groupID; g != 0 {
			j.groups[g] = true
		}
	}
}

// leave judges the invocations segment i produced, from index start on. depth is the number of wrappers of the list.
func (j *concurrencyJudge) leave(p *parser, i, start int, isCall bool, depth int) {
	seg := j.segs[i]
	if j.anyCall && seg.afterBackground {
		j.background = true
	}
	owned := seg.owner >= 0 && seg.owner < i && (j.calls[seg.owner] || j.owned[seg.owner])
	j.owned[i] = owned
	member := j.closerTied ||
		seg.pipelineInputEdge != 0 && j.edges[seg.pipelineInputEdge] ||
		seg.stderrPipeEdge != 0 && j.edges[seg.stderrPipeEdge] ||
		seg.owner >= 0 && seg.owner < i && j.members[seg.owner] ||
		j.memberFrom > 0 && len(j.open) >= j.memberFrom
	if !member && start == len(p.result.Invocations) && i > 0 && seg.pipelineOutputEdge != 0 {
		// A segment of redirects alone after a group closer (`{ bash x.sh; } 2>&1 | y`) carries the group's pipe.
		prev := j.segs[i-1]
		member = j.calls[i-1] || j.members[i-1] || prev.groupID != 0 && j.groups[prev.groupID]
	}
	if member {
		j.members[i] = true
		j.tie(i)
		if j.pushed > 0 && j.memberFrom == 0 {
			j.memberFrom = j.pushed
		}
	}
	// A segment that holds a call is judged before it counts as one: the typed commands of its -c string run with an
	// earlier call, or before the call whose substitution holds it, like any other command in that place.
	concurrent := j.anyCall && (j.background || j.heredoc || j.callInLoop)
	for k := start; k < len(p.result.Invocations); k++ {
		inv := &p.result.Invocations[k]
		if inv.Script != nil {
			continue
		}
		j.stateSeen = j.stateSeen || seg.funcDef || mayRedefineWait(inv)
		if !inv.Inspectable {
			continue
		}
		reason := RuleNone
		switch {
		case owned || concurrent:
			reason = predecessorProblem(inv)
		case member && writesUnnamedFiles(inv):
			reason = RulePredecessorMayWrite
		}
		if reason != RuleNone {
			inv.Inspectable, inv.Reason = false, ruleRefusal(reason, nil)
		}
	}
	if isCall {
		j.calls[i] = true
		j.anyCall = true
		j.heredoc = j.heredoc || hasHeredoc(seg.raw)
		j.callInLoop = j.callInLoop || j.inherited || j.loops > 0
		j.callInCompound = j.callInCompound || len(j.open) > 0
		j.tie(i)
		return
	}
	if j.background && !j.stateSeen && !member && !owned && len(j.open) == 0 && plainWait(p, seg, start, depth) {
		j.waitPending = true
	}
}

// mayRedefineWait reports a command after which a plain `wait` may not wait for the call: an alias, a sourced or
// evaluated text, a function definition or enable may replace the builtin, and disown takes jobs out of the table wait
// reads (bash 5.3: `sleep 2 & disown; wait` returns at once). Every disown counts, -h included, which only over-refuses,
// and so does a command word built at run time, which may be any of them.
func mayRedefineWait(inv *Invocation) bool {
	words := stripBuiltin(inv.Words)
	if len(words) == 0 {
		return false
	}
	if !words[0].Literal {
		return true
	}
	exe := strings.ToLower(ExecutableName(words[0].Text))
	return stateCommands[exe] || exe == "enable" || exe == "disown"
}

// plainWait reports a segment that is exactly the wait builtin without operands, run by the list's own shell in
// sequence: not in a group, substitution or pipeline, not after && or ||, with no wrapper or assignment. Such a wait
// returns only after every background job of the shell ended. `wait -n` and `wait PID` wait for one job, and a wait in
// a subshell waits for none of the list's jobs.
func plainWait(p *parser, seg rawSegment, start, depth int) bool {
	if seg.groupID != 0 || seg.owner >= 0 || seg.parentSegment >= 0 || seg.pipelineInputEdge != 0 || seg.pipelineOutputEdge != 0 ||
		seg.stderrPipeEdge != 0 || seg.before != Sequence && seg.before != Background || start+1 != len(p.result.Invocations) {
		return false
	}
	inv := p.result.Invocations[start]
	return inv.Dialect == POSIX && len(inv.Words) == 1 && inv.Words[0].Literal && inv.Words[0].Text == "wait" &&
		len(inv.Wrappers) == depth && len(inv.Assignments) == 0
}

// hasHeredoc reports a << that starts a heredoc rather than a here-string.
func hasHeredoc(raw string) bool {
	for i := 0; i+1 < len(raw); i++ {
		if raw[i] == '<' && raw[i+1] == '<' {
			if i+2 < len(raw) && raw[i+2] == '<' {
				i += 2
				continue
			}
			return true
		}
	}
	return false
}
