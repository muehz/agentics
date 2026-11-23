package flow

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func bindRows(t *testing.T, repo, folder string) []string {
	t.Helper()
	var out []string
	for _, line := range readLines(t, filepath.Join(repo, folder, "state.tsv")) {
		if strings.Contains(line, "\tbind\t") {
			out = append(out, line)
		}
	}
	return out
}

func bindAt(t *testing.T, dir string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(append([]string{"bind"}, args...), dir, func(name string) string {
		if name == "FLOW_SESSION" {
			return "bind-session"
		}
		return ""
	}, func() time.Time { return time.Date(2026, 11, 10, 6, 0, 1, 0, time.FixedZone("offset", 3600)) }, &stdout, &stderr)
	return result{code, stdout.String(), stderr.String()}
}

func bindBody(t *testing.T, repo, folder string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(repo, folder, "state.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func expectBindFailure(t *testing.T, res result, code int, message string) {
	t.Helper()
	if res.code != code || res.stdout != "" || res.stderr != message {
		t.Fatalf("bind: exit=%d stdout=%q stderr=%q, want exit=%d stderr=%q", res.code, res.stdout, res.stderr, code, message)
	}
}

func TestBindAppendsOnceAndIsIdempotent(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "feat/z")
	seedFlow(t, repo, flowA, "")
	commitAll(t, repo, "a")
	before := bindBody(t, repo, flowA)
	res := bindAt(t, repo)
	mustExit(t, res, 0, "first bind")
	if res.combined() != "" {
		t.Fatalf("bind output = %q", res.combined())
	}
	want := before + "2026-11-10T05:00:01Z\tbind-session\tintent\tbind\tfeat/z\tok\tbound feat/z to this flow\tnone\tspec go\n"
	if got := bindBody(t, repo, flowA); got != want {
		t.Fatalf("bind bytes = %q, want %q", got, want)
	}
	mustExit(t, bindAt(t, repo), 0, "second bind")
	if got := bindBody(t, repo, flowA); got != want {
		t.Fatalf("second bind changed bytes: %q", got)
	}
	wantFolder(t, whereOf(t, repo), flowA)
}

func TestBindNamesTheLaneBranchOfTheSameFlow(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	seedFlow(t, repo, flowA, "feat/export-s1")
	commitAll(t, repo, "a")
	git(t, repo, "switch", "-q", "-c", "feat/export-s2")
	mustExit(t, runFlow(t, repo, nil, "bind", "--folder", flowA), 0, "bind second lane")
	rows := bindRows(t, repo, flowA)
	if len(rows) != 2 || cells(rows[1])[4] != "feat/export-s2" {
		t.Fatalf("want two lane binds, got %q", rows)
	}
	wantFolder(t, whereOf(t, repo), flowA)
}

func TestBindRefusesTheDefaultBranchADetachedHeadAndASmallFix(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	seedFlow(t, repo, flowA, "")
	commitAll(t, repo, "a")
	before := bindBody(t, repo, flowA)
	expectBindFailure(t, runFlow(t, repo, nil, "bind", "--folder", flowA), 2, "flow: invalid branch: 'main' -- the default branch is never bound to a flow\n")
	git(t, repo, "switch", "-q", "--detach", "HEAD")
	expectBindFailure(t, runFlow(t, repo, nil, "bind", "--folder", flowA), 2, "flow: invalid branch: '' -- a detached HEAD has no branch to bind\n")
	git(t, repo, "switch", "-q", "-c", "fix/q")
	logOK(t, repo, "docs/specs/unassigned", "execute", "skip", "chain fix/q", "ok", "bug fix: one concern", "base=abc", "execute go")
	expectBindFailure(t, runFlow(t, repo, nil, "bind"), 2, "flow: invalid folder: 'docs/specs/unassigned' -- a small fix has no flow to bind\n")
	if got := bindBody(t, repo, flowA); got != before {
		t.Fatal("refused bind changed the flow")
	}
}

func TestBindFindsABranchWhoseNameNeedsTheFormulaGuard(t *testing.T) {
	t.Parallel()
	for _, branch := range []string{"+feature", "@x", "=y", "'z"} {
		t.Run(branch, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			seedFlow(t, repo, flowA, "")
			commitAll(t, repo, "a")
			switched := runCmd(t, repo, "git", []string{"switch", "-q", "-c", branch}, baseEnv())
			if switched.code != 0 {
				t.Skipf("git refuses branch %q: %s", branch, switched.stderr)
			}
			mustExit(t, runFlow(t, repo, nil, "bind", "--folder", flowA), 0, "guarded bind")
			rows := bindRows(t, repo, flowA)
			if len(rows) != 1 || cells(rows[0])[4] != "'"+branch {
				t.Fatalf("want guarded branch %q, got %q", branch, rows)
			}
			commitAll(t, repo, "bound")
			git(t, repo, "switch", "-q", "main")
			git(t, repo, "merge", "-q", "--ff-only", "refs/heads/"+branch)
			git(t, repo, "switch", "-q", branch)
			wantFolder(t, whereOf(t, repo), flowA)
			before := bindBody(t, repo, flowA)
			mustExit(t, runFlow(t, repo, nil, "bind", "--folder", flowA), 0, "guarded second bind")
			if got := bindBody(t, repo, flowA); got != before {
				t.Fatal("guarded second bind wrote a row")
			}
		})
	}
}

