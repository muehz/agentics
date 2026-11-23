// Command launcher is bin/agx.exe, the native Windows entry point for agx.
//
// It asks the installed dispatcher to provision the verified runtime, then runs
// that executable with the caller's arguments, directory and streams. It exits
// with the child's full status, including when Ctrl+C reaches their console.
// A provisioning or launch failure exits 126 with a diagnostic on stderr.
//
// With AGX_SOURCE_DIGEST set to exactly 1 it prints its source digest and the
// Go toolchain that built it, one per line, and exits 0 without running the
// binary.
//
// The launcher imports only the standard library so it stays a small,
// self-contained executable.
package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// sourceDigest is stamped at link time with -X main.sourceDigest. The
// env-gated print in run references it, which keeps the linker from
// discarding the symbol and silently dropping the stamp.
var sourceDigest = "agentics-src-unset"

// printDigestEnv must equal exactly "1" so a stray empty or "0" value never
// turns an ordinary invocation into a provenance print.
const printDigestEnv = "AGX_SOURCE_DIGEST"

// unavailableExit is the shell convention for "found but cannot execute";
// callers read the stderr line to learn why.
const unavailableExit = 126

// launch carries the process inputs so run can be tested in-process.
type launch struct {
	args       []string
	getenv     func(string) string
	executable func() (string, error)
	goos       string
	goarch     string
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
}

func main() {
	os.Exit(run(launch{
		args:       os.Args[1:],
		getenv:     os.Getenv,
		executable: os.Executable,
		goos:       runtime.GOOS,
		goarch:     runtime.GOARCH,
		stdin:      os.Stdin,
		stdout:     os.Stdout,
		stderr:     os.Stderr,
	}))
}

// binaryName returns the agx binary for goos and the processor getenv
// describes. The launcher's own goarch does not decide: an x64 launcher
// emulated on ARM64 still selects the native arm64 binary, which reports
// itself through PROCESSOR_IDENTIFIER even when PROCESSOR_ARCHITECTURE says
// AMD64. Comparisons ignore case, as run-hook.cmd's "if /i" does.
func binaryName(goos, goarch string, getenv func(string) string) (string, error) {
	if goos != "windows" {
		return "", fmt.Errorf("agx: no binary for this platform (%s-%s)", goos, goarch)
	}
	arch := "amd64"
	identifier := getenv("PROCESSOR_IDENTIFIER")
	if strings.EqualFold(getenv("PROCESSOR_ARCHITECTURE"), "ARM64") ||
		strings.EqualFold(getenv("PROCESSOR_ARCHITEW6432"), "ARM64") ||
		(len(identifier) >= 5 && strings.EqualFold(identifier[:5], "ARMv8")) {
		arch = "arm64"
	}
	return "agx-windows-" + arch + ".exe", nil
}

func run(l launch) int {
	if l.getenv(printDigestEnv) == "1" {
		fmt.Fprintln(l.stdout, sourceDigest)
		fmt.Fprintln(l.stdout, "built-by "+runtime.Version())
		return 0
	}
	name, err := binaryName(l.goos, l.goarch, l.getenv)
	if err != nil {
		fmt.Fprintln(l.stderr, err)
		return unavailableExit
	}
	self, err := l.executable()
	if err != nil {
		fmt.Fprintf(l.stderr, "agx: cannot run %s: cannot locate the launcher: %v\n", name, err)
		return unavailableExit
	}
	path, err := provision(self, name, l.getenv, l.stderr)
	if err != nil {
		fmt.Fprintf(l.stderr, "agx: cannot provision runtime: %v\n", err)
		return unavailableExit
	}

	cmd := exec.Command(path, l.args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = l.stdin, l.stdout, l.stderr

	// The binary shares the console, so Ctrl+C reaches it directly and it
	// decides how to stop. The launcher only has to outlive it to return its
	// code. A registered channel marks the event handled; an ignore-only
	// disposition would let the default console handler terminate the
	// launcher. The registration is never stopped: an event arriving between
	// the binary's exit and the launcher's would otherwise replace the code.
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	go func() {
		for range interrupts {
		}
	}()

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(l.stderr, "agx: cannot run %s: %v\n", path, err)
		return unavailableExit
	}
	// Wait's error only repeats the exit status or reports a stdio copy
	// failure after the binary ran; either way the binary's code is the
	// answer. Windows exit codes are 32 bits, and os.Exit passes them whole.
	_ = cmd.Wait()
	return cmd.ProcessState.ExitCode()
}

