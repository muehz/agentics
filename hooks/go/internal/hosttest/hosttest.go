package hosttest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// Call is one tool call a test sends to a hook. Tool is a neutral name
// (Bash, PowerShell, Read, Write, Edit, MultiEdit) and Input a map with
// Claude Code's keys, which Stdin renames for the surface. With Native set,
// Tool and Input are written as given: a surface's own tool name such as
// apply_patch, or a non-map Input such as Copilot's string toolArgs.
type Call struct {
	Tool   string
	Input  any
	Cwd    string
	Native bool
}

// nativeTools maps each neutral tool name to the name each surface's host
// sends for it, in the order claude-code, copilot, codex, antigravity.
var nativeTools = map[string][4]string{
	"Bash":       {"Bash", "bash", "exec_command", "run_command"},
	"PowerShell": {"PowerShell", "powershell", "exec_command", "run_command"},
	"Read":       {"Read", "view", "Read", "view_file"},
	"Write":      {"Write", "create", "Write", "write_to_file"},
	"Edit":       {"Edit", "edit", "Edit", "replace_file_content"},
	"MultiEdit":  {"MultiEdit", "edit", "MultiEdit", "multi_replace_file_content"},
}

// copilotKeys is the input-key rename Copilot's host applies per neutral
// tool. A key a tool's row does not name passes through unchanged.
var copilotKeys = map[string]map[string]string{
	"Bash":       {"command": "cmd"},
	"PowerShell": {"command": "cmd"},
	"Read":       {"file_path": "path"},
	"Edit":       {"file_path": "path", "old_string": "old_str", "new_string": "new_str"},
}

// antigravityKeys is the input-key rename Antigravity's host applies per
// neutral tool. A key a tool's row does not name passes through unchanged.
var antigravityKeys = map[string]map[string]string{
	"Bash":       {"command": "CommandLine"},
	"PowerShell": {"command": "CommandLine"},
	"Read":       {"file_path": "AbsolutePath"},
	"Write":      {"file_path": "TargetFile", "content": "CodeContent"},
	"Edit":       {"file_path": "TargetFile", "old_string": "TargetContent", "new_string": "ReplacementContent"},
	"MultiEdit":  {"file_path": "TargetFile"},
}

// surfaceIndex returns the column of nativeTools for id. It panics on an ID
// that names no surface, because a test that builds one has a typo, not a case.
func surfaceIndex(id surface.ID) int {
	switch id {
	case surface.ClaudeCode:
		return 0
	case surface.Copilot:
		return 1
	case surface.Codex:
		return 2
	case surface.Antigravity:
		return 3
	}
	panic(fmt.Sprintf("hosttest: unknown surface %q", string(id)))
}

// NativeTool returns the tool name the surface's host sends for a neutral
// tool name. It panics on an unknown surface or neutral name, because a test
// that names one has a typo, not a case.
func NativeTool(id surface.ID, neutral string) string {
	index := surfaceIndex(id)
	row, ok := nativeTools[neutral]
	if !ok {
		panic(fmt.Sprintf("hosttest: unknown neutral tool %q", neutral))
	}
	return row[index]
}

// Stdin returns the pre-tool payload the surface's host writes for c: the
// envelope hook_event_name, tool_name, tool_input, and cwd on claude-code and
// codex, toolName, toolArgs, and cwd on copilot, and hookEventName, cwd, and
// toolCall on antigravity. No session, turn, or model field is written. It
// panics on an unknown surface, an unknown neutral tool, a non-map Input
// without Native, and an empty Cwd, which Codex rejects as a malformed
// payload; callers pass t.TempDir().
func Stdin(id surface.ID, c Call) []byte {
	surfaceIndex(id)
	if c.Cwd == "" {
		panic(fmt.Sprintf("hosttest: Stdin call %+v has no cwd; pass t.TempDir()", c))
	}
	tool, input := c.Tool, c.Input
	if !c.Native {
		fields, ok := c.Input.(map[string]any)
		if !ok {
			panic(fmt.Sprintf("hosttest: Stdin call %+v: Input must be a map unless Native is set", c))
		}
		tool = NativeTool(id, c.Tool)
		switch id {
		case surface.Copilot:
			input = renameKeys(fields, copilotKeys[c.Tool])
		case surface.Antigravity:
			input = antigravityInput(c.Tool, fields)
		}
	}
	var payload map[string]any
	switch id {
	case surface.Copilot:
		payload = map[string]any{"toolName": tool, "toolArgs": input, "cwd": c.Cwd}
	case surface.Antigravity:
		payload = map[string]any{
			"hookEventName": "PreToolUse",
			"cwd":           c.Cwd,
			"toolCall":      map[string]any{"name": tool, "args": input},
		}
	default:
		payload = map[string]any{"hook_event_name": "PreToolUse", "tool_name": tool, "tool_input": input, "cwd": c.Cwd}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("hosttest: Stdin call %+v: %v", c, err))
	}
	return raw
}

