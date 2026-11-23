package releaseguard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/settings/settingstest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestBashInspectionPrecision(t *testing.T) {
	for _, command := range []string{"bash --version", "bash --help", "bash -n script.sh", "bash -n -- '.env'"} {
		t.Run(command, func(t *testing.T) {
			kind, message := run(t, buildEvent(t, "Bash", map[string]any{"command": command}))
			if kind != "pass" {
				t.Fatalf("inspection = %s %s", kind, message)
			}
		})
	}
	for _, command := range []string{"bash --help; kubectl apply -f prod.yaml", "bash -n script.sh; kubectl apply -f prod.yaml"} {
		kind, message := run(t, buildEvent(t, "Bash", map[string]any{"command": command}))
		if kind != "deny" || !strings.Contains(message, "production-shaped deploy command") {
			t.Fatalf("deployment erased: %s %s", kind, message)
		}
	}
	for _, input := range []map[string]any{
		{"command": "bash --help", "shell": "bash"},
		{"command": "bash --help", "env": map[string]any{}},
		{"command": "bash --help", "metadata": map[string]any{"shell": "bash"}},
		{"command": "alias bash=helper; bash --help"},
		{"command": "PATH=other; bash --help"},
		{"command": "echo public; bash --help"},
		{"command": "bash --help; bash --version"},
	} {
		kind, message := run(t, buildEvent(t, "Bash", input))
		if kind != "deny" || !strings.Contains(message, "inspection authority unavailable") {
			t.Fatalf("unqualified proof: %s %s", kind, message)
		}
	}
}

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

func TestPowerShellFileDataPrecision(t *testing.T) {
	command := "@'\ndon't; kubectl apply -f prod.yaml; 'literal\n'@ | Set-Content -LiteralPath draft.txt"
	kind, message := run(t, buildEvent(t, "PowerShell", map[string]any{"command": command}))
	if kind != "pass" {
		t.Fatalf("literal data = %s %s", kind, message)
	}
	for _, unsafe := range []string{command + "; kubectl apply -f prod.yaml", "@'\nordinary text\n'@ | Set-Content -LiteralPath draft.txt; kubectl apply -f prod.yaml", strings.Replace(command, "Set-Content -LiteralPath draft.txt", "Invoke-Expression", 1)} {
		kind, message := run(t, buildEvent(t, "PowerShell", map[string]any{"command": unsafe}))
		if kind != "deny" || !strings.Contains(message, "production-shaped deploy command (kubernetes_deploy)") {
			t.Fatalf("actual deploy lost: %s %s", kind, message)
		}
	}
	kind, message = run(t, buildEvent(t, "PowerShell", map[string]any{"command": command, "shell": "pwsh"}))
	if kind != "deny" || !strings.Contains(message, "production-shaped deploy command (kubernetes_deploy)") {
		t.Fatalf("unknown context granted proof: %s %s", kind, message)
	}
}

func TestPowerShellFileDataCancelled(t *testing.T) {
	t.Setenv("AGX_RELEASE_RECORD", "")
	t.Setenv("AGX_RELEASE_ENVIRONMENTS", "")
	if err := os.Unsetenv("AGX_RELEASE_ENVIRONMENTS"); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"body", "suffix"} {
		t.Run(stage, func(t *testing.T) {
			command := "@'\ndon't; kubectl apply -f prod.yaml; 'literal\n" + strings.Repeat("x", 6*64*1024) + "\n'@ | Set-Content -LiteralPath draft.txt" + strings.Repeat(" ", 6*64*1024)
			baseline := &hookspectest.CancelContext{Context: context.Background()}
			shellwords.ParseContext(baseline, command, shellwords.PowerShell)
			if stage == "suffix" {
				shellwords.PowerShellLiteralData(baseline, command)
			}
			ctx := &hookspectest.CancelContext{Context: context.Background(), At: baseline.Calls + 4}
			out := Run(hookspec.Request{Context: ctx, Event: buildEvent(t, "PowerShell", map[string]any{"command": command})})
			if ctx.Calls < ctx.At || out.Kind() != "deny" || !strings.Contains(out.Payload(), "production-shaped deploy command (kubernetes_deploy)") {
				t.Fatalf("cancellation lost original deploy: checks=%d/%d %s %s", ctx.Calls, ctx.At, out.Kind(), out.Payload())
			}
		})
	}
}

func TestPowerShellEnvironmentCancellation(t *testing.T) {
	t.Setenv("AGX_RELEASE_RECORD", "")
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
		for _, tool := range []string{"PowerShell", "mcp__host__PowerShell"} {
			payload := buildEvent(t, tool, map[string]any{"command": prefix + `rg --no-config '.env' source.txt; kubectl apply -f prod.yaml`, "shell": nil})
			kind, message := run(t, payload)
			if kind != "deny" || message == "" {
				t.Fatalf("protected deployment lost: %s %s", kind, message)
			}
		}
	}
}

