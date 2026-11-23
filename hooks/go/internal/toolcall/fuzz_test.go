package toolcall_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/surface"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

// The external test package keeps these tests on the exported interface, which
// is all a guard sees.

// quietReader is a script reader that reads nothing. ScriptOptions only
// carries it, so a test can tell an enabled result from the zero value.
type quietReader struct{}

func (quietReader) ReadScript(string) shellwords.ScriptRead { return shellwords.ScriptRead{} }

// seedRow is one corpus entry: the surface index, tool name, cwd, and tool
// input text FuzzCall takes.
type seedRow struct {
	surface int
	tool    string
	cwd     string
	input   string
}

// The rosters below are the rows of the package's four tables: path keys,
// content keys, list keys, and the old and new pairing. The write-tool names
// and the whole-file keys are rows of the same tables' companions. The package
// keeps them unexported, so a row added there needs a seed added here;
// TestSeedRowsAreRead fails when a seed no longer reaches the row it names.
var (
	seedPathKeys    = []string{"file_path", "path", "filePath", "notebook_path", "file"}
	seedContentKeys = []string{"content", "new_string", "new_str", "new_source", "file_text"}
	seedListKeys    = []string{"edits", "files"}
	seedPairs       = [][2]string{{"new_string", "old_string"}, {"new_str", "old_str"}}
	seedWriteTools  = []string{"Write", "Edit", "MultiEdit", "NotebookEdit"}
)

func quote(text string) string {
	raw, _ := json.Marshal(text)
	return string(raw)
}

// seedRows returns one corpus entry for every table row, followed by entries
// for each shape of the value envelope, the override containers, the command
// evidence keys, and the working-directory arguments.
func seedRows() []seedRow {
	const cwd = "/work"
	var rows []seedRow
	for i, key := range seedPathKeys {
		rows = append(rows, seedRow{i % 4, "Write", cwd, fmt.Sprintf(`{%s:"/work/a.txt"}`, quote(key))})
	}
	for i, key := range seedContentKeys {
		rows = append(rows, seedRow{i % 4, "Write", cwd, fmt.Sprintf(`{"path":"/work/a.txt",%s:"text"}`, quote(key))})
	}
	for i, key := range seedListKeys {
		rows = append(rows, seedRow{i % 4, "MultiEdit", cwd,
			fmt.Sprintf(`{"path":"/work/a.txt",%s:[{"path":"/work/b.txt","content":"one"},{"new_string":"two"},"skip",7]}`, quote(key))})
	}
	for i, pair := range seedPairs {
		rows = append(rows, seedRow{i, "Edit", cwd,
			fmt.Sprintf(`{"path":"/work/a.txt",%s:"new",%s:"old"}`, quote(pair[0]), quote(pair[1]))})
	}
	rows = append(rows,
		seedRow{1, "Edit", cwd, `{"path":"/work/a.txt","command":"insert","insert_line":null,"new_str":"n","old_str":"o"}`},
		seedRow{1, "Edit", cwd, `{"path":"/work/a.txt","command":5,"new_string":"n","old_string":"o"}`},
		seedRow{0, "Bash", cwd, `{"command":"ls","write_parse_error":true}`},
	)
	for i, name := range seedWriteTools {
		rows = append(rows, seedRow{i, name, cwd, `{"file_path":"/work/a.txt","content":"x"}`})
	}
	rows = append(rows,
		// Value envelopes: object, string, null, list, a value inside value, and a root path repeated in it.
		seedRow{3, "Bash", cwd, `{"command":"ls","value":{"command":"ls","cwd":"/work"}}`},
		seedRow{2, "Bash", cwd, `{"command":"ls","value":"x"}`},
		seedRow{2, "Bash", cwd, `{"command":"ls","value":null}`},
		seedRow{3, "Bash", cwd, `{"command":"ls","value":["x"]}`},
		seedRow{3, "Bash", cwd, `{"command":"ls","value":{"value":{"command":"ls"}}}`},
		seedRow{3, "Write", cwd, `{"path":"/work/a.txt","value":{"path":"/work/a.txt","content":"x"}}`},
		// Command evidence keys the providers add beside command.
		seedRow{3, "Bash", cwd, `{"command":"ls","value":{"cmd":"ls","command":"ls"}}`},
		seedRow{3, "Bash", cwd, `{"command":"ls","value":{"cmd":"pwd"}}`},
		seedRow{0, "Bash", cwd, `{"command":"ls","value":{"CommandLine":"ls"}}`},
		seedRow{0, "PowerShell", cwd, `{"command":"ls","value":{"CommandLine":5}}`},
		seedRow{1, "Bash", cwd, `{"command":"ls","shell":"zsh"}`},
		seedRow{1, "Bash", cwd, `{"command":"ls","options":{"a":1}}`},
		seedRow{1, "Bash", cwd, `{"command":5}`},
		seedRow{1, "Bash", cwd, `{}`},
		// Override containers: canonical, null, variant case, both names, non-string values, and a name's other case.
		seedRow{1, "Bash", cwd, `{"command":"ls","env":{"PATH":"/bin","CDPATH":"/x"}}`},
		seedRow{1, "Bash", cwd, `{"command":"ls","env":null}`},
		seedRow{1, "Bash", cwd, `{"command":"ls","environment":"x"}`},
		seedRow{1, "Bash", cwd, `{"command":"ls","ENV":{"RIPGREP_CONFIG_PATH":"/c"}}`},
		seedRow{1, "Bash", cwd, `{"command":"ls","env":{"A":"1"},"environment":{"B":"2"}}`},
		seedRow{1, "Bash", cwd, `{"command":"ls","env":{"RIPGREP_CONFIG_PATH":null,"globignore":"*"}}`},
		seedRow{3, "Bash", cwd, `{"command":"ls","value":{"Environment":{"BASHOPTS":"x"}}}`},
		// Working-directory arguments: agreeing, disagreeing, malformed, and a relative event cwd.
		seedRow{1, "Bash", cwd, `{"command":"ls","workdir":"/work","cwd":"/work"}`},
		seedRow{1, "Bash", cwd, `{"command":"ls","cwd":"/elsewhere"}`},
		seedRow{1, "Bash", cwd, `{"command":"ls","workdir":5}`},
		seedRow{1, "Bash", "work", `{"command":"ls"}`},
		seedRow{1, "Bash", "", `{"command":"ls"}`},
		seedRow{1, "Bash", `C:\work`, `{"command":"ls","cwd":"C:/work"}`},
	)
	// Inspection: a PowerShell here-string whose data view is proven, and the
	// script call under each working-directory and execution-context shape.
	rows = append(rows,
		seedRow{1, "PowerShell", "/ws", fmt.Sprintf(`{"command":%s}`, quote("@'\ngh stack submit; bash t.sh\n'@ | Set-Content -LiteralPath x"))},
		seedRow{1, "Bash", "/ws", `{"command":"bash t.sh"}`},
		seedRow{1, "Bash", "/ws", `{"command":"bash t.sh","cwd":"/other"}`},
		seedRow{1, "Bash", "/ws", `{"command":"bash t.sh","workdir":"/other"}`},
		seedRow{1, "Bash", "ws", `{"command":"bash t.sh"}`},
		seedRow{1, "Bash", "", `{"command":"bash t.sh"}`},
		seedRow{1, "powershell", "/ws", `{"command":"bash t.sh"}`},
		seedRow{2, "Bash", "/ws", `{"command":"bash t.sh","value":"x"}`},
	)
	return rows
}

