package secretsguard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/scriptsnap"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/surface"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

func TestBashInspectionCancelled(t *testing.T) {
	for _, at := range []int{1, 4, 10} {
		ctx := &hookspectest.CancelContext{Context: context.Background(), At: at}
		command := "bash" + strings.Repeat(" ", 6*64*1024) + "--help"
		out := Run(hookspec.Request{Context: ctx, Event: buildEvent(t, "Bash", map[string]any{"command": command})})
		if ctx.Calls < at || out.Kind() != "deny" {
			t.Fatalf("cancellation granted permission: %d/%d %s", ctx.Calls, at, out.Kind())
		}
	}
}

func TestBashInspectionPrecision(t *testing.T) {
	for _, command := range []string{"bash --help", "bash --version"} {
		kind, message := run(t, buildEvent(t, "Bash", map[string]any{"command": command}), nil)
		if kind != "pass" {
			t.Fatalf("metadata: %s %s", kind, message)
		}
	}
	kind, message := run(t, buildEvent(t, "Bash", map[string]any{"command": "bash --help; cat .env"}), nil)
	if kind != "deny" || !strings.Contains(message, "credential file read(s) blocked") {
		t.Fatalf("credential read erased: %s %s", kind, message)
	}
	for _, input := range []map[string]any{
		{"command": "bash --help", "shell": "bash"},
		{"command": "bash --help", "env": map[string]any{}},
		{"command": "bash --help", "metadata": map[string]any{"shell": "bash"}},
		{"command": "alias bash=helper; bash --help"},
		{"command": "PATH=other; bash --help"},
		{"command": "echo public; bash --help"},
	} {
		kind, message := run(t, buildEvent(t, "Bash", input), nil)
		if kind != "deny" || !strings.Contains(message, "inspection authority unavailable") {
			t.Fatalf("unqualified proof: %s %s", kind, message)
		}
	}
}

func TestSyntaxReadPrecision(t *testing.T) {
	root := globFixture(t, "source.sh", ".env")
	for _, tc := range []struct{ command, want, reason string }{
		{"bash --version", "pass", ""},
		{"bash --help", "pass", ""},
		{"bash -n source.sh", "pass", ""},
		{"bash -n -- source.sh", "pass", ""},
		{`bash -n '.e[n]v'`, "pass", ""},
		{`bash -n '.e'*`, "deny", "recognized execution path"},
		{"bash -n .env", "deny", "credential file read(s) blocked"},
		{"bash -n -- .env", "deny", "credential file read(s) blocked"},
		{"bash source.sh", "deny", "recognized execution path"},
		{"bash -n source.sh; cat .env", "deny", "credential file read(s) blocked"},
		{"bash -n source.sh <.env", "deny", "recognized execution path"},
		{"bash -n source.sh; echo " + fakeAWSKey, "deny", "AKIA*"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": tc.command}, root), nil)
			if kind != tc.want || !strings.Contains(message, tc.reason) {
				t.Fatalf("outcome = %s %s; want %s containing %q", kind, message, tc.want, tc.reason)
			}
		})
	}
	t.Run("literal operand provenance", func(t *testing.T) {
		hosttest.ScrubGuardEnv(t)
		command := `bash -n '.e[n]v'`
		call := readCall(t, buildEventAt(t, "Bash", map[string]any{"command": command}, root))
		ctx := context.Background()
		inspection := inspectShellReads(ctx, call, call.Inspect(ctx, nil))
		if inspection.Reason.Refused() || len(inspection.Paths) != 1 || inspection.Paths[0].Text != ".e[n]v" || inspection.Paths[0].ExpandPattern || inspection.Paths[0].Cwd != root {
			t.Fatalf("literal syntax operand lost: %#v", inspection)
		}
	})
	t.Run("canceled publication", func(t *testing.T) {
		hosttest.ScrubGuardEnv(t)
		command := "bash -n source.sh"
		call := readCall(t, buildEventAt(t, "Bash", map[string]any{"command": command}, root))
		baseline := &hookspectest.CancelContext{Context: context.Background()}
		inspection := inspectShellReads(baseline, call, call.Inspect(baseline, nil))
		if inspection.Reason.Refused() {
			t.Fatal(inspection.Reason)
		}
		ctx := &hookspectest.CancelContext{Context: context.Background(), At: baseline.Calls}
		inspection = inspectShellReads(ctx, call, call.Inspect(ctx, nil))
		if ctx.Calls < ctx.At || inspection.Reason.String() != "Bash inspection canceled" {
			t.Fatalf("canceled syntax proof published: %d/%d %#v", ctx.Calls, ctx.At, inspection)
		}
	})
}

func TestPowerShellFileDataPrecision(t *testing.T) {
	command := "@'\ndon't; cat .env; 'literal\n'@ | Set-Content -LiteralPath draft.txt"
	kind, message := run(t, buildEvent(t, "PowerShell", map[string]any{"command": command}), nil)
	if kind != "pass" {
		t.Fatalf("literal data = %s %s", kind, message)
	}
	for _, unsafe := range []string{
		command + "; Get-Content .env",
		"@'\nordinary text\n'@ | Set-Content -LiteralPath draft.txt; Get-Content .env",
		strings.Replace(command, "Set-Content -LiteralPath draft.txt", "Invoke-Expression", 1),
	} {
		kind, message := run(t, buildEvent(t, "PowerShell", map[string]any{"command": unsafe}), nil)
		if kind != "deny" || !strings.Contains(message, "credential file read(s) blocked") {
			t.Fatalf("actual read lost: %s %s", kind, message)
		}
	}
	kind, message = run(t, buildEvent(t, "PowerShell", map[string]any{"command": "@'\n" + fakeAWSKey + "\n'@ | Set-Content -LiteralPath draft.txt"}), nil)
	if kind != "deny" || !strings.Contains(message, "AKIA*") || strings.Contains(message, fakeAWSKey) {
		t.Fatalf("original secret scan lost: %s %s", kind, message)
	}
	kind, message = run(t, buildEvent(t, "PowerShell", map[string]any{"command": command, "shell": "pwsh"}), nil)
	if kind != "deny" || !strings.Contains(message, "credential file read(s) blocked") {
		t.Fatalf("unknown context granted proof: %s %s", kind, message)
	}
}

func TestPowerShellFileDataCancelled(t *testing.T) {
	for _, stage := range []string{"body", "suffix"} {
		t.Run(stage, func(t *testing.T) {
			command := "@'\ndon't; cat .env; 'literal\n" + strings.Repeat("x", 6*64*1024) + "\n'@ | Set-Content -LiteralPath draft.txt" + strings.Repeat(" ", 6*64*1024)
			baseline := &hookspectest.CancelContext{Context: context.Background()}
			shellwords.ParseContext(baseline, command, shellwords.PowerShell)
			if stage == "suffix" {
				shellwords.PowerShellLiteralData(baseline, command)
			}
			ctx := &hookspectest.CancelContext{Context: context.Background(), At: baseline.Calls + 4}
			out := Run(hookspec.Request{Context: ctx, Event: buildEvent(t, "PowerShell", map[string]any{"command": command})})
			if ctx.Calls < ctx.At || out.Kind() != "deny" || !strings.Contains(out.Payload(), "credential file read(s) blocked") {
				t.Fatalf("cancellation lost original read: checks=%d/%d %s %s", ctx.Calls, ctx.At, out.Kind(), out.Payload())
			}
		})
	}
}

func TestPowerShellEnvironmentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	event := buildEvent(t, "PowerShell", map[string]any{"command": `[Environment]::SetEnvironmentVariable('EXAMPLE','ordinary')`})
	out := Run(hookspec.Request{Context: ctx, Event: event})
	if out.Kind() != "deny" || !strings.Contains(out.Payload(), "canceled") {
		t.Fatalf("canceled inspection = %s %s", out.Kind(), out.Payload())
	}
}

func TestExecutionContextPrecisionRetainsProtection(t *testing.T) {
	for _, prefix := range []string{"", "opaque-helper; ", "Set-Alias rg helper; ", "function rg { helper }; "} {
		for _, tool := range []string{"PowerShell"} {
			payload := buildEvent(t, tool, map[string]any{"command": prefix + `rg --no-config '.env' source.txt; Get-Content .env`, "shell": nil})
			kind, message := run(t, payload, nil)
			if kind != "deny" || message == "" {
				t.Fatalf("protected read lost: %s %s", kind, message)
			}
		}
	}
}

func TestPowerShellEnvironmentLargeCancellation(t *testing.T) {
	for _, at := range []int{0, 4, 12} {
		t.Run(fmt.Sprint(at), func(t *testing.T) {
			ctx := &hookspectest.CancelContext{Context: context.Background(), At: at}
			command := `[Environment]::SetEnvironmentVariable('EXAMPLE','local-value-marker` + strings.Repeat("x", 256*1024) + `')`
			event := buildEvent(t, "PowerShell", map[string]any{"command": command})
			out := Run(hookspec.Request{Context: ctx, Event: event})
			if at == 0 {
				if out.Kind() != "pass" {
					t.Fatalf("normal large call: %s %s", out.Kind(), out.Payload())
				}
			} else if out.Kind() != "deny" || !strings.Contains(out.Payload(), "canceled") || strings.Contains(out.Payload(), "local-value-marker") {
				t.Fatalf("canceled large call did not return sanitized inspection denial: %s", out.Kind())
			}
		})
	}
}

