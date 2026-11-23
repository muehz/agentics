// The Antigravity surface adapter. Its protocol is in
// docs/surfaces/antigravity.md: Antigravity reads a JSON decision document
// from stdout and treats non-zero exits as non-blocking hook errors, so every
// branch exits 0 with a decision document — {"decision":"allow"} when passing
// or reporting a fault, and {"decision":"deny","reason":...} when blocking on
// PreToolUse.
//
// Only PreToolUse is served: no registered hook maps to any other event.
package provider

import (
	"fmt"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/surface"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

// antigravityTraits: every fault exits 0 after {"decision":"allow"}, so
// Antigravity always receives a valid JSON decision document and never
// mistakes an adapter fault for a hook crash.
var antigravityTraits = traits{
	name:          string(surface.Antigravity),
	label:         surface.Antigravity.Label(),
	emptyBody:     []byte("{\"decision\":\"allow\"}\n"),
	errorExit:     hookspec.ExitOK,
	failedMessage: "Antigravity hook adapter failed",
}

// antigravityEvents is the set of native event names the adapter accepts. It
// holds exactly one, the surface's tool-use event, because no registered hook
// serves any other event.
var antigravityEvents = map[string]struct{}{
	surface.Antigravity.PreToolEvent(): {},
}

// antigravityToolNames maps Antigravity's IDE, CLI, and ACP tool names to the
// neutral vocabulary the hook cores inspect. Unknown and MCP names pass
// through unchanged.
var antigravityToolNames = map[string]string{
	"run_command":                "Bash",
	"view_file":                  "Read",
	"read_file":                  "Read",
	"write_to_file":              "Write",
	"write_file":                 "Write",
	"replace_file_content":       "Edit",
	"edit_file":                  "Edit",
	"multi_replace_file_content": "MultiEdit",
	"grep_search":                "Grep",
	"find_by_name":               "Glob",
	"list_dir":                   "Glob",
	"search_web":                 "WebSearch",
	"read_url_content":           "WebFetch",
	"invoke_subagent":            "Agent",
	"ask_question":               "AskUserQuestion",
}

func init() {
	register(surface.Antigravity, adapter{
		traits:    antigravityTraits,
		normalize: antigravityNormalize,
		render:    antigravityRender,
	})
}

// antigravityNormalize builds the neutral event parts from an Antigravity payload.
//
// hookEventName is optional on the wire and must equal providerEvent when
// present and non-null. toolCall must be an object with a string name and an
// object (or absent/null) args. Raw toolCall.args is preserved untouched under
// tool.input.value while canonical keys are lifted onto top-level tool.input
// using each tool's native schema precedence.
func antigravityNormalize(providerEvent string, payload map[string]any) (contract.EventParts, error) {
	_, ok := antigravityEvents[providerEvent]
	if !ok {
		return contract.EventParts{}, fmt.Errorf("expected Antigravity %s payload", providerEvent)
	}
	if rawEvent, present := payload["hookEventName"]; present && rawEvent != nil {
		name, ok := rawEvent.(string)
		if !ok || name != providerEvent {
			return contract.EventParts{}, fmt.Errorf("expected Antigravity %s payload", providerEvent)
		}
	}

	toolCall, ok := payload["toolCall"].(map[string]any)
	if !ok {
		return contract.EventParts{}, fieldError(antigravityTraits, "toolCall", "a present object")
	}
	rawToolName, ok := toolCall["name"].(string)
	if !ok {
		return contract.EventParts{}, fieldError(antigravityTraits, "toolCall.name", "a present string")
	}
	args, err := antigravityToolArgs(toolCall)
	if err != nil {
		return contract.EventParts{}, err
	}

	cwd, err := optionalString(antigravityTraits, payload, "cwd")
	if err != nil {
		return contract.EventParts{}, err
	}
	workspacePaths, err := optionalStringSlice(antigravityTraits, payload, "workspacePaths")
	if err != nil {
		return contract.EventParts{}, err
	}
	if (cwd == nil || *cwd == "") && len(workspacePaths) > 0 && workspacePaths[0] != "" {
		first := workspacePaths[0]
		cwd = &first
	}
	if rawToolName == "run_command" {
		if rawCwd, present := args["Cwd"]; present && rawCwd != nil {
			if cmdCwd, ok := rawCwd.(string); ok {
				if cmdCwd != "" {
					cwd = &cmdCwd
				}
			} else {
				// A malformed run_command.Cwd leaves the target directory unproven:
				// bind cwd to a relative sentinel so repository settings resolve to
				// strict defaults instead of inheriting another workspace's policy.
				unproven := "."
				cwd = &unproven
			}
		}
	}

	conversationID, err := optionalString(antigravityTraits, payload, "conversationId")
	if err != nil {
		return contract.EventParts{}, err
	}
	turnID, err := optionalString(antigravityTraits, payload, "turnId")
	if err != nil {
		return contract.EventParts{}, err
	}
	transcriptPath, err := optionalString(antigravityTraits, payload, "transcriptPath")
	if err != nil {
		return contract.EventParts{}, err
	}
	modelName, err := optionalString(antigravityTraits, payload, "modelName")
	if err != nil {
		return contract.EventParts{}, err
	}

	toolInput := normalizeAntigravityToolInput(rawToolName, args)
	toolName := antigravityCanonicalToolName(rawToolName, args)
	// No check reads transcriptPath or modelName, so they are only walked for
	// NUL when the event is read.
	return contract.EventParts{
		Surface:   surface.Antigravity,
		SessionID: conversationID,
		TurnID:    turnID,
		Cwd:       cwd,
		Runtime:   runtimeRoots("PLUGIN_ROOT", "PLUGIN_DATA"),
		ToolName:  toolName,
		ToolInput: toolInput,
		NULOnly:   []*string{transcriptPath, modelName},
	}, nil
}

func antigravityToolArgs(toolCall map[string]any) (map[string]any, error) {
	rawArgs, present := toolCall["args"]
	if !present || rawArgs == nil {
		return map[string]any{}, nil
	}
	args, ok := rawArgs.(map[string]any)
	if !ok {
		return nil, fieldError(antigravityTraits, "toolCall.args", "an object or null")
	}
	return args, nil
}

func optionalStringSlice(t traits, payload map[string]any, name string) ([]string, error) {
	raw, present := payload[name]
	if !present || raw == nil {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fieldError(t, name, "an array of strings or null")
	}
	values := make([]string, len(list))
	for i, item := range list {
		text, ok := item.(string)
		if !ok {
			return nil, fieldError(t, name, "an array of strings or null")
		}
		values[i] = text
	}
	return values, nil
}

// normalizeAntigravityToolInput keeps args untouched under tool.input.value
// and lifts canonical neutral keys according to rawToolName's native schema.
// Native-first path precedence and strict schema pairing prevent cross-alias
// smuggling from redirecting path checks or forging old_string exemptions.
func normalizeAntigravityToolInput(rawToolName string, args map[string]any) map[string]any {
	toolInput := map[string]any{"value": args}

	switch rawToolName {
	case "run_command":
		if cmd, ok := firstPresentString(args, "CommandLine", "command"); ok {
			toolInput["command"] = cmd
		}
		// Copy non-null Cwd verbatim so toolcall.Call.ProvenCwd rejects both
		// disagreeing directory strings and malformed non-string overrides.
		if rawCwd, present := args["Cwd"]; present && rawCwd != nil {
			toolInput["cwd"] = rawCwd
		}

	case "view_file":
		liftAntigravityPaths(toolInput, args, "AbsolutePath", "Path")

	case "read_file":
		liftAntigravityPaths(toolInput, args, "Path", "AbsolutePath")

	case "write_to_file":
		liftAntigravityPaths(toolInput, args, "TargetFile", "Path")
		if content, ok := firstPresentString(args, "CodeContent", "Content"); ok {
			toolInput["content"] = content
		}

	case "write_file":
		liftAntigravityPaths(toolInput, args, "Path", "TargetFile")
		if content, ok := firstPresentString(args, "Content", "CodeContent"); ok {
			toolInput["content"] = content
		}

	case "replace_file_content":
		liftAntigravityPaths(toolInput, args, "TargetFile", "Path")
		liftAntigravityReplacement(toolInput, args, "TargetContent", "ReplacementContent")

	case "edit_file":
		liftAntigravityPaths(toolInput, args, "Path", "TargetFile")
		liftAntigravityReplacement(toolInput, args, "Target", "Replacement")

	case "multi_replace_file_content":
		liftAntigravityPaths(toolInput, args, "TargetFile", "Path")
		edits, parseError := normalizeAntigravityReplacementChunks(args)
		if len(edits) > 0 {
			toolInput["edits"] = edits
		}
		if parseError {
			toolcall.MarkWriteParseError(toolInput)
		}

	case "grep_search":
		if searchPath, ok := args["SearchPath"].(string); ok && searchPath != "" {
			toolInput["path"] = searchPath
		}
		if query, ok := args["Query"].(string); ok {
			toolInput["pattern"] = query
		}

	case "find_by_name":
		if searchDir, ok := args["SearchDirectory"].(string); ok && searchDir != "" {
			toolInput["path"] = searchDir
		}
		if pattern, ok := args["Pattern"].(string); ok {
			toolInput["pattern"] = pattern
		}

	case "list_dir":
		if dirPath, ok := args["DirectoryPath"].(string); ok && dirPath != "" {
			toolInput["path"] = dirPath
		}

	case "manage_task":
		if !antigravityManageTaskSafeLifecycle(args) {
			liftAntigravityTaskInput(toolInput, args)
		}

	case "send_command_input":
		if !antigravitySendCommandInputPureTerminate(args) {
			liftAntigravityTaskInput(toolInput, args)
		}

	default:
		if cmd, ok := args["command"].(string); ok {
			toolInput["command"] = cmd
		}
	}

	return toolInput
}

// antigravityCanonicalToolName returns the neutral tool name for rawToolName
// and its arguments. Interactive stdin to an existing background task cannot be
// classified against the target interpreter in a stateless pre-tool hook, so
// non-lifecycle manage_task calls and non-terminating send_command_input calls
// normalize to Bash to route through both command and secrets checks.
func antigravityCanonicalToolName(rawToolName string, args map[string]any) string {
	switch rawToolName {
	case "manage_task":
		if !antigravityManageTaskSafeLifecycle(args) {
			return "Bash"
		}
		return rawToolName
	case "send_command_input":
		if !antigravitySendCommandInputPureTerminate(args) {
			return "Bash"
		}
		return rawToolName
	}
	if canonical, known := antigravityToolNames[rawToolName]; known {
		return canonical
	}
	return rawToolName
}

// antigravityManageTaskSafeLifecycle reports whether a manage_task call is a
// pure task lifecycle inspection or termination with no stdin payload attached.
func antigravityManageTaskSafeLifecycle(args map[string]any) bool {
	action, ok := args["Action"].(string)
	if !ok {
		return false
	}
	switch action {
	case "list", "status", "kill":
	default:
		return false
	}
	for _, key := range []string{"Input", "input"} {
		if raw, present := args[key]; present && raw != nil {
			return false
		}
	}
	return true
}

// antigravitySendCommandInputPureTerminate reports whether a send_command_input
// call only terminates the target command without supplying stdin.
func antigravitySendCommandInputPureTerminate(args map[string]any) bool {
	terminate, ok := args["Terminate"].(bool)
	if !ok || !terminate {
		return false
	}
	for _, key := range []string{"Input", "input"} {
		raw, present := args[key]
		if !present || raw == nil {
			continue
		}
		text, isString := raw.(string)
		if !isString || text != "" {
			return false
		}
	}
	return true
}

// liftAntigravityTaskInput lifts any string stdin payload onto command for
// direct inspection by command, release, and secrets guards and marks the call
// as unclassified background task input so command-guardian fails closed.
func liftAntigravityTaskInput(toolInput, args map[string]any) {
	if input, ok := firstNonEmptyOrPresentString(args, "Input", "input"); ok {
		toolInput["command"] = input
	}
	toolInput["unclassified_task_input"] = true
}

// firstNonEmptyOrPresentString prefers the first non-empty string value across
// keys so an empty primary alias cannot mask a non-empty fallback payload,
// while still preserving an explicit empty string when no key is non-empty.
func firstNonEmptyOrPresentString(args map[string]any, keys ...string) (string, bool) {
	foundEmpty := false
	for _, key := range keys {
		if value, ok := args[key].(string); ok {
			if value != "" {
				return value, true
			}
			foundEmpty = true
		}
	}
	if foundEmpty {
		return "", true
	}
	return "", false
}

// liftAntigravityPaths promotes the tool's native primary path to file_path
// (falling back to secondary when primary is absent or empty). If both keys
// carry distinct non-empty paths, the secondary path is also lifted onto path
// so guardians inspect both destinations.
func liftAntigravityPaths(toolInput, args map[string]any, primaryKey, secondaryKey string) {
	primary, _ := args[primaryKey].(string)
	secondary, _ := args[secondaryKey].(string)
	switch {
	case primary != "":
		toolInput["file_path"] = primary
		if secondary != "" && secondary != primary {
			toolInput["path"] = secondary
		}
	case secondary != "":
		toolInput["file_path"] = secondary
	}
}

// firstPresentString returns the first key whose value is a string, including
// the empty string so empty-file overwrites stay visible to hook-bypass checks.
func firstPresentString(args map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := args[key].(string); ok {
			return value, true
		}
	}
	return "", false
}

