package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hooks"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/provider"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func invokeWithDependencies(t *testing.T, deps hookDependencies, stdin string, argv ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = runWithDependencies(deps, argv, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

// invoke runs the entry point with the given stdin and returns the exit code
// plus both captured streams.
func invoke(t *testing.T, stdin string, argv ...string) (code int, stdout, stderr string) {
	t.Helper()
	return invokeWithDependencies(t, productionHookDependencies(), stdin, argv...)
}

// anyProvider returns a known provider name without hardcoding one here; the
// roster is the surface package's business.
func anyProvider(t *testing.T) surface.ID {
	t.Helper()
	all := surface.All()
	if len(all) == 0 {
		t.Fatal("no providers registered")
	}
	return all[0]
}

// legalArgs returns compatibility argv for retained legacy hooks and reads
// the shipped config for event commands. The config/dispatch join test still
// validates every actual registration independently of this helper.
func legalArgs(providerName surface.ID, hook string) []string {
	if _, known := surface.Parse(string(providerName)); !known {
		return nil
	}
	switch hook {
	case "command-guardian", "release-guardian", "secrets-guardian":
		return []string{"--mode", "block"}
	}
	args, ok := configArgs()[string(providerName)][hook]
	if !ok {
		return nil
	}
	result := make([]string, len(args))
	copy(result, args)
	return result
}

func TestLegalArgsRetainsLegacyHooksAfterEventMigration(t *testing.T) {
	// Model migrated registrations without modifying the cached real config join.
	original := configArgs
	grouped := map[string]map[string][]string{}
	for _, id := range surface.All() {
		grouped[string(id)] = map[string][]string{"pre-tool": {}}
	}
	configArgs = func() map[string]map[string][]string { return grouped }
	t.Cleanup(func() { configArgs = original })
	for _, id := range surface.All() {
		name := string(id)
		for _, hook := range hooks.Names() {
			t.Run(name+"/"+hook, func(t *testing.T) {
				args := legalArgs(id, hook)
				if len(args) == 0 {
					t.Fatalf("retained hook %s/%s has no compatibility argv after migration", name, hook)
				}
				called := false
				deps := stubNormalize(t, stubLookup(productionHookDependencies(), func(req hookspec.Request) hookspec.Outcome {
					called = true
					if req.Provider != name || req.Hook != hook || req.Context != nil {
						t.Errorf("legacy request changed: %+v", req)
					}
					return contract.Pass()
				}))
				exit, out, errout := invokeWithLegalArgsAndDependencies(t, deps, `{}`, id, hook)
				wantOut := ""
				switch name {
				case "codex":
					wantOut = "{}\n"
				case "antigravity":
					wantOut = "{\"decision\":\"allow\"}\n"
				}
				if !called || exit != 0 || out != wantOut || errout != "" {
					t.Fatalf("legacy path: called=%v exit=%d stdout=%q stderr=%q", called, exit, out, errout)
				}
			})
		}
		const verb = "pre-tool"
		args := legalArgs(id, verb)
		if args == nil || len(args) != 0 {
			t.Fatalf("event argv must be present and empty: %s %s %v", name, verb, args)
		}
		if _, reject := provider.Dispatch(name, verb, args); reject != nil {
			t.Fatalf("event argv rejected: %s %s", name, verb)
		}
	}
	if legalArgs("unknown", "command-guardian") != nil || legalArgs("codex", "missing") != nil {
		t.Fatal("unknown provider/hook received compatibility arguments")
	}
}

// configArgs is the provider -> hook -> argv map the three shipped configs
// pin, built once for the whole test binary.
//
// It returns an EMPTY map on any failure rather than calling t.Fatal, because
// sync.OnceValue has no *testing.T. invokeWithLegalArgs checks for the empty
// map — a config-reading legalArgs can fail where a hardcoded one could not,
// and a silent empty argv would turn every caller into a bare invocation that
// two surfaces reject for the wrong reason.
var configArgs = sync.OnceValue(func() map[string]map[string][]string {
	root, err := hosttest.FindCheckout()
	if err != nil {
		return map[string]map[string][]string{}
	}
	registrations, err := hosttest.Registrations(filepath.Join(root, "hooks"))
	if err != nil {
		return map[string]map[string][]string{}
	}
	byProvider := map[string]map[string][]string{}
	for _, r := range registrations {
		if byProvider[string(r.Surface)] == nil {
			byProvider[string(r.Surface)] = map[string][]string{}
		}
		byProvider[string(r.Surface)][r.Hook] = r.Args
	}
	return byProvider
})

func invokeWithLegalArgsAndDependencies(t *testing.T, deps hookDependencies, stdin string, providerName surface.ID, hook string) (int, string, string) {
	t.Helper()
	if len(configArgs()) == 0 {
		t.Fatal("no argv could be read from the shipped hook configs — " +
			"every invokeWithLegalArgs caller would run with no argv at all")
	}
	args := legalArgs(providerName, hook)
	if args == nil {
		t.Fatalf("the shipped hook configs pin no argv for %s %s", providerName, hook)
	}
	argv := append([]string{string(providerName), hook}, args...)
	return invokeWithDependencies(t, deps, stdin, argv...)
}

// invokeWithLegalArgs drives run() for (provider, hook) with that pair's
// pinned argv prepended, so the invocation clears dispatch on every surface.
func invokeWithLegalArgs(t *testing.T, stdin string, providerName surface.ID, hook string) (int, string, string) {
	t.Helper()
	return invokeWithLegalArgsAndDependencies(t, productionHookDependencies(), stdin, providerName, hook)
}

// ---------------------------------------------------------------------------
// The shipped hook configs, read back and checked against the argv parser
// ---------------------------------------------------------------------------

// The argv contract is written down in two places: the shipped hook configs,
// which the host executes, and internal/provider's argv parser with each
// surface's tool-use event, which read what the host sent. This test is the join, and the cost of a drifted
// mode word is not the same on every surface:
//
//	claude-code  exit 1  — visible; the host surfaces it
//	copilot      exit 0  — the guard is OFF, and the one stderr line is discarded
//	codex        exit 0  — the same
//
// Every shipped row on every surface is validated, so a drifted mode word
// anywhere is caught here. That matters most on the two fail-open surfaces,
// where a config typo silently disables a guardrail.
func TestShippedHookConfigsAgreeWithTheArgvTables(t *testing.T) {
	root, err := hosttest.FindCheckout()
	if err != nil {
		t.Fatal(err)
	}
	hooksDir := filepath.Join(root, "hooks")
	all, err := hosttest.Registrations(hooksDir)
	if err != nil {
		t.Fatal(err)
	}

	// Each event has six command fields across the four providers. Counting
	// them separately catches a reader that mistakes startup for a guard.
	guards, startups := 0, 0
	for _, r := range all {
		if r.Hook == "startup" {
			startups++
			want := "SessionStart"
			if r.Surface == surface.Copilot {
				want = "sessionStart"
			}
			if r.Event != want || len(r.Args) != 0 {
				t.Errorf("startup registration has event %q and args %v, want %q and no args", r.Event, r.Args, want)
			}
			continue
		}
		guards++
		event, reject := provider.Dispatch(string(r.Surface), r.Hook, r.Args)
		if reject != nil {
			t.Errorf("%s would reject its own shipped argv %v for %s:\n  %s\n  %s",
				r.Surface, r.Args, r.Hook, r.Command,
				strings.TrimRight(string(reject.Stderr), "\n"))
			continue
		}
		// The event key the command is declared under must be the surface's
		// tool-use event, which every hook serves. A guard wired to the wrong
		// event never fires, and nothing else would notice.
		if event != r.Event {
			t.Errorf("%s %s is declared under event %q but the surface's tool-use event is %q",
				r.Surface, r.Hook, r.Event, event)
		}
	}
	if guards != 6 || startups != 6 {
		t.Fatalf("found %d guard and %d startup command strings, want six each", guards, startups)
	}
}

// stubLookup swaps the registry lookup for one test's dependency value so a
// deliberately misbehaving handler can be driven through the real run().
func stubLookup(deps hookDependencies, h hookspec.Handler) hookDependencies {
	deps.lookup = func(string) (hookspec.Handler, bool) { return h, true }
	return deps
}

// stubNormalize forces the normalize stage to SUCCEED, handing the handler a
// fixed neutral event.
//
// It keeps a test that is about the HANDLER, validation, or Render stage from
// depending on which payload shapes a surface's real Normalize accepts.
func stubNormalize(t *testing.T, deps hookDependencies) hookDependencies {
	t.Helper()
	event, err := contract.NewEvent(contract.EventParts{
		Surface:   surface.Codex,
		ToolName:  "",
		ToolInput: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	deps.normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) {
		return event, nil
	}
	return deps
}

// renderCall records one Render invocation.
type renderCall struct {
	provider string
	event    string
	outcome  hookspec.Outcome
	stderr   []byte
}

// stubRender captures what the pipeline hands to Render and returns a fixed
// response, so a test can assert on the OUTCOME rather than on one surface's
// rendering of it.
func stubRender(deps hookDependencies, out provider.Rendered) (hookDependencies, *[]renderCall) {
	calls := new([]renderCall)
	deps.render = func(p, e string, o hookspec.Outcome, s []byte) provider.Rendered {
		*calls = append(*calls, renderCall{provider: p, event: e, outcome: o, stderr: s})
		return out
	}
	return deps, calls
}

// The roster must not silently drift. This guards the exact sorted name
// lists, not just their counts, so adding or renaming a hook or provider is
// a deliberate edit here.
func TestRegistryRoster(t *testing.T) {
	wantHooks := []string{"command-guardian", "release-guardian", "secrets-guardian"}
	wantProviders := []surface.ID{"antigravity", "claude-code", "codex", "copilot"}
	if got := hooks.Names(); !slices.Equal(got, wantHooks) {
		t.Errorf("registered hooks = %v, want %v", got, wantHooks)
	}
	if got := surface.All(); !slices.Equal(got, wantProviders) {
		t.Errorf("surfaces = %v, want %v", got, wantProviders)
	}
}

// Every hook must resolve through the registry to its OWN handler. A copy-paste
// slip that wired two names to one function would pass any behavioral check
// while the stubs all return the same outcome, so distinctness is asserted on
// the function value itself.
func TestEachHookResolvesToADistinctHandler(t *testing.T) {
	seen := make(map[uintptr]string, len(hooks.Names()))

	for _, name := range hooks.Names() {
		handler, ok := hooks.Lookup(name)
		if !ok {
			t.Fatalf("%s: not resolvable through the registry", name)
		}
		identity := reflect.ValueOf(handler).Pointer()
		if prev, dup := seen[identity]; dup {
			t.Errorf("%s and %s resolve to the same function — they share a handler", prev, name)
		}
		seen[identity] = name
	}
}

// Every hook resolves through the registry to a distinct handler. Outcome
// validity and Diag discipline are each hook package's own concern.
// The entry point guards against an invalid outcome at runtime:
// main checks Outcome.Valid and substitutes an error
// outcome, which TestAnInvalidOutcomeBecomesTheFixedDiagnostic pins.
func TestEveryHandlerResolves(t *testing.T) {
	for _, name := range hooks.Names() {
		if _, ok := hooks.Lookup(name); !ok {
			t.Fatalf("%s: not resolvable through the registry", name)
		}
	}
}

// The no-args invocation is the one shape CI asserts on every pull request. An
// empty payload never normalizes on any surface. These values are pinned by
// this suite and the behavior corpus, so changing them produces a reviewed diff.
func TestNoArgsFailsInEachSurfacesOwnShape(t *testing.T) {
	want := map[string]struct {
		exit   int
		stdout string
	}{
		"antigravity": {exit: 0, stdout: "{\"decision\":\"allow\"}\n"},
		"claude-code": {exit: 1, stdout: ""},
		"copilot":     {exit: 0, stdout: ""},
		"codex":       {exit: 0, stdout: "{}\n"},
	}

	for _, id := range surface.All() {
		p := string(id)
		expected, known := want[p]
		if !known {
			t.Fatalf("provider %q has no pinned no-args expectation", p)
		}
		for _, name := range hooks.Names() {
			code, stdout, stderr := invoke(t, "", p, name)

			if code != expected.exit {
				t.Errorf("%s %s: exit = %d, want %d (stderr: %q)",
					p, name, code, expected.exit, stderr)
			}
			if stdout != expected.stdout {
				t.Errorf("%s %s: stdout = %q, want %q", p, name, stdout, expected.stdout)
			}
			if stderr == "" {
				t.Errorf("%s %s: want a diagnostic on stderr, got none", p, name)
			}
		}
	}
}

// A core's progress chatter is captured and DISCARDED on a pass. Only an error
// outcome can render the captured diagnostic, so the behavior corpus pins empty
// stderr for every core-reaching pass.
func TestCoreChatterIsDiscardedOnPass(t *testing.T) {
	deps := productionHookDependencies()
	deps = stubNormalize(t, deps)
	deps = stubLookup(deps, func(req hookspec.Request) hookspec.Outcome {
		_, _ = req.Diag.Write([]byte("scanning 12 files\n"))
		return contract.Pass()
	})

	code, stdout, stderr := invokeWithLegalArgsAndDependencies(t, deps, "{}", anyProvider(t), hooks.Names()[0])

	if code != hookspec.ExitOK {
		t.Errorf("exit = %d, want %d", code, hookspec.ExitOK)
	}
	if strings.Contains(stderr, "scanning") {
		t.Errorf("stderr = %q, want the core's chatter discarded on a pass", stderr)
	}
	if strings.Contains(stdout, "scanning") {
		t.Errorf("stdout = %q, want the core's chatter kept off stdout too", stdout)
	}
}

// On an error outcome the CAPTURED core stderr takes precedence over the
// outcome's own message on every surface.
func TestCoreChatterWinsOverTheMessageOnError(t *testing.T) {
	deps := productionHookDependencies()
	deps = stubNormalize(t, deps)
	deps = stubLookup(deps, func(req hookspec.Request) hookspec.Outcome {
		_, _ = req.Diag.Write([]byte("core said this\n"))
		return contract.Error("outcome said that")
	})

	_, _, stderr := invokeWithLegalArgsAndDependencies(t, deps, "{}", anyProvider(t), hooks.Names()[0])

	if !strings.Contains(stderr, "core said this") {
		t.Errorf("stderr = %q, want the captured core stderr", stderr)
	}
	if strings.Contains(stderr, "outcome said that") {
		t.Errorf("stderr = %q, want the captured stderr to REPLACE the message", stderr)
	}
}

// The entry point validates the handler's outcome — the handler does not, and
// neither does the surface. Every surface renders the same fixed diagnostic
// for a rejected outcome.
func TestAnInvalidOutcomeBecomesTheFixedDiagnostic(t *testing.T) {
	deps := productionHookDependencies()
	deps = stubNormalize(t, deps)
	deps = stubLookup(deps, func(req hookspec.Request) hookspec.Outcome {
		_, _ = req.Diag.Write([]byte("chatter that must be discarded\n"))
		return hookspec.Outcome{}
	})

	deps, calls := stubRender(deps, provider.Rendered{Exit: hookspec.ExitOK})

	invokeWithLegalArgsAndDependencies(t, deps, "{}", anyProvider(t), hooks.Names()[0])

	if len(*calls) != 1 {
		t.Fatalf("Render called %d times, want 1", len(*calls))
	}
	call := (*calls)[0]

	if call.outcome.Kind() != contract.KindError {
		t.Errorf("outcome kind = %q, want %q", call.outcome.Kind(), contract.KindError)
	}
	if want := "core produced an invalid neutral outcome"; call.outcome.Payload() != want {
		t.Errorf("outcome message = %q, want %q", call.outcome.Payload(), want)
	}
	if len(call.stderr) != 0 {
		t.Errorf("coreStderr = %q, want it discarded alongside the rejected outcome", call.stderr)
	}
}

// A panic must be RENDERED THROUGH THE PROVIDER, not turned into a flat exit.
// A recovered panic becomes a kind=error outcome, which is exit 1 on claude-code
// but exit 0 on copilot and exit 0 with `{}` on stdout for codex. A panic that
// exited 1 with no body would break two surfaces' fail-open contract.
func TestAPanicIsRenderedThroughTheProvider(t *testing.T) {
	want := map[string]struct {
		exit   int
		stdout string
	}{
		"antigravity": {exit: 0, stdout: "{\"decision\":\"allow\"}\n"},
		"claude-code": {exit: 1, stdout: ""},
		"copilot":     {exit: 0, stdout: ""},
		"codex":       {exit: 0, stdout: "{}\n"},
	}

	for _, id := range surface.All() {
		p := string(id)
		expected, known := want[p]
		if !known {
			t.Fatalf("provider %q has no pinned panic expectation", p)
		}

		t.Run(p, func(t *testing.T) {
			deps := productionHookDependencies()
			deps = stubNormalize(t, deps)
			deps = stubLookup(deps, func(hookspec.Request) hookspec.Outcome {
				panic("boom")
			})

			code, stdout, stderr := invokeWithLegalArgsAndDependencies(t, deps, "{}", id, hooks.Names()[0])

			if code == hookspec.ExitDeny {
				t.Fatalf("a panic exited %d — the reserved blocking code", hookspec.ExitDeny)
			}
			if code != expected.exit {
				t.Errorf("exit = %d, want %d", code, expected.exit)
			}
			if stdout != expected.stdout {
				t.Errorf("stdout = %q, want %q", stdout, expected.stdout)
			}
			if !strings.Contains(stderr, "boom") {
				t.Errorf("stderr = %q, want it to report the panic value", stderr)
			}
			if !strings.Contains(stderr, "internal error") {
				t.Errorf("stderr = %q, want it to name the fault as internal", stderr)
			}
		})
	}
}

func TestAPipelineStagePanicIsRenderedThroughTheProvider(t *testing.T) {
	want := map[string]struct {
		exit   int
		stdout string
	}{
		"antigravity": {0, "{\"decision\":\"allow\"}\n"}, "claude-code": {1, ""}, "copilot": {0, ""}, "codex": {0, "{}\n"},
	}
	for _, id := range surface.All() {
		name := string(id)
		expected, ok := want[name]
		if !ok {
			t.Fatalf("provider %q has no pinned panic expectation", name)
		}
		for _, stage := range []string{"lookup", "dispatch", "normalize", "render"} {
			t.Run(name+"/"+stage, func(t *testing.T) {
				deps := productionHookDependencies()
				marker := "CQ22 " + stage + " panic"
				renderCalls := 0
				switch stage {
				case "lookup":
					deps.lookup = func(string) (hookspec.Handler, bool) { panic(marker) }
				case "dispatch":
					deps.dispatch = func(string, string, []string) (string, *provider.Rendered) { panic(marker) }
				case "normalize":
					deps.normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) { panic(marker) }
				case "render":
					deps = stubNormalize(t, deps)
					deps = stubLookup(deps, func(hookspec.Request) hookspec.Outcome { return contract.Pass() })
					deps.render = func(string, string, hookspec.Outcome, []byte) provider.Rendered { renderCalls++; panic(marker) }
				}
				code, stdout, stderr := invokeWithLegalArgsAndDependencies(t, deps, "{}", id, hooks.Names()[0])
				if code != expected.exit || stdout != expected.stdout {
					t.Fatalf("exit=%d stdout=%q, want %d/%q", code, stdout, expected.exit, expected.stdout)
				}
				if !strings.Contains(stderr, marker) || !strings.Contains(stderr, "internal error") {
					t.Fatalf("panic diagnostic lost: %q", stderr)
				}
				if stage == "render" && renderCalls != 1 {
					t.Fatalf("injected renderer called %d times, want once", renderCalls)
				}
			})
		}
	}
}

func TestInvocationIsolation(t *testing.T) {
	surface := anyProvider(t)
	entered := make(chan string, 2)
	release := make(chan struct{})
	type invocationResult struct {
		label          string
		code           int
		stdout, stderr string
	}
	results := make(chan invocationResult, 2)

	start := func(label string) {
		deps := productionHookDependencies()
		deps.lookup = func(string) (hookspec.Handler, bool) {
			entered <- label
			<-release
			return func(req hookspec.Request) hookspec.Outcome {
				_, _ = fmt.Fprintf(req.Diag, "%s diagnostic\n", label)
				return contract.Error(label + " outcome")
			}, true
		}
		deps.dispatch = func(string, string, []string) (string, *provider.Rendered) {
			return "PreToolUse", nil
		}
		deps.normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) {
			return hookspec.Event{}, nil
		}
		deps.render = func(_ string, _ string, outcome hookspec.Outcome, coreStderr []byte) provider.Rendered {
			if outcome.Payload() != label+" outcome" || string(coreStderr) != label+" diagnostic\n" {
				return provider.Rendered{Stdout: []byte("crossed dependency\n"), Exit: hookspec.ExitUsage}
			}
			return provider.Rendered{
				Stdout: []byte(label + " stdout\n"),
				Stderr: []byte(label + " stderr\n"),
				Exit:   hookspec.ExitOK,
			}
		}
		var stdout, stderr bytes.Buffer
		code := runWithDependencies(deps, []string{string(surface), "isolated-" + label}, strings.NewReader("{}"), &stdout, &stderr)
		results <- invocationResult{label: label, code: code, stdout: stdout.String(), stderr: stderr.String()}
	}

	go start("alpha")
	go start("beta")
	seen := map[string]bool{}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	timedOut := false
	for len(seen) < 2 && !timedOut {
		select {
		case label := <-entered:
			seen[label] = true
		case <-timer.C:
			timedOut = true
		}
	}
	close(release)

	got := map[string]invocationResult{}
	completion := time.NewTimer(2 * time.Second)
	defer completion.Stop()
	for range 2 {
		select {
		case result := <-results:
			got[result.label] = result
		case <-completion.C:
			t.Fatalf("invocations did not finish after release; completed=%d", len(got))
		}
	}
	if timedOut {
		t.Fatalf("both dependency-local lookups were not entered: %v", seen)
	}
	for _, label := range []string{"alpha", "beta"} {
		result := got[label]
		if result.code != hookspec.ExitOK || result.stdout != label+" stdout\n" || result.stderr != label+" stderr\n" {
			t.Errorf("%s: exit=%d stdout=%q stderr=%q", label, result.code, result.stdout, result.stderr)
		}
	}
}

// THE SURVIVING INVARIANT: exit 2 reaches the caller ONLY through a provider's
// Render mapping. Never from argv rejection, never from a Dispatch or Normalize
// rejection, and never from a panic.
func TestExitDenyComesOnlyFromRender(t *testing.T) {
	surface := anyProvider(t)

	// Every path that is not Render must refuse to produce 2.
	t.Run("no other path produces it", func(t *testing.T) {
		cases := []struct {
			desc  string
			stdin string
			argv  []string
			setup func(*testing.T, hookDependencies) hookDependencies
		}{
			{desc: "no arguments"},
			{desc: "unknown provider", argv: []string{"bogus-provider", hooks.Names()[0]}},
			{desc: "unknown hook", argv: []string{string(surface), "bogus-hook"}},
			{desc: "malformed payload", stdin: "not json", argv: []string{string(surface), hooks.Names()[0]}},
			{
				desc:  "panicking handler",
				stdin: "{}",
				argv:  []string{string(surface), hooks.Names()[0]},
				setup: func(t *testing.T, deps hookDependencies) hookDependencies {
					deps = stubNormalize(t, deps)
					return stubLookup(deps, func(hookspec.Request) hookspec.Outcome { panic("boom") })
				},
			},
			{
				desc:  "handler returning garbage",
				stdin: "{}",
				argv:  []string{string(surface), hooks.Names()[0]},
				setup: func(t *testing.T, deps hookDependencies) hookDependencies {
					deps = stubNormalize(t, deps)
					return stubLookup(deps, func(hookspec.Request) hookspec.Outcome { return hookspec.Outcome{} })
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.desc, func(t *testing.T) {
				deps := productionHookDependencies()
				if tc.setup != nil {
					deps = tc.setup(t, deps)
				}
				if code, _, _ := invokeWithDependencies(t, deps, tc.stdin, tc.argv...); code == hookspec.ExitDeny {
					t.Errorf("exit = %d — the reserved blocking code escaped a non-Render path",
						hookspec.ExitDeny)
				}
			})
		}
	})

	// And Render's own mapping must reach the caller unaltered: nothing
	// downstream may swallow a surface's blocking decision.
	t.Run("Render's mapping reaches the caller", func(t *testing.T) {
		deps := productionHookDependencies()
		deps = stubNormalize(t, deps)
		deps = stubLookup(deps, func(hookspec.Request) hookspec.Outcome {
			return contract.Deny("dangerous command")
		})
		deps, _ = stubRender(deps, provider.Rendered{
			Stderr: []byte("dangerous command\n"),
			Exit:   hookspec.ExitDeny,
		})

		code, _, stderr := invokeWithLegalArgsAndDependencies(t, deps, "{}", surface, hooks.Names()[0])
		if code != hookspec.ExitDeny {
			t.Errorf("exit = %d, want %d", code, hookspec.ExitDeny)
		}
		if !strings.Contains(stderr, "dangerous command") {
			t.Errorf("stderr = %q, want the deny reason", stderr)
		}
	})
}

// An invalid environment mode must be observable at the host seam when the
// block-mode fallback reports a finding; a core-only diagnostic is discarded
// by pass and deny render paths.
func TestSecretsGuardianInvalidModeDiagnosticReachesEveryProvider(t *testing.T) {
	t.Setenv("SECRETS_GUARD_MODE", "blcok")
	const warning = `SECRETS_GUARD_MODE="blcok" is not warn or block; using block`
	for _, surface := range surface.All() {
		t.Run(string(surface), func(t *testing.T) {
			payload := hosttest.Stdin(surface, hosttest.Call{Tool: "Read", Input: map[string]any{"file_path": "/tmp/.env"}, Cwd: "/tmp"})
			code, stdout, stderr := invokeWithLegalArgs(t, string(payload), surface, "secrets-guardian")
			verdict := hosttest.Read(surface, code, []byte(stdout), []byte(stderr))
			if verdict.Decision != hosttest.Deny {
				t.Fatalf("host reads %v, want Deny: exit=%d stdout=%q stderr=%q", verdict.Decision, code, stdout, stderr)
			}
			// The Claude Code reason is the stderr itself; the JSON surfaces
			// carry it in the document and leave stderr empty.
			if surface != "claude-code" && stderr != "" {
				t.Fatalf("stderr=%q, want it empty beside a %s JSON deny", stderr, surface)
			}
			if !strings.Contains(verdict.Reason, warning) || !strings.Contains(verdict.Reason, "credential file read") {
				t.Fatalf("reason=%q, want invalid-mode warning and finding", verdict.Reason)
			}
		})
	}
}

// Bad argv is a usage error (1), never a deny (2), and it never reaches a
// surface: the provider may not even be valid on this path.
func TestArgvValidationIsUsageErrorNeverDeny(t *testing.T) {
	surface := anyProvider(t)

	cases := []struct {
		desc string
		argv []string
	}{
		{"no arguments", nil},
		{"provider only", []string{string(surface)}},
		{"unknown provider", []string{"bogus-provider", hooks.Names()[0]}},
		{"unknown hook", []string{string(surface), "bogus-hook"}},
		{"empty provider", []string{"", hooks.Names()[0]}},
		{"empty hook", []string{string(surface), ""}},
		{"provider and hook transposed", []string{hooks.Names()[0], string(surface)}},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			code, stdout, stderr := invoke(t, "", tc.argv...)

			if code != hookspec.ExitUsage {
				t.Errorf("exit = %d, want %d", code, hookspec.ExitUsage)
			}
			if stderr == "" {
				t.Error("want a diagnostic on stderr, got none")
			}
			if !strings.Contains(stderr, usage) {
				t.Errorf("stderr = %q, want it to contain the usage line", stderr)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want diagnostics on stderr only", stdout)
			}
		})
	}
}

