package flow

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStatusPathsKeepsTheRenameDestinationAndSkipsItsSource(t *testing.T) {
	t.Parallel()
	out := "R  docs/specs/2026-11/new/state.tsv\x00docs/specs/2026-11/old/state.tsv\x00 M docs/specs/2026-11/b/state.tsv\x00"
	got := statusPaths(out)
	want := map[string]bool{"docs/specs/2026-11/new/state.tsv": true, "docs/specs/2026-11/b/state.tsv": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("statusPaths = %v, want %v", got, want)
	}
}

func TestWhereSkipsInactiveAtEveryStep(t *testing.T) {
	t.Parallel()
	for _, step := range []string{"bind", "changed", "default"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			if step == "changed" {
				git(t, repo, "switch", "-q", "-c", "topic")
			}
			branch := ""
			if step == "bind" {
				branch = "topic"
			}
			lookupSeed(t, repo, flowA, branch)
			lookupAppend(t, repo, flowA, "gate", "done", "branch", "ok", "done", "none", "done")
			writeState(t, repo, flowB)
			if step == "default" {
				mustExit(t, whereOf(t, repo), 3, "inactive uncommitted flows on the default branch")
				return
			}
			commitAll(t, repo, "inactive flows")
			if step != "changed" {
				git(t, repo, "switch", "-q", "-c", "topic")
			}
			mustExit(t, whereOf(t, repo), 3, "inactive "+step)
		})
	}
}

func TestWhereFlowEnumerationUsesShellDepthAndHiddenRules(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	for _, folder := range []string{"docs/specs/.hidden/item", "docs/specs/2026-10/.hidden", "docs/specs/unassigned", "docs/specs/2026-10/item/nested"} {
		lookupSeed(t, repo, folder, "topic")
	}
	commitAll(t, repo, "non-flow paths")
	git(t, repo, "switch", "-q", "-c", "topic")
	mustExit(t, whereOf(t, repo), 3, "no enumerated flow")
}

func TestWhereDetachedAndMissingRepository(t *testing.T) {
	repo := newRepo(t)
	lookupSeed(t, repo, flowA, "main")
	lookupSeed(t, repo, flowB, "")
	commitAll(t, repo, "flows")
	git(t, repo, "checkout", "-q", "--detach")
	mustExit(t, whereOf(t, repo), 3, "detached")
	mustExit(t, whereOf(t, t.TempDir()), 3, "outside repo")
	t.Setenv("PATH", t.TempDir())
	mustExit(t, whereOf(t, repo), 3, "missing Git")
}

// A committed change to a flow singles it out off the default branch and never
// on it, so each case tells which branch lookup took for the default.
func TestWhereDefaultRefOrder(t *testing.T) {
	t.Parallel()
	change := func(t *testing.T, repo string) {
		t.Helper()
		lookupAppend(t, repo, flowA, "intent", "challenge", "intent.md", "ok", "later", "none", "spec go")
		commitAll(t, repo, "change a")
	}
	for _, name := range []string{"main before master", "master fallback", "origin before main", "no default ref"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			lookupSeed(t, repo, flowA, "")
			commitAll(t, repo, "flow")
			switch name {
			case "main before master":
				git(t, repo, "branch", "master")
			case "master fallback":
				git(t, repo, "branch", "-m", "master")
			case "origin before main":
				git(t, repo, "branch", "trunk")
				git(t, repo, "update-ref", "refs/remotes/origin/trunk", "HEAD")
				git(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
				change(t, repo)
				wantFolder(t, whereOf(t, repo), flowA)
				git(t, repo, "switch", "-q", "trunk")
			case "no default ref":
				git(t, repo, "branch", "-m", "topic")
				change(t, repo)
				mustExit(t, whereOf(t, repo), 3, "no default branch to compare with")
				return
			}
			change(t, repo)
			mustExit(t, whereOf(t, repo), 3, "default branch")
			git(t, repo, "switch", "-q", "-c", "feature")
			change(t, repo)
			wantFolder(t, whereOf(t, repo), flowA)
		})
	}
}

