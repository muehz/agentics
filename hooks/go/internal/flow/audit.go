package flow

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"slices"
	"strconv"
	"strings"
)

// audit reports, for the commits of <base>..HEAD, every commit no record
// accounts for, every recorded commit that no longer matches, every changed
// path outside the flow's scope, and every invalid verify row. It measures
// from the base it is given and never reads the default branch, so a stacked
// branch is audited against the layer below it. It only reads: no row and no
// ledger line is written, and it exits 0 whatever it finds. On a branch with
// an open small fix and no bound flow, it audits the branch's small-fix record
// in the unassigned folder instead, as smallFixRecord defines it.
func (p *program) audit(args []string) int {
	folder, base := "", ""
	baseGiven := false
	for i := 0; i < len(args); i++ {
		if (args[i] != "--folder" && args[i] != "--base") || i+1 >= len(args) {
			fmt.Fprint(p.stderr, usage)
			return 1
		}
		if args[i] == "--folder" {
			folder = args[i+1]
		} else {
			base, baseGiven = args[i+1], true
		}
		i++
	}
	if code := p.checkStorage(); code != 0 {
		return code
	}
	top := p.repo.top()
	if !baseGiven {
		fmt.Fprintln(p.stderr, "flow: audit needs --base <sha>")
		return 2
	}
	baseSHA := p.resolveBase(top, base)
	if baseSHA == "" {
		fmt.Fprintln(p.stderr, "flow: --base "+base+" names no commit")
		return 2
	}
	// A small fix is audited only when the lookup finds no bound flow, so a
	// flow bound to the branch wins over an open skip row, as in flow where.
	smallFix := false
	branch := ""
	if top != "" {
		branch = p.repo.branch()
	}
	if folder == "" {
		found := p.lookupFlow()
		switch found.outcome {
		case lookupFound:
			folder = found.folder
		case lookupSmallFix:
			smallFix = true
		case lookupAmbiguous:
			return 4
		default:
			fmt.Fprintln(p.stderr, "flow: no flow found")
			return 3
		}
	}
	var (
		t          *trail
		workFolder string
		// rows holds the trail's rows without their cells, for the bind
		// readers, built once.
		rows []stateRow
	)
	// workOnly reports whether every path a commit changes is a file the flow
	// itself writes, which needs no row.
	var workOnly func(paths []string) bool
	if smallFix {
		// Both stores may hold the unassigned file, so the rows of both are
		// read, as the lookup that found the small fix read them, and Git runs
		// in the repository root. That folder logs every branch's small fixes,
		// so only its trail file, and no other file beside it, is this
		// record's own. This branch refuses nothing, so a folder locateFlow
		// refuses still gives its ledger key.
		folder = p.unassignedName()
		t = p.smallFixTrail()
		trail := folder + "/state.tsv"
		workOnly = func(paths []string) bool {
			return len(paths) > 0 && !slices.ContainsFunc(paths, func(changed string) bool { return changed != trail })
		}
	} else {
		var loc locatedFlow
		var code int
		loc, t, code = p.openTrail(folder)
		if code != 0 {
			return code
		}
		rows = t.stateRows()
		if branch == "" || latestBind(rows, branch) == nil {
			fmt.Fprintln(p.stdout, "audit: not run (this branch is not bound to the flow)")
			return 0
		}
		workFolder = loc.rel
		workOnly = func(paths []string) bool { return onlyUnder(paths, workFolder) }
	}
	gitDir := t.gitDir

	// Every Git read that measures the range is checked: a failed read taken
	// for an empty answer would hide findings and print audit: clean. The
	// first failure is kept, and the audit then prints no finding at all.
	failure := ""
	fail := func(what string, err error) {
		if failure == "" {
			failure = what + ": " + gitErrorText(err)
		}
	}
	short := abbrev(baseSHA)
	commits, err := checkedRangeCommits(top, "HEAD", baseSHA)
	if err != nil {
		return p.auditFailed("the commits of " + short + "..HEAD: " + gitErrorText(err))
	}
	// mergeBaseWith returns the merge base of tip and the base, read in dir,
	// or "" when the histories share no commit or the read failed.
	mergeBaseWith := func(dir, tip, name string) string {
		out, err := gitBytes(dir, nil, "merge-base", tip, baseSHA)
		if err != nil {
			// Exit 1 without output is Git's answer that the histories share
			// no commit, so the base holds no change of tip's history.
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 || len(out) > 0 {
				fail("the merge base of "+short+" and "+name, err)
			}
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	// idsSince returns the patch-ids of the non-merge commits of
	// mergeBase..<base>, read once per merge base: what the base added since
	// the history it shares with a commit, so the pass costs only as much as
	// the base moved since then. An empty mergeBase gives no ids.
	idsByMergeBase := map[string]map[string]bool{}
	idsSince := func(mergeBase string) map[string]bool {
		if mergeBase == "" {
			return nil
		}
		if ids, ok := idsByMergeBase[mergeBase]; ok {
			return ids
		}
		baseCommits, err := checkedRangeCommits(top, baseSHA, mergeBase)
		if err != nil {
			fail("the commits of "+abbrev(mergeBase)+".."+short, err)
		}
		ids := patchIDSet(baseCommits)
		idsByMergeBase[mergeBase] = ids
		return ids
	}
	// The base's new commits since the fork bound what a cherry-pick from it
	// can be. They are read only when a commit or a row needs the answer.
	headMergeBase, headMergeBaseRead := "", false
	headMergeBaseOf := func() string {
		if !headMergeBaseRead {
			headMergeBase, headMergeBaseRead = mergeBaseWith(top, "HEAD", "HEAD"), true
		}
		return headMergeBase
	}
	baseIDsOf := func() map[string]bool { return idsSince(headMergeBaseOf()) }
	inRange := rangeLister(top)

	// evidence holds the rows that may account for a commit, and naming the
	// valid rows that name one whether or not they count, so that a commit
	// only an unattested commit row names can be marked. checked is the trail
	// the supersession and commit checks read: a small fix's record alone.
	var evidence, naming []trailRow
	checked := t
	patchID := patchIDsIn(gitDir)
	if smallFix {
		// HEAD reaches every commit of the branch and the base none, merges
		// included, so this listing holds each commit a row may name.
		out, err := gitBytes(top, nil, "rev-list", "HEAD", "--not", baseSHA, "--")
		if err != nil {
			fail("the commits of "+short+"..HEAD", err)
		}
		record := smallFixRecord(t.rows, branch, strings.Fields(string(out)), patchIDSet(commits),
			func(r trailRow) bool { return t.commitAttested(r, patchID) })
		checked, evidence, naming = t.within(record.rows), record.counting, record.naming
	} else {
		evidence = t.accounting(patchID)
		naming = t.valid()
	}

	// A commit no counting row names is exempt when Git proves it
	// needs no row, and unaccounted otherwise.
	var unaccounted []rangeCommit
	exempt := map[string][]string{}
	unnamed := unnamedCommits(commits, evidence, func(rangeCommit) bool { return true }, inRange)
	isUnnamed := make(map[string]bool, len(unnamed))
	for _, sha := range unnamed {
		isUnnamed[sha] = true
	}
	for _, commit := range commits {
		if !isUnnamed[commit.sha] {
			continue
		}
		switch {
		case workOnly(commit.paths):
			exempt["work-folder"] = append(exempt["work-folder"], commit.sha)
		case len(commit.paths) == 0:
			exempt["empty"] = append(exempt["empty"], commit.sha)
		case commit.patchID != "" && baseIDsOf()[commit.patchID]:
			exempt["from-base"] = append(exempt["from-base"], commit.sha)
		default:
			unaccounted = append(unaccounted, commit)
		}
	}
	var findings []string
	if len(unaccounted) > 0 {
		// A commit that only an unattested commit row names is marked, so the
		// reader knows a record exists that this clone did not write.
		open := make(map[string]bool, len(unaccounted))
		for _, commit := range unaccounted {
			open[commit.sha] = true
		}
		stillOpen := map[string]bool{}
		for _, sha := range unnamedCommits(commits, naming, func(c rangeCommit) bool { return open[c.sha] }, inRange) {
			stillOpen[sha] = true
		}
		subjects, err := commitSubjects(top, baseSHA)
		if err != nil {
			fail("the subjects of "+short+"..HEAD", err)
		}
		for _, commit := range unaccounted {
			line := "unaccounted: " + abbrev(commit.sha) + " " + subjects[commit.sha]
			if !stillOpen[commit.sha] {
				line += " (row not attested here)"
			}
			findings = append(findings, line)
		}
	}

	// A row whose commit the base holds, or whose change the base holds by
	// another sha, records a layer below this branch. The change is looked
	// for in what the base added since the row's own commit forked from it:
	// when a lower layer was rewritten and this branch rebased onto it, the
	// base and HEAD share the rewritten layer, so the range from their merge
	// base would hold none of it. A row whose commit this clone lacks has no
	// fork point, so it is looked for since HEAD's.
	lowerLayer := func(r trailRow) bool {
		cell := r.cell
		sha := cell.textLast("commit")
		if _, err := gitBytes(gitDir, nil, "merge-base", "--is-ancestor", sha, baseSHA); err == nil {
			return true
		}
		if !cell.textHasValue("patch-id") {
			return false
		}
		id := cell.textLast("patch-id")
		// Only a hex value reaches Git as a revision, so a cell cannot pass
		// it an option.
		if hexAtLeast4(sha) {
			if _, err := gitBytes(gitDir, nil, "cat-file", "-e", sha+"^{commit}"); err == nil {
				return idsSince(mergeBaseWith(gitDir, sha, abbrev(sha)))[id]
			}
		}
		return baseIDsOf()[id]
	}
	findings = append(findings, mismatchLines(checked.commitChecks(), gitDir, func() map[string]bool { return patchIDSet(commits) }, lowerLayer)...)

	// A changed path is in scope when an in-scope task lists it, a recorded
	// divergence entry names it, it lies in the work folder, or every commit
	// that changes it is accounted for without a task: by an attested commit
	// row or by an exemption. A verify row does not cover a path, since the
	// task whose commit it names must list the paths that commit changes. A
	// small fix has no plan: gate's size signal bounds it instead.
	var scope planScope
	var tasks []planTask
	scopeNote := "note: scope not checked (small fix: the size signal applies)"
	if !smallFix {
		var planned bool
		scope, planned = readPlanScope(t.flow.stored.dir, t.divergenceRecorded())
		// The audit never reads the default branch, so on it a hand-written
		// bind row scopes the check to that row's slices while flow status
		// prints slice: not bound. That is harmless: gate does not run from
		// the default branch.
		ids, _ := bindSlice(rows, branch)
		tasks, scopeNote = scopeTasks(scope, planned, ids)
	}
	if scopeNote == "" {
		paths, err := branchPaths(top, baseSHA, headMergeBaseOf())
		if err != nil {
			fail("the changed paths of "+short+"...HEAD", err)
		}
		isUnaccounted := make(map[string]bool, len(unaccounted))
		for _, commit := range unaccounted {
			isUnaccounted[commit.sha] = true
		}
		var commitRows []trailRow
		for _, r := range evidence {
			if r.stateRow[3] == "commit" {
				commitRows = append(commitRows, r)
			}
		}
		notByCommitRow := make(map[string]bool)
		for _, sha := range unnamedCommits(commits, commitRows, func(rangeCommit) bool { return true }, inRange) {
			notByCommitRow[sha] = true
		}
		// A commit is accounted for without a task when an attested commit
		// row names it or it is exempt from needing a row.
		accounted := func(sha string) bool {
			return !notByCommitRow[sha] || isUnnamed[sha] && !isUnaccounted[sha]
		}
		changedBy := make(map[string][]string)
		for _, commit := range commits {
			for _, changed := range commit.paths {
				changedBy[changed] = append(changedBy[changed], commit.sha)
			}
		}
		for _, changed := range paths {
			if underFolder(changed, workFolder) || listedIn(changed, scope.divergencePaths) || slices.ContainsFunc(tasks, func(task planTask) bool { return listedIn(changed, task.paths) }) {
				continue
			}
			// A path no commit of the range changes reached the branch only
			// through a merge, which no record accounts for.
			if shas := changedBy[changed]; len(shas) > 0 && !slices.ContainsFunc(shas, func(sha string) bool { return !accounted(sha) }) {
				continue
			}
			findings = append(findings, "out-of-scope: "+changed)
		}
	}

	// A superseded verify row was corrected, so only the rest are findings.
	for i, r := range checked.rows {
		if r.stateRow[3] != "verify" || r.reason == "" {
			continue
		}
		if line, superseded := checked.invalidLine(i); !superseded {
			findings = append(findings, line)
		}
	}

	merges, err := gitBytes(top, nil, "rev-list", "--merges", "--reverse", "HEAD", "--not", baseSHA, "--")
	if err != nil {
		fail("the merge commits of "+short+"..HEAD", err)
	}
	if failure != "" {
		return p.auditFailed(failure)
	}
	for _, line := range findings {
		fmt.Fprintln(p.stdout, line)
	}
	for _, class := range []string{"work-folder", "empty", "from-base"} {
		if shas := exempt[class]; len(shas) > 0 {
			line := "note: exempt (" + class + ")"
			for _, sha := range shas {
				line += " " + abbrev(sha)
			}
			fmt.Fprintln(p.stdout, line)
		}
	}
	if scopeNote != "" {
		fmt.Fprintln(p.stdout, scopeNote)
	}
	for _, sha := range strings.Fields(string(merges)) {
		fmt.Fprintln(p.stdout, "note: merge commit "+abbrev(sha)+" not audited")
	}
	if len(findings) == 0 {
		fmt.Fprintln(p.stdout, "audit: clean")
	} else {
		fmt.Fprintln(p.stdout, "audit: "+strconv.Itoa(len(findings))+" findings")
	}
	return 0
}

// smallFixSet is a small fix's record as the audit reads it: the record's
// rows, each with its reason, the rows check 1 counts, and the rows that would
// count if this clone's ledger attested every commit row, which tell a commit
// only an unattested row names.
type smallFixSet struct {
	rows, counting, naming []trailRow
}

// smallFixRecord returns branch's small-fix record among rows, the trail rows
// of the unassigned folder in time order. Every branch's small fix logs to
// that folder and writes the target fix, so the record, not the target, keeps
// another branch's rows out of the checks.
//
// The record is the latest skip chain row of branch and, after it: the verify
// fix and verify red-fix rows whose branch= is branch or whose commit is on
// the branch, the review fix rows, the enter run row of branch, and the commit
// rows of any phase that attested reports and whose commit= or patch-id= is on
// the branch, so a rebase that keeps the commit's change keeps its row.
// Rows before that skip row belong to an earlier record a pivot closed. A
// value is on the branch when a commit=, integrated= or head= value is a
// unique prefix of one of branchCommits, or a patch-id= value is one of
// rangeIDs. A row whose branch= names another branch is never in the record.
// An invalid row is in the record only when its branch= is branch or its
// commit= is on the branch, since its other values follow no form.
//
// Check 1 counts what the audit of a fix commit always counted: a valid row
// with status ok that is an execute verify row not in the no-commit form, or
// an attested commit row. A row that only counts for checks 2 and 4, such as a
// failed verify row, accounts for no commit.
func smallFixRecord(rows []trailRow, branch string, branchCommits []string, rangeIDs map[string]bool, attested func(trailRow) bool) smallFixSet {
	var set smallFixSet
	start := -1
	for i, r := range rows {
		if r.stateRow[3] == "skip" && r.stateRow[4] == "chain "+branch {
			start = i
		}
	}
	if start < 0 {
		return set
	}
	// A quoted no-commit cell still places its row, by the words as written.
	onBranch := func(r trailRow, keys ...string) bool {
		for _, token := range r.cell.asWritten().pairs() {
			if !slices.Contains(keys, token.key) {
				continue
			}
			if token.key == "patch-id" {
				if token.value != "" && rangeIDs[token.value] {
					return true
				}
			} else if uniquePrefix(branchCommits, token.value) {
				return true
			}
		}
		return false
	}
	for i := start; i < len(rows); i++ {
		r := rows[i]
		rowBranch := r.cell.textLast("branch")
		ownBranch := rowBranch == branch
		in, counts, names := false, false, false
		switch {
		case i == start:
			in = true
		case rowBranch != "" && !ownBranch:
			// A row that names another branch belongs to that branch's
			// record, even when its commit is also on this one.
		case r.reason != "":
			in = ownBranch || onBranch(r, "commit")
		case r.stateRow[3] == "verify" && (r.stateRow[4] == "fix" || r.stateRow[4] == "red-fix"):
			in = ownBranch || onBranch(r, "commit", "integrated", "head", "patch-id")
			counts = in && r.stateRow[5] == "ok" && r.stateRow[2] == "execute" && !r.cell.quoted
			names = counts
		case r.stateRow[3] == "review" && r.stateRow[4] == "fix":
			in = true
		case r.stateRow[3] == "enter" && r.stateRow[4] == "run":
			in = ownBranch
		case r.stateRow[3] == "commit" && onBranch(r, "commit", "patch-id"):
			// An unattested commit row is no record, but it still names its
			// commit, so check 1 can say a record this clone did not write
			// exists.
			in = attested(r)
			names = r.stateRow[5] == "ok"
			counts = in && names
		}
		if in {
			set.rows = append(set.rows, r)
		}
		if counts {
			set.counting = append(set.counting, r)
		}
		if names {
			set.naming = append(set.naming, r)
		}
	}
	return set
}

// uniquePrefix reports whether value is at least four hex characters that
// start exactly one of shas, compared without case.
func uniquePrefix(shas []string, value string) bool {
	if !hexAtLeast4(value) {
		return false
	}
	value = strings.ToLower(value)
	found := false
	for _, sha := range shas {
		if strings.HasPrefix(sha, value) {
			if found {
				return false
			}
			found = true
		}
	}
	return found
}

// scopeTasks returns the tasks whose paths are in scope: every task of scope,
// or, when ids names bound slices, the tasks under those slices' headings.
// note is the line that says why the scope cannot be checked, and "" when it
// can: planned is false, or a bound slice has no task in the plan, whether its
// heading is missing or has no task under it, so no path may be called out of
// scope.
func scopeTasks(scope planScope, planned bool, ids string) (tasks []planTask, note string) {
	if !planned {
		return nil, "note: scope not checked (no task in the plan)"
	}
	if ids == "" {
		return scope.tasks, ""
	}
	bound := strings.Split(ids, ",")
	for _, id := range bound {
		found := false
		for slice := range scope.slices {
			found = found || strings.EqualFold(slice, id)
		}
		if !found {
			return nil, "note: scope not checked (slice " + id + " has no task in the plan)"
		}
	}
	for _, task := range scope.tasks {
		if slices.ContainsFunc(bound, func(id string) bool { return strings.EqualFold(id, task.slice) }) {
			tasks = append(tasks, task)
		}
	}
	return tasks, ""
}

// branchPaths returns the paths base...HEAD changes, sorted, read in top.
// mergeBase is the merge base of HEAD and base; with none the histories share
// nothing, so every path of HEAD is a change of the branch. Renames are listed
// as a deletion and an addition, so moving a file out of a place no task
// covers still names that place.
func branchPaths(top, base, mergeBase string) ([]string, error) {
	args := []string{"diff", "--name-only", "-z", "--no-renames", base + "...HEAD", "--"}
	if mergeBase == "" {
		args = []string{"ls-tree", "-r", "-z", "--name-only", "HEAD"}
	}
	out, err := gitBytes(top, nil, args...)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range bytes.Split(out, []byte{0}) {
		if len(p) > 0 {
			paths = append(paths, string(p))
		}
	}
	slices.Sort(paths)
	return paths, nil
}

// underFolder reports whether path lies in the repository-relative folder.
func underFolder(path, folder string) bool {
	return folder != "" && strings.HasPrefix(path, strings.TrimSuffix(folder, "/")+"/")
}

// listedIn reports whether any of listed names p. A listed path names p
// exactly, or, ending in "/", every path under it, or, holding *, ? or [,
// every path it matches as a glob. A brace set {a,b} stands for each of its
// alternatives; braces do not nest.
func listedIn(p string, listed []string) bool {
	for _, entry := range listed {
		for _, alt := range expandBraces(strings.ReplaceAll(entry, `\`, "/")) {
			switch {
			case strings.HasSuffix(alt, "/"):
				if strings.HasPrefix(p, alt) {
					return true
				}
			case strings.ContainsAny(alt, "*?["):
				if ok, _ := path.Match(alt, p); ok {
					return true
				}
			case alt == p:
				return true
			}
		}
	}
	return false
}

// Plans and divergence entries arrive with a branch, so the brace expansion of
// a listed path is bounded three ways: maxBraceExpansions bounds the
// alternatives, the product of its sets; maxBraceSets bounds the recursion,
// one level per opening brace, which a set of one alternative deepens without
// adding to the product; and maxBracePathLen bounds the text each level copies.
// No real path is longer than maxBracePathLen.
const (
	maxBraceExpansions = 256
	maxBraceSets       = 64
	maxBracePathLen    = 4096
)

// expandBraces returns pattern with each brace set replaced by each of its
// comma-separated alternatives in turn; a brace with no closing brace is
// literal. A pattern past one of the bounds above is returned whole, so it
// names only the path spelled that way and its changed paths are reported out
// of scope rather than left unchecked.
func expandBraces(pattern string) []string {
	if len(pattern) > maxBracePathLen || strings.Count(pattern, "{") > maxBraceSets {
		return []string{pattern}
	}
	if expanded, ok := expandBracesWithin(pattern, maxBraceExpansions); ok {
		return expanded
	}
	return []string{pattern}
}

// expandBracesWithin is expandBraces that gives up, returning false, as soon
// as the alternatives pass limit, so the work done stays bounded by limit.
func expandBracesWithin(pattern string, limit int) ([]string, bool) {
	if limit < 1 {
		return nil, false
	}
	open := strings.IndexByte(pattern, '{')
	if open < 0 {
		return []string{pattern}, true
	}
	length := strings.IndexByte(pattern[open:], '}')
	if length < 0 {
		return []string{pattern}, true
	}
	end := open + length
	var expanded []string
	for alt := range strings.SplitSeq(pattern[open+1:end], ",") {
		more, ok := expandBracesWithin(pattern[:open]+alt+pattern[end+1:], limit-len(expanded))
		if !ok {
			return nil, false
		}
		expanded = append(expanded, more...)
	}
	return expanded, true
}

// auditFailed reports a Git read the audit could not make, and returns the
// exit code of an unexpected failure.
func (p *program) auditFailed(what string) int {
	fmt.Fprintln(p.stderr, "flow: audit cannot read "+what)
	return 1
}

// gitErrorText returns err's text followed by the first line Git wrote to
// stderr, when it wrote one, since the exit status alone does not say what
// Git could not read.
func gitErrorText(err error) string {
	text := err.Error()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if line, _, _ := strings.Cut(strings.TrimSpace(string(exit.Stderr)), "\n"); line != "" {
			text += " (" + strings.TrimSpace(line) + ")"
		}
	}
	return text
}

// resolveBase returns the full sha of the commit base names, or "" when it
// names none. A value that starts with "-" never reaches Git as an option, and
// a hex value must abbreviate the commit it resolves to, so a branch or tag
// whose name looks like a sha cannot stand in for one.
func (p *program) resolveBase(top, base string) string {
	if base == "" || strings.HasPrefix(base, "-") {
		return ""
	}
	dir := top
	if dir == "" {
		dir = p.dir
	}
	sha := gitText(dir, "rev-parse", "--verify", "--quiet", base+"^{commit}")
	if sha == "" {
		return ""
	}
	if isHex(base) && !strings.HasPrefix(sha, strings.ToLower(base)) {
		return ""
	}
	return sha
}

// isHex reports whether value is one or more hex digits.
func isHex(value string) bool {
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return value != ""
}

// commitSubjects returns the subject of each commit of base..HEAD by its full
// sha, read in one Git call however long the range is, and the error of a
// failed call.
func commitSubjects(top, base string) (map[string]string, error) {
	subjects := make(map[string]string)
	out, err := gitBytes(top, nil, "log", "--no-show-signature", "--format=%H%x00%s", "HEAD", "--not", base, "--")
	if err != nil {
		return subjects, err
	}
	for _, line := range bytes.Split(out, []byte{'\n'}) {
		if sha, subject, ok := bytes.Cut(line, []byte{0}); ok {
			subjects[string(sha)] = string(subject)
		}
	}
	return subjects, nil
}

// abbrev returns the first twelve characters of sha.
func abbrev(sha string) string { return sha[:min(len(sha), 12)] }