// The usage line is spelled out here rather than read from the constant, so a
// change to the constant's text fails this test.
func TestUsageLineIsExact(t *testing.T) {
	t.Setenv(printDigestEnv, "")
	const want = "usage: agx <provider> <hook> [hook args...] | agx flow <command> [args...] | agx help"

	code, _, stderr := invoke(t, "")
	if code != hookspec.ExitUsage {
		t.Errorf("exit = %d, want %d", code, hookspec.ExitUsage)
	}
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr, want)
	}
}

// "agx help" and its flag spellings print the command list and exit 0 without
// reading stdin. Only a lone argument is help: a second argument keeps the
// provider-validation result.
func TestHelpPrintsCommandList(t *testing.T) {
	t.Setenv(printDigestEnv, "")
	const want = `agx: the Agentics command-line tool

  agx <provider> <hook> [hook args...]   run a hook (called by the hook configuration)
  agx flow <command> [args...]           record or read flow state (log, status, where, bind, path, audit)
  agx help                               print this list

Run "agx flow --help" for the flow commands.
`
	for _, arg := range []string{"help", "--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{arg}, unreadFlowInput{t}, &stdout, &stderr)
			if code != 0 {
				t.Errorf("exit = %d, want 0", code)
			}
			if stdout.String() != want {
				t.Errorf("stdout = %q, want %q", stdout.String(), want)
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
	}

	t.Run("help with an argument", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"help", "x"}, unreadFlowInput{t}, &stdout, &stderr)
		if code != hookspec.ExitUsage || stdout.Len() != 0 ||
			!strings.HasPrefix(stderr.String(), `agx: unknown provider "help"`) {
			t.Fatalf("help x: exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})
}

// An unknown name must be reported with the roster so the operator can see what
// was expected.
func TestUnknownNamesListTheKnownRoster(t *testing.T) {
	known := anyProvider(t)

	code, stdout, stderr := invoke(t, "", "bogus-provider", hooks.Names()[0])
	for _, id := range surface.All() {
		if !strings.Contains(stderr, string(id)) {
			t.Errorf("unknown-provider diagnostic %q omits known provider %q", stderr, id)
		}
	}
	// The literal is the text a host user sees, so it is not built from the
	// roster: a change to the bytes must fail here.
	const wantUsage = "agx: unknown provider \"bogus-provider\" (known: antigravity, claude-code, codex, copilot)\n" +
		"usage: agx <provider> <hook> [hook args...] | agx flow <command> [args...] | agx help\n"
	if code != hookspec.ExitUsage || stdout != "" || stderr != wantUsage {
		t.Errorf("unknown provider: exit=%d stdout=%q stderr=%q, want exit %d, empty stdout, stderr %q",
			code, stdout, stderr, hookspec.ExitUsage, wantUsage)
	}

	_, _, stderr = invoke(t, "", string(known), "bogus-hook")
	for _, h := range hooks.Names() {
		if !strings.Contains(stderr, h) {
			t.Errorf("unknown-hook diagnostic %q omits known hook %q", stderr, h)
		}
	}
}

// The post-tool verb is gone: an installed config that still names it must fail
// as an unknown hook, never silently run a reporting check.
func TestRemovedPostToolVerbIsAnUnknownHook(t *testing.T) {
	for _, id := range surface.All() {
		t.Run(string(id), func(t *testing.T) {
			code, stdout, stderr := invoke(t, "{}", string(id), "post-tool")
			if code != hookspec.ExitUsage || stdout != "" || !strings.Contains(stderr, `unknown hook "post-tool"`) {
				t.Fatalf("exit=%d stdout=%q stderr=%q, want exit %d and an unknown hook usage message",
					code, stdout, stderr, hookspec.ExitUsage)
			}
		})
	}
}

// Dispatch rejects BEFORE the handler runs: a hook config typo stays a usage
// error (or the surface's fail-open dispatch rejection) even when the handler
// would have passed.
//
// TestExitDenyComesOnlyFromRender already covers these same argv shapes. This
// test is NOT a duplicate: it additionally pins the exact exit code and stdout
// shape (ExitUsage for pre-dispatch rejections and claude-code; ExitOK plus
// the surface's fail-open body for copilot, codex, and antigravity), which
// that test does not assert. Do not delete it as redundant.
func TestDispatchRejectsBeforeTheHandlerRuns(t *testing.T) {
	reached := false
	deps := productionHookDependencies()
	deps = stubLookup(deps, func(hookspec.Request) hookspec.Outcome {
		reached = true
		return contract.Pass()
	})
	defer func() {
		if reached {
			t.Error("the handler ran; the rejection must happen BEFORE it")
		}
	}()

	for _, argv := range [][]string{
		nil,
		{"bogus-provider", hooks.Names()[0]},
	} {
		code, stdout, stderr := invokeWithDependencies(t, deps, "", argv...)

		if code != hookspec.ExitUsage {
			t.Errorf("%v: exit = %d, want %d", argv, code, hookspec.ExitUsage)
		}
		if stdout != "" {
			t.Errorf("%v: stdout = %q, want empty", argv, stdout)
		}
		if stderr == "" {
			t.Errorf("%v: want a diagnostic on stderr, got none", argv)
		}
	}

	wantReject := map[surface.ID]struct {
		exit   int
		stdout string
	}{
		surface.ClaudeCode:  {exit: hookspec.ExitUsage, stdout: ""},
		surface.Copilot:     {exit: hookspec.ExitOK, stdout: ""},
		surface.Codex:       {exit: hookspec.ExitOK, stdout: "{}\n"},
		surface.Antigravity: {exit: hookspec.ExitOK, stdout: "{\"decision\":\"allow\"}\n"},
	}

	for _, id := range surface.All() {
		expected, known := wantReject[id]
		if !known {
			t.Fatalf("provider %q has no pinned unsupported-hook expectation", id)
		}
		argv := []string{string(id), "bogus-hook"}
		code, stdout, stderr := invokeWithDependencies(t, deps, "", argv...)

		if code != expected.exit {
			t.Errorf("%v: exit = %d, want %d (stderr: %q)", argv, code, expected.exit, stderr)
		}
		if stdout != expected.stdout {
			t.Errorf("%v: stdout = %q, want %q", argv, stdout, expected.stdout)
		}
		if stderr == "" {
			t.Errorf("%v: want a diagnostic on stderr, got none", argv)
		}
	}
}

// Normalize rejects BEFORE the handler runs: a malformed payload is still the
// surface's own rejection even when the handler would have passed.
//
// TestExitDenyComesOnlyFromRender already covers a malformed payload. This
// test is NOT a duplicate: it additionally pins the "<provider>: malformed "
// stderr prefix, which that test does not assert. Do not delete it as
// redundant.
func TestNormalizeRejectsBeforeTheHandlerRuns(t *testing.T) {
	reached := false
	deps := productionHookDependencies()
	deps = stubLookup(deps, func(hookspec.Request) hookspec.Outcome {
		reached = true
		return contract.Pass()
	})
	defer func() {
		if reached {
			t.Error("the handler ran; the rejection must happen BEFORE it")
		}
	}()

	for _, id := range surface.All() {
		p := string(id)
		code, _, stderr := invokeWithLegalArgsAndDependencies(t, deps, "not json", id, hooks.Names()[0])

		if !strings.HasPrefix(stderr, p+": malformed ") {
			t.Errorf("%s: stderr = %q, want the surface's malformed-payload rejection", p, stderr)
		}
		if p == "claude-code" && code != hookspec.ExitUsage {
			t.Errorf("%s: exit = %d, want %d", p, code, hookspec.ExitUsage)
		}
	}
}

// Exit codes cross a process boundary into provider hook configs; they are
// contract, not implementation detail.
func TestExitCodeContract(t *testing.T) {
	if hookspec.ExitOK != 0 || hookspec.ExitUsage != 1 || hookspec.ExitDeny != 2 {
		t.Errorf("exit codes = (%d, %d, %d), want (0, 1, 2)",
			hookspec.ExitOK, hookspec.ExitUsage, hookspec.ExitDeny)
	}
}

// The provenance print answers before dispatch — no provider, no hook, no
// stdin — so a human can ask any binary which source and which toolchain
// built it. The default digest value proves the var survives to runtime; the
// Makefile replaces it at link time. The toolchain line is never stamped: the
// linker records the version itself.
func TestSourceDigestEnvPrintsAndExitsZero(t *testing.T) {
	t.Setenv(printDigestEnv, "1")

	code, stdout, stderr := invoke(t, "")
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	want := sourceDigest + "\n" + toolchainPrefix + runtime.Version() + "\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

// The exact match is the discipline: anything but "1" — the empty string
// included — must leave a real hook invocation untouched.
func TestSourceDigestEnvIgnoresOtherValues(t *testing.T) {
	for _, value := range []string{"", "0", "true", " 1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(printDigestEnv, value)

			code, stdout, _ := invoke(t, "")
			if code == 0 || strings.HasPrefix(stdout, sourceDigest+"\n") {
				t.Errorf("%q triggered the digest print (exit=%d stdout=%q); only \"1\" may", value, code, stdout)
			}
		})
	}
}

type panicHookWriter struct{}

func (panicHookWriter) Write([]byte) (int, error) { panic("CQ22 prevalidation writer panic") }

func TestAPrevalidationPanicRemainsUsage(t *testing.T) {
	t.Setenv(printDigestEnv, "1")
	var stderr bytes.Buffer
	code := run(nil, strings.NewReader(""), panicHookWriter{}, &stderr)
	if code != hookspec.ExitUsage || !strings.Contains(stderr.String(), "CQ22 prevalidation writer panic") || !strings.Contains(stderr.String(), usage) {
		t.Fatalf("prevalidation panic: exit=%d stderr=%q", code, stderr.String())
	}
}
