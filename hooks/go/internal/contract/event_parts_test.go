package contract

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// parts returns an all-null claude-code value with an empty tool input.
func parts(t *testing.T) EventParts {
	t.Helper()
	return EventParts{Surface: surface.ClaudeCode, ToolInput: map[string]any{}}
}

func readParts(t *testing.T, p EventParts) NeutralEvent {
	t.Helper()
	event, err := NewEvent(p)
	if err != nil {
		t.Fatal(err)
	}
	if !event.Present() {
		t.Fatal("constructed event absent")
	}
	got, err := event.Read()
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestNewEventAcceptsEverySurface(t *testing.T) {
	for _, id := range surface.All() {
		p := parts(t)
		p.Surface = id
		if got := readParts(t, p); got.Surface != string(id) {
			t.Fatalf("Surface = %q, want %q", got.Surface, id)
		}
	}
}

func TestNewEventRefusesSurface(t *testing.T) {
	for _, tc := range []struct {
		surface surface.ID
		want    string
	}{
		{"", `contract: invalid normalized event surface ""`},
		{"unknown", `contract: invalid normalized event surface "unknown"`},
		{"copilot-cli", `contract: invalid normalized event surface "copilot-cli"`},
		{"codex\x00", `contract: invalid normalized event surface "codex\x00"`},
		{"codex\xff", "contract: event field is not valid UTF-8"},
	} {
		t.Run(string(tc.surface), func(t *testing.T) {
			p := parts(t)
			p.Surface = tc.surface
			// A non-finite input shows the surface is checked first.
			p.ToolInput = map[string]any{"n": json.Number("1e999")}
			event, err := NewEvent(p)
			if err == nil || err.Error() != tc.want || event.Present() {
				t.Fatalf("construction = %#v, %v; want %s", event, err, tc.want)
			}
		})
	}
}

func TestNewEventRefusesToolInput(t *testing.T) {
	cases := map[string]struct {
		input map[string]any
		want  string
	}{
		"nonfinite_in_array": {map[string]any{"n": []any{json.Number("1e999")}}, "contract: non-finite JSON constant: 1e999"},
		"nonfinite_in_key":   {map[string]any{"a": map[string]any{"b": json.Number("-1e999")}}, "contract: non-finite JSON constant: -1e999"},
		"undecoded_type":     {map[string]any{"n": int(1)}, "contract: unexpected decoded JSON type int"},
		"nil":                {nil, "contract: normalized event tool input must be an object"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := parts(t)
			p.ToolInput = tc.input
			event, err := NewEvent(p)
			if err == nil || err.Error() != tc.want || event.Present() {
				t.Fatalf("construction = %#v, %v; want %s", event, err, tc.want)
			}
		})
	}
}

func TestNewEventKeepsEncoderError(t *testing.T) {
	input := map[string]any{"n": json.Number("abc")}
	_, want := encode(input)
	if want == nil {
		t.Fatal("encoder accepted an invalid number")
	}
	p := parts(t)
	p.ToolInput = input
	event, err := NewEvent(p)
	if err == nil || err.Error() != want.Error() || event.Present() {
		t.Fatalf("construction = %#v, %v; want %v", event, err, want)
	}
}

func TestNewEventPreservesCanonicalInput(t *testing.T) {
	p := parts(t)
	p.Surface = surface.Codex
	p.ToolInput = map[string]any{"z": json.Number("1.00e+2"), "command": "echo <a> && b\u2028\u2029"}
	got := readParts(t, p)
	// HTML characters stay unescaped; the encoder always escapes U+2028 and
	// U+2029, so the text holds their escape sequences.
	want := `{"command":"echo <a> && b\u2028\u2029","z":1.00e+2}`
	if string(got.Tool.InputRaw) != want {
		t.Fatalf("InputRaw = %s, want %s", got.Tool.InputRaw, want)
	}
	input, err := got.Tool.DecodeInput()
	if err != nil || !reflect.DeepEqual(input["z"], float64(100)) {
		t.Fatalf("DecodeInput = %#v, %v", input, err)
	}
}

