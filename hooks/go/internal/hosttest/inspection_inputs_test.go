package hosttest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestShellPayloadsCoverCorpusAndRegressions(t *testing.T) {
	payloads := ShellPayloads(t)

	names := map[string]bool{}
	perSurface := map[surface.ID]int{}
	corpus := 0
	for _, p := range payloads {
		if names[p.Name] {
			t.Errorf("duplicate payload name %q", p.Name)
		}
		names[p.Name] = true
		perSurface[p.Surface]++
		if p.Hook == "" || len(p.Stdin) == 0 {
			t.Errorf("%s: hook %q, %d stdin bytes", p.Name, p.Hook, len(p.Stdin))
		}
		var decoded map[string]any
		if err := json.Unmarshal(p.Stdin, &decoded); err != nil {
			t.Errorf("%s: stdin is not one JSON object: %v", p.Name, err)
		}
		if strings.Contains(string(p.Stdin), "{{") {
			t.Errorf("%s: stdin keeps an unexpanded token", p.Name)
		}
		if strings.HasPrefix(p.Name, "corpus/") {
			corpus++
		}
	}
	for _, id := range surface.All() {
		if perSurface[id] == 0 {
			t.Errorf("no payload on %s", id)
		}
	}
	if corpus == 0 {
		t.Error("no corpus payload")
	}

	raw, err := os.ReadFile(filepath.Join(Checkout(t), filepath.FromSlash(guardRegressionsFile)))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct {
			ID string `json:"id"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &file); err != nil || len(file.Cases) == 0 {
		t.Fatalf("regression file: %d cases, %v", len(file.Cases), err)
	}
	byName := map[string]ShellPayload{}
	for _, p := range payloads {
		byName[p.Name] = p
	}
	for _, tc := range file.Cases {
		for _, id := range surface.All() {
			p, ok := byName["regression/"+tc.ID+"/"+string(id)]
			if !ok {
				t.Errorf("no regression payload for %s on %s", tc.ID, id)
				continue
			}
			if p.Surface != id || p.Hook != regressionHook || len(p.Args) != 0 {
				t.Errorf("%s: surface %s hook %q args %v", p.Name, p.Surface, p.Hook, p.Args)
			}
			if p.Dir == "" {
				t.Errorf("%s: empty Dir", p.Name)
			} else if _, err := os.Stat(filepath.Join(p.Dir, "p.sh")); err != nil {
				t.Errorf("%s: Dir has no p.sh: %v", p.Name, err)
			}
			if !strings.Contains(string(p.Stdin), jsonEscaped(t, p.Dir)) {
				t.Errorf("%s: stdin does not carry its Dir as cwd", p.Name)
			}
		}
	}
}

func jsonEscaped(t *testing.T, s string) string {
	t.Helper()
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Trim(string(raw), `"`)
}

func TestShellPayloadsKeepCorpusHookAndArgv(t *testing.T) {
	var warn, benign bool
	for _, p := range ShellPayloads(t) {
		if !strings.HasPrefix(p.Name, "corpus/") {
			continue
		}
		if p.Hook != "command-guardian" && p.Hook != "secrets-guardian" && p.Hook != "release-guardian" {
			t.Errorf("%s: hook %q", p.Name, p.Hook)
		}
		if strings.Join(p.Args, " ") == "--mode warn" {
			warn = true
		}
		if strings.Join(p.Args, " ") == "--mode block" {
			benign = true
		}
	}
	if !warn || !benign {
		t.Errorf("argv kinds seen: warn=%v block=%v", warn, benign)
	}
}

// TestCorpusPayloadsBuildFixtureDirs covers the entries that name a fixture or
// carry the fixture token, on a corpus the checkout may not hold today.
func TestCorpusPayloadsBuildFixtureDirs(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".claude-plugin/plugin.json", "{}")
	write(corpusPayloadsDir+"/claude-code/payloads/shell.json",
		`{"tool_name":"Bash","cwd":"{{FIXTURE}}","tool_input":{"command":"echo {{AWS_KEY}}"}}`)
	write(corpusPayloadsDir+"/codex/payloads/read.json", `{"tool_name":"Read","cwd":"/tmp","tool_input":{}}`)
	write(corpusEntriesFile, `{
  "templates": {
    "shell": {
      "claude-code": {"tool_name": "mcp__host__PowerShell", "tool_input": {"command": "x"}, "cwd": "{{FIXTURE}}"},
      "copilot": {"toolName": "powershell", "toolArgs": {"cmd": "x"}, "cwd": "/tmp"},
      "antigravity": {"toolCall": {"name": "run_command", "args": {"CommandLine": "x"}}},
      "codex": {"tool_name": "exec_command", "tool_input": {"command": "x"}}
    }
  },
  "entries": [
    {"name": "file", "hook": "command-guardian", "argv": ["--mode", "block"], "stdin": "@shell.json", "only": ["claude-code"],
     "fixture": {"kind": "repo", "files": {"sub/f.txt": "key {{AWS_KEY}}"}}},
    {"name": "read-file", "hook": "secrets-guardian", "argv": [], "stdin": "@read.json", "only": ["codex"]},
    {"name": "template", "hook": "release-guardian", "argv": ["--mode", "block"], "stdin": "$shell"},
    {"name": "missing", "hook": "command-guardian", "argv": [], "stdin": "$shell", "only": ["claude-code"], "fixture": {"kind": "missing"}},
    {"name": "per-surface", "hook": "command-guardian", "argv": [], "only": ["codex", "copilot"],
     "stdin": {"codex": {"tool_name": "Bash", "tool_input": {"command": "x"}}}},
    {"name": "literal", "hook": "command-guardian", "argv": [], "stdin": "not json"},
    {"name": "object", "hook": "command-guardian", "argv": [], "only": ["claude-code"], "stdin": {"tool_name": "Edit", "tool_input": {}}}
  ]
}`)
	write(guardRegressionsFile, `{"files": {"p.sh": "x"}, "cases": []}`)
	t.Setenv("AGX_REPO_ROOT", root)

	got := map[string]ShellPayload{}
	for _, p := range ShellPayloads(t) {
		got[p.Name] = p
	}

	file := got["corpus/file/claude-code"]
	if file.Dir == "" || !strings.Contains(string(file.Stdin), jsonEscaped(t, file.Dir)) || !strings.Contains(string(file.Stdin), corpusAWSKey()) {
		t.Errorf("file payload: dir %q stdin %s", file.Dir, file.Stdin)
	}
	if data, err := os.ReadFile(filepath.Join(file.Dir, "sub", "f.txt")); err != nil || string(data) != "key "+corpusAWSKey() {
		t.Errorf("fixture file = %q, %v", data, err)
	}
	if len(file.Args) != 2 || file.Hook != "command-guardian" {
		t.Errorf("file payload hook/args = %q %v", file.Hook, file.Args)
	}

	for _, name := range []string{"corpus/template/claude-code", "corpus/template/copilot", "corpus/template/antigravity", "corpus/template/codex", "corpus/per-surface/codex"} {
		if _, ok := got[name]; !ok {
			t.Errorf("missing %s", name)
		}
	}
	if p := got["corpus/template/claude-code"]; p.Dir == "" {
		t.Error("a template that carries the fixture token needs a Dir")
	}
	if p := got["corpus/template/copilot"]; p.Dir != "" {
		t.Errorf("a payload with no fixture has Dir %q", p.Dir)
	}
	if p := got["corpus/missing/claude-code"]; p.Dir == "" {
		t.Error("missing fixture has no Dir")
	} else if _, err := os.Stat(p.Dir); err == nil {
		t.Errorf("missing fixture %s was created", p.Dir)
	}
	for _, name := range []string{"corpus/read-file/codex", "corpus/per-surface/copilot", "corpus/literal/claude-code", "corpus/object/claude-code"} {
		if _, ok := got[name]; ok {
			t.Errorf("non-shell or absent payload %s was returned", name)
		}
	}
}