func TestBindRowParityAndIsolation(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	seedFlow(t, repo, flowA, "")
	seedFlow(t, repo, flowB, "")
	commitAll(t, repo, "flows")
	git(t, repo, "switch", "-q", "-c", "feat/z")
	beforeA, beforeB := bindBody(t, repo, flowA), bindBody(t, repo, flowB)
	mustExit(t, bindAt(t, repo, "--folder", flowA), 0, "first flow")
	if got := bindBody(t, repo, flowB); got != beforeB {
		t.Fatal("binding A changed B")
	}
	mustExit(t, bindAt(t, repo, "--folder", flowB), 0, "second flow")
	want := "2026-11-10T05:00:01Z\tbind-session\tintent\tbind\tfeat/z\tok\tbound feat/z to this flow\tnone\tspec go\n"
	if bindBody(t, repo, flowA) != beforeA+want || bindBody(t, repo, flowB) != beforeB+want {
		t.Fatal("the two flow binds differ from the expected row bytes")
	}
	mustExit(t, bindAt(t, repo, "--folder", flowB), 0, "second flow again")
	if bindBody(t, repo, flowB) != beforeB+want {
		t.Fatal("second flow bind was not idempotent")
	}
}

func TestBindRejectsUsageWithoutWrites(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--metrics", "--folder", flowA}, {"--metrics", "value", "--folder", flowA}, {"--other", flowA}, {"--folder"}, {"extra"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			seedFlow(t, repo, flowA, "")
			git(t, repo, "switch", "-q", "-c", "feat/z")
			before := bindBody(t, repo, flowA)
			expectBindFailure(t, bindAt(t, repo, args...), 1, usage)
			if bindBody(t, repo, flowA) != before {
				t.Fatal("usage error wrote a row")
			}
		})
	}
}

func TestBindPropagatesResolutionFailures(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "topic")
	expectBindFailure(t, bindAt(t, repo), 3, "flow: no flow found; ask which flow, or pass --folder\n")
	seedFlow(t, repo, flowA, "")
	seedFlow(t, repo, flowB, "")
	commitAll(t, repo, "flows")
	expectBindFailure(t, bindAt(t, repo), 4, "flow: more than one flow matches; ask which one:\n  "+flowA+"\n  "+flowB+"\n")
	if len(bindRows(t, repo, flowA)) != 0 || len(bindRows(t, repo, flowB)) != 0 {
		t.Fatal("resolution failure wrote a bind")
	}
}

func TestBindMissingStateAndNonRepository(t *testing.T) {
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "topic")
	expectBindFailure(t, bindAt(t, repo, "--folder", "missing"), 2, "flow: invalid folder: 'missing' -- no state.tsv to bind to\n")
	if err := os.MkdirAll(filepath.Join(repo, "directory/state.tsv"), 0o755); err != nil {
		t.Fatal(err)
	}
	expectBindFailure(t, bindAt(t, repo, "--folder", "directory"), 2, "flow: invalid folder: 'directory' -- no state.tsv to bind to\n")
	// A valid ambient repository must not replace the caller's non-repository directory.
	t.Chdir(repo)
	dir := t.TempDir()
	writeState(t, dir, "f")
	expectBindFailure(t, bindAt(t, dir, "--folder", "f"), 2, "flow: invalid branch: '' -- a detached HEAD has no branch to bind\n")
}

func TestBindHeaderOnlyDefaults(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "topic")
	writeState(t, repo, flowA)
	mustExit(t, bindAt(t, repo, "--folder", flowA), 0, "header-only flow")
	got := cells(bindRows(t, repo, flowA)[0])
	if got[2] != "intent" || got[8] != "intent resume" {
		t.Fatalf("header-only defaults = %q", got)
	}
}

func TestBindPropagatesRowValidationFailure(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "topic")
	writeState(t, repo, flowA, row("2026-11-01T00:00:00Z", "not-a-phase", "create", "file", "ok", "seed", "none", "spec go"))
	before := bindBody(t, repo, flowA)
	expectBindFailure(t, bindAt(t, repo, "--folder", flowA), 2, "flow: invalid phase: 'not-a-phase' -- expected one of: intent spec decompose execute gate\n")
	if bindBody(t, repo, flowA) != before {
		t.Fatal("invalid inherited phase wrote a row")
	}
}

func TestBindRelativeFolderAtRootAndLastOptionWins(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	seedFlow(t, repo, flowA, "")
	seedFlow(t, repo, flowB, "")
	commitAll(t, repo, "flows")
	git(t, repo, "switch", "-q", "-c", "topic")
	inner := filepath.Join(repo, "inner")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	beforeA, beforeB := bindBody(t, repo, flowA), bindBody(t, repo, flowB)
	mustExit(t, bindAt(t, inner, "--folder", flowA, "--folder", flowB), 0, "relative last folder")
	if bindBody(t, repo, flowA) != beforeA || bindBody(t, repo, flowB) == beforeB {
		t.Fatal("bind did not select the last folder relative to the repository root")
	}
	if _, err := os.Stat(filepath.Join(inner, "docs")); !os.IsNotExist(err) {
		t.Fatalf("bind resolved the folder below the caller's directory: %v", err)
	}
}

func TestBindInheritsLatestRowAndDefaults(t *testing.T) {
	t.Parallel()
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "latest", true: "defaults"}[empty], func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			git(t, repo, "switch", "-q", "-c", "topic")
			phase, next := "execute", "gate check branch"
			if empty {
				phase, next = "", ""
			}
			writeState(t, repo, flowA,
				row("2026-11-01T00:00:02Z", phase, "verify", "task", "ok", "new", "none", next),
				row("2026-11-01T00:00:01Z", "spec", "create", "spec.md", "ok", "old", "none", "decompose go"))
			mustExit(t, bindAt(t, repo, "--folder", flowA), 0, "inherit")
			if empty {
				phase, next = "intent", "intent resume"
			}
			got := cells(bindRows(t, repo, flowA)[0])
			if got[2] != phase || got[8] != next {
				t.Fatalf("inherited phase/next = %q/%q, want %q/%q", got[2], got[8], phase, next)
			}
		})
	}
}
