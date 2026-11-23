package contract

import (
	"strings"
	"unicode/utf8"
)

// Outcome kinds distinguish hook decisions before provider rendering.
const (
	KindPass     = "pass"
	KindContinue = "continue"
	KindDeny     = "deny"
	KindContext  = "context"
	KindError    = "error"
)

// Outcome is one hook result. Only the five constructors create valid values;
// the zero value represents a handler that failed to produce an outcome.
type Outcome struct {
	kind    string
	payload string
}

// Kind identifies the decision independently of its provider rendering.
func (o Outcome) Kind() string { return o.kind }

// Payload is the deny reason or the context or error message.
// Pass and continue have no payload.
func (o Outcome) Payload() string { return o.payload }

// Valid reports whether a handler produced one of the five supported outcomes.
func (o Outcome) Valid() bool {
	switch o.kind {
	case KindPass, KindContinue:
		return o.payload == ""
	case KindDeny, KindContext, KindError:
		return true
	default:
		return false
	}
}

// Pass reports that the hook ran and raised no objection.
func Pass() Outcome { return Outcome{kind: KindPass} }

// Continue reports that the hook declined to act and the turn should proceed.
func Continue() Outcome { return Outcome{kind: KindContinue} }

// Deny blocks the tool call with a reason shown to the user.
func Deny(reason string) Outcome { return newOutcome(KindDeny, reason) }

// Context injects additional context into the turn.
func Context(message string) Outcome { return newOutcome(KindContext, message) }

// Error reports that the hook itself failed.
func Error(message string) Outcome { return newOutcome(KindError, message) }

func newOutcome(kind, payload string) Outcome {
	if !utf8.ValidString(payload) {
		// Match encoding/json: replace each invalid byte, not each run of bytes.
		var normalized strings.Builder
		normalized.Grow(len(payload))
		for _, r := range payload {
			normalized.WriteRune(r)
		}
		payload = normalized.String()
	}
	return Outcome{kind: kind, payload: payload}
}