func TestTask06QuoteProvenanceDoesNotDropActiveGlob(t *testing.T) {
	inspection := shellReads(t, "Bash", `cat '.e[n]v' .e[n]v`, globFixture(t, ".env"))
	if inspection.Reason.Refused() || len(inspection.Paths) != 2 || inspection.Paths[0].ExpandPattern || !inspection.Paths[1].ExpandPattern {
		t.Fatalf("inspection = %#v", inspection)
	}
	findings, err := collectShell(inspection.Paths, nil, nil)
	if err != nil || len(findings) != 1 || findings[0].path != `.e[n]v` {
		t.Fatalf("findings = %#v, want one active glob finding", findings)
	}
}

func TestR04ShellReadCandidates(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"input redirect", `cat <.env`, ".env"},
		{"quoted redirect", `cat 0<".env"`, ".env"},
		{"literal bracket pattern", `cat .e[n]v`, ".e[n]v"},
		{"git show", `git -C repo show HEAD:.env`, ".env"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inspection := shellReads(t, "Bash", tc.command, "")
			if inspection.Reason.Refused() || !slices.ContainsFunc(inspection.Paths, func(path shellPath) bool { return path.Text == tc.want }) {
				t.Fatalf("inspection = %#v, want %q", inspection, tc.want)
			}
		})
	}
}

func TestR04ShellReadControls(t *testing.T) {
	for _, command := range []string{
		`printf '%s' 'cat <.env'`,
		`cat .env.example`,
		`git show HEAD:README.md`,
		"cat >note <<'EOF'\n.env\nEOF\n",
		"cat <<'.env'\nsynthetic data\n.env\n",
		`cat <<< '.env'`,
	} {
		t.Run(command, func(t *testing.T) {
			inspection := shellReads(t, "Bash", command, "")
			if inspection.Reason.Refused() {
				t.Fatalf("reason = %q", inspection.Reason)
			}
			for _, candidate := range inspection.Paths {
				if finding, ok := classifyLiteralCandidate(candidate, inspection.Overrides); ok {
					t.Fatalf("finding = %#v", finding)
				}
			}
		})
	}
}

func TestHeredocReadRedirectKindsDoNotBecomeCredentialPaths(t *testing.T) {
	for _, command := range []string{"cat <<'.env'\nsynthetic data\n.env\n", `cat <<< '.env'`} {
		inspection := shellReads(t, "Bash", command, "")
		if inspection.Reason.Refused() || len(inspection.Paths) != 0 {
			t.Fatalf("inspection for %q = %#v, want no file input path", command, inspection)
		}
	}

	inspection := shellReads(t, "Bash", "cat <<EOF\n$(cat .env)\nEOF\n", "")
	if !slices.ContainsFunc(inspection.Paths, func(path shellPath) bool { return path.Text == ".env" }) {
		t.Fatalf("active substitution inspection = %#v, want .env path", inspection)
	}
}

func TestGitObjectGlobClassificationDoesNotTouchFilesystem(t *testing.T) {
	got, ok := classifyLiteralCandidate(shellPath{Text: `config/.e[n]v`, ExpandPattern: true, GitObject: true}, nil)
	if !ok || got.category != "dotenv" || got.path != `config/.e[n]v` {
		t.Fatalf("finding = %#v, ok = %v", got, ok)
	}
	if _, ok := classifyLiteralCandidate(shellPath{Text: `config/READM[E].md`, ExpandPattern: true, GitObject: true}, nil); ok {
		t.Fatal("ordinary literal glob classified as credential")
	}
}

// Every name in the pattern roster, with its last character bracketed, is
// denied behind `git show`, so a name added to a table cannot be missed by the
// pattern path. The expected category comes from classifying the literal path.
func TestGitObjectPatternCoversEveryTableName(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	for _, name := range patternCandidates {
		dir := ""
		for _, explicit := range explicitSuffixes {
			if path.Base(explicit.suffix) == name {
				dir = strings.TrimPrefix(path.Dir(explicit.suffix), "/") + "/"
			}
		}
		literal := dir + name
		want, ok := classify(literal)
		if !ok {
			t.Fatalf("classify(%q) found no credential", literal)
		}
		pattern := literal[:len(literal)-1] + "[" + literal[len(literal)-1:] + "]"
		t.Run(name, func(t *testing.T) {
			kind, message := run(t, buildEvent(t, "Bash", map[string]any{"command": "git show HEAD:" + pattern}), nil)
			if kind != "deny" || !strings.Contains(message, "("+want.category+", ") {
				t.Fatalf("git show HEAD:%s: kind=%s message=%s want deny naming %s", pattern, kind, message, want.category)
			}
		})
	}
}

func TestQuotedPOSIXPatternStaysLiteral(t *testing.T) {
	inspection := shellReads(t, "Bash", `cat '.e[n]v' .e[n]v`, globFixture(t, ".env"))
	if inspection.Reason.Refused() || len(inspection.Paths) != 2 || inspection.Paths[0].ExpandPattern || !inspection.Paths[1].ExpandPattern {
		t.Fatalf("inspection = %#v", inspection)
	}
}

func TestPOSIXMixedQuoteGlobProvenance(t *testing.T) {
	cases := []struct {
		name, command string
		expand        bool
	}{
		{"unquoted bracket expands", `cat ".e"[n]v`, true},
		{"quoted bracket is inert", `cat .e"[n]"v`, false},
		{"escaped bracket is inert", `cat .e\[n\]v`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inspection := shellReads(t, "Bash", tc.command, "")
			if inspection.Reason.Refused() || len(inspection.Paths) != 1 || inspection.Paths[0].ExpandPattern != tc.expand {
				t.Fatalf("inspection = %#v, want ExpandPattern=%v", inspection, tc.expand)
			}
		})
	}
}

func TestPOSIXMixedQuoteGlobDoesNotReactivateQuotedMetacharacters(t *testing.T) {
	inspection := shellReads(t, "Bash", `cat .e"[n]"*`, "")
	if inspection.Reason.Refused() || len(inspection.Paths) != 1 || !inspection.Paths[0].ExpandPattern {
		t.Fatalf("inspection = %#v, want one active trailing glob", inspection)
	}
	if finding, ok := classifyLiteralCandidate(inspection.Paths[0], nil); ok {
		t.Fatalf("finding = %#v, want quoted bracket to remain literal", finding)
	}
	inspection = shellReads(t, "Bash", `cat .e"[n]"* .e[n]*`, globFixture(t, ".env"))
	findings, err := collectShell(inspection.Paths, nil, nil)
	if err != nil || len(findings) != 1 || findings[0].path != `.e[n]*` {
		t.Fatalf("findings = %#v, want only the active second glob", findings)
	}
}

func TestPowerShellLiteralPathDisablesPatternExpansion(t *testing.T) {
	inspection := shellReads(t, "PowerShell", `Get-Content -LiteralPath '.e[n]v'`, "")
	if inspection.Reason.Refused() || len(inspection.Paths) != 1 || inspection.Paths[0].ExpandPattern {
		t.Fatalf("inspection = %#v", inspection)
	}
}

func TestPowerShellReaderPathElements(t *testing.T) {
	for _, command := range []string{
		`Get-Content -LiteralPath 'notes.txt','.env'`,
		`Get-Content -LiteralPath:'notes.txt','.env'`,
		`Get-Content -Lit:'notes.txt','.env'`,
		`Get-Content -Pat:'notes.txt','.env'`,
	} {
		inspection := shellReads(t, "PowerShell", command, "")
		findings, err := collectShell(inspection.Paths, nil, nil)
		if inspection.Reason.Refused() || err != nil || len(findings) != 1 || findings[0].path != ".env" {
			t.Errorf("%q: expected one classified path, got paths %#v, findings %#v, error %v", command, inspection.Paths, findings, err)
		}
	}
	inspection := shellReads(t, "PowerShell", `Get-Content -LiteralPath 'notes.txt,.env'`, "")
	if inspection.Reason.Refused() || len(inspection.Paths) != 1 || inspection.Paths[0].Text != "notes.txt,.env" {
		t.Fatalf("quoted comma must stay in one path, got %#v", inspection)
	}
}

func TestPowerShellReaderAliasPathElements(t *testing.T) {
	for _, command := range []string{
		"type alpha.txt,beta.txt",
		"copy alpha.txt,beta.txt output",
		"move alpha.txt,beta.txt output",
	} {
		inspection := shellReads(t, "PowerShell", command, "")
		if inspection.Reason.Refused() || len(inspection.Paths) < 2 || inspection.Paths[0].Text != "alpha.txt" || inspection.Paths[1].Text != "beta.txt" {
			t.Errorf("%q: expected two separate leading paths, got %#v", command, inspection)
		}
	}
}

