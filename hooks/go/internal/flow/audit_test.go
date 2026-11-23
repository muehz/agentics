package flow

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const auditFolder = "docs/specs/2026-11/feature-audit"

// srcPlan is a plan whose one task lists every path under src/, so a test of
// another check sees no out-of-scope line for the files it commits there.
const srcPlan = "# Plan\n\n## Tasks\n\n### Task 1: Change src\n\n**ID:** task-1\n\n**Files:**\n\n- Modify: `src/`\n"

// writePlan writes body as the audit flow's plan.md.
func writePlan(t *testing.T, repo, body string) {
	t.Helper()
	writeFile(t, repo, auditFolder+"/plan.md", body)
}

// auditRepo returns a repository on branch topic, forked from main, with a
// flow bound to topic whose plan covers src/, and the sha of main. The work
// root is excluded from Git, so an ordinary commit holds no flow file and a
// test commits one only on purpose.
func auditRepo(t *testing.T) (repo, base string) {
	t.Helper()
	repo = newRepo(t)
	writeFile(t, repo, ".git/info/exclude", "docs/specs/\n")
	git(t, repo, "switch", "-q", "-c", "topic")
	seedFlow(t, repo, auditFolder, "topic")
	writePlan(t, repo, srcPlan)
	return repo, gitOut(t, repo, "rev-parse", "main")
}

// commitFile writes content to rel, commits everything with msg, and returns
// the new commit's sha.
func commitFile(t *testing.T, repo, rel, content, msg string) string {
	t.Helper()
	writeFile(t, repo, rel, content)
	commitAll(t, repo, msg)
	return gitOut(t, repo, "rev-parse", "HEAD")
}

// appendRows appends rows to the audit flow's state.tsv as a hand would.
func appendRows(t *testing.T, repo string, rows ...string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(repo, filepath.FromSlash(auditFolder), "state.tsv"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, r := range rows {
		if _, err := fmt.Fprintln(f, r); err != nil {
			t.Fatal(err)
		}
	}
}

// auditOf runs flow audit with args, requires exit 0 and no stderr, and
// returns its stdout lines.
func auditOf(t *testing.T, repo string, args ...string) []string {
	t.Helper()
	res := runFlow(t, repo, nil, append([]string{"audit"}, args...)...)
	mustExit(t, res, 0, "audit")
	if res.stderr != "" {
		t.Fatalf("audit stderr = %q", res.stderr)
	}
	return strings.Split(strings.TrimRight(res.stdout, "\n"), "\n")
}

func auditFolderOf(t *testing.T, repo, base string) []string {
	t.Helper()
	return auditOf(t, repo, "--folder", auditFolder, "--base", base)
}

func linesWith(lines []string, prefix string) []string {
	var found []string
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			found = append(found, l)
		}
	}
	return found
}

func wantLines(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("lines:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAuditListsEveryUnaccountedCommitWithoutACap(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	var want []string
	for i := 1; i <= 21; i++ {
		sha := commitFile(t, repo, fmt.Sprintf("src/f%02d.txt", i), "x\n", fmt.Sprintf("change %02d", i))
		want = append(want, fmt.Sprintf("unaccounted: %s change %02d", sha[:12], i))
	}
	want = append(want, "audit: 21 findings")
	wantLines(t, auditFolderOf(t, repo, base), want)
	// Without --folder the lookup finds the flow bound to this branch.
	wantLines(t, auditOf(t, repo, "--base", base), want)
}

func TestAuditMarksACommitOnlyAnUnattestedRowNames(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	forged := commitFile(t, repo, "src/a.txt", "a\n", "forged")
	attested := commitFile(t, repo, "src/b.txt", "b\n", "attested")
	appendRows(t, repo, row("2026-11-01T10:00:00Z", "execute", "commit", "user", "ok", "by hand", "commit="+forged, "execute go"))
	logOK(t, repo, auditFolder, "execute", "commit", "user", "ok", "user commit", "auto:"+attested, "execute go")
	wantLines(t, auditFolderOf(t, repo, base), []string{
		"unaccounted: " + forged[:12] + " forged (row not attested here)",
		"audit: 1 findings",
	})
}

// A no-commit cell quotes a failing command, so a commit= inside its cmd=
// text names no commit: the commit stays unaccounted, and since no row names
// it at all, it carries no unattested-row mark.
func TestAuditNoCommitRowNamesNoCommit(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	work := commitFile(t, repo, "src/a.txt", "a\n", "work")
	appendRows(t, repo, row("2026-11-01T10:00:00Z", "execute", "verify", "red-task-1", "failed", "red",
		"no-commit exit=1 cmd=git log --grep commit="+work+" patch-id="+patchIDOf(repo, work)+" out=no match", "execute go"))
	wantLines(t, auditFolderOf(t, repo, base), []string{
		"unaccounted: " + work[:12] + " work",
		"audit: 1 findings",
	})
}

func TestAuditExemptsWhatGitProvesNeedsNoRow(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, ".git/info/exclude", "docs/specs/\n")
	old := commitFile(t, repo, "src/old.txt", "old\n", "old change")
	git(t, repo, "switch", "-q", "-c", "topic")
	seedFlow(t, repo, auditFolder, "topic")
	writePlan(t, repo, srcPlan)
	writeFile(t, repo, auditFolder+"/notes.md", "notes\n")
	git(t, repo, "add", "-f", auditFolder+"/notes.md")
	git(t, repo, "commit", "-q", "-m", "trail")
	trail := gitOut(t, repo, "rev-parse", "HEAD")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "nothing")
	empty := gitOut(t, repo, "rev-parse", "HEAD")
	git(t, repo, "switch", "-q", "main")
	upstream := commitFile(t, repo, "src/up.txt", "up\n", "upstream fix")
	git(t, repo, "switch", "-q", "topic")
	git(t, repo, "cherry-pick", upstream)
	picked := gitOut(t, repo, "rev-parse", "HEAD")
	git(t, repo, "rm", "-q", "src/old.txt")
	git(t, repo, "commit", "-q", "-m", "drop old")
	drop := gitOut(t, repo, "rev-parse", "HEAD")
	// A change older than the merge base is no change of the base's new
	// commits, so applying it again needs a row.
	git(t, repo, "cherry-pick", old)
	reapplied := gitOut(t, repo, "rev-parse", "HEAD")
	wantLines(t, auditFolderOf(t, repo, gitOut(t, repo, "rev-parse", "main")), []string{
		"unaccounted: " + drop[:12] + " drop old",
		"unaccounted: " + reapplied[:12] + " old change",
		"note: exempt (work-folder) " + trail[:12],
		"note: exempt (empty) " + empty[:12],
		"note: exempt (from-base) " + picked[:12],
		"audit: 2 findings",
	})
}

