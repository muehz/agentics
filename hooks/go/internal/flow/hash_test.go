package flow

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func hashExpected(t *testing.T, repo, path, body string) string {
	t.Helper()
	cmd := exec.Command("git", "hash-object", "--stdin", "--path="+path)
	cmd.Dir, cmd.Env, cmd.Stdin = repo, baseEnv(), strings.NewReader(body)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hash expected: %v: %s", err, out)
	}
	return strings.TrimRight(string(out), "\n")
}

// firstRuleHash is the first-rule hash of artifact in the flow folder below
// repo, read and named where it lies. folder is relative to repo.
func firstRuleHash(repo, folder, artifact string) string {
	dir := filepath.Join(repo, folder)
	return contentHashAt(repo, dir, dir, artifact)
}

func assertHash(t *testing.T, artifact, body, kept string) {
	t.Helper()
	repo := newRepo(t)
	writeFile(t, repo, "fa/"+artifact, body)
	want := hashExpected(t, repo, "fa/"+artifact, kept)
	if got := firstRuleHash(repo, "fa", artifact); got != want {
		t.Fatalf("hash %s: got %q want %q (kept %q)", artifact, got, want, kept)
	}
}

func TestPowerShellHashesPlanBytesLikeBash(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, body, kept string }{
		{"bom", "\xEF\xBB\xBF# Plan\n\n## Tasks\n\nbody\n\n## Divergences\n\nnone\n", "\xEF\xBB\xBF# Plan\n\n## Tasks\n\nbody\n\n"},
		{"invalid utf8", "# Plan\n\n## Tasks\n\nbad \xFF\xFE byte\n\n## Standing orders\n\nx\n\n## After\n\ny\n", "# Plan\n\n## Tasks\n\nbad \xFF\xFE byte\n\n## After\n\ny\n"},
		{"crlf", "# Plan\r\n\r\n## Tasks\r\n\r\nbody\r\n\r\n## Divergences\r\n\r\nnone\r\n", "# Plan\r\n\r\n## Tasks\r\n\r\nbody\r\n\r\n"},
		{"no final newline", "# Plan\n\n## Tasks\n\nbody", "# Plan\n\n## Tasks\n\nbody\n"},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertHash(t, "plan.md", tc.body, tc.kept)
		})
	}
}

func TestPowerShellHashesIntentAndSpecLikeBash(t *testing.T) {
	t.Parallel()
	for _, artifact := range []string{"intent.md", "spec.md"} {
		for _, tc := range []struct{ name, body, kept string }{
			{"bom", "\xEF\xBB\xBF# Doc\n\n## Constraints\n\nbody\n\n## Divergences\n\nnone\n", "\xEF\xBB\xBF# Doc\n\n## Constraints\n\nbody\n"},
			{"invalid utf8", "# Doc\n\n## Constraints\n\nbad \xFF\xFE byte\n\n## Divergences\n\nx\n\n## After\n\ny\n", "# Doc\n\n## Constraints\n\nbad \xFF\xFE byte\n\n## After\n\ny\n"},
			{"crlf", "# Doc\r\n\r\n## Constraints\r\n\r\nbody\r\n\r\n## Divergences\r\n\r\nnone\r\n", "# Doc\r\n\r\n## Constraints\r\n\r\nbody\r\n"},
			{"standing orders", "# Doc\n\n## Standing orders\n\nkept\n\n## Divergences\n\ndropped\n", "# Doc\n\n## Standing orders\n\nkept\n"},
			{"no final newline", "# Doc\n\n## Constraints\n\nbody", "# Doc\n\n## Constraints\n\nbody\n"},
			{"empty", "", ""},
			{"new section appended", "# Doc\n\n## Constraints\n\nbody\n\n## Divergences\n\n- entry\n", "# Doc\n\n## Constraints\n\nbody\n"},
			{"trailing blank lines dropped without a section", "# Doc\n\n## Constraints\n\nbody\n\n\n", "# Doc\n\n## Constraints\n\nbody\n"},
		} {
			t.Run(artifact+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				assertHash(t, artifact, tc.body, tc.kept)
			})
		}
	}
}

