package provider

import (
	"fmt"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
)

// argvSurface is one surface under test: its traits and the tool-use event
// its hooks serve, plus the rejection shape the surface stamps on every argv
// error. The event is a literal so the test does not read it from the code
// under test.
type argvSurface struct {
	traits traits
	event  string
	stdout string
	exit   int
}

// argvSurfaces is the full roster. Every case dispatches through Dispatch,
// the registry path the binary uses, so the test proves the registered
// pairing of traits and surface.
var argvSurfaces = []argvSurface{
	{traits: claudeTraits, event: "PreToolUse", stdout: "", exit: hookspec.ExitUsage},
	{traits: copilotTraits, event: "preToolUse", stdout: "", exit: hookspec.ExitOK},
	{traits: codexTraits, event: "PreToolUse", stdout: "{}\n", exit: hookspec.ExitOK},
	{traits: antigravityTraits, event: "PreToolUse", stdout: "{\"decision\":\"allow\"}\n", exit: hookspec.ExitOK},
}

// argvGuards are the hook names the shipped configurations pass to the
// single-check form.
var argvGuards = []string{"command-guardian", "release-guardian", "secrets-guardian"}

func TestDispatchEventCommands(t *testing.T) {
	for _, surface := range argvSurfaces {
		t.Run(surface.traits.name+"/pre-tool", func(t *testing.T) {
			want := "PreToolUse"
			if surface.traits.name == "copilot" {
				want = "preToolUse"
			}
			event, reject := Dispatch(surface.traits.name, "pre-tool", nil)
			if reject != nil || event != want {
				t.Fatalf("event=%q reject=%+v, want event=%q", event, reject, want)
			}
			_, reject = Dispatch(surface.traits.name, "pre-tool", []string{"--mode", "block"})
			if reject == nil || string(reject.Stderr) != surface.traits.name+": pre-tool: event commands take no arguments\n" || reject.Exit != surface.exit {
				t.Fatalf("unexpected event argv rejection: %+v", reject)
			}
		})
	}
}

func TestEventRenderDispatchContract(t *testing.T) {
	cases := []struct {
		surface, verb, stdout, stderr string
		exit                          int
	}{
		{"claude-code", "pre-tool", "", "blocked\n", 2},
		{"copilot", "pre-tool", "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"blocked\"}\n", "", 0},
		{"codex", "pre-tool", "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"blocked\"}}\n", "", 0},
		{"antigravity", "pre-tool", "{\"decision\":\"deny\",\"reason\":\"blocked\"}\n", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.surface+"/"+tc.verb, func(t *testing.T) {
			event, reject := Dispatch(tc.surface, tc.verb, nil)
			if reject != nil {
				t.Fatalf("event dispatch rejected: %+v", reject)
			}
			rendered := Render(tc.surface, event, contract.Deny("blocked"), nil)
			if rendered.Exit != tc.exit || string(rendered.Stdout) != tc.stdout || string(rendered.Stderr) != tc.stderr {
				t.Fatalf("event dispatch/render protocol changed: %+v; want exit=%d stdout=%q stderr=%q", rendered, tc.exit, tc.stdout, tc.stderr)
			}
		})
	}
}

// argvCase is one invocation and its expected result. A non-empty wantErr is
// the text after `<surface>: <hook>: `; otherwise the dispatch must succeed.
type argvCase struct {
	name    string
	args    []string
	wantErr string
}

// TestDispatch walks every surface and every guard and pins the event each
// accepted argv selects and every diagnostic the parser can produce. The
// diagnostics have one shape on every surface — `<surface>: <hook>: <fault>` —
// and the same rules for every hook: --mode must carry block, and nothing else
// is accepted.
func TestDispatch(t *testing.T) {
	for _, surface := range argvSurfaces {
		for _, hook := range argvGuards {
			for _, tc := range argvCasesFor() {
				t.Run(surface.traits.name+"/"+hook+"/"+tc.name, func(t *testing.T) {
					event, reject := Dispatch(surface.traits.name, hook, tc.args)
					if tc.wantErr != "" {
						if reject == nil {
							t.Fatalf("expected a rejection, got event %q", event)
						}
						want := fmt.Sprintf("%s: %s: %s\n", surface.traits.name, hook, tc.wantErr)
						if got := string(reject.Stderr); got != want {
							t.Errorf("stderr = %q, want %q", got, want)
						}
						if got := string(reject.Stdout); got != surface.stdout {
							t.Errorf("stdout = %q, want %q", got, surface.stdout)
						}
						if reject.Exit != surface.exit {
							t.Errorf("exit = %d, want %d (an argv fault is never a deny)", reject.Exit, surface.exit)
						}
						return
					}
					if reject != nil {
						t.Fatalf("unexpected rejection: exit %d stderr %q", reject.Exit, reject.Stderr)
					}
					if event != surface.event {
						t.Errorf("event = %q, want %q", event, surface.event)
					}
				})
			}
		}
	}
}

// argvCasesFor is every case the single-check argv grammar distinguishes. The
// only accepted mode word is block.
func argvCasesFor() []argvCase {
	const mode = "block"
	needMode := "--mode must be " + mode
	return []argvCase{
		{name: "no args", args: nil, wantErr: needMode},
		{name: "an empty argv", args: []string{}, wantErr: needMode},
		{name: "the wrong mode word", args: []string{"--mode", "nonsense"}, wantErr: needMode},
		{name: "an empty mode word", args: []string{"--mode", ""}, wantErr: needMode},
		{name: "the mode flag alone", args: []string{"--mode"}, wantErr: "flag needs an argument: -mode"},
		{name: "an unknown flag", args: []string{"--frobnicate"}, wantErr: "flag provided but not defined: -frobnicate"},
		{name: "help", args: []string{"-h"}, wantErr: "flag: help requested"},
		{name: "a bare empty string", args: []string{""}, wantErr: "unexpected argument: "},
		{name: "the exact " + mode + " argv is accepted", args: []string{"--mode", mode}},
		{name: "the equals " + mode + " form is accepted", args: []string{"--mode=" + mode}},
		{name: "a scope flag is unknown", args: []string{"--scope", "diff"}, wantErr: "flag provided but not defined: -scope"},
		{name: "an extra flag", args: []string{"--mode", mode, "--extra"}, wantErr: "flag provided but not defined: -extra"},
		{name: "an extra word", args: []string{"--mode", mode, "extra"}, wantErr: "unexpected argument: extra"},
	}
}

// TestDiagnosticsUseTheHookName runs every surface and guard through Dispatch,
// so the surface and hook names in the diagnostic are proven to come from the
// parameters, not a literal.
func TestDiagnosticsUseTheHookName(t *testing.T) {
	for _, surface := range argvSurfaces {
		for _, hook := range argvGuards {
			for _, tc := range []struct {
				name string
				args []string
				want string
			}{
				{"unknown flag", []string{"--bogus"}, "flag provided but not defined: -bogus"},
				{"wrong mode word", []string{"--mode", "nope"}, "--mode must be block"},
			} {
				t.Run(surface.traits.name+"/"+hook+"/"+tc.name, func(t *testing.T) {
					_, reject := Dispatch(surface.traits.name, hook, tc.args)
					if reject == nil {
						t.Fatal("expected a rejection")
					}
					want := surface.traits.name + ": " + hook + ": " + tc.want + "\n"
					if got := string(reject.Stderr); got != want {
						t.Errorf("stderr = %q, want %q", got, want)
					}
				})
			}
		}
	}
}