func TestAuditCleanRebaseOntoAMovedBase(t *testing.T) {
	t.Parallel()
	repo, _ := auditRepo(t)
	a := commitFile(t, repo, "src/a.txt", "a\n", "a")
	b := commitFile(t, repo, "src/b.txt", "b\n", "b")
	logOK(t, repo, auditFolder, "execute", "verify", "task-1", "ok", "tests pass", "auto:"+a, "execute go")
	logOK(t, repo, auditFolder, "execute", "verify", "task-2", "ok", "tests pass", "auto:"+b, "execute go")
	git(t, repo, "switch", "-q", "main")
	commitFile(t, repo, "src/m.txt", "m\n", "moved")
	git(t, repo, "switch", "-q", "topic")
	git(t, repo, "rebase", "-q", "main")
	wantLines(t, auditFolderOf(t, repo, gitOut(t, repo, "rev-parse", "main")), []string{"audit: clean"})
}

func TestAuditReportsACommitAConflictingRebaseRewrote(t *testing.T) {
	t.Parallel()
	repo, _ := auditRepo(t)
	a := commitFile(t, repo, "src/f.txt", "a\n", "a")
	logOK(t, repo, auditFolder, "execute", "verify", "task-1", "ok", "tests pass", "auto:"+a, "execute go")
	id := patchIDOf(repo, a)
	git(t, repo, "switch", "-q", "main")
	commitFile(t, repo, "src/f.txt", "m\n", "moved")
	// A conflicting rebase replays the commit with the resolution in it.
	git(t, repo, "switch", "-q", "topic")
	git(t, repo, "reset", "-q", "--hard", "main")
	rewritten := commitFile(t, repo, "src/f.txt", "m\na\n", "a")
	wantLines(t, auditFolderOf(t, repo, gitOut(t, repo, "rev-parse", "main")), []string{
		"unaccounted: " + rewritten[:12] + " a",
		"mismatch: verify task-1 commit " + a + " is not an ancestor of HEAD and patch-id " + id + " matches no commit on this branch",
		"audit: 2 findings",
	})
}

func TestAuditSkipsRowsOfALowerLayer(t *testing.T) {
	t.Parallel()
	repo, _ := auditRepo(t)
	git(t, repo, "switch", "-q", "-c", "side", "main")
	side := commitFile(t, repo, "src/y.txt", "y\n", "y")
	git(t, repo, "switch", "-q", "-c", "lower", "main")
	lower := commitFile(t, repo, "src/x.txt", "x\n", "x")
	git(t, repo, "cherry-pick", side)
	git(t, repo, "switch", "-q", "topic")
	// One row names a commit under the base, the other a commit whose change
	// the base holds by another sha; neither commit is on this branch.
	logOK(t, repo, auditFolder, "execute", "verify", "task-1", "ok", "tests pass", "auto:"+lower, "execute go")
	logOK(t, repo, auditFolder, "execute", "verify", "task-2", "ok", "tests pass", "auto:"+side, "execute go")
	wantLines(t, auditFolderOf(t, repo, gitOut(t, repo, "rev-parse", "lower")), []string{"audit: clean"})
}

func TestAuditReportsUnsupersededInvalidVerifyRows(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	appendRows(t, repo,
		row("2026-11-01T10:00:00Z", "execute", "verify", "task-1", "ok", "bad", "commit=zz", "execute go"),
		row("2026-11-01T10:00:01Z", "execute", "verify", "task-2", "ok", "bad", "commit=zz", "execute go"),
		row("2026-11-01T10:00:03Z", "execute", "waive", "bogus", "ok", "bad", "none", "execute go"),
	)
	// A later valid row for task-2 supersedes its invalid one.
	logOK(t, repo, auditFolder, "execute", "verify", "task-2", "ok", "fixed", "auto", "execute go")
	status := linesWith(statusOf(t, repo, auditFolder), "invalid:")
	var want []string
	for _, line := range status {
		if strings.Contains(line, " verify task-1: ") {
			want = append(want, line)
		}
	}
	if len(want) != 1 {
		t.Fatalf("status invalid lines: %q", status)
	}
	got := auditFolderOf(t, repo, base)
	wantLines(t, got, append(want, "audit: 1 findings"))
}

func TestAuditNotesAMergeFromTheBase(t *testing.T) {
	t.Parallel()
	repo, _ := auditRepo(t)
	git(t, repo, "switch", "-q", "main")
	commitFile(t, repo, "src/m.txt", "m\n", "moved")
	git(t, repo, "switch", "-q", "topic")
	git(t, repo, "merge", "-q", "--no-ff", "-m", "merge main", "main")
	merge := gitOut(t, repo, "rev-parse", "HEAD")
	wantLines(t, auditFolderOf(t, repo, gitOut(t, repo, "rev-parse", "main")), []string{
		"note: merge commit " + merge[:12] + " not audited",
		"audit: clean",
	})
}

// A pair run records the same commit evidence as any other mode, so no mode
// waives the audit.
func TestAuditRunsInPairMode(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	appendRows(t, repo, row("2026-11-01T10:00:00Z", "execute", "enter", "run", "ok", "pairing", "mode=pair branch=topic", "execute go"))
	hand := commitFile(t, repo, "src/a.txt", "a\n", "a")
	wantLines(t, auditFolderOf(t, repo, base), []string{
		"unaccounted: " + hand[:12] + " a",
		"audit: 1 findings",
	})
}

func TestAuditDoesNotRunForAnUnboundBranch(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "topic")
	seedFlow(t, repo, auditFolder, "")
	commitFile(t, repo, "src/a.txt", "a\n", "a")
	wantLines(t, auditFolderOf(t, repo, gitOut(t, repo, "rev-parse", "main")), []string{"audit: not run (this branch is not bound to the flow)"})
}

func TestAuditRefusesABaseThatIsNotACommit(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	// A hex-looking branch name resolves, but to a commit its name does not
	// abbreviate.
	git(t, repo, "branch", "abcdef12", "main")
	for _, args := range [][]string{
		{"--folder", auditFolder},
		{"--folder", auditFolder, "--base", "nope"},
		{"--folder", auditFolder, "--base", "-x"},
		{"--folder", auditFolder, "--base", "abcdef12"},
	} {
		res := runFlow(t, repo, nil, append([]string{"audit"}, args...)...)
		mustExit(t, res, 2, strings.Join(args, " "))
		if res.stdout != "" || res.stderr == "" {
			t.Fatalf("%v: stdout=%q stderr=%q", args, res.stdout, res.stderr)
		}
	}
	// An abbreviated sha names its own commit.
	wantLines(t, auditFolderOf(t, repo, strings.ToUpper(base[:12])), []string{"audit: clean"})
}