func TestDirectScriptReaderUsesFileDialect(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "report")
	src := "#!/usr/bin/env pwsh\nGet-Content -LiteralPath 'one.txt','two.txt'\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	call := readCall(t, buildEventAt(t, "Bash", map[string]any{"command": "./report"}, dir))
	ctx := t.Context()
	result := inspectShellReads(ctx, call, call.Inspect(ctx, scriptsnap.New()))
	if result.Reason.Refused() || len(result.Paths) != 2 {
		t.Fatalf("expected both PowerShell operands from the inspected script, got %#v", result)
	}
	for i, want := range []string{"one.txt", "two.txt"} {
		p := result.Paths[i]
		if p.Text != want || p.Dialect != shellwords.PowerShell || p.ExpandPattern || p.Origin == nil || p.Origin.Line != 2 {
			t.Errorf("operand %d: got %#v, want literal PowerShell path %q from line 2", i, p, want)
		}
	}
}

func TestCanceledPowerShellDataProofStillDeniesInWarnMode(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	t.Setenv("SECRETS_GUARD_MODE", "warn")
	const command = `Get-Content -LiteralPath .env`
	event := buildEvent(t, "PowerShell", map[string]any{"command": command})
	call := readCall(t, event)
	checkpoint := &hookspectest.CancelContext{Context: t.Context()}
	shellwords.ParseScripts(checkpoint, command, shellwords.PowerShell, call.ScriptOptions(scriptsnap.New()))
	ctx := &hookspectest.CancelContext{Context: t.Context(), At: checkpoint.Calls + 1}
	control := Run(hookspec.Request{Context: t.Context(), Event: event, Scripts: scriptsnap.New()})
	if control.Kind() != contract.KindContext {
		t.Fatalf("warn-mode control returned %v, want context", control.Kind())
	}
	out := Run(hookspec.Request{Context: ctx, Event: event, Scripts: scriptsnap.New()})
	if out.Kind() != contract.KindDeny || !strings.Contains(out.Payload(), "credential file read(s) blocked") {
		t.Errorf("canceled data proof returned %v with %q, want denial preserving the credential finding", out.Kind(), out.Payload())
	}
}

func TestPowerShellCanonicalLocationGlob(t *testing.T) {
	root := globFixture(t, "docs/sample.txt")
	inspection := shellReads(t, "PowerShell", `Set-Location docs && Get-Content *.txt`, root)
	findings, err := collectShell(inspection.Paths, nil, nil)
	if inspection.Reason.Refused() || err != nil || len(findings) != 0 {
		t.Fatalf("expected benign glob resolved from the new location, got paths %#v, findings %#v, error %v", inspection.Paths, findings, err)
	}
}

func TestLiteralOverrideWithoutReadIsOnlyComparisonState(t *testing.T) {
	result := shellReads(t, "Bash", `PGPASSFILE=/tmp/custom echo ok`, "")
	if result.Reason.Refused() || len(result.Overrides) != 1 || result.Overrides[0] != "/tmp/custom" || len(result.Paths) != 0 {
		t.Fatalf("inspection = %#v", result)
	}
	if findings, err := collectShell(result.Paths, nil, result.Overrides); err != nil || len(findings) != 0 {
		t.Fatalf("findings = %#v, want pass", findings)
	}
}

