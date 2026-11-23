package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

type guardRegressionExpectation struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

type guardRegressionCase struct {
	ID           string            `json:"id"`
	Provenance   string            `json:"provenance"`
	Origins      []string          `json:"origins"`
	Tool         string            `json:"tool"`
	Dialect      string            `json:"dialect"`
	Command      string            `json:"command"`
	Shell        string            `json:"shell,omitempty"`
	Capabilities []string          `json:"capabilities"`
	Env          map[string]string `json:"env,omitempty"`
	ToolEnvField string            `json:"tool_env_field,omitempty"`
	Files        map[string]string `json:"files,omitempty"`
	ToolEnv      map[string]string `json:"tool_env,omitempty"`
	// ToolEnvRaw is a JSON value used in place of ToolEnv, so a case can carry
	// a null container or a null entry. It is not a pointer: a literal null
	// must stay distinguishable from an absent field.
	ToolEnvRaw json.RawMessage                       `json:"tool_env_raw,omitempty"`
	Pending    string                                `json:"pending,omitempty"`
	Setup      string                                `json:"setup,omitempty"`
	Expected   map[string]guardRegressionExpectation `json:"expected"`
}

var requiredGuardPrecisionIDs = []string{
	"precision-git-message", "precision-git-abbrev", "precision-git-real-bypass",
	"precision-config-core", "precision-config-other", "precision-ps-env", "precision-alt-config",
	"precision-delete-name", "precision-delete-later", "precision-delete-env-text", "precision-docker-rm",
	"precision-delete-protected", "precision-ps-delete", "precision-ps-delete-protected",
	"precision-ps-pattern", "precision-ps-filter", "precision-ps-file",
	"precision-ps-here-data", "precision-ps-here-exec", "precision-ps-here-followed",
	"precision-bash-version", "precision-bash-syntax", "precision-bash-credential", "precision-bash-exec", "precision-bash-exec-option",
}

func supportedGuardRegressionDialect(tool, dialect string) bool {
	return tool == "Bash" && dialect == "posix" || tool == "PowerShell" && dialect == "powershell"
}

func validateGuardPrecisionInventory(cases []guardRegressionCase) error {
	remaining := make(map[string]bool, len(requiredGuardPrecisionIDs))
	for _, id := range requiredGuardPrecisionIDs {
		remaining[id] = true
	}
	seen := map[string]bool{}
	for _, tc := range cases {
		if !strings.HasPrefix(tc.ID, "precision-") {
			continue
		}
		if seen[tc.ID] {
			return fmt.Errorf("duplicate precision case %s", tc.ID)
		}
		seen[tc.ID] = true
		if tc.Pending != "" {
			return fmt.Errorf("precision case %s must be enabled", tc.ID)
		}
		delete(remaining, tc.ID)
	}
	for _, id := range requiredGuardPrecisionIDs {
		if remaining[id] {
			return fmt.Errorf("missing required precision case %s", id)
		}
	}
	return nil
}

func TestGuardPrecisionInventory(t *testing.T) {
	var fixture struct {
		Cases []guardRegressionCase `json:"cases"`
	}
	data, err := os.ReadFile("testdata/guard-regressions.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if err := validateGuardPrecisionInventory(fixture.Cases); err != nil {
		t.Fatal(err)
	}
	for _, id := range requiredGuardPrecisionIDs {
		t.Run("remove/"+id, func(t *testing.T) {
			var reduced []guardRegressionCase
			for _, tc := range fixture.Cases {
				if tc.ID != id {
					reduced = append(reduced, tc)
				}
			}
			if err := validateGuardPrecisionInventory(reduced); err == nil || err.Error() != "missing required precision case "+id {
				t.Fatalf("inventory removal error = %v, want missing case %s", err, id)
			}
		})
	}
	t.Run("duplicate", func(t *testing.T) {
		var duplicated []guardRegressionCase
		duplicated = append(duplicated, fixture.Cases...)
		for _, tc := range fixture.Cases {
			if tc.ID == requiredGuardPrecisionIDs[0] {
				duplicated = append(duplicated, tc)
				break
			}
		}
		if err := validateGuardPrecisionInventory(duplicated); err == nil || err.Error() != "duplicate precision case "+requiredGuardPrecisionIDs[0] {
			t.Fatalf("duplicate inventory error = %v", err)
		}
	})
	t.Run("pending", func(t *testing.T) {
		copied := append([]guardRegressionCase(nil), fixture.Cases...)
		for i := range copied {
			if copied[i].ID == requiredGuardPrecisionIDs[0] {
				copied[i].Pending = "must not hide a required behavior"
			}
		}
		if err := validateGuardPrecisionInventory(copied); err == nil || err.Error() != "precision case "+requiredGuardPrecisionIDs[0]+" must be enabled" {
			t.Fatalf("pending inventory error = %v", err)
		}
	})
}

