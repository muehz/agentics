package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hookcost"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

const eventProcessProbeSource = `package main
import("errors";"fmt";"os";"os/exec")
func main(){os.Exit(forward())}
func forward()int{
 countPath:=os.Getenv("AGX_EVENT_TEST_COUNT");binaryPath:=os.Getenv("AGX_EVENT_TEST_REAL_BINARY")
 if countPath==""||binaryPath==""{fmt.Fprintln(os.Stderr,"event probe: missing invocation-owned paths");return 127}
 count,err:=os.OpenFile(countPath,os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600)
 if err!=nil{fmt.Fprintln(os.Stderr,"event probe: cannot open count file");return 127}
 _,writeErr:=count.WriteString("start\n");closeErr:=count.Close()
 if writeErr!=nil||closeErr!=nil{fmt.Fprintln(os.Stderr,"event probe: cannot record invocation");return 127}
 command:=exec.Command(binaryPath,os.Args[1:]...);command.Env=os.Environ();command.Stdin,command.Stdout,command.Stderr=os.Stdin,os.Stdout,os.Stderr
 err=command.Run();if err==nil{return 0}
 var exitErr *exec.ExitError;if errors.As(err,&exitErr)&&exitErr.ExitCode()>=0{return exitErr.ExitCode()}
 fmt.Fprintln(os.Stderr,"event probe: real binary could not complete");return 127
}
`

func eventProcessEnv(values map[string]string) []string {
	remove := map[string]bool{"NODEFAULTCURRENTDIRECTORYINEXEPATH": true, "AGX_DEBUG": true, "AGX_RELEASE_URL": true, "AGX_EVENT_TEST_COUNT": true, "AGX_EVENT_TEST_REAL_BINARY": true}
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS"} {
		remove[key] = true
	}
	for _, key := range hosttest.GuardEnv {
		remove[strings.ToUpper(key)] = true
	}
	for key := range values {
		remove[strings.ToUpper(key)] = true
	}
	var env []string
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !remove[strings.ToUpper(key)] {
			env = append(env, value)
		}
	}
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}

