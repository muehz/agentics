package flow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// trailRow is one state row with its evidence cell, read once, and the reason
// it fits no documented form, or "" when it fits one or is not a verify, waive,
// or demote row.
type trailRow struct {
	stateRow
	cell   evidenceCell
	reason string
}

// trail is a flow's rows as status and audit read them: each row with its cell
// and validity, plus this clone's ledger, which attests the rows written here.
//
// flow is the located flow with stored set to the store the rows were read
// from, which for the unassigned folder may differ from the store a write
// goes to. The flow's files are read in flow.stored.dir and Git runs in
// gitDir. The ledger of the common directory common is read at most once, and
// only when a method needs it, through load, so that a test can count the
// reads or give the ledger as a map. A trail is not safe for concurrent use.
type trail struct {
	flow   locatedFlow
	header string
	rows   []trailRow
	gitDir string
	common string

	load    func() map[ledgerEntry]string
	entries map[ledgerEntry]string
	loaded  bool
}

// newTrail reads rows into a trail of flow. Each row's reason comes from
// invalidRowReasons, whose merge test of every bare commit value runs in one
// Git call in gitDir, and each row's cell is parsed once.
func newTrail(flow locatedFlow, header string, rows []stateRow, gitDir, common string) *trail {
	reasons := invalidRowReasons(gitDir, rows)
	t := &trail{flow: flow, header: header, gitDir: gitDir, common: common, rows: make([]trailRow, len(rows))}
	for i, r := range rows {
		t.rows[i] = trailRow{stateRow: r, cell: cellOf(r), reason: reasons[i]}
	}
	t.load = func() map[ledgerEntry]string { return readLedger(common) }
	return t
}

// invalidRowReasons returns, by index, why each of rows fits no form, or ""
// for a row that fits one or is not a verify, waive, or demote row. The
// invalid: lines and the evidence that status counts read this one slice, so a row
// that a line names is exactly a row the evidence skips. Git runs in gitDir,
// as for the ancestry check. The merge test of every bare commit= value runs
// in one Git call, so the cost of status does not grow with those rows.
func invalidRowReasons(gitDir string, rows []stateRow) []string {
	reasons := make([]string, len(rows))
	var bareRows []int
	var bareValues []string
	for i, r := range rows {
		reason, bare := invalidRowReason(r)
		reasons[i] = reason
		if bare != "" {
			bareRows = append(bareRows, i)
			bareValues = append(bareValues, bare)
		}
	}
	for k, reason := range bareCommitReasons(gitDir, "commit", bareValues) {
		reasons[bareRows[k]] = reason
	}
	return reasons
}

// invalidRowReason returns why row r fits no form, or "" when it fits one or
// is not a verify, waive, or demote row. A commit-form value without a
// patch-id fits only when it names a merge, which the shape cannot show, so
// that value is returned as bare for the caller's merge test. A snapshot hash
// written with a patch-id is not resolved: the commit check reports it as a
// mismatch.
func invalidRowReason(r stateRow) (reason, bare string) {
	var err error
	switch r[3] {
	case "verify":
		var parsed verifyEvidence
		if parsed, err = parseVerify(r[4], r[5], r[7]); err == nil && parsed.form == formCommit && parsed.patchID == "" {
			return "", parsed.commit
		}
		var cell *cellError
		if errors.As(err, &cell) {
			return cell.reason, ""
		}
	case "waive":
		err = parseWaive(r[4])
	case "demote":
		err = parseDemote(r[4])
	}
	if err != nil {
		return err.Error(), ""
	}
	return "", ""
}

