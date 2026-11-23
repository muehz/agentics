package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
)

// RunSchema ports tests/hooks/run-hook-config-schema-fixtures.sh: it enforces
// the complete, provider-documented config field set, event commands, matcher
// roster, budgets, and forbidden fields for every enabled Agentics hook config —
// hooks/hooks.claude.json (Claude Code), hooks/hooks.copilot.json (Copilot CLI),
// hooks/hooks.codex.json (Codex), and hooks.json (Antigravity).
func RunSchema(repoRoot string) ([]Failure, error) {
	var failures []Failure

	configs := []struct {
		relPath  string
		provider string
		assert   func(path string, cfg map[string]any) []Failure
	}{
		{"hooks/hooks.claude.json", "claude-code", assertClaudeConfig},
		{"hooks/hooks.copilot.json", "copilot", assertCopilotConfig},
		{"hooks/hooks.codex.json", "codex", assertCodexConfig},
		{"hooks.json", "antigravity", assertAntigravityConfig},
	}

	parsed := map[string]map[string]any{} // provider -> config, for dispatch routing
	for _, c := range configs {
		path := filepath.Join(repoRoot, c.relPath)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				failures = append(failures, Failure{
					Rule: "config-schema-file", Path: c.relPath,
					Message: "expected hook config file is missing",
				})
				continue
			}
			return failures, fmt.Errorf("reading %s: %w", c.relPath, err)
		}
		var cfg map[string]any
		if err := json.Unmarshal(data, &cfg); err != nil {
			failures = append(failures, Failure{
				Rule: "config-schema-json", Path: c.relPath,
				Message: fmt.Sprintf("invalid JSON: %v", err),
			})
			continue
		}
		parsed[c.provider] = cfg
		failures = append(failures, c.assert(c.relPath, cfg)...)
	}

	failures = append(failures, assertDispatchRouting(parsed)...)
	return failures, nil
}

// dispatchCommand renders a legacy per-hook command with explicit flags.
func dispatchCommand(prefix, provider, hook, flags string) string {
	return fmt.Sprintf("%s %s %s %s", prefix, provider, hook, flags)
}

func fail(rule, path, format string, args ...any) Failure {
	return Failure{Rule: rule, Path: path, Message: fmt.Sprintf(format, args...)}
}