func TestGuardRegressionDialectPairs(t *testing.T) {
	for _, tool := range []string{"Bash", "PowerShell", "bash", ""} {
		for _, dialect := range []string{"posix", "powershell", "", "PowerShell"} {
			t.Run(tool+"/"+dialect, func(t *testing.T) {
				want := tool == "Bash" && dialect == "posix" || tool == "PowerShell" && dialect == "powershell"
				if got := supportedGuardRegressionDialect(tool, dialect); got != want {
					t.Fatalf("supported(%q, %q) = %v, want %v", tool, dialect, got, want)
				}
			})
		}
	}
}

// guardPrecisionCall is the shell call a precision case sends: Input carries
// the command and any extra tool-input fields, such as env or shell. Codex's
// host sends every shell call as exec_command, so its PowerShell case keeps the
// neutral tool name through Native to exercise adapter passthrough, not native
// Codex tool support.
func guardPrecisionCall(surface surface.ID, root, tool, command string, extra map[string]any) hosttest.Call {
	input := map[string]any{"command": command}
	for key, value := range extra {
		input[key] = value
	}
	return hosttest.Call{Tool: tool, Input: input, Cwd: root, Native: (surface == "codex" || surface == "antigravity") && tool == "PowerShell"}
}

func TestGuardPrecisionPayloadDialect(t *testing.T) {
	for _, surface := range surface.All() {
		for _, tc := range []guardRegressionCase{
			{ID: "posix", Tool: "Bash", Dialect: "posix", Command: "echo public"},
			{ID: "powershell", Tool: "PowerShell", Dialect: "powershell", Command: "Write-Output public"},
		} {
			t.Run(string(surface)+"/"+tc.ID, func(t *testing.T) {
				payload := string(hosttest.Stdin(surface, guardPrecisionCall(surface, t.TempDir(), tc.Tool, tc.Command, nil)))
				assertGuardRegressionDialect(t, surface, payload, tc)
			})
		}
	}
}

func assertGuardRegressionDialect(t *testing.T, surface surface.ID, payload string, tc guardRegressionCase) {
	t.Helper()
	eventName := "PreToolUse"
	if surface == "copilot" {
		eventName = "preToolUse"
	}
	event, rejected := productionHookDependencies().normalize(string(surface), eventName, []byte(payload))
	if rejected != nil {
		t.Fatalf("%s fixture normalization rejected: %+v", tc.ID, rejected)
	}
	fields, err := event.Read()
	if err != nil {
		t.Fatal(err)
	}
	if fields.Tool.Name == nil || *fields.Tool.Name != tc.Tool {
		t.Fatalf("%s normalized tool = %v, want %s", tc.ID, fields.Tool.Name, tc.Tool)
	}
	// The neutral tool name selects the initial dialect; wrappers/selectors remain guard input.
	dialect := "posix"
	if *fields.Tool.Name == "PowerShell" {
		dialect = "powershell"
	}
	if dialect != tc.Dialect {
		t.Fatalf("%s normalized dialect = %s, want %s", tc.ID, dialect, tc.Dialect)
	}
}