func TestNewEventReplacesInvalidUTF8(t *testing.T) {
	p := parts(t)
	text := "a\xff\xfez"
	p.SessionID, p.TurnID, p.Cwd = &text, &text, &text
	p.Runtime = NeutralRuntime{PluginRoot: &text, DataRoot: &text, VaultOverride: &text, LogRoot: &text}
	p.ToolName = text
	got := readParts(t, p)
	for _, ptr := range []*string{
		got.SessionID, got.TurnID, got.Cwd,
		got.Runtime.PluginRoot, got.Runtime.DataRoot, got.Runtime.VaultOverride, got.Runtime.LogRoot,
		got.Tool.Name,
	} {
		if ptr == nil || *ptr != "a\ufffd\ufffdz" {
			t.Fatalf("normalized string = %v", ptr)
		}
	}
}

func TestNewEventCopiesStrings(t *testing.T) {
	p := parts(t)
	session, turn, cwd, plugin, data, vault, log := "s", "t", "c", "p", "d", "v", "l"
	p.SessionID, p.TurnID, p.Cwd = &session, &turn, &cwd
	p.Runtime = NeutralRuntime{PluginRoot: &plugin, DataRoot: &data, VaultOverride: &vault, LogRoot: &log}
	p.ToolInput = map[string]any{"a": "b"}
	event, err := NewEvent(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, ptr := range []*string{&session, &turn, &cwd, &plugin, &data, &vault, &log} {
		*ptr = "changed\x00"
	}
	p.ToolInput["new"] = "changed"
	got, err := event.Read()
	if err != nil {
		t.Fatal(err)
	}
	if *got.SessionID != "s" || *got.TurnID != "t" || *got.Cwd != "c" ||
		*got.Runtime.PluginRoot != "p" || *got.Runtime.DataRoot != "d" ||
		*got.Runtime.VaultOverride != "v" || *got.Runtime.LogRoot != "l" ||
		string(got.Tool.InputRaw) != `{"a":"b"}` {
		t.Fatalf("caller writes reached the event: %#v", got)
	}
}

func TestNewEventPreservesNullAndEmpty(t *testing.T) {
	p := parts(t)
	empty := ""
	p.SessionID = &empty
	p.Runtime.PluginRoot = &empty
	got := readParts(t, p)
	if got.Surface != "claude-code" || got.SessionID == nil || *got.SessionID != "" || got.TurnID != nil || got.Cwd != nil {
		t.Fatalf("nullable IDs = %#v", got)
	}
	if got.Runtime.PluginRoot == nil || *got.Runtime.PluginRoot != "" ||
		got.Runtime.DataRoot != nil || got.Runtime.VaultOverride != nil || got.Runtime.LogRoot != nil {
		t.Fatalf("runtime = %#v", got.Runtime)
	}
	// The tool name is a plain string, so the event never reports it as null.
	if got.Tool.Name == nil || *got.Tool.Name != "" || string(got.Tool.InputRaw) != "{}" {
		t.Fatalf("tool = %#v", got.Tool)
	}
}

func TestNewEventDefersNULToRead(t *testing.T) {
	nul := func() *string { s := "a\x00b"; return &s }
	benign := "x"
	cases := map[string]func(*EventParts){
		"session":         func(p *EventParts) { p.SessionID = nul() },
		"turn":            func(p *EventParts) { p.TurnID = nul() },
		"cwd":             func(p *EventParts) { p.Cwd = nul() },
		"tool_name":       func(p *EventParts) { p.ToolName = "\x00" },
		"plugin_root":     func(p *EventParts) { p.Runtime.PluginRoot = nul() },
		"data_root":       func(p *EventParts) { p.Runtime.DataRoot = nul() },
		"vault_override":  func(p *EventParts) { p.Runtime.VaultOverride = nul() },
		"log_root":        func(p *EventParts) { p.Runtime.LogRoot = nul() },
		"input_key":       func(p *EventParts) { p.ToolInput = map[string]any{"a\x00b": nil} },
		"input_value":     func(p *EventParts) { p.ToolInput = map[string]any{"a": "\x00"} },
		"nested_input":    func(p *EventParts) { p.ToolInput = map[string]any{"a": []any{map[string]any{"b": "\x00"}}} },
		"nul_only_first":  func(p *EventParts) { p.NULOnly = []*string{nul()} },
		"nul_only_second": func(p *EventParts) { p.NULOnly = []*string{nil, nul()} },
		"nul_only_after":  func(p *EventParts) { p.NULOnly = []*string{&benign, nil, nul()} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := parts(t)
			mutate(&p)
			event, err := NewEvent(p)
			if err != nil || !event.Present() {
				t.Fatalf("construction = %#v, %v", event, err)
			}
			got, err := event.Read()
			if !errors.Is(err, ErrEventInvalid) || !reflect.DeepEqual(got, NeutralEvent{}) {
				t.Fatalf("Read = %#v, %v", got, err)
			}
		})
	}
}