func TestWhereRemoteTipDropsSquashMergedFlow(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowB, "")
	commitAll(t, repo, "base")
	git(t, repo, "switch", "-q", "-c", "feature")
	lookupSeed(t, repo, flowA, "")
	commitAll(t, repo, "feature")
	git(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
	git(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	// Against the remote tip the branch changed nothing; against local main it changed flow A.
	mustExit(t, whereOf(t, repo), 3, "the remote tip already holds flow A")
}

func assertLookupAmbiguous(t *testing.T, res result, folders ...string) {
	t.Helper()
	want := "flow: more than one flow matches; ask which one:\n  " + strings.Join(folders, "\n  ") + "\n"
	if res.code != 4 || res.stdout != "" || res.stderr != want {
		t.Fatalf("got %+v, want exit 4 stderr %q", res, want)
	}
}

func TestWhereTiedBindsAndChangedAmbiguity(t *testing.T) {
	t.Parallel()
	for _, bound := range []bool{true, false} {
		t.Run(map[bool]string{true: "bind tie", false: "changed"}[bound], func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			git(t, repo, "switch", "-q", "-c", "topic")
			branch := ""
			if bound {
				branch = "topic"
			}
			lookupSeed(t, repo, flowB, branch)
			lookupSeed(t, repo, flowA, branch)
			assertLookupAmbiguous(t, whereOf(t, repo), flowA, flowB)
		})
	}
}

func TestWhereSmallFixPrecedenceAndPivot(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowA, "")
	commitAll(t, repo, "flow")
	git(t, repo, "switch", "-q", "-c", "fix/q")
	lookupAppend(t, repo, flowA, "intent", "challenge", "intent.md", "ok", "later", "none", "spec go")
	lookupAppend(t, repo, "docs/specs/unassigned", "execute", "skip", "chain fix/q", "ok", "fix", "none", "execute go")
	mustExit(t, whereOf(t, repo), 5, "small fix wins over a changed state file")
	lookupSeed(t, repo, flowB, "fix/q")
	wantFolder(t, whereOf(t, repo), flowB)
	lookupAppend(t, repo, flowB, "gate", "done", "branch", "ok", "done", "none", "done")
	mustExit(t, whereOf(t, repo), 5, "closed bound flow")
	lookupAppend(t, repo, "docs/specs/unassigned", "execute", "pivot", "fix someone-else", "ok", "pivot", "none", "intent go")
	mustExit(t, whereOf(t, repo), 5, "unrelated pivot")
	lookupAppend(t, repo, "docs/specs/unassigned", "execute", "pivot", "fix fix/q", "ok", "pivot", "none", "intent go")
	wantFolder(t, whereOf(t, repo), flowA)
}

func TestWhereRootAndCallerDirectory(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, "plans/2026-10/item", "topic")
	lookupSeed(t, repo, flowA, "topic")
	commitAll(t, repo, "flows")
	git(t, repo, "switch", "-q", "-c", "topic")
	sub := filepath.Join(repo, "sub", "directory")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	for _, cwd := range []string{repo, sub} {
		wantFolder(t, whereOf(t, cwd, "FLOW_WORK_ROOT=plans", "FLOW_SESSION=one", "CLAUDE_CODE_SESSION_ID=two"), "plans/2026-10/item")
		wantFolder(t, whereOf(t, cwd, "FLOW_WORK_ROOT="), flowA)
		// The reference prefixes even a slash-rooted work root with the repository top.
		wantFolder(t, whereOf(t, cwd, "FLOW_WORK_ROOT=/plans"), "/plans/2026-10/item")
	}
}

// A slash-rooted work root names a folder inside the repository, so commands
// that act on the looked-up flow read and write it there.
func TestLookupSlashRootedWorkRootActsInsideTheRepository(t *testing.T) {
	t.Parallel()
	const folder = "plans/2026-10/item"
	repo := newRepo(t)
	lookupSeed(t, repo, folder, "topic")
	commitAll(t, repo, "flow")
	git(t, repo, "switch", "-q", "-c", "topic")
	env := []string{"FLOW_WORK_ROOT=/plans"}
	state := filepath.Join(repo, filepath.FromSlash(folder), "state.tsv")
	outside := filepath.Join(string(filepath.Separator)+"plans", "2026-10", "item")

	res := runFlow(t, repo, env, "status")
	mustExit(t, res, 0, "status")
	if !strings.HasPrefix(res.stdout, "flow: "+folder+"\nstorage: committed\n") {
		t.Fatalf("status printed %q", res.stdout)
	}
	before := len(rowsOf(state))
	res = runFlow(t, repo, env, "log", "intent", "challenge", "intent.md", "ok", "checked", "none", "spec go")
	mustExit(t, res, 0, "log")
	if after := len(rowsOf(state)); after != before+1 {
		t.Fatalf("%s has %d rows after log, want %d", state, after, before+1)
	}
	if _, err := os.Lstat(outside); !os.IsNotExist(err) {
		t.Fatalf("log created %s outside the repository (err %v)", outside, err)
	}
	res = runFlow(t, repo, env, "path")
	mustExit(t, res, 0, "path")
	if want := filepath.Join(repo, filepath.FromSlash(folder)) + "\n"; res.stdout != want || res.stderr != "" {
		t.Fatalf("path printed %+v, want %q", res, want)
	}
	res = runFlow(t, repo, env, "audit", "--base", "main")
	named := runFlow(t, repo, env, "audit", "--folder", folder, "--base", "main")
	mustExit(t, named, 0, "audit --folder")
	if res != named {
		t.Fatalf("audit printed %+v, audit --folder printed %+v", res, named)
	}
}