// openTrail locates folder and reads its state.tsv into a trail. When it
// cannot, it prints why and returns the exit code status gives for that
// failure: 2 or 4 from locateFlow, 3 for no state.tsv, 1 for one it cannot
// read. The unassigned folder is a log each clone may keep in either store, so
// its rows are read from both files and its store is the local one whenever
// that holds a file; the located flow it returns carries that store and the
// Git directory that follows from it.
func (p *program) openTrail(folder string) (locatedFlow, *trail, int) {
	loc, err := p.locateFlow(folder)
	if err != nil {
		return locatedFlow{}, nil, p.locateFailed(err)
	}
	var merged []stateRow
	if loc.unassigned {
		var local string
		merged, local = p.unassignedRows(loc.dir)
		loc.stored, loc.gitDir = storedFolder{loc.dir, storageCommitted}, loc.dir
		if local != "" && holdsFlow(local) {
			// Git never runs in the local store, where it would answer for the
			// main checkout's HEAD.
			loc.stored, loc.gitDir = storedFolder{local, storageLocal}, p.repo.top()
		}
	}
	file := filepath.Join(loc.stored.dir, "state.tsv")
	if info, err := os.Stat(file); err != nil || !info.Mode().IsRegular() {
		fmt.Fprintln(p.stderr, "flow: no state.tsv in "+loc.logical)
		return locatedFlow{}, nil, 3
	}
	body, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(p.stderr, "flow: cannot read %s/state.tsv: %v\n", loc.logical, err)
		return locatedFlow{}, nil, 1
	}
	rows := rowsOf(file)
	if loc.unassigned {
		rows = merged
	}
	return loc, newTrail(loc, headerLine(body), rows, loc.gitDir, p.repo.common()), 0
}

// smallFixTrail reads the unassigned folder's rows from both stores, as the
// lookup that found a small fix read them, into a trail whose Git runs in the
// repository root. A refusal of the folder does not stop the read: the small
// fix's audit refuses nothing, and the located flow still gives its ledger key.
func (p *program) smallFixTrail() *trail {
	top := p.repo.top()
	rows, _ := p.unassignedRows(filepath.Join(p.rootPath(top), "unassigned"))
	loc, _ := p.locateFlow(p.unassignedName())
	return newTrail(loc, "", rows, top, p.repo.common())
}

// within returns a trail of the same flow that holds only rows, which are rows
// of t, with their cells and reasons as t read them, so a small fix's record
// is checked as if its rows were the whole trail. It runs no Git, and it reads
// the ledger through t, so the two read it at most once between them.
func (t *trail) within(rows []trailRow) *trail {
	return &trail{flow: t.flow, header: t.header, rows: rows, gitDir: t.gitDir, common: t.common, load: t.ledger}
}

// stateRows returns the trail's rows without their cells, for the readers
// that take plain rows.
func (t *trail) stateRows() []stateRow {
	rows := make([]stateRow, len(t.rows))
	for i, r := range t.rows {
		rows[i] = r.stateRow
	}
	return rows
}

// ledger returns the ledger's entries, read on the first call. An empty
// ledger is an answer too, so it is not read again.
func (t *trail) ledger() map[ledgerEntry]string {
	if !t.loaded {
		t.entries, t.loaded = t.load(), true
	}
	return t.entries
}

// recorded returns the latest timestamp of the ledger line of kind, target,
// and hash under the flow's ledger key, or "" when this clone holds none.
func (t *trail) recorded(kind, target, hash string) string {
	return t.ledger()[ledgerEntry{kind, t.flow.ledgerKey, target, hash}]
}

// invalidLines returns an invalid: line for each verify, waive, and demote row
// that fits no documented form, in row order and without a cap, so that every
// row a reader must skip is named.
func (t *trail) invalidLines() []string {
	var lines []string
	for i := range t.rows {
		if t.rows[i].reason != "" {
			line, _ := t.invalidLine(i)
			lines = append(lines, line)
		}
	}
	return lines
}

// invalidLine returns the invalid: line of row i, which has a reason, and
// whether a later valid row replaced it. Only a verify row can be replaced, by
// the first later valid row for the same target; a waive or demote row's
// target is what is wrong, so a corrected row names a different target.
func (t *trail) invalidLine(i int) (string, bool) {
	r := t.rows[i]
	session := r.stateRow[1]
	if session == "" {
		session = "-"
	}
	line := "invalid: " + r.stateRow[0] + " " + session + " " + r.stateRow[3] + " " + r.stateRow[4] + ": " + r.reason
	if r.stateRow[3] == "verify" {
		for _, later := range t.rows[i+1:] {
			if later.stateRow[3] == r.stateRow[3] && later.stateRow[4] == r.stateRow[4] && later.reason == "" {
				return line + " (superseded " + later.stateRow[0] + ")", true
			}
		}
	}
	return line, false
}

// valid returns the rows that fit a form, in row order: the rows that may name
// a commit, whether or not they count for it.
func (t *trail) valid() []trailRow {
	var rows []trailRow
	for _, r := range t.rows {
		if r.reason == "" {
			rows = append(rows, r)
		}
	}
	return rows
}

