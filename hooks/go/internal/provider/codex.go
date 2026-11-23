// The Codex surface adapter. Its protocol is in docs/surfaces/codex.md, Hook
// protocol: Codex reads a decision from the response document, so Agentics
// always emits one — `{}` when it has nothing to say — renders a deny as
// hookSpecificOutput.permissionDecision on exit 0, and never uses exit 2.
//
// Only PreToolUse is served: no registered hook maps to any other event.
package provider

import (
	"fmt"

	"strings"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/surface"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

// codexTraits: every fault exits 0 after `{}`, so a fault is never read as a
// block and Codex always has a document to read.
var codexTraits = traits{
	name:          string(surface.Codex),
	label:         surface.Codex.Label(),
	emptyBody:     []byte("{}\n"),
	errorExit:     hookspec.ExitOK,
	failedMessage: "Codex hook adapter failed",
}

// codexToolNames maps Codex's tool names to the neutral vocabulary the cores
// reason about (docs/surfaces/codex.md, Hook protocol). An unknown name
// passes through unchanged.
var codexToolNames = map[string]string{
	"apply_patch":  "Edit",
	"exec_command": "Bash",
}

// codexEvents is the set of native event names the adapter accepts. It
// holds exactly one, the surface's tool-use event, because no registered hook
// serves any other event.
var codexEvents = map[string]struct{}{
	surface.Codex.PreToolEvent(): {},
}

func init() {
	register(surface.Codex, adapter{
		traits:    codexTraits,
		normalize: codexNormalize,
		render:    codexRender,
	})
}

// codexNormalize builds the neutral event parts from a Codex payload.
//
// hook_event_name must name the event being served. The fields a core reads
// are required: tool_name and cwd as strings and tool_input present. Everything
// else Codex sends — session_id, turn_id, transcript_path, model,
// permission_mode, tool_use_id, agent_id, agent_type — is optional: null when
// absent, an error when present and not a string. Requiring more would turn a
// new Codex field value into a rejected payload, and Codex fails open on a
// rejection, so the guards would silently stop guarding.
//
// tool_input is kept whole under tool.input.value with its command lifted
// beside it. No check reads permission_mode, so it is only walked for NUL
// when the event is read, as it always has been.
func codexNormalize(providerEvent string, payload map[string]any) (contract.EventParts, error) {
	if name, _ := payload["hook_event_name"].(string); name != providerEvent {
		return contract.EventParts{}, fmt.Errorf("expected Codex %s payload", providerEvent)
	}
	_, ok := codexEvents[providerEvent]
	if !ok {
		return contract.EventParts{}, fmt.Errorf("expected Codex %s payload", providerEvent)
	}

	toolName, err := requiredString(codexTraits, payload, "tool_name")
	if err != nil {
		return contract.EventParts{}, err
	}
	cwd, err := requiredString(codexTraits, payload, "cwd")
	if err != nil {
		return contract.EventParts{}, err
	}
	rawToolInput, present := payload["tool_input"]
	if !present {
		return contract.EventParts{}, fieldError(codexTraits, "tool_input", "present")
	}

	optional := map[string]*string{}
	for _, name := range []string{
		"session_id", "turn_id", "transcript_path", "model", "permission_mode",
		"tool_use_id", "agent_id", "agent_type",
	} {
		value, err := optionalString(codexTraits, payload, name)
		if err != nil {
			return contract.EventParts{}, err
		}
		optional[name] = value
	}

	normalizedToolInput, patchParseError := normalizeCodexToolInput(providerEvent, toolName, rawToolInput)
	toolInput := map[string]any{"value": normalizedToolInput}
	if patchParseError {
		toolcall.MarkWriteParseError(toolInput)
	}
	inputObject, inputIsObject := normalizedToolInput.(map[string]any)
	if inputIsObject {
		if command, isString := inputObject["command"].(string); isString {
			toolInput["command"] = command
		}
	}

	if mapped, known := codexToolNames[toolName]; known {
		toolName = mapped
	}
	return contract.EventParts{
		Surface:   surface.Codex,
		SessionID: optional["session_id"],
		TurnID:    optional["turn_id"],
		Cwd:       &cwd,
		Runtime:   runtimeRoots("PLUGIN_ROOT", "PLUGIN_DATA"),
		ToolName:  toolName,
		ToolInput: toolInput,
		NULOnly:   []*string{optional["permission_mode"]},
	}, nil
}

// ---------------------------------------------------------------------------
// apply_patch
// ---------------------------------------------------------------------------

type codexPatchWriteState struct {
	path               string
	kind               string
	hasHunk            bool
	hasBody            bool
	currentHunkHasBody bool
	moved              bool
	added              []string
}

// parsePatchWrites validates the apply_patch grammar and returns only
// added/replacement text. Removed and context lines stay out of the write
// content because they do not land on disk. Both Codex's apply_patch and
// Copilot CLI's apply_patch (docs/surfaces/copilot-cli.md) share this
// grammar, so this parser serves both surfaces.
func parsePatchWrites(command string) ([]toolcall.FileWrite, bool) {
	lines := strings.Split(strings.ReplaceAll(command, "\r", ""), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 || lines[0] != "*** Begin Patch" || lines[len(lines)-1] != "*** End Patch" {
		return nil, false
	}

	var states []*codexPatchWriteState
	active, movable := -1, -1
	environmentSeen := false
	hasOperation := false
	emptyUpdateHunk := func() bool {
		if active < 0 {
			return false
		}
		state := states[active]
		return state.kind == "update" && state.hasHunk && !state.currentHunkHasBody
	}
	validPath := func(value string) bool { return value != "" && !strings.Contains(value, "\x00") }

	for _, line := range lines[1 : len(lines)-1] {
		switch {
		case strings.HasPrefix(line, "*** Environment ID: "):
			if environmentSeen || hasOperation {
				return nil, false
			}
			environmentID := line[len("*** Environment ID: "):]
			if !validPath(environmentID) {
				return nil, false
			}
			environmentSeen = true

		case strings.HasPrefix(line, "*** Add File: "):
			if emptyUpdateHunk() {
				return nil, false
			}
			filePath := line[len("*** Add File: "):]
			if !validPath(filePath) {
				return nil, false
			}
			states = append(states, &codexPatchWriteState{path: filePath, kind: "add"})
			hasOperation = true
			active, movable = len(states)-1, -1

		case strings.HasPrefix(line, "*** Update File: "):
			if emptyUpdateHunk() {
				return nil, false
			}
			filePath := line[len("*** Update File: "):]
			if !validPath(filePath) {
				return nil, false
			}
			states = append(states, &codexPatchWriteState{path: filePath, kind: "update"})
			hasOperation = true
			active, movable = len(states)-1, len(states)-1

		case strings.HasPrefix(line, "*** Delete File: "):
			if emptyUpdateHunk() || !validPath(line[len("*** Delete File: "):]) {
				return nil, false
			}
			hasOperation = true
			active, movable = -1, -1

		case strings.HasPrefix(line, "*** Move to: "):
			destination := line[len("*** Move to: "):]
			if movable < 0 || !validPath(destination) {
				return nil, false
			}
			states[movable].path = destination
			states[movable].moved = true
			movable = -1

		case line == "@@" || strings.HasPrefix(line, "@@ "):
			if active < 0 || states[active].kind != "update" || emptyUpdateHunk() {
				return nil, false
			}
			states[active].hasHunk = true
			states[active].currentHunkHasBody = false
			movable = -1

		case line == "*** End of File":
			if active < 0 || states[active].kind != "update" || emptyUpdateHunk() {
				return nil, false
			}
			active, movable = -1, -1

		case strings.HasPrefix(line, "*** "):
			return nil, false

		default:
			movable = -1
			if active < 0 {
				return nil, false
			}
			state := states[active]
			if state.kind == "add" {
				if !strings.HasPrefix(line, "+") {
					return nil, false
				}
				state.added = append(state.added, line[1:])
				continue
			}
			if !state.hasHunk || (line != "" &&
				!strings.HasPrefix(line, " ") &&
				!strings.HasPrefix(line, "+") &&
				!strings.HasPrefix(line, "-")) {
				return nil, false
			}
			state.hasBody = true
			state.currentHunkHasBody = true
			if strings.HasPrefix(line, "+") {
				state.added = append(state.added, line[1:])
			}
		}
	}
	if emptyUpdateHunk() {
		return nil, false
	}
	if !hasOperation {
		return nil, false
	}

	writes := make([]toolcall.FileWrite, 0, len(states))
	for _, state := range states {
		if state.kind == "update" && !(state.moved || (state.hasHunk && state.hasBody)) {
			return nil, false
		}
		if len(state.added) == 0 {
			continue
		}
		writes = append(writes, toolcall.FileWrite{
			Path: state.path, Content: strings.Join(state.added, "\n") + "\n",
		})
	}
	return writes, true
}

// normalizeCodexToolInput rebuilds apply_patch input from its command. Raw
// sibling fields are intentionally discarded so they cannot redirect or
// exempt the derived writes.
func normalizeCodexToolInput(providerEvent, toolName string, raw any) (any, bool) {
	if providerEvent != surface.Codex.PreToolEvent() || toolName != "apply_patch" {
		return raw, false
	}
	rawObject, ok := raw.(map[string]any)
	if !ok {
		return raw, true
	}
	command, ok := rawObject["command"].(string)
	if !ok {
		return map[string]any{}, true
	}
	object := map[string]any{"command": command}
	writes, ok := parsePatchWrites(command)
	if !ok {
		return object, true
	}
	toolcall.SetFileWrites(object, writes)
	return object, false
}

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------

// codexHookSpecificDocument wraps the two response bodies below.
type codexHookSpecificDocument struct {
	HookSpecificOutput any `json:"hookSpecificOutput"`
}

// codexContextBody is the additionalContext document. Field order is
// declaration order.
type codexContextBody struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// codexDenyBody is the permissionDecision document.
type codexDenyBody struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// codexRender renders one outcome for Codex. Every path exits 0 and every
// path puts a document on stdout. A context renders as additionalContext; a
// deny renders as permissionDecision on PreToolUse and is an invalid outcome
// on any other event.
func codexRender(providerEvent string, outcome hookspec.Outcome, coreStderr []byte) Rendered {
	switch outcome.Kind() {
	default:
		return renderError(codexTraits, outcome.Payload(), coreStderr)
	case contract.KindError:
		return renderError(codexTraits, outcome.Payload(), coreStderr)
	case contract.KindPass, contract.KindContinue:
		return renderPass(codexTraits)

	case contract.KindContext:
		document, err := codexDocument(codexContextBody{
			HookEventName:     providerEvent,
			AdditionalContext: outcome.Payload(),
		})
		if err != nil {
			break // unreachable encode failure: fail into the error branch
		}
		return Rendered{Stdout: document, Exit: hookspec.ExitOK}

	case contract.KindDeny:
		if providerEvent != surface.Codex.PreToolEvent() {
			return rejectInvalidOutcome(codexTraits, providerEvent, contract.KindDeny)
		}
		document, err := codexDocument(codexDenyBody{
			HookEventName:            providerEvent,
			PermissionDecision:       "deny",
			PermissionDecisionReason: outcome.Payload(),
		})
		if err != nil {
			break // unreachable encode failure: fail into the error branch
		}
		return Rendered{Stdout: document, Exit: hookspec.ExitOK}
	}
	return renderError(codexTraits, outcome.Payload(), coreStderr)
}

// codexDocument wraps one response body in hookSpecificOutput. It returns an
// error rather than panicking, because codex must fail open at 0.
func codexDocument(body any) ([]byte, error) {
	return encodeDocument(codexHookSpecificDocument{HookSpecificOutput: body})
}
