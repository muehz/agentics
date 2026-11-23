package conformance

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// launcherArgs reach the agx binary through each launcher unchanged. They
// carry the characters cmd.exe, MSYS and sh each rewrite when quoting is
// wrong. None starts with "/": MSYS rewrites such arguments as paths when it
// starts a native program, which is the host's behavior, not the launcher's.
var launcherArgs = []string{
	`say "hi"`,
	"a&b",
	"x|y",
	"out>file",
	"caret^up",
	"%PATH%",
	"$(id)",
	`back\"quote`,
	`trailing\`,
	"",
	"two words",
	"plain",
	"--flag=value",
}

// launcherBinaries are every name bin/agx can select.
var launcherBinaries = []string{
	"agx-darwin-amd64",
	"agx-darwin-arm64",
	"agx-linux-amd64",
	"agx-linux-arm64",
	"agx-windows-amd64.exe",
	"agx-windows-arm64.exe",
}

const launcherStubSource = `package main

import (
	"encoding/json"
	"os"
)

func main() {
	_ = json.NewEncoder(os.Stdout).Encode(os.Args[1:])
	os.Exit(7)
}
`

// launcherScript returns the repository's bin/agx bytes.
func launcherScript(t *testing.T) []byte {
	t.Helper()
	script, err := os.ReadFile(filepath.Join(hosttest.Checkout(t), "bin", "agx"))
	if err != nil {
		t.Fatalf("read bin/agx: %v", err)
	}
	return script
}

// launcherTree writes bin/agx into a fresh plugin root and returns the root.
// Nothing touches the repository's bin/ or hooks/bin/.
func launcherTree(t *testing.T, script []byte) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"bin", filepath.Join("hooks", "bin")} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "agx"), script, 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// launcherStub builds the argv-printing stub with the go on PATH, which
// go test points at the module's toolchain.
func launcherStub(t *testing.T) []byte {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: the launcher argv cases build Go programs")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not on PATH")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stub.go"), []byte(launcherStubSource), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "stub")
	cmd := exec.Command(goBin, "build", "-o", out, "stub.go")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stub: %v\n%s", err, output)
	}
	stub, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return stub
}

// installLauncherStub writes the stub under every binary name so the
// selection on the test host does not decide whether the argv case runs.
func installLauncherStub(t *testing.T, root string, stub []byte) {
	t.Helper()
	for _, file := range []string{"run-hook.cmd", "fetch-hook.ps1"} {
		data, err := os.ReadFile(filepath.Join(hosttest.Checkout(t), "hooks", file))
		if err != nil {
			t.Fatal(err)
		}
		writeRel(t, root, "hooks/"+file, string(data))
	}
	writeRel(t, root, ".claude-plugin/plugin.json", `{"version":"1.2.3"}`)
	var manifest strings.Builder
	digest := sha256.Sum256(stub)
	for _, name := range launcherBinaries {
		fmt.Fprintf(&manifest, "%x  bin/%s\n", digest, name)
		if err := os.WriteFile(filepath.Join(root, "hooks", "bin", name), stub, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeRel(t, root, "hooks/SHA256SUMS", manifest.String())
}

// Match the dispatcher to the asset the fixture serves, even when the Go
// test process is running under emulation on an ARM64 Windows machine.
func launcherProcessorEnv(name string) []string {
	target := hookbuild.ContractTargets[name]
	if target[0] != "windows" {
		return nil
	}
	// Translated x64 launchers also consult the processor identifier.
	// Give them the same architecture through both inputs.
	identifier := "Intel64"
	if target[1] == "arm64" {
		identifier = "ARMv8"
	}
	return []string{
		"PROCESSOR_ARCHITECTURE=" + strings.ToUpper(target[1]),
		"PROCESSOR_ARCHITEW6432=",
		"PROCESSOR_IDENTIFIER=" + identifier,
	}
}

func launcherEnv(name string) []string {
	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "CLAUDE_") || strings.HasPrefix(upper, "CODEX_") ||
			strings.HasPrefix(upper, "COPILOT_") || strings.HasPrefix(upper, "ANTIGRAVITY_") ||
			strings.HasPrefix(upper, "AGX_") || upper == "PROCESSOR_ARCHITECTURE" ||
			upper == "PROCESSOR_ARCHITEW6432" || upper == "PROCESSOR_IDENTIFIER" {
			continue
		}
		env = append(env, kv)
	}
	return append(env, launcherProcessorEnv(name)...)
}

// shQuote single-quotes s for a generated sh script.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// gitBashLauncher returns a command that runs root's bin/agx under Git Bash
// with args. Go's Windows command-line quoting does not survive MSYS's
// parsing, so the arguments travel single-quoted inside a generated script,
// and every path is forward-slashed so $0 carries a "/" separator.
func gitBashLauncher(t *testing.T, sh, root string, args []string) *exec.Cmd {
	t.Helper()
	return gitBashLauncherAt(t, sh, filepath.ToSlash(filepath.Join(root, "bin", "agx")), args)
}

// gitBashLauncherAt is gitBashLauncher with the script path given as $0
// exactly, so a caller can pass a native backslash path.
func gitBashLauncherAt(t *testing.T, sh, script string, args []string) *exec.Cmd {
	t.Helper()
	var b strings.Builder
	b.WriteString("exec sh " + shQuote(script))
	for _, arg := range args {
		b.WriteString(" " + shQuote(arg))
	}
	b.WriteString("\n")
	driver := filepath.Join(t.TempDir(), "drive.sh")
	if err := os.WriteFile(driver, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return exec.Command(sh, filepath.ToSlash(driver))
}

type launcherResult struct {
	exit           int
	stdout, stderr string
}

func runLauncher(t *testing.T, cmd *exec.Cmd) launcherResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if cmd.Stdin == nil {
		cmd.Stdin = strings.NewReader("")
	}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	exit := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exit = exitErr.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return launcherResult{exit: exit, stdout: stdout.String(), stderr: stderr.String()}
}

func assertLauncherArgv(t *testing.T, got launcherResult) {
	t.Helper()
	if got.exit != 7 {
		t.Fatalf("exit = %d, want the stub's 7 (stdout %q, stderr %q)", got.exit, got.stdout, got.stderr)
	}
	var argv []string
	if err := json.Unmarshal([]byte(got.stdout), &argv); err != nil {
		t.Fatalf("stub output %q: %v", got.stdout, err)
	}
	if len(argv) != len(launcherArgs) {
		t.Fatalf("argv = %q, want %q", argv, launcherArgs)
	}
	for i := range launcherArgs {
		if argv[i] != launcherArgs[i] {
			t.Errorf("arg %d = %q, want %q", i, argv[i], launcherArgs[i])
		}
	}
}

func TestLauncherPassesArgvThroughSh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/bin/sh is the non-Windows entry; Windows runs the Git Bash case")
	}
	script := launcherScript(t)
	stub := launcherStub(t)
	root := launcherTree(t, script)
	installLauncherStub(t, root, stub)
	cmd := exec.Command("/bin/sh", append([]string{filepath.Join(root, "bin", "agx")}, launcherArgs...)...)
	assertLauncherArgv(t, runLauncher(t, cmd))
}

// rebuildLauncher builds the agx launcher from hooksDir into outDir, and fails
// the test on the error and on a failed launcher build. Rebuild reports a failed
// target in its Result, and the launcher's error carries the go command's
// output.
func rebuildLauncher(t *testing.T, hooksDir, goBin, outDir string) {
	t.Helper()
	res, err := hookbuild.Rebuild(hookbuild.Request{SourceDir: hooksDir, GoBin: goBin, OutDir: outDir, Launcher: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Launcher.Err != nil {
		t.Fatal(res.Launcher.Err)
	}
}

func TestLauncherPassesArgvThroughExe(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("bin/agx.exe is the native Windows entry")
	}
	stub := launcherStub(t)
	root := launcherTree(t, nil)
	installLauncherStub(t, root, stub)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not on PATH")
	}
	hooksDir := filepath.Join(hosttest.Checkout(t), "hooks", "go")
	exe := filepath.Join(root, filepath.FromSlash(hookbuild.LauncherOutput))
	rebuildLauncher(t, hooksDir, goBin, filepath.Dir(exe))
	cmd := exec.Command(exe, launcherArgs...)
	cmd.Env = launcherEnv(hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH))
	assertLauncherArgv(t, runLauncher(t, cmd))
}

func TestLauncherPassesArgvThroughGitBash(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Git Bash runs bin/agx only on Windows")
	}
	sh := hookutiltest.RequireInterpreter(t, "sh")
	script := launcherScript(t)
	stub := launcherStub(t)
	root := launcherTree(t, script)
	installLauncherStub(t, root, stub)
	cmd := gitBashLauncher(t, sh, root, launcherArgs)
	cmd.Env = launcherEnv(hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH))
	assertLauncherArgv(t, runLauncher(t, cmd))
}

// TestLauncherPassesArgvThroughGitBashBackslashPath runs bin/agx with $0 as
// a native Windows path, the form a Windows caller hands Git Bash, so the
// plugin root must come from splitting $0 on a backslash.
func TestLauncherPassesArgvThroughGitBashBackslashPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Git Bash runs bin/agx only on Windows")
	}
	sh := hookutiltest.RequireInterpreter(t, "sh")
	script := launcherScript(t)
	stub := launcherStub(t)
	root := launcherTree(t, script)
	installLauncherStub(t, root, stub)
	native := filepath.Join(root, "bin", "agx")
	if strings.Contains(native, "/") {
		t.Fatalf("native path %q holds a forward slash", native)
	}
	cmd := gitBashLauncherAt(t, sh, native, launcherArgs)
	cmd.Env = launcherEnv(hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH))
	assertLauncherArgv(t, runLauncher(t, cmd))
}

// Each install has its own release server and cache. Direct calls run outside
// the install with provider-specific environment removed.
func TestLauncherDirectProvision(t *testing.T) {
	for _, surface := range launcherSurfaces() {
		for _, state := range []string{"cold", "preseed", "provider-warmed", "offline miss"} {
			t.Run(surface+"/"+state, func(t *testing.T) {
				f := newFetchFixture(t)
				root := filepath.Dir(f.hooksDir)
				installLauncherEntry(t, root, surface)
				if state == "preseed" {
					if err := os.WriteFile(filepath.Join(f.hooksDir, "bin", f.name), f.binary, 0755); err != nil {
						t.Fatal(err)
					}
				}
				if state == "provider-warmed" {
					providerEnv := append(launcherProcessorEnv(f.name), "CLAUDE_PLUGIN_ROOT="+root, "CLAUDE_PLUGIN_DATA="+t.TempDir())
					code, _, diag := f.dispatch(t, providerEnv, "claude-code", "command-guardian")
					if code != 1 || f.requests.Load() != 1 {
						t.Fatalf("provider warm-up: exit %d, requests %d, stderr %q", code, f.requests.Load(), diag)
					}
				}
				before := f.requests.Load()
				cmd := launchCommand(t, root, surface, "help")
				cmd.Dir = t.TempDir()
				cmd.Env = append(launcherEnv(f.name), f.cacheEnv...)
				cmd.Env = append(cmd.Env, "AGX_RELEASE_URL="+f.server.URL)
				if state != "cold" {
					cmd.Env = append(cmd.Env, "AGX_NO_FETCH=1")
				}
				got := runLauncher(t, cmd)
				if state == "offline miss" {
					if got.exit != 126 || got.stdout != "" || !strings.Contains(got.stderr, "provision") {
						t.Fatalf("offline: %+v", got)
					}
				} else if got.exit != 0 || !strings.Contains(got.stdout, "agx") {
					t.Fatalf("direct help: %+v", got)
				}
				if state == "provider-warmed" {
					payload, err := os.ReadFile(filepath.Join(hosttest.Checkout(t), "hooks", "go", "surfaces", "claude-code", "payloads", "pre-tool-use-dangerous.json"))
					if err != nil {
						t.Fatal(err)
					}
					guard := launchCommand(t, root, surface, "claude-code", "command-guardian", "--mode", "block")
					guard.Env, guard.Dir, guard.Stdin = cmd.Env, cmd.Dir, bytes.NewReader(payload)
					if result := runLauncher(t, guard); result.exit != 2 || result.stdout != "" || !strings.Contains(result.stderr, "destructive_file_ops") {
						t.Fatalf("direct guard: %+v", result)
					}
				}
				want := int64(0)
				if state == "cold" {
					want = 1
				}
				if n := f.requests.Load() - before; n != want {
					t.Fatalf("direct requests = %d, want %d", n, want)
				}
			})
		}
	}
}

func launcherSurfaces() []string {
	if runtime.GOOS == "windows" {
		return []string{"exe", "git-bash"}
	}
	return []string{"sh"}
}

func installLauncherEntry(t *testing.T, root, surface string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if surface == "exe" {
		rebuildLauncher(t, filepath.Join(hosttest.Checkout(t), "hooks", "go"), "go", filepath.Join(root, "bin"))
		return
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "agx"), launcherScript(t), 0755); err != nil {
		t.Fatal(err)
	}
}

func launchCommand(t *testing.T, root, surface string, args ...string) *exec.Cmd {
	t.Helper()
	switch surface {
	case "exe":
		return exec.Command(filepath.Join(root, "bin", "agx.exe"), args...)
	case "git-bash":
		return gitBashLauncher(t, hookutiltest.RequireInterpreter(t, "sh"), root, args)
	default:
		return exec.Command("/bin/sh", append([]string{filepath.Join(root, "bin", "agx")}, args...)...)
	}
}

func TestLauncherRefusesNonImageBinary(t *testing.T) {
	surface := "sh"
	if runtime.GOOS == "windows" {
		surface = "git-bash"
	}
	for _, tc := range []struct{ name, body, reason string }{
		{"empty", "", "empty file"}, {"text", "echo ran\n", "not an executable image"}, {"short", "M", "not an executable image"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := launcherTree(t, launcherScript(t))
			installLauncherStub(t, root, []byte(tc.body))
			got := runLauncher(t, launchCommand(t, root, surface))
			if got.exit != 126 || got.stdout != "" || !strings.Contains(got.stderr, tc.reason) {
				t.Fatalf("invalid native image: %+v", got)
			}
		})
	}
}

// Exercise an unavailable inspector with a verified, runnable image. The
// installed launcher's system utility lookup is independent of the test PATH.
func TestLauncherFailsClosedWithoutOd(t *testing.T) {
	script := launcherScript(t)
	probe := []byte("command -p od -An -tx1 -N4 ")
	if bytes.Count(script, probe) != 1 {
		t.Fatal("cannot identify the launcher's image inspection command")
	}
	root := launcherTree(t, bytes.Replace(script, probe, []byte("false "), 1))
	installLauncherStub(t, root, launcherStub(t))
	surface := "sh"
	if runtime.GOOS == "windows" {
		surface = "git-bash"
	}
	cmd := launchCommand(t, root, surface, launcherArgs...)
	cmd.Env = append(launcherEnv(hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH)), "AGX_NO_FETCH=1")
	got := runLauncher(t, cmd)
	if got.exit != 126 || got.stdout != "" || !strings.HasSuffix(got.stderr, ": cannot inspect the file\n") {
		t.Fatalf("failed image inspection: %+v", got)
	}
}

// launcherModeExecutable reports whether a `git ls-files -s` line records
// mode 100755. An empty line means the file is untracked.
func launcherModeExecutable(line string) error {
	line = strings.TrimSpace(line)
	if line == "" {
		return errors.New("bin/agx is not tracked")
	}
	if !strings.HasPrefix(line, "100755 ") {
		mode, _, _ := strings.Cut(line, " ")
		return fmt.Errorf("bin/agx is committed with mode %s, want 100755", mode)
	}
	return nil
}

func TestLauncherModeExecutable(t *testing.T) {
	cases := []struct {
		line string
		ok   bool
	}{
		{line: "100755 3b18e512dba79e4c8300dd08aeb37f8e728b8dad 0\tbin/agx\n", ok: true},
		{line: "100644 3b18e512dba79e4c8300dd08aeb37f8e728b8dad 0\tbin/agx\n", ok: false},
		{line: "", ok: false},
	}
	for _, tc := range cases {
		err := launcherModeExecutable(tc.line)
		if (err == nil) != tc.ok {
			t.Errorf("launcherModeExecutable(%q) = %v, want ok=%v", tc.line, err, tc.ok)
		}
	}
}

func TestLauncherCommittedExecutable(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	root := hosttest.Checkout(t)
	if err := exec.Command(git, "-C", root, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		t.Skipf("%s is not a usable git work tree (a source archive or a mounted worktree): %v", root, err)
	}
	out, err := exec.Command(git, "-C", root, "ls-files", "-s", "--", "bin/agx").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	if err := launcherModeExecutable(string(out)); err != nil {
		t.Fatal(err)
	}
}

func TestLauncherRejectsMalformedResolution(t *testing.T) {
	stub := launcherStub(t)
	for _, surface := range launcherSurfaces() {
		t.Run(surface, func(t *testing.T) {
			root := launcherTree(t, launcherScript(t))
			installLauncherEntry(t, root, surface)
			installLauncherStub(t, root, stub)
			name := hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH)
			path := filepath.Join(root, "hooks", "bin", name)
			if surface == "sh" {
				// The POSIX dispatcher prints pwd -P even when TMPDIR names a symlink.
				var err error
				path, err = filepath.EvalSymlinks(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			caller := t.TempDir()
			unrelated := filepath.Join(caller, name)
			if err := os.WriteFile(unrelated, stub, 0755); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name, output string
				status       int
			}{
				{"valid", path + "\n", 0}, {"no LF", path, 0}, {"extra LF", path + "\n\n", 0}, {"trailing bytes", path + "\nx", 0},
				{"CRLF", path + "\r\n", 0}, {"NUL", path + "\x00\n", 0}, {"two paths", path + "\n" + path + "\n", 0},
				{"unrelated image", unrelated + "\n", 0}, {"relative", name + "\n", 0},
				{"invalid UTF8", path + "\xff\n", 0},
				{"truncated UTF8", path + "\xe2\x82\n", 0},
				{"overlong UTF8", path + "\xe0\x80\x80\n", 0},
				{"surrogate UTF8", path + "\xed\xa0\x80\n", 0},
				{"out of range UTF8", path + "\xf4\x90\x80\x80\n", 0},
				{"failed resolver", path + "\n", 23},
			} {
				t.Run(tc.name, func(t *testing.T) {
					writeRel(t, root, "hooks/response", tc.output)
					body := fmt.Sprintf("#!/bin/sh\n[ \"$#\" = 1 ] && [ \"$1\" = --provision-only ] || exit 99\ncommand -p cat \"${0%%/*}/response\"\nexit %d\n", tc.status)
					if runtime.GOOS == "windows" {
						body = fmt.Sprintf("@echo off\r\nif not \"%%~1\"==\"--provision-only\" exit /b 99\r\nif not \"%%~2\"==\"\" exit /b 99\r\ntype response\r\nexit /b %d\r\n", tc.status)
					}
					writeRel(t, root, "hooks/run-hook.cmd", body)
					cmd := launchCommand(t, root, surface, launcherArgs...)
					cmd.Dir, cmd.Env = caller, launcherEnv(name)
					got := runLauncher(t, cmd)
					if tc.name == "valid" {
						assertLauncherArgv(t, got)
						return
					}
					reason := "provision"
					if strings.HasSuffix(tc.name, "UTF8") {
						reason = "invalid UTF-8 in provision-only response"
					}
					if got.exit != 126 || got.stdout != "" || !strings.Contains(got.stderr, reason) {
						t.Fatalf("malformed response: %+v", got)
					}
				})
			}
		})
	}
}
