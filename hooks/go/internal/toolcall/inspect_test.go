package toolcall

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// hereString is a PowerShell here-string holding body, piped to sink.
func hereString(body, sink string) string {
	return "@'\n" + body + "\n'@ | " + sink
}

func commandCall(t *testing.T, surfaceName, tool, command string) Call {
	t.Helper()
	return executionCall(t, surfaceName, tool, nil, map[string]any{"command": command})
}

// assertPolicyIsSource fails unless the inspection kept the typed command's own parse as its policy.
func assertPolicyIsSource(t *testing.T, name string, got Inspection, command string) {
	t.Helper()
	if !reflect.DeepEqual(got.Policy, got.Source) || got.DataProven || got.DataView != command {
		t.Fatalf("%s: policy swapped or data view proven: proven=%v view=%q policy=%+v source=%+v", name, got.DataProven, got.DataView, got.Policy, got.Source)
	}
}

func TestInspectPolicyIsSourceOutsidePowerShellData(t *testing.T) {
	for _, tc := range []struct{ name, tool, command string }{
		{"posix", "Bash", "echo ok && rg needle file.txt"},
		{"powershell without data", "PowerShell", "Get-Content -LiteralPath x | Select-String needle"},
		{"here-string to Out-File", "PowerShell", hereString("gh stack submit", "Out-File -LiteralPath x")},
	} {
		call := commandCall(t, "claude-code", tc.tool, tc.command)
		got := call.Inspect(context.Background(), nil)
		want := shellwords.ParseScripts(context.Background(), tc.command, call.Execution().Dialect, shellwords.ScriptOptions{})
		if !reflect.DeepEqual(got.Source, want) {
			t.Fatalf("%s: source=%+v, want %+v", tc.name, got.Source, want)
		}
		assertPolicyIsSource(t, tc.name, got, tc.command)
	}
}

func TestInspectProvenPowerShellDataSwapsPolicy(t *testing.T) {
	command := hereString("gh stack submit; bash t.sh", "Set-Content -LiteralPath x")
	call := executionCall(t, "claude-code", "PowerShell", nil, map[string]any{"command": command})
	ctx := context.Background()
	reader := hookspectest.Scripts{}
	got := call.Inspect(ctx, reader)
	view, proven, err := shellwords.PowerShellFileData(ctx, command, call.Execution())
	if err != nil || !proven || view == command {
		t.Fatalf("fixture is not proven data: proven=%v err=%v", proven, err)
	}
	if !got.DataProven || got.DataView != view {
		t.Fatalf("data view: proven=%v view=%q, want %q", got.DataProven, got.DataView, view)
	}
	if want := shellwords.ParseContext(ctx, view, shellwords.PowerShell); !reflect.DeepEqual(got.Policy, want) {
		t.Fatalf("policy=%+v, want the parse of the data view %+v", got.Policy, want)
	}
	source := shellwords.ParseScripts(ctx, command, shellwords.PowerShell, call.ScriptOptions(reader))
	if !reflect.DeepEqual(got.Source, source) {
		t.Fatalf("source=%+v, want the typed command's parse %+v", got.Source, source)
	}
	if reflect.DeepEqual(got.Policy, got.Source) {
		t.Fatal("a proven data view must change the policy parse")
	}
}

func TestInspectCanceledReparseKeepsSource(t *testing.T) {
	command := hereString("gh stack submit", "Set-Content -LiteralPath x")
	call := commandCall(t, "claude-code", "PowerShell", command)
	baseline := &hookspectest.CancelContext{Context: context.Background()}
	if !call.Inspect(baseline, nil).DataProven {
		t.Fatal("fixture must prove its data view uncancelled")
	}
	canceled := &hookspectest.CancelContext{Context: context.Background(), At: baseline.Calls}
	got := call.Inspect(canceled, nil)
	if canceled.Calls != baseline.Calls {
		t.Fatalf("checks=%d, want %d", canceled.Calls, baseline.Calls)
	}
	assertPolicyIsSource(t, "canceled reparse", got, command)

	noncanonical := commandCall(t, "claude-code", "powershell", command)
	assertPolicyIsSource(t, "noncanonical tool", noncanonical.Inspect(context.Background(), nil), command)
}

