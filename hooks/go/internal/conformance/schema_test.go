package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// configSpec maps each provider to its fixture basename and repo-relative path.
type configSpec struct {
	fixture string
	relPath string
}

var configSpecs = map[string]configSpec{
	"claude-code": {fixture: "hooks.claude.json", relPath: "hooks/hooks.claude.json"},
	"copilot":     {fixture: "hooks.copilot.json", relPath: "hooks/hooks.copilot.json"},
	"codex":       {fixture: "hooks.codex.json", relPath: "hooks/hooks.codex.json"},
	"antigravity": {fixture: "hooks.json", relPath: "hooks.json"},
}

// writeConfigRepo lays out a minimal repo with the four canonical hook
// configs from testdata, applying an optional per-provider mutation.
func writeConfigRepo(t *testing.T, mutate func(provider, content string) string) string {
	t.Helper()
	root := t.TempDir()
	for provider, spec := range configSpecs {
		data, err := os.ReadFile(filepath.Join("testdata", "hooks-config", spec.fixture))
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		if mutate != nil {
			content = mutate(provider, content)
		}
		dest := filepath.Join(root, filepath.FromSlash(spec.relPath))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func mustReplace(t *testing.T, content, old, new string) string {
	t.Helper()
	if !strings.Contains(content, old) {
		t.Fatalf("mutation target %q not found — the canonical testdata configs moved", old)
	}
	changed := strings.ReplaceAll(content, old, new)
	if changed == content {
		t.Fatal("mutation did not change config bytes")
	}
	return changed
}

func assertRuleFired(t *testing.T, failures []Failure, rule string) {
	t.Helper()
	for _, f := range failures {
		if f.Rule == rule {
			return
		}
	}
	t.Fatalf("expected rule %q to fire, got failures: %v", rule, failures)
}

func assertGreen(t *testing.T, failures []Failure, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected no failures, got: %v", failures)
	}
}

func TestSchemaPassesOnCanonicalConfigs(t *testing.T) {
	root := writeConfigRepo(t, nil)
	failures, err := RunSchema(root)
	assertGreen(t, failures, err)
}

// TestSchemaBites mutates one config per rule and asserts the exact rule
// fires — the parity-gate discipline: every ported assertion must be shown
// to bite.
func TestSchemaBites(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		old, new string
		rule     string
	}{
		{"claude pre flags", "claude-code", " pre-tool", " pre-tool --mode baseline", "config-schema-claude-pre"},
		{"claude startup filter", "claude-code", `"matcher": "startup"`, `"matcher": "resume"`, "config-schema-claude-startup"},
		{"codex startup Windows command", "codex", `%PLUGIN_ROOT%\\hooks\\run-hook.cmd\" codex startup`, `%PLUGIN_ROOT%\\hooks\\run-hook.cmd\" codex pre-tool`, "config-schema-codex-startup"},
		{"copilot startup PowerShell command", "copilot", `$env:COPILOT_PLUGIN_ROOT\\hooks\\run-hook.cmd\" copilot startup`, `$env:COPILOT_PLUGIN_ROOT\\hooks\\run-hook.cmd\" copilot pre-tool`, "config-schema-copilot-startup"},
		{"antigravity direct startup handler", "antigravity", `"SessionStart": [`, `"SessionStart": [{"hooks": []},`, "config-schema-antigravity-startup"},
		{"copilot pre flags", "copilot", " pre-tool", " pre-tool --mode baseline", "config-schema-copilot-pre"},
		{"claude events", "claude-code", `"PreToolUse"`, `"Stop"`, "config-schema-claude-events"},
		{"claude pre timeout", "claude-code", `"timeout": 25`, `"timeout": 26`, "config-schema-claude-pre"},
		{"claude forbidden field", "claude-code", `"timeout": 25`, `"timeout": 25, "env": {}`, "config-schema-claude-fields"},
		{"claude misplaced matcher", "claude-code", `"type": "command"`, `"matcher": "Bash", "type": "command"`, "config-schema-claude-matcher"},
		{"claude missing matcher", "claude-code", `"matcher": "^(?:Read|Grep|Glob|NotebookRead|Bash|PowerShell|Write|Edit|MultiEdit|NotebookEdit|mcp__.*)$",`, ``, "config-schema-claude-matcher"},
		{"claude altered matcher", "claude-code", `"matcher": "^(?:Read|Grep|Glob|NotebookRead|Bash|PowerShell|Write|Edit|MultiEdit|NotebookEdit|mcp__.*)$"`, `"matcher": "^(?:Bash|PowerShell)$"`, "config-schema-claude-matcher"},
		{"claude group extra field", "claude-code", `"matcher": "^(?:Read|Grep|Glob|NotebookRead|Bash|PowerShell|Write|Edit|MultiEdit|NotebookEdit|mcp__.*)$",`, `"matcher": "^(?:Read|Grep|Glob|NotebookRead|Bash|PowerShell|Write|Edit|MultiEdit|NotebookEdit|mcp__.*)$", "ignored": true,`, "config-schema-claude-group-fields"},
		{"copilot version", "copilot", `"version": 1`, `"version": 2`, "config-schema-copilot-version"},
		{"copilot events", "copilot", `"preToolUse"`, `"PreToolUse"`, "config-schema-copilot-events"},
		{"copilot pre timeout", "copilot", `"timeoutSec": 25`, `"timeoutSec": 26`, "config-schema-copilot-pre"},
		{"copilot forbidden field", "copilot", `"timeoutSec": 25`, `"timeoutSec": 25, "timeout": 25`, "config-schema-copilot-fields"},
		{"codex events", "codex", `"PreToolUse"`, `"preToolUse"`, "config-schema-codex-events"},
		{"codex pre mode", "codex", "codex pre-tool", "codex pre-tool --mode warn", "config-schema-codex-pre"},
		{"codex forbidden field", "codex", `"timeout": 25`, `"timeout": 25, "timeoutSec": 25`, "config-schema-codex-fields"},
		{"codex group extra field", "codex", `"hooks": [`, `"ignored": true, "hooks": [`, "config-schema-codex-group-fields"},
		{"codex misplaced matcher", "codex", `"type": "command"`, `"matcher": "Bash", "type": "command"`, "config-schema-codex-matcher"},
		{"codex missing matcher", "codex", `"matcher": "*",`, ``, "config-schema-codex-matcher"},
		{"codex altered matcher", "codex", `"matcher": "*"`, `"matcher": "Bash"`, "config-schema-codex-matcher"},
		{"copilot missing matcher", "copilot", `"matcher": ".*",`, ``, "config-schema-copilot-pre"},
		{"copilot altered matcher", "copilot", `"matcher": ".*"`, `"matcher": "Bash"`, "config-schema-copilot-pre"},
		{"copilot misplaced matcher", "copilot", `"matcher": ".*"`, `"hooks": [{"matcher": ".*"}]`, "config-schema-copilot-fields"},
		{"codex windows provider", "codex", `cmd /c \"%PLUGIN_ROOT%\\hooks\\run-hook.cmd\" codex`, `cmd /c \"%PLUGIN_ROOT%\\hooks\\run-hook.cmd\" copilot`, "config-schema-codex-pre"},
		{"codex windows root", "codex", `%PLUGIN_ROOT%`, `%CLAUDE_PLUGIN_ROOT%`, "config-schema-codex-pre"},
		{"copilot powershell root", "copilot", `$env:COPILOT_PLUGIN_ROOT`, `$env:PLUGIN_ROOT`, "config-schema-copilot-pre"},
		{"routing wrong provider", "claude-code", "run-hook.cmd\\\" claude-code pre-tool", "run-hook.cmd\\\" copilot pre-tool", "config-schema-dispatch-routing"},
		{"routing undocumented hook", "codex", "run-hook.cmd\\\" codex pre-tool", "run-hook.cmd\\\" codex beautifier", "config-schema-dispatch-routing"},
		{"routing unquoted", "copilot", "run-hook.cmd\\\" copilot pre-tool", "run-hook.cmd copilot pre-tool", "config-schema-dispatch-routing"},
		{"antigravity missing agentics root", "antigravity", `"agentics"`, `"hooks"`, "config-schema-antigravity-root"},
		{"antigravity extra root key", "antigravity", `"agentics": {`, `"version": 1, "agentics": {`, "config-schema-antigravity-root"},
		{"antigravity events", "antigravity", `"PreToolUse"`, `"preToolUse"`, "config-schema-antigravity-events"},
		{"antigravity pre flags", "antigravity", "antigravity pre-tool", "antigravity pre-tool --mode warn", "config-schema-antigravity-pre"},
		{"antigravity pre timeout", "antigravity", `"timeout": 25`, `"timeout": 26`, "config-schema-antigravity-pre"},
		{"antigravity forbidden commandWindows", "antigravity", `"timeout": 25`, `"timeout": 25, "commandWindows": "\"./hooks/run-hook.cmd\" antigravity pre-tool"`, "config-schema-antigravity-fields"},
		{"antigravity forbidden timeoutSec", "antigravity", `"timeout": 25`, `"timeout": 25, "timeoutSec": 25`, "config-schema-antigravity-fields"},
		{"antigravity forbidden bash", "antigravity", `"timeout": 25`, `"timeout": 25, "bash": "\"./hooks/run-hook.cmd\" antigravity pre-tool"`, "config-schema-antigravity-fields"},
		{"antigravity forbidden powershell", "antigravity", `"timeout": 25`, `"timeout": 25, "powershell": "\"./hooks/run-hook.cmd\" antigravity pre-tool"`, "config-schema-antigravity-fields"},
		{"antigravity forbidden env", "antigravity", `"timeout": 25`, `"timeout": 25, "env": {}`, "config-schema-antigravity-fields"},
		{"antigravity forbidden windows", "antigravity", `"timeout": 25`, `"timeout": 25, "windows": {}`, "config-schema-antigravity-fields"},
		{"antigravity group extra field", "antigravity", `"hooks": [`, `"ignored": true, "hooks": [`, "config-schema-antigravity-group-fields"},
		{"antigravity misplaced matcher", "antigravity", `"type": "command"`, `"matcher": "Bash", "type": "command"`, "config-schema-antigravity-matcher"},
		{"antigravity missing matcher", "antigravity", `"matcher": "*",`, ``, "config-schema-antigravity-matcher"},
		{"antigravity altered matcher", "antigravity", `"matcher": "*"`, `"matcher": "Bash"`, "config-schema-antigravity-matcher"},
		{"antigravity routing wrong provider", "antigravity", "run-hook.cmd\\\" antigravity pre-tool", "run-hook.cmd\\\" codex pre-tool", "config-schema-dispatch-routing"},
		{"antigravity routing undocumented hook", "antigravity", "run-hook.cmd\\\" antigravity pre-tool", "run-hook.cmd\\\" antigravity beautifier", "config-schema-dispatch-routing"},
		{"antigravity routing unquoted", "antigravity", "run-hook.cmd\\\" antigravity pre-tool", "run-hook.cmd antigravity pre-tool", "config-schema-dispatch-routing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeConfigRepo(t, func(provider, content string) string {
				if provider != tt.provider {
					return content
				}
				return mustReplace(t, content, tt.old, tt.new)
			})
			failures, err := RunSchema(root)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertRuleFired(t, failures, tt.rule)
		})
	}
}

