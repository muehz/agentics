package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
)

func pinAntigravityRuntime(t *testing.T, pluginRoot, dataRoot, vault string) {
	t.Helper()
	t.Setenv("PLUGIN_ROOT", pluginRoot)
	t.Setenv("PLUGIN_DATA", dataRoot)
	t.Setenv("AGX_VAULT", vault)
}

func marshalAntigravityPayload(t *testing.T, payload map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshaling payload: %v", err)
	}
	return raw
}

func TestAntigravityRegisteredAndSupportsGuardians(t *testing.T) {
	if _, ok := adapterFor("antigravity"); !ok {
		t.Fatal("antigravity adapter is not registered")
	}
}

func TestAntigravityNormalizeToolMappings(t *testing.T) {
	pinAntigravityRuntime(t, "", "", "")
	cases := []struct {
		rawName  string
		wantName string
		args     map[string]any
		wantKeys map[string]any
	}{
		{
			rawName:  "run_command",
			wantName: "Bash",
			args:     map[string]any{"CommandLine": "git status", "Cwd": "/repo", "WaitMsBeforeAsync": float64(5000)},
			wantKeys: map[string]any{"command": "git status", "cwd": "/repo"},
		},
		{
			rawName:  "run_command",
			wantName: "Bash",
			args:     map[string]any{"command": "echo fallback"},
			wantKeys: map[string]any{"command": "echo fallback"},
		},
		{
			rawName:  "view_file",
			wantName: "Read",
			args:     map[string]any{"AbsolutePath": "/repo/a.txt"},
			wantKeys: map[string]any{"file_path": "/repo/a.txt"},
		},
		{
			rawName:  "read_file",
			wantName: "Read",
			args:     map[string]any{"Path": "/repo/b.txt"},
			wantKeys: map[string]any{"file_path": "/repo/b.txt"},
		},
		{
			rawName:  "write_to_file",
			wantName: "Write",
			args:     map[string]any{"TargetFile": "/repo/c.txt", "CodeContent": "hello"},
			wantKeys: map[string]any{"file_path": "/repo/c.txt", "content": "hello"},
		},
		{
			rawName:  "write_file",
			wantName: "Write",
			args:     map[string]any{"Path": "/repo/d.txt", "Content": "world"},
			wantKeys: map[string]any{"file_path": "/repo/d.txt", "content": "world"},
		},
		{
			rawName:  "replace_file_content",
			wantName: "Edit",
			args:     map[string]any{"TargetFile": "/repo/e.txt", "TargetContent": "old", "ReplacementContent": "new"},
			wantKeys: map[string]any{"file_path": "/repo/e.txt", "old_string": "old", "new_string": "new"},
		},
		{
			rawName:  "edit_file",
			wantName: "Edit",
			args:     map[string]any{"Path": "/repo/f.txt", "Target": "before", "Replacement": "after"},
			wantKeys: map[string]any{"file_path": "/repo/f.txt", "old_string": "before", "new_string": "after"},
		},
		{
			rawName:  "multi_replace_file_content",
			wantName: "MultiEdit",
			args: map[string]any{
				"TargetFile": "/repo/g.txt",
				"ReplacementChunks": []any{
					map[string]any{"TargetContent": "one", "ReplacementContent": "1"},
					map[string]any{"TargetContent": "two", "ReplacementContent": "2"},
				},
			},
			wantKeys: map[string]any{
				"file_path": "/repo/g.txt",
				"edits": []any{
					map[string]any{"old_string": "one", "new_string": "1"},
					map[string]any{"old_string": "two", "new_string": "2"},
				},
			},
		},
		{
			rawName:  "grep_search",
			wantName: "Grep",
			args:     map[string]any{"SearchPath": "/repo", "Query": "needle"},
			wantKeys: map[string]any{"path": "/repo", "pattern": "needle"},
		},
		{
			rawName:  "find_by_name",
			wantName: "Glob",
			args:     map[string]any{"SearchDirectory": "/repo", "Pattern": "*.go"},
			wantKeys: map[string]any{"path": "/repo", "pattern": "*.go"},
		},
		{
			rawName:  "list_dir",
			wantName: "Glob",
			args:     map[string]any{"DirectoryPath": "/repo/sub"},
			wantKeys: map[string]any{"path": "/repo/sub"},
		},
		{
			rawName:  "search_web",
			wantName: "WebSearch",
			args:     map[string]any{"query": "agentics"},
			wantKeys: map[string]any{},
		},
		{
			rawName:  "read_url_content",
			wantName: "WebFetch",
			args:     map[string]any{"Url": "https://example.invalid"},
			wantKeys: map[string]any{},
		},
		{
			rawName:  "invoke_subagent",
			wantName: "Agent",
			args:     map[string]any{"TypeName": "research", "Prompt": "check"},
			wantKeys: map[string]any{},
		},
		{
			rawName:  "ask_question",
			wantName: "AskUserQuestion",
			args:     map[string]any{"questions": []any{}},
			wantKeys: map[string]any{},
		},
		{
			rawName:  "mcp__github__create_pull_request",
			wantName: "mcp__github__create_pull_request",
			args:     map[string]any{"owner": "muehz", "repo": "agentics"},
			wantKeys: map[string]any{},
		},
		{
			rawName:  "custom_shell_tool",
			wantName: "custom_shell_tool",
			args:     map[string]any{"command": "echo custom"},
			wantKeys: map[string]any{"command": "echo custom"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.rawName+"->"+tc.wantName, func(t *testing.T) {
			raw := marshalAntigravityPayload(t, map[string]any{
				"hookEventName": "PreToolUse",
				"toolCall":      map[string]any{"name": tc.rawName, "args": tc.args},
			})
			event, reject := Normalize("antigravity", "PreToolUse", raw)
			if reject != nil {
				t.Fatalf("unexpected rejection: %s", reject.Stderr)
			}
			doc := readEvent(t, event)
			if doc.Surface != "antigravity" {
				t.Fatalf("surface = %q, want antigravity", doc.Surface)
			}
			if doc.Tool.Name == nil || *doc.Tool.Name != tc.wantName {
				t.Fatalf("tool.name = %v, want %q", doc.Tool.Name, tc.wantName)
			}
			input := eventInput(t, doc)
			want := map[string]any{"value": tc.args}
			for k, v := range tc.wantKeys {
				want[k] = v
			}
			if !reflect.DeepEqual(input, want) {
				t.Fatalf("tool.input = %#v, want %#v", input, want)
			}
		})
	}
}

func TestAntigravityNormalizePathPrecedenceAndDualPath(t *testing.T) {
	pinAntigravityRuntime(t, "", "", "")
	cases := []struct {
		name         string
		tool         string
		args         map[string]any
		wantFilePath string
		wantPath     string
		wantNoPaths  bool
	}{
		{
			name:         "view_file prefers AbsolutePath and lifts distinct Path",
			tool:         "view_file",
			args:         map[string]any{"AbsolutePath": "/repo/safe.txt", "Path": "/repo/.env"},
			wantFilePath: "/repo/safe.txt",
			wantPath:     "/repo/.env",
		},
		{
			name:         "view_file falls back to Path when AbsolutePath empty",
			tool:         "view_file",
			args:         map[string]any{"AbsolutePath": "", "Path": "/repo/.env"},
			wantFilePath: "/repo/.env",
		},
		{
			name:         "view_file omits path when AbsolutePath and Path match",
			tool:         "view_file",
			args:         map[string]any{"AbsolutePath": "/repo/same.txt", "Path": "/repo/same.txt"},
			wantFilePath: "/repo/same.txt",
		},
		{
			name:         "read_file prefers Path and lifts distinct AbsolutePath",
			tool:         "read_file",
			args:         map[string]any{"Path": "/repo/primary.txt", "AbsolutePath": "/repo/.env"},
			wantFilePath: "/repo/primary.txt",
			wantPath:     "/repo/.env",
		},
		{
			name:         "read_file falls back to AbsolutePath when Path missing",
			tool:         "read_file",
			args:         map[string]any{"AbsolutePath": "/repo/fallback.txt"},
			wantFilePath: "/repo/fallback.txt",
		},
		{
			name:         "write_to_file prefers TargetFile and lifts distinct Path",
			tool:         "write_to_file",
			args:         map[string]any{"TargetFile": "/repo/.agentics/hooks.json", "Path": "/repo/safe.txt", "CodeContent": "{}"},
			wantFilePath: "/repo/.agentics/hooks.json",
			wantPath:     "/repo/safe.txt",
		},
		{
			name:         "write_file prefers Path and lifts distinct TargetFile",
			tool:         "write_file",
			args:         map[string]any{"Path": "/repo/safe.txt", "TargetFile": "/repo/.agentics/hooks.json", "Content": "{}"},
			wantFilePath: "/repo/safe.txt",
			wantPath:     "/repo/.agentics/hooks.json",
		},
		{
			name:         "replace_file_content prefers TargetFile and lifts distinct Path",
			tool:         "replace_file_content",
			args:         map[string]any{"TargetFile": "/repo/a.txt", "Path": "/repo/b.txt", "TargetContent": "x", "ReplacementContent": "y"},
			wantFilePath: "/repo/a.txt",
			wantPath:     "/repo/b.txt",
		},
		{
			name:         "edit_file prefers Path and lifts distinct TargetFile",
			tool:         "edit_file",
			args:         map[string]any{"Path": "/repo/b.txt", "TargetFile": "/repo/a.txt", "Target": "x", "Replacement": "y"},
			wantFilePath: "/repo/b.txt",
			wantPath:     "/repo/a.txt",
		},
		{
			name:         "multi_replace_file_content prefers TargetFile and lifts distinct Path",
			tool:         "multi_replace_file_content",
			args:         map[string]any{"TargetFile": "/repo/a.txt", "Path": "/repo/b.txt", "ReplacementChunks": []any{}},
			wantFilePath: "/repo/a.txt",
			wantPath:     "/repo/b.txt",
		},
		{
			name:        "empty and non-string path keys lift nothing",
			tool:        "view_file",
			args:        map[string]any{"AbsolutePath": "", "Path": float64(42)},
			wantNoPaths: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := marshalAntigravityPayload(t, map[string]any{
				"toolCall": map[string]any{"name": tc.tool, "args": tc.args},
			})
			event, reject := Normalize("antigravity", "PreToolUse", raw)
			if reject != nil {
				t.Fatalf("unexpected rejection: %s", reject.Stderr)
			}
			input := eventInput(t, readEvent(t, event))
			if tc.wantNoPaths {
				if _, has := input["file_path"]; has {
					t.Errorf("unexpected file_path: %v", input["file_path"])
				}
				if _, has := input["path"]; has {
					t.Errorf("unexpected path: %v", input["path"])
				}
				return
			}
			if got, _ := input["file_path"].(string); got != tc.wantFilePath {
				t.Errorf("file_path = %q, want %q", got, tc.wantFilePath)
			}
			gotPath, hasPath := input["path"].(string)
			if tc.wantPath == "" {
				if hasPath {
					t.Errorf("path = %q, want absent", gotPath)
				}
			} else if !hasPath || gotPath != tc.wantPath {
				t.Errorf("path = %q (present=%v), want %q", gotPath, hasPath, tc.wantPath)
			}
		})
	}
}