func TestAuditUsageErrors(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	for _, args := range [][]string{
		{"--base", base, "--bogus"},
		{"--base"},
		{"--base", base, "--folder"},
	} {
		res := runFlow(t, repo, nil, append([]string{"audit"}, args...)...)
		mustExit(t, res, 1, strings.Join(args, " "))
		if res.stdout != "" || res.stderr != usage {
			t.Fatalf("%v: stdout=%q stderr=%q, want usage", args, res.stdout, res.stderr)
		}
	}
}

func TestAuditPassesLookupFailuresThrough(t *testing.T) {
	t.Parallel()
	t.Run("no-flow", func(t *testing.T) {
		t.Parallel()
		repo := newRepo(t)
		git(t, repo, "switch", "-q", "-c", "topic")
		res := runFlow(t, repo, nil, "audit", "--base", "main")
		mustExit(t, res, 3, "audit without a flow")
		if res.stdout != "" || res.stderr != "flow: no flow found\n" {
			t.Fatalf("stdout=%q stderr=%q", res.stdout, res.stderr)
		}
	})
	t.Run("ambiguous", func(t *testing.T) {
		t.Parallel()
		repo := newRepo(t)
		git(t, repo, "switch", "-q", "-c", "topic")
		lookupSeed(t, repo, flowA, "")
		lookupSeed(t, repo, flowB, "")
		commitAll(t, repo, "two flows")
		res := runFlow(t, repo, nil, "audit", "--base", "main")
		mustExit(t, res, 4, "audit with two flows")
		if res.stdout != "" || !strings.Contains(res.stderr, "feature-a") || !strings.Contains(res.stderr, "feature-b") {
			t.Fatalf("stdout=%q stderr=%q", res.stdout, res.stderr)
		}
	})
	// A pivot ends the small fix, so the branch has neither a record nor a
	// flow.
	t.Run("closed-small-fix", func(t *testing.T) {
		t.Parallel()
		repo, _ := smallFixRepo(t, "fix/a")
		logOK(t, repo, unassignedFolder, "execute", "pivot", "fix fix/a", "ok", "moved into the chain", "none", "intent go")
		res := runFlow(t, repo, nil, "audit", "--base", "main")
		mustExit(t, res, 3, "audit after the pivot")
		if res.stdout != "" || res.stderr != "flow: no flow found\n" {
			t.Fatalf("stdout=%q stderr=%q", res.stdout, res.stderr)
		}
	})
}

const (
	unassignedFolder = "docs/specs/unassigned"
	smallFixNote     = "note: scope not checked (small fix: the size signal applies)"
)

// smallFixRepo returns a repository on branch, forked from main, with an open
// small-fix record for branch whose skip row records main as its base, and
// the sha of main. The work root is excluded from Git, as in auditRepo.
func smallFixRepo(t *testing.T, branch string) (repo, base string) {
	t.Helper()
	repo = newRepo(t)
	writeFile(t, repo, ".git/info/exclude", "docs/specs/\n")
	base = gitOut(t, repo, "rev-parse", "main")
	git(t, repo, "switch", "-q", "-c", branch)
	logSkip(t, repo, branch, base)
	return repo, base
}

// logSkip writes the skip chain row that opens a small fix on branch.
func logSkip(t *testing.T, repo, branch, base string) {
	t.Helper()
	logOK(t, repo, unassignedFolder, "execute", "skip", "chain "+branch, "ok", "bug fix: one concern", "base="+base, "execute go")
}

// logVerifyFix writes a verify fix row with status for commit sha.
func logVerifyFix(t *testing.T, repo, status, sha string) {
	t.Helper()
	logOK(t, repo, unassignedFolder, "execute", "verify", "fix", status, "fix checked", "auto:"+sha, "gate go")
}

