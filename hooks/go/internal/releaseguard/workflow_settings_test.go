package releaseguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/settings"
	"github.com/muehz/agentics/hooks/go/internal/settings/settingstest"
)

// policyDeploys are recognized deployments that are workflow policy: none of
// their operation names deletes what it targets.
var policyDeploys = []string{
	"kubectl apply -f production.yaml",
	"kubectl scale deployment web --replicas=3",
	"kubectl rollout restart deployment web",
	"kubectl rollout undo deployment web",
	"helm install app ./chart",
	"helm upgrade app ./chart",
	"helm rollback app 1",
	"terraform apply plan.tfplan",
	"tofu apply plan.tfplan",
	"pulumi up",
	"terragrunt run-all apply",
	"helmfile sync",
	"helmfile apply",
	"aws cloudformation deploy --template-file t.yaml --stack-name s",
	"aws cloudformation create-stack --stack-name s --template-body file://t.json",
	"aws cloudformation update-stack --stack-name s --template-body file://t.json",
	"aws cloudformation execute-change-set --change-set-name c",
	"gcloud run deploy svc --image img",
	"vercel --prod",
	"az deployment group create -g rg --template-file t.json",
}

// destructiveDeploys are recognized deployments whose operation name deletes.
var destructiveDeploys = []string{
	"kubectl delete -f production.yaml",
	"helm uninstall app",
	"terraform destroy -auto-approve",
	"tofu destroy -auto-approve",
	"pulumi destroy",
	"terragrunt run-all destroy",
	"helmfile destroy",
	"helmfile delete",
	"aws cloudformation delete-stack --stack-name s",
}

func TestPolicyDeploysPassWithoutRecordWhenTheGateIsOff(t *testing.T) {
	stub := settingstest.Policy(settings.RequireDeployRecord)
	for _, command := range policyDeploys {
		t.Run(command, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("RELEASE_GUARD_LOG_DIR", dir)
			if kind, message := bashCommandFrom(t, stub.Source(), command); kind != "pass" {
				t.Fatalf("kind = %q (%s), want pass", kind, message)
			}
			if _, err := os.Stat(filepath.Join(dir, "guard.log")); err == nil {
				t.Fatal("a deployment that is no longer gated wrote an audit record")
			}
		})
	}
}

func TestPolicyDeploysAreGatedByDefaultAndNameTheKey(t *testing.T) {
	stub := settingstest.Policy()
	for _, command := range policyDeploys {
		t.Run(command, func(t *testing.T) {
			kind, message := bashCommandFrom(t, stub.Source(), command)
			if kind != "deny" {
				t.Fatalf("kind = %q (%s), want deny", kind, message)
			}
			if !strings.Contains(message, "\n\nSetting: policy.require_deploy_record. ") {
				t.Fatalf("the denial does not name its key: %q", message)
			}
		})
	}
}

func TestDestructiveDeploysKeepTheGateAndNameNoKey(t *testing.T) {
	stub := settingstest.Policy(settings.Keys...)
	for _, command := range destructiveDeploys {
		t.Run(command, func(t *testing.T) {
			kind, message := bashCommandFrom(t, stub.Source(), command)
			if kind != "deny" {
				t.Fatalf("kind = %q (%s), want deny with every setting off", kind, message)
			}
			if strings.Contains(message, "Setting") {
				t.Fatalf("a fixed gate advertised a setting: %q", message)
			}
		})
	}
	if stub.Calls() != 0 {
		t.Fatalf("settings were resolved %d times for destructive deployments", stub.Calls())
	}
}

func TestADestructiveInvocationAnywhereKeepsTheGate(t *testing.T) {
	stub := settingstest.Policy(settings.RequireDeployRecord)
	for _, command := range []string{
		"kubectl apply -f a.yaml && terraform destroy",
		"terraform destroy && kubectl apply -f a.yaml",
		"kubectl apply -f a.yaml; helm upgrade app ./chart; helm uninstall app",
		"kubectl apply -f a.yaml && doas -- true",
		"terraform apply plan.tfplan || bash -c \"$DEPLOY\"",
	} {
		t.Run(command, func(t *testing.T) {
			kind, message := bashCommandFrom(t, stub.Source(), command)
			if kind != "deny" {
				t.Fatalf("kind = %q (%s), want deny", kind, message)
			}
			if strings.Contains(message, "Setting") {
				t.Fatalf("a mixed command's denial advertised a setting: %q", message)
			}
		})
	}
}

func TestAnOpaqueInvocationKeepsTheGateAndNamesNoKey(t *testing.T) {
	stub := settingstest.Policy(settings.RequireDeployRecord)
	kind, message := bashCommandFrom(t, stub.Source(), "doas -- kubectl apply -f a.yaml")
	if kind != "deny" || strings.Contains(message, "Setting") {
		t.Fatalf("kind = %q, message %q", kind, message)
	}
}