func TestInspectWithoutCommandParsesNothing(t *testing.T) {
	for name, input := range map[string]map[string]any{
		"absent":     {},
		"non-string": {"command": true},
		"null":       {"command": nil},
	} {
		ctx := &hookspectest.CancelContext{Context: context.Background()}
		got := executionCall(t, "claude-code", "Bash", nil, input).Inspect(ctx, hookspectest.Scripts{})
		if !reflect.DeepEqual(got.Source, shellwords.Result{}) || !reflect.DeepEqual(got.Policy, shellwords.Result{}) ||
			got.DataView != "" || got.DataProven || ctx.Calls != 0 {
			t.Fatalf("%s: parsed without a command: %+v checks=%d", name, got, ctx.Calls)
		}
	}
	ctx := &hookspectest.CancelContext{Context: context.Background()}
	commandCall(t, "claude-code", "Bash", "").Inspect(ctx, nil)
	if ctx.Calls < 1 {
		t.Fatal("an empty-string command must be parsed")
	}
}

// scriptRefusal returns the rule of the first refused invocation of the source parse.
func scriptRefusal(got Inspection) shellwords.Rule {
	for _, inv := range got.Source.Invocations {
		if !inv.Inspectable {
			return inv.Reason.Rule()
		}
	}
	return shellwords.RuleNone
}

func TestInspectScriptOptions(t *testing.T) {
	ctx := context.Background()
	reader := hookspectest.Scripts{"/ws/t.sh": "echo hi\n"}
	ws := "/ws"
	relative := "ws"
	empty := ""

	expanded := func(got Inspection) bool { return len(got.Source.Scripts) == 1 && len(got.Source.ScriptCalls) == 1 }

	if got := cwdCall(t, &ws, map[string]any{"command": "bash t.sh"}).Inspect(ctx, nil); len(got.Source.Scripts) != 0 || len(got.Source.ScriptCalls) != 0 {
		t.Fatalf("nil reader expanded a script: %+v", got.Source)
	}
	if got := cwdCall(t, &ws, map[string]any{"command": "bash t.sh"}).Inspect(ctx, reader); !expanded(got) || scriptRefusal(got) != shellwords.RuleNone {
		t.Fatalf("proven cwd did not expand the script: %+v", got.Source)
	}
	for name, tc := range map[string]struct {
		cwd   *string
		input map[string]any
	}{
		"cwd disagrees":     {&ws, map[string]any{"command": "bash t.sh", "cwd": "/other"}},
		"workdir disagrees": {&ws, map[string]any{"command": "bash t.sh", "workdir": "/other"}},
		"relative cwd":      {&relative, map[string]any{"command": "bash t.sh"}},
		"empty cwd":         {&empty, map[string]any{"command": "bash t.sh"}},
		"no cwd":            {nil, map[string]any{"command": "bash t.sh"}},
	} {
		got := cwdCall(t, tc.cwd, tc.input).Inspect(ctx, reader)
		if expanded(got) || scriptRefusal(got) != shellwords.RuleCwdUnproven {
			t.Errorf("%s: rule=%v source=%+v", name, scriptRefusal(got), got.Source)
		}
	}
	for name, call := range map[string]Call{
		"noncanonical tool": executionCall(t, "claude-code", "powershell", &ws, map[string]any{"command": "bash t.sh"}),
		"malformed value":   executionCall(t, "codex", "Bash", &ws, map[string]any{"command": "bash t.sh", "value": "x"}),
	} {
		if call.Execution().AllowsNewProof() {
			t.Fatalf("%s: fixture context is proven", name)
		}
		got := call.Inspect(ctx, reader)
		if expanded(got) || scriptRefusal(got) != shellwords.RuleContextUnqualified {
			t.Errorf("%s: rule=%v source=%+v", name, scriptRefusal(got), got.Source)
		}
	}
}

var authorityRefusal = shellwords.GuardRefusal(shellwords.GuardBashAuthorityUnavailable)

// refusalCases are inspections with the index Refusal is asked about and the answer it must give.
func refusalCases(t *testing.T) []struct {
	name      string
	got       Inspection
	index     int
	want      shellwords.Refusal
	refused   bool
	checksCtx bool
} {
	t.Helper()
	ctx := context.Background()
	uninspectable := commandCall(t, "claude-code", "Bash", `bash -c "$x"`).Inspect(ctx, nil)
	if len(uninspectable.Policy.Invocations) == 0 || uninspectable.Policy.Invocations[0].Inspectable || !uninspectable.Policy.Invocations[0].Reason.Refused() {
		t.Fatalf("fixture is not an uninspectable invocation with a reason: %+v", uninspectable.Policy)
	}
	zeroReason := Inspection{Policy: shellwords.Result{Invocations: []shellwords.Invocation{{Inspectable: false}}}}
	syntax := commandCall(t, "claude-code", "Bash", "bash -n x").Inspect(ctx, nil)
	if syntax.Policy.Invocations[0].Inspection != shellwords.BashSyntaxRead {
		t.Fatalf("fixture is not a syntax inspection: %+v", syntax.Policy)
	}
	unauthorised := commandCall(t, "claude-code", "bash", "bash -n x").Inspect(ctx, nil)
	plain := commandCall(t, "claude-code", "Bash", "echo ok").Inspect(ctx, nil)
	return []struct {
		name      string
		got       Inspection
		index     int
		want      shellwords.Refusal
		refused   bool
		checksCtx bool
	}{
		{"uninspectable", uninspectable, 0, uninspectable.Policy.Invocations[0].Reason, true, false},
		{"uninspectable zero reason", zeroReason, 0, shellwords.Refusal{}, true, false},
		{"syntax without authority", unauthorised, 0, authorityRefusal, true, false},
		{"syntax with authority", syntax, 0, shellwords.Refusal{}, false, true},
		{"plain", plain, 0, shellwords.Refusal{}, false, false},
		{"index -1", syntax, -1, authorityRefusal, true, false},
		{"index past the end", syntax, len(syntax.Policy.Invocations), authorityRefusal, true, false},
	}
}

