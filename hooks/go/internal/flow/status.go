package flow

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

func (p *program) status(args []string) int {
	if code := p.checkStorage(); code != 0 {
		return code
	}
	folder := ""
	metrics := false
	usageError := func() int { fmt.Fprint(p.stderr, usage); return 1 }
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--folder":
			if i+1 >= len(args) {
				return usageError()
			}
			i++
			folder = args[i]
		case "--metrics":
			metrics = true
		default:
			return usageError()
		}
	}
	top := p.repo.top()
	if folder == "" {
		found := p.lookupFlow()
		switch found.outcome {
		case lookupSmallFix:
			fmt.Fprintln(p.stdout, "small fix: "+found.branch)
			return 0
		case lookupAmbiguous:
			return 4
		case lookupNone:
			fmt.Fprintln(p.stderr, "flow: no flow found; ask which flow, or pass --folder")
			return 3
		}
		// The logical folder, so that locateFlow finds the flow in whichever
		// store holds it.
		folder = found.folder
	}
	loc, t, code := p.openTrail(folder)
	if code != 0 {
		return code
	}
	stored := loc.stored
	logical := loc.logical
	dir := stored.dir
	report := p.statusDetails(top, loc, t)
	invalid := t.invalidLines()
	sessions := make(map[string]bool)
	approvals := make(map[string]stateRow)
	// revoked holds the ts of each target's latest revoke row.
	revoked := make(map[string]string)
	for _, tr := range t.rows {
		row := tr.stateRow
		sessions[row[1]] = true
		if row[3] == "approve" && row[5] == "ok" {
			approvals[row[4]] = row
		}
		if row[3] == "revoke" {
			delete(approvals, row[4])
			if row[0] > revoked[row[4]] {
				revoked[row[4]] = row[0]
			}
		}
	}
	var targets []string
	for target := range approvals {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	var approvalLines, staleLines, unconfirmedLines []string
	for _, target := range targets {
		row := approvals[target]
		fields := statusFields(target, row[0], row[7])
		target, evidence := fields[0], fields[2]
		date, _, _ := strings.Cut(fields[1], "T")
		// The row's key names the rule its hash was written under. A row
		// without a recorded hash keeps the first rule, so its line reads as
		// it did before the second rule existed.
		key, want := evidenceOf("approve", target, evidence).approved()
		var current string
		if key == "text" {
			current, _ = fileTextHash(dir, target)
		} else {
			current = contentHashAt(p.dir, dir, loc.dir, target)
		}
		state := "stale"
		if current != "" && current == want {
			state = "current"
			// A revoke withdraws the consent every earlier record holds, so an
			// approve row with the same hash that arrives later by merge or copy
			// confirms nothing; only a record written after the revoke does. A
			// record in the revoke's own second is not after it.
			if recorded := t.recorded("approve", target, want); recorded == "" || recorded <= revoked[target] {
				state = "unconfirmed here"
				unconfirmedLines = append(unconfirmedLines, "unconfirmed: "+logical+"/"+target+" was approved "+date+", and this clone has no record of it. Present it and ask once; a yes writes a new row.")
			}
		}
		approvalLines = append(approvalLines, "approvals: "+target+" approved "+date+" "+state)
		if state != "stale" {
			continue
		}
		approved := "blob=unrecorded"
		if want != "" {
			approved = key + "=" + want
		}
		pointer := stalePointer(stored, p.dir, loc, target, key, want)
		next := "Review it, then approve it again; a phase that needs this approval will not start until you do."
		if target == "plan.md" {
			next = "Review it, then approve it again; execute will not start until you do."
		}
		if current == "" {
			current = "unavailable"
		}
		staleLines = append(staleLines, "stale: "+logical+"/"+target+" differs from the content approved "+date+" (approved "+approved+", current "+key+"="+current+"). "+pointer+". "+next)
	}
	fmt.Fprintln(p.stdout, "flow: "+logical)
	// The line names the store that holds this flow, not the setting, which
	// decides only where a new flow goes.
	fmt.Fprintln(p.stdout, "storage: "+stored.store)
	if len(t.rows) == 0 {
		fmt.Fprint(p.stdout, "phase: none (no rows)\nnext: none\n")
	} else {
		last := t.rows[len(t.rows)-1].stateRow
		fields := statusFields(last[2], last[3], last[5], last[8])
		fmt.Fprintf(p.stdout, "phase: %s (%s, %s)\nnext: %s\n", fields[0], fields[1], fields[2], fields[3])
	}
	for _, artifact := range textArtifacts {
		if hash, ok := fileTextHash(dir, artifact); ok {
			fmt.Fprintln(p.stdout, "hashes: "+artifact+" text="+hash)
		}
	}
	if len(approvalLines) == 0 {
		fmt.Fprintln(p.stdout, "approvals: none")
	} else {
		fmt.Fprintln(p.stdout, strings.Join(approvalLines, "\n"))
	}
	for _, line := range staleLines {
		fmt.Fprintln(p.stdout, line)
	}
	for _, line := range unconfirmedLines {
		fmt.Fprintln(p.stdout, line)
	}
	for _, line := range t.entryStatusLines() {
		fmt.Fprintln(p.stdout, line)
	}
	if len(report.blocks) == 0 {
		fmt.Fprintln(p.stdout, "blocks: none")
	} else {
		fmt.Fprintln(p.stdout, strings.Join(report.blocks, "\n"))
	}
	if report.mode != "" {
		fmt.Fprintln(p.stdout, report.mode)
	}
	fmt.Fprintln(p.stdout, report.slice)
	fmt.Fprintln(p.stdout, report.unaccounted)
	for _, line := range invalid {
		fmt.Fprintln(p.stdout, line)
	}
	for _, line := range t.waiverLines() {
		fmt.Fprintln(p.stdout, line)
	}
	fmt.Fprintf(p.stdout, "sessions: %d\n", len(sessions))
	if len(t.rows) > 0 {
		last := t.rows[len(t.rows)-1].stateRow
		fields := statusFields(last[0], last[1])
		fmt.Fprintf(p.stdout, "last: %s %s\n", fields[0], fields[1])
	}
	if metrics {
		fmt.Fprintln(p.stdout, strings.Join(report.measures, "\n"))
	}
	for _, mismatch := range report.mismatches {
		fmt.Fprintln(p.stdout, mismatch)
	}
	if t.header != stateHeaderRow {
		fmt.Fprintf(p.stdout, "mismatch: header row is '%s', expected '%s'\n", t.header, stateHeaderRow)
	}
	return 0
}

