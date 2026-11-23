package flow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	runTimeout        = 60 * time.Second
	legacyVersionLine = "# flow-state 1"
	headerRow         = "ts\tsession\tphase\tevent\ttarget\tstatus\twhy\tevidence\tnext"
)

type result struct {
	code           int
	stdout, stderr string
}

func (r result) combined() string { return r.stdout + r.stderr }

func baseEnv(extra ...string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + suite.home, "USERPROFILE=" + suite.home,
		"TMPDIR=" + os.TempDir(), "TEMP=" + os.TempDir(), "TMP=" + os.TempDir(),
		"FLOW_SESSION=test-session", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + suite.gitConfig,
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	}
	for _, name := range []string{"PATHEXT", "SystemRoot", "windir", "ComSpec"} {
		if value := os.Getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	for _, setting := range extra {
		name, _, hasValue := strings.Cut(setting, "=")
		kept := env[:0]
		for _, have := range env {
			key, _, _ := strings.Cut(have, "=")
			if key != name {
				kept = append(kept, have)
			}
		}
		env = kept
		if hasValue {
			env = append(env, setting)
		}
	}
	return env
}

func runFlow(t *testing.T, dir string, extraEnv []string, args ...string) result {
	t.Helper()
	env := make(map[string]string)
	for _, setting := range baseEnv(extraEnv...) {
		key, value, _ := strings.Cut(setting, "=")
		env[key] = value
	}
	var stdout, stderr bytes.Buffer
	code := Run(args, dir, func(key string) string { return env[key] }, time.Now, &stdout, &stderr)
	return result{code, stdout.String(), stderr.String()}
}

func runCmd(t *testing.T, dir, path string, args, env []string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env, cmd.Dir = env, dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	res := result{stdout: stdout.String(), stderr: stderr.String()}
	if err != nil {
		var exitErr *exec.ExitError
		if ctx.Err() != nil {
			t.Fatalf("run %s %v exceeded %s", path, args, runTimeout)
		}
		if !errors.As(err, &exitErr) {
			t.Fatalf("run %s %v failed to start: %v", path, args, err)
		}
		res.code = exitErr.ExitCode()
	}
	return res
}

func mustExit(t *testing.T, res result, want int, what string) {
	t.Helper()
	if res.code != want {
		t.Fatalf("%s: exit %d, want %d\n%s", what, res.code, want, res.combined())
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(body), "\n"), "\n")
}

// dataLines drops the lines of a state.tsv that are not rows: comment lines,
// such as the version line of an older file, and header rows.
func dataLines(lines []string) []string {
	var rows []string
	for _, line := range lines {
		if strings.HasPrefix(line, "#") || line == headerRow {
			continue
		}
		rows = append(rows, line)
	}
	return rows
}

func cells(line string) []string { return strings.Split(line, "\t") }

func row(ts, phase, event, target, status, why, evidence, next string) string {
	return strings.Join([]string{ts, "s1", phase, event, target, status, why, evidence, next}, "\t")
}

func writeState(t *testing.T, repo, folder string, rows ...string) {
	t.Helper()
	writeFile(t, repo, folder+"/state.tsv", headerRow+"\n"+strings.Join(rows, "\n")+"\n")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	res := runCmd(t, dir, "git", args, baseEnv())
	mustExit(t, res, 0, "git "+strings.Join(args, " "))
	return res.stdout
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return strings.TrimRight(git(t, dir, args...), "\n")
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo returns a fresh repository on branch main with one commit "init"
// holding README.md ("x\n"), copied from the suite template. The returned path
// is the physical repository root.
func newRepo(t *testing.T) string {
	t.Helper()
	// Git reports the physical repository root even when the temp parent is a symlink.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	template, err := templateRepo()
	if err != nil {
		t.Fatal(err)
	}
	if err := copyTree(template, dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// copyTree copies the directories and regular files under src into dst, which
// exists. Files keep their mode plus owner write, because Git writes its
// objects read-only and Windows cannot delete a read-only file when the test
// directory is cleaned up.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("copyTree: %s is not a regular file", path)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode().Perm()|0o200)
	})
}

func commitAll(t *testing.T, repo, msg string) {
	t.Helper()
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", msg)
}

func logOK(t *testing.T, repo, folder string, c ...string) {
	t.Helper()
	if len(c) != 7 {
		t.Fatalf("logOK wants 7 cells, got %d", len(c))
	}
	args := append([]string{"log"}, c...)
	args = append(args, "--folder", folder)
	mustExit(t, runFlow(t, repo, nil, args...), 0, "flow log")
}

// seedFlow writes a flow with one create row and an optional branch binding.
func seedFlow(t *testing.T, repo, folder, bind string) {
	t.Helper()
	logOK(t, repo, folder, "intent", "create", "intent.md", "ok", "seed", "intent.md", "spec go")
	if bind != "" {
		logOK(t, repo, folder, "intent", "bind", bind, "ok", "bound", "none", "spec go")
	}
}

func TestGlobalConfigIsolation(t *testing.T) {
	r := runCmd(t, t.TempDir(), "git", []string{"config", "--global", "--list"}, baseEnv())
	mustExit(t, r, 0, "isolated global config")
	if r.stdout != "" {
		t.Fatalf("isolated global config contains host settings: %q", r.stdout)
	}
}

// TestNewRepoState pins what newRepo hands every test: the repository a
// per-test "git init" used to build.
func TestNewRepoState(t *testing.T) {
	repo := newRepo(t)
	if got := gitOut(t, repo, "rev-parse", "--show-toplevel"); filepath.FromSlash(got) != repo {
		t.Errorf("toplevel = %q, want %q", got, repo)
	}
	if got := gitOut(t, repo, "symbolic-ref", "--short", "HEAD"); got != "main" {
		t.Errorf("branch = %q, want main", got)
	}
	if got := gitOut(t, repo, "log", "--format=%s"); got != "init" {
		t.Errorf("log = %q, want one commit init", got)
	}
	if got := gitOut(t, repo, "ls-tree", "-r", "--name-only", "HEAD"); got != "README.md" {
		t.Errorf("tree = %q, want README.md", got)
	}
	if got := gitOut(t, repo, "show", "HEAD:README.md"); got != "x" {
		t.Errorf("README.md = %q, want x", got)
	}
	if got := gitOut(t, repo, "config", "core.autocrlf"); got != "false" {
		t.Errorf("core.autocrlf = %q, want false", got)
	}
	if got := gitOut(t, repo, "status", "--porcelain"); got != "" {
		t.Errorf("status = %q, want clean", got)
	}
}
