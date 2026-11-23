package checkrun

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/provider"
)

type eventBlockingReader struct {
	closed chan struct{}
	once   sync.Once
}

func (r *eventBlockingReader) Read([]byte) (int, error) {
	<-r.closed
	return 0, io.ErrClosedPipe
}

func (r *eventBlockingReader) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

func TestEventDispatchBoundsInputAndDecode(t *testing.T) {
	for _, mode := range []string{"read", "normalize"} {
		t.Run(mode, func(t *testing.T) {
			stages := Production()
			blockedReader := &eventBlockingReader{closed: make(chan struct{})}
			t.Cleanup(func() { _ = blockedReader.Close() })
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			var reader io.Reader = blockedReader
			if mode == "normalize" {
				reader = strings.NewReader(`{}`)
				stages.Normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) {
					<-release
					return hookspec.Event{}, nil
				}
			}
			pl := eventPlan
			pl.inputBudget = 20 * time.Millisecond
			done := make(chan eventInput, 1)
			go func() {
				event, fault := readEvent(stages, pl, "claude-code", "PreToolUse", reader)
				done <- eventInput{event: event, fault: fault}
			}()
			select {
			case result := <-done:
				if result.event.Present() || result.fault != faultInputDeadline || pl.cells[result.fault].outcome("", nil, nil).Payload() != "agx: event payload receive/decode deadline exceeded" {
					t.Fatalf("deadline did not reject input: %+v", result)
				}
			case <-time.After(time.Second):
				t.Fatal("event input/decode exceeded its budget")
			}
			if mode == "read" {
				select {
				case <-blockedReader.closed:
				default:
					t.Fatal("deadline did not close the invocation-owned reader")
				}
			}
		})
	}
}
