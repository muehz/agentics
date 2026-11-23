package flow

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// cellRows gives each of rows its evidence cell, as a trail reads it, and no
// reason, so each fits a form.
func cellRows(rows ...stateRow) []trailRow {
	read := make([]trailRow, len(rows))
	for i, r := range rows {
		read[i] = trailRow{stateRow: r, cell: cellOf(r)}
	}
	return read
}

// A user's format.pretty must not reach the patches patch-id reads: without a
// commit header per patch, patch-id cannot tell which commit an id belongs to.
func TestPatchIDsIgnoreUserPrettyFormat(t *testing.T) {
	repo := newRepo(t)
	base := gitOut(t, repo, "rev-parse", "HEAD")
	writeFile(t, repo, "src/one.txt", "one\n")
	commitAll(t, repo, "one")
	writeFile(t, repo, "src/two.txt", "two\n")
	commitAll(t, repo, "two")
	shas := strings.Fields(gitOut(t, repo, "rev-list", "--reverse", "HEAD", "--not", base))
	want := make([]string, len(shas))
	for i, sha := range shas {
		if want[i] = patchIDOf(repo, sha); want[i] == "" {
			t.Fatalf("no patch-id for %s without the setting", sha)
		}
	}
	// Production Git processes inherit the test's environment.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "format.pretty")
	t.Setenv("GIT_CONFIG_VALUE_0", "format:%s")
	commits := rangeCommits(repo, "HEAD", base)
	if len(commits) != len(shas) {
		t.Fatalf("rangeCommits listed %d commits, want %d", len(commits), len(shas))
	}
	for i, commit := range commits {
		if commit.sha != shas[i] || commit.patchID != want[i] {
			t.Errorf("commit %d = %s patch-id %q, want %s patch-id %q", i, commit.sha, commit.patchID, shas[i], want[i])
		}
		if got := patchIDOf(repo, shas[i]); got != want[i] {
			t.Errorf("patchIDOf(%s) = %q, want %q", shas[i], got, want[i])
		}
	}
}

// A user's diff settings must not reach the patches patch-id reads: colored
// output leaves patch-id nothing to hash, so auto refuses an ordinary commit,
// and a textconv filter changes the id one change gets.
func TestPatchIDsIgnoreUserDiffSettings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		settings [][2]string
	}{
		{"color-ui-always", [][2]string{{"color.ui", "always"}}},
		{"textconv", [][2]string{{"diff.upper.textconv", "sed s/one/ONE/"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			folder := "docs/specs/2026-11/feature-x"
			base := gitOut(t, repo, "rev-parse", "HEAD")
			writeFile(t, repo, ".gitattributes", "*.txt diff=upper\n")
			writeFile(t, repo, "src/one.txt", "one\n")
			commitAll(t, repo, "one")
			head := gitOut(t, repo, "rev-parse", "HEAD")
			want := patchIDOf(repo, head)
			if want == "" {
				t.Fatalf("no patch-id for %s without the settings", head)
			}
			for _, setting := range tc.settings {
				git(t, repo, "config", setting[0], setting[1])
			}

			logOK(t, repo, folder, "execute", "verify", "task-1", "ok", "recorded", "auto", "execute go")
			logOK(t, repo, folder, "execute", "verify", "task-2", "ok", "recorded", "auto:"+head, "execute go")
			lines := dataLines(readLines(t, filepath.Join(repo, folder, "state.tsv")))
			for _, line := range lines {
				if got, wantCell := cells(line)[7], "commit="+head+" patch-id="+want; got != wantCell {
					t.Errorf("auto evidence %q, want %q", got, wantCell)
				}
			}

			commits := rangeCommits(repo, "HEAD", base)
			if len(commits) != 1 || commits[0].sha != head || commits[0].patchID != want {
				t.Fatalf("rangeCommits = %+v, want %s with patch-id %s", commits, head, want)
			}
			// A row naming the change by its patch-id alone still accounts for it.
			byPatch := cellRows(stateRow{"2026-11-01T11:00:00Z", "s1", "execute", "verify", "task-1", "ok", "green", "patch-id=" + want, "execute go"})
			noRanges := func(base, head string) map[string]bool { return nil }
			if open := unaccountedCommits(commits, byPatch, folder, noRanges); len(open) != 0 {
				t.Errorf("unaccountedCommits = %q, want none", open)
			}
		})
	}
}

func TestUnaccountedCommitsPrefixMustBeUnique(t *testing.T) {
	t.Parallel()
	const first = "abcdef1000000000000000000000000000000000"
	const second = "abcdef2000000000000000000000000000000000"
	commits := []rangeCommit{
		{sha: first, paths: []string{"src/first.txt"}},
		{sha: second, paths: []string{"src/second.txt"}},
	}
	naming := func(value string) []trailRow {
		return cellRows(stateRow{"2026-11-01T11:00:00Z", "s1", "execute", "verify", "task-1", "ok", "green", "commit=" + value, "execute go"})
	}
	noRanges := func(base, head string) map[string]bool {
		t.Fatalf("no row has a range, but inRange was called with %s..%s", base, head)
		return nil
	}
	for _, tc := range []struct {
		name  string
		value string
		want  []string
	}{
		{"a-shared-prefix-names-neither", "abcdef", []string{first, second}},
		{"a-unique-prefix-names-its-commit-only", "abcdef2", []string{first}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := unaccountedCommits(commits, naming(tc.value), "docs/specs/2026-11/x", noRanges)
			if !slices.Equal(got, tc.want) {
				t.Errorf("unaccountedCommits with commit=%s = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}
