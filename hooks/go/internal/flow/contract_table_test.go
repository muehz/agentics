package flow

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// contractFolder is the flow folder of every contract fixture, relative to the fixture repository.
const contractFolder = "docs/specs/2026-11/feature-c"

// contractFixture is what a case's setup hands to its steps and checks.
type contractFixture struct {
	repo   string
	folder string
	// values carries what a setup computed for later steps, such as a commit id.
	values map[string]string
	// local places the flow in the local store of repo instead of under its work root.
	local bool
}

// statePath is the flow's state.tsv in the store the fixture names.
func (fx *contractFixture) statePath() string {
	if fx.local {
		return filepath.Join(localStoreOf(fx.repo, fx.folder), "state.tsv")
	}
	return filepath.Join(fx.repo, filepath.FromSlash(fx.folder), "state.tsv")
}

// localStoreOf is the local-store directory of folder in repo, a repository
// whose Git directory is .git.
func localStoreOf(repo, folder string) string {
	return filepath.Join(repo, ".git", "agentics", "work", filepath.FromSlash(folder))
}

// lineCount expects exactly n output lines that start with prefix; n is 0 for a line that must be absent.
type lineCount struct {
	prefix string
	n      int
}

// contractStep is one flow call and what it must produce.
type contractStep struct {
	// before changes the fixture ahead of the call, such as a merge between two calls.
	before func(t *testing.T, fx *contractFixture)
	// dir is the caller's directory below the fixture repository; empty is the repository itself.
	dir string
	// dirOf replaces dir with a directory that depends on the fixture, such as a second clone.
	dirOf func(fx *contractFixture) string
	args  []string
	// argsOf replaces args when a call depends on the fixture.
	argsOf func(fx *contractFixture) []string
	env    []string
	code   int
	// stdoutLines and stderrLines must each equal a whole output line.
	stdoutLines, stderrLines []string
	// stdoutHas and stderrHas must each occur somewhere in the stream.
	stdoutHas, stderrHas []string
	stdoutCount          []lineCount
	stderrCount          []lineCount
	check                func(t *testing.T, fx *contractFixture, res result)
}

// contractCase is one row of the contract table. Its name starts with the
// prefix of its group, so that -run "TestContractTable/<prefix>" selects the group.
type contractCase struct {
	name  string
	setup func(t *testing.T) *contractFixture
	steps []contractStep
	// rows, when set, are the data rows of state.tsv after the last step,
	// without the ts and session cells.
	rows  []string
	check func(t *testing.T, fx *contractFixture)
	// serial keeps the case out of the parallel set: it changes process-global
	// state (t.Setenv, t.Chdir, os.Setenv), which parallel cases would read.
	serial bool
}

// contractGroups lists the case groups; a new group is added here.
var contractGroups = []func() []contractCase{
	formatCases,
	folderCases,
	eventsCases,
	formsCases,
	hashCases,
	approvalGuardCases,
	approvalStatusCases,
	ledgerCases,
	entryCases,
	entryStatusCases,
	lookupCases,
	closingCases,
	slicebindCases,
	sliceverdictCases,
	autoCases,
	unaccountedCases,
	settingCases,
	localCases,
	localunassignedCases,
	locallookupCases,
	pathCases,
}

func TestContractTable(t *testing.T) {
	for _, group := range contractGroups {
		for _, c := range group() {
			t.Run(c.name, func(t *testing.T) {
				if !c.serial {
					t.Parallel()
				}
				runContractCase(t, c)
			})
		}
	}
}

func runContractCase(t *testing.T, c contractCase) {
	t.Helper()
	fx := c.setup(t)
	for i, step := range c.steps {
		if step.before != nil {
			step.before(t, fx)
		}
		args := step.args
		if step.argsOf != nil {
			args = step.argsOf(fx)
		}
		what := "step " + strconv.Itoa(i+1) + " flow " + strings.Join(args, " ")
		dir := filepath.Join(fx.repo, filepath.FromSlash(step.dir))
		if step.dirOf != nil {
			dir = step.dirOf(fx)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		res := runFlow(t, dir, step.env, args...)
		mustExit(t, res, step.code, what)
		checkStream(t, what+" stdout", res.stdout, step.stdoutLines, step.stdoutHas, step.stdoutCount)
		checkStream(t, what+" stderr", res.stderr, step.stderrLines, step.stderrHas, step.stderrCount)
		if step.check != nil {
			step.check(t, fx, res)
		}
	}
	if c.rows != nil {
		got := contractRows(t, fx)
		if strings.Join(got, "\n") != strings.Join(c.rows, "\n") {
			t.Errorf("rows:\n got %q\nwant %q", got, c.rows)
		}
	}
	if c.check != nil {
		c.check(t, fx)
	}
}

func checkStream(t *testing.T, what, text string, exact, has []string, counts []lineCount) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for _, want := range exact {
		found := false
		for _, line := range lines {
			found = found || line == want
		}
		if !found {
			t.Errorf("%s: no line %q in %q", what, want, text)
		}
	}
	for _, want := range has {
		if !strings.Contains(text, want) {
			t.Errorf("%s: %q not in %q", what, want, text)
		}
	}
	for _, want := range counts {
		n := 0
		for _, line := range lines {
			if strings.HasPrefix(line, want.prefix) {
				n++
			}
		}
		if n != want.n {
			t.Errorf("%s: %d lines start with %q, want %d: %q", what, n, want.prefix, want.n, text)
		}
	}
}

// contractRows returns the data rows of the fixture's state.tsv without their ts and session cells.
func contractRows(t *testing.T, fx *contractFixture) []string {
	t.Helper()
	var rows []string
	for _, line := range dataLines(readLines(t, fx.statePath())) {
		c := cells(line)
		if len(c) != 9 {
			t.Fatalf("row has %d cells, want 9: %q", len(c), line)
		}
		rows = append(rows, strings.Join(c[2:], "\t"))
	}
	return rows
}

// cellsRow builds an expected row for contractCase.rows.
func cellsRow(phase, event, target, status, why, evidence, next string) string {
	return strings.Join([]string{phase, event, target, status, why, evidence, next}, "\t")
}

// contractLog builds a flow log call into the fixture folder.
func contractLog(c ...string) []string {
	return append(append([]string{"log"}, c...), "--folder", contractFolder)
}

// emptyRepo is a setup with a fresh repository and no flow folder.
func emptyRepo(t *testing.T) *contractFixture {
	t.Helper()
	return &contractFixture{repo: newRepo(t), folder: contractFolder, values: map[string]string{}}
}

// stateBody is a setup whose state.tsv holds exactly body.
func stateBody(body string) func(t *testing.T) *contractFixture {
	return func(t *testing.T) *contractFixture {
		t.Helper()
		fx := emptyRepo(t)
		writeFile(t, fx.repo, fx.folder+"/state.tsv", body)
		return fx
	}
}

// stateAt is a setup whose state.tsv starts with the given version line, then
// the header row, and holds the given rows.
func stateAt(version string, rows ...string) func(t *testing.T) *contractFixture {
	body := headerRow + "\n"
	if version != "" {
		body = version + "\n" + body
	}
	if len(rows) > 0 {
		body += strings.Join(rows, "\n") + "\n"
	}
	return stateBody(body)
}

// newState is a setup whose state.tsv starts with the header row, as a file
// the helper creates does, and holds the given rows.
func newState(rows ...string) func(t *testing.T) *contractFixture {
	return stateAt("", rows...)
}

// firstLineIs fails unless the state.tsv starts with want and the header row.
func firstLineIs(want string) func(t *testing.T, fx *contractFixture) {
	return func(t *testing.T, fx *contractFixture) {
		t.Helper()
		lines := readLines(t, fx.statePath())
		if lines[0] != want || len(lines) < 2 || lines[1] != headerRow {
			t.Errorf("file starts %q, want %q and the header row", lines[:min(2, len(lines))], want)
		}
	}
}

// startsWithHeader fails unless the state.tsv starts with the header row, with
// no version line before it.
func startsWithHeader(t *testing.T, fx *contractFixture) {
	t.Helper()
	if lines := readLines(t, fx.statePath()); lines[0] != headerRow {
		t.Errorf("file starts %q, want the header row %q", lines[0], headerRow)
	}
}

func formatCases() []contractCase {
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	seededCells := cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	challenge := []string{"intent", "challenge", "intent.md", "ok", "direction challenged", "none", "spec go"}
	status := []string{"status", "--folder", contractFolder}
	otherHeader := "ts\tsession\tphase\tevent\ttarget\tstatus\twhy\tproof\tnext"
	return []contractCase{
		{
			name:  "format/new-file-has-no-version-line",
			setup: emptyRepo,
			steps: []contractStep{
				{args: contractLog("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")},
				// The file's only row is its second line, so a reader that skips
				// two lines finds no rows.
				{args: status, stdoutLines: []string{"phase: intent (create, ok)"}, stdoutCount: []lineCount{{"mismatch:", 0}}},
			},
			rows:  []string{seededCells},
			check: startsWithHeader,
		},
		{
			name:  "format/version-1-file-keeps-its-line",
			setup: stateAt(legacyVersionLine, seeded),
			steps: []contractStep{
				{args: contractLog(challenge...)},
				{args: status, stdoutLines: []string{"phase: intent (challenge, ok)"}, stdoutCount: []lineCount{{"mismatch:", 0}}},
			},
			rows:  []string{seededCells, cellsRow(challenge[0], challenge[1], challenge[2], challenge[3], challenge[4], challenge[5], challenge[6])},
			check: firstLineIs(legacyVersionLine),
		},
		{
			name:  "format/other-header-row-reports-one-mismatch",
			setup: stateBody(otherHeader + "\n" + seeded + "\n"),
			steps: []contractStep{{
				args:        status,
				stdoutLines: []string{"mismatch: header row is '" + otherHeader + "', expected '" + headerRow + "'"},
				stdoutCount: []lineCount{{"mismatch:", 1}},
			}},
		},
		{
			name:  "format/version-1-file-with-another-header-reports-it",
			setup: stateBody(legacyVersionLine + "\n" + otherHeader + "\n" + seeded + "\n"),
			steps: []contractStep{{
				args:        status,
				stdoutLines: []string{"mismatch: header row is '" + otherHeader + "', expected '" + headerRow + "'"},
				stdoutCount: []lineCount{{"mismatch:", 1}},
			}},
		},
		{
			name:  "format/status-accepts-a-version-1-file",
			setup: stateAt(legacyVersionLine, seeded),
			steps: []contractStep{{
				args:        status,
				stdoutLines: []string{"phase: intent (create, ok)"},
				stdoutCount: []lineCount{{"mismatch:", 0}},
			}},
		},
	}
}

// onBranch is a setup that runs inner and then switches the repository to a new branch.
func onBranch(branch string, inner func(t *testing.T) *contractFixture) func(t *testing.T) *contractFixture {
	return func(t *testing.T) *contractFixture {
		t.Helper()
		fx := inner(t)
		git(t, fx.repo, "switch", "-q", "-c", branch)
		return fx
	}
}

// nothingUnder fails when a step created rel below the fixture repository.
func nothingUnder(rel string) func(t *testing.T, fx *contractFixture) {
	return func(t *testing.T, fx *contractFixture) {
		t.Helper()
		if _, err := os.Lstat(filepath.Join(fx.repo, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("%s exists below the repository (err %v); the folder must resolve at the repository root", rel, err)
		}
	}
}

func folderCases() []contractCase {
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	seededCells := cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	create := []string{"intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"}
	flowLine := "flow: " + contractFolder
	absolute := func(command string) func(fx *contractFixture) []string {
		return func(fx *contractFixture) []string {
			return []string{command, "--folder", filepath.Join(fx.repo, filepath.FromSlash(contractFolder))}
		}
	}
	outside := func(fx *contractFixture) string { return fx.values["outside"] }
	return []contractCase{
		{
			name:  "folder/log-from-a-subdirectory-writes-at-the-root",
			setup: emptyRepo,
			steps: []contractStep{{dir: "sub", args: contractLog(create...)}},
			rows:  []string{seededCells},
			check: nothingUnder("sub/docs"),
		},
		{
			name:  "folder/status-from-a-subdirectory-reads-at-the-root",
			setup: newState(seeded),
			steps: []contractStep{{
				dir:         "sub",
				args:        []string{"status", "--folder", contractFolder},
				stdoutLines: []string{flowLine, "phase: intent (create, ok)"},
			}},
		},
		{
			name:  "folder/bind-from-a-subdirectory-writes-at-the-root",
			setup: onBranch("topic", newState(seeded)),
			steps: []contractStep{{dir: "sub", args: []string{"bind", "--folder", contractFolder}}},
			rows:  []string{seededCells, cellsRow("intent", "bind", "topic", "ok", "bound topic to this flow", "none", "spec go")},
			check: nothingUnder("sub/docs"),
		},
		{
			name:  "folder/status-prints-an-absolute-folder-inside-the-repository-root-relative",
			setup: newState(seeded),
			steps: []contractStep{{dir: "sub", argsOf: absolute("status"), stdoutLines: []string{flowLine}}},
		},
		{
			name:  "folder/status-prints-a-looked-up-folder-root-relative",
			setup: onBranch("topic", newState(seeded)),
			steps: []contractStep{{dir: "sub", args: []string{"status"}, stdoutLines: []string{flowLine}}},
		},
		{
			name: "folder/absolute-folder-outside-the-repository-is-used-and-printed-as-given",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := emptyRepo(t)
				fx.values["outside"] = filepath.Join(t.TempDir(), "flow")
				return fx
			},
			steps: []contractStep{
				{dir: "sub", argsOf: func(fx *contractFixture) []string {
					return append(append([]string{"log"}, create...), "--folder", outside(fx))
				}},
				{dir: "sub", argsOf: func(fx *contractFixture) []string { return []string{"status", "--folder", outside(fx)} },
					check: func(t *testing.T, fx *contractFixture, res result) {
						t.Helper()
						if lines := strings.Split(res.stdout, "\n"); lines[0] != "flow: "+outside(fx) {
							t.Errorf("status starts %q, want the folder as given, %q", lines[0], outside(fx))
						}
					}},
			},
			check: func(t *testing.T, fx *contractFixture) {
				t.Helper()
				rows := readLines(t, filepath.Join(outside(fx), "state.tsv"))
				if len(rows) != 2 {
					t.Errorf("outside state.tsv has %d lines, want the header and one row: %q", len(rows), rows)
				}
				nothingUnder("docs")(t, fx)
				nothingUnder("sub/docs")(t, fx)
			},
		},
		{
			name:   "folder/outside-a-repository-resolves-at-the-working-directory",
			serial: true,
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				repo := t.TempDir()
				// The flow's Git calls inherit the process environment, not a step's, and the
				// temporary directory may itself lie inside a repository.
				t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(repo))
				return &contractFixture{repo: repo, folder: "sub/rel", values: map[string]string{}}
			},
			steps: []contractStep{
				{dir: "sub", args: append(append([]string{"log"}, create...), "--folder", "rel")},
				{dir: "sub", args: []string{"status", "--folder", "rel"}, stdoutLines: []string{"flow: rel"}},
			},
			rows:  []string{seededCells},
			check: nothingUnder("rel"),
		},
	}
}

// withOlderCommit is a setup that runs inner, records the current commit as
// the value "older" and its patch id as "older-patch", and then adds a commit,
// so that a call naming the older commit cannot pass by reading HEAD.
func withOlderCommit(inner func(t *testing.T) *contractFixture) func(t *testing.T) *contractFixture {
	return func(t *testing.T) *contractFixture {
		t.Helper()
		fx := inner(t)
		fx.values["older"] = gitOut(t, fx.repo, "rev-parse", "HEAD")
		fx.values["older-patch"] = patchIDOf(fx.repo, fx.values["older"])
		writeFile(t, fx.repo, "later.txt", "later\n")
		git(t, fx.repo, "add", "later.txt")
		git(t, fx.repo, "commit", "-q", "-m", "later")
		return fx
	}
}

// rowsAre compares the data rows of state.tsv with rows that depend on the fixture.
func rowsAre(want func(fx *contractFixture) []string) func(t *testing.T, fx *contractFixture) {
	return func(t *testing.T, fx *contractFixture) {
		t.Helper()
		got, rows := contractRows(t, fx), want(fx)
		if strings.Join(got, "\n") != strings.Join(rows, "\n") {
			t.Errorf("rows:\n got %q\nwant %q", got, rows)
		}
	}
}

// noStateFile fails when a refused call left a state.tsv behind.
func noStateFile(t *testing.T, fx *contractFixture) {
	t.Helper()
	if _, err := os.Lstat(fx.statePath()); !os.IsNotExist(err) {
		t.Errorf("a refused row must create no state.tsv (err %v)", err)
	}
}

// contractWhy and contractNext fill the why and next cells of a case that is about the other cells.
const contractWhy, contractNext = "recorded", "execute go"

// acceptedLog is a case whose single log call in the execute phase is stored as given.
func acceptedLog(name, event, target, status, evidence string) contractCase {
	return contractCase{
		name:  name,
		setup: emptyRepo,
		steps: []contractStep{{args: contractLog("execute", event, target, status, contractWhy, evidence, contractNext)}},
		rows:  []string{cellsRow("execute", event, target, status, contractWhy, evidence, contractNext)},
	}
}

// refusedLog is a case whose single log call exits 2 with the message and writes nothing.
func refusedLog(name, event, target, status, evidence, message string) contractCase {
	return contractCase{
		name:  name,
		setup: emptyRepo,
		steps: []contractStep{{
			args:        contractLog("execute", event, target, status, contractWhy, evidence, contractNext),
			code:        2,
			stderrLines: []string{"flow: invalid " + message},
		}},
		check: noStateFile,
	}
}

func eventsCases() []contractCase {
	const why, next = contractWhy, contractNext
	accepted := func(name, event, target, status, evidence string) contractCase {
		return acceptedLog("events/"+name, event, target, status, evidence)
	}
	refused := func(name, event, target, status, evidence, message string) contractCase {
		return refusedLog("events/"+name, event, target, status, evidence, message)
	}
	commitArgs := func(kind string) func(fx *contractFixture) []string {
		return func(fx *contractFixture) []string {
			return contractLog("execute", "commit", kind, "ok", why, "auto:"+fx.values["older"], next)
		}
	}
	commitRow := func(fx *contractFixture, kind string) string {
		return cellsRow("execute", "commit", kind, "ok", why, "commit="+fx.values["older"]+" patch-id="+fx.values["older-patch"], next)
	}
	var cases []contractCase
	for _, kind := range []string{"fix", "triage", "user", "housekeeping"} {
		cases = append(cases, contractCase{
			name:  "events/commit-" + kind + "-stores-the-named-commit",
			setup: withOlderCommit(emptyRepo),
			steps: []contractStep{{argsOf: commitArgs(kind)}},
			check: rowsAre(func(fx *contractFixture) []string { return []string{commitRow(fx, kind)} }),
		})
	}
	const (
		commitEvidence = " -- commit evidence is auto:<sha>, naming the commit"
		noteTargets    = " -- a note row names one of: slices slice-S<n> lane-S<n> coverage policy"
		merged         = "merged=0123456789abcdef0123456789abcdef01234567"
	)
	cases = append(cases,
		refused("commit-with-bare-auto-is-refused", "commit", "fix", "ok", "auto", "evidence: 'auto'"+commitEvidence),
		refused("commit-with-an-empty-commit-name-is-refused", "commit", "fix", "ok", "auto:", "evidence: 'auto:'"+commitEvidence),
		refused("commit-with-literal-evidence-is-refused", "commit", "fix", "ok", "commit=abc patch-id=def", "evidence: 'commit=abc patch-id=def'"+commitEvidence),
		refused("commit-naming-no-commit-is-refused", "commit", "fix", "ok", "auto:missing-commit", "evidence: 'auto:missing-commit' -- no such commit to account for"),
		refused("commit-with-another-target-is-refused", "commit", "other", "ok", "auto:HEAD", "target: 'other' -- a commit row names one of: fix triage user housekeeping"),
		refused("commit-with-another-status-is-refused", "commit", "fix", "failed", "auto:HEAD", "status: 'failed' -- a commit row is ok"),

		accepted("drop-cancelled-is-accepted", "drop", "task-1", "cancelled", "none"),
		accepted("drop-takes-a-pointer", "drop", "task-1", "cancelled", "plan.md task 1"),
		refused("drop-with-status-ok-is-refused", "drop", "task-1", "ok", "none", "status: 'ok' -- a drop row is cancelled"),

		accepted("note-slices-is-accepted", "note", "slices", "ok", "slices=S1,S2 final=yes"),
		accepted("note-slices-takes-one-slice-and-no-final-review", "note", "slices", "failed", "final=no slices=S10"),
		refused("note-slices-without-final-is-refused", "note", "slices", "ok", "slices=S1,S2", "evidence: 'slices=S1,S2' -- note slices evidence is exactly: slices final"),
		refused("note-slices-with-another-key-is-refused", "note", "slices", "ok", "slices=S1 final=yes issue=#1", "evidence: 'slices=S1 final=yes issue=#1' -- note slices evidence is exactly: slices final"),
		refused("note-slices-with-a-bad-id-is-refused", "note", "slices", "ok", "slices=S1,2 final=yes", "evidence: 'slices=S1,2 final=yes' -- slices must be a comma-separated list of S<n> ids"),
		refused("note-slices-with-a-bad-final-is-refused", "note", "slices", "ok", "slices=S1 final=maybe", "evidence: 'slices=S1 final=maybe' -- final must be one of: yes no"),

		accepted("note-slice-is-accepted", "note", "slice-S1", "ok", "issue=#12"),
		accepted("note-slice-takes-an-issue-in-another-repository", "note", "slice-S12", "ok", "issue=owner/repo#12"),
		refused("note-slice-without-an-issue-is-refused", "note", "slice-S1", "ok", "none", "evidence: 'none' -- 'none' is not a key=value pair"),
		refused("note-slice-with-a-bad-issue-is-refused", "note", "slice-S1", "ok", "issue=12", "evidence: 'issue=12' -- issue must be #<n> or <owner>/<repo>#<n>"),
		refused("note-slice-zero-is-refused", "note", "slice-S0", "ok", "issue=#12", "target: 'slice-S0'"+noteTargets),

		accepted("note-lane-is-accepted", "note", "lane-S2", "ok", merged),
		refused("note-lane-without-a-merge-is-refused", "note", "lane-S2", "ok", "issue=#12", "evidence: 'issue=#12' -- unknown key issue; expected: merged"),
		refused("note-lane-with-a-bad-commit-id-is-refused", "note", "lane-S2", "ok", "merged=main", "evidence: 'merged=main' -- merged must be a commit id"),
		refused("note-lane-without-a-number-is-refused", "note", "lane-S", "ok", merged, "target: 'lane-S'"+noteTargets),

		accepted("note-coverage-is-accepted", "note", "coverage", "ok", "none"),
		accepted("note-coverage-takes-a-pointer", "note", "coverage", "failed", "coverage.txt line 4"),
		refused("note-coverage-pending-is-refused", "note", "coverage", "pending", "none", "status: 'pending' -- a note row is ok or failed"),

		accepted("note-policy-is-accepted", "note", "policy", "ok", "skill=execute version=1.4.0"),
		refused("note-policy-without-a-version-is-refused", "note", "policy", "ok", "skill=execute", "evidence: 'skill=execute' -- note policy evidence is exactly: skill version"),

		refused("note-with-another-target-is-refused", "note", "other", "ok", "none", "target: 'other'"+noteTargets),
	)
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	return append(cases, contractCase{
		name:  "events/version-1-file-takes-each-new-event",
		setup: withOlderCommit(stateAt(legacyVersionLine, seeded)),
		steps: []contractStep{
			{argsOf: commitArgs("housekeeping")},
			{args: contractLog("execute", "drop", "task-1", "cancelled", why, "none", next)},
			{args: contractLog("execute", "note", "policy", "ok", why, "skill=execute version=1.4.0", next)},
		},
		check: func(t *testing.T, fx *contractFixture) {
			t.Helper()
			firstLineIs(legacyVersionLine)(t, fx)
			rowsAre(func(fx *contractFixture) []string {
				return []string{
					cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"),
					commitRow(fx, "housekeeping"),
					cellsRow("execute", "drop", "task-1", "cancelled", why, "none", next),
					cellsRow("execute", "note", "policy", "ok", why, "skill=execute version=1.4.0", next),
				}
			})(t, fx)
		},
	})
}

