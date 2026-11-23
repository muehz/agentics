// Package hookcost is test support imported only by tests: it measures what a
// hook costs by crossing probes with routes. A probe is a set of registered
// commands, a route is how one launch reaches the hook, and a measurement is
// the samples that crossing returns. It is separate from the tests that drive
// it so the records and statistics have one owner, and it is listed in
// digest-exclude because no hook binary imports it.
package hookcost

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

// CheckBinary reports whether path is the regular, executable file a capture
// must measure. A missing binary would make a fail-open launch look like a
// cheap one, so it is an error rather than a skipped probe.
func CheckBinary(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("measured binary unavailable (fail-open output is not evidence): %w", err)
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0) {
		return fmt.Errorf("measured binary not executable regular file: %s", path)
	}
	return nil
}

// Exec runs c once with env and returns the elapsed nanoseconds, stdout and
// stderr. A launch that outlives its timeout reports the context error even
// when the process also exited with a status.
func Exec(c Command, fixture string, env []string) (int64, string, string, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()
	cmd := Process(ctx, c.Executable, c.Args)
	cmd.Dir = c.workingDir(fixture)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(c.Payload)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	ns := time.Since(start).Nanoseconds()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return ns, stdout.String(), stderr.String(), err
}
