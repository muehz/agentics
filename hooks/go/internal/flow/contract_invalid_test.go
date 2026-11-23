package flow

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// invalidLines returns the invalid: lines of a status output, in order.
func invalidLines(stdout string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		if strings.HasPrefix(line, "invalid:") {
			lines = append(lines, line)
		}
	}
	return lines
}

// invalidLinesAre expects exactly the invalid: lines want, in that order,
// after the placeholders of each are filled from the fixture.
func invalidLinesAre(expand func(fx *contractFixture, text string) string, want ...string) func(t *testing.T, fx *contractFixture, res result) {
	return func(t *testing.T, fx *contractFixture, res result) {
		t.Helper()
		var expanded []string
		for _, line := range want {
			expanded = append(expanded, expand(fx, line))
		}
		if got := invalidLines(res.stdout); !slices.Equal(got, expanded) {
			t.Errorf("invalid: lines\n got %q\nwant %q", got, expanded)
		}
	}
}

// invalidBetweenUnaccountedAndSessions expects the invalid: lines to follow
// the unaccounted: line directly and the sessions: line to follow them.
func invalidBetweenUnaccountedAndSessions(t *testing.T, _ *contractFixture, res result) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(res.stdout, "\n"), "\n")
	at := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, "unaccounted:") })
	n := len(invalidLines(res.stdout))
	if at < 0 || n == 0 || at+n+1 >= len(lines) {
		t.Fatalf("want %d invalid: lines between unaccounted: and sessions: in %q", n, res.stdout)
	}
	for i, line := range lines[at+1 : at+1+n] {
		if !strings.HasPrefix(line, "invalid:") {
			t.Errorf("line %d after unaccounted: is %q, want an invalid: line", i+1, line)
		}
	}
	if !strings.HasPrefix(lines[at+1+n], "sessions:") {
		t.Errorf("the line after the invalid: lines is %q, want sessions:", lines[at+1+n])
	}
}