func TestAntigravityNormalizePreservesEmptyWriteContent(t *testing.T) {
	pinAntigravityRuntime(t, "", "", "")
	cases := []struct {
		name        string
		tool        string
		args        map[string]any
		wantContent string
		wantPresent bool
	}{
		{
			name:        "write_to_file preserves empty CodeContent over Content",
			tool:        "write_to_file",
			args:        map[string]any{"TargetFile": ".husky/pre-commit", "CodeContent": "", "Content": "fallback"},
			wantContent: "",
			wantPresent: true,
		},
		{
			name:        "write_to_file falls back to empty Content when CodeContent absent",
			tool:        "write_to_file",
			args:        map[string]any{"TargetFile": ".husky/pre-commit", "Content": ""},
			wantContent: "",
			wantPresent: true,
		},
		{
			name:        "write_file preserves empty Content over CodeContent",
			tool:        "write_file",
			args:        map[string]any{"Path": ".husky/pre-commit", "Content": "", "CodeContent": "fallback"},
			wantContent: "",
			wantPresent: true,
		},
		{
			name:        "write_file falls back to CodeContent when Content is null",
			tool:        "write_file",
			args:        map[string]any{"Path": ".husky/pre-commit", "Content": nil, "CodeContent": "body"},
			wantContent: "body",
			wantPresent: true,
		},
		{
			name:        "write_to_file omits content when neither key is a string",
			tool:        "write_to_file",
			args:        map[string]any{"TargetFile": "a.txt", "CodeContent": float64(123)},
			wantPresent: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := marshalAntigravityPayload(t, map[string]any{
				"toolCall": map[string]any{"name": tc.tool, "args": tc.args},
			})
			event, reject := Normalize("antigravity", "PreToolUse", raw)
			if reject != nil {
				t.Fatalf("unexpected rejection: %s", reject.Stderr)
			}
			input := eventInput(t, readEvent(t, event))
			val, present := input["content"]
			if present != tc.wantPresent {
				t.Fatalf("content present = %v (%#v), want %v", present, val, tc.wantPresent)
			}
			if tc.wantPresent {
				text, ok := val.(string)
				if !ok || text != tc.wantContent {
					t.Fatalf("content = %#v, want %q", val, tc.wantContent)
				}
			}
			if _, hasOld := input["old_string"]; hasOld {
				t.Fatalf("write tool must never lift old_string: %#v", input)
			}
		})
	}
}