// guardRegressionFixture is the regression contract: the fixture files every
// case gets unless it names its own, and the cases.
type guardRegressionFixture struct {
	Files map[string]string     `json:"files"`
	Cases []guardRegressionCase `json:"cases"`
}

// loadGuardRegressions reads the regression contract and requires a
// non-empty case list with the complete precision inventory.
func loadGuardRegressions(t *testing.T) guardRegressionFixture {
	t.Helper()
	var fixture guardRegressionFixture
	data, err := os.ReadFile("testdata/guard-regressions.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("empty guard regression contract")
	}
	if err := validateGuardPrecisionInventory(fixture.Cases); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// prepareGuardRegressionCase sets the case's environment on t, writes its
// fixture files into a fresh directory, and returns that directory.
func prepareGuardRegressionCase(t *testing.T, fixture guardRegressionFixture, tc guardRegressionCase) string {
	t.Helper()
	hosttest.ScrubGuardEnv(t)
	clearHardeningCredentialOverrides(t)
	t.Setenv("POSIXLY_CORRECT", "")
	t.Setenv("CDPATH", "")
	t.Setenv("BASHOPTS", "")
	t.Setenv("GLOBIGNORE", "")
	for key, value := range tc.Env {
		t.Setenv(key, value)
	}
	root := t.TempDir()
	files := fixture.Files
	if tc.Files != nil {
		files = tc.Files
	}
	for name, content := range files {
		if !filepath.IsLocal(name) {
			t.Fatalf("non-local fixture path %q", name)
		}
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		writeHardeningFile(t, path, content)
	}
	return root
}

// guardRegressionPayload is what surface's host sends for the case's shell
// call run in root.
func guardRegressionPayload(t *testing.T, surface surface.ID, root string, tc guardRegressionCase) string {
	t.Helper()
	extra := map[string]any{}
	field, value, err := guardRegressionToolEnv(tc)
	if err != nil {
		t.Fatalf("%s: %v", tc.ID, err)
	}
	if field != "" {
		extra[field] = value
	}
	if tc.Shell != "" {
		extra["shell"] = tc.Shell
	}
	return string(hosttest.Stdin(surface, guardPrecisionCall(surface, root, tc.Tool, tc.Command, extra)))
}

// guardRegressionToolEnv returns the tool input key and value that carry the
// case's tool environment, or an empty field when the case has none. The field
// is env or environment in any letter case and keeps its spelling. A raw value
// is used whenever it is present, an empty object included, while an empty
// tool_env is ignored. A case that gives both forms is an error, so one cannot
// silently hide the other.
func guardRegressionToolEnv(tc guardRegressionCase) (field string, value any, err error) {
	if tc.ToolEnv != nil && tc.ToolEnvRaw != nil {
		return "", nil, fmt.Errorf("tool_env and tool_env_raw are both set")
	}
	if tc.ToolEnvRaw == nil && len(tc.ToolEnv) == 0 {
		return "", nil, nil
	}
	field = tc.ToolEnvField
	if field == "" {
		field = "env"
	}
	if !strings.EqualFold(field, "env") && !strings.EqualFold(field, "environment") {
		return "", nil, fmt.Errorf("unsupported tool environment field %q", field)
	}
	if tc.ToolEnvRaw == nil {
		return field, tc.ToolEnv, nil
	}
	if err := json.Unmarshal(tc.ToolEnvRaw, &value); err != nil {
		return "", nil, fmt.Errorf("tool_env_raw is not JSON: %w", err)
	}
	return field, value, nil
}

// TestGuardRegressionToolEnv pins which tool environment each case form sends
// and which forms the harness refuses.
func TestGuardRegressionToolEnv(t *testing.T) {
	entries := map[string]string{"RIPGREP_CONFIG_PATH": "config.txt"}
	for _, tc := range []struct {
		name      string
		in        guardRegressionCase
		wantField string
		wantJSON  string
		wantErr   bool
	}{
		{name: "no environment", in: guardRegressionCase{}},
		{name: "empty tool_env is ignored", in: guardRegressionCase{ToolEnv: map[string]string{}}},
		{name: "default field", in: guardRegressionCase{ToolEnv: entries}, wantField: "env", wantJSON: `{"RIPGREP_CONFIG_PATH":"config.txt"}`},
		{name: "upper case field", in: guardRegressionCase{ToolEnvField: "ENV", ToolEnv: entries}, wantField: "ENV", wantJSON: `{"RIPGREP_CONFIG_PATH":"config.txt"}`},
		{name: "mixed case field", in: guardRegressionCase{ToolEnvField: "Environment", ToolEnv: entries}, wantField: "Environment", wantJSON: `{"RIPGREP_CONFIG_PATH":"config.txt"}`},
		{name: "other field", in: guardRegressionCase{ToolEnvField: "envx", ToolEnv: entries}, wantErr: true},
		{name: "raw null", in: guardRegressionCase{ToolEnvRaw: json.RawMessage(`null`)}, wantField: "env", wantJSON: `null`},
		{name: "raw empty object", in: guardRegressionCase{ToolEnvField: "ENV", ToolEnvRaw: json.RawMessage(`{}`)}, wantField: "ENV", wantJSON: `{}`},
		{name: "raw null entry", in: guardRegressionCase{ToolEnvRaw: json.RawMessage(`{"RIPGREP_CONFIG_PATH":null}`)}, wantField: "env", wantJSON: `{"RIPGREP_CONFIG_PATH":null}`},
		{name: "raw other field", in: guardRegressionCase{ToolEnvField: "envx", ToolEnvRaw: json.RawMessage(`null`)}, wantErr: true},
		{name: "raw not JSON", in: guardRegressionCase{ToolEnvRaw: json.RawMessage(`{`)}, wantErr: true},
		{name: "both forms", in: guardRegressionCase{ToolEnv: entries, ToolEnvRaw: json.RawMessage(`null`)}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			field, value, err := guardRegressionToolEnv(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("accepted field=%q value=%v", field, value)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if field != tc.wantField {
				t.Fatalf("field = %q, want %q", field, tc.wantField)
			}
			if field == "" {
				return
			}
			got, err := json.Marshal(value)
			if err != nil || string(got) != tc.wantJSON {
				t.Fatalf("value = %s (%v), want %s", got, err, tc.wantJSON)
			}
		})
	}

	var parsed guardRegressionCase
	if err := json.Unmarshal([]byte(`{"tool_env_raw": null}`), &parsed); err != nil || string(parsed.ToolEnvRaw) != "null" {
		t.Fatalf("a literal null must reach the harness: %v %q", err, parsed.ToolEnvRaw)
	}
}

func TestGuardRegression(t *testing.T) {
	fixture := loadGuardRegressions(t)
	guards := []string{"command-guardian", "secrets-guardian", "release-guardian"}
	seen := map[string]bool{}
	enabled, pending, executed, grouped := 0, 0, 0, 0
	for _, tc := range fixture.Cases {
		if tc.ID == "" || seen[tc.ID] || len(tc.Origins) == 0 || tc.Provenance == "" || tc.Command == "" || len(tc.Capabilities) == 0 {
			t.Fatalf("missing or duplicate case contract: %+v", tc)
		}
		seen[tc.ID] = true
		if !supportedGuardRegressionDialect(tc.Tool, tc.Dialect) {
			t.Fatalf("unsupported tool/dialect for %s", tc.ID)
		}
		if tc.Pending != "" {
			pending++
			t.Run("capability-pending/"+tc.ID, func(t *testing.T) {
				want, ok := tc.Expected["commit-reminder"]
				if !ok || want.Decision != "pass" || tc.Setup == "" || len(tc.Capabilities) != 1 || tc.Capabilities[0] != "commit-reminder-deny-mode" {
					t.Fatal("pending reminder must specify its setup and safe decision")
				}
				t.Skip("capability pending, not executed: " + tc.Pending)
			})
			continue
		}
		enabled++
		if len(tc.Capabilities) != 1 || tc.Capabilities[0] != "current-pretool-guards" {
			t.Fatalf("%s requires an unsupported capability", tc.ID)
		}
		if len(tc.Expected) != len(guards) {
			t.Fatalf("%s must specify all current guard decisions", tc.ID)
		}
		t.Run("enabled/"+tc.ID, func(t *testing.T) {
			root := prepareGuardRegressionCase(t, fixture, tc)
			for _, surface := range surface.All() {
				payload := guardRegressionPayload(t, surface, root, tc)
				assertGuardRegressionDialect(t, surface, payload, tc)
				for _, guard := range guards {
					t.Run(string(surface)+"/"+guard, func(t *testing.T) {
						want, ok := tc.Expected[guard]
						if !ok || (want.Decision != "pass" && want.Decision != "deny") || (want.Decision == "deny" && want.Reason == "") {
							t.Fatalf("invalid expected decision for %s", guard)
						}
						if guard != "release-guardian" && !strings.Contains(strings.Join(legalArgs(surface, guard), " "), "--mode block") {
							t.Fatal("regression contract requires block mode")
						}
						// Command lines are handler input only; none are launched by this test.
						executed++
						code, out, diagnostic := invokeWithLegalArgs(t, payload, surface, guard)
						requireOutcome(t, surface, want.Decision, want.Reason, code, out, diagnostic)
					})
				}
				t.Run(string(surface)+"/pre-tool", func(t *testing.T) {
					grouped++
					guardRegressionEvent(t, payload, surface, guards, tc.Expected)
				})
			}
		})
	}
	t.Logf("guard regression contract: enabled=%d provider_guard_executions=%d grouped_events=%d capability_pending=%d", enabled, executed, grouped, pending)
}

func guardRegressionEvent(t *testing.T, payload string, surface surface.ID, guards []string, expected map[string]guardRegressionExpectation) {
	t.Helper()
	deps := productionHookDependencies()
	lookup := deps.lookup
	var mu sync.Mutex
	calls := map[string]int{}
	// The grouped event runs the command rules and the release rules as steps
	// of its commands check, each resolved by its own name, so each step
	// answers for its own expectation.
	members := map[string][]string{
		"command-guardian": {"command-guardian"},
		"release-guardian": {"release-guardian"},
		"secrets-guardian": {"secrets-guardian"},
	}
	deps.lookup = func(name string) (hookspec.Handler, bool) {
		handler, ok := lookup(name)
		if !ok {
			return nil, false
		}
		return func(req hookspec.Request) hookspec.Outcome {
			mu.Lock()
			calls[name]++
			mu.Unlock()
			if req.Context == nil {
				t.Errorf("grouped %s lost its cancellation context", name)
			}
			outcome := handler(req)
			denies := false
			for _, member := range members[name] {
				want := expected[member]
				if want.Decision == "deny" {
					denies = true
					if outcome.Kind() != contract.KindDeny || !strings.Contains(outcome.Payload(), want.Reason) {
						t.Errorf("grouped %s lost its expected denial %q: %s %s", name, want.Reason, outcome.Kind(), outcome.Payload())
					}
				} else if want.Decision != "pass" {
					t.Errorf("grouped %s has no expected decision for %s", name, member)
				}
			}
			if !denies && outcome.Kind() != contract.KindPass {
				t.Errorf("grouped %s returned %s, want pass: %s", name, outcome.Kind(), outcome.Payload())
			}
			return outcome
		}, true
	}
	code, out, diagnostic := invokeWithLegalArgsAndDependencies(t, deps, payload, surface, "pre-tool")
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != len(members) {
		t.Errorf("grouped event selected unexpected guards: %v", calls)
	}
	for name := range members {
		if calls[name] != 1 {
			t.Errorf("grouped event ran %s %d times, want once", name, calls[name])
		}
	}
	denied := false
	for _, guard := range guards {
		want := expected[guard]
		if want.Decision == "deny" {
			denied = true
			requireOutcome(t, surface, "deny", want.Reason, code, out, diagnostic)
		}
	}
	if !denied {
		requireOutcome(t, surface, "pass", "", code, out, diagnostic)
	}
}