// appendUnassigned appends rows to the committed unassigned state.tsv as a
// hand would.
func appendUnassigned(t *testing.T, repo string, rows ...string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(repo, filepath.FromSlash(unassignedFolder), "state.tsv"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, r := range rows {
		if _, err := fmt.Fprintln(f, r); err != nil {
			t.Fatal(err)
		}
	}
}

// nowStamp is a row timestamp for the current second, so a hand-written row
// sorts after the rows flow log wrote before it.
func nowStamp() string { return time.Now().UTC().Format("2006-01-02T15:04:05Z") }

// Two branches share the unassigned file, so each audit reads only its own
// branch's record, and a commit row flow log writes accounts for a hand
// commit.
func TestAuditSmallFixRecordOfInterleavedBranches(t *testing.T) {
	t.Parallel()
	repo, base := smallFixRepo(t, "fix/a")
	git(t, repo, "switch", "-q", "-c", "fix/b", "main")
	logSkip(t, repo, "fix/b", base)
	other := commitFile(t, repo, "src/b.txt", "b\n", "fix b")
	logVerifyFix(t, repo, "ok", other)
	git(t, repo, "switch", "-q", "fix/a")
	fix := commitFile(t, repo, "src/a.txt", "a\n", "fix a")
	logVerifyFix(t, repo, "ok", fix)
	hand := commitFile(t, repo, "src/hand.txt", "h\n", "hand change")
	// A failed fix row of the other branch names that branch, not this one.
	appendUnassigned(t, repo, row(nowStamp(), "execute", "verify", "fix", "failed", "b broke", "branch=fix/b patch-id="+strings.Repeat("0", 40), "execute go"))
	wantLines(t, auditOf(t, repo, "--base", base), []string{
		"unaccounted: " + hand[:12] + " hand change",
		smallFixNote,
		"audit: 1 findings",
	})
	logOK(t, repo, unassignedFolder, "execute", "commit", "user", "ok", "user commit", "auto:"+hand, "gate go")
	wantLines(t, auditOf(t, repo, "--base", base), []string{smallFixNote, "audit: clean"})
	git(t, repo, "switch", "-q", "fix/b")
	wantLines(t, auditOf(t, repo, "--base", base), []string{smallFixNote, "audit: clean"})
}

func TestAuditSmallFixRecord(t *testing.T) {
	t.Parallel()
	// Rows before the latest skip row belong to an earlier record, which a
	// pivot closed.
	t.Run("reopened", func(t *testing.T) {
		t.Parallel()
		repo, base := smallFixRepo(t, "fix/a")
		fix := commitFile(t, repo, "src/a.txt", "a\n", "fix a")
		logVerifyFix(t, repo, "ok", fix)
		logOK(t, repo, unassignedFolder, "execute", "pivot", "fix fix/a", "ok", "moved into the chain", "none", "intent go")
		// A skip row identical to the first counts once, so the reopening row
		// differs from it even when both land in the same second.
		logOK(t, repo, unassignedFolder, "execute", "skip", "chain fix/a", "ok", "bug fix: reopened", "base="+base, "execute go")
		wantLines(t, auditOf(t, repo, "--base", base), []string{
			"unaccounted: " + fix[:12] + " fix a",
			smallFixNote,
			"audit: 1 findings",
		})
	})
	// A red-fix row names its commit by an abbreviated sha, which accounts
	// for it even with a patch-id that matches no commit.
	t.Run("red-fix-prefix", func(t *testing.T) {
		t.Parallel()
		repo, base := smallFixRepo(t, "fix/a")
		red := commitFile(t, repo, "src/a_test.txt", "red\n", "red test")
		appendUnassigned(t, repo, row(nowStamp(), "execute", "verify", "red-fix", "ok", "fails first", "commit="+red[:12]+" patch-id="+strings.Repeat("0", 40), "execute go"))
		fix := commitFile(t, repo, "src/a.txt", "a\n", "fix a")
		logVerifyFix(t, repo, "ok", fix)
		wantLines(t, auditOf(t, repo, "--base", base), []string{smallFixNote, "audit: clean"})
	})
	// A fix row has no integrated form, so such a row is invalid, names no
	// commit and, carrying no commit= or branch=, is not in the record.
	t.Run("integrated", func(t *testing.T) {
		t.Parallel()
		repo, base := smallFixRepo(t, "fix/a")
		fix := commitFile(t, repo, "src/a.txt", "a\n", "fix a")
		appendUnassigned(t, repo, row(nowStamp(), "execute", "verify", "fix", "ok", "fixed", "integrated="+fix, "gate go"))
		wantLines(t, auditOf(t, repo, "--base", base), []string{
			"unaccounted: " + fix[:12] + " fix a",
			smallFixNote,
			"audit: 1 findings",
		})
	})
	// Only a passing fix row accounts for its commit, though the failed row
	// stays in the record and its commit still matches.
	t.Run("failed-verify", func(t *testing.T) {
		t.Parallel()
		repo, base := smallFixRepo(t, "fix/a")
		fix := commitFile(t, repo, "src/a.txt", "a\n", "fix a")
		logVerifyFix(t, repo, "failed", fix)
		wantLines(t, auditOf(t, repo, "--base", base), []string{
			"unaccounted: " + fix[:12] + " fix a",
			smallFixNote,
			"audit: 1 findings",
		})
		logVerifyFix(t, repo, "ok", fix)
		wantLines(t, auditOf(t, repo, "--base", base), []string{smallFixNote, "audit: clean"})
	})
	// A verify row that is not from execute, such as one gate wrote, does
	// not account for a commit.
	t.Run("not-execute", func(t *testing.T) {
		t.Parallel()
		repo, base := smallFixRepo(t, "fix/a")
		fix := commitFile(t, repo, "src/a.txt", "a\n", "fix a")
		logOK(t, repo, unassignedFolder, "gate", "verify", "fix", "ok", "fix checked", "auto:"+fix, "gate go")
		wantLines(t, auditOf(t, repo, "--base", base), []string{
			"unaccounted: " + fix[:12] + " fix a",
			smallFixNote,
			"audit: 1 findings",
		})
	})
	t.Run("unattested-commit-row", func(t *testing.T) {
		t.Parallel()
		repo, base := smallFixRepo(t, "fix/a")
		hand := commitFile(t, repo, "src/hand.txt", "h\n", "hand change")
		appendUnassigned(t, repo, row(nowStamp(), "execute", "commit", "user", "ok", "by hand", "commit="+hand, "gate go"))
		wantLines(t, auditOf(t, repo, "--base", base), []string{
			"unaccounted: " + hand[:12] + " hand change (row not attested here)",
			smallFixNote,
			"audit: 1 findings",
		})
	})
	// The ledger records an attested commit row's sha alone, so a row that
	// borrows that sha to carry another commit's patch-id= or a range
	// accounts for nothing.
	for _, borrowed := range []string{"patch-id", "range"} {
		t.Run("borrowed-"+borrowed, func(t *testing.T) {
			t.Parallel()
			repo, base := smallFixRepo(t, "fix/a")
			hand := commitFile(t, repo, "src/hand.txt", "h\n", "hand change")
			logOK(t, repo, unassignedFolder, "execute", "commit", "user", "ok", "user commit", "auto:"+hand, "gate go")
			other := commitFile(t, repo, "src/other.txt", "o\n", "other change")
			extra := "base=" + base + " head=" + other
			if borrowed == "patch-id" {
				extra = "patch-id=" + patchIDOf(repo, other)
			}
			appendUnassigned(t, repo, row(nowStamp(), "execute", "commit", "user", "ok", "by hand", "commit="+hand+" "+extra, "gate go"))
			lines := auditOf(t, repo, "--base", base)
			if !slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, "unaccounted: "+other[:12]+" other change") }) ||
				!slices.Contains(lines, "audit: 1 findings") {
				t.Fatalf("audit = %q, want other change unaccounted and 1 finding", lines)
			}
		})
	}
	// A conflicting rebase rewrites the fix commit with a new patch-id, so its
	// row no longer names a commit of the branch and leaves the record: the
	// rewritten commit is unaccounted rather than the row a mismatch.
	t.Run("conflicting-rebase", func(t *testing.T) {
		t.Parallel()
		repo, _ := smallFixRepo(t, "fix/a")
		fix := commitFile(t, repo, "src/a.txt", "fix\n", "fix a")
		logVerifyFix(t, repo, "ok", fix)
		git(t, repo, "switch", "-q", "main")
		moved := commitFile(t, repo, "src/a.txt", "main\n", "main a")
		git(t, repo, "switch", "-q", "fix/a")
		if res := runCmd(t, repo, "git", []string{"rebase", "-q", "main"}, baseEnv()); res.code == 0 {
			t.Fatal("rebase did not conflict")
		}
		writeFile(t, repo, "src/a.txt", "main\nfix\n")
		git(t, repo, "add", "src/a.txt")
		git(t, repo, "-c", "core.editor=true", "rebase", "--continue")
		rebased := gitOut(t, repo, "rev-parse", "HEAD")
		wantLines(t, auditOf(t, repo, "--base", moved), []string{
			"unaccounted: " + rebased[:12] + " fix a",
			smallFixNote,
			"audit: 1 findings",
		})
	})
	// A clean rebase gives the hand commit a new sha but keeps its patch-id,
	// so its attested commit row stays in the record and still accounts for
	// it.
	t.Run("clean-rebase", func(t *testing.T) {
		t.Parallel()
		repo, _ := smallFixRepo(t, "fix/a")
		hand := commitFile(t, repo, "src/hand.txt", "h\n", "hand change")
		logOK(t, repo, unassignedFolder, "execute", "commit", "user", "ok", "user commit", "auto:"+hand, "gate go")
		git(t, repo, "switch", "-q", "main")
		moved := commitFile(t, repo, "src/other.txt", "o\n", "unrelated")
		git(t, repo, "switch", "-q", "fix/a")
		git(t, repo, "rebase", "-q", "main")
		if gitOut(t, repo, "rev-parse", "HEAD") == hand {
			t.Fatal("the rebase kept the hand commit's sha")
		}
		wantLines(t, auditOf(t, repo, "--base", moved), []string{smallFixNote, "audit: clean"})
	})
	// A pair-mode row in the unassigned trail changes nothing about a small
	// fix's audit.
	t.Run("pair-mode", func(t *testing.T) {
		t.Parallel()
		repo, base := smallFixRepo(t, "fix/a")
		appendUnassigned(t, repo, row(nowStamp(), "execute", "enter", "run", "ok", "pairing", "mode=pair branch=fix/a", "execute go"))
		hand := commitFile(t, repo, "src/hand.txt", "h\n", "hand change")
		wantLines(t, auditOf(t, repo, "--base", base), []string{
			"unaccounted: " + hand[:12] + " hand change",
			smallFixNote,
			"audit: 1 findings",
		})
	})
	t.Run("bound-flow-wins", func(t *testing.T) {
		t.Parallel()
		repo, base := smallFixRepo(t, "fix/a")
		seedFlow(t, repo, auditFolder, "fix/a")
		writePlan(t, repo, srcPlan)
		change := commitFile(t, repo, "src/x.txt", "x\n", "change x")
		wantLines(t, auditOf(t, repo, "--base", base), []string{
			"unaccounted: " + change[:12] + " change x",
			"audit: 1 findings",
		})
	})
	// Only the trail file itself is exempt as the work folder; another file
	// under the unassigned folder needs a row.
	t.Run("trail-exemption", func(t *testing.T) {
		t.Parallel()
		repo, base := smallFixRepo(t, "fix/a")
		fix := commitFile(t, repo, "src/a.txt", "a\n", "fix a")
		logVerifyFix(t, repo, "ok", fix)
		git(t, repo, "add", "-f", unassignedFolder+"/state.tsv")
		git(t, repo, "commit", "-q", "-m", "trail")
		trail := gitOut(t, repo, "rev-parse", "HEAD")
		writeFile(t, repo, unassignedFolder+"/notes.md", "n\n")
		git(t, repo, "add", "-f", unassignedFolder+"/notes.md")
		git(t, repo, "commit", "-q", "-m", "notes")
		notes := gitOut(t, repo, "rev-parse", "HEAD")
		wantLines(t, auditOf(t, repo, "--base", base), []string{
			"unaccounted: " + notes[:12] + " notes",
			"note: exempt (work-folder) " + trail[:12],
			smallFixNote,
			"audit: 1 findings",
		})
	})
	// A row whose branch= names another branch stays out of this branch's
	// record even when its commit= names a commit of this branch. A fix row
	// with commit= and branch= fits no form, so only its invalid: line shows
	// whether it joined the record.
	t.Run("other-branch-row", func(t *testing.T) {
		t.Parallel()
		repo, base := smallFixRepo(t, "fix/a")
		fix := commitFile(t, repo, "src/a.txt", "a\n", "fix a")
		appendUnassigned(t, repo, row(nowStamp(), "execute", "verify", "fix", "ok", "fixed", "commit="+fix+" patch-id="+strings.Repeat("0", 40)+" branch=fix/b", "gate go"))
		wantLines(t, auditOf(t, repo, "--base", base), []string{
			"unaccounted: " + fix[:12] + " fix a",
			smallFixNote,
			"audit: 1 findings",
		})
	})
	// An invalid row is in the record when its commit= names a commit of the
	// branch, and out of it when it names another branch's commit.
	t.Run("invalid-rows", func(t *testing.T) {
		t.Parallel()
		repo, base := smallFixRepo(t, "fix/a")
		git(t, repo, "switch", "-q", "-c", "fix/b", "main")
		other := commitFile(t, repo, "src/b.txt", "b\n", "fix b")
		git(t, repo, "switch", "-q", "fix/a")
		fix := commitFile(t, repo, "src/a.txt", "a\n", "fix a")
		stamp := nowStamp()
		appendUnassigned(t, repo,
			row(stamp, "execute", "verify", "fix", "ok", "fixed", "commit="+fix, "gate go"),
			row(stamp, "execute", "verify", "red-fix", "ok", "fails first", "commit="+other, "execute go"))
		got := auditOf(t, repo, "--base", base)
		invalid := linesWith(got, "invalid:")
		if len(invalid) != 1 || !strings.HasPrefix(invalid[0], "invalid: "+stamp+" s1 verify fix: ") {
			t.Fatalf("invalid lines = %q", invalid)
		}
		wantLines(t, got, []string{
			"unaccounted: " + fix[:12] + " fix a",
			invalid[0],
			smallFixNote,
			"audit: 2 findings",
		})
	})
}