func TestAntigravityNormalizeSchemaPairedReplacements(t *testing.T) {
	pinAntigravityRuntime(t, "", "", "")
	cases := []struct {
		name       string
		tool       string
		args       map[string]any
		wantNew    string
		wantHasNew bool
		wantOld    string
		wantHasOld bool
	}{
		{
			name:       "replace_file_content pairs TargetContent and ReplacementContent",
			tool:       "replace_file_content",
			args:       map[string]any{"TargetFile": "a.txt", "TargetContent": "old", "ReplacementContent": "new"},
			wantNew:    "new",
			wantHasNew: true,
			wantOld:    "old",
			wantHasOld: true,
		},
		{
			name:       "replace_file_content preserves empty ReplacementContent",
			tool:       "replace_file_content",
			args:       map[string]any{"TargetFile": "a.txt", "TargetContent": "old", "ReplacementContent": ""},
			wantNew:    "",
			wantHasNew: true,
			wantOld:    "old",
			wantHasOld: true,
		},
		{
			name:       "replace_file_content refuses cross-schema Target with ReplacementContent",
			tool:       "replace_file_content",
			args:       map[string]any{"TargetFile": "a.txt", "Target": "forged-old", "ReplacementContent": "secret"},
			wantNew:    "secret",
			wantHasNew: true,
			wantHasOld: false,
		},
		{
			name:       "replace_file_content ignores edit_file Replacement",
			tool:       "replace_file_content",
			args:       map[string]any{"TargetFile": "a.txt", "TargetContent": "old", "Replacement": "ignored"},
			wantHasNew: false,
			wantHasOld: false,
		},
		{
			name:       "edit_file pairs Target and Replacement",
			tool:       "edit_file",
			args:       map[string]any{"Path": "a.txt", "Target": "old", "Replacement": "new"},
			wantNew:    "new",
			wantHasNew: true,
			wantOld:    "old",
			wantHasOld: true,
		},
		{
			name:       "edit_file refuses cross-schema TargetContent with Replacement",
			tool:       "edit_file",
			args:       map[string]any{"Path": "a.txt", "TargetContent": "forged-old", "Replacement": "secret"},
			wantNew:    "secret",
			wantHasNew: true,
			wantHasOld: false,
		},
		{
			name:       "edit_file ignores replace_file_content ReplacementContent",
			tool:       "edit_file",
			args:       map[string]any{"Path": "a.txt", "Target": "old", "ReplacementContent": "ignored"},
			wantHasNew: false,
			wantHasOld: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := marshalAntigravityPayload(t, map[string]any{
				"toolCall": map[string]any{"name": tc.tool, "args": tc.args},
			})
			event, reject := Normalize("antigravity", "PreToolUse", raw)
			if reject != nil {
				t.Fatalf("unexpected rejection: %s", reject.Stderr)
			}
			input := eventInput(t, readEvent(t, event))
			gotNew, hasNew := input["new_string"].(string)
			if hasNew != tc.wantHasNew || (tc.wantHasNew && gotNew != tc.wantNew) {
				t.Errorf("new_string = %q (present=%v), want %q (present=%v)", gotNew, hasNew, tc.wantNew, tc.wantHasNew)
			}
			gotOld, hasOld := input["old_string"].(string)
			if hasOld != tc.wantHasOld || (tc.wantHasOld && gotOld != tc.wantOld) {
				t.Errorf("old_string = %q (present=%v), want %q (present=%v)", gotOld, hasOld, tc.wantOld, tc.wantHasOld)
			}
		})
	}
}

