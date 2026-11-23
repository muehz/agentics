package hookcost

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestExecTimeout(t *testing.T) {
	spec := Command{TimeoutSeconds: 1}
	if runtime.GOOS == "windows" {
		spec.Executable = os.Getenv("ComSpec")
		spec.Args = []string{"/d", "/s", "/c", `powershell.exe -NoProfile -NonInteractive -Command "Start-Sleep -Seconds 30"`}
	} else {
		spec.Executable = "sh"
		spec.Args = []string{"-c", "sleep 30"}
	}
	start := time.Now()
	_, _, _, err := Exec(spec, t.TempDir(), os.Environ())
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 8*time.Second {
		t.Fatalf("shell-child timeout unbounded or misclassified: elapsed=%s err=%v", time.Since(start), err)
	}
}
