package checkrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hooks"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/provider"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// postureOracle is what a surface's host must make of a row's response,
// checked through hosttest.Read independently of the recorded bytes.
type postureOracle int

const (
	// postureOracleNone holds a row whose decision the bytes alone pin.
	postureOracleNone postureOracle = iota
	// postureOracleDeny requires the host to read a deny.
	postureOracleDeny
	// postureOracleErrorShape requires the surface's fault shape, which no
	// host reads as a deny.
	postureOracleErrorShape
	// postureOracleUsage requires the entry point's usage failure.
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
	extraArgs   []string
	// stdin builds the payload; nil sends the surface's safe shell call.
	stdin func(t *testing.T, id surface.ID) io.Reader
	// stub replaces the stages the fault needs on a production value.
	stub func(t *testing.T, id surface.ID, stages *Stages)
	// tune shortens the posture's budgets; nil keeps production's.
	tune func(p *posture)
	// parent is the invocation's parent context; nil is a background one.
	parent func() context.Context
	// settle waits for work the reply does not wait for before the stages
	// are counted.
	settle func(t *testing.T, stdin io.Reader)
	oracle postureOracle
	stages postureStages
}

func (r postureRow) key() string { return r.plan + "/" + r.fault }

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

func postureLookupByName(handlers map[string]hookspec.Handler) func(string) (hookspec.Handler, bool) {
	return func(name string) (hookspec.Handler, bool) {
		handler, ok := handlers[name]
		return handler, ok
	}
}

func postureOtherSurface(id surface.ID) surface.ID {
	all := surface.All()
	for i, candidate := range all {
		if candidate == id {
			return all[(i+1)%len(all)]
		}
	}
	return all[0]
}

func postureStub(stub func(stages *Stages)) func(*testing.T, surface.ID, *Stages) {
	return func(_ *testing.T, _ surface.ID, stages *Stages) { stub(stages) }
}

// postureStubHandler resolves every name to handler.
func postureStubHandler(handler hookspec.Handler) func(*testing.T, surface.ID, *Stages) {
	return postureStub(func(stages *Stages) {
		stages.Lookup = func(string) (hookspec.Handler, bool) { return handler, true }
	})
}

// postureErrorReader fails every read.
type postureErrorReader struct{}

func (postureErrorReader) Read([]byte) (int, error) { return 0, errors.New("private input error") }

func postureUnreadable(*testing.T, surface.ID) io.Reader { return postureErrorReader{} }

func postureMalformed(*testing.T, surface.ID) io.Reader { return strings.NewReader("{") }

// postureBlockingReader blocks every read until it is closed and reports
// that a read began.
type postureBlockingReader struct {
	closed, entered chan struct{}
	closeOnce       sync.Once
	enterOnce       sync.Once
}

