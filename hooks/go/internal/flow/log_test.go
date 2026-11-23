package flow

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLogWritesHeaderAndRowWithNoVersionLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	folder := filepath.Join(dir, "f")
	mustExit(t, runFlow(t, dir, nil, "log", "intent", "create", "intent.md", "ok", "capture the problem", "intent.md", "spec define the solution", "--folder", folder), 0, "log")
	lines := readLines(t, filepath.Join(folder, "state.tsv"))
	if len(lines) != 2 || lines[0] != headerRow {
		t.Fatalf("want the header and one row, with no version line; got %q", lines)
	}
	c := cells(lines[1])
	if len(c) != 9 {
		t.Fatalf("row has %d cells, want 9: %q", len(c), lines[1])
	}
	want := []string{"test-session", "intent", "create", "intent.md", "ok", "capture the problem", "intent.md", "spec define the solution"}
	for i, w := range want {
		if c[i+1] != w {
			t.Errorf("cell %d = %q, want %q", i+1, c[i+1], w)
		}
	}
	if !strings.HasSuffix(c[0], "Z") || len(c[0]) != 20 {
		t.Errorf("ts %q is not ISO 8601 UTC", c[0])
	}
}

func TestLogSecondRowDoesNotRewriteHeader(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	folder := filepath.Join(dir, "f")
	logOK(t, dir, folder, "intent", "create", "intent.md", "ok", "first", "intent.md", "spec go")
	logOK(t, dir, folder, "intent", "challenge", "intent.md", "ok", "second", "none", "spec go")
	lines := readLines(t, filepath.Join(folder, "state.tsv"))
	if len(lines) != 3 || lines[0] != headerRow {
		t.Fatalf("want 3 lines with one header row, got %q", lines)
	}
}

func TestLogAcceptsEveryEventAndStatusInTheClosedSets(t *testing.T) {
	t.Parallel()
	// A commit row names a commit, so the calls run inside a repository.
	dir := newRepo(t)
	// An order row hashes a standing order of the folder's plan.
	writeFile(t, dir, "f/plan.md", "## Standing orders\n\n1. an order\n")
	all := strings.Fields(events)
	cycle := strings.Fields(statuses)
	// These events fix their target, status or evidence, so each gets a valid
	// row instead of the generic target, the cycled status and none.
	shaped := map[string][3]string{
		"review": {"task-1", "ok", "kind=quality dispatched=2026-10-30T10:00:00Z round=1 blocked=40"},
		"escape": {"task-1", "failed", "found-by=copilot severity=important review=quality"},
		"commit": {"fix", "ok", "auto:HEAD"},
		"drop":   {"task-1", "cancelled", "none"},
		"order":  {"1", "ok", "auto"},
		"note":   {"coverage", "ok", "none"},
		"waive":  {"aggregate-review", "ok", "none"},
		"demote": {"wave-2", "ok", "none"},
	}
	used := make(map[string]bool)
	for i, ev := range all {
		target, st, evidence := "target-"+ev, cycle[i%len(cycle)], "none"
		if s, ok := shaped[ev]; ok {
			target, st, evidence = s[0], s[1], s[2]
		}
		// A verify cell must fit a documented form on every status, so it names
		// HEAD and keeps the cycled status.
		if ev == "verify" {
			evidence = "auto:HEAD"
		}
		used[st] = true
		folder := filepath.Join(dir, "f")
		mustExit(t, runFlow(t, dir, nil, "log", "execute", ev, target, st, "why", evidence, "execute go", "--folder", folder), 0, ev)
	}
	if got := len(readLines(t, filepath.Join(dir, "f", "state.tsv"))); got != 1+len(all) {
		t.Fatalf("want %d lines, got %d", 1+len(all), got)
	}
	for _, st := range cycle {
		if !used[st] {
			t.Errorf("status %s was never logged", st)
		}
	}
}

func TestLogRejectsInvalidCells(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"phase outside the set":   {"plan", "create", "plan.md", "ok", "why", "plan.md", "execute go"},
		"event outside the set":   {"intent", "verdict", "intent.md", "ok", "why", "intent.md", "spec go"},
		"status outside the set":  {"intent", "create", "intent.md", "staged", "why", "intent.md", "spec go"},
		"empty target":            {"intent", "create", " ", "ok", "why", "intent.md", "spec go"},
		"empty why":               {"intent", "create", "intent.md", "ok", " ", "intent.md", "spec go"},
		"empty evidence":          {"intent", "create", "intent.md", "ok", "why", " ", "spec go"},
		"empty next":              {"intent", "create", "intent.md", "ok", "why", "intent.md", " "},
		"next not led by a phase": {"intent", "create", "intent.md", "ok", "why", "intent.md", "review it"},
		"case-sensitive phase":    {"Intent", "create", "intent.md", "ok", "why", "intent.md", "spec go"},
		"case-sensitive event":    {"intent", "Create", "intent.md", "ok", "why", "intent.md", "spec go"},
		"case-sensitive status":   {"intent", "create", "intent.md", "OK", "why", "intent.md", "spec go"},
		"case-sensitive next":     {"intent", "create", "intent.md", "ok", "why", "intent.md", "Spec go"},
		"case-sensitive done":     {"intent", "create", "intent.md", "ok", "why", "intent.md", "Done"},
		"auto on a create row":    {"intent", "create", "intent.md", "ok", "why", "auto", "spec go"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			folder := filepath.Join(dir, "f")
			args := append([]string{"log"}, c...)
			args = append(args, "--folder", folder)
			mustExit(t, runFlow(t, dir, nil, args...), 2, name)
			if _, err := os.Stat(filepath.Join(folder, "state.tsv")); err == nil {
				t.Fatalf("%s: a rejected row must create no file", name)
			}
		})
	}
}

