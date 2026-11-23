package hookcost

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// TestMain turns the test binary into a stub hook when a route test launches
// it, so the routes run a real process without a committed hook binary. The
// stub runs before flag parsing, so the probe's argv never reaches the test
// runner, and it exits before any test output can reach its streams.
func TestMain(m *testing.M) {
	if os.Getenv("AGX_BENCH_TEST_PROCESS") == "1" {
		os.Exit(stubHook())
	}
	os.Exit(m.Run())
}

// stubHook checks the working directory and stdin it was given, prints the
// resolution diagnostic when AGX_DEBUG is set, and answers with the
// configured streams and exit code.
func stubHook() int {
	if want := os.Getenv("AGX_BENCH_TEST_CWD"); want != "" {
		got, err := os.Getwd()
		gotInfo, gotErr := os.Stat(got)
		wantInfo, wantErr := os.Stat(want)
		if err != nil || gotErr != nil || wantErr != nil || !os.SameFile(gotInfo, wantInfo) {
			fmt.Fprintf(os.Stderr, "dispatcher working directory: got %q, want %q, error=%v", got, want, err)
			return 3
		}
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil || string(input) != os.Getenv("AGX_BENCH_TEST_INPUT") {
		fmt.Fprintf(os.Stderr, "dispatcher stdin mismatch: %q error=%v", input, err)
		return 4
	}
	if os.Getenv("AGX_DEBUG") != "" {
		fmt.Fprint(os.Stderr, os.Getenv("AGX_BENCH_TEST_DIAGNOSTIC"))
	}
	fmt.Fprint(os.Stdout, os.Getenv("AGX_BENCH_TEST_STDOUT"))
	fmt.Fprint(os.Stderr, os.Getenv("AGX_BENCH_TEST_STDERR"))
	code, _ := strconv.Atoi(os.Getenv("AGX_BENCH_TEST_EXIT"))
	return code
}

const stubInput = "safe input"

func stubProbe() Probe {
	return Probe{Name: "command-guardian", Argv: []string{"claude-code", "command-guardian", "--mode", "block"}, Stdin: stubInput, Timeout: 10 * time.Second}
}

// stubEnv is the stub hook's environment: the test process's own plus the
// stub's switches.
func stubEnv(extra ...string) []string {
	return append(append(os.Environ(), "AGX_BENCH_TEST_PROCESS=1", "AGX_BENCH_TEST_INPUT="+stubInput), extra...)
}

// reference answers every probe with code, stdout, and stderr.
func reference(code int, stdout, stderr string) RunFunc {
	return func(_ []string, stdin io.Reader, out, errw io.Writer) int {
		_, _ = io.Copy(io.Discard, stdin)
		fmt.Fprint(out, stdout)
		fmt.Fprint(errw, stderr)
		return code
	}
}

func testExecutable(t *testing.T) string {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return binary
}

// stubTarget writes a run-hook.cmd that runs the test executable into a
// hooks directory whose name has spaces, as the shipped path can.
func stubTarget(t *testing.T) Target {
	t.Helper()
	binary := testExecutable(t)
	dir := filepath.Join(t.TempDir(), "hooks with spaces")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "exec \"" + binary + "\" \"$@\"\n"
	if runtime.GOOS == "windows" {
		// A bare exit /b leaves a cmd /c process's exit code at 0, so the
		// status is passed on from a top-level line, as run-hook.cmd does.
		script = "@echo off\r\n\"" + binary + "\" %*\r\nexit /b %ERRORLEVEL%\r\n"
	}
	dispatcher := filepath.Join(dir, "run-hook.cmd")
	if err := os.WriteFile(dispatcher, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return Target{HooksDir: dir, Binary: binary, Dispatcher: dispatcher}
}

type resolutionRow struct {
	name, stderr, rejection string
}

// resolutionRows are the dispatcher diagnostics a resolution check accepts
// (empty rejection) or rejects. The case-changed row depends on whether the
// host's paths fold case.
func resolutionRows(t *testing.T, binary, cwd string) []resolutionRow {
	t.Helper()
	relative, err := filepath.Rel(cwd, binary)
	if err != nil {
		t.Fatal(err)
	}
	unix := "run-hook resolved binary: "
	windows := "run-hook.cmd resolved binary: "
	wrong := filepath.Join(cwd, "other", filepath.Base(binary))
	cases := []resolutionRow{
		{"relative", unix + relative + "\n", ""},
		{"absolute", unix + binary + "\n", ""},
		{"windows-prefix", windows + filepath.ToSlash(binary) + "\r\n", ""},
		{"same-basename", unix + wrong + "\n", "dispatcher resolution identified"},
		{"path-suffix", unix + binary + "-other\n", "dispatcher resolution identified"},
		{"missing", "unrelated log containing " + binary + "\n", "dispatcher resolution requires exactly one binary path"},
		{"empty", unix + "\n", "dispatcher resolution requires exactly one binary path"},
		{"duplicate", unix + binary + "\n" + unix + binary + "\n", "dispatcher resolution requires exactly one binary path"},
		{"conflicting", unix + binary + "\n" + windows + wrong + "\n", "dispatcher resolution requires exactly one binary path"},
	}
	rejection := "dispatcher resolution identified"
	if runtime.GOOS == "windows" {
		rejection = ""
	}
	return append(cases, resolutionRow{"case-changed", unix + strings.ToUpper(binary) + "\n", rejection})
}

func TestCheckResolution(t *testing.T) {
	binary := testExecutable(t)
	cwd := filepath.Join(t.TempDir(), "hooks with spaces")
	rows := resolutionRows(t, binary, cwd)
	// A hook's debug stderr can be long; the scan must stay linear in it.
	rows = append(rows, resolutionRow{"100KB", strings.Repeat("run-hook noise line without the marker\n", 100*1024/39+1), "dispatcher resolution requires exactly one binary path"})
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			start := time.Now()
			err := CheckResolution(row.stderr, cwd, binary)
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Fatalf("resolution check took %s", elapsed)
			}
			if row.rejection == "" {
				if err != nil {
					t.Fatalf("correct resolution rejected: %v", err)
				}
			} else if err == nil || !strings.HasPrefix(err.Error(), row.rejection) {
				t.Fatalf("got %v, want %q", err, row.rejection)
			}
		})
	}
}

