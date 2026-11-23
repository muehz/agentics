package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
)

func envFrom(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestBinaryNameSelection(t *testing.T) {
	cases := []struct {
		name string
		vars map[string]string
		want string
	}{
		{"identifier ARMv8", map[string]string{"PROCESSOR_ARCHITECTURE": "AMD64", "PROCESSOR_IDENTIFIER": "ARMv8 (64-bit) Family 8"}, "agx-windows-arm64.exe"},
		{"identifier lowercase armv8", map[string]string{"PROCESSOR_ARCHITECTURE": "AMD64", "PROCESSOR_IDENTIFIER": "armv8 (64-bit) Family 8"}, "agx-windows-arm64.exe"},
		{"architecture ARM64", map[string]string{"PROCESSOR_ARCHITECTURE": "ARM64"}, "agx-windows-arm64.exe"},
		{"architecture lowercase arm64", map[string]string{"PROCESSOR_ARCHITECTURE": "arm64"}, "agx-windows-arm64.exe"},
		{"WOW64 architecture ARM64", map[string]string{"PROCESSOR_ARCHITECTURE": "AMD64", "PROCESSOR_ARCHITEW6432": "ARM64"}, "agx-windows-arm64.exe"},
		{"AMD64", map[string]string{"PROCESSOR_ARCHITECTURE": "AMD64", "PROCESSOR_IDENTIFIER": "Intel64 Family 6 Model 154"}, "agx-windows-amd64.exe"},
		{"all unset", map[string]string{}, "agx-windows-amd64.exe"},
		{"short identifier", map[string]string{"PROCESSOR_IDENTIFIER": "ARM"}, "agx-windows-amd64.exe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := binaryName("windows", "amd64", envFrom(tc.vars))
			if err != nil {
				t.Fatalf("binaryName: unexpected error %v", err)
			}
			if got != tc.want {
				t.Fatalf("binaryName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBinaryNameNoPlatform(t *testing.T) {
	got, err := binaryName("linux", "amd64", envFrom(map[string]string{"PROCESSOR_ARCHITECTURE": "ARM64"}))
	if err == nil {
		t.Fatalf("binaryName = %q, want an error", got)
	}
	if err.Error() != "agx: no binary for this platform (linux-amd64)" {
		t.Fatalf("error = %q", err.Error())
	}
}

// layout returns a plugin root with the launcher's expected position inside
// it and the path the launcher resolves for an amd64 Windows host.
func layout(t *testing.T) (launcherPath, binaryPath string) {
	t.Helper()
	root := t.TempDir()
	return filepath.Join(root, "bin", "agx.exe"), filepath.Join(root, "hooks", "bin", "agx-windows-amd64.exe")
}

func runInProcess(t *testing.T, launcherPath string, vars map[string]string, goos string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(launch{
		args:       []string{"flow", "status"},
		getenv:     envFrom(vars),
		executable: func() (string, error) { return launcherPath, nil },
		goos:       goos,
		goarch:     "amd64",
		stdin:      strings.NewReader(""),
		stdout:     &stdout,
		stderr:     &stderr,
	})
	return code, stdout.String(), stderr.String()
}

func TestRunPrintsDigestWithoutStartingBinary(t *testing.T) {
	launcherPath, binaryPath := layout(t)
	code, stdout, stderr := runInProcess(t, launcherPath, map[string]string{"AGX_SOURCE_DIGEST": "1"}, "windows")
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr %q)", code, stderr)
	}
	want := sourceDigest + "\nbuilt-by " + runtime.Version() + "\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty: the binary at %s must not be started", stderr, binaryPath)
	}
}

func TestRunDigestNeedsExactlyOne(t *testing.T) {
	launcherPath, _ := layout(t)
	code, stdout, stderr := runInProcess(t, launcherPath, map[string]string{"AGX_SOURCE_DIGEST": "true"}, "windows")
	if code != 126 || stdout != "" || !strings.Contains(stderr, "cannot provision runtime") {
		t.Fatalf("exit %d stdout %q stderr %q: a value other than 1 must attempt provisioning", code, stdout, stderr)
	}
}

func TestRunNoPlatform(t *testing.T) {
	launcherPath, _ := layout(t)
	code, stdout, stderr := runInProcess(t, launcherPath, nil, "linux")
	if code != 126 {
		t.Fatalf("exit = %d, want 126", code)
	}
	if stdout != "" || stderr != "agx: no binary for this platform (linux-amd64)\n" {
		t.Fatalf("stdout %q stderr %q", stdout, stderr)
	}
}

func TestRunCannotLocateLauncher(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(launch{
		getenv:     envFrom(nil),
		executable: func() (string, error) { return "", errors.New("no path") },
		goos:       "windows",
		goarch:     "amd64",
		stdin:      strings.NewReader(""),
		stdout:     &stdout,
		stderr:     &stderr,
	})
	if code != 126 || !strings.HasPrefix(stderr.String(), "agx: cannot run agx-windows-amd64.exe: ") {
		t.Fatalf("exit %d stderr %q", code, stderr.String())
	}
}

// stubSource is the stand-in for the agx binary. STUB_MODE picks what it
// proves: argv arrives intact, stdin is shared, or an interrupt reaches it
// while the launcher survives.
const stubSource = `package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
)

func main() {
	switch os.Getenv("STUB_MODE") {
	case "args":
		json.NewEncoder(os.Stdout).Encode(os.Args[1:])
		os.Exit(7)
	case "stdin":
		io.Copy(os.Stdout, os.Stdin)
	case "cwd":
        dir,_:=os.Getwd()
        fmt.Println(dir)
    case "wide":
		os.Exit(300)
	case "interrupt":
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt)
		fmt.Println("ready")
		<-ch
		os.Exit(9)
	default:
		os.Exit(70)
	}
}
`

// driverSource starts the launcher in its own process group, waits for the
// stub's readiness line, sends CTRL_BREAK_EVENT to that group and reports the
// launcher's exit code. It runs as a separate console process so the test
// works whether or not the test process owns a console: a console program
// started from a console-less parent gets a console of its own, which the
// launcher then inherits.
const driverSource = `package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func main() {
	cmd := exec.Command(os.Args[1])
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}
	if err := cmd.Start(); err != nil {
		fmt.Println("ERROR", err)
		os.Exit(1)
	}
	ready := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(out).ReadString('\n')
		ready <- strings.TrimSpace(line)
	}()
	select {
	case line := <-ready:
		if line != "ready" {
			fmt.Println("ERROR readiness line", line)
			cmd.Process.Kill()
			os.Exit(1)
		}
	case <-time.After(30 * time.Second):
		fmt.Println("ERROR no readiness line")
		cmd.Process.Kill()
		os.Exit(1)
	}
	gen := syscall.NewLazyDLL("kernel32.dll").NewProc("GenerateConsoleCtrlEvent")
	if r, _, err := gen.Call(syscall.CTRL_BREAK_EVENT, uintptr(cmd.Process.Pid)); r == 0 {
		fmt.Println("SKIP GenerateConsoleCtrlEvent failed:", err)
		cmd.Process.Kill()
		os.Exit(0)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		fmt.Println("EXIT", cmd.ProcessState.ExitCode())
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		fmt.Println("SKIP the break event never arrived: the process has no usable console")
	}
}
`

func goTool(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go command not found: %v", err)
	}
	return path
}