func TestSchemaBitesMissingFile(t *testing.T) {
	for _, relPath := range []string{"hooks/hooks.codex.json", "hooks.json"} {
		t.Run(relPath, func(t *testing.T) {
			root := writeConfigRepo(t, nil)
			if err := os.Remove(filepath.Join(root, filepath.FromSlash(relPath))); err != nil {
				t.Fatal(err)
			}
			failures, err := RunSchema(root)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertRuleFired(t, failures, "config-schema-file")
		})
	}
}

func TestSchemaBitesAntigravityNonObjectAgentics(t *testing.T) {
	root := writeConfigRepo(t, func(provider, content string) string {
		if provider != "antigravity" {
			return content
		}
		return `{"agentics": []}`
	})
	failures, err := RunSchema(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "config-schema-antigravity-root")
}

func TestSchemaBitesInvalidJSON(t *testing.T) {
	root := writeConfigRepo(t, func(provider, content string) string {
		if provider == "copilot" {
			return "{ not json"
		}
		return content
	})
	failures, err := RunSchema(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "config-schema-json")
}

func TestSchemaBitesMatcherShape(t *testing.T) {
	// Two handlers inside one Claude matcher group violates the
	// exactly-one-handler shape.
	root := writeConfigRepo(t, func(provider, content string) string {
		if provider != "claude-code" {
			return content
		}
		handler := `{
                        "type": "command",
                        "command": "\"${CLAUDE_PLUGIN_ROOT}/hooks/run-hook.cmd\" claude-code pre-tool",
                        "timeout": 25
                    }`
		return mustReplace(t, content, handler, handler+", "+handler)
	})
	failures, err := RunSchema(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "config-schema-claude-shape")
}