func TestDispatcherPrepare(t *testing.T) {
	target := stubTarget(t)
	fixture := t.TempDir()
	for _, row := range resolutionRows(t, target.Binary, target.HooksDir) {
		t.Run(row.name, func(t *testing.T) {
			route := Dispatcher(target, fixture, stubEnv("AGX_BENCH_TEST_DIAGNOSTIC="+row.stderr), reference(0, "", ""))
			err := route.Prepare(stubProbe())
			if row.rejection == "" {
				if err != nil {
					t.Fatalf("correct resolution rejected: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "resolution verification: "+row.rejection) {
				t.Fatalf("wrong resolution was not distinctly rejected: got %v, want %q", err, row.rejection)
			}
		})
	}
}

func TestDispatcherDebugRun(t *testing.T) {
	target := stubTarget(t)
	fixture := t.TempDir()
	diagnostic := "AGX_BENCH_TEST_DIAGNOSTIC=run-hook resolved binary: " + target.Binary + "\n"
	t.Run("stdout differs from the reference", func(t *testing.T) {
		route := Dispatcher(target, fixture, stubEnv(diagnostic, "AGX_BENCH_TEST_STDOUT=chatter"), reference(0, "", ""))
		err := route.Prepare(stubProbe())
		if err == nil || !strings.HasPrefix(err.Error(), "resolution verification: ") || !strings.Contains(err.Error(), `"chatter"`) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("exit differs from the reference", func(t *testing.T) {
		route := Dispatcher(target, fixture, stubEnv(diagnostic, "AGX_BENCH_TEST_EXIT=2"), reference(0, "", ""))
		err := route.Prepare(stubProbe())
		if err == nil || !strings.HasPrefix(err.Error(), "resolution verification: ") || !strings.Contains(err.Error(), "stdout=") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("missing dispatcher", func(t *testing.T) {
		missing := target
		missing.HooksDir = t.TempDir()
		missing.Dispatcher = filepath.Join(missing.HooksDir, "run-hook.cmd")
		route := Dispatcher(missing, fixture, stubEnv(diagnostic), reference(0, "", ""))
		if err := route.Prepare(stubProbe()); err == nil || !strings.HasPrefix(err.Error(), "resolution verification: ") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("no shell on PATH", func(t *testing.T) {
		t.Setenv("PATH", "")
		route := Dispatcher(target, fixture, stubEnv(diagnostic), reference(0, "", ""))
		if err := route.Prepare(stubProbe()); err == nil || !strings.HasPrefix(err.Error(), "find ") {
			t.Fatalf("got %v", err)
		}
		if _, err := route.Invoke(stubProbe()); err == nil || !strings.HasPrefix(err.Error(), "find ") {
			t.Fatalf("got %v", err)
		}
	})
}

func TestRouteWorkingDir(t *testing.T) {
	target := stubTarget(t)
	fixture := t.TempDir()
	diagnostic := "AGX_BENCH_TEST_DIAGNOSTIC=run-hook resolved binary: " + target.Binary + "\n"
	cases := []struct {
		name   string
		route  func(env []string) *LaunchRoute
		cwd    string
		record string
	}{
		{"dispatcher", func(env []string) *LaunchRoute { return Dispatcher(target, fixture, env, reference(0, "", "")) }, target.HooksDir, target.HooksDir},
		{"native", func(env []string) *LaunchRoute { return Native(target, fixture, env, reference(0, "", "")) }, fixture, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route := tc.route(stubEnv(diagnostic, "AGX_BENCH_TEST_CWD="+tc.cwd))
			if err := route.Prepare(stubProbe()); err != nil {
				t.Fatal(err)
			}
			if _, err := route.Invoke(stubProbe()); err != nil {
				t.Fatalf("launch did not run in %s: %v", tc.cwd, err)
			}
			if got := route.Command(stubProbe()).WorkingDir; got != tc.record {
				t.Fatalf("recorded working directory %q, want %q", got, tc.record)
			}
			wrong := tc.route(stubEnv(diagnostic, "AGX_BENCH_TEST_CWD="+t.TempDir()))
			if _, err := wrong.Invoke(stubProbe()); err == nil || !strings.Contains(err.Error(), "dispatcher working directory") {
				t.Fatalf("working directory check passed in the wrong directory: %v", err)
			}
		})
	}
}

func TestRouteCommand(t *testing.T) {
	target := Target{HooksDir: filepath.Join("x", "hooks"), Binary: filepath.Join("x", "hooks", "bin", "hook"), Dispatcher: filepath.Join("x", "hooks", "run-hook.cmd")}
	p := Probe{Name: "n", Argv: []string{"claude-code", "command-guardian"}, Stdin: "{}", Timeout: 1500 * time.Millisecond}
	native := Native(target, "fixture", nil, reference(0, "", ""))
	if got, want := native.Command(p), (Command{Executable: target.Binary, Args: p.Argv, Payload: "{}", TimeoutSeconds: 2}); !reflect.DeepEqual(got, want) {
		t.Fatalf("native command = %+v, want %+v", got, want)
	}
	if native.Name() != "native" {
		t.Fatalf("name %q", native.Name())
	}
	dispatcher := Dispatcher(target, "fixture", nil, reference(0, "", ""))
	got := dispatcher.Command(p)
	wantArgs := append([]string{"run-hook.cmd"}, p.Argv...)
	if runtime.GOOS == "windows" {
		wantArgs = append([]string{"/d", "/c", target.Dispatcher}, p.Argv...)
	}
	if !filepath.IsAbs(got.Executable) || !reflect.DeepEqual(got.Args, wantArgs) || got.WorkingDir != target.HooksDir || got.Payload != "{}" || got.TimeoutSeconds != 2 {
		t.Fatalf("dispatcher command = %+v", got)
	}
	if dispatcher.Name() != "dispatcher" {
		t.Fatalf("name %q", dispatcher.Name())
	}
}

func TestLaunchInvoke(t *testing.T) {
	target := stubTarget(t)
	fixture := t.TempDir()
	cases := []struct {
		name string
		env  []string
		want string
	}{
		{"same result", nil, ""},
		{"changed exit", []string{"AGX_BENCH_TEST_EXIT=1"}, `exit status 1 stdout="" stderr=""`},
		{"changed stdout", []string{"AGX_BENCH_TEST_STDOUT=deny"}, `output mismatch stdout="deny" stderr=""`},
		{"changed stderr", []string{"AGX_BENCH_TEST_STDERR=warn"}, `output mismatch stdout="" stderr="warn"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route := Native(target, fixture, stubEnv(tc.env...), reference(0, "", ""))
			if err := route.Prepare(stubProbe()); err != nil {
				t.Fatal(err)
			}
			ns, err := route.Invoke(stubProbe())
			if tc.want == "" {
				if err != nil || ns <= 0 {
					t.Fatalf("got %s, %v", ns, err)
				}
				return
			}
			if err == nil || err.Error() != tc.want {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLaunchPrepare(t *testing.T) {
	target := stubTarget(t)
	fixture := t.TempDir()
	t.Run("rejected reference", func(t *testing.T) {
		err := Native(target, fixture, stubEnv(), reference(2, "", "no")).Prepare(stubProbe())
		if want := `reference input rejected: exit=2 stderr="no"`; err == nil || err.Error() != want {
			t.Fatalf("got %v, want %q", err, want)
		}
	})
	t.Run("zero timeout", func(t *testing.T) {
		p := stubProbe()
		p.Timeout = 0
		route := Native(target, fixture, stubEnv(), reference(0, "", ""))
		if err := route.Prepare(p); err == nil || !strings.Contains(err.Error(), "timeout") {
			t.Fatalf("got %v", err)
		}
		if _, err := route.Invoke(p); err == nil || !strings.Contains(err.Error(), "timeout") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("missing binary", func(t *testing.T) {
		missing := target
		missing.Binary = filepath.Join(t.TempDir(), "absent")
		route := Native(missing, fixture, stubEnv(), reference(0, "", ""))
		if _, err := route.Invoke(stubProbe()); err == nil || !strings.HasPrefix(err.Error(), "measured binary unavailable (fail-open output is not evidence): ") {
			t.Fatalf("got %v", err)
		}
		dispatcher := Dispatcher(missing, fixture, stubEnv(), reference(0, "", ""))
		if err := dispatcher.Prepare(stubProbe()); err == nil || !strings.HasPrefix(err.Error(), "measured binary unavailable (fail-open output is not evidence): ") {
			t.Fatalf("got %v", err)
		}
	})
}

// A cold call runs before Prepare, so its result is kept and judged when the
// reference is recorded afterwards.
func TestLaunchColdPath(t *testing.T) {
	target := stubTarget(t)
	fixture := t.TempDir()
	t.Run("matching", func(t *testing.T) {
		route := Native(target, fixture, stubEnv(), reference(0, "", ""))
		if _, err := route.Invoke(stubProbe()); err != nil {
			t.Fatal(err)
		}
		if err := route.Prepare(stubProbe()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("mismatch", func(t *testing.T) {
		route := Native(target, fixture, stubEnv("AGX_BENCH_TEST_STDOUT=deny"), reference(0, "", ""))
		if _, err := route.Invoke(stubProbe()); err != nil {
			t.Fatalf("an unprepared call compared its result: %v", err)
		}
		err := route.Prepare(stubProbe())
		if want := `first-call output mismatch stdout="deny" stderr=""`; err == nil || err.Error() != want {
			t.Fatalf("got %v, want %q", err, want)
		}
	})
}

func TestLaunchArm(t *testing.T) {
	target := stubTarget(t)
	fixture := t.TempDir()
	c := Config{Provider: "claude-code", Hook: "command-guardian"}
	t.Run("cold mismatch is rejected evidence", func(t *testing.T) {
		route := Native(target, fixture, stubEnv("AGX_BENCH_TEST_STDOUT=deny"), reference(0, "", ""))
		arm := LaunchArm("p/native/h", "native-launch", route, stubProbe(), c)
		records := Measure([]Arm{arm}, true, 1, nil)
		if want := `first-call output mismatch stdout="deny" stderr=""`; records[0].Error != want || len(records[0].SamplesNS) != 0 || len(records[0].RejectedSamplesNS) != 1 {
			t.Fatalf("got %+v", records[0])
		}
	})
	t.Run("warm", func(t *testing.T) {
		route := Native(target, fixture, stubEnv(), reference(0, "", ""))
		arm := LaunchArm("p/native/h", "native-launch", route, stubProbe(), c)
		records := Measure([]Arm{arm}, false, 2, nil)
		if records[0].Error != "" || len(records[0].SamplesNS) != 2 || !reflect.DeepEqual(records[0].Commands, []Command{route.Command(stubProbe())}) || records[0].Layer != "native-launch" {
			t.Fatalf("got %+v", records[0])
		}
		if arm.Config == nil || !reflect.DeepEqual(*arm.Config, c) || arm.Spans() != nil {
			t.Fatalf("arm = %+v", arm)
		}
	})
	t.Run("error texts", func(t *testing.T) {
		route := Native(target, fixture, stubEnv("AGX_BENCH_TEST_STDOUT=deny"), reference(0, "", ""))
		arm := LaunchArm("p/native/h", "native-launch", route, stubProbe(), c)
		if err := arm.Preflight(); err != nil {
			t.Fatal(err)
		}
		if _, err := arm.Run(); err == nil || err.Error() != `p/native/h: output mismatch stdout="deny" stderr=""` {
			t.Fatalf("got %v", err)
		}
		failing := LaunchArm("p/native/h", "native-launch", Native(target, fixture, stubEnv("AGX_BENCH_TEST_EXIT=1"), reference(0, "", "")), stubProbe(), c)
		if _, err := failing.Run(); err == nil || err.Error() != `p/native/h: exit status 1 stdout="" stderr=""` {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("missing binary", func(t *testing.T) {
		missing := target
		missing.Binary = filepath.Join(t.TempDir(), "absent")
		arm := LaunchArm("missing", "config-command-replay", Native(missing, fixture, stubEnv(), reference(0, "", "")), stubProbe(), c)
		records := Measure([]Arm{arm}, true, 1, nil)
		if !strings.HasPrefix(records[0].Error, "measured binary unavailable (fail-open output is not evidence)") || len(records[0].SamplesNS) != 0 {
			t.Fatalf("accepted missing fail-open binary: %+v", records[0])
		}
	})
}

func TestConfigured(t *testing.T) {
	target := stubTarget(t)
	fixture := t.TempDir()
	command := `"` + target.Binary + `" claude-code command-guardian`
	diagnostic := "AGX_BENCH_TEST_DIAGNOSTIC=run-hook resolved binary: " + target.Binary + "\n"
	cases := []struct {
		name    string
		surface surface.ID
		cwd     string
	}{
		{"fixture", surface.ClaudeCode, fixture},
		{"antigravity plugin root", surface.Antigravity, filepath.Dir(target.HooksDir)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := hosttest.Registration{Surface: tc.surface, Event: "PreToolUse", Field: "command", Command: command, Hook: "command-guardian", Timeout: 10 * time.Second}
			route, err := Configured(r, target, fixture, stubEnv(diagnostic, "AGX_BENCH_TEST_CWD="+tc.cwd), reference(0, "", ""))
			if err != nil {
				t.Fatal(err)
			}
			if err := route.Prepare(stubProbe()); err != nil {
				t.Fatal(err)
			}
			if _, err := route.Invoke(stubProbe()); err != nil {
				t.Fatal(err)
			}
			got := route.Command(stubProbe())
			if route.Name() != "config" || got.RawConfig != command || got.Payload != stubInput || got.TimeoutSeconds != 10 {
				t.Fatalf("config command = %+v", got)
			}
		})
	}
	t.Run("empty command", func(t *testing.T) {
		r := hosttest.Registration{Surface: surface.ClaudeCode, Field: "command", Timeout: time.Second}
		if route, err := Configured(r, target, fixture, nil, reference(0, "", "")); err == nil || route != nil {
			t.Fatalf("got %v, %v", route, err)
		}
	})
}

func TestInProcess(t *testing.T) {
	calls := 0
	outputs := []string{"deny", "deny", "deny", "allow"}
	run := func(_ []string, _ io.Reader, stdout, _ io.Writer) int {
		fmt.Fprint(stdout, outputs[calls])
		calls++
		return 2
	}
	route := InProcess(run)
	if route.Name() != "in-process" {
		t.Fatalf("name %q", route.Name())
	}
	if err := route.Prepare(stubProbe()); err != nil {
		t.Fatalf("a denying first result was refused: %v", err)
	}
	for k := 0; k < 2; k++ {
		if _, err := route.Invoke(stubProbe()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := route.Invoke(stubProbe()); err == nil || err.Error() != `output mismatch exit=2 stdout="allow" stderr=""` {
		t.Fatalf("got %v", err)
	}
	unprepared := InProcess(reference(0, "x", ""))
	if _, err := unprepared.Invoke(stubProbe()); err != nil {
		t.Fatalf("the first result is the reference: %v", err)
	}
	if _, err := unprepared.Invoke(stubProbe()); err != nil {
		t.Fatal(err)
	}
}

func TestSamples(t *testing.T) {
	calls := 0
	run := func(_ []string, _ io.Reader, stdout, _ io.Writer) int {
		calls++
		return 0
	}
	samples, err := Samples(InProcess(run), stubProbe(), 2, 3)
	if err != nil || len(samples) != 3 || calls != 6 {
		t.Fatalf("got %v, %v after %d calls", samples, err, calls)
	}
	calls = 0
	changing := func(_ []string, _ io.Reader, stdout, _ io.Writer) int {
		calls++
		if calls == 4 {
			fmt.Fprint(stdout, "changed")
		}
		return 0
	}
	if samples, err := Samples(InProcess(changing), stubProbe(), 1, 5); samples != nil || err == nil {
		t.Fatalf("got %v, %v", samples, err)
	}
	if samples, err := Samples(Native(Target{}, "", nil, reference(1, "", "")), stubProbe(), 0, 1); samples != nil || err == nil {
		t.Fatalf("a failed Prepare returned %v, %v", samples, err)
	}
	if samples, err := Samples(InProcess(changing), stubProbe(), 0, 0); err != nil || len(samples) != 0 {
		t.Fatalf("got %v, %v", samples, err)
	}
}
