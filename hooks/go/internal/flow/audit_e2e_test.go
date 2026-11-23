package flow

import "testing"

// TestAuditEndToEnd runs flow audit over one branch that holds each kind of
// commit the checks tell apart: a task commit, a gate housekeeping commit, a
// hand-made commit with a forged row, a merge from the base, and a commit
// outside every task's files. Only the forged row and the stray path are
// findings, and attesting the forged commit through the gate clears the first.
func TestAuditEndToEnd(t *testing.T) {
	t.Parallel()
	repo, _ := auditRepo(t)

	task := commitFile(t, repo, "src/a.go", "package a\n", "add a")
	logOK(t, repo, auditFolder, "execute", "verify", "task-1", "ok", "tests pass", "auto:"+task, "execute go")

	housekeeping := commitFile(t, repo, "src/b.go", "package b\n", "tidy b")
	logOK(t, repo, auditFolder, "gate", "commit", "housekeeping", "ok", "gate commit", "auto:"+housekeeping, "gate")

	hand := commitFile(t, repo, "src/c.go", "package c\n", "hand edit c")
	appendRows(t, repo, row("2026-11-01T10:00:00Z", "execute", "commit", "user", "ok", "by hand", "commit="+hand, "execute go"))

	git(t, repo, "switch", "-q", "main")
	commitFile(t, repo, "base.txt", "base\n", "base moves on")
	git(t, repo, "switch", "-q", "topic")
	git(t, repo, "merge", "-q", "--no-ff", "-m", "merge main", "main")
	merge := gitOut(t, repo, "rev-parse", "HEAD")
	base := gitOut(t, repo, "rev-parse", "main")

	stray := commitFile(t, repo, "outside/z.txt", "z\n", "stray z")
	logOK(t, repo, auditFolder, "execute", "verify", "task-1", "ok", "tests pass", "auto:"+stray, "execute go")

	wantLines(t, auditFolderOf(t, repo, base), []string{
		"unaccounted: " + hand[:12] + " hand edit c (row not attested here)",
		"out-of-scope: outside/z.txt",
		"note: merge commit " + merge[:12] + " not audited",
		"audit: 2 findings",
	})

	logOK(t, repo, auditFolder, "gate", "commit", "user", "ok", "accepted", "auto:"+hand, "gate")
	wantLines(t, auditFolderOf(t, repo, base), []string{
		"out-of-scope: outside/z.txt",
		"note: merge commit " + merge[:12] + " not audited",
		"audit: 1 findings",
	})
}
