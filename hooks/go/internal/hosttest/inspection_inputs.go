package hosttest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// ShellPayload is one shell-shaped hook invocation a test replays through a
// provider's dispatch and normalize steps: the surface, the hook name and argv
// the invocation names, and the raw payload the surface's host writes on stdin.
// Dir is a directory that holds the fixture files the payload's cwd names, or
// empty when the payload needs none.
type ShellPayload struct {
	Name    string
	Surface surface.ID
	Hook    string
	Args    []string
	Stdin   []byte
	Dir     string
}

// Probe is one neutral-level tool call: the surface and tool name an event
// carries, the decoded tool input, and the event's working directory. An empty
// Cwd means the event has none.
type Probe struct {
	Name    string
	Surface surface.ID
	Tool    string
	Input   map[string]any
	Cwd     string
}

const (
	corpusEntriesFile     = "hooks/go/cmd/hook/testdata/corpus/entries.json"
	guardRegressionsFile  = "hooks/go/cmd/hook/testdata/guard-regressions.json"
	corpusPayloadsDir     = "hooks/go/surfaces"
	corpusFixtureToken    = "{{FIXTURE}}"
	corpusAWSKeyToken     = "{{AWS_KEY}}"
	regressionHook        = "pre-tool"
	corpusFixtureSubdir   = "fixture"
	regressionFixtureName = "regression-fixture"
)

// corpusAWSKey is the AWS-key-shaped text the corpus token stands for. A literal
// one in a committed file would trip the secrets guard the corpus exercises.
func corpusAWSKey() string { return "AKIA" + strings.Repeat("0", 16) }

type corpusTable struct {
	Templates map[string]map[string]json.RawMessage `json:"templates"`
	Entries   []corpusEntry                         `json:"entries"`
}

type corpusEntry struct {
	Name    string          `json:"name"`
	Hook    string          `json:"hook"`
	Argv    []string        `json:"argv"`
	Stdin   json.RawMessage `json:"stdin"`
	Fixture *struct {
		Kind  string            `json:"kind"`
		Files map[string]string `json:"files"`
	} `json:"fixture"`
	Only []string `json:"only"`
}

type regressionFile struct {
	Files map[string]string `json:"files"`
	Cases []struct {
		ID           string            `json:"id"`
		Tool         string            `json:"tool"`
		Command      string            `json:"command"`
		Shell        string            `json:"shell"`
		ToolEnvField string            `json:"tool_env_field"`
		ToolEnv      map[string]string `json:"tool_env"`
		ToolEnvRaw   json.RawMessage   `json:"tool_env_raw"`
	} `json:"cases"`
}

// ShellPayloads returns every shell-shaped payload the behavior corpus and the
// guard-regression contract hold, found through Checkout. A corpus entry is
// taken once per surface it applies to, with its own hook and argv, and an
// entry whose payload is not a shell call on a surface is skipped without
// failing. Each regression case is taken once on each of the four surfaces, as
// the grouped pre-tool event, with the case's tool, command, shell, and tool
// environment, run in one directory that holds the file's fixture files.
func ShellPayloads(t testing.TB) []ShellPayload {
	t.Helper()
	root := Checkout(t)
	payloads := corpusPayloads(t, root)
	return append(payloads, regressionPayloads(t, root)...)
}

func corpusPayloads(t testing.TB, root string) []ShellPayload {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(corpusEntriesFile)))
	if err != nil {
		t.Fatalf("read the corpus: %v", err)
	}
	var table corpusTable
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatalf("parse the corpus: %v", err)
	}
	var payloads []ShellPayload
	for _, entry := range table.Entries {
		dir := ""
		for _, id := range surface.All() {
			if !entryApplies(entry, id) {
				continue
			}
			stdin, ok := corpusStdin(t, root, table, entry, id)
			if !ok || !shellShaped(id, stdin) {
				continue
			}
			payloadDir := ""
			if entry.Fixture != nil || bytes.Contains(stdin, []byte(corpusFixtureToken)) {
				if dir == "" {
					dir = corpusFixtureDir(t, entry)
				}
				payloadDir = dir
				stdin = expandFixture(stdin, dir)
			}
			payloads = append(payloads, ShellPayload{
				Name:    "corpus/" + entry.Name + "/" + string(id),
				Surface: id,
				Hook:    entry.Hook,
				Args:    append([]string(nil), entry.Argv...),
				Stdin:   stdin,
				Dir:     payloadDir,
			})
		}
	}
	return payloads
}

