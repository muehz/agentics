package provider

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
)

// An invalid outcome cannot occur on the wired path (cmd/hook validates
// first), but the render table still falls into the error branch — the
// fail-open direction — rather than any objection-shaped response.
func TestCopilotRenderTreatsInvalidOutcomeAsError(t *testing.T) {
	got := copilotRender(copilotPreToolUse, hookspec.Outcome{}, nil)
	if got.Exit != hookspec.ExitOK {
		t.Errorf("exit = %d, want %d — copilot never fails closed", got.Exit, hookspec.ExitOK)
	}
	if len(got.Stdout) != 0 {
		t.Errorf("stdout = %q, want empty", got.Stdout)
	}
	if want := copilotTraits.failedMessage + "\n"; string(got.Stderr) != want {
		t.Errorf("stderr = %q, want the last-resort diagnostic %q", got.Stderr, want)
	}
}

func readEvent(t *testing.T, event hookspec.Event) contract.NeutralEvent {
	t.Helper()
	doc, err := event.Read()
	if err != nil {
		t.Fatalf("normalized event is invalid: %v", err)
	}
	return doc
}

func eventInput(t *testing.T, doc contract.NeutralEvent) map[string]any {
	t.Helper()
	input, err := doc.Tool.DecodeInput()
	if err != nil {
		t.Fatalf("tool input is invalid: %v", err)
	}
	return input
}

// With no data root the log root falls back to the plugin root, and empty
// environment values collapse to null.
func TestCopilotNormalizeRuntimeFallsBackToPluginRoot(t *testing.T) {
	t.Setenv("COPILOT_PLUGIN_ROOT", "/roots/plugin")
	t.Setenv("COPILOT_PLUGIN_DATA", "")
	t.Setenv("AGX_VAULT", "")

	event, reject := Normalize("copilot", copilotPreToolUse, []byte(`{"toolName":"view","toolArgs":{"path":"/tmp/f"}}`))
	if reject != nil {
		t.Fatalf("rejected: %s", reject.Stderr)
	}
	doc := readEvent(t, event)
	if doc.SessionID != nil || doc.Cwd != nil {
		t.Errorf("session/cwd = %v/%v, want null/null", doc.SessionID, doc.Cwd)
	}
	if name := doc.Tool.Name; name == nil || *name != "Read" {
		t.Errorf("tool.name = %v, want the view->Read mapping", name)
	}
	runtime := doc.Runtime
	if runtime.DataRoot != nil || runtime.VaultOverride != nil {
		t.Errorf("empty env must normalize to null, got %v", runtime)
	}
	if runtime.LogRoot == nil || *runtime.LogRoot != "/roots/plugin" {
		t.Errorf("log_root = %v, want the plugin-root fallback", runtime.LogRoot)
	}
}

// A non-object toolArgs is kept whole; nothing is lifted from it.
func TestCopilotNormalizeKeepsNonObjectToolArgs(t *testing.T) {
	event, reject := Normalize("copilot", copilotPreToolUse, []byte(`{"toolName":"bash","toolArgs":[1,2]}`))
	if reject != nil {
		t.Fatalf("rejected: %s", reject.Stderr)
	}
	input := eventInput(t, readEvent(t, event))
	if !reflect.DeepEqual(input, map[string]any{"value": []any{float64(1), float64(2)}}) {
		t.Errorf("input = %#v, want the array under value alone", input)
	}
}

// sessionId, turnId and timestamp are not required on preToolUse. cwd is still
// carried onto the normalized event, so handlers can locate the repository.
func TestCopilotNormalizePreToolUseWithoutOptionalSessionFields(t *testing.T) {
	payload := `{"cwd":"/tmp/repo","toolName":"create","toolArgs":{}}`
	event, reject := Normalize("copilot", copilotPreToolUse, []byte(payload))
	if reject != nil {
		t.Fatalf("rejected: %s", reject.Stderr)
	}
	doc := readEvent(t, event)
	if doc.SessionID != nil || doc.TurnID != nil || doc.Cwd == nil || *doc.Cwd != "/tmp/repo" {
		t.Errorf("session/turn/cwd = %v/%v/%v, want null/null//tmp/repo",
			doc.SessionID, doc.TurnID, doc.Cwd)
	}
}

func TestCopilotNormalizePreservesPowerShellDialect(t *testing.T) {
	event, reject := Normalize("copilot", copilotPreToolUse, []byte(`{"toolName":"powershell","toolArgs":{"command":"& 'C:\\Program Files\\kubectl\\kubectl.exe' apply"}}`))
	if reject != nil {
		t.Fatalf("rejected: %s", reject.Stderr)
	}
	doc := readEvent(t, event)
	if name := doc.Tool.Name; name == nil || *name != "PowerShell" {
		t.Fatalf("tool.name = %v, want PowerShell so backticks and backslashes retain PowerShell semantics", name)
	}
	input, err := doc.Tool.DecodeInput()
	if err != nil {
		t.Fatalf("tool.input = %v, want object", err)
	}
	if command := input["command"]; command != `& 'C:\Program Files\kubectl\kubectl.exe' apply` {
		t.Fatalf("tool.input.command = %v", command)
	}
}

// A valid patch that adds no text writes no file, and says so with an empty
// file list: never a null list, never an absent key, and never a parse error.
func TestCopilotApplyPatchWithoutAddedTextHasEmptyFileList(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"toolName": "apply_patch", "toolArgs": "*** Begin Patch\n*** Delete File: x\n*** End Patch\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	event, reject := Normalize("copilot", copilotPreToolUse, payload)
	if reject != nil {
		t.Fatalf("rejected: %s", reject.Stderr)
	}
	input := eventInput(t, readEvent(t, event))
	want := map[string]any{"value": map[string]any{"files": []any{}}}
	if !reflect.DeepEqual(input, want) {
		t.Fatalf("input = %#v, want %#v", input, want)
	}
}
