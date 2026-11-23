package toolcall

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// buildEvent returns a neutral event whose tool input is the given object.
func buildEvent(t *testing.T, input map[string]any) contract.NeutralEvent {
	t.Helper()
	event, err := contract.NewEvent(contract.EventParts{
		Surface:   surface.Codex,
		ToolName:  "Bash",
		ToolInput: input,
	})
	if err != nil {
		t.Fatal(err)
	}
	neutral, err := event.Read()
	if err != nil {
		t.Fatal(err)
	}
	return neutral
}

func obj(pairs ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i].(string)] = pairs[i+1]
	}
	return m
}

// TestReadingsOfArgumentLayers states both readings for each shape of the
// value envelope.
func TestReadingsOfArgumentLayers(t *testing.T) {
	root := func(value any) map[string]any { return obj("command", "ls", "value", value) }
	rootOnly := obj("command", "ls")

	tests := []struct {
		name       string
		input      map[string]any
		layers     []map[string]any
		proof      []map[string]any
		proofOK    bool
		proofFault fault
	}{
		{
			name: "root alone", input: rootOnly,
			layers: []map[string]any{rootOnly}, proof: []map[string]any{rootOnly}, proofOK: true,
		},
		{
			name: "value is an empty object", input: root(map[string]any{}),
			layers:  []map[string]any{root(map[string]any{}), {}},
			proof:   []map[string]any{root(map[string]any{}), {}},
			proofOK: true,
		},
		{
			name: "value is an object", input: root(obj("cwd", "/w")),
			layers:  []map[string]any{root(obj("cwd", "/w")), obj("cwd", "/w")},
			proof:   []map[string]any{root(obj("cwd", "/w")), obj("cwd", "/w")},
			proofOK: true,
		},
		{
			name: "value is a string", input: root("x"),
			layers: []map[string]any{root("x")}, proofFault: faultMalformed,
		},
		{
			name: "value is null", input: root(nil),
			layers: []map[string]any{root(nil)}, proofFault: faultMalformed,
		},
		{
			name: "value is a list", input: root([]any{"x"}),
			layers: []map[string]any{root([]any{"x"})}, proofFault: faultMalformed,
		},
		{
			name: "value is a number", input: root(json.Number("1")),
			layers: []map[string]any{root(json.Number("1"))}, proofFault: faultMalformed,
		},
		{
			name: "value holds a value", input: root(obj("value", "x")),
			layers:     []map[string]any{root(obj("value", "x")), obj("value", "x")},
			proofFault: faultDeep,
		},
		{
			name: "value holds a null value", input: root(obj("value", nil)),
			layers:     []map[string]any{root(obj("value", nil)), obj("value", nil)},
			proofFault: faultDeep,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call, err := Read(buildEvent(t, tt.input))
			if err != nil {
				t.Fatal(err)
			}
			if got := call.Layers(); !layersEqual(got, tt.layers) {
				t.Errorf("Layers() = %v, want %v", got, tt.layers)
			}
			proof, ok := call.ProofLayers()
			if ok != tt.proofOK {
				t.Fatalf("ProofLayers() ok = %v, want %v", ok, tt.proofOK)
			}
			if ok && !layersEqual(proof, tt.proof) {
				t.Errorf("ProofLayers() = %v, want %v", proof, tt.proof)
			}
			if !ok && proof != nil {
				t.Errorf("ProofLayers() = %v, want a nil slice when not readable", proof)
			}
			if got := call.envelopeFault(); got != tt.proofFault {
				t.Errorf("envelopeFault() = %v, want %v", got, tt.proofFault)
			}
		})
	}
}

func layersEqual(got, want []map[string]any) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		// The input is decoded from JSON, so numbers are float64 where the want
		// side holds json.Number; compare through their encoding.
		a, _ := json.Marshal(got[i])
		b, _ := json.Marshal(want[i])
		if string(a) != string(b) {
			return false
		}
	}
	return true
}