// reviewEscapeCase is one log call for a review or escape row and the exit code
// the helper must return for it.
type reviewEscapeCase struct {
	name, event, status, evidence string
	wantExit                      int
}

// reviewEscapeCases holds the valid and invalid forms of review and escape rows.
var reviewEscapeCases = []reviewEscapeCase{
	{"valid review", "review", "ok", "kind=quality dispatched=2026-10-30T10:00:00Z round=1 blocked=40", 0},
	{"valid failed review", "review", "failed", "kind=plan dispatched=2026-10-30T10:00:00Z round=2 blocked=0", 0},
	{"valid escape", "escape", "failed", "found-by=copilot severity=important review=quality", 0},
	{"valid spec review", "review", "ok", "kind=spec dispatched=2026-10-30T10:00:00Z round=1 blocked=0", 0},
	{"review with the optional pairs", "review", "ok", "kind=spec dispatched=2026-10-30T10:00:00Z round=1 blocked=0 rev=2 by=independent commit=0123abc", 0},
	{"escape with a word", "escape", "failed", "found-by=person severity=critical review=quality word=blocker", 0},
	{"review optional pair in place of a required one", "review", "ok", "kind=spec dispatched=2026-10-30T10:00:00Z round=1 rev=2", 2},
	{"review by another reviewer", "review", "ok", "kind=spec dispatched=2026-10-30T10:00:00Z round=1 blocked=0 by=author", 2},
	{"escape word in place of a required pair", "escape", "failed", "found-by=person severity=critical word=blocker", 2},
	{"escape review spec", "escape", "failed", "found-by=person severity=critical review=spec", 2},
	{"review status pending", "review", "pending", "kind=quality dispatched=2026-10-30T10:00:00Z round=1 blocked=0", 2},
	{"review kind correction", "review", "ok", "kind=correction dispatched=2026-10-30T10:00:00Z round=1 blocked=0", 2},
	{"review missing blocked", "review", "ok", "kind=quality dispatched=2026-10-30T10:00:00Z round=1", 2},
	{"review repeats round", "review", "ok", "kind=quality dispatched=2026-10-30T10:00:00Z round=1 round=2 blocked=0", 2},
	{"review repeats a key in place of another", "review", "ok", "kind=quality dispatched=2026-10-30T10:00:00Z round=1 round=2", 2},
	{"review unknown key", "review", "ok", "kind=quality dispatched=2026-10-30T10:00:00Z round=1 blocked=0 extra=1", 2},
	{"review round zero", "review", "ok", "kind=quality dispatched=2026-10-30T10:00:00Z round=0 blocked=0", 2},
	{"review negative blocked", "review", "ok", "kind=quality dispatched=2026-10-30T10:00:00Z round=1 blocked=-1", 2},
	{"review dispatched not a ts", "review", "ok", "kind=quality dispatched=yesterday round=1 blocked=0", 2},
	{"review dispatched without Z", "review", "ok", "kind=quality dispatched=2026-10-30T10:00:00 round=1 blocked=0", 2},
	{"review empty pair", "review", "ok", "kind=quality  dispatched=2026-10-30T10:00:00Z round=1 blocked=0", 2},
	{"review evidence none", "review", "ok", "none", 2},
	{"review evidence auto", "review", "ok", "auto", 2},
	{"escape status ok", "escape", "ok", "found-by=ci severity=critical review=plan", 2},
	{"escape found-by bot", "escape", "failed", "found-by=bot severity=critical review=plan", 2},
	{"escape severity minor", "escape", "failed", "found-by=ci severity=minor review=plan", 2},
	{"escape review wave", "escape", "failed", "found-by=ci severity=critical review=wave", 2},
	{"escape empty pair", "escape", "failed", "found-by=ci  severity=critical review=plan", 2},
	{"review keys in any order", "review", "ok", "blocked=40 round=1 dispatched=2026-10-30T10:00:00Z kind=quality", 0},
	{"review tab between pairs", "review", "ok", "kind=quality\tdispatched=2026-10-30T10:00:00Z round=1 blocked=40", 0},
	{"review round leading zero", "review", "ok", "kind=quality dispatched=2026-10-30T10:00:00Z round=01 blocked=0", 2},
	{"review blocked double zero", "review", "ok", "kind=quality dispatched=2026-10-30T10:00:00Z round=1 blocked=00", 2},
	{"review empty kind value", "review", "ok", "kind= dispatched=2026-10-30T10:00:00Z round=1 blocked=0", 2},
	{"review uppercase key", "review", "ok", "Kind=quality dispatched=2026-10-30T10:00:00Z round=1 blocked=0", 2},
	{"review value with a second equals", "review", "ok", "kind=quality=x dispatched=2026-10-30T10:00:00Z round=1 blocked=0", 2},
}

