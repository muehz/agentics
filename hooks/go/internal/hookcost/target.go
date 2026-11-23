package hookcost

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// Target is the hooks directory a measurement launches from, with the
// committed binary and the dispatcher in it.
type Target struct {
	HooksDir   string
	Binary     string
	Dispatcher string
}

// ErrOffContract is wrapped by ResolveTarget's error when no hook binary is
// committed for the running platform, so a consumer that measures the packaged
// layout can skip rather than fail.
var ErrOffContract = errors.New("no hook binary is committed")

// ResolveTarget returns the hooks directory to measure: AGX_BENCH_HOOKS_DIR
// when set, which must name an existing directory by an absolute path and is
// cleaned, else the checkout's hooks directory. It does not check that the
// binary or the dispatcher exist: each launch checks the binary itself.
func ResolveTarget(checkout string, getenv func(string) string) (Target, error) {
	return resolveTarget(checkout, getenv, hosttest.CommittedBinaryPath)
}

func resolveTarget(checkout string, getenv func(string) string, binaryPath func(string) (string, hosttest.BinaryState)) (Target, error) {
	root, err := filepath.Abs(checkout)
	if err != nil {
		return Target{}, err
	}
	hooksDir := filepath.Join(root, "hooks")
	if override := getenv("AGX_BENCH_HOOKS_DIR"); override != "" {
		if !filepath.IsAbs(override) {
			return Target{}, errors.New("AGX_BENCH_HOOKS_DIR must be an absolute directory")
		}
		info, err := os.Stat(override)
		if err != nil || !info.IsDir() {
			return Target{}, fmt.Errorf("AGX_BENCH_HOOKS_DIR %q is not a directory: %v", override, err)
		}
		hooksDir = filepath.Clean(override)
	}
	binary, state := binaryPath(hooksDir)
	if state == hosttest.OffContract {
		return Target{}, fmt.Errorf("%w for %s/%s", ErrOffContract, runtime.GOOS, runtime.GOARCH)
	}
	return Target{HooksDir: hooksDir, Binary: binary, Dispatcher: filepath.Join(hooksDir, "run-hook.cmd")}, nil
}

// Prepare isolates the measured hooks from the developer's environment for
// the rest of the test and returns a clean Git fixture and the environment to
// launch them with. It scrubs every guard variable, points Git at an empty
// global config, and roots the plugin variables at target's plugin
// directory; tb.Setenv restores all of it at cleanup, so a test that calls
// it cannot run in parallel.
func Prepare(tb testing.TB, target Target) (fixture string, env []string) {
	tb.Helper()
	hosttest.ScrubGuardEnv(tb)
	scratch := tb.TempDir()
	gitConfig := filepath.Join(scratch, "gitconfig")
	if err := os.WriteFile(gitConfig, nil, 0600); err != nil {
		tb.Fatal(err)
	}
	pluginRoot := filepath.Dir(target.HooksDir)
	for key, value := range map[string]string{
		"GIT_CONFIG_GLOBAL": gitConfig, "GIT_CONFIG_NOSYSTEM": "1", "LC_ALL": "C", "LANG": "C",
		"AGX_NO_FETCH":          "1",
		"COMMAND_GUARD_LOG_DIR": filepath.Join(scratch, "logs"), "RELEASE_GUARD_LOG_DIR": filepath.Join(scratch, "logs"),
		"CLAUDE_PLUGIN_ROOT": pluginRoot, "PLUGIN_ROOT": pluginRoot, "COPILOT_PLUGIN_ROOT": pluginRoot,
		"AGX_DEBUG": "",
	} {
		tb.Setenv(key, value)
	}
	env = os.Environ()
	return CleanRepo(tb, filepath.Join(scratch, "repo"), env), env
}

// CleanRepo creates dir as a Git repository under env with one committed
// file, fixture.txt, and a clean working tree, and returns dir.
func CleanRepo(tb testing.TB, dir string, env []string) string {
	tb.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatal(err)
	}
	runGit := func(args ...string) {
		tb.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			tb.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "packaged-hook-bench@example.invalid")
	runGit("config", "user.name", "Packaged Hook Benchmark")
	if err := os.WriteFile(filepath.Join(dir, "fixture.txt"), []byte("safe baseline\n"), 0o600); err != nil {
		tb.Fatal(err)
	}
	runGit("add", "--", "fixture.txt")
	runGit("commit", "-qm", "baseline")
	return dir
}
