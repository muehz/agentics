package provider

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// ---------------------------------------------------------------------------
// Payload decoding
// ---------------------------------------------------------------------------

// TestDecodeJSONDocument pins the two properties every caller depends on: a
// non-whitespace tail is rejected as trailing data, and number literals
// survive as literals. The literal check matters because the contract splits
// on the literal's text — a token with '.', 'e' or 'E' is a float that must
// be finite, everything else is an integer of any length.
func TestDecodeJSONDocument(t *testing.T) {
	t.Run("rejects trailing data", func(t *testing.T) {
		tails := map[string]string{
			"second document": `{"a":1} {}`,
			"close brace":     `{"a":1} }`,
			"close bracket":   `{"a":1} ]`,
			"bare word":       `{"a":1} tail`,
		}
		for name, payload := range tails {
			t.Run(name, func(t *testing.T) {
				if _, err := decodeJSONDocument([]byte(payload)); !errors.Is(err, errTrailingData) {
					t.Fatalf("err = %v, want errTrailingData", err)
				}
			})
		}
	})

	t.Run("accepts JSON whitespace around the document", func(t *testing.T) {
		value, err := decodeJSONDocument([]byte(" \t\r\n{\"a\":1}\n \t\r"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, isObject := value.(map[string]any); !isObject {
			t.Fatalf("value = %#v, want an object", value)
		}
	})

	t.Run("preserves number literals", func(t *testing.T) {
		bigInteger := "1" + strings.Repeat("0", 400)
		for _, literal := range []string{"1", "1.0", "1e999", "-0", bigInteger} {
			value, err := decodeJSONDocument([]byte(`{"n":` + literal + `}`))
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", literal, err)
			}
			number, isNumber := value.(map[string]any)["n"].(json.Number)
			if !isNumber {
				t.Fatalf("%s: n is %#v, want json.Number", literal, value.(map[string]any)["n"])
			}
			if number.String() != literal {
				t.Errorf("n = %s, want %s", number.String(), literal)
			}
		}
	})

	t.Run("reports a parse failure as itself", func(t *testing.T) {
		_, err := decodeJSONDocument([]byte(`{nope`))
		if err == nil {
			t.Fatal("expected a parse error")
		}
		if errors.Is(err, errTrailingData) {
			t.Fatal("a parse failure must not be reported as trailing data")
		}
	})
}

// TestDecodePayload pins the three shared checks and their wording.
func TestDecodePayload(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string // exact error text, or a prefix when it ends in ": "
	}{
		{"invalid UTF-8", "{\"a\":\"\xff\"}", "payload is not valid UTF-8"},
		{"not JSON", `{nope`, "payload is not valid JSON: "},
		{"trailing data", `{} {}`, "payload is not valid JSON: trailing data after the JSON document"},
		{"array", `[]`, errPayloadNotObject.Error()},
		{"null", `null`, errPayloadNotObject.Error()},
		{"string", `"x"`, errPayloadNotObject.Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodePayload([]byte(tc.raw))
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.HasSuffix(tc.want, ": ") {
				if !strings.HasPrefix(err.Error(), tc.want) {
					t.Errorf("err = %q, want prefix %q", err, tc.want)
				}
				return
			}
			if err.Error() != tc.want {
				t.Errorf("err = %q, want %q", err, tc.want)
			}
		})
	}
	payload, err := decodePayload([]byte(`{"n":1.0}`))
	if err != nil {
		t.Fatal(err)
	}
	if number, ok := payload["n"].(json.Number); !ok || number.String() != "1.0" {
		t.Errorf("n = %#v, want the literal 1.0", payload["n"])
	}
}

