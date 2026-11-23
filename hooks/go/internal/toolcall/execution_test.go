package toolcall

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/pathshape"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// executionCall reads a call with this surface, tool, cwd, and input. An event
// contract.NewEvent refuses, an unknown surface or a null input, is built as a
// neutral event literal so the reader still sees it.
func executionCall(t *testing.T, surfaceName, tool string, cwd *string, input map[string]any) Call {
	t.Helper()
	var neutral contract.NeutralEvent
	if id, known := surface.Parse(surfaceName); known && input != nil {
		event, err := contract.NewEvent(contract.EventParts{Surface: id, ToolName: tool, Cwd: cwd, ToolInput: input})
		if err != nil {
			t.Fatal(err)
		}
		if neutral, err = event.Read(); err != nil {
			t.Fatal(err)
		}
	} else {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		neutral = contract.NeutralEvent{Surface: surfaceName, Cwd: cwd, Tool: contract.NeutralTool{Name: &tool, InputRaw: raw}}
	}
	call, err := Read(neutral)
	if err != nil {
		t.Fatal(err)
	}
	return call
}

func refused(dialect shellwords.Dialect, reason string) shellwords.ExecutionContext {
	return shellwords.ExecutionContext{Dialect: dialect, Reason: reason}
}

func proven(dialect shellwords.Dialect) shellwords.ExecutionContext {
	return shellwords.ExecutionContext{Dialect: dialect, Proven: true}
}

func TestExecutionNestedSelectors(t *testing.T) {
	for _, input := range []map[string]any{
		{"command": "echo ok", "options": map[string]any{"shell": "pwsh"}},
		{"command": "echo ok", "value": map[string]any{"command": "echo ok", "options": map[string]any{"shell": nil}}},
		{"command": "echo ok", "options": []any{map[string]any{"shell": "pwsh"}}},
	} {
		got := executionCall(t, "codex", "PowerShell", nil, input).Execution()
		if want := refused(shellwords.PowerShell, "unqualified nested arguments"); got != want {
			t.Fatalf("input %v: context=%+v, want %+v", input, got, want)
		}
	}
}

