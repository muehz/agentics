package flow

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestLedgerRecordOf(t *testing.T) {
	const (
		hash   = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		other  = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
		sha    = "0123456789abcdef0123456789abcdef01234567"
		finder = "finding audit commit:" + sha
	)
	tests := []struct {
		name                string
		event, target, text string
		kind, hash          string
		ok                  bool
		// refusedWaive is set when parseWaive must refuse the target, so that no
		// log call reaches the writer with it.
		refusedWaive bool
	}{
		{name: "approve with text= gives its hash", event: "approve", target: "plan.md", text: "text=" + hash, kind: "approve", hash: hash, ok: true},
		{name: "approve with blob= gives its hash", event: "approve", target: "plan.md", text: "blob=" + hash, kind: "approve", hash: hash, ok: true},
		{name: "a typed text= row is shaped and left to the writer's guard", event: "approve", target: "plan.md", text: "text=" + other, kind: "approve", hash: other, ok: true},
		{name: "approve hashes the whole remainder, a trailing word with it", event: "approve", target: "plan.md", text: "text=" + hash + " extra", kind: "approve", hash: hash + " extra", ok: true},
		{name: "a declined approval records nothing", event: "approve", target: "plan.md", text: "none"},
		{name: "blob= with no hash records nothing", event: "approve", target: "plan.md", text: "blob="},
		{name: "text= with no hash records nothing", event: "approve", target: "plan.md", text: "text="},
		{name: "an approval whose cell does not start with the hash records nothing", event: "approve", target: "plan.md", text: "x text=" + hash},

		{name: "order gives its entry hash", event: "order", target: "3", text: "entry=" + hash, kind: "order", hash: hash, ok: true},
		{name: "order without entry= records nothing", event: "order", target: "3", text: "auto"},
		{name: "order with an empty entry= records nothing", event: "order", target: "3", text: "entry="},

		{name: "a rule divergence gives rule and its entry hash", event: "diverge", target: "task-2 rule", text: "none entry=" + hash, kind: "rule", hash: hash, ok: true},
		{name: "a rule divergence reads the entry after commit evidence", event: "diverge", target: "run rule", text: "commit=" + sha + " patch-id=" + other + " entry=" + hash, kind: "rule", hash: hash, ok: true},
		{name: "a rule divergence without entry= records nothing", event: "diverge", target: "task-2 rule", text: "none"},
		{name: "a diverge whose target is one word records nothing", event: "diverge", target: "x", text: "none entry=" + hash},
		{name: "a diverge of two words not ending in rule records nothing", event: "diverge", target: "spec.md Constraints", text: "none entry=" + hash},
		{name: "a diverge of three words ending in rule records nothing", event: "diverge", target: "a b rule", text: "none entry=" + hash},
		{name: "a diverge naming rule alone records nothing", event: "diverge", target: "rule", text: "none entry=" + hash},
		{name: "rule is matched in lower case only", event: "diverge", target: "task-2 Rule", text: "none entry=" + hash},

		{name: "a commit row gives its sha", event: "commit", target: "user", text: "commit=" + sha, kind: "commit", hash: sha, ok: true},
		{name: "a commit row gives the sha as written", event: "commit", target: "user", text: "commit=ABCDEF0", kind: "commit", hash: "ABCDEF0", ok: true},
		{name: "patch-id and slice pairs do not change a commit row", event: "commit", target: "fix", text: "commit=" + sha + " patch-id=" + other + " slice=S1", kind: "commit", hash: sha, ok: true},
		{name: "a commit row with two commit= values records nothing", event: "commit", target: "user", text: "commit=" + sha + " commit=" + other},
		{name: "a commit row with no commit= records nothing", event: "commit", target: "user", text: "patch-id=" + other},
		{name: "a commit row with an empty commit= records nothing", event: "commit", target: "user", text: "commit="},

		{name: "a finding waiver gives a dash", event: "waive", target: finder, text: "none", kind: "waive", hash: "-", ok: true},
		{name: "a waiver of another head records nothing", event: "waive", target: "review", text: "none"},
		{name: "a waiver of a head that starts with finding records nothing", event: "waive", target: "findings audit x", text: "none", refusedWaive: true},
		{name: "a finding waiver with one argument shapes as finding and parseWaive refuses it", event: "waive", target: "finding x", text: "none", kind: "waive", hash: "-", ok: true, refusedWaive: true},
		{name: "a first word split by next line shapes as finding and parseWaive refuses it", event: "waive", target: "finding\u0085x y", text: "none", kind: "waive", hash: "-", ok: true, refusedWaive: true},
		{name: "a first word split by a vertical tab shapes as finding and parseWaive refuses it", event: "waive", target: "finding\vx y", text: "none", kind: "waive", hash: "-", ok: true, refusedWaive: true},
		{name: "a first word split by a no-break space shapes as finding and parseWaive refuses it", event: "waive", target: "finding\u00a0x y", text: "none", kind: "waive", hash: "-", ok: true, refusedWaive: true},

		{name: "a verify row records nothing", event: "verify", target: "task-1", text: "commit=" + sha},
		{name: "a no-commit verify row records nothing", event: "verify", target: "red-1", text: "no-commit exit=1 cmd=go test out=commit=" + sha},
		{name: "a review row records nothing", event: "review", target: "plan", text: "kind=plan entry=" + hash},
		{name: "a create row records nothing", event: "create", target: "intent.md", text: "intent.md"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kind, got, ok := ledgerRecordOf(tc.event, tc.target, evidenceOf(tc.event, tc.target, tc.text))
			if kind != tc.kind || got != tc.hash || ok != tc.ok {
				t.Errorf("ledgerRecordOf(%q, %q, %q) = (%q, %q, %v), want (%q, %q, %v)", tc.event, tc.target, tc.text, kind, got, ok, tc.kind, tc.hash, tc.ok)
			}
			if ok && (kind == "" || got == "") {
				t.Errorf("ok with an empty kind or hash: (%q, %q)", kind, got)
			}
			if tc.event == "waive" {
				if err := parseWaive(tc.target); (err != nil) != tc.refusedWaive {
					t.Errorf("parseWaive(%q) = %v, want refused %v", tc.target, err, tc.refusedWaive)
				}
			}
		})
	}
}