// newCall builds the event FuzzCall and its seeds start from. It reports false
// for an input that cannot become a readable event, which is not the fuzz
// target's concern.
func newCall(surfaceIndex int, tool, cwd, input string) (toolcall.Call, bool) {
	all := surface.All()
	index := surfaceIndex % len(all)
	if index < 0 {
		index += len(all)
	}
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	var object map[string]any
	if decoder.Decode(&object) != nil {
		return toolcall.Call{}, false
	}
	event, err := contract.NewEvent(contract.EventParts{
		Surface:   all[index],
		Cwd:       &cwd,
		ToolName:  tool,
		ToolInput: object,
	})
	if err != nil {
		return toolcall.Call{}, false
	}
	neutral, err := event.Read()
	if err != nil {
		return toolcall.Call{}, false
	}
	call, err := toolcall.Read(neutral)
	if err != nil {
		return toolcall.Call{}, false
	}
	return call, true
}

// TestSeedRowsAreRead makes the seeds earn their place: each table row's seed
// must produce the write view that row exists to produce.
func TestSeedRowsAreRead(t *testing.T) {
	for _, row := range seedRows() {
		if _, ok := newCall(row.surface, row.tool, row.cwd, row.input); !ok {
			t.Errorf("seed does not build an event: %+v", row)
		}
	}
	read := func(tool, input string) toolcall.Writes {
		t.Helper()
		call, ok := newCall(1, tool, "/work", input)
		if !ok {
			t.Fatalf("no call for %s", input)
		}
		return call.Writes()
	}
	for _, key := range seedPathKeys {
		got := read("Write", fmt.Sprintf(`{%s:"/work/a.txt"}`, quote(key)))
		if !reflect.DeepEqual(got.Targets, []string{"/work/a.txt"}) {
			t.Errorf("path key %s: targets %v", key, got.Targets)
		}
	}
	for _, key := range seedContentKeys {
		got := read("Write", fmt.Sprintf(`{"path":"/work/a.txt",%s:"text"}`, quote(key)))
		want := []toolcall.WrittenText{{Path: "/work/a.txt", Text: "text"}}
		if !reflect.DeepEqual(got.Texts, want) {
			t.Errorf("content key %s: texts %v", key, got.Texts)
		}
	}
	for _, key := range seedListKeys {
		got := read("MultiEdit", fmt.Sprintf(`{"path":"/work/a.txt",%s:[{"path":"/work/b.txt","content":"one"},{"new_string":"two"},"skip",7]}`, quote(key)))
		want := []toolcall.WrittenText{{Path: "/work/b.txt", Text: "one"}, {Path: "/work/a.txt", Text: "two"}}
		if !reflect.DeepEqual(got.Texts, want) {
			t.Errorf("list key %s: texts %v", key, got.Texts)
		}
	}
	for _, pair := range seedPairs {
		got := read("Edit", fmt.Sprintf(`{"path":"/work/a.txt",%s:"new",%s:"old"}`, quote(pair[0]), quote(pair[1])))
		want := []toolcall.WrittenText{{Path: "/work/a.txt", Text: "new", Old: "old"}}
		if !reflect.DeepEqual(got.Texts, want) {
			t.Errorf("pair %v: texts %v", pair, got.Texts)
		}
	}
	for _, name := range seedWriteTools {
		if !toolcall.IsFileWrite(name) {
			t.Errorf("%s is not a file-write tool", name)
		}
	}
}

