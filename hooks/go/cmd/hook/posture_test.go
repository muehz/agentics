package main

import (
	"bytes"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hooks"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/provider"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// postureRunner is the entry point a posture row drives. The table takes it
// as a parameter so a replacement runner can be held to the same bytes.
type postureRunner func(deps hookDependencies, argv []string, stdin io.Reader, stdout, stderr io.Writer) int

// postureOracle is what a surface's host must make of a row's response,
// checked through hosttest.Read independently of the recorded bytes.
type postureOracle int

const (
	// postureOracleNone holds a row whose decision the bytes alone pin.
	postureOracleNone postureOracle = iota
	// postureOracleDeny requires the host to read a deny.
	postureOracleDeny
	// postureOracleErrorShape requires the surface's fault shape, which no
	// host reads as a deny: claude-code exit 1 with empty stdout, copilot exit
	// 0 with empty stdout, codex exit 0 with "{}", antigravity exit 0 with an
	// allow document.
	postureOracleErrorShape
	// postureOracleUsage requires the binary's own usage failure: exit 1,
	// empty stdout, and no deny on any surface.
	postureOracleUsage
)

// postureStages counts what one run reached: each replaceable stage's calls
// and whether stdin was read at all.
type postureStages struct {
	lookup, dispatch, normalize, render int32
	stdinRead                           bool
}

// postureWant is one surface's expected response. When stack is set, stderr
// holds the text before the first "\ngoroutine " line, and a goroutine dump
// must follow it.
type postureWant struct {
	exit           int
	stdout, stderr string
	stack          bool
}

// postureRow is one plan and fault kind, run on every surface.
type postureRow struct {
	plan, fault string
	// extraArgs follow the plan's fixed argv.
	extraArgs []string
	// stdin builds the payload; nil sends the surface's safe shell call.
	stdin func(t *testing.T, id surface.ID) io.Reader
	// stub replaces the stages the fault needs on a production dependency value.
	stub   func(t *testing.T, id surface.ID, deps *hookDependencies)
	oracle postureOracle
	stages postureStages
}

func (r postureRow) key() string { return r.plan + "/" + r.fault }

// postureChatter is the diagnostic text a misbehaving check writes.
const postureChatter = "chatter\n"

func postureHandler(outcome hookspec.Outcome, chatter bool) hookspec.Handler {
	return func(req hookspec.Request) hookspec.Outcome {
		if chatter {
			_, _ = req.Diag.Write([]byte(postureChatter))
		}
		return outcome
	}
}

func posturePanicHandler(hookspec.Request) hookspec.Outcome { panic("handler boom") }

// postureLookupByName resolves each check name through its own handler and
// misses every other name.
func postureLookupByName(handlers map[string]hookspec.Handler) func(string) (hookspec.Handler, bool) {
	return func(name string) (hookspec.Handler, bool) {
		handler, ok := handlers[name]
		return handler, ok
	}
}

// postureOtherSurface is a surface other than id, so an event built for it
// mismatches the invocation.
func postureOtherSurface(id surface.ID) surface.ID {
	all := surface.All()
	for i, candidate := range all {
		if candidate == id {
			return all[(i+1)%len(all)]
		}
	}
	return all[0]
}

func postureStub(stub func(deps *hookDependencies)) func(*testing.T, surface.ID, *hookDependencies) {
	return func(_ *testing.T, _ surface.ID, deps *hookDependencies) { stub(deps) }
}

func postureStubHandler(handler hookspec.Handler) func(*testing.T, surface.ID, *hookDependencies) {
	return postureStub(func(deps *hookDependencies) { *deps = stubLookup(*deps, handler) })
}

func postureUnreadable(*testing.T, surface.ID) io.Reader { return eventErrorReader{} }

func postureMalformed(*testing.T, surface.ID) io.Reader { return strings.NewReader("{") }

var posturePass = postureHandler(contract.Pass(), false)

// legacyPostureRows run "<surface> command-guardian --mode block".
var legacyPostureRows = []postureRow{
	{plan: "legacy", fault: "unreadable-stdin", stdin: postureUnreadable, stub: postureStubHandler(posturePass),
		oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1, render: 1, stdinRead: true}},
	{plan: "legacy", fault: "malformed-payload", stdin: postureMalformed, stub: postureStubHandler(posturePass),
		oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1, normalize: 1, stdinRead: true}},
	{plan: "legacy", fault: "bad-argv", extraArgs: []string{"private-argument"}, stub: postureStubHandler(posturePass),
		oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1}},
	{plan: "legacy", fault: "unknown-hook", stub: postureStub(func(deps *hookDependencies) {
		deps.lookup = func(string) (hookspec.Handler, bool) { return nil, false }
	}), oracle: postureOracleUsage, stages: postureStages{lookup: 1}},
	{plan: "legacy", fault: "lookup-panic", stub: postureStub(func(deps *hookDependencies) {
		deps.lookup = func(string) (hookspec.Handler, bool) { panic("lookup boom") }
	}), oracle: postureOracleErrorShape, stages: postureStages{lookup: 1}},
	{plan: "legacy", fault: "dispatch-panic", stub: postureStub(func(deps *hookDependencies) {
		*deps = stubLookup(*deps, posturePass)
		deps.dispatch = func(string, string, []string) (string, *provider.Rendered) { panic("dispatch boom") }
	}), oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1}},
	{plan: "legacy", fault: "normalize-panic", stub: postureStub(func(deps *hookDependencies) {
		*deps = stubLookup(*deps, posturePass)
		deps.normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) { panic("normalize boom") }
	}), oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1, normalize: 1, stdinRead: true}},
	{plan: "legacy", fault: "render-panic", stub: postureStub(func(deps *hookDependencies) {
		*deps = stubLookup(*deps, posturePass)
		deps.render = func(string, string, hookspec.Outcome, []byte) provider.Rendered { panic("render boom") }
	}), oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "legacy", fault: "handler-panic", stub: postureStubHandler(posturePanicHandler),
		oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "legacy", fault: "invalid-outcome-with-chatter", stub: postureStubHandler(postureHandler(hookspec.Outcome{}, true)),
		oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "legacy", fault: "error-with-chatter", stub: postureStubHandler(postureHandler(contract.Error("check error"), true)),
		oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "legacy", fault: "context", stub: postureStubHandler(postureHandler(contract.Context("context note"), false)),
		stages: postureStages{lookup: 1, dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "legacy", fault: "pass-with-chatter", stub: postureStubHandler(postureHandler(contract.Pass(), true)),
		stages: postureStages{lookup: 1, dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "legacy", fault: "deny", stub: postureStubHandler(postureHandler(contract.Deny("denied by stub"), false)),
		oracle: postureOracleDeny, stages: postureStages{lookup: 1, dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
}

// eventChecked is the stage count of an event row whose two checks both run.
var eventChecked = postureStages{lookup: 2, dispatch: 1, normalize: 1, render: 1, stdinRead: true}

// eventPostureRows run "<surface> pre-tool" on a Bash call, which selects the
// commands check and then secrets-guardian on every surface.
var eventPostureRows = []postureRow{
	{plan: "event", fault: "bad-argv", extraArgs: []string{"private-argument"}, stub: postureStubHandler(posturePass),
		oracle: postureOracleDeny, stages: postureStages{dispatch: 1, render: 1}},
	{plan: "event", fault: "unreadable-stdin", stdin: postureUnreadable, stub: postureStubHandler(posturePass),
		oracle: postureOracleDeny, stages: postureStages{dispatch: 1, render: 1, stdinRead: true}},
	{plan: "event", fault: "malformed-payload", stdin: postureMalformed, stub: postureStubHandler(posturePass),
		oracle: postureOracleDeny, stages: postureStages{dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "event", fault: "invalid-normalized-event", stub: postureStub(func(deps *hookDependencies) {
		*deps = stubLookup(*deps, posturePass)
		deps.normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) { return hookspec.Event{}, nil }
	}), oracle: postureOracleDeny, stages: postureStages{dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "event", fault: "surface-mismatch", stub: func(t *testing.T, id surface.ID, deps *hookDependencies) {
		event, err := contract.NewEvent(contract.EventParts{
			Surface: postureOtherSurface(id), ToolName: "Bash", ToolInput: map[string]any{"command": "echo safe"},
		})
		if err != nil {
			t.Fatal(err)
		}
		*deps = stubLookup(*deps, posturePass)
		deps.normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) { return event, nil }
	}, oracle: postureOracleDeny, stages: postureStages{dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "event", fault: "dispatch-panic", stub: postureStub(func(deps *hookDependencies) {
		*deps = stubLookup(*deps, posturePass)
		deps.dispatch = func(string, string, []string) (string, *provider.Rendered) { panic("dispatch boom") }
	}), oracle: postureOracleDeny, stages: postureStages{dispatch: 1}},
	{plan: "event", fault: "dispatch-wrong-event", stub: postureStub(func(deps *hookDependencies) {
		*deps = stubLookup(*deps, posturePass)
		deps.dispatch = func(string, string, []string) (string, *provider.Rendered) { return "wrong", nil }
	}), oracle: postureOracleDeny, stages: postureStages{dispatch: 1, render: 1}},
	{plan: "event", fault: "normalize-panic", stub: postureStub(func(deps *hookDependencies) {
		*deps = stubLookup(*deps, posturePass)
		deps.normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) { panic("normalize boom") }
	}), oracle: postureOracleDeny, stages: postureStages{dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "event", fault: "render-panic", stub: postureStub(func(deps *hookDependencies) {
		*deps = stubLookup(*deps, posturePass)
		deps.render = func(string, string, hookspec.Outcome, []byte) provider.Rendered { panic("render boom") }
	}), oracle: postureOracleDeny, stages: eventChecked},
	// The production lookup resolves every name but the one that misses, so
	// the commands check falls back to its own handler and only
	// secrets-guardian is unavailable.
	{plan: "event", fault: "missing-handler", stub: postureStub(func(deps *hookDependencies) {
		deps.lookup = func(name string) (hookspec.Handler, bool) {
			if name == "secrets-guardian" {
				return nil, false
			}
			return hooks.Lookup(name)
		}
	}), oracle: postureOracleDeny,
		// The commands check looks up its two steps after a miss, so this row
		// counts four lookups.
		stages: postureStages{lookup: 4, dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "event", fault: "lookup-panic", stub: postureStub(func(deps *hookDependencies) {
		deps.lookup = func(string) (hookspec.Handler, bool) { panic("lookup boom") }
	}), oracle: postureOracleDeny, stages: eventChecked},
	{plan: "event", fault: "handler-panic", stub: postureStubHandler(posturePanicHandler),
		oracle: postureOracleDeny, stages: eventChecked},
	{plan: "event", fault: "invalid-outcome-with-chatter", stub: postureStubHandler(postureHandler(hookspec.Outcome{}, true)),
		oracle: postureOracleDeny, stages: eventChecked},
	{plan: "event", fault: "guard-error-with-chatter", stub: postureStubHandler(postureHandler(contract.Error("check error"), true)),
		oracle: postureOracleDeny, stages: eventChecked},
	{plan: "event", fault: "guard-empty-error", stub: postureStubHandler(postureHandler(contract.Error(""), false)),
		oracle: postureOracleDeny, stages: eventChecked},
	{plan: "event", fault: "context", stub: postureStubHandler(postureHandler(contract.Context("context note"), false)),
		stages: eventChecked},
	{plan: "event", fault: "deny-and-context", stub: postureStub(func(deps *hookDependencies) {
		deps.lookup = postureLookupByName(map[string]hookspec.Handler{
			"commands":         postureHandler(contract.Deny("denied by stub"), false),
			"secrets-guardian": postureHandler(contract.Context("context note"), false),
		})
	}), oracle: postureOracleDeny, stages: eventChecked},
	{plan: "event", fault: "pass-with-chatter", stub: postureStubHandler(postureHandler(contract.Pass(), true)),
		stages: eventChecked},
}

// postureReader records whether the runner read stdin at all.
type postureReader struct {
	inner io.Reader
	read  atomic.Bool
}

func (r *postureReader) Read(p []byte) (int, error) {
	r.read.Store(true)
	return r.inner.Read(p)
}

// postureCounter counts each replaceable stage's calls in one run. The
// counters are atomic because the grouped event calls stages from its check
// workers.
type postureCounter struct {
	lookup, dispatch, normalize, render atomic.Int32
}

// postureInvocation is one run's whole input: counted stages, argv and a
// reader that records whether it was read. Each run builds its own, so two
// runners driven with the same input share no state.
type postureInvocation struct {
	deps   hookDependencies
	argv   []string
	stdin  *postureReader
	counts *postureCounter
}

// newCountedInvocation wraps deps' four stages in counters and stdin in a
// reader that records whether the runner read it.
func newCountedInvocation(deps hookDependencies, argv []string, stdin io.Reader) postureInvocation {
	counts := &postureCounter{}
	lookup, dispatch, normalize, render := deps.lookup, deps.dispatch, deps.normalize, deps.render
	deps.lookup = func(name string) (hookspec.Handler, bool) {
		counts.lookup.Add(1)
		return lookup(name)
	}
	deps.dispatch = func(s, hook string, args []string) (string, *provider.Rendered) {
		counts.dispatch.Add(1)
		return dispatch(s, hook, args)
	}
	deps.normalize = func(s, event string, raw []byte) (hookspec.Event, *provider.Rendered) {
		counts.normalize.Add(1)
		return normalize(s, event, raw)
	}
	deps.render = func(s, event string, outcome hookspec.Outcome, diag []byte) provider.Rendered {
		counts.render.Add(1)
		return render(s, event, outcome, diag)
	}
	return postureInvocation{deps: deps, argv: argv, stdin: &postureReader{inner: stdin}, counts: counts}
}

// stages reports what the run reached.
func (inv postureInvocation) stages() postureStages {
	return postureStages{
		lookup: inv.counts.lookup.Load(), dispatch: inv.counts.dispatch.Load(),
		normalize: inv.counts.normalize.Load(), render: inv.counts.render.Load(),
		stdinRead: inv.stdin.read.Load(),
	}
}

// newPostureInvocation builds one fresh run of row on id: production stages
// with the row's stubs applied, the plan's argv, and the row's stdin. shell
// is the payload sent when the row names no stdin; the caller builds it once
// so every run of the row reads the same bytes.
func newPostureInvocation(t *testing.T, row postureRow, id surface.ID, shell []byte) postureInvocation {
	t.Helper()
	deps := productionHookDependencies()
	if row.stub != nil {
		row.stub(t, id, &deps)
	}
	argv := []string{string(id), "pre-tool"}
	if row.plan == "legacy" {
		argv = []string{string(id), "command-guardian", "--mode", "block"}
	}
	argv = append(argv, row.extraArgs...)
	var payload io.Reader
	if row.stdin != nil {
		payload = row.stdin(t, id)
	} else {
		payload = bytes.NewReader(shell)
	}
	return newCountedInvocation(deps, argv, payload)
}

// postureShellStdin is the payload a row without its own stdin sends, or nil
// when the row builds its own.
func postureShellStdin(t *testing.T, row postureRow, id surface.ID) []byte {
	t.Helper()
	if row.stdin != nil {
		return nil
	}
	return safeShellStdin(t, id)
}

// postureRun runs one row on one surface and returns the response and the
// stages it reached.
func postureRun(t *testing.T, runner postureRunner, row postureRow, id surface.ID) (exit int, stdout, stderr string, stages postureStages) {
	t.Helper()
	inv := newPostureInvocation(t, row, id, postureShellStdin(t, row, id))
	var out, errout bytes.Buffer
	exit = runner(inv.deps, inv.argv, inv.stdin, &out, &errout)
	return exit, out.String(), errout.String(), inv.stages()
}

// postureStackMarker starts the goroutine dump debug.Stack appends.
const postureStackMarker = "\ngoroutine "

// postureErrorExit is the exit code of each surface's fault shape; its stdout
// is the surface's quiet body.
func postureErrorExit(id surface.ID) int {
	if id == surface.ClaudeCode {
		return 1
	}
	return 0
}

// requirePostureOracle checks a response against what the row's host must
// make of it, so a recorded expectation cannot pin a fail-open posture.
func requirePostureOracle(t *testing.T, row postureRow, id surface.ID, exit int, stdout, stderr string) {
	t.Helper()
	verdict := hosttest.Read(id, exit, []byte(stdout), []byte(stderr))
	switch row.oracle {
	case postureOracleDeny:
		if verdict.Decision != hosttest.Deny {
			t.Errorf("host reads %v, want Deny", verdict.Decision)
		}
	case postureOracleErrorShape:
		if verdict.Decision == hosttest.Deny || exit != postureErrorExit(id) || stdout != quietBody(id) {
			t.Errorf("host reads %v with exit=%d stdout=%q, want the surface's fault shape", verdict.Decision, exit, stdout)
		}
	case postureOracleUsage:
		if verdict.Decision == hosttest.Deny || exit != hookspec.ExitUsage || stdout != "" {
			t.Errorf("host reads %v with exit=%d stdout=%q, want a usage failure", verdict.Decision, exit, stdout)
		}
	}
}

// runPostureTable drives every row on every surface through runner.
func runPostureTable(t *testing.T, runner postureRunner, rows []postureRow, wants map[string]map[surface.ID]postureWant) {
	t.Helper()
	if len(wants) != len(rows) {
		t.Fatalf("%d rows hold %d expectations; every row needs exactly one", len(rows), len(wants))
	}
	for _, row := range rows {
		for _, id := range surface.All() {
			t.Run(row.plan+"/"+string(id)+"/"+row.fault, func(t *testing.T) {
				exit, stdout, stderr, stages := postureRun(t, runner, row, id)
				want, ok := wants[row.key()][id]
				if !ok {
					t.Fatalf("no expectation recorded for %s on %s", row.key(), id)
				}
				if stages != row.stages {
					t.Errorf("stages = %+v, want %+v", stages, row.stages)
				}
				if exit != want.exit || stdout != want.stdout {
					t.Errorf("exit=%d stdout=%q, want exit=%d stdout=%q", exit, stdout, want.exit, want.stdout)
				}
				if want.stack {
					prefix, rest, found := strings.Cut(stderr, postureStackMarker)
					if !found || prefix != want.stderr || !strings.Contains(rest, "runtime/debug.Stack(") {
						t.Errorf("stderr = %q, want %q followed by a goroutine dump", stderr, want.stderr)
					}
				} else if stderr != want.stderr {
					t.Errorf("stderr = %q, want %q", stderr, want.stderr)
				}
				requirePostureOracle(t, row, id, exit, stdout, stderr)
			})
		}
	}
}

// TestRunnerPostureTable pins every fault posture of the two plans the
// entry point runs, the legacy single check ("<surface> command-guardian
// --mode block") and the grouped event ("<surface> pre-tool"), on every
// surface. The rows hold the eleven differences between the two plans:
//
//  1. A fault before any check (bad-argv, unreadable-stdin,
//     malformed-payload) keeps the surface's error shape on the legacy plan
//     and denies on the event plan.
//  2. A pipeline panic (dispatch, normalize, render, lookup) shows the panic
//     value and a stack on the legacy plan and fixed text on the event plan.
//  3. A handler panic shows a stack on the legacy plan and "check panicked"
//     on the event plan.
//  4. A guard's error outcome keeps the error shape on the legacy plan
//     (error-with-chatter) and denies on the event plan, with or without a
//     message (guard-error-with-chatter, guard-empty-error).
//  5. An invalid outcome drops the check's chatter on the legacy plan and
//     keeps it, labelled, on the event plan.
//  6. Only the event plan labels each message "[<check>] " and joins them
//     with a blank line.
//  7. A lookup miss is a usage error on the legacy plan (unknown-hook) and a
//     deny naming the unavailable check on the event plan (missing-handler).
//  8. Only the event plan cross-checks the stages: dispatch-wrong-event,
//     surface-mismatch, and invalid-normalized-event.
//  9. Only the event plan gives each check a budget and its own worker. The
//     deadline and cleanup postures take the full check budget at this seam,
//     so this table stubs them out and the check runner's isolation tests
//     pins them.
//  10. Grouped context reaches stderr on copilot only.
//  11. A NUL in the event is denied by the grouped event even with every skip
//     switch set, while a single check's skip switch wins:
//     TestEventBoundaryGroupedNULDeniesDespiteSkips and
//     TestEventBoundarySkipPrecedesNULValidation hold it.
func TestRunnerPostureTable(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	runPostureTable(t, runWithDependencies, postureRows(), postureWants)
}

// postureRows is every row of both plans, legacy first.
func postureRows() []postureRow {
	return append(append([]postureRow(nil), legacyPostureRows...), eventPostureRows...)
}

// postureWants holds each row's response per surface, recorded from the
// runner and read against the oracle each row names.
var postureWants = map[string]map[surface.ID]postureWant{
	"legacy/unreadable-stdin": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: "unable to read the hook payload: private input error\n"},
		surface.ClaudeCode:  {exit: 1, stdout: "", stderr: "unable to read the hook payload: private input error\n"},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: "unable to read the hook payload: private input error\n"},
		surface.Copilot:     {exit: 0, stdout: "", stderr: "unable to read the hook payload: private input error\n"},
	},
	"legacy/malformed-payload": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: "antigravity: malformed PreToolUse payload: payload is not valid JSON: unexpected EOF\n"},
		surface.ClaudeCode:  {exit: 1, stdout: "", stderr: "claude-code: malformed PreToolUse payload: payload is not valid JSON: unexpected EOF\n"},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: "codex: malformed PreToolUse payload: payload is not valid JSON: unexpected EOF\n"},
		surface.Copilot:     {exit: 0, stdout: "", stderr: "copilot: malformed preToolUse payload: payload is not valid JSON: unexpected EOF\n"},
	},
	"legacy/bad-argv": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: "antigravity: command-guardian: unexpected argument: private-argument\n"},
		surface.ClaudeCode:  {exit: 1, stdout: "", stderr: "claude-code: command-guardian: unexpected argument: private-argument\n"},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: "codex: command-guardian: unexpected argument: private-argument\n"},
		surface.Copilot:     {exit: 0, stdout: "", stderr: "copilot: command-guardian: unexpected argument: private-argument\n"},
	},
	"legacy/unknown-hook": {
		surface.Antigravity: {exit: 1, stdout: "", stderr: "agx: unknown hook \"command-guardian\" (known: command-guardian, release-guardian, secrets-guardian)\nusage: agx <provider> <hook> [hook args...] | agx flow <command> [args...] | agx help\n"},
		surface.ClaudeCode:  {exit: 1, stdout: "", stderr: "agx: unknown hook \"command-guardian\" (known: command-guardian, release-guardian, secrets-guardian)\nusage: agx <provider> <hook> [hook args...] | agx flow <command> [args...] | agx help\n"},
		surface.Codex:       {exit: 1, stdout: "", stderr: "agx: unknown hook \"command-guardian\" (known: command-guardian, release-guardian, secrets-guardian)\nusage: agx <provider> <hook> [hook args...] | agx flow <command> [args...] | agx help\n"},
		surface.Copilot:     {exit: 1, stdout: "", stderr: "agx: unknown hook \"command-guardian\" (known: command-guardian, release-guardian, secrets-guardian)\nusage: agx <provider> <hook> [hook args...] | agx flow <command> [args...] | agx help\n"},
	},
	"legacy/lookup-panic": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: "agx: internal error: lookup boom", stack: true},
		surface.ClaudeCode:  {exit: 1, stdout: "", stderr: "agx: internal error: lookup boom", stack: true},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: "agx: internal error: lookup boom", stack: true},
		surface.Copilot:     {exit: 0, stdout: "", stderr: "agx: internal error: lookup boom", stack: true},
	},
	"legacy/dispatch-panic": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: "agx: internal error: dispatch boom", stack: true},
		surface.ClaudeCode:  {exit: 1, stdout: "", stderr: "agx: internal error: dispatch boom", stack: true},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: "agx: internal error: dispatch boom", stack: true},
		surface.Copilot:     {exit: 0, stdout: "", stderr: "agx: internal error: dispatch boom", stack: true},
	},
	"legacy/normalize-panic": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: "agx: internal error: normalize boom", stack: true},
		surface.ClaudeCode:  {exit: 1, stdout: "", stderr: "agx: internal error: normalize boom", stack: true},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: "agx: internal error: normalize boom", stack: true},
		surface.Copilot:     {exit: 0, stdout: "", stderr: "agx: internal error: normalize boom", stack: true},
	},
	"legacy/render-panic": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: "agx: internal error: render boom", stack: true},
		surface.ClaudeCode:  {exit: 1, stdout: "", stderr: "agx: internal error: render boom", stack: true},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: "agx: internal error: render boom", stack: true},
		surface.Copilot:     {exit: 0, stdout: "", stderr: "agx: internal error: render boom", stack: true},
	},
	"legacy/handler-panic": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: "agx: internal error in command-guardian hook: handler boom", stack: true},
		surface.ClaudeCode:  {exit: 1, stdout: "", stderr: "agx: internal error in command-guardian hook: handler boom", stack: true},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: "agx: internal error in command-guardian hook: handler boom", stack: true},
		surface.Copilot:     {exit: 0, stdout: "", stderr: "agx: internal error in command-guardian hook: handler boom", stack: true},
	},
	"legacy/invalid-outcome-with-chatter": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: "core produced an invalid neutral outcome\n"},
		surface.ClaudeCode:  {exit: 1, stdout: "", stderr: "core produced an invalid neutral outcome\n"},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: "core produced an invalid neutral outcome\n"},
		surface.Copilot:     {exit: 0, stdout: "", stderr: "core produced an invalid neutral outcome\n"},
	},
	"legacy/error-with-chatter": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: "chatter\n"},
		surface.ClaudeCode:  {exit: 1, stdout: "", stderr: "chatter\n"},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: "chatter\n"},
		surface.Copilot:     {exit: 0, stdout: "", stderr: "chatter\n"},
	},
	"legacy/context": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"additionalContext\":\"context note\"}}\n", stderr: ""},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"additionalContext\":\"context note\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "", stderr: ""},
	},
	"legacy/pass-with-chatter": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 0, stdout: "", stderr: ""},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "", stderr: ""},
	},
	"legacy/deny": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"denied by stub\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "denied by stub\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"denied by stub\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"denied by stub\"}\n", stderr: ""},
	},
	"event/bad-argv": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"agx: grouped event dispatch rejected the invocation\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "agx: grouped event dispatch rejected the invocation\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: grouped event dispatch rejected the invocation\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: grouped event dispatch rejected the invocation\"}\n", stderr: ""},
	},
	"event/unreadable-stdin": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"agx: unable to read the bounded event payload\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "agx: unable to read the bounded event payload\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: unable to read the bounded event payload\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: unable to read the bounded event payload\"}\n", stderr: ""},
	},
	"event/malformed-payload": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"agx: malformed or mismatched event payload\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "agx: malformed or mismatched event payload\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: malformed or mismatched event payload\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: malformed or mismatched event payload\"}\n", stderr: ""},
	},
	"event/invalid-normalized-event": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"agx: normalized event fields are invalid\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "agx: normalized event fields are invalid\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: normalized event fields are invalid\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: normalized event fields are invalid\"}\n", stderr: ""},
	},
	"event/surface-mismatch": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"agx: event selection failed\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "agx: event selection failed\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: event selection failed\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: event selection failed\"}\n", stderr: ""},
	},
	"event/dispatch-panic": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"agx: grouped event infrastructure panicked\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "agx: grouped event infrastructure panicked\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: grouped event infrastructure panicked\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: grouped event infrastructure panicked\"}\n", stderr: ""},
	},
	"event/dispatch-wrong-event": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"agx: grouped event dispatch rejected the invocation\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "agx: grouped event dispatch rejected the invocation\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: grouped event dispatch rejected the invocation\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: grouped event dispatch rejected the invocation\"}\n", stderr: ""},
	},
	"event/normalize-panic": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"agx: event input normalization panicked\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "agx: event input normalization panicked\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: event input normalization panicked\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: event input normalization panicked\"}\n", stderr: ""},
	},
	"event/render-panic": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"agx: grouped event infrastructure panicked\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "agx: grouped event infrastructure panicked\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: grouped event infrastructure panicked\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: grouped event infrastructure panicked\"}\n", stderr: ""},
	},
	"event/missing-handler": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"[secrets-guardian] registered check is unavailable\\n\\n[secrets-guardian] missing handler\"}\n", stderr: "[secrets-guardian] registered check is unavailable\n\n[secrets-guardian] missing handler\n"},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "[secrets-guardian] registered check is unavailable\n\n[secrets-guardian] missing handler\n[secrets-guardian] registered check is unavailable\n\n[secrets-guardian] missing handler\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[secrets-guardian] registered check is unavailable\\n\\n[secrets-guardian] missing handler\"}}\n", stderr: "[secrets-guardian] registered check is unavailable\n\n[secrets-guardian] missing handler\n"},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[secrets-guardian] registered check is unavailable\\n\\n[secrets-guardian] missing handler\"}\n", stderr: "[secrets-guardian] registered check is unavailable\n\n[secrets-guardian] missing handler\n"},
	},
	"event/lookup-panic": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"[commands] check infrastructure panicked\\n\\n[commands] worker infrastructure panic\\n\\n[secrets-guardian] check infrastructure panicked\\n\\n[secrets-guardian] worker infrastructure panic\"}\n", stderr: "[commands] check infrastructure panicked\n\n[commands] worker infrastructure panic\n\n[secrets-guardian] check infrastructure panicked\n\n[secrets-guardian] worker infrastructure panic\n"},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "[commands] check infrastructure panicked\n\n[commands] worker infrastructure panic\n\n[secrets-guardian] check infrastructure panicked\n\n[secrets-guardian] worker infrastructure panic\n[commands] check infrastructure panicked\n\n[commands] worker infrastructure panic\n\n[secrets-guardian] check infrastructure panicked\n\n[secrets-guardian] worker infrastructure panic\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] check infrastructure panicked\\n\\n[commands] worker infrastructure panic\\n\\n[secrets-guardian] check infrastructure panicked\\n\\n[secrets-guardian] worker infrastructure panic\"}}\n", stderr: "[commands] check infrastructure panicked\n\n[commands] worker infrastructure panic\n\n[secrets-guardian] check infrastructure panicked\n\n[secrets-guardian] worker infrastructure panic\n"},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] check infrastructure panicked\\n\\n[commands] worker infrastructure panic\\n\\n[secrets-guardian] check infrastructure panicked\\n\\n[secrets-guardian] worker infrastructure panic\"}\n", stderr: "[commands] check infrastructure panicked\n\n[commands] worker infrastructure panic\n\n[secrets-guardian] check infrastructure panicked\n\n[secrets-guardian] worker infrastructure panic\n"},
	},
	"event/handler-panic": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"[commands] check panicked\\n\\n[commands] panic\\n\\n[secrets-guardian] check panicked\\n\\n[secrets-guardian] panic\"}\n", stderr: "[commands] check panicked\n\n[commands] panic\n\n[secrets-guardian] check panicked\n\n[secrets-guardian] panic\n"},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "[commands] check panicked\n\n[commands] panic\n\n[secrets-guardian] check panicked\n\n[secrets-guardian] panic\n[commands] check panicked\n\n[commands] panic\n\n[secrets-guardian] check panicked\n\n[secrets-guardian] panic\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] check panicked\\n\\n[commands] panic\\n\\n[secrets-guardian] check panicked\\n\\n[secrets-guardian] panic\"}}\n", stderr: "[commands] check panicked\n\n[commands] panic\n\n[secrets-guardian] check panicked\n\n[secrets-guardian] panic\n"},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] check panicked\\n\\n[commands] panic\\n\\n[secrets-guardian] check panicked\\n\\n[secrets-guardian] panic\"}\n", stderr: "[commands] check panicked\n\n[commands] panic\n\n[secrets-guardian] check panicked\n\n[secrets-guardian] panic\n"},
	},
	"event/invalid-outcome-with-chatter": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"[commands] core produced an invalid neutral outcome\\n\\n[commands] invalid outcome\\n\\n[secrets-guardian] core produced an invalid neutral outcome\\n\\n[secrets-guardian] invalid outcome\"}\n", stderr: "[commands] core produced an invalid neutral outcome\n\n[commands] invalid outcome\n\n[commands] chatter\n\n\n[secrets-guardian] core produced an invalid neutral outcome\n\n[secrets-guardian] invalid outcome\n\n[secrets-guardian] chatter\n"},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "[commands] core produced an invalid neutral outcome\n\n[commands] invalid outcome\n\n[secrets-guardian] core produced an invalid neutral outcome\n\n[secrets-guardian] invalid outcome\n[commands] core produced an invalid neutral outcome\n\n[commands] invalid outcome\n\n[commands] chatter\n\n\n[secrets-guardian] core produced an invalid neutral outcome\n\n[secrets-guardian] invalid outcome\n\n[secrets-guardian] chatter\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] core produced an invalid neutral outcome\\n\\n[commands] invalid outcome\\n\\n[secrets-guardian] core produced an invalid neutral outcome\\n\\n[secrets-guardian] invalid outcome\"}}\n", stderr: "[commands] core produced an invalid neutral outcome\n\n[commands] invalid outcome\n\n[commands] chatter\n\n\n[secrets-guardian] core produced an invalid neutral outcome\n\n[secrets-guardian] invalid outcome\n\n[secrets-guardian] chatter\n"},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] core produced an invalid neutral outcome\\n\\n[commands] invalid outcome\\n\\n[secrets-guardian] core produced an invalid neutral outcome\\n\\n[secrets-guardian] invalid outcome\"}\n", stderr: "[commands] core produced an invalid neutral outcome\n\n[commands] invalid outcome\n\n[commands] chatter\n\n\n[secrets-guardian] core produced an invalid neutral outcome\n\n[secrets-guardian] invalid outcome\n\n[secrets-guardian] chatter\n"},
	},
	"event/guard-error-with-chatter": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"[commands] check error\\n\\n[commands] check failed\\n\\n[secrets-guardian] check error\\n\\n[secrets-guardian] check failed\"}\n", stderr: "[commands] check error\n\n[commands] check failed\n\n[commands] chatter\n\n\n[secrets-guardian] check error\n\n[secrets-guardian] check failed\n\n[secrets-guardian] chatter\n"},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "[commands] check error\n\n[commands] check failed\n\n[secrets-guardian] check error\n\n[secrets-guardian] check failed\n[commands] check error\n\n[commands] check failed\n\n[commands] chatter\n\n\n[secrets-guardian] check error\n\n[secrets-guardian] check failed\n\n[secrets-guardian] chatter\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] check error\\n\\n[commands] check failed\\n\\n[secrets-guardian] check error\\n\\n[secrets-guardian] check failed\"}}\n", stderr: "[commands] check error\n\n[commands] check failed\n\n[commands] chatter\n\n\n[secrets-guardian] check error\n\n[secrets-guardian] check failed\n\n[secrets-guardian] chatter\n"},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] check error\\n\\n[commands] check failed\\n\\n[secrets-guardian] check error\\n\\n[secrets-guardian] check failed\"}\n", stderr: "[commands] check error\n\n[commands] check failed\n\n[commands] chatter\n\n\n[secrets-guardian] check error\n\n[secrets-guardian] check failed\n\n[secrets-guardian] chatter\n"},
	},
	"event/guard-empty-error": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"[commands] check failed\\n\\n[secrets-guardian] check failed\"}\n", stderr: "[commands] check failed\n\n[secrets-guardian] check failed\n"},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "[commands] check failed\n\n[secrets-guardian] check failed\n[commands] check failed\n\n[secrets-guardian] check failed\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] check failed\\n\\n[secrets-guardian] check failed\"}}\n", stderr: "[commands] check failed\n\n[secrets-guardian] check failed\n"},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] check failed\\n\\n[secrets-guardian] check failed\"}\n", stderr: "[commands] check failed\n\n[secrets-guardian] check failed\n"},
	},
	"event/context": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"additionalContext\":\"[commands] context note\\n\\n[secrets-guardian] context note\"}}\n", stderr: ""},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"additionalContext\":\"[commands] context note\\n\\n[secrets-guardian] context note\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "", stderr: "[commands] context note\n\n[secrets-guardian] context note\n"},
	},
	"event/deny-and-context": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"[commands] denied by stub\\n\\n[secrets-guardian] context note\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "[commands] denied by stub\n\n[secrets-guardian] context note\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] denied by stub\\n\\n[secrets-guardian] context note\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] denied by stub\\n\\n[secrets-guardian] context note\"}\n", stderr: ""},
	},
	"event/pass-with-chatter": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 0, stdout: "", stderr: ""},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "", stderr: ""},
	},
}