func TestLogReviewAndEscapeEvidence(t *testing.T) {
	t.Parallel()
	for _, c := range reviewEscapeCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			folder := filepath.Join(dir, "f")
			res := runFlow(t, dir, nil, "log", "execute", c.event, "task-1", c.status, "why", c.evidence, "execute go", "--folder", folder)
			mustExit(t, res, c.wantExit, c.name)
			_, err := os.Stat(filepath.Join(folder, "state.tsv"))
			if c.wantExit != 0 && err == nil {
				t.Fatalf("%s: a rejected row must create no file", c.name)
			}
			if c.wantExit != 0 {
				mustExit(t, runFlow(t, t.TempDir(), nil, "log", "execute", c.event, "task-1", c.status, "why", c.evidence, "execute go"), 2, "evidence before lookup")
			}
			if c.wantExit == 0 {
				// A tab inside a cell is stored as one space.
				want := strings.ReplaceAll(c.evidence, "\t", " ")
				if got := lastRow(t, dir)[7]; got != want {
					t.Fatalf("%s: evidence stored %q, want %q", c.name, got, want)
				}
			}
		})
	}
}

func TestLogUsageErrorExitsOne(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustExit(t, runFlow(t, dir, nil, "log", "intent", "create"), 1, "too few args")
	mustExit(t, runFlow(t, dir, nil, "frobnicate"), 1, "unknown command")
}

var guardCases = []struct{ name, input, want string }{
	{"equals", "=1", "'=1"},
	{"leading-quote-equals", "'=1", "''=1"},
	{"plus-formula", "+SUM(A1)", "'+SUM(A1)"},
	{"minus", "-2", "'-2"},
	{"at", "@x", "'@x"},
	{"leading-quote-plain", "'plain", "''plain"},
	{"plain", "plain", "plain"},
	{"embedded-equals", "a=1", "a=1"},
	{"tab-then-equals", "\t=1+1", "'=1+1"},
	{"space-then-equals", " =1+1", "'=1+1"},
	{"newline-then-at", "\n@x", "'@x"},
	{"space-then-quote", " 'x", "''x"},
	{"space-then-at", " @link", "'@link"},
	{"trailing-space-plain", "plain ", "plain"},
	{"surrounded-plain", "  plain  ", "plain"},
}

// freeTextColumn is a stored column whose cell passes through the strip, trim,
// and formula guard. invoke returns the arguments and extra environment that
// put input in that column. The next column is left out of the guard table: its
// first word must be a phase name or done, so no value that needs the guard can
// reach it. The session column takes its value from FLOW_SESSION.
type freeTextColumn struct {
	name   string
	cell   int
	invoke func(input string) (args, env []string)
}

func logArgs(target, why, evidence, next string) []string {
	return []string{"log", "intent", "create", target, "ok", why, evidence, next, "--folder", "f"}
}

var freeTextColumns = []freeTextColumn{
	{"session", 1, func(in string) ([]string, []string) {
		return logArgs("intent.md", "why", "none", "spec go"), []string{"FLOW_SESSION=" + in}
	}},
	{"target", 4, func(in string) ([]string, []string) { return logArgs(in, "why", "none", "spec go"), nil }},
	{"why", 6, func(in string) ([]string, []string) { return logArgs("intent.md", in, "none", "spec go"), nil }},
	{"evidence", 7, func(in string) ([]string, []string) { return logArgs("intent.md", "why", in, "spec go"), nil }},
}

// separatorCases are control characters that must not survive inside a cell:
// each becomes one space.
var separatorCases = []struct{ name, input, want string }{
	{"tab", "a\tb", "a b"},
	{"newline", "a\nb", "a b"},
	{"carriage-return", "a\rb", "a b"},
}

// nextWith builds a next value whose first word and second word are split by sep.
func nextWith(sep string) string { return "spec" + sep + "go" }

func lastRow(t *testing.T, dir string) []string {
	t.Helper()
	lines := readLines(t, filepath.Join(dir, "f", "state.tsv"))
	return cells(lines[len(lines)-1])
}

func TestLogGuardTable(t *testing.T) {
	t.Parallel()
	for _, col := range freeTextColumns {
		for _, gc := range guardCases {
			t.Run(col.name+"/"+gc.name, func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				args, env := col.invoke(gc.input)
				mustExit(t, runFlow(t, dir, env, args...), 0, "log")
				if got := lastRow(t, dir)[col.cell]; got != gc.want {
					t.Fatalf("%s stored %q, want %q", col.name, got, gc.want)
				}
			})
		}
	}
}

// TestGateVerdictOnTheDefaultBranchLandsInUnassigned holds an active flow on
// main and logs a gate release row there. The row goes to unassigned/state.tsv
// and the active flow's file does not change, because on the default branch
// only a bind can resolve a flow.
func TestGateVerdictOnTheDefaultBranchLandsInUnassigned(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	seedFlow(t, repo, flowA, "") // an active flow that main holds
	commitAll(t, repo, "a on main")
	statePath := filepath.Join(repo, flowA, "state.tsv")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}

	logOK(t, repo, "docs/specs/unassigned", "gate", "release", "v1", "ok", "tagged", "v1", "done")

	unassigned := readLines(t, filepath.Join(repo, "docs/specs/unassigned/state.tsv"))
	if last := unassigned[len(unassigned)-1]; !strings.Contains(last, "\tgate\trelease\tv1\t") {
		t.Fatalf("release row is not in unassigned/state.tsv: %q", unassigned)
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("the active flow's state.tsv changed:\n%s\n---\n%s", before, after)
	}
	mustExit(t, whereOf(t, repo), 3, "main must not resolve to the active flow")
}