func eventProcessBuild(t testing.TB, dir, output string, args ...string) {
	t.Helper()
	command := exec.Command("go", append([]string{"build", "-trimpath", "-buildvcs=false", "-o", output}, args...)...)
	command.Dir = dir
	command.Env = eventProcessEnv(map[string]string{"CGO_ENABLED": "0", "GOAMD64": "v1", "GOARM64": "v8.0", "GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH, "GOTOOLCHAIN": "go1.27.4"})
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native build: %v\n%s", err, out)
	}
}

func eventProcessExecutable(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func eventProcessProbe(t testing.TB, dir string) string {
	t.Helper()
	source := filepath.Join(dir, "probe.go")
	if err := os.WriteFile(source, []byte(eventProcessProbeSource), 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, eventProcessExecutable("probe"))
	eventProcessBuild(t, dir, output, source)
	return output
}

func eventProcessRun(t testing.TB, command *exec.Cmd, input string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command.Stdin, command.Stdout, command.Stderr = strings.NewReader(input), &stdout, &stderr
	err := command.Run()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() < 0 {
			t.Fatalf("process did not complete normally: %v; stderr=%q", err, stderr.String())
		}
		code = exit.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

func eventProcessCount(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if len(raw) == 0 {
		return 0, nil
	}
	lines := strings.Split(string(raw), "\n")
	if lines[len(lines)-1] != "" {
		return 0, fmt.Errorf("incomplete count line")
	}
	for _, line := range lines[:len(lines)-1] {
		if line != "start" {
			return 0, fmt.Errorf("malformed count line %q", line)
		}
	}
	return len(lines) - 1, nil
}

func eventProcessHarnessError(stdout, stderr string) error {
	if strings.Contains(stdout, "event probe:") || strings.Contains(stderr, "event probe:") {
		return errors.New("invocation probe failed; no Agentics result may be credited")
	}
	return nil
}

func TestEventProcessProbe(t *testing.T) {
	dir := t.TempDir()
	probe := eventProcessProbe(t, dir)
	childSource := filepath.Join(dir, "child.go")
	const child = `package main
import("fmt";"io";"os";"strconv")
func main(){b,_:=io.ReadAll(os.Stdin);fmt.Printf("argv=%q env=%q stdin=%q",os.Args[2:],os.Getenv("AGX_EVENT_CHILD"),b);fmt.Fprint(os.Stderr,"child stderr\n");n,_:=strconv.Atoi(os.Args[1]);os.Exit(n)}`
	if err := os.WriteFile(childSource, []byte(child), 0600); err != nil {
		t.Fatal(err)
	}
	childPath := filepath.Join(dir, eventProcessExecutable("child"))
	eventProcessBuild(t, dir, childPath, childSource)
	for _, status := range []int{0, 2} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			count := filepath.Join(t.TempDir(), "count")
			command := exec.Command(probe, fmt.Sprint(status), "argument with spaces", "quote\"value", "")
			command.Env = eventProcessEnv(map[string]string{"AGX_EVENT_TEST_COUNT": count, "AGX_EVENT_TEST_REAL_BINARY": childPath, "AGX_EVENT_CHILD": "environment with spaces"})
			code, out, diag := eventProcessRun(t, command, "stdin\nwith bytes\x00")
			want := fmt.Sprintf("argv=%q env=%q stdin=%q", []string{"argument with spaces", "quote\"value", ""}, "environment with spaces", []byte("stdin\nwith bytes\x00"))
			if code != status || out != want || diag != "child stderr\n" {
				t.Fatalf("forwarding changed: exit=%d stdout=%q stderr=%q want=%q", code, out, diag, want)
			}
			if n, err := eventProcessCount(count); err != nil || n != 1 {
				t.Fatalf("native count=%d err=%v", n, err)
			}
		})
	}
	for _, failure := range []string{"missing paths", "bad count", "missing child"} {
		t.Run(failure, func(t *testing.T) {
			values := map[string]string{"AGX_EVENT_TEST_COUNT": filepath.Join(t.TempDir(), "count"), "AGX_EVENT_TEST_REAL_BINARY": childPath}
			switch failure {
			case "missing paths":
				values["AGX_EVENT_TEST_COUNT"] = ""
			case "bad count":
				values["AGX_EVENT_TEST_COUNT"] = t.TempDir()
			case "missing child":
				values["AGX_EVENT_TEST_REAL_BINARY"] = filepath.Join(t.TempDir(), "absent")
			}
			command := exec.Command(probe, "0")
			command.Env = eventProcessEnv(values)
			code, out, diag := eventProcessRun(t, command, "")
			if code != 127 || out != "" || !strings.HasPrefix(diag, "event probe:") || eventProcessHarnessError(out, diag) == nil {
				t.Fatalf("probe failure became a decision: exit=%d out=%q stderr=%q", code, out, diag)
			}
		})
	}
	for _, malformed := range []string{"start", "other\n"} {
		path := filepath.Join(t.TempDir(), "count")
		os.WriteFile(path, []byte(malformed), 0600)
		if _, err := eventProcessCount(path); err == nil {
			t.Fatalf("accepted malformed count %q", malformed)
		}
	}
}

type eventProcessFixture struct {
	plugin, real, probe, count, root string
	env                              []string
	// regs are read from the copied plugin's configs, so the commands the
	// test starts are the ones beside the wrapper it replaced.
	regs []hosttest.Registration
}