// TestMalformedPayloadWordingIsShared proves every surface words the three
// shared rejections the same way behind its own prefix and shape.
func TestMalformedPayloadWordingIsShared(t *testing.T) {
	surfaces := []struct {
		traits traits
		event  string
	}{
		{claudeTraits, "PreToolUse"},
		{copilotTraits, copilotPreToolUse},
		{codexTraits, "PreToolUse"},
		{antigravityTraits, "PreToolUse"},
	}
	for _, s := range surfaces {
		t.Run(s.traits.name, func(t *testing.T) {
			prefix := s.traits.name + ": malformed " + s.event + " payload: "
			_, reject := Normalize(s.traits.name, s.event, []byte(`{} {}`))
			if reject == nil {
				t.Fatal("trailing data was accepted")
			}
			if want := prefix + "payload is not valid JSON: trailing data after the JSON document\n"; string(reject.Stderr) != want {
				t.Errorf("stderr = %q, want %q", reject.Stderr, want)
			}
			_, reject = Normalize(s.traits.name, s.event, []byte(`[]`))
			if reject == nil {
				t.Fatal("a non-object was accepted")
			}
			if want := prefix + "expected " + s.traits.label + " " + s.event + " payload\n"; string(reject.Stderr) != want {
				t.Errorf("stderr = %q, want %q", reject.Stderr, want)
			}
			_, reject = Normalize(s.traits.name, s.event, []byte("{\"a\":\"\xff\"}"))
			if reject == nil {
				t.Fatal("invalid UTF-8 was accepted")
			}
			if want := prefix + "payload is not valid UTF-8\n"; string(reject.Stderr) != want {
				t.Errorf("stderr = %q, want %q", reject.Stderr, want)
			}
			if string(reject.Stdout) != string(s.traits.emptyBody) || reject.Exit != s.traits.errorExit {
				t.Errorf("shape = %q/%d, want %q/%d", reject.Stdout, reject.Exit, s.traits.emptyBody, s.traits.errorExit)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Document emission
// ---------------------------------------------------------------------------

// A response document is written to stdout verbatim, so its bytes matter:
// no HTML escaping and exactly one newline.
func TestEncodeDocument(t *testing.T) {
	document, err := encodeDocument(map[string]string{"m": "a <b> & c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "{\"m\":\"a <b> & c\"}\n"
	if string(document) != want {
		t.Fatalf("document = %q, want %q", document, want)
	}
}

func TestDecodedComponentsPreserveCanonicalInput(t *testing.T) {
	t.Setenv("AGX_TEST_PLUGIN_ROOT", "/plugin")
	t.Setenv("AGX_TEST_PLUGIN_DATA", "")
	t.Setenv("AGX_VAULT", "")

	event, err := neutralEvent(contract.EventParts{
		Surface:   surface.Codex,
		Runtime:   runtimeRoots("AGX_TEST_PLUGIN_ROOT", "AGX_TEST_PLUGIN_DATA"),
		ToolName:  "Bash",
		ToolInput: map[string]any{"value": "a <b> & c"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fields := readEvent(t, event)
	if got, want := string(fields.Tool.InputRaw), `{"value":"a <b> & c"}`; got != want {
		t.Fatalf("input = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Runtime roots
// ---------------------------------------------------------------------------

// runtimeCases are the environment rules runtimeRoots keeps: an
// exported-but-empty root is null exactly as an unset one is, and log_root is
// the data root when there is one, else the plugin root.
var runtimeCases = []struct {
	name       string
	unset      bool
	pluginRoot string
	dataRoot   string
	vault      string
	want       map[string]any
}{
	{name: "all unset", unset: true,
		want: map[string]any{"plugin_root": nil, "data_root": nil, "vault_override": nil, "log_root": nil}},
	{name: "empty is null, exactly as unset is",
		want: map[string]any{"plugin_root": nil, "data_root": nil, "vault_override": nil, "log_root": nil}},
	{name: "log_root falls back to plugin_root", pluginRoot: "/plugin",
		want: map[string]any{"plugin_root": "/plugin", "data_root": nil, "vault_override": nil, "log_root": "/plugin"}},
	{name: "log_root prefers data_root", pluginRoot: "/plugin", dataRoot: "/data", vault: "/vault",
		want: map[string]any{"plugin_root": "/plugin", "data_root": "/data", "vault_override": "/vault", "log_root": "/data"}},
	{name: "data_root alone still wins log_root", dataRoot: "/data",
		want: map[string]any{"plugin_root": nil, "data_root": "/data", "vault_override": nil, "log_root": "/data"}},
}

// setRuntimeCase exports one case's variables, or unsets them all.
func setRuntimeCase(t *testing.T, pluginRoot, dataRoot, vault string, unset bool) {
	t.Helper()
	names := []string{"AGX_TEST_PLUGIN_ROOT", "AGX_TEST_PLUGIN_DATA", "AGX_VAULT"}
	t.Setenv(names[0], pluginRoot)
	t.Setenv(names[1], dataRoot)
	t.Setenv(names[2], vault)
	if unset {
		for _, name := range names {
			if err := os.Unsetenv(name); err != nil {
				t.Fatalf("unsetting %s: %v", name, err)
			}
		}
	}
}

// TestRuntimeRoots pins the environment rules on runtimeRoots.
func TestRuntimeRoots(t *testing.T) {
	for _, tc := range runtimeCases {
		t.Run(tc.name, func(t *testing.T) {
			setRuntimeCase(t, tc.pluginRoot, tc.dataRoot, tc.vault, tc.unset)
			runtime := runtimeRoots("AGX_TEST_PLUGIN_ROOT", "AGX_TEST_PLUGIN_DATA")
			got := map[string]*string{
				"plugin_root":    runtime.PluginRoot,
				"data_root":      runtime.DataRoot,
				"vault_override": runtime.VaultOverride,
				"log_root":       runtime.LogRoot,
			}
			for key, want := range tc.want {
				value := got[key]
				switch {
				case want == nil && value != nil:
					t.Errorf("%s = %q, want nil", key, *value)
				case want != nil && value == nil:
					t.Errorf("%s = nil, want %q", key, want)
				case want != nil && *value != want.(string):
					t.Errorf("%s = %q, want %q", key, *value, want)
				}
			}
		})
	}
}

// TestCopilotStringToolArgsDecoded pins #685: Copilot's toolArgs is decoded
// before it reaches tool.input.value, because the guards read commands and
// paths from that value. A string that does not parse as JSON is kept whole.
func TestCopilotStringToolArgsDecoded(t *testing.T) {
	t.Run("decodes a JSON object", func(t *testing.T) {
		event, reject := Normalize("copilot", copilotPreToolUse, []byte(`{"toolName":"view","toolArgs":"{\"path\":\"/tmp/f\"}"}`))
		if reject != nil {
			t.Fatalf("rejected: %s", reject.Stderr)
		}
		input := eventInput(t, readEvent(t, event))
		want := map[string]any{"value": map[string]any{"path": "/tmp/f"}}
		if !reflect.DeepEqual(input, want) {
			t.Errorf("input = %#v, want %#v", input, want)
		}
	})

	t.Run("keeps a non-JSON string", func(t *testing.T) {
		event, reject := Normalize("copilot", copilotPreToolUse, []byte(`{"toolName":"bash","toolArgs":"{not json"}`))
		if reject != nil {
			t.Fatalf("rejected: %s", reject.Stderr)
		}
		input := eventInput(t, readEvent(t, event))
		want := map[string]any{"value": "{not json"}
		if !reflect.DeepEqual(input, want) {
			t.Errorf("input = %#v, want %#v", input, want)
		}
	})
}

// TestCopilotApplyPatchNormalized pins #686: Copilot's apply_patch reads its
// patch text the way Copilot CLI's own extractor does (1.0.88) — raw string
// toolArgs, else an object's input, else its patch — and replaces
// tool.input.value with the derived writes alone, so a sibling command or
// path in the arguments is never lifted beside them. A patch the grammar
// rejects denies through write_parse_error rather than surfacing partial
// content.
func TestCopilotApplyPatchNormalized(t *testing.T) {
	patch := "*** Begin Patch\n*** Add File: app.txt\n+new\n*** End Patch\n"
	want := map[string]any{"value": map[string]any{
		"files": []any{map[string]any{"path": "app.txt", "content": "new\n"}},
	}}

	for _, tc := range []struct {
		name     string
		toolArgs any
	}{
		{"raw string toolArgs", patch},
		{"object with input", map[string]any{"input": patch, "command": "sibling", "path": "sibling.txt"}},
		{"object with patch", map[string]any{"patch": patch, "command": "sibling", "path": "sibling.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{"toolName": "apply_patch", "toolArgs": tc.toolArgs})
			if err != nil {
				t.Fatal(err)
			}
			event, reject := Normalize("copilot", copilotPreToolUse, payload)
			if reject != nil {
				t.Fatalf("rejected: %s", reject.Stderr)
			}
			doc := readEvent(t, event)
			if doc.Tool.Name == nil || *doc.Tool.Name != "Edit" {
				t.Fatalf("tool.name = %v, want the apply_patch->Edit mapping", doc.Tool.Name)
			}
			input := eventInput(t, doc)
			if !reflect.DeepEqual(input, want) {
				t.Fatalf("input = %#v, want %#v", input, want)
			}
		})
	}

	t.Run("malformed patch sets write_parse_error", func(t *testing.T) {
		payload, err := json.Marshal(map[string]any{
			"toolName": "apply_patch", "toolArgs": "*** Begin Patch\n*** Add File: app.txt",
		})
		if err != nil {
			t.Fatal(err)
		}
		event, reject := Normalize("copilot", copilotPreToolUse, payload)
		if reject != nil {
			t.Fatalf("rejected: %s", reject.Stderr)
		}
		input := eventInput(t, readEvent(t, event))
		want := map[string]any{"value": map[string]any{}, "write_parse_error": true}
		if !reflect.DeepEqual(input, want) {
			t.Fatalf("input = %#v, want %#v", input, want)
		}
	})
}

// TestGroupedContextOnStderr pins which surface shows a grouped event's joined
// context on stderr. Antigravity has no context channel either, yet its
// grouped context stays off stderr, so the answer is a provider trait and not
// a property of the surface list.
func TestGroupedContextOnStderr(t *testing.T) {
	for name, want := range map[string]bool{
		string(surface.Antigravity): false,
		string(surface.ClaudeCode):  false,
		string(surface.Codex):       false,
		string(surface.Copilot):     true,
		"bogus":                     false,
	} {
		if got := GroupedContextOnStderr(name); got != want {
			t.Errorf("GroupedContextOnStderr(%q) = %v, want %v", name, got, want)
		}
	}
}
