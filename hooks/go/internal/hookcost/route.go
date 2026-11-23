package hookcost

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// Probe is one hook call: the argv the binary receives, its stdin, and how
// long a launch of it may run.
type Probe struct {
	Name    string
	Argv    []string
	Stdin   string
	Timeout time.Duration
}

// Result is what one hook call returned.
type Result struct {
	Code           int
	Stdout, Stderr string
}

// Route is one way a call reaches the hook. Prepare records the reference
// result for a probe and verifies that the route measures the intended hook;
// Invoke runs the probe once, timed, and fails when its result differs from
// the reference of the last Prepare.
type Route interface {
	Name() string
	Prepare(Probe) error
	Invoke(Probe) (time.Duration, error)
}

// RunFunc is the hook's in-process entry point.
type RunFunc func(argv []string, stdin io.Reader, stdout, stderr io.Writer) int

func call(run RunFunc, p Probe) Result {
	var stdout, stderr bytes.Buffer
	code := run(p.Argv, strings.NewReader(p.Stdin), &stdout, &stderr)
	return Result{Code: code, Stdout: stdout.String(), Stderr: stderr.String()}
}

type inProcess struct {
	run  RunFunc
	want *Result
}

// InProcess returns the route that calls run directly. Its reference is its
// own first result, whatever the exit code, because a latency probe may be
// one the hook denies.
func InProcess(run RunFunc) Route { return &inProcess{run: run} }

func (r *inProcess) Name() string { return "in-process" }

func (r *inProcess) Prepare(p Probe) error {
	want := call(r.run, p)
	r.want = &want
	return nil
}

func (r *inProcess) Invoke(p Probe) (time.Duration, error) {
	start := time.Now()
	got := call(r.run, p)
	elapsed := time.Since(start)
	if r.want == nil {
		r.want = &got
		return elapsed, nil
	}
	if got != *r.want {
		return elapsed, fmt.Errorf("output mismatch exit=%d stdout=%q stderr=%q", got.Code, got.Stdout, got.Stderr)
	}
	return elapsed, nil
}

// LaunchRoute is a route that starts the hook as a process. Its reference
// is the in-process result for the probe, which must exit zero, and every
// launch first checks that the measured binary exists, so a fail-open launch
// cannot pass as a cheap one.
type LaunchRoute struct {
	name      string
	target    Target
	fixture   string
	env       []string
	reference RunFunc
	command   func(Probe) Command
	// resolve: the launch reaches the binary through a wrapper, so Prepare
	// proves which binary the wrapper selected.
	resolve bool
	// setupErr is a failure building the launch, reported by every call.
	setupErr error
	want     *Result
	// kept is the result of a call made before any Prepare, judged by the
	// next Prepare so a cold sample is still checked.
	kept *Result
}

// Native returns the route that starts target.Binary with the probe's argv
// in fixture, under env.
func Native(target Target, fixture string, env []string, reference RunFunc) *LaunchRoute {
	return &LaunchRoute{name: "native", target: target, fixture: fixture, env: env, reference: reference, command: func(p Probe) Command {
		return Command{Executable: target.Binary, Args: p.Argv, Payload: p.Stdin, TimeoutSeconds: timeoutSeconds(p)}
	}}
}

// Dispatcher returns the route that starts the shipped run-hook.cmd from
// target.HooksDir, under cmd.exe on Windows and bash elsewhere, as the hosts
// do. Its Prepare reruns the probe with AGX_DEBUG=1 and requires the
// dispatcher to name target.Binary.
func Dispatcher(target Target, fixture string, env []string, reference RunFunc) *LaunchRoute {
	shell, prefix := "bash", []string{"run-hook.cmd"}
	if runtime.GOOS == "windows" {
		// Absolute: NoDefaultCurrentDirectoryInExePath hosts refuse bare names.
		shell, prefix = "cmd.exe", []string{"/d", "/c", target.Dispatcher}
	}
	path, err := exec.LookPath(shell)
	if err != nil {
		err = fmt.Errorf("find %s: %w", shell, err)
	}
	return &LaunchRoute{name: "dispatcher", target: target, fixture: fixture, env: env, reference: reference, resolve: true, setupErr: err, command: func(p Probe) Command {
		return Command{Executable: path, Args: append(append([]string(nil), prefix...), p.Argv...), WorkingDir: target.HooksDir, Payload: p.Stdin, TimeoutSeconds: timeoutSeconds(p)}
	}}
}

// Configured returns the route that replays r's shipped command under the
// host shell, as Launch resolves it. Antigravity runs its commands from the
// plugin root, the parent of target.HooksDir. The error is Launch's.
func Configured(r hosttest.Registration, target Target, fixture string, env []string, reference RunFunc) (*LaunchRoute, error) {
	base, err := Launch(r)
	if err != nil {
		return nil, err
	}
	if r.Surface == surface.Antigravity {
		base.WorkingDir = filepath.Dir(target.HooksDir)
	}
	return &LaunchRoute{name: "config", target: target, fixture: fixture, env: env, reference: reference, resolve: true, command: func(p Probe) Command {
		c := base
		c.Args = append([]string(nil), base.Args...)
		c.Payload = p.Stdin
		c.TimeoutSeconds = timeoutSeconds(p)
		return c
	}}, nil
}

func timeoutSeconds(p Probe) int { return int(math.Ceil(p.Timeout.Seconds())) }

// Name is the route's layer word: native, dispatcher, or config.
func (l *LaunchRoute) Name() string { return l.name }

// Command is the launch for p, as a capture records it.
func (l *LaunchRoute) Command(p Probe) Command { return l.command(p) }