func TestPowerShellEnvironmentLargeCancellation(t *testing.T) {
	t.Setenv("AGX_RELEASE_RECORD", "")
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

func buildEvent(t *testing.T, tool string, input map[string]any) hookspec.Event {
	t.Helper()
	return buildEventAt(t, "/tmp/releaseguard", tool, input)
}

// buildEventAt is buildEvent with an explicit event working directory.
func buildEventAt(t *testing.T, dir, tool string, input map[string]any) hookspec.Event {
	t.Helper()
	payload, err := contract.NewEvent(contract.EventParts{
		Surface:   surface.ClaudeCode,
		Cwd:       &dir,
		ToolName:  tool,
		ToolInput: input,
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func run(t *testing.T, payload hookspec.Event) (kind, message string) {
	t.Helper()
	return runWithRecord(t, payload, "")
}

func runWithRecord(t *testing.T, payload hookspec.Event, record string) (kind, message string) {
	t.Helper()
	return runWithRecordFrom(t, nil, payload, record)
}

// runFrom is run with the request carrying source, so a test fixes the workflow
// settings without a settings file. A nil source reads the files.
func runFrom(t *testing.T, source *settings.Source, payload hookspec.Event) (kind, message string) {
	t.Helper()
	return runWithRecordFrom(t, source, payload, "")
}

func runWithRecordFrom(t *testing.T, source *settings.Source, payload hookspec.Event, record string) (kind, message string) {
	t.Helper()
	return runRequest(t, hookspec.Request{Provider: "claude-code", Hook: "release-guardian", Event: payload, Settings: source}, record)
}

// runRequest runs the guard on req with the release record set to record and the record hash and log directory
// isolated as runWithRecord describes.
func runRequest(t *testing.T, req hookspec.Request, record string) (kind, message string) {
	t.Helper()
	t.Setenv("AGX_RELEASE_RECORD", record)
	// AGX_RELEASE_RECORD_SHA256 must be genuinely absent for a test that
	// does not set it itself: t.Setenv cannot express "unset" on its own, so
	// this sets it (registering the restore) and then unsets it directly,
	// restored through t.Cleanup. A test that wants the hash configured
	// calls t.Setenv for it before calling runWithRecord, so the lookup
	// below finds it already present and skips this.
	if _, ok := os.LookupEnv("AGX_RELEASE_RECORD_SHA256"); !ok {
		t.Setenv("AGX_RELEASE_RECORD_SHA256", "")
		os.Unsetenv("AGX_RELEASE_RECORD_SHA256")
		t.Cleanup(func() { os.Unsetenv("AGX_RELEASE_RECORD_SHA256") })
	}
	// An authorized deploy appends to guard.log. Isolate the destination
	// under a scratch directory unless the caller already pointed
	// RELEASE_GUARD_LOG_DIR somewhere on purpose (e.g. the symlink-fallback
	// tests, which set it to "" to exercise the relative default).
	if _, ok := os.LookupEnv("RELEASE_GUARD_LOG_DIR"); !ok {
		t.Setenv("RELEASE_GUARD_LOG_DIR", t.TempDir())
	}
	outcome := Run(req)
	if !outcome.Valid() {
		t.Fatalf("invalid outcome: %#v", outcome)
	}
	return outcome.Kind(), outcome.Payload()
}

// toolCommand runs one command through the guard as the named tool with no
// record set.
func toolCommand(t *testing.T, tool, command string) (kind, message string) {
	t.Helper()
	return run(t, buildEvent(t, tool, map[string]any{"command": command}))
}

func bashCommand(t *testing.T, command string) (kind, message string) {
	t.Helper()
	return toolCommand(t, "Bash", command)
}

// bashCommandFrom is bashCommand with the request carrying source.
func bashCommandFrom(t *testing.T, source *settings.Source, command string) (kind, message string) {
	t.Helper()
	return runFrom(t, source, buildEvent(t, "Bash", map[string]any{"command": command}))
}

func assertEachTool(t *testing.T, tool string, commands []string, want string) {
	t.Helper()
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			if kind, message := toolCommand(t, tool, command); kind != want {
				t.Errorf("kind = %q, want %s (%s)", kind, want, message)
			}
		})
	}
}

func assertEach(t *testing.T, commands []string, want string) {
	t.Helper()
	assertEachTool(t, "Bash", commands, want)
}

func TestEmptyEventPasses(t *testing.T) {
	if kind, _ := run(t, hookspec.Event{}); kind != "pass" {
		t.Errorf("kind = %q, want pass", kind)
	}
}

func TestDeploymentOptionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		command, want, reason string
	}{
		{"terraform", "pass", ""},
		{"terraform -chdir", "deny", "literal value"},
		{"terraform -chdir '' apply", "deny", "literal value"},
		{"terraform -unknown apply", "deny", "frozen parser contract"},
		{"terraform $OPERATION", "deny", "non-literal"},
		{"pulumi --non-interactive", "pass", ""},
		{"pulumi --stack", "deny", "literal value"},
		{"pulumi --unknown up", "deny", "frozen parser contract"},
		{"aws --debug", "pass", ""},
		{"aws --profile", "deny", "literal value"},
		{"aws cloudformation", "pass", ""},
		{"aws cloudformation --profile", "deny", "literal value"},
		{"aws cloudformation --profile synthetic deploy", "deny", "cloudformation"},
		{"aws cloudformation deploy --stack-name help", "deny", "cloudformation"},
		{"aws cloudformation deploy --stack-name synthetic help", "pass", ""},
		{"gcloud --quiet", "pass", ""},
		{"gcloud --project", "deny", "literal value"},
		{"gcloud run", "pass", ""},
		{"gcloud run --project", "deny", "literal value"},
		{"gcloud run --project synthetic deploy", "deny", "gcloud"},
		{"helm --namespace", "deny", "literal value"},
		{"helm --namespace synthetic", "pass", ""},
		{"helm upgrade app chart --dry-run=client --hide-secret --help=false -h", "deny", "helm"},
		{"helm upgrade app chart --dry-run=$MODE --hide-secret", "deny", "helm"},
		{"helm upgrade app chart --dry-run=client --hide-secret=invalid", "deny", "helm"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			kind, message := bashCommand(t, tc.command)
			if kind != tc.want || !strings.Contains(message, tc.reason) {
				t.Fatalf("got %s (%s), want %s containing %q", kind, message, tc.want, tc.reason)
			}
		})
	}
}

// TestReleaseScanPosture pins release-guardian's answer to a command line its option grammar cannot read: an unknown
// option, a value option with no usable value, and a non-literal word before the operation each deny with the
// matcher's whole message, byte for byte. A script line names its location in place of the text it could not read.
func TestReleaseScanPosture(t *testing.T) {
	unreadable := func(reason string) string {
		return "release-guardian could not inspect a recognized execution path (" + reason + "); " + recordRemedy
	}
	for _, tc := range []struct{ command, reason string }{
		{"kubectl --bogus apply", "kubectl option --bogus is outside the frozen parser contract"},
		{"kubectl --namespace", "kubectl --namespace is missing its literal value"},
		{"kubectl $X apply", "kubectl non-literal option or operation"},
		{"kubectl --namespace $X apply", "kubectl --namespace is missing its literal value"},
		{"kubectl --namespace '' apply", "kubectl --namespace is missing its literal value"},
		{"kubectl -x=1 apply", "kubectl option -x=1 is outside the frozen parser contract"},
		{"kubectl set $X deployment/api", "kubectl non-literal option or operation"},
		{"kubectl rollout", "kubectl rollout is missing a literal subcommand"},
		{"helm --bogus install app chart", "helm option --bogus is outside the frozen parser contract"},
		{"helm --namespace", "helm --namespace is missing its literal value"},
		{"helm $X install app chart", "helm non-literal option or operation"},
		{"helm --namespace $X install app chart", "helm --namespace is missing its literal value"},
		{"helm --namespace '' install app chart", "helm --namespace is missing its literal value"},
		{"terragrunt --bogus apply", "terragrunt option --bogus is outside the frozen parser contract"},
		{"terragrunt --config", "terragrunt --config is missing its literal value"},
		{"terragrunt $X apply", "terragrunt non-literal option or operation"},
		{"terragrunt --config $X apply", "terragrunt --config is missing its literal value"},
		{"terragrunt --config '' apply", "terragrunt --config is missing its literal value"},
		{"terragrunt --x=1 apply", "terragrunt option --x is outside the frozen parser contract"},
		{"terragrunt -- $X", "terragrunt non-literal operation after --"},
		{"terragrunt stack --bogus run apply", "terragrunt stack option --bogus is outside the frozen parser contract"},
		{"terragrunt run --bogus apply", "terragrunt run option --bogus is outside the frozen parser contract"},
		{"terragrunt run-all --bogus apply", "terragrunt run option --bogus is outside the frozen parser contract"},
		{"helmfile --bogus sync", "helmfile option --bogus is outside the frozen parser contract"},
		{"terraform -bogus apply", "terraform option -bogus is outside the frozen parser contract"},
		{"tofu -bogus apply", "tofu option -bogus is outside the frozen parser contract"},
		{"pulumi --bogus up", "pulumi option --bogus is outside the frozen parser contract"},
		{"aws --bogus cloudformation deploy", "aws option --bogus is outside the frozen parser contract"},
		{"aws cloudformation --bogus deploy", "aws cloudformation option --bogus is outside the frozen parser contract"},
		{"gcloud --bogus run deploy", "gcloud option --bogus is outside the frozen parser contract"},
		{"gcloud run --bogus deploy", "gcloud run option --bogus is outside the frozen parser contract"},
		{"az --bogus deployment group create", "az option --bogus is outside the frozen parser contract"},
		{"az deployment --bogus group create", "az deployment option --bogus is outside the frozen parser contract"},
		{"az deployment group --bogus create", "az deployment group option --bogus is outside the frozen parser contract"},
		{"vercel --bogus", "vercel option --bogus is outside the frozen parser contract"},
		{"vercel $X", "vercel non-literal option or operation"},
		{"vercel --TOKEN --scope", "vercel option is missing its literal value"},
		{"vercel --TOKEN --target", "vercel --target is missing its literal value"},
		{"vercel --target -x", "vercel --target has an invalid literal value"},
		{"vercel --target=production --target=preview", "vercel production flags conflict"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			kind, message := bashCommand(t, tc.command)
			if want := unreadable(tc.reason); kind != "deny" || message != want {
				t.Fatalf("got %s %q, want deny %q", kind, message, want)
			}
		})
	}
	t.Run("script line", func(t *testing.T) {
		req := hookspec.Request{
			Provider: "claude-code",
			Hook:     "release-guardian",
			Event:    buildEventAt(t, t.TempDir(), "Bash", map[string]any{"command": "bash /ws/t.sh"}),
			Scripts:  hookspectest.Scripts{"/ws/t.sh": "echo a\nkubectl --bogus apply\n"},
		}
		kind, message := runRequest(t, req, "")
		if want := unreadable("deploy option could not be read in /ws/t.sh:2"); kind != "deny" || message != want {
			t.Fatalf("got %s %q, want deny %q", kind, message, want)
		}
	})
}

