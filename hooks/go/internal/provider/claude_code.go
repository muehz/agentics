// The Claude Code surface adapter. Its protocol is in
// docs/surfaces/claude-code.md, Hook protocol: exit 2 blocks with stderr as
// the reason; exit 1 is a non-blocking error; a stdout JSON document on exit 0
// may carry hookSpecificOutput.additionalContext.
//
// Only PreToolUse is served: no registered hook maps to any other event.
package provider

import (
	"fmt"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// claudeTraits: a fault exits 1 with no body. Claude Code treats that as a
// non-blocking error, so an adapter or core fault is never mistaken for a
// deny.
var claudeTraits = traits{
	name:          string(surface.ClaudeCode),
	label:         surface.ClaudeCode.Label(),
	emptyBody:     nil,
	errorExit:     hookspec.ExitUsage,
	failedMessage: "Claude hook adapter failed",
}

func init() {
	register(surface.ClaudeCode, adapter{
		traits:    claudeTraits,
		normalize: claudeNormalize,
		render:    claudeRender,
	})
}

// claudeNormalize builds the neutral event parts from a Claude Code payload.
//
// hook_event_name must name the event being served. tool_name must be a
// string and tool_input an object. session_id, cwd and turn_id are optional.
// Top-level keys the neutral event does not name are dropped; keys inside
// tool_input are payload and survive.
func claudeNormalize(providerEvent string, payload map[string]any) (contract.EventParts, error) {
	if name, _ := payload["hook_event_name"].(string); name != providerEvent {
		return contract.EventParts{}, fmt.Errorf("expected Claude %s payload", providerEvent)
	}
	_, ok := claudeEvents[providerEvent]
	if !ok {
		return contract.EventParts{}, fmt.Errorf("expected Claude %s payload", providerEvent)
	}

	toolName, err := requiredString(claudeTraits, payload, "tool_name")
	if err != nil {
		return contract.EventParts{}, err
	}
	toolInput, ok := payload["tool_input"].(map[string]any)
	if !ok {
		return contract.EventParts{}, fieldError(claudeTraits, "tool_input", "a present object")
	}

	sessionID, err := optionalString(claudeTraits, payload, "session_id")
	if err != nil {
		return contract.EventParts{}, err
	}
	cwd, err := optionalString(claudeTraits, payload, "cwd")
	if err != nil {
		return contract.EventParts{}, err
	}
	turnID, err := optionalString(claudeTraits, payload, "turn_id")
	if err != nil {
		return contract.EventParts{}, err
	}

	return contract.EventParts{
		Surface:   surface.ClaudeCode,
		SessionID: sessionID,
		TurnID:    turnID,
		Cwd:       cwd,
		Runtime:   runtimeRoots("CLAUDE_PLUGIN_ROOT", "CLAUDE_PLUGIN_DATA"),
		ToolName:  toolName,
		ToolInput: toolInput,
	}, nil
}

// claudeEvents is the set of native event names the adapter accepts. It
// holds exactly one, the surface's tool-use event, because no registered hook
// serves any other event.
var claudeEvents = map[string]struct{}{
	surface.ClaudeCode.PreToolEvent(): {},
}

// claudeRender renders one outcome for Claude Code. The table does not depend
// on the event: deny exits 2 with its text on stderr; context is a
// hookSpecificOutput document on exit 0; error exits 1.
func claudeRender(providerEvent string, outcome hookspec.Outcome, coreStderr []byte) Rendered {
	switch outcome.Kind() {
	default:
		return renderError(claudeTraits, outcome.Payload(), coreStderr)
	case contract.KindError:
		return renderError(claudeTraits, outcome.Payload(), coreStderr)
	case contract.KindPass, contract.KindContinue:
		return renderPass(claudeTraits)

	case contract.KindDeny:
		return Rendered{Stderr: []byte(outcome.Payload() + "\n"), Exit: hookspec.ExitDeny}

	case contract.KindContext:
		document, err := encodeDocument(claudeContextDocument{
			HookSpecificOutput: claudeHookOutput{HookEventName: providerEvent, AdditionalContext: outcome.Payload()},
		})
		if err != nil {
			break // unreachable encode failure: fail into the error branch
		}
		return Rendered{Stdout: document, Exit: hookspec.ExitOK}
	}
	return renderError(claudeTraits, outcome.Payload(), coreStderr)
}

// claudeContextDocument is the context response:
//
//	{"hookSpecificOutput":{"hookEventName":<event>,"additionalContext":<msg>}}
type claudeContextDocument struct {
	HookSpecificOutput claudeHookOutput `json:"hookSpecificOutput"`
}

// claudeHookOutput is the body of claudeContextDocument. Field order is
// declaration order.
type claudeHookOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}
