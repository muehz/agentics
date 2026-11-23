package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
)

const wantUsage = "usage: sourcedigest [--list] [dir]\n"

// listedTree is a module whose digest-exclude names a directory, a sibling
// sharing its prefix, and a final line without a trailing newline, so the
// tree exercises every way the list shapes the file set.
var listedTree = map[string]string{
	"go.mod":          "module example.com/fixture\n\ngo 1.27.4\n",
	"main.go":         "package main\n",
	"main_test.go":    "package main\n",
	"a/b/excluded.go": "package b\n",
	"a/bc/kept.go":    "package bc\n",
	"skip/gone.go":    "package skip\n",
	"bin/built.go":    "package bin\n",
	"digest-exclude":  "# directories outside every hook binary\n\na/b\nskip",
}

// plainTree is a module with no digest-exclude, so only bin/ and test files
// are left out.
var plainTree = map[string]string{
	"go.mod":               "module example.com/plain\n\ngo 1.27.4\n",
	"cmd/tool/main.go":     "package main\n",
	"internal/x/x.go":      "package x\n",
	"internal/x/y.go":      "package x\n\n// second file\n",
	"internal/x/x_test.go": "package x\n",
	"bin/built.go":         "package bin\n",
}

// malformedTree carries a list whose second line names no directory, so both
// modes must fail on that line.
var malformedTree = map[string]string{
	"go.mod":         "module example.com/fixture\n\ngo 1.27.4\n",
	"a/b/x.go":       "package b\n",
	"digest-exclude": "a/b\na/nope\n",
}

const malformedLine = `digest-exclude line 2 "a/nope" names no directory under the module root`

func TestCommandDigest(t *testing.T) {
	for name, tree := range map[string]map[string]string{"listed": listedTree, "plain": plainTree} {
		t.Run(name, func(t *testing.T) {
			dir := writeTree(t, tree)
			want, err := hookprovenance.SourceDigest(dir)
			if err != nil {
				t.Fatal(err)
			}
			code, out, errOut := runCommand(t, dir)
			if code != 0 || errOut != "" {
				t.Fatalf("exit %d stderr %q", code, errOut)
			}
			if out != want+"\n" {
				t.Fatalf("stdout %q, want %q", out, want+"\n")
			}
			if !regexp.MustCompile(`^[0-9a-f]{64}\n$`).MatchString(out) {
				t.Fatalf("stdout %q is not one hex digest line", out)
			}
		})
	}
}

func TestCommandList(t *testing.T) {
	for name, c := range map[string]struct {
		tree map[string]string
		want string
	}{
		"listed": {listedTree, "a/bc/kept.go\ndigest-exclude\ngo.mod\nmain.go\n"},
		"plain":  {plainTree, "cmd/tool/main.go\ngo.mod\ninternal/x/x.go\ninternal/x/y.go\n"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeTree(t, c.tree)
			code, out, errOut := runCommand(t, "--list", dir)
			if code != 0 || errOut != "" {
				t.Fatalf("exit %d stderr %q", code, errOut)
			}
			if out != c.want {
				t.Fatalf("stdout %q, want %q", out, c.want)
			}
		})
	}
}

func TestCommandDefaultDirectory(t *testing.T) {
	dir := writeTree(t, listedTree)
	digest, err := hookprovenance.SourceDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if code, out, errOut := runCommand(t); code != 0 || errOut != "" || out != digest+"\n" {
		t.Fatalf("digest: exit %d stdout %q stderr %q, want %q", code, out, errOut, digest+"\n")
	}
	want := "a/bc/kept.go\ndigest-exclude\ngo.mod\nmain.go\n"
	if code, out, errOut := runCommand(t, "--list"); code != 0 || errOut != "" || out != want {
		t.Fatalf("list: exit %d stdout %q stderr %q, want %q", code, out, errOut, want)
	}
}

func TestCommandUsage(t *testing.T) {
	dir := writeTree(t, listedTree)
	for name, args := range map[string][]string{
		"second-dir":        {dir, dir},
		"list-second-dir":   {"--list", dir, dir},
		"unknown-option":    {"--lst"},
		"unknown-with-dir":  {"--lst", dir},
		"short-option":      {"-l"},
		"list-second-place": {dir, "--list"},
		"list-twice":        {"--list", "--list"},
		"option-after-list": {"--list", "--lst"},
		"double-dash":       {"--"},
	} {
		t.Run(name, func(t *testing.T) {
			code, out, errOut := runCommand(t, args...)
			if code != 2 || out != "" || errOut != wantUsage {
				t.Fatalf("exit %d stdout %q stderr %q, want exit 2 and %q", code, out, errOut, wantUsage)
			}
		})
	}
}

func TestCommandMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for _, args := range [][]string{{missing}, {"--list", missing}} {
		code, out, errOut := runCommand(t, args...)
		if code != 1 || out != "" || !strings.Contains(errOut, missing) {
			t.Errorf("%q: exit %d stdout %q stderr %q, want exit 1 naming %s", args, code, out, errOut, missing)
		}
	}
}

func TestCommandNotADirectory(t *testing.T) {
	file := filepath.Join(writeTree(t, listedTree), "go.mod")
	for _, args := range [][]string{{file}, {"--list", file}} {
		code, out, errOut := runCommand(t, args...)
		if code != 1 || out != "" || !strings.Contains(errOut, "is not a directory") {
			t.Errorf("%q: exit %d stdout %q stderr %q, want exit 1 and a not-a-directory error", args, code, out, errOut)
		}
	}
}

func TestCommandMalformedList(t *testing.T) {
	dir := writeTree(t, malformedTree)
	for name, args := range map[string][]string{"digest": {dir}, "list": {"--list", dir}} {
		t.Run(name, func(t *testing.T) {
			code, out, errOut := runCommand(t, args...)
			if code != 1 || out != "" {
				t.Fatalf("exit %d stdout %q, want exit 1 and empty stdout", code, out)
			}
			if !strings.Contains(errOut, malformedLine) || strings.Count(errOut, "\n") != 1 {
				t.Fatalf("stderr %q, want one line naming %s", errOut, malformedLine)
			}
		})
	}
}

// runCommand runs the command in-process and returns its exit code, stdout
// and stderr.
func runCommand(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// writeTree writes each slash-relative path of files under a fresh temporary
// directory and returns it. Bodies are written byte for byte, so every
// fixture is LF regardless of the checkout's line endings.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		dest := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
