package hookspec

import (
	"context"
	"testing"
)

func TestExecutionContext(t *testing.T) {
	var legacy Request
	if legacy.Context != nil || legacy.ExecutionContext() == nil || legacy.ExecutionContext().Err() != nil {
		t.Fatal("nil context did not preserve background behavior")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := Request{Context: ctx}
	if req.ExecutionContext() != ctx {
		t.Fatal("request context identity changed")
	}
	cancel()
	if req.ExecutionContext().Err() != context.Canceled {
		t.Fatal("request cancellation did not propagate")
	}
}
