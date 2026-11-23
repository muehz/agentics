package releaseguard

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/toolcall"
)

func inspectScript(t *testing.T, command, body string) inspection {
	t.Helper()
	event, err := buildEventAt(t, "/ws", "Bash", map[string]any{"command": command}).Read()
	if err != nil {
		t.Fatal(err)
	}
	call, err := toolcall.Read(event)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	return inspectDeployExecution(ctx, call.Inspect(ctx, hookspectest.Scripts{"/ws/t.sh": body}), nil, false)
}

// A matcher error quotes the option it could not read; inside a script that option is script content, so the reason
// names the script line instead. A typed call keeps the matcher's own text, and a script refusal gains its remedy.
func TestInspectionReasonNamesTheScriptLineForAMatcherError(t *testing.T) {
	const marker = "leakmarkerq7"
	script := inspectScript(t, "bash /ws/t.sh", "echo a\nhelmfile --"+marker+" apply\n")
	if !script.failed() || script.origin == nil || script.reason() != "deploy option could not be read in /ws/t.sh:2" {
		t.Fatalf("script inspection %+v reason %q", script, script.reason())
	}
	typed := inspectScript(t, "helmfile --"+marker+" apply", "")
	if !typed.failed() || typed.origin != nil || typed.reason() != typed.err.Error() || !strings.Contains(typed.reason(), marker) {
		t.Fatalf("typed inspection %+v", typed)
	}
	refused := inspectScript(t, "bash /ws/missing.sh", "")
	want := "release-guardian could not inspect a recognized execution path: The script must be a plain UTF-8 file that stays unchanged while it is inspected; check the path and the file. (script-unreadable); " + recordRemedy
	if refused.refusal.Rule() != shellwords.RuleUnreadable || inspectionMessage(refused) != want {
		t.Fatalf("refusal %q: %q", refused.reason(), inspectionMessage(refused))
	}
	if got := inspectionMessage(inspection{err: errors.New("unsupported-syntax")}); got != "release-guardian could not inspect a recognized execution path (unsupported-syntax); "+recordRemedy {
		t.Fatalf("typed message changed: %q", got)
	}
}

// A canceled script child is no named rule, so its reason is the script line and its message carries no remedy.
func TestInspectionReasonNamesTheScriptLineForACanceledChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	parsed := shellwords.ParseScripts(ctx, "kubectl apply -f x", shellwords.POSIX, shellwords.ScriptOptions{})
	if len(parsed.Invocations) == 0 {
		t.Fatal("a canceled parse yielded no invocation")
	}
	canceled := inspection{refusal: parsed.Invocations[0].Reason, uninspectable: true, origin: &shellwords.ScriptOrigin{Path: "/ws/t.sh", Line: 2}}
	if !canceled.refusal.Canceled() {
		t.Fatalf("refusal %q is not a cancellation", canceled.refusal)
	}
	if !canceled.failed() || canceled.reason() != "deploy option could not be read in /ws/t.sh:2" {
		t.Fatalf("canceled inspection reason %q", canceled.reason())
	}
	if got := inspectionMessage(canceled); got != "release-guardian could not inspect a recognized execution path (deploy option could not be read in /ws/t.sh:2); "+recordRemedy {
		t.Fatalf("canceled message carries a hint: %q", got)
	}
}

// A non-inspectable invocation with no reason still fails closed, with an empty reason.
func TestUninspectableWithoutReasonFails(t *testing.T) {
	bare := inspection{uninspectable: true}
	if !bare.failed() || bare.reason() != "" {
		t.Fatalf("failed %v reason %q", bare.failed(), bare.reason())
	}
}
