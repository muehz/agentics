package conformance

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// TestDispatcherResolvesPlatformBinary runs the Unix half of run-hook.cmd
// with Bash platform variables and asserts which binary name it execs.
// The fake binaries print their own name, so the assertion is the exec
// itself, not a debug line. Fetch is disabled and PATH holds only the
// verification tools and poison probe, so invoking uname leaves a marker.
func TestDispatcherResolvesPlatformBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Unix half of the polyglot never runs under cmd.exe")
	}
	bash := hookutiltest.RequireInterpreter(t, "bash")
	repoRoot := hosttest.Checkout(t)
	script, err := os.ReadFile(filepath.Join(repoRoot, "hooks", "run-hook.cmd"))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		uname, binary string
		env           []string
		sysctl        string
	}{
		{uname: "Darwin arm64", binary: "agx-darwin-arm64"},
		{uname: "Darwin x86_64", binary: "agx-darwin-amd64", sysctl: "0"},
		{uname: "Darwin x86_64", binary: "agx-darwin-arm64", sysctl: "1"},
		{uname: "Linux x86_64", binary: "agx-linux-amd64"},
		{uname: "Linux aarch64", binary: "agx-linux-arm64"},
		{uname: "MINGW64_NT-10.0-26200 x86_64", binary: "agx-windows-amd64.exe"},
		{uname: "MINGW64_NT-10.0 aarch64", binary: "agx-windows-arm64.exe"},
		{uname: "MINGW64_NT-10.0 x86_64", binary: "agx-windows-arm64.exe", env: []string{"PROCESSOR_IDENTIFIER=ARMv8 (64-bit) Family 8"}},
		{uname: "MINGW64_NT-10.0 x86_64", binary: "agx-windows-amd64.exe", env: []string{"PROCESSOR_IDENTIFIER=Intel64 Family 6"}},
		{uname: "MSYS_NT-10.0 x86_64", binary: "agx-windows-amd64.exe"},
		{uname: "CYGWIN_NT-10.0 x86_64", binary: "agx-windows-amd64.exe"},
		{uname: "SunOS i86pc", binary: ""},
		{uname: "Linux unknown", binary: ""},
		{uname: "Linux amd64", binary: "agx-linux-amd64"},
		{uname: "Linux x86_64", binary: "agx-linux-amd64", env: []string{"AGX_TEST_FALLBACK=1"}},
		{uname: "MINGW64 x86_64", binary: "agx-windows-arm64.exe", env: []string{"PROCESSOR_ARCHITECTURE=ARM64"}},
		{uname: "MINGW64 x86_64", binary: "agx-windows-arm64.exe", env: []string{"PROCESSOR_ARCHITEW6432=ARM64"}},
	}
	for _, tc := range cases {
		name := tc.uname
		if tc.sysctl == "1" {
			name += " rosetta"
		}
		if len(tc.env) > 0 {
			name += " " + strings.Join(tc.env, ",")
		}
		t.Run(name, func(t *testing.T) {
			hooksDir := filepath.Join(t.TempDir(), "hooks")
			if err := os.MkdirAll(filepath.Join(hooksDir, "bin"), 0o755); err != nil {
				t.Fatal(err)
			}
			fixtureScript := script
			if tc.uname == "Darwin x86_64" {
				// Substitute only in the fixture copy to simulate Rosetta on either host OS.
				// Quoting PWD also supports temporary directories with shell metacharacters.
				fixtureScript = bytes.ReplaceAll(script, []byte("/usr/sbin/sysctl"), []byte(`"$PWD/system-sysctl"`))
				probe := "#!/bin/sh\n[ \"$#\" = 2 ] && [ \"$1\" = -n ] && [ \"$2\" = sysctl.proc_translated ] || exit 1\nprintf '%s\\n' '" + tc.sysctl + "'\n"
				if err := os.WriteFile(filepath.Join(hooksDir, "system-sysctl"), []byte(probe), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(hooksDir, "run-hook.cmd"), fixtureScript, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(filepath.Dir(hooksDir), ".claude-plugin"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(filepath.Dir(hooksDir), ".claude-plugin", "plugin.json"), []byte(`{"version":"9.9.9"}`), 0644); err != nil {
				t.Fatal(err)
			}
			var manifest strings.Builder
			for _, binName := range []string{
				"agx-darwin-amd64",
				"agx-darwin-arm64",
				"agx-linux-amd64",
				"agx-linux-arm64",
				"agx-windows-amd64.exe",
				"agx-windows-arm64.exe",
			} {
				fake := "#!/bin/sh\nprintf '%s\\n' \"${0##*/}\"\n"
				fmt.Fprintf(&manifest, "%x  bin/%s\n", sha256.Sum256([]byte(fake)), binName)
				if err := os.WriteFile(filepath.Join(hooksDir, "bin", binName), []byte(fake), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(hooksDir, "SHA256SUMS"), []byte(manifest.String()), 0644); err != nil {
				t.Fatal(err)
			}
			stub := t.TempDir()
			linkVerificationTools(t, stub)
			marker := filepath.Join(stub, "uname-called")
			uname := "#!/bin/sh\nprintf forbidden > \"$AGX_TEST_UNAME_MARKER\"\nexit 99\n"
			if err := os.WriteFile(filepath.Join(stub, "uname"), []byte(uname), 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.uname == "Darwin x86_64" {
				// A harmless system false shadows a bare sysctl on PATH. Trace the
				// selected command even when its exit status would yield the right architecture.
				poison, err := exec.LookPath("false")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(poison, filepath.Join(stub, "sysctl")); err != nil {
					t.Fatal(err)
				}
				for _, args := range [][]string{{"--provision-only"}, {"claude-code", "pre-tool"}} {
					launch := append([]string{"-x", "-c", `OSTYPE=darwin; HOSTTYPE=x86_64; source ./run-hook.cmd`, "platform-launcher"}, args...)
					cmd := exec.Command(bash, launch...)
					cmd.Dir = hooksDir
					cmd.Env = []string{"PATH=" + stub, "AGX_NO_FETCH=1", "AGX_TEST_UNAME_MARKER=" + marker}
					var trace bytes.Buffer
					cmd.Stderr = &trace
					out, runErr := cmd.Output()
					if strings.Contains(trace.String(), "+ sysctl -n sysctl.proc_translated") {
						t.Errorf("%v used caller PATH for sysctl", args)
					}
					if !strings.Contains(trace.String(), "/system-sysctl") {
						t.Errorf("%v did not run the fixed probe", args)
					}
					want := tc.binary
					if args[0] == "--provision-only" {
						want, err = filepath.EvalSymlinks(filepath.Join(hooksDir, "bin", tc.binary))
						if err != nil {
							t.Fatal(err)
						}
					}
					if runErr != nil || string(out) != want+"\n" {
						t.Errorf("%v returned %q, %v; want %q", args, out, runErr, want)
					}
				}
				if _, err := os.Stat(marker); !os.IsNotExist(err) {
					t.Fatalf("dispatcher invoked forbidden uname: %v", err)
				}
				return
			}

			platform := strings.SplitN(tc.uname, " ", 2)
			osType := strings.ToLower(platform[0])
			cmd := exec.Command(bash, "-c", `OSTYPE="$1"; HOSTTYPE="$2"; MACHTYPE="$2-test"; if [ -n "${AGX_TEST_FALLBACK:-}" ]; then HOSTTYPE=""; fi; shift 2; source ./run-hook.cmd`, "platform-launcher", osType, platform[1], "claude-code", "pre-tool")
			cmd.Dir = hooksDir
			cmd.Env = append([]string{"PATH=" + stub, "AGX_TEST_UNAME_MARKER=" + marker, "AGX_NO_FETCH=1"}, tc.env...)
			cmd.Stdin = strings.NewReader("")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			exit := 0
			if exitErr, ok := err.(*exec.ExitError); ok {
				exit = exitErr.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}

			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("dispatcher invoked forbidden uname: %v", err)
			}
			if tc.binary == "" {
				if exit != 2 || !strings.Contains(stderr.String(), "agx: guard runtime unavailable - failing closed") {
					t.Fatalf("unknown platform must fail closed at exit 2, got %d\n%s", exit, stderr.String())
				}
				agCmd := exec.Command(bash, "-c", `OSTYPE="$1"; HOSTTYPE="$2"; MACHTYPE="$2-test"; shift 2; source ./run-hook.cmd`, "platform-launcher", osType, platform[1], "antigravity", "pre-tool")
				agCmd.Dir = hooksDir
				agCmd.Env = append([]string{"PATH=" + stub, "AGX_TEST_UNAME_MARKER=" + marker, "AGX_NO_FETCH=1"}, tc.env...)
				agCmd.Stdin = strings.NewReader("")
				var agOut, agErr bytes.Buffer
				agCmd.Stdout, agCmd.Stderr = &agOut, &agErr
				if err := agCmd.Run(); err != nil {
					t.Fatalf("unknown platform for antigravity must exit 0 with deny JSON: %v (stdout=%q stderr=%q)", err, agOut.String(), agErr.String())
				}
				assertAntigravityFailClosedJSON(t, agOut.String(), "agx: guard runtime unavailable - failing closed")
				if !strings.Contains(agErr.String(), "agx: guard runtime unavailable - failing closed") {
					t.Fatalf("unknown platform for antigravity lost stderr diagnostic: %q", agErr.String())
				}
				return
			}
			if exit != 0 || strings.TrimSpace(stdout.String()) != tc.binary {
				t.Fatalf("exit %d stdout %q stderr %q, want exec of %s", exit, stdout.String(), stderr.String(), tc.binary)
			}
		})
	}
}

func TestDispatcherPOSIXEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a real /bin/sh entry")
	}
	bash := hookutiltest.RequireInterpreter(t, "bash")
	for _, provider := range []string{"claude-code", "antigravity"} {
		for _, missingBash := range []bool{false, true} {
			for _, hook := range []string{"pre-tool", "command-guardian", "release-guardian", "secrets-guardian", "unknown-hook"} {
				t.Run(provider+"/"+hook+map[bool]string{false: "/reexec", true: "/missing-bash"}[missingBash], func(t *testing.T) {
					f := newFetchFixture(t)
					stub := t.TempDir()
					marker := filepath.Join(stub, "uname-called")
					if err := os.WriteFile(filepath.Join(stub, "uname"), []byte("#!/bin/sh\necho forbidden > \"$AGX_TEST_UNAME_MARKER\"\nexit 99\n"), 0o755); err != nil {
						t.Fatal(err)
					}
					if !missingBash {
						linkVerificationTools(t, stub)
						if err := os.Symlink(bash, filepath.Join(stub, "bash")); err != nil {
							t.Fatal(err)
						}
						fake := "#!/bin/sh\nprintf 'executed:%s:%s\\n' \"$1\" \"$2\"\n"
						recordManifest(t, f.hooksDir, f.name, []byte(fake))
						if err := os.WriteFile(filepath.Join(f.hooksDir, "bin", f.name), []byte(fake), 0o755); err != nil {
							t.Fatal(err)
						}
					}
					args := []string{provider, hook}
					cmd := exec.Command("/bin/sh", append([]string{filepath.Join(f.hooksDir, "run-hook.cmd")}, args...)...)
					cmd.Env = []string{"PATH=" + stub, "AGX_NO_FETCH=1", "AGX_TEST_UNAME_MARKER=" + marker}
					var stdout, stderr bytes.Buffer
					cmd.Stdout, cmd.Stderr = &stdout, &stderr
					err := cmd.Run()
					if cmd.ProcessState == nil {
						t.Fatal(err)
					}
					exit := cmd.ProcessState.ExitCode()
					if !missingBash {
						want := "executed:" + args[0] + ":" + args[1] + "\n"
						if exit != 0 || stdout.String() != want || stderr.Len() != 0 {
							t.Fatalf("real sh entry must reexec Bash and host binary: exit %d stdout %q stderr %q", exit, stdout.String(), stderr.String())
						}
					} else if hook == "pre-tool" || strings.HasSuffix(hook, "guardian") {
						if provider == "antigravity" {
							if exit != 0 || !strings.Contains(stderr.String(), "failing closed") {
								t.Fatalf("missing Bash antigravity guard: exit %d stdout %q stderr %q", exit, stdout.String(), stderr.String())
							}
							assertAntigravityFailClosedJSON(t, stdout.String(), "agx: guard runtime unavailable - failing closed")
						} else if exit != 2 || !strings.Contains(stderr.String(), "failing closed") {
							t.Fatalf("missing Bash guard: exit %d stderr %q", exit, stderr.String())
						}
					} else if exit != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
						t.Fatalf("missing Bash with an unknown hook name must be silent: exit %d stdout %q stderr %q", exit, stdout.String(), stderr.String())
					}
					if _, err := os.Stat(marker); !os.IsNotExist(err) {
						t.Fatalf("POSIX entry invoked uname: %v", err)
					}
				})
			}
		}
	}
}

