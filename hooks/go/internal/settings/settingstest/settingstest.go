// Package settingstest is test support imported only by tests: a settings
// source that applies a fixed policy and counts how often it is asked, and the
// stripper that removes the settings note from a message. It imports only
// settings, and it is listed in digest-exclude because no hook binary imports
// it.
package settingstest

import (
	"strings"
	"sync/atomic"

	"github.com/muehz/agentics/hooks/go/internal/settings"
)

// Stub is a settings resolver with a fixed policy. It reads no file.
type Stub struct {
	source *settings.Source
	calls  atomic.Int64
}

// Policy returns a Stub whose Source resolves every directory to
// settings.Defaults with the given keys off.
func Policy(off ...settings.Key) *Stub {
	s := &Stub{}
	s.source = settings.NewSource(func(string) settings.Result {
		s.calls.Add(1)
		policy := settings.Defaults()
		for _, k := range off {
			policy[k] = false
		}
		return settings.Result{Policy: policy}
	})
	return s
}

// Source returns the source that serves the stub's policy. It calls the
// stub's resolver once per distinct directory, as any settings.Source does.
func (s *Stub) Source() *settings.Source { return s.source }

// Calls returns how many times the stub's resolver has run.
func (s *Stub) Calls() int { return int(s.calls.Load()) }

// WithoutNote returns message up to its settings note, so a test can pin a
// rule's own wording. A message with no note is returned unchanged.
func WithoutNote(message string) string {
	if i := strings.Index(message, "\n\nSetting"); i >= 0 {
		return message[:i]
	}
	return message
}