// A UNC working directory can make cmd.exe silently fall back to the Windows
// directory. Restrict the resolver's working directory to an installed local
// drive and reject links or mount points on the path to its scripts.
func provision(self, name string, getenv func(string) string, stderr io.Writer) (string, error) {
	if !filepath.IsAbs(self) || len(filepath.VolumeName(self)) != 2 || filepath.Clean(self) != self {
		return "", errors.New("the installed launcher needs an absolute local drive path")
	}
	hooks := filepath.Join(filepath.Dir(filepath.Dir(self)), "hooks")
	for _, path := range []string{self, filepath.Join(hooks, "run-hook.cmd"), filepath.Join(hooks, "fetch-hook.ps1")} {
		if err := checkInstalledPath(path); err != nil {
			return "", err
		}
	}
	// The dispatcher locates Windows PowerShell through SystemRoot. Bind that
	// directory to the kernel's system-root link before trusting either program.
	systemRoot := getenv("SystemRoot")
	if !filepath.IsAbs(systemRoot) || len(filepath.VolumeName(systemRoot)) != 2 {
		return "", errors.New("invalid Windows system root")
	}
	root, err := os.Stat(systemRoot)
	if err != nil {
		return "", err
	}
	actual, err := os.Stat(`\\?\GLOBALROOT\SystemRoot`)
	if err != nil || !os.SameFile(root, actual) {
		return "", errors.New("untrusted Windows system root")
	}
	resolver := exec.Command(filepath.Join(systemRoot, "System32", "cmd.exe"), "/d", "/e:on", "/v:off", "/c", `.\run-hook.cmd`, "--provision-only")
	resolver.Dir = hooks
	resolver.Stderr = stderr
	// cmd expands these variables as batch syntax. Only the resolved architecture
	// is needed, and an inherited ERRORLEVEL would shadow cmd's actual status.
	resolver.Env = resolverEnvironment(name)
	response, err := resolver.Output()
	if err != nil {
		return "", err
	}
	return runtimePath(response, hooks, getenv("LOCALAPPDATA"), name)
}

func resolverEnvironment(name string) []string {
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "PROCESSOR_ARCHITECTURE", "PROCESSOR_ARCHITEW6432", "PROCESSOR_IDENTIFIER", "ERRORLEVEL":
		default:
			env = append(env, entry)
		}
	}
	if name == "agx-windows-arm64.exe" {
		return append(env, "PROCESSOR_ARCHITECTURE=ARM64")
	}
	return append(env, "PROCESSOR_ARCHITECTURE=AMD64")
}

// The installed resolver verifies the bytes. Accept its one framed path only
// for the selected asset in the installation or versioned digest cache.
func runtimePath(response []byte, hooks, cache, name string) (string, error) {
	path, framed := strings.CutSuffix(string(response), "\n")
	if !utf8.ValidString(path) {
		return "", errors.New("invalid UTF-8 in provision-only response")
	}
	if !framed || strings.ContainsAny(path, "\x00\r\n") || !filepath.IsAbs(path) || filepath.Base(path) != name {
		return "", errors.New("invalid provision-only response")
	}
	if installed := filepath.Join(hooks, "bin", name); filepath.Clean(path) == installed {
		return installed, nil
	}
	rel, err := filepath.Rel(filepath.Join(cache, "agentics"), path)
	parts := strings.Split(rel, string(filepath.Separator))
	if !filepath.IsAbs(cache) || err != nil || len(parts) != 3 || parts[0] == ".." || len(parts[1]) != 64 || parts[2] != name {
		return "", errors.New("unexpected provisioned runtime path")
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return "", errors.New("invalid runtime cache digest")
	}
	return path, nil
}

func checkInstalledPath(path string) error {
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if (current == path && !info.Mode().IsRegular()) || (current != path && info.Mode().Type() != os.ModeDir) {
			return fmt.Errorf("unsafe installed path %s", current)
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}