func TestShellShaped(t *testing.T) {
	for _, tc := range []struct {
		name  string
		id    surface.ID
		stdin string
		want  bool
	}{
		{"claude Bash", surface.ClaudeCode, `{"tool_name":"Bash"}`, true},
		{"codex exec_command", surface.Codex, `{"tool_name":"exec_command"}`, true},
		{"copilot powershell", surface.Copilot, `{"toolName":"powershell"}`, true},
		{"namespaced", surface.ClaudeCode, `{"tool_name":"mcp__shell__PowerShell"}`, true},
		{"antigravity run_command", surface.Antigravity, `{"toolCall":{"name":"run_command"}}`, true},
		{"antigravity no call", surface.Antigravity, `{}`, false},
		{"Read", surface.ClaudeCode, `{"tool_name":"Read"}`, false},
		{"name not a string", surface.ClaudeCode, `{"tool_name":3}`, false},
		{"wrong envelope", surface.Copilot, `{"tool_name":"Bash"}`, false},
		{"not JSON", surface.ClaudeCode, `nope`, false},
		{"null", surface.ClaudeCode, `null`, false},
	} {
		if got := shellShaped(tc.id, []byte(tc.stdin)); got != tc.want {
			t.Errorf("%s: shellShaped = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRegressionToolEnv(t *testing.T) {
	entries := map[string]string{"RIPGREP_CONFIG_PATH": "config.txt"}
	for _, tc := range []struct {
		name      string
		field     string
		env       map[string]string
		raw       string
		wantField string
		wantJSON  string
		wantErr   bool
	}{
		{name: "none"},
		{name: "empty tool_env is ignored", env: map[string]string{}},
		{name: "default field", env: entries, wantField: "env", wantJSON: `{"RIPGREP_CONFIG_PATH":"config.txt"}`},
		{name: "kept spelling", field: "Environment", env: entries, wantField: "Environment", wantJSON: `{"RIPGREP_CONFIG_PATH":"config.txt"}`},
		{name: "other field", field: "envx", env: entries, wantErr: true},
		{name: "raw null", raw: `null`, wantField: "env", wantJSON: `null`},
		{name: "raw empty object", field: "ENV", raw: `{}`, wantField: "ENV", wantJSON: `{}`},
		{name: "raw not JSON", raw: `{`, wantErr: true},
		{name: "both forms", env: entries, raw: `null`, wantErr: true},
	} {
		var raw json.RawMessage
		if tc.raw != "" {
			raw = json.RawMessage(tc.raw)
		}
		field, value, err := regressionToolEnv(tc.field, tc.env, raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: accepted field=%q value=%v", tc.name, field, value)
			}
			continue
		}
		if err != nil || field != tc.wantField {
			t.Errorf("%s: field = %q, err = %v, want %q", tc.name, field, err, tc.wantField)
			continue
		}
		if field == "" {
			continue
		}
		if got, err := json.Marshal(value); err != nil || string(got) != tc.wantJSON {
			t.Errorf("%s: value = %s (%v), want %s", tc.name, got, err, tc.wantJSON)
		}
	}
}

func TestInspectionProbesCrossEveryAxis(t *testing.T) {
	probes := InspectionProbes()
	names := map[string]bool{}
	for _, p := range probes {
		if names[p.Name] {
			t.Errorf("duplicate probe name %q", p.Name)
		}
		names[p.Name] = true
		if p.Input == nil || p.Tool == "" {
			t.Errorf("%s: tool %q input %v", p.Name, p.Tool, p.Input)
		}
	}

	segment := func(kind string, index int) map[string]int {
		seen := map[string]int{}
		for _, p := range probes {
			parts := strings.Split(p.Name, "/")
			if parts[0] == kind {
				seen[parts[index]]++
			}
		}
		return seen
	}
	requireAll := func(axis string, seen map[string]int, want ...string) {
		t.Helper()
		for _, value := range want {
			if seen[value] == 0 {
				t.Errorf("axis %s never takes %q: %v", axis, value, seen)
			}
		}
	}
	commands := []string{"posix", "bash-n", "bash-script", "here-set-content", "here-out-file", "empty", "no-command"}
	envelopes := []string{"none", "value-agrees", "value-competing-command", "value-alternate-key", "value-not-object", "value-nests-value"}
	tools := []string{"Bash", "PowerShell", "powershell", "mcp__shell__PowerShell", "exec_command"}
	surfaces := []string{"claude-code", "copilot", "codex", "antigravity"}
	requireAll("tool", segment("product", 1), tools...)
	requireAll("surface", segment("product", 2), surfaces...)
	requireAll("envelope", segment("product", 3), envelopes...)
	requireAll("command", segment("product", 4), commands...)
	requireAll("environment", segment("environment", 2), "none", "env-object", "Environment-object", "env-null", "env-string")
	requireAll("environment command", segment("environment", 3), commands...)
	requireAll("environment tool", segment("environment", 1), "Bash", "PowerShell")
	requireAll("cwd", segment("cwd", 2), "absolute", "tool-cwd-disagrees", "nested-workdir-disagrees", "relative", "none")
	requireAll("cwd command", segment("cwd", 3), commands...)
	requireAll("cwd tool", segment("cwd", 1), "Bash", "PowerShell")

	// The product is complete: every combination of the four axes is present.
	want := len(tools) * len(surfaces) * len(envelopes) * len(commands)
	got := 0
	for _, p := range probes {
		if strings.HasPrefix(p.Name, "product/") {
			got++
		}
	}
	if got != want {
		t.Errorf("product rows = %d, want %d", got, want)
	}

	// The axes show in the data, not only in the names.
	var sawNoCommand, sawEmpty, sawNull, sawString, sawEnvironment, sawAlternate, sawNested, sawNotObject bool
	cwds := map[string]bool{}
	for _, p := range probes {
		command, hasCommand := p.Input["command"]
		sawNoCommand = sawNoCommand || !hasCommand
		sawEmpty = sawEmpty || command == ""
		if env, present := p.Input["env"]; present {
			sawNull = sawNull || env == nil
			_, isString := env.(string)
			sawString = sawString || isString
		}
		_, environment := p.Input["Environment"].(map[string]any)
		sawEnvironment = sawEnvironment || environment
		if value, present := p.Input["value"]; present {
			object, isObject := value.(map[string]any)
			sawNotObject = sawNotObject || !isObject
			if isObject {
				_, alt := object["cmd"]
				_, alt2 := object["CommandLine"]
				sawAlternate = sawAlternate || alt || alt2
				_, deeper := object["value"]
				sawNested = sawNested || deeper
			}
		}
		cwds[p.Cwd] = true
	}
	for name, seen := range map[string]bool{
		"no command key": sawNoCommand, "empty command": sawEmpty, "null env": sawNull, "string env": sawString,
		"Environment object": sawEnvironment, "alternate key": sawAlternate, "value in value": sawNested, "value not an object": sawNotObject,
		"empty event cwd": cwds[""], "relative event cwd": cwds["ws"], "absolute event cwd": cwds["/ws"],
	} {
		if !seen {
			t.Errorf("no probe carries %s", name)
		}
	}
}

func TestInspectionProbesAreFresh(t *testing.T) {
	first, second := InspectionProbes(), InspectionProbes()
	if len(first) != len(second) {
		t.Fatalf("lengths differ: %d, %d", len(first), len(second))
	}
	for i := range first {
		first[i].Input["mutated"] = true
		if _, shared := second[i].Input["mutated"]; shared {
			t.Fatalf("%s: input shared between calls", first[i].Name)
		}
	}
}

func TestCorpusPayloadPathStaysInsidePayloadsDir(t *testing.T) {
	root := t.TempDir()
	payloads := filepath.Join(root, "hooks", "go", "surfaces", string(surface.ClaudeCode), "payloads")

	got, err := corpusPayloadPath(root, surface.ClaudeCode, "pre-tool-use.json")
	if err != nil {
		t.Fatalf("a plain payload name is refused: %v", err)
	}
	if want := filepath.Join(payloads, "pre-tool-use.json"); got != want {
		t.Errorf("path = %q, want %q", got, want)
	}

	absolute := filepath.Join(root, "outside.json")
	for _, name := range []string{"../x", "../../x.json", "a/../../x", absolute, ""} {
		if got, err := corpusPayloadPath(root, surface.ClaudeCode, name); err == nil {
			t.Errorf("name %q was accepted as %q", name, got)
		}
	}
}