func TestNULNormalizedEventIsDenied(t *testing.T) {
	kind, message := run(t, buildEvent(t, "Bash", map[string]any{"command": "\x00"}))
	if kind != "deny" {
		t.Errorf("kind = %q, want deny", kind)
	}
	if message != "release-guardian could not validate normalized input" {
		t.Errorf("message = %q, want normalized-input error", message)
	}
}

func TestNamespacedShellToolIsInspected(t *testing.T) {
	if kind, msg := toolCommand(t, "mcp__host__Bash", "kubectl apply -f production.yaml"); kind != "deny" {
		t.Fatalf("mcp__host__Bash: kind = %q (%s), want deny", kind, msg)
	}
	if kind, msg := toolCommand(t, "mcp__host__PowerShell", `C:\tools\kubectl.exe apply -f prod.yaml`); kind != "deny" {
		t.Fatalf("mcp__host__PowerShell: kind = %q (%s), want deny", kind, msg)
	}
	if kind, _ := toolCommand(t, "mcp__host__Read", "kubectl apply -f production.yaml"); kind != "pass" {
		t.Fatalf("a non-shell tool must pass, got %q", kind)
	}
}

func TestNonShellToolPasses(t *testing.T) {
	for _, tool := range []string{"Write", "Read", "Edit"} {
		t.Run(tool, func(t *testing.T) {
			payload := buildEvent(t, tool, map[string]any{"command": "kubectl apply -f production.yaml"})
			if kind, _ := run(t, payload); kind != "pass" {
				t.Errorf("kind = %q, want pass", kind)
			}
		})
	}
}

// TestPowerShellToolIsInspected covers Claude Code's native PowerShell tool.
// Backslash is a path separator there, the backtick is the escape and the
// line-continuation character, and `&` is the call operator.
func TestPowerShellToolIsInspected(t *testing.T) {
	assertEachTool(t, "PowerShell", []string{
		"kubectl apply -f production.yaml",
		`C:\tools\kubectl.exe apply -f production.yaml`,
		`& 'C:\Program Files\kubectl\kubectl.exe' apply -f production.yaml`,
		`.\kubectl.exe apply -f production.yaml`,
		"kubectl `\n apply -f production.yaml",
		"kubectl -n prod apply -f production.yaml",
		"$env:KUBECONFIG='x'; helm upgrade app ./chart",
	}, "deny")
	assertEachTool(t, "PowerShell", []string{
		"kubectl get pods",
		"Write-Output 'kubectl apply -f production.yaml'",
		`kubec\tl apply -f production.yaml`, // a backslash is not an escape here
	}, "pass")
}

func TestProductionDeployWithoutRecordIsDenied(t *testing.T) {
	kind, message := bashCommand(t, "kubectl apply -f production.yaml")
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	for _, want := range []string{"kubectl apply", "/agentics:gate change"} {
		if !strings.Contains(message, want) {
			t.Errorf("message missing %q: %s", want, message)
		}
	}
}

func TestEveryNamedDeployAtASegmentStartIsDenied(t *testing.T) {
	assertEach(t, []string{
		"printf ready && kubectl apply -f production.yaml",
		"printf ready; helm upgrade app ./chart",
		"printf ready || terraform apply plan.tfplan",
		"printf ready & kubectl apply -f production.yaml",
	}, "deny")
}

// TestBypassCorpusIsDenied pins the matcher's grammar. Every row is a real
// spelling of a production deploy that a two-word exact match let through.
func TestBypassCorpusIsDenied(t *testing.T) {
	assertEach(t, []string{
		// assignment prefix — the deny message's own remediation hint
		"AGX_RELEASE_RECORD=/missing kubectl apply -f production.yaml",
		"FOO=1 BAR=2 helm upgrade app ./chart",
		// global flags before the subcommand
		"kubectl -n prod apply -f production.yaml",
		"kubectl --context=prod apply -f production.yaml",
		"kubectl --namespace prod apply -f production.yaml",
		"terraform -chdir=infra apply",
		"helm --kube-context prod upgrade app ./chart",
		// wrappers
		"sudo kubectl apply -f production.yaml",
		"sudo -E kubectl apply -f production.yaml",
		"env FOO=1 kubectl apply -f production.yaml",
		"env -i kubectl apply -f production.yaml",
		"command kubectl apply -f production.yaml",
		"exec kubectl apply -f production.yaml",
		"nohup kubectl apply -f production.yaml",
		"time kubectl apply -f production.yaml",
		"xargs kubectl apply -f",
		// grouping and control words
		"(kubectl apply -f production.yaml)",
		"{ kubectl apply -f production.yaml; }",
		"if kubectl apply -f production.yaml; then echo ok; fi",
		"if true; then kubectl apply -f production.yaml; fi",
		"for f in a b; do kubectl apply -f $f; done",
		"while :; do helm upgrade app ./chart; done",
		"! kubectl apply -f production.yaml",
		// path and extension
		"/usr/local/bin/kubectl apply -f production.yaml",
		"./kubectl apply -f production.yaml",
		"kubectl.exe apply -f production.yaml",
		"$HOME/bin/terraform apply",
		// case, which NTFS ignores
		"Kubectl apply -f production.yaml",
		"KUBECTL APPLY -f production.yaml",
		"Helm Upgrade app ./chart",
		"Terraform.EXE Apply",
		// line continuation
		"kubectl \\\n apply -f production.yaml",
		"kubectl \\\n  -n prod \\\n  apply -f production.yaml",
		// wrapper spellings the matcher must see through, not just the two
		// executable-plus-operation words at the front of the line
		"timeout 30 kubectl apply -f production.yaml",
		"timeout -k 5 30s helm upgrade app ./chart",
		"doas -u deploy terraform apply plan.tfplan",
		"nice -n 10 kubectl apply -f production.yaml",
		"ionice -c 3 kubectl apply -f production.yaml",
		"stdbuf -oL kubectl apply -f production.yaml",
		"setsid -w kubectl apply -f production.yaml",
		"flock -n /tmp/deploy.lock kubectl apply -f production.yaml",
		"flock /tmp/deploy.lock -c 'kubectl apply -f production.yaml'",
		"flock -n /tmp/deploy.lock -c 'helm upgrade app ./chart'",
		"timeout 10 -s KILL kubectl apply -f production.yaml",
	}, "deny")
}