// textHashLine is the hashes: line status prints for artifact whose
// normalized text is normalized. The value comes from crypto/sha256 in the
// test, so every platform proves the same literal hash.
func textHashLine(artifact, normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return "hashes: " + artifact + " text=" + hex.EncodeToString(sum[:])
}

// withFiles is a setup with one seeded row and the given files in the flow folder.
func withFiles(files map[string]string) func(t *testing.T) *contractFixture {
	return func(t *testing.T) *contractFixture {
		t.Helper()
		fx := newState(row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"))(t)
		for name, body := range files {
			writeFile(t, fx.repo, fx.folder+"/"+name, body)
		}
		return fx
	}
}

// statusPrints is a case whose one status call prints each wanted line and none of the unwanted ones.
func statusPrints(name string, files map[string]string, want []string, unwanted ...string) contractCase {
	var counts []lineCount
	for _, line := range unwanted {
		counts = append(counts, lineCount{line, 0})
	}
	return contractCase{
		name:  name,
		setup: withFiles(files),
		steps: []contractStep{{args: []string{"status", "--folder", contractFolder}, stdoutLines: want, stdoutCount: counts}},
	}
}

// hashesBetweenNextAndApprovals checks that the lines after next: and before the first approvals: line are exactly want.
func hashesBetweenNextAndApprovals(want ...string) func(t *testing.T, fx *contractFixture, res result) {
	return func(t *testing.T, fx *contractFixture, res result) {
		t.Helper()
		lines := strings.Split(strings.TrimRight(res.stdout, "\n"), "\n")
		start, end := -1, -1
		for i, line := range lines {
			if start < 0 && strings.HasPrefix(line, "next: ") {
				start = i + 1
			}
			if end < 0 && strings.HasPrefix(line, "approvals: ") {
				end = i
			}
		}
		if start < 0 || end < start {
			t.Fatalf("no next: line before an approvals: line in %q", res.stdout)
		}
		if got := lines[start:end]; strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("lines between next: and approvals:\n got %q\nwant %q", got, want)
		}
	}
}

func hashCases() []contractCase {
	const plan = "# Plan\n\n## Tasks\n\n```sh\n  indented\n```\n\nbody line\n"
	const doc = "# Doc\n\n## Goal\n\nthe goal\n"
	const divergences = "\n## Divergences\n\n- 2026-11-01, entry\n"
	const orders = "\n## Standing orders\n\n1. one order\n"
	status := []string{"status", "--folder", contractFolder}
	planLine := textHashLine("plan.md", plan)
	inLine := strings.Replace(plan, "body line", "body  line", 1)
	inFence := strings.Replace(plan, "  indented", "    indented", 1)
	cases := []contractCase{
		statusPrints("hash/value-is-sha256-of-the-normalized-text", map[string]string{"plan.md": plan}, []string{planLine}),
		statusPrints("hash/crlf-copy-prints-the-lf-line", map[string]string{"plan.md": strings.ReplaceAll(plan, "\n", "\r\n")}, []string{planLine}),
		statusPrints("hash/trailing-spaces-leave-the-hash", map[string]string{"plan.md": strings.ReplaceAll(plan, "\n", "  \n")}, []string{planLine}),
		statusPrints("hash/trailing-tabs-leave-the-hash", map[string]string{"plan.md": strings.ReplaceAll(plan, "\n", "\t\t\n")}, []string{planLine}),
		statusPrints("hash/trailing-blank-lines-leave-the-hash", map[string]string{"plan.md": plan + "\n\n \t\r\n\n"}, []string{planLine}),
		statusPrints("hash/missing-final-newline-leaves-the-hash", map[string]string{"plan.md": strings.TrimSuffix(plan, "\n")}, []string{planLine}),
		statusPrints("hash/change-inside-a-line-changes-the-hash", map[string]string{"plan.md": inLine}, []string{textHashLine("plan.md", inLine)}, planLine),
		statusPrints("hash/indentation-inside-a-fence-changes-the-hash", map[string]string{"plan.md": inFence}, []string{textHashLine("plan.md", inFence)}, planLine),
		statusPrints("hash/standing-orders-are-left-out-of-plan", map[string]string{"plan.md": plan + orders}, []string{planLine}),
		statusPrints("hash/standing-orders-are-kept-in-brief", map[string]string{"brief.md": plan + orders},
			[]string{textHashLine("brief.md", plan+orders)}, textHashLine("brief.md", plan)),
		statusPrints("hash/heading-with-trailing-blanks-starts-the-left-out-section", map[string]string{"plan.md": plan + "\n## Divergences \t\r\n\n- entry\n"}, []string{planLine}),
		statusPrints("hash/left-out-section-ends-at-the-next-heading", map[string]string{"spec.md": doc + divergences + "\n## After\n\nkept\n"},
			[]string{textHashLine("spec.md", doc+"\n## After\n\nkept\n")}),
		statusPrints("hash/deeper-heading-stays-in-the-left-out-section", map[string]string{"spec.md": doc + divergences + "\n### Child\n\nleft out\n"},
			[]string{textHashLine("spec.md", doc)}),
		statusPrints("hash/only-a-left-out-section-hashes-the-empty-text", map[string]string{"spec.md": "## Divergences\n\n- entry\n"},
			[]string{textHashLine("spec.md", "")}),
		statusPrints("hash/other-files-print-no-line", map[string]string{"notes.md": doc, "intent.md.bak": doc}, nil, "hashes:"),
		{
			name:  "hash/no-artifact-prints-no-line",
			setup: withFiles(nil),
			steps: []contractStep{{args: status, check: hashesBetweenNextAndApprovals()}},
		},
		{
			name: "hash/directory-named-like-an-artifact-prints-no-line",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := withFiles(map[string]string{"plan.md": plan})(t)
				if err := os.Mkdir(filepath.Join(fx.repo, filepath.FromSlash(fx.folder), "spec.md"), 0o755); err != nil {
					t.Fatal(err)
				}
				return fx
			},
			steps: []contractStep{{args: status, check: hashesBetweenNextAndApprovals(planLine)}},
		},
		{
			name:  "hash/lines-follow-next-in-artifact-order",
			setup: withFiles(map[string]string{"brief.md": doc, "plan.md": plan, "notes.md": doc, "spec.md": doc, "intent.md": doc}),
			steps: []contractStep{{
				args:  status,
				check: hashesBetweenNextAndApprovals(textHashLine("intent.md", doc), textHashLine("spec.md", doc), planLine, textHashLine("brief.md", doc)),
			}},
		},
		{
			name: "hash/lines-follow-next-when-the-file-has-no-rows",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := newState()(t)
				writeFile(t, fx.repo, fx.folder+"/plan.md", plan)
				return fx
			},
			steps: []contractStep{{args: status, stdoutLines: []string{"next: none"}, check: hashesBetweenNextAndApprovals(planLine)}},
		},
		{
			name: "hash/gitattributes-filter-does-not-apply",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := withFiles(map[string]string{"plan.md": plan})(t)
				git(t, fx.repo, "config", "filter.approval.clean", "sed s/body/filtered/g")
				git(t, fx.repo, "config", "filter.approval.required", "true")
				writeFile(t, fx.repo, ".gitattributes", "*.md filter=approval text eol=crlf\n")
				return fx
			},
			steps: []contractStep{{args: status, stdoutLines: []string{planLine}}},
		},
	}
	for _, artifact := range []string{"intent.md", "spec.md", "plan.md", "brief.md"} {
		cases = append(cases, statusPrints("hash/divergences-are-left-out-of-"+strings.TrimSuffix(artifact, ".md"),
			map[string]string{artifact: doc + divergences}, []string{textHashLine(artifact, doc)}))
	}
	return cases
}

// sha256Hex is the second-rule hash of a text that is already normalized.
func sha256Hex(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

func approvalGuardCases() []contractCase {
	const plan = "# Plan\n\n## Tasks\n\nbody line\n"
	const notes = "# Notes\n\n## Divergences\n\nkept in a file outside the chain\n"
	seededCells := cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	approve := func(target, status, evidence string, extra ...string) []string {
		return append(contractLog("execute", "approve", target, status, contractWhy, evidence, contractNext), extra...)
	}
	approvedRow := func(target, status, evidence string) string {
		return cellsRow("execute", "approve", target, status, contractWhy, evidence, contractNext)
	}
	planHash := sha256Hex(plan)
	other := sha256Hex("other text\n")
	missingOption := "approve needs --expect-blob <hash>: run flow status and pass the hash of the content the user was shown"
	cases := []contractCase{
		{
			name:  "guard/auto-approval-without-expect-blob-writes-no-row",
			setup: withFiles(map[string]string{"plan.md": plan}),
			steps: []contractStep{{
				args:      approve("plan.md", "ok", "auto"),
				code:      2,
				stderrHas: []string{"flow: invalid evidence: 'auto' -- ", "flow status", "--expect-blob"},
			}},
			rows: []string{seededCells},
		},
		{
			name:  "guard/matching-expect-blob-stores-the-text-hash",
			setup: withFiles(map[string]string{"plan.md": plan}),
			steps: []contractStep{{args: approve("plan.md", "ok", "auto", "--expect-blob", planHash)}},
			rows:  []string{seededCells, approvedRow("plan.md", "ok", "text="+planHash)},
		},
		{
			name:  "guard/hash-from-status-passes-the-guard",
			setup: withFiles(map[string]string{"plan.md": plan}),
			steps: []contractStep{
				{args: []string{"status", "--folder", contractFolder}, stdoutLines: []string{"hashes: plan.md text=" + planHash}},
				{args: approve("plan.md", "ok", "auto", "--expect-blob", planHash)},
			},
			rows: []string{seededCells, approvedRow("plan.md", "ok", "text="+planHash)},
		},
		{
			name:  "guard/other-hash-writes-no-row",
			setup: withFiles(map[string]string{"plan.md": plan}),
			steps: []contractStep{{
				args:      approve("plan.md", "ok", "auto", "--expect-blob", other),
				code:      2,
				stderrHas: []string{contractFolder + "/plan.md current text=" + planHash + " does not match --expect-blob"},
			}},
			rows: []string{seededCells},
		},
		{
			name: "guard/first-rule-hash-writes-no-row",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := withFiles(map[string]string{"plan.md": plan})(t)
				// An empty hash would be refused as a mismatch too, so the case would
				// pass without offering the guard a first-rule hash.
				if fx.values["blob"] = firstRuleHash(fx.repo, contractFolder, "plan.md"); fx.values["blob"] == "" {
					t.Fatal("no first-rule hash of plan.md")
				}
				return fx
			},
			steps: []contractStep{{
				argsOf: func(fx *contractFixture) []string {
					return approve("plan.md", "ok", "auto", "--expect-blob", fx.values["blob"])
				},
				code:      2,
				stderrHas: []string{contractFolder + "/plan.md current text=" + planHash + " does not match --expect-blob"},
			}},
			rows: []string{seededCells},
		},
		{
			name:  "guard/missing-artifact-writes-no-row",
			setup: withFiles(nil),
			steps: []contractStep{{
				args:      approve("plan.md", "ok", "auto", "--expect-blob", planHash),
				code:      2,
				stderrHas: []string{contractFolder + "/plan.md current text=unavailable does not match --expect-blob"},
			}},
			rows: []string{seededCells},
		},
		{
			name:  "guard/file-outside-the-chain-hashes-its-whole-text",
			setup: withFiles(map[string]string{"notes.md": notes}),
			steps: []contractStep{{args: approve("notes.md", "ok", "auto", "--expect-blob", sha256Hex(notes))}},
			rows:  []string{seededCells, approvedRow("notes.md", "ok", "text="+sha256Hex(notes))},
		},
		{
			name:  "guard/other-evidence-is-stored-unchanged",
			setup: withFiles(map[string]string{"plan.md": plan}),
			steps: []contractStep{{args: approve("plan.md", "ok", "plan.md line 1")}},
			rows:  []string{seededCells, approvedRow("plan.md", "ok", "plan.md line 1")},
		},
	}
	for _, status := range []string{"failed", "pending"} {
		cases = append(cases, contractCase{
			name:  "guard/auto-approval-" + status + "-without-expect-blob-writes-no-row",
			setup: withFiles(map[string]string{"plan.md": plan}),
			steps: []contractStep{{
				args:        approve("plan.md", status, "auto"),
				code:        2,
				stderrLines: []string{"flow: invalid evidence: 'auto' -- " + missingOption},
			}},
			rows: []string{seededCells},
		})
	}
	return cases
}

// approvalOf is a setup whose state.tsv holds one ok approve row for artifact,
// dated 2026-11-01 and written while the artifact held approved, with the hash
// of the rule key names; that hash is the value "approved". With commit set,
// the approved text is committed and its short id is the value "short". The
// artifact then holds current.
func approvalOf(artifact, key, approved, current string, commit bool) func(t *testing.T) *contractFixture {
	return func(t *testing.T) *contractFixture {
		t.Helper()
		fx := emptyRepo(t)
		writeFile(t, fx.repo, fx.folder+"/"+artifact, approved)
		hash := textHash(artifact, []byte(approved))
		if key == "blob" {
			hash = firstRuleHash(fx.repo, fx.folder, artifact)
		}
		if hash == "" {
			t.Fatal("empty approval hash")
		}
		writeState(t, fx.repo, fx.folder, row("2026-11-01T11:00:00Z", "decompose", "approve", artifact, "ok", "approved", key+"="+hash, "execute go"))
		if commit {
			commitAll(t, fx.repo, "approved")
			fx.values["short"] = gitOut(t, fx.repo, "rev-parse", "--short", "HEAD")
		}
		writeFile(t, fx.repo, fx.folder+"/"+artifact, current)
		fx.values["approved"] = hash
		return fx
	}
}

// staleLineIs checks the whole stale: line of plan.md. current is the current
// hash under key; an empty current is the first-rule hash of the file as it
// is. found expects the pointer to the commit in the value "short".
func staleLineIs(key, current string, found bool) func(t *testing.T, fx *contractFixture, res result) {
	return func(t *testing.T, fx *contractFixture, res result) {
		t.Helper()
		if current == "" {
			current = firstRuleHash(fx.repo, contractFolder, "plan.md")
		}
		pointer := stalePointerText
		if found {
			short := fx.values["short"]
			pointer = "compare with commit " + short + " (from the repository root): git diff " + short + " -- " + contractFolder + "/plan.md"
		}
		want := "stale: " + contractFolder + "/plan.md differs from the content approved 2026-11-01 (approved " + key + "=" + fx.values["approved"] +
			", current " + key + "=" + current + "). " + pointer + ". " + planNext
		checkStream(t, "stale: line", res.stdout, []string{want}, nil, nil)
	}
}

func approvalStatusCases() []contractCase {
	const plan = "# Plan\n\n## Tasks\n\nbody line\n"
	const edited = plan + "more\n"
	const reformatted = "# Plan\r\n\r\n## Tasks  \r\n\r\nbody line\t\r\n\r\n\r\n"
	const ordered = plan + "\n## Standing orders\n\n1. x\n"
	status := []string{"status", "--folder", contractFolder}
	// A fixture row has no ledger line, so a matching hash reads unconfirmed here.
	matching := "approvals: plan.md approved 2026-11-01 unconfirmed here"
	stale := "approvals: plan.md approved 2026-11-01 stale"
	noStaleLine := []lineCount{{"stale:", 0}}
	return []contractCase{
		{
			name:  "approvalstatus/text-approval-of-the-same-text-matches",
			setup: approvalOf("plan.md", "text", plan, plan, false),
			steps: []contractStep{{args: status, stdoutLines: []string{matching}, stdoutCount: noStaleLine}},
		},
		{
			name:  "approvalstatus/text-approval-ignores-line-endings-and-trailing-whitespace",
			setup: approvalOf("plan.md", "text", plan, reformatted, false),
			steps: []contractStep{{args: status, stdoutLines: []string{matching}, stdoutCount: noStaleLine}},
		},
		{
			name:  "approvalstatus/text-approval-of-changed-text-prints-text-keys",
			setup: approvalOf("plan.md", "text", plan, edited, false),
			steps: []contractStep{{
				args:        status,
				stdoutLines: []string{stale, "stale: " + contractFolder + "/plan.md differs from the content approved 2026-11-01 (approved text=" + sha256Hex(plan) + ", current text=" + sha256Hex(edited) + "). " + stalePointerText + ". " + planNext},
			}},
		},
		{
			name:  "approvalstatus/blob-approval-of-the-same-text-matches",
			setup: approvalOf("plan.md", "blob", plan, plan, false),
			steps: []contractStep{{args: status, stdoutLines: []string{matching}, stdoutCount: noStaleLine}},
		},
		{
			name:  "approvalstatus/blob-approval-of-changed-text-prints-blob-keys",
			setup: approvalOf("plan.md", "blob", plan, edited, false),
			steps: []contractStep{{args: status, stdoutLines: []string{stale}, check: staleLineIs("blob", "", false)}},
		},
		{
			name:  "approvalstatus/blob-approval-keeps-the-first-rule-for-line-endings",
			setup: approvalOf("plan.md", "blob", plan, reformatted, false),
			steps: []contractStep{{args: status, stdoutLines: []string{stale}, check: staleLineIs("blob", "", false)}},
		},
		{
			name:  "approvalstatus/first-standing-orders-section-keeps-a-text-approval",
			setup: approvalOf("plan.md", "text", plan, ordered, false),
			steps: []contractStep{{args: status, stdoutLines: []string{matching}, stdoutCount: noStaleLine}},
		},
		{
			name:  "approvalstatus/first-standing-orders-section-makes-a-blob-approval-stale",
			setup: approvalOf("plan.md", "blob", plan, ordered, false),
			steps: []contractStep{{args: status, stdoutLines: []string{stale}, check: staleLineIs("blob", "", false)}},
		},
		{
			name:  "approvalstatus/text-approval-of-committed-text-points-at-the-commit",
			setup: approvalOf("plan.md", "text", plan, edited, true),
			steps: []contractStep{{args: status, stdoutLines: []string{stale}, check: staleLineIs("text", sha256Hex(edited), true)}},
		},
		{
			name:  "approvalstatus/blob-approval-of-committed-text-points-at-the-commit",
			setup: approvalOf("plan.md", "blob", plan, edited, true),
			steps: []contractStep{{args: status, stdoutLines: []string{stale}, check: staleLineIs("blob", "", true)}},
		},
	}
}

const unconfirmedTail = ", and this clone has no record of it. Present it and ask once; a yes writes a new row."

// planReads checks that status lists one approval of plan.md and that it reads
// state. An approval that reads unconfirmed here must print its unconfirmed:
// line, naming folder as the stale: line would; any other prints none.
func planReads(state, folder string) func(t *testing.T, fx *contractFixture, res result) {
	return func(t *testing.T, fx *contractFixture, res result) {
		t.Helper()
		var date string
		n := 0
		for _, line := range strings.Split(strings.TrimRight(res.stdout, "\n"), "\n") {
			if rest, ok := strings.CutPrefix(line, "approvals: plan.md approved "); ok {
				n++
				var got string
				date, got, _ = strings.Cut(rest, " ")
				if got != state {
					t.Errorf("plan.md approval reads %q, want %q", got, state)
				}
			}
		}
		if n != 1 {
			t.Fatalf("%d approvals: lines for plan.md, want 1: %q", n, res.stdout)
		}
		if state != "unconfirmed here" {
			checkStream(t, "status", res.stdout, nil, nil, []lineCount{{"unconfirmed:", 0}})
			return
		}
		want := "unconfirmed: " + folder + "/plan.md was approved " + date + unconfirmedTail
		checkStream(t, "status", res.stdout, []string{want}, nil, []lineCount{{"unconfirmed:", 1}, {"stale:", 0}})
	}
}

// ledgerPathOf is the ledger of repo, a repository whose Git directory is .git.
func ledgerPathOf(repo string) string {
	return filepath.Join(repo, ".git", "agentics", "flow", "ledger.tsv")
}