// liftAntigravityReplacement lifts new_string and its schema-paired old_string
// only when the tool's own replacement key is a present string.
func liftAntigravityReplacement(toolInput, args map[string]any, oldKey, newKey string) {
	replacement, ok := args[newKey].(string)
	if !ok {
		return
	}
	toolInput["new_string"] = replacement
	if target, ok := args[oldKey].(string); ok {
		toolInput["old_string"] = target
	}
}

// normalizeAntigravityReplacementChunks converts ReplacementChunks into the
// neutral []any slice of edit maps. Any malformed structure marks
// write_parse_error while still preserving valid chunks from mixed arrays.
func normalizeAntigravityReplacementChunks(args map[string]any) ([]any, bool) {
	rawChunks, present := args["ReplacementChunks"]
	if !present {
		return nil, true
	}
	chunks, ok := rawChunks.([]any)
	if !ok {
		return nil, true
	}
	edits := make([]any, 0, len(chunks))
	parseError := false
	for _, item := range chunks {
		chunk, ok := item.(map[string]any)
		if !ok {
			parseError = true
			continue
		}
		replacement, ok := chunk["ReplacementContent"].(string)
		if !ok {
			parseError = true
			continue
		}
		edit := map[string]any{"new_string": replacement}
		if rawTarget, hasTarget := chunk["TargetContent"]; hasTarget && rawTarget != nil {
			target, ok := rawTarget.(string)
			if !ok {
				parseError = true
			} else {
				edit["old_string"] = target
			}
		}
		edits = append(edits, edit)
	}
	return edits, parseError
}