func TestAntigravityNormalizeMultiReplaceChunks(t *testing.T) {
	pinAntigravityRuntime(t, "", "", "")

	t.Run("valid chunks produce []any edits without write_parse_error", func(t *testing.T) {
		// Call normalizeAntigravityToolInput directly first to prove the in-memory
		// slice type is []any (not []map[string]any) before JSON encoding.
		args := map[string]any{
			"TargetFile": "/repo/a.txt",
			"ReplacementChunks": []any{
				map[string]any{"TargetContent": "a", "ReplacementContent": "b"},
				map[string]any{"ReplacementContent": "c"},
			},
		}
		direct := normalizeAntigravityToolInput("multi_replace_file_content", args)
		directEdits, ok := direct["edits"].([]any)
		if !ok {
			t.Fatalf("direct edits type = %T, want []any", direct["edits"])
		}
		if len(directEdits) != 2 {
			t.Fatalf("direct edits len = %d, want 2", len(directEdits))
		}
		if _, hasErr := direct["write_parse_error"]; hasErr {
			t.Fatalf("unexpected write_parse_error on valid chunks: %#v", direct)
		}

		raw := marshalAntigravityPayload(t, map[string]any{
			"toolCall": map[string]any{"name": "multi_replace_file_content", "args": args},
		})
		event, reject := Normalize("antigravity", "PreToolUse", raw)
		if reject != nil {
			t.Fatalf("unexpected rejection: %s", reject.Stderr)
		}
		input := eventInput(t, readEvent(t, event))
		wantEdits := []any{
			map[string]any{"old_string": "a", "new_string": "b"},
			map[string]any{"new_string": "c"},
		}
		if !reflect.DeepEqual(input["edits"], wantEdits) {
			t.Fatalf("edits = %#v, want %#v", input["edits"], wantEdits)
		}
		if _, hasErr := input["write_parse_error"]; hasErr {
			t.Fatalf("unexpected write_parse_error: %#v", input)
		}
	})

	malformedCases := []struct {
		name      string
		args      map[string]any
		wantEdits []any
	}{
		{
			name: "missing ReplacementChunks",
			args: map[string]any{"TargetFile": "/repo/a.txt"},
		},
		{
			name: "null ReplacementChunks",
			args: map[string]any{"TargetFile": "/repo/a.txt", "ReplacementChunks": nil},
		},
		{
			name: "non-slice ReplacementChunks",
			args: map[string]any{"TargetFile": "/repo/a.txt", "ReplacementChunks": "not-a-list"},
		},
		{
			name: "non-object chunk",
			args: map[string]any{"TargetFile": "/repo/a.txt", "ReplacementChunks": []any{"bad"}},
		},
		{
			name: "chunk missing ReplacementContent",
			args: map[string]any{"TargetFile": "/repo/a.txt", "ReplacementChunks": []any{map[string]any{"TargetContent": "a"}}},
		},
		{
			name: "chunk with non-string ReplacementContent",
			args: map[string]any{"TargetFile": "/repo/a.txt", "ReplacementChunks": []any{map[string]any{"TargetContent": "a", "ReplacementContent": float64(7)}}},
		},
		{
			name: "chunk with non-string TargetContent sets write_parse_error and still lifts new_string",
			args: map[string]any{
				"TargetFile": "/repo/a.txt",
				"ReplacementChunks": []any{
					map[string]any{"TargetContent": float64(9), "ReplacementContent": "secret"},
				},
			},
			wantEdits: []any{map[string]any{"new_string": "secret"}},
		},
		{
			name: "mixed valid and malformed chunks set write_parse_error and retain valid chunks",
			args: map[string]any{
				"TargetFile": "/repo/a.txt",
				"ReplacementChunks": []any{
					map[string]any{"TargetContent": "old1", "ReplacementContent": "new1"},
					"bad-chunk",
					map[string]any{"TargetContent": "old2", "ReplacementContent": float64(2)},
					map[string]any{"TargetContent": "old3", "ReplacementContent": "new3"},
				},
			},
			wantEdits: []any{
				map[string]any{"old_string": "old1", "new_string": "new1"},
				map[string]any{"old_string": "old3", "new_string": "new3"},
			},
		},
	}

	for _, tc := range malformedCases {
		t.Run(tc.name, func(t *testing.T) {
			raw := marshalAntigravityPayload(t, map[string]any{
				"toolCall": map[string]any{"name": "multi_replace_file_content", "args": tc.args},
			})
			event, reject := Normalize("antigravity", "PreToolUse", raw)
			if reject != nil {
				t.Fatalf("unexpected rejection: %s", reject.Stderr)
			}
			input := eventInput(t, readEvent(t, event))
			if got, ok := input["write_parse_error"].(bool); !ok || !got {
				t.Fatalf("write_parse_error = %#v, want true", input["write_parse_error"])
			}
			if tc.wantEdits == nil {
				if _, hasEdits := input["edits"]; hasEdits {
					t.Fatalf("edits = %#v, want absent", input["edits"])
				}
			} else {
				edits, ok := input["edits"].([]any)
				if !ok || !reflect.DeepEqual(edits, tc.wantEdits) {
					t.Fatalf("edits = %#v, want %#v", input["edits"], tc.wantEdits)
				}
			}
		})
	}
}

func TestAntigravityNormalizeRunCommandCwdLifting(t *testing.T) {
	pinAntigravityRuntime(t, "", "", "")
	for _, tc := range []struct {
		name    string
		args    map[string]any
		wantCwd any
		present bool
	}{
		{"string Cwd lifted verbatim", map[string]any{"CommandLine": "ls", "Cwd": "/other"}, "/other", true},
		{"non-string Cwd lifted verbatim for ProvenCwd fail-closed", map[string]any{"CommandLine": "ls", "Cwd": float64(123)}, float64(123), true},
		{"null Cwd omitted", map[string]any{"CommandLine": "ls", "Cwd": nil}, nil, false},
		{"absent Cwd omitted", map[string]any{"CommandLine": "ls"}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := marshalAntigravityPayload(t, map[string]any{
				"cwd":      "/repo",
				"toolCall": map[string]any{"name": "run_command", "args": tc.args},
			})
			event, reject := Normalize("antigravity", "PreToolUse", raw)
			if reject != nil {
				t.Fatalf("unexpected rejection: %s", reject.Stderr)
			}
			input := eventInput(t, readEvent(t, event))
			got, ok := input["cwd"]
			if ok != tc.present || (tc.present && !reflect.DeepEqual(got, tc.wantCwd)) {
				t.Fatalf("tool.input.cwd = %#v (present=%v), want %#v (present=%v)", got, ok, tc.wantCwd, tc.present)
			}
		})
	}
}