// checkCall calls every accessor of call and returns the first broken
// invariant, or "" when none is broken. A panic in an accessor fails the
// calling test by itself.
func checkCall(call toolcall.Call) string {
	call.Name()
	call.Surface()
	call.Input()
	command, commandOK := call.Command()
	_ = command
	layers := call.Layers()
	proof, proofOK := call.ProofLayers()
	if proofOK && !reflect.DeepEqual(proof, layers) {
		return "ProofLayers is ok but differs from Layers"
	}
	if !proofOK && proof != nil {
		return "ProofLayers is not ok but returned layers"
	}
	environment := call.Environment()
	execution := call.Execution()
	if execution.Proven {
		switch {
		case !proofOK:
			return "execution is proven without readable layers"
		case !commandOK:
			return "execution is proven without a command"
		case environment.Present():
			return "execution is proven beside an override container"
		}
	}
	names := environment.Names()
	if environment.Canonical() {
		for _, name := range names {
			if _, uncertain := environment.Values(name); uncertain {
				return fmt.Sprintf("canonical overrides read %q as uncertain", name)
			}
		}
	}
	environment.Values("")
	writes := call.Writes()
	seen := map[string]bool{}
	for _, target := range writes.Targets {
		if seen[target] {
			return fmt.Sprintf("target %q repeats", target)
		}
		seen[target] = true
	}
	call.PathArguments()
	cwd := call.ProvenCwd()
	if options := call.ScriptOptions(nil); !reflect.DeepEqual(options, shellwords.ScriptOptions{}) {
		return "ScriptOptions(nil) is not the zero value"
	}
	options := call.ScriptOptions(quietReader{})
	if options.Reader == nil || options.Cwd != cwd || !reflect.DeepEqual(options.Context, execution) {
		return "ScriptOptions with a reader does not carry the execution and the proven directory"
	}
	toolcall.IsFileWrite(call.Name())
	return checkInspection(call, command, commandOK)
}

// checkInspection inspects call with no reader, an empty reader, and a reader
// holding the script the seeds name, and returns the first broken invariant of
// the Inspection, or "" when none is broken. command and commandOK are what
// call.Command returned.
func checkInspection(call toolcall.Call, command string, commandOK bool) string {
	ctx := context.Background()
	execution := call.Execution()
	for name, reader := range map[string]shellwords.ScriptReader{
		"nil reader":    nil,
		"empty reader":  hookspectest.Scripts{},
		"script reader": hookspectest.Scripts{"/ws/t.sh": "echo hi\n"},
	} {
		inspection := call.Inspect(ctx, reader)
		if !inspection.DataProven && !reflect.DeepEqual(inspection.Policy, inspection.Source) {
			return name + ": the policy differs from the source without a proven data view"
		}
		if !inspection.DataProven && inspection.DataView != command {
			return fmt.Sprintf("%s: an unproven data view is %q, not the command %q", name, inspection.DataView, command)
		}
		if !commandOK && (inspection.DataProven || !reflect.DeepEqual(inspection.Source, shellwords.Result{})) {
			return name + ": a call without a command was parsed"
		}
		for index := -1; index <= len(inspection.Policy.Invocations); index++ {
			inspection.Refusal(ctx, index)
			if !reflect.DeepEqual(inspection.Authority(index), execution.ForInvocation(inspection.Policy, index)) {
				return fmt.Sprintf("%s: authority of invocation %d differs from the execution context narrowed to it", name, index)
			}
		}
	}
	return ""
}