func invalidCases() []contractCase {
	status := []string{"status", "--folder", contractFolder}
	wrong := strings.Repeat("b", 40)
	absent := strings.Repeat("0", 39) + "1"
	expand := func(fx *contractFixture, text string) string {
		pairs := []string{"<wrong>", wrong, "<absent>", absent}
		for _, key := range []string{"T", "M"} {
			if value := fx.values[key]; value != "" {
				pairs = append(pairs, "<"+key+">", value)
			}
		}
		return strings.NewReplacer(pairs...).Replace(text)
	}
	verify := func(ts, target, status, evidence string) string {
		return row("2026-11-01T"+ts+":00Z", "execute", "verify", target, status, "green", evidence, "execute go")
	}
	// inRepo is resolveRepo with a state file whose rows name its commits.
	inRepo := func(rows ...string) func(t *testing.T) *contractFixture {
		return withFixtureSteps(resolveRepo, func(t *testing.T, fx *contractFixture) {
			t.Helper()
			var filled []string
			for _, r := range rows {
				filled = append(filled, expand(fx, r))
			}
			writeState(t, fx.repo, fx.folder, filled...)
		})
	}
	// inLocalRepo is inRepo with the flow in the local store, where status
	// runs Git at the repository root rather than in the flow folder.
	inLocalRepo := func(rows ...string) func(t *testing.T) *contractFixture {
		return withFixtureSteps(resolveRepo, func(t *testing.T, fx *contractFixture) {
			t.Helper()
			fx.local = true
			body := headerRow + "\n"
			for _, r := range rows {
				body += expand(fx, r) + "\n"
			}
			writeFile(t, fx.repo, ".git/agentics/work/"+fx.folder+"/state.tsv", body)
		})
	}
	// namedFile adds an untracked file named after the fixture value key in
	// rel, the directory status or log runs Git in.
	namedFile := func(rel, key string) fixtureStep {
		return func(t *testing.T, fx *contractFixture) {
			t.Helper()
			writeFile(t, fx.repo, rel+fx.values[key], "not a commit\n")
		}
	}
	printed := func(name string, setup func(t *testing.T) *contractFixture, want ...string) contractCase {
		return contractCase{
			name:  "invalid/" + name,
			setup: setup,
			steps: []contractStep{{args: status, check: invalidLinesAre(expand, want...)}},
		}
	}
	notAPair := func(token string) string { return token + " is not a key=value pair" }

	var cases []contractCase
	for i, cell := range []struct{ evidence, token string }{
		{"docs/specs/2026-11/feature-x/plan.md", "docs/specs/2026-11/feature-x/plan.md"},
		{"none", "none"},
		{"uncommitted: tests pass in the worktree", "uncommitted:"},
		{"tests pass C:/work/repo/out.txt", "tests"},
		{"abc1234", "abc1234"},
		// The writer turns auto into the commit form before it stores a row,
		// so a stored auto cell was written by hand and names no commit.
		{"auto", "auto"},
		{"auto:abc1234", "auto:abc1234"},
	} {
		cases = append(cases, printed("a-hand-written-cell-prints-one-line-"+strconv.Itoa(i+1),
			newState(verify("11:00", "task-1", "ok", cell.evidence)),
			"invalid: 2026-11-01T11:00:00Z s1 verify task-1: "+notAPair(cell.token)))
	}
	var many []string
	var manyLines []string
	for i := 1; i <= 25; i++ {
		target := "task-" + strconv.Itoa(i)
		many = append(many, verify("11:00", target, "ok", "none"))
		manyLines = append(manyLines, "invalid: 2026-11-01T11:00:00Z s1 verify "+target+": "+notAPair("none"))
	}
	return append(cases,
		printed("a-slice-value-that-is-no-slice-list-prints-one-line",
			newState(verify("11:00", "task-1", "failed", "cause=repair slice=S1,not-a-slice")),
			"invalid: 2026-11-01T11:00:00Z s1 verify task-1: slice=S1,not-a-slice is not a comma-separated list of S<n> ids"),
		printed("bare-commits-are-checked-by-the-merge-test",
			inRepo(
				verify("11:00", "task-1", "ok", "commit=<T>"),
				verify("11:01", "task-2", "ok", "commit=<M>"),
				verify("11:02", "task-3", "ok", "commit=<absent> patch-id=<wrong>"),
				verify("11:03", "red-task-1", "failed", "commit=<T>"),
				verify("11:04", "fix", "ok", "commit=<T>"),
				verify("11:05", "task-4", "ok", "commit=<absent>"),
				verify("11:06", "wave-1", "ok", "commit=<absent>"),
			),
			"invalid: 2026-11-01T11:00:00Z s1 verify task-1: commit <T> is not a merge, so it needs patch-id=",
			"invalid: 2026-11-01T11:03:00Z s1 verify red-task-1: commit <T> is not a merge, so it needs patch-id=",
			"invalid: 2026-11-01T11:04:00Z s1 verify fix: commit <T> is not a merge, so it needs patch-id=",
			"invalid: 2026-11-01T11:05:00Z s1 verify task-4: commit <absent> does not resolve to a commit"),
		contractCase{
			name: "invalid/a-snapshot-hash-with-a-patch-id-and-a-bare-wave-commit-are-mismatches",
			setup: inRepo(
				verify("11:00", "task-1", "ok", "commit=<absent> patch-id=<wrong>"),
				verify("11:01", "wave-1", "ok", "commit=<absent>"),
			),
			steps: []contractStep{{
				args:        status,
				stdoutCount: []lineCount{{"invalid:", 0}},
				check: func(t *testing.T, fx *contractFixture, res result) {
					t.Helper()
					checkStream(t, "stdout", res.stdout, []string{
						expand(fx, "mismatch: verify task-1 commit <absent> is not an ancestor of HEAD and patch-id <wrong> matches no commit on this branch"),
						expand(fx, "mismatch: verify wave-1 commit <absent> is not an ancestor of HEAD"),
					}, nil, nil)
				},
			}},
		},
		printed("waive-and-demote-targets-outside-the-operands-print-lines",
			newState(
				row("2026-11-01T11:00:00Z", "execute", "waive", "wave-3-aggregate", "ok", "waived", "none", "execute go"),
				row("2026-11-01T11:01:00Z", "execute", "demote", "2", "ok", "demoted", "none", "execute go"),
			),
			"invalid: 2026-11-01T11:00:00Z s1 waive wave-3-aggregate: head wave-3-aggregate is not in the Operands list of flow-state.md",
			"invalid: 2026-11-01T11:01:00Z s1 demote 2: demote names wave-<id>"),
		printed("a-later-valid-row-supersedes-and-the-first-one-is-named",
			newState(
				// File order differs from ts order, which decides what is later.
				verify("13:00", "task-1", "failed", "cause=repair"),
				verify("11:00", "task-1", "ok", "none"),
				verify("11:30", "task-1", "ok", "abc1234"),
				verify("12:00", "task-1", "failed", "cause=context"),
				verify("10:00", "task-2", "failed", "cause=repair"),
				verify("11:00", "task-2", "ok", "none"),
				verify("11:00", "task-3", "ok", "none"),
				verify("12:00", "task-3", "ok", "tests pass"),
				verify("12:00", "task-4", "failed", "cause=repair"),
			),
			"invalid: 2026-11-01T11:00:00Z s1 verify task-1: "+notAPair("none")+" (superseded 2026-11-01T12:00:00Z)",
			"invalid: 2026-11-01T11:00:00Z s1 verify task-2: "+notAPair("none"),
			"invalid: 2026-11-01T11:00:00Z s1 verify task-3: "+notAPair("none"),
			"invalid: 2026-11-01T11:30:00Z s1 verify task-1: "+notAPair("abc1234")+" (superseded 2026-11-01T12:00:00Z)",
			"invalid: 2026-11-01T12:00:00Z s1 verify task-3: "+notAPair("tests")),
		printed("the-merge-test-reads-the-repository-of-a-local-store-flow",
			inLocalRepo(
				verify("11:00", "task-1", "ok", "commit=<M>"),
				verify("11:01", "task-2", "ok", "commit=<T>"),
			),
			"invalid: 2026-11-01T11:01:00Z s1 verify task-2: commit <T> is not a merge, so it needs patch-id="),
		printed("a-file-named-like-a-merge-commit-leaves-the-merge-valid",
			withFixtureSteps(inRepo(
				verify("11:00", "task-1", "ok", "commit=<M>"),
				verify("11:01", "task-2", "ok", "commit=<T>"),
			), namedFile(contractFolder+"/", "M"), namedFile(contractFolder+"/", "T")),
			"invalid: 2026-11-01T11:01:00Z s1 verify task-2: commit <T> is not a merge, so it needs patch-id="),
		contractCase{
			name:  "invalid/log-writes-a-bare-merge-commit-beside-a-file-named-like-it",
			setup: withFixtureSteps(resolveRepo, namedFile("", "M")),
			steps: []contractStep{{argsOf: func(fx *contractFixture) []string {
				return contractLog("execute", "verify", "task-1", "ok", contractWhy, "commit="+fx.values["M"], contractNext)
			}}},
			check: rowsAre(func(fx *contractFixture) []string {
				return []string{cellsRow("execute", "verify", "task-1", "ok", contractWhy, "commit="+fx.values["M"], contractNext)}
			}),
		},
		printed("a-later-row-that-fails-the-merge-test-does-not-supersede",
			inRepo(
				verify("11:00", "task-1", "ok", "none"),
				verify("12:00", "task-1", "ok", "commit=<T>"),
				verify("11:00", "task-2", "ok", "none"),
				verify("12:00", "task-2", "ok", "commit=<M>"),
			),
			"invalid: 2026-11-01T11:00:00Z s1 verify task-1: "+notAPair("none"),
			"invalid: 2026-11-01T11:00:00Z s1 verify task-2: "+notAPair("none")+" (superseded 2026-11-01T12:00:00Z)",
			"invalid: 2026-11-01T12:00:00Z s1 verify task-1: commit <T> is not a merge, so it needs patch-id="),
		printed("a-waive-row-is-never-superseded",
			newState(
				row("2026-11-01T11:00:00Z", "execute", "waive", "wave-3-aggregate", "ok", "waived", "none", "execute go"),
				row("2026-11-01T12:00:00Z", "execute", "waive", "aggregate-review", "ok", "waived", "none", "execute go"),
			),
			"invalid: 2026-11-01T11:00:00Z s1 waive wave-3-aggregate: head wave-3-aggregate is not in the Operands list of flow-state.md"),
		printed("every-invalid-row-prints-its-line-without-a-cap", newState(many...), manyLines...),
		printed("an-empty-session-prints-as-a-dash",
			newState(strings.Join([]string{"2026-11-01T11:00:00Z", "", "execute", "verify", "task-1", "ok", "green", "none", "execute go"}, "\t")),
			"invalid: 2026-11-01T11:00:00Z - verify task-1: "+notAPair("none")),
		printed("diverge-and-revert-rows-print-no-line",
			newState(
				row("2026-11-01T11:00:00Z", "execute", "diverge", "task-1", "ok", "diverged", "none", "execute go"),
				row("2026-11-01T11:01:00Z", "execute", "revert", "task-1", "ok", "reverted", "tests pass", "execute go"),
			)),
		contractCase{
			name: "invalid/the-lines-sit-between-unaccounted-and-sessions",
			setup: newState(
				row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"),
				verify("11:00", "task-1", "ok", "none"),
				verify("11:01", "task-2", "ok", "abc1234"),
			),
			steps: []contractStep{
				{args: status, stdoutCount: []lineCount{{"invalid:", 2}}, check: invalidBetweenUnaccountedAndSessions},
				{args: append(slices.Clone(status), "--metrics"), stdoutCount: []lineCount{{"invalid:", 2}}, check: invalidBetweenUnaccountedAndSessions},
			},
		},
	)
}

