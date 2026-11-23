package hosttest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// runHookMarker ends the plugin-relative path in every shipped command
// string. The surface, hook, and arguments follow it; a Windows prefix such
// as Copilot's `& ` or Codex's `cmd /c ` falls in front of it.
const runHookMarker = `run-hook.cmd"`

// commandFields are the keys a config command can come from, in the order a
// registration list reports them.
var commandFields = []string{"command", "commandWindows", "bash", "powershell"}

// configShape is how one shipped config file nests its commands.
type configShape struct {
	name    string
	rootKey string
	// inParent is true when the config sits in hooksDir's parent directory
	// (repository-root hooks.json for Antigravity).
	inParent bool
	// grouped is true when each event entry is a group holding a nested
	// hooks array of commands (Claude Code, Codex, and Antigravity); Copilot's
	// entries and Antigravity's SessionStart entries are the commands themselves.
	grouped    bool
	timeoutKey string
	otherKey   string
}

// configShapes lists the shipped config files in the order Registrations
// reads them.
var configShapes = []configShape{
	{"hooks.claude.json", "hooks", false, true, "timeout", "timeoutSec"},
	{"hooks.copilot.json", "hooks", false, false, "timeoutSec", "timeout"},
	{"hooks.codex.json", "hooks", false, true, "timeout", "timeoutSec"},
	{"hooks.json", "agentics", true, true, "timeout", "timeoutSec"},
}

// Registration is one shipped hook command: one command field of one config
// entry. Event is the key as declared (Copilot spells it preToolUse), Surface
// and Hook are the first two words after the run-hook.cmd marker, and Args
// the words after those, empty when there are none. Matcher is the entry's
// matcher string and HasMatcher whether the key is present, because Copilot
// reads an omitted matcher differently from an empty one.
type Registration struct {
	Surface    surface.ID
	Event      string
	Matcher    string
	HasMatcher bool
	Field      string
	Command    string
	Hook       string
	Args       []string
	Timeout    time.Duration
}

// Registrations reads hooks.claude.json, hooks.copilot.json, and
// hooks.codex.json in hooksDir, plus hooks.json in hooksDir's parent
// directory, and returns every command field in them: files in that order,
// then events sorted, then entries as declared, then fields in the order
// command, commandWindows, bash, powershell. It fails, with an error that
// starts with the file path, on a shape it does not recognise, on a command
// object or file that yields no command, on a surface name that surface.Parse
// refuses, and on a command field without the quoted run-hook.cmd" marker, so
// no command is skipped unseen.
func Registrations(hooksDir string) ([]Registration, error) {
	var all []Registration
	for _, shape := range configShapes {
		base := hooksDir
		if shape.inParent {
			base = filepath.Dir(hooksDir)
		}
		path := filepath.Join(base, shape.name)
		found, err := readConfig(path, shape)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("%s: no registrations", path)
		}
		all = append(all, found...)
	}
	return all, nil
}

func readConfig(path string, shape configShape) ([]Registration, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	root, ok := document.(map[string]any)
	if !ok {
		return nil, errors.New("the document is not an object")
	}
	events, ok := root[shape.rootKey].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("`%s` is missing or not an object", shape.rootKey)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("`%s` is empty", shape.rootKey)
	}
	names := make([]string, 0, len(events))
	for name := range events {
		names = append(names, name)
	}
	sort.Strings(names)
	var found []Registration
	for _, event := range names {
		entries, ok := events[event].([]any)
		if !ok {
			return nil, fmt.Errorf("event %s is not an array", event)
		}
		for i, value := range entries {
			entry, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s entry %d is not an object", event, i)
			}
			matcher, hasMatcher, err := readMatcher(entry)
			if err != nil {
				return nil, fmt.Errorf("%s entry %d: %w", event, i, err)
			}
			commands := []any{entry}
			if shape.grouped && !(shape.rootKey == "agentics" && event == "SessionStart") {
				if commands, ok = entry["hooks"].([]any); !ok {
					return nil, fmt.Errorf("%s entry %d has no hooks array", event, i)
				}
			}
			for j, value := range commands {
				command, ok := value.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%s entry %d command %d is not an object", event, i, j)
				}
				read, err := readCommand(command, shape)
				if err != nil {
					return nil, fmt.Errorf("%s entry %d command %d: %w", event, i, j, err)
				}
				for _, r := range read {
					r.Event, r.Matcher, r.HasMatcher = event, matcher, hasMatcher
					found = append(found, r)
				}
			}
		}
	}
	return found, nil
}

// readMatcher reads a present matcher only as a string, so a null or other
// value is never mistaken for an omitted matcher.
func readMatcher(entry map[string]any) (string, bool, error) {
	value, present := entry["matcher"]
	if !present {
		return "", false, nil
	}
	matcher, ok := value.(string)
	if !ok {
		return "", false, fmt.Errorf("matcher %v is not a string", value)
	}
	return matcher, true, nil
}

