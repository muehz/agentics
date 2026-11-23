package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func whereOf(t *testing.T, repo string, env ...string) result {
	t.Helper()
	r := runFlow(t, repo, env, "where")
	switch r.code {
	case 0:
		if r.stderr != "" {
			t.Fatalf("success stderr: %q", r.stderr)
		}
	case 3:
		if r.stdout != "" || r.stderr != "flow: no flow found\n" {
			t.Fatalf("no flow: %+v", r)
		}
	case 4:
		if r.stdout != "" || !strings.HasPrefix(r.stderr, "flow: more than one flow matches; ask which one:\n  ") {
			t.Fatalf("ambiguous: %+v", r)
		}
	case 5:
		branch := gitOut(t, repo, "symbolic-ref", "--short", "HEAD")
		if r.stdout != "" || r.stderr != "flow: small fix in progress on "+branch+"; no work folder\n" {
			t.Fatalf("small fix: %+v", r)
		}
	}
	return r
}

func wantFolder(t *testing.T, res result, folder string) {
	t.Helper()
	mustExit(t, res, 0, "where")
	if got := res.stdout; got != folder+"\n" {
		t.Fatalf("want %s, got %q", folder, got)
	}
}

// Direct rows keep lookup fixtures independent of the log command.
func lookupAppend(t *testing.T, repo, folder string, c ...string) {
	t.Helper()
	if len(c) != 7 {
		t.Fatal("fixture requires seven cells")
	}
	n := len(rowsOf(filepath.Join(repo, folder, "state.tsv")))
	err := appendState(filepath.Join(repo, folder), folder, "test-session", [7]string(c), func() time.Time { return time.Date(2026, 10, 14, 10, 0, n, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
}

func lookupSeed(t *testing.T, repo, folder, bind string) {
	t.Helper()
	lookupAppend(t, repo, folder, "intent", "create", "intent.md", "ok", "seed", "intent.md", "spec go")
	if bind != "" {
		lookupAppend(t, repo, folder, "intent", "bind", bind, "ok", "bound", "none", "spec go")
	}
}

const (
	flowA = "docs/specs/2026-10/feature-a"
	flowB = "docs/specs/2026-10/feature-b"
)

func TestWhereStep1BindWins(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowA, "feat/x")
	lookupSeed(t, repo, flowB, "feat/y")
	commitAll(t, repo, "flows")
	git(t, repo, "switch", "-q", "-c", "feat/x")
	wantFolder(t, whereOf(t, repo), flowA)
}

// A bind row is the only lookup that names the branch, so it wins even when
// the branch's own diff changes a different flow's state file.
func TestWhereStep1BindWinsOverAChangedStateFile(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowA, "feat/x")
	commitAll(t, repo, "a on main")
	git(t, repo, "switch", "-q", "-c", "feat/x")
	lookupSeed(t, repo, flowB, "")
	commitAll(t, repo, "b on branch")
	wantFolder(t, whereOf(t, repo), flowA)
}

func TestWhereOneFlowBoundToSeveralLaneBranches(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowA, "feat/export-s1")
	lookupAppend(t, repo, flowA, "execute", "bind", "feat/export-s2", "ok", "second lane", "none", "execute go")
	lookupSeed(t, repo, flowB, "feat/other")
	commitAll(t, repo, "flows")
	for _, branch := range []string{"feat/export-s1", "feat/export-s2"} {
		git(t, repo, "switch", "-q", "-C", branch)
		wantFolder(t, whereOf(t, repo), flowA)
	}
}

func TestWhereTheLaterBindWinsWhenTwoFlowsNameABranch(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeState(t, repo, flowA,
		row("2026-10-14T10:00:00Z", "intent", "bind", "feat/x", "ok", "bound", "none", "spec go"))
	writeState(t, repo, flowB,
		row("2026-10-14T11:00:00Z", "intent", "bind", "feat/x", "ok", "bound", "none", "spec go"))
	commitAll(t, repo, "flows")
	git(t, repo, "switch", "-q", "-c", "feat/x")
	wantFolder(t, whereOf(t, repo), flowB)
}

func TestWhereStep2ChangedStateFile(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowA, "")
	commitAll(t, repo, "a on main")
	git(t, repo, "switch", "-q", "-c", "feat/y")
	lookupSeed(t, repo, flowB, "")
	commitAll(t, repo, "b on branch")
	wantFolder(t, whereOf(t, repo), flowB)
}

func TestWhereStep2SeesUntrackedChanges(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowA, "")
	commitAll(t, repo, "a on main")
	git(t, repo, "switch", "-q", "-c", "feat/y")
	lookupSeed(t, repo, flowB, "") // untracked, not staged
	wantFolder(t, whereOf(t, repo), flowB)
}

