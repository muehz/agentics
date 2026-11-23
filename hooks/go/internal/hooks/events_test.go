package hooks

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func selectionEvent(t *testing.T, id surface.ID, tool string) hookspec.Event {
	t.Helper()
	normalized, err := contract.NewEvent(contract.EventParts{
		Surface:   id,
		ToolName:  tool,
		ToolInput: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return normalized
}

func TestEventSelection(t *testing.T) {
	type selectionCase struct {
		tool string
		want []string
	}
	for _, provider := range []string{"claude-code", "copilot", "codex", "antigravity"} {
		cases := []selectionCase{
			{"Bash", []string{"commands", "secrets-guardian"}},
			{"PowerShell", []string{"commands", "secrets-guardian"}},
			// A namespaced shell spelling reaches the release rules on every
			// surface, including claude-code, whose own registration omits it.
			{"host.PowerShell", []string{"commands"}},
			{"host.Bash", []string{"commands"}},
		}
		for _, tool := range []string{"Read", "Grep", "Glob", "NotebookRead"} {
			want := []string{"secrets-guardian"}
			if provider != "claude-code" {
				want = []string{"commands", "secrets-guardian"}
			}
			cases = append(cases, selectionCase{tool, want})
		}
		// The file-writing tools reach the commands check on every surface: it
		// judges their target path alone, to protect the hook settings files.
		for _, tool := range []string{"Write", "Edit", "MultiEdit", "NotebookEdit"} {
			cases = append(cases, selectionCase{tool, []string{"commands", "secrets-guardian"}})
		}
		for _, tool := range []string{"mcp__fs__read_text_file", "mcp__fs__read_media_file", "mcp__fs__get_file_contents", "mcp__gh__create_or_update_file", "mcp__fs__read_multiple_files"} {
			cases = append(cases, selectionCase{tool, []string{"commands", "secrets-guardian"}})
		}
		unknown := []string{"commands"}
		if provider == "claude-code" {
			unknown = nil
		}
		cases = append(cases, selectionCase{"UnknownTool", unknown})
		for _, tc := range cases {
			t.Run(provider+"/pre-tool-use/"+tc.tool, func(t *testing.T) {
				checks, err := SelectEventChecks(provider, selectionEvent(t, surface.ID(provider), tc.tool))
				if err != nil {
					t.Fatal(err)
				}
				var got []string
				for _, check := range checks {
					got = append(got, check.Name)
					if len(check.Steps) == 0 {
						if _, registered := Lookup(check.Name); !registered {
							t.Fatalf("selected unregistered check %q", check.Name)
						}
					}
					if (check.Name == "commands") != (len(check.Steps) == 2) {
						t.Fatalf("only the commands check carries its two steps: %+v", check)
					}
					if !check.Guard {
						t.Fatalf("wrong execution metadata: %+v", check)
					}
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("selected=%v want=%v", got, tc.want)
				}
			})
		}
	}
}

// TestShippedClaudeMatcherSelectsACheck reads the shipped Claude registration
// and requires every tool name its matcher admits to reach at least one check,
// so a name added to the matcher cannot reach the grouped hook and run nothing.
// The matcher must keep the form ^(?:a|b|...)$ with literal alternatives, apart
// from the one namespace alternative, so a changed matcher cannot pass unread.
func TestShippedClaudeMatcherSelectsACheck(t *testing.T) {
	registrations, err := hosttest.Registrations(filepath.Join(hosttest.Checkout(t), "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	var matchers []string
	for _, r := range registrations {
		if r.Surface == surface.ClaudeCode && r.Event == "PreToolUse" && r.Hook == "pre-tool" {
			matchers = append(matchers, r.Matcher)
		}
	}
	if len(matchers) != 1 {
		t.Fatalf("want one claude-code PreToolUse pre-tool registration, got %d", len(matchers))
	}
	matcher := matchers[0]
	const open, closing = "^(?:", ")$"
	if !strings.HasPrefix(matcher, open) || !strings.HasSuffix(matcher, closing) {
		t.Fatalf("matcher %q does not have the form ^(?:a|b|...)$", matcher)
	}
	alternatives := strings.Split(strings.TrimSuffix(strings.TrimPrefix(matcher, open), closing), "|")
	for _, alternative := range alternatives {
		name := alternative
		switch {
		case alternative == "mcp__.*":
			name = "mcp__server__tool"
		case alternative == "" || regexp.QuoteMeta(alternative) != alternative:
			t.Fatalf("matcher alternative %q is neither a literal tool name nor mcp__.*", alternative)
		}
		t.Run(name, func(t *testing.T) {
			checks, err := SelectEventChecks("claude-code", selectionEvent(t, surface.ClaudeCode, name))
			if err != nil {
				t.Fatal(err)
			}
			if len(checks) == 0 {
				t.Fatalf("the shipped matcher admits %q but no check is selected for it", name)
			}
		})
	}
}

func TestEventSelectionRejectsInvalidInput(t *testing.T) {
	if _, err := SelectEventChecks("claude-code", hookspec.Event{}); err == nil {
		t.Fatal("absent event accepted")
	}
	event := selectionEvent(t, surface.ClaudeCode, "Bash")
	for _, provider := range []string{"unknown", "copilot", "codex", "antigravity"} {
		if _, err := SelectEventChecks(provider, event); err == nil {
			t.Fatalf("provider mismatch accepted: %s", provider)
		}
	}
}

func TestCommandsCheckSteps(t *testing.T) {
	checks, err := SelectEventChecks("codex", selectionEvent(t, surface.Codex, "Bash"))
	if err != nil {
		t.Fatal(err)
	}
	var commands *EventCheck
	for i := range checks {
		if checks[i].Name == "commands" {
			commands = &checks[i]
		}
	}
	if commands == nil {
		t.Fatalf("no commands check selected: %+v", checks)
	}
	want := []Step{{"command rules", "command-guardian"}, {"release rules", "release-guardian"}}
	if !reflect.DeepEqual(commands.Steps, want) {
		t.Fatalf("steps=%+v want=%+v", commands.Steps, want)
	}
	for _, step := range commands.Steps {
		if handler, ok := Lookup(step.Hook); !ok || handler == nil {
			t.Fatalf("step %q names an unregistered hook %q", step.Label, step.Hook)
		}
	}
	for _, check := range checks {
		if check.Name != "commands" && len(check.Steps) != 0 {
			t.Fatalf("only the commands check declares steps: %+v", check)
		}
	}
}

func TestNewScriptSnapshotRefusesProtectedPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.sh"), []byte("echo hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	protected := filepath.ToSlash(filepath.Join(dir, ".env.sh"))
	if got := NewScriptSnapshot().ReadScript(protected); got.Limit != shellwords.RulePathProtected {
		t.Fatalf("%+v", got)
	}
}