func newPostureBlockingReader(t *testing.T) *postureBlockingReader {
	r := &postureBlockingReader{closed: make(chan struct{}), entered: make(chan struct{})}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func (r *postureBlockingReader) Read([]byte) (int, error) {
	r.enterOnce.Do(func() { close(r.entered) })
	<-r.closed
	return 0, io.ErrClosedPipe
}

func (r *postureBlockingReader) Close() error {
	r.closeOnce.Do(func() { close(r.closed) })
	return nil
}

// postureSettleBlocked requires the runner to have closed the blocking
// reader, and waits for the input goroutine's read to begin.
func postureSettleBlocked(t *testing.T, stdin io.Reader) {
	t.Helper()
	reader := stdin.(*postureBlockingReader)
	select {
	case <-reader.closed:
	default:
		t.Error("the input deadline did not close the invocation-owned reader")
	}
	select {
	case <-reader.entered:
	case <-time.After(10 * time.Second):
		t.Error("the input goroutine never read stdin")
	}
}

var posturePass = postureHandler(contract.Pass(), false)

// postureSteps resolves the commands check through its steps: the lookup
// misses "commands", the command rules run first, and the release rules
// write chatter into the buffer both steps share.
func postureSteps(first hookspec.Handler) func(*testing.T, surface.ID, *Stages) {
	return postureStub(func(stages *Stages) {
		stages.Lookup = postureLookupByName(map[string]hookspec.Handler{
			"command-guardian": first,
			"release-guardian": postureHandler(contract.Pass(), true),
			"secrets-guardian": posturePass,
		})
	})
}

var legacyPostureRows = []postureRow{
	{plan: "legacy", fault: "unreadable-stdin", stdin: postureUnreadable, stub: postureStubHandler(posturePass),
		oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1, render: 1, stdinRead: true}},
	{plan: "legacy", fault: "malformed-payload", stdin: postureMalformed, stub: postureStubHandler(posturePass),
		oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1, normalize: 1, stdinRead: true}},
	{plan: "legacy", fault: "bad-argv", extraArgs: []string{"private-argument"}, stub: postureStubHandler(posturePass),
		oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1}},
	{plan: "legacy", fault: "unknown-hook", stub: postureStub(func(stages *Stages) {
		stages.Lookup = func(string) (hookspec.Handler, bool) { return nil, false }
	}), oracle: postureOracleUsage, stages: postureStages{lookup: 1}},
	{plan: "legacy", fault: "lookup-panic", stub: postureStub(func(stages *Stages) {
		stages.Lookup = func(string) (hookspec.Handler, bool) { panic("lookup boom") }
	}), oracle: postureOracleErrorShape, stages: postureStages{lookup: 1}},
	{plan: "legacy", fault: "dispatch-panic", stub: func(t *testing.T, id surface.ID, stages *Stages) {
		postureStubHandler(posturePass)(t, id, stages)
		stages.Dispatch = func(string, string, []string) (string, *provider.Rendered) { panic("dispatch boom") }
	}, oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1}},
	{plan: "legacy", fault: "normalize-panic", stub: func(t *testing.T, id surface.ID, stages *Stages) {
		postureStubHandler(posturePass)(t, id, stages)
		stages.Normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) { panic("normalize boom") }
	}, oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1, normalize: 1, stdinRead: true}},
	{plan: "legacy", fault: "render-panic", stub: func(t *testing.T, id surface.ID, stages *Stages) {
		postureStubHandler(posturePass)(t, id, stages)
		stages.Render = func(string, string, hookspec.Outcome, []byte) provider.Rendered { panic("render boom") }
	}, oracle: postureOracleErrorShape, stages: postureStages{lookup: 1, dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
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

// eventChecked is the stage count of an event row whose two checks both run
// through a lookup of their own names.
var eventChecked = postureStages{lookup: 2, dispatch: 1, normalize: 1, render: 1, stdinRead: true}

// eventStepped is the stage count of an event row whose commands check runs
// its two steps: one lookup of "commands", one per step, one of
// secrets-guardian.
var eventStepped = postureStages{lookup: 4, dispatch: 1, normalize: 1, render: 1, stdinRead: true}

var eventPostureRows = []postureRow{
	{plan: "event", fault: "bad-argv", extraArgs: []string{"private-argument"}, stub: postureStubHandler(posturePass),
		oracle: postureOracleDeny, stages: postureStages{dispatch: 1, render: 1}},
	{plan: "event", fault: "unreadable-stdin", stdin: postureUnreadable, stub: postureStubHandler(posturePass),
		oracle: postureOracleDeny, stages: postureStages{dispatch: 1, render: 1, stdinRead: true}},
	{plan: "event", fault: "malformed-payload", stdin: postureMalformed, stub: postureStubHandler(posturePass),
		oracle: postureOracleDeny, stages: postureStages{dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "event", fault: "invalid-normalized-event", stub: func(t *testing.T, id surface.ID, stages *Stages) {
		postureStubHandler(posturePass)(t, id, stages)
		stages.Normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) { return hookspec.Event{}, nil }
	}, oracle: postureOracleDeny, stages: postureStages{dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "event", fault: "surface-mismatch", stub: func(t *testing.T, id surface.ID, stages *Stages) {
		event, err := contract.NewEvent(contract.EventParts{
			Surface: postureOtherSurface(id), ToolName: "Bash", ToolInput: map[string]any{"command": "echo safe"},
		})
		if err != nil {
			t.Fatal(err)
		}
		postureStubHandler(posturePass)(t, id, stages)
		stages.Normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) { return event, nil }
	}, oracle: postureOracleDeny, stages: postureStages{dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "event", fault: "dispatch-panic", stub: func(t *testing.T, id surface.ID, stages *Stages) {
		postureStubHandler(posturePass)(t, id, stages)
		stages.Dispatch = func(string, string, []string) (string, *provider.Rendered) { panic("dispatch boom") }
	}, oracle: postureOracleDeny, stages: postureStages{dispatch: 1}},
	{plan: "event", fault: "dispatch-wrong-event", stub: func(t *testing.T, id surface.ID, stages *Stages) {
		postureStubHandler(posturePass)(t, id, stages)
		stages.Dispatch = func(string, string, []string) (string, *provider.Rendered) { return "wrong", nil }
	}, oracle: postureOracleDeny, stages: postureStages{dispatch: 1, render: 1}},
	{plan: "event", fault: "normalize-panic", stub: func(t *testing.T, id surface.ID, stages *Stages) {
		postureStubHandler(posturePass)(t, id, stages)
		stages.Normalize = func(string, string, []byte) (hookspec.Event, *provider.Rendered) { panic("normalize boom") }
	}, oracle: postureOracleDeny, stages: postureStages{dispatch: 1, normalize: 1, render: 1, stdinRead: true}},
	{plan: "event", fault: "render-panic", stub: func(t *testing.T, id surface.ID, stages *Stages) {
		postureStubHandler(posturePass)(t, id, stages)
		stages.Render = func(string, string, hookspec.Outcome, []byte) provider.Rendered { panic("render boom") }
	}, oracle: postureOracleDeny, stages: eventChecked},
	// The lookup misses only secrets-guardian. The commands check is not a
	// registry key, so it runs its two steps through the registry, and only
	// secrets-guardian is unavailable.
	{plan: "event", fault: "missing-handler", stub: postureStub(func(stages *Stages) {
		stages.Lookup = func(name string) (hookspec.Handler, bool) {
			if name == "secrets-guardian" {
				return nil, false
			}
			return hooks.Lookup(name)
		}
	}), oracle: postureOracleDeny, stages: eventStepped},
	{plan: "event", fault: "lookup-panic", stub: postureStub(func(stages *Stages) {
		stages.Lookup = func(string) (hookspec.Handler, bool) { panic("lookup boom") }
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
	{plan: "event", fault: "deny-and-context", stub: postureStub(func(stages *Stages) {
		stages.Lookup = postureLookupByName(map[string]hookspec.Handler{
			"commands":         postureHandler(contract.Deny("denied by stub"), false),
			"secrets-guardian": postureHandler(contract.Context("context note"), false),
		})
	}), oracle: postureOracleDeny, stages: eventChecked},
	{plan: "event", fault: "pass-with-chatter", stub: postureStubHandler(postureHandler(contract.Pass(), true)),
		stages: eventChecked},
}

// eventShortPostureRows need a budget shorter than production's, a canceled
// parent, the step path, or a look at the budget a check receives, which the
// entry point's seam cannot inject.
var eventShortPostureRows = []postureRow{
	// Each check waits for its budget to end, then writes and returns a
	// partial verdict within the grace.
	{plan: "event", fault: "deadline-completed", stub: postureStubHandler(func(req hookspec.Request) hookspec.Outcome {
		<-req.ExecutionContext().Done()
		_, _ = io.WriteString(req.Diag, "cleanup diagnostic\n")
		return contract.Context("partial result")
	}), tune: func(p *posture) {
		p.checkBudgets = map[string]time.Duration{"commands": 20 * time.Millisecond, "secrets-guardian": 20 * time.Millisecond}
	}, oracle: postureOracleDeny, stages: eventChecked},
	// Each check ignores its context and outlives the grace.
	{plan: "event", fault: "deadline-cleanup-unacknowledged", stub: func(t *testing.T, id surface.ID, stages *Stages) {
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		postureStubHandler(func(hookspec.Request) hookspec.Outcome {
			<-release
			return contract.Pass()
		})(t, id, stages)
	}, tune: func(p *posture) {
		p.event.checkBudget = 20 * time.Millisecond
		p.event.grace = 20 * time.Millisecond
	}, oracle: postureOracleDeny, stages: eventChecked},
	{plan: "event", fault: "canceled-parent", stub: postureStubHandler(posturePass), parent: func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}, oracle: postureOracleDeny, stages: eventChecked},
	{plan: "event", fault: "input-deadline", stdin: func(t *testing.T, _ surface.ID) io.Reader { return newPostureBlockingReader(t) },
		stub: postureStubHandler(posturePass), tune: func(p *posture) { p.event.inputBudget = 20 * time.Millisecond },
		settle: postureSettleBlocked, oracle: postureOracleDeny, stages: postureStages{dispatch: 1, render: 1, stdinRead: true}},
	{plan: "event", fault: "step-panic", stub: postureSteps(posturePanicHandler),
		oracle: postureOracleDeny, stages: eventStepped},
	{plan: "event", fault: "step-invalid", stub: postureSteps(postureHandler(hookspec.Outcome{}, false)),
		oracle: postureOracleDeny, stages: eventStepped},
	// Each selected check runs under the production check budget of 10 s,
	// read from the deadline of the context it receives.
	{plan: "event", fault: "check-budget", stub: func(t *testing.T, id surface.ID, stages *Stages) {
		var seen atomic.Int32
		t.Cleanup(func() {
			if got := seen.Load(); got != 2 {
				t.Errorf("%d checks read their budget, want 2", got)
			}
		})
		postureStubHandler(func(req hookspec.Request) hookspec.Outcome {
			seen.Add(1)
			deadline, ok := req.ExecutionContext().Deadline()
			if remaining := time.Until(deadline); !ok || remaining > 10*time.Second || remaining <= 9*time.Second {
				t.Errorf("check %q has %v left of its budget (deadline set: %v), want a 10s budget", req.Hook, remaining, ok)
			}
			return contract.Pass()
		})(t, id, stages)
	}, stages: eventChecked},
}

// postureReader records whether the runner read stdin and passes a close on
// to the payload, so the input deadline can end a blocked read.
type postureReader struct {
	inner io.Reader
	read  atomic.Bool
}

func (r *postureReader) Read(p []byte) (int, error) {
	r.read.Store(true)
	return r.inner.Read(p)
}

func (r *postureReader) Close() error {
	if closer, ok := r.inner.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// postureUsage is the entry point's usage line, which it prints after an
// unanswered invocation.
const postureUsage = "usage: agx <provider> <hook> [hook args...] | agx flow <command> [args...] | agx help"

// postureEmit plays the entry point's part: it writes an answered reply and
// reports an unanswered one as an unknown hook.
func postureEmit(rendered provider.Rendered, answered bool, verb string) (exit int, stdout, stderr string) {
	if !answered {
		return hookspec.ExitUsage, "", fmt.Sprintf("agx: unknown hook %q (known: %s)\n%s\n", verb, strings.Join(hooks.Names(), ", "), postureUsage)
	}
	return rendered.Exit, string(rendered.Stdout), string(rendered.Stderr)
}

func postureSafeShellStdin(t *testing.T, id surface.ID) []byte {
	t.Helper()
	return hosttest.Stdin(id, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "echo safe"}, Cwd: t.TempDir()})
}

// postureRun runs one row on one surface through answer.
func postureRun(t *testing.T, row postureRow, id surface.ID) (exit int, stdout, stderr string, stages postureStages) {
	t.Helper()
	base := Production()
	if row.stub != nil {
		row.stub(t, id, &base)
	}
	var lookups, dispatches, normalizes, renders atomic.Int32
	counted := Stages{
		Lookup: func(name string) (hookspec.Handler, bool) {
			lookups.Add(1)
			return base.Lookup(name)
		},
		Dispatch: func(s, hook string, args []string) (string, *provider.Rendered) {
			dispatches.Add(1)
			return base.Dispatch(s, hook, args)
		},
		Normalize: func(s, event string, raw []byte) (hookspec.Event, *provider.Rendered) {
			normalizes.Add(1)
			return base.Normalize(s, event, raw)
		},
		Render: func(s, event string, outcome hookspec.Outcome, diag []byte) provider.Rendered {
			renders.Add(1)
			return base.Render(s, event, outcome, diag)
		},
	}
	verb, args := "pre-tool", row.extraArgs
	if row.plan == "legacy" {
		verb, args = "command-guardian", append([]string{"--mode", "block"}, row.extraArgs...)
	}
	var payload io.Reader
	if row.stdin != nil {
		payload = row.stdin(t, id)
	} else {
		payload = bytes.NewReader(postureSafeShellStdin(t, id))
	}
	p := productionPosture()
	if row.tune != nil {
		row.tune(&p)
	}
	parent := context.Background()
	if row.parent != nil {
		parent = row.parent()
	}
	reader := &postureReader{inner: payload}
	rendered, answered := answer(parent, counted, p, id, verb, args, reader)
	if row.settle != nil {
		row.settle(t, payload)
	}
	exit, stdout, stderr = postureEmit(rendered, answered, verb)
	stages = postureStages{
		lookup: lookups.Load(), dispatch: dispatches.Load(), normalize: normalizes.Load(), render: renders.Load(),
		stdinRead: reader.read.Load(),
	}
	return exit, stdout, stderr, stages
}

const postureStackMarker = "\ngoroutine "

func postureErrorExit(id surface.ID) int {
	if id == surface.ClaudeCode {
		return 1
	}
	return 0
}

func postureQuietBody(id surface.ID) string {
	switch id {
	case surface.Codex:
		return "{}\n"
	case surface.Antigravity:
		return "{\"decision\":\"allow\"}\n"
	}
	return ""
}

func requirePostureOracle(t *testing.T, row postureRow, id surface.ID, exit int, stdout, stderr string) {
	t.Helper()
	verdict := hosttest.Read(id, exit, []byte(stdout), []byte(stderr))
	switch row.oracle {
	case postureOracleDeny:
		if verdict.Decision != hosttest.Deny {
			t.Errorf("host reads %v, want Deny", verdict.Decision)
		}
	case postureOracleErrorShape:
		if verdict.Decision == hosttest.Deny || exit != postureErrorExit(id) || stdout != postureQuietBody(id) {
			t.Errorf("host reads %v with exit=%d stdout=%q, want the surface's fault shape", verdict.Decision, exit, stdout)
		}
	case postureOracleUsage:
		if verdict.Decision == hosttest.Deny || exit != hookspec.ExitUsage || stdout != "" {
			t.Errorf("host reads %v with exit=%d stdout=%q, want a usage failure", verdict.Decision, exit, stdout)
		}
	}
}

// TestPostureTable pins every fault posture of both plans on every surface
// through answer. Its rows are those of the entry point's posture table,
// whose expectations it shares byte for byte, plus the rows that need a
// short budget, a canceled parent or the step path. Only the missing-handler
// row counts more lookups, because the commands check resolves its two steps
// where the entry point's runner called the composite directly.
func TestPostureTable(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	rows := append(append(append([]postureRow(nil), legacyPostureRows...), eventPostureRows...), eventShortPostureRows...)
	if len(postureWants) != len(rows) {
		t.Fatalf("%d rows hold %d expectations; every row needs exactly one", len(rows), len(postureWants))
	}
	for _, row := range rows {
		for _, id := range surface.All() {
			t.Run(row.plan+"/"+string(id)+"/"+row.fault, func(t *testing.T) {
				exit, stdout, stderr, stages := postureRun(t, row, id)
				want, ok := postureWants[row.key()][id]
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

// TestPostureTableIsClosed requires both plans to answer every fault kind:
// each cell holds a posture or says the plan has no such stage, and no cell
// can show a panic value it does not declare.
func TestPostureTableIsClosed(t *testing.T) {
	seen := map[string]bool{}
	for f := fault(0); f < faultCount; f++ {
		name := faultNames[f]
		if name == "" || seen[name] {
			t.Fatalf("fault %d has the name %q, want a unique one", f, name)
		}
		seen[name] = true
	}
	p := productionPosture()
	for _, pl := range []plan{p.legacy, p.event} {
		for f := fault(0); f < faultCount; f++ {
			c := pl.cells[f]
			t.Run(pl.name+"/"+faultNames[f], func(t *testing.T) {
				switch c.response {
				case responseUnset:
					t.Fatal("no posture and not marked as having no such stage")
				case responseAbsent, responseRejection, responseUnanswered:
					if c != (cell{response: c.response}) {
						t.Fatalf("a cell answered by %d carries posture fields: %+v", c.response, c)
					}
				case responseOutcome:
					if c.kind != contract.KindError && c.kind != contract.KindDeny {
						t.Fatalf("kind %q, want error or deny", c.kind)
					}
					if c.message == "" {
						t.Fatal("an outcome cell needs a message")
					}
				case responseOwn:
					if c.kind != "" || c.message != "" {
						t.Fatalf("a cell that keeps the check's outcome carries an outcome: %+v", c)
					}
				default:
					t.Fatalf("unknown response %d", c.response)
				}
				if c.formatted != strings.Contains(c.message, "%[") {
					t.Fatalf("formatted=%v disagrees with message %q", c.formatted, c.message)
				}
				if !c.showsValue && (strings.Contains(c.message, "%[2]") || strings.Contains(c.message, "%[3]")) {
					t.Fatalf("message %q shows a value the cell does not declare", c.message)
				}
				if c.showsValue && pl.name == "event" {
					t.Fatalf("an event cell shows a panic value or a read error: %+v", c)
				}
				if c.response == responseOutcome && c.kind == contract.KindError && pl.name == "event" && c.fault != "" && !c.guardDenies {
					t.Fatalf("an event fault with a notice must deny a guard: %+v", c)
				}
			})
		}
	}
	if p.event.inputBudget != 2*time.Second || p.event.checkBudget != 10*time.Second || p.event.grace != time.Second {
		t.Fatalf("event budgets = %v input, %v check, %v grace; want 2s, 10s, 1s", p.event.inputBudget, p.event.checkBudget, p.event.grace)
	}
	if p.event.label != "[%s] " || p.event.join != "\n\n" || p.event.chatterOnValid {
		t.Fatalf("event label %q join %q chatterOnValid %v", p.event.label, p.event.join, p.event.chatterOnValid)
	}
	if p.legacy.inputBudget != 0 || p.legacy.checkBudget != 0 || p.legacy.grace != 0 || p.legacy.label != "" || p.legacy.join != "" || !p.legacy.chatterOnValid {
		t.Fatalf("legacy plan carries event-only fields: %+v", p.legacy)
	}
	if !p.event.cells[faultGuardError].guardDenies || p.legacy.cells[faultGuardError].guardDenies {
		t.Fatal("only the event plan's guard error denies")
	}
	if p.checkBudgets != nil {
		t.Fatalf("production overrides check budgets: %v", p.checkBudgets)
	}
}

// postureWants holds each row's response per surface. The rows the entry
// point's table shares are copied from it; the short rows were recorded from
// the entry point's own input, check and fold functions composed with the
// same short budgets.
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
	"event/check-budget": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 0, stdout: "", stderr: ""},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "", stderr: ""},
	},
	"event/pass-with-chatter": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"allow\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 0, stdout: "", stderr: ""},
		surface.Codex:       {exit: 0, stdout: "{}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "", stderr: ""},
	},
	"event/deadline-completed": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"[commands] partial result\\n\\n[commands] deadline exceeded; incomplete check\\n\\n[secrets-guardian] partial result\\n\\n[secrets-guardian] deadline exceeded; incomplete check\"}\n", stderr: "[commands] deadline exceeded; incomplete check\n\n[commands] cleanup diagnostic\n\n\n[secrets-guardian] deadline exceeded; incomplete check\n\n[secrets-guardian] cleanup diagnostic\n"},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "[commands] partial result\n\n[commands] deadline exceeded; incomplete check\n\n[secrets-guardian] partial result\n\n[secrets-guardian] deadline exceeded; incomplete check\n[commands] deadline exceeded; incomplete check\n\n[commands] cleanup diagnostic\n\n\n[secrets-guardian] deadline exceeded; incomplete check\n\n[secrets-guardian] cleanup diagnostic\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] partial result\\n\\n[commands] deadline exceeded; incomplete check\\n\\n[secrets-guardian] partial result\\n\\n[secrets-guardian] deadline exceeded; incomplete check\"}}\n", stderr: "[commands] deadline exceeded; incomplete check\n\n[commands] cleanup diagnostic\n\n\n[secrets-guardian] deadline exceeded; incomplete check\n\n[secrets-guardian] cleanup diagnostic\n"},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] partial result\\n\\n[commands] deadline exceeded; incomplete check\\n\\n[secrets-guardian] partial result\\n\\n[secrets-guardian] deadline exceeded; incomplete check\"}\n", stderr: "[commands] deadline exceeded; incomplete check\n\n[commands] cleanup diagnostic\n\n\n[secrets-guardian] deadline exceeded; incomplete check\n\n[secrets-guardian] cleanup diagnostic\n"},
	},
	"event/deadline-cleanup-unacknowledged": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"[commands] check did not acknowledge cancellation within cleanup grace\\n\\n[commands] deadline exceeded; cleanup unacknowledged\\n\\n[secrets-guardian] check did not acknowledge cancellation within cleanup grace\\n\\n[secrets-guardian] deadline exceeded; cleanup unacknowledged\"}\n", stderr: "[commands] check did not acknowledge cancellation within cleanup grace\n\n[commands] deadline exceeded; cleanup unacknowledged\n\n[secrets-guardian] check did not acknowledge cancellation within cleanup grace\n\n[secrets-guardian] deadline exceeded; cleanup unacknowledged\n"},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "[commands] check did not acknowledge cancellation within cleanup grace\n\n[commands] deadline exceeded; cleanup unacknowledged\n\n[secrets-guardian] check did not acknowledge cancellation within cleanup grace\n\n[secrets-guardian] deadline exceeded; cleanup unacknowledged\n[commands] check did not acknowledge cancellation within cleanup grace\n\n[commands] deadline exceeded; cleanup unacknowledged\n\n[secrets-guardian] check did not acknowledge cancellation within cleanup grace\n\n[secrets-guardian] deadline exceeded; cleanup unacknowledged\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] check did not acknowledge cancellation within cleanup grace\\n\\n[commands] deadline exceeded; cleanup unacknowledged\\n\\n[secrets-guardian] check did not acknowledge cancellation within cleanup grace\\n\\n[secrets-guardian] deadline exceeded; cleanup unacknowledged\"}}\n", stderr: "[commands] check did not acknowledge cancellation within cleanup grace\n\n[commands] deadline exceeded; cleanup unacknowledged\n\n[secrets-guardian] check did not acknowledge cancellation within cleanup grace\n\n[secrets-guardian] deadline exceeded; cleanup unacknowledged\n"},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] check did not acknowledge cancellation within cleanup grace\\n\\n[commands] deadline exceeded; cleanup unacknowledged\\n\\n[secrets-guardian] check did not acknowledge cancellation within cleanup grace\\n\\n[secrets-guardian] deadline exceeded; cleanup unacknowledged\"}\n", stderr: "[commands] check did not acknowledge cancellation within cleanup grace\n\n[commands] deadline exceeded; cleanup unacknowledged\n\n[secrets-guardian] check did not acknowledge cancellation within cleanup grace\n\n[secrets-guardian] deadline exceeded; cleanup unacknowledged\n"},
	},
	"event/canceled-parent": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"[commands] deadline exceeded; incomplete check\\n\\n[secrets-guardian] deadline exceeded; incomplete check\"}\n", stderr: "[commands] deadline exceeded; incomplete check\n\n[secrets-guardian] deadline exceeded; incomplete check\n"},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "[commands] deadline exceeded; incomplete check\n\n[secrets-guardian] deadline exceeded; incomplete check\n[commands] deadline exceeded; incomplete check\n\n[secrets-guardian] deadline exceeded; incomplete check\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] deadline exceeded; incomplete check\\n\\n[secrets-guardian] deadline exceeded; incomplete check\"}}\n", stderr: "[commands] deadline exceeded; incomplete check\n\n[secrets-guardian] deadline exceeded; incomplete check\n"},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] deadline exceeded; incomplete check\\n\\n[secrets-guardian] deadline exceeded; incomplete check\"}\n", stderr: "[commands] deadline exceeded; incomplete check\n\n[secrets-guardian] deadline exceeded; incomplete check\n"},
	},
	"event/input-deadline": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"agx: event payload receive/decode deadline exceeded\"}\n", stderr: ""},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "agx: event payload receive/decode deadline exceeded\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: event payload receive/decode deadline exceeded\"}}\n", stderr: ""},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"agx: event payload receive/decode deadline exceeded\"}\n", stderr: ""},
	},
	"event/step-panic": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"[commands] command rules panicked\\n\\n[commands] check failed\"}\n", stderr: "[commands] command rules panicked\n\n[commands] check failed\n\n[commands] chatter\n"},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "[commands] command rules panicked\n\n[commands] check failed\n[commands] command rules panicked\n\n[commands] check failed\n\n[commands] chatter\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] command rules panicked\\n\\n[commands] check failed\"}}\n", stderr: "[commands] command rules panicked\n\n[commands] check failed\n\n[commands] chatter\n"},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] command rules panicked\\n\\n[commands] check failed\"}\n", stderr: "[commands] command rules panicked\n\n[commands] check failed\n\n[commands] chatter\n"},
	},
	"event/step-invalid": {
		surface.Antigravity: {exit: 0, stdout: "{\"decision\":\"deny\",\"reason\":\"[commands] rule set returned an invalid outcome\\n\\n[commands] check failed\"}\n", stderr: "[commands] rule set returned an invalid outcome\n\n[commands] check failed\n\n[commands] chatter\n"},
		surface.ClaudeCode:  {exit: 2, stdout: "", stderr: "[commands] rule set returned an invalid outcome\n\n[commands] check failed\n[commands] rule set returned an invalid outcome\n\n[commands] check failed\n\n[commands] chatter\n"},
		surface.Codex:       {exit: 0, stdout: "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] rule set returned an invalid outcome\\n\\n[commands] check failed\"}}\n", stderr: "[commands] rule set returned an invalid outcome\n\n[commands] check failed\n\n[commands] chatter\n"},
		surface.Copilot:     {exit: 0, stdout: "{\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"[commands] rule set returned an invalid outcome\\n\\n[commands] check failed\"}\n", stderr: "[commands] rule set returned an invalid outcome\n\n[commands] check failed\n\n[commands] chatter\n"},
	},
}