// ledgerAre checks the lines of the fixture repository's ledger without their
// ts field, and that each ts has the row timestamp format. An empty want
// expects no ledger file at all.
func ledgerAre(want func(fx *contractFixture) []string) func(t *testing.T, fx *contractFixture) {
	return func(t *testing.T, fx *contractFixture) {
		t.Helper()
		body, err := os.ReadFile(ledgerPathOf(fx.repo))
		lines := want(fx)
		if len(lines) == 0 {
			if !os.IsNotExist(err) {
				t.Errorf("want no ledger, read %q (err %v)", body, err)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, line := range strings.Split(strings.TrimRight(string(body), "\n"), "\n") {
			ts, rest, _ := strings.Cut(line, "\t")
			if _, err := time.Parse("2006-01-02T15:04:05Z", ts); err != nil || len(ts) != 20 {
				t.Errorf("ledger line %q starts with %q, not a row timestamp", line, ts)
			}
			got = append(got, rest)
		}
		if strings.Join(got, "\n") != strings.Join(lines, "\n") {
			t.Errorf("ledger without ts:\n got %q\nwant %q", got, lines)
		}
	}
}

func noLedger(*contractFixture) []string { return nil }

// quiet fails when a call printed anything.
func quiet(t *testing.T, _ *contractFixture, res result) {
	t.Helper()
	if res.stdout != "" || res.stderr != "" {
		t.Errorf("want no output, got stdout %q stderr %q", res.stdout, res.stderr)
	}
}

func ledgerCases() []contractCase {
	const plan = "# Plan\n\n## Tasks\n\nbody line\n"
	planHash := sha256Hex(plan)
	status := []string{"status", "--folder", contractFolder}
	approveCells := []string{"execute", "approve", "plan.md", "ok", contractWhy, "auto", contractNext, "--expect-blob", planHash}
	approve := contractLog(approveCells...)
	approvedRow := cellsRow("execute", "approve", "plan.md", "ok", contractWhy, "text="+planHash, contractNext)
	seededCells := cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	withPlan := withFiles(map[string]string{"plan.md": plan})
	planLine := func(folder string) func(fx *contractFixture) []string {
		return func(fx *contractFixture) []string { return []string{"approve\t" + folder + "\tplan.md\t" + planHash} }
	}
	clone := func(fx *contractFixture) string { return fx.values["clone"] }
	approvedAt := func(ts string) string {
		return row(ts, "decompose", "approve", "plan.md", "ok", "approved", "text="+planHash, "execute go")
	}
	// revoked is plan.md approved on 2026-11-01 with a ledger line at ledgerTS,
	// then target revoked on 2026-11-02, then the later rows.
	revoked := func(ledgerTS, target string, later ...string) func(t *testing.T) *contractFixture {
		return func(t *testing.T) *contractFixture {
			t.Helper()
			fx := emptyRepo(t)
			writeFile(t, fx.repo, fx.folder+"/plan.md", plan)
			writeState(t, fx.repo, fx.folder, append([]string{
				approvedAt("2026-11-01T11:00:00Z"),
				row("2026-11-02T11:00:00Z", "decompose", "revoke", target, "ok", "withdrawn", "none", "decompose go"),
			}, later...)...)
			writeFile(t, fx.repo, ".git/agentics/flow/ledger.tsv", ledgerTS+"\tapprove\t"+contractFolder+"\tplan.md\t"+planHash+"\n")
			return fx
		}
	}
	// commitHead is the plan fixture with HEAD a commit of the flow folder and a work file.
	commitHead := withHead(func(t *testing.T) *contractFixture {
		t.Helper()
		fx := withPlan(t)
		writeFile(t, fx.repo, "src/a.txt", "one\n")
		commitAll(t, fx.repo, "task")
		return fx
	})
	commitUser := func(evidence string) []string {
		return contractLog("execute", "commit", "user", "ok", "user commit", evidence, contractNext)
	}
	commitLine := func(fx *contractFixture) []string {
		return []string{"commit\t" + contractFolder + "\tuser\t" + fx.values["head"]}
	}
	const finding = "finding audit commit:0123456789abcdef0123456789abcdef01234567"
	waiveCells := func(target, status string) []string {
		return []string{"execute", "waive", target, status, contractWhy, "none", contractNext}
	}
	waiveRow := func(target, status string) string { return strings.Join(waiveCells(target, status), "\t") }
	waiveLine := func(*contractFixture) []string { return []string{"waive\t" + contractFolder + "\t" + finding + "\t-"} }
	return []contractCase{
		{
			name:  "ledger/commit-row-with-an-abbreviated-sha-appends-a-commit-line-with-the-full-sha",
			setup: commitHead,
			steps: []contractStep{{argsOf: func(fx *contractFixture) []string { return commitUser("auto:" + fx.values["head"][:7]) }, check: quiet}},
			check: ledgerAre(commitLine),
		},
		{
			name:  "ledger/commit-row-with-auto-head-appends-a-commit-line-with-the-full-sha",
			setup: commitHead,
			steps: []contractStep{{args: commitUser("auto:HEAD"), check: quiet}},
			check: ledgerAre(commitLine),
		},
		{
			name:  "ledger/refused-commit-row-writes-no-ledger-line",
			setup: commitHead,
			steps: []contractStep{{args: commitUser("auto:0000000000000000000000000000000000000001"), code: 2,
				stderrHas: []string{"no such commit to account for"}}},
			rows:  []string{seededCells},
			check: ledgerAre(noLedger),
		},
		{
			name:  "ledger/ok-finding-waiver-appends-a-waive-line",
			setup: withPlan,
			steps: []contractStep{{args: contractLog(waiveCells(finding, "ok")...), check: quiet}},
			rows:  []string{seededCells, waiveRow(finding, "ok")},
			check: ledgerAre(waiveLine),
		},
		{
			name:  "ledger/waiver-of-another-head-writes-no-ledger-line",
			setup: withPlan,
			steps: []contractStep{{args: contractLog(waiveCells("review", "ok")...)}},
			rows:  []string{seededCells, waiveRow("review", "ok")},
			check: ledgerAre(noLedger),
		},
		{
			name:  "ledger/declined-finding-waiver-writes-no-ledger-line",
			setup: withPlan,
			steps: []contractStep{{args: contractLog(waiveCells(finding, "declined")...)}},
			rows:  []string{seededCells, waiveRow(finding, "declined")},
			check: ledgerAre(noLedger),
		},
		{
			name:  "ledger/failed-finding-waiver-writes-no-ledger-line",
			setup: withPlan,
			steps: []contractStep{{args: contractLog(waiveCells(finding, "failed")...)}},
			rows:  []string{seededCells, waiveRow(finding, "failed")},
			check: ledgerAre(noLedger),
		},
		{
			name:  "ledger/finding-waiver-through-an-absolute-folder-is-keyed-by-the-relative-one",
			setup: withPlan,
			steps: []contractStep{{dir: "sub", argsOf: func(fx *contractFixture) []string {
				return append(append([]string{"log"}, waiveCells(finding, "ok")...), "--folder", filepath.Join(fx.repo, filepath.FromSlash(contractFolder)))
			}}},
			rows:  []string{seededCells, waiveRow(finding, "ok")},
			check: ledgerAre(waiveLine),
		},
		{
			name:   "ledger/outside-a-repository-the-finding-waiver-is-written-and-no-ledger-line",
			serial: true,
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				repo := t.TempDir()
				// The flow's Git calls inherit the process environment, not a step's, and the
				// temporary directory may itself lie inside a repository.
				t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(repo))
				return &contractFixture{repo: repo, folder: contractFolder, values: map[string]string{}}
			},
			steps: []contractStep{{argsOf: func(fx *contractFixture) []string {
				return append(append([]string{"log"}, waiveCells(finding, "ok")...), "--folder", filepath.Join(fx.repo, filepath.FromSlash(contractFolder)))
			}, check: quiet}},
			rows:  []string{waiveRow(finding, "ok")},
			check: ledgerAre(noLedger),
		},
		{
			name:  "ledger/approval-reads-current-in-the-clone-that-wrote-it",
			setup: withPlan,
			steps: []contractStep{
				{args: approve, check: quiet},
				{args: status, check: planReads("current", contractFolder)},
			},
			rows: []string{seededCells, approvedRow},
			check: func(t *testing.T, fx *contractFixture) {
				t.Helper()
				ledgerAre(planLine(contractFolder))(t, fx)
				if listed := gitOut(t, fx.repo, "status", "--porcelain", "--untracked-files=all", "--ignored"); strings.Contains(listed, "ledger") || strings.Contains(listed, "agentics/") {
					t.Errorf("git status lists the ledger: %q", listed)
				}
			},
		},
		{
			name: "ledger/approval-copied-into-another-clone-reads-unconfirmed-here",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := withPlan(t)
				fx.values["clone"] = filepath.Join(t.TempDir(), "clone")
				git(t, fx.repo, "clone", "-q", fx.repo, fx.values["clone"])
				return fx
			},
			steps: []contractStep{
				{args: approve},
				{args: status, check: func(t *testing.T, fx *contractFixture, res result) {
					t.Helper()
					planReads("current", contractFolder)(t, fx, res)
					for _, name := range []string{"state.tsv", "plan.md"} {
						body, err := os.ReadFile(filepath.Join(fx.repo, filepath.FromSlash(contractFolder), name))
						if err != nil {
							t.Fatal(err)
						}
						writeFile(t, clone(fx), contractFolder+"/"+name, string(body))
					}
				}},
				{dirOf: clone, args: status, check: planReads("unconfirmed here", contractFolder)},
			},
			check: func(t *testing.T, fx *contractFixture) {
				t.Helper()
				if _, err := os.Lstat(ledgerPathOf(clone(fx))); !os.IsNotExist(err) {
					t.Errorf("a ledger exists in the other clone (err %v)", err)
				}
			},
		},
		{
			name:  "ledger/fixture-blob-row-with-a-matching-hash-reads-unconfirmed-here",
			setup: approvalOf("plan.md", "blob", plan, plan, false),
			steps: []contractStep{{args: status, check: planReads("unconfirmed here", contractFolder)}},
		},
		{
			name:  "ledger/fixture-blob-row-of-a-changed-file-reads-stale",
			setup: approvalOf("plan.md", "blob", plan, plan+"more\n", false),
			steps: []contractStep{{args: status, check: planReads("stale", contractFolder), stdoutCount: []lineCount{{"stale:", 1}}}},
		},
		{
			name:  "ledger/fixture-text-row-with-a-matching-hash-reads-unconfirmed-here",
			setup: approvalOf("plan.md", "text", plan, plan, false),
			steps: []contractStep{{args: status, check: planReads("unconfirmed here", contractFolder)}},
		},
		{
			name: "ledger/unconfirmed-lines-follow-the-stale-lines",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := emptyRepo(t)
				writeFile(t, fx.repo, fx.folder+"/plan.md", plan)
				writeFile(t, fx.repo, fx.folder+"/spec.md", "changed\n")
				writeState(t, fx.repo, fx.folder,
					row("2026-11-01T11:00:00Z", "decompose", "approve", "plan.md", "ok", "approved", "text="+planHash, "execute go"),
					row("2026-11-02T11:00:00Z", "spec", "approve", "spec.md", "ok", "approved", "text="+sha256Hex("approved\n"), "decompose go"),
					row("2026-11-03T11:00:00Z", "intent", "approve", "intent.md", "ok", "approved", "text="+sha256Hex("gone\n"), "spec go"),
				)
				return fx
			},
			steps: []contractStep{{args: status, check: func(t *testing.T, fx *contractFixture, res result) {
				t.Helper()
				var kinds []string
				for _, line := range strings.Split(strings.TrimRight(res.stdout, "\n"), "\n") {
					if prefix, _, ok := strings.Cut(line, ": "); ok && slices.Contains([]string{"approvals", "stale", "unconfirmed", "blocks"}, prefix) {
						kinds = append(kinds, prefix)
					}
				}
				if want := []string{"approvals", "approvals", "approvals", "stale", "stale", "unconfirmed", "blocks"}; !slices.Equal(kinds, want) {
					t.Errorf("line order %q, want %q: %q", kinds, want, res.stdout)
				}
				checkStream(t, "status", res.stdout, []string{
					"approvals: plan.md approved 2026-11-01 unconfirmed here",
					"unconfirmed: " + contractFolder + "/plan.md was approved 2026-11-01" + unconfirmedTail,
				}, nil, nil)
			}}},
		},
		{
			name:  "ledger/literal-text-evidence-writes-no-ledger-line",
			setup: withPlan,
			steps: []contractStep{
				{args: contractLog("execute", "approve", "plan.md", "ok", contractWhy, "text="+planHash, contractNext)},
				{args: status, check: planReads("unconfirmed here", contractFolder)},
			},
			rows:  []string{seededCells, approvedRow},
			check: ledgerAre(noLedger),
		},
		{
			name:  "ledger/guarded-approval-that-is-not-ok-writes-no-ledger-line",
			setup: withPlan,
			steps: []contractStep{
				{args: contractLog("execute", "approve", "plan.md", "pending", contractWhy, "auto", contractNext, "--expect-blob", planHash)},
			},
			rows:  []string{seededCells, cellsRow("execute", "approve", "plan.md", "pending", contractWhy, "text="+planHash, contractNext)},
			check: ledgerAre(noLedger),
		},
		{
			name:  "ledger/approval-copied-in-after-a-revoke-reads-unconfirmed-here",
			setup: revoked("2026-11-01T11:00:00Z", "plan.md", approvedAt("2026-11-03T11:00:00Z")),
			steps: []contractStep{{args: status, check: planReads("unconfirmed here", contractFolder)}},
		},
		{
			name:  "ledger/ledger-line-in-the-second-of-the-revoke-does-not-count",
			setup: revoked("2026-11-02T11:00:00Z", "plan.md", approvedAt("2026-11-03T11:00:00Z")),
			steps: []contractStep{{args: status, check: planReads("unconfirmed here", contractFolder)}},
		},
		{
			name:  "ledger/ledger-line-after-the-revoke-reads-current",
			setup: revoked("2026-11-02T11:00:01Z", "plan.md", approvedAt("2026-11-03T11:00:00Z")),
			steps: []contractStep{{args: status, check: planReads("current", contractFolder)}},
		},
		{
			name:  "ledger/guarded-approval-after-a-revoke-reads-current",
			setup: revoked("2026-11-01T11:00:00Z", "plan.md"),
			steps: []contractStep{
				{args: status, stdoutLines: []string{"approvals: none"}},
				{args: approve},
				{args: status, check: planReads("current", contractFolder)},
			},
		},
		{
			name:  "ledger/revoke-of-another-target-changes-nothing",
			setup: revoked("2026-11-01T11:00:00Z", "spec.md"),
			steps: []contractStep{{args: status, check: planReads("current", contractFolder)}},
		},
		{
			name:  "ledger/approval-through-an-absolute-folder-reads-current-through-the-relative-one",
			setup: withPlan,
			steps: []contractStep{
				{dir: "sub", argsOf: func(fx *contractFixture) []string {
					return append(append([]string{"log"}, approveCells...), "--folder", filepath.Join(fx.repo, filepath.FromSlash(contractFolder)))
				}},
				{args: status, check: planReads("current", contractFolder)},
			},
			rows:  []string{seededCells, approvedRow},
			check: ledgerAre(planLine(contractFolder)),
		},
		{
			name:  "ledger/approval-into-a-looked-up-folder-reads-current-through-the-relative-one",
			setup: onBranch("topic", withPlan),
			steps: []contractStep{
				{dir: "sub", args: append([]string{"log"}, approveCells...)},
				{args: status, check: planReads("current", contractFolder)},
			},
			rows:  []string{seededCells, approvedRow},
			check: ledgerAre(planLine(contractFolder)),
		},
		{
			name: "ledger/approval-through-a-symlinked-path-is-keyed-root-relative",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := withPlan(t)
				link := filepath.Join(t.TempDir(), "link")
				if err := os.Symlink(fx.repo, link); err != nil {
					t.Skipf("this platform does not let the test create a symlink: %v", err)
				}
				fx.values["linked"] = filepath.Join(link, filepath.FromSlash(contractFolder))
				return fx
			},
			steps: []contractStep{
				{argsOf: func(fx *contractFixture) []string {
					return append(append([]string{"log"}, approveCells...), "--folder", fx.values["linked"])
				}},
				{argsOf: func(fx *contractFixture) []string { return []string{"status", "--folder", fx.values["linked"]} },
					stdoutLines: []string{"flow: " + contractFolder}, check: planReads("current", contractFolder)},
				{args: status, check: planReads("current", contractFolder)},
			},
			rows:  []string{seededCells, approvedRow},
			check: ledgerAre(planLine(contractFolder)),
		},
		{
			name: "ledger/approval-of-a-folder-outside-the-repository-is-keyed-by-its-resolved-path",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := emptyRepo(t)
				outside := filepath.Join(t.TempDir(), "flow")
				writeFile(t, outside, "plan.md", plan)
				resolved, err := filepath.EvalSymlinks(outside)
				if err != nil {
					t.Fatal(err)
				}
				fx.values["outside"], fx.values["key"] = outside, filepath.ToSlash(filepath.Clean(resolved))
				return fx
			},
			steps: []contractStep{
				{argsOf: func(fx *contractFixture) []string {
					return append(append([]string{"log"}, approveCells...), "--folder", fx.values["outside"])
				}},
				{argsOf: func(fx *contractFixture) []string { return []string{"status", "--folder", fx.values["outside"]} },
					check: planReads("current", "")},
			},
			check: ledgerAre(func(fx *contractFixture) []string { return planLine(fx.values["key"])(fx) }),
		},
		{
			name: "ledger/approval-in-the-main-worktree-reads-current-in-a-linked-worktree",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := withPlan(t)
				fx.values["linked"] = filepath.Join(t.TempDir(), "linked")
				return fx
			},
			steps: []contractStep{
				{args: approve, check: func(t *testing.T, fx *contractFixture, res result) {
					t.Helper()
					commitAll(t, fx.repo, "approved")
					git(t, fx.repo, "worktree", "add", "-q", "-b", "linked", fx.values["linked"])
				}},
				{dirOf: func(fx *contractFixture) string { return fx.values["linked"] }, args: status, check: planReads("current", contractFolder)},
			},
		},
		{
			name:   "ledger/outside-a-repository-the-approval-is-written-and-reads-unconfirmed-here",
			serial: true,
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				repo := t.TempDir()
				// The flow's Git calls inherit the process environment, not a step's, and the
				// temporary directory may itself lie inside a repository.
				t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(repo))
				writeFile(t, repo, contractFolder+"/plan.md", plan)
				return &contractFixture{repo: repo, folder: contractFolder, values: map[string]string{}}
			},
			steps: []contractStep{
				{args: approve, check: quiet},
				{args: status, check: planReads("unconfirmed here", contractFolder)},
			},
			rows: []string{approvedRow},
		},
		{
			name:   "ledger/without-git-the-approval-is-written-and-no-ledger-line",
			serial: true,
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := withPlan(t)
				fx.values["path"] = os.Getenv("PATH")
				// A step's environment reaches only the flow's getenv; the Git it runs
				// is looked up on the process PATH.
				t.Setenv("PATH", t.TempDir())
				return fx
			},
			steps: []contractStep{
				{args: approve, check: func(t *testing.T, fx *contractFixture, res result) {
					t.Helper()
					quiet(t, fx, res)
					// Git is back for the read, so only a missing line can make it unconfirmed.
					if err := os.Setenv("PATH", fx.values["path"]); err != nil {
						t.Fatal(err)
					}
				}},
				{args: status, check: planReads("unconfirmed here", contractFolder)},
			},
			rows:  []string{seededCells, approvedRow},
			check: ledgerAre(noLedger),
		},
	}
}

func entryCases() []contractCase {
	const plan = "# Plan\n\n## Tasks\n\nbody line\n\n## Standing orders\n\n" +
		"1. Change only these paths.\n   Second line of the first order.\n" +
		"2. Old order. Reconciled: 2026-11-10.\n" +
		"3. Third order.\n\n" +
		"## Divergences\n\n" +
		"- **2026-11-01, task-2** — first divergence.\n" +
		"- **2026-11-02, run** — second divergence.\n  Reconciled:\n  2026-11-12.\n"
	const brief = "# Brief\n\n## Standing orders\n\n1. Brief order.\n\n## Divergences\n\n- brief divergence one.\n- brief divergence two.\n"
	const spec = "# Spec\n\n## Divergences\n\n- spec divergence.\n"
	order1 := sha256Hex("Change only these paths.\n   Second line of the first order.\n")
	order2 := sha256Hex("Old order.\n")
	order3 := sha256Hex("Third order.\n")
	planDiv1 := sha256Hex("**2026-11-01, task-2** — first divergence.\n")
	planDiv2 := sha256Hex("**2026-11-02, run** — second divergence.\n")
	briefDiv2 := sha256Hex("brief divergence two.\n")
	specDiv1 := sha256Hex("spec divergence.\n")
	seededCells := cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	withPlan := withFiles(map[string]string{"plan.md": plan})
	const entryPairReason = "entry= is written by --entry only; pass --entry <n|last>"
	order := func(target string, extra ...string) []string {
		return append(contractLog("execute", "order", target, "ok", contractWhy, "auto", contractNext), extra...)
	}
	orderRow := func(target, hash string) string {
		return cellsRow("execute", "order", target, "ok", contractWhy, "entry="+hash, contractNext)
	}
	diverge := func(target, evidence, entry string) []string {
		return append(contractLog("execute", "diverge", target, "ok", contractWhy, evidence, contractNext), "--entry", entry)
	}
	divergeRow := func(target, evidence string) string {
		return cellsRow("execute", "diverge", target, "ok", contractWhy, evidence, contractNext)
	}
	ledger := func(lines ...string) func(t *testing.T, fx *contractFixture) {
		return ledgerAre(func(*contractFixture) []string { return lines })
	}
	line := func(kind, target, hash string) string {
		return kind + "\t" + contractFolder + "\t" + target + "\t" + hash
	}
	// refused is a case whose one call exits code, with the whole stderr line
	// or, for a usage error, the usage text, and writes neither a row nor a
	// ledger line.
	refused := func(name string, files map[string]string, args []string, code int, message string) contractCase {
		step := contractStep{args: args, code: code, stderrLines: []string{message}}
		if code == 1 {
			step.stderrLines, step.stderrHas = nil, []string{"Usage: flow log "}
		}
		return contractCase{name: "entries/" + name, setup: withFiles(files), steps: []contractStep{step}, rows: []string{seededCells}, check: ledger()}
	}
	return []contractCase{
		{
			name:  "entries/order-stores-the-entry-hash-and-an-order-ledger-line",
			setup: withPlan,
			steps: []contractStep{{args: order("3"), check: quiet}},
			rows:  []string{seededCells, orderRow("3", order3)},
			check: ledger(line("order", "3", order3)),
		},
		{
			name:  "entries/number-counts-every-top-level-item-and-keeps-continuation-lines",
			setup: withPlan,
			steps: []contractStep{{args: order("1")}, {args: order("2")}},
			rows:  []string{seededCells, orderRow("1", order1), orderRow("2", order2)},
			check: ledger(line("order", "1", order1), line("order", "2", order2)),
		},
		{
			name:  "entries/hash-is-the-same-without-the-reconciled-mark",
			setup: withFiles(map[string]string{"plan.md": strings.Replace(plan, " Reconciled: 2026-11-10.", "", 1)}),
			steps: []contractStep{{args: order("2")}},
			rows:  []string{seededCells, orderRow("2", order2)},
		},
		{
			name:  "entries/crlf-copy-hashes-the-same-entry",
			setup: withFiles(map[string]string{"plan.md": strings.ReplaceAll(plan, "\n", " \r\n")}),
			steps: []contractStep{{args: order("1")}, {args: diverge("run", "none", "2")}},
			rows:  []string{seededCells, orderRow("1", order1), divergeRow("run", "none entry="+planDiv2)},
		},
		{
			name: "entries/order-is-accepted-on-a-version-1-file",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := stateAt(legacyVersionLine, row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"))(t)
				writeFile(t, fx.repo, fx.folder+"/plan.md", plan)
				return fx
			},
			steps: []contractStep{{args: order("3")}},
			rows:  []string{seededCells, orderRow("3", order3)},
			check: func(t *testing.T, fx *contractFixture) {
				t.Helper()
				firstLineIs(legacyVersionLine)(t, fx)
				ledger(line("order", "3", order3))(t, fx)
			},
		},
		refused("order-without-such-entry-writes-nothing", map[string]string{"plan.md": plan}, order("4"), 2,
			"flow: invalid target: '4' -- "+contractFolder+"/plan.md has no entry 4 under ## Standing orders"),
		refused("order-without-a-plan-writes-nothing", map[string]string{"brief.md": brief}, order("1"), 2,
			"flow: invalid target: '1' -- an order row reads plan.md, and "+contractFolder+" has none; a brief's standing orders are inside its approved text"),
		refused("order-target-must-be-a-positive-integer", map[string]string{"plan.md": plan}, order("0"), 2,
			"flow: invalid target: '0' -- an order row names the standing order's position, a positive integer"),
		refused("order-target-must-be-a-number", map[string]string{"plan.md": plan}, order("task-1"), 2,
			"flow: invalid target: 'task-1' -- an order row names the standing order's position, a positive integer"),
		refused("order-status-must-be-ok", map[string]string{"plan.md": plan},
			contractLog("execute", "order", "1", "failed", contractWhy, "auto", contractNext), 2,
			"flow: invalid status: 'failed' -- an order row is ok"),
		refused("order-evidence-must-be-auto", map[string]string{"plan.md": plan},
			contractLog("execute", "order", "1", "ok", contractWhy, "none", contractNext), 2,
			"flow: invalid evidence: 'none' -- order evidence is auto, stored as the entry hash"),
		refused("order-takes-no-entry-option", map[string]string{"plan.md": plan}, order("1", "--entry", "1"), 1, ""),
		refused("order-takes-no-expect-blob", map[string]string{"plan.md": plan}, order("1", "--expect-blob", order1), 1, ""),
		{
			name:  "entries/diverge-of-a-task-reads-the-plan",
			setup: withFiles(map[string]string{"plan.md": plan, "brief.md": brief}),
			steps: []contractStep{{args: diverge("task-2", "plan.md#divergences", "1"), check: quiet}},
			rows:  []string{seededCells, divergeRow("task-2", "plan.md#divergences entry="+planDiv1)},
			check: ledger(),
		},
		{
			name:  "entries/diverge-entry-last-reads-the-last-entry-reconciled-or-not",
			setup: withPlan,
			steps: []contractStep{{args: diverge("run", "none", "last")}},
			rows:  []string{seededCells, divergeRow("run", "none entry="+planDiv2)},
			check: ledger(),
		},
		{
			name:  "entries/diverge-of-run-without-a-plan-reads-the-brief",
			setup: withFiles(map[string]string{"brief.md": brief}),
			steps: []contractStep{{args: diverge("run", "none", "2")}},
			rows:  []string{seededCells, divergeRow("run", "none entry="+briefDiv2)},
		},
		{
			name:  "entries/diverge-of-an-artifact-reads-that-artifact",
			setup: withFiles(map[string]string{"plan.md": plan, "spec.md": spec}),
			steps: []contractStep{{args: diverge("spec.md Constraints", "none", "1")}},
			rows:  []string{seededCells, divergeRow("spec.md Constraints", "none entry="+specDiv1)},
		},
		{
			name:  "entries/diverge-auto-evidence-ends-with-the-entry-pair",
			setup: withHead(withPlan),
			steps: []contractStep{{args: diverge("task-2", "auto", "1")}},
			check: rowsAre(func(fx *contractFixture) []string {
				return []string{seededCells, divergeRow("task-2", "commit="+fx.values["head"]+" patch-id="+fx.values["head-patch"]+" entry="+planDiv1)}
			}),
		},
		{
			name:  "entries/rule-rows-with-entry-append-rule-ledger-lines",
			setup: withPlan,
			steps: []contractStep{{args: diverge("task-2 rule", "none", "1"), check: quiet}, {args: diverge("run rule", "none", "last")}},
			rows:  []string{seededCells, divergeRow("task-2 rule", "none entry="+planDiv1), divergeRow("run rule", "none entry="+planDiv2)},
			check: ledger(line("rule", "task-2 rule", planDiv1), line("rule", "run rule", planDiv2)),
		},
		{
			name:  "entries/rule-row-without-entry-writes-no-ledger-line",
			setup: withPlan,
			steps: []contractStep{{args: contractLog("execute", "diverge", "task-2 rule", "ok", contractWhy, "none", contractNext)}},
			rows:  []string{seededCells, divergeRow("task-2 rule", "none")},
			check: ledger(),
		},
		refused("entry-without-such-entry-writes-nothing", map[string]string{"plan.md": plan}, diverge("task-2 rule", "none", "3"), 2,
			"flow: invalid entry: '3' -- "+contractFolder+"/plan.md has no entry 3 under ## Divergences"),
		refused("entry-last-on-an-empty-section-writes-nothing", map[string]string{"plan.md": "# Plan\n\n## Divergences\n\nNone yet.\n"}, diverge("run rule", "none", "last"), 2,
			"flow: invalid entry: 'last' -- "+contractFolder+"/plan.md has no entry under ## Divergences"),
		refused("entry-last-on-an-absent-section-writes-nothing", map[string]string{"plan.md": "# Plan\n\n## Tasks\n\n- a task\n"}, diverge("run rule", "none", "last"), 2,
			"flow: invalid entry: 'last' -- "+contractFolder+"/plan.md has no entry under ## Divergences"),
		refused("entry-of-a-missing-artifact-writes-nothing", map[string]string{"plan.md": plan}, diverge("spec.md Constraints", "none", "1"), 2,
			"flow: invalid entry: '1' -- "+contractFolder+"/spec.md is not a readable file"),
		refused("entry-without-plan-or-brief-writes-nothing", nil, diverge("run rule", "none", "1"), 2,
			"flow: invalid entry: '1' -- "+contractFolder+"/brief.md is not a readable file"),
		refused("entry-zero-writes-nothing", map[string]string{"plan.md": plan}, diverge("run rule", "none", "0"), 2,
			"flow: invalid entry: '0' -- --entry takes a positive integer or last"),
		refused("entry-word-writes-nothing", map[string]string{"plan.md": plan}, diverge("run rule", "none", "first"), 2,
			"flow: invalid entry: 'first' -- --entry takes a positive integer or last"),
		refused("diverge-with-a-literal-entry-pair-writes-nothing", map[string]string{"plan.md": plan},
			contractLog("execute", "diverge", "task-2 rule", "ok", contractWhy, "entry="+planDiv1, contractNext), 2,
			"flow: invalid evidence: 'entry="+planDiv1+"' -- "+entryPairReason),
		refused("diverge-with-a-literal-entry-pair-and-entry-writes-nothing", map[string]string{"plan.md": plan},
			diverge("task-2 rule", "none entry="+planDiv1, "1"), 2,
			"flow: invalid evidence: 'none entry="+planDiv1+"' -- "+entryPairReason),
		refused("verify-with-a-literal-entry-pair-writes-nothing", map[string]string{"plan.md": plan},
			contractLog("execute", "verify", "task-2", "ok", contractWhy, "none entry="+planDiv1, contractNext), 2,
			"flow: invalid evidence: 'none entry="+planDiv1+"' -- "+entryPairReason),
		{
			name:  "entries/entry-on-another-event-is-a-usage-error",
			setup: withPlan,
			steps: []contractStep{{
				args:      append(contractLog("execute", "verify", "task-2", "ok", contractWhy, "none", contractNext), "--entry", "1"),
				code:      1,
				stderrHas: []string{"Usage: flow log ", "[--entry <n|last>]", "\norder ", " order "},
			}},
			rows:  []string{seededCells},
			check: ledger(),
		},
	}
}