func TestInspectionRefusal(t *testing.T) {
	for _, tc := range refusalCases(t) {
		got, refused := tc.got.Refusal(context.Background(), tc.index)
		if got != tc.want || refused != tc.refused {
			t.Errorf("%s: (%v, %v), want (%v, %v)", tc.name, got, refused, tc.want, tc.refused)
		}
	}
	syntax := commandCall(t, "claude-code", "Bash", "bash -n x").Inspect(context.Background(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, refused := syntax.Refusal(ctx, 0); got != authorityRefusal || !refused {
		t.Fatalf("cancelled context with authority: (%v, %v)", got, refused)
	}
}

func TestInspectionRefusalChecksContextOnlyWhenAuthorised(t *testing.T) {
	for _, tc := range refusalCases(t) {
		ctx := &hookspectest.CancelContext{Context: context.Background()}
		tc.got.Refusal(ctx, tc.index)
		want := 0
		if tc.checksCtx {
			want = 1
		}
		if ctx.Calls != want {
			t.Errorf("%s: checks=%d, want %d", tc.name, ctx.Calls, want)
		}
	}
}

func TestInspectionAuthority(t *testing.T) {
	ws := "/ws"
	calls := []Call{
		commandCall(t, "claude-code", "Bash", "echo ok && rg needle file.txt"),
		commandCall(t, "claude-code", "PowerShell", "Get-Content -LiteralPath x | Select-String needle"),
		commandCall(t, "claude-code", "PowerShell", hereString("gh stack submit", "Out-File -LiteralPath x")),
		commandCall(t, "claude-code", "PowerShell", hereString("gh stack submit; bash t.sh", "Set-Content -LiteralPath x")),
		commandCall(t, "claude-code", "powershell", hereString("gh stack submit", "Set-Content -LiteralPath x")),
		commandCall(t, "claude-code", "Bash", `bash -c "$x"`),
		commandCall(t, "claude-code", "Bash", "bash -n x"),
		commandCall(t, "claude-code", "bash", "bash -n x"),
		cwdCall(t, &ws, map[string]any{"command": "bash t.sh"}),
		executionCall(t, "claude-code", "Bash", nil, map[string]any{}),
	}
	for _, call := range calls {
		got := call.Inspect(context.Background(), hookspectest.Scripts{"/ws/t.sh": "echo hi\n"})
		for i := -1; i <= len(got.Policy.Invocations); i++ {
			if want := call.Execution().ForInvocation(got.Policy, i); got.Authority(i) != want {
				t.Errorf("%q index %d: %+v, want %+v", got.DataView, i, got.Authority(i), want)
			}
		}
		if reason := got.Authority(len(got.Policy.Invocations)).Reason; reason != "missing invocation" {
			t.Errorf("%q: out-of-range reason %q", got.DataView, reason)
		}
	}
}

func TestInspectLongCommand(t *testing.T) {
	ctx := context.Background()
	reader := hookspectest.Scripts{}
	for _, tc := range []struct{ tool, command string }{
		{"Bash", "echo " + strings.Repeat("a", 100<<10)},
		{"PowerShell", hereString(strings.Repeat("a", 100<<10), "Set-Content -LiteralPath x")},
	} {
		call := commandCall(t, "claude-code", tc.tool, tc.command)
		got := call.Inspect(ctx, reader)
		want := shellwords.ParseScripts(ctx, tc.command, call.Execution().Dialect, call.ScriptOptions(reader))
		if !reflect.DeepEqual(got.Source, want) {
			t.Fatalf("%s: source differs from a direct parse", tc.tool)
		}
	}
}
