package shellwords

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
)

// oracleShell is a host interpreter that has been shown to run a file from a temporary directory. Every run it makes
// gets an environment built from nothing, so the host's startup variables (BASH_ENV, ENV, CDPATH, SHELLOPTS, BASHOPTS,
// OLDPWD and the rest) cannot change what the interpreter does.
type oracleShell struct {
	path         string
	name         string
	major, minor int
	home         string
	pathEnv      string // the PATH every run sees: the host's, or the stub directory alone
	stubDir      string // empty when the shell has no recorder stubs
	stubLog      string
	stubTmp      string
}

type oracleConfig struct {
	interpreter string
	bash44      bool
	stubs       []string
}

type oracleOption func(*oracleConfig)

// oracleInterpreter selects the interpreter: "bash" (the default) or "sh".
func oracleInterpreter(name string) oracleOption {
	return func(c *oracleConfig) { c.interpreter = name }
}

// oracleBash44 skips the test when the bash is older than 4.4, the oldest the parser models.
func oracleBash44() oracleOption {
	return func(c *oracleConfig) { c.bash44 = true }
}

// oracleStubs makes PATH hold only recorder stubs for the given command names, so nothing real can run and the
// commands a script reaches are recorded for StubLog.
func oracleStubs(names ...string) oracleOption {
	return func(c *oracleConfig) { c.stubs = append(c.stubs, names...) }
}

// oracleRun is one interpreter run. Exactly one of Command, File and Stdin is set.
type oracleRun struct {
	Dir     string        // working directory; a fresh temporary directory when empty
	Command string        // run with -c
	File    string        // or: the path of a script file to run
	Stdin   string        // or: text fed to the interpreter with -s
	Env     []string      // NAME=value pairs added to the fixed environment
	Timeout time.Duration // 10 s when zero
}

const (
	oracleDefaultTimeout = 10 * time.Second
	oracleWaitDelay      = time.Second
)

type oracleSkipReason int

const (
	oracleSkipCannotRunFile oracleSkipReason = iota
	oracleSkipOldBash
)

// oracleSkipMessage builds the two fixed skip reasons. name is the interpreter; major and minor are read only for
// an old bash.
func oracleSkipMessage(reason oracleSkipReason, name string, major, minor int) string {
	switch reason {
	case oracleSkipCannotRunFile:
		return fmt.Sprintf("%s could not run a script file from a temporary directory, so there is no interpreter to compare with", name)
	default:
		return oldBashSkipMessage(major, minor)
	}
}

// oracleFixedNames are the environment names a run owns; a caller addition cannot replace them.
var oracleFixedNames = []string{"PATH", "HOME", "SYSTEMROOT"}

// oracleRejectedEnv returns the first addition that names a fixed variable. Names compare without regard to case
// because Windows treats them that way.
func oracleRejectedEnv(additions []string) (string, bool) {
	for _, kv := range additions {
		name, _, _ := strings.Cut(kv, "=")
		for _, fixed := range oracleFixedNames {
			if strings.EqualFold(name, fixed) {
				return name, true
			}
		}
	}
	return "", false
}

// oracleMode reports how a run feeds its text, or why the run is malformed.
func (r oracleRun) mode() (string, error) {
	var set []string
	if r.Command != "" {
		set = append(set, "Command")
	}
	if r.File != "" {
		set = append(set, "File")
	}
	if r.Stdin != "" {
		set = append(set, "Stdin")
	}
	if len(set) != 1 {
		return "", fmt.Errorf("an oracle run needs exactly one of Command, File and Stdin, got %d (%s)", len(set), strings.Join(set, ", "))
	}
	return set[0], nil
}

// oracleVersions caches the version each interpreter path reported, so a test binary reads it once.
var (
	oracleVersionMu    sync.Mutex
	oracleVersionCache = map[string][2]int{}
)

