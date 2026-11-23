package hookspectest

import (
	"context"
	"errors"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func TestScriptsReadsHeldPath(t *testing.T) {
	scripts := Scripts{"/work/run.sh": "echo hi\n"}
	got := scripts.ReadScript("/work/run.sh")
	want := shellwords.ScriptRead{Path: "/work/run.sh", Text: "echo hi\n", Bytes: len("echo hi\n")}
	if got != want {
		t.Errorf("ReadScript(held) = %+v, want %+v", got, want)
	}
}

func TestScriptsRefusesUnheldPath(t *testing.T) {
	scripts := Scripts{"/work/run.sh": "echo hi\n"}
	got := scripts.ReadScript("/work/other.sh")
	want := shellwords.ScriptRead{Path: "/work/other.sh", Limit: shellwords.RuleUnreadable}
	if got != want {
		t.Errorf("ReadScript(unheld) = %+v, want %+v", got, want)
	}
}

func TestScriptsNilReadsNothing(t *testing.T) {
	var scripts Scripts
	if got := scripts.ReadScript("/x").Limit; got != shellwords.RuleUnreadable {
		t.Errorf("nil Scripts Limit = %q, want script-unreadable", got)
	}
}

func TestCancelContextCancelsFromAt(t *testing.T) {
	c := &CancelContext{Context: context.Background(), At: 3}
	for call := 1; call <= 2; call++ {
		if err := c.Err(); err != nil {
			t.Fatalf("call %d: Err() = %v, want nil", call, err)
		}
	}
	if err := c.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("call 3: Err() = %v, want context.Canceled", err)
	}
	if c.Calls != 3 {
		t.Errorf("Calls = %d, want 3", c.Calls)
	}
	if err := c.Err(); !errors.Is(err, context.Canceled) {
		t.Errorf("call 4: Err() = %v, want context.Canceled", err)
	}
	if c.Calls != 4 {
		t.Errorf("Calls = %d, want 4", c.Calls)
	}
}

func TestCancelContextZeroAtNeverCancels(t *testing.T) {
	c := &CancelContext{Context: context.Background()}
	for call := 1; call <= 10; call++ {
		if err := c.Err(); err != nil {
			t.Fatalf("call %d: Err() = %v, want nil", call, err)
		}
	}
	if c.Calls != 10 {
		t.Errorf("Calls = %d, want 10", c.Calls)
	}
}