func TestExecutionPrecision(t *testing.T) {
	const (
		posix = shellwords.POSIX
		pwsh  = shellwords.PowerShell
	)
	for _, tc := range []struct {
		name, surface, tool string
		input               map[string]any
		want                shellwords.ExecutionContext
	}{
		{"absent", "claude-code", "Bash", nil, refused(posix, "missing command evidence")},
		{"default", "claude-code", "Bash", map[string]any{"command": "echo ok"}, proven(posix)},
		{"native neutral", "codex", "PowerShell", map[string]any{"command": "rg --no-config 'needle' source.txt"}, proven(pwsh)},
		{"codex actual", "codex", "Bash", map[string]any{"command": "echo ok", "value": map[string]any{"command": "echo ok", "shell": "pwsh"}}, refused(posix, "unqualified shell selector")},
		{"namespace", "copilot", "mcp__x__PowerShell", map[string]any{"command": "echo ok"}, refused(pwsh, "noncanonical shell tool")},
		{"case", "claude-code", "powershell", map[string]any{"command": "echo ok"}, refused(pwsh, "noncanonical shell tool")},
		{"unknown provider", "other", "PowerShell", map[string]any{"command": "echo ok"}, refused(pwsh, "unqualified provider")},
		{"canonical envelope", "copilot", "PowerShell", map[string]any{"command": "echo ok", "value": map[string]any{"command": "echo ok"}}, proven(pwsh)},
		{"conflicting command", "copilot", "PowerShell", map[string]any{"command": "echo ok", "value": map[string]any{"command": "other"}}, refused(pwsh, "competing command evidence")},
		{"missing child command", "copilot", "PowerShell", map[string]any{"command": "echo ok", "value": map[string]any{}}, refused(pwsh, "competing command evidence")},
		{"deep envelope", "copilot", "PowerShell", map[string]any{"command": "echo ok", "value": map[string]any{"command": "echo ok", "value": map[string]any{}}}, refused(pwsh, "deep argument envelope")},
		{"null envelope", "copilot", "PowerShell", map[string]any{"command": "echo ok", "value": nil}, refused(pwsh, "malformed argument envelope")},
		{"path override", "copilot", "PowerShell", map[string]any{"command": "echo ok", "env": map[string]any{"PATH": "synthetic"}}, refused(pwsh, "unqualified nested arguments")},
		{"pathext override", "copilot", "PowerShell", map[string]any{"command": "echo ok", "environment": map[string]any{"PATHEXT": "synthetic"}}, refused(pwsh, "unqualified nested arguments")},
		{"malformed override", "copilot", "PowerShell", map[string]any{"command": "echo ok", "env": nil}, refused(pwsh, "unqualified execution environment")},

		{"64 root keys", "claude-code", "Bash", keyed(63, nil), proven(posix)},
		{"65 root keys", "claude-code", "Bash", keyed(64, nil), refused(posix, "argument evidence limit")},
		{"65 value keys", "codex", "Bash", map[string]any{"command": "echo ok", "value": keyed(64, nil)}, refused(posix, "argument evidence limit")},
		{"65 root keys and a nested value", "claude-code", "Bash", keyed(64, map[string]any{"k": "v"}), refused(posix, "argument evidence limit")},
		{"cmd alone on claude-code", "claude-code", "Bash", map[string]any{"cmd": "x"}, refused(posix, "missing command evidence")},
		{"env object", "claude-code", "Bash", map[string]any{"command": "echo ok", "env": map[string]any{}}, refused(posix, "unqualified nested arguments")},
		{"variant env string", "claude-code", "Bash", map[string]any{"command": "echo ok", "ENV": "x"}, refused(posix, "unqualified execution environment")},
		{"null env in value", "codex", "Bash", map[string]any{"command": "echo ok", "value": map[string]any{"command": "echo ok", "env": nil}}, refused(posix, "unqualified execution environment")},
		{"selector beside env", "claude-code", "Bash", map[string]any{"command": "echo ok", "shell": "bash", "env": "x"}, refused(posix, "unqualified shell selector")},
		{"long s selector", "claude-code", "Bash", map[string]any{"command": "echo ok", "ſhell": "bash"}, refused(posix, "unqualified shell selector")},
		{"powershell on other", "other", "powershell", map[string]any{"command": "echo ok"}, refused(pwsh, "noncanonical shell tool")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := executionCall(t, tc.surface, tc.tool, nil, tc.input).Execution(); got != tc.want {
				t.Fatalf("context=%+v; want %+v", got, tc.want)
			}
		})
	}
	for _, selector := range []any{"bash", "bash.exe", "pwsh", "powershell", "pwsh.exe", `C:\Program Files\PowerShell\7\pwsh.exe`, "/usr/bin/bash", "/usr/bin/pwsh", "fish", "", nil, true, []any{"pwsh"}, map[string]any{"name": "pwsh"}} {
		want := refused(pwsh, "unqualified shell selector")
		switch selector.(type) {
		case []any, map[string]any:
			want = refused(pwsh, "unqualified nested arguments")
		}
		for _, key := range []string{"shell", "Shell", "SHELL"} {
			for _, wrapped := range []bool{false, true} {
				input := map[string]any{"command": "echo ok", key: selector}
				if wrapped {
					input = map[string]any{"command": "echo ok", "value": input}
				}
				if got := executionCall(t, "codex", "PowerShell", nil, input).Execution(); got != want {
					t.Fatalf("selector %v key %s wrapped=%v: context=%+v, want %+v", selector, key, wrapped, got, want)
				}
			}
		}
	}
}

// keyed returns a command plus extra string keys, and a nested key when nested
// is not nil.
func keyed(extra int, nested map[string]any) map[string]any {
	input := map[string]any{"command": "echo ok"}
	for i := range extra {
		input[fmt.Sprintf("k%02d", i)] = "v"
	}
	if nested != nil {
		input["k00"] = nested
	}
	return input
}

