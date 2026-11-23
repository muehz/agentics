package conformance

import (
	"os/exec"
	"testing"
	"time"
)

func runReentryProcess(t *testing.T, cmd *exec.Cmd, deadline time.Duration) ([]byte, error, bool) {
	t.Helper()
	t.Skip("process-tree deadline containment is not implemented on Windows")
	return nil, nil, false
}