// headerLine returns the first line of a state.tsv body that does not start
// with "#", which names the columns: comment lines, such as the version line
// an older file starts with, come before it. It is empty when no such line
// exists.
func headerLine(body []byte) string {
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

// divergeRow is a diverge row that names a divergence entry by its hash.
type divergeRow struct{ target, hash string }

// rule reports whether the row records a rule divergence, whose target is a
// task or the run followed by "rule".
func (d divergeRow) rule() bool {
	target := strings.Fields(d.target)
	return len(target) == 2 && target[1] == "rule"
}

// The reference reads aggregate records with tab as shell IFS whitespace,
// collapsing empty fields. The read path checks only the evidence of verify
// rows and the targets of waive and demote rows, which the trail's
// invalid: lines report; every other cell is shown as written.
func statusFields(values ...string) []string {
	fields := make([]string, len(values))
	n := 0
	for _, value := range values {
		if value != "" {
			fields[n] = value
			n++
		}
	}
	return fields
}

// patchIDsIn returns a patch-id lookup for commits in dir that runs Git once
// per commit, and returns "" for a commit Git cannot read, so a row whose
// commit is gone stays unattested.
func patchIDsIn(dir string) func(string) string {
	known := make(map[string]string)
	return func(sha string) string {
		id, seen := known[sha]
		if !seen {
			id = strings.ToLower(patchIDOf(dir, sha))
			known[sha] = id
		}
		return id
	}
}

// statusReport holds the lines status prints from a flow's rows beyond the
// approvals and entries: the blocks: lines, the mode: line or "", the slice:
// and unaccounted: lines, the mismatch: lines, and the measures --metrics
// prints.
type statusReport struct {
	blocks      []string
	mode        string
	slice       string
	unaccounted string
	mismatches  []string
	measures    []string
}

// statusDetails reads the trail t of the located flow loc, checking a row's
// commit by running Git in the trail's Git directory. A row with a reason is
// no evidence, so it names no commit for unaccounted: and is never the row the
// commit check reads. The ledger attests the commit rows unaccounted: reads;
// the commit check, blocks and verdicts read commit rows whatever the ledger
// holds. loc.rel is the work folder's repository-relative path, empty outside
// the repository.
func (p *program) statusDetails(top string, loc locatedFlow, t *trail) (report statusReport) {
	// The bind readers take plain rows, built once.
	rows := t.stateRows()
	blocked := make(map[string]string)
	var ci, ai, as, cp, ap, gb string
	branch := ""
	if top != "" {
		branch = p.repo.branch()
	}
	var reworks []string
	reviews := make(map[string]int)
	var latency, blockedSeconds float64
	escapes := 0
	for _, tr := range t.rows {
		r := tr.stateRow
		event, target := r[3], r[4]
		ok := r[5] == "ok"
		switch event {
		case "block":
			blocked[target] = r[0]
		case "unblock":
			delete(blocked, target)
		}
		if event == "create" && target == "intent.md" && ci == "" {
			ci = r[0]
		}
		if event == "approve" && ok && target == "intent.md" && ai == "" {
			ai = r[0]
		}
		if event == "approve" && ok && target == "spec.md" && as == "" {
			as = r[0]
		}
		if event == "create" && target == "plan.md" && cp == "" {
			cp = r[0]
		}
		if event == "approve" && ok && target == "plan.md" && ap == "" {
			ap = r[0]
		}
		if event == "branch" && gb == "" {
			gb = r[0]
		}
		if ((event == "approve" && ok) || event == "revoke") && target == "spec.md" {
			reworks = append(reworks, r[0])
		}
		if event == "review" {
			cell := tr.cell
			if kind := cell.textRun("kind", asciiLower); kind != "" {
				reviews[kind]++
			}
			if dispatched := cell.textTimestamp("dispatched"); dispatched != "" {
				if elapsed := statusEpoch(r[0]) - statusEpoch(dispatched); elapsed > 0 {
					latency += elapsed
				}
			}
			if seconds := cell.textRun("blocked", asciiDigit); seconds != "" {
				blockedSeconds += statusNumber(seconds)
			}
		}
		if event == "escape" {
			escapes++
		}
	}
	var targets []string
	for target := range blocked {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		f := statusFields(target, blocked[target])
		date, _, _ := strings.Cut(f[1], "T")
		report.blocks = append(report.blocks, "blocks: "+f[0]+" blocked since "+date)
	}
	if name := runMode(rows, branch); name != "" {
		report.mode = "mode: " + name + " (" + branch + ")"
	}
	// The default ref and the branch's commits are read at most once, and only
	// when a check below needs them.
	var tip string
	tipRead := false
	defaultTip := func() string {
		if !tipRead && top != "" {
			tip = p.repo.defaultRef()
		}
		tipRead = true
		return tip
	}
	var branchCommits []rangeCommit
	branchRead := false
	commitsOnBranch := func() []rangeCommit {
		if !branchRead && defaultTip() != "" {
			base := gitText(top, "merge-base", tip, "HEAD")
			if base == "" {
				// Without a common commit every commit of HEAD is on the branch.
				base = tip
			}
			branchCommits = rangeCommits(top, "HEAD", base)
		}
		branchRead = true
		return branchCommits
	}
	// The default branch reads as not bound even when a hand-written bind row
	// names it, since bind never binds it and no slice work lands there.
	report.slice = "slice: not bound"
	if ids, bound := bindSlice(rows, branch); bound && branch != "" && !isDefaultBranch(branch, defaultTip()) {
		if ids == "" {
			ids = "all"
		}
		report.slice = "slice: " + ids + " (" + branch + ")"
	}
	report.unaccounted = "unaccounted: not computed (this branch is not bound to the flow)"
	if branch != "" && latestBind(rows, branch) != nil {
		// A bind row for the default branch, which only a hand-written row can
		// make, is computed too: the branch is bound, and its range from the
		// merge base holds no commit unless HEAD is ahead of the default ref.
		if defaultTip() == "" {
			report.unaccounted = "unaccounted: not computed (no default branch)"
		} else {
			report.unaccounted = "unaccounted: none"
			evidence := t.accounting(patchIDsIn(t.gitDir))
			if open := unaccountedCommits(commitsOnBranch(), evidence, loc.rel, rangeLister(top)); len(open) > 0 {
				line := "unaccounted: " + strconv.Itoa(len(open))
				for _, sha := range open[:min(len(open), 20)] {
					line += " " + sha[:min(len(sha), 7)]
				}
				report.unaccounted = line
			}
		}
	}
	report.mismatches = mismatchLines(t.commitChecks(), t.gitDir, func() map[string]bool { return patchIDSet(commitsOnBranch()) }, nil)
	rework := "n/a"
	if cp != "" {
		count := 0
		for _, ts := range reworks {
			if ts > cp {
				count++
			}
		}
		rework = strconv.Itoa(count)
	}
	var reviewCounts []string
	for _, kind := range strings.Fields(reviewKinds) {
		if reviews[kind] != 0 {
			reviewCounts = append(reviewCounts, kind+"="+strconv.Itoa(reviews[kind]))
		}
	}
	reviewLine := strings.Join(reviewCounts, " ")
	if reviewLine == "" {
		reviewLine = "none"
	}
	report.measures = []string{
		"time-to-intent: " + statusInterval(ci, ai), "intent-to-spec: " + statusInterval(ai, as), "spec-rework: " + rework, "plan-to-verdict: " + statusInterval(ap, gb),
		"reviews: " + reviewLine, "review-time: latency=" + statusNumericText(latency) + "s blocked=" + statusNumericText(blockedSeconds) + "s", "escapes: " + strconv.Itoa(escapes),
	}
	return report
}

// bindSlice returns the slice ids the latest bind row of branch carries, ""
// when it carries none, and whether branch has a bind row at all.
func bindSlice(rows []stateRow, branch string) (ids string, bound bool) {
	if latestBind(rows, branch) == nil {
		return "", false
	}
	return boundSlices(rows, branch), true
}

// runMode returns the mode= value of the last enter run row whose branch= is
// branch, "unknown" when that row holds none, and "" when no row names branch
// or branch is "". Each branch records its own run mode, so another branch's
// row says nothing about this one.
func runMode(rows []stateRow, branch string) string {
	evidence := ""
	for _, r := range rows {
		if r[3] == "enter" && r[4] == "run" && branch != "" && cellOf(r).textLast("branch") == branch {
			evidence = r[7]
		}
	}
	if evidence == "" {
		return ""
	}
	if name := evidenceOf("enter", "run", evidence).textLast("mode"); name != "" {
		return name
	}
	return "unknown"
}

// mismatchLines returns a mismatch: line for each of selected whose commit is
// not an ancestor of HEAD and whose patch-id, when it has one, is not in the
// set patchIDs returns: the patch-ids of the range being measured. patchIDs is
// called at most once, and only when a row needs it. A row lowerLayer reports,
// when lowerLayer is not nil, belongs to a layer below the measured range and
// prints nothing. Git runs in gitDir. The cell is read again from the fields
// as the shell reading assigns them, which shift when the event or target is
// empty, so a row's parsed cell is not used here.
func mismatchLines(selected []trailRow, gitDir string, patchIDs func() map[string]bool, lowerLayer func(trailRow) bool) []string {
	var lines []string
	var ids map[string]bool
	for _, row := range selected {
		r := row.stateRow
		f := statusFields(r[3], r[4], r[7])
		cell := evidenceOf(f[0], f[1], f[2])
		sha := cell.textLast("commit")
		if _, err := gitBytes(gitDir, nil, "merge-base", "--is-ancestor", sha, "HEAD"); err == nil {
			continue
		}
		if lowerLayer != nil && lowerLayer(row) {
			continue
		}
		// A row without a patch-id, such as one for a merge commit or a
		// hand-written wave or final row, is checked by ancestry alone.
		line := "mismatch: " + f[0] + " " + f[1] + " commit " + sha + " is not an ancestor of HEAD"
		if cell.textHasValue("patch-id") {
			id := cell.textLast("patch-id")
			if ids == nil {
				ids = patchIDs()
			}
			if ids[id] {
				continue
			}
			line += " and patch-id " + id + " matches no commit on this branch"
		}
		lines = append(lines, line)
	}
	return lines
}

// patchIDSet returns the patch-ids of commits, leaving out the empty id of a
// commit that changes nothing. It is never nil.
func patchIDSet(commits []rangeCommit) map[string]bool {
	ids := make(map[string]bool)
	for _, commit := range commits {
		if commit.patchID != "" {
			ids[commit.patchID] = true
		}
	}
	return ids
}

// rangeLister returns a function that lists the commits of base..head, run in
// top, for the ranges a row's base= and head= pair records.
func rangeLister(top string) func(base, head string) map[string]bool {
	return func(base, head string) map[string]bool {
		held := make(map[string]bool)
		for _, sha := range strings.Fields(gitText(top, "rev-list", head, "^"+base, "--")) {
			held[sha] = true
		}
		return held
	}
}

func asciiLower(b byte) bool { return b >= 'a' && b <= 'z' }
func asciiDigit(b byte) bool { return b >= '0' && b <= '9' }

func statusNumber(text string) float64 {
	text = strings.TrimLeft(text, " \t\r\n\v\f")
	if text == "" {
		return 0
	}
	first := text[0]
	if first == '+' || first == '-' {
		if len(text) == 1 {
			return 0
		}
		first = text[1]
	}
	if !asciiDigit(first) && first != '.' {
		return 0
	}
	for end := len(text); end > 0; end-- {
		value, err := strconv.ParseFloat(text[:end], 64)
		if err == nil || math.IsInf(value, 0) {
			return value
		}
	}
	return 0
}

// Keep the reference's arithmetic normalization of impossible date fields.
// Rejecting them with a calendar parser would change existing review totals.
func statusEpoch(text string) float64 {
	part := func(start, length int) float64 {
		if start >= len(text) {
			return 0
		}
		end := start + length
		if end > len(text) {
			end = len(text)
		}
		return statusNumber(text[start:end])
	}
	y, m, d := part(0, 4), part(5, 2), part(8, 2)
	h, minute, second := part(11, 2), part(14, 2), part(17, 2)
	if m <= 2 {
		y--
	}
	era := math.Trunc(y / 400)
	yoe := y - era*400
	mm := m + 9
	if m > 2 {
		mm = m - 3
	}
	doy := math.Trunc((153*mm+2)/5) + d - 1
	doe := yoe*365 + math.Trunc(yoe/4) - math.Trunc(yoe/100) + doy
	return (era*146097+doe-719468)*86400 + h*3600 + minute*60 + second
}

// Aggregates follow 64-bit Linux mawk's integer printing range. The unsigned
// maximum rounds up in float64, but mawk clamps its rendering to that maximum.
func statusNumericText(value float64) string {
	const unsignedLimit = 18446744073709551616.0
	if value == unsignedLimit {
		return "18446744073709551615"
	}
	if value >= -9223372036854775808 && value < unsignedLimit && value == math.Trunc(value) {
		return strconv.FormatFloat(value, 'f', 0, 64)
	}
	if math.IsInf(value, 1) {
		return "inf"
	}
	if math.IsInf(value, -1) {
		return "-inf"
	}
	if math.IsNaN(value) {
		return "-nan"
	}
	return strconv.FormatFloat(value, 'g', 6, 64)
}

func statusInterval(start, end string) string {
	if start == "" || end == "" {
		return "n/a"
	}
	return statusNumericText(statusEpoch(end)-statusEpoch(start)) + "s"
}