// entryLines returns the orders:, divergences: and unrecorded: lines of a status output, in order.
func entryLines(stdout string) []string {
	var got []string
	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		for _, prefix := range []string{"orders: ", "divergences: ", "unrecorded: "} {
			if strings.HasPrefix(line, prefix) {
				got = append(got, line)
			}
		}
	}
	return got
}

// entryLinesAre checks that the orders:, divergences: and unrecorded: lines are exactly want, in order.
func entryLinesAre(want ...string) func(t *testing.T, fx *contractFixture, res result) {
	return func(t *testing.T, _ *contractFixture, res result) {
		t.Helper()
		if got := entryLines(res.stdout); !slices.Equal(got, want) {
			t.Errorf("entry lines:\n got %q\nwant %q\nin %q", got, want, res.stdout)
		}
	}
}

func entryStatusCases() []contractCase {
	const plan = "# Plan\n\n## Tasks\n\nbody line\n\n## Standing orders\n\n" +
		"1. Change only these paths.\n" +
		"2. Old order. Reconciled: 2026-11-10.\n" +
		"3. Divergence (2026-11-05): third order.\n\n" +
		"## Divergences\n\n" +
		"- **2026-11-01, task-2** — first divergence.\n" +
		"- **2026-11-02, run** — second divergence. Reconciled: 2026-11-12.\n"
	order3 := sha256Hex("Divergence (2026-11-05): third order.\n")
	planDiv1 := sha256Hex("**2026-11-01, task-2** — first divergence.\n")
	planDiv2 := sha256Hex("**2026-11-02, run** — second divergence.\n")
	status := []string{"status", "--folder", contractFolder}
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	// entryFixture holds files in the flow folder and, after the seeded row, rows
	// written without the helper, so the ledger holds none of them.
	entryFixture := func(files map[string]string, rows ...string) func(t *testing.T) *contractFixture {
		return func(t *testing.T) *contractFixture {
			t.Helper()
			fx := newState(append([]string{seeded}, rows...)...)(t)
			for name, body := range files {
				writeFile(t, fx.repo, fx.folder+"/"+name, body)
			}
			return fx
		}
	}
	order := func(target string) []string {
		return contractLog("execute", "order", target, "ok", contractWhy, "auto", contractNext)
	}
	diverge := func(target, entry string) []string {
		return append(contractLog("execute", "diverge", target, "ok", contractWhy, "none", contractNext), "--entry", entry)
	}
	fixtureRow := func(event, target, evidence string) string {
		return row("2026-11-02T10:00:00Z", "execute", event, target, "ok", contractWhy, evidence, contractNext)
	}
	threeOrders := []string{"unrecorded: plan.md standing order 1", "unrecorded: plan.md standing order 2", "unrecorded: plan.md standing order 3"}
	lines := func(head []string, rest ...string) []string {
		return append(append(slices.Clone(head), threeOrders...), rest...)
	}
	clone := func(fx *contractFixture) string { return fx.values["clone"] }
	// fourOrders is a plan whose only section is the standing orders, one per item.
	fourOrders := func(items ...string) string {
		return "# Plan\n\n## Standing orders\n\n" + strings.Join(items, "\n") + "\n"
	}
	rewritePlan := func(body string) func(t *testing.T, fx *contractFixture, res result) {
		return func(t *testing.T, fx *contractFixture, _ result) {
			t.Helper()
			writeFile(t, fx.repo, fx.folder+"/plan.md", body)
		}
	}
	return []contractCase{
		{
			name: "entrystatus/entry-lines-follow-stale-and-unconfirmed-and-precede-blocks",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := entryFixture(map[string]string{"plan.md": plan, "spec.md": "changed\n"})(t)
				// The line order is under test, not the hash, so the plan's own kept-text hash makes its approval match.
				planText, ok := fileTextHash(filepath.Join(fx.repo, filepath.FromSlash(fx.folder)), "plan.md")
				if !ok {
					t.Fatal("no text hash for plan.md")
				}
				writeState(t, fx.repo, fx.folder, seeded,
					row("2026-11-01T11:00:00Z", "decompose", "approve", "plan.md", "ok", "approved", "text="+planText, "execute go"),
					row("2026-11-01T12:00:00Z", "spec", "approve", "spec.md", "ok", "approved", "text="+sha256Hex("approved\n"), "decompose go"),
				)
				return fx
			},
			steps: []contractStep{{args: status, check: func(t *testing.T, _ *contractFixture, res result) {
				t.Helper()
				var kinds []string
				for _, line := range strings.Split(strings.TrimRight(res.stdout, "\n"), "\n") {
					if prefix, _, ok := strings.Cut(line, ": "); ok && slices.Contains([]string{"stale", "unconfirmed", "orders", "divergences", "unrecorded", "blocks", "sessions"}, prefix) {
						kinds = append(kinds, prefix)
					}
				}
				want := []string{"stale", "unconfirmed", "orders", "divergences", "unrecorded", "unrecorded", "unrecorded", "unrecorded", "blocks", "sessions"}
				if !slices.Equal(kinds, want) {
					t.Errorf("line order %q, want %q: %q", kinds, want, res.stdout)
				}
			}}},
		},
		{
			name:  "entrystatus/every-standing-order-is-evaluated-and-an-order-row-with-the-ledger-records-it",
			setup: entryFixture(map[string]string{"plan.md": plan}),
			steps: []contractStep{
				{args: status, check: entryLinesAre(lines([]string{"orders: 0 recorded, 3 unrecorded", "divergences: 0 recorded, 1 unrecorded"}, "unrecorded: plan.md divergence 1")...)},
				{args: order("3")},
				{args: order("2")},
				{args: status, check: entryLinesAre(
					"orders: 2 recorded, 1 unrecorded",
					"divergences: 0 recorded, 1 unrecorded",
					"unrecorded: plan.md standing order 1",
					"unrecorded: plan.md divergence 1",
				)},
			},
		},
		{
			name: "entrystatus/order-row-without-a-ledger-line-or-with-another-hash-leaves-the-order-unrecorded",
			setup: entryFixture(map[string]string{"plan.md": plan},
				fixtureRow("order", "3", "entry="+order3),
				fixtureRow("order", "1", "entry="+sha256Hex("An earlier wording.\n")),
			),
			steps: []contractStep{{args: status, check: entryLinesAre(lines([]string{"orders: 0 recorded, 3 unrecorded", "divergences: 0 recorded, 1 unrecorded"}, "unrecorded: plan.md divergence 1")...)}},
		},
		{
			name:  "entrystatus/order-row-matches-its-entry-by-hash-not-by-number",
			setup: entryFixture(map[string]string{"plan.md": plan}),
			steps: []contractStep{
				{args: order("3"), check: func(t *testing.T, fx *contractFixture, _ result) {
					t.Helper()
					// An order inserted above renumbers every order, so the recorded one becomes number 4.
					moved := strings.NewReplacer(
						"1. Change only these paths.\n", "1. New first order.\n2. Change only these paths.\n",
						"2. Old order.", "3. Old order.",
						"3. Divergence", "4. Divergence",
					).Replace(plan)
					writeFile(t, fx.repo, fx.folder+"/plan.md", moved)
				}},
				{args: status, check: entryLinesAre(lines([]string{"orders: 1 recorded, 3 unrecorded", "divergences: 0 recorded, 1 unrecorded"}, "unrecorded: plan.md divergence 1")...)},
			},
		},
		{
			name:  "entrystatus/renumbering-and-a-new-marker-keep-an-order-recorded-and-a-text-change-unrecords-it",
			setup: entryFixture(map[string]string{"plan.md": fourOrders("1. First order.", "2. Second order.", "3. Third order.", "4. Fourth order.")}),
			steps: []contractStep{
				{args: order("3")},
				{args: order("4"), check: rewritePlan(fourOrders("1. First order.", "2. Third order.", "3. Fourth order."))},
				{args: status, check: func(t *testing.T, fx *contractFixture, res result) {
					t.Helper()
					entryLinesAre("orders: 2 recorded, 1 unrecorded", "divergences: none", "unrecorded: plan.md standing order 1")(t, fx, res)
					rewritePlan(fourOrders("1. First order.", "- Third order.", "3. Fourth order."))(t, fx, res)
				}},
				{args: status, check: func(t *testing.T, fx *contractFixture, res result) {
					t.Helper()
					entryLinesAre("orders: 2 recorded, 1 unrecorded", "divergences: none", "unrecorded: plan.md standing order 1")(t, fx, res)
					rewritePlan(fourOrders("1. First order.", "- Third order.", "3. Fourth order, reworded."))(t, fx, res)
				}},
				{args: status, check: entryLinesAre("orders: 1 recorded, 2 unrecorded", "divergences: none", "unrecorded: plan.md standing order 1", "unrecorded: plan.md standing order 3")},
			},
		},
		{
			name:  "entrystatus/one-order-row-records-every-order-with-the-same-text",
			setup: entryFixture(map[string]string{"plan.md": fourOrders("1. Same order.", "2. Other order.", "3. Same order.")}),
			steps: []contractStep{
				{args: order("1")},
				{args: status, check: entryLinesAre("orders: 2 recorded, 1 unrecorded", "divergences: none", "unrecorded: plan.md standing order 2")},
			},
		},
		{
			name: "entrystatus/open-divergences-of-every-artifact-are-evaluated-and-a-diverge-row-records-one",
			setup: entryFixture(map[string]string{
				"plan.md":   plan,
				"intent.md": "# Intent\n\n## Divergences\n\n- intent divergence.\n",
				"spec.md":   "# Spec\n\n## Divergences\n\n- spec divergence one.\n- spec divergence two.\n",
				"brief.md":  "# Brief\n\n## Divergences\n\n- brief divergence. Reconciled: 2026-11-03.\n- brief divergence two.\n",
			},
				// A diverge row records the entry whatever artifact its target names.
				fixtureRow("diverge", "plan.md Tasks", "none entry="+sha256Hex("spec divergence two.\n")),
			),
			steps: []contractStep{
				{args: diverge("task-2", "1")},
				{args: status, check: entryLinesAre(lines([]string{"orders: 0 recorded, 3 unrecorded", "divergences: 2 recorded, 3 unrecorded"},
					"unrecorded: intent.md divergence 1",
					"unrecorded: spec.md divergence 1",
					"unrecorded: brief.md divergence 2",
				)...)},
			},
		},
		{
			name: "entrystatus/rule-row-the-ledger-lacks-prints-a-rule-divergence-line-whether-or-not-its-entry-is-open",
			setup: entryFixture(map[string]string{"plan.md": plan},
				fixtureRow("diverge", "task-2 rule", "none entry="+planDiv1),
				fixtureRow("diverge", "run rule", "none entry="+planDiv2),
			),
			steps: []contractStep{{args: status, check: entryLinesAre(lines([]string{"orders: 0 recorded, 3 unrecorded", "divergences: 1 recorded, 2 unrecorded"},
				"unrecorded: plan.md rule divergence task-2 rule",
				"unrecorded: plan.md rule divergence run rule",
			)...)}},
		},
		{
			name:  "entrystatus/rule-row-the-ledger-holds-prints-no-rule-divergence-line",
			setup: entryFixture(map[string]string{"plan.md": plan}),
			steps: []contractStep{
				{args: diverge("task-2 rule", "1")},
				{args: diverge("run rule", "2")},
				{args: status, check: entryLinesAre(lines([]string{"orders: 0 recorded, 3 unrecorded", "divergences: 1 recorded, 0 unrecorded"})...)},
			},
		},
		{
			name: "entrystatus/rule-row-of-a-folder-without-a-plan-names-the-brief",
			setup: entryFixture(map[string]string{"brief.md": "# Brief\n\n## Divergences\n\n- brief divergence.\n"},
				fixtureRow("diverge", "task-1 rule", "none entry="+sha256Hex("brief divergence.\n"))),
			steps: []contractStep{{args: status, check: entryLinesAre(
				"orders: none",
				"divergences: 1 recorded, 1 unrecorded",
				"unrecorded: brief.md rule divergence task-1 rule",
			)}},
		},
		{
			name: "entrystatus/rows-copied-into-a-clone-whose-ledger-lacks-them-are-unrecorded",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := entryFixture(map[string]string{"plan.md": plan})(t)
				fx.values["clone"] = filepath.Join(t.TempDir(), "clone")
				git(t, fx.repo, "clone", "-q", fx.repo, fx.values["clone"])
				return fx
			},
			steps: []contractStep{
				{args: order("1")},
				{args: diverge("task-2 rule", "1")},
				{args: status, stdoutLines: []string{"orders: 1 recorded, 2 unrecorded", "divergences: 1 recorded, 0 unrecorded"}, check: func(t *testing.T, fx *contractFixture, _ result) {
					t.Helper()
					for _, name := range []string{"state.tsv", "plan.md"} {
						body, err := os.ReadFile(filepath.Join(fx.repo, filepath.FromSlash(contractFolder), name))
						if err != nil {
							t.Fatal(err)
						}
						writeFile(t, clone(fx), contractFolder+"/"+name, string(body))
					}
				}},
				{dirOf: clone, args: status, check: entryLinesAre(lines([]string{"orders: 0 recorded, 3 unrecorded", "divergences: 1 recorded, 1 unrecorded"},
					"unrecorded: plan.md rule divergence task-2 rule",
				)...)},
			},
		},
		{
			name:  "entrystatus/plan-without-the-sections-prints-none",
			setup: entryFixture(map[string]string{"plan.md": "# Plan\n\n## Tasks\n\n- a task\n"}),
			steps: []contractStep{{args: status, check: entryLinesAre("orders: none", "divergences: none")}},
		},
		{
			name:  "entrystatus/only-reconciled-divergences-print-none",
			setup: entryFixture(map[string]string{"brief.md": "# Brief\n\n## Divergences\n\n- done. Reconciled: 2026-11-03.\n"}),
			steps: []contractStep{{args: status, check: entryLinesAre("orders: none", "divergences: none")}},
		},
	}
}

// lookupOther is a second flow folder for lookup cases that need two flows.
const lookupOther = "docs/specs/2026-11/feature-d"

// withSteps is a setup that runs inner and then each step against the fixture repository.
func withSteps(inner func(t *testing.T) *contractFixture, steps ...func(t *testing.T, repo string)) func(t *testing.T) *contractFixture {
	return func(t *testing.T) *contractFixture {
		t.Helper()
		fx := inner(t)
		for _, step := range steps {
			step(t, fx.repo)
		}
		return fx
	}
}

func commitStep(t *testing.T, repo string) { commitAll(t, repo, "flows") }

func gitStep(args ...string) func(t *testing.T, repo string) {
	return func(t *testing.T, repo string) { git(t, repo, args...) }
}

// stateStep writes the state file of folder with the given rows.
func stateStep(folder string, rows ...string) func(t *testing.T, repo string) {
	return func(t *testing.T, repo string) { writeState(t, repo, folder, rows...) }
}

// appendStep adds raw rows to the state file of folder.
func appendStep(folder string, rows ...string) func(t *testing.T, repo string) {
	return func(t *testing.T, repo string) {
		t.Helper()
		path := filepath.Join(repo, filepath.FromSlash(folder), "state.tsv")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, repo, folder+"/state.tsv", string(body)+strings.Join(rows, "\n")+"\n")
	}
}

func lookupCases() []contractCase {
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	later := row("2026-11-01T11:00:00Z", "intent", "challenge", "intent.md", "ok", "challenged", "none", "spec go")
	done := row("2026-11-01T12:00:00Z", "gate", "done", "branch", "ok", "merged", "none", "done nothing left")
	bound := func(branch string) string {
		return row("2026-11-01T10:30:00Z", "intent", "bind", branch, "ok", "bound", "none", "spec go")
	}
	smallFix := row("2026-11-01T13:00:00Z", "execute", "skip", "chain fix/q", "ok", "fix", "none", "execute go")
	where := []string{"where"}
	noFlow := []string{"flow: no flow found"}
	found := func(folder string) contractStep {
		return contractStep{args: where, check: func(t *testing.T, _ *contractFixture, res result) {
			t.Helper()
			if res.stdout != folder+"\n" || res.stderr != "" {
				t.Errorf("want only %s on stdout, got %+v", folder, res)
			}
		}}
	}
	notFound := contractStep{args: where, code: 3, stderrLines: noFlow}
	ambiguous := contractStep{args: where, code: 4, stderrLines: []string{
		"flow: more than one flow matches; ask which one:", "  " + contractFolder, "  " + lookupOther,
	}}
	committed := newState(seeded)
	return []contractCase{
		{
			name:  "lookup/the-only-unbound-flow-is-not-matched-off-the-default-branch",
			setup: onBranch("topic", withSteps(committed, commitStep)),
			steps: []contractStep{notFound},
		},
		{
			name:  "lookup/the-only-unbound-flow-is-not-matched-on-the-default-branch",
			setup: withSteps(committed, commitStep),
			steps: []contractStep{notFound},
		},
		{
			name:  "lookup/default-branch-finds-an-untracked-state-file",
			setup: withSteps(emptyRepo, stateStep(lookupOther, seeded), commitStep, stateStep(contractFolder, seeded)),
			steps: []contractStep{found(contractFolder)},
		},
		{
			name:  "lookup/default-branch-finds-a-staged-state-file",
			setup: withSteps(committed, stateStep(lookupOther, seeded), commitStep, appendStep(contractFolder, later), gitStep("add", contractFolder+"/state.tsv")),
			steps: []contractStep{found(contractFolder)},
		},
		{
			name:  "lookup/default-branch-finds-a-new-staged-state-file",
			setup: withSteps(emptyRepo, stateStep(lookupOther, seeded), commitStep, stateStep(contractFolder, seeded), gitStep("add", contractFolder+"/state.tsv")),
			steps: []contractStep{found(contractFolder)},
		},
		{
			name:  "lookup/default-branch-finds-a-modified-state-file",
			setup: withSteps(committed, stateStep(lookupOther, seeded), commitStep, appendStep(contractFolder, later)),
			steps: []contractStep{found(contractFolder)},
		},
		{
			name:  "lookup/default-branch-lists-two-changed-state-files",
			setup: withSteps(committed, stateStep(lookupOther, seeded), commitStep, appendStep(contractFolder, later), appendStep(lookupOther, later)),
			steps: []contractStep{ambiguous},
		},
		{
			name:  "lookup/default-branch-skips-a-closed-changed-flow",
			setup: withSteps(committed, commitStep, appendStep(contractFolder, done)),
			steps: []contractStep{notFound},
		},
		{
			name: "lookup/default-branch-ignores-a-changed-file-beside-the-state-file",
			setup: withSteps(committed, stateStep(lookupOther, seeded), commitStep, func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, contractFolder+"/intent.md", "# Intent\n")
			}),
			steps: []contractStep{notFound},
		},
		{
			name: "lookup/default-branch-finds-a-renamed-state-file-by-its-destination",
			setup: withSteps(emptyRepo, stateStep("docs/specs/2026-11/feature-old", seeded), stateStep(lookupOther, seeded), commitStep,
				gitStep("mv", "docs/specs/2026-11/feature-old", contractFolder)),
			steps: []contractStep{found(contractFolder)},
		},
		{
			name:  "lookup/a-bind-row-wins-over-a-changed-state-file-on-the-default-branch",
			setup: withSteps(committed, stateStep(lookupOther, seeded, bound("main")), commitStep, appendStep(contractFolder, later)),
			steps: []contractStep{found(lookupOther)},
		},
		{
			name:  "lookup/off-the-default-branch-a-changed-state-file-matches",
			setup: withSteps(onBranch("topic", withSteps(committed, commitStep)), stateStep(lookupOther, seeded), commitStep),
			steps: []contractStep{found(lookupOther)},
		},
		{
			name:  "lookup/off-the-default-branch-two-changed-state-files-exit-4",
			setup: withSteps(onBranch("topic", committed), stateStep(lookupOther, seeded)),
			steps: []contractStep{ambiguous},
		},
		{
			name:  "lookup/a-small-fix-wins-over-a-changed-state-file",
			setup: withSteps(onBranch("fix/q", withSteps(committed, commitStep)), appendStep(contractFolder, later), stateStep("docs/specs/unassigned", smallFix)),
			steps: []contractStep{{args: where, code: 5, stderrLines: []string{"flow: small fix in progress on fix/q; no work folder"}}},
		},
		{
			name:  "lookup/a-detached-head-matches-nothing",
			setup: withSteps(committed, commitStep, gitStep("checkout", "-q", "--detach"), appendStep(contractFolder, later)),
			steps: []contractStep{notFound},
		},
	}
}