func TestAntigravityNormalizeMetadataWorkspaceFallbackAndRuntime(t *testing.T) {
	pinAntigravityRuntime(t, "/plugin/root", "/plugin/data", "/vault/root")

	t.Run("cwd falls back to workspacePaths[0] and metadata is read", func(t *testing.T) {
		raw := []byte(`{
			"conversationId": "conv-1",
			"turnId": "turn-1",
			"workspacePaths": ["/ws/first", "/ws/second"],
			"transcriptPath": "/logs/transcript.jsonl",
			"modelName": "gemini-3.6-flash-medium",
			"toolCall": {"name": "view_file", "args": null}
		}`)
		event, reject := Normalize("antigravity", "PreToolUse", raw)
		if reject != nil {
			t.Fatalf("unexpected rejection: %s", reject.Stderr)
		}
		doc := readEvent(t, event)
		if doc.Cwd == nil || *doc.Cwd != "/ws/first" {
			t.Errorf("cwd = %v, want /ws/first", doc.Cwd)
		}
		if doc.SessionID == nil || *doc.SessionID != "conv-1" {
			t.Errorf("session_id = %v, want conv-1", doc.SessionID)
		}
		if doc.TurnID == nil || *doc.TurnID != "turn-1" {
			t.Errorf("turn_id = %v, want turn-1", doc.TurnID)
		}
		if doc.Runtime.PluginRoot == nil || *doc.Runtime.PluginRoot != "/plugin/root" ||
			doc.Runtime.DataRoot == nil || *doc.Runtime.DataRoot != "/plugin/data" ||
			doc.Runtime.VaultOverride == nil || *doc.Runtime.VaultOverride != "/vault/root" ||
			doc.Runtime.LogRoot == nil || *doc.Runtime.LogRoot != "/plugin/data" {
			t.Errorf("runtime = %+v, want pinned PLUGIN_ROOT/PLUGIN_DATA/AGX_VAULT", doc.Runtime)
		}
		input := eventInput(t, doc)
		if !reflect.DeepEqual(input, map[string]any{"value": map[string]any{}}) {
			t.Errorf("null args normalized to %#v, want empty object under value", input)
		}
	})

	t.Run("explicit cwd beats workspacePaths and absent args defaults to empty object", func(t *testing.T) {
		pinAntigravityRuntime(t, "/plugin/root", "", "")
		raw := []byte(`{
			"cwd": "/explicit",
			"workspacePaths": ["/ws/first"],
			"toolCall": {"name": "search_web"}
		}`)
		event, reject := Normalize("antigravity", "PreToolUse", raw)
		if reject != nil {
			t.Fatalf("unexpected rejection: %s", reject.Stderr)
		}
		doc := readEvent(t, event)
		if doc.Cwd == nil || *doc.Cwd != "/explicit" {
			t.Errorf("cwd = %v, want /explicit", doc.Cwd)
		}
		if doc.Runtime.LogRoot == nil || *doc.Runtime.LogRoot != "/plugin/root" {
			t.Errorf("log_root = %v, want plugin-root fallback", doc.Runtime.LogRoot)
		}
		input := eventInput(t, doc)
		if !reflect.DeepEqual(input, map[string]any{"value": map[string]any{}}) {
			t.Errorf("absent args normalized to %#v, want empty object under value", input)
		}
	})

	t.Run("empty cwd falls back to workspacePaths[0]", func(t *testing.T) {
		raw := []byte(`{"cwd":"","workspacePaths":["/ws/only"],"toolCall":{"name":"list_dir","args":{}}}`)
		event, reject := Normalize("antigravity", "PreToolUse", raw)
		if reject != nil {
			t.Fatalf("unexpected rejection: %s", reject.Stderr)
		}
		doc := readEvent(t, event)
		if doc.Cwd == nil || *doc.Cwd != "/ws/only" {
			t.Errorf("cwd = %v, want /ws/only", doc.Cwd)
		}
	})

	t.Run("run_command Cwd binds event cwd over workspacePaths and session cwd", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			payload map[string]any
			wantCwd string
		}{
			{
				name: "workspacePaths multi-workspace uses run_command.Cwd",
				payload: map[string]any{
					"workspacePaths": []string{"/ws/relaxed", "/ws/protected"},
					"toolCall":       map[string]any{"name": "run_command", "args": map[string]any{"CommandLine": "kubectl apply -f prod.yaml", "Cwd": "/ws/protected"}},
				},
				wantCwd: "/ws/protected",
			},
			{
				name: "explicit session cwd yields to run_command.Cwd",
				payload: map[string]any{
					"cwd":      "/ws/relaxed",
					"toolCall": map[string]any{"name": "run_command", "args": map[string]any{"CommandLine": "kubectl apply -f prod.yaml", "Cwd": "/ws/protected"}},
				},
				wantCwd: "/ws/protected",
			},
			{
				name: "relative run_command.Cwd binds relative cwd so settings apply strict defaults",
				payload: map[string]any{
					"workspacePaths": []string{"/ws/relaxed"},
					"toolCall":       map[string]any{"name": "run_command", "args": map[string]any{"CommandLine": "kubectl apply -f prod.yaml", "Cwd": "../protected"}},
				},
				wantCwd: "../protected",
			},
			{
				name: "malformed non-string run_command.Cwd binds relative sentinel instead of workspacePaths[0]",
				payload: map[string]any{
					"workspacePaths": []string{"/ws/relaxed"},
					"toolCall":       map[string]any{"name": "run_command", "args": map[string]any{"CommandLine": "kubectl apply -f prod.yaml", "Cwd": 123}},
				},
				wantCwd: ".",
			},
			{
				name: "empty run_command.Cwd preserves workspacePaths[0]",
				payload: map[string]any{
					"workspacePaths": []string{"/ws/only"},
					"toolCall":       map[string]any{"name": "run_command", "args": map[string]any{"CommandLine": "git status", "Cwd": ""}},
				},
				wantCwd: "/ws/only",
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				raw := marshalAntigravityPayload(t, tc.payload)
				event, reject := Normalize("antigravity", "PreToolUse", raw)
				if reject != nil {
					t.Fatalf("unexpected rejection: %s", reject.Stderr)
				}
				doc := readEvent(t, event)
				if doc.Cwd == nil || *doc.Cwd != tc.wantCwd {
					t.Fatalf("cwd = %v, want %q", doc.Cwd, tc.wantCwd)
				}
			})
		}
	})

	t.Run("transcriptPath and modelName retain deferred NUL validation", func(t *testing.T) {
		for _, field := range []string{"transcriptPath", "modelName"} {
			raw := []byte(`{"` + field + `":"\u0000","toolCall":{"name":"view_file","args":{}}}`)
			event, reject := Normalize("antigravity", "PreToolUse", raw)
			if reject != nil {
				t.Fatalf("%s: NUL must reach handler validation: %s", field, reject.Stderr)
			}
			if _, err := event.Read(); err == nil {
				t.Fatalf("%s bypassed deferred NUL validation", field)
			}
		}
	})
}