func TestExecutionProviderCommandKeys(t *testing.T) {
	for _, spec := range []struct {
		provider string
		altKey   string
	}{
		{"copilot", "cmd"},
		{"antigravity", "CommandLine"},
	} {
		for _, tc := range []struct {
			name string
			args map[string]any
			want shellwords.ExecutionContext
		}{
			{"matching alt key", map[string]any{spec.altKey: "echo ok"}, proven(shellwords.PowerShell)},
			{"matching command", map[string]any{"command": "echo ok"}, proven(shellwords.PowerShell)},
			{"both matching", map[string]any{spec.altKey: "echo ok", "command": "echo ok"}, proven(shellwords.PowerShell)},
			{"competing alt and command", map[string]any{spec.altKey: "echo ok", "command": "other"}, refused(shellwords.PowerShell, "competing command evidence")},
			{"mismatched alt vs root", map[string]any{spec.altKey: "other"}, refused(shellwords.PowerShell, "competing command evidence")},
			{"non-string alt", map[string]any{spec.altKey: nil}, refused(shellwords.PowerShell, "competing command evidence")},
			{"non-string command", map[string]any{spec.altKey: "echo ok", "command": nil}, refused(shellwords.PowerShell, "competing command evidence")},
		} {
			t.Run(spec.provider+"/"+tc.name, func(t *testing.T) {
				input := map[string]any{"command": "echo ok", "value": tc.args}
				if got := executionCall(t, spec.provider, "PowerShell", nil, input).Execution(); got != tc.want {
					t.Fatalf("context=%+v; want %+v", got, tc.want)
				}
			})
		}
	}
}

// stubReader is a script reader the options carry and never call.
type stubReader struct{ name string }

func (*stubReader) ReadScript(string) shellwords.ScriptRead { return shellwords.ScriptRead{} }

// cwdCall reads a Bash call on claude-code with this event cwd and input.
func cwdCall(t *testing.T, cwd *string, input map[string]any) Call {
	t.Helper()
	return executionCall(t, "claude-code", "Bash", cwd, input)
}

func TestScriptOptionsDisabledWithoutReader(t *testing.T) {
	root := t.TempDir()
	call := cwdCall(t, &root, map[string]any{"command": "echo ok"})
	if got := call.ScriptOptions(nil); got != (shellwords.ScriptOptions{}) {
		t.Fatalf("a nil reader must yield zero options, got %+v", got)
	}
}

func TestScriptOptionsCarriesReaderAndContext(t *testing.T) {
	root := t.TempDir()
	reader := &stubReader{name: "carried"}
	call := cwdCall(t, &root, map[string]any{"command": "echo ok"})
	got := call.ScriptOptions(reader)
	if got.Reader != shellwords.ScriptReader(reader) || got.Context != call.Execution() || got.Cwd != root {
		t.Fatalf("got %+v", got)
	}
	if got.Context != proven(shellwords.POSIX) || got.Host != (pathshape.Host{}) {
		t.Fatalf("context or host: %+v", got)
	}
}

func TestScriptOptionsCwdIsProvenOnlyWhenArgumentsAgree(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	reader := &stubReader{name: "agree"}

	agreeCall := cwdCall(t, &root, map[string]any{"cwd": root})
	if agree := agreeCall.ScriptOptions(reader); agree.Cwd != root || agree.Context != agreeCall.Execution() {
		t.Fatalf("agreeing cwd rejected: %+v", agree)
	}
	differCall := cwdCall(t, &root, map[string]any{"cwd": other})
	if differ := differCall.ScriptOptions(reader); differ.Cwd != "" || differ.Reader == nil || differ.Context != differCall.Execution() {
		t.Fatalf("competing cwd trusted or reader dropped: %+v", differ)
	}
	noneCall := cwdCall(t, nil, map[string]any{})
	if none := noneCall.ScriptOptions(reader); none.Cwd != "" || none.Reader == nil || none.Context != noneCall.Execution() {
		t.Fatalf("missing cwd trusted: %+v", none)
	}
	empty := ""
	if got := cwdCall(t, &empty, map[string]any{}).ScriptOptions(reader); got.Cwd != "" {
		t.Fatalf("empty cwd trusted: %+v", got)
	}
}