// antigravityDecision is the PreToolUse deny document. Field order is
// declaration order.
type antigravityDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// antigravityRender renders one outcome for Antigravity. Every path exits 0
// with a JSON decision document on stdout. Pass, continue, and context emit
// {"decision":"allow"}; deny on PreToolUse emits {"decision":"deny","reason":...}.
func antigravityRender(providerEvent string, outcome hookspec.Outcome, coreStderr []byte) Rendered {
	switch outcome.Kind() {
	default:
		return renderError(antigravityTraits, outcome.Payload(), coreStderr)
	case contract.KindError:
		return renderError(antigravityTraits, outcome.Payload(), coreStderr)
	case contract.KindPass, contract.KindContinue, contract.KindContext:
		return renderPass(antigravityTraits)

	case contract.KindDeny:
		if providerEvent != surface.Antigravity.PreToolEvent() {
			return rejectInvalidOutcome(antigravityTraits, providerEvent, contract.KindDeny)
		}
		document, err := encodeDocument(antigravityDecision{
			Decision: "deny",
			Reason:   outcome.Payload(),
		})
		if err != nil {
			break // unreachable encode failure: fail into the error branch
		}
		return Rendered{Stdout: document, Exit: hookspec.ExitOK}
	}
	return renderError(antigravityTraits, outcome.Payload(), coreStderr)
}
