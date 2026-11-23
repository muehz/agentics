package provider

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
)

type fuzzProviderMapping struct {
	provider string
	event    string
	payload  string
}

var fuzzProviderMappings = []fuzzProviderMapping{
	{provider: "claude-code", event: "PreToolUse", payload: `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"printf seed"}}`},
	{provider: "copilot", event: copilotPreToolUse, payload: `{"toolName":"bash","toolArgs":{"cmd":"printf seed"}}`},
	{provider: "codex", event: "PreToolUse", payload: `{"hook_event_name":"PreToolUse","cwd":"/tmp","tool_name":"exec_command","tool_input":{"command":"printf seed"}}`},
	{provider: "antigravity", event: "PreToolUse", payload: `{"hookEventName":"PreToolUse","cwd":"/tmp","toolCall":{"name":"run_command","args":{"CommandLine":"printf seed"}}}`},
}

var fuzzProviderRegressionSeeds = []struct {
	mapping uint8
	payload string
}{
	{mapping: 2, payload: `{"hook_event_name":"PreToolUse","cwd":"/tmp","tool_name":"apply_patch","tool_input":{"command":"malformed patch","file_path":"forged.txt","path":"also-forged.txt"}}`},
	{mapping: 3, payload: `{"hookEventName":"PreToolUse","workspacePaths":["/tmp"],"toolCall":{"name":"multi_replace_file_content","args":{"TargetFile":"app.txt","ReplacementChunks":[{"TargetContent":"a","ReplacementContent":"b"},"bad-chunk"]}}}`},
	{mapping: 3, payload: `{"toolCall":{"name":"replace_file_content","args":{"TargetFile":"app.txt","Target":"forged","ReplacementContent":"secret"}}}`},
	{mapping: 3, payload: `{"toolCall":{"name":"view_file","args":{"AbsolutePath":"/tmp/safe.txt","Path":"/tmp/.env"}}}`},
	{mapping: 3, payload: `{"toolCall":{"name":"write_to_file","args":{"TargetFile":".husky/pre-commit","CodeContent":""}}}`},
}

func FuzzNormalize(f *testing.F) {
	for mappingID, mapping := range fuzzProviderMappings {
		for outcomeID := uint8(0); outcomeID < 5; outcomeID++ {
			f.Add(uint8(mappingID), []byte(mapping.payload), outcomeID, "seed message")
		}
	}
	for _, seed := range fuzzProviderRegressionSeeds {
		f.Add(seed.mapping, []byte(seed.payload), uint8(0), "seed message")
	}

	f.Fuzz(func(t *testing.T, mappingID uint8, raw []byte, outcomeID uint8, message string) {
		message = strings.ToValidUTF8(message, "\uFFFD")
		mapping := fuzzProviderMappings[int(mappingID)%len(fuzzProviderMappings)]

		firstEvent, firstReject := Normalize(mapping.provider, mapping.event, raw)
		secondEvent, secondReject := Normalize(mapping.provider, mapping.event, raw)
		if !reflect.DeepEqual(firstEvent, secondEvent) || !reflect.DeepEqual(firstReject, secondReject) {
			t.Fatalf("Normalize is nondeterministic for %s/%s and %q", mapping.provider, mapping.event, raw)
		}
		if firstReject != nil {
			assertFuzzFaultShape(t, mapping, *firstReject)
			return
		}

		firstDocument, firstReadErr := firstEvent.Read()
		secondDocument, secondReadErr := secondEvent.Read()
		if !reflect.DeepEqual(firstDocument, secondDocument) || !reflect.DeepEqual(firstReadErr, secondReadErr) {
			t.Fatalf("normalized event read is nondeterministic for %s/%s and %q", mapping.provider, mapping.event, raw)
		}

		outcome := providerFuzzOutcome(outcomeID, message)
		firstRendered := Render(mapping.provider, mapping.event, outcome, nil)
		secondRendered := Render(mapping.provider, mapping.event, outcome, nil)
		if !reflect.DeepEqual(firstRendered, secondRendered) {
			t.Fatalf("Render is nondeterministic for %s/%s/%s", mapping.provider, mapping.event, outcome.Kind())
		}
		assertFuzzRendered(t, mapping, outcome, firstRendered)
	})
}

func providerFuzzOutcome(outcomeID uint8, message string) hookspec.Outcome {
	switch outcomeID % 5 {
	case 0:
		return contract.Pass()
	case 1:
		return contract.Continue()
	case 2:
		return contract.Deny(message)
	case 3:
		return contract.Context(message)
	default:
		return contract.Error(message)
	}
}

func assertFuzzFaultShape(t *testing.T, mapping fuzzProviderMapping, rendered Rendered) {
	t.Helper()
	providerTraits := fuzzAdapter(t, mapping).traits
	if !bytes.Equal(rendered.Stdout, providerTraits.emptyBody) {
		t.Fatalf("%s/%s fault stdout = %q, want %q", mapping.provider, mapping.event, rendered.Stdout, providerTraits.emptyBody)
	}
	if rendered.Exit != providerTraits.errorExit {
		t.Fatalf("%s/%s fault exit = %d, want %d", mapping.provider, mapping.event, rendered.Exit, providerTraits.errorExit)
	}
	if len(rendered.Stderr) == 0 || !bytes.HasSuffix(rendered.Stderr, []byte("\n")) || !utf8.Valid(rendered.Stderr) {
		t.Fatalf("%s/%s fault stderr is not a non-empty newline-terminated UTF-8 diagnostic: %q", mapping.provider, mapping.event, rendered.Stderr)
	}
}