func TestProvenCwdRejectsEveryDisagreement(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	cases := map[string]map[string]any{
		"workdir differs":      {"workdir": other},
		"cwd differs":          {"cwd": other},
		"nested cwd differs":   {"value": map[string]any{"cwd": other}},
		"nested workdir":       {"value": map[string]any{"workdir": other}},
		"non-string cwd":       {"cwd": json.Number("7")},
		"relative cwd":         {"cwd": "rel/dir"},
		"one agrees one fails": {"workdir": root, "cwd": other},
	}
	for name, input := range cases {
		if got := cwdCall(t, &root, input).ProvenCwd(); got != "" {
			t.Errorf("%s: trusted %q", name, got)
		}
	}
	trusted := map[string]map[string]any{
		"no arguments": {},
		"nil cwd":      {"cwd": nil},
		"empty cwd":    {"cwd": ""},
		"both agree":   {"workdir": root, "cwd": root, "value": map[string]any{"cwd": root}},
	}
	for name, input := range trusted {
		if got := cwdCall(t, &root, input).ProvenCwd(); got != root {
			t.Errorf("%s: got %q, want %q", name, got, root)
		}
	}
}

func TestProvenCwdIgnoresCaseOnlyOnWindows(t *testing.T) {
	root := t.TempDir()
	swapped := strings.ToUpper(root)
	if swapped == root {
		t.Skip("temp dir has no letters to change")
	}
	got := cwdCall(t, &root, map[string]any{"cwd": swapped}).ProvenCwd()
	if runtime.GOOS == "windows" {
		if got != root {
			t.Fatalf("case difference rejected on windows: %q", got)
		}
		return
	}
	if got != "" {
		t.Fatalf("case difference trusted: %q", got)
	}
}

// A relative event cwd names no single directory, so it is unproven with or
// without a tool override, while the absolute spellings the guards accept,
// including /ws and C:/ws on a Windows host, are returned unchanged.
func TestProvenCwdRequiresAnAbsoluteEventCwd(t *testing.T) {
	for _, cwd := range []string{".", "sub/dir", "..", `sub\dir`, "./ws"} {
		for name, input := range map[string]map[string]any{
			"no override":       {},
			"agreeing override": {"cwd": cwd},
			"absolute override": {"workdir": "/ws"},
		} {
			if got := cwdCall(t, &cwd, input).ProvenCwd(); got != "" {
				t.Errorf("%q with %s: trusted %q", cwd, name, got)
			}
		}
		if got := cwdCall(t, &cwd, map[string]any{}).ScriptOptions(&stubReader{name: "relative"}); got.Cwd != "" || got.Reader == nil {
			t.Errorf("%q: options %+v", cwd, got)
		}
	}
	for _, cwd := range []string{"/ws", "C:/ws", `C:\ws`, "/c/ws", "//server/share"} {
		if got := cwdCall(t, &cwd, map[string]any{}).ProvenCwd(); got != cwd {
			t.Errorf("%q: got %q, want it unchanged", cwd, got)
		}
	}
}

// A relative directory that agrees textually with the event cwd is still
// unproven: only an absolute path names one place.
func TestProvenCwdRejectsAgreeingRelativeDirectory(t *testing.T) {
	cwd := "rel/dir"
	if got := cwdCall(t, &cwd, map[string]any{"cwd": "rel/dir"}).ProvenCwd(); got != "" {
		t.Fatalf("relative cwd trusted: %q", got)
	}
}

func TestProvenCwdIsEmptyWithoutAnEventCwd(t *testing.T) {
	empty := ""
	for name, cwd := range map[string]*string{"nil": nil, "empty": &empty} {
		if got := cwdCall(t, cwd, map[string]any{}).ProvenCwd(); got != "" {
			t.Errorf("%s event cwd: got %q", name, got)
		}
	}
}