func TestSchemaBitesCodexMatcherShape(t *testing.T) {
	// An empty handler list in the codex PreToolUse matcher group violates
	// the exactly-one-handler shape.
	root := writeConfigRepo(t, func(provider, content string) string {
		if provider != "codex" {
			return content
		}
		if !strings.Contains(content, `"hooks": [`) {
			t.Fatal("mutation target not found — the canonical codex testdata config moved")
		}
		// Empty only the first matcher group's handler list (PreToolUse).
		return strings.Replace(content, `"hooks": [`, `"hooks": [], "ignored": [`, 1)
	})
	failures, err := RunSchema(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "config-schema-codex-shape")
}

func TestSchemaBitesNoDispatchCommandsAnywhere(t *testing.T) {
	// All three configs unparseable: nothing to walk, and the harness's
	// found_any guard must refuse to pass on silence.
	root := writeConfigRepo(t, func(provider, content string) string {
		return "not json at all"
	})
	failures, err := RunSchema(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "config-schema-dispatch-routing")
}

// TestSchemaRejectsRemovedPostToolEvent adds the retired Post-tool event back
// to each surface: the roster is PreToolUse and SessionStart, so another event
// must fail even when its handler is well formed.
func TestSchemaRejectsRemovedPostToolEvent(t *testing.T) {
	for _, id := range surface.All() {
		surface := string(id)
		t.Run(surface, func(t *testing.T) {
			root := writeConfigRepo(t, func(provider, content string) string {
				if provider != surface {
					return content
				}
				var d map[string]any
				if err := json.Unmarshal([]byte(content), &d); err != nil {
					t.Fatal(err)
				}
				preName, postName := "PreToolUse", "PostToolUse"
				if provider == "copilot" {
					preName, postName = "preToolUse", "postToolUse"
				}
				rootKey := "hooks"
				if provider == "antigravity" {
					rootKey = "agentics"
				}
				hooks := d[rootKey].(map[string]any)
				hooks[postName] = hooks[preName]
				raw, err := json.Marshal(d)
				if err != nil {
					t.Fatal(err)
				}
				return string(raw)
			})
			failures, err := RunSchema(root)
			if err != nil {
				t.Fatal(err)
			}
			prefix := map[string]string{"claude-code": "claude", "copilot": "copilot", "codex": "codex", "antigravity": "antigravity"}[surface]
			assertRuleFired(t, failures, "config-schema-"+prefix+"-events")
		})
	}
}