func TestWhereStep2IgnoresAFlowOnlyMainChanged(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowA, "")
	commitAll(t, repo, "a on main")
	git(t, repo, "switch", "-q", "-c", "feat/y")
	lookupSeed(t, repo, flowB, "")
	commitAll(t, repo, "b on branch")
	// main moves on and changes flow A. That is not this branch's flow.
	git(t, repo, "switch", "-q", "main")
	lookupAppend(t, repo, flowA, "intent", "challenge", "intent.md", "ok", "later", "none", "spec go")
	commitAll(t, repo, "a changes on main")
	git(t, repo, "switch", "-q", "feat/y")
	wantFolder(t, whereOf(t, repo), flowB)
}

func TestWhereStep2SkipsTempDirectories(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "feat/y")
	lookupSeed(t, repo, "docs/specs/2026-10/feature-a/temp/x", "")
	mustExit(t, whereOf(t, repo), 3, "a state.tsv under temp/ is not a flow")
}

// Being the only active flow without a bind row does not make a flow this
// branch's: it may belong to an item someone has not started yet.
func TestWhereTheOnlyUnboundFlowIsNotMatched(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowA, "")
	commitAll(t, repo, "a")
	git(t, repo, "switch", "-q", "-c", "feat/z")
	mustExit(t, whereOf(t, repo), 3, "an unbound flow the branch did not change")

	// A flow some other branch is bound to never matches here.
	repo2 := newRepo(t)
	lookupSeed(t, repo2, flowA, "feat/other")
	commitAll(t, repo2, "a")
	git(t, repo2, "switch", "-q", "-c", "feat/new")
	mustExit(t, whereOf(t, repo2), 3, "a bound flow belongs to its branch")
}

func TestWhereOnTheDefaultBranchABindOrAnUncommittedChangeMatches(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowA, "") // the only active flow, and unbound
	commitAll(t, repo, "a")
	mustExit(t, whereOf(t, repo), 3, "main must not resolve to another item's flow")

	lookupAppend(t, repo, flowA, "intent", "challenge", "intent.md", "ok", "later", "none", "spec go")
	wantFolder(t, whereOf(t, repo), flowA)
	commitAll(t, repo, "a changed")
	mustExit(t, whereOf(t, repo), 3, "a committed change no longer singles the flow out")

	lookupSeed(t, repo, flowB, "main")
	commitAll(t, repo, "b bound to main")
	lookupAppend(t, repo, flowA, "intent", "challenge", "intent.md", "ok", "again", "none", "spec go")
	wantFolder(t, whereOf(t, repo), flowB)
}

func TestWhereUnassignedIsNeverAFlow(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupAppend(t, repo, "docs/specs/unassigned", "gate", "release", "v1", "ok", "tagged", "v1", "done")
	commitAll(t, repo, "unassigned")
	git(t, repo, "switch", "-q", "-c", "feat/z")
	mustExit(t, whereOf(t, repo), 3, "unassigned/state.tsv is not a flow")
}

func TestWhereSkipsDoneFlows(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowA, "")
	lookupAppend(t, repo, flowA, "gate", "done", "branch", "ok", "merged", "none", "done nothing left")
	commitAll(t, repo, "a done")
	git(t, repo, "switch", "-q", "-c", "feat/z")
	mustExit(t, whereOf(t, repo), 3, "a done flow is not matched")
}

func TestWhereNoFlowExitsThree(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "feat/z")
	mustExit(t, whereOf(t, repo), 3, "no flow")
}

func TestWhereSeveralMatchesExitFourAndList(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "feat/z")
	lookupSeed(t, repo, flowA, "")
	lookupSeed(t, repo, flowB, "")
	commitAll(t, repo, "two flows")
	res := whereOf(t, repo)
	mustExit(t, res, 4, "two flows the branch changed")
	if !strings.Contains(res.stderr, "feature-a") || !strings.Contains(res.stderr, "feature-b") {
		t.Fatalf("exit 4 must list both flows, got %q", res.stderr)
	}
}

