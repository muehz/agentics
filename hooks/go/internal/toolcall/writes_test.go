package toolcall

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
)

// writeCall reads a call named name whose tool input is the JSON encoding of
// input, so numbers and nesting arrive as a host's input would.
func writeCall(t *testing.T, name string, input map[string]any) Call {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	call, err := Read(contract.NeutralEvent{
		Surface: "claude-code",
		Tool:    contract.NeutralTool{Name: new(name), InputRaw: raw},
	})
	if err != nil {
		t.Fatal(err)
	}
	return call
}

// TestWritesTargets holds the file-path table the secrets check pinned for its
// write-target reader, row for row.
func TestWritesTargets(t *testing.T) {
	for _, c := range []struct {
		name  string
		input map[string]any
		want  []string
	}{
		{"claude Write", map[string]any{"file_path": "a.txt", "content": "x"}, []string{"a.txt"}},
		{"claude NotebookEdit", map[string]any{"notebook_path": "n.ipynb"}, []string{"n.ipynb"}},
		{"copilot value envelope", map[string]any{"value": map[string]any{"path": "b.txt"}}, []string{"b.txt"}},
		{"every key counts", map[string]any{"file_path": "a.txt", "path": "b.txt"}, []string{"a.txt", "b.txt"}},
		{"repeats collapse", map[string]any{"file_path": "a.txt", "value": map[string]any{"file_path": "a.txt"}}, []string{"a.txt"}},
		{"patch files", map[string]any{"value": map[string]any{"files": []any{
			map[string]any{"path": "c.txt", "content": "x"},
			map[string]any{"path": "d.txt", "content": "y"},
		}}}, []string{"c.txt", "d.txt"}},
		{"edit list items", map[string]any{"file_path": "e.txt", "edits": []any{
			map[string]any{"file_path": "f.txt", "old_string": "a", "new_string": "b"},
			"not an object",
		}}, []string{"e.txt", "f.txt"}},
		{"lists in both places", map[string]any{"files": []any{map[string]any{"path": "benign.txt"}}, "value": map[string]any{"files": []any{
			map[string]any{"path": ".agentics/hooks.json", "content": "{}"},
		}}}, []string{"benign.txt", ".agentics/hooks.json"}},
		{"edit lists in both places", map[string]any{"edits": []any{map[string]any{"file_path": "a.txt"}}, "value": map[string]any{"edits": []any{
			map[string]any{"file_path": "b.txt"},
		}}}, []string{"a.txt", "b.txt"}},
		{"no path", map[string]any{"command": "ls"}, nil},
		{"empty strings are ignored", map[string]any{"file_path": ""}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := writeCall(t, "Write", c.input).Writes().Targets; !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Targets = %#v, want %#v", got, c.want)
			}
		})
	}
}

// TestWritesTexts holds the written-text table the secrets check pinned for
// its text reader, row for row.
func TestWritesTexts(t *testing.T) {
	for _, c := range []struct {
		name  string
		input map[string]any
		want  []WrittenText
	}{
		{"write", map[string]any{"file_path": "a.txt", "content": "x"}, []WrittenText{{"a.txt", "x", ""}}},
		{"edit pairs its old text", map[string]any{"file_path": "a.txt", "old_string": "o", "new_string": "n"}, []WrittenText{{"a.txt", "n", "o"}}},
		{"copilot create", map[string]any{"path": "a.txt", "file_text": "x"}, []WrittenText{{"a.txt", "x", ""}}},
		{"copilot edit under the value envelope", map[string]any{"value": map[string]any{"path": "a.txt", "old_str": "o", "new_str": "n"}}, []WrittenText{{"a.txt", "n", "o"}}},
		{"multiedit items inherit the call path", map[string]any{"file_path": "a.txt", "edits": []any{
			map[string]any{"old_string": "o1", "new_string": "n1"},
			map[string]any{"old_string": "o2", "new_string": "n2"},
		}}, []WrittenText{{"a.txt", "n1", "o1"}, {"a.txt", "n2", "o2"}}},
		{"files items name their own paths", map[string]any{"files": []any{
			map[string]any{"path": "a.txt", "content": "x"},
			map[string]any{"path": "b.txt", "content": "y"},
		}}, []WrittenText{{"a.txt", "x", ""}, {"b.txt", "y", ""}}},
		{"an empty text is not a text", map[string]any{"file_path": "a.txt", "content": ""}, []WrittenText{}},
		{"an insert keeps no old text", map[string]any{"command": "insert", "insert_line": 1, "path": "a.txt", "old_str": "o", "new_str": "n"}, []WrittenText{{"a.txt", "n", ""}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := writeCall(t, "Write", c.input).Writes().Texts; !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Texts = %#v, want %#v", got, c.want)
			}
		})
	}
}