// mergeTopic merges branch topic into main and returns to topic, as a merged pull request does.
func mergeTopic(t *testing.T, fx *contractFixture) {
	t.Helper()
	git(t, fx.repo, "switch", "-q", "main")
	git(t, fx.repo, "merge", "-q", "--no-ff", "-m", "merge topic", "topic")
	git(t, fx.repo, "switch", "-q", "topic")
}

// commitRows appends rows to the state file of folder and commits them on the current branch.
func commitRows(folder string, rows ...string) func(t *testing.T, fx *contractFixture) {
	return func(t *testing.T, fx *contractFixture) {
		t.Helper()
		appendStep(folder, rows...)(t, fx.repo)
		commitStep(t, fx.repo)
	}
}

func closingCases() []contractCase {
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	bound := func(branch, evidence string) string {
		return row("2026-11-01T10:30:00Z", "intent", "bind", branch, "ok", "bound", evidence, "spec go")
	}
	done := row("2026-11-01T12:00:00Z", "gate", "done", "branch", "ok", "merged", "none", "done nothing left")
	reopened := row("2026-11-01T13:00:00Z", "intent", "challenge", "intent.md", "ok", "reopened", "none", "spec go")
	verdict := func(ts, status, evidence string) string {
		return row(ts, "gate", "branch", "topic", status, "verdict", evidence, "done go")
	}
	slicesNote := func(ts, evidence string) string {
		return row(ts, "decompose", "note", "slices", "ok", "sliced", evidence, "execute go")
	}
	finalOK := row("2026-11-01T16:00:00Z", "execute", "verify", "final", "ok", "final review", "commit=abc1234", "gate go")
	where := []string{"where"}
	found := func(folder string) contractStep {
		return contractStep{args: where, check: func(t *testing.T, _ *contractFixture, res result) {
			t.Helper()
			if res.stdout != folder+"\n" || res.stderr != "" {
				t.Errorf("want only %s on stdout, got %+v", folder, res)
			}
		}}
	}
	foundAfter := func(before func(t *testing.T, fx *contractFixture), folder string) contractStep {
		step := found(folder)
		step.before = before
		return step
	}
	closedAfter := func(before func(t *testing.T, fx *contractFixture)) contractStep {
		return contractStep{before: before, args: where, code: 3, stderrLines: []string{"flow: no flow found"}}
	}
	// onTopic is a flow bound to branch topic and committed there only, so that
	// after a merge only its bind row can match it and the default-tip copy decides.
	onTopic := func(bindEvidence string, rows ...string) func(t *testing.T) *contractFixture {
		return withSteps(onBranch("topic", emptyRepo), stateStep(contractFolder, append([]string{seeded, bound("topic", bindEvidence)}, rows...)...), commitStep)
	}
	mergedAfterCommit := func(rows ...string) func(t *testing.T, fx *contractFixture) {
		return func(t *testing.T, fx *contractFixture) {
			t.Helper()
			commitRows(contractFolder, rows...)(t, fx)
			mergeTopic(t, fx)
		}
	}
	return []contractCase{
		{
			name:  "closing/a-done-current-row-is-not-matched",
			setup: onTopic("none", done),
			steps: []contractStep{closedAfter(nil)},
		},
		{
			name:  "closing/a-row-after-done-reopens-the-flow",
			setup: onTopic("none", done, reopened),
			steps: []contractStep{found(contractFolder)},
		},
		{
			name:  "closing/a-merged-whole-plan-verdict-closes-the-flow",
			setup: onTopic("none", verdict("2026-11-01T14:00:00Z", "ok", "#7")),
			steps: []contractStep{found(contractFolder), closedAfter(mergeTopic)},
		},
		{
			name:  "closing/a-merged-failed-verdict-leaves-the-flow-open",
			setup: onTopic("none", verdict("2026-11-01T14:00:00Z", "failed", "#7")),
			steps: []contractStep{foundAfter(mergeTopic, contractFolder)},
		},
		{
			name:  "closing/only-the-latest-verdict-on-the-default-tip-counts",
			setup: onTopic("none", verdict("2026-11-01T14:00:00Z", "ok", "#7"), verdict("2026-11-01T15:00:00Z", "failed", "#8")),
			steps: []contractStep{foundAfter(mergeTopic, contractFolder)},
		},
		{
			name:  "closing/a-latest-slice-verdict-does-not-close-a-flow-without-a-slices-note",
			setup: onTopic("none", verdict("2026-11-01T14:00:00Z", "ok", "#7 slice=S1")),
			steps: []contractStep{foundAfter(mergeTopic, contractFolder)},
		},
		{
			name:  "closing/a-merged-verdict-with-an-empty-slice-pair-leaves-the-flow-open",
			setup: onTopic("none", verdict("2026-11-01T14:00:00Z", "ok", "#7 slice=")),
			steps: []contractStep{foundAfter(mergeTopic, contractFolder)},
		},
		{
			name:  "closing/the-remote-default-tip-decides-over-a-lagging-local-default",
			setup: onTopic("none", verdict("2026-11-01T14:00:00Z", "ok", "#7")),
			steps: []contractStep{closedAfter(func(t *testing.T, fx *contractFixture) {
				t.Helper()
				mergeTopic(t, fx)
				git(t, fx.repo, "update-ref", "refs/remotes/origin/main", "main")
				git(t, fx.repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
				git(t, fx.repo, "branch", "-f", "main", "main~1")
			})},
		},
		{
			name:  "closing/with-no-default-ref-only-a-done-row-closes-the-flow",
			setup: onTopic("none", verdict("2026-11-01T14:00:00Z", "ok", "#7")),
			steps: []contractStep{
				foundAfter(func(t *testing.T, fx *contractFixture) {
					t.Helper()
					mergeTopic(t, fx)
					git(t, fx.repo, "branch", "-m", "main", "trunk")
				}, contractFolder),
				closedAfter(commitRows(contractFolder, row("2026-11-01T15:00:00Z", "gate", "done", "branch", "ok", "merged", "none", "done nothing left"))),
			},
		},
		{
			name:  "closing/a-sliced-flow-stays-open-until-every-slice-and-the-final-review-are-merged",
			setup: onTopic("none", slicesNote("2026-11-01T11:00:00Z", "slices=S1,S2 final=yes"), verdict("2026-11-01T14:00:00Z", "ok", "#7 slice=S1")),
			steps: []contractStep{
				found(contractFolder),
				foundAfter(mergeTopic, contractFolder),
				foundAfter(mergedAfterCommit(verdict("2026-11-01T15:00:00Z", "ok", "#8 slice=S2")), contractFolder),
				foundAfter(commitRows(contractFolder, finalOK), contractFolder),
				closedAfter(mergeTopic),
			},
		},
		{
			name:  "closing/a-sliced-flow-without-a-final-review-closes-on-its-last-slice",
			setup: onTopic("none", slicesNote("2026-11-01T11:00:00Z", "slices=S1,S2 final=no"), verdict("2026-11-01T14:00:00Z", "ok", "#7 slice=S1")),
			steps: []contractStep{
				foundAfter(mergeTopic, contractFolder),
				foundAfter(commitRows(contractFolder, verdict("2026-11-01T15:00:00Z", "failed", "#8 slice=S2")), contractFolder),
				foundAfter(mergeTopic, contractFolder),
				closedAfter(mergedAfterCommit(verdict("2026-11-01T16:00:00Z", "ok", "#9 slice=S2"))),
			},
		},
		{
			name: "closing/a-later-failed-slice-verdict-takes-back-an-earlier-ok-one",
			setup: onTopic("none", slicesNote("2026-11-01T11:00:00Z", "slices=S1 final=no"), verdict("2026-11-01T14:00:00Z", "ok", "#7 slice=S1"),
				verdict("2026-11-01T15:00:00Z", "failed", "#8 slice=S1")),
			steps: []contractStep{
				foundAfter(mergeTopic, contractFolder),
				closedAfter(mergedAfterCommit(verdict("2026-11-01T16:00:00Z", "ok", "#9 slice=S1"))),
			},
		},
		{
			name: "closing/a-later-failed-final-review-takes-back-an-earlier-ok-one",
			setup: onTopic("none", slicesNote("2026-11-01T11:00:00Z", "slices=S1 final=yes"), verdict("2026-11-01T14:00:00Z", "ok", "#7 slice=S1"), finalOK,
				row("2026-11-01T17:00:00Z", "execute", "verify", "final", "failed", "final review found a gap", "commit=abc1234", "execute fix the gap")),
			steps: []contractStep{
				foundAfter(mergeTopic, contractFolder),
				closedAfter(mergedAfterCommit(row("2026-11-01T18:00:00Z", "execute", "verify", "final", "ok", "final review", "commit=abc1234", "gate go"))),
			},
		},
		{
			name: "closing/a-sliced-flow-reads-the-latest-slices-note",
			setup: onTopic("none", slicesNote("2026-11-01T11:00:00Z", "slices=S1 final=no"), slicesNote("2026-11-01T11:30:00Z", "slices=S1,S2 final=no"),
				verdict("2026-11-01T14:00:00Z", "ok", "#7 slice=S1")),
			steps: []contractStep{
				foundAfter(mergeTopic, contractFolder),
				closedAfter(mergedAfterCommit(verdict("2026-11-01T15:00:00Z", "ok", "#8 slice=S2"))),
			},
		},
		{
			name:  "closing/a-sliced-flow-is-not-closed-by-a-whole-plan-verdict",
			setup: onTopic("none", slicesNote("2026-11-01T11:00:00Z", "slices=S1 final=no"), verdict("2026-11-01T14:00:00Z", "ok", "#7")),
			steps: []contractStep{foundAfter(mergeTopic, contractFolder)},
		},
		{
			name:  "closing/one-verdict-for-two-slices-counts-for-both",
			setup: onTopic("none", slicesNote("2026-11-01T11:00:00Z", "slices=S2,S3 final=no"), verdict("2026-11-01T14:00:00Z", "ok", "#7 slice=S2,S3")),
			steps: []contractStep{found(contractFolder), closedAfter(mergeTopic)},
		},
		{
			name:  "closing/a-slice-bound-flow-without-a-slices-note-stays-open-after-its-lane-merges",
			setup: onTopic("slice=S1", verdict("2026-11-01T14:00:00Z", "ok", "#7 slice=S1")),
			steps: []contractStep{found(contractFolder), foundAfter(mergeTopic, contractFolder)},
		},
		{
			name:  "closing/a-slice-bound-flow-without-a-slices-note-closes-by-a-whole-plan-verdict",
			setup: onTopic("slice=S1", verdict("2026-11-01T14:00:00Z", "ok", "#7 slice=S1")),
			steps: []contractStep{
				foundAfter(mergeTopic, contractFolder),
				foundAfter(commitRows(contractFolder, verdict("2026-11-01T15:00:00Z", "ok", "#8")), contractFolder),
				closedAfter(mergeTopic),
			},
		},
		{
			name:  "closing/on-the-default-branch-a-verdict-only-in-the-working-file-leaves-the-flow-open",
			setup: withSteps(emptyRepo, stateStep(contractFolder, seeded, bound("main", "none")), commitStep, appendStep(contractFolder, verdict("2026-11-01T14:00:00Z", "ok", "#7"))),
			steps: []contractStep{
				found(contractFolder),
				closedAfter(func(t *testing.T, fx *contractFixture) { t.Helper(); commitStep(t, fx.repo) }),
			},
		},
		{
			name: "closing/a-closed-flow-with-a-later-bind-does-not-hide-an-open-one",
			setup: withSteps(onBranch("topic", emptyRepo),
				stateStep(lookupOther, seeded, bound("topic", "none")),
				stateStep(contractFolder, seeded, row("2026-11-01T11:30:00Z", "intent", "bind", "topic", "ok", "bound", "none", "spec go"), verdict("2026-11-01T14:00:00Z", "ok", "#7")),
				commitStep),
			steps: []contractStep{found(contractFolder), foundAfter(mergeTopic, lookupOther)},
		},
		{
			name:  "closing/a-folder-call-reads-a-merged-flow",
			setup: onTopic("none", verdict("2026-11-01T14:00:00Z", "ok", "#7")),
			steps: []contractStep{
				{before: mergeTopic, args: contractLog("gate", "pause", "branch", "pending", "after merge", "none", "gate go")},
				{args: []string{"status", "--folder", contractFolder}, stdoutHas: []string{"flow: " + contractFolder}},
			},
			rows: []string{
				cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"),
				cellsRow("intent", "bind", "topic", "ok", "bound", "none", "spec go"),
				cellsRow("gate", "branch", "topic", "ok", "verdict", "#7", "done go"),
				cellsRow("gate", "pause", "branch", "pending", "after merge", "none", "gate go"),
			},
		},
	}
}

func slicebindCases() []contractCase {
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	seededRow := cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	bindRow := func(evidence string) string {
		return cellsRow("intent", "bind", "topic", "ok", "bound topic to this flow", evidence, "spec go")
	}
	bind := func(ids ...string) contractStep {
		args := []string{"bind", "--folder", contractFolder}
		for _, id := range ids {
			args = append(args, "--slice", id)
		}
		return contractStep{args: args}
	}
	refusedBind := func(message string, ids ...string) contractStep {
		step := bind(ids...)
		step.code, step.stderrLines = 2, []string{"flow: invalid " + message}
		return step
	}
	onTopic := onBranch("topic", withSteps(newState(seeded), commitStep))
	// worked is onTopic with a task commit on top, so that a bare auto names a
	// commit that changes a file outside the work folder.
	worked := withHead(withSteps(onTopic, func(t *testing.T, repo string) {
		t.Helper()
		writeFile(t, repo, "src/a.txt", "one\n")
		commitAll(t, repo, "task")
	}))
	verdict := contractLog("gate", "branch", "topic", "ok", "verdict", "#7", "done go")
	verified := contractLog("execute", "verify", "task-1", "ok", "green", "auto", "execute go")
	challenged := contractLog("execute", "challenge", "plan.md", "ok", "checked", "none", "execute go")
	// tagged is the rows after a bind with slice, or with none when slice is
	// empty, and then verdict, verified and challenged at the fixture's HEAD.
	tagged := func(slice string) func(fx *contractFixture) []string {
		return func(fx *contractFixture) []string {
			pair, bindEvidence := "", "none"
			if slice != "" {
				pair, bindEvidence = " slice="+slice, "slice="+slice
			}
			return []string{
				seededRow,
				bindRow(bindEvidence),
				cellsRow("gate", "branch", "topic", "ok", "verdict", "#7"+pair, "done go"),
				cellsRow("execute", "verify", "task-1", "ok", "green", "commit="+fx.values["head"]+" patch-id="+fx.values["head-patch"]+pair, "execute go"),
				cellsRow("execute", "challenge", "plan.md", "ok", "checked", "none", "execute go"),
			}
		}
	}
	malformed := " -- a slice id is S followed by a positive integer; pass one, or several joined by commas, each once"
	finished := "folder: '" + contractFolder + "' -- the flow is finished: its verdict is on the default branch; start new work in a new flow"
	mergedVerdict := func(t *testing.T, fx *contractFixture) {
		t.Helper()
		commitRows(contractFolder,
			row("2026-11-01T10:30:00Z", "intent", "bind", "topic", "ok", "bound", "none", "spec go"),
			row("2026-11-01T14:00:00Z", "gate", "branch", "topic", "ok", "verdict", "#7", "done go"))(t, fx)
		mergeTopic(t, fx)
	}
	mergedRows := []string{
		seededRow,
		cellsRow("intent", "bind", "topic", "ok", "bound", "none", "spec go"),
		cellsRow("gate", "branch", "topic", "ok", "verdict", "#7", "done go"),
	}
	// refusedThrough binds the merge-closed flow through the absolute folder path
	// that alias makes of the repository path, and expects the refusal.
	refusedThrough := func(alias func(t *testing.T, repo string) string) contractStep {
		return contractStep{
			before: func(t *testing.T, fx *contractFixture) {
				t.Helper()
				fx.values["alias"] = alias(t, fx.repo)
				mergedVerdict(t, fx)
			},
			argsOf: func(fx *contractFixture) []string {
				return []string{"bind", "--folder", filepath.Join(fx.values["alias"], filepath.FromSlash(contractFolder))}
			},
			code:      2,
			stderrHas: []string{"the flow is finished: its verdict is on the default branch"},
		}
	}
	const divergence = "**2026-11-01, task-1** — first divergence.\n"
	// withPlan is worked with a plan whose one open divergence a diverge row records.
	withPlan := withSteps(worked, func(t *testing.T, repo string) {
		t.Helper()
		writeFile(t, repo, contractFolder+"/plan.md", "# Plan\n\n## Divergences\n\n- "+divergence)
	})
	diverged := func(target string) []string {
		return append(contractLog("execute", "diverge", target, "ok", contractWhy, "auto", contractNext), "--entry", "1")
	}
	// entryThenSlice expects the last row's evidence to carry the entry pair
	// before the slice pair the bound branch appends.
	entryThenSlice := func(t *testing.T, fx *contractFixture, _ result) {
		t.Helper()
		rows := contractRows(t, fx)
		evidence := cells(rows[len(rows)-1])[5]
		if want := " entry=" + sha256Hex(divergence) + " slice=S1"; !strings.HasSuffix(evidence, want) {
			t.Errorf("evidence %q does not end with %q", evidence, want)
		}
	}
	status := []string{"status", "--folder", contractFolder}
	var malformedSteps []contractStep
	for _, ids := range []string{"", "1", "s1", "S0", "S01", "S1,", ",S1", "S1,,S2", "S1,S1", "S1 S2", "S-1", "S1x"} {
		malformedSteps = append(malformedSteps, refusedBind("slice: '"+ids+"'"+malformed, ids))
	}
	return []contractCase{
		{
			name:  "slicebind/one-slice-id-is-stored-as-the-bind-evidence",
			setup: onTopic,
			steps: []contractStep{bind("S1")},
			rows:  []string{seededRow, bindRow("slice=S1")},
		},
		{
			name:  "slicebind/several-slice-ids-are-stored-in-the-order-given",
			setup: onTopic,
			steps: []contractStep{bind("S3,S2")},
			rows:  []string{seededRow, bindRow("slice=S3,S2")},
		},
		{
			name:  "slicebind/an-id-that-is-not-a-slice-id-exits-2",
			setup: onTopic,
			steps: malformedSteps,
			rows:  []string{seededRow},
		},
		{
			name:  "slicebind/a-slice-option-without-a-value-is-a-usage-error",
			setup: onTopic,
			steps: []contractStep{{args: []string{"bind", "--folder", contractFolder, "--slice"}, code: 1, stderrHas: []string{"flow bind [--folder <dir>] [--slice <ids>]"}}},
			rows:  []string{seededRow},
		},
		{
			name:  "slicebind/the-usage-text-lists-the-slice-option",
			setup: emptyRepo,
			steps: []contractStep{{args: []string{"bind", "--unknown"}, code: 1, stderrHas: []string{"flow bind [--folder <dir>] [--slice <ids>]", "--slice <ids>"}}},
		},
		{
			name:  "slicebind/the-same-ids-again-write-nothing",
			setup: onTopic,
			steps: []contractStep{bind("S2,S3"), bind("S2,S3"), bind()},
			rows:  []string{seededRow, bindRow("slice=S2,S3")},
		},
		{
			name:  "slicebind/ids-on-a-branch-bound-with-none-write-a-new-row",
			setup: onTopic,
			steps: []contractStep{bind(), bind("S1")},
			rows:  []string{seededRow, bindRow("none"), bindRow("slice=S1")},
		},
		{
			name:  "slicebind/other-ids-exit-2",
			setup: onTopic,
			steps: []contractStep{
				bind("S1,S2"),
				refusedBind("slice: 'S3' -- topic is bound to slice=S1,S2 in this flow", "S3"),
				refusedBind("slice: 'S2,S1' -- topic is bound to slice=S1,S2 in this flow", "S2,S1"),
				refusedBind("slice: 'S1' -- topic is bound to slice=S1,S2 in this flow", "S1"),
			},
			rows: []string{seededRow, bindRow("slice=S1,S2")},
		},
		{
			name:  "slicebind/the-default-branch-is-refused-with-a-slice",
			setup: withSteps(newState(seeded), commitStep),
			steps: []contractStep{refusedBind("branch: 'main' -- the default branch is never bound to a flow", "S1")},
			rows:  []string{seededRow},
		},
		{
			name:  "slicebind/a-detached-head-is-refused-with-a-slice",
			setup: withSteps(newState(seeded), commitStep, gitStep("checkout", "-q", "--detach")),
			steps: []contractStep{refusedBind("branch: '' -- a detached HEAD has no branch to bind", "S1")},
			rows:  []string{seededRow},
		},
		{
			name:  "slicebind/a-flow-closed-by-merge-is-refused-with-a-slice",
			setup: onTopic,
			steps: []contractStep{{before: mergedVerdict, args: bind("S1").args, code: 2, stderrLines: []string{"flow: invalid " + finished}}},
			rows:  mergedRows,
		},
		{
			name:  "slicebind/a-flow-closed-by-merge-is-refused-without-a-slice",
			setup: onTopic,
			steps: []contractStep{{before: mergedVerdict, args: bind().args, code: 2, stderrLines: []string{"flow: invalid " + finished}}},
			rows:  mergedRows,
		},
		{
			// No commit holds a local flow's state.tsv, so a committed copy of the
			// same folder on the default branch says nothing about it.
			name: "slicebind/a-local-flow-is-not-closed-by-a-verdict-on-the-default-branch",
			setup: inLocalStore(withSteps(onBranch("topic", emptyRepo),
				gitStep("switch", "-q", "main"),
				stateStep(contractFolder, seeded,
					row("2026-11-01T10:30:00Z", "intent", "bind", "topic", "ok", "bound", "none", "spec go"),
					row("2026-11-01T14:00:00Z", "gate", "branch", "topic", "ok", "verdict", "#7", "done go")),
				commitStep,
				gitStep("switch", "-q", "topic"),
				stateStep(".git/agentics/work/"+contractFolder, seeded))),
			steps: []contractStep{{args: bind().args, check: quiet}},
			rows:  []string{seededRow, bindRow("none")},
			check: nothingUnder("docs"),
		},
		{
			name:  "slicebind/a-flow-closed-by-a-done-row-is-reopened-by-the-bind",
			setup: withSteps(onTopic, appendStep(contractFolder, row("2026-11-01T12:00:00Z", "gate", "done", "branch", "ok", "merged", "none", "done nothing left"))),
			steps: []contractStep{bind("S1"), {args: []string{"where"}, stdoutLines: []string{contractFolder}}},
			rows: []string{
				seededRow,
				cellsRow("gate", "done", "branch", "ok", "merged", "none", "done nothing left"),
				cellsRow("gate", "bind", "topic", "ok", "bound topic to this flow", "slice=S1", "done nothing left"),
			},
		},
		{
			name:  "slicebind/a-slice-bound-branch-tags-its-branch-and-commit-rows",
			setup: worked,
			steps: []contractStep{bind("S2,S3"), {args: verdict}, {args: verified}, {args: challenged}},
			check: rowsAre(tagged("S2,S3")),
		},
		{
			name:  "slicebind/a-branch-bound-with-no-slice-tags-nothing",
			setup: worked,
			steps: []contractStep{bind(), {args: verdict}, {args: verified}, {args: challenged}},
			check: rowsAre(tagged("")),
		},
		{
			name: "slicebind/the-latest-bind-row-of-the-current-branch-decides",
			setup: withSteps(onTopic, appendStep(contractFolder,
				row("2026-11-01T10:30:00Z", "intent", "bind", "other", "ok", "bound", "slice=S9", "spec go"),
				row("2026-11-01T10:50:00Z", "intent", "bind", "topic", "ok", "bound", "slice=S4", "spec go"),
				row("2026-11-01T10:40:00Z", "intent", "bind", "topic", "ok", "bound", "slice=S1", "spec go"))),
			steps: []contractStep{{args: verdict}},
			rows: []string{
				seededRow,
				cellsRow("intent", "bind", "other", "ok", "bound", "slice=S9", "spec go"),
				cellsRow("intent", "bind", "topic", "ok", "bound", "slice=S4", "spec go"),
				cellsRow("intent", "bind", "topic", "ok", "bound", "slice=S1", "spec go"),
				cellsRow("gate", "branch", "topic", "ok", "verdict", "#7 slice=S4", "done go"),
			},
		},
		{
			name:  "slicebind/another-branch-slice-bind-tags-nothing-here",
			setup: withSteps(onTopic, appendStep(contractFolder, row("2026-11-01T10:30:00Z", "intent", "bind", "other", "ok", "bound", "slice=S9", "spec go"))),
			steps: []contractStep{{args: verdict}},
			rows: []string{
				seededRow,
				cellsRow("intent", "bind", "other", "ok", "bound", "slice=S9", "spec go"),
				cellsRow("gate", "branch", "topic", "ok", "verdict", "#7", "done go"),
			},
		},
		{
			name:  "slicebind/a-written-pair-equal-to-the-branch-slice-is-stored-unchanged",
			setup: onTopic,
			steps: []contractStep{bind("S1,S2"), {args: contractLog("gate", "branch", "topic", "ok", "verdict", "#7 slice=S1,S2", "done go")}},
			rows:  []string{seededRow, bindRow("slice=S1,S2"), cellsRow("gate", "branch", "topic", "ok", "verdict", "#7 slice=S1,S2", "done go")},
		},
		{
			name:  "slicebind/a-written-pair-other-than-the-branch-slice-exits-2",
			setup: onTopic,
			steps: []contractStep{
				bind("S1,S2"),
				{
					args:        contractLog("gate", "branch", "topic", "ok", "verdict", "#7 slice=S2", "done go"),
					code:        2,
					stderrLines: []string{"flow: invalid evidence: '#7 slice=S2' -- topic is bound to slice=S1,S2 in this flow"},
				},
			},
			rows: []string{seededRow, bindRow("slice=S1,S2")},
		},
		{
			name:  "slicebind/a-written-pair-on-a-branch-bound-with-no-slice-is-stored-unchanged",
			setup: onTopic,
			steps: []contractStep{bind(), {args: contractLog("gate", "branch", "topic", "ok", "verdict", "#7 slice=S2", "done go")}},
			rows:  []string{seededRow, bindRow("none"), cellsRow("gate", "branch", "topic", "ok", "verdict", "#7 slice=S2", "done go")},
		},
		{
			name:  "slicebind/a-tagged-divergence-row-records-its-entry",
			setup: withPlan,
			steps: []contractStep{
				bind("S1"),
				{args: diverged("task-1"), check: entryThenSlice},
				{args: status, check: entryLinesAre("orders: none", "divergences: 1 recorded, 0 unrecorded")},
			},
		},
		{
			name: "slicebind/a-tagged-rule-row-copied-into-a-clone-whose-ledger-lacks-it-is-unrecorded",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := withPlan(t)
				fx.values["clone"] = filepath.Join(t.TempDir(), "clone")
				git(t, fx.repo, "clone", "-q", fx.repo, fx.values["clone"])
				return fx
			},
			steps: []contractStep{
				bind("S1"),
				{args: diverged("task-1 rule"), check: entryThenSlice},
				{args: status, check: func(t *testing.T, fx *contractFixture, res result) {
					t.Helper()
					entryLinesAre("orders: none", "divergences: 1 recorded, 0 unrecorded")(t, fx, res)
					for _, name := range []string{"state.tsv", "plan.md"} {
						body, err := os.ReadFile(filepath.Join(fx.repo, filepath.FromSlash(contractFolder), name))
						if err != nil {
							t.Fatal(err)
						}
						writeFile(t, fx.values["clone"], contractFolder+"/"+name, string(body))
					}
				}},
				{dirOf: func(fx *contractFixture) string { return fx.values["clone"] }, args: status, check: entryLinesAre(
					"orders: none",
					"divergences: 1 recorded, 1 unrecorded",
					"unrecorded: plan.md rule divergence task-1 rule",
				)},
			},
		},
		{
			name:  "slicebind/a-flow-closed-by-merge-is-refused-through-another-drive-letter-case",
			setup: onTopic,
			steps: []contractStep{refusedThrough(func(_ *testing.T, repo string) string {
				// Where paths have no drive letter the first character is a
				// separator and the path is unchanged.
				first := repo[:1]
				if upper := strings.ToUpper(first); upper != first {
					return upper + repo[1:]
				}
				return strings.ToLower(first) + repo[1:]
			})},
			rows: mergedRows,
		},
		{
			name:  "slicebind/a-flow-closed-by-merge-is-refused-through-a-symlinked-folder",
			setup: onTopic,
			steps: []contractStep{refusedThrough(func(t *testing.T, repo string) string {
				t.Helper()
				link := filepath.Join(t.TempDir(), "link")
				if err := os.Symlink(repo, link); err != nil {
					t.Skipf("this host cannot create a symlink: %v", err)
				}
				return link
			})},
			rows: mergedRows,
		},
	}
}