func TestLogReplacesTabsAndNewlinesInsideACell(t *testing.T) {
	t.Parallel()
	for _, col := range freeTextColumns {
		for _, sc := range separatorCases {
			t.Run(col.name+"/"+sc.name, func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				args, env := col.invoke(sc.input)
				mustExit(t, runFlow(t, dir, env, args...), 0, "log")
				row := lastRow(t, dir)
				if len(row) != 9 {
					t.Fatalf("row has %d cells, want 9: %q", len(row), row)
				}
				if got := row[col.cell]; got != sc.want {
					t.Fatalf("%s stored %q, want %q", col.name, got, sc.want)
				}
			})
		}
	}
	for _, sc := range separatorCases {
		t.Run("next/"+sc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			sep := strings.TrimSuffix(strings.TrimPrefix(sc.input, "a"), "b")
			mustExit(t, runFlow(t, dir, nil, logArgs("intent.md", "why", "none", nextWith(sep))...), 0, "log")
			row := lastRow(t, dir)
			if len(row) != 9 || row[8] != "spec go" {
				t.Fatalf("next stored %q in a %d-cell row, want %q", row[len(row)-1], len(row), "spec go")
			}
		})
	}
}

func TestLogWritesNoBOMAndBareLF(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustExit(t, runFlow(t, dir, nil, logArgs("intent.md", "why", "none", "spec go")...), 0, "log")
	mustExit(t, runFlow(t, dir, nil, logArgs("intent.md", "again", "none", "spec go")...), 0, "second log")
	raw, err := os.ReadFile(filepath.Join(dir, "f", "state.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("flow wrote a BOM")
	}
	if bytes.Contains(raw, []byte("\r")) {
		t.Fatal("flow wrote a CR; rows must end in a bare LF")
	}
}

// statusTextHash returns the text hash flow status prints for artifact in a
// folder that already holds a state.tsv, the hash a caller passes to
// --expect-blob.
func statusTextHash(t *testing.T, dir, folder, artifact string) string {
	t.Helper()
	res := runFlow(t, dir, nil, "status", "--folder", folder)
	mustExit(t, res, 0, "status")
	prefix := "hashes: " + artifact + " text="
	for _, line := range strings.Split(res.stdout, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("status prints no %q line: %q", prefix, res.stdout)
	return ""
}

func TestLogAutoEvidenceForApproval(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	folder := "docs/specs/2026-10/feature-x"
	writeFile(t, repo, folder+"/spec.md", "# spec\n")
	logOK(t, repo, folder, "spec", "create", "spec.md", "ok", "made", "none", "spec go")
	hash := statusTextHash(t, repo, folder, "spec.md")
	mustExit(t, runFlow(t, repo, nil, expectBlobArgs("spec.md", "auto", hash, folder)...), 0, "guarded approve")
	lines := readLines(t, filepath.Join(repo, folder, "state.tsv"))
	if got := cells(lines[len(lines)-1])[7]; got != "text="+hash {
		t.Fatalf("approve evidence %q, want text=%s", got, hash)
	}
}

func expectBlobArgs(target, evidence, hash, folder string) []string {
	return []string{"log", "spec", "approve", target, "ok", "reapproved", evidence, "decompose go", "--expect-blob", hash, "--folder", folder}
}

func TestLogExpectBlobMatchStoresTheStatusHash(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	folder := "docs/specs/2026-10/feature-x"
	writeFile(t, repo, folder+"/spec.md", "# spec\n")
	logOK(t, repo, folder, "spec", "create", "spec.md", "ok", "made", "none", "spec go")
	hash := statusTextHash(t, repo, folder, "spec.md")
	state := filepath.Join(repo, folder, "state.tsv")
	lines := readLines(t, state)
	mustExit(t, runFlow(t, repo, nil, expectBlobArgs("spec.md", "auto", hash, folder)...), 0, "guarded approve")
	mustExit(t, runFlow(t, repo, nil, expectBlobArgs("spec.md", "auto", hash, folder)...), 0, "second guarded approve")
	after := readLines(t, state)
	if len(after) != len(lines)+2 {
		t.Fatalf("rows %d, want %d", len(after), len(lines)+2)
	}
	for _, line := range after[len(lines):] {
		if got := cells(line)[7]; got != "text="+hash {
			t.Fatalf("guarded evidence %q, want text=%s", got, hash)
		}
	}
}

func TestLogExpectBlobMismatchWritesNoRow(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	folder := "docs/specs/2026-10/feature-x"
	writeFile(t, repo, folder+"/spec.md", "# spec\n")
	logOK(t, repo, folder, "spec", "create", "spec.md", "ok", "made", "none", "spec go")
	cur := statusTextHash(t, repo, folder, "spec.md")
	state := filepath.Join(repo, folder, "state.tsv")
	lines := readLines(t, state)
	wrong := cur[:len(cur)-1] + "0"
	if wrong == cur {
		wrong = cur[:len(cur)-1] + "1"
	}
	res := runFlow(t, repo, nil, expectBlobArgs("spec.md", "auto", wrong, folder)...)
	mustExit(t, res, 2, "mismatched guard")
	want := "invalid evidence: '" + wrong + "' -- " + folder + "/spec.md current text=" + cur + " does not match --expect-blob"
	if !strings.Contains(res.stderr, want) {
		t.Fatalf("stderr %q lacks %q", res.stderr, want)
	}
	if got := readLines(t, state); len(got) != len(lines) {
		t.Fatalf("rows changed: %d, want %d", len(got), len(lines))
	}
}

func TestLogExpectBlobMissingArtifactWritesNoRow(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	folder := "docs/specs/2026-10/feature-x"
	writeFile(t, repo, folder+"/intent.md", "# intent\n")
	logOK(t, repo, folder, "intent", "create", "intent.md", "ok", "made", "none", "spec go")
	state := filepath.Join(repo, folder, "state.tsv")
	before := readLines(t, state)
	res := runFlow(t, repo, nil, expectBlobArgs("spec.md", "auto", "abc123", folder)...)
	mustExit(t, res, 2, "unavailable guard")
	want := "invalid evidence: 'abc123' -- " + folder + "/spec.md current text=unavailable does not match --expect-blob"
	if !strings.Contains(res.stderr, want) {
		t.Fatalf("stderr %q lacks %q", res.stderr, want)
	}
	if got := readLines(t, state); len(got) != len(before) {
		t.Fatalf("rows changed: %d, want %d", len(got), len(before))
	}
	// A missing artifact has no hash, so an empty expected hash must not match it.
	res = runFlow(t, repo, nil, expectBlobArgs("spec.md", "auto", "", folder)...)
	mustExit(t, res, 2, "unavailable guard with empty hash")
	want = "invalid evidence: '' -- " + folder + "/spec.md current text=unavailable does not match --expect-blob"
	if !strings.Contains(res.stderr, want) {
		t.Fatalf("stderr %q lacks %q", res.stderr, want)
	}
	if got := readLines(t, state); len(got) != len(before) {
		t.Fatalf("rows changed: %d, want %d", len(got), len(before))
	}
}

func TestLogExpectBlobRejectsOtherCombinations(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	folder := "docs/specs/2026-10/feature-x"
	writeFile(t, repo, folder+"/spec.md", "# spec\n")
	logOK(t, repo, folder, "spec", "create", "spec.md", "ok", "made", "none", "spec go")
	state := filepath.Join(repo, folder, "state.tsv")
	before := readLines(t, state)
	create := []string{"log", "spec", "create", "spec.md", "ok", "again", "none", "spec go", "--expect-blob", "abc", "--folder", folder}
	mustExit(t, runFlow(t, repo, nil, create...), 1, "guard on create")
	mustExit(t, runFlow(t, repo, nil, expectBlobArgs("spec.md", "none", "abc", folder)...), 1, "guard on approve none")
	mustExit(t, runFlow(t, repo, nil, "log", "spec", "approve", "spec.md", "ok", "x", "auto", "decompose go", "--folder", folder, "--expect-blob"), 1, "guard without value")
	if got := readLines(t, state); len(got) != len(before) {
		t.Fatalf("rows changed: %d, want %d", len(got), len(before))
	}
}

func TestLogAutoEvidenceForCommitRows(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	folder := "docs/specs/2026-10/feature-x"
	writeFile(t, repo, "src/a.txt", "one\n")
	commitAll(t, repo, "task commit")
	head := gitOut(t, repo, "rev-parse", "HEAD")
	patch := git(t, repo, "show", "HEAD")
	cmd := exec.Command("git", "patch-id", "--stable")
	cmd.Dir, cmd.Env, cmd.Stdin = repo, baseEnv(), strings.NewReader(patch)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	wantID := strings.Fields(string(output))[0]

	for _, ev := range []string{"verify", "revert", "diverge"} {
		logOK(t, repo, folder, "execute", ev, "task-1", "ok", "recorded", "auto", "execute go")
		lines := readLines(t, filepath.Join(repo, folder, "state.tsv"))
		if got, want := cells(lines[len(lines)-1])[7], "commit="+head+" patch-id="+wantID; got != want {
			t.Fatalf("%s evidence %q, want %q", ev, got, want)
		}
	}
	// auto:<sha> names another commit.
	first := gitOut(t, repo, "rev-parse", "HEAD~1")
	logOK(t, repo, folder, "execute", "verify", "task-0", "ok", "older", "auto:"+first, "execute go")
	lines := readLines(t, filepath.Join(repo, folder, "state.tsv"))
	if got := cells(lines[len(lines)-1])[7]; !strings.HasPrefix(got, "commit="+first+" patch-id=") {
		t.Fatalf("auto:<sha> evidence %q", got)
	}
}

func TestSessionFallsBackToTheHostVariableThenDash(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		env  []string
		want string
	}{
		"flow session wins":  {[]string{"FLOW_SESSION=explicit", "CLAUDE_CODE_SESSION_ID=host"}, "explicit"},
		"host variable used": {[]string{"FLOW_SESSION=", "CLAUDE_CODE_SESSION_ID=host"}, "host"},
		"neither is a dash":  {[]string{"FLOW_SESSION=", "CLAUDE_CODE_SESSION_ID="}, "-"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			res := runFlow(t, dir, tc.env, "log", "intent", "create", "intent.md", "ok", "why", "intent.md", "spec go", "--folder", "f")
			mustExit(t, res, 0, name)
			if got := cells(readLines(t, filepath.Join(dir, "f", "state.tsv"))[1])[1]; got != tc.want {
				t.Fatalf("session cell %q, want %q", got, tc.want)
			}
		})
	}
}

