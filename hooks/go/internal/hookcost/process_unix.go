//go:build !windows

package hookcost

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

// Process builds the command for one launch. It starts the child in its own
// process group and, on cancel, kills the whole group so a shell's children
// cannot outlive the timeout.
func Process(ctx context.Context, executable string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return cmd
}
