package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
)

func TestStartupProviderOutput(t *testing.T) {
	root := filepath.Join(t.TempDir(), "install 'é' & $money!")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	name := "agx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	launcher := filepath.Join(bin, name)
	writeStartupResolverFixture(t, root)
	if err := os.WriteFile(launcher, []byte("launcher fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	wrong := t.TempDir()
	if err := os.WriteFile(filepath.Join(wrong, name), []byte("different launcher"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", wrong)
	t.Setenv("AGX_SOURCE_DIGEST", "1")
	hint := "For agx commands, run PowerShell: & '" + strings.ReplaceAll(launcher, "'", "''") +
		"' help or Bash: '" + strings.ReplaceAll(filepath.ToSlash(launcher), "'", "'\\''") + "' help."
	nested := map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "SessionStart", "additionalContext": hint}}
	for provider, want := range map[string]any{
		"claude-code": nested, "codex": nested,
		"copilot":     map[string]any{"additionalContext": hint},
		"antigravity": map[string]any{"injectSteps": []any{map[string]any{"systemMessage": map[string]any{"systemMessage": hint}}}},
	} {
		t.Run(provider, func(t *testing.T) {
			code, out, diag := invoke(t, `{"source":"startup","initialPrompt":"NEVER_INCLUDE_THIS"}`, "--startup-hint", provider, root)
			var got any
			if code != 0 || diag != "" || json.Unmarshal([]byte(out), &got) != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("startup exit %d stdout %q stderr %q, want %#v", code, out, diag, want)
			}
		})
	}
}

func TestStartupIgnoresInvalidCopilotInput(t *testing.T) {
	for _, input := range []string{
		"", `{}`, `null`, `{"source":null}`, `{"source":7}`, `{"source":"resume"}`,
		`{"source":"compact"}`, `{"source":"startup"} {}`, `{"source":"new"`,
		`{"source":"startup","initialPrompt":"` + strings.Repeat("p", 1<<20) + `"}`,
	} {
		if freshCopilotSession(strings.NewReader(input)) {
			t.Fatalf("accepted unexpected input (%d bytes)", len(input))
		}
	}
	for _, source := range []string{"new", "startup"} {
		if !freshCopilotSession(strings.NewReader(`{"source":"` + source + `"}`)) {
			t.Errorf("rejected fresh source %q", source)
		}
	}
}

func TestStartupRejectsRedirectedLauncher(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	name := "agx"
	if runtime.GOOS == "windows" {
		name = "agx.exe"
	}
	if err := os.WriteFile(filepath.Join(target, name), []byte("other"), 0755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	writeStartupResolverFixture(t, root)
	if runtime.GOOS == "windows" {
		hookutiltest.JunctionOrSkip(t, target, bin)
	} else if err := os.Symlink(target, bin); err != nil {
		t.Fatal(err)
	}
	if path := installedLauncher(root); path != "" {
		t.Fatalf("advertised redirected directory: %q", path)
	}
	if runtime.GOOS != "windows" {
		if err := os.Remove(bin); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(bin, 0755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(bin, name)
		if err := os.Symlink(filepath.Join(target, name), path); err != nil {
			t.Fatal(err)
		}
		if got := installedLauncher(root); got != "" {
			t.Fatalf("advertised symlink: %q", got)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("not executable"), 0644); err != nil {
			t.Fatal(err)
		}
		if got := installedLauncher(root); got != "" {
			t.Fatalf("advertised non-executable: %q", got)
		}
	}
}

func TestStartupRejectsWindowsAncestorJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows launcher contract")
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "plugin")
	writeStartupResolverFixture(t, root)
	if err := os.Mkdir(filepath.Join(root, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "agx.exe"), []byte("installed fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "linked-parent")
	hookutiltest.JunctionOrSkip(t, parent, link)
	for _, tc := range []struct {
		root     string
		wantHint bool
	}{{root, true}, {filepath.Join(link, "plugin"), false}} {
		code, out, diag := invoke(t, "{}", "--startup-hint", "claude-code", tc.root)
		if code != 0 || diag != "" || (out != "") != tc.wantHint {
			t.Fatalf("root %q: startup exit %d stdout %q stderr %q, want hint %t", tc.root, code, out, diag, tc.wantHint)
		}
	}
	hooks := filepath.Join(root, "hooks")
	physical := filepath.Join(parent, "resolver")
	if err := os.Rename(hooks, physical); err != nil {
		t.Fatal(err)
	}
	hookutiltest.JunctionOrSkip(t, physical, hooks)
	if path := installedLauncher(root); path != "" {
		t.Fatalf("advertised launcher with redirected resolver scripts: %q", path)
	}
}

func writeStartupResolverFixture(t *testing.T, root string) {
	t.Helper()
	hooks := filepath.Join(root, "hooks")
	if err := os.MkdirAll(hooks, 0755); err != nil {
		t.Fatal(err)
	}
	scripts := []string{"run-hook.cmd"}
	if runtime.GOOS == "windows" {
		scripts = append(scripts, "fetch-hook.ps1")
	}
	for _, script := range scripts {
		if err := os.WriteFile(filepath.Join(hooks, script), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStartupWindowsRequiresDriveRoot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows launcher contract")
	}
	// These names must be rejected before any filesystem or network access.
	for _, root := range []string{`\\server\share\plugin`, `\\?\UNC\server\share\plugin`, `C:plugin`, `\plugin`} {
		if startupRootAllowed(root) {
			t.Errorf("accepted startup root %q", root)
		}
	}
	if !startupRootAllowed(`C:\installed\plugin`) {
		t.Fatal("rejected absolute drive path")
	}
}

func TestStartupUnixAllowsLinkedInstallAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher contract")
	}
	parent := t.TempDir()
	bin := filepath.Join(parent, "plugin", "bin")
	writeStartupResolverFixture(t, filepath.Dir(bin))
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "agx"), nil, 0755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(parent, alias); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(alias, "plugin")
	if got := installedLauncher(root); got != filepath.Join(root, "bin", "agx") {
		t.Fatalf("linked Unix ancestor: launcher %q", got)
	}
}

func TestStartupRejectsUnixLinkedResolver(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher contract")
	}
	for _, rel := range []string{"hooks", "hooks/run-hook.cmd"} {
		t.Run(rel, func(t *testing.T) {
			root := t.TempDir()
			writeStartupResolverFixture(t, root)
			bin := filepath.Join(root, "bin")
			if err := os.Mkdir(bin, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(bin, "agx"), nil, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, rel)
			target := filepath.Join(root, "original")
			if err := os.Rename(path, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			code, out, diag := invoke(t, `{}`, "--startup-hint", "claude-code", root)
			if code != 0 || out != "" || diag != "" {
				t.Fatalf("linked %s: exit %d stdout %q stderr %q", rel, code, out, diag)
			}
		})
	}
}