func TestInvalidContract(t *testing.T) {
	t.Parallel()
	for _, c := range invalidCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			runContractCase(t, c)
		})
	}
}

// TestInvalidMergeTestRunsAtTheRepositoryRoot pins where the merge test of a
// local-store flow runs. The store sits in the Git directory, and when that
// directory is not named .git, as with --separate-git-dir, Git run inside it
// finds an implicit bare repository, which safe.bareRepository=explicit
// refuses. Run from the repository root, the test still reads the commits.
func TestInvalidMergeTestRunsAtTheRepositoryRoot(t *testing.T) {
	fx := resolveRepo(t)
	git(t, fx.repo, "init", "-q", "--separate-git-dir="+filepath.Join(t.TempDir(), "store.git"))
	common := gitOut(t, fx.repo, "rev-parse", "--path-format=absolute", "--git-common-dir")
	verify := func(ts, target, evidence string) string {
		return row("2026-11-01T"+ts+":00Z", "execute", "verify", target, "ok", "green", evidence, "execute go")
	}
	writeFile(t, common, "agentics/work/"+fx.folder+"/state.tsv", headerRow+"\n"+
		verify("11:00", "task-1", "commit="+fx.values["M"])+"\n"+
		verify("11:01", "task-2", "commit="+fx.values["T"])+"\n")
	// Production Git processes inherit the test's environment.
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "safe.bareRepository")
	t.Setenv("GIT_CONFIG_VALUE_0", "explicit")
	res := runFlow(t, fx.repo, nil, "status", "--folder", contractFolder)
	mustExit(t, res, 0, "flow status")
	checkStream(t, "stdout", res.stdout, []string{"storage: local"}, nil, nil)
	want := []string{"invalid: 2026-11-01T11:01:00Z s1 verify task-2: commit " + fx.values["T"] + " is not a merge, so it needs patch-id="}
	if got := invalidLines(res.stdout); !slices.Equal(got, want) {
		t.Errorf("invalid: lines\n got %q\nwant %q", got, want)
	}
}
