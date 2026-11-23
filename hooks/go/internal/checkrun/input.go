package checkrun

import (
	"context"
	"errors"
	"io"

	"github.com/muehz/agentics/hooks/go/internal/hookspec"
)

// maxPayloadBytes bounds what a hook reads from stdin, so a hostile or
// runaway writer cannot exhaust memory before any check runs.
const maxPayloadBytes int64 = 64 << 20

var errPayloadTooLarge = errors.New("hook payload exceeds 64 MiB limit")

// readPayload reads the whole payload, refusing one over maxPayloadBytes. It
// reads one byte past the bound so an exact-size payload still passes.
func readPayload(reader io.Reader) ([]byte, error) {
	limited := &io.LimitedReader{R: reader, N: maxPayloadBytes + 1}
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > maxPayloadBytes {
		return nil, errPayloadTooLarge
	}
	return payload, nil
}

// eventInput is what the input goroutine hands back.
type eventInput struct {
	event hookspec.Event
	fault fault
}

// readEvent reads, normalizes and reads back the event payload within the
// plan's input budget. It runs in its own goroutine so a stalled writer or a
// stalled normalize stage cannot hold the reply past the budget; on the
// deadline it closes stdin when it can, which ends a read blocked on it.
//
// The budget starts from a fresh context, not from the invocation's parent:
// the parent bounds the checks, and a payload that arrived in time is read
// whatever happens to the parent afterwards.
func readEvent(stages Stages, pl plan, surface, event string, stdin io.Reader) (hookspec.Event, fault) {
	ctx, cancel := context.WithTimeout(context.Background(), pl.inputBudget)
	defer cancel()
	done := make(chan eventInput, 1)
	go func() {
		result := eventInput{fault: noFault}
		defer func() {
			if recover() != nil {
				result = eventInput{fault: faultInputPanic}
			}
			done <- result
		}()
		payload, err := readPayload(stdin)
		if err != nil {
			result.fault = faultUnreadableInput
			return
		}
		normalized, malformed := stages.Normalize(surface, event, payload)
		if malformed != nil {
			result.fault = faultMalformedPayload
			return
		}
		if _, err := normalized.Read(); err != nil {
			result.fault = faultInvalidEvent
			return
		}
		result.event = normalized
	}()
	select {
	case result := <-done:
		if ctx.Err() != nil {
			return hookspec.Event{}, faultInputDeadline
		}
		return result.event, result.fault
	case <-ctx.Done():
		if closer, ok := stdin.(io.Closer); ok {
			_ = closer.Close()
		}
		return hookspec.Event{}, faultInputDeadline
	}
}