func TestInlineAssignmentNeverAuthorizes(t *testing.T) {
	record := t.TempDir() + "/change.md"
	if err := os.WriteFile(record, []byte("# Change Record: test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := "AGX_RELEASE_RECORD=" + record + " kubectl apply -f production.yaml"
	payload := buildEvent(t, "Bash", map[string]any{"command": command})
	// The inline assignment names a real file, but the host environment is
	// what authorizes, and it is empty here.
	if kind, message := runWithRecord(t, payload, ""); kind != "deny" {
		t.Errorf("inline assignment authorized: kind = %q (%s)", kind, message)
	}
	// With the host environment set, the same command passes; the prefix is
	// simply ignored.
	if kind, message := runWithRecord(t, payload, record); kind != "pass" {
		t.Errorf("kind = %q, want pass (%s)", kind, message)
	}
}

func TestShellEquivalentQuotedOrEscapedWordsAreDenied(t *testing.T) {
	assertEach(t, []string{
		`"kubectl" apply -f production.yaml`,
		`kube''ctl apply -f production.yaml`,
		`kubec\tl apply -f production.yaml`,
		`helm "upgrade" app ./chart`,
		`terra\form 'apply' plan.tfplan`,
	}, "deny")
}

func TestQuotedSeparatorsAndGroupingAreInert(t *testing.T) {
	assertEach(t, []string{
		`printf '%s\n' 'note; kubectl apply -f production.yaml'`,
		`printf '%s\n' 'note | kubectl apply -f production.yaml'`,
		`printf '%s\n' 'note & helm upgrade app ./chart'`,
		`printf "%s\n" "note; terraform apply plan.tfplan"`,
		`printf "%s\n" "note && kubectl apply -f production.yaml"`,
		`printf "%s\n" "note || helm upgrade app ./chart"`,
		`printf "%s\n" "(kubectl apply -f production.yaml)"`,
		`printf '%s\n' '{ helm upgrade app ./chart; }'`,
	}, "pass")
}

func TestCommandSubstitutionStartsAnExecutableSegment(t *testing.T) {
	assertEach(t, []string{
		`result=$(kubectl apply -f production.yaml)`,
		"result=`helm upgrade app ./chart`",
		`printf "%s\n" "$(terraform apply plan.tfplan)"`,
		`result=$(printf ready; kubectl apply -f production.yaml)`,
	}, "deny")
}

func TestSingleQuotesKeepCommandSubstitutionInert(t *testing.T) {
	assertEach(t, []string{
		`printf '%s\n' '$(kubectl apply -f production.yaml)'`,
		"printf '%s\\n' '`helm upgrade app ./chart`'",
	}, "pass")
}

func TestParameterExpansionBracesDoNotOpenASegment(t *testing.T) {
	// `${` is an expansion, not a group. If its brace split the segment, the
	// remainder "HOME}/kubectl apply" would read as a deploy.
	assertEach(t, []string{
		`echo ${HOME}/kubectl apply`,
		`printf '%s' "${PATH}" kubectl apply`,
	}, "pass")
}

func TestNearMissesPass(t *testing.T) {
	assertEach(t, []string{
		"kubectl diff -f production.yaml",
		"kubectl get pods -n prod",
		"helm template app ./chart",
		"helm status app",
		"terraform plan",
		"terraform show",
		"echo kubectl apply -f production.yaml",
		"grep apply kubectl.log",
		"kubectl apply-config",
		"kubectl -- apply",
		// Under a POSIX shell the backslashes are escapes: this runs
		// C:toolskubectl.exe, which is not kubectl. The PowerShell tool reads
		// the same text as a path and denies it.
		`C:\tools\kubectl.exe apply -f production.yaml`,
	}, "pass")
}

// TestDocumentedGapsPass pins the residual gaps named in the
// Release-guardian limitations section of hooks/README.md. Closing one of
// these is welcome; do it by moving the row to TestBypassCorpusIsDenied and
// updating that README section in the same change.
func TestDocumentedGapsPass(t *testing.T) {
	assertEach(t, []string{
		"$KUBECTL apply -f production.yaml",
	}, "pass")
}

func TestD05LiteralWrappers(t *testing.T) {
	assertEach(t, []string{
		`bash -c 'kubectl apply -f production.yaml'`,
		`sh -lc 'helm upgrade app ./chart' ignored`,
		`pwsh -NoProfile -Command 'terraform apply plan.tfplan'`,
		`env -i -u HOME FOO=1 -- kubectl apply -f production.yaml`,
		`command -- helm upgrade app ./chart`,
		`nohup -- terraform apply plan.tfplan`,
		`exec kubectl apply -f production.yaml`,
		`sudo -E -n -u root -g wheel -- kubectl apply -f production.yaml`,
		`time -p -- helm upgrade app ./chart`,
		`xargs -0 -r -n 1 -P 2 -I ITEM -- kubectl apply -f ITEM`,
	}, "deny")
}

func TestD05InertWrapperDataPasses(t *testing.T) {
	assertEach(t, []string{
		`printf '%s\n' "bash -c 'kubectl apply -f production.yaml'"`,
		`bash -c 'printf %s "kubectl apply -f production.yaml"'`,
		`command -v kubectl`,
		`command -V helm`,
	}, "pass")
}

func TestD05UninspectableWrappersFailClosedWithoutRecord(t *testing.T) {
	for _, command := range []string{
		`bash script.sh`,
		`bash -c "$COMMAND"`,
		`pwsh -EncodedCommand ZgBvAG8A`,
		`env -S 'kubectl apply'`,
		`sudo env nohup exec time kubectl apply`,
	} {
		t.Run(command, func(t *testing.T) {
			kind, message := bashCommand(t, command)
			if kind != "deny" || !strings.Contains(message, "could not inspect a recognized execution path") {
				t.Fatalf("kind/message = %q/%q", kind, message)
			}
		})
	}
}

func TestUnreadableBashInputIsDenied(t *testing.T) {
	huge := "1" + strings.Repeat("0", 400)
	payload := buildEvent(t, "Bash", map[string]any{"command": "printf safe", "n": json.Number(huge)})
	kind, message := run(t, payload)
	if kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	for _, want := range []string{"could not read normalized input", "run /agentics:gate change ($agentics:gate change on Codex) and set the release record in the host environment"} {
		if !strings.Contains(message, want) {
			t.Errorf("message missing %q: %s", want, message)
		}
	}
}

func TestMissingOrNonStringBashCommandIsDenied(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
	}{
		{name: "missing", input: map[string]any{}},
		{name: "null", input: map[string]any{"command": nil}},
		{name: "object", input: map[string]any{"command": map[string]any{}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := buildEvent(t, "Bash", tc.input)
			kind, message := run(t, payload)
			if kind != "deny" {
				t.Fatalf("kind = %q, want deny (%s)", kind, message)
			}
			for _, want := range []string{"could not read normalized input", "run /agentics:gate change ($agentics:gate change on Codex) and set the release record in the host environment"} {
				if !strings.Contains(message, want) {
					t.Errorf("message missing %q: %s", want, message)
				}
			}
		})
	}
}

func TestExistingReleaseRecordAuthorizesDeploy(t *testing.T) {
	record := t.TempDir() + "/change.md"
	if err := os.WriteFile(record, []byte("# Change Record: test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload := buildEvent(t, "Bash", map[string]any{"command": "helm upgrade app ./chart"})
	if kind, message := runWithRecord(t, payload, record); kind != "pass" {
		t.Errorf("kind = %q, want pass (%s)", kind, message)
	}
}

func TestD01KubernetesMutations(t *testing.T) {
	assertEach(t, []string{
		`kubectl apply -f production.yaml`,
		`kubectl create -f production.yaml`,
		`kubectl delete deployment api`,
		`kubectl patch deployment api -p '{}'`,
		`kubectl replace -f production.yaml`,
		`kubectl scale deployment api --replicas=3`,
		`kubectl create job review-probe --image --help`,
		`kubectl create deployment review-probe --port --help`,
		`kubectl create deployment review-probe -r --help`,
		`kubectl create configmap review-probe --from-literal --help`,
		`kubectl create configmap review-probe --from-file --help`,
		`kubectl create secret generic review-probe --type --help`,
		`kubectl create secret tls review-probe --cert --help`,
		`kubectl create --raw --help`,
		`kubectl create --validate --help`,
		`kubectl set image deployment/api api=image:v2`,
		`kubectl set env deployment/api FEATURE=true`,
		`kubectl rollout restart deployment/api`,
		`kubectl rollout undo deployment/api`,
		`kubectl -n prod --context cluster apply -f production.yaml`,
	}, "deny")
}

func TestKubernetesCreateImageHelpOperandRecord(t *testing.T) {
	command := `kubectl create job review-probe --image --help`
	payload := buildEvent(t, "Bash", map[string]any{"command": command})
	if kind, message := run(t, payload); kind != "deny" {
		t.Fatalf("kind = %q, want deny (%s)", kind, message)
	}
	record := t.TempDir() + "/change.md"
	if err := os.WriteFile(record, []byte("# Change Record: test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if kind, message := runWithRecord(t, payload, record); kind != "pass" {
		t.Fatalf("kind = %q, want pass (%s)", kind, message)
	}
}

func TestD01KubernetesSafeForms(t *testing.T) {
	assertEach(t, []string{
		`kubectl get pods`,
		`kubectl describe deployment api`,
		`kubectl diff -f production.yaml`,
		`kubectl rollout status deployment/api`,
		`kubectl rollout history deployment/api`,
		`kubectl --namespace apply get pods`,
		`kubectl apply --dry-run=client -f production.yaml`,
		`kubectl apply --dry-run client -f production.yaml`,
		`kubectl apply --help`,
		`kubectl apply -h`,
		`kubectl --help`,
	}, "pass")
	assertEach(t, []string{
		`kubectl apply --dry-run=server -f production.yaml`,
		`kubectl apply --dry-run=client --dry-run=server -f production.yaml`,
		`kubectl apply --dry-run --help`,
		`kubectl apply -f --dry-run=client`,
		`kubectl apply --dry-run=client -f production.yaml --help --help=false`,
		`kubectl apply -f production.yaml --help --help=false`,
		`xargs kubectl apply --dry-run=client`,
	}, "deny")
	assertEach(t, []string{
		`kubectl apply -k --dry-run=client`,
		`kubectl apply --kustomize --help`,
	}, "deny")
}

func TestD02HelmMutationsAndSafeForms(t *testing.T) {
	assertEach(t, []string{
		`helm install app ./chart`,
		`helm upgrade app ./chart`,
		`helm rollback app 1`,
		`helm uninstall app`,
		`helm --kube-context prod upgrade app ./chart`,
		`helm install app ./chart --dry-run=client`,
		`helm upgrade app ./chart --dry-run=client --hide-secret=false`,
		`helm upgrade app ./chart --dry-run=server --hide-secret`,
		`helm upgrade app ./chart --dry-run=client --hide-secret --hide-secret=false`,
		`helm upgrade app ./chart --dry-run=client --set 'name=--hide-secret' --hide-secret=false`,
		`xargs helm upgrade app ./chart --dry-run=client --hide-secret`,
	}, "deny")
	assertEach(t, []string{
		`helm list`,
		`helm status app`,
		`helm history app`,
		`helm template app ./chart`,
		`helm lint ./chart`,
		`helm install app ./chart --dry-run=client --hide-secret`,
		`helm upgrade app ./chart --dry-run client --hide-secret=true`,
		`helm upgrade app ./chart --dry-run=client --set 'name=--dry-run=server' --hide-secret=true`,
		`helm upgrade --help`,
		`helm --help`,
	}, "pass")
	assertEach(t, []string{
		`helm upgrade app ./chart --dry-run=client --hide-secret --hide-secret=false`,
		`helm upgrade app ./chart --dry-run=client --set --hide-secret --hide-secret=false`,
		`helm upgrade app ./chart --dry-run=client --hide-secret --help --help=false`,
		`helm install app ./chart --dry-run=client --post-renderer-args --hide-secret`,
		`helm upgrade app ./chart --dry-run=client --post-renderer-args=--hide-secret`,
		`helm upgrade app ./chart --dry-run=server --post-renderer-args --help`,
		`helm upgrade app ./chart --dry-run=client --hide-secret=false --post-renderer-args --help`,
	}, "deny")
}

func TestD03InfrastructureMutations(t *testing.T) {
	assertEach(t, []string{
		`terraform apply plan.tfplan`,
		`terraform destroy -auto-approve`,
		`terraform -chdir=infra apply plan.tfplan`,
		`tofu apply plan.tfplan`,
		`tofu destroy -auto-approve`,
		`pulumi up --yes`,
		`pulumi destroy --yes`,
		`bash -c 'tofu apply plan.tfplan'`,
		`env -i aws cloudformation deploy --template-file stack.yaml`,
		`command gcloud run deploy api`,
		`bash -c 'vercel --prod'`,
	}, "deny")
	assertEach(t, []string{
		`terraform plan`,
		`terraform plan -destroy`,
		`terraform validate`,
		`terraform fmt -check`,
		`tofu plan -destroy`,
		`pulumi preview`,
		`terraform apply --help`,
	}, "pass")
}

func TestD04CloudAndHostingMutations(t *testing.T) {
	assertEach(t, []string{
		`aws cloudformation deploy --template-file stack.yaml`,
		`aws --profile prod cloudformation create-stack --stack-name app`,
		`aws cloudformation update-stack --stack-name app`,
		`aws cloudformation delete-stack --stack-name app`,
		`aws cloudformation execute-change-set --change-set-name app`,
		`aws cloudformation deploy --no-execute-changeset`,
		`gcloud run deploy api --image image:v2`,
		`gcloud --project prod run deploy api --no-traffic`,
		`vercel --prod`,
		`vercel --prod=false`,
		`vercel deploy --target production`,
		`vercel --target=production`,
		`vercel --target=production --target=preview`,
	}, "deny")
	assertEach(t, []string{
		`aws cloudformation describe-stacks`,
		`aws cloudformation list-stacks`,
		`aws cloudformation create-change-set --stack-name app`,
		`aws cloudformation deploy help`,
		`gcloud run services describe api`,
		`gcloud run services list`,
		`gcloud run deploy --help`,
		`vercel`,
		`vercel deploy`,
		`vercel deploy --target preview`,
		`vercel prod`,
		`vercel --prod --help`,
		`vercel --prod --help=false`,
		`vercel --prod --help=anything`,
		`vercel --version`,
		`vercel -v`,
	}, "pass")
	assertEach(t, []string{
		`vercel list --target=production`,
		`vercel ls --target=production`,
		`vercel deploy --name --prod`,
		`vercel deploy --name=--prod`,
		`vercel deploy --target preview --name --target=production`,
	}, "pass")
}

func TestD03D04OperandsCannotManufactureHelpOrVerbs(t *testing.T) {
	assertEach(t, []string{
		`terraform apply -var --help`,
		`pulumi up --config --help`,
		`aws --profile help cloudformation deploy`,
		`aws cloudformation deploy --stack-name help`,
		`gcloud --format --help run deploy`,
		`gcloud run deploy --format --help`,
		`vercel deploy --target --help`,
	}, "deny")
}

func TestD03D04ReleaseRecordPermitsNamedMutations(t *testing.T) {
	record := t.TempDir() + "/change.md"
	if err := os.WriteFile(record, []byte("# Change Record: test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		`tofu destroy`,
		`pulumi up`,
		`aws cloudformation execute-change-set --change-set-name app`,
		`gcloud run deploy api`,
		`vercel --prod`,
	} {
		t.Run(command, func(t *testing.T) {
			payload := buildEvent(t, "Bash", map[string]any{"command": command})
			if kind, message := runWithRecord(t, payload, record); kind != "pass" {
				t.Fatalf("kind = %q, want pass (%s)", kind, message)
			}
		})
	}
}

func TestMissingOrDirectoryReleaseRecordDoesNotAuthorize(t *testing.T) {
	dir := t.TempDir()
	payload := buildEvent(t, "Bash", map[string]any{"command": "terraform apply plan.tfplan"})
	for _, record := range []string{dir + "/missing.md", dir} {
		t.Run(record, func(t *testing.T) {
			if kind, message := runWithRecord(t, payload, record); kind != "deny" {
				t.Errorf("kind = %q, want deny (%s)", kind, message)
			}
		})
	}
}

// TestEnvironmentFilterConfigErrorDeniesEveryInspectedCall: a misconfigured
// AGX_RELEASE_ENVIRONMENTS must not silently narrow the gate, so it denies
// every inspected shell call, deploy-shaped or not.
func TestEnvironmentFilterConfigErrorDeniesEveryInspectedCall(t *testing.T) {
	for _, raw := range []string{"", " , ", ","} {
		t.Run("empty/"+raw, func(t *testing.T) {
			t.Setenv(environmentsVar, raw)
			for _, command := range []string{"echo hi", "kubectl apply -f production.yaml"} {
				if kind, message := bashCommand(t, command); kind != "deny" || !strings.Contains(message, environmentsVar+" is set but names no environment") {
					t.Fatalf("command %q: kind=%q message=%q", command, kind, message)
				}
			}
		})
	}
	t.Run("control character", func(t *testing.T) {
		t.Setenv(environmentsVar, "prod\x01")
		for _, command := range []string{"echo hi", "kubectl apply -f production.yaml"} {
			if kind, message := bashCommand(t, command); kind != "deny" || !strings.Contains(message, environmentsVar+" holds a control character") {
				t.Fatalf("command %q: kind=%q message=%q", command, kind, message)
			}
		}
	})
	t.Run("non-shell tool still passes", func(t *testing.T) {
		t.Setenv(environmentsVar, "")
		payload := buildEvent(t, "Read", map[string]any{"command": "kubectl apply -f production.yaml"})
		if kind, message := run(t, payload); kind != "pass" {
			t.Fatalf("kind = %q, want pass (%s)", kind, message)
		}
	})
}

// TestEnvironmentFilterDeniesMarkedDeploys pins the configured-filter deny
// side: every one of these carries one of the configured markers in its own
// invocation and is a recognized deploy shape, so it denies without a
// record even though other, unmarked deploys would now pass.
func TestEnvironmentFilterDeniesMarkedDeploys(t *testing.T) {
	t.Setenv(environmentsVar, "prod-sub,production.tfvars,customer-facing")
	assertEach(t, []string{
		`az deployment group create --subscription prod-sub -f main.bicep`,
		`terraform apply -var-file=production.tfvars`,
		`helmfile --environment customer-facing sync`,
		`AWS_PROFILE=prod-sub aws cloudformation deploy --stack-name s --template-file t.yaml`,
		// A non-literal word could expand to any marker, so it fails closed.
		`kubectl apply -f $MANIFEST`,
	}, "deny")
}

// TestEnvironmentFilterPassesUnmarkedDeploys: a recognized deploy shape that
// does not carry a configured marker in its own invocation passes when the
// filter is configured.
func TestEnvironmentFilterPassesUnmarkedDeploys(t *testing.T) {
	t.Setenv(environmentsVar, "prod-sub,production.tfvars,customer-facing")
	assertEach(t, []string{
		`terraform apply -var-file=nonprod-production.tfvars`,
		`kubectl apply -f x.yaml --context nonprod`,
		`echo prod-sub; kubectl apply -f x.yaml`,
		`kubectl apply -f x.yaml # prod-sub`,
		`terraform plan -var-file=production.tfvars`,
	}, "pass")
}

// TestEnvironmentFilterParserLimitStillDenies: the parser-limit deny message
// is unaffected by the filter, configured or not.
func TestEnvironmentFilterParserLimitStillDenies(t *testing.T) {
	command := `bash -c "$COMMAND"`
	for _, raw := range []string{"", "prod-sub,production.tfvars"} {
		t.Run("configured="+raw, func(t *testing.T) {
			if raw != "" {
				t.Setenv(environmentsVar, raw)
			}
			kind, message := bashCommand(t, command)
			if kind != "deny" || !strings.Contains(message, "could not inspect a recognized execution path") {
				t.Fatalf("kind/message = %q/%q", kind, message)
			}
		})
	}
}

// TestEnvironmentFilterAuditLineCarriesRule: an authorized, filter-matched
// deploy's audit line names the matched AGX_RELEASE_ENVIRONMENTS entry by
// index, never the marker literal itself.
func TestEnvironmentFilterAuditLineCarriesRule(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RELEASE_GUARD_LOG_DIR", dir)
	t.Setenv(environmentsVar, "prod-sub,production.tfvars,customer-facing")
	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "terraform apply -var-file=production.tfvars"})
	if kind, msg := runWithRecord(t, payload, record); kind != "pass" {
		t.Fatalf("kind = %q (%s), want pass", kind, msg)
	}
	data, err := os.ReadFile(filepath.Join(dir, "guard.log"))
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)
	if !strings.Contains(line, `"environment_rule":"`+environmentsVar+`[1]"`) {
		t.Fatalf("audit line missing the environment_rule field:\n%s", line)
	}
	var decoded struct {
		EnvironmentRule string `json:"environment_rule"`
	}
	if err := json.Unmarshal([]byte(strings.TrimRight(line, "\n")), &decoded); err != nil {
		t.Fatalf("audit line is not valid JSON: %v\n%s", err, line)
	}
	if decoded.EnvironmentRule == "production.tfvars" {
		t.Fatalf("audit line names the marker literal instead of the rule index:\n%s", line)
	}
}

// TestEnvironmentFilterAuditLineOmitsRuleWhenUnconfigured: without the
// filter configured, the field is omitted entirely rather than empty.
func TestEnvironmentFilterAuditLineOmitsRuleWhenUnconfigured(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RELEASE_GUARD_LOG_DIR", dir)
	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	if kind, msg := runWithRecord(t, payload, record); kind != "pass" {
		t.Fatalf("kind = %q (%s), want pass", kind, msg)
	}
	data, err := os.ReadFile(filepath.Join(dir, "guard.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "environment_rule") {
		t.Fatalf("audit line carries environment_rule though the filter is unconfigured:\n%s", data)
	}
}

func writeRecord(t *testing.T, content string, age time.Duration) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "change-record.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(-age)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRecordMustCarryTheGateMarkerAndBeFresh(t *testing.T) {
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	cases := []struct {
		name, content string
		age           time.Duration
		want          string
	}{
		{"fresh gate record", "# Change Record: rotate keys\n\n**ID:** 2026-09-12-rotate\n", time.Hour, "pass"},
		{"empty file", "", time.Minute, "deny"},
		{"touch without marker", "approved\n", time.Minute, "deny"},
		{"stale record", "# Change Record: old\n", 25 * time.Hour, "deny"},
		{"future mtime beyond tolerance", "# Change Record: rotate keys\n", -2 * time.Hour, "deny"},
		{"future mtime within clock skew tolerance", "# Change Record: rotate keys\n", -2 * time.Minute, "pass"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RELEASE_GUARD_LOG_DIR", t.TempDir())
			kind, msg := runWithRecord(t, payload, writeRecord(t, tc.content, tc.age))
			if kind != tc.want {
				t.Fatalf("kind = %q (%s), want %s", kind, msg, tc.want)
			}
			if strings.Contains(msg, "AGX_RELEASE_RECORD") {
				t.Fatalf("the reason names the variable: %s", msg)
			}
		})
	}
}

// TestRecordHashFailureDeniesWithTheDistinctReason exercises the AGX_RELEASE_RECORD_SHA256
// branch at the Run seam, not just validReleaseRecord directly: a hash
// mismatch, a not-hex value (including set-but-empty), and an oversized
// record with the hash set must each deny with the exact host-set-hash
// reason, not the generic production-shaped-deploy message that would
// appear if the branch selecting that reason were deleted. A matching hash
// still passes.
func TestRecordHashFailureDeniesWithTheDistinctReason(t *testing.T) {
	record := writeRecord(t, "# Change Record: rotate keys\n", time.Minute)
	content, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})

	for _, tc := range []struct {
		name string
		hash string
		want string
	}{
		{
			name: "hash mismatch",
			hash: strings.Repeat("a", 64),
			want: "release-guardian: the release record does not match the host-set record hash; " + recordRemedy,
		},
		{
			name: "not hex, set but empty",
			hash: "",
			want: "release-guardian: the host-set record hash is not a SHA-256 hex digest; " + recordRemedy,
		},
		{
			name: "not hex, wrong length",
			hash: "deadbeef",
			want: "release-guardian: the host-set record hash is not a SHA-256 hex digest; " + recordRemedy,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(recordHashVar, tc.hash)
			kind, message := runWithRecord(t, payload, record)
			if kind != "deny" {
				t.Fatalf("kind = %q, want deny (%s)", kind, message)
			}
			if settingstest.WithoutNote(message) != tc.want {
				t.Fatalf("message = %q, want %q", message, tc.want)
			}
		})
	}

	t.Run("oversized record with hash set", func(t *testing.T) {
		big := make([]byte, recordHashLimit+1)
		for i := range big {
			big[i] = 'x'
		}
		bigRecord := writeRecord(t, recordMarker+"\n"+string(big), time.Minute)
		t.Setenv(recordHashVar, strings.Repeat("a", 64))
		kind, message := runWithRecord(t, payload, bigRecord)
		if kind != "deny" {
			t.Fatalf("kind = %q, want deny (%s)", kind, message)
		}
		want := "release-guardian: the release record exceeds 16 MiB; " + recordRemedy
		if settingstest.WithoutNote(message) != want {
			t.Fatalf("message = %q, want %q", message, want)
		}
	})

	t.Run("matching hash passes", func(t *testing.T) {
		t.Setenv(recordHashVar, sha256Hex(content))
		kind, message := runWithRecord(t, payload, record)
		if kind != "pass" {
			t.Fatalf("kind = %q, want pass (%s)", kind, message)
		}
	})
}