func TestSchemaFixturesMatchShippedConfigs(t *testing.T) {
	for provider, spec := range configSpecs {
		t.Run(provider, func(t *testing.T) {
			fixture, err := os.ReadFile(filepath.Join("testdata", "hooks-config", spec.fixture))
			if err != nil {
				t.Fatal(err)
			}
			shipped, err := os.ReadFile(filepath.Join(hosttest.Checkout(t), filepath.FromSlash(spec.relPath)))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(fixture, shipped) {
				t.Fatal("schema fixture differs from shipped config bytes")
			}
		})
	}
}

func TestSchemaBitesRegistrationCounts(t *testing.T) {
	for provider := range configSpecs {
		eventName := "PreToolUse"
		if provider == "copilot" {
			eventName = "preToolUse"
		}
		rootKey := "hooks"
		if provider == "antigravity" {
			rootKey = "agentics"
		}
		for _, count := range []int{0, 2} {
			t.Run(provider+"/pre/"+string(rune('0'+count)), func(t *testing.T) {
				root := writeConfigRepo(t, func(surface, content string) string {
					if surface != provider {
						return content
					}
					var d map[string]any
					if err := json.Unmarshal([]byte(content), &d); err != nil {
						t.Fatal(err)
					}
					hooks := d[rootKey].(map[string]any)
					entries := hooks[eventName].([]any)
					changed := []any{}
					for i := 0; i < count; i++ {
						changed = append(changed, entries[0])
					}
					hooks[eventName] = changed
					raw, err := json.Marshal(d)
					if err != nil {
						t.Fatal(err)
					}
					if string(raw) == content {
						t.Fatal("mutation did not change config bytes")
					}
					return string(raw)
				})
				failures, err := RunSchema(root)
				if err != nil {
					t.Fatal(err)
				}
				prefix := map[string]string{"claude-code": "claude", "copilot": "copilot", "codex": "codex", "antigravity": "antigravity"}[provider]
				suffix := "shape"
				if provider == "copilot" {
					suffix = "pre"
				}
				assertRuleFired(t, failures, "config-schema-"+prefix+"-"+suffix)
			})
		}
	}
}

