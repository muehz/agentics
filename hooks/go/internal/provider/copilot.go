// The Copilot CLI surface adapter. Its protocol is in
// docs/surfaces/copilot-cli.md, Hook protocol: any non-zero preToolUse exit is
// a deny, so a deny is a flat permissionDecision document on exit 0 and every
// fault exits 0 with its diagnostic on stderr.
//
// Only preToolUse is served: no registered hook maps to any other event.
package provider

import (
	"fmt"

	"strings"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/surface"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

// copilotTraits: every fault exits 0 with no body, so a hook fault never
// becomes a deny the host attributes to an errored hook.
var copilotTraits = traits{
	name:          string(surface.Copilot),
	label:         surface.Copilot.Label(),
	emptyBody:     nil,
	errorExit:     hookspec.ExitOK,
	failedMessage: "Copilot hook adapter failed",

	// Copilot renders a context outcome as no document, so the grouped event
	// shows the joined context on stderr, while the single check stays silent.
	groupedContextOnStderr: true,
}

// copilotPreToolUse is Copilot's tool-use event. Copilot's event names are
// camelCase, unlike the other surfaces.
var copilotPreToolUse = surface.Copilot.PreToolEvent()

// copilotToolNames maps Copilot's tool names to the neutral vocabulary the
// cores reason about (docs/surfaces/copilot-cli.md, Hook protocol). Lookup
// is exact: Copilot emits lowercase names, and an unknown name passes through
// unchanged.
var copilotToolNames = map[string]string{
	"bash":               "Bash",
	"powershell":         "PowerShell",
	"view":               "Read",
	"create":             "Write",
	"edit":               "Edit",
	"str_replace_editor": "Edit",
	"apply_patch":        "Edit",
	"grep":               "Grep",
	"rg":                 "Grep",
	"glob":               "Glob",
	"web_fetch":          "WebFetch",
	"web_search":         "WebSearch",
	"ask_user":           "AskUserQuestion",
	"update_todo":        "TodoWrite",
	"task":               "Agent",
}

// copilotEvents is the set of native event names the adapter accepts. It
// holds exactly one, copilotPreToolUse, because no registered hook serves any
// other event.
var copilotEvents = map[string]struct{}{
	copilotPreToolUse: {},
}

func init() {
	register(surface.Copilot, adapter{
		traits:    copilotTraits,
		normalize: copilotNormalize,
		render:    copilotRender,
	})
}

// copilotNormalize builds the neutral event parts from a Copilot payload.
//
// toolName must be a string and toolArgs present. toolArgs is decoded when it
// is a JSON string, and kept whole under tool.input.value. On preToolUse,
// apply_patch instead replaces tool.input.value with its derived writes (see
// copilotPatchInput) and every other tool lifts the command (cmd before
// command for the shell tools, command before cmd otherwise) when the
// arguments are an object. sessionId, turnId and cwd are optional. No other
// field is read.
func copilotNormalize(providerEvent string, payload map[string]any) (contract.EventParts, error) {
	_, ok := copilotEvents[providerEvent]
	if !ok {
		return contract.EventParts{}, fmt.Errorf("expected Copilot %s payload", providerEvent)
	}
	toolName, err := requiredString(copilotTraits, payload, "toolName")
	if err != nil {
		return contract.EventParts{}, err
	}
	toolArgs, present := payload["toolArgs"]
	if !present {
		return contract.EventParts{}, fieldError(copilotTraits, "toolArgs", "present")
	}

	args := toolArgs
	if text, isString := toolArgs.(string); isString {
		if value, err := decodeJSONDocument([]byte(text)); err == nil {
			args = value
		}
	}
	toolInput := map[string]any{"value": args}
	if providerEvent == copilotPreToolUse && toolName == "apply_patch" {
		value, parseError := copilotPatchInput(args)
		toolInput = map[string]any{"value": value}
		if parseError {
			toolcall.MarkWriteParseError(toolInput)
		}
	} else if object, isObject := args.(map[string]any); isObject {
		if command, found := copilotCommand(toolName, object); found {
			toolInput["command"] = command
		}
	}

	sessionID, err := optionalString(copilotTraits, payload, "sessionId")
	if err != nil {
		return contract.EventParts{}, err
	}
	cwd, err := optionalString(copilotTraits, payload, "cwd")
	if err != nil {
		return contract.EventParts{}, err
	}
	turnID, err := optionalString(copilotTraits, payload, "turnId")
	if err != nil {
		return contract.EventParts{}, err
	}

	if canonical, known := copilotToolNames[toolName]; known {
		toolName = canonical
	}
	return contract.EventParts{
		Surface:   surface.Copilot,
		SessionID: sessionID,
		TurnID:    turnID,
		Cwd:       cwd,
		Runtime:   runtimeRoots("COPILOT_PLUGIN_ROOT", "COPILOT_PLUGIN_DATA"),
		ToolName:  toolName,
		ToolInput: toolInput,
	}, nil
}

// copilotCommand finds the command in a tool's arguments. The shell tools
// spell it cmd, so cmd is read before command for them and after it for every
// other tool.
func copilotCommand(toolName string, args map[string]any) (string, bool) {
	keys := []string{"command", "cmd"}
	if native := strings.ToLower(toolName); native == "bash" || native == "powershell" {
		keys = []string{"cmd", "command"}
	}
	for _, key := range keys {
		if command, isString := args[key].(string); isString {
			return command, true
		}
	}
	return "", false
}

// copilotPatchInput reads the patch text where Copilot CLI's own apply_patch
// extractor does (1.0.88): the arguments as a string, else their input or
// patch string. Only the derived writes are kept, as for Codex, so a sibling
// field cannot redirect or exempt them.
func copilotPatchInput(args any) (any, bool) {
	text, ok := args.(string)
	if object, isObject := args.(map[string]any); isObject {
		if text, ok = object["input"].(string); !ok {
			text, ok = object["patch"].(string)
		}
	}
	if !ok {
		return map[string]any{}, true
	}
	writes, valid := parsePatchWrites(text)
	if !valid {
		return map[string]any{}, true
	}
	input := map[string]any{}
	toolcall.SetFileWrites(input, writes)
	return input, false
}

// copilotPermissionDecision is the preToolUse deny document. Field
// order is declaration order.
type copilotPermissionDecision struct {
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// copilotRender renders one outcome for Copilot. Every branch exits 0. A deny
// is a permissionDecision document on preToolUse. Context produces no
// document, preserving the host's normal permission flow.
func copilotRender(providerEvent string, outcome hookspec.Outcome, coreStderr []byte) Rendered {
	switch outcome.Kind() {
	default:
		return renderError(copilotTraits, outcome.Payload(), coreStderr)
	case contract.KindError:
		return renderError(copilotTraits, outcome.Payload(), coreStderr)
	case contract.KindPass, contract.KindContinue:
		return renderPass(copilotTraits)

	case contract.KindDeny:
		if providerEvent != copilotPreToolUse {
			return rejectInvalidOutcome(copilotTraits, providerEvent, contract.KindDeny)
		}
		document, err := encodeDocument(copilotPermissionDecision{
			PermissionDecision:       "deny",
			PermissionDecisionReason: outcome.Payload(),
		})
		if err != nil {
			break // unreachable encode failure: fail into the error branch
		}
		return Rendered{Stdout: document, Exit: hookspec.ExitOK}

	case contract.KindContext:
		return renderPass(copilotTraits)
	}
	return renderError(copilotTraits, outcome.Payload(), coreStderr)
}