func TestWhereFormulaBranchAndIgnoresArguments(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, flowA, "+topic")
	commitAll(t, repo, "flow")
	git(t, repo, "switch", "-q", "-c", "+topic")
	wantFolder(t, whereOf(t, repo), flowA)
	wantFolder(t, runFlow(t, repo, nil, "where", "ignored"), flowA)
}

func TestWhereKeepsFoldersNamedTempWithOtherCase(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	lookupSeed(t, repo, "docs/specs/2026-10/other", "feat/other")
	commitAll(t, repo, "other")
	git(t, repo, "switch", "-q", "-c", "feat/a")
	lookupSeed(t, repo, "docs/specs/2026-10/Temp", "")
	commitAll(t, repo, "Temp")
	wantFolder(t, whereOf(t, repo), "docs/specs/2026-10/Temp")
}

const lookupParityFolder = "docs/specs/2026-10/feature-x"

func TestWhereStatusParityScenarios(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, ".gitattributes", "docs/specs/**/state.tsv merge=union\n")
	writeFile(t, repo, lookupParityFolder+"/spec.md", "# Spec\n")
	writeFile(t, repo, lookupParityFolder+"/plan.md", "# Plan\n\n## Tasks\n\nbody\n")
	hash := gitOut(t, repo, "hash-object", "--", lookupParityFolder+"/spec.md")
	writeState(t, repo, lookupParityFolder,
		row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"),
		row("2026-10-14T10:10:00Z", "intent", "approve", "intent.md", "ok", "approved", "blob=aaaa", "spec go"),
		row("2026-10-14T11:00:00Z", "spec", "approve", "spec.md", "ok", "approved", "blob="+hash, "decompose go"),
		row("2026-10-14T12:00:00Z", "decompose", "create", "plan.md", "ok", "plan", "plan.md", "execute go"),
		row("2026-10-14T12:30:00Z", "spec", "revoke", "spec.md", "ok", "rework", "none", "spec go"),
		row("2026-10-14T12:40:00Z", "spec", "approve", "spec.md", "ok", "re-approved", "blob="+hash, "decompose go"),
		row("2026-10-14T13:00:00Z", "decompose", "approve", "plan.md", "ok", "approved", "blob=dddd", "execute go"),
		row("2026-10-14T13:05:00Z", "execute", "enter", "run", "ok", "start", "mode=pair branch=feat/x", "execute go"),
		row("2026-10-14T13:10:00Z", "execute", "block", "task-3", "blocked", "waiting", "none", "execute go"),
		row("2026-10-14T13:20:00Z", "execute", "verify", "task-1", "ok", "v", "commit=0000000000000000000000000000000000000009 patch-id=deadbeef", "execute go"),
		row("2026-10-14T13:00:00Z", "decompose", "approve", "plan.md", "ok", "approved", "blob=dddd", "execute go"),
		row("2026-10-14T13:30:00Z", "intent", "bind", "main", "ok", "bound", "none", "execute go"),
	)
	commitAll(t, repo, "flow")

	wantFolder(t, whereOf(t, repo), lookupParityFolder)
	git(t, repo, "switch", "-q", "-c", "feat/y")
	lookupSeed(t, repo, flowB, "")
	commitAll(t, repo, "b on branch")
	wantFolder(t, whereOf(t, repo), flowB)
	git(t, repo, "switch", "-q", "-c", "fix/q")
	lookupAppend(t, repo, "docs/specs/unassigned", "execute", "skip", "chain fix/q", "ok", "bug fix: one concern", "base=abc", "execute go")
	mustExit(t, whereOf(t, repo), 5, "small fix")
	git(t, repo, "switch", "-q", "-c", "feat/nobody", "main")
	mustExit(t, whereOf(t, repo), 3, "main's sole flow is bound elsewhere")
}