// A default invocation, with neither session variable in the environment at
// all, writes the single character - in the session cell. An empty variable is
// a different input, covered above.
func TestSessionCellIsADashWhenNoSessionVariableIsSet(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustExit(t, runFlow(t, dir, []string{"FLOW_SESSION", "CLAUDE_CODE_SESSION_ID"}, logArgs("intent.md", "why", "none", "spec go")...), 0, "no session variables")
	if got := lastRow(t, dir)[1]; got != "-" {
		t.Fatalf("session %q, want exactly -", got)
	}
}
func TestConcurrentFirstWritersKeepEveryRow(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	folder := filepath.Join(dir, "f")
	const writers = 12
	done := make(chan result, writers)
	for i := 0; i < writers; i++ {
		go func(i int) {
			done <- runFlow(t, dir, nil, "log", "execute", "enter", "task-"+strconv.Itoa(i), "ok", "parallel row", "none", "execute go", "--folder", folder)
		}(i)
	}
	for i := 0; i < writers; i++ {
		if r := <-done; r.code != 0 {
			t.Fatalf("writer failed: %s", r.combined())
		}
	}
	lines := readLines(t, filepath.Join(folder, "state.tsv"))
	if len(lines) != 1+writers || lines[0] != headerRow {
		t.Fatalf("want a header and %d rows; got %d lines", writers, len(lines))
	}
}