// antigravityInput renames Claude Code neutral keys to Antigravity's native
// PascalCase keys without mutating fields. For MultiEdit, it also converts
// edits into ReplacementChunks.
func antigravityInput(neutralTool string, fields map[string]any) map[string]any {
	out := renameKeys(fields, antigravityKeys[neutralTool])
	if neutralTool != "MultiEdit" {
		return out
	}
	rawEdits, present := out["edits"]
	if !present {
		return out
	}
	var items []map[string]any
	switch typed := rawEdits.(type) {
	case []any:
		items = make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]any); ok {
				items = append(items, m)
			}
		}
	case []map[string]any:
		items = typed
	default:
		return out
	}
	delete(out, "edits")
	chunks := make([]any, 0, len(items))
	for _, edit := range items {
		chunks = append(chunks, renameKeys(edit, map[string]string{
			"old_string": "TargetContent",
			"new_string": "ReplacementContent",
		}))
	}
	out["ReplacementChunks"] = chunks
	return out
}

// renameKeys copies fields with the renamed keys, so the caller's map is
// never changed.
func renameKeys(fields map[string]any, renames map[string]string) map[string]any {
	out := make(map[string]any, len(fields))
	for key, value := range fields {
		if renamed, ok := renames[key]; ok {
			key = renamed
		}
		out[key] = value
	}
	return out
}

// Decision is what a host makes of a hook's response.
type Decision int

const (
	// Allow means the host runs the tool.
	Allow Decision = iota
	// Deny means the host blocks the tool with Verdict.Reason.
	Deny
	// Unreadable means the response fits none of the surface's documented
	// forms. It is never a pass: a test expecting allow or deny fails on it.
	Unreadable
)

func (d Decision) String() string {
	switch d {
	case Allow:
		return "Allow"
	case Deny:
		return "Deny"
	case Unreadable:
		return "Unreadable"
	}
	return fmt.Sprintf("Decision(%d)", int(d))
}

// Verdict is one hook response as the surface's host reads it. Stderr is the
// hook's stderr verbatim on every verdict.
type Verdict struct {
	Decision Decision
	Reason   string
	Context  string
	Stderr   string
}

// Silent reports a pass with nothing for the agent to read: an allow with no
// context and no stderr.
func (v Verdict) Silent() bool {
	return v.Decision == Allow && v.Context == "" && v.Stderr == ""
}

// Read returns the verdict the surface's host reaches from a pre-tool hook's
// exit code and output. The reason comes from the reason field, never the
// whole envelope. Any output outside a surface's documented forms is
// Unreadable, including a Claude Code exit 1 without a diagnostic, so a
// failing hook never reads as a silent pass. It panics on an unknown surface.
func Read(id surface.ID, exit int, stdout, stderr []byte) Verdict {
	verdict := Verdict{Decision: Unreadable, Stderr: string(stderr)}
	switch surfaceIndex(id) {
	case 0:
		readClaudeCode(&verdict, exit, stdout, stderr)
	case 1:
		readCopilot(&verdict, exit, stdout)
	case 2:
		readCodex(&verdict, exit, stdout)
	case 3:
		readAntigravity(&verdict, exit, stdout)
	}
	return verdict
}

func blank(b []byte) bool { return len(bytes.TrimSpace(b)) == 0 }

// readClaudeCode follows Claude Code's exit-code protocol: exit 2 blocks with
// stderr as the reason, exit 0 passes with an optional context document, and
// exit 1 is a non-blocking error the host shows but does not act on.
func readClaudeCode(v *Verdict, exit int, stdout, stderr []byte) {
	switch exit {
	case 2:
		if len(stdout) == 0 && !blank(stderr) {
			v.Decision, v.Reason = Deny, string(stderr)
		}
	case 1:
		if blank(stdout) && !blank(stderr) {
			v.Decision = Allow
		}
	case 0:
		if blank(stdout) {
			v.Decision = Allow
			return
		}
		document, ok := oneObject(stdout)
		if !ok || len(document) != 1 {
			return
		}
		body, ok := document["hookSpecificOutput"].(map[string]any)
		if !ok || len(body) != 2 || body["hookEventName"] != "PreToolUse" {
			return
		}
		if context, ok := body["additionalContext"].(string); ok && context != "" {
			v.Decision, v.Context = Allow, context
		}
	}
}

