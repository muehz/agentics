package hookcost

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

func TestResolveTarget(t *testing.T) {
	checkout := t.TempDir()
	dir := filepath.Join(checkout, "scratch")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(checkout, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(checkout, "absent")
	dotted := dir + string(filepath.Separator) + "sub" + string(filepath.Separator) + ".." + string(filepath.Separator) + "sub"
	present := func(hooksDir string) (string, hosttest.BinaryState) {
		return filepath.Join(hooksDir, "bin", "hook"), hosttest.Present
	}
	cases := []struct {
		name     string
		value    *string
		hooksDir string
		err      string
	}{
		{"unset", nil, filepath.Join(checkout, "hooks"), ""},
		{"empty", ptr(""), filepath.Join(checkout, "hooks"), ""},
		{"relative", ptr("hooks"), "", "AGX_BENCH_HOOKS_DIR must be an absolute directory"},
		{"absolute directory", ptr(dir), dir, ""},
		{"absolute file", ptr(file), "", fmt.Sprintf("AGX_BENCH_HOOKS_DIR %q is not a directory: <nil>", file)},
		{"absolute missing", ptr(missing), "", fmt.Sprintf("AGX_BENCH_HOOKS_DIR %q is not a directory: ", missing)},
		{"dot-dot through a directory", ptr(dotted), filepath.Join(dir, "sub"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(name string) string {
				if name != "AGX_BENCH_HOOKS_DIR" || tc.value == nil {
					return ""
				}
				return *tc.value
			}
			got, err := resolveTarget(checkout, getenv, present)
			if tc.err != "" {
				if err == nil || !strings.HasPrefix(err.Error(), tc.err) || (strings.HasSuffix(tc.err, "<nil>") && err.Error() != tc.err) {
					t.Fatalf("got %+v, %v; want error %q", got, err, tc.err)
				}
				return
			}
			want := Target{HooksDir: tc.hooksDir, Binary: filepath.Join(tc.hooksDir, "bin", "hook"), Dispatcher: filepath.Join(tc.hooksDir, "run-hook.cmd")}
			if err != nil || got != want {
				t.Fatalf("got %+v, %v; want %+v", got, err, want)
			}
		})
	}
	t.Run("off contract", func(t *testing.T) {
		off := func(string) (string, hosttest.BinaryState) { return "", hosttest.OffContract }
		_, err := resolveTarget(checkout, func(string) string { return "" }, off)
		if want := fmt.Sprintf("no hook binary is committed for %s/%s", runtime.GOOS, runtime.GOARCH); !errors.Is(err, ErrOffContract) || err.Error() != want {
			t.Fatalf("got %v, want %q wrapping ErrOffContract", err, want)
		}
	})
	t.Run("committed binary", func(t *testing.T) {
		got, err := ResolveTarget(checkout, func(string) string { return "" })
		path, state := hosttest.CommittedBinaryPath(filepath.Join(checkout, "hooks"))
		if state == hosttest.OffContract {
			if !errors.Is(err, ErrOffContract) {
				t.Fatalf("got %v", err)
			}
			return
		}
		if err != nil || got.Binary != path {
			t.Fatalf("got %+v, %v; want binary %q", got, err, path)
		}
	})
}

func ptr(s string) *string { return &s }

func TestPrepare(t *testing.T) {
	hooksDir := filepath.Join(t.TempDir(), "plugin", "hooks")
	t.Setenv("SKIP_COMMAND_GUARD", "1")
	t.Setenv("AGX_DEBUG", "1")
	fixture, env := Prepare(t, Target{HooksDir: hooksDir})
	want := map[string]string{
		"GIT_CONFIG_NOSYSTEM": "1", "LC_ALL": "C", "LANG": "C", "AGX_NO_FETCH": "1", "AGX_DEBUG": "", "SKIP_COMMAND_GUARD": "",
		"CLAUDE_PLUGIN_ROOT": filepath.Dir(hooksDir), "PLUGIN_ROOT": filepath.Dir(hooksDir), "COPILOT_PLUGIN_ROOT": filepath.Dir(hooksDir),
	}
	values := map[string]string{}
	for _, item := range env {
		key, value, _ := strings.Cut(item, "=")
		if runtime.GOOS == "windows" {
			key = strings.ToUpper(key)
		}
		values[key] = value
	}
	for key, value := range want {
		if got, ok := values[key]; !ok || got != value {
			t.Fatalf("%s = %q (set %v), want %q", key, got, ok, value)
		}
	}
	scratch := filepath.Dir(fixture)
	for key, path := range map[string]string{"GIT_CONFIG_GLOBAL": "gitconfig", "COMMAND_GUARD_LOG_DIR": "logs", "RELEASE_GUARD_LOG_DIR": "logs"} {
		if values[key] != filepath.Join(scratch, path) {
			t.Fatalf("%s = %q, want it under %s", key, values[key], scratch)
		}
	}
	if filepath.Base(fixture) != "repo" {
		t.Fatalf("fixture %q", fixture)
	}
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = fixture
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if status := git("status", "--porcelain"); status != "" {
		t.Fatalf("fixture is not clean: %q", status)
	}
	if log := git("log", "--format=%an <%ae> %s"); log != "Packaged Hook Benchmark <packaged-hook-bench@example.invalid> baseline" {
		t.Fatalf("fixture history %q", log)
	}
	if data, err := os.ReadFile(filepath.Join(fixture, "fixture.txt")); err != nil || string(data) != "safe baseline\n" {
		t.Fatalf("fixture.txt = %q, %v", data, err)
	}
}
