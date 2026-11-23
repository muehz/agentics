package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hookcost"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// BenchmarkPackagedHook measures the process boundary users actually pay for:
// the native binary, and the dispatcher that selects it. It runs under the
// environment and fixture the hook cost capture prepares, so its numbers
// compare with a capture's; the routes inherit that environment from the
// process. AGX_BENCH_HOOKS_DIR selects a scratch hooks directory; the
// default is the committed payload.
func BenchmarkPackagedHook(b *testing.B) {
	checkout, err := filepath.Abs(hosttest.Checkout(b))
	if err != nil {
		b.Fatal(err)
	}
	target, err := hookcost.ResolveTarget(checkout, os.Getenv)
	if errors.Is(err, hookcost.ErrOffContract) {
		b.Skipf("no packaged hook binary for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if err != nil {
		b.Fatal(err)
	}
	fixture, _ := packagedHookEnvironment(b, target)

	probe := hookcost.Probe{
		Name:    "command-guardian",
		Argv:    []string{"claude-code", "command-guardian", "--mode", "block"},
		Stdin:   string(hosttest.Stdin(surface.ClaudeCode, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "git status --short"}, Cwd: fixture})),
		Timeout: 25 * time.Second,
	}
	// A route compares each launch with this reference, so a reference that
	// denies the call would let a route that denies it the same way pass.
	var stdout, stderr bytes.Buffer
	code := run(probe.Argv, strings.NewReader(probe.Stdin), &stdout, &stderr)
	if !hosttest.Read(surface.ClaudeCode, code, stdout.Bytes(), stderr.Bytes()).Silent() || stdout.Len() != 0 {
		b.Fatalf("reference hook returned exit=%d stdout=%q stderr=%q, want exit=0 with empty output", code, stdout.String(), stderr.String())
	}

	routes := []hookcost.Route{
		hookcost.Native(target, fixture, nil, run),
		hookcost.Dispatcher(target, fixture, nil, run),
	}
	for _, route := range routes {
		b.Run(route.Name()+"/"+probe.Name, func(b *testing.B) {
			for _, path := range []string{target.Binary, target.Dispatcher} {
				info, err := os.Stat(path)
				if err != nil || !info.Mode().IsRegular() {
					b.Fatalf("packaged hook path %q is not a regular file: %v", path, err)
				}
			}
			if err := route.Prepare(probe); err != nil {
				b.Fatalf("prepare %s: %v", route.Name(), err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := route.Invoke(probe); err != nil {
					b.Fatalf("packaged hook failed: %v", err)
				}
			}
		})
	}
}

// packagedHookEnvironment prepares the process environment and the Git
// fixture the packaged hook is measured in: the capture's own. The process
// environment is set for the rest of the test, so the routes and the
// in-process reference inherit it; env is the same list, for a caller that
// compares it.
func packagedHookEnvironment(tb testing.TB, target hookcost.Target) (fixture string, env []string) {
	tb.Helper()
	return hookcost.Prepare(tb, target)
}

func packagedHookNativeBinary(b testing.TB, hooksDir string) string {
	b.Helper()
	path, state := hosttest.CommittedBinaryPath(hooksDir)
	if state == hosttest.OffContract {
		b.Skipf("no packaged hook binary for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return path
}

// TestPackagedHookEnvironmentIsTheCaptureEnvironment requires the packaged
// benchmark's environment and fixture to equal the capture's, so a number
// from one compares with a number from the other. Each preparation makes its
// own scratch directory, so the three variables that hold a path there are
// compared by their path below it.
func TestPackagedHookEnvironmentIsTheCaptureEnvironment(t *testing.T) {
	target := hookcost.Target{HooksDir: filepath.Join(t.TempDir(), "hooks")}
	gotFixture, gotEnv := packagedHookEnvironment(t, target)
	wantFixture, wantEnv := hookcost.Prepare(t, target)

	got, want := scratchRelativeEnvironment(t, gotEnv), scratchRelativeEnvironment(t, wantEnv)
	if !slices.Equal(got, want) {
		counts := map[string]int{}
		for _, item := range got {
			counts[item]++
		}
		for _, item := range want {
			counts[item]--
		}
		var onlyBenchmark, onlyCapture []string
		for item, n := range counts {
			switch {
			case n > 0:
				onlyBenchmark = append(onlyBenchmark, item)
			case n < 0:
				onlyCapture = append(onlyCapture, item)
			}
		}
		sort.Strings(onlyBenchmark)
		sort.Strings(onlyCapture)
		t.Errorf("environments differ\nonly in the benchmark's: %q\nonly in the capture's: %q", onlyBenchmark, onlyCapture)
	}

	type fixture struct {
		name, dir string
		env       []string
	}
	for _, f := range []fixture{{"benchmark", gotFixture, gotEnv}, {"capture", wantFixture, wantEnv}} {
		if status := fixtureGit(t, f.dir, f.env, "status", "--short"); status != "" {
			t.Errorf("%s fixture status = %q, want clean", f.name, status)
		}
		var subjects []string
		for _, line := range strings.Split(fixtureGit(t, f.dir, f.env, "log", "--oneline"), "\n") {
			_, subject, _ := strings.Cut(line, " ")
			subjects = append(subjects, subject)
		}
		if !slices.Equal(subjects, []string{"baseline"}) {
			t.Errorf("%s fixture history = %q, want one commit named baseline", f.name, subjects)
		}
	}
}

// scratchRelativeEnvironment returns env sorted, with the variables that hold
// a path in the preparation's scratch directory reduced to the path below it.
func scratchRelativeEnvironment(t *testing.T, env []string) []string {
	t.Helper()
	var scratch string
	for _, item := range env {
		if key, value, _ := strings.Cut(item, "="); strings.EqualFold(key, "GIT_CONFIG_GLOBAL") {
			scratch = filepath.Dir(value)
		}
	}
	if scratch == "" {
		t.Fatal("environment has no GIT_CONFIG_GLOBAL")
	}
	out := make([]string, 0, len(env))
	for _, item := range env {
		key, value, _ := strings.Cut(item, "=")
		switch strings.ToUpper(key) {
		case "GIT_CONFIG_GLOBAL", "COMMAND_GUARD_LOG_DIR", "RELEASE_GUARD_LOG_DIR":
			rel, err := filepath.Rel(scratch, value)
			if err != nil {
				t.Fatal(err)
			}
			item = key + "=" + filepath.ToSlash(rel)
		}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func fixtureGit(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := hookcost.Process(context.Background(), "git", args)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