func entryApplies(entry corpusEntry, id surface.ID) bool {
	if len(entry.Only) == 0 {
		return true
	}
	for _, only := range entry.Only {
		if only == string(id) {
			return true
		}
	}
	return false
}

// corpusStdin resolves an entry's stdin field for one surface into the bytes
// the hook would read, leaving the fixture token in place. ok is false for an
// entry the table does not give a form on that surface.
func corpusStdin(t testing.TB, root string, table corpusTable, entry corpusEntry, id surface.ID) (stdin []byte, ok bool) {
	t.Helper()
	raw := entry.Stdin
	var bySurface map[string]json.RawMessage
	if json.Unmarshal(raw, &bySurface) == nil && len(bySurface) > 0 && allSurfaceKeys(bySurface) {
		if raw, ok = bySurface[string(id)]; !ok {
			return nil, false
		}
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		switch {
		case strings.HasPrefix(text, "@"):
			path, err := corpusPayloadPath(root, id, text[1:])
			if err != nil {
				t.Fatalf("corpus entry %q: %v", entry.Name, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("corpus entry %q: %v", entry.Name, err)
			}
			return expandAWSKey(data), true
		case strings.HasPrefix(text, "$"):
			form, found := table.Templates[text[1:]][string(id)]
			if !found {
				return nil, false
			}
			raw = form
		default:
			return expandAWSKey([]byte(text)), true
		}
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		t.Fatalf("corpus entry %q: stdin is not JSON: %v", entry.Name, err)
	}
	return expandAWSKey(compact.Bytes()), true
}

// corpusPayloadPath returns the path of the payload file an "@name" stdin form
// names on one surface. The name comes from the corpus table, so it must stay
// inside that surface's payloads directory.
func corpusPayloadPath(root string, id surface.ID, name string) (string, error) {
	local := filepath.FromSlash(name)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("payload file %q is not local to the payloads directory", name)
	}
	return filepath.Join(root, filepath.FromSlash(corpusPayloadsDir), string(id), "payloads", local), nil
}

func allSurfaceKeys(fields map[string]json.RawMessage) bool {
	for key := range fields {
		if _, known := surface.Parse(key); !known {
			return false
		}
	}
	return true
}

func expandAWSKey(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte(corpusAWSKeyToken), []byte(corpusAWSKey()))
}

// expandFixture substitutes the fixture directory, JSON-escaped, so a Windows
// path survives as a string value.
func expandFixture(stdin []byte, dir string) []byte {
	escaped, _ := json.Marshal(dir)
	return bytes.ReplaceAll(stdin, []byte(corpusFixtureToken), bytes.Trim(escaped, `"`))
}

// corpusFixtureDir writes an entry's fixture files into a fresh directory. It
// runs no git: a shell inspection reads files, not repository state. A
// "missing" fixture names a directory that is not created.
func corpusFixtureDir(t testing.TB, entry corpusEntry) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), corpusFixtureSubdir)
	if entry.Fixture != nil && entry.Fixture.Kind == "missing" {
		return dir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("corpus entry %q: %v", entry.Name, err)
	}
	if entry.Fixture != nil {
		writeFixtureFiles(t, dir, entry.Fixture.Files)
	}
	return dir
}

func writeFixtureFiles(t testing.TB, dir string, files map[string]string) {
	t.Helper()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !filepath.IsLocal(filepath.FromSlash(name)) {
			t.Fatalf("fixture path %q is not local", name)
		}
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
		content := strings.ReplaceAll(files[name], corpusAWSKeyToken, corpusAWSKey())
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
}

// shellShaped reports whether stdin is a tool call of a shell tool on the
// surface. The names are the ones each host sends, read from the surface's own
// envelope; a payload that is not one JSON object is not shell-shaped.
func shellShaped(id surface.ID, stdin []byte) bool {
	var payload map[string]any
	if err := json.Unmarshal(stdin, &payload); err != nil || payload == nil {
		return false
	}
	var name any
	switch id {
	case surface.Copilot:
		name = payload["toolName"]
	case surface.Antigravity:
		call, _ := payload["toolCall"].(map[string]any)
		name = call["name"]
	default:
		name = payload["tool_name"]
	}
	text, ok := name.(string)
	if !ok {
		return false
	}
	for _, sep := range []string{"__", ".", "/", ":"} {
		if i := strings.LastIndex(text, sep); i >= 0 {
			text = text[i+len(sep):]
		}
	}
	switch strings.ToLower(text) {
	case "bash", "powershell", "exec_command", "run_command":
		return true
	}
	return false
}