func TestRelativeFolderResolvesAgainstTheCallersDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	mustExit(t, runFlow(t, sub, nil, "log", "intent", "create", "intent.md", "ok", "why", "intent.md", "spec go", "--folder", "rel"), 0, "relative folder")
	if _, err := os.Stat(filepath.Join(sub, "rel", "state.tsv")); err != nil {
		t.Fatalf("the row went somewhere else: %v", err)
	}
}

func TestLogResolvesTheBoundFlowAndTheSmallFixWhenFolderIsOmitted(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "feat/z")
	seedFlow(t, repo, flowA, "")
	commitAll(t, repo, "a")
	mustExit(t, runFlow(t, repo, nil, "log", "spec", "enter", "spec.md", "ok", "start spec", "none", "spec go"), 0, "log by lookup")
	lines := readLines(t, filepath.Join(repo, flowA, "state.tsv"))
	if !strings.Contains(lines[len(lines)-1], "\tspec\tenter\tspec.md\t") {
		t.Fatalf("row went to the wrong place: %q", lines)
	}

	// A small fix in progress sends the row to unassigned/state.tsv, even though
	// this branch changed the state file of flow A.
	git(t, repo, "switch", "-q", "-c", "fix/q")
	logOK(t, repo, "docs/specs/unassigned", "execute", "skip", "chain fix/q", "ok", "change: one concern", "base=abc", "execute go")
	mustExit(t, runFlow(t, repo, nil, "log", "execute", "verify", "fix", "ok", "green", "auto", "gate go"), 0, "log for a small fix")
	last := readLines(t, filepath.Join(repo, "docs/specs/unassigned/state.tsv"))
	if !strings.Contains(last[len(last)-1], "\texecute\tverify\tfix\t") {
		t.Fatalf("small fix row went to the wrong place: %q", last)
	}
}

func TestLogExactBytesAndInjectedClock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	now := func() time.Time { return time.Date(2026, 11, 10, 8, 9, 10, 0, time.FixedZone("east", 3600)) }
	code := Run([]string{"log", "intent", "create", " =cell\t", "ok", "line1\nline2", " @proof ", "spec\tgo", "--folder", "f"}, dir,
		func(key string) string {
			if key == "FLOW_SESSION" {
				return "=session"
			}
			return ""
		}, now, &stdout, &stderr)
	if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(filepath.Join(dir, "f/state.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	want := headerRow + "\n2026-11-10T07:09:10Z\t'=session\tintent\tcreate\t'=cell\tok\tline1 line2\t'@proof\tspec go\n"
	if string(got) != want {
		t.Fatalf("stored bytes=%q want=%q", got, want)
	}
}

func TestLogOptionsAndArity(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, "f/spec.md", "# spec\n")
	hash := textHash("spec.md", []byte("# spec\n"))
	for _, args := range [][]string{
		{"log", "spec", "approve", "spec.md", "ok", "why", "auto", "decompose go", "--folder", "f", "--expect-blob", hash},
		{"log", " spec\t", "\napprove ", " spec.md ", " ok\n", "why", "\tauto ", "decompose go", "--folder", "f", "--expect-blob", hash},
		{"log", "--folder", "wrong", "spec", "--folder", "f", "approve", "spec.md", "ok", "why", "--expect-blob", hash, "auto", "decompose go"},
	} {
		mustExit(t, runFlow(t, repo, nil, args...), 0, "interspersed options")
		if got := lastRow(t, repo)[7]; got != "text="+hash {
			t.Fatalf("option form stored evidence %q want text=%s", got, hash)
		}
	}
	for _, tail := range [][]string{{"extra"}, {"--folder"}, {"--expect-blob"}} {
		args := append(logArgs("intent.md", "why", "none", "spec go"), tail...)
		res := runFlow(t, repo, nil, args...)
		mustExit(t, res, 1, "usage")
		if res.stdout != "" || !strings.HasPrefix(res.stderr, "Usage: flow log ") {
			t.Fatalf("usage stdout=%q stderr=%q", res.stdout, res.stderr)
		}
	}
	args := expectBlobArgs("spec.md", "auto", hash, "f")
	args[1] = "invalid"
	mustExit(t, runFlow(t, repo, nil, args...), 2, "cells still validated with valid guard combination")
	args[2] = "create"
	mustExit(t, runFlow(t, repo, nil, args...), 1, "invalid guard combination before cells")
}