// Prepare records the reference for p, proves the resolved binary for a
// wrapped launch, and judges a result kept from an earlier unprepared call.
func (l *LaunchRoute) Prepare(p Probe) error {
	if err := l.check(p); err != nil {
		return err
	}
	want := call(l.reference, p)
	if want.Code != 0 {
		return fmt.Errorf("reference input rejected: exit=%d stderr=%q", want.Code, want.Stderr)
	}
	l.want = &want
	if l.resolve {
		if err := l.verifyResolution(p, want); err != nil {
			return fmt.Errorf("resolution verification: %w", err)
		}
	}
	if kept := l.kept; kept != nil {
		l.kept = nil
		if kept.Stdout != want.Stdout || kept.Stderr != want.Stderr {
			return fmt.Errorf("first-call output mismatch stdout=%q stderr=%q", kept.Stdout, kept.Stderr)
		}
	}
	return nil
}

// verifyResolution reruns p with AGX_DEBUG=1, which makes the dispatcher
// name the binary it selected on stderr. The run must otherwise answer as the
// reference does, so the debug switch cannot hide a different hook result.
func (l *LaunchRoute) verifyResolution(p Probe, want Result) error {
	base := l.env
	if base == nil {
		base = os.Environ()
	}
	c := l.command(p)
	_, got, err := l.exec(c, append(append([]string(nil), base...), "AGX_DEBUG=1"))
	if err != nil {
		return fmt.Errorf("%w stdout=%q stderr=%q", err, got.Stdout, got.Stderr)
	}
	// A non-zero exit is already an error above, and the reference exits zero.
	if got.Stdout != want.Stdout {
		return fmt.Errorf("debug run stdout=%q, want the reference stdout %q", got.Stdout, want.Stdout)
	}
	return CheckResolution(got.Stderr, c.workingDir(l.fixture), l.target.Binary)
}

// Invoke launches p once and compares the result with the reference. Before
// any Prepare it keeps the result for the next Prepare to judge.
func (l *LaunchRoute) Invoke(p Probe) (time.Duration, error) {
	if err := l.check(p); err != nil {
		return 0, err
	}
	return l.invoke(p)
}

// check holds the failures that come before a launch, whose text a capture
// records without the arm's id.
func (l *LaunchRoute) check(p Probe) error {
	if p.Timeout <= 0 {
		return fmt.Errorf("probe %q has no launch timeout", p.Name)
	}
	if l.setupErr != nil {
		return l.setupErr
	}
	return CheckBinary(l.target.Binary)
}

func (l *LaunchRoute) invoke(p Probe) (time.Duration, error) {
	elapsed, got, err := l.exec(l.command(p), l.env)
	if err != nil {
		return elapsed, fmt.Errorf("%w stdout=%q stderr=%q", err, got.Stdout, got.Stderr)
	}
	if l.want == nil {
		l.kept = &got
		return elapsed, nil
	}
	if got.Stdout != l.want.Stdout || got.Stderr != l.want.Stderr {
		return elapsed, fmt.Errorf("output mismatch stdout=%q stderr=%q", got.Stdout, got.Stderr)
	}
	return elapsed, nil
}

func (l *LaunchRoute) exec(c Command, env []string) (time.Duration, Result, error) {
	ns, stdout, stderr, err := Exec(c, l.fixture, env)
	got := Result{Stdout: stdout, Stderr: stderr}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		got.Code = exit.ExitCode()
	}
	return time.Duration(ns), got, err
}

// CheckResolution reports whether a dispatcher's AGX_DEBUG stderr names
// exactly one resolved binary and that it is binary. A relative path is read
// from cwd, and paths compare without case on Windows, as its file system
// does.
func CheckResolution(stderr, cwd, binary string) error {
	var paths []string
	for line := range strings.SplitSeq(stderr, "\n") {
		line = strings.TrimSuffix(line, "\r")
		for _, prefix := range []string{"run-hook resolved binary: ", "run-hook.cmd resolved binary: "} {
			if path, ok := strings.CutPrefix(line, prefix); ok {
				paths = append(paths, path)
			}
		}
	}
	if len(paths) != 1 || paths[0] == "" {
		return fmt.Errorf("dispatcher resolution requires exactly one binary path: %q", stderr)
	}
	resolved := paths[0]
	// Git Bash reports the install drive as /c/... even when the benchmark
	// runs as a native Windows process and expects C:\... .
	if drive := filepath.VolumeName(binary); runtime.GOOS == "windows" && len(drive) == 2 && drive[1] == ':' {
		prefix := "/" + drive[:1] + "/"
		if len(resolved) >= len(prefix) && strings.EqualFold(resolved[:len(prefix)], prefix) {
			resolved = drive + resolved[2:]
		}
	}
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(cwd, resolved)
	}
	resolved, err := filepath.Abs(resolved)
	if err != nil {
		return fmt.Errorf("dispatcher resolution path: %w", err)
	}
	want, err := filepath.Abs(binary)
	if err != nil {
		return fmt.Errorf("dispatcher expected path: %w", err)
	}
	match := resolved == want
	if runtime.GOOS == "windows" {
		match = strings.EqualFold(resolved, want)
	}
	if !match {
		return fmt.Errorf("dispatcher resolution identified %q, want %q", resolved, want)
	}
	return nil
}

// Samples prepares r for p, discards warmups calls, and returns the next n
// call times. It stops at the first error and returns only that error.
func Samples(r Route, p Probe, warmups, n int) ([]time.Duration, error) {
	if err := r.Prepare(p); err != nil {
		return nil, err
	}
	for k := 0; k < warmups; k++ {
		if _, err := r.Invoke(p); err != nil {
			return nil, err
		}
	}
	samples := make([]time.Duration, 0, n)
	for k := 0; k < n; k++ {
		d, err := r.Invoke(p)
		if err != nil {
			return nil, err
		}
		samples = append(samples, d)
	}
	return samples, nil
}