func sliceverdictCases() []contractCase {
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	status := []string{"status", "--folder", contractFolder}
	// verified is a verify row whose commit is in no repository, so status
	// reports it unless a later verdict turns it into history.
	verified := func(ts, task, slice string) string {
		evidence := "commit=0000000000000000000000000000000000000001 patch-id=" + pid40
		if slice != "" {
			evidence += " slice=" + slice
		}
		return row(ts, "execute", "verify", task, "ok", "green", evidence, "execute go")
	}
	verdict := func(event, status, evidence string) string {
		return row("2026-11-01T12:00:00Z", "gate", event, "topic", status, "verdict", evidence, "done go")
	}
	reported := func(tasks ...string) []lineCount {
		counts := []lineCount{{"mismatch:", len(tasks)}}
		for _, task := range tasks {
			counts = append(counts, lineCount{"mismatch: verify " + task + " commit ", 1})
		}
		return counts
	}
	checked := func(name string, rows []string, tasks ...string) contractCase {
		return contractCase{
			name:  "sliceverdict/" + name,
			setup: newState(append([]string{seeded}, rows...)...),
			steps: []contractStep{{args: status, stdoutCount: reported(tasks...)}},
		}
	}
	tagged := []string{
		verified("2026-11-01T11:00:00Z", "task-1", "S1"),
		verified("2026-11-01T11:01:00Z", "task-2", "S2"),
		verified("2026-11-01T11:02:00Z", "task-3", "S2,S3"),
		verified("2026-11-01T11:03:00Z", "task-4", ""),
	}
	enterRun := func(ts, evidence string) string {
		return row(ts, "execute", "enter", "run", "ok", "start", evidence, "execute go")
	}
	modes := withSteps(newState(seeded,
		enterRun("2026-11-01T11:00:00Z", "mode=solo branch=topic"),
		enterRun("2026-11-01T11:10:00Z", "mode=pair branch=topic"),
		enterRun("2026-11-01T11:20:00Z", "mode=auto branch=other"),
		enterRun("2026-11-01T11:30:00Z", "mode=auto"),
	), commitStep)
	return []contractCase{
		checked("a-failed-branch-verdict-leaves-earlier-commit-rows-checked",
			[]string{verified("2026-11-01T11:00:00Z", "task-1", ""), verdict("branch", "failed", "#7")}, "task-1"),
		checked("a-failed-slice-verdict-leaves-its-slice-rows-checked",
			[]string{verified("2026-11-01T11:00:00Z", "task-1", "S1"), verdict("branch", "failed", "#7 slice=S1")}, "task-1"),
		checked("an-ok-branch-verdict-without-a-slice-retires-every-earlier-commit-row",
			append(append([]string{}, tagged...), verdict("branch", "ok", "#7"))),
		checked("an-ok-slice-verdict-retires-only-rows-that-share-a-slice",
			append(append([]string{}, tagged...), verdict("branch", "ok", "#7 slice=S1,S3")), "task-2", "task-4"),
		checked("an-ok-slice-verdict-leaves-later-rows-of-its-slice-checked",
			[]string{verdict("branch", "ok", "#7 slice=S1"), verified("2026-11-01T13:00:00Z", "task-1", "S1")}, "task-1"),
		checked("an-empty-slice-pair-retires-nothing",
			append(append([]string{}, tagged...), verdict("branch", "ok", "#7 slice=")), "task-1", "task-2", "task-3", "task-4"),
		checked("a-failed-release-row-retires-every-earlier-commit-row",
			append(append([]string{}, tagged...), verdict("release", "failed", "none"))),
		checked("a-failed-change-row-retires-every-earlier-commit-row",
			append(append([]string{}, tagged...), verdict("change", "failed", "none"))),
		{
			name:  "sliceverdict/mode-comes-from-the-latest-enter-run-row-of-the-current-branch",
			setup: onBranch("topic", modes),
			steps: []contractStep{{args: status, stdoutLines: []string{"mode: pair (topic)"}, stdoutCount: []lineCount{{"mode:", 1}}}},
		},
		{
			name:  "sliceverdict/another-branch-prints-no-mode",
			setup: onBranch("third", modes),
			steps: []contractStep{{args: status, stdoutCount: []lineCount{{"mode:", 0}}}},
		},
		{
			name:  "sliceverdict/a-detached-head-prints-no-mode",
			setup: withSteps(modes, gitStep("checkout", "-q", "--detach")),
			steps: []contractStep{{args: status, stdoutCount: []lineCount{{"mode:", 0}}}},
		},
	}
}

func autoCases() []contractCase {
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	seededCells := cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	status := []string{"status", "--folder", contractFolder}
	committed := func(rel string) func(t *testing.T, repo string) {
		return func(t *testing.T, repo string) {
			t.Helper()
			writeFile(t, repo, rel, rel+"\n")
			commitAll(t, repo, rel)
		}
	}
	// artifactHead commits only the flow's state file, so HEAD changes
	// nothing outside the work folder.
	artifactHead := withHead(withSteps(newState(seeded), commitStep))
	// workHead commits the state file together with a file outside the folder.
	workHead := withHead(withSteps(newState(seeded), func(t *testing.T, repo string) {
		t.Helper()
		writeFile(t, repo, "src/a.txt", "one\n")
		commitAll(t, repo, "task")
	}))
	mergeHead := withHead(withSteps(newState(seeded), commitStep,
		gitStep("switch", "-q", "-c", "side"), committed("side.txt"),
		gitStep("switch", "-q", "main"), committed("main.txt"),
		gitStep("merge", "-q", "--no-ff", "--no-edit", "side")))
	emptyHead := withSteps(newState(seeded), commitStep, gitStep("commit", "-q", "--allow-empty", "-m", "empty"))
	logAuto := func(event, evidence string) []string {
		return contractLog("execute", event, "task-1", "ok", "green", evidence, "execute go")
	}
	const (
		artifactOnly = "flow: invalid evidence: 'auto' -- auto names HEAD, which changes only the work folder; pass auto:<sha> for the task commit"
		nothing      = " -- the commit changes nothing, so it has no patch-id"
		absent       = "0000000000000000000000000000000000000001"
	)
	// withCommitRow adds a verify row whose evidence is commit= alone.
	withCommitRow := func(inner func(t *testing.T) *contractFixture, sha func(fx *contractFixture) string) func(t *testing.T) *contractFixture {
		return func(t *testing.T) *contractFixture {
			t.Helper()
			fx := inner(t)
			appendStep(contractFolder, row("2026-11-01T11:00:00Z", "execute", "verify", "wave-1", "ok", "green", "commit="+sha(fx), "execute go"))(t, fx.repo)
			return fx
		}
	}
	var cases []contractCase
	for _, event := range []string{"verify", "revert", "diverge"} {
		cases = append(cases,
			contractCase{
				name:  "auto/a-bare-auto-on-" + event + "-is-refused-when-head-changes-only-the-work-folder",
				setup: artifactHead,
				steps: []contractStep{{args: logAuto(event, "auto"), code: 2, stderrLines: []string{artifactOnly}}},
				rows:  []string{seededCells},
			},
			contractCase{
				name:  "auto/a-bare-auto-on-" + event + "-is-accepted-when-head-changes-a-file-outside-the-work-folder",
				setup: workHead,
				steps: []contractStep{{args: logAuto(event, "auto")}},
				check: rowsAre(func(fx *contractFixture) []string {
					return []string{seededCells, cellsRow("execute", event, "task-1", "ok", "green", "commit="+fx.values["head"]+" patch-id="+fx.values["head-patch"], "execute go")}
				}),
			})
	}
	return append(cases,
		contractCase{
			name:  "auto/auto-naming-an-artifact-only-commit-is-accepted",
			setup: artifactHead,
			steps: []contractStep{{argsOf: func(fx *contractFixture) []string { return logAuto("verify", "auto:"+fx.values["head"]) }}},
			check: rowsAre(func(fx *contractFixture) []string {
				return []string{seededCells, cellsRow("execute", "verify", "task-1", "ok", "green", "commit="+fx.values["head"]+" patch-id="+fx.values["head-patch"], "execute go")}
			}),
		},
		contractCase{
			name:  "auto/auto-on-a-merge-commit-stores-the-commit-alone",
			setup: mergeHead,
			steps: []contractStep{{args: logAuto("verify", "auto")}},
			check: rowsAre(func(fx *contractFixture) []string {
				return []string{seededCells, cellsRow("execute", "verify", "task-1", "ok", "green", "commit="+fx.values["head"], "execute go")}
			}),
		},
		contractCase{
			name:  "auto/a-commit-row-on-a-merge-commit-stores-the-commit-alone",
			setup: mergeHead,
			steps: []contractStep{{args: contractLog("execute", "commit", "fix", "ok", "green", "auto:HEAD", "execute go")}},
			check: rowsAre(func(fx *contractFixture) []string {
				return []string{seededCells, cellsRow("execute", "commit", "fix", "ok", "green", "commit="+fx.values["head"], "execute go")}
			}),
		},
		contractCase{
			name:  "auto/auto-on-an-empty-commit-is-refused",
			setup: emptyHead,
			steps: []contractStep{{args: logAuto("verify", "auto"), code: 2, stderrLines: []string{"flow: invalid evidence: 'auto'" + nothing}}},
			rows:  []string{seededCells},
		},
		contractCase{
			name:  "auto/a-commit-row-naming-an-empty-commit-is-refused",
			setup: emptyHead,
			steps: []contractStep{{args: contractLog("execute", "commit", "fix", "ok", "green", "auto:HEAD", "execute go"), code: 2, stderrLines: []string{"flow: invalid evidence: 'auto:HEAD'" + nothing}}},
			rows:  []string{seededCells},
		},
		contractCase{
			name:  "auto/a-row-without-a-patch-id-whose-commit-is-an-ancestor-is-no-mismatch",
			setup: withCommitRow(workHead, func(fx *contractFixture) string { return fx.values["head"] }),
			steps: []contractStep{{args: status, stdoutCount: []lineCount{{"mismatch:", 0}}}},
		},
		contractCase{
			name:  "auto/a-row-without-a-patch-id-whose-commit-is-no-ancestor-prints-the-mismatch",
			setup: withCommitRow(workHead, func(*contractFixture) string { return absent }),
			steps: []contractStep{{
				args:        status,
				stdoutLines: []string{"mismatch: verify wave-1 commit " + absent + " is not an ancestor of HEAD"},
				stdoutCount: []lineCount{{"mismatch:", 1}},
			}},
		},
	)
}

// fixtureStep changes a contract fixture and may record values for later steps.
type fixtureStep func(t *testing.T, fx *contractFixture)

// withFixtureSteps is a setup that runs inner and then each step against the fixture.
func withFixtureSteps(inner func(t *testing.T) *contractFixture, steps ...fixtureStep) func(t *testing.T) *contractFixture {
	return func(t *testing.T) *contractFixture {
		t.Helper()
		fx := inner(t)
		for _, step := range steps {
			step(t, fx)
		}
		return fx
	}
}

// committedAs writes rel and commits it alone, and records the commit under key.
func committedAs(key, rel string) fixtureStep {
	return func(t *testing.T, fx *contractFixture) {
		t.Helper()
		writeFile(t, fx.repo, rel, key+"\n")
		git(t, fx.repo, "add", "--", rel)
		git(t, fx.repo, "commit", "-q", "-m", key)
		fx.values[key] = gitOut(t, fx.repo, "rev-parse", "HEAD")
	}
}

// headAs records the current HEAD under key.
func headAs(key string) fixtureStep {
	return func(t *testing.T, fx *contractFixture) {
		t.Helper()
		fx.values[key] = gitOut(t, fx.repo, "rev-parse", "HEAD")
	}
}

// gitFixture runs git in the fixture repository.
func gitFixture(args ...string) fixtureStep {
	return func(t *testing.T, fx *contractFixture) {
		t.Helper()
		git(t, fx.repo, args...)
	}
}

// rowOf appends to the flow's state file the row that build returns for the fixture.
func rowOf(build func(fx *contractFixture) string) fixtureStep {
	return func(t *testing.T, fx *contractFixture) {
		t.Helper()
		appendStep(contractFolder, build(fx))(t, fx.repo)
	}
}

// unaccountedIs expects the unaccounted: line to list the commits recorded
// under keys, in that order, and to follow the mode: and slice: lines directly.
func unaccountedIs(keys ...string) func(t *testing.T, fx *contractFixture, res result) {
	return unaccountedOf(len(keys), keys...)
}

// unaccountedOf is unaccountedIs for a line whose count exceeds the commits it lists.
func unaccountedOf(count int, keys ...string) func(t *testing.T, fx *contractFixture, res result) {
	return func(t *testing.T, fx *contractFixture, res result) {
		t.Helper()
		want := "unaccounted: none"
		if count > 0 {
			want = "unaccounted: " + strconv.Itoa(count)
			for _, key := range keys {
				want += " " + fx.values[key][:7]
			}
		}
		lines := strings.Split(strings.TrimRight(res.stdout, "\n"), "\n")
		at := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, "unaccounted:") })
		if at < 2 || lines[at] != want || !strings.HasPrefix(lines[at-1], "slice:") || !strings.HasPrefix(lines[at-2], "mode:") {
			t.Errorf("want %q right after the mode: and slice: lines in %q", want, res.stdout)
		}
	}
}

func unaccountedCases() []contractCase {
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	bound := row("2026-11-01T10:10:00Z", "execute", "bind", "topic", "ok", "bound topic to this flow", "none", "execute go")
	run := row("2026-11-01T10:20:00Z", "execute", "enter", "run", "ok", "start", "mode=solo branch=topic", "execute go")
	status := []string{"status", "--folder", contractFolder}
	// topic is a flow committed on main with a bind row for topic and a run
	// mode for it, checked out on topic, followed by steps.
	topic := func(steps ...fixtureStep) func(t *testing.T) *contractFixture {
		return withFixtureSteps(onBranch("topic", withSteps(newState(seeded, bound, run), commitStep)), steps...)
	}
	verify := func(evidence string) string {
		return row("2026-11-01T11:00:00Z", "execute", "verify", "task-1", "ok", "green", evidence, "execute go")
	}
	red := func(evidence string) string {
		return row("2026-11-01T11:00:00Z", "execute", "verify", "red-task-1", "failed", "red", evidence, "execute go")
	}
	listed := func(name string, setup func(t *testing.T) *contractFixture, keys ...string) contractCase {
		return contractCase{
			name:  "unaccounted/" + name,
			setup: setup,
			steps: []contractStep{{args: status, stdoutCount: []lineCount{{"unaccounted:", 1}}, check: unaccountedIs(keys...)}},
		}
	}
	notComputed := func(name, why string, setup func(t *testing.T) *contractFixture) contractCase {
		return contractCase{
			name:  "unaccounted/" + name,
			setup: setup,
			steps: []contractStep{{args: status, stdoutLines: []string{"unaccounted: not computed (" + why + ")"}, stdoutCount: []lineCount{{"unaccounted:", 1}}}},
		}
	}
	const notBound = "this branch is not bound to the flow"
	twoWork := topic(committedAs("one", "src/one.txt"), committedAs("two", "src/two.txt"))
	var many []fixtureStep
	var firstTwenty []string
	for i := 1; i <= 21; i++ {
		key := "c" + strconv.Itoa(i)
		many = append(many, committedAs(key, "src/"+key+".txt"))
		if i <= 20 {
			firstTwenty = append(firstTwenty, key)
		}
	}
	return []contractCase{
		listed("a-bound-branch-without-commits-prints-none", topic()),
		listed("unnamed-commits-are-listed-oldest-first", twoWork, "one", "two"),
		listed("a-seven-character-commit-value-names-its-commit",
			topic(committedAs("one", "src/one.txt"), committedAs("two", "src/two.txt"),
				rowOf(func(fx *contractFixture) string {
					return verify("commit=" + fx.values["one"][:7] + " patch-id=" + pid40)
				})),
			"two"),
		// A verify cell refuses a commit id under seven characters, so the short
		// value sits on a review row, whose cell status does not parse.
		listed("a-commit-value-shorter-than-four-characters-names-nothing",
			topic(committedAs("one", "src/one.txt"),
				rowOf(func(fx *contractFixture) string {
					return row("2026-11-01T11:00:00Z", "execute", "review", "task-1", "ok", "clean", "kind=quality commit="+fx.values["one"][:3], "execute go")
				})),
			"one"),
		// The range base..head is empty and the patch-id matches no commit, so
		// only integrated= can name the commit.
		listed("an-integrated-value-names-its-commit",
			topic(committedAs("one", "src/one.txt"), committedAs("two", "src/two.txt"),
				rowOf(func(fx *contractFixture) string {
					return verify("base=" + fx.values["one"] + " head=" + fx.values["one"] + " integrated=" + fx.values["two"] + " patch-id=" + pid40)
				})),
			"one"),
		listed("an-equal-patch-id-names-its-commit",
			topic(committedAs("one", "src/one.txt"), committedAs("two", "src/two.txt"),
				rowOf(func(fx *contractFixture) string {
					return verify("commit=0000000000000000000000000000000000000001 patch-id=" + patchIDOf(fx.repo, fx.values["one"]))
				})),
			"two"),
		// A no-commit cell quotes a failing command, so the pairs inside its
		// cmd= text name no commit, and the commit stays unaccounted.
		listed("a-no-commit-row-names-no-commit-its-quoted-commit-and-patch-id-name",
			topic(committedAs("one", "src/one.txt"),
				rowOf(func(fx *contractFixture) string {
					return red("no-commit exit=1 cmd=git log --grep commit=" + fx.values["one"] + " patch-id=" + patchIDOf(fx.repo, fx.values["one"]) + " out=no match")
				})),
			"one"),
		listed("a-no-commit-row-names-no-commit-its-quoted-integrated-value-names",
			topic(committedAs("one", "src/one.txt"),
				rowOf(func(fx *contractFixture) string {
					return red("no-commit exit=1 cmd=git log --grep integrated=" + fx.values["one"] + " out=no match")
				})),
			"one"),
		listed("a-no-commit-row-names-no-commit-of-its-quoted-range",
			topic(headAs("base"), committedAs("one", "src/one.txt"),
				rowOf(func(fx *contractFixture) string {
					return red("no-commit exit=1 cmd=git log --grep base=" + fx.values["base"] + " head=" + fx.values["one"] + " out=no match")
				})),
			"one"),
		listed("an-isolated-packet-row-names-every-commit-of-its-range-and-the-merge-is-never-listed",
			topic(committedAs("one", "src/one.txt"), headAs("base"),
				gitFixture("switch", "-q", "-c", "packet"),
				committedAs("p1", "src/p1.txt"), committedAs("p2", "src/p2.txt"),
				gitFixture("switch", "-q", "topic"),
				gitFixture("merge", "-q", "--no-ff", "-m", "integrate packet", "packet"), headAs("merge"),
				rowOf(func(fx *contractFixture) string {
					return verify("base=" + fx.values["base"] + " head=" + fx.values["p2"] + " integrated=" + fx.values["merge"] + " patch-id=" + pid40)
				})),
			"one"),
		listed("a-commit-that-changes-only-the-work-folder-is-never-listed",
			topic(committedAs("art", contractFolder+"/notes.md"), committedAs("one", "src/one.txt")),
			"one"),
		listed("an-empty-commit-is-never-listed",
			topic(gitFixture("commit", "-q", "--allow-empty", "-m", "retrigger"))),
		listed("an-empty-commit-is-left-out-beside-a-listed-commit",
			topic(gitFixture("commit", "-q", "--allow-empty", "-m", "retrigger"), committedAs("one", "src/one.txt")),
			"one"),
		listed("a-review-row-names-the-commit-it-covered",
			topic(committedAs("one", "src/one.txt"),
				rowOf(func(fx *contractFixture) string {
					return row("2026-11-01T11:00:00Z", "execute", "review", "task-1", "ok", "clean", "kind=quality commit="+fx.values["one"], "execute go")
				}))),
		{
			name:  "unaccounted/more-than-twenty-commits-count-all-and-list-the-first-twenty",
			setup: topic(many...),
			steps: []contractStep{{args: status, stdoutCount: []lineCount{{"unaccounted:", 1}}, check: unaccountedOf(21, firstTwenty...)}},
		},
		{
			name:  "unaccounted/a-commit-user-row-clears-a-listed-commit",
			setup: twoWork,
			steps: []contractStep{
				{args: status, check: unaccountedIs("one", "two")},
				{argsOf: func(fx *contractFixture) []string {
					return contractLog("execute", "commit", "user", "ok", "user commit", "auto:"+fx.values["one"], "execute go")
				}},
				{args: status, stdoutCount: []lineCount{{"unaccounted:", 1}}, check: unaccountedIs("two")},
			},
		},
		notComputed("a-branch-without-a-bind-row-is-not-computed", notBound,
			withFixtureSteps(onBranch("other", withSteps(newState(seeded, bound), commitStep)), committedAs("one", "src/one.txt"))),
		listed("a-bind-row-for-the-default-branch-prints-none",
			withFixtureSteps(withSteps(newState(seeded,
				row("2026-11-01T10:10:00Z", "execute", "bind", "main", "ok", "bound", "none", "execute go"),
				row("2026-11-01T10:20:00Z", "execute", "enter", "run", "ok", "start", "mode=solo branch=main", "execute go")), commitStep),
				committedAs("one", "src/one.txt"))),
		notComputed("a-detached-head-is-not-computed", notBound,
			topic(committedAs("one", "src/one.txt"), gitFixture("checkout", "-q", "--detach"))),
		notComputed("a-repository-without-a-default-branch-is-not-computed", "no default branch",
			topic(committedAs("one", "src/one.txt"), gitFixture("branch", "-q", "-m", "main", "trunk"))),
	}
}