func TestAntigravityNormalizeRejectsMalformedInputs(t *testing.T) {
	pinAntigravityRuntime(t, "", "", "")
	cases := []struct {
		name      string
		event     string
		payload   string
		wantError string
	}{
		{
			name:      "unsupported providerEvent",
			event:     "PostToolUse",
			payload:   `{"toolCall":{"name":"run_command","args":{}}}`,
			wantError: "expected Antigravity PostToolUse payload",
		},
		{
			name:      "mismatched hookEventName",
			event:     "PreToolUse",
			payload:   `{"hookEventName":"PostToolUse","toolCall":{"name":"run_command","args":{}}}`,
			wantError: "expected Antigravity PreToolUse payload",
		},
		{
			name:      "non-string hookEventName",
			event:     "PreToolUse",
			payload:   `{"hookEventName":123,"toolCall":{"name":"run_command","args":{}}}`,
			wantError: "expected Antigravity PreToolUse payload",
		},
		{
			name:      "missing toolCall",
			event:     "PreToolUse",
			payload:   `{"hookEventName":"PreToolUse"}`,
			wantError: "Antigravity field toolCall must be a present object",
		},
		{
			name:      "non-object toolCall",
			event:     "PreToolUse",
			payload:   `{"toolCall":"run_command"}`,
			wantError: "Antigravity field toolCall must be a present object",
		},
		{
			name:      "missing toolCall.name",
			event:     "PreToolUse",
			payload:   `{"toolCall":{"args":{}}}`,
			wantError: "Antigravity field toolCall.name must be a present string",
		},
		{
			name:      "non-string toolCall.name",
			event:     "PreToolUse",
			payload:   `{"toolCall":{"name":42,"args":{}}}`,
			wantError: "Antigravity field toolCall.name must be a present string",
		},
		{
			name:      "non-object toolCall.args",
			event:     "PreToolUse",
			payload:   `{"toolCall":{"name":"run_command","args":["echo"]}}`,
			wantError: "Antigravity field toolCall.args must be an object or null",
		},
		{
			name:      "non-string cwd",
			event:     "PreToolUse",
			payload:   `{"cwd":123,"toolCall":{"name":"run_command","args":{}}}`,
			wantError: "Antigravity field cwd must be a string or null",
		},
		{
			name:      "non-array workspacePaths",
			event:     "PreToolUse",
			payload:   `{"workspacePaths":"/repo","toolCall":{"name":"run_command","args":{}}}`,
			wantError: "Antigravity field workspacePaths must be an array of strings or null",
		},
		{
			name:      "non-string element in workspacePaths",
			event:     "PreToolUse",
			payload:   `{"workspacePaths":[123],"toolCall":{"name":"run_command","args":{}}}`,
			wantError: "Antigravity field workspacePaths must be an array of strings or null",
		},
		{
			name:      "non-string conversationId",
			event:     "PreToolUse",
			payload:   `{"conversationId":9,"toolCall":{"name":"run_command","args":{}}}`,
			wantError: "Antigravity field conversationId must be a string or null",
		},
		{
			name:      "non-string turnId",
			event:     "PreToolUse",
			payload:   `{"turnId":9,"toolCall":{"name":"run_command","args":{}}}`,
			wantError: "Antigravity field turnId must be a string or null",
		},
		{
			name:      "non-string transcriptPath",
			event:     "PreToolUse",
			payload:   `{"transcriptPath":true,"toolCall":{"name":"run_command","args":{}}}`,
			wantError: "Antigravity field transcriptPath must be a string or null",
		},
		{
			name:      "non-string modelName",
			event:     "PreToolUse",
			payload:   `{"modelName":[],"toolCall":{"name":"run_command","args":{}}}`,
			wantError: "Antigravity field modelName must be a string or null",
		},
		{
			name:      "non-finite float in toolCall.args fails normalization",
			event:     "PreToolUse",
			payload:   `{"toolCall":{"name":"run_command","args":{"WaitMsBeforeAsync":1e999}}}`,
			wantError: "payload cannot be normalized: contract: non-finite JSON constant: 1e999",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, reject := Normalize("antigravity", tc.event, []byte(tc.payload))
			if reject == nil {
				t.Fatal("expected malformed rejection")
			}
			if reject.Exit != hookspec.ExitOK {
				t.Errorf("exit = %d, want %d", reject.Exit, hookspec.ExitOK)
			}
			if string(reject.Stdout) != "{\"decision\":\"allow\"}\n" {
				t.Errorf("stdout = %q, want allow JSON", reject.Stdout)
			}
			wantStderr := "antigravity: malformed " + tc.event + " payload: " + tc.wantError + "\n"
			if string(reject.Stderr) != wantStderr {
				t.Errorf("stderr = %q, want %q", reject.Stderr, wantStderr)
			}
		})
	}
}

func TestAntigravityRenderAllOutcomes(t *testing.T) {
	cases := []struct {
		name       string
		event      string
		outcome    hookspec.Outcome
		coreStderr []byte
		wantStdout string
		wantStderr string
		wantExit   int
	}{
		{
			name:       "pass",
			event:      "PreToolUse",
			outcome:    contract.Pass(),
			wantStdout: "{\"decision\":\"allow\"}\n",
			wantExit:   hookspec.ExitOK,
		},
		{
			name:       "continue",
			event:      "PreToolUse",
			outcome:    contract.Continue(),
			wantStdout: "{\"decision\":\"allow\"}\n",
			wantExit:   hookspec.ExitOK,
		},
		{
			name:       "context renders allow",
			event:      "PreToolUse",
			outcome:    contract.Context("advisory note"),
			wantStdout: "{\"decision\":\"allow\"}\n",
			wantExit:   hookspec.ExitOK,
		},
		{
			name:       "deny on PreToolUse",
			event:      "PreToolUse",
			outcome:    contract.Deny("blocked <unsafe> & denied"),
			wantStdout: "{\"decision\":\"deny\",\"reason\":\"blocked <unsafe> & denied\"}\n",
			wantExit:   hookspec.ExitOK,
		},
		{
			name:       "deny on non-PreToolUse is invalid outcome",
			event:      "PostToolUse",
			outcome:    contract.Deny("too late"),
			wantStdout: "{\"decision\":\"allow\"}\n",
			wantStderr: "antigravity: PostToolUse core produced an invalid deny outcome\n",
			wantExit:   hookspec.ExitOK,
		},
		{
			name:       "error prefers coreStderr over outcome payload",
			event:      "PreToolUse",
			outcome:    contract.Error("fallback"),
			coreStderr: []byte("core diagnostic\n\n"),
			wantStdout: "{\"decision\":\"allow\"}\n",
			wantStderr: "core diagnostic\n",
			wantExit:   hookspec.ExitOK,
		},
		{
			name:       "error uses payload when coreStderr empty",
			event:      "PreToolUse",
			outcome:    contract.Error("boom"),
			wantStdout: "{\"decision\":\"allow\"}\n",
			wantStderr: "boom\n",
			wantExit:   hookspec.ExitOK,
		},
		{
			name:       "zero invalid outcome uses last-resort diagnostic",
			event:      "PreToolUse",
			outcome:    hookspec.Outcome{},
			wantStdout: "{\"decision\":\"allow\"}\n",
			wantStderr: antigravityTraits.failedMessage + "\n",
			wantExit:   hookspec.ExitOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Render("antigravity", tc.event, tc.outcome, tc.coreStderr)
			if got.Exit != tc.wantExit || string(got.Stdout) != tc.wantStdout || string(got.Stderr) != tc.wantStderr {
				t.Fatalf("Render = {Exit:%d Stdout:%q Stderr:%q}, want {Exit:%d Stdout:%q Stderr:%q}",
					got.Exit, got.Stdout, got.Stderr, tc.wantExit, tc.wantStdout, tc.wantStderr)
			}
		})
	}
}