func TestScriptsHashTrailingBlankLinesAcrossFirstDivergencesSection(t *testing.T) {
	t.Parallel()
	for _, artifact := range []string{"intent.md", "spec.md"} {
		for _, eol := range []string{"\n", "\r\n"} {
			for _, section := range []string{"", "\n## Divergences\n\n- entry\n"} {
				assertHash(t, artifact, strings.ReplaceAll("# Doc\n\n## Constraints\n\nbody\n\n\n"+section, "\n", eol), strings.ReplaceAll("# Doc\n\n## Constraints\n\nbody\n", "\n", eol))
			}
		}
	}
}

func TestApprovalHashExactRecords(t *testing.T) {
	t.Parallel()
	assertHash(t, "spec.md", "body\r\n \n\t\n\u00a0\n\n", "body\r\n \n\t\n\u00a0\n")
	assertHash(t, "spec.md", "body\n## Divergences \nkept\n## Divergences\r\nremoved\n### Child\nremoved\n## After\r\nend\r", "body\n## Divergences \nkept\n## After\r\nend\r\n")
	assertHash(t, "plan.md", "body\n\n", "body\n\n")
	assertHash(t, "notes.md", "## Divergences\r\nraw\xFF", "## Divergences\r\nraw\xFF")
	assertHash(t, "nested/spec.md", "## Divergences\nraw", "## Divergences\nraw")
}

func TestApprovalHashAppliesGitAttributes(t *testing.T) {
	t.Parallel()
	for _, artifact := range []string{"plan.md", "intent.md", "spec.md", "notes.md"} {
		t.Run(artifact, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			git(t, repo, "config", "filter.approval.clean", "sed s/raw/clean/g")
			git(t, repo, "config", "filter.approval.required", "true")
			writeFile(t, repo, ".gitattributes", "*.md filter=approval\n")
			writeFile(t, repo, "fa/"+artifact, "raw\n")
			want := hashExpected(t, repo, "unfiltered.txt", "clean\n")
			if got := firstRuleHash(repo, "fa", artifact); got != want {
				t.Fatalf("attributes: got %q want %q", got, want)
			}
		})
	}
}

func TestApprovalHashFailures(t *testing.T) {
	repo := newRepo(t)
	if got := firstRuleHash(repo, "missing", "spec.md"); got != "" {
		t.Fatalf("missing file: %q", got)
	}
	writeFile(t, repo, "fa/spec.md", "body\n")
	t.Setenv("PATH", t.TempDir())
	if got := firstRuleHash(repo, "fa", "spec.md"); got != "" {
		t.Fatalf("missing Git: %q", got)
	}
}

func TestApprovalHashFailedAttributeFilter(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "config", "filter.broken.clean", "exit 1")
	git(t, repo, "config", "filter.broken.required", "true")
	writeFile(t, repo, ".gitattributes", "*.md filter=broken\n")
	writeFile(t, repo, "fa/spec.md", "body\n")
	if got := firstRuleHash(repo, "fa", "spec.md"); got != "" {
		t.Fatalf("failed filter: %q", got)
	}
}

func TestPatchIDOfStableFirstField(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "patch.txt", "new content\n")
	writeFile(t, repo, "another.txt", "another change\n")
	commitAll(t, repo, "patch")
	sha := gitOut(t, repo, "rev-parse", "HEAD")
	cmd := exec.Command("git", "patch-id", "--stable")
	cmd.Dir, cmd.Env, cmd.Stdin = repo, baseEnv(), strings.NewReader(git(t, repo, "show", sha))
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Fields(string(out))[0]
	if got := patchIDOf(repo, sha); got != want {
		t.Fatalf("patch id: %q want %q", got, want)
	}
	if got := patchIDOf(repo, "missing-revision"); got != "" {
		t.Fatalf("missing revision: %q", got)
	}
	git(t, repo, "commit", "--allow-empty", "-q", "-m", "empty")
	if got := patchIDOf(repo, "HEAD"); got != "" {
		t.Fatalf("empty commit: %q", got)
	}
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "missing"))
	if got := patchIDOf(repo, sha); got != "" {
		t.Fatalf("failed Git: %q", got)
	}
}