func TestAuthorizedDeployWritesOneAuditLine(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RELEASE_GUARD_LOG_DIR", dir)
	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	if kind, msg := runWithRecord(t, payload, record); kind != "pass" {
		t.Fatalf("kind = %q (%s), want pass", kind, msg)
	}
	data, err := os.ReadFile(filepath.Join(dir, "guard.log"))
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)
	for _, want := range []string{`"event":"deploy_authorized"`, `"tool":"Bash"`, `"executable":"kubectl"`, `"operation":"apply"`, `"category":"kubernetes_deploy"`, `"record":"` + strings.ReplaceAll(record, `\`, `\\`) + `"`} {
		if !strings.Contains(line, want) {
			t.Errorf("audit line missing %s:\n%s", want, line)
		}
	}
	if strings.Count(line, "\n") != 1 {
		t.Fatalf("want exactly one JSONL record, got:\n%s", line)
	}
}

// TestParseLimitPassWritesAnAuditLine: a wrapper the parser cannot fully
// read (here "doas --", which is outside doas's literal wrapper contract)
// still authorizes with a valid record. That pass gets its own audit
// event, distinct from deploy_authorized, carrying the parse-limit reason.
func TestParseLimitPassWritesAnAuditLine(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RELEASE_GUARD_LOG_DIR", dir)
	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "doas -- kubectl apply -f production.yaml"})
	if kind, msg := runWithRecord(t, payload, record); kind != "pass" {
		t.Fatalf("kind = %q (%s), want pass", kind, msg)
	}
	data, err := os.ReadFile(filepath.Join(dir, "guard.log"))
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)
	if !strings.Contains(line, `"event":"deploy_parse_limit_authorized"`) {
		t.Errorf("audit line missing the parse-limit event:\n%s", line)
	}
	var decoded struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(strings.TrimRight(line, "\n")), &decoded); err != nil {
		t.Fatalf("audit line is not valid JSON: %v\n%s", err, line)
	}
	if decoded.Reason == "" {
		t.Fatalf("audit line carries an empty reason:\n%s", line)
	}
	if strings.Count(line, "\n") != 1 {
		t.Fatalf("want exactly one JSONL record, got:\n%s", line)
	}
}