func keySet(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// fieldSetFailures checks one handler entry against the allowed and forbidden
// field sets. These assertions remain defense in depth after the exact-match
// comparison so malformed extras get a targeted failure.
func fieldSetFailures(rule, path, label string, entry map[string]any, allowed, forbidden []string) []Failure {
	var failures []Failure
	allowedSet := map[string]bool{}
	for _, f := range allowed {
		allowedSet[f] = true
	}
	var extra, hit []string
	for k := range entry {
		if !allowedSet[k] {
			extra = append(extra, k)
		}
	}
	for _, f := range forbidden {
		if _, ok := entry[f]; ok {
			hit = append(hit, f)
		}
	}
	sort.Strings(extra)
	sort.Strings(hit)
	if len(extra) > 0 {
		failures = append(failures, fail(rule, path, "%s carries undocumented fields: %v", label, extra))
	}
	if len(hit) > 0 {
		failures = append(failures, fail(rule, path, "%s carries forbidden fields: %v", label, hit))
	}
	return failures
}

// nestedGroup extracts a Claude/Codex-format group and its single handler:
// entries[index].hooks must be a list of exactly one map.
func nestedGroup(entries []any, index int) (map[string]any, map[string]any, bool) {
	group, ok := entries[index].(map[string]any)
	if !ok {
		return nil, nil, false
	}
	handlers, ok := group["hooks"].([]any)
	if !ok || len(handlers) != 1 {
		return nil, nil, false
	}
	handler, ok := handlers[0].(map[string]any)
	return group, handler, ok
}

func hooksMap(cfg map[string]any) map[string]any {
	hooks, _ := cfg["hooks"].(map[string]any)
	if hooks == nil {
		return map[string]any{}
	}
	return hooks
}

func eventList(hooks map[string]any, event string) ([]any, bool) {
	entries, ok := hooks[event].([]any)
	return entries, ok
}

// assertNestedConfig covers the shared Claude/Codex shape: nested groups under
// PascalCase events, one handler per expected entry, exact group fields, and
// provider-specific matcher values when expected matchers are supplied.
//
// The group count is driven by len(expectedPre) rather than a literal, so
// adding or retiring a hook on the event is a one-line change at the caller
// instead of a rewrite here.
func assertNestedConfig(path, rulePrefix string, cfg map[string]any, expectedPre []map[string]any, expectedPreMatchers []string, startup []any, allowed, forbidden []string) []Failure {
	var failures []Failure
	hooks := hooksMap(cfg)

	events := keySet(hooks)
	if !reflect.DeepEqual(events, []string{"PreToolUse", "SessionStart"}) {
		failures = append(failures, fail(rulePrefix+"-events", path, "event roster mismatch: %v (want PreToolUse and SessionStart)", events))
		return failures
	}
	if !reflect.DeepEqual(hooks["SessionStart"], startup) {
		failures = append(failures, fail(rulePrefix+"-startup", path, "SessionStart config mismatch: got %v, want %v", hooks["SessionStart"], startup))
	}

	preEntries, ok := eventList(hooks, "PreToolUse")
	if !ok || len(preEntries) != len(expectedPre) {
		failures = append(failures, fail(rulePrefix+"-shape", path, "PreToolUse must contain exactly %d matcher group(s), got %d", len(expectedPre), len(preEntries)))
		return failures
	}
	preGroups := make([]map[string]any, len(expectedPre))
	pre := make([]map[string]any, len(expectedPre))
	for i := range pre {
		group, handler, ok := nestedGroup(preEntries, i)
		if !ok {
			failures = append(failures, fail(rulePrefix+"-shape", path, "PreToolUse matcher group %d must contain exactly one handler", i))
			return failures
		}
		preGroups[i] = group
		pre[i] = handler
	}
	if !reflect.DeepEqual(pre, expectedPre) {
		failures = append(failures, fail(rulePrefix+"-pre", path, "PreToolUse config mismatch: got %v, want %v", pre, expectedPre))
	}

	for i, entry := range pre {
		label := fmt.Sprintf("PreToolUse[%d]", i)
		groupAllowed := []string{"hooks"}
		if expectedPreMatchers != nil {
			groupAllowed = append(groupAllowed, "matcher")
			matcher, ok := preGroups[i]["matcher"].(string)
			if !ok {
				failures = append(failures, fail(rulePrefix+"-matcher", path, "%s group must carry a string matcher", label))
			} else if matcher != expectedPreMatchers[i] {
				failures = append(failures, fail(rulePrefix+"-matcher", path, "%s matcher mismatch: got %q, want %q", label, matcher, expectedPreMatchers[i]))
			}
			if _, misplaced := entry["matcher"]; misplaced {
				failures = append(failures, fail(rulePrefix+"-matcher", path, "%s carries matcher inside the handler; matcher belongs on the containing group", label))
			}
		}
		failures = append(failures, fieldSetFailures(rulePrefix+"-group-fields", path, label+" group", preGroups[i], groupAllowed, nil)...)
		failures = append(failures, fieldSetFailures(rulePrefix+"-fields", path, label, entry, allowed, forbidden)...)
	}
	return failures
}

// eventCommand renders the canonical event dispatch without legacy hook flags.
func eventCommand(prefix, provider, verb string) string {
	return fmt.Sprintf("%s %s %s", prefix, provider, verb)
}

func freshSessionGroup(command map[string]any) []any {
	return []any{map[string]any{"matcher": "startup", "hooks": []any{command}}}
}

func assertClaudeConfig(path string, cfg map[string]any) []Failure {
	prefix := `"${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd"`
	expectedPre := []map[string]any{{
		"type": "command", "command": eventCommand(prefix, "claude-code", "pre-tool"), "timeout": float64(25),
	}}
	return assertNestedConfig(path, "config-schema-claude", cfg, expectedPre,
		[]string{`^(?:Read|Grep|Glob|NotebookRead|Bash|PowerShell|Write|Edit|MultiEdit|NotebookEdit|mcp__.*)$`},
		freshSessionGroup(map[string]any{"type": "command", "command": eventCommand(prefix, "claude-code", "startup"), "timeout": float64(25)}),
		[]string{"type", "command", "timeout"}, []string{"windows", "env", "timeoutSec", "bash", "powershell", "commandWindows"})
}

func assertCodexConfig(path string, cfg map[string]any) []Failure {
	prefix := `"${PLUGIN_ROOT}/hooks/run-hook.cmd"`
	winPrefix := `cmd /c "%PLUGIN_ROOT%\hooks\run-hook.cmd"`
	expectedPre := []map[string]any{{
		"type": "command", "command": eventCommand(prefix, "codex", "pre-tool"),
		"commandWindows": eventCommand(winPrefix, "codex", "pre-tool"), "timeout": float64(25),
	}}
	return assertNestedConfig(path, "config-schema-codex", cfg, expectedPre,
		[]string{`*`},
		freshSessionGroup(map[string]any{
			"type": "command", "command": eventCommand(prefix, "codex", "startup"),
			"commandWindows": eventCommand(winPrefix, "codex", "startup"), "timeout": float64(25),
		}),
		[]string{"type", "command", "commandWindows", "timeout"}, []string{"windows", "env", "timeoutSec", "bash", "powershell"})
}

func assertCopilotConfig(path string, cfg map[string]any) []Failure {
	var failures []Failure
	rulePrefix := "config-schema-copilot"
	if version, ok := cfg["version"].(float64); !ok || version != 1 {
		failures = append(failures, fail(rulePrefix+"-version", path, "config must declare version: 1, got %v", cfg["version"]))
	}
	hooks := hooksMap(cfg)
	events := keySet(hooks)
	if !reflect.DeepEqual(events, []string{"preToolUse", "sessionStart"}) {
		failures = append(failures, fail(rulePrefix+"-events", path, "event roster mismatch: %v (want preToolUse and sessionStart)", events))
		return failures
	}
	prefix := `"${COPILOT_PLUGIN_ROOT}/hooks/run-hook.cmd"`
	psPrefix := `& "$env:COPILOT_PLUGIN_ROOT\hooks\run-hook.cmd"`
	startup := []any{map[string]any{
		"type": "command", "bash": eventCommand(prefix, "copilot", "startup"),
		"powershell": eventCommand(psPrefix, "copilot", "startup"), "timeoutSec": float64(25),
	}}
	if !reflect.DeepEqual(hooks["sessionStart"], startup) {
		failures = append(failures, fail(rulePrefix+"-startup", path, "sessionStart config mismatch: got %v, want %v", hooks["sessionStart"], startup))
	}
	expectedPre := []any{map[string]any{
		"type": "command", "matcher": ".*", "bash": eventCommand(prefix, "copilot", "pre-tool"),
		"powershell": eventCommand(psPrefix, "copilot", "pre-tool"), "timeoutSec": float64(25),
	}}
	if !reflect.DeepEqual(hooks["preToolUse"], expectedPre) {
		failures = append(failures, fail(rulePrefix+"-pre", path, "preToolUse config mismatch: got %v, want %v", hooks["preToolUse"], expectedPre))
	}
	allowed := []string{"type", "matcher", "bash", "powershell", "timeoutSec"}
	forbidden := []string{"command", "commandWindows", "timeout", "windows", "env"}
	entries, _ := eventList(hooks, "preToolUse")
	for i, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok {
			failures = append(failures, fail(rulePrefix+"-fields", path, "preToolUse[%d] is not an object", i))
			continue
		}
		failures = append(failures, fieldSetFailures(rulePrefix+"-fields", path, fmt.Sprintf("preToolUse[%d]", i), entry, allowed, forbidden)...)
	}
	return failures
}