func TestTextHashKeptTextPerArtifact(t *testing.T) {
	t.Parallel()
	const divergences = "## Divergences\n- entry\n"
	const orders = "## Standing orders\n1. order\n"
	for _, tc := range []struct{ name, artifact, body, normalized string }{
		{"empty", "spec.md", "", ""},
		{"only blank lines", "spec.md", "\n \t\r\n\n", ""},
		{"interior blank lines kept", "spec.md", "a\n\n\nb\n", "a\n\n\nb\n"},
		{"leading blanks kept", "spec.md", "  a  \n\tb\t\n", "  a\n\tb\n"},
		{"bare cr inside a line kept", "spec.md", "a\rb\r\n", "a\rb\n"},
		{"divergences left out of brief", "brief.md", "a\n" + divergences + "## After\nb\n", "a\n## After\nb\n"},
		{"standing orders kept in spec", "spec.md", "a\n" + orders, "a\n" + orders},
		{"standing orders kept in intent", "intent.md", "a\n" + orders, "a\n" + orders},
		{"standing orders left out of plan", "plan.md", "a\n" + orders + divergences, "a\n"},
		{"longer heading is not the section", "spec.md", "a\n## Divergences log\nb\n", "a\n## Divergences log\nb\n"},
		{"leading space is not the heading", "spec.md", "a\n ## Divergences\nb\n", "a\n ## Divergences\nb\n"},
		{"fenced heading is not the section", "plan.md", "a\n```md\n## Divergences   \n```\nb\n", "a\n```md\n## Divergences\n```\nb\n"},
		{"tilde fence hides the heading", "spec.md", "a\n~~~~\n## Divergences\n~~~\nstill code\n~~~~\nb\n", "a\n~~~~\n## Divergences\n~~~\nstill code\n~~~~\nb\n"},
		{"inline backticks open no fence", "spec.md", "a\n``` x ```\n" + divergences + "b\n", "a\n``` x ```\n"},
		{"fenced heading does not end the section", "plan.md", "a\n" + divergences + "  ```\n## After\n  ```\n- more\n## Kept\nc\n", "a\n## Kept\nc\n"},
		{"unclosed fence runs to the end", "spec.md", "a\n```\n## Divergences\nb\n", "a\n```\n## Divergences\nb\n"},
		{"other file keeps every section", "notes.md", "a\n" + divergences + orders, "a\n" + divergences + orders},
		{"other file is still normalized", "notes.md", "a \r\n\r\n", "a\n"},
		{"base name names the artifact", "nested/dir/plan.md", "a\n" + orders, "a\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sum := sha256.Sum256([]byte(tc.normalized))
			if got, want := textHash(tc.artifact, []byte(tc.body)), hex.EncodeToString(sum[:]); got != want {
				t.Fatalf("textHash(%q, %q) = %s, want the hash of %q (%s)", tc.artifact, tc.body, got, tc.normalized, want)
			}
		})
	}
}

func TestTextHashNeedsNoGit(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, "fa/spec.md", "body\r\n")
	t.Setenv("PATH", t.TempDir())
	sum := sha256.Sum256([]byte("body\n"))
	if got, ok := fileTextHash(filepath.Join(repo, "fa"), "spec.md"); !ok || got != hex.EncodeToString(sum[:]) {
		t.Fatalf("no Git on PATH: got %q, %v", got, ok)
	}
}

func TestFileTextHashMissingOrDirectoryIsAbsent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "spec.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []string{"spec.md", "plan.md"} {
		if got, ok := fileTextHash(dir, artifact); ok || got != "" {
			t.Fatalf("%s: got %q, %v", artifact, got, ok)
		}
	}
}

func TestApprovalHashDirectoryFails(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	if err := os.Mkdir(filepath.Join(repo, "spec.md"), 0700); err != nil {
		t.Fatal(err)
	}
	if got := firstRuleHash(repo, ".", "spec.md"); got != "" {
		t.Fatalf("directory: %q", got)
	}
}
