package releaseguard

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func TestSubstitutionPreservesDeployContext(t *testing.T) {
	for _, command := range []string{
		`kubectl --context $(kubectl config current-context) apply -f prod.yaml`,
		`terraform -chdir=$(pwd)/infra apply`,
		`kubectl $(printf apply) -f prod.yaml`,
		"terraform -chdir=`pwd`/infra apply",
		`terraform -chdir=$(echo "$(pwd)")/infra apply`,
	} {
		t.Run(command, func(t *testing.T) {
			kind, message := bashCommand(t, command)
			if kind != "deny" || !strings.Contains(message, "literal") {
				t.Fatalf("got %s %q; want non-literal parsing-limit denial; parsed: %#v", kind, message, shellwords.Parse(command, shellwords.POSIX))
			}
		})
	}
}

func TestSubstitutionAndANSICControls(t *testing.T) {
	assertEach(t, []string{
		`echo $(date)`,
		`printf '%s' 'k$()ubectl apply'`,
		`printf '%s' '$\x27kubectl\x27 apply'`,
		`echo $'it\'s; $(kubectl apply)'`,
		`$'kubectl' get pods`,
		`echo "\$(kubectl apply)"`,
		`echo $(printf '%s' $'it\'s)')`,
	}, "pass")
	assertEach(t, []string{
		`echo $(kubectl apply)`,
		`echo $(echo $(kubectl apply))`,
		`echo $'it\'s <<EOF'; kubectl apply`,
		`echo $(printf '%s' $'it\'s)'); kubectl apply`,
	}, "deny")
	assertEachTool(t, "PowerShell", []string{`terraform -chdir=$(Get-Location)/infra apply`}, "deny")
}

func TestSubstitutedExecutableIsParsingLimit(t *testing.T) {
	for _, command := range []string{`k$()ubectl apply`, `$(printf kubectl) apply`, "k``ubectl apply"} {
		t.Run(command, func(t *testing.T) {
			kind, message := bashCommand(t, command)
			if kind != "deny" || !strings.Contains(message, "command-substituted executable is not literal") {
				t.Fatalf("got %s %q; want command-substituted executable denial", kind, message)
			}
		})
	}
}

func TestANSICQuotedDeploy(t *testing.T) {
	for _, command := range []string{`$'kubectl' apply`, `k$'ube'ctl apply`, `$'\x6bubectl' apply`, `$'\153ubectl' apply`, `$'\u006bubectl' apply`, `kubectl $'apply'`} {
		t.Run(command, func(t *testing.T) {
			kind, message := bashCommand(t, command)
			if kind != "deny" || !strings.Contains(message, "kubectl apply is a production-shaped deploy command (kubernetes_deploy)") {
				t.Fatalf("got %s %q; want kubectl apply denial", kind, message)
			}
		})
	}
}

func TestANSICParsingLimits(t *testing.T) {
	for _, tc := range []struct{ command, reason string }{
		{`$'kubectl\0ignored' apply`, "ANSI-C non-ASCII or NUL escape"},
		{`$'\u212aUBECTL' apply`, "ANSI-C non-ASCII or NUL escape"},
		{`$'\cK' apply`, "ANSI-C control escape"},
		{`$'kubectl\'`, "unterminated ANSI-C quoted word"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			kind, message := bashCommand(t, tc.command)
			if kind != "deny" || !strings.Contains(message, tc.reason) {
				t.Fatalf("got %s %q; want %q denial", kind, message, tc.reason)
			}
		})
	}
}