func TestNewEventAcceptsNilNULOnlyElements(t *testing.T) {
	benign := "x"
	want := readParts(t, parts(t))
	for name, list := range map[string][]*string{
		"nil_slice":     nil,
		"empty_slice":   {},
		"nil_element":   {nil},
		"mixed":         {nil, &benign},
		"benign_values": {&benign, &benign, &benign},
	} {
		t.Run(name, func(t *testing.T) {
			p := parts(t)
			p.NULOnly = list
			// The list is walked for NUL only, so the event equals one built
			// without it.
			if got := readParts(t, p); !reflect.DeepEqual(got, want) {
				t.Fatalf("Read = %#v, want %#v", got, want)
			}
		})
	}
}

func TestNewEventPreservesLazyInputDecoding(t *testing.T) {
	p := parts(t)
	huge := "1" + strings.Repeat("0", 400)
	p.ToolInput = map[string]any{"huge": json.Number(huge)}
	got := readParts(t, p)
	if string(got.Tool.InputRaw) != `{"huge":`+huge+`}` {
		t.Fatalf("integer literal changed: %s", got.Tool.InputRaw)
	}
	if _, err := got.Tool.DecodeInput(); err == nil {
		t.Fatal("huge integer must fail only when decoded as float64")
	}
}

func TestNewEventAcceptsSpelledOutNULEscape(t *testing.T) {
	p := parts(t)
	command := `printf '\u0000'`
	p.ToolInput = map[string]any{"command": command}
	input, err := readParts(t, p).Tool.DecodeInput()
	if err != nil || input["command"] != command {
		t.Fatalf("DecodeInput = %#v, %v", input, err)
	}
}

// The walks are linear in the input, so a large payload is the hostile-input
// bound as well as the truncation check.
func TestNewEventLargeInputIsNotTruncated(t *testing.T) {
	p := parts(t)
	large := strings.Repeat("x", 1100000)
	p.ToolInput = map[string]any{"payload": large}
	input, err := readParts(t, p).Tool.DecodeInput()
	if err != nil || input["payload"] != large {
		t.Fatalf("large input truncated or rejected: %v", err)
	}
}

func TestNewEventZeroIsAbsent(t *testing.T) {
	event, err := NewEvent(EventParts{})
	if err == nil || err.Error() != `contract: invalid normalized event surface ""` || event.Present() {
		t.Fatalf("zero parts = %#v, %v", event, err)
	}
	got, err := event.Read()
	if !errors.Is(err, ErrEventInvalid) || !reflect.DeepEqual(got, NeutralEvent{}) {
		t.Fatalf("Read = %#v, %v", got, err)
	}
}
