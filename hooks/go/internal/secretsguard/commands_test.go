package secretsguard

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func TestSecretOutputOptionBoundariesThroughRun(t *testing.T) {
	for _, tc := range []struct {
		command, want, reason string
	}{
		{`helm install api chart --dry-run client`, "deny", "helm"},
		{`helm install api chart --dry-run server`, "deny", "helm"},
		{`helm install api chart --dry-run true`, "deny", "helm"},
		{`helm install api chart --dry-run false`, "pass", ""},
		{`helm install api chart --dry-run`, "deny", "helm"},
		{`helm install api chart --dry-run true --dry-run false`, "deny", "helm"},
		{`helm install api chart`, "pass", ""},
		{`helm install api chart --dry-run=false`, "pass", ""},
		{`helm install api chart --description --dry-run`, "pass", ""},
		{`helm install api chart --dry-run --wait`, "deny", "helm"},
		{`kubectl get secret api -o=json`, "deny", "kubectl"},
		{`kubectl get secret api -oyaml`, "deny", "kubectl"},
		{`kubectl get secret api -o ''`, "deny", "kubectl"},
		{`kubectl get secret api -o --watch`, "deny", "kubectl"},
		{`kubectl get secret api --output=`, "deny", "kubectl"},
		{`kubectl get secret api -o name -o yaml`, "deny", "kubectl"},
		{`kubectl get secret api -oname`, "pass", ""},
		{`kubectl get secret api -o=wide`, "pass", ""},
	} {
		t.Run(tc.command, func(t *testing.T) {
			payload := buildEvent(t, "Bash", map[string]any{
				"command": tc.command, "value": map[string]any{"command": tc.command},
			})
			kind, message := run(t, payload, nil)
			if kind != tc.want || (tc.reason != "" && !strings.Contains(message, tc.reason)) || (tc.want == "pass" && message != "") {
				t.Fatalf("outcome = %s %q, want %s with reason %q", kind, message, tc.want, tc.reason)
			}
		})
	}
}

// TestSecretsScanPosture pins what secrets-guardian does when the option scan
// cannot read a word: each kind of problem ends in a deny where the word may
// hide structured Secret output, and in a pass where the output is only a name.
func TestSecretsScanPosture(t *testing.T) {
	for _, tc := range []struct {
		command, want, reason string
	}{
		{`kubectl --unknown get secret x -o yaml`, "deny", "ambiguous option before its resource"},
		{`kubectl get --unknown x secret -o yaml`, "deny", "unknown resource or output mode"},
		{`kubectl get --unknown x pods -o name`, "pass", ""},
		{`kubectl get $RESOURCE -o yaml`, "deny", "unknown resource or output mode"},
		{`kubectl get secret x -o`, "deny", "uninspectable output mode"},
		{`helm install r c --dry-run`, "deny", "add --hide-secret=true"},
		{`helm install r c --dry-run --wait`, "deny", "add --hide-secret=true"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			payload := buildEvent(t, "Bash", map[string]any{
				"command": tc.command, "value": map[string]any{"command": tc.command},
			})
			kind, message := run(t, payload, nil)
			if kind != tc.want || (tc.reason != "" && !strings.Contains(message, tc.reason)) || (tc.want == "pass" && message != "") {
				t.Fatalf("outcome = %s %q, want %s with reason %q", kind, message, tc.want, tc.reason)
			}
		})
	}
}

func TestFrozenSecretPrintingContract(t *testing.T) {
	cases := []struct {
		command string
		deny    bool
	}{
		{"helm install api ./chart --dry-run=client", true},
		{"helm upgrade api ./chart --dry-run=client --hide-secret=false", true},
		{"helm upgrade api ./chart --dry-run=client --hide-secret=true", false},
		{"helm install api ./chart --dry-run=client --post-renderer-args --hide-secret", true},
		{"helm install api ./chart --dry-run=client --post-renderer-args --hide-secret=true", true},
		{"helm install api ./chart --dry-run=client --post-renderer-args=--hide-secret", true},
		{"helm upgrade api ./chart --dry-run=server --post-renderer-args --help", true},
		{"helm upgrade api ./chart --dry-run=client --hide-secret=false --post-renderer-args --help", true},
		{"aws secretsmanager get-secret-value --secret-id api --query SecretString", true},
		{"aws secretsmanager describe-secret --secret-id api", false},
		{"aws ssm get-parameter --name api --with-decryption", true},
		{"aws ssm get-parameters --cli-input-json=file://request.json", true},
		{"aws ssm get-parameter --name public", false},
		{"kubectl get secret/api -o yaml", true},
		{"kubectl get secrets --output=jsonpath={.items[*].data}", true},
		{"kubectl get secret/api -o name", false},
		{"kubectl get secrets", false},
		{"kubectl config view --raw", true},
		{"kubectl config view", false},
		{"terraform output database_password", true},
		{"terraform output -json", true},
		{"terraform output", false},
		{"terraform show -json plan.out", true},
		{"terraform state pull", true},
		{"terraform state list", false},
		{"tofu output token", true},
		{"kubectl get secret/api -o yaml --help", false},
		{"aws secretsmanager get-secret-value help", false},
		{"aws secretsmanager get-secret-value --secret-id help", true},
		{"aws ssm get-parameter --with-decryption --name help", true},
		{"kubectl --context --help get secret app -o yaml", true},
		{"helm --namespace --help upgrade api chart --dry-run=client", true},
		{"helm upgrade api chart --dry-run=client --content-cache --help", true},
		{"kubectl --context synthetic get secret app -o yaml --help", false},
		{"helm --namespace synthetic upgrade api chart --dry-run=client --help", false},
	}
	for _, test := range cases {
		t.Run(test.command, func(t *testing.T) {
			parsed := shellwords.Parse(test.command, shellwords.POSIX)
			if len(parsed.Invocations) != 1 {
				t.Fatalf("invocations = %d, want 1", len(parsed.Invocations))
			}
			_, denied := commandSecretReason(parsed.Invocations[0])
			if denied != test.deny {
				t.Fatalf("denied = %v, want %v", denied, test.deny)
			}
		})
	}
}