// storageAfterFlow checks that the storage line directly follows the flow line.
func storageAfterFlow(want string) func(t *testing.T, fx *contractFixture, res result) {
	return func(t *testing.T, fx *contractFixture, res result) {
		t.Helper()
		lines := strings.Split(res.stdout, "\n")
		if len(lines) < 2 || lines[0] != "flow: "+contractFolder || lines[1] != want {
			t.Errorf("status starts %q, want the flow line and then %q", lines[:min(2, len(lines))], want)
		}
	}
}

// statusStartsWith checks that stdout starts with the given whole lines.
func statusStartsWith(want ...string) func(t *testing.T, fx *contractFixture, res result) {
	return func(t *testing.T, fx *contractFixture, res result) {
		t.Helper()
		if !strings.HasPrefix(res.stdout, strings.Join(want, "\n")+"\n") {
			t.Errorf("stdout %q, want it to start with %q", res.stdout, want)
		}
	}
}

// stdoutEmpty checks that a refused call printed nothing to stdout.
func stdoutEmpty(t *testing.T, fx *contractFixture, res result) {
	t.Helper()
	if res.stdout != "" {
		t.Errorf("stdout %q, want nothing", res.stdout)
	}
}

func settingCases() []contractCase {
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	seededCells := cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	bound := row("2026-11-01T10:10:00Z", "intent", "bind", "topic", "ok", "bound topic to this flow", "none", "spec go")
	boundCells := cellsRow("intent", "bind", "topic", "ok", "bound topic to this flow", "none", "spec go")
	status := []string{"status", "--folder", contractFolder}
	// committed and bound are flows committed on main, checked out on topic;
	// bound also binds topic, so that flow where finds the flow.
	committed := onBranch("topic", withSteps(newState(seeded), commitStep))
	boundFlow := onBranch("topic", withSteps(newState(seeded, bound), commitStep))
	configured := func(value string, inner func(t *testing.T) *contractFixture) func(t *testing.T) *contractFixture {
		return withSteps(inner, gitStep("config", "agentics.flowStorage", value))
	}
	var cases []contractCase
	for _, setting := range []struct {
		name  string
		setup func(t *testing.T) *contractFixture
		env   []string
	}{
		{"by-default", committed, nil},
		{"under-config-local", configured("local", committed), nil},
		{"under-environment-local", committed, []string{"FLOW_STORAGE=local"}},
	} {
		cases = append(cases, contractCase{
			name:  "setting/status-prints-committed-for-a-work-root-flow-" + setting.name,
			setup: setting.setup,
			steps: []contractStep{{
				args:        status,
				env:         setting.env,
				stdoutCount: []lineCount{{"storage:", 1}},
				check:       storageAfterFlow("storage: committed"),
			}},
		})
	}
	commands := []struct {
		name string
		args []string
	}{
		{"log", contractLog("intent", "challenge", "intent.md", "ok", "direction challenged", "none", "spec go")},
		{"status", status},
		{"where", []string{"where"}},
		{"bind", []string{"bind", "--folder", contractFolder, "--slice", "S1"}},
	}
	for _, source := range []struct {
		name, value string
		setup       func(t *testing.T) *contractFixture
		env         []string
	}{
		{"config", "Git config agentics.flowStorage", configured("shared", boundFlow), nil},
		{"environment", "FLOW_STORAGE", boundFlow, []string{"FLOW_STORAGE=shared"}},
	} {
		for _, command := range commands {
			cases = append(cases, contractCase{
				name:  "setting/an-invalid-" + source.name + "-value-stops-" + command.name,
				setup: source.setup,
				steps: []contractStep{{
					args: command.args,
					env:  source.env,
					code: 2,
					stderrLines: []string{
						`flow: storage setting "shared" from ` + source.value + " is invalid; use committed or local",
					},
					check: stdoutEmpty,
				}},
				rows: []string{seededCells, boundCells},
			})
		}
	}
	return cases
}