// TestDeployAuthorityRefusalBytes pins the deny message and the audit reason of an inspection role that has no
// authority to run. Without a record the call is denied with the authority text; a valid record authorizes it and the
// parse-limit audit line carries the same text.
func TestDeployAuthorityRefusalBytes(t *testing.T) {
	const reason = "Bash inspection authority unavailable"
	input := map[string]any{"command": "bash --help", "shell": "bash"}

	t.Run("no record denies", func(t *testing.T) {
		kind, message := run(t, buildEvent(t, "Bash", input))
		want := "release-guardian could not inspect a recognized execution path (" + reason + "); " + recordRemedy
		if kind != "deny" || message != want {
			t.Fatalf("kind = %q, message = %q; want deny %q", kind, message, want)
		}
	})

	t.Run("valid record passes and logs the reason", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("RELEASE_GUARD_LOG_DIR", dir)
		record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
		if kind, message := runWithRecord(t, buildEvent(t, "Bash", input), record); kind != "pass" {
			t.Fatalf("kind = %q (%s), want pass", kind, message)
		}
		data, err := os.ReadFile(filepath.Join(dir, "guard.log"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(string(data), "\n") != 1 {
			t.Fatalf("want exactly one JSONL record, got:\n%s", data)
		}
		var decoded struct {
			Event  string `json:"event"`
			Tool   string `json:"tool"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(strings.TrimRight(string(data), "\n")), &decoded); err != nil {
			t.Fatalf("audit line is not valid JSON: %v\n%s", err, data)
		}
		if decoded.Event != "deploy_parse_limit_authorized" || decoded.Tool != "Bash" || decoded.Reason != reason {
			t.Fatalf("audit line = %+v, want deploy_parse_limit_authorized, Bash, %q", decoded, reason)
		}
	})
}

func TestAuditWriteFailureDenies(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELEASE_GUARD_LOG_DIR", filepath.Join(blocker, "logs"))
	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	if kind, msg := runWithRecord(t, payload, record); kind != "deny" || !strings.Contains(msg, "could not record the authorized deploy") {
		t.Fatalf("kind = %q (%s), want deny naming the audit failure", kind, msg)
	}
}

// auditAliasSentence is the message a redirected guard.log path must carry,
// pinned as a literal so a change to the shared wording is seen here.
const auditAliasSentence = "release-guardian refused to append guard.log: a linked path component or a non-regular guard.log would redirect the write"

// TestAuditLogSymlinkLeafDenies: a guard.log that is a symlink inside the
// configured log directory must not be followed — the victim keeps its bytes
// and the authorized deploy is denied rather than silently unrecorded. The
// deny reason must carry the alias sentinel text, not just the generic
// "could not record" wrapper, so a regression that drops the check still
// fails this test even where O_NOFOLLOW alone would have caught the write.
func TestAuditLogSymlinkLeafDenies(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RELEASE_GUARD_LOG_DIR", dir)
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(victim, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hookutiltest.SymlinkOrSkip(t, victim, filepath.Join(dir, "guard.log"))

	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	kind, msg := runWithRecord(t, payload, record)
	if kind != "deny" || !strings.Contains(msg, "could not record the authorized deploy") {
		t.Fatalf("kind = %q (%s), want deny naming the audit failure", kind, msg)
	}
	if !strings.Contains(msg, auditAliasSentence) {
		t.Fatalf("deny reason %q does not carry the alias sentinel text %q", msg, auditAliasSentence)
	}
	raw, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "original\n" {
		t.Fatalf("the symlink target was written: %q", raw)
	}
}

// TestAuditLogRelativeSymlinkedParentDenies: a symlinked `.agentics` under the
// process working directory is the parent-link form of the same redirect,
// reached only through the relative fallback when no log root and no
// RELEASE_GUARD_LOG_DIR are configured.
func TestAuditLogRelativeSymlinkedParentDenies(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	t.Setenv("RELEASE_GUARD_LOG_DIR", "")
	outside := t.TempDir()
	hookutiltest.SymlinkOrSkip(t, outside, filepath.Join(cwd, ".agentics"))

	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	// buildEvent always sets runtime.log_root to nil, which is the fallback
	// branch this test needs: the relative ".agentics/logs/release-guardian"
	// path under the (chdir'd) working directory.
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	kind, msg := runWithRecord(t, payload, record)
	if kind != "deny" || !strings.Contains(msg, "could not record the authorized deploy") {
		t.Fatalf("kind = %q (%s), want deny naming the audit failure", kind, msg)
	}
	if _, err := os.Stat(filepath.Join(outside, "logs", "release-guardian", "guard.log")); !os.IsNotExist(err) {
		t.Fatalf("guard.log was written through the parent link (err=%v)", err)
	}
}

// TestAuditLogAbsoluteDirUnderWorkingDirectoryIsWalked: this repository is
// its own plugin, so an absolute RELEASE_GUARD_LOG_DIR under the process
// working directory (e.g. <cwd>/agentics/logs/release-guardian) is still
// repository-controlled, and every component from the working directory
// down to it must be walked the same as the relative fallback.
func TestAuditLogAbsoluteDirUnderWorkingDirectoryIsWalked(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	outside := t.TempDir()
	hookutiltest.SymlinkOrSkip(t, outside, filepath.Join(cwd, "agentics"))
	t.Setenv("RELEASE_GUARD_LOG_DIR", filepath.Join(cwd, "agentics", "logs", "release-guardian"))

	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	kind, msg := runWithRecord(t, payload, record)
	if kind != "deny" || !strings.Contains(msg, "could not record the authorized deploy") {
		t.Fatalf("kind = %q (%s), want deny naming the audit failure", kind, msg)
	}
	if _, err := os.Stat(filepath.Join(outside, "logs", "release-guardian", "guard.log")); !os.IsNotExist(err) {
		t.Fatalf("guard.log was written through the parent link (err=%v)", err)
	}
}

// TestAuditLogAbsoluteDirUnderWorkingDirectoryIsWalkedJunction is the
// Windows-only sibling of the symlink case above: a junction reports
// ModeIrregular rather than ModeSymlink, so the component walk must refuse
// it too.
func TestAuditLogAbsoluteDirUnderWorkingDirectoryIsWalkedJunction(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	outside := t.TempDir()
	hookutiltest.JunctionOrSkip(t, outside, filepath.Join(cwd, "agentics"))
	t.Setenv("RELEASE_GUARD_LOG_DIR", filepath.Join(cwd, "agentics", "logs", "release-guardian"))

	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	kind, msg := runWithRecord(t, payload, record)
	if kind != "deny" || !strings.Contains(msg, "could not record the authorized deploy") {
		t.Fatalf("kind = %q (%s), want deny naming the audit failure", kind, msg)
	}
	if _, err := os.Stat(filepath.Join(outside, "logs", "release-guardian", "guard.log")); !os.IsNotExist(err) {
		t.Fatalf("guard.log was written through the parent link (err=%v)", err)
	}
}

// TestAuditLogAbsoluteDirStillWrites: an operator-configured absolute
// RELEASE_GUARD_LOG_DIR outside the working directory that is itself a
// symlinked directory alias keeps its parents unchecked and still writes —
// only a symlinked guard.log leaf, or an owned parent (the relative fallback
// or an absolute dir under the working directory), is refused.
func TestAuditLogAbsoluteDirStillWrites(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	hookutiltest.SymlinkOrSkip(t, real, link)
	t.Setenv("RELEASE_GUARD_LOG_DIR", link)

	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	kind, msg := runWithRecord(t, payload, record)
	if kind != "pass" {
		t.Fatalf("kind = %q (%s), want pass", kind, msg)
	}
	if _, err := os.Stat(filepath.Join(real, "guard.log")); err != nil {
		t.Fatalf("trusted root did not receive the record: %v", err)
	}
}