// requireOracle finds the interpreter, builds its environment, probes it and applies the version gate. It skips the
// test with one of two fixed messages: cannot run a file from a temporary directory, older than 4.4. A missing or broken
// interpreter is hookutiltest.RequireInterpreter's skip, or its failure when AGX_REQUIRED_INTERPRETERS lists it.
func requireOracle(t *testing.T, opts ...oracleOption) *oracleShell {
	t.Helper()
	cfg := oracleConfig{interpreter: "bash"}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.interpreter != "bash" && cfg.interpreter != "sh" {
		t.Fatalf("oracleInterpreter must be bash or sh, got %q", cfg.interpreter)
	}
	if cfg.bash44 && cfg.interpreter != "bash" {
		t.Fatalf("oracleBash44 applies to bash only, not %s", cfg.interpreter)
	}
	path := hookutiltest.RequireInterpreter(t, cfg.interpreter)
	o := &oracleShell{path: path, name: cfg.interpreter, home: t.TempDir(), pathEnv: os.Getenv("PATH")}
	if len(cfg.stubs) > 0 {
		o.makeStubs(t, cfg.stubs)
	}
	if !o.canRunFile(t, cfg.stubs) {
		t.Skip(oracleSkipMessage(oracleSkipCannotRunFile, o.name, 0, 0))
	}
	o.major, o.minor = o.readVersion(t)
	if cfg.bash44 && bashOlderThan44(o.major, o.minor) {
		t.Skip(oracleSkipMessage(oracleSkipOldBash, o.name, o.major, o.minor))
	}
	return o
}

