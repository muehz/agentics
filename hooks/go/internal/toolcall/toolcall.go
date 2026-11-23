// Package toolcall reads one tool call from a normalized event: its name, its
// surface, and the layers its arguments arrive in.
//
// Claude Code hands a tool's input through verbatim, so the arguments sit at
// the root. Copilot and Codex wrap the provider's raw arguments as
// {"value": <raw arguments>} and promote only a few keys to the root. A read of
// the root alone therefore finds nothing for some tools on two of the three
// surfaces, which for a guard means passing a call it was built to stop.
//
// The package offers two readings of those layers.
//
// The lenient reading, Layers, is the root followed by value when value is an
// object. A value that is not an object adds no layer, and a value inside value
// is not followed. A reader that only looks for fields, and that treats a miss
// as unknown, uses it.
//
// The strict reading, ProofLayers, is the same two layers, but it reports ok
// false when value is present and is not an object, or when the value object
// itself holds a value key. No provider adds another envelope, so a malformed
// or deeper one cannot establish anything about how the tool will run. ok false
// is the distinct "not readable" result: it returns a nil slice, which a caller
// must not mistake for a call that has no arguments. Each consumer keeps its
// own posture for it:
//
//   - The execution context names the envelope fault and withholds proof.
//   - command-guardian's data proof and its empty-pager proof are withheld.
//   - command-guardian's alternate-config reading ignores the fault and uses
//     the lenient layers.
//
// A Call holds no memo and no lazily computed state, and no method writes, so
// it is safe to share and to read repeatedly. It holds the decoded input by
// reference, so callers must not write to the maps it returns.
//
// Every accessor visits each key and each list item a bounded number of times,
// so its cost is linear in the input a hook can receive. The one extra factor
// is the sort of an override container's entries by key in Environment, which
// is logarithmic in the entries of that container. Execution refuses an
// argument layer of more than maxEvidenceKeys keys before it walks the layer.
package toolcall

import (
	"github.com/muehz/agentics/hooks/go/internal/contract"
)

// Call is one tool call read from a normalized event. The zero Call has no
// name and no arguments.
type Call struct {
	name    string
	surface string
	// cwd is the event's working directory as the event carried it, unproven.
	cwd   string
	input map[string]any
}

// Read decodes the tool input of the event. It returns exactly the error
// event.Tool.DecodeInput returns, along with the zero Call, so each guard keeps
// its own posture for an input that cannot be decoded. A nil tool name or a nil
// event cwd reads as the empty string.
func Read(event contract.NeutralEvent) (Call, error) {
	input, err := event.Tool.DecodeInput()
	if err != nil {
		return Call{}, err
	}
	call := Call{surface: event.Surface, input: input}
	if event.Tool.Name != nil {
		call.name = *event.Tool.Name
	}
	if event.Cwd != nil {
		call.cwd = *event.Cwd
	}
	return call, nil
}

// Name returns the tool name, or the empty string when the event had none.
func (c Call) Name() string { return c.name }

// Surface returns the event's surface name exactly as the event carried it.
func (c Call) Surface() string { return c.surface }

// Input returns the decoded tool input, which is nil when the input was JSON
// null. The map is shared with the Call and never copied: callers must not
// write to it.
func (c Call) Input() map[string]any { return c.input }

// Command returns the root command argument. It is ok only when command is a
// JSON string, and an empty string counts. An absent, null, or non-string
// command returns "" and false. It never reads the value layer.
func (c Call) Command() (string, bool) {
	command, ok := c.input["command"].(string)
	return command, ok
}

// Layers returns the lenient reading: the root, then value when it is an
// object. Each call returns a new slice over the same maps, and the callers
// must not write to the maps.
func (c Call) Layers() []map[string]any {
	layers := []map[string]any{c.input}
	if nested, ok := c.input["value"].(map[string]any); ok {
		layers = append(layers, nested)
	}
	return layers
}

// ProofLayers returns the strict reading. It is the same layers as Layers, but
// ok is false, with a nil slice, when value is present and is not an object, or
// when the value object holds a value key. Absent and null are different here:
// a value that is present and null is not an object, so it is not readable.
func (c Call) ProofLayers() ([]map[string]any, bool) {
	if c.envelopeFault() != faultNone {
		return nil, false
	}
	return c.Layers(), true
}

// fault names why the strict reading of the layers failed.
type fault int

const (
	faultNone fault = iota
	// faultMalformed is a value that is present and is not an object.
	faultMalformed
	// faultDeep is a value object that holds a value key of its own.
	faultDeep
)

// envelopeFault reports which strict-reading fault applies, so a consumer can
// give each its own message.
func (c Call) envelopeFault() fault {
	raw, wrapped := c.input["value"]
	if !wrapped {
		return faultNone
	}
	nested, ok := raw.(map[string]any)
	if !ok {
		return faultMalformed
	}
	if _, deeper := nested["value"]; deeper {
		return faultDeep
	}
	return faultNone
}