func TestResolveStorage(t *testing.T) {
	cases := []struct {
		name, config, env, want string
		invalid                 bool
	}{
		{name: "no-setting-is-committed", want: "committed"},
		{name: "the-environment-beats-the-default", env: "local", want: "local"},
		{name: "config-beats-the-environment", config: "local", env: "committed", want: "local"},
		{name: "config-committed-beats-environment-local", config: "committed", env: "local", want: "committed"},
		{name: "an-invalid-config-value-is-refused", config: "shared", env: "local", invalid: true},
		{name: "an-invalid-environment-value-is-refused", env: "Local", invalid: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := newRepo(t)
			if c.config != "" {
				git(t, repo, "config", "agentics.flowStorage", c.config)
			}
			env := map[string]string{"FLOW_STORAGE": c.env}
			got, err := newRepoView(repo, func(key string) string { return env[key] }).storage()
			if c.invalid {
				if err == nil || !strings.Contains(err.Error(), "committed or local") {
					t.Fatalf("storage() = %q, %v; want an error naming committed and local", got, err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("storage() = %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

// localState is a setup whose flow is in the local store of a fresh
// repository, with a state.tsv holding the given rows and the given files.
func localState(files map[string]string, rows ...string) func(t *testing.T) *contractFixture {
	return func(t *testing.T) *contractFixture {
		t.Helper()
		fx := emptyRepo(t)
		fx.local = true
		store := ".git/agentics/work/" + fx.folder + "/"
		body := headerRow + "\n"
		if len(rows) > 0 {
			body += strings.Join(rows, "\n") + "\n"
		}
		writeFile(t, fx.repo, store+"state.tsv", body)
		for name, content := range files {
			writeFile(t, fx.repo, store+name, content)
		}
		return fx
	}
}

// inLocalStore is a setup that runs inner and names the local store as the
// store of the flow, for a case whose flow a step creates there.
func inLocalStore(inner func(t *testing.T) *contractFixture) func(t *testing.T) *contractFixture {
	return func(t *testing.T) *contractFixture {
		t.Helper()
		fx := inner(t)
		fx.local = true
		return fx
	}
}

// localSetting is a setup that runs inner with the local setting in Git config.
func localSetting(inner func(t *testing.T) *contractFixture) func(t *testing.T) *contractFixture {
	return withSteps(inner, gitStep("config", "agentics.flowStorage", "local"))
}

// stateFileIs fails unless rel, a state.tsv below the fixture repository,
// holds exactly the rows, without their ts and session cells.
func stateFileIs(rel string, rows ...string) func(t *testing.T, fx *contractFixture) {
	return func(t *testing.T, fx *contractFixture) {
		t.Helper()
		var got []string
		for _, line := range dataLines(readLines(t, filepath.Join(fx.repo, filepath.FromSlash(rel)))) {
			got = append(got, strings.Join(cells(line)[2:], "\t"))
		}
		if strings.Join(got, "\n") != strings.Join(rows, "\n") {
			t.Errorf("%s rows:\n got %q\nwant %q", rel, got, rows)
		}
	}
}

// statusClean fails when git status lists anything in the directory dir
// returns, or in the fixture repository when dir is nil.
func statusClean(dir func(fx *contractFixture) string) func(t *testing.T, fx *contractFixture) {
	return func(t *testing.T, fx *contractFixture) {
		t.Helper()
		where := fx.repo
		if dir != nil {
			where = dir(fx)
		}
		if listed := gitOut(t, where, "status", "--short", "--untracked-files=all"); listed != "" {
			t.Errorf("git status --short in %s lists %q, want nothing", where, listed)
		}
	}
}

// checksOf runs each check in turn.
func checksOf(checks ...func(t *testing.T, fx *contractFixture)) func(t *testing.T, fx *contractFixture) {
	return func(t *testing.T, fx *contractFixture) {
		t.Helper()
		for _, check := range checks {
			check(t, fx)
		}
	}
}

func localCases() []contractCase {
	const plan = "# Plan\n\n## Tasks\n\nbody line\n"
	const other = "# Plan\n\n## Tasks\n\nanother body\n"
	planHash := sha256Hex(plan)
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	seededCells := cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	create := []string{"intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"}
	logInto := func(folder string) []string { return append(append([]string{"log"}, create...), "--folder", folder) }
	bindCells := cellsRow("intent", "bind", "topic", "ok", "bound topic to this flow", "none", "spec go")
	status := []string{"status", "--folder", contractFolder}
	bind := []string{"bind", "--folder", contractFolder}
	approve := contractLog("execute", "approve", "plan.md", "ok", contractWhy, "auto", contractNext, "--expect-blob", planHash)
	approvedCells := cellsRow("execute", "approve", "plan.md", "ok", contractWhy, "text="+planHash, contractNext)
	planLine := func(fx *contractFixture) []string {
		return []string{"approve\t" + contractFolder + "\tplan.md\t" + planHash}
	}
	localRel := ".git/agentics/work/" + contractFolder + "/state.tsv"
	workRel := contractFolder + "/state.tsv"
	absolute := func(command string, rest ...string) func(fx *contractFixture) []string {
		return func(fx *contractFixture) []string {
			return append(append([]string{command}, rest...), "--folder", filepath.Join(fx.repo, filepath.FromSlash(contractFolder)))
		}
	}
	worktree := func(fx *contractFixture) string { return fx.values["worktree"] }
	type setupFunc = func(t *testing.T) *contractFixture
	settings := []struct {
		name  string
		setup func(inner setupFunc) setupFunc
	}{
		{"under-committed", func(inner setupFunc) setupFunc { return inner }},
		{"under-local", localSetting},
	}
	cases := []contractCase{
		{
			name:  "local/log-writes-the-local-store-and-stages-nothing",
			setup: inLocalStore(localSetting(emptyRepo)),
			steps: []contractStep{
				{args: contractLog(create...), check: quiet},
				{args: status, stdoutCount: []lineCount{{"storage:", 1}}, check: storageAfterFlow("storage: local")},
			},
			rows:  []string{seededCells},
			check: checksOf(nothingUnder("docs"), statusClean(nil)),
		},
		{
			name:  "local/log-from-a-subdirectory-writes-the-same-local-store",
			setup: inLocalStore(localSetting(emptyRepo)),
			steps: []contractStep{
				{dir: "sub", args: contractLog(create...)},
				{dir: "sub", args: status, check: storageAfterFlow("storage: local")},
			},
			rows:  []string{seededCells},
			check: checksOf(nothingUnder("docs"), nothingUnder("sub/docs"), statusClean(nil)),
		},
		{
			name:  "local/the-environment-setting-writes-the-local-store",
			setup: inLocalStore(emptyRepo),
			steps: []contractStep{{args: contractLog(create...), env: []string{"FLOW_STORAGE=local"}}},
			rows:  []string{seededCells},
			check: nothingUnder("docs"),
		},
		{
			name:  "local/the-committed-setting-writes-the-work-root",
			setup: withSteps(emptyRepo, gitStep("config", "agentics.flowStorage", "committed")),
			steps: []contractStep{{args: contractLog(create...), env: []string{"FLOW_STORAGE=local"}}},
			rows:  []string{seededCells},
			check: nothingUnder(".git/agentics/work"),
		},
		{
			name:  "local/a-work-root-flow-is-found-under-local",
			setup: localSetting(onBranch("topic", newState(seeded))),
			steps: []contractStep{
				{args: bind, check: quiet},
				{args: status, check: storageAfterFlow("storage: committed")},
			},
			rows:  []string{seededCells, bindCells},
			check: nothingUnder(".git/agentics/work"),
		},
	}
	for _, setting := range settings {
		cases = append(cases,
			contractCase{
				name:  "local/status-finds-a-local-flow-" + setting.name,
				setup: setting.setup(localState(nil, seeded)),
				steps: []contractStep{{
					args:        status,
					stdoutLines: []string{"phase: intent (create, ok)"},
					stdoutCount: []lineCount{{"storage:", 1}},
					check:       storageAfterFlow("storage: local"),
				}},
			},
			contractCase{
				name:  "local/bind-finds-a-local-flow-" + setting.name,
				setup: setting.setup(onBranch("topic", localState(nil, seeded))),
				steps: []contractStep{{args: bind, check: quiet}},
				rows:  []string{seededCells, bindCells},
				check: nothingUnder("docs"),
			},
			contractCase{
				name:  "local/an-absolute-folder-is-used-as-given-" + setting.name,
				setup: setting.setup(localState(nil, seeded)),
				steps: []contractStep{
					{argsOf: absolute("log", create...)},
					{argsOf: absolute("status"), stdoutCount: []lineCount{{"storage:", 1}}, check: storageAfterFlow("storage: committed")},
				},
				rows:  []string{seededCells},
				check: stateFileIs(workRel, seededCells),
			},
			contractCase{
				name:  "local/a-folder-outside-the-shape-is-used-as-given-" + setting.name,
				setup: setting.setup(emptyRepo),
				steps: []contractStep{
					{args: logInto("notes/flow")},
					{args: logInto("docs/specs/2026-1/x")},
					{args: logInto("docs/specs/2026-11/x/deeper")},
					{args: logInto("docs/specs/unassigned/x")},
				},
				check: checksOf(
					stateFileIs("notes/flow/state.tsv", seededCells),
					stateFileIs("docs/specs/2026-1/x/state.tsv", seededCells),
					stateFileIs("docs/specs/2026-11/x/deeper/state.tsv", seededCells),
					stateFileIs("docs/specs/unassigned/x/state.tsv", seededCells),
					nothingUnder(".git/agentics/work"),
				),
			},
		)
	}
	cases = append(cases,
		contractCase{
			name:  "local/the-unassigned-folder-maps-to-the-local-store",
			setup: localSetting(emptyRepo),
			steps: []contractStep{{args: logInto("docs/specs/unassigned")}},
			check: checksOf(stateFileIs(".git/agentics/work/docs/specs/unassigned/state.tsv", seededCells), nothingUnder("docs")),
		},
		contractCase{
			name:  "local/the-shape-follows-the-work-root-in-force",
			setup: localSetting(emptyRepo),
			steps: []contractStep{
				{args: logInto("plans/2026-11/x"), env: []string{"FLOW_WORK_ROOT=plans"}},
				{args: logInto("docs/specs/2026-11/x"), env: []string{"FLOW_WORK_ROOT=plans"}},
			},
			check: checksOf(
				stateFileIs(".git/agentics/work/plans/2026-11/x/state.tsv", seededCells),
				stateFileIs("docs/specs/2026-11/x/state.tsv", seededCells),
				nothingUnder("plans"),
				nothingUnder(".git/agentics/work/docs"),
			),
		},
		contractCase{
			name: "local/an-approval-hashes-the-artifact-in-the-local-store",
			setup: localSetting(withSteps(localState(map[string]string{"plan.md": plan}, seeded),
				func(t *testing.T, repo string) { writeFile(t, repo, contractFolder+"/plan.md", other) })),
			steps: []contractStep{
				{
					args:      contractLog("execute", "approve", "plan.md", "ok", contractWhy, "auto", contractNext, "--expect-blob", sha256Hex(other)),
					code:      2,
					stderrHas: []string{contractFolder + "/plan.md current text=" + planHash + " does not match --expect-blob"},
					check:     stdoutEmpty,
				},
				{args: approve, check: quiet},
				{args: status, stdoutLines: []string{"hashes: plan.md text=" + planHash}, check: planReads("current", contractFolder)},
			},
			rows:  []string{seededCells, approvedCells},
			check: ledgerAre(planLine),
		},
		contractCase{
			name: "local/a-blob-approval-hashes-the-artifact-in-the-local-store",
			setup: func(t *testing.T) *contractFixture {
				t.Helper()
				fx := localState(map[string]string{"plan.md": plan})(t)
				writeFile(t, fx.repo, contractFolder+"/plan.md", other)
				blob := contentHashAt(fx.repo, localStoreOf(fx.repo, contractFolder), filepath.Join(fx.repo, filepath.FromSlash(contractFolder)), "plan.md")
				if blob == "" || blob == firstRuleHash(fx.repo, contractFolder, "plan.md") {
					t.Fatalf("blob hash %q does not tell the two plans apart", blob)
				}
				writeFile(t, fx.repo, localRel, headerRow+"\n"+
					row("2026-11-01T11:00:00Z", "decompose", "approve", "plan.md", "ok", "approved", "blob="+blob, "execute go")+"\n")
				return fx
			},
			steps: []contractStep{{args: status, check: planReads("unconfirmed here", contractFolder)}},
		},
	)
	for _, command := range []struct {
		name string
		args []string
	}{
		{"log", contractLog("intent", "challenge", "intent.md", "ok", "direction challenged", "none", "spec go")},
		{"status", status},
		{"bind", bind},
	} {
		cases = append(cases, contractCase{
			name: "local/a-folder-in-both-stores-stops-" + command.name,
			setup: localSetting(onBranch("topic", withSteps(newState(seeded),
				stateStep(".git/agentics/work/"+contractFolder, seeded)))),
			steps: []contractStep{{
				args:      command.args,
				code:      4,
				stderrHas: []string{"flow: " + contractFolder + " is held by both stores, committed at ", " and local at "},
				check:     stdoutEmpty,
			}},
			check: checksOf(stateFileIs(workRel, seededCells), stateFileIs(localRel, seededCells)),
		})
	}
	cases = append(cases, contractCase{
		name: "local/a-linked-worktree-shares-the-local-store-of-the-main-checkout",
		setup: func(t *testing.T) *contractFixture {
			t.Helper()
			fx := inLocalStore(localSetting(emptyRepo))(t)
			writeFile(t, fx.repo, ".git/agentics/work/"+contractFolder+"/plan.md", plan)
			parent, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			fx.values["worktree"] = filepath.Join(parent, "worktree")
			git(t, fx.repo, "worktree", "add", "-q", "-b", "topic", fx.values["worktree"])
			writeFile(t, worktree(fx), "src/one.txt", "one\n")
			git(t, worktree(fx), "add", "--", "src/one.txt")
			git(t, worktree(fx), "commit", "-q", "-m", "one")
			fx.values["one"] = gitOut(t, worktree(fx), "rev-parse", "HEAD")
			return fx
		},
		steps: []contractStep{
			{dirOf: worktree, args: contractLog(create...)},
			{dirOf: worktree, args: approve, check: quiet},
			{
				// The row names a commit that only the worktree's branch holds, and
				// no patch-id, so only ancestry from the worktree's HEAD accepts it.
				before: func(t *testing.T, fx *contractFixture) {
					t.Helper()
					body, err := os.ReadFile(fx.statePath())
					if err != nil {
						t.Fatal(err)
					}
					writeFile(t, fx.repo, localRel, string(body)+
						row("2026-11-01T12:00:00Z", "execute", "verify", "wave-1", "ok", "verified", "commit="+fx.values["one"], "execute go")+"\n")
				},
				dirOf:       worktree,
				args:        status,
				stdoutCount: []lineCount{{"mismatch:", 0}},
				check: func(t *testing.T, fx *contractFixture, res result) {
					t.Helper()
					storageAfterFlow("storage: local")(t, fx, res)
					planReads("current", contractFolder)(t, fx, res)
				},
			},
		},
		check: checksOf(
			ledgerAre(planLine),
			nothingUnder("docs"),
			func(t *testing.T, fx *contractFixture) {
				t.Helper()
				if _, err := os.Lstat(filepath.Join(worktree(fx), "docs")); !os.IsNotExist(err) {
					t.Errorf("the worktree has a docs folder (err %v); the flow belongs in the local store", err)
				}
				rows := contractRows(t, fx)
				if len(rows) != 3 || rows[0] != seededCells || rows[1] != approvedCells {
					t.Errorf("local rows %q, want the create, the approval and the verify row", rows)
				}
			},
			statusClean(nil),
			statusClean(worktree),
		),
	})
	return append(cases, storeFolderCases()...)
}

// refusedInStore fails unless the step printed only the refusal of the folder
// given returns, with the advice to pass logical instead, or to pass the
// logical folder when logical is empty.
func refusedInStore(given func(fx *contractFixture) string, logical string) func(t *testing.T, fx *contractFixture, res result) {
	return func(t *testing.T, fx *contractFixture, res result) {
		t.Helper()
		advice := "pass --folder " + logical
		if logical == "" {
			advice = "pass the logical folder with --folder"
		}
		want := "flow: invalid folder: '" + given(fx) + "' -- this is inside the clone-local store; " + advice + "\n"
		if res.stderr != want || res.stdout != "" {
			t.Errorf("want only %q on stderr, got stdout %q stderr %q", want, res.stdout, res.stderr)
		}
	}
}

// storeFolderCases cover a --folder that names a directory inside the local
// store, such as the one flow path prints for a local flow.
func storeFolderCases() []contractCase {
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	seededCells := cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	localRel := ".git/agentics/work/" + contractFolder + "/state.tsv"
	printed := func(fx *contractFixture) string { return fx.values["printed"] }
	storeDir := func(fx *contractFixture) string { return localStoreOf(fx.repo, contractFolder) }
	worktree := func(fx *contractFixture) string { return fx.values["worktree"] }
	// pathStep runs flow path and keeps the directory it printed for the steps after it.
	pathStep := func(dirOf func(fx *contractFixture) string) contractStep {
		return contractStep{dirOf: dirOf, args: []string{"path", "--folder", contractFolder}, check: func(t *testing.T, fx *contractFixture, res result) {
			t.Helper()
			fx.values["printed"] = strings.TrimSuffix(res.stdout, "\n")
		}}
	}
	withFolder := func(folder func(fx *contractFixture) string, args ...string) func(fx *contractFixture) []string {
		return func(fx *contractFixture) []string { return append(append([]string{}, args...), "--folder", folder(fx)) }
	}
	linked := func(t *testing.T) *contractFixture {
		t.Helper()
		fx := localSetting(localState(nil, seeded))(t)
		parent, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		fx.values["worktree"] = filepath.Join(parent, "worktree")
		git(t, fx.repo, "worktree", "add", "-q", "-b", "topic", fx.values["worktree"])
		return fx
	}
	unchanged := checksOf(stateFileIs(localRel, seededCells), nothingUnder("docs"), statusClean(nil))
	cases := []contractCase{
		{
			name:  "local/status-refuses-the-directory-flow-path-printed",
			setup: localSetting(localState(nil, seeded)),
			steps: []contractStep{
				pathStep(nil),
				{argsOf: withFolder(printed, "status"), code: 2, check: refusedInStore(printed, contractFolder)},
			},
			check: unchanged,
		},
		{
			name:  "local/status-in-a-linked-worktree-refuses-the-directory-flow-path-printed",
			setup: linked,
			steps: []contractStep{
				pathStep(worktree),
				{dirOf: worktree, argsOf: withFolder(printed, "status"), code: 2, check: refusedInStore(printed, contractFolder)},
			},
			check: checksOf(unchanged, statusClean(worktree)),
		},
		{
			name:  "local/log-into-the-local-store-directory-writes-nothing",
			setup: localSetting(localState(nil, seeded)),
			steps: []contractStep{{
				argsOf: withFolder(storeDir, "log", "intent", "challenge", "intent.md", "ok", "direction challenged", "none", "spec go"),
				code:   2,
				check:  refusedInStore(storeDir, contractFolder),
			}},
			check: unchanged,
		},
		{
			name:  "local/log-in-a-linked-worktree-into-the-local-store-directory-writes-nothing",
			setup: linked,
			steps: []contractStep{{
				dirOf:  worktree,
				argsOf: withFolder(storeDir, "log", "intent", "challenge", "intent.md", "ok", "direction challenged", "none", "spec go"),
				code:   2,
				check:  refusedInStore(storeDir, contractFolder),
			}},
			check: checksOf(unchanged, statusClean(worktree)),
		},
	}
	relative := ".git/agentics/work/" + contractFolder
	for _, command := range []string{"log", "status", "bind", "path"} {
		args := []string{command}
		if command == "log" {
			args = append(args, "intent", "challenge", "intent.md", "ok", "direction challenged", "none", "spec go")
		}
		cases = append(cases,
			contractCase{
				name:  "local/" + command + "-refuses-an-absolute-folder-in-the-local-store",
				setup: localSetting(onBranch("topic", localState(nil, seeded))),
				steps: []contractStep{{argsOf: withFolder(storeDir, args...), code: 2, check: refusedInStore(storeDir, contractFolder)}},
				check: unchanged,
			},
			contractCase{
				name:  "local/" + command + "-refuses-a-relative-folder-in-the-local-store",
				setup: localSetting(onBranch("topic", localState(nil, seeded))),
				steps: []contractStep{{
					dir:   "sub",
					args:  append(append([]string{}, args...), "--folder", relative),
					code:  2,
					check: refusedInStore(func(*contractFixture) string { return relative }, contractFolder),
				}},
				check: checksOf(unchanged, nothingUnder("sub/.git")),
			},
			contractCase{
				name:  "local/" + command + "-refuses-the-local-store-itself",
				setup: localSetting(onBranch("topic", localState(nil, seeded))),
				steps: []contractStep{{
					args:  append(append([]string{}, args...), "--folder", ".git/agentics/work"),
					code:  2,
					check: refusedInStore(func(*contractFixture) string { return ".git/agentics/work" }, ""),
				}},
				check: unchanged,
			},
		)
	}
	return cases
}

func localunassignedCases() []contractCase {
	const unassigned = "docs/specs/unassigned"
	const localUnassigned = ".git/agentics/work/" + unassigned
	const plan = "# Plan\n\n## Tasks\n\nbody line\n"
	const edited = plan + "more\n"
	release := row("2026-11-01T10:00:00Z", "gate", "release", "v1", "ok", "tagged", "v1", "done")
	releaseCells := cellsRow("gate", "release", "v1", "ok", "tagged", "v1", "done")
	skip := row("2026-11-01T11:00:00Z", "execute", "skip", "chain fix/q", "ok", "fix", "none", "execute go")
	pivot := row("2026-11-01T12:00:00Z", "execute", "pivot", "fix fix/q", "ok", "moved into the chain", "none", "intent go")
	skipCells := []string{"execute", "skip", "chain fix/q", "ok", "fix", "none", "execute go"}
	released := []string{"gate", "release", "v2", "ok", "tagged", "v2", "done"}
	logArgs := func(cells []string, rest ...string) []string {
		return append(append([]string{"log"}, cells...), rest...)
	}
	smallFix := "flow: small fix in progress on fix/q; no work folder"
	committedUnassigned := func(rows ...string) func(t *testing.T) *contractFixture {
		return onBranch("fix/q", withSteps(emptyRepo, stateStep(unassigned, rows...), commitStep))
	}
	cases := []contractCase{
		{
			name:  "localunassigned/a-small-fix-row-goes-to-the-local-file-while-a-committed-one-exists",
			setup: localSetting(committedUnassigned(release)),
			steps: []contractStep{{args: logArgs(skipCells, "--folder", unassigned), check: quiet}},
			check: checksOf(
				stateFileIs(unassigned+"/state.tsv", releaseCells),
				stateFileIs(localUnassigned+"/state.tsv", strings.Join(skipCells, "	")),
				statusClean(nil),
			),
		},
		{
			name:  "localunassigned/a-small-fix-row-in-both-stores-raises-no-conflict",
			setup: localSetting(withSteps(committedUnassigned(skip), stateStep(localUnassigned, release))),
			steps: []contractStep{
				{args: logArgs(released, "--folder", unassigned), check: quiet},
				// With no --folder the open small fix names the folder, and the
				// row still goes to the store the setting names.
				{args: logArgs(released), check: quiet},
			},
			check: checksOf(
				stateFileIs(unassigned+"/state.tsv", strings.Join(skipCells, "	")),
				stateFileIs(localUnassigned+"/state.tsv", releaseCells, strings.Join(released, "	"), strings.Join(released, "	")),
				statusClean(nil),
			),
		},
		{
			name:  "localunassigned/a-small-fix-row-under-committed-leaves-the-local-file",
			setup: withSteps(committedUnassigned(release), stateStep(localUnassigned, release)),
			steps: []contractStep{{args: logArgs(skipCells, "--folder", unassigned), check: quiet}},
			check: checksOf(
				stateFileIs(unassigned+"/state.tsv", releaseCells, strings.Join(skipCells, "	")),
				stateFileIs(localUnassigned+"/state.tsv", releaseCells),
			),
		},
		{
			name: "localunassigned/a-stale-approval-in-the-local-store-points-at-no-commit",
			setup: localState(map[string]string{"plan.md": edited},
				row("2026-11-01T11:00:00Z", "decompose", "approve", "plan.md", "ok", "approved", "text="+sha256Hex(plan), "execute go")),
			steps: []contractStep{{
				args: []string{"status", "--folder", contractFolder},
				stdoutLines: []string{
					"approvals: plan.md approved 2026-11-01 stale",
					"stale: " + contractFolder + "/plan.md differs from the content approved 2026-11-01 (approved text=" + sha256Hex(plan) +
						", current text=" + sha256Hex(edited) + "). the work folder is local to this clone; no commit holds it. " + planNext,
				},
				stdoutCount: []lineCount{{"stale:", 1}},
			}},
		},
	}
	cases = append(cases,
		contractCase{
			name:  "localunassigned/path-names-the-work-root-folder-of-a-small-fix-under-committed",
			setup: committedUnassigned(skip),
			steps: []contractStep{{
				args:  []string{"path"},
				check: printsOnly(func(fx *contractFixture) string { return filepath.Join(fx.repo, filepath.FromSlash(unassigned)) }),
			}},
			check: nothingUnder(".git/agentics/work"),
		},
		contractCase{
			name:  "localunassigned/path-names-the-local-store-folder-of-a-small-fix-under-local",
			setup: localSetting(onBranch("fix/q", withSteps(emptyRepo, stateStep(localUnassigned, skip)))),
			steps: []contractStep{{
				args:  []string{"path"},
				check: printsOnly(func(fx *contractFixture) string { return localStoreOf(fx.repo, unassigned) }),
			}},
			check: nothingUnder("docs"),
		},
	)
	type setupFunc = func(t *testing.T) *contractFixture
	for _, setting := range []struct {
		name  string
		setup func(inner setupFunc) setupFunc
	}{
		{"under-committed", func(inner setupFunc) setupFunc { return inner }},
		{"under-local", localSetting},
	} {
		cases = append(cases,
			contractCase{
				name:  "localunassigned/a-skip-row-in-the-committed-file-is-a-small-fix-" + setting.name,
				setup: setting.setup(committedUnassigned(skip)),
				steps: []contractStep{{args: []string{"where"}, code: 5, stderrLines: []string{smallFix}, check: stdoutEmpty}},
			},
			contractCase{
				name:  "localunassigned/a-skip-row-in-the-local-file-is-a-small-fix-" + setting.name,
				setup: setting.setup(withSteps(committedUnassigned(release), stateStep(localUnassigned, skip))),
				steps: []contractStep{{args: []string{"where"}, code: 5, stderrLines: []string{smallFix}, check: stdoutEmpty}},
			},
			contractCase{
				name:  "localunassigned/a-pivot-row-in-the-other-file-ends-the-small-fix-" + setting.name,
				setup: setting.setup(withSteps(committedUnassigned(skip), stateStep(localUnassigned, pivot))),
				steps: []contractStep{{args: []string{"where"}, code: 3, stderrLines: []string{"flow: no flow found"}, check: stdoutEmpty}},
			},
			contractCase{
				// The committed file is read first, so only the time order puts
				// the local skip before the committed pivot.
				name:  "localunassigned/a-committed-pivot-after-a-local-skip-ends-the-small-fix-" + setting.name,
				setup: setting.setup(withSteps(committedUnassigned(pivot), stateStep(localUnassigned, skip))),
				steps: []contractStep{{args: []string{"where"}, code: 3, stderrLines: []string{"flow: no flow found"}, check: stdoutEmpty}},
			},
			contractCase{
				name:  "localunassigned/status-reads-the-rows-of-both-files-in-time-order-" + setting.name,
				setup: setting.setup(withSteps(committedUnassigned(release, pivot), stateStep(localUnassigned, skip))),
				steps: []contractStep{{
					args:        []string{"status", "--folder", unassigned},
					stdoutLines: []string{"phase: execute (pivot, ok)", "last: 2026-11-01T12:00:00Z s1"},
					stdoutCount: []lineCount{{"storage:", 1}},
					check:       statusStartsWith("flow: "+unassigned, "storage: local"),
				}},
			},
			contractCase{
				name:  "localunassigned/status-reads-the-committed-file-when-no-local-file-exists-" + setting.name,
				setup: setting.setup(committedUnassigned(release, skip)),
				steps: []contractStep{{
					args:        []string{"status", "--folder", unassigned},
					stdoutLines: []string{"phase: execute (skip, ok)", "last: 2026-11-01T11:00:00Z s1"},
					check:       statusStartsWith("flow: "+unassigned, "storage: committed"),
				}},
				check: nothingUnder(localUnassigned),
			},
		)
	}
	return cases
}

func locallookupCases() []contractCase {
	const plan = "# Plan\n\n## Tasks\n\nbody line\n"
	planHash := sha256Hex(plan)
	localOther := ".git/agentics/work/" + lookupOther
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	bound := row("2026-11-01T10:30:00Z", "intent", "bind", "topic", "ok", "bound", "none", "spec go")
	approved := row("2026-11-01T11:00:00Z", "decompose", "approve", "plan.md", "ok", "approved", "text="+planHash, "execute go")
	done := row("2026-11-01T12:00:00Z", "gate", "done", "branch", "ok", "merged", "none", "done nothing left")
	create := []string{"intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"}
	challenge := []string{"intent", "challenge", "intent.md", "ok", "direction challenged", "none", "spec go"}
	approve := []string{"execute", "approve", "plan.md", "ok", contractWhy, "auto", contractNext, "--expect-blob", planHash}
	finish := []string{"gate", "done", "branch", "ok", "merged", "none", "done nothing left"}
	logHere := func(cells ...string) []string { return append([]string{"log"}, cells...) }
	where := []string{"where"}
	status := []string{"status"}
	found := contractStep{args: where, stdoutLines: []string{contractFolder}, check: func(t *testing.T, _ *contractFixture, res result) {
		t.Helper()
		if res.stdout != contractFolder+"\n" || res.stderr != "" {
			t.Errorf("want only %s on stdout, got %+v", contractFolder, res)
		}
	}}
	notFound := contractStep{args: where, code: 3, stderrLines: []string{"flow: no flow found"}, check: stdoutEmpty}
	seededCells := cellsRow("intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	cases := []contractCase{
		{
			name:  "locallookup/a-bound-local-flow-is-read-and-written-without-a-folder",
			setup: onBranch("topic", localState(nil, seeded, bound)),
			steps: []contractStep{
				found,
				{args: status, stdoutCount: []lineCount{{"storage:", 1}}, check: storageAfterFlow("storage: local")},
				{args: logHere(challenge...), check: quiet},
			},
			rows: []string{
				seededCells,
				cellsRow("intent", "bind", "topic", "ok", "bound", "none", "spec go"),
				strings.Join(challenge, "\t"),
			},
			check: checksOf(nothingUnder("docs"), statusClean(nil)),
		},
		{
			name:  "locallookup/the-default-branch-finds-the-one-unbound-local-flow",
			setup: localState(nil, seeded),
			steps: []contractStep{
				found,
				{args: status, check: storageAfterFlow("storage: local")},
				{args: logHere(challenge...), check: quiet},
			},
			rows:  []string{seededCells, strings.Join(challenge, "\t")},
			check: checksOf(nothingUnder("docs"), statusClean(nil)),
		},
		{
			name:  "locallookup/two-unbound-local-flows-exit-3",
			setup: withSteps(localState(nil, seeded), stateStep(localOther, seeded)),
			steps: []contractStep{notFound},
		},
		{
			name:  "locallookup/an-approved-unbound-local-flow-exits-3",
			setup: localState(nil, seeded, approved),
			steps: []contractStep{notFound},
		},
		{
			name:  "locallookup/a-local-flow-bound-to-another-branch-is-not-the-unbound-one",
			setup: localState(nil, seeded, bound),
			steps: []contractStep{notFound},
		},
		{
			name:  "locallookup/a-closed-local-flow-leaves-the-one-open-flow-unbound",
			setup: withSteps(localState(nil, seeded), stateStep(localOther, seeded, done)),
			steps: []contractStep{found},
		},
		{
			name:  "locallookup/a-done-local-flow-is-not-found-by-its-bind-row",
			setup: onBranch("topic", localState(nil, seeded, bound, done)),
			steps: []contractStep{notFound},
		},
	}
	for _, command := range [][]string{where, status, {"path"}, logHere(challenge...)} {
		cases = append(cases, contractCase{
			name: "locallookup/a-flow-in-both-stores-is-reported-once-by-" + command[0],
			setup: onBranch("topic", withSteps(localState(nil, seeded, bound),
				stateStep(contractFolder, seeded, bound))),
			steps: []contractStep{{
				args:        command,
				code:        4,
				stderrHas:   []string{"flow: " + contractFolder + " is held by both stores, committed at ", " and local at "},
				stderrCount: []lineCount{{"flow: more than one flow matches", 0}, {"  " + contractFolder, 0}},
				check:       stdoutEmpty,
			}},
			rows:  []string{seededCells, cellsRow("intent", "bind", "topic", "ok", "bound", "none", "spec go")},
			check: stateFileIs(contractFolder+"/state.tsv", seededCells, cellsRow("intent", "bind", "topic", "ok", "bound", "none", "spec go")),
		})
	}
	type setupFunc = func(t *testing.T) *contractFixture
	for _, setting := range []struct {
		name, storage, planAt string
		setup                 func(inner setupFunc) setupFunc
		check                 func(t *testing.T, fx *contractFixture)
	}{
		{"under-committed", "storage: committed", contractFolder, func(inner setupFunc) setupFunc { return inner }, nothingUnder(".git/agentics/work")},
		{"under-local", "storage: local", ".git/agentics/work/" + contractFolder, func(inner setupFunc) setupFunc { return inLocalStore(localSetting(inner)) },
			checksOf(nothingUnder("docs"), statusClean(nil))},
	} {
		cases = append(cases, contractCase{
			name: "locallookup/a-full-flow-" + setting.name,
			setup: setting.setup(onBranch("topic", withSteps(emptyRepo, func(t *testing.T, repo string) {
				t.Helper()
				writeFile(t, repo, setting.planAt+"/plan.md", plan)
			}))),
			steps: []contractStep{
				{args: contractLog(create...), check: quiet},
				{args: []string{"bind", "--folder", contractFolder}, check: quiet},
				{args: status, check: storageAfterFlow(setting.storage)},
				{args: logHere(approve...), check: quiet},
				{args: logHere(finish...), check: quiet},
				notFound,
			},
			rows: []string{
				seededCells,
				cellsRow("intent", "bind", "topic", "ok", "bound topic to this flow", "none", "spec go"),
				cellsRow("execute", "approve", "plan.md", "ok", contractWhy, "text="+planHash, contractNext),
				strings.Join(finish, "\t"),
			},
			check: setting.check,
		})
	}
	return cases
}

// printsOnly fails unless the step printed exactly the directory want returns
// and nothing on stderr.
func printsOnly(want func(fx *contractFixture) string) func(t *testing.T, fx *contractFixture, res result) {
	return func(t *testing.T, fx *contractFixture, res result) {
		t.Helper()
		if res.stdout != want(fx)+"\n" || res.stderr != "" {
			t.Errorf("want only %q on stdout, got stdout %q stderr %q", want(fx), res.stdout, res.stderr)
		}
	}
}

func pathCases() []contractCase {
	const fresh = "docs/specs/2026-11/x"
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	bound := row("2026-11-01T10:30:00Z", "intent", "bind", "topic", "ok", "bound", "none", "spec go")
	inWorkRoot := func(folder string) func(fx *contractFixture) string {
		return func(fx *contractFixture) string { return filepath.Join(fx.repo, filepath.FromSlash(folder)) }
	}
	inLocal := func(folder string) func(fx *contractFixture) string {
		return func(fx *contractFixture) string { return localStoreOf(fx.repo, folder) }
	}
	createsNothing := checksOf(nothingUnder("docs"), nothingUnder(".git/agentics/work"), statusClean(nil))
	usageStep := func(args ...string) contractStep {
		return contractStep{args: append([]string{"path"}, args...), code: 1, stderrHas: []string{"flow path [--folder <dir>]"}, check: stdoutEmpty}
	}
	return []contractCase{
		{
			name:  "path/a-new-folder-prints-the-work-root-directory-under-committed",
			setup: emptyRepo,
			steps: []contractStep{
				{args: []string{"path", "--folder", fresh}, check: printsOnly(inWorkRoot(fresh))},
				{dir: "sub", args: []string{"path", "--folder", fresh}, check: printsOnly(inWorkRoot(fresh))},
			},
			check: checksOf(createsNothing, nothingUnder("sub/docs")),
		},
		{
			name:  "path/a-new-folder-prints-the-local-store-directory-under-local",
			setup: localSetting(emptyRepo),
			steps: []contractStep{
				{args: []string{"path", "--folder", fresh}, check: printsOnly(inLocal(fresh))},
				{dir: "sub", args: []string{"path", "--folder", fresh}, check: printsOnly(inLocal(fresh))},
			},
			check: checksOf(createsNothing, nothingUnder("sub/docs")),
		},
		{
			name:  "path/the-environment-setting-prints-the-local-store-directory",
			setup: emptyRepo,
			steps: []contractStep{{args: []string{"path", "--folder", fresh}, env: []string{"FLOW_STORAGE=local"}, check: printsOnly(inLocal(fresh))}},
			check: createsNothing,
		},
		{
			name:  "path/a-local-flow-is-found-under-committed",
			setup: localState(nil, seeded),
			steps: []contractStep{{args: []string{"path", "--folder", contractFolder}, check: printsOnly(inLocal(contractFolder))}},
			check: nothingUnder("docs"),
		},
		{
			name:  "path/a-work-root-flow-is-found-under-local",
			setup: localSetting(newState(seeded)),
			steps: []contractStep{{args: []string{"path", "--folder", contractFolder}, check: printsOnly(inWorkRoot(contractFolder))}},
			check: nothingUnder(".git/agentics/work"),
		},
		{
			name:  "path/an-invalid-setting-exits-2",
			setup: emptyRepo,
			steps: []contractStep{{args: []string{"path", "--folder", fresh}, env: []string{"FLOW_STORAGE=shared"}, code: 2, stderrHas: []string{"FLOW_STORAGE"}, check: stdoutEmpty}},
			check: createsNothing,
		},
		{
			name:  "path/a-folder-in-both-stores-exits-4",
			setup: withSteps(newState(seeded), stateStep(".git/agentics/work/"+contractFolder, seeded)),
			steps: []contractStep{{
				args:      []string{"path", "--folder", contractFolder},
				code:      4,
				stderrHas: []string{"flow: " + contractFolder + " is held by both stores"},
				check:     stdoutEmpty,
			}},
		},
		{
			name:  "path/no-option-prints-the-directory-of-the-bound-local-flow",
			setup: onBranch("topic", localState(nil, seeded, bound)),
			steps: []contractStep{{args: []string{"path"}, check: printsOnly(inLocal(contractFolder))}},
			check: checksOf(nothingUnder("docs"), statusClean(nil)),
		},
		{
			name:  "path/no-option-prints-the-directory-of-the-bound-work-root-flow",
			setup: localSetting(onBranch("topic", newState(seeded, bound))),
			steps: []contractStep{{args: []string{"path"}, check: printsOnly(inWorkRoot(contractFolder))}},
			check: nothingUnder(".git/agentics/work"),
		},
		{
			name:  "path/no-option-and-no-flow-exits-3",
			setup: onBranch("topic", emptyRepo),
			steps: []contractStep{{args: []string{"path"}, code: 3, stderrLines: []string{"flow: no flow found; ask which flow, or pass --folder"}, check: stdoutEmpty}},
			check: createsNothing,
		},
		{
			name:  "path/no-option-and-two-bound-flows-exits-4",
			setup: onBranch("topic", withSteps(newState(seeded, bound), stateStep(lookupOther, seeded, bound))),
			steps: []contractStep{{
				args:        []string{"path"},
				code:        4,
				stderrLines: []string{"flow: more than one flow matches; ask which one:", "  " + contractFolder, "  " + lookupOther},
				check:       stdoutEmpty,
			}},
		},
		{
			name:  "path/an-unknown-option-is-a-usage-error",
			setup: emptyRepo,
			steps: []contractStep{
				usageStep("--bogus"),
				usageStep(fresh),
				usageStep("--folder"),
				usageStep("--folder", fresh, "extra"),
			},
			check: createsNothing,
		},
	}
}