// TestLedgerWriterAgreesWithRecordOf runs every log step of the ledger and
// entry cases and requires the line the writer appended, or the absence of one,
// to equal what ledgerRecordOf says of the row the step wrote.
func TestLedgerWriterAgreesWithRecordOf(t *testing.T) {
	for _, group := range []func() []contractCase{ledgerCases, entryCases} {
		for _, c := range group() {
			t.Run(c.name, func(t *testing.T) {
				if !c.serial {
					t.Parallel()
				}
				fx := c.setup(t)
				for i, step := range c.steps {
					agreeStep(t, fx, step, i)
				}
			})
		}
	}
}

// agreeStep runs one step of a case as the contract table does, and when it is
// a log call compares the ledger with the shape of the row it wrote.
func agreeStep(t *testing.T, fx *contractFixture, step contractStep, index int) {
	t.Helper()
	if step.before != nil {
		step.before(t, fx)
	}
	args := step.args
	if step.argsOf != nil {
		args = step.argsOf(fx)
	}
	dir := filepath.Join(fx.repo, filepath.FromSlash(step.dir))
	if step.dirOf != nil {
		dir = step.dirOf(fx)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	logging := len(args) > 0 && args[0] == "log"
	state := stateFileOf(fx, args)
	ledger, inRepo := ledgerOf(fx, dir)
	rowsBefore, linesBefore := stateRowsIn(t, state), ledgerLinesIn(t, ledger)

	res := runFlow(t, dir, step.env, args...)
	what := "step " + strconv.Itoa(index+1) + " flow " + strings.Join(args, " ")
	mustExit(t, res, step.code, what)
	rowsAfter, linesAfter := stateRowsIn(t, state), ledgerLinesIn(t, ledger)
	if step.check != nil {
		step.check(t, fx, res)
	}
	if !logging {
		return
	}
	if !inRepo {
		if _, err := os.Lstat(ledgerPathOf(fx.repo)); !os.IsNotExist(err) {
			t.Errorf("%s: a ledger exists outside a repository (err %v)", what, err)
		}
	}
	if len(rowsAfter) == len(rowsBefore) {
		if len(linesAfter) != len(linesBefore) {
			t.Errorf("%s: wrote no row and added ledger lines %q", what, linesAfter[len(linesBefore):])
		}
		return
	}
	if len(rowsAfter) != len(rowsBefore)+1 {
		t.Fatalf("%s: %d rows became %d", what, len(rowsBefore), len(rowsAfter))
	}
	newest := rowsAfter[len(rowsAfter)-1]
	event, target, status := newest[3], newest[4], newest[5]
	kind, hash, ok := ledgerRecordOf(event, target, cellOf(stateRow(newest)))
	// The writer's own conditions, from the call and the stored row: an approval
	// is recorded only when guarded, and an approval or a waiver only when ok.
	switch event {
	case "approve":
		ok = ok && status == "ok" && slices.Contains(args, "--expect-blob")
	case "waive":
		ok = ok && status == "ok"
	}
	ok = ok && inRepo
	if !ok {
		if len(linesAfter) != len(linesBefore) {
			t.Errorf("%s: the %s row %q added ledger lines %q, want none", what, event, target, linesAfter[len(linesBefore):])
		}
		return
	}
	if len(linesAfter) != len(linesBefore)+1 {
		t.Fatalf("%s: the %s row %q added %d ledger lines, want one (%s %s)", what, event, target, len(linesAfter)-len(linesBefore), kind, hash)
	}
	fields := strings.Split(linesAfter[len(linesAfter)-1], "\t")
	if len(fields) != 5 {
		t.Fatalf("%s: ledger line %q has %d fields, want 5", what, linesAfter[len(linesAfter)-1], len(fields))
	}
	if fields[1] != kind || fields[3] != target || fields[4] != hash {
		t.Errorf("%s: ledger line kind %q target %q hash %q, want %q %q %q", what, fields[1], fields[3], fields[4], kind, target, hash)
	}
}

// stateFileOf is the state.tsv a log call writes: the one in an absolute
// --folder, else the fixture's.
func stateFileOf(fx *contractFixture, args []string) string {
	if i := slices.Index(args, "--folder"); i >= 0 && i+1 < len(args) && filepath.IsAbs(args[i+1]) {
		return filepath.Join(args[i+1], "state.tsv")
	}
	return fx.statePath()
}

// ledgerOf is the ledger of the repository that holds dir, read with the
// process environment as the flow's Git calls read it, and whether dir is in
// one that Git can reach.
func ledgerOf(fx *contractFixture, dir string) (string, bool) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return ledgerPathOf(fx.repo), false
	}
	return ledgerFile(filepath.Clean(filepath.FromSlash(strings.TrimSpace(string(out))))), true
}

func stateRowsIn(t *testing.T, path string) [][]string {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	var rows [][]string
	for _, line := range dataLines(readLines(t, path)) {
		c := cells(line)
		if len(c) != 9 {
			t.Fatalf("row has %d cells, want 9: %q", len(c), line)
		}
		rows = append(rows, c)
	}
	return rows
}

func ledgerLinesIn(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(body), "\n"), "\n")
}