func TestCommandReadsOnlyTheRootString(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]any
		want   string
		wantOK bool
	}{
		{"string", obj("command", "ls -l"), "ls -l", true},
		{"empty string", obj("command", ""), "", true},
		{"absent", obj(), "", false},
		{"null", obj("command", nil), "", false},
		{"number", obj("command", json.Number("1")), "", false},
		{"list", obj("command", []any{"ls"}), "", false},
		{"only under value", obj("value", obj("command", "ls")), "", false},
		{"root wins over value", obj("command", "a", "value", obj("command", "b")), "a", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call, err := Read(buildEvent(t, tt.input))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := call.Command()
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("Command() = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestReadCarriesNameAndSurface(t *testing.T) {
	call, err := Read(buildEvent(t, obj("command", "ls")))
	if err != nil {
		t.Fatal(err)
	}
	if call.Name() != "Bash" || call.Surface() != "codex" {
		t.Fatalf("Name, Surface = %q, %q; want Bash, codex", call.Name(), call.Surface())
	}
	if !reflect.DeepEqual(call.Input(), map[string]any{"command": "ls"}) {
		t.Fatalf("Input() = %v", call.Input())
	}
}

func TestReadNilToolNameReadsEmpty(t *testing.T) {
	call, err := Read(contract.NeutralEvent{
		Surface: "claude-code",
		Tool:    contract.NeutralTool{InputRaw: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if call.Name() != "" || call.Surface() != "claude-code" {
		t.Fatalf("Name, Surface = %q, %q", call.Name(), call.Surface())
	}
}

func TestReadNullInputHasNoArguments(t *testing.T) {
	call, err := Read(contract.NeutralEvent{Tool: contract.NeutralTool{InputRaw: json.RawMessage(`null`)}})
	if err != nil {
		t.Fatal(err)
	}
	if call.Input() != nil {
		t.Fatalf("Input() = %v, want nil", call.Input())
	}
	if command, ok := call.Command(); command != "" || ok {
		t.Fatalf("Command() = %q, %v", command, ok)
	}
	if got := call.Layers(); len(got) != 1 || got[0] != nil {
		t.Fatalf("Layers() = %v, want the nil root alone", got)
	}
	if got, ok := call.ProofLayers(); !ok || len(got) != 1 {
		t.Fatalf("ProofLayers() = %v, %v", got, ok)
	}
}

// TestReadReturnsTheDecodeError pins that the guards see the decode error
// itself and receive the zero Call with it.
func TestReadReturnsTheDecodeError(t *testing.T) {
	for _, raw := range []string{`{`, `[1]`, `"x"`, `{"n":1e999}`, `{"n":` + strings.Repeat("9", 400) + `}`} {
		t.Run(raw[:min(len(raw), 12)], func(t *testing.T) {
			event := contract.NeutralEvent{
				Surface: "codex",
				Tool:    contract.NeutralTool{Name: new("Bash"), InputRaw: json.RawMessage(raw)},
			}
			_, want := event.Tool.DecodeInput()
			if want == nil {
				t.Fatal("fixture decodes cleanly; it must not")
			}
			call, err := Read(event)
			if err == nil || err.Error() != want.Error() {
				t.Fatalf("Read error = %v, want %v", err, want)
			}
			if call.Name() != "" || call.Surface() != "" || call.Input() != nil {
				t.Fatalf("Read returned a non-zero Call with an error: %+v", call)
			}
		})
	}
}

func TestLayersReturnsAFreshSliceOverTheSameMaps(t *testing.T) {
	call, err := Read(buildEvent(t, obj("value", obj("a", "b"))))
	if err != nil {
		t.Fatal(err)
	}
	first, second := call.Layers(), call.Layers()
	first[0] = nil
	if second[0] == nil {
		t.Fatal("two Layers calls share one slice")
	}
	if reflect.ValueOf(call.Layers()[1]).Pointer() != reflect.ValueOf(call.Layers()[1]).Pointer() {
		t.Fatal("Layers copies the value map")
	}
}