// The trail may live in the local store, which the audit reads as it reads
// the committed file.
func TestAuditSmallFixRecordInTheLocalStore(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "config", "agentics.flowStorage", "local")
	base := gitOut(t, repo, "rev-parse", "main")
	git(t, repo, "switch", "-q", "-c", "fix/a")
	logSkip(t, repo, "fix/a", base)
	if _, err := os.Stat(filepath.Join(repo, ".git", "agentics", "work", "docs", "specs", "unassigned", "state.tsv")); err != nil {
		t.Fatalf("the skip row is not in the local store: %v", err)
	}
	fix := commitFile(t, repo, "src/a.txt", "a\n", "fix a")
	logVerifyFix(t, repo, "ok", fix)
	hand := commitFile(t, repo, "src/hand.txt", "h\n", "hand change")
	wantLines(t, auditOf(t, repo, "--base", base), []string{
		"unaccounted: " + hand[:12] + " hand change",
		smallFixNote,
		"audit: 1 findings",
	})
	logOK(t, repo, unassignedFolder, "execute", "commit", "user", "ok", "user commit", "auto:"+hand, "gate go")
	wantLines(t, auditOf(t, repo, "--base", base), []string{smallFixNote, "audit: clean"})
}

func TestAuditFullOutputOrder(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "side")
	side := commitFile(t, repo, "src/s.txt", "s\n", "side")
	git(t, repo, "switch", "-q", "-c", "topic", "main")
	seedFlow(t, repo, auditFolder, "topic")
	// The plan lists only a.txt, so b.txt is out of scope; up.txt is in
	// scope because its one commit is exempt as coming from the base.
	writePlan(t, repo, "# Plan\n\n### Task 1: Add a\n\n**ID:** task-1\n\n**Files:**\n\n- Create: `src/a.txt`\n")
	git(t, repo, "add", "-f", auditFolder+"/state.tsv", auditFolder+"/plan.md")
	git(t, repo, "commit", "-q", "-m", "trail")
	trail := gitOut(t, repo, "rev-parse", "HEAD")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "nothing")
	empty := gitOut(t, repo, "rev-parse", "HEAD")
	git(t, repo, "switch", "-q", "main")
	commitFile(t, repo, "src/m.txt", "m\n", "moved")
	git(t, repo, "switch", "-q", "topic")
	git(t, repo, "merge", "-q", "--no-ff", "-m", "merge main", "main")
	merge := gitOut(t, repo, "rev-parse", "HEAD")
	git(t, repo, "switch", "-q", "main")
	upstream := commitFile(t, repo, "src/up.txt", "up\n", "upstream fix")
	git(t, repo, "switch", "-q", "topic")
	git(t, repo, "cherry-pick", upstream)
	picked := gitOut(t, repo, "rev-parse", "HEAD")
	a := commitFile(t, repo, "src/a.txt", "a\n", "add a")
	b := commitFile(t, repo, "src/b.txt", "b\n", "add b")
	appendRows(t, repo,
		row("2026-11-01T10:00:00Z", "execute", "commit", "user", "ok", "by hand", "commit="+b, "execute go"),
		row("2026-11-01T10:00:01Z", "execute", "verify", "task-9", "ok", "bad", "commit=zz", "execute go"),
	)
	logOK(t, repo, auditFolder, "execute", "verify", "task-1", "ok", "tests pass", "auto:"+side, "execute go")
	invalid := linesWith(statusOf(t, repo, auditFolder), "invalid:")
	if len(invalid) != 1 {
		t.Fatalf("status invalid lines: %q", invalid)
	}
	wantLines(t, auditFolderOf(t, repo, gitOut(t, repo, "rev-parse", "main")), []string{
		"unaccounted: " + a[:12] + " add a",
		"unaccounted: " + b[:12] + " add b (row not attested here)",
		"mismatch: verify task-1 commit " + side + " is not an ancestor of HEAD and patch-id " + patchIDOf(repo, side) + " matches no commit on this branch",
		"out-of-scope: src/b.txt",
		invalid[0],
		"note: exempt (work-folder) " + trail[:12],
		"note: exempt (empty) " + empty[:12],
		"note: exempt (from-base) " + picked[:12],
		"note: merge commit " + merge[:12] + " not audited",
		"audit: 5 findings",
	})
}