func regressionPayloads(t testing.TB, root string) []ShellPayload {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(guardRegressionsFile)))
	if err != nil {
		t.Fatalf("read the guard regressions: %v", err)
	}
	var file regressionFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parse the guard regressions: %v", err)
	}
	dir := filepath.Join(t.TempDir(), regressionFixtureName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixtureFiles(t, dir, file.Files)
	var payloads []ShellPayload
	for _, tc := range file.Cases {
		input := map[string]any{"command": tc.Command}
		field, value, err := regressionToolEnv(tc.ToolEnvField, tc.ToolEnv, tc.ToolEnvRaw)
		if err != nil {
			t.Fatalf("guard regression %s: %v", tc.ID, err)
		}
		if field != "" {
			input[field] = value
		}
		if tc.Shell != "" {
			input["shell"] = tc.Shell
		}
		for _, id := range surface.All() {
			// Codex and Antigravity hosts send every shell call under their own
			// tool name, so a PowerShell case there keeps the neutral name to
			// exercise adapter passthrough.
			native := (id == surface.Codex || id == surface.Antigravity) && tc.Tool == "PowerShell"
			payloads = append(payloads, ShellPayload{
				Name:    "regression/" + tc.ID + "/" + string(id),
				Surface: id,
				Hook:    regressionHook,
				Stdin:   Stdin(id, Call{Tool: tc.Tool, Input: cloneInput(input), Cwd: dir, Native: native}),
				Dir:     dir,
			})
		}
	}
	return payloads
}