// readCopilot reads Copilot's flat document. Copilot shows a context as
// stderr with no document, so Context is never set; any non-zero exit is
// outside the forms an Agentics hook renders.
func readCopilot(v *Verdict, exit int, stdout []byte) {
	if exit != 0 {
		return
	}
	if blank(stdout) {
		v.Decision = Allow
		return
	}
	document, ok := oneObject(stdout)
	if !ok || caseFolded(document) {
		return
	}
	// The old readers read hookSpecificOutput on Copilot too, so a document
	// that carries it, even as null, has two readings.
	if _, nested := document["hookSpecificOutput"]; nested {
		return
	}
	if reason, ok := deniedReason(document); ok {
		v.Decision, v.Reason = Deny, reason
	}
}

// readCodex reads Codex's document, which is required: {} passes, and a
// decision or context sits in a hookSpecificOutput that names PreToolUse.
func readCodex(v *Verdict, exit int, stdout []byte) {
	if exit != 0 {
		return
	}
	// The old pass rule compares the trimmed body with the text {}, so only
	// that spelling of the empty object passes.
	if string(bytes.TrimSpace(stdout)) == "{}" {
		v.Decision = Allow
		return
	}
	document, ok := oneObject(stdout)
	if !ok || len(document) == 0 || caseFolded(document) {
		return
	}
	if _, flat := document["permissionDecision"]; flat {
		return
	}
	body, ok := document["hookSpecificOutput"].(map[string]any)
	if !ok || caseFolded(body) || body["hookEventName"] != "PreToolUse" {
		return
	}
	if _, decided := body["permissionDecision"]; decided {
		if reason, ok := deniedReason(body); ok {
			v.Decision, v.Reason = Deny, reason
		}
		return
	}
	if context, ok := body["additionalContext"].(string); ok && context != "" {
		v.Decision, v.Context = Allow, context
	}
}

// readAntigravity reads Antigravity's flat decision document. Every branch
// exits 0 with either {"decision":"allow"} or {"decision":"deny","reason":...}.
// Context is never set because Antigravity has no PreToolUse context channel.
func readAntigravity(v *Verdict, exit int, stdout []byte) {
	if exit != 0 {
		return
	}
	document, ok := oneObject(stdout)
	if !ok || caseFolded(document) {
		return
	}
	if _, nested := document["hookSpecificOutput"]; nested {
		return
	}
	if _, copilot := document["permissionDecision"]; copilot {
		return
	}
	decision, ok := document["decision"].(string)
	if !ok {
		return
	}
	switch decision {
	case "allow":
		if len(document) == 1 {
			v.Decision = Allow
		}
	case "deny":
		if reason, ok := document["reason"].(string); ok && reason != "" && len(document) == 2 {
			v.Decision, v.Reason = Deny, reason
		}
	}
}

// deniedReason returns the reason of a deny decision: permissionDecision
// "deny" and a non-empty string permissionDecisionReason.
func deniedReason(fields map[string]any) (string, bool) {
	if fields["permissionDecision"] != "deny" {
		return "", false
	}
	reason, ok := fields["permissionDecisionReason"].(string)
	return reason, ok && reason != ""
}

// documentKeys are the keys the Copilot, Codex, and Antigravity readers act on.
var documentKeys = []string{
	"hookSpecificOutput", "hookEventName", "permissionDecision",
	"permissionDecisionReason", "additionalContext",
	"decision", "reason",
}

// caseFolded reports a key that equals a known key only when letter case is
// ignored. encoding/json matches struct fields that way, so the old readers
// could take such a key for the known one; the document then has two
// readings and is not one documented form.
func caseFolded(fields map[string]any) bool {
	for key := range fields {
		for _, known := range documentKeys {
			if key != known && strings.EqualFold(key, known) {
				return true
			}
		}
	}
	return false
}

// oneObject decodes exactly one JSON object. Trailing data, a second
// document, an array, and null all fail.
func oneObject(raw []byte) (map[string]any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var document map[string]any
	if err := dec.Decode(&document); err != nil || document == nil {
		return nil, false
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return document, true
}