func TestDynamicCommandOutputControlsRemainConservative(t *testing.T) {
	cases := []struct {
		command string
		deny    bool
	}{
		{"kubectl get secret api --output=$FORMAT", true},
		{"kubectl config view --raw=$RAW", true},
		{"aws ssm get-parameter --name api --with-decryption=$DECRYPT", true},
		{"helm upgrade api chart --dry-run=$MODE", true},
		{"terraform output $NAME", true},
		{"terraform output -json=$MODE", true},
		{"terraform output -raw=$MODE", true},
		{"terraform show -json=$MODE", true},
		{"kubectl get $RESOURCE -o name", false},
		{"kubectl get secret $RESOURCE -o name", false},
		{"kubectl get pods secret -o yaml", false},
		{"kubectl get configmap namedsecret -o yaml", false},
		{"kubectl get sec${RESOURCE_PART}ret -o yaml", true},
		{"kubectl get --namespace app pods -o yaml", false},
		{"kubectl get --context synthetic secret app -o yaml", true},
		{"kubectl get --kubeconfig synthetic secrets -o json", true},
		{"kubectl get --as synthetic secret/app --output=go-template={{.data}}", true},
		{"kubectl get --context=synthetic secret app -o yaml", true},
		{"kubectl get --mystery synthetic secret -o yaml", true},
		{"kubectl --mystery get secret app -o yaml", true},
		{"kubectl get --mystery synthetic pods -o name", false},
		{"kubectl --context get get secret app -o yaml", true},
		{"kubectl --context=get get secret app -o yaml", true},
		{"kubectl --context get get pods -o yaml", false},
		{"kubectl get secrets.v1 -o yaml", true},
		{"kubectl get secret.v1/default-token -o json", true},
		{"kubectl get secrets.v1.core/default-token -o go-template={{.data}}", true},
		{"kubectl get configmaps.v1 -o yaml", false},
		{"kubectl get configmap.v1/namedsecret -o json", false},
		{"kubectl get --raw /api/v1/namespaces/default/secrets/default-token", true},
		{"kubectl get --raw=/api/v1/namespaces/default/secrets", true},
		{"kubectl get --raw /version", false},
		{"kubectl get --raw /api/v1", false},
		{"kubectl get --raw /apis/apps/v1", false},
		{"kubectl get --raw /version --raw /api/v1/namespaces/default/secrets/default-token", true},
		{"kubectl get --raw /version --raw=$ENDPOINT", true},
		{"kubectl get --raw /version --raw /api/v1", false},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			parsed := shellwords.Parse(tc.command, shellwords.POSIX)
			if len(parsed.Invocations) != 1 {
				t.Fatalf("invocations = %d, want 1", len(parsed.Invocations))
			}
			_, denied := commandSecretReason(parsed.Invocations[0])
			if denied != tc.deny {
				t.Fatalf("denied = %v, want %v", denied, tc.deny)
			}
		})
	}
}

func TestIncompleteHelpDoesNotSuppressCommandGuard(t *testing.T) {
	parsed := shellwords.Parse("xargs -- kubectl get secret --help", shellwords.POSIX)
	if len(parsed.Invocations) != 1 {
		t.Fatalf("invocations = %d, want 1", len(parsed.Invocations))
	}
	if parsed.Invocations[0].Complete {
		t.Fatal("xargs invocation unexpectedly complete")
	}
	if reason, denied := commandSecretReason(parsed.Invocations[0]); !denied || strings.Contains(reason, "malformed") {
		t.Fatalf("reason=%q denied=%v, want actionable incomplete-command denial", reason, denied)
	}
}