// commitAttested reports whether commit row r holds the evidence flow log
// writes for a commit this clone's ledger records: one commit= value, a full
// sha a commit line of the ledger records for r's target, at most one
// patch-id=, which must be that commit's own, and slice= tags. The ledger
// records the sha alone, so a row that paired an attested sha with another
// commit's patch-id= or with a base= and head= range would otherwise account
// for commits the ledger never saw. The ledger holds full shas, so an
// abbreviated value stays unattested without a Git call; patchID runs only for
// a row the ledger already records.
func (t *trail) commitAttested(r trailRow, patchID func(string) string) bool {
	var sha, id string
	shas, ids := 0, 0
	// A bare slice, commit, or patch-id word counts as a key, so every token is read.
	for _, token := range r.cell.pairs() {
		switch token.key {
		case "commit":
			sha, shas = strings.ToLower(token.value), shas+1
		case "patch-id":
			id, ids = strings.ToLower(token.value), ids+1
		case "slice":
		default:
			return false
		}
	}
	if shas != 1 || ids > 1 || t.recorded("commit", r.stateRow[4], sha) == "" {
		return false
	}
	return id == "" || id == patchID(sha)
}

// accounting returns the rows that may account for a commit: those that fit a
// form and, of commit rows, only those commitAttested attests. A commit row
// exists to account for a commit, so one that arrived by merge or was written
// by hand would otherwise clear any commit it names. Such a row is dropped
// whole, so its patch-id= and ranges account for nothing either.
func (t *trail) accounting(patchID func(string) string) []trailRow {
	var rows []trailRow
	for _, r := range t.rows {
		if r.reason != "" {
			continue
		}
		if r.stateRow[3] == "commit" && !t.commitAttested(r, patchID) {
			continue
		}
		rows = append(rows, r)
	}
	return rows
}

