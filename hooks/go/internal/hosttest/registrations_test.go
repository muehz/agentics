package hosttest_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestRegistrationsShipped(t *testing.T) {
	all, err := hosttest.Registrations(filepath.Join(hosttest.Checkout(t), "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		surface               surface.ID
		event, field, matcher string
	}
	want := []row{
		{"claude-code", "PreToolUse", "command", "^(?:Read|Grep|Glob|NotebookRead|Bash|PowerShell|Write|Edit|MultiEdit|NotebookEdit|mcp__.*)$"},
		{"claude-code", "SessionStart", "command", "startup"},
		{"copilot", "preToolUse", "bash", ".*"},
		{"copilot", "preToolUse", "powershell", ".*"},
		{"copilot", "sessionStart", "bash", ""},
		{"copilot", "sessionStart", "powershell", ""},
		{"codex", "PreToolUse", "command", "*"},
		{"codex", "PreToolUse", "commandWindows", "*"},
		{"codex", "SessionStart", "command", "startup"},
		{"codex", "SessionStart", "commandWindows", "startup"},
		{"antigravity", "PreToolUse", "command", "*"},
		{"antigravity", "SessionStart", "command", ""},
	}
	if len(all) != len(want) {
		t.Fatalf("Registrations returned %d commands, want %d: %+v", len(all), len(want), all)
	}
	for i, r := range all {
		got := row{r.Surface, r.Event, r.Field, r.Matcher}
		hook := "pre-tool"
		if strings.EqualFold(want[i].event, "SessionStart") {
			hook = "startup"
		}
		if got != want[i] || r.HasMatcher != (want[i].matcher != "") || r.Hook != hook || len(r.Args) != 0 || r.Timeout != 25*time.Second {
			t.Errorf("registration %d = %+v, want %+v with hook %s, no args, 25s", i, r, want[i], hook)
		}
		if !strings.Contains(r.Command, `run-hook.cmd" `+string(r.Surface)+" "+hook) {
			t.Errorf("registration %d command %q is not the field verbatim", i, r.Command)
		}
	}
}

// validConfigs are minimal well-formed configs in the four shipped shapes.
// hooks.json lives at the fixture root; the others live under hooks/.
var validConfigs = map[string]string{
	"hooks.claude.json":  `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"\"${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd\" claude-code pre-tool","timeout":25}]}]}}`,
	"hooks.copilot.json": `{"version":1,"hooks":{"preToolUse":[{"type":"command","bash":"\"${COPILOT_PLUGIN_ROOT}/hooks/run-hook.cmd\" copilot pre-tool","timeoutSec":25}]}}`,
	"hooks.codex.json":   `{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"\"${PLUGIN_ROOT}/hooks/run-hook.cmd\" codex pre-tool","timeout":25}]}]}}`,
	"hooks.json":         `{"agentics":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"\"./hooks/run-hook.cmd\" antigravity pre-tool","timeout":25}]}]}}`,
}

func configPath(hooksDir, name string) string {
	if name == "hooks.json" {
		return filepath.Join(filepath.Dir(hooksDir), name)
	}
	return filepath.Join(hooksDir, name)
}

func writeConfigs(t *testing.T, overrides map[string]string) string {
	t.Helper()
	root := t.TempDir()
	hooksDir := filepath.Join(root, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range validConfigs {
		if override, ok := overrides[name]; ok {
			body = override
		}
		if err := os.WriteFile(configPath(hooksDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return hooksDir
}

func TestRegistrationsFixtureValues(t *testing.T) {
	dir := writeConfigs(t, map[string]string{
		"hooks.claude.json":  `{"hooks":{"Stop":[{"hooks":[{"command":"\"x/run-hook.cmd\" claude-code stop --flag v","timeout":1.5}]}],"PreToolUse":[{"hooks":[{"command":"\"x/run-hook.cmd\" claude-code pre-tool","timeout":2}]}]}}`,
		"hooks.copilot.json": `{"hooks":{"preToolUse":[{"matcher":"","bash":"\"x/run-hook.cmd\" copilot pre-tool","timeoutSec":0.25}]}}`,
	})
	all, err := hosttest.Registrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []hosttest.Registration{
		{Surface: "claude-code", Event: "PreToolUse", Field: "command", Command: `"x/run-hook.cmd" claude-code pre-tool`, Hook: "pre-tool", Args: []string{}, Timeout: 2 * time.Second},
		{Surface: "claude-code", Event: "Stop", Field: "command", Command: `"x/run-hook.cmd" claude-code stop --flag v`, Hook: "stop", Args: []string{"--flag", "v"}, Timeout: 1500 * time.Millisecond},
		{Surface: "copilot", Event: "preToolUse", HasMatcher: true, Field: "bash", Command: `"x/run-hook.cmd" copilot pre-tool`, Hook: "pre-tool", Args: []string{}, Timeout: 250 * time.Millisecond},
		{Surface: "codex", Event: "PreToolUse", Matcher: "*", HasMatcher: true, Field: "command", Command: `"${PLUGIN_ROOT}/hooks/run-hook.cmd" codex pre-tool`, Hook: "pre-tool", Args: []string{}, Timeout: 25 * time.Second},
		{Surface: "antigravity", Event: "PreToolUse", Matcher: "*", HasMatcher: true, Field: "command", Command: `"./hooks/run-hook.cmd" antigravity pre-tool`, Hook: "pre-tool", Args: []string{}, Timeout: 25 * time.Second},
	}
	if !reflect.DeepEqual(all, want) {
		t.Fatalf("Registrations =\n%+v\nwant\n%+v", all, want)
	}
}

func TestRegistrationsRejectsMalformed(t *testing.T) {
	claude := func(group string) string { return `{"hooks":{"PreToolUse":[` + group + `]}}` }
	command := func(fields string) string { return claude(`{"hooks":[{` + fields + `}]}`) }
	copilot := func(entry string) string { return `{"hooks":{"preToolUse":[{` + entry + `}]}}` }
	const okCommand = `"command":"\"x/run-hook.cmd\" claude-code pre-tool"`
	const okBash = `"bash":"\"x/run-hook.cmd\" copilot pre-tool"`
	cases := []struct {
		name, file, body string
	}{
		{"unreadable", "hooks.claude.json", ""},
		{"invalid json", "hooks.claude.json", `{"hooks":`},
		{"trailing data", "hooks.claude.json", validConfigs["hooks.claude.json"] + ` {}`},
		{"hooks missing", "hooks.claude.json", `{}`},
		{"hooks not an object", "hooks.claude.json", `{"hooks":[]}`},
		{"hooks empty", "hooks.claude.json", `{"hooks":{}}`},
		{"event not an array", "hooks.claude.json", `{"hooks":{"PreToolUse":{}}}`},
		{"group not an object", "hooks.claude.json", claude(`"x"`)},
		{"group without hooks", "hooks.claude.json", claude(`{"matcher":"Bash"}`)},
		{"group hooks not an array", "hooks.codex.json", claude(`{"hooks":{}}`)},
		{"nested command not an object", "hooks.claude.json", claude(`{"hooks":[1]}`)},
		{"copilot entry not an object", "hooks.copilot.json", `{"hooks":{"preToolUse":[null]}}`},
		{"matcher null", "hooks.claude.json", claude(`{"matcher":null,"hooks":[{` + okCommand + `,"timeout":1}]}`)},
		{"matcher number", "hooks.codex.json", claude(`{"matcher":1,"hooks":[{` + okCommand + `,"timeout":1}]}`)},
		{"copilot matcher null", "hooks.copilot.json", copilot(`"matcher":null,` + okBash + `,"timeoutSec":1`)},
		{"copilot matcher boolean", "hooks.copilot.json", copilot(`"matcher":true,` + okBash + `,"timeoutSec":1`)},
		{"copilot matcher array", "hooks.copilot.json", copilot(`"matcher":[],` + okBash + `,"timeoutSec":1`)},
		{"copilot matcher object", "hooks.copilot.json", copilot(`"matcher":{},` + okBash + `,"timeoutSec":1`)},
		{"command field not a string", "hooks.claude.json", command(`"command":1,"timeout":1`)},
		{"powershell field not a string", "hooks.copilot.json", copilot(okBash + `,"powershell":null,"timeoutSec":1`)},
		{"timeout missing", "hooks.claude.json", command(okCommand)},
		{"timeout zero", "hooks.claude.json", command(okCommand + `,"timeout":0`)},
		{"timeout negative", "hooks.codex.json", command(okCommand + `,"timeout":-1`)},
		{"timeout string", "hooks.claude.json", command(okCommand + `,"timeout":"25"`)},
		{"claude timeout under the copilot key", "hooks.claude.json", command(okCommand + `,"timeout":1,"timeoutSec":1`)},
		{"copilot timeout under the claude key", "hooks.copilot.json", copilot(okBash + `,"timeout":1`)},
		{"no command fields", "hooks.claude.json", command(`"type":"command","timeout":1`)},
		{"no registrations", "hooks.claude.json", claude(`{"hooks":[]}`)},
		{"run-hook without the quoted marker", "hooks.claude.json", command(`"command":"x/run-hook.cmd claude-code pre-tool","timeout":1`)},
		{"command without run-hook", "hooks.claude.json", command(`"command":"echo hi","timeout":1`)},
		{"one word after the marker", "hooks.claude.json", command(`"command":"\"x/run-hook.cmd\" claude-code","timeout":1`)},
		{"unknown surface", "hooks.claude.json", command(`"command":"\"x/run-hook.cmd\" cursor pre-tool","timeout":1`)},
		{"surface in another letter case", "hooks.claude.json", command(`"command":"\"x/run-hook.cmd\" Claude-Code pre-tool","timeout":1`)},
		{"antigravity agentics missing", "hooks.json", `{}`},
		{"antigravity agentics empty", "hooks.json", `{"agentics":{}}`},
		{"antigravity timeout under copilot key", "hooks.json", `{"agentics":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"\"./hooks/run-hook.cmd\" antigravity pre-tool","timeoutSec":25}]}]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeConfigs(t, map[string]string{tc.file: tc.body})
			path := configPath(dir, tc.file)
			if tc.name == "unreadable" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			all, err := hosttest.Registrations(dir)
			if err == nil {
				t.Fatalf("Registrations accepted %s: %+v", tc.body, all)
			}
			if !strings.HasPrefix(err.Error(), path) {
				t.Fatalf("error %q does not start with the file path %q", err, path)
			}
		})
	}
}

// selectionField is the field Selected takes for a surface on linux.
var selectionField = map[surface.ID]string{
	surface.Antigravity: "command",
	surface.ClaudeCode:  "command",
	surface.Copilot:     "bash",
	surface.Codex:       "command",
}

// matches reports whether Selected keeps one pre-tool registration with the
// given matcher for tool.
func matches(t *testing.T, surface surface.ID, pattern string, hasMatcher bool, tool string) (bool, error) {
	t.Helper()
	r := hosttest.Registration{Surface: surface, Event: "PreToolUse", Matcher: pattern, HasMatcher: hasMatcher, Field: selectionField[surface], Command: "c", Hook: "pre-tool", Timeout: time.Second}
	selected, err := hosttest.Selected([]hosttest.Registration{r}, surface, tool, "linux")
	return len(selected) == 1, err
}

func TestSelectedMatcher(t *testing.T) {
	cases := []struct {
		surface       surface.ID
		pattern, tool string
		want          bool
	}{
		// Rows carried over from the matcher test this one replaces, tools unchanged.
		{"claude-code", "Edit", "Edit", true},
		{"claude-code", "Edit", "CustomEdit", false},
		{"claude-code", "Edit", "mcp__foo__Edit", false},
		{"claude-code", "Edit.*", "CustomEdit", true},
		{"codex", "Edit", "apply_patch", true},
		{"copilot", "edit", "edit", true},
		{"copilot", "edit", "CustomEdit", false},
		{"copilot", "edit", "customedit", false},
		// Rows carried over from the hook-cost matcher test, whose Copilot rows
		// naming Bash or Read pass through NativeTool.
		{"claude-code", "Edit|Write", "Edit", true},
		{"claude-code", "Edit|Write", "CustomEdit", false},
		{"claude-code", "Edit|Write", "mcp__foo__Edit", false},
		{"claude-code", "^Edit$", "CustomEdit", false},
		{"claude-code", "Edit", "CustomEdit", false},
		{"claude-code", "Edit, Write", "Write", true},
		{"claude-code", "Edit.*", "CustomEdit", true},
		{"codex", "*", "unknown", true},
		{"codex", "^Edit$", "apply_patch", true},
		{"codex", "^Write$", "apply_patch", true},
		{"copilot", "bash|edit", hosttest.NativeTool("copilot", "Bash"), true},
		{"copilot", "bash|edit", hosttest.NativeTool("copilot", "Read"), false},
		{"copilot", "edit", "edit", true},
		{"copilot", "edit", "CustomEdit", false},
		{"copilot", "edit", "customedit", false},
		// The named difference: an explicit empty Copilot matcher matches nothing.
		{"copilot", "", "unknown", false},
		// Further vendor rules.
		{"claude-code", "*", "anything", true},
		{"claude-code", "", "anything", true},
		{"codex", "", "anything", true},
		{"codex", "^Read$", "apply_patch", false},
		{"claude-code", "^Edit$", "apply_patch", false},
	}
	for _, c := range cases {
		t.Run(string(c.surface)+"/"+c.pattern+"/"+c.tool, func(t *testing.T) {
			got, err := matches(t, c.surface, c.pattern, true, c.tool)
			if err != nil || got != c.want {
				t.Fatalf("matches = %v, %v; want %v", got, err, c.want)
			}
		})
	}
	t.Run("copilot omitted matcher matches every tool", func(t *testing.T) {
		for _, tool := range []string{"bash", "view", "unknown", ""} {
			if got, err := matches(t, "copilot", "", false, tool); err != nil || !got {
				t.Fatalf("omitted matcher for %q = %v, %v; want true", tool, got, err)
			}
		}
	})
	for _, c := range []struct {
		surface surface.ID
		pattern string
	}{{"copilot", "["}, {"codex", "("}, {"claude-code", "Edit["}, {"copilot", "*"}} {
		t.Run("bad pattern "+string(c.surface)+"/"+c.pattern, func(t *testing.T) {
			_, err := matches(t, c.surface, c.pattern, true, "edit")
			prefix := string(c.surface) + ` matcher "` + c.pattern + `": `
			if err == nil || !strings.HasPrefix(err.Error(), prefix) {
				t.Fatalf("bad matcher error = %v, want prefix %q", err, prefix)
			}
		})
	}
}

func TestSelectedPerOS(t *testing.T) {
	all, err := hosttest.Registrations(filepath.Join(hosttest.Checkout(t), "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	fields := map[surface.ID]map[string]string{
		surface.Antigravity: {"linux": "command", "darwin": "command", "windows": "command"},
		surface.ClaudeCode:  {"linux": "command", "darwin": "command", "windows": "command"},
		surface.Codex:       {"linux": "command", "darwin": "command", "windows": "commandWindows"},
		surface.Copilot:     {"linux": "bash", "darwin": "bash", "windows": "powershell"},
	}
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, surface := range surface.All() {
			for _, neutral := range []string{"Bash", "Read"} {
				selected, err := hosttest.Selected(all, surface, hosttest.NativeTool(surface, neutral), goos)
				if err != nil {
					t.Fatal(err)
				}
				if len(selected) != 1 {
					t.Fatalf("%s %s/%s selected %d registrations, want 1: %+v", goos, surface, neutral, len(selected), selected)
				}
				if got := selected[0]; got.Field != fields[surface][goos] || got.Surface != surface || got.Event != map[bool]string{true: "preToolUse", false: "PreToolUse"}[surface == "copilot"] {
					t.Fatalf("%s %s/%s selected %+v, want field %s", goos, surface, neutral, got, fields[surface][goos])
				}
			}
		}
	}
}

func TestSelectedKeepsOnlyPreToolUse(t *testing.T) {
	all := []hosttest.Registration{
		{Surface: "claude-code", Event: "Stop", Field: "command", Command: "c"},
		{Surface: "claude-code", Event: "preToolUse", Field: "command", Command: "c"},
		{Surface: "claude-code", Event: "PostToolUse", Field: "command", Command: "c"},
	}
	selected, err := hosttest.Selected(all, "claude-code", "Bash", "linux")
	if err != nil || len(selected) != 1 || selected[0].Event != "preToolUse" {
		t.Fatalf("Selected = %+v, %v; want only the preToolUse registration", selected, err)
	}
}

func TestSelectedUnknownSurface(t *testing.T) {
	_, err := hosttest.Selected(nil, "cursor", "Bash", "linux")
	if err == nil || err.Error() != `unknown surface "cursor"` {
		t.Fatalf("Selected error = %v, want unknown surface \"cursor\"", err)
	}
}

func TestHostCommand(t *testing.T) {
	const command = `"${CLAUDE_PLUGIN_ROOT}/a" "${PLUGIN_ROOT}/b" "${COPILOT_PLUGIN_ROOT}/c" "${OTHER}/d"`
	const rewritten = `"%CLAUDE_PLUGIN_ROOT%/a" "%PLUGIN_ROOT%/b" "%COPILOT_PLUGIN_ROOT%/c" "${OTHER}/d"`
	powershell := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", command + "; exit $LASTEXITCODE"}
	cases := []struct {
		field, goos, name string
		args              []string
	}{
		{"command", "windows", "cmd.exe", []string{"/d", "/s", "/c", rewritten}},
		{"command", "linux", "sh", []string{"-c", command}},
		{"commandWindows", "windows", "cmd.exe", []string{"/d", "/s", "/c", command}},
		{"commandWindows", "linux", "sh", []string{"-c", command}},
		{"bash", "windows", "cmd.exe", []string{"/d", "/s", "/c", command}},
		{"bash", "linux", "bash", []string{"-c", command}},
		{"powershell", "windows", "powershell.exe", powershell},
		{"powershell", "linux", "powershell.exe", powershell},
	}
	for _, c := range cases {
		t.Run(c.field+"/"+c.goos, func(t *testing.T) {
			name, args, err := hosttest.Registration{Field: c.field, Command: command}.HostCommand(c.goos)
			if err != nil || name != c.name || !reflect.DeepEqual(args, c.args) {
				t.Fatalf("HostCommand = %q, %q, %v; want %q, %q", name, args, err, c.name, c.args)
			}
		})
	}
	for _, r := range []hosttest.Registration{{Field: "shell", Command: command}, {Field: "", Command: command}, {Field: "command"}} {
		if name, args, err := r.HostCommand("linux"); err == nil {
			t.Fatalf("HostCommand(%+v) = %q, %q; want an error", r, name, args)
		}
	}
}