// makeStubs writes one recorder script per name into a directory that becomes the whole PATH.
func (o *oracleShell) makeStubs(t *testing.T, names []string) {
	t.Helper()
	root := t.TempDir()
	o.stubDir = filepath.Join(root, "bin")
	if err := os.Mkdir(o.stubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	o.stubLog = filepath.Join(root, "log")
	o.stubTmp = filepath.Join(root, "tmp")
	for _, name := range names {
		stub := "#!/bin/sh\nprintf '%s\\n' \"" + name + " $*\" >> \"$ORACLE_LOG\"\n"
		if name == "mktemp" {
			stub += "printf '%s\\n' \"$ORACLE_TMP\"\n"
		}
		if err := os.WriteFile(filepath.Join(o.stubDir, name), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	o.pathEnv = o.stubDir
}

// canRunFile writes a script to a temporary directory and runs it. A bash that does not pass the environment through
// (WSL's System32\bash.exe, for one) or cannot read a host path prints nothing useful, so it is unusable. With stubs the
// script also runs the first stub, which shows the stub directory is reachable; that run's log line is then discarded.
func (o *oracleShell) canRunFile(t *testing.T, stubs []string) bool {
	t.Helper()
	const probe = "agentics-oracle-probe"
	script := filepath.Join(t.TempDir(), "probe.sh")
	text := "printf '%s' \"$ORACLE_PROBE\"\n"
	if len(stubs) > 0 {
		text += stubs[0] + " probe\n"
	}
	if err := os.WriteFile(script, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, err := o.run(t, oracleRun{File: script, Env: []string{"ORACLE_PROBE=" + probe}}, "File")
	if err != nil || stdout != probe {
		return false
	}
	if len(stubs) > 0 {
		recorded := o.StubLog()
		_ = os.Remove(o.stubLog)
		return slices.Equal(recorded, []string{stubs[0]})
	}
	return true
}

// readVersion returns the bash version the interpreter reports, or 0.0 for a shell that is not bash. It uses only
// builtins, so a host profile or a stub-only PATH cannot change the answer, and it asks once per interpreter path.
func (o *oracleShell) readVersion(t *testing.T) (major, minor int) {
	t.Helper()
	oracleVersionMu.Lock()
	defer oracleVersionMu.Unlock()
	if v, ok := oracleVersionCache[o.path]; ok {
		return v[0], v[1]
	}
	stdout, _, err := o.run(t, oracleRun{Command: `printf '%s' "${BASH_VERSION:-}"`}, "Command")
	if err != nil {
		t.Fatalf("could not ask %s for its version: %v", o.path, err)
	}
	if stdout != "" {
		if _, err := fmt.Sscanf(stdout, "%d.%d", &major, &minor); err != nil {
			t.Fatalf("could not read a bash version from %q: %v", stdout, err)
		}
	}
	oracleVersionCache[o.path] = [2]int{major, minor}
	return major, minor
}

// Version returns the interpreter's bash major and minor version, or 0.0 when it is not bash.
func (o *oracleShell) Version() (major, minor int) { return o.major, o.minor }

// StubLog returns the command names the stubs recorded, sorted and distinct.
func (o *oracleShell) StubLog() []string {
	if o.stubLog == "" {
		return nil
	}
	data, _ := os.ReadFile(o.stubLog)
	var ran []string
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) > 0 && !slices.Contains(ran, f[0]) {
			ran = append(ran, f[0])
		}
	}
	slices.Sort(ran)
	return ran
}

// env builds a run's environment from nothing: PATH, a temporary HOME, SYSTEMROOT when the host sets it, the
// variables the stubs read, and the caller's additions.
func (o *oracleShell) env(additions []string) []string {
	env := []string{"PATH=" + o.pathEnv, "HOME=" + o.home}
	if root, ok := os.LookupEnv("SYSTEMROOT"); ok {
		env = append(env, "SYSTEMROOT="+root)
	}
	if o.stubDir != "" {
		env = append(env, "ORACLE_LOG="+filepath.ToSlash(o.stubLog), "ORACLE_TMP="+filepath.ToSlash(o.stubTmp))
	}
	return append(env, additions...)
}

// Run runs the interpreter once and returns what it wrote. A malformed run, or one that tries to replace PATH, HOME
// or SYSTEMROOT, fails the test. A run that used a stub-only PATH and reports a missing command fails the test too,
// because a fixture naming an unstubbed command would otherwise look like one that ran nothing.
func (o *oracleShell) Run(t *testing.T, r oracleRun) (stdout, stderr string, err error) {
	t.Helper()
	mode, modeErr := r.mode()
	if modeErr != nil {
		t.Fatal(modeErr)
	}
	if name, bad := oracleRejectedEnv(r.Env); bad {
		t.Fatalf("an oracle run cannot set %s: the oracle fixes PATH, HOME and SYSTEMROOT", name)
	}
	stdout, stderr, err = o.run(t, r, mode)
	if o.stubDir != "" && strings.Contains(stderr, "not found") {
		t.Errorf("%s could not find a command, so a stub is missing", o.name)
	}
	return stdout, stderr, err
}

func (o *oracleShell) run(t *testing.T, r oracleRun, mode string) (stdout, stderr string, err error) {
	t.Helper()
	timeout := r.Timeout
	if timeout == 0 {
		timeout = oracleDefaultTimeout
	}
	var args []string
	if o.name == "bash" {
		args = []string{"--norc", "--noprofile"}
	}
	switch mode {
	case "Command":
		args = append(args, "-c", r.Command)
	case "File":
		args = append(args, r.File)
	case "Stdin":
		args = append(args, "-s")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, o.path, args...)
	cmd.Dir = r.Dir
	if cmd.Dir == "" {
		cmd.Dir = t.TempDir()
	}
	cmd.Env = o.env(r.Env)
	cmd.WaitDelay = oracleWaitDelay
	if mode == "Stdin" {
		cmd.Stdin = strings.NewReader(r.Stdin)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err = cmd.Run()
	return out.String(), errOut.String(), err
}

func TestOracleEnvironmentIsFixed(t *testing.T) {
	const sentinel = "AGX_ORACLE_SENTINEL_LEAK"
	leaked := []string{sentinel, "BASH_ENV", "CDPATH", "OLDPWD"}
	for _, name := range leaked {
		t.Setenv(name, "leaked-from-test-process")
	}
	o := requireOracle(t)
	stdout, stderr, err := o.Run(t, oracleRun{Command: "env", Env: []string{"AGX_ORACLE_ADDED=yes"}})
	if err != nil {
		t.Fatalf("env failed: %v\n%s", err, stderr)
	}
	names := map[string]bool{}
	for _, line := range strings.Split(stdout, "\n") {
		name, _, _ := strings.Cut(strings.TrimRight(line, "\r"), "=")
		names[name] = true
	}
	if !names["AGX_ORACLE_ADDED"] {
		t.Errorf("the caller's addition did not reach the interpreter:\n%s", stdout)
	}
	for _, name := range leaked {
		if names[name] {
			t.Errorf("%s reached the interpreter from the test process:\n%s", name, stdout)
		}
	}
}

func TestOracleRunTimesOut(t *testing.T) {
	o := requireOracle(t)
	script := filepath.Join(t.TempDir(), "sleep.sh")
	if err := os.WriteFile(script, []byte("exec sleep 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, _, err := o.Run(t, oracleRun{File: script, Timeout: 500 * time.Millisecond})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("a run that sleeps past its timeout returned no error")
	}
	if elapsed > 3*time.Second {
		t.Errorf("the run took %v, want it to end within 3s: the timeout plus the one-second WaitDelay", elapsed)
	}
}

func TestOracleStubLogIsSortedAndDistinct(t *testing.T) {
	o := requireOracle(t, oracleStubs("rm", "git", "mktemp"))
	if got := o.StubLog(); len(got) != 0 {
		t.Fatalf("a fresh oracle has recorded %v, want nothing", got)
	}
	_, stderr, err := o.Run(t, oracleRun{Command: "rm a; git status; rm b; git log"})
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, stderr)
	}
	if got, want := o.StubLog(), []string{"git", "rm"}; !slices.Equal(got, want) {
		t.Errorf("StubLog() = %v, want %v", got, want)
	}
}

func TestOracleSkipMessagesAreFixed(t *testing.T) {
	for _, c := range []struct {
		reason       oracleSkipReason
		major, minor int
		want         string
	}{
		{oracleSkipCannotRunFile, 0, 0, "bash could not run a script file from a temporary directory, so there is no interpreter to compare with"},
		{oracleSkipOldBash, 3, 2, "bash 3.2 is older than 4.4: the parser models bash 4.4 and later, and these constructs read differently on older bash"},
	} {
		if got := oracleSkipMessage(c.reason, "bash", c.major, c.minor); got != c.want {
			t.Errorf("oracleSkipMessage(%d) = %q, want %q", c.reason, got, c.want)
		}
	}
}

func TestOracleRunRejectsMalformedInput(t *testing.T) {
	for _, c := range []struct {
		name string
		run  oracleRun
		ok   bool
	}{
		{"command", oracleRun{Command: "true"}, true},
		{"file", oracleRun{File: "x.sh"}, true},
		{"stdin", oracleRun{Stdin: "true"}, true},
		{"none", oracleRun{}, false},
		{"command and file", oracleRun{Command: "true", File: "x.sh"}, false},
		{"all three", oracleRun{Command: "true", File: "x.sh", Stdin: "true"}, false},
	} {
		if _, err := c.run.mode(); (err == nil) != c.ok {
			t.Errorf("%s: mode() error = %v, want ok=%v", c.name, err, c.ok)
		}
	}
	for _, name := range []string{"PATH", "HOME", "SYSTEMROOT", "path", "Home"} {
		if got, bad := oracleRejectedEnv([]string{"KEEP=1", name + "=x"}); !bad || got != name {
			t.Errorf("oracleRejectedEnv with %s = (%q, %v), want it rejected", name, got, bad)
		}
	}
	if _, bad := oracleRejectedEnv([]string{"ORACLE_LOG=x", "PATHEXT=y"}); bad {
		t.Error("oracleRejectedEnv rejected a name that only starts with a fixed one")
	}
}
