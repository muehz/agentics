package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// Event is a provider-normalized event with deferred handler validation. Its
// zero value is absent, distinct from a present event with null fields.
type Event struct {
	present bool
	value   NeutralEvent
	readErr error
}

// Present reports whether the provider produced an event.
func (e Event) Present() bool { return e.present }

// Read returns the normalized fields, or the handler-facing validation error.
func (e Event) Read() (NeutralEvent, error) {
	if !e.present || e.readErr != nil {
		return NeutralEvent{}, ErrEventInvalid
	}
	return e.value, nil
}

// EventParts are the typed pieces a provider adapter decodes from a payload.
// A nil pointer is JSON null, distinct from an empty string.
type EventParts struct {
	Surface                surface.ID
	SessionID, TurnID, Cwd *string
	Runtime                NeutralRuntime
	ToolName               string
	ToolInput              map[string]any // JSON-decoded, numbers as json.Number
	// NULOnly holds payload strings that no check reads. They are walked for
	// NUL only and are not kept in the event. Each adapter fixes the position
	// of every element and keeps nil elements, so a position always names the
	// same payload field.
	NULOnly []*string
}

// NewEvent validates typed provider parts and snapshots the fields handlers
// read. Each invalid UTF-8 byte in a string field becomes U+FFFD, and each
// string is copied, so a later write through the caller's pointer does not
// reach the event. Tool.Name is never nil. Only ToolInput is encoded, because
// its canonical text is itself observable: compact, HTML escaping off, number
// literals verbatim.
//
// A NUL anywhere in the parts but Surface does not fail construction: the
// event stays present and Read returns ErrEventInvalid. A single check
// consults its skip switch before reading the event, so the switch still
// wins, while the grouped event denies even with every skip switch set.
func NewEvent(parts EventParts) (Event, error) {
	surfaceName := string(parts.Surface)
	if !utf8.ValidString(surfaceName) {
		return Event{}, errors.New("contract: event field is not valid UTF-8")
	}
	if _, ok := surface.Parse(surfaceName); !ok {
		return Event{}, fmt.Errorf("contract: invalid normalized event surface %q", surfaceName)
	}
	if err := rejectNonFinite(parts.ToolInput); err != nil {
		return Event{}, err
	}
	if parts.ToolInput == nil {
		return Event{}, errors.New("contract: normalized event tool input must be an object")
	}
	raw, err := encode(parts.ToolInput)
	if err != nil {
		return Event{}, err
	}
	e := Event{present: true, value: NeutralEvent{
		Surface:   surfaceName,
		SessionID: snapshotString(parts.SessionID),
		TurnID:    snapshotString(parts.TurnID),
		Cwd:       snapshotString(parts.Cwd),
		Runtime: NeutralRuntime{
			PluginRoot:    snapshotString(parts.Runtime.PluginRoot),
			DataRoot:      snapshotString(parts.Runtime.DataRoot),
			VaultOverride: snapshotString(parts.Runtime.VaultOverride),
			LogRoot:       snapshotString(parts.Runtime.LogRoot),
		},
		Tool: NeutralTool{Name: snapshotString(&parts.ToolName), InputRaw: json.RawMessage(strings.TrimSuffix(raw, "\n"))},
	}}
	scalars := []*string{
		parts.SessionID, parts.TurnID, parts.Cwd,
		parts.Runtime.PluginRoot, parts.Runtime.DataRoot, parts.Runtime.VaultOverride, parts.Runtime.LogRoot,
		&parts.ToolName,
	}
	if anyHoldsNUL(scalars) || anyHoldsNUL(parts.NULOnly) || holdsNUL(parts.ToolInput) {
		e.readErr = ErrEventInvalid
	}
	return e, nil
}

// snapshotString copies a nullable string, replacing each invalid UTF-8 byte
// with U+FFFD. Rune conversion keeps one replacement per byte rather than
// collapsing adjacent invalid bytes.
func snapshotString(ptr *string) *string {
	if ptr == nil {
		return nil
	}
	s := *ptr
	if !utf8.ValidString(s) {
		s = string([]rune(s))
	}
	return &s
}

// anyHoldsNUL reports whether a non-nil element contains NUL.
func anyHoldsNUL(values []*string) bool {
	for _, ptr := range values {
		if ptr != nil && strings.ContainsRune(*ptr, 0) {
			return true
		}
	}
	return false
}