func TestDispatcherNativeWindowsNeverStartsBash(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires native cmd.exe")
	}
	for _, present := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "present"}[present], func(t *testing.T) {
			f := newFetchFixture(t)
			stub := t.TempDir()
			marker := filepath.Join(stub, "bash-called")
			if err := os.WriteFile(filepath.Join(stub, "bash.cmd"), []byte("@echo forbidden>\"%AGX_TEST_BASH_MARKER%\"\r\n@exit /b 99\r\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			if present {
				if err := os.WriteFile(filepath.Join(f.hooksDir, "bin", f.name), f.binary, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			exit, _, stderr := f.dispatch(t, []string{"PATH=" + stub, "AGX_TEST_BASH_MARKER=" + marker, "AGX_NO_FETCH=1"}, "claude-code", "command-guardian")
			want := 2
			if present {
				want = 1 // Legacy binary's empty-input rejection proves direct execution.
			}
			if exit != want {
				t.Fatalf("cmd fast path: exit %d want %d stderr %q", exit, want, stderr)
			}
			if !present {
				agExit, agStdout, agStderr := f.dispatch(t, []string{"PATH=" + stub, "AGX_TEST_BASH_MARKER=" + marker, "AGX_NO_FETCH=1"}, "antigravity", "pre-tool")
				if agExit != 0 || !strings.Contains(agStderr, "agx: guard binary missing - failing closed") {
					t.Fatalf("cmd antigravity missing binary: exit %d stdout %q stderr %q", agExit, agStdout, agStderr)
				}
				assertAntigravityFailClosedJSON(t, agStdout, "agx: guard binary missing - failing closed")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("cmd attempted Bash: %v", err)
			}
		})
	}
}

func linkVerificationTools(t *testing.T, dir string) {
	t.Helper()
	hashTool := "sha256sum"
	if _, err := exec.LookPath(hashTool); err != nil {
		hashTool = "shasum"
	}
	for _, tool := range []string{hashTool, "awk", "sed"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(dir, tool)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDispatcherProvisionWithoutBash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX sh")
	}
	f := newFetchFixture(t)
	cmd := exec.Command("/bin/sh", filepath.Join(f.hooksDir, "run-hook.cmd"), "--provision-only")
	cmd.Env = []string{"PATH=" + t.TempDir()}
	var out, diag bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &diag
	if err := cmd.Run(); err == nil || out.Len() != 0 || !strings.Contains(diag.String(), "provision") {
		t.Fatalf("missing Bash: %v stdout %q stderr %q", err, out.String(), diag.String())
	}
}
