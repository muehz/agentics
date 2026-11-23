package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"testing/iotest"
)

// A payload that cannot be read is an error outcome rendered by the calling
// surface, never a usage exit and never a deny.
func TestAStdinReadFailureIsRenderedThroughTheProvider(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"claude-code", "command-guardian", "--mode", "block"},
		iotest.ErrReader(errors.New("pipe closed")), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "unable to read the hook payload: pipe closed") {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
}