// assertAntigravityConfig checks root hooks.json: exact top-level "agentics"
// namespace carrying a grouped PreToolUse command and a flat SessionStart command.
func assertAntigravityConfig(path string, cfg map[string]any) []Failure {
	const rulePrefix = "config-schema-antigravity"
	topKeys := keySet(cfg)
	if !reflect.DeepEqual(topKeys, []string{"agentics"}) {
		return []Failure{fail(rulePrefix+"-root", path, "top-level key mismatch: %v (want exactly agentics)", topKeys)}
	}
	agentics, ok := cfg["agentics"].(map[string]any)
	if !ok {
		return []Failure{fail(rulePrefix+"-root", path, "top-level agentics value must be an object")}
	}
	prefix := `"./hooks/run-hook.cmd"`
	expectedPre := []map[string]any{{
		"type": "command", "command": eventCommand(prefix, "antigravity", "pre-tool"), "timeout": float64(25),
	}}
	return assertNestedConfig(path, rulePrefix, map[string]any{"hooks": agentics}, expectedPre,
		[]string{`*`},
		[]any{map[string]any{"type": "command", "command": eventCommand(prefix, "antigravity", "startup"), "timeout": float64(25)}},
		[]string{"type", "command", "timeout"},
		[]string{"windows", "env", "timeoutSec", "bash", "powershell", "commandWindows"})
}

