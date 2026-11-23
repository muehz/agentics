package hookutil

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
)

const testGuardEnv = "HOOKUTIL_TEST_GUARD_LOG_DIR"

func TestCheckWorkingDirectoryRequiresMatchingIdentity(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	if err := checkWorkingDirectory(work); err != nil {
		t.Fatalf("current directory: %v", err)
	}
	other := t.TempDir()
	for _, path := range []string{other, filepath.Join(other, "missing")} {
		if err := checkWorkingDirectory(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("checkWorkingDirectory(%q) = %v, want not-exist error", path, err)
		}
	}
}

func TestGuardLogDir(t *testing.T) {
	g := GuardLog{Guard: "demo-guardian", EnvDir: testGuardEnv}
	t.Setenv(testGuardEnv, "")
	if got, want := g.Dir("root"), "root/agentics/logs/demo-guardian"; got != want {
		t.Errorf("log root: Dir = %q, want %q", got, want)
	}
	if got, want := g.Dir(""), ".agentics/logs/demo-guardian"; got != want {
		t.Errorf("fallback: Dir = %q, want %q", got, want)
	}
	t.Setenv(testGuardEnv, "custom/dir")
	if got, want := g.Dir("root"), "custom/dir"; got != want {
		t.Errorf("override: Dir = %q, want %q", got, want)
	}
}

func TestGuardLogAppend(t *testing.T) {
	g := GuardLog{Guard: "demo-guardian", EnvDir: testGuardEnv}
	const aliasSentence = "demo-guardian refused to append guard.log: a linked path component or a non-regular guard.log would redirect the write"

	t.Run("relative directory anchors at the working directory", func(t *testing.T) {
		work := t.TempDir()
		t.Chdir(work)
		t.Setenv(testGuardEnv, "rel/logs")
		if err := g.Append("ignored", map[string]string{"k": "v"}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(work, "rel", "logs", "guard.log"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "{\"k\":\"v\"}\n" {
			t.Fatalf("log = %q", data)
		}
	})

	t.Run("two appends give two lines unescaped", func(t *testing.T) {
		work := t.TempDir()
		t.Chdir(work)
		t.Setenv(testGuardEnv, "")
		for _, text := range []string{"a<b", "c&d"} {
			if err := g.Append("", map[string]string{"m": text}); err != nil {
				t.Fatal(err)
			}
		}
		data, err := os.ReadFile(filepath.Join(work, ".agentics", "logs", "demo-guardian", "guard.log"))
		if err != nil {
			t.Fatal(err)
		}
		if want := "{\"m\":\"a<b\"}\n{\"m\":\"c&d\"}\n"; string(data) != want {
			t.Fatalf("log = %q, want %q", data, want)
		}
	})

	t.Run("encoding error is returned as is", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv(testGuardEnv, "")
		err := g.Append("", make(chan int))
		if err == nil || errors.Is(err, ErrLogAlias) || strings.Contains(err.Error(), "demo-guardian") {
			t.Fatalf("Append = %v, want the bare encoder error", err)
		}
	})

	t.Run("linked parent is refused", func(t *testing.T) {
		work := t.TempDir()
		t.Chdir(work)
		t.Setenv(testGuardEnv, "")
		outside := t.TempDir()
		hookutiltest.SymlinkOrSkip(t, outside, filepath.Join(work, ".agentics"))
		err := g.Append("", map[string]string{"k": "v"})
		if !errors.Is(err, ErrLogAlias) {
			t.Fatalf("Append = %v, want ErrLogAlias", err)
		}
		if err.Error() != aliasSentence {
			t.Fatalf("Append error = %q, want %q", err.Error(), aliasSentence)
		}
		if entries, _ := os.ReadDir(outside); len(entries) != 0 {
			t.Fatalf("a write went through the link: %v", entries)
		}
	})

	t.Run("non-regular leaf is refused", func(t *testing.T) {
		work := t.TempDir()
		t.Chdir(work)
		t.Setenv(testGuardEnv, "logs")
		if err := os.MkdirAll(filepath.Join(work, "logs"), 0o755); err != nil {
			t.Fatal(err)
		}
		victim := filepath.Join(t.TempDir(), "victim")
		if err := os.WriteFile(victim, []byte("original\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		hookutiltest.SymlinkOrSkip(t, victim, filepath.Join(work, "logs", "guard.log"))
		err := g.Append("", map[string]string{"k": "v"})
		if !errors.Is(err, ErrLogAlias) {
			t.Fatalf("Append = %v, want ErrLogAlias", err)
		}
		if err.Error() != aliasSentence {
			t.Fatalf("Append error = %q, want %q", err.Error(), aliasSentence)
		}
		if data, _ := os.ReadFile(victim); string(data) != "original\n" {
			t.Fatalf("victim changed: %q", data)
		}
	})

	t.Run("removed working directory", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows cannot remove the current directory")
		}
		outside := t.TempDir()
		work := filepath.Join(t.TempDir(), "gone")
		if err := os.Mkdir(work, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Chdir(work)
		if err := os.Remove(work); err != nil {
			t.Fatal(err)
		}
		t.Setenv(testGuardEnv, "")
		check := func() {
			t.Helper()
			for _, root := range []string{"", outside} {
				err := g.Append(root, map[string]string{"k": "v"})
				if !errors.Is(err, os.ErrNotExist) || !strings.HasPrefix(err.Error(), "demo-guardian could not resolve the working directory: ") {
					t.Errorf("Append(%q) = %v, want the working-directory sentence and not-exist error", root, err)
				}
			}
		}
		check()
		if err := os.Mkdir(work, 0o755); err != nil {
			t.Fatal(err)
		}
		check()
		for _, dir := range []string{work, outside} {
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 0 {
				t.Errorf("invalid cwd wrote to %q: entries=%v, error=%v", dir, entries, err)
			}
		}
	})
}