func assertFuzzRendered(t *testing.T, mapping fuzzProviderMapping, outcome hookspec.Outcome, rendered Rendered) {
	t.Helper()
	if outcome.Kind() == contract.KindError {
		assertFuzzFaultShape(t, mapping, rendered)
		return
	}

	wantExit := hookspec.ExitOK
	if mapping.provider == "claude-code" && outcome.Kind() == contract.KindDeny {
		wantExit = hookspec.ExitDeny
	}
	if rendered.Exit != wantExit {
		t.Fatalf("%s/%s/%s exit = %d, want %d", mapping.provider, mapping.event, outcome.Kind(), rendered.Exit, wantExit)
	}
	if !utf8.Valid(rendered.Stdout) || !utf8.Valid(rendered.Stderr) {
		t.Fatalf("%s/%s/%s returned non-UTF-8 output", mapping.provider, mapping.event, outcome.Kind())
	}

	providerTraits := fuzzAdapter(t, mapping).traits
	if outcome.Kind() == contract.KindPass || outcome.Kind() == contract.KindContinue ||
		(mapping.provider == "copilot" && mapping.event == copilotPreToolUse && outcome.Kind() == contract.KindContext) ||
		(mapping.provider == "antigravity" && mapping.event == "PreToolUse" && outcome.Kind() == contract.KindContext) {
		if !bytes.Equal(rendered.Stdout, providerTraits.emptyBody) || len(rendered.Stderr) != 0 {
			t.Fatalf("%s/%s/%s response = stdout %q stderr %q, want the provider empty body and no diagnostic", mapping.provider, mapping.event, outcome.Kind(), rendered.Stdout, rendered.Stderr)
		}
		return
	}

	if mapping.provider == "claude-code" && outcome.Kind() == contract.KindDeny {
		if len(rendered.Stdout) != 0 || len(rendered.Stderr) == 0 || !bytes.HasSuffix(rendered.Stderr, []byte("\n")) {
			t.Fatalf("%s/%s/%s response = stdout %q stderr %q, want stderr-only decision", mapping.provider, mapping.event, outcome.Kind(), rendered.Stdout, rendered.Stderr)
		}
		return
	}

	if len(rendered.Stderr) != 0 {
		t.Fatalf("%s/%s/%s unexpectedly wrote stderr %q", mapping.provider, mapping.event, outcome.Kind(), rendered.Stderr)
	}
	if len(rendered.Stdout) > 0 {
		if !bytes.HasSuffix(rendered.Stdout, []byte("\n")) || !json.Valid(bytes.TrimSpace(rendered.Stdout)) {
			t.Fatalf("%s/%s/%s stdout is not one newline-terminated JSON document: %q", mapping.provider, mapping.event, outcome.Kind(), rendered.Stdout)
		}
	}
	if len(rendered.Stdout) == 0 {
		t.Fatalf("%s/%s/%s omitted its required response document", mapping.provider, mapping.event, outcome.Kind())
	}
}

func TestCodexPatchRegressionSeedsNormalizeExpectedFields(t *testing.T) {
	for _, tc := range []struct {
		name    string
		event   string
		payload string
		want    map[string]any
	}{
		{
			name:    "pre-tool malformed patch discards sibling paths",
			event:   "PreToolUse",
			payload: `{"hook_event_name":"PreToolUse","cwd":"/tmp","tool_name":"apply_patch","tool_input":{"command":"malformed patch","file_path":"forged.txt","path":"also-forged.txt"}}`,
			want: map[string]any{
				"value":             map[string]any{"command": "malformed patch"},
				"command":           "malformed patch",
				"write_parse_error": true,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event, reject := Normalize("codex", tc.event, []byte(tc.payload))
			if reject != nil {
				t.Fatalf("Normalize rejected the retained seed: exit=%d stdout=%q stderr=%q", reject.Exit, reject.Stdout, reject.Stderr)
			}

			document := readEvent(t, event)
			if document.Tool.Name == nil || *document.Tool.Name != "Edit" {
				t.Fatalf("tool.name = %v, want apply_patch mapped to Edit", document.Tool.Name)
			}
			if got := eventInput(t, document); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("normalized tool input = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// fuzzAdapter resolves the mapping's provider through the same lookup the
// entry points use. A miss fails the test by name rather than reading a zero
// adapter, which would compare every response against an empty body.
func fuzzAdapter(t *testing.T, mapping fuzzProviderMapping) adapter {
	t.Helper()
	a, ok := adapterFor(mapping.provider)
	if !ok {
		t.Fatalf("provider %q has no registered adapter", mapping.provider)
	}
	return a
}