func buildSource(t *testing.T, source, out string) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(goTool(t), "build", "-o", out, file)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", out, err, output)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

// childEnv pins the processor variables so the selected binary name does not
// depend on the host the test runs on.
func childEnv(extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(key) {
		case "PROCESSOR_ARCHITECTURE", "PROCESSOR_ARCHITEW6432", "PROCESSOR_IDENTIFIER", "STUB_MODE", "AGX_SOURCE_DIGEST":
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "PROCESSOR_ARCHITECTURE=AMD64", "PROCESSOR_IDENTIFIER=Intel64 Family 6 Model 154")
	return append(append(env, "AGX_NO_FETCH=1"), extra...)
}

func childBinaryName(t *testing.T, env []string) string {
	t.Helper()
	vars := map[string]string{}
	for _, kv := range env {
		key, value, _ := strings.Cut(kv, "=")
		vars[strings.ToUpper(key)] = value
	}
	name, err := binaryName("windows", runtime.GOARCH, envFrom(vars))
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("launcher did not run: %v", err)
	}
	return exitErr.ExitCode()
}

func TestLauncherProcess(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skipf("the launcher process runs only on Windows; this host is %s", runtime.GOOS)
	}
	built := t.TempDir()
	launcherBuild := filepath.Join(built, "agx.exe")
	stubBuild := filepath.Join(built, "stub.exe")
	cmd := exec.Command(goTool(t), "build", "-o", launcherBuild, ".")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build launcher: %v\n%s", err, output)
	}
	buildSource(t, stubSource, stubBuild)

	// install lays out a plugin root and returns the launcher path and the
	// path the launcher will select for env.
	install := func(t *testing.T, env []string, stub string) (string, string) {
		root := t.TempDir()
		launcher := filepath.Join(root, "bin", "agx.exe")
		copyFile(t, launcherBuild, launcher)
		binary := filepath.Join(root, "hooks", "bin", childBinaryName(t, env))
		if stub != "" {
			copyFile(t, stub, binary)
		}
		hooks := filepath.Join(root, "hooks")
		for _, file := range []string{"run-hook.cmd", "fetch-hook.ps1"} {
			copyFile(t, filepath.Join("..", "..", "..", file), filepath.Join(hooks, file))
		}
		meta := filepath.Join(root, ".claude-plugin")
		if err := os.MkdirAll(meta, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(meta, "plugin.json"), []byte(`{"version":"1.0.0"}`), 0644); err != nil {
			t.Fatal(err)
		}
		var data []byte
		if stub != "" {
			var err error
			data, err = os.ReadFile(stub)
			if err != nil {
				t.Fatal(err)
			}
		}
		setManifest(t, binary, data)
		return launcher, binary
	}

	t.Run("argv passes through and exit code returns", func(t *testing.T) {
		env := childEnv("STUB_MODE=args")
		launcher, _ := install(t, env, stubBuild)
		args := []string{`say "hi"`, "a&b", "two words", "", `back\slash\`, "plain"}
		cmd := exec.Command(launcher, args...)
		cmd.Env = env
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := exitCode(t, cmd.Run())
		if code != 7 {
			t.Fatalf("exit = %d, want 7 (stderr %q)", code, stderr.String())
		}
		var got []string
		if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
			t.Fatalf("stub output %q: %v", stdout.String(), err)
		}
		if strings.Join(got, "\x00") != strings.Join(args, "\x00") || len(got) != len(args) {
			t.Fatalf("argv = %q, want %q", got, args)
		}
	})

	t.Run("rejects a junction in the resolver path", func(t *testing.T) {
		env := childEnv("STUB_MODE=args")
		launcher, binary := install(t, env, stubBuild)
		hooks := filepath.Dir(filepath.Dir(binary))
		target := filepath.Join(filepath.Dir(hooks), "original-hooks")
		if err := os.Rename(hooks, target); err != nil {
			t.Fatal(err)
		}
		hookutiltest.JunctionOrSkip(t, target, hooks)
		cmd := exec.Command(launcher, "help")
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if exitCode(t, err) != 126 || !strings.Contains(string(out), "unsafe installed path") {
			t.Fatalf("resolver junction: %v: %s", err, out)
		}
	})
	t.Run("rejects an unrelated system root", func(t *testing.T) {
		env := childEnv("STUB_MODE=args")
		launcher, _ := install(t, env, stubBuild)
		cmd := exec.Command(launcher, "help")
		cmd.Env = append(env, "SystemRoot="+t.TempDir())
		out, err := cmd.CombinedOutput()
		if exitCode(t, err) != 126 || !strings.Contains(string(out), "untrusted Windows system root") {
			t.Fatalf("system root: %v: %s", err, out)
		}
	})
	t.Run("runtime inherits caller directory", func(t *testing.T) {
		env := childEnv("STUB_MODE=cwd")
		launcher, _ := install(t, env, stubBuild)
		cmd := exec.Command(launcher)
		cmd.Dir, cmd.Env = t.TempDir(), env
		out, err := cmd.CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != cmd.Dir {
			t.Fatalf("cwd=%q want %q: %v", out, cmd.Dir, err)
		}
	})

	t.Run("stdin reaches the binary", func(t *testing.T) {
		env := childEnv("STUB_MODE=stdin")
		launcher, _ := install(t, env, stubBuild)
		cmd := exec.Command(launcher)
		cmd.Env = env
		cmd.Stdin = strings.NewReader("line one\nline two\n")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if code := exitCode(t, cmd.Run()); code != 0 {
			t.Fatalf("exit = %d (stderr %q)", code, stderr.String())
		}
		if stdout.String() != "line one\nline two\n" {
			t.Fatalf("stdout = %q", stdout.String())
		}
	})

	t.Run("exit code above 255 returns whole", func(t *testing.T) {
		env := childEnv("STUB_MODE=wide")
		launcher, _ := install(t, env, stubBuild)
		cmd := exec.Command(launcher)
		cmd.Env = env
		if code := exitCode(t, cmd.Run()); code != 300 {
			t.Fatalf("exit = %d, want 300", code)
		}
	})

	t.Run("missing binary", func(t *testing.T) {
		env := childEnv()
		launcher, _ := install(t, env, "")
		cmd := exec.Command(launcher, "flow")
		cmd.Env = env
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if code := exitCode(t, cmd.Run()); code != 126 {
			t.Fatalf("exit = %d, want 126", code)
		}
		if !strings.Contains(stderr.String(), "cannot provision runtime") {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})

	t.Run("binary that cannot start", func(t *testing.T) {
		env := childEnv()
		launcher, binary := install(t, env, "")
		if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(binary, []byte("this is text, not a program\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		setManifest(t, binary, []byte("this is text, not a program\n"))
		cmd := exec.Command(launcher)
		cmd.Env = env
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if code := exitCode(t, cmd.Run()); code != 126 {
			t.Fatalf("exit = %d, want 126", code)
		}
		prefix := "agx: cannot run " + binary + ": "
		if !strings.HasPrefix(stderr.String(), prefix) {
			t.Fatalf("stderr = %q, want prefix %q", stderr.String(), prefix)
		}
	})

	t.Run("Ctrl+C leaves the launcher waiting for the binary", func(t *testing.T) {
		env := childEnv("STUB_MODE=interrupt")
		launcher, _ := install(t, env, stubBuild)
		driver := filepath.Join(t.TempDir(), "driver.exe")
		buildSource(t, driverSource, driver)
		cmd := exec.Command(driver, launcher)
		cmd.Env = env
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("driver: %v\nstdout %q\nstderr %q", err, stdout.String(), stderr.String())
		}
		line, _ := bufio.NewReader(&stdout).ReadString('\n')
		line = strings.TrimSpace(line)
		if reason, ok := strings.CutPrefix(line, "SKIP "); ok {
			t.Skipf("cannot deliver a console control event: %s", reason)
		}
		got, ok := strings.CutPrefix(line, "EXIT ")
		if !ok {
			t.Fatalf("driver output %q (stderr %q)", line, stderr.String())
		}
		if code, err := strconv.Atoi(got); err != nil || code != 9 {
			t.Fatalf("launcher exit = %s, want 9: the launcher must survive the interrupt and return the binary's code (stderr %q)", got, stderr.String())
		}
	})
}

func setManifest(t *testing.T, binary string, data []byte) {
	t.Helper()
	line := fmt.Sprintf("%x  bin/%s\n", sha256.Sum256(data), filepath.Base(binary))
	if err := os.WriteFile(filepath.Join(filepath.Dir(filepath.Dir(binary)), "SHA256SUMS"), []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
}