// commitChecks returns the rows whose commit the commit check reads, in the
// order of their event and target: the latest valid row of each event and
// target whose evidence holds commit=, unless a later verdict retired it. An
// invalid row leaves the latest valid row for its target in place. The commit
// check reads these whatever the ledger holds.
func (t *trail) commitChecks() []trailRow {
	type commitRecord struct {
		row   trailRow
		index int
	}
	// verdict is a row that turns earlier commit rows into history: every one
	// when all is set, and otherwise those that share one of its slice ids.
	type verdict struct {
		index int
		all   bool
		ids   []string
	}
	commits := make(map[string]commitRecord)
	var verdicts []verdict
	for i, r := range t.rows {
		switch r.stateRow[3] {
		case "branch":
			// A failed verdict proves nothing about the commits before it, and a
			// slice verdict speaks only for the commits of its slices.
			if r.stateRow[5] == "ok" {
				verdicts = append(verdicts, verdict{i + 1, !r.cell.has("slice"), r.cell.list("slice")})
			}
		case "release", "change":
			verdicts = append(verdicts, verdict{index: i + 1, all: true})
		}
		if r.reason == "" {
			commits[r.stateRow[3]+"\t"+r.stateRow[4]] = commitRecord{r, i + 1}
		}
	}
	var keys []string
	for key, rec := range commits {
		// A review row's commit= pair names what the review covered, and a no-commit
		// cell quotes a command; neither claims a commit of this branch.
		if rec.row.stateRow[3] == "review" || rec.row.cell.quoted {
			continue
		}
		retired := false
		for _, v := range verdicts {
			retired = retired || v.index >= rec.index && (v.all || sharesSliceID(v.ids, rec.row.cell))
		}
		if !retired && rec.row.cell.textHasValue("commit") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	selected := make([]trailRow, len(keys))
	for i, key := range keys {
		selected[i] = commits[key].row
	}
	return selected
}

// sharesSliceID reports whether the slice= pair of cell lists one of ids.
func sharesSliceID(ids []string, cell evidenceCell) bool {
	for _, id := range cell.list("slice") {
		for _, want := range ids {
			if id == want {
				return true
			}
		}
	}
	return false
}

// waiverLines returns a waivers: line for each finding waiver that fits its
// form, in row order, saying whether this clone's ledger holds the record flow
// log writes for it. A row copied or committed in from another clone reads as
// unattested, because the ledger never leaves the clone that wrote it. The
// head is cut at an ASCII space, which agrees with the writer's split on every
// valid row.
func (t *trail) waiverLines() []string {
	var lines []string
	for _, r := range t.rows {
		if r.stateRow[3] != "waive" || r.reason != "" {
			continue
		}
		if head, _, _ := strings.Cut(r.stateRow[4], " "); head != "finding" {
			continue
		}
		state := "unattested"
		if t.recorded("waive", r.stateRow[4], "-") != "" {
			state = "attested"
		}
		date, _, _ := strings.Cut(r.stateRow[0], "T")
		lines = append(lines, "waivers: "+r.stateRow[4]+" "+date+" "+state)
	}
	return lines
}

// divergeRecords returns, in row order, every diverge row that names an entry.
func (t *trail) divergeRecords() []divergeRow {
	var found []divergeRow
	for _, r := range t.rows {
		if hash := r.cell.first("entry"); r.stateRow[3] == "diverge" && hash != "" {
			found = append(found, divergeRow{r.stateRow[4], hash})
		}
	}
	return found
}

// entryStatusLines returns the orders:, divergences: and unrecorded: lines of
// the flow. Rows name their entry by the hash of its text without the list
// marker, so an entry keeps its record when the list is renumbered or its
// marker changes, and loses it when its text changes.
func (t *trail) entryStatusLines() []string {
	dir := t.flow.stored.dir
	orderTargets := make(map[string][]string)
	for _, r := range t.rows {
		if hash := r.cell.first("entry"); r.stateRow[3] == "order" && hash != "" {
			orderTargets[hash] = append(orderTargets[hash], r.stateRow[4])
		}
	}
	diverged := make(map[string]bool)
	var rules []divergeRow
	for _, d := range t.divergeRecords() {
		diverged[d.hash] = true
		if d.rule() {
			rules = append(rules, d)
		}
	}
	var unrecorded []string
	// A standing order binds work only in a clone that holds its own record of
	// it, as an approval does, so the row alone does not record it.
	orders, _ := artifactEntries(dir, "plan.md", standingOrdersHeading)
	ordersRecorded := 0
	for _, entry := range orders {
		recorded := false
		for _, target := range orderTargets[entry.hash] {
			recorded = recorded || t.recorded("order", target, entry.hash) != ""
		}
		if recorded {
			ordersRecorded++
		} else {
			unrecorded = append(unrecorded, "unrecorded: plan.md standing order "+strconv.Itoa(entry.number))
		}
	}
	divergencesRecorded, divergencesUnrecorded := 0, 0
	for _, artifact := range textArtifacts {
		entries, _ := artifactEntries(dir, artifact, divergencesHeading)
		for _, entry := range entries {
			switch {
			case entry.reconciled:
			case diverged[entry.hash]:
				divergencesRecorded++
			default:
				divergencesUnrecorded++
				unrecorded = append(unrecorded, "unrecorded: "+artifact+" divergence "+strconv.Itoa(entry.number))
			}
		}
	}
	// A rule divergence changes how later work is checked, so it needs this
	// clone's record even after its entry is reconciled.
	for _, rule := range rules {
		if t.recorded("rule", rule.target, rule.hash) == "" {
			divergencesUnrecorded++
			unrecorded = append(unrecorded, "unrecorded: "+divergeArtifact(dir, rule.target)+" rule divergence "+rule.target)
		}
	}
	lines := []string{"orders: none", "divergences: none"}
	if len(orders) > 0 {
		lines[0] = "orders: " + strconv.Itoa(ordersRecorded) + " recorded, " + strconv.Itoa(len(orders)-ordersRecorded) + " unrecorded"
	}
	if divergencesRecorded+divergencesUnrecorded > 0 {
		lines[1] = "divergences: " + strconv.Itoa(divergencesRecorded) + " recorded, " + strconv.Itoa(divergencesUnrecorded) + " unrecorded"
	}
	return append(lines, unrecorded...)
}

// divergenceRecorded returns whether a row records the divergence entry of
// artifact whose hash is hash: a diverge row names that hash and targets that
// artifact. A rule divergence changes how later work is checked, so its row
// also needs this clone's ledger record, as entryStatusLines requires before
// it stops listing the rule as unrecorded.
func (t *trail) divergenceRecorded() func(artifact, hash string) bool {
	dir := t.flow.stored.dir
	records := t.divergeRecords()
	return func(artifact, hash string) bool {
		for _, d := range records {
			if d.hash != hash || divergeArtifact(dir, d.target) != artifact {
				continue
			}
			if !d.rule() || t.recorded("rule", d.target, d.hash) != "" {
				return true
			}
		}
		return false
	}
}