var routePattern = regexp.MustCompile(`run-hook\.cmd" (\S+) (\S+)`)

// commandKeys are every field a provider config carries a shell command in.
var commandKeys = map[string]bool{
	"command": true, "commandWindows": true, "bash": true, "powershell": true,
}

// providerHooks is the supported event and legacy hook roster per provider token.
var providerHooks = map[string]map[string]bool{
	"claude-code": {"startup": true, "pre-tool": true, "command-guardian": true, "secrets-guardian": true, "release-guardian": true},
	"copilot":     {"startup": true, "pre-tool": true, "command-guardian": true, "secrets-guardian": true, "release-guardian": true},
	"codex":       {"startup": true, "pre-tool": true, "command-guardian": true, "secrets-guardian": true, "release-guardian": true},
	"antigravity": {"startup": true, "pre-tool": true, "command-guardian": true, "secrets-guardian": true, "release-guardian": true},
}

// assertDispatchRouting: every hook command across all four configs must
// route through run-hook.cmd with the config's own provider token and a
// documented hook name, wrapped in a quoted path so spaces in the install
// directory survive. Finding zero dispatch commands is itself a violation.
func assertDispatchRouting(parsed map[string]map[string]any) []Failure {
	var failures []Failure
	relPaths := map[string]string{
		"claude-code": "hooks/hooks.claude.json",
		"copilot":     "hooks/hooks.copilot.json",
		"codex":       "hooks/hooks.codex.json",
		"antigravity": "hooks.json",
	}
	foundAny := false
	for _, provider := range []string{"claude-code", "copilot", "codex", "antigravity"} {
		cfg, ok := parsed[provider]
		if !ok {
			continue // missing or unparseable config already failed above
		}
		path := relPaths[provider]
		for _, command := range walkCommands(cfg) {
			match := routePattern.FindStringSubmatch(command)
			if match == nil {
				failures = append(failures, fail("config-schema-dispatch-routing", path, "command does not route through a quoted run-hook.cmd: %q", command))
				continue
			}
			matchedProvider, hookName := match[1], match[2]
			if matchedProvider != provider {
				failures = append(failures, fail("config-schema-dispatch-routing", path, "command targets provider %q, expected %q: %q", matchedProvider, provider, command))
				continue
			}
			if !providerHooks[provider][hookName] {
				failures = append(failures, fail("config-schema-dispatch-routing", path, "command targets undocumented hook %q: %q", hookName, command))
				continue
			}
			foundAny = true
		}
	}
	if !foundAny {
		failures = append(failures, fail("config-schema-dispatch-routing", "",
			"no dispatch commands were found across any provider config"))
	}
	return failures
}

// walkCommands yields every string value under a command-carrying key,
// depth-first, in stable key order — the Go analogue of the harness's walk().
func walkCommands(node any) []string {
	var out []string
	switch typed := node.(type) {
	case map[string]any:
		for _, key := range keySet(typed) {
			value := typed[key]
			if str, ok := value.(string); ok && commandKeys[key] {
				out = append(out, str)
				continue
			}
			out = append(out, walkCommands(value)...)
		}
	case []any:
		for _, item := range typed {
			out = append(out, walkCommands(item)...)
		}
	}
	return out
}
