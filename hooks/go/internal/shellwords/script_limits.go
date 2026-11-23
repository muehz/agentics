package shellwords

import (
	"sync/atomic"
	"time"
)

const (
	// maxScriptDepth is the number of scripts one call chain may hold: a call or source from the third nested script
	// is refused as script-depth.
	maxScriptDepth = 3
	// The prescan refuses text past these sizes before any lexing, because the lexer and the per-line passes are not
	// cancellable and their cost grows with the text.
	maxScriptLines   = 10000
	maxScriptTokens  = 100000
	maxScriptNesting = 64
	// maxLeakedForks bounds the child parses that outlived the deadline and still run: Go cannot stop a goroutine, so
	// each one keeps its memory and CPU until it finishes on its own. Past the bound a script call is refused at once.
	maxLeakedForks = 4
)

// scriptCeiling is the time one ParseScripts call may spend in script parses, shared by every script call it
// expands, so a typed command with several calls still finishes within one ceiling. A test shortens it.
var scriptCeiling = 2 * time.Second

// scriptParseHook runs in each forked child parse before it starts, with the child parser; a test makes it block to
// stand in for a slow parse, or keeps the child to see what an abandoned parse still holds. It is read once per
// ParseScripts call, on the caller's goroutine, so a test may reset it while an abandoned parse still runs.
var scriptParseHook = func(*parser) {}

// leakedForks counts the child parses that were abandoned at the deadline and have not finished yet.
var leakedForks atomic.Int32

const (
	forkRunning int32 = iota
	forkFinished
	forkAbandoned
)

// prescan refuses script text too large to parse within the budget, in one pass that ignores quotes and comments:
// counting brackets inside a string or a comment only makes it stricter. A ${ counts as an opening brace.
func prescan(text string) Rule {
	lines, tokens, nesting, deepest := 0, 0, 0, 0
	inToken := false
	for i := 0; i < len(text); i++ {
		switch c := text[i]; c {
		case ' ', '\t', '\n', '\v', '\f', '\r':
			inToken = false
			if c == '\n' {
				lines++
			}
			continue
		case '(', '{', '[':
			nesting++
			deepest = max(deepest, nesting)
		case ')', '}', ']':
			nesting = max(nesting-1, 0)
		}
		if !inToken {
			tokens++
			inToken = true
		}
	}
	if len(text) > 0 && text[len(text)-1] != '\n' {
		lines++ // a last line without a newline still counts
	}
	if lines > maxScriptLines || tokens > maxScriptTokens || deepest > maxScriptNesting {
		return RuleBudget
	}
	return RuleNone
}

// parseForked runs parse on a copy of the parser, on a goroutine of its own, and adopts the copy when parse returns
// before the parse deadline. It reports false, leaving p unchanged, when the deadline passes first, when too many
// abandoned parses still run, or when the copy panicked. An abandoned copy keeps running until it finishes, but it
// shares no state the caller writes (see fork), so the caller goes on undisturbed. parse must use only the parser it
// is given.
func (p *parser) parseForked(parse func(child *parser)) bool {
	if leakedForks.Load() >= maxLeakedForks {
		return false
	}
	child := p.fork()
	hook := p.script.parseHook
	var state atomic.Int32
	done := make(chan struct{})
	// failed is written before done is closed and read only after it, so it needs no lock.
	failed := false
	go func() {
		defer func() {
			if recover() != nil {
				failed = true
			}
			if !state.CompareAndSwap(forkRunning, forkFinished) {
				// The caller gave up on this parse and counted it; release the slot exactly once.
				leakedForks.Add(-1)
			}
			close(done)
		}()
		hook(child)
		parse(child)
	}()
	timer := time.NewTimer(time.Until(p.script.deadline))
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		// Count the slot before claiming it, so a late finisher that sees the abandoned state always has a slot to
		// release and the counter never goes below zero.
		leakedForks.Add(1)
		if state.CompareAndSwap(forkRunning, forkAbandoned) {
			return false
		}
		leakedForks.Add(-1)
		<-done // it finished in the instant the timer fired: adopt the result
	}
	if failed {
		return false
	}
	*p = *child
	return true
}