// FuzzCall reads arbitrary tool input through every accessor of a Call and
// checks the invariants the guards lean on, so a misread cannot slip past all
// of them at once.
func FuzzCall(f *testing.F) {
	for _, row := range seedRows() {
		f.Add(row.surface, row.tool, row.cwd, row.input)
	}
	f.Fuzz(func(t *testing.T, surfaceIndex int, tool, cwd, input string) {
		call, ok := newCall(surfaceIndex, tool, cwd, input)
		if !ok {
			t.Skip()
		}
		if broken := checkCall(call); broken != "" {
			t.Fatalf("%s: surface %d tool %q cwd %q input %q", broken, surfaceIndex, tool, cwd, input)
		}
	})
}

// TestCallLargeInput reads an input of about 100 KB, an env container of 4,000
// entries and 2,000 edits, through every accessor once and checks what each
// returns.
func TestCallLargeInput(t *testing.T) {
	const entries, edits = 4000, 2000
	var builder strings.Builder
	builder.WriteString(`{"command":"ls","path":"/work/big.txt","env":{`)
	for i := range entries {
		if i > 0 {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, `"K%04d":"v"`, i)
	}
	builder.WriteString(`},"edits":[`)
	for i := range edits {
		if i > 0 {
			builder.WriteByte(',')
		}
		fmt.Fprintf(&builder, `{"new_str":"t%04d","old_str":"o"}`, i)
	}
	builder.WriteString(`]}`)
	input := builder.String()
	if len(input) < 90_000 || len(input) > 130_000 {
		t.Fatalf("input is %d bytes, want about 100 KB", len(input))
	}

	call, ok := newCall(1, "Bash", "/work", input)
	if !ok {
		t.Fatal("the large input does not build a call")
	}
	if broken := checkCall(call); broken != "" {
		t.Fatal(broken)
	}

	if call.Name() != "Bash" || call.Surface() != string(surface.ClaudeCode) {
		t.Errorf("name %q surface %q", call.Name(), call.Surface())
	}
	if got := len(call.Input()); got != 4 {
		t.Errorf("input has %d keys, want 4", got)
	}
	if command, ok := call.Command(); !ok || command != "ls" {
		t.Errorf("command %q %v", command, ok)
	}
	if got := len(call.Layers()); got != 1 {
		t.Errorf("%d layers, want 1", got)
	}
	if layers, ok := call.ProofLayers(); !ok || len(layers) != 1 {
		t.Errorf("proof layers %d %v", len(layers), ok)
	}
	if got := call.ProvenCwd(); got != "/work" {
		t.Errorf("proven cwd %q", got)
	}
	if got := call.PathArguments(); !reflect.DeepEqual(got, []string{"/work/big.txt"}) {
		t.Errorf("path arguments %v", got)
	}

	environment := call.Environment()
	if !environment.Present() || !environment.Canonical() {
		t.Errorf("environment present %v canonical %v", environment.Present(), environment.Canonical())
	}
	names := environment.Names()
	if len(names) != entries || names[0] != "K0000" || names[entries-1] != "K3999" {
		t.Fatalf("%d names, want %d from K0000 to K3999", len(names), entries)
	}
	if values, uncertain := environment.Values("K1234"); uncertain || !reflect.DeepEqual(values, []string{"v"}) {
		t.Errorf("values %v uncertain %v", values, uncertain)
	}

	execution := call.Execution()
	if execution.Proven || execution.Reason != "unqualified nested arguments" {
		t.Errorf("execution proven %v reason %q", execution.Proven, execution.Reason)
	}
	if options := call.ScriptOptions(quietReader{}); options.Cwd != "/work" || options.Context.Proven {
		t.Errorf("script options %+v", options)
	}
	if options := call.ScriptOptions(nil); !reflect.DeepEqual(options, shellwords.ScriptOptions{}) {
		t.Errorf("script options without a reader %+v", options)
	}

	writes := call.Writes()
	if !reflect.DeepEqual(writes.Targets, []string{"/work/big.txt"}) {
		t.Errorf("targets %v", writes.Targets)
	}
	if len(writes.Texts) != edits {
		t.Fatalf("%d texts, want %d", len(writes.Texts), edits)
	}
	if first, last := writes.Texts[0], writes.Texts[edits-1]; first != (toolcall.WrittenText{Path: "/work/big.txt", Text: "t0000", Old: "o"}) ||
		last != (toolcall.WrittenText{Path: "/work/big.txt", Text: "t1999", Old: "o"}) {
		t.Errorf("first text %+v last text %+v", first, last)
	}
	if writes.WholeFileTexts != nil || writes.ParseError {
		t.Errorf("whole-file texts %v parse error %v", writes.WholeFileTexts, writes.ParseError)
	}
	if toolcall.IsFileWrite(call.Name()) {
		t.Error("Bash is not a file-write tool")
	}
}