func eventProcessSetup(t testing.TB) *eventProcessFixture {
	t.Helper()
	if (runtime.GOOS != "windows" && runtime.GOOS != "linux" && runtime.GOOS != "darwin") || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skipf("native wrapper target unsupported: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" && (strings.EqualFold(os.Getenv("PROCESSOR_ARCHITECTURE"), "ARM64") || strings.EqualFold(os.Getenv("PROCESSOR_ARCHITEW6432"), "ARM64") || strings.HasPrefix(os.Getenv("PROCESSOR_IDENTIFIER"), "ARMv8")) {
		t.Skip("wrapper selects native ARM64; amd64 test process cannot build a native forwarding child")
	}
	repo, err := filepath.Abs(hosttest.Checkout(t))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	plugin := filepath.Join(dir, "plugin with spaces")
	if err = os.MkdirAll(filepath.Join(plugin, "hooks", "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"hooks/run-hook.cmd", "hooks/hooks.claude.json", "hooks/hooks.codex.json", "hooks/hooks.copilot.json", "hooks.json"} {
		raw, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(plugin, filepath.FromSlash(rel)), raw, 0700); err != nil {
			t.Fatal(err)
		}
	}
	module := filepath.Join(repo, "hooks", "go")
	digest, err := hookprovenance.SourceDigest(module)
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(dir, eventProcessExecutable("real-agentics"))
	flags := "-s -w -X main.sourceDigest=agentics-src-" + digest
	eventProcessBuild(t, module, real, "-ldflags", flags, "./cmd/hook")
	command := exec.Command(real, "--source-digest")
	command.Env = eventProcessEnv(map[string]string{printDigestEnv: "1"})
	code, out, diag := eventProcessRun(t, command, "")
	if code != 0 || out != "agentics-src-"+digest+"\nbuilt-by go1.27.4\n" || diag != "" {
		t.Fatalf("scratch provenance exit=%d out=%q stderr=%q", code, out, diag)
	}
	raw, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("scratch source=agentics-src-%s binary-sha256=%x toolchain=go1.27.4 target=%s/%s CGO_ENABLED=0 GOAMD64=v1 GOARM64=v8.0 flags=-trimpath -buildvcs=false -ldflags %q; provider selection simulated; configured commands/wrapper/native child real", digest, sha256.Sum256(raw), runtime.GOOS, runtime.GOARCH, flags)
	probe := eventProcessProbe(t, dir)
	native, _ := hosttest.CommittedBinaryPath(filepath.Join(plugin, "hooks"))
	raw, err = os.ReadFile(probe)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(native, raw, 0700); err != nil {
		t.Fatal(err)
	}
	stageFixtureProvisioning(t, native)
	regs, err := hosttest.Registrations(filepath.Join(plugin, "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	return &eventProcessFixture{plugin: plugin, real: real, probe: native, root: dir, regs: regs}
}

func (f *eventProcessFixture) selected(t testing.TB, provider surface.ID, tool string) []hosttest.Registration {
	t.Helper()
	regs, err := hosttest.Selected(f.regs, provider, tool, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	return regs
}

func (f *eventProcessFixture) first(t testing.TB, provider surface.ID, tool string) hosttest.Registration {
	t.Helper()
	regs := f.selected(t, provider, tool)
	if len(regs) == 0 {
		t.Fatalf("no pre-tool registration selected for provider=%s tool=%s", provider, tool)
	}
	return regs[0]
}

func (f *eventProcessFixture) environment(t testing.TB, root string) []string {
	t.Helper()
	f.count = filepath.Join(t.TempDir(), "count")
	values := map[string]string{"AGX_NO_FETCH": "1", "AGX_EVENT_TEST_COUNT": f.count, "AGX_EVENT_TEST_REAL_BINARY": f.real, "CLAUDE_PLUGIN_ROOT": f.plugin, "COPILOT_PLUGIN_ROOT": f.plugin, "PLUGIN_ROOT": f.plugin, "COMMAND_GUARD_LOG_DIR": t.TempDir(), "RELEASE_GUARD_LOG_DIR": t.TempDir(), "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": filepath.Join(f.root, "absent-gitconfig")}
	f.env = eventProcessEnv(values)
	return f.env
}

func (f *eventProcessFixture) repo(t testing.TB, worktree string) string {
	t.Helper()
	root := t.TempDir()
	env := f.environment(t, root)
	git := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-c", "user.name=Agentics Process Fixture", "-c", "user.email=agentics-process@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + filepath.Join(root, "absent-hooks"), "-C", root}, args...)...)
		command.Env = env
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("fixture git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(root, "probe.txt"), []byte("safe\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "--", "probe.txt")
	git("commit", "-qm", "fixture baseline")
	if worktree != "" {
		if err := os.WriteFile(filepath.Join(root, "probe.txt"), []byte(worktree+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func (f *eventProcessFixture) invoke(t testing.TB, registration hosttest.Registration, payload string) (int, string, string) {
	t.Helper()
	name, args, err := registration.HostCommand(runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	// Every host shell a shipped command needs is required, so a missing one
	// fails rather than skips.
	shell, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("host shell %s for %s %s: %v", name, registration.Surface, registration.Field, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), registration.Timeout)
	defer cancel()
	// hookcost.Process passes cmd.exe's command string unescaped and cancels
	// the whole process tree, which a plain argv launch cannot do.
	command := hookcost.Process(ctx, shell, args)
	command.WaitDelay = time.Second
	command.Env = f.env
	command.Dir = f.root
	if registration.Surface == surface.Antigravity {
		command.Dir = f.plugin
	}
	code, out, diag := eventProcessRun(t, command, payload)
	t.Logf("configured provider=%s shell=%q command=%q exit=%d stdout=%q stderr=%q", registration.Surface, shell, registration.Command, code, out, diag)
	return code, out, diag
}

func (f *eventProcessFixture) replay(t testing.TB, provider surface.ID, event, tool, payload string) (int, string, string, int, error) {
	t.Helper()
	if event != "PreToolUse" {
		t.Fatalf("replay selects pre-tool registrations only, not event %q", event)
	}
	var stdout, stderr strings.Builder
	code := 0
	for _, registration := range f.selected(t, provider, tool) {
		status, out, diag := f.invoke(t, registration, payload)
		if err := eventProcessHarnessError(out, diag); err != nil {
			return status, out, diag, 0, fmt.Errorf("%w: exit=%d stdout=%q stderr=%q", err, status, out, diag)
		}
		stdout.WriteString(out)
		stderr.WriteString(diag)
		if status != 0 {
			code = status
		}
	}
	n, err := eventProcessCount(f.count)
	if err != nil {
		return code, stdout.String(), stderr.String(), 0, err
	}
	t.Logf("observed provider=%s event=%s tool=%s total Agentics starts=%d", provider, event, tool, n)
	return code, stdout.String(), stderr.String(), n, nil
}

func TestEventProcessContract(t *testing.T) {
	f := eventProcessSetup(t)
	for _, provider := range surface.All() {
		t.Run(string(provider), func(t *testing.T) {
			shell, read := hosttest.NativeTool(provider, "Bash"), hosttest.NativeTool(provider, "Read")
			t.Run("safe-shell-command", func(t *testing.T) {
				root := f.repo(t, "")
				f.environment(t, root)
				payload := string(hosttest.Stdin(provider, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "echo safe"}, Cwd: root}))
				code, out, diag, n, err := f.replay(t, provider, "PreToolUse", shell, payload)
				if err != nil {
					t.Fatal(err)
				}
				if code != 0 || diag != "" || strings.Contains(out, "deny") {
					t.Errorf("safe shell event changed: exit=%d starts=%d stdout=%q stderr=%q", code, n, out, diag)
				}
				if n != 1 {
					t.Fatalf("one process per shell event: observed starts=%d want=1", n)
				}
			})
			t.Run("credential-read", func(t *testing.T) {
				root := f.repo(t, "")
				f.environment(t, root)
				payload := string(hosttest.Stdin(provider, hosttest.Call{Tool: "Read", Input: map[string]any{"file_path": filepath.Join(root, ".env")}, Cwd: root}))
				code, out, diag, n, err := f.replay(t, provider, "PreToolUse", read, payload)
				if err != nil {
					t.Fatal(err)
				}
				reason := requireDeny(t, provider, code, out, diag)
				if n != 1 || !strings.Contains(strings.ToLower(reason), "secret") {
					t.Fatalf("credential routing: starts=%d reason=%q", n, reason)
				}
			})
			t.Run("command-denial", func(t *testing.T) {
				root := f.repo(t, "")
				f.environment(t, root)
				payload := string(hosttest.Stdin(provider, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "rm -rf /"}, Cwd: root}))
				code, out, diag, n, err := f.replay(t, provider, "PreToolUse", shell, payload)
				if err != nil {
					t.Fatal(err)
				}
				reason := requireDeny(t, provider, code, out, diag)
				if n != 1 || !strings.Contains(reason, "[commands]") {
					t.Fatalf("grouped command denial lost: starts=%d reason=%q", n, reason)
				}
			})
			for _, hook := range []string{"command-guardian", "release-guardian", "secrets-guardian"} {
				t.Run("legacy/"+hook, func(t *testing.T) {
					root := f.repo(t, "")
					f.environment(t, root)
					call := hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "echo safe"}, Cwd: root}
					if hook == "secrets-guardian" {
						call = hosttest.Call{Tool: "Read", Input: map[string]any{"file_path": "probe.txt"}, Cwd: root}
					}
					registration := f.first(t, provider, shell)
					registration.Command = strings.ReplaceAll(registration.Command, " pre-tool", " "+hook+" --mode block")
					code, out, diag := f.invoke(t, registration, string(hosttest.Stdin(provider, call)))
					if err := eventProcessHarnessError(out, diag); err != nil {
						t.Fatal(err)
					}
					n, err := eventProcessCount(f.count)
					want := ""
					switch provider {
					case "codex":
						want = "{}\n"
					case "antigravity":
						want = "{\"decision\":\"allow\"}\n"
					}
					if err != nil || n != 1 || code != 0 || diag != "" || out != want {
						t.Fatalf("legacy forwarding: exit=%d starts=%d err=%v stdout=%q stderr=%q", code, n, err, out, diag)
					}
				})
			}
		})
	}
	t.Run("harness-failure", func(t *testing.T) {
		f.environment(t, f.root)
		f.env = append(f.env, "AGX_EVENT_TEST_REAL_BINARY="+filepath.Join(f.root, "missing-real-agentics"))
		code, out, diag, _, err := f.replay(t, "claude-code", "PreToolUse", "Bash", string(hosttest.Stdin("claude-code", hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "echo safe"}, Cwd: f.root})))
		want := 127
		if runtime.GOOS == "windows" {
			want = 2
		}
		if code != want || err == nil || !strings.Contains(err.Error(), "invocation probe failed") || !strings.Contains(diag, "event probe: real binary could not complete") {
			t.Fatalf("harness failure credited by replay: exit=%d want=%d err=%v stdout=%q stderr=%q", code, want, err, out, diag)
		}
		if n, err := eventProcessCount(f.count); err != nil || n != 1 {
			t.Fatalf("harness control did not reach forwarding attempt: starts=%d err=%v", n, err)
		}
	})
	for _, fault := range []string{"missing", "unexecutable", "no-execute-bit"} {
		t.Run("wrapper/"+fault, func(t *testing.T) {
			if fault == "no-execute-bit" && runtime.GOOS == "windows" {
				t.Skip("native Windows does not use POSIX executable permission bits; bad PE is exercised separately")
			}
			raw, err := os.ReadFile(f.probe)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.WriteFile(f.probe, raw, 0700); err != nil {
					t.Error(err)
				}
				if err := os.Chmod(f.probe, 0700); err != nil {
					t.Error(err)
				}
				fixtureRuntimeDigest(t, f.probe)
			}()
			if err = os.Remove(f.probe); err != nil {
				t.Fatal(err)
			}
			if fault == "unexecutable" || fault == "no-execute-bit" {
				body := []byte("#!/missing/agentics-test-interpreter\n")
				mode := os.FileMode(0700)
				if fault == "no-execute-bit" {
					mode = 0600
				}
				if runtime.GOOS == "windows" {
					body = []byte("invalid native PE")
				}
				if err = os.WriteFile(f.probe, body, mode); err != nil {
					t.Fatal(err)
				}
				fixtureRuntimeDigest(t, f.probe)
			}
			f.environment(t, f.root)
			for _, provider := range surface.All() {
				tool := hosttest.NativeTool(provider, "Bash")
				reg := f.first(t, provider, tool)
				code, out, diag := f.invoke(t, reg, string(hosttest.Stdin(provider, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "echo safe"}, Cwd: f.root})))
				if provider == surface.Antigravity {
					reason := requireDeny(t, provider, code, out, diag)
					if !strings.Contains(reason, "failing closed") || !strings.Contains(diag, "failing closed") {
						t.Fatalf("antigravity wrapper failure posture exit=%d stdout=%q stderr=%q", code, out, diag)
					}
				} else if code != 2 || out != "" || !strings.Contains(diag, "failing closed") {
					t.Fatalf("wrapper failure posture exit=%d want=2 stdout=%q stderr=%q", code, out, diag)
				}
				if n, err := eventProcessCount(f.count); err != nil || n != 0 {
					t.Fatalf("unavailable binary counted=%d err=%v", n, err)
				}
			}
		})
	}
}
