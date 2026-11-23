// Package hookspectest is test support imported only by tests: an in-memory
// script reader and a context that cancels on a chosen call. It imports
// context and, from the module, only shellwords, and is listed in
// digest-exclude because no hook binary imports it.
package hookspectest

import (
	"context"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// Scripts is an in-memory shellwords.ScriptReader keyed by path.
type Scripts map[string]string

// ReadScript returns the text held for path with its byte count, and the
// script-unreadable refusal for a path the map does not hold.
func (s Scripts) ReadScript(path string) shellwords.ScriptRead {
	if text, ok := s[path]; ok {
		return shellwords.ScriptRead{Path: path, Text: text, Bytes: len(text)}
	}
	return shellwords.ScriptRead{Path: path, Limit: shellwords.RuleUnreadable}
}

// CancelContext wraps a context and reports it cancelled from the At-th call
// of Err onward, so a test can stop a long walk at a chosen check. At zero
// never cancels. Err has a pointer receiver because it counts calls in Calls.
type CancelContext struct {
	context.Context
	// Calls counts the Err calls made so far.
	Calls int
	// At is the 1-based call from which Err returns context.Canceled.
	At int
}

// Err counts the call and returns context.Canceled once Calls reaches At.
func (c *CancelContext) Err() error {
	c.Calls++
	if c.At > 0 && c.Calls >= c.At {
		return context.Canceled
	}
	return nil
}