// removeObject deletes the loose object sha from repo, as a damaged clone
// would lack it. Git writes objects read-only, which Windows will not delete.
func removeObject(t *testing.T, repo, sha string) {
	t.Helper()
	path := filepath.Join(repo, ".git", "objects", sha[:2], sha[2:])
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

// A Git read that fails must not pass for an empty range: that would print
// audit: clean for a branch the audit never saw.
func TestAuditFailsClosedWhenTheRangeCannotBeRead(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		damage func(t *testing.T, repo, first string)
	}{
		// The listing of the range walks every commit of it.
		{"missing-commit", func(t *testing.T, repo, first string) { removeObject(t, repo, first) }},
		// Only the patch pass reads file contents, so the listing succeeds.
		{"missing-blob", func(t *testing.T, repo, first string) {
			removeObject(t, repo, gitOut(t, repo, "rev-parse", first+":src/a.txt"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo, base := auditRepo(t)
			first := commitFile(t, repo, "src/a.txt", "a\n", "a")
			commitFile(t, repo, "src/b.txt", "b\n", "b")
			tc.damage(t, repo, first)
			res := runFlow(t, repo, nil, "audit", "--folder", auditFolder, "--base", base)
			mustExit(t, res, 1, "audit of an unreadable range")
			if want := "flow: audit cannot read the commits of " + base[:12] + "..HEAD: "; res.stdout != "" || !strings.HasPrefix(res.stderr, want) {
				t.Fatalf("stdout=%q stderr=%q, want no stdout and stderr starting %q", res.stdout, res.stderr, want)
			}
		})
	}
}

// A commit row the ledger does not attest is no record, so a commit under the
// work folder that only such a row names is exempt like any other.
func TestAuditExemptsAWorkFolderCommitOnlyAnUnattestedRowNames(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	writeFile(t, repo, auditFolder+"/notes.md", "notes\n")
	git(t, repo, "add", "-f", auditFolder+"/notes.md")
	git(t, repo, "commit", "-q", "-m", "trail")
	trail := gitOut(t, repo, "rev-parse", "HEAD")
	appendRows(t, repo, row("2026-11-01T10:00:00Z", "execute", "commit", "user", "ok", "by hand", "commit="+trail, "execute go"))
	wantLines(t, auditFolderOf(t, repo, base), []string{
		"note: exempt (work-folder) " + trail[:12],
		"audit: clean",
	})
}

// Histories that share no commit have no merge base, so no commit is exempt
// as coming from the base, even one whose change the base holds.
func TestAuditRunsWithoutAMergeBase(t *testing.T) {
	t.Parallel()
	repo, _ := auditRepo(t)
	a := commitFile(t, repo, "src/a.txt", "a\n", "add a")
	git(t, repo, "switch", "-q", "--orphan", "other")
	commitFile(t, repo, "src/a.txt", "a\n", "add a elsewhere")
	other := gitOut(t, repo, "rev-parse", "HEAD")
	git(t, repo, "switch", "-q", "topic")
	first := gitOut(t, repo, "rev-parse", "main")
	// With no merge base every path of HEAD is a change of the branch.
	wantLines(t, auditFolderOf(t, repo, other), []string{
		"unaccounted: " + first[:12] + " init",
		"unaccounted: " + a[:12] + " add a",
		"out-of-scope: README.md",
		"audit: 3 findings",
	})
}

// When the lower slice of a stack is rewritten after its rows were written
// and this branch is rebased onto the rewritten head, the base and HEAD share
// that head, so the change an old row names lies below their merge base. The
// row still records the lower layer, not a mismatch of this branch.
func TestAuditSkipsRowsOfARewrittenLowerLayer(t *testing.T) {
	t.Parallel()
	repo, _ := auditRepo(t)
	git(t, repo, "switch", "-q", "-c", "lower", "main")
	old := commitFile(t, repo, "src/x.txt", "x\n", "x")
	git(t, repo, "switch", "-q", "topic")
	git(t, repo, "reset", "-q", "--hard", "lower")
	upper := commitFile(t, repo, "src/y.txt", "y\n", "y")
	logOK(t, repo, auditFolder, "execute", "verify", "task-1", "ok", "tests pass", "auto:"+old, "execute go")
	logOK(t, repo, auditFolder, "execute", "verify", "task-2", "ok", "tests pass", "auto:"+upper, "execute go")
	git(t, repo, "switch", "-q", "main")
	commitFile(t, repo, "src/m.txt", "m\n", "moved")
	git(t, repo, "switch", "-q", "lower")
	git(t, repo, "rebase", "-q", "main")
	git(t, repo, "switch", "-q", "topic")
	git(t, repo, "rebase", "-q", "--onto", "lower", old)
	if gitOut(t, repo, "merge-base", "lower", "HEAD") != gitOut(t, repo, "rev-parse", "lower") {
		t.Fatal("the branch must sit on the rewritten lower head")
	}
	wantLines(t, auditFolderOf(t, repo, gitOut(t, repo, "rev-parse", "lower")), []string{"audit: clean"})
}

// scopeListPlan lists paths in each form the scope check matches, and three
// divergence entries: one a row records, one no row records, and one
// reconciled with no row.
const scopeListPlan = "# Plan\n\n## Divergences\n\n" +
	"- **d1** — added `src/recorded.txt`.\n" +
	"- **d2** — added `src/unrecorded.txt`.\n" +
	"- **d3** — added `src/reconciled.txt`. Reconciled: 2026-11-19.\n\n" +
	"## Tasks\n\n### Task 1: Change things\n\n**ID:** task-1\n\n**Files:**\n\n" +
	"- Modify: `src/listed.txt:10-20` (the loop)\n" +
	"- Modify: `lib/`\n" +
	"- Test: `glob/*.go`\n" +
	"- Create: `brace/{x,y}.txt`\n"

func TestAuditReportsChangedPathsNoListCovers(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	writePlan(t, repo, scopeListPlan)
	res := runFlow(t, repo, nil, "log", "execute", "diverge", "task-1 files", "ok", "approved", "none", "execute go", "--folder", auditFolder, "--entry", "1")
	mustExit(t, res, 0, "flow log diverge")
	for _, rel := range []string{
		"src/listed.txt", "lib/deep/x.txt", "glob/a.go", "glob/a.txt", "brace/x.txt", "brace/y.txt", "brace/z.txt",
		"src/recorded.txt", "src/unrecorded.txt", "src/reconciled.txt", "src/other.txt",
	} {
		writeFile(t, repo, rel, rel+"\n")
	}
	writeFile(t, repo, auditFolder+"/notes.md", "notes\n")
	git(t, repo, "add", "-f", auditFolder+"/notes.md")
	sha := commitFile(t, repo, "src/listed.txt", "listed\n", "change")
	wantLines(t, auditFolderOf(t, repo, base), []string{
		"unaccounted: " + sha[:12] + " change",
		"out-of-scope: brace/z.txt",
		"out-of-scope: glob/a.txt",
		"out-of-scope: src/other.txt",
		"out-of-scope: src/reconciled.txt",
		"out-of-scope: src/unrecorded.txt",
		"audit: 6 findings",
	})
}

// Only an attested commit row or an exemption covers a path by its
// commits: a verify row names a task's commit, whose paths the task lists.
func TestAuditCoversAPathOnlyAnAttestedCommitRowChanges(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	attested := commitFile(t, repo, "other/attested.txt", "a\n", "attested")
	verified := commitFile(t, repo, "other/verified.txt", "v\n", "verified")
	forged := commitFile(t, repo, "other/forged.txt", "f\n", "forged")
	// A path an accounted commit and an unaccounted one both change is out
	// of scope, because the second commit has no record.
	commitFile(t, repo, "other/attested.txt", "a2\n", "again")
	again := gitOut(t, repo, "rev-parse", "HEAD")
	commitFile(t, repo, "other/solo.txt", "s\n", "solo")
	solo := gitOut(t, repo, "rev-parse", "HEAD")
	logOK(t, repo, auditFolder, "execute", "commit", "user", "ok", "user commit", "auto:"+attested, "execute go")
	logOK(t, repo, auditFolder, "execute", "commit", "user", "ok", "user commit", "auto:"+solo, "execute go")
	logOK(t, repo, auditFolder, "execute", "verify", "task-1", "ok", "tests pass", "auto:"+verified, "execute go")
	appendRows(t, repo, row("2026-11-01T10:00:00Z", "execute", "commit", "user", "ok", "by hand", "commit="+forged, "execute go"))
	wantLines(t, auditFolderOf(t, repo, base), []string{
		"unaccounted: " + forged[:12] + " forged (row not attested here)",
		"unaccounted: " + again[:12] + " again",
		"out-of-scope: other/attested.txt",
		"out-of-scope: other/forged.txt",
		"out-of-scope: other/verified.txt",
		"audit: 5 findings",
	})
}

func TestAuditScopesToTheBoundSlice(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	writePlan(t, repo, "# Plan\n\n## S1: First\n\n### Task 1: One\n\n**ID:** task-1\n\n**Files:**\n\n- Modify: `src/one.txt`\n\n"+
		"## S2: Second\n\n### Task 2: Two\n\n**ID:** task-2\n\n**Files:**\n\n- Modify: `src/two.txt`\n")
	mustExit(t, runFlow(t, repo, nil, "bind", "--folder", auditFolder, "--slice", "S2"), 0, "flow bind")
	writeFile(t, repo, "src/one.txt", "1\n")
	commitFile(t, repo, "src/two.txt", "2\n", "both")
	wantLines(t, linesWith(auditFolderOf(t, repo, base), "out-of-scope:"), []string{"out-of-scope: src/one.txt"})
}

func TestAuditScopesABriefFlowToItsFilesItem(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	if err := os.Remove(filepath.Join(repo, filepath.FromSlash(auditFolder), "plan.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, repo, auditFolder+"/brief.md", "# Brief\n\n## Task\n\nFix the loop.\n")
	writeFile(t, repo, auditFolder+"/notes.md", "notes\n")
	git(t, repo, "add", "-f", auditFolder+"/notes.md")
	writeFile(t, repo, "src/a.txt", "a\n")
	commitFile(t, repo, "README.md", "y\n", "fix")
	wantLines(t, linesWith(auditFolderOf(t, repo, base), "out-of-scope:"), []string{"out-of-scope: README.md", "out-of-scope: src/a.txt"})
}

func TestAuditNotesAScopeItCannotCheck(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, plan, slice, note string
	}{
		{"no-task", "# Plan\n\nNothing yet.\n", "", "note: scope not checked (no task in the plan)"},
		{"no-section", srcPlan, "S1,S3", "note: scope not checked (slice S1 has no task in the plan)"},
		{"empty-section", "# Plan\n\n## S1: First\n\nNothing yet.\n\n## S2: Second\n\n### Task 2: Two\n\n**ID:** task-2\n\n**Files:**\n\n- Modify: `src/`\n", "S2,S1", "note: scope not checked (slice S1 has no task in the plan)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo, _ := auditRepo(t)
			writePlan(t, repo, tc.plan)
			if tc.slice != "" {
				mustExit(t, runFlow(t, repo, nil, "bind", "--folder", auditFolder, "--slice", tc.slice), 0, "flow bind")
			}
			sha := commitFile(t, repo, "other/a.txt", "a\n", "a")
			git(t, repo, "switch", "-q", "main")
			commitFile(t, repo, "src/m.txt", "m\n", "moved")
			git(t, repo, "switch", "-q", "topic")
			git(t, repo, "merge", "-q", "--no-ff", "-m", "merge main", "main")
			merge := gitOut(t, repo, "rev-parse", "HEAD")
			wantLines(t, auditFolderOf(t, repo, gitOut(t, repo, "rev-parse", "main")), []string{
				"unaccounted: " + sha[:12] + " a",
				tc.note,
				"note: merge commit " + merge[:12] + " not audited",
				"audit: 1 findings",
			})
		})
	}
}

// A failed read of the changed paths must not pass for a branch that changed
// nothing out of scope. A tree only a merge commit adds is read by the diff
// and by no other read of the audit.
func TestAuditFailsClosedWhenTheChangedPathsCannotBeRead(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	git(t, repo, "switch", "-q", "-c", "side", "main")
	commitFile(t, repo, "src/s.txt", "s\n", "side")
	git(t, repo, "switch", "-q", "topic")
	git(t, repo, "merge", "-q", "--no-ff", "--no-commit", "side")
	writeFile(t, repo, "evil/z.txt", "z\n")
	commitAll(t, repo, "merge side")
	removeObject(t, repo, gitOut(t, repo, "rev-parse", "HEAD:evil"))
	res := runFlow(t, repo, nil, "audit", "--folder", auditFolder, "--base", base)
	mustExit(t, res, 1, "audit of unreadable changed paths")
	if want := "flow: audit cannot read the changed paths of " + base[:12] + "...HEAD: "; res.stdout != "" || !strings.HasPrefix(res.stderr, want) {
		t.Fatalf("stdout=%q stderr=%q, want no stdout and stderr starting %q", res.stdout, res.stderr, want)
	}
}

// A merge commit is never audited, so a path that only a merge changes has no
// commit that could account for it.
func TestAuditReportsAPathOnlyAMergeChanges(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	git(t, repo, "switch", "-q", "-c", "side", "main")
	side := commitFile(t, repo, "src/s.txt", "s\n", "side")
	git(t, repo, "switch", "-q", "topic")
	git(t, repo, "merge", "-q", "--no-ff", "--no-commit", "side")
	writeFile(t, repo, "evil/z.txt", "z\n")
	commitAll(t, repo, "merge side")
	merge := gitOut(t, repo, "rev-parse", "HEAD")
	wantLines(t, auditFolderOf(t, repo, base), []string{
		"unaccounted: " + side[:12] + " side",
		"out-of-scope: evil/z.txt",
		"note: merge commit " + merge[:12] + " not audited",
		"audit: 2 findings",
	})
}

// A diverge row records the entry of the artifact its target names, so a row
// for an entry of the same text in another artifact covers nothing here.
func TestAuditDivergenceCoversOnlyTheArtifactItsRowNames(t *testing.T) {
	t.Parallel()
	repo, base := auditRepo(t)
	entry := "## Divergences\n\n- **d1** — added `other/x.txt`.\n"
	writePlan(t, repo, srcPlan+"\n"+entry)
	writeFile(t, repo, auditFolder+"/spec.md", "# Spec\n\n"+entry)
	res := runFlow(t, repo, nil, "log", "execute", "diverge", "spec.md Constraints", "ok", "approved", "none", "execute go", "--folder", auditFolder, "--entry", "1")
	mustExit(t, res, 0, "flow log diverge")
	writeFile(t, repo, auditFolder+"/spec.md", "# Spec\n")
	commitFile(t, repo, "other/x.txt", "x\n", "x")
	wantLines(t, linesWith(auditFolderOf(t, repo, base), "out-of-scope:"), []string{"out-of-scope: other/x.txt"})
}

// A rule divergence changes how later work is checked, so it covers its paths
// only in a clone whose ledger holds its record.
func TestAuditRuleDivergenceCoversOnlyWithItsLedgerLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		ledger bool
		want   []string
	}{
		{"ledger", true, nil},
		{"no-ledger", false, []string{"out-of-scope: other/r.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo, base := auditRepo(t)
			writePlan(t, repo, srcPlan+"\n## Divergences\n\n- **r1** — checks run on `other/r.txt`.\n")
			res := runFlow(t, repo, nil, "log", "execute", "diverge", "task-1 rule", "ok", "approved", "none", "execute go", "--folder", auditFolder, "--entry", "1")
			mustExit(t, res, 0, "flow log diverge")
			if !tc.ledger {
				if err := os.Remove(filepath.Join(repo, ".git", "agentics", "flow", "ledger.tsv")); err != nil {
					t.Fatal(err)
				}
			}
			commitFile(t, repo, "other/r.txt", "r\n", "r")
			wantLines(t, linesWith(auditFolderOf(t, repo, base), "out-of-scope:"), tc.want)
		})
	}
}