// TestWritesForms states the whole view, and the path arguments, for each
// form a write reaches the hooks in.
func TestWritesForms(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input map[string]any
		want  Writes
		paths []string
	}{
		{
			name: "plain Write", tool: "Write",
			input: obj("file_path", "a.txt", "content", "x"),
			want: Writes{
				Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "x", ""}},
				WholeFileTexts: []string{"x"},
			},
			paths: []string{"a.txt"},
		},
		{
			name: "Bash name carrying writes reads the same", tool: "Bash",
			input: obj("file_path", "a.txt", "content", "x"),
			want: Writes{
				Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "x", ""}},
				WholeFileTexts: []string{"x"},
			},
			paths: []string{"a.txt"},
		},
		{
			name: "Copilot file_text create", tool: "create",
			input: obj("path", "a.txt", "file_text", "x"),
			want: Writes{
				Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "x", ""}},
				WholeFileTexts: []string{"x"},
			},
			paths: []string{"a.txt"},
		},
		{
			name: "Edit pairs its old text", tool: "Edit",
			input: obj("file_path", "a.txt", "old_string", "o", "new_string", "n"),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "n", "o"}}},
			paths: []string{"a.txt"},
		},
		{
			name: "str_replace command pairs its old text", tool: "str_replace_editor",
			input: obj("command", "str_replace", "path", "a.txt", "old_str", "o", "new_str", "n"),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "n", "o"}}},
			paths: []string{"a.txt"},
		},
		{
			name: "insert_line keeps no old text", tool: "str_replace_editor",
			input: obj("insert_line", 3, "path", "a.txt", "old_str", "o", "new_str", "n"),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "n", ""}}},
			paths: []string{"a.txt"},
		},
		{
			name: "null insert_line keeps no old text", tool: "str_replace_editor",
			input: obj("insert_line", nil, "path", "a.txt", "old_str", "o", "new_str", "n"),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "n", ""}}},
			paths: []string{"a.txt"},
		},
		{
			name: "a command other than str_replace keeps no old text", tool: "str_replace_editor",
			input: obj("command", "create", "path", "a.txt", "old_str", "o", "new_str", "n"),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "n", ""}}},
			paths: []string{"a.txt"},
		},
		{
			name: "a numeric command keeps no old text", tool: "str_replace_editor",
			input: obj("command", 5, "path", "a.txt", "old_str", "o", "new_str", "n"),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "n", ""}}},
			paths: []string{"a.txt"},
		},
		{
			name: "a non-string old text is no old text", tool: "Edit",
			input: obj("file_path", "a.txt", "old_string", 5, "new_string", "n"),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "n", ""}}},
			paths: []string{"a.txt"},
		},
		{
			name: "an unpaired key never supplies old text", tool: "Write",
			input: obj("file_path", "a.txt", "old_string", "o", "content", "n"),
			want: Writes{
				Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "n", ""}},
				WholeFileTexts: []string{"n"},
			},
			paths: []string{"a.txt"},
		},
		{
			name: "content keys read in roster order", tool: "Write",
			input: obj("file_path", "a.txt", "file_text", "f", "new_source", "s", "new_str", "r", "new_string", "g", "content", "c"),
			want: Writes{
				Targets: []string{"a.txt"},
				Texts: []WrittenText{
					{"a.txt", "c", ""}, {"a.txt", "g", ""}, {"a.txt", "r", ""}, {"a.txt", "s", ""}, {"a.txt", "f", ""},
				},
				WholeFileTexts: []string{"c", "f"},
			},
			paths: []string{"a.txt"},
		},
		{
			name: "MultiEdit list", tool: "MultiEdit",
			input: obj("file_path", "a.txt", "edits", []any{
				obj("old_string", "o1", "new_string", "n1"),
				obj("old_string", "o2", "new_string", "n2"),
			}),
			want: Writes{
				Targets: []string{"a.txt"},
				Texts:   []WrittenText{{"a.txt", "n1", "o1"}, {"a.txt", "n2", "o2"}},
			},
			paths: []string{"a.txt"},
		},
		{
			name: "pushed file list", tool: "push_files",
			input: obj("files", []any{obj("path", "a.txt", "content", "x"), obj("path", "b.txt", "content", "y")}),
			want: Writes{
				Targets: []string{"a.txt", "b.txt"},
				Texts:   []WrittenText{{"a.txt", "x", ""}, {"b.txt", "y", ""}},
			},
		},
		{
			name: "Write inside value", tool: "Write",
			input: obj("value", obj("file_path", "a.txt", "content", "x")),
			want: Writes{
				Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "x", ""}},
				WholeFileTexts: []string{"x"},
			},
			paths: []string{"a.txt"},
		},
		{
			name: "Copilot create inside value", tool: "create",
			input: obj("value", obj("path", "a.txt", "file_text", "x")),
			want: Writes{
				Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "x", ""}},
				WholeFileTexts: []string{"x"},
			},
			paths: []string{"a.txt"},
		},
		{
			name: "MultiEdit inside value", tool: "MultiEdit",
			input: obj("value", obj("file_path", "a.txt", "edits", []any{obj("old_string", "o", "new_string", "n")})),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "n", "o"}}},
			paths: []string{"a.txt"},
		},
		{
			name: "pushed file list inside value", tool: "apply_patch",
			input: obj("value", obj("files", []any{obj("path", "a.txt", "content", "x")})),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "x", ""}}},
		},
		{
			name: "root texts come before value texts", tool: "Write",
			input: obj("new_string", "r", "value", obj("content", "v", "path", "v.txt")),
			want: Writes{
				Targets:        []string{"v.txt"},
				Texts:          []WrittenText{{"v.txt", "r", ""}, {"v.txt", "v", ""}},
				WholeFileTexts: []string{"v"},
			},
			paths: []string{"v.txt"},
		},
		{
			name: "edits before files, each root before value", tool: "MultiEdit",
			input: obj(
				"files", []any{obj("content", "f1")},
				"edits", []any{obj("new_string", "e1")},
				"value", obj("files", []any{obj("content", "f2")}, "edits", []any{obj("new_string", "e2")}),
			),
			want: Writes{Texts: []WrittenText{{"", "e1", ""}, {"", "e2", ""}, {"", "f1", ""}, {"", "f2", ""}}},
		},
		{
			name: "non-object list items are skipped", tool: "MultiEdit",
			input: obj("file_path", "a.txt", "edits", []any{"x", 5, nil, []any{obj("new_string", "hidden")}, obj("new_string", "n")}),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{{"a.txt", "n", ""}}},
			paths: []string{"a.txt"},
		},
		{
			name: "a list that is not a list adds nothing", tool: "MultiEdit",
			input: obj("file_path", "a.txt", "edits", obj("new_string", "n")),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{}},
			paths: []string{"a.txt"},
		},
		{
			name: "an empty content is kept as whole-file text only", tool: "Write",
			input: obj("file_path", "a.txt", "content", ""),
			want: Writes{
				Targets: []string{"a.txt"}, Texts: []WrittenText{},
				WholeFileTexts: []string{""},
			},
			paths: []string{"a.txt"},
		},
		{
			name: "empty whole-file texts in both layers", tool: "Write",
			input: obj("content", " ", "file_text", "", "value", obj("content", "")),
			want:  Writes{Texts: []WrittenText{{"", " ", ""}}, WholeFileTexts: []string{" ", "", ""}},
		},
		{
			name: "a non-string content is no text", tool: "Write",
			input: obj("file_path", "a.txt", "content", 5),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{}},
			paths: []string{"a.txt"},
		},
		{
			name: "item path from the item's value", tool: "MultiEdit",
			input: obj("file_path", "a.txt", "edits", []any{obj("value", obj("file_path", "b.txt"), "new_string", "n")}),
			want:  Writes{Targets: []string{"a.txt", "b.txt"}, Texts: []WrittenText{{"b.txt", "n", ""}}},
			paths: []string{"a.txt"},
		},
		{
			name: "item text only under its value is not read", tool: "MultiEdit",
			input: obj("file_path", "a.txt", "edits", []any{obj("value", obj("new_string", "n"))}),
			want:  Writes{Targets: []string{"a.txt"}, Texts: []WrittenText{}},
			paths: []string{"a.txt"},
		},
		{
			name: "paths only in files items leave the call path empty", tool: "apply_patch",
			input: obj("content", "x", "files", []any{obj("path", "b.txt", "content", "y")}),
			want: Writes{
				Targets:        []string{"b.txt"},
				Texts:          []WrittenText{{"", "x", ""}, {"b.txt", "y", ""}},
				WholeFileTexts: []string{"x"},
			},
		},
		{
			name: "files items with empty content add no whole-file text", tool: "apply_patch",
			input: obj("files", []any{obj("path", "a.txt", "content", ""), obj("path", "b.txt", "file_text", "")}),
			want:  Writes{Targets: []string{"a.txt", "b.txt"}, Texts: []WrittenText{}},
		},
		{
			name: "path arguments keep repeats in layer and key order", tool: "Read",
			input: obj("path", "b", "file_path", "a", "file", "f", "value", obj("filePath", "c", "notebook_path", "n", "file_path", "a")),
			want:  Writes{Targets: []string{"a", "b", "f", "c", "n"}, Texts: []WrittenText{}},
			paths: []string{"a", "b", "f", "a", "c", "n"},
		},
		{
			name: "write_parse_error true", tool: "apply_patch",
			input: obj("value", obj(), "write_parse_error", true),
			want:  Writes{Texts: []WrittenText{}, ParseError: true},
		},
		{
			name: "write_parse_error true inside value", tool: "apply_patch",
			input: obj("value", obj("write_parse_error", true)),
			want:  Writes{Texts: []WrittenText{}, ParseError: true},
		},
		{
			name: "write_parse_error false", tool: "apply_patch",
			input: obj("write_parse_error", false),
			want:  Writes{Texts: []WrittenText{}},
		},
		{
			name: "write_parse_error as a string", tool: "apply_patch",
			input: obj("write_parse_error", "true"),
			want:  Writes{Texts: []WrittenText{}},
		},
		{
			name: "no arguments", tool: "Write",
			input: obj(),
			want:  Writes{Texts: []WrittenText{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := writeCall(t, tt.tool, tt.input)
			if got := call.Writes(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Writes() =\n%#v\nwant\n%#v", got, tt.want)
			}
			if got := call.PathArguments(); !reflect.DeepEqual(got, tt.paths) {
				t.Fatalf("PathArguments() = %#v, want %#v", got, tt.paths)
			}
		})
	}
}

