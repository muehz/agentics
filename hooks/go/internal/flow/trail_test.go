package flow

import (
	"path/filepath"
	"strings"
	"testing"
)

// rowCells splits a row written by row into the cells of a state row.
func rowCells(line string) stateRow { return stateRow(strings.Split(line, "\t")) }

// TestTrailCommitAttested pins when a commit row accounts for its commit: one
// commit= value that this clone's ledger records for the row's target under
// the flow's key, at most one patch-id=, which must be that commit's own, and
// slice= tags. patchID runs only for a row the ledger already records.
func TestTrailCommitAttested(t *testing.T) {
	const (
		key     = "docs/specs/2026-11/feature-c"
		sha     = "0123456789abcdef0123456789abcdef01234567"
		other   = "89abcdef0123456789abcdef0123456789abcdef"
		ownID   = "1111111111111111111111111111111111111111"
		otherID = "2222222222222222222222222222222222222222"
	)
	ids := map[string]string{sha: ownID, other: otherID}
	ledger := map[ledgerEntry]string{
		{"commit", key, "user", sha}:                               "2026-11-01T10:00:00Z",
		{"commit", "docs/specs/2026-11/feature-other", "fix", sha}: "2026-11-01T10:00:00Z",
	}
	tests := []struct {
		name, target, evidence string
		want                   bool
		// patchID is whether the lookup of the commit's patch-id may run.
		patchID bool
	}{
		{name: "a recorded sha alone is attested", target: "user", evidence: "commit=" + sha, want: true},
		{name: "a recorded sha with its own patch-id is attested", target: "user", evidence: "commit=" + sha + " patch-id=" + ownID, want: true, patchID: true},
		{name: "the sha is read without case", target: "user", evidence: "commit=" + strings.ToUpper(sha), want: true},
		{name: "the patch-id is read without case", target: "user", evidence: "commit=" + sha + " patch-id=" + strings.ToUpper(ownID), want: true, patchID: true},
		{name: "slice tags change nothing", target: "user", evidence: "commit=" + sha + " slice=S1,S2", want: true},
		{name: "a bare slice word counts as a key", target: "user", evidence: "commit=" + sha + " slice", want: true},
		{name: "a wrong patch-id is not attested", target: "user", evidence: "commit=" + sha + " patch-id=" + otherID, patchID: true},
		{name: "two patch-ids are not attested", target: "user", evidence: "commit=" + sha + " patch-id=" + ownID + " patch-id=" + ownID},
		{name: "two shas are not attested", target: "user", evidence: "commit=" + sha + " commit=" + other},
		{name: "a bare commit word is a second sha", target: "user", evidence: "commit=" + sha + " commit"},
		{name: "a base= and head= range is not attested", target: "user", evidence: "commit=" + sha + " base=" + other + " head=" + sha},
		{name: "an abbreviated sha is not attested", target: "user", evidence: "commit=" + sha[:7]},
		{name: "a sha the ledger does not record is not attested", target: "user", evidence: "commit=" + other},
		{name: "a record for another target does not attest", target: "fix", evidence: "commit=" + sha},
		{name: "a row without commit= is not attested", target: "user", evidence: "patch-id=" + ownID},
		{name: "an empty cell is not attested", target: "user", evidence: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := rowCells(row("2026-11-01T11:00:00Z", "execute", "commit", tc.target, "ok", "by hand", tc.evidence, "execute go"))
			tr := newTrail(locatedFlow{ledgerKey: key}, headerRow, []stateRow{r}, t.TempDir(), "")
			tr.load = func() map[ledgerEntry]string { return ledger }
			looked := 0
			patchID := func(sha string) string { looked++; return ids[sha] }
			if got := tr.commitAttested(tr.rows[0], patchID); got != tc.want {
				t.Errorf("commitAttested(%q) = %v, want %v", tc.evidence, got, tc.want)
			}
			if looked > 0 && !tc.patchID {
				t.Errorf("commitAttested(%q) looked up a patch-id %d times, want none", tc.evidence, looked)
			}
		})
	}
}

// TestTrailReadsTheLedgerOnce counts the reads of the ledger a trail makes: one
// for a trail that asks every question that needs it, an empty answer kept as
// the answer, and none for a trail that asks only for its invalid: lines.
func TestTrailReadsTheLedgerOnce(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "plan.md", "# Plan\n\n## Standing orders\n\n1. Keep the API stable.\n\n## Divergences\n\n- **d1** — added `other/x.txt`.\n")
	orders, _ := artifactEntries(dir, "plan.md", standingOrdersHeading)
	divergences, _ := artifactEntries(dir, "plan.md", divergencesHeading)
	if len(orders) != 1 || len(divergences) != 1 {
		t.Fatalf("setup: %d orders and %d divergences, want one of each", len(orders), len(divergences))
	}
	const sha = "0123456789abcdef0123456789abcdef01234567"
	rows := []stateRow{
		rowCells(row("2026-11-01T10:00:00Z", "execute", "order", "1", "ok", "ordered", "entry="+orders[0].hash, "execute go")),
		rowCells(row("2026-11-01T10:01:00Z", "execute", "diverge", "task-1 rule", "ok", "approved", "none entry="+divergences[0].hash, "execute go")),
		rowCells(row("2026-11-01T10:02:00Z", "execute", "waive", "finding audit src/a.go:3", "ok", "waived", "none", "execute go")),
		rowCells(row("2026-11-01T10:03:00Z", "execute", "commit", "user", "ok", "user commit", "commit="+sha, "execute go")),
		rowCells(row("2026-11-01T10:04:00Z", "execute", "verify", "task-1", "ok", "green", "none", "execute go")),
	}
	located := locatedFlow{ledgerKey: "docs/specs/2026-11/feature-c", stored: storedFolder{dir, storageCommitted}}
	for _, held := range []map[ledgerEntry]string{{}, {{"commit", located.ledgerKey, "user", sha}: "2026-11-01T10:30:00Z"}} {
		tr := newTrail(located, headerRow, rows, dir, "")
		reads := 0
		tr.load = func() map[ledgerEntry]string { reads++; return held }
		tr.recorded("approve", "plan.md", sha)
		tr.waiverLines()
		tr.entryStatusLines()
		tr.accounting(func(string) string { return "" })
		tr.divergenceRecorded()("plan.md", divergences[0].hash)
		tr.recorded("commit", "user", sha)
		if reads != 1 {
			t.Errorf("with %d ledger entries the trail read the ledger %d times, want 1", len(held), reads)
		}
	}
	tr := newTrail(located, headerRow, rows, dir, filepath.Join(dir, "no-such-common"))
	reads := 0
	tr.load = func() map[ledgerEntry]string { reads++; return nil }
	if lines := tr.invalidLines(); len(lines) != 1 {
		t.Errorf("invalidLines() = %q, want the verify row's line", lines)
	}
	if reads != 0 {
		t.Errorf("invalidLines read the ledger %d times, want none", reads)
	}
}