// A bound slice id matches its heading in either case, a listed path may use
// backslashes, and a rename names its source path too.
func TestAuditScopeMatchesSliceCasePathSeparatorsAndRenames(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, ".git/info/exclude", "docs/specs/\n")
	commitFile(t, repo, "old/r.txt", "r\n", "old")
	base := gitOut(t, repo, "rev-parse", "HEAD")
	git(t, repo, "switch", "-q", "-c", "topic")
	seedFlow(t, repo, auditFolder, "topic")
	writePlan(t, repo, "# Plan\n\n## S1: First\n\n### Task 1: One\n\n**ID:** task-1\n\n**Files:**\n\n- Modify: `src/one.txt`\n\n"+
		"## s2: Second\n\n### Task 2: Two\n\n**ID:** task-2\n\n**Files:**\n\n- Modify: `src\\two.txt`\n- Create: `src/r.txt`\n")
	mustExit(t, runFlow(t, repo, nil, "bind", "--folder", auditFolder, "--slice", "S2"), 0, "flow bind")
	writeFile(t, repo, "src/two.txt", "2\n")
	git(t, repo, "mv", "old/r.txt", "src/r.txt")
	commitAll(t, repo, "move")
	wantLines(t, linesWith(auditFolderOf(t, repo, base), "out-of-scope:"), []string{"out-of-scope: old/r.txt"})
}