func TestOnlyDeploymentCommandsAskForSettings(t *testing.T) {
	stub := settingstest.Policy()
	for _, command := range []string{"ls -la", "terraform plan", "kubectl get pods", "bash --help"} {
		bashCommandFrom(t, stub.Source(), command)
	}
	if stub.Calls() != 0 {
		t.Fatalf("settings were resolved %d times for commands that are no policy deployment", stub.Calls())
	}
	bashCommandFrom(t, stub.Source(), "kubectl apply -f a.yaml && helm upgrade app ./chart")
	if stub.Calls() != 1 {
		t.Fatalf("settings were resolved %d times for one command, want 1", stub.Calls())
	}
}

func TestAnEnabledGateStillHonorsAValidRecord(t *testing.T) {
	stub := settingstest.Policy()
	dir := t.TempDir()
	t.Setenv("RELEASE_GUARD_LOG_DIR", dir)
	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	if kind, message := runWithRecordFrom(t, stub.Source(), payload, record); kind != "pass" {
		t.Fatalf("kind = %q (%s), want pass", kind, message)
	}
	if _, err := os.Stat(filepath.Join(dir, "guard.log")); err != nil {
		t.Fatalf("an authorized deploy under an enabled gate lost its audit record: %v", err)
	}
}

func TestAnAuditFailureUnderAnEnabledGateNamesTheKey(t *testing.T) {
	stub := settingstest.Policy()
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELEASE_GUARD_LOG_DIR", filepath.Join(blocker, "logs"))
	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	kind, message := runWithRecordFrom(t, stub.Source(), payload, record)
	if kind != "deny" || !strings.Contains(message, "could not record the authorized deploy") || !strings.Contains(message, "policy.require_deploy_record") {
		t.Fatalf("kind = %q (%s)", kind, message)
	}
}

func TestADisabledGateWritesNoAuditRecordEvenWithAValidRecord(t *testing.T) {
	stub := settingstest.Policy(settings.RequireDeployRecord)
	dir := t.TempDir()
	t.Setenv("RELEASE_GUARD_LOG_DIR", dir)
	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	if kind, message := runWithRecordFrom(t, stub.Source(), payload, record); kind != "pass" {
		t.Fatalf("kind = %q (%s), want pass", kind, message)
	}
	if _, err := os.Stat(filepath.Join(dir, "guard.log")); !os.IsNotExist(err) {
		t.Fatalf("a deployment that is no longer gated wrote an audit record (stat err = %v)", err)
	}
}

func TestADisabledGateIgnoresAnUnwritableAuditLog(t *testing.T) {
	stub := settingstest.Policy(settings.RequireDeployRecord)
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELEASE_GUARD_LOG_DIR", filepath.Join(blocker, "logs"))
	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	if kind, message := runWithRecordFrom(t, stub.Source(), payload, record); kind != "pass" {
		t.Fatalf("kind = %q (%s), want pass: a disabled gate attempts no audit append", kind, message)
	}
}

func TestAHashFailureOnAPolicyDeployNamesTheKey(t *testing.T) {
	stub := settingstest.Policy()
	record := writeRecord(t, "# Change Record: deploy\n", time.Minute)
	t.Setenv(recordHashVar, strings.Repeat("a", 64))
	payload := buildEvent(t, "Bash", map[string]any{"command": "kubectl apply -f production.yaml"})
	kind, message := runWithRecordFrom(t, stub.Source(), payload, record)
	if kind != "deny" || !strings.Contains(message, "policy.require_deploy_record") {
		t.Fatalf("kind = %q (%s), want deny naming policy.require_deploy_record", kind, message)
	}
}

func TestTheEnvironmentFilterKeepsADestructiveInvocationGated(t *testing.T) {
	stub := settingstest.Policy(settings.RequireDeployRecord)
	t.Setenv(environmentsVar, "production")
	// A marker counts only as a whole word or the value side of name=value, so
	// both invocations carry "production" in one of those forms.
	command := "kubectl apply -f a.yaml --context production && terraform destroy -var env=production"
	if kind, message := bashCommandFrom(t, stub.Source(), command); kind != "deny" {
		t.Fatalf("kind = %q (%s), want deny", kind, message)
	}
}

func TestSettingsFilesDriveTheGateThroughRealFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(root, body string) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, ".agentics"), 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, ".agentics", "hooks.json")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	deploy := func() (string, string) {
		payload := buildEventAt(t, repo, "Bash", map[string]any{"command": "terraform apply plan.tfplan"})
		return run(t, payload)
	}

	write(repo, `{"version":1,"policy":{"require_deploy_record":false}}`)
	if kind, message := deploy(); kind != "pass" {
		t.Fatalf("repository false: kind = %q (%s)", kind, message)
	}

	write(home, `{"version":1,"policy":{"require_deploy_record":false}}`)
	write(repo, `{"version":1,"policy":{"require_deploy_record":true}}`)
	if kind, message := deploy(); kind != "deny" {
		t.Fatalf("repository true over user false: kind = %q (%s)", kind, message)
	}

	path := write(repo, `{"version":1,"policy":{"merge":false}}`)
	kind, message := deploy()
	if kind != "deny" || !strings.Contains(message, "ignored invalid "+path+" (unknown-key)") {
		t.Fatalf("broken repository file: kind = %q (%s)", kind, message)
	}
}
