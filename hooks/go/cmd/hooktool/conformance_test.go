package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/conformance"
	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

func TestAuditResult(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "hooktool")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build hooktool: %v: %s", err, out)
	}
	cmd := exec.Command(binary, "audit", "--repo", t.TempDir(), "--source-only=false")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("missing dispatcher must make audit exit nonzero")
	}
	var result struct {
		Version int `json:"version"`
		Areas   []struct {
			Name     string            `json:"name"`
			Failures []json.RawMessage `json:"failures"`
			Error    *string           `json:"error"`
		} `json:"areas"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("audit must emit JSON on operational error: %v; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if result.Version != 1 || len(result.Areas) != 4 {
		t.Fatalf("invalid inventory: %+v", result)
	}
	for i, name := range []string{"ascii", "schema", "module", "freshness"} {
		a := result.Areas[i]
		if a.Name != name || a.Failures == nil || a.Error == nil {
			t.Fatalf("area %d: %+v", i, a)
		}
		if i > 0 && *a.Error != "not-run: prior operational error" {
			t.Fatalf("area %s: %q", name, *a.Error)
		}
	}
	if stderr.Len() == 0 {
		t.Fatal("missing diagnostic on stderr")
	}
	t.Run("reached failures survive operational error", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "hooks"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "hooks", "run-hook.cmd"), []byte{0xff}, 0644); err != nil {
			t.Fatal(err)
		}
		var out, diag bytes.Buffer
		cmd := exec.Command(binary, "audit", "--repo", root, "--source-only=false")
		cmd.Stdout, cmd.Stderr = &out, &diag
		if err := cmd.Run(); err == nil {
			t.Fatal("missing module must fail")
		}
		var got struct {
			Version int `json:"version"`
			Areas   []struct {
				Name     string `json:"name"`
				Failures []struct{ Rule, Path, Message string }
				Error    *string `json:"error"`
			} `json:"areas"`
		}
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("JSON: %v stdout=%q stderr=%q", err, out.String(), diag.String())
		}
		if got.Version != 1 || len(got.Areas) != 4 {
			t.Fatalf("inventory: %+v", got)
		}
		for i, name := range []string{"ascii", "schema", "module", "freshness"} {
			if got.Areas[i].Name != name || got.Areas[i].Failures == nil {
				t.Fatalf("area %d: %+v", i, got.Areas[i])
			}
		}
		if len(got.Areas[0].Failures) != 1 || got.Areas[0].Failures[0].Rule != "run-hook-ascii" || got.Areas[0].Failures[0].Path != "hooks/run-hook.cmd" || !strings.Contains(got.Areas[0].Failures[0].Message, "0xFF") {
			t.Fatalf("lost finding: %+v", got.Areas[0])
		}
		if len(got.Areas[1].Failures) == 0 || got.Areas[2].Error == nil || got.Areas[3].Error == nil || *got.Areas[3].Error != "not-run: prior operational error" || diag.Len() == 0 {
			t.Fatalf("error result=%+v stderr=%q", got, diag.String())
		}
	})

	root := auditFixture(t)
	run := func() (result struct {
		Version int `json:"version"`
		Areas   []struct {
			Name     string `json:"name"`
			Failures []struct{ Rule, Path, Message string }
			Error    *string `json:"error"`
		} `json:"areas"`
	}, stderr string, err error) {
		t.Helper()
		var out, diag bytes.Buffer
		cmd := exec.Command(binary, "audit", "--repo", root, "--source-only=false")
		cmd.Stdout, cmd.Stderr = &out, &diag
		err = cmd.Run()
		if e := json.Unmarshal(out.Bytes(), &result); e != nil {
			t.Fatalf("audit JSON: %v; stdout=%q stderr=%q", e, out.String(), diag.String())
		}
		if result.Version != 1 || len(result.Areas) != 4 {
			t.Fatalf("inventory: %+v", result)
		}
		for i, name := range []string{"ascii", "schema", "module", "freshness"} {
			if result.Areas[i].Name != name || result.Areas[i].Failures == nil {
				t.Fatalf("area %d: %+v", i, result.Areas[i])
			}
		}
		return result, withoutWindowsSkip(diag.String()), err
	}
	t.Run("success", func(t *testing.T) {
		got, diag, err := run()
		if err != nil || diag != "" {
			t.Fatalf("audit: %v stderr=%q", err, diag)
		}
		for _, area := range got.Areas {
			if area.Error != nil || len(area.Failures) != 0 {
				t.Fatalf("unexpected area: %+v", area)
			}
		}
	})
	t.Run("skip tests", func(t *testing.T) {
		var out, diag bytes.Buffer
		cmd := exec.Command(binary, "audit", "--repo", root, "--skip-tests", "--source-only=false")
		cmd.Stdout, cmd.Stderr = &out, &diag
		if err := cmd.Run(); err != nil {
			t.Fatalf("audit --skip-tests: %v stdout=%q stderr=%q", err, out.String(), diag.String())
		}
		var got struct {
			Version int `json:"version"`
			Areas   []struct {
				Name     string            `json:"name"`
				Failures []json.RawMessage `json:"failures"`
				Error    *string           `json:"error"`
			} `json:"areas"`
		}
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("audit JSON: %v; stdout=%q", err, out.String())
		}
		if got.Version != 1 || len(got.Areas) != 4 {
			t.Fatalf("inventory: %+v", got)
		}
		for _, area := range got.Areas {
			if area.Error != nil || len(area.Failures) != 0 {
				t.Fatalf("unexpected area: %+v", area)
			}
		}
		lines := strings.Split(strings.TrimRight(diag.String(), "\n"), "\n")
		if len(lines) != 1 || !strings.HasPrefix(lines[0], "SKIP: ") || !strings.Contains(lines[0], "module-test") || !strings.Contains(lines[0], "module-coverage-floor") {
			t.Fatalf("want one SKIP line naming module-test and module-coverage-floor, got stderr=%q", diag.String())
		}
	})
	t.Run("unknown audit flag", func(t *testing.T) {
		var out, diag bytes.Buffer
		cmd := exec.Command(binary, "audit", "--repo", root, "--skip-everything")
		cmd.Stdout, cmd.Stderr = &out, &diag
		if err := cmd.Run(); err == nil {
			t.Fatalf("an unknown audit flag must fail; stdout=%q stderr=%q", out.String(), diag.String())
		}
		if out.Len() != 0 {
			t.Fatalf("a rejected flag must not emit an audit result: %q", out.String())
		}
	})
	t.Run("rule failure", func(t *testing.T) {
		path := filepath.Join(root, "hooks", "run-hook.cmd")
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(original, 0xff), 0644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.WriteFile(path, original, 0644); err != nil {
				t.Error(err)
			}
		})
		got, diag, err := run()
		if err != nil || diag != "" {
			t.Fatalf("audit: %v stderr=%q", err, diag)
		}
		if len(got.Areas[0].Failures) != 1 || got.Areas[0].Failures[0].Rule != "run-hook-ascii" || got.Areas[0].Failures[0].Path != "hooks/run-hook.cmd" {
			t.Fatalf("failure: %+v", got.Areas[0])
		}
	})
	t.Run("stale rebuild deduplication", func(t *testing.T) {
		path := filepath.Join(root, "hooks", "go", "internal", "provider", "provider.go")
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("\n// changed source\n"); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		// A fresh binary with altered bytes must retain its rebuild mismatch.
		freshDir := t.TempDir()
		rebuild(t, filepath.Join(root, "hooks", "go"), freshDir, hookbuild.ContractBinaries, false)
		freshName := hookbuild.ContractBinaries[0]
		freshBytes, err := os.ReadFile(filepath.Join(freshDir, freshName))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "hooks", "bin", freshName), append(freshBytes, 0), 0755); err != nil {
			t.Fatal(err)
		}
		before, err := conformance.RunModuleGates(root)
		if err != nil {
			t.Fatal(err)
		}
		mismatches := map[string]bool{}
		for _, f := range before {
			if f.Rule == "module-rebuild-mismatch" {
				mismatches[f.Path] = true
			}
		}
		for _, name := range hookbuild.ContractBinaries {
			if !mismatches["hooks/bin/"+name] {
				t.Fatalf("pre-dedup rebuild mismatch missing for %s: %+v", name, before)
			}
		}
		wantModule := map[string]int{}
		for _, f := range before {
			if f.Rule == "module-rebuild-mismatch" && f.Path != "hooks/bin/"+freshName {
				continue
			}
			wantModule[fmt.Sprintf("%s\x00%s\x00%s", f.Rule, f.Path, f.Message)]++
		}
		got, diag, err := run()
		if err != nil || diag != "" {
			t.Fatalf("audit: %v stderr=%q", err, diag)
		}
		freshMismatch := false
		gotModule := map[string]int{}
		for _, f := range got.Areas[2].Failures {
			gotModule[fmt.Sprintf("%s\x00%s\x00%s", f.Rule, f.Path, f.Message)]++
			if f.Rule == "module-rebuild-mismatch" && f.Path == "hooks/bin/"+freshName {
				freshMismatch = true
				continue
			}
			if f.Rule == "module-rebuild-manifest" {
				continue
			}
			if f.Rule == "module-rebuild-mismatch" || f.Rule == "module-rebuild-manifest" {
				t.Fatalf("redundant rebuild failure: %+v", f)
			}
		}
		if !freshMismatch {
			t.Fatal("fresh binary rebuild mismatch was removed")
		}
		if !reflect.DeepEqual(gotModule, wantModule) {
			t.Fatalf("module findings differ beyond redundant stale mismatches: got=%v want=%v", gotModule, wantModule)
		}
		stale := 0
		for _, f := range got.Areas[3].Failures {
			if f.Rule == "freshness-stale-digest" {
				stale++
			}
		}
		// Five stale hook binaries plus the stale launcher.
		if stale != 6 {
			t.Fatalf("stale count %d, want 6: %+v", stale, got.Areas[3])
		}
	})
}

// windowsSkipLine is the one diagnostic the module gates print on native Windows.
// It must match the message printed by checkHostTests in
// hooks/go/internal/conformance/module.go.
const windowsSkipLine = "SKIP: hooks cmd/hook integration tests and module-wide coverage on native Windows; run the complete suite and 90% coverage gate on Linux/macOS\n"

// withoutWindowsSkip drops one windowsSkipLine on Windows; other stderr text, and all stderr on other OSes, stays.
func withoutWindowsSkip(diag string) string {
	if runtime.GOOS != "windows" {
		return diag
	}
	return strings.Replace(diag, windowsSkipLine, "", 1)
}

func TestSourceOnlyAudit(t *testing.T) {
	root := auditFixture(t)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "init.templateDir="}, args...)...)
		cmd.Dir = root
		cmd.Env = slices.DeleteFunc(os.Environ(), func(e string) bool { return strings.HasPrefix(strings.ToUpper(e), "GIT_") })
		cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_ALLOW_PROTOCOL=")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write := func(rel string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, rel), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	images := map[string][]byte{}
	for _, name := range hookbuild.ContractBinaries {
		path := filepath.Join(root, "hooks", "bin", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		images[name] = data
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{hookbuild.LauncherOutput, ".gitattributes"} {
		if err := os.Remove(filepath.Join(root, rel)); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	git("-c", "core.autocrlf=false", "add", ".")
	binary := filepath.Join(t.TempDir(), "hooktool.exe")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	audit := func(t *testing.T, repo string, wantError bool, flags ...string) conformance.AuditResult {
		t.Helper()
		args := append([]string{"audit", "--repo", repo, "--skip-tests"}, flags...)
		cmd := exec.Command(binary, args...)
		var diag bytes.Buffer
		cmd.Stderr = &diag
		out, err := cmd.Output()
		if (err != nil) != wantError {
			t.Fatalf("audit %v: %v: %s", args, err, diag.String())
		}
		var r conformance.AuditResult
		if err := json.Unmarshal(out, &r); err != nil || r.Version != 1 || len(r.Areas) != 4 {
			t.Fatalf("inventory: %v: %s", err, out)
		}
		for i, name := range []string{"ascii", "schema", "module", "freshness"} {
			if a := r.Areas[i]; a.Name != name || a.Failures == nil || (a.Error != nil) != (wantError && i >= 2) {
				t.Fatalf("area %d: %+v", i, a)
			}
		}
		return r
	}
	legacy := audit(t, root, false, "--source-only=false")
	missing := false
	for _, f := range legacy.Areas[3].Failures {
		if f.Rule == "bin-launchers" && f.Path == hookbuild.LauncherOutput {
			missing = true
		}
	}
	if !missing {
		t.Fatalf("compatibility audit must require its launcher: %+v", legacy)
	}
	assertClean := func(r conformance.AuditResult) {
		t.Helper()
		for _, a := range r.Areas {
			if len(a.Failures) != 0 {
				t.Fatalf("unexpected %s findings: %v", a.Name, a.Failures)
			}
		}
	}
	assertClean(audit(t, root, false))
	write("bin/agx.exe", []byte("local output"))
	write("hooks/bin/agx-linux-amd64", []byte("local runtime"))
	write("local-output", images["agx-darwin-arm64"])
	write("staged-text", []byte("MZ documentation"))
	git("add", "staged-text")
	write("staged-text", images["agx-linux-arm64"])
	assertClean(audit(t, root, false))
	want := map[string]string{"one": "agx-windows-arm64.exe", "two": "agx-linux-amd64", "three": "agx-darwin-amd64"}
	for rel, name := range want {
		write(rel, images[name])
		git("-c", "core.autocrlf=false", "add", rel)
		write(rel, []byte("worktree replacement"))
	}
	// An invalid module would error if any audited command ran past the scan.
	if err := os.Rename(filepath.Join(root, "hooks/go"), filepath.Join(root, "module-aside")); err != nil {
		t.Fatal(err)
	}
	// The CLI must scan the selected candidate even when its caller exports a
	// different index. Only the read-only audit inherits these overrides.
	assertRejected := func(t *testing.T, repo string) {
		t.Helper()
		result := audit(t, repo, false)
		var paths []string
		for _, f := range result.Areas[2].Failures {
			if _, ok := want[f.Path]; f.Rule != "tracked-native-executable" || !ok {
				t.Fatalf("unexpected finding: %+v", f)
			}
			paths = append(paths, f.Path)
		}
		slices.Sort(paths)
		if !slices.Equal(paths, []string{"one", "three", "two"}) {
			t.Fatalf("tracked-native-executable paths = %v, want [one three two]", paths)
		}
	}
	t.Run("caller points at another index", func(t *testing.T) {
		t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), ".git"))
		t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "index"))
		assertRejected(t, root)
	})
	t.Run("fresh checkout of committed executables", func(t *testing.T) {
		git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
		checkout := filepath.Join(t.TempDir(), "checkout")
		git("worktree", "add", "--quiet", "--detach", checkout, "HEAD")
		assertRejected(t, checkout)
	})
	write(".git/index", []byte("corrupt index"))
	failed := audit(t, root, true)
	if !strings.Contains(*failed.Areas[2].Error, "ls-files") || *failed.Areas[3].Error != "not-run: prior operational error" {
		t.Fatalf("index error: %+v", failed)
	}
}

// auditFixture reuses the tiny module's checked-in test programs while exercising the CLI.
func auditFixture(t *testing.T) string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "../../internal/conformance/tinyhook_test.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if ok && len(spec.Names) == 1 && len(spec.Values) == 1 {
			if lit, ok := spec.Values[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, e := strconv.Unquote(lit.Value); e == nil {
					values[spec.Names[0].Name] = s
				}
			}
		}
		return true
	})
	root := t.TempDir()
	put := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	put("hooks/go/go.mod", strings.Replace(string(mod), "github.com/muehz/agentics/hooks/go", "example.com/tinyhook", 1))
	for rel, key := range map[string]string{"hooks/go/cmd/hook/main.go": "tinyMainGo", "hooks/go/cmd/hook/main_test.go": "tinyMainTestGo", "hooks/go/internal/provider/provider.go": "tinyProviderGo", "hooks/go/cmd/launcher/main.go": "tinyLauncherGo", "bin/agx": "tinyLauncherScript", ".gitattributes": "tinyGitAttributes"} {
		if values[key] == "" {
			t.Fatalf("missing fixture %s", key)
		}
		put(rel, values[key])
	}
	put("hooks/run-hook.cmd", "@echo off\nexit /b 0\n")
	for _, rel := range []string{"hooks/hooks.claude.json", "hooks/hooks.copilot.json", "hooks/hooks.codex.json", "hooks.json"} {
		data, err := os.ReadFile(filepath.Join(hosttest.Checkout(t), filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		put(rel, string(data))
	}
	bin := filepath.Join(root, "hooks", "bin")
	rebuild(t, filepath.Join(root, "hooks", "go"), bin, hookbuild.ContractBinaries, false)
	rebuild(t, filepath.Join(root, "hooks", "go"), filepath.Join(root, filepath.Dir(filepath.FromSlash(hookbuild.LauncherOutput))), nil, true)
	var manifest strings.Builder
	for _, name := range hookbuild.ContractBinaries {
		sum, err := hookbuild.FileSHA256(filepath.Join(bin, name))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&manifest, "%s  bin/%s\n", sum, name)
	}
	put("hooks/SHA256SUMS", manifest.String())
	return root
}