func TestSchemaBitesCopilotNonobject(t *testing.T) {
	root := writeConfigRepo(t, func(provider, content string) string {
		if provider != "copilot" {
			return content
		}
		var d map[string]any
		if err := json.Unmarshal([]byte(content), &d); err != nil {
			t.Fatal(err)
		}
		d["hooks"].(map[string]any)["preToolUse"] = []any{"not a handler"}
		raw, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	})
	failures, err := RunSchema(root)
	if err != nil {
		t.Fatal(err)
	}
	assertRuleFired(t, failures, "config-schema-copilot-fields")
}

func TestSchemaRetainsLegacyDispatchRouting(t *testing.T) {
	for provider := range configSpecs {
		for _, hook := range []string{"command-guardian", "secrets-guardian", "release-guardian"} {
			t.Run(provider+"/"+hook, func(t *testing.T) {
				command := dispatchCommand(`"/plugin/hooks/run-hook.cmd"`, provider, hook, "--mode report")
				parsed := map[string]map[string]any{provider: {"command": command}}
				if failures := assertDispatchRouting(parsed); len(failures) != 0 {
					t.Fatalf("legacy route rejected: %v", failures)
				}
			})
		}
	}
}

// TestSchemaRejectsRemovedHookNames pins that the three retired hook names no
// longer route: a config that still registered one would ship a command the
// wrapper treats as an unknown name and lets through.
func TestSchemaRejectsRemovedHookNames(t *testing.T) {
	for provider := range configSpecs {
		for _, hook := range []string{"post-tool", "secrets-scanner", "advisor"} {
			t.Run(provider+"/"+hook, func(t *testing.T) {
				command := dispatchCommand(`"/plugin/hooks/run-hook.cmd"`, provider, hook, "")
				parsed := map[string]map[string]any{provider: {"command": command}}
				failures := assertDispatchRouting(parsed)
				assertRuleFired(t, failures, "config-schema-dispatch-routing")
				want := fmt.Sprintf("undocumented hook %q", hook)
				for _, f := range failures {
					if f.Rule == "config-schema-dispatch-routing" && strings.Contains(f.Message, want) {
						return
					}
				}
				t.Fatalf("routing must reject %s as an undocumented hook, got failures: %v", hook, failures)
			})
		}
	}
}

// TestWalkCommandsAgreesWithRegistrations keeps the audit reader, which stays
// separate from the test-support reader, from drifting: both must find the
// same number of dispatcher commands in the shipped configs.
func TestWalkCommandsAgreesWithRegistrations(t *testing.T) {
	checkout := hosttest.Checkout(t)
	hooksDir := filepath.Join(checkout, "hooks")
	walked := map[string]int{}
	for _, rel := range []string{"hooks/hooks.claude.json", "hooks/hooks.copilot.json", "hooks/hooks.codex.json", "hooks.json"} {
		raw, err := os.ReadFile(filepath.Join(checkout, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		var cfg map[string]any
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		for _, command := range walkCommands(cfg) {
			if match := routePattern.FindStringSubmatch(command); match != nil {
				walked[match[2]]++
			}
		}
	}
	registrations, err := hosttest.Registrations(hooksDir)
	if err != nil {
		t.Fatal(err)
	}
	read := map[string]int{}
	for _, r := range registrations {
		read[r.Hook]++
	}
	want := map[string]int{"pre-tool": 6, "startup": 6}
	if !maps.Equal(walked, want) || !maps.Equal(read, want) {
		t.Fatalf("walkCommands found %v, hosttest.Registrations found %v, want %v", walked, read, want)
	}
}