func TestWhereSquashMergedBranchIsNotMatchedByStep2(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowB, "") // another active flow, unbound
	commitAll(t, repo, "b on main")
	git(t, repo, "switch", "-q", "-c", "feat/a")
	lookupSeed(t, repo, flowA, "")
	commitAll(t, repo, "a on branch")
	// Before the merge, step 2 singles out A: B is the same as on main.
	wantFolder(t, whereOf(t, repo), flowA)

	git(t, repo, "switch", "-q", "main")
	git(t, repo, "merge", "-q", "--squash", "feat/a")
	git(t, repo, "commit", "-q", "-m", "squash feat/a")
	git(t, repo, "switch", "-q", "feat/a")
	// After the squash merge, A equals the default branch's tip, so step 2
	// finds nothing, and two unbound active flows match nothing either.
	mustExit(t, whereOf(t, repo), 3, "after squash merge")
}

// TestWhereSquashMergedBranchWithADoneFlowResolvesToNothing covers the
// ordinary end of a flow: gate's merge writes a done row, the squash merge
// lands the branch on the default branch, and nothing else is active. Step 2
// finds nothing because the branch now equals the default branch's tip. Step
// 1 must skip the done flow in the bound case, so the lookup exits 3 and not 0.
func TestWhereSquashMergedBranchWithADoneFlowResolvesToNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, bind string }{
		{"unbound flow", ""},
		{"flow bound to the branch", "feat/a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			git(t, repo, "switch", "-q", "-c", "feat/a")
			lookupSeed(t, repo, flowA, tc.bind)
			commitAll(t, repo, "a on branch")
			wantFolder(t, whereOf(t, repo), flowA)

			lookupAppend(t, repo, flowA, "gate", "done", "branch", "ok", "merged", "none", "done nothing left")
			commitAll(t, repo, "a done")

			git(t, repo, "switch", "-q", "main")
			git(t, repo, "merge", "-q", "--squash", "feat/a")
			git(t, repo, "commit", "-q", "-m", "squash feat/a")
			git(t, repo, "switch", "-q", "feat/a")
			mustExit(t, whereOf(t, repo), 3, "a squash merged branch whose flow is done")
		})
	}
}

func TestWhereRenamedBranchFindsItsFlowByStep2(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, "docs/specs/2026-10/feature-z", "feat/z")
	commitAll(t, repo, "z on main")
	git(t, repo, "switch", "-q", "-c", "feat/old")
	lookupSeed(t, repo, flowA, "feat/old")
	commitAll(t, repo, "a on branch")
	git(t, repo, "branch", "-m", "feat/old", "feat/renamed")
	wantFolder(t, whereOf(t, repo), flowA)
}

func TestWhereHonorsFlowWorkRoot(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupAppend(t, repo, "plans/2026-10/feature-a", "intent", "create", "intent.md", "ok", "seed", "intent.md", "spec go")
	commitAll(t, repo, "plans")
	git(t, repo, "switch", "-q", "-c", "feat/z")
	lookupAppend(t, repo, "plans/2026-10/feature-a", "intent", "challenge", "intent.md", "ok", "later", "none", "spec go")
	mustExit(t, whereOf(t, repo), 3, "default root has no flow")
	wantFolder(t, whereOf(t, repo, "FLOW_WORK_ROOT=plans"), "plans/2026-10/feature-a")
}

func TestWhereExitsFiveForASmallFix(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "fix/zero-limit")
	lookupAppend(t, repo, "docs/specs/unassigned", "execute", "skip", "chain fix/zero-limit", "ok", "bug fix: one concern", "base=abc", "execute go")
	mustExit(t, whereOf(t, repo), 5, "a small fix in progress")
	// A different branch is not that small fix.
	git(t, repo, "switch", "-q", "-c", "feat/other")
	mustExit(t, whereOf(t, repo), 3, "another branch")
}

// TestWhereSquashMergedBranchWithABindAndNoDoneRowResolvesToNothing covers a
// branch whose flow was never closed with a done row. Its bind row names the
// branch, so it matches only that branch. A new branch cut from the default
// branch after the squash merge has no bind and no changed state file, so the
// active flow bound to the old branch matches nothing.
func TestWhereSquashMergedBranchWithABindAndNoDoneRowResolvesToNothing(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "feat/a")
	lookupSeed(t, repo, flowA, "feat/a")
	commitAll(t, repo, "a on branch")
	wantFolder(t, whereOf(t, repo), flowA)

	git(t, repo, "switch", "-q", "main")
	git(t, repo, "merge", "-q", "--squash", "feat/a")
	git(t, repo, "commit", "-q", "-m", "squash feat/a")
	git(t, repo, "switch", "-q", "-c", "feat/next")
	if _, err := os.Stat(filepath.Join(repo, flowA, "state.tsv")); err != nil {
		t.Fatalf("the squash merge must land the flow on the default branch: %v", err)
	}
	mustExit(t, whereOf(t, repo), 3, "a new branch after a squash merge of a bound flow with no done row")
}