func TestAntigravitySurfacePayloadFixtures(t *testing.T) {
	pinAntigravityRuntime(t, "", "", "")
	payloadsDir := filepath.Join("..", "..", "surfaces", "antigravity", "payloads")

	validCases := []struct {
		file     string
		wantTool string
		wantKey  string
	}{
		{"pre-tool-use.json", "Bash", "command"},
		{"pre-tool-use-dangerous.json", "Bash", "command"},
		{"pre-tool-use-secret-read.json", "Read", "file_path"},
		{"pre-tool-use-secret-write.json", "Write", "content"},
		{"pre-tool-use-replace.json", "Edit", "new_string"},
		{"pre-tool-use-multi-replace.json", "MultiEdit", "edits"},
		{"pre-tool-use-mcp-publication.json", "mcp__github__create_pull_request", "value"},
		{"pre-tool-use-mcp-credential-read.json", "mcp__github__get_file_contents", "value"},
	}

	for _, tc := range validCases {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(payloadsDir, tc.file))
			if err != nil {
				t.Fatalf("reading fixture: %v", err)
			}
			event, reject := Normalize("antigravity", "PreToolUse", raw)
			if reject != nil {
				t.Fatalf("fixture %s rejected: %s", tc.file, reject.Stderr)
			}
			doc := readEvent(t, event)
			if doc.Tool.Name == nil || *doc.Tool.Name != tc.wantTool {
				t.Fatalf("tool.name = %v, want %q", doc.Tool.Name, tc.wantTool)
			}
			if doc.Cwd == nil || *doc.Cwd != "/tmp/agentics-antigravity-fixture" {
				t.Fatalf("cwd = %v, want /tmp/agentics-antigravity-fixture", doc.Cwd)
			}
			input := eventInput(t, doc)
			if _, ok := input[tc.wantKey]; !ok {
				t.Fatalf("expected key %q in normalized input: %#v", tc.wantKey, input)
			}
		})
	}

	for _, tc := range []struct {
		file       string
		wantSubstr string
	}{
		{"malformed.json", "payload is not valid JSON"},
		{"invalid-utf8.json", "payload is not valid UTF-8"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(payloadsDir, tc.file))
			if err != nil {
				t.Fatalf("reading fixture: %v", err)
			}
			_, reject := Normalize("antigravity", "PreToolUse", raw)
			if reject == nil {
				t.Fatalf("fixture %s unexpectedly accepted", tc.file)
			}
			if reject.Exit != hookspec.ExitOK || string(reject.Stdout) != "{\"decision\":\"allow\"}\n" {
				t.Fatalf("reject shape = %d / %q, want 0 / allow JSON", reject.Exit, reject.Stdout)
			}
			if !strings.Contains(string(reject.Stderr), tc.wantSubstr) {
				t.Fatalf("stderr = %q, want substring %q", reject.Stderr, tc.wantSubstr)
			}
		})
	}
}