// TestWritesOfNullInput pins that a null input reads as a call with no writes.
func TestWritesOfNullInput(t *testing.T) {
	call, err := Read(contract.NeutralEvent{Tool: contract.NeutralTool{InputRaw: json.RawMessage(`null`)}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := call.Writes(), (Writes{Texts: []WrittenText{}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("Writes() = %#v, want %#v", got, want)
	}
	if got := call.PathArguments(); got != nil {
		t.Fatalf("PathArguments() = %#v, want nil", got)
	}
}

// TestWritesOfManyEdits reads a call with thousands of edits and checks every
// text and target arrives once, in order.
func TestWritesOfManyEdits(t *testing.T) {
	const n = 5000
	edits := make([]any, n)
	wantTexts := make([]WrittenText, n)
	wantTargets := []string{"call.txt"}
	for i := range n {
		path := ""
		if i%2 == 1 {
			path = fmt.Sprintf("item%d.txt", i)
			wantTargets = append(wantTargets, path)
		}
		edits[i] = obj("file_path", path, "old_string", fmt.Sprintf("o%d", i), "new_string", fmt.Sprintf("n%d", i))
		itemPath := path
		if itemPath == "" {
			itemPath = "call.txt"
		}
		wantTexts[i] = WrittenText{itemPath, fmt.Sprintf("n%d", i), fmt.Sprintf("o%d", i)}
	}
	writes := writeCall(t, "MultiEdit", obj("file_path", "call.txt", "edits", edits)).Writes()
	if len(writes.Texts) != n || len(writes.Targets) != n/2+1 {
		t.Fatalf("got %d texts and %d targets, want %d and %d", len(writes.Texts), len(writes.Targets), n, n/2+1)
	}
	if !reflect.DeepEqual(writes.Texts, wantTexts) {
		t.Fatal("texts differ from the edits in order or content")
	}
	if !reflect.DeepEqual(writes.Targets, wantTargets) {
		t.Fatal("targets differ from the edits in order or content")
	}
}

func TestIsFileWrite(t *testing.T) {
	for _, name := range []string{"Write", "Edit", "MultiEdit", "NotebookEdit"} {
		if !IsFileWrite(name) {
			t.Errorf("IsFileWrite(%q) = false, want true", name)
		}
	}
	for _, name := range []string{
		"", "write", "WRITE", "edit", "Write ", " Edit", "multiedit", "notebookedit", "NotebookRead",
		"Read", "Bash", "apply_patch", "create", "str_replace_editor", "mcp__fs__Write", "functions.Write",
	} {
		if IsFileWrite(name) {
			t.Errorf("IsFileWrite(%q) = true, want false", name)
		}
	}
}

// TestSetFileWrites pins the file list the provider adapters store, and that
// the view reads it back.
func TestSetFileWrites(t *testing.T) {
	object := map[string]any{"files": "replaced", "other": "kept"}
	SetFileWrites(object, []FileWrite{{Path: "a.txt", Content: "x"}, {Path: "b.txt", Content: ""}})
	want := map[string]any{
		"files": []any{
			map[string]any{"path": "a.txt", "content": "x"},
			map[string]any{"path": "b.txt", "content": ""},
		},
		"other": "kept",
	}
	if !reflect.DeepEqual(object, want) {
		t.Fatalf("object = %#v, want %#v", object, want)
	}

	writes := writeCall(t, "apply_patch", obj("value", object)).Writes()
	wantWrites := Writes{
		Targets: []string{"a.txt", "b.txt"},
		Texts:   []WrittenText{{"a.txt", "x", ""}},
	}
	if !reflect.DeepEqual(writes, wantWrites) {
		t.Fatalf("Writes() after SetFileWrites = %#v, want %#v", writes, wantWrites)
	}
}

func TestSetFileWritesWithNoWritesStoresAnEmptyList(t *testing.T) {
	for _, writes := range [][]FileWrite{nil, {}} {
		object := map[string]any{}
		SetFileWrites(object, writes)
		files, ok := object["files"].([]any)
		if !ok || files == nil || len(files) != 0 {
			t.Fatalf("files = %#v, want a non-nil empty list", object["files"])
		}
		raw, err := json.Marshal(object)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != `{"files":[]}` {
			t.Fatalf("encoded = %s, want {\"files\":[]}", raw)
		}
	}
}

func TestMarkWriteParseError(t *testing.T) {
	input := map[string]any{"value": map[string]any{}, "write_parse_error": false}
	MarkWriteParseError(input)
	if want := (map[string]any{"value": map[string]any{}, "write_parse_error": true}); !reflect.DeepEqual(input, want) {
		t.Fatalf("input = %#v, want %#v", input, want)
	}
	if !writeCall(t, "apply_patch", input).Writes().ParseError {
		t.Fatal("ParseError = false after MarkWriteParseError")
	}
	raw, _ := json.Marshal(input)
	if !strings.Contains(string(raw), `"write_parse_error":true`) {
		t.Fatalf("encoded = %s", raw)
	}
}