func readCommand(command map[string]any, shape configShape) ([]Registration, error) {
	if _, present := command[shape.otherKey]; present {
		return nil, fmt.Errorf("timeout is under %q, not %q", shape.otherKey, shape.timeoutKey)
	}
	seconds, ok := command[shape.timeoutKey].(float64)
	if !ok || seconds <= 0 {
		return nil, fmt.Errorf("%q is missing or not a positive number", shape.timeoutKey)
	}
	var found []Registration
	for _, field := range commandFields {
		value, present := command[field]
		if !present {
			continue
		}
		literal, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("field %s is not a string", field)
		}
		at := strings.Index(literal, runHookMarker)
		if at < 0 {
			return nil, fmt.Errorf("field %s value %q lacks the quoted %q marker", field, literal, runHookMarker)
		}
		words := strings.Fields(literal[at+len(runHookMarker):])
		if len(words) < 2 {
			return nil, fmt.Errorf("field %s value %q names no surface and hook", field, literal)
		}
		id, ok := surface.Parse(words[0])
		if !ok {
			return nil, fmt.Errorf("field %s value %q names the unknown surface %q", field, literal, words[0])
		}
		found = append(found, Registration{
			Surface: id,
			Field:   field,
			Command: literal,
			Hook:    words[1],
			Args:    words[2:],
			Timeout: time.Duration(seconds * float64(time.Second)),
		})
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("has none of the fields %s", strings.Join(commandFields, ", "))
	}
	return found, nil
}

// Selected returns the pre-tool registrations the surface's host runs on goos
// for nativeTool, the tool name as that host sends it (see NativeTool); it
// does no translation. Claude Code runs its command field on every OS, Codex
// commandWindows on Windows and command elsewhere, and Copilot powershell on
// Windows and bash elsewhere. Each matcher is applied with its vendor's
// rules.
func Selected(all []Registration, id surface.ID, nativeTool, goos string) ([]Registration, error) {
	windows := goos == "windows"
	var field string
	switch id {
	case surface.Antigravity, surface.ClaudeCode:
		field = "command"
	case surface.Codex:
		field = map[bool]string{true: "commandWindows", false: "command"}[windows]
	case surface.Copilot:
		field = map[bool]string{true: "powershell", false: "bash"}[windows]
	default:
		return nil, fmt.Errorf("unknown surface %q", string(id))
	}
	var selected []Registration
	for _, r := range all {
		if r.Surface != id || r.Field != field || r.Event == "" || strings.ToUpper(r.Event[:1])+r.Event[1:] != "PreToolUse" {
			continue
		}
		ok, err := r.matches(nativeTool)
		if err != nil {
			return nil, err
		}
		if ok {
			selected = append(selected, r)
		}
	}
	return selected, nil
}

// matches applies the matcher with the rules of r's surface. Claude Code reads
// a pattern of names, spaces, commas, and bars as a list of exact names and
// anything else as an unanchored expression; Copilot anchors its expression;
// Codex also tries an apply_patch call as Edit and Write.
func (r Registration) matches(tool string) (bool, error) {
	pattern := r.Matcher
	if r.Surface == surface.Copilot {
		if !r.HasMatcher {
			return true, nil
		}
		// An explicit empty Copilot matcher is read as matching nothing. This
		// is an assumed reading, pending the vendor documentation, which does
		// not yet say whether it means every tool or none.
		if pattern == "" {
			return false, nil
		}
	} else if pattern == "" || pattern == "*" {
		return true, nil
	}
	if r.Surface == surface.ClaudeCode && simpleMatcher(pattern) {
		for _, name := range strings.FieldsFunc(pattern, func(c rune) bool { return c == '|' || c == ',' }) {
			if strings.TrimSpace(name) == tool {
				return true, nil
			}
		}
		return false, nil
	}
	expression := pattern
	if r.Surface == surface.Copilot {
		expression = "^(?:" + expression + ")$"
	}
	// Captures are unused, so ordinary groups serve the declared subset.
	compiled, err := regexp.Compile(strings.ReplaceAll(expression, "(?:", "("))
	if err != nil {
		return false, fmt.Errorf("%s matcher %q: %w", string(r.Surface), pattern, err)
	}
	candidates := []string{tool}
	if r.Surface == surface.Codex && tool == "apply_patch" {
		candidates = append(candidates, "Edit", "Write")
	}
	for _, candidate := range candidates {
		if compiled.MatchString(candidate) {
			return true, nil
		}
	}
	return false, nil
}

func simpleMatcher(pattern string) bool {
	for _, c := range pattern {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("_- ,|", c)) {
			return false
		}
	}
	return true
}

// HostCommand returns the program and arguments that start r's command as the
// host does on goos. name is a bare program name, not resolved on PATH. On
// Windows a command field has its ${CLAUDE_PLUGIN_ROOT}, ${PLUGIN_ROOT}, and
// ${COPILOT_PLUGIN_ROOT} rewritten to cmd's %NAME% form and every field but
// powershell runs under cmd.exe /d /s /c; elsewhere bash runs under bash -c
// and the rest under sh -c. A powershell field runs under powershell.exe on
// every OS and exits with the hook's exit code. It fails only on an unknown
// field or an empty command.
func (r Registration) HostCommand(goos string) (name string, args []string, err error) {
	switch r.Field {
	case "command", "commandWindows", "bash", "powershell":
	default:
		return "", nil, fmt.Errorf("registration field %q is not a command field", r.Field)
	}
	if r.Command == "" {
		return "", nil, fmt.Errorf("registration field %s has an empty command", r.Field)
	}
	literal := r.Command
	if goos == "windows" && r.Field == "command" {
		for _, key := range []string{"CLAUDE_PLUGIN_ROOT", "PLUGIN_ROOT", "COPILOT_PLUGIN_ROOT"} {
			literal = strings.ReplaceAll(literal, "${"+key+"}", "%"+key+"%")
		}
	}
	switch {
	case r.Field == "powershell":
		return "powershell.exe", []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", literal + "; exit $LASTEXITCODE"}, nil
	case goos == "windows":
		return "cmd.exe", []string{"/d", "/s", "/c", literal}, nil
	case r.Field == "bash":
		return "bash", []string{"-c", literal}, nil
	}
	return "sh", []string{"-c", literal}, nil
}