func cloneInput(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

// regressionToolEnv returns the tool input key and value that carry a case's
// tool environment, or an empty field when it has none. The field is env or
// environment in any letter case and keeps its spelling. A raw value is used
// whenever it is present, an empty object included, while an empty tool_env is
// ignored; a case that gives both forms is an error.
func regressionToolEnv(field string, env map[string]string, raw json.RawMessage) (string, any, error) {
	if env != nil && raw != nil {
		return "", nil, fmt.Errorf("tool_env and tool_env_raw are both set")
	}
	if raw == nil && len(env) == 0 {
		return "", nil, nil
	}
	if field == "" {
		field = "env"
	}
	if !strings.EqualFold(field, "env") && !strings.EqualFold(field, "environment") {
		return "", nil, fmt.Errorf("unsupported tool environment field %q", field)
	}
	if raw == nil {
		return field, env, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", nil, fmt.Errorf("tool_env_raw is not JSON: %w", err)
	}
	return field, value, nil
}

// probeCommand is one value of the command axis. A nil text means the input
// has no command key at all.
type probeCommand struct {
	name string
	text *string
}

func commandAxis() []probeCommand {
	text := func(s string) *string { return &s }
	hereString := func(sink string) *string {
		return text("@'\ngh stack submit; bash t.sh\n'@ | " + sink)
	}
	return []probeCommand{
		{"posix", text("echo ok && rg needle file.txt")},
		{"bash-n", text("bash -n x")},
		{"bash-script", text("bash t.sh")},
		{"here-set-content", hereString("Set-Content -LiteralPath x")},
		{"here-out-file", hereString("Out-File -LiteralPath x")},
		{"empty", text("")},
		{"no-command", nil},
	}
}

// probeInput builds a fresh tool input: the command at the root when there is
// one, then the extras.
func probeInput(command probeCommand, extras map[string]any) map[string]any {
	input := map[string]any{}
	if command.text != nil {
		input["command"] = *command.text
	}
	for key, value := range extras {
		input[key] = value
	}
	return input
}

// probeEnvelope is one value of the argument-envelope axis. extras is built
// per surface and command, because the competing and alternate-key rows depend
// on both.
type probeEnvelope struct {
	name   string
	extras func(id surface.ID, command probeCommand) map[string]any
}

// alternateKey is the provider's own command key beside command: cmd on
// Copilot, CommandLine on Antigravity. The other surfaces have none, so cmd
// stands in to probe a key their calls never carry.
func alternateKey(id surface.ID) string {
	if id == surface.Antigravity {
		return "CommandLine"
	}
	return "cmd"
}

func envelopeAxis() []probeEnvelope {
	valueWith := func(command probeCommand, more map[string]any) map[string]any {
		return map[string]any{"value": probeInput(command, more)}
	}
	return []probeEnvelope{
		{"none", func(surface.ID, probeCommand) map[string]any { return nil }},
		{"value-agrees", func(_ surface.ID, command probeCommand) map[string]any {
			return valueWith(command, nil)
		}},
		{"value-competing-command", func(_ surface.ID, command probeCommand) map[string]any {
			return valueWith(probeCommand{text: ptr("echo competing")}, nil)
		}},
		{"value-alternate-key", func(id surface.ID, command probeCommand) map[string]any {
			text := "echo alternate"
			if command.text != nil {
				text = *command.text
			}
			return valueWith(command, map[string]any{alternateKey(id): text + " # alternate"})
		}},
		{"value-not-object", func(surface.ID, probeCommand) map[string]any {
			return map[string]any{"value": "not an object"}
		}},
		{"value-nests-value", func(_ surface.ID, command probeCommand) map[string]any {
			return valueWith(command, map[string]any{"value": map[string]any{}})
		}},
	}
}

func ptr(s string) *string { return &s }

// probeEnvironment is one value of the environment axis, as a root key.
type probeEnvironment struct {
	name   string
	extras func() map[string]any
}

func environmentAxis() []probeEnvironment {
	return []probeEnvironment{
		{"none", func() map[string]any { return nil }},
		{"env-object", func() map[string]any { return map[string]any{"env": map[string]any{"EXAMPLE": "value"}} }},
		{"Environment-object", func() map[string]any { return map[string]any{"Environment": map[string]any{"EXAMPLE": "value"}} }},
		{"env-null", func() map[string]any { return map[string]any{"env": nil} }},
		{"env-string", func() map[string]any { return map[string]any{"env": "EXAMPLE=value"} }},
	}
}

// probeCwd is one value of the working-directory axis: the event cwd and the
// tool input keys that disagree or agree with it.
type probeCwd struct {
	name string
	cwd  string
	// extras builds the tool input keys; a nested value layer needs the command.
	extras func(command probeCommand) map[string]any
}

func cwdAxis() []probeCwd {
	const workspace = "/ws"
	return []probeCwd{
		{"absolute", workspace, func(probeCommand) map[string]any { return nil }},
		{"tool-cwd-disagrees", workspace, func(probeCommand) map[string]any {
			return map[string]any{"cwd": "/elsewhere"}
		}},
		{"nested-workdir-disagrees", workspace, func(command probeCommand) map[string]any {
			return map[string]any{"value": probeInput(command, map[string]any{"workdir": "/elsewhere"})}
		}},
		{"relative", "ws", func(probeCommand) map[string]any { return nil }},
		{"none", "", func(probeCommand) map[string]any { return nil }},
	}
}

// InspectionProbes returns neutral-level tool calls that exercise the shell
// inspection's inputs. The first group is the full product of the tool, the
// surface, the argument envelope, and the command. Two more groups cross the
// environment axis and the working-directory axis with the command axis, on
// Bash and PowerShell for claude-code, so each of those axes is covered without
// multiplying the first group by them. Each call returns fresh maps.
func InspectionProbes() []Probe {
	tools := []string{"Bash", "PowerShell", "powershell", "mcp__shell__PowerShell", "exec_command"}
	var probes []Probe
	for _, tool := range tools {
		for _, id := range surface.All() {
			for _, envelope := range envelopeAxis() {
				for _, command := range commandAxis() {
					probes = append(probes, Probe{
						Name:    strings.Join([]string{"product", tool, string(id), envelope.name, command.name}, "/"),
						Surface: id,
						Tool:    tool,
						Input:   probeInput(command, envelope.extras(id, command)),
						Cwd:     "/ws",
					})
				}
			}
		}
	}
	for _, tool := range []string{"Bash", "PowerShell"} {
		for _, environment := range environmentAxis() {
			for _, command := range commandAxis() {
				probes = append(probes, Probe{
					Name:    strings.Join([]string{"environment", tool, environment.name, command.name}, "/"),
					Surface: surface.ClaudeCode,
					Tool:    tool,
					Input:   probeInput(command, environment.extras()),
					Cwd:     "/ws",
				})
			}
		}
		for _, dir := range cwdAxis() {
			for _, command := range commandAxis() {
				probes = append(probes, Probe{
					Name:    strings.Join([]string{"cwd", tool, dir.name, command.name}, "/"),
					Surface: surface.ClaudeCode,
					Tool:    tool,
					Input:   probeInput(command, dir.extras(command)),
					Cwd:     dir.cwd,
				})
			}
		}
	}
	return probes
}