func TestAntigravityNormalizeTaskInputTools(t *testing.T) {
	pinAntigravityRuntime(t, "", "", "")
	cases := []struct {
		name             string
		rawTool          string
		args             map[string]any
		wantTool         string
		wantUnclassified bool
		wantCommand      string
		wantHasCommand   bool
	}{
		{
			name:     "manage_task list stays lifecycle",
			rawTool:  "manage_task",
			args:     map[string]any{"Action": "list"},
			wantTool: "manage_task",
		},
		{
			name:     "manage_task status stays lifecycle",
			rawTool:  "manage_task",
			args:     map[string]any{"Action": "status", "TaskId": "task-1"},
			wantTool: "manage_task",
		},
		{
			name:     "manage_task kill stays lifecycle",
			rawTool:  "manage_task",
			args:     map[string]any{"Action": "kill", "TaskId": "task-1"},
			wantTool: "manage_task",
		},
		{
			name:     "manage_task status with null Input stays lifecycle",
			rawTool:  "manage_task",
			args:     map[string]any{"Action": "status", "TaskId": "task-1", "Input": nil},
			wantTool: "manage_task",
		},
		{
			name:             "manage_task send_input Python reset normalizes to Bash and unclassified",
			rawTool:          "manage_task",
			args:             map[string]any{"Action": "send_input", "TaskId": "task-1", "Input": "import os; os.system(\"git reset --hard\")\n"},
			wantTool:         "Bash",
			wantUnclassified: true,
			wantCommand:      "import os; os.system(\"git reset --hard\")\n",
			wantHasCommand:   true,
		},
		{
			name:             "manage_task send_input lowercase input lifts command",
			rawTool:          "manage_task",
			args:             map[string]any{"Action": "send_input", "TaskId": "task-1", "input": "cat .env\n"},
			wantTool:         "Bash",
			wantUnclassified: true,
			wantCommand:      "cat .env\n",
			wantHasCommand:   true,
		},
		{
			name:             "manage_task send_input empty Input does not mask lowercase input",
			rawTool:          "manage_task",
			args:             map[string]any{"Action": "send_input", "TaskId": "task-1", "Input": "", "input": "git reset --hard\n"},
			wantTool:         "Bash",
			wantUnclassified: true,
			wantCommand:      "git reset --hard\n",
			wantHasCommand:   true,
		},
		{
			name:             "manage_task send_input empty Input stays unclassified",
			rawTool:          "manage_task",
			args:             map[string]any{"Action": "send_input", "TaskId": "task-1", "Input": ""},
			wantTool:         "Bash",
			wantUnclassified: true,
			wantCommand:      "",
			wantHasCommand:   true,
		},
		{
			name:             "manage_task send_input non-string Input stays unclassified without command",
			rawTool:          "manage_task",
			args:             map[string]any{"Action": "send_input", "TaskId": "task-1", "Input": float64(42)},
			wantTool:         "Bash",
			wantUnclassified: true,
			wantHasCommand:   false,
		},
		{
			name:             "manage_task smuggled Input alongside status fails closed",
			rawTool:          "manage_task",
			args:             map[string]any{"Action": "status", "TaskId": "task-1", "Input": "open('.env').read()\n"},
			wantTool:         "Bash",
			wantUnclassified: true,
			wantCommand:      "open('.env').read()\n",
			wantHasCommand:   true,
		},
		{
			name:             "manage_task smuggled empty Input alongside status fails closed",
			rawTool:          "manage_task",
			args:             map[string]any{"Action": "status", "TaskId": "task-1", "Input": ""},
			wantTool:         "Bash",
			wantUnclassified: true,
			wantCommand:      "",
			wantHasCommand:   true,
		},
		{
			name:             "manage_task smuggled lowercase input alongside kill fails closed",
			rawTool:          "manage_task",
			args:             map[string]any{"Action": "kill", "TaskId": "task-1", "input": "rm -rf /\n"},
			wantTool:         "Bash",
			wantUnclassified: true,
			wantCommand:      "rm -rf /\n",
			wantHasCommand:   true,
		},
		{
			name:             "manage_task missing Action fails closed",
			rawTool:          "manage_task",
			args:             map[string]any{"TaskId": "task-1"},
			wantTool:         "Bash",
			wantUnclassified: true,
		},
		{
			name:             "manage_task non-string Action fails closed",
			rawTool:          "manage_task",
			args:             map[string]any{"Action": float64(1)},
			wantTool:         "Bash",
			wantUnclassified: true,
		},
		{
			name:             "manage_task unknown Action fails closed",
			rawTool:          "manage_task",
			args:             map[string]any{"Action": "exec", "TaskId": "task-1"},
			wantTool:         "Bash",
			wantUnclassified: true,
		},
		{
			name:     "send_command_input pure terminate without Input stays send_command_input",
			rawTool:  "send_command_input",
			args:     map[string]any{"CommandId": "cmd-1", "Terminate": true},
			wantTool: "send_command_input",
		},
		{
			name:     "send_command_input pure terminate with null Input stays send_command_input",
			rawTool:  "send_command_input",
			args:     map[string]any{"CommandId": "cmd-1", "Terminate": true, "Input": nil},
			wantTool: "send_command_input",
		},
		{
			name:     "send_command_input pure terminate with empty Input stays send_command_input",
			rawTool:  "send_command_input",
			args:     map[string]any{"CommandId": "cmd-1", "Terminate": true, "Input": ""},
			wantTool: "send_command_input",
		},
		{
			name:             "send_command_input terminate with non-empty Input fails closed",
			rawTool:          "send_command_input",
			args:             map[string]any{"CommandId": "cmd-1", "Terminate": true, "Input": "git reset --hard\n"},
			wantTool:         "Bash",
			wantUnclassified: true,
			wantCommand:      "git reset --hard\n",
			wantHasCommand:   true,
		},
		{
			name:             "send_command_input terminate with empty Input and non-empty lowercase input fails closed",
			rawTool:          "send_command_input",
			args:             map[string]any{"CommandId": "cmd-1", "Terminate": true, "Input": "", "input": "cat .env\n"},
			wantTool:         "Bash",
			wantUnclassified: true,
			wantCommand:      "cat .env\n",
			wantHasCommand:   true,
		},
		{
			name:             "send_command_input terminate with non-string Input fails closed",
			rawTool:          "send_command_input",
			args:             map[string]any{"CommandId": "cmd-1", "Terminate": true, "Input": float64(7)},
			wantTool:         "Bash",
			wantUnclassified: true,
		},
		{
			name:             "send_command_input Terminate false fails closed",
			rawTool:          "send_command_input",
			args:             map[string]any{"CommandId": "cmd-1", "Terminate": false, "Input": ""},
			wantTool:         "Bash",
			wantUnclassified: true,
			wantCommand:      "",
			wantHasCommand:   true,
		},
		{
			name:             "send_command_input missing Terminate fails closed",
			rawTool:          "send_command_input",
			args:             map[string]any{"CommandId": "cmd-1", "Input": "open('.env').read()\n"},
			wantTool:         "Bash",
			wantUnclassified: true,
			wantCommand:      "open('.env').read()\n",
			wantHasCommand:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := marshalAntigravityPayload(t, map[string]any{
				"hookEventName": "PreToolUse",
				"toolCall":      map[string]any{"name": tc.rawTool, "args": tc.args},
			})
			event, reject := Normalize("antigravity", "PreToolUse", raw)
			if reject != nil {
				t.Fatalf("unexpected rejection: %s", reject.Stderr)
			}
			doc := readEvent(t, event)
			if doc.Tool.Name == nil || *doc.Tool.Name != tc.wantTool {
				t.Fatalf("tool.name = %v, want %q", doc.Tool.Name, tc.wantTool)
			}
			input := eventInput(t, doc)
			if !reflect.DeepEqual(input["value"], tc.args) {
				t.Errorf("tool.input.value = %#v, want %#v", input["value"], tc.args)
			}
			gotUnclassified, _ := input["unclassified_task_input"].(bool)
			if gotUnclassified != tc.wantUnclassified {
				t.Errorf("unclassified_task_input = %v, want %v (input=%#v)", gotUnclassified, tc.wantUnclassified, input)
			}
			gotCmd, hasCmd := input["command"].(string)
			if hasCmd != tc.wantHasCommand || (tc.wantHasCommand && gotCmd != tc.wantCommand) {
				t.Errorf("command = %q (present=%v), want %q (present=%v)", gotCmd, hasCmd, tc.wantCommand, tc.wantHasCommand)
			}
		})
	}
}