func TestLogAutoRejectionsAndEmptyPatch(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	for _, tc := range []struct{ event, evidence, reason string }{
		{"approve", "auto", "approve needs --expect-blob <hash>: run flow status and pass the hash of the content the user was shown"},
		{"approve", "auto:HEAD", "auto:<sha> applies to commit rows only"},
		{"verify", "auto:missing-commit", "no such commit to verify against"},
		{"create", "auto:HEAD", "auto applies to approve, order, verify, revert, diverge and commit rows only"},
	} {
		res := runFlow(t, repo, nil, "log", "spec", tc.event, "spec.md", "ok", "why", tc.evidence, "spec go", "--folder", "f")
		mustExit(t, res, 2, tc.evidence)
		if res.stderr != "flow: invalid evidence: '"+tc.evidence+"' -- "+tc.reason+"\n" {
			t.Fatalf("stderr=%q", res.stderr)
		}
		if _, err := os.Stat(filepath.Join(repo, "f/state.tsv")); !os.IsNotExist(err) {
			t.Fatalf("rejection created state: %v", err)
		}
	}
	mustExit(t, runFlow(t, t.TempDir(), nil, "log", "execute", "verify", "task", "ok", "why", "auto", "execute go", "--folder", "f"), 2, "no Git commit")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "empty")
	// An empty commit has no patch-id, and no row may carry an empty one.
	res := runFlow(t, repo, nil, "log", "execute", "verify", "empty", "ok", "why", "auto", "execute go", "--folder", "f")
	mustExit(t, res, 2, "empty commit")
	if res.stderr != "flow: invalid evidence: 'auto' -- the commit changes nothing, so it has no patch-id\n" {
		t.Fatalf("stderr=%q", res.stderr)
	}
	if _, err := os.Stat(filepath.Join(repo, "f/state.tsv")); !os.IsNotExist(err) {
		t.Fatalf("an empty commit created state: %v", err)
	}
}

func TestLogFailuresWriteNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	res := runFlow(t, dir, nil, "log", "invalid", "create", "intent.md", "ok", "why", "none", "spec go")
	mustExit(t, res, 2, "validation before lookup")
	if !strings.HasPrefix(res.stderr, "flow: invalid phase:") {
		t.Fatal(res.stderr)
	}
	res = runFlow(t, dir, nil, "log", "intent", "create", "intent.md", "ok", "why", "none", "spec go")
	mustExit(t, res, 3, "no flow")
	if res.stderr != "flow: no flow found; ask which flow, or pass --folder\n" {
		t.Fatal(res.stderr)
	}
	writeFile(t, dir, "blocked", "file\n")
	res = runFlow(t, dir, nil, "log", "intent", "create", "intent.md", "ok", "why", "none", "spec go", "--folder", "blocked/child")
	mustExit(t, res, 1, "unwritable folder")
	if !strings.HasPrefix(res.stderr, "flow: cannot create folder ") {
		t.Fatal(res.stderr)
	}
	if err := os.MkdirAll(filepath.Join(dir, "bad/state.tsv"), 0o755); err != nil {
		t.Fatal(err)
	}
	res = runFlow(t, dir, nil, "log", "intent", "create", "intent.md", "ok", "why", "none", "spec go", "--folder", "bad")
	mustExit(t, res, 1, "unwritable state")
	if !strings.HasPrefix(res.stderr, "flow: cannot append ") {
		t.Fatal(res.stderr)
	}
}

func TestLogAmbiguousFlowPropagatesExitFour(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "topic")
	lookupSeed(t, repo, flowA, "")
	lookupSeed(t, repo, flowB, "")
	commitAll(t, repo, "flows")
	beforeA, _ := os.ReadFile(filepath.Join(repo, flowA, "state.tsv"))
	beforeB, _ := os.ReadFile(filepath.Join(repo, flowB, "state.tsv"))
	res := runFlow(t, repo, nil, "log", "intent", "create", "intent.md", "ok", "why", "none", "spec go")
	mustExit(t, res, 4, "ambiguous log")
	if res.stderr != "flow: more than one flow matches; ask which one:\n  "+flowA+"\n  "+flowB+"\n" {
		t.Fatalf("ambiguity=%q", res.stderr)
	}
	afterA, _ := os.ReadFile(filepath.Join(repo, flowA, "state.tsv"))
	afterB, _ := os.ReadFile(filepath.Join(repo, flowB, "state.tsv"))
	if !bytes.Equal(beforeA, afterA) || !bytes.Equal(beforeB, afterB) {
		t.Fatal("ambiguous log wrote a row")
	}
}