func TestLiteralOverrideDeniesMatchingRead(t *testing.T) {
	result := shellReads(t, "Bash", `PGPASSFILE=/tmp/custom cat /tmp/custom`, "")
	if result.Reason.Refused() || len(result.Paths) != 1 || len(result.Overrides) != 1 {
		t.Fatalf("inspection = %#v", result)
	}
	findings, err := collectShell(result.Paths, nil, result.Overrides)
	if err != nil || len(findings) != 1 || findings[0].category != "credential_path_override" || findings[0].severity != sevCritical {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestRecognizedUninspectableReadWrapperReturnsReason(t *testing.T) {
	inspection := shellReads(t, "Bash", `env -S 'cat .env'`, "")
	if len(inspection.Paths) != 0 || !inspection.Reason.Refused() {
		t.Fatalf("inspection = %#v", inspection)
	}
}

// Fake credentials for the write-direction tests, built by concatenation so
// this source file cannot match the very table it exercises — the same
// technique patterns.go uses for its PGP literal. The runtime values match;
// the literals in this file do not.
var (
	fakeAWSKey         = "AKIA" + "IOSFODNN7AAAAAAA"
	fakeGitHubPAT      = "ghp_" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fakePlaceholderKey = "AKIA" + "IOSFODNN7EXAMPLE"
)

func buildEvent(t *testing.T, toolName string, input map[string]any) hookspec.Event {
	t.Helper()
	return buildEventAt(t, toolName, input, "/tmp/agentics-secretsguard")
}

func buildEventAt(t *testing.T, toolName string, input map[string]any, cwd string) hookspec.Event {
	t.Helper()
	session := "secretsguard-core-session"
	payload, err := contract.NewEvent(contract.EventParts{
		Surface:   surface.ClaudeCode,
		SessionID: &session,
		Cwd:       &cwd,
		ToolName:  toolName,
		ToolInput: input,
	})
	if err != nil {
		t.Fatalf("constructing the event fixture: %v", err)
	}
	return payload
}

// readCall reads payload as Run does, so a test that drives the shell
// inspection directly gets the execution context and working directory Run
// would derive from the same event.
func readCall(t *testing.T, payload hookspec.Event) toolcall.Call {
	t.Helper()
	event, err := payload.Read()
	if err != nil {
		t.Fatalf("reading the event fixture: %v", err)
	}
	call, err := toolcall.Read(event)
	if err != nil {
		t.Fatalf("reading the tool call: %v", err)
	}
	return call
}

// shellReads gathers the shell reads of one command through the same inspection
// Run uses, with the host guard environment scrubbed. With cwd "" the event
// carries no usable working directory.
func shellReads(t *testing.T, tool, command, cwd string) shellInspection {
	t.Helper()
	hosttest.ScrubGuardEnv(t)
	call := readCall(t, buildEventAt(t, tool, map[string]any{"command": command}, cwd))
	ctx := context.Background()
	return inspectShellReads(ctx, call, call.Inspect(ctx, nil))
}

func run(t *testing.T, payload hookspec.Event, env map[string]string) (kind, message string) {
	t.Helper()
	hosttest.ScrubGuardEnv(t)
	for key, value := range env {
		t.Setenv(key, value)
	}
	var diag bytes.Buffer
	outcome := Run(hookspec.Request{
		Provider: "claude-code",
		Hook:     "secrets-guardian",
		Event:    payload,
		Diag:     &diag,
	})
	if !outcome.Valid() {
		t.Fatalf("invalid outcome: %#v", outcome)
	}
	return outcome.Kind(), outcome.Payload()
}

// A literal assignment of any host credential override name, followed by a
// read of the assigned path, is denied as a credential path override.
func TestEveryOverrideNameGatesALiteralAssignment(t *testing.T) {
	for _, name := range hostCredentialOverrideNames {
		t.Run(name, func(t *testing.T) {
			command := name + "=/tmp/custom cat /tmp/custom"
			kind, message := run(t, buildEvent(t, "Bash", map[string]any{"command": command}), nil)
			if kind != "deny" || !strings.Contains(message, "credential_path_override") {
				t.Fatalf("%s: kind=%s message=%s; want a deny naming credential_path_override", command, kind, message)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// classify

func TestClassifyMatchesCredentialPaths(t *testing.T) {
	clearCredentialPathEnv(t)
	cases := []struct{ path, category string }{
		{".env", "dotenv"},
		{"/srv/app/.env", "dotenv"},
		{".env.production", "dotenv"},
		{"/home/u/.ssh/id_rsa", "ssh_private_key"},
		{"id_ed25519", "ssh_private_key"},
		{"/home/u/.ssh/id_custom_deploy", "ssh_private_key"},
		{"certs/server.pem", "private_key_file"},
		{"a/b/private.key", "private_key_file"},
		{"bundle.pfx", "private_key_file"},
		{"/home/u/.aws/credentials", "cloud_credentials"},
		{"/home/u/.config/gcloud/application_default_credentials.json", "cloud_credentials"},
		{"/home/u/.kube/config", "kubeconfig"},
		{"staging.kubeconfig", "kubeconfig"},
		{"/home/u/.gnupg/secring.gpg", "gpg_keyring"},
		{"/home/u/.docker/config.json", "registry_credentials"},
		{".npmrc", "registry_credentials"},
		{".netrc", "netrc"},
		{".git-credentials", "git_credentials"},
		{"infra/terraform.tfstate", "terraform_state"},
		{"infra/terraform.tfstate.backup", "terraform_state"},
		// Case variants resolve to the same file on macOS and Windows, so a
		// differing case is evasion rather than a different file.
		{"/home/u/.SSH/ID_RSA", "ssh_private_key"},
		{".ENV", "dotenv"},
		// Backslashes are the same path on Windows.
		{`C:\Users\u\.aws\credentials`, "cloud_credentials"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got, ok := classify(tc.path)
			if !ok {
				t.Fatalf("classify(%q) = not credential, want %s", tc.path, tc.category)
			}
			if got.category != tc.category {
				t.Errorf("category = %q, want %q", got.category, tc.category)
			}
		})
	}
}

// The exemptions are the whole reason this guard is usable. A template is
// committed on purpose, holds placeholders, and is read constantly.
func TestClassifyLetsTemplatesAndPublicKeysThrough(t *testing.T) {
	clearCredentialPathEnv(t)
	for _, p := range []string{
		".env.example", ".env.sample", ".env.template", ".env.dist",
		"/srv/.env.example", ".env.defaults",
		"/home/u/.ssh/id_rsa.pub", "id_ed25519.pub",
		"README.md", "src/main.go", "package.json", "config.yaml",
		"environment.ts", "prevented.key.md", "docs/dotenv.md",
	} {
		t.Run(p, func(t *testing.T) {
			if got, ok := classify(p); ok {
				t.Errorf("classify(%q) = %s, want not credential", p, got.category)
			}
		})
	}
}

func TestCredentialPathContract(t *testing.T) {
	clearCredentialPathEnv(t)
	blocked := []string{
		".pgpass", ".my.cnf", ".vault-token",
		"/home/u/.config/gh/hosts.yml",
		"/home/u/.terraform.d/credentials.tfrc.json",
		"/home/u/.config/sops/age/keys.txt",
		`C:\Users\u\AppData\Roaming\sops\age\keys.txt`,
		`"/Users/u/Library/Application Support/sops/age/keys.txt"`,
		"application_default_credentials.json",
		"credentials.example", "server.key.example",
	}
	for _, candidate := range blocked {
		if _, ok := classify(candidate); !ok {
			t.Errorf("classify(%q) passed, want credential", candidate)
		}
	}
	allowed := []string{
		".env.example", ".env.sample", ".sops.yaml", "age1publicrecipient",
		"production.tfvars", "service.key.pub", "/home/u/.config/gh/config.yml",
	}
	for _, candidate := range allowed {
		if got, ok := classify(candidate); ok {
			t.Errorf("classify(%q) = %s, want pass", candidate, got.category)
		}
	}
	if _, ok := classifyWithOverrides("/tmp/custom-auth", []string{"/tmp/custom-auth"}); !ok {
		t.Fatal("exact host/literal override path passed")
	}
	if _, ok := classifyWithOverrides("/tmp/other", []string{"/tmp/custom-auth"}); ok {
		t.Fatal("override broadened beyond its exact literal path")
	}
	if got, ok := classifyWithOverrides(`"C:\\Users\\U\\vault\\..\\custom-auth"`, []string{`c:/users/u/custom-auth`}); !ok || got.category != "credential_path_override" {
		t.Fatalf("Windows override normalization = %#v, %t; want credential_path_override", got, ok)
	}
	if got, ok := classifyWithOverrides(`'/Users/U/Library/Application Support/../custom-auth'`, []string{"/users/u/library/custom-auth"}); !ok || got.category != "credential_path_override" {
		t.Fatalf("quoted macOS override normalization = %#v, %t; want credential_path_override", got, ok)
	}
}

func TestR08ProtectedDirectoryTemplatesRemainCredentials(t *testing.T) {
	blocked := []struct {
		path, category string
	}{
		{"/synthetic/.aws/.env.example", "cloud_credentials"},
		{"/synthetic/.kube/config.sample", "kubeconfig"},
	}
	for _, tc := range blocked {
		t.Run("deny "+tc.path, func(t *testing.T) {
			got, ok := classify(tc.path)
			if !ok || got.category != tc.category {
				t.Fatalf("classify(%q) = %#v, %t; want %s", tc.path, got, ok, tc.category)
			}
		})
	}

	for _, p := range []string{"/synthetic/.env.example", "/synthetic/.env.sample", "/synthetic/readme.sample"} {
		t.Run("allow outside protected directory "+p, func(t *testing.T) {
			if got, ok := classify(p); ok {
				t.Fatalf("classify(%q) = %#v, want template control", p, got)
			}
		})
	}
}

func TestTerraformStateSuggestionUsesUnnamedRedactedOutput(t *testing.T) {
	got := suggestionFor("terraform_state")
	want := "Use `terraform output` without a name for redacted output, or `terraform state list` for structure"
	if got != want {
		t.Fatalf("suggestionFor(terraform_state) = %q, want %q", got, want)
	}
	if strings.Contains(got, "named") {
		t.Fatalf("suggestionFor(terraform_state) recommends named output: %q", got)
	}
}

func clearCredentialPathEnv(t *testing.T) {
	t.Helper()
	for _, key := range hostCredentialOverrideNames {
		t.Setenv(key, "")
	}
}

// ---------------------------------------------------------------------------
// Bash command parsing

// candidatePaths lists the path candidates the shell inspection reports for a
// POSIX command; the tests below pin that set.
func candidatePaths(t *testing.T, command string) []string {
	t.Helper()
	result := shellReads(t, "Bash", command, "")
	out := make([]string, len(result.Paths))
	for i, candidate := range result.Paths {
		out[i] = candidate.Text
	}
	return out
}

// The contract is deliberately loose on the POSITIVE side: a reader's
// non-path arguments (grep's pattern, a flag's value) are allowed through as
// candidates because classify rejects anything that is not credential
// material. Asserting an exact candidate list would pin an intermediate that
// is free to change. What must hold exactly is the negative side — a command
// that reads nothing must yield nothing, because that is what keeps the guard
// from firing on prose.
func TestPathsFromCommandFindsTheCredentialArgument(t *testing.T) {
	cases := []struct {
		name    string
		command string
		want    string
	}{
		{"cat", "cat .env", ".env"},
		{"after an operator", "echo hi && cat .env", ".env"},
		{"through a pipe", "cat .env | grep KEY", ".env"},
		{"sudo stripped", "sudo cat /root/.aws/credentials", "/root/.aws/credentials"},
		{"assignment stripped", "FOO=1 cat .env", ".env"},
		{"flag value present", "head -n 5 .env", ".env"},
		{"quoted", `cat ".env"`, ".env"},
		{"powershell", "Get-Content .env", ".env"},
		{"copy counts as a read", "cp .env /tmp/x", ".env"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := candidatePaths(t, tc.command)
			for _, candidate := range got {
				if candidate == tc.want {
					return
				}
			}
			t.Errorf("candidatePaths(%q) = %v, missing %q", tc.command, got, tc.want)
		})
	}
}

// These are the false positives that would get the guard switched off.
func TestPathsFromCommandIgnoresNonReaders(t *testing.T) {
	for _, command := range []string{
		`echo "add .env to .gitignore"`,
		"git add .env",
		"rm .env",
		"cat",
		"ls -la",
	} {
		t.Run(command, func(t *testing.T) {
			if got := candidatePaths(t, command); len(got) != 0 {
				t.Errorf("candidatePaths(%q) = %v, want no candidates", command, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Run

func TestRunDeniesACredentialRead(t *testing.T) {
	payload := buildEvent(t, "Read", map[string]any{"file_path": "/srv/app/.env"})
	kind, message := run(t, payload, nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	for _, want := range []string{"/srv/app/.env", "dotenv", "critical", ".env.example"} {
		if !strings.Contains(message, want) {
			t.Errorf("message missing %q:\n%s", want, message)
		}
	}
}

func TestRunWarnModeNeverDenies(t *testing.T) {
	payload := buildEvent(t, "Read", map[string]any{"file_path": ".env"})
	kind, _ := run(t, payload, map[string]string{"SECRETS_GUARD_MODE": "warn"})
	if kind != "context" {
		t.Errorf("kind = %q, want context — warn mode must never block", kind)
	}
}

func TestRunIgnoresToolsThatCannotSurfaceContent(t *testing.T) {
	payload := buildEvent(t, "WebFetch", map[string]any{"file_path": ".env"})
	if kind, _ := run(t, payload, nil); kind != "pass" {
		t.Errorf("kind = %q, want pass for an uninspected tool", kind)
	}
}

func TestGitShowPathsGlobalOptionArity(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"plain", []string{"show", "HEAD:.env"}, []string{".env"}},
		{"joined git dir", []string{"--git-dir=repo", "show", "HEAD:.env"}, []string{".env"}},
		{"joined work tree", []string{"--work-tree=repo", "show", "HEAD:.env"}, []string{".env"}},
		{"joined namespace", []string{"--namespace=show", "show", "HEAD:.env"}, []string{".env"}},
		{"joined exec path", []string{"--exec-path=helpers", "show", "HEAD:.env"}, []string{".env"}},
		{"bare exec path exits", []string{"--exec-path", "ignored", "show", "HEAD:.env"}, nil},
		{"unknown option", []string{"--unknown", "show", "HEAD:.env"}, nil},
		{"attached C", []string{"-Crepo", "show", "HEAD:.env"}, []string{".env"}},
		{"attached c", []string{"-ccore.quotePath=false", "show", "HEAD:.env"}, []string{".env"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			words := make([]shellwords.Word, len(tc.args))
			for i, arg := range tc.args {
				words[i] = shellwords.Word{Text: arg, Literal: true}
			}
			if got := gitShowObjectPaths(words); !slices.Equal(got, tc.want) {
				t.Fatalf("gitShowObjectPaths(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
	for _, option := range []string{"-C", "-c", "--git-dir", "--work-tree", "--namespace"} {
		t.Run(option, func(t *testing.T) {
			flag := shellwords.Word{Text: option, Literal: true}
			show := shellwords.Word{Text: "show", Literal: true}
			path := shellwords.Word{Text: "HEAD:.env", Literal: true}
			for _, words := range [][]shellwords.Word{
				{flag},
				{flag, show, path},
			} {
				if got := gitShowObjectPaths(words); len(got) != 0 {
					t.Fatalf("gitShowObjectPaths(%#v) = %q, want no paths", words, got)
				}
			}
			if got := gitShowObjectPaths([]shellwords.Word{flag, {Text: "$value"}, show, path}); !slices.Equal(got, []string{".env"}) {
				t.Fatalf("gitShowObjectPaths behind a non-literal value = %q, want [.env]", got)
			}
			if got := gitShowObjectPaths([]shellwords.Word{flag, show, show, path}); !slices.Equal(got, []string{".env"}) {
				t.Fatalf("value named show consumed incorrectly: %q", got)
			}
		})
	}
	for _, option := range []string{"--no-pager", "-P", "--paginate", "-p"} {
		t.Run(option+" joined rejected", func(t *testing.T) {
			words := []shellwords.Word{
				{Text: option + "=true", Literal: true},
				{Text: "show", Literal: true},
				{Text: "HEAD:.env", Literal: true},
			}
			if got := gitShowObjectPaths(words); len(got) != 0 {
				t.Fatalf("gitShowObjectPaths(%#v) = %q, want no paths", words, got)
			}
		})
	}
}

// gitShowWord builds a word whose Raw differs from its Text, so a reader that
// read Raw where it should read Text would give other paths.
func gitShowWord(text string, literal bool) shellwords.Word {
	if literal {
		return shellwords.Word{Text: text, Raw: "'" + text + "'", Quoted: true, Literal: true}
	}
	return shellwords.Word{Text: text, Raw: `"` + text + `"`, Quoted: true}
}

func gitShowWords(texts ...string) []shellwords.Word {
	words := make([]shellwords.Word, len(texts))
	for i, text := range texts {
		words[i] = gitShowWord(text, true)
	}
	return words
}

// TestGitShowObjectPathsGlobalOptions pins every global option the show reading
// walks past, in the shape git reads it, and the options on which git prints
// and exits, after which no word names an object path.
func TestGitShowObjectPathsGlobalOptions(t *testing.T) {
	flags := []string{
		"--no-pager", "-P", "--paginate", "-p",
		"--literal-pathspecs", "--glob-pathspecs", "--noglob-pathspecs", "--icase-pathspecs",
		"--no-optional-locks", "--bare", "--no-replace-objects", "--no-lazy-fetch", "--no-advice",
	}
	values := []string{"-C", "-c", "--git-dir", "--work-tree", "--namespace", "--config-env"}
	var lines [][]string
	for _, name := range flags {
		lines = append(lines, []string{name, "show", "HEAD:.env"})
	}
	for _, name := range values {
		lines = append(lines, []string{name, "v", "show", "HEAD:.env"})
	}
	lines = append(lines, []string{"--exec-path=x", "show", "HEAD:.env"})
	for _, texts := range lines {
		if got := gitShowObjectPaths(gitShowWords(texts...)); !slices.Equal(got, []string{".env"}) {
			t.Errorf("gitShowObjectPaths(%q) = %q, want [.env]", texts, got)
		}
	}
	for _, name := range []string{"--exec-path", "--html-path", "--man-path", "--info-path", "--help", "-h", "--version", "-v"} {
		forms := []string{name, name + "=x"}
		if name == "--exec-path" {
			forms = forms[:1]
		}
		for _, form := range forms {
			if got := gitShowObjectPaths(gitShowWords(form, "show", "HEAD:.env")); len(got) != 0 {
				t.Errorf("printing option %s still gives paths: %q", form, got)
			}
		}
	}
}

// TestGitShowObjectPathsPosture pins what the show reading returns for a line it
// cannot read as a show: no paths.
func TestGitShowObjectPathsPosture(t *testing.T) {
	for _, texts := range [][]string{
		{"--", "show", "HEAD:.env"},
		{"--unknown", "show", "HEAD:.env"},
		{"--help", "show", "HEAD:.env"},
		{"-C"},
		{"log", "HEAD:.env"},
		{},
	} {
		if got := gitShowObjectPaths(gitShowWords(texts...)); len(got) != 0 {
			t.Errorf("gitShowObjectPaths(%q) = %q, want no paths", texts, got)
		}
	}
	if got := gitShowObjectPaths([]shellwords.Word{gitShowWord("show", false), gitShowWord("HEAD:.env", true)}); len(got) != 0 {
		t.Errorf("a non-literal verb gave %q, want no paths", got)
	}
}

// TestSecretsGrammarRows pins the helm and kubectl tables row by row: no name
// repeats, every row but the two the helm rule asks about takes a value, and
// each row consumes the next word exactly when its arity says so, in any
// letter case.
func TestSecretsGrammarRows(t *testing.T) {
	special := map[string]argv.Option{
		"--dry-run":     {Name: "--dry-run", Arity: argv.ArityOptional, Spellings: []string{"client", "server", "true", "false"}},
		"--hide-secret": {Name: "--hide-secret", Arity: argv.ArityFlag},
	}
	for _, grammar := range []struct {
		label   string
		grammar argv.Spec
		rows    []argv.Option
		special map[string]argv.Option
	}{
		{"helm", helmSecretGrammar, helmSecretOptionRows, special},
		{"kubectl", kubectlSecretGrammar, kubectlSecretOptionRows, nil},
	} {
		seen := map[string]bool{}
		for _, row := range grammar.rows {
			if seen[row.Name] {
				t.Errorf("%s repeats %s", grammar.label, row.Name)
			}
			seen[row.Name] = true
			if want, ok := grammar.special[row.Name]; ok {
				if !reflect.DeepEqual(row, want) {
					t.Errorf("%s gives %s %+v, want %+v", grammar.label, row.Name, row, want)
				}
			} else if row.Arity != argv.ArityValue || len(row.Spellings) != 0 {
				t.Errorf("%s gives %s %+v, want a value row", grammar.label, row.Name, row)
			}
			for _, spelling := range []string{row.Name, strings.ToUpper(row.Name)} {
				scan := grammar.grammar.Scan([]argv.Word{{Text: spelling, Literal: true}, {Text: "client", Literal: true}})
				consumed := scan.Args[1].Role == argv.RoleValue
				if want := row.Arity != argv.ArityFlag; consumed != want {
					t.Errorf("%s reads %s client: consumed %v, want %v", grammar.label, spelling, consumed, want)
				}
			}
		}
		for name := range grammar.special {
			if !seen[name] {
				t.Errorf("%s lacks %s", grammar.label, name)
			}
		}
	}
}

// TestRunGitShowUnreadablePosture pins how this rule treats a git line whose
// global options git prints and exits on (GitPrints) or that cannot be read
// (GitUnread): it names no object path, so the line passes as it did before
// the shared reader. Treating it as a credential read would deny commands
// git itself never runs as a show.
func TestRunGitShowUnreadablePosture(t *testing.T) {
	for _, command := range []string{
		"git --unknown show HEAD:.env",
		"git -- show HEAD:.env",
		"git --no-pager=x show HEAD:.env",
	} {
		t.Run(command, func(t *testing.T) {
			words := gitShowWords(strings.Fields(command)[1:]...)
			if got := gitShowObjectPaths(words); len(got) != 0 {
				t.Fatalf("gitShowObjectPaths(%q) = %q, want no paths", command, got)
			}
			payload := buildEvent(t, "Bash", map[string]any{"command": command})
			if kind, message := run(t, payload, nil); kind != "pass" {
				t.Fatalf("kind=%q message=%q, want pass", kind, message)
			}
		})
	}
}

func TestRunGitShowPagerOptions(t *testing.T) {
	for _, prefix := range []string{
		"git --no-pager", "git -P", "git --paginate", "git -p",
		"git --no-pager -C repo -c core.quotePath=false",
		"git -C repo -P -c core.quotePath=false",
		"git -c core.quotePath=false --paginate -C repo",
		"git -C repo -c core.quotePath=false -p",
	} {
		for _, path := range []string{".env", "README.md"} {
			command := prefix + " show HEAD:" + path
			t.Run(command, func(t *testing.T) {
				payload := buildEvent(t, "Bash", map[string]any{"command": command})
				kind, message := run(t, payload, nil)
				if path == ".env" {
					if kind != "deny" || !strings.Contains(message, "dotenv") {
						t.Fatalf("kind=%q message=%q, want deny with dotenv", kind, message)
					}
				} else if kind != "pass" {
					t.Fatalf("kind=%q message=%q, want pass", kind, message)
				}
			})
		}
	}
}

func TestRunGitShowExecPath(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    string
	}{
		{"git --exec-path ignored show HEAD:.env", "pass"},
		{"git --exec-path show HEAD:.env", "pass"},
		{"git --exec-path=helpers show HEAD:.env", "deny"},
		{"git --exec-path=helpers show HEAD:README.md", "pass"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			payload := buildEvent(t, "Bash", map[string]any{"command": tc.command})
			kind, message := run(t, payload, nil)
			if kind != tc.want || (tc.want == "deny" && !strings.Contains(message, "dotenv")) {
				t.Fatalf("kind=%q message=%q, want %s (dotenv for deny)", kind, message, tc.want)
			}
		})
	}
}

func TestRunDeniesABashRead(t *testing.T) {
	payload := buildEvent(t, "Bash",
		map[string]any{"command": "cat ~/.ssh/id_rsa"})
	kind, message := run(t, payload, nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	if !strings.Contains(message, "ssh_private_key") {
		t.Errorf("message missing the category:\n%s", message)
	}
}

func TestRunDeniesPowerShellCredentialRead(t *testing.T) {
	for _, command := range []string{
		"Get-Content .env",
		"& Get-Content .env",
		"&Get-Content .env",
		`& 'Get-Content' .env`,
		`Microsoft.PowerShell.Management\Get-Content .env`,
		`& Microsoft.PowerShell.Management\Get-Content .env`,
		`&'Microsoft.PowerShell.Management\Get-Content' .env`,
		`Microsoft.PowerShell.Utility\Select-String -Pattern TOKEN -Path .env`,
		". Get-Content .env",
		". .env",
		"Move-Item .env out",
		"mi .env out",
		"move .env out",
		"sls TOKEN .env",
		"cpi .env out",
	} {
		t.Run(command, func(t *testing.T) {
			payload := buildEvent(t, "PowerShell",
				map[string]any{"command": command})
			kind, message := run(t, payload, nil)
			if kind != "deny" {
				t.Fatalf("kind = %q, want deny (%s)", kind, message)
			}
			for _, want := range []string{".env", "dotenv", "critical"} {
				if !strings.Contains(message, want) {
					t.Errorf("message missing %q:\n%s", want, message)
				}
			}
		})
	}
}

func TestRunAllowsBenignBash(t *testing.T) {
	payload := buildEvent(t, "Bash",
		map[string]any{"command": "git status --short"})
	if kind, _ := run(t, payload, nil); kind != "pass" {
		t.Errorf("kind = %q, want pass", kind)
	}
}

func TestRunAllowsBenignPowerShell(t *testing.T) {
	for _, command := range []string{
		"Get-Location",
		"& Get-Location",
		"&Get-Content .env.example",
		`&'Microsoft.PowerShell.Management\Get-Content' .env.example`,
		`Microsoft.PowerShell.Utility\Write-Output .env`,
		"Move-Item a.txt b.txt",
		"mi a.txt b.txt",
		"move a.txt b.txt",
		"sls TOKEN README.md",
		"cpi a.txt b.txt",
	} {
		t.Run(command, func(t *testing.T) {
			payload := buildEvent(t, "PowerShell",
				map[string]any{"command": command})
			if kind, _ := run(t, payload, nil); kind != "pass" {
				t.Errorf("kind = %q, want pass", kind)
			}
		})
	}
}

func TestAllowlistAndSkipSwitch(t *testing.T) {
	payload := buildEvent(t, "Read", map[string]any{"file_path": "fixtures/.env"})

	kind, _ := run(t, payload,
		map[string]string{"SECRETS_GUARD_ALLOWLIST": "fixtures/"})
	if kind != "pass" {
		t.Errorf("allowlisted path: kind = %q, want pass", kind)
	}

	kind, _ = run(t, payload,
		map[string]string{"SKIP_SECRETS_GUARD": "true"})
	if kind != "pass" {
		t.Errorf("SKIP_SECRETS_GUARD=true: kind = %q, want pass", kind)
	}

	// Anything other than the exact string leaves the guard ON.
	kind, _ = run(t, payload,
		map[string]string{"SKIP_SECRETS_GUARD": "1"})
	if kind != "deny" {
		t.Errorf("SKIP_SECRETS_GUARD=1: kind = %q, want deny — only \"true\" disables", kind)
	}
}

// ---------------------------------------------------------------------------
// The write direction

func TestRunBlocksASecretBeforeItIsWritten(t *testing.T) {
	payload := buildEvent(t, "Write", map[string]any{
		"file_path": "src/config.ts",
		"content":   "export const key = \"" + fakeAWSKey + "\";\n",
	})
	kind, message := run(t, payload, nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	if !strings.Contains(message, "AWS_ACCESS_KEY") {
		t.Errorf("message missing the pattern name:\n%s", message)
	}
	if !strings.Contains(message, "src/config.ts") {
		t.Errorf("message missing the target path:\n%s", message)
	}
	// A deny reason is replayed to the model. Printing the secret to explain
	// why it must not be printed would defeat the hook.
	if strings.Contains(message, fakeAWSKey) {
		t.Errorf("message reproduced the secret verbatim:\n%s", message)
	}
	if !strings.Contains(message, "AKIA*") {
		t.Errorf("message should keep a short prefix so the reader can tell which credential:\n%s", message)
	}
}

func TestRunBlocksASecretInAnEditReplacement(t *testing.T) {
	payload := buildEvent(t, "Edit", map[string]any{
		"file_path":  "src/config.ts",
		"new_string": "const t = \"" + fakeGitHubPAT + "\";",
	})
	if kind, msg := run(t, payload, nil); kind != "deny" {
		t.Errorf("kind = %q, want deny (%s)", kind, msg)
	}
}

// TestEditContext pins #671: an edit may repeat a secret-shaped value its
// own old text already carries, because that value is on disk already (an
// edit whose old text is absent fails and writes nothing). A value the new
// text adds, including an extra copy, is still a finding, and pairing never
// crosses input objects.
func TestEditContext(t *testing.T) {
	row := "id = \"" + fakeAWSKey + "\"\n"
	ip := "10.1." + "2.3:8080"
	// Marker-only rules match a header, not the key, so these are built
	// from pieces like every other fixture here.
	pemHead := "-----BEGIN " + "RSA PRIVATE" + " KEY-----"
	pgpHead := "-----BEGIN PGP PRIVATE" + " KEY BLOCK-----"
	saType := `{"type": "service` + `_account"`
	cases := []struct {
		name, tool, want string
		input            map[string]any
	}{
		{"edit repeats context", "Edit", "pass", map[string]any{
			"file_path": "x_test.go", "old_string": row, "new_string": row + "next = 1\n"}},
		{"edit adds a second copy", "Edit", "deny", map[string]any{
			"file_path": "x_test.go", "old_string": row, "new_string": row + row}},
		{"edit adds a different value", "Edit", "deny", map[string]any{
			"file_path": "x_test.go", "old_string": row, "new_string": row + "t = \"" + fakeGitHubPAT + "\"\n"}},
		{"edit renames the key around a carried value", "Edit", "pass", map[string]any{
			"file_path": "x_test.go", "old_string": row, "new_string": "key = \"" + fakeAWSKey + "\"\n"}},
		{"copilot old_str pair in the value envelope", "Edit", "pass", map[string]any{"value": map[string]any{
			"path": "x_test.go", "old_str": row, "new_str": row + "next = 1\n"}}},
		{"multiedit pairs per replacement", "MultiEdit", "pass", map[string]any{
			"file_path": "x_test.go", "edits": []any{
				map[string]any{"old_string": row, "new_string": row + "a = 1\n"},
				map[string]any{"old_string": "b = 1\n", "new_string": "b = 2\n"}}}},
		{"multiedit never borrows another replacement's old text", "MultiEdit", "deny", map[string]any{
			"file_path": "x_test.go", "edits": []any{
				map[string]any{"old_string": row, "new_string": "gone = 1\n"},
				map[string]any{"old_string": "b = 1\n", "new_string": row}}}},
		{"write keeps the full scan", "Write", "deny", map[string]any{
			"file_path": "x_test.go", "content": row, "old_string": row}},
		{"an unpaired empty key excuses nothing", "Write", "deny", map[string]any{
			"file_path": "x_test.go", "content": row, "": row}},
		{"notebook edit keeps the full scan", "NotebookEdit", "deny", map[string]any{
			"notebook_path": "x.ipynb", "new_source": row, "old_string": row}},
		{"carried advisory endpoint passes", "Edit", "pass", map[string]any{
			"file_path": "x_test.go", "old_string": "addr = " + ip + "\n", "new_string": "addr = " + ip + "\nb = 1\n"}},
		{"str_replace command pairs", "Edit", "pass", map[string]any{"value": map[string]any{
			"command": "str_replace", "path": "x_test.go", "old_str": row, "new_str": row + "next = 1\n"}}},
		{"insert command ignores old_str", "Edit", "deny", map[string]any{"value": map[string]any{
			"command": "insert", "path": "x_test.go", "insert_line": json.Number("1"), "old_str": row, "new_str": row}}},
		{"insert_line without a command ignores old_str", "Edit", "deny", map[string]any{"value": map[string]any{
			"path": "x_test.go", "insert_line": json.Number("1"), "old_str": row, "new_str": row}}},
		{"top-level old text never excuses the value envelope", "Edit", "deny", map[string]any{
			"file_path": "x_test.go", "old_string": row, "value": map[string]any{"new_str": row}}},
		{"value envelope old text never excuses the top level", "Edit", "deny", map[string]any{
			"file_path": "x_test.go", "new_string": row, "value": map[string]any{"old_str": row}}},
		{"a carried key header never excuses a new key body", "Edit", "deny", map[string]any{
			"file_path": "id_rsa", "old_string": pemHead + "\n", "new_string": pemHead + "\nMIIEnewbody\n"}},
		{"a key rotation denies", "Edit", "deny", map[string]any{
			"file_path": "id_rsa", "old_string": pemHead + "\nMIIEoldbody\n", "new_string": pemHead + "\nMIIEnewbody\n"}},
		{"a carried PGP block header never excuses a new block", "Edit", "deny", map[string]any{
			"file_path": "key.asc", "old_string": pgpHead + "\n", "new_string": pgpHead + "\nlQOYBnew\n"}},
		{"a service-account rotation denies", "Edit", "deny", map[string]any{
			"file_path": "sa.json", "old_string": saType + ", \"private_key_id\": \"old\"", "new_string": saType + ", \"private_key_id\": \"new\""}},
		{"a value envelope list is scanned beside a top-level list", "Edit", "deny", map[string]any{
			"files": []any{map[string]any{"path": "notes.md", "content": "benign"}},
			"value": map[string]any{"files": []any{map[string]any{"path": "x_test.go", "content": row}}}}},
		{"an MCP push item keeps the full scan", "mcp__github__push_files", "deny", map[string]any{
			"owner": "o", "repo": "r", "branch": "b", "message": "m", "files": []any{
				map[string]any{"path": "x_test.go", "content": row, "old_string": row}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := buildEvent(t, tc.tool, tc.input)
			if kind, msg := run(t, payload, nil); kind != tc.want {
				t.Errorf("kind = %q, want %q (%s)", kind, tc.want, msg)
			}
		})
	}
}

// TestEditContextDenyMessageSteersAwayFromTheShell pins #671's steering line:
// a blocked write tells the agent to narrow the edit, not to route around the
// guard.
func TestEditContextDenyMessageSteersAwayFromTheShell(t *testing.T) {
	payload := buildEvent(t, "Edit", map[string]any{
		"file_path": "x_test.go", "new_string": "id = \"" + fakeAWSKey + "\"\n",
	})
	kind, message := run(t, payload, nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	for _, want := range []string{"narrow the edit", "shell command"} {
		if !strings.Contains(message, want) {
			t.Errorf("message missing %q:\n%s", want, message)
		}
	}
}

// TestMarkerDenyMessageDoesNotPromiseACarry pins the Copilot review note on
// PR #691: a marker-only deny must not tell the agent that narrowing the edit
// helps, because a marker is flagged whatever the old text carries. A deny
// with both kinds of hit carries both lines.
func TestMarkerDenyMessageDoesNotPromiseACarry(t *testing.T) {
	pemHead := "-----BEGIN " + "RSA PRIVATE" + " KEY-----"
	cases := []struct {
		name          string
		newText       string
		carry, marker bool
	}{
		{"marker only", pemHead + "\nMIIEnewbody\n", false, true},
		{"value only", "id = \"" + fakeAWSKey + "\"\n", true, false},
		{"both", pemHead + "\nid = \"" + fakeAWSKey + "\"\n", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := buildEvent(t, "Edit", map[string]any{
				"file_path": "id_rsa", "old_string": pemHead + "\n", "new_string": tc.newText,
			})
			kind, message := run(t, payload, nil)
			if kind != "deny" {
				t.Fatalf("kind = %q, want deny (%s)", kind, message)
			}
			if got := strings.Contains(message, "narrow the edit"); got != tc.carry {
				t.Errorf("carry advice present = %v, want %v:\n%s", got, tc.carry, message)
			}
			if got := strings.Contains(message, "marker is flagged even when"); got != tc.marker {
				t.Errorf("marker advice present = %v, want %v:\n%s", got, tc.marker, message)
			}
			if !strings.Contains(message, "shell command") {
				t.Errorf("message missing the shell warning:\n%s", message)
			}
		})
	}
}

// A documentation edit that shows an example key must not be blocked — the
// placeholder filter guards this.
func TestRunAllowsPlaceholderSecretsInDocs(t *testing.T) {
	payload := buildEvent(t, "Write", map[string]any{
		"file_path": "README.md",
		"content":   "Set it like this: " + fakePlaceholderKey + "\n",
	})
	if kind, msg := run(t, payload, nil); kind != "pass" {
		t.Errorf("kind = %q, want pass — placeholders must not block docs (%s)", kind, msg)
	}
}

// Source that reads a credential from the environment holds no credential, so
// writing it must pass. The same line with a literal in the accessor's place
// is still denied: the literal carries a digit, so no code-shaped reading of
// it can excuse it. The content is assembled from pieces so this source holds
// no line the guard itself would deny.
func TestRunAllowsAnEnvironmentAccessorWrite(t *testing.T) {
	kw := "to" + "ken"
	literal := "abcdefghijk" + "7"
	write := func(value string) hookspec.Event {
		return buildEvent(t, "Write", map[string]any{
			"file_path": "src/main.rs",
			"content":   "let " + kw + " = " + value + ";\n",
		})
	}
	if kind, msg := run(t, write(`env::var("HOME")`), nil); kind != "pass" {
		t.Errorf("kind = %q, want pass — an environment read is not a secret (%s)", kind, msg)
	}
	kind, message := run(t, write(literal), nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny for a literal value (%s)", kind, message)
	}
	if !strings.Contains(message, "GENERIC_SECRET") {
		t.Errorf("message missing the pattern name:\n%s", message)
	}
}

// A manifest that passes a user-configured value into a server's environment
// names the value's source and holds no credential, so writing it must pass.
// The same member with characters after the reference is still denied: the
// reference is then only part of a value. The content is assembled from pieces
// so this source holds no line the guard itself would deny.
func TestRunAllowsAUserConfigReferenceWrite(t *testing.T) {
	name := "api" + "_key"
	reference := "${user_config." + name + "}"
	write := func(value string) hookspec.Event {
		return buildEvent(t, "Write", map[string]any{
			"file_path": "manifest.json",
			"content":   `{"env": {"` + name + `"` + `: "` + value + `"}}` + "\n",
		})
	}
	if kind, msg := run(t, write(reference), nil); kind != "pass" {
		t.Errorf("kind = %q, want pass — a whole-value user-config reference is not a secret (%s)", kind, msg)
	}
	kind, message := run(t, write(reference+"abcd"+"1234"), nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny for a reference with a suffix (%s)", kind, message)
	}
	if !strings.Contains(message, "GENERIC_SECRET") {
		t.Errorf("message missing the pattern name:\n%s", message)
	}
}

// An edit may repeat a finding its old text already carries. An exempt
// accessor read is not a finding, so the old text carries nothing, and the
// new text is judged as a fresh write. Here the old text reads through the
// accessor with its bracket and the new text is the same line in the
// bracket-less call form, which the accessor list does not exempt: the edit
// is denied rather than excused by the equal name in the text it replaces.
func TestRunDoesNotCarryAnAccessorReadIntoANonExemptCopy(t *testing.T) {
	kw := "to" + "ken"
	accessor := "ENV" + ".fetch"
	payload := buildEvent(t, "Edit", map[string]any{
		"file_path":  "config/app.rb",
		"old_string": kw + " = " + accessor + `("X")` + "\n",
		"new_string": kw + " = " + accessor + ` "X"` + "\n",
	})
	kind, message := run(t, payload, nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	if !strings.Contains(message, "GENERIC_SECRET") {
		t.Errorf("message missing the pattern name:\n%s", message)
	}
}

// Writing a .env is ordinary setup. It is READING one back that cannot be
// undone, so the write path checks content and never the path.
func TestRunAllowsWritingToACredentialPath(t *testing.T) {
	payload := buildEvent(t, "Write", map[string]any{
		"file_path": ".env",
		"content":   "PORT=3000\nDEBUG=true\n",
	})
	if kind, msg := run(t, payload, nil); kind != "pass" {
		t.Errorf("kind = %q, want pass — writing a .env is not a leak (%s)", kind, msg)
	}
}

func TestRunAllowsAnOrdinaryWrite(t *testing.T) {
	payload := buildEvent(t, "Write", map[string]any{
		"file_path": "src/main.go",
		"content":   "package main\n\nfunc main() {}\n",
	})
	if kind, msg := run(t, payload, nil); kind != "pass" {
		t.Errorf("kind = %q, want pass (%s)", kind, msg)
	}
}

func TestEmptyPayloadPasses(t *testing.T) {
	if kind, _ := run(t, hookspec.Event{}, nil); kind != "pass" {
		t.Errorf("kind = %q, want pass on an empty payload", kind)
	}
}

// ---------------------------------------------------------------------------
// The shell command text itself is scanned in the write direction, after the
// read-direction checks find nothing. A secret on a command line is exposed
// in transcripts and logs even when nothing lands on disk.

func TestShellWriteSecretHeredocDenies(t *testing.T) {
	command := "cat <<EOF > out.env\nAWS_ACCESS_KEY_ID=" + fakeAWSKey + "\nEOF"
	payload := buildEvent(t, "Bash", map[string]any{"command": command})
	kind, message := run(t, payload, nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	if !strings.Contains(message, "AWS_ACCESS_KEY") {
		t.Errorf("message missing the pattern name:\n%s", message)
	}
	if !strings.Contains(message, "the shell command") {
		t.Errorf("message missing the shell command target:\n%s", message)
	}
	if strings.Contains(message, fakeAWSKey) {
		t.Errorf("message reproduced the secret verbatim:\n%s", message)
	}
}

func TestShellWriteSecretPrintfDenies(t *testing.T) {
	value := "Zq8" + "Lm2Rt7Vx9Kp4"
	command := "printf 'TOKEN=" + value + `\n' > out.txt`
	payload := buildEvent(t, "Bash", map[string]any{"command": command})
	kind, message := run(t, payload, nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	if !strings.Contains(message, "GENERIC_SECRET") {
		t.Errorf("message missing the pattern name:\n%s", message)
	}
}

func TestShellWriteSecretSedDenies(t *testing.T) {
	command := "sed -i 's/OLD/" + fakeGitHubPAT + "/' config.txt"
	payload := buildEvent(t, "Bash", map[string]any{"command": command})
	kind, message := run(t, payload, nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	if !strings.Contains(message, "GITHUB_PAT") {
		t.Errorf("message missing the pattern name:\n%s", message)
	}
}

func TestShellWriteSecretPowerShellSetContentDenies(t *testing.T) {
	command := "Set-Content -Path out.txt -Value '" + fakeAWSKey + "'"
	payload := buildEvent(t, "PowerShell", map[string]any{"command": command})
	kind, message := run(t, payload, nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	if !strings.Contains(message, "AWS_ACCESS_KEY") {
		t.Errorf("message missing the pattern name:\n%s", message)
	}
}

// TestShellWriteSecretAdviceTargetsTheShell pins #684: the write-direction
// edit-carry and marker-carry sentences ("narrow the edit", "the text it
// replaces") talk about an edit and do not fit a shell command, so the shell
// target gets exactly one steering line instead of either one.
func TestShellWriteSecretAdviceTargetsTheShell(t *testing.T) {
	command := "printf '" + fakeGitHubPAT + `\n' > out.txt`
	payload := buildEvent(t, "Bash", map[string]any{"command": command})
	kind, message := run(t, payload, nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	if !strings.Contains(message, "Keep the value out of the command line: write it through a file tool with a placeholder, or read it from the environment inside the command.") {
		t.Errorf("message missing the shell-specific steering line:\n%s", message)
	}
	for _, unwanted := range []string{
		"narrow the edit",
		"the text it replaces",
		"Do not write it through a shell command instead.",
	} {
		if strings.Contains(message, unwanted) {
			t.Errorf("message should not carry the edit-direction wording %q:\n%s", unwanted, message)
		}
	}
}

func TestShellWriteSecretWarnModeReturnsContext(t *testing.T) {
	command := "printf 'TOKEN=" + fakeGitHubPAT + `\n' > out.txt`
	payload := buildEvent(t, "Bash", map[string]any{"command": command})
	kind, message := run(t, payload, map[string]string{"SECRETS_GUARD_MODE": "warn"})
	if kind != "context" {
		t.Fatalf("kind = %q, want context — warn mode never denies (%s)", kind, message)
	}
}

// TestShellWriteSecretAdvisoryOnlyNeverDenies pins that INTERNAL_IP_PORT, the
// one advisory-only rule, never escalates a shell command to a deny even in
// block mode — it only ever adds context.
func TestShellWriteSecretAdvisoryOnlyNeverDenies(t *testing.T) {
	ip := "10.1." + "2.3:8080"
	command := "echo 'connecting to " + ip + "'"
	payload := buildEvent(t, "Bash", map[string]any{"command": command})
	kind, message := run(t, payload, nil)
	if kind != "context" {
		t.Fatalf("kind = %q, want context — advisory rules never deny (%s)", kind, message)
	}
	if !strings.Contains(message, "INTERNAL_IP_PORT") {
		t.Errorf("message missing the advisory rule name:\n%s", message)
	}
}

func TestShellWriteSecretPasses(t *testing.T) {
	// The reference-chain line is built from pieces, as TestSecretsSweep699
	// does: a contiguous "${{ a || b }}" literal in this source is itself a
	// GENERIC_SECRET-shaped match the checked-in secrets-guardian flags.
	chain := "TOKEN: " + "${{ secrets.A || secrets.B }}"
	cases := []struct{ name, command string }{
		{"export from another variable", "export TOKEN=$OTHER"},
		{"secrets reference chain", "echo '" + chain + "'"},
		{"git commit message mentioning token parsing", `git commit -m "fix     token parsing"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := buildEvent(t, "Bash", map[string]any{"command": tc.command})
			if kind, msg := run(t, payload, nil); kind != "pass" {
				t.Errorf("kind = %q, want pass (%s)", kind, msg)
			}
		})
	}
}

// TestShellReadStillWinsOverShellWriteScan pins that a credential path read
// through a reader command still denies through the existing read-direction
// check, before the new write-direction command-text scan ever runs.
func TestShellReadStillWinsOverShellWriteScan(t *testing.T) {
	payload := buildEvent(t, "Bash", map[string]any{"command": "cat .env"})
	kind, message := run(t, payload, nil)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	if !strings.Contains(message, "credential file read") {
		t.Errorf("message should still be the credential-read reason:\n%s", message)
	}
}

// TestShellWriteSecretAllowlistDoesNotExempt pins that
// SECRETS_GUARD_ALLOWLIST, a path allowlist, does not exempt the shell
// command-text scan: there is no reliable redirect target to compare
// against it without shellwords.
func TestShellWriteSecretAllowlistDoesNotExempt(t *testing.T) {
	command := "printf 'TOKEN=" + fakeGitHubPAT + `\n' > out.txt`
	payload := buildEvent(t, "Bash", map[string]any{"command": command})
	kind, message := run(t, payload,
		map[string]string{"SECRETS_GUARD_ALLOWLIST": "out.txt"})
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny — the path allowlist must not exempt the command-text scan (%s)", kind, message)
	}
}

// clearHostCredentialOverrides keeps the host's credential override variables
// out of classify, so a probe's finding depends only on the tables.
func clearHostCredentialOverrides(t *testing.T) {
	t.Helper()
	for _, name := range hostCredentialOverrideNames {
		t.Setenv(name, "")
	}
}

func TestPatternCandidatesAreTheTableNames(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range patternCandidates {
		if seen[name] {
			t.Errorf("patternCandidates holds %q twice", name)
		}
		seen[name] = true
	}
	want := map[string]bool{".env": true, "terraform.tfstate": true}
	for name := range exactNames {
		want[name] = true
	}
	for name := range sshKeyNames {
		want[name] = true
	}
	for _, explicit := range explicitSuffixes {
		want[path.Base(explicit.suffix)] = true
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("patternCandidates lacks %q", name)
		}
	}
	for name := range seen {
		if !want[name] {
			t.Errorf("patternCandidates holds %q, which no table names", name)
		}
	}
}

// TestClassifyPatternLongPattern pins termination on a long input, well formed
// and malformed.
func TestClassifyPatternLongPattern(t *testing.T) {
	clearHostCredentialOverrides(t)
	long := strings.Repeat("*a", 51200)
	for _, basePattern := range []string{long, "[" + long} {
		got, ok := classifyPattern(".", basePattern)
		if ok || got != (finding{}) {
			t.Errorf("pattern of %d bytes starting %q: classifyPattern = %#v, %v; want no finding", len(basePattern), basePattern[:4], got, ok)
		}
	}
}

// TestClassifyPatternCandidateOrder pins which candidate a pattern that matches
// several names reports first.
func TestClassifyPatternCandidateOrder(t *testing.T) {
	clearHostCredentialOverrides(t)
	pinned := []struct{ basePattern, category string }{
		{".[pm]*", "database_credentials"},
		{"[ia]*[kn]", "cloud_credentials"},
	}
	for _, pin := range pinned {
		got, ok := classifyPattern(".", pin.basePattern)
		if !ok || got.category != pin.category {
			t.Errorf("pattern %q: classifyPattern = %#v, %v; want category %q", pin.basePattern, got, ok, pin.category)
		}
	}
}
