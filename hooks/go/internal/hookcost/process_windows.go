package hookcost

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Process builds the command for one launch. A cmd.exe command string is
// shell syntax rather than a CommandLineToArgvW argument, so it is passed
// through verbatim, and on cancel the whole process tree is killed so a
// shell's children cannot outlive the timeout.
func Process(ctx context.Context, executable string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, executable, args...)
	if strings.EqualFold(filepath.Base(executable), "cmd.exe") && len(args) == 4 && args[2] == "/c" {
		// cmd's command string is shell syntax, not a CommandLineToArgvW argument.
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: syscall.EscapeArg(executable) + " /d /s /c \"" + args[3] + "\""}
	}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		killer := filepath.Join(os.Getenv("SystemRoot"), "System32", "taskkill.exe")
		err := exec.CommandContext(cleanup, killer, "/pid", strconv.Itoa(cmd.Process.Pid), "/t", "/f").Run()
		if err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	return cmd
}