func TestLogParityEmptyCellsAndNonASCIISeparator(t *testing.T) {
	t.Parallel()
	for _, at := range []int{2, 4, 5, 6} {
		for _, value := range []string{"", " ", "\t\n"} {
			args := []string{"log", "intent", "create", "intent.md", "ok", "why", "none", "spec go", "--folder", "f"}
			args[at+1] = value
			mustExit(t, runFlow(t, t.TempDir(), nil, args...), 2, "empty cell")
		}
	}
	mustExit(t, runFlow(t, t.TempDir(), nil, logArgs("intent.md", "why", "none", "spec\u00a0go")...), 2, "NBSP is not a next separator")
}

func TestLogRelativeFolderAutoEvidenceInOpenSession(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	for _, folder := range []string{"sub/fa", "sub/fb"} {
		writeFile(t, repo, folder+"/spec.md", "# Spec\n")
	}
	hash := gitOut(t, repo, "hash-object", "--", "sub/fa/spec.md")
	head := gitOut(t, repo, "rev-parse", "HEAD")
	writeState(t, repo, "sub/fa", row("2026-10-14T10:00:00Z", "spec", "approve", "spec.md", "ok", "approved", "blob="+hash, "decompose go"), row("2026-10-14T10:01:00Z", "execute", "verify", "task-1", "ok", "v", "commit="+head+" patch-id=bb", "execute go"))
	commitAll(t, repo, "flow")
	text := statusTextHash(t, repo, "sub/fa", "spec.md")
	for _, folder := range []string{"sub/fa", "sub/fb"} {
		for _, c := range [][]string{{"spec", "approve", "spec.md", "ok", "approved", "auto", "decompose go", "--expect-blob", text}, {"execute", "verify", "task-2", "ok", "verified", "auto", "execute go"}} {
			args := append(append([]string{"log"}, c...), "--folder", folder)
			mustExit(t, runFlow(t, filepath.Join(repo, "sub"), nil, args...), 0, "relative log auto")
		}
	}
	a := readLines(t, filepath.Join(repo, "sub/fa/state.tsv"))
	b := readLines(t, filepath.Join(repo, "sub/fb/state.tsv"))
	for i := 1; i <= 2; i++ {
		if strings.Join(cells(a[len(a)-i])[1:], "\t") != strings.Join(cells(b[len(b)-i])[1:], "\t") {
			t.Fatalf("relative stored rows differ: %q %q", a, b)
		}
	}
	if got := cells(b[len(b)-2])[7]; got != "text="+text {
		t.Fatalf("relative approval=%q want=text=%s", got, text)
	}
}

func TestLogReapprovalGuardChain(t *testing.T) {
	t.Parallel()
	for _, artifact := range []string{"intent.md", "spec.md", "plan.md", "notes.md"} {
		t.Run(artifact, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\none\n")
			oldHash := textHash(artifact, []byte("## Goal\none\n"))
			mustExit(t, runFlow(t, repo, nil, expectBlobArgs(artifact, "auto", oldHash, folderX)...), 0, "first approval")
			oldLines := readLines(t, filepath.Join(repo, folderX, "state.tsv"))
			commitAll(t, repo, "approved")
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\none\ntwo\n")
			current := textHash(artifact, []byte("## Goal\none\ntwo\n"))
			res := runFlow(t, repo, nil, expectBlobArgs(artifact, "auto", oldHash, folderX)...)
			mustExit(t, res, 2, "reapproval bound to old hash")
			if got := readLines(t, filepath.Join(repo, folderX, "state.tsv")); len(got) != len(oldLines) {
				t.Fatal("rejected reapproval wrote row")
			}
			mustExit(t, runFlow(t, repo, nil, expectBlobArgs(artifact, "auto", current, folderX)...), 0, "reapproval bound to current hash")
			got := readLines(t, filepath.Join(repo, folderX, "state.tsv"))
			if len(got) != len(oldLines)+1 || cells(got[len(got)-1])[7] != "text="+current {
				t.Fatalf("reapproval rows=%q", got)
			}
		})
	}
}

func TestLogExpectedBlobStoresTheComparedHash(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, "f/spec.md", "# Spec\none\n")
	hash := textHash("spec.md", []byte("# Spec\none\n"))
	var stdout, stderr bytes.Buffer
	getenv := func(key string) string {
		if key == "FLOW_SESSION" {
			writeFile(t, repo, "f/spec.md", "# Spec\nunseen edit\n")
			return "session"
		}
		return ""
	}
	code := Run(expectBlobArgs("spec.md", "auto", hash, "f"), repo, getenv, time.Now, &stdout, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if got := lastRow(t, repo)[7]; got != "text="+hash {
		t.Fatalf("stored %q instead of compared text=%s", got, hash)
	}
	if newHash, _ := fileTextHash(filepath.Join(repo, "f"), "spec.md"); newHash == hash {
		t.Fatal("fixture did not edit artifact after comparison")
	}
}
