package flow

import (
	"fmt"
	"path/filepath"
	"strings"
)

// sliceTagged returns the text of cell with the slice= pair of the current
// branch's latest bind row in the flow at dir appended, so that a verdict or a
// commit row counts for the slice the branch carries. A cell that already names
// that slice is returned unchanged, and one that names another is refused, since
// one row cannot count for two slice sets. The caller passes only a cell whose
// pairs are evidence: a quoted cell has none to read.
func (p *program) sliceTagged(dir string, cell evidenceCell) (string, error) {
	evidence := cell.text
	if p.repo.top() == "" {
		return evidence, nil
	}
	branch := p.repo.branch()
	if branch == "" {
		return evidence, nil
	}
	want := boundSlices(rowsOf(filepath.Join(dir, "state.tsv")), branch)
	if want == "" {
		return evidence, nil
	}
	named := false
	for _, value := range cell.values("slice") {
		if value != want {
			return evidence, invalidCell("evidence", evidence, branch+" is bound to slice="+want+" in this flow")
		}
		named = true
	}
	if named {
		return evidence, nil
	}
	return evidence + " slice=" + want, nil
}

// commitEvidence returns the stored evidence for an auto or auto:<sha> cell
// that resolved to the commit sha. A merge has no single diff, so its row
// names the commit alone and status checks it by ancestry. A commit that
// changes nothing has no patch-id and is refused, so that no row carries an
// empty one. A bare auto names HEAD, and when HEAD changes only the flow's
// work folder the row would name a commit that holds no work, so it is
// refused; auto:<sha> names its commit on purpose and is kept.
func (p *program) commitEvidence(loc locatedFlow, evidence, sha string) (string, error) {
	stored := "commit=" + sha
	if isMergeCommit(p.dir, sha) {
		return stored, nil
	}
	paths, ok := changedPaths(p.dir, sha)
	if ok && len(paths) == 0 {
		return "", invalidCell("evidence", evidence, "the commit changes nothing, so it has no patch-id")
	}
	if evidence == "auto" {
		if loc.inside && onlyUnder(paths, loc.rel) {
			return "", invalidCell("evidence", evidence, "auto names HEAD, which changes only the work folder; pass auto:<sha> for the task commit")
		}
	}
	// A non-merge row without its patch-id reads as invalid, so a commit whose
	// patch-id git cannot compute is refused rather than stored bare.
	id := patchIDOf(p.dir, sha)
	if id == "" {
		return "", invalidCell("evidence", evidence, "git computed no patch-id for commit "+sha)
	}
	return stored + " patch-id=" + id, nil
}

// bareCommitReasons returns, by index, why each of values, written as the
// key= pair of the commit form without its patch-id, fails that form, or ""
// when the value names a merge commit, which has no patch-id to carry. flow
// log and flow status both take the reason from here, so the writer's refusal
// and the reader's invalid: line read alike. Status may hold many such rows,
// so one cat-file call answers for every value: <v>^{commit} resolves the
// value, and <v>^2 names a second parent, which only a merge has. cat-file
// reads the names from stdin, so no file in the working tree can be taken for
// one, and Git runs in dir.
func bareCommitReasons(dir, key string, values []string) []string {
	reasons := make([]string, len(values))
	if len(values) == 0 {
		return reasons
	}
	var input strings.Builder
	for _, value := range values {
		input.WriteString(value + "^{commit}\n" + value + "^2\n")
	}
	var answers []string
	if out, err := runGit(dir, []byte(input.String()), "cat-file", "--batch-check=%(objecttype)"); err == nil {
		answers = strings.Split(strings.ReplaceAll(string(out), "\r", ""), "\n")
	}
	// A missing or ambiguous name prints "<name> missing" or "<name>
	// ambiguous", so only a line that is exactly "commit" resolved.
	resolved := func(i int) bool { return i < len(answers) && answers[i] == "commit" }
	for i, value := range values {
		switch {
		case !resolved(2 * i):
			reasons[i] = key + " " + value + " does not resolve to a commit"
		case !resolved(2*i + 1):
			reasons[i] = key + " " + value + " is not a merge, so it needs patch-id="
		}
	}
	return reasons
}

// resolveVerify checks a hand-typed verify cell against the repository: each
// commit it names must resolve to a commit, and the commit form must carry
// the patch-id auto would store, which a merge does not have. Only the parsed
// values reach Git, so text quoted inside a no-commit cell is never resolved,
// and those values are hex, so none can be read as an option. The isolated
// form's patch-id describes the packet diff, which no single commit holds, so
// it is checked for shape only.
func (p *program) resolveVerify(target, status, evidence string) error {
	parsed, err := parseVerify(target, status, evidence)
	if err != nil {
		return err
	}
	refuse := func(reason string) error { return invalidCell("evidence", evidence, reason) }
	if parsed.form == formCommit && parsed.patchID == "" {
		if reason := bareCommitReasons(p.dir, "commit", []string{parsed.commit})[0]; reason != "" {
			return refuse(reason)
		}
		return nil
	}
	named := []struct{ key, value string }{
		{"commit", parsed.commit}, {"base", parsed.base}, {"head", parsed.head}, {"integrated", parsed.integrated},
	}
	full := map[string]string{}
	for _, pair := range named {
		if pair.value == "" {
			continue
		}
		sha := gitText(p.dir, "rev-parse", "--verify", "--quiet", pair.value+"^{commit}")
		if sha == "" {
			return refuse(pair.key + " " + pair.value + " does not resolve to a commit")
		}
		full[pair.key] = sha
	}
	if parsed.form != formCommit && parsed.form != formIntegrated {
		return nil
	}
	sha := full["commit"]
	// The merge test comes first, so a patch-id on a merge is reported as
	// such rather than as a mismatch with a patch-id no merge has.
	if isMergeCommit(p.dir, sha) {
		if parsed.patchID != "" {
			return refuse("commit " + parsed.commit + " is a merge, which has no patch-id")
		}
		return nil
	}
	if parsed.patchID == "" {
		return nil
	}
	if id := patchIDOf(p.dir, sha); id != parsed.patchID {
		if id == "" {
			id = "none"
		}
		return refuse("patch-id=" + parsed.patchID + " does not match commit " + parsed.commit + " (patch-id " + id + ")")
	}
	return nil
}

func (p *program) log(args []string) int {
	if code := p.checkStorage(); code != 0 {
		return code
	}
	var values [7]string
	folder, expect, hasExpect, count := "", "", false, 0
	entryArg, hasEntry := "", false
	usageError := func() int { fmt.Fprint(p.stderr, usage); return 1 }
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--folder", "--expect-blob", "--entry":
			if i+1 >= len(args) {
				return usageError()
			}
			switch args[i] {
			case "--folder":
				folder = args[i+1]
			case "--expect-blob":
				expect, hasExpect = args[i+1], true
			default:
				entryArg, hasEntry = args[i+1], true
			}
			i++
		default:
			if count < len(values) {
				values[count] = args[i]
			}
			count++
		}
	}
	if count != len(values) {
		return usageError()
	}
	if hasExpect && (stripCell(values[1]) != "approve" || stripCell(values[5]) != "auto") {
		return usageError()
	}
	if hasEntry && stripCell(values[1]) != "diverge" {
		return usageError()
	}
	reject := func(err error) int { fmt.Fprintln(p.stderr, err); return 2 }
	var err error
	values, err = validateCells(values)
	if err != nil {
		return reject(err)
	}
	// auto and auto:<sha> are resolved into the commit form further down, so
	// only evidence the caller typed is resolved here.
	if values[1] == "verify" && values[5] != "auto" && !strings.HasPrefix(values[5], "auto:") {
		if err := p.resolveVerify(values[2], values[3], values[5]); err != nil {
			return reject(err)
		}
	}
	if folder == "" {
		var code int
		folder, code = p.resolveInto()
		if code != 0 {
			return code
		}
	}
	loc, err := p.locateFlow(folder)
	if err != nil {
		return p.locateFailed(err)
	}
	dir := loc.stored.dir
	// Messages name the logical folder, whatever form named it and whichever
	// store holds it.
	logical := loc.logical
	evidence := values[5]
	guarded := false
	if evidence == "auto" || strings.HasPrefix(evidence, "auto:") {
		switch values[1] {
		case "order":
			// A brief's standing orders are inside its approved text, so its
			// approval already binds them and no order row is taken.
			entries, ok := artifactEntries(dir, "plan.md", standingOrdersHeading)
			if !ok {
				return reject(invalidCell("target", values[2], "an order row reads plan.md, and "+logical+" has none; a brief's standing orders are inside its approved text"))
			}
			picked, ok := pickEntry(entries, values[2])
			if !ok {
				return reject(invalidCell("target", values[2], logical+"/plan.md has no entry "+values[2]+" under "+standingOrdersHeading))
			}
			values[5] = "entry=" + picked.hash
		case "approve":
			if evidence != "auto" {
				return reject(invalidCell("evidence", evidence, "auto:<sha> applies to commit rows only"))
			}
			// The caller must name the hash of the text the user was shown, so that an
			// edit made after the user saw it cannot be approved in its place.
			if !hasExpect {
				return reject(invalidCell("evidence", evidence, "approve needs --expect-blob <hash>: run flow status and pass the hash of the content the user was shown"))
			}
			hash, ok := fileTextHash(dir, values[2])
			if !ok || hash != expect {
				if !ok {
					hash = "unavailable"
				}
				return reject(invalidCell("evidence", expect, logical+"/"+values[2]+" current text="+hash+" does not match --expect-blob"))
			}
			values[5] = "text=" + hash
			guarded = true
		case "verify", "revert", "diverge", "commit":
			revision := "HEAD"
			if evidence != "auto" {
				revision = strings.TrimPrefix(evidence, "auto:")
			}
			sha := gitText(p.dir, "rev-parse", "--verify", revision+"^{commit}")
			if sha == "" {
				reason := "no such commit to verify against"
				if values[1] == "commit" {
					reason = "no such commit to account for"
				}
				return reject(invalidCell("evidence", evidence, reason))
			}
			values[5], err = p.commitEvidence(loc, evidence, sha)
			if err != nil {
				return reject(err)
			}
		default:
			return reject(invalidCell("evidence", evidence, "auto applies to approve, order, verify, revert, diverge and commit rows only"))
		}
	}
	if hasEntry {
		if entryArg != "last" && !evidenceRound().MatchString(entryArg) {
			return reject(invalidCell("entry", entryArg, "--entry takes a positive integer or last"))
		}
		artifact := divergeArtifact(dir, values[2])
		entries, ok := artifactEntries(dir, artifact, divergencesHeading)
		if !ok {
			return reject(invalidCell("entry", entryArg, logical+"/"+artifact+" is not a readable file"))
		}
		picked, ok := pickEntry(entries, entryArg)
		if !ok {
			which := "entry"
			if entryArg != "last" {
				which += " " + entryArg
			}
			return reject(invalidCell("entry", entryArg, logical+"/"+artifact+" has no "+which+" under "+divergencesHeading))
		}
		values[5] += " entry=" + picked.hash
	}
	if cell := evidenceOf(values[1], values[2], values[5]); values[1] == "branch" || cell.has("commit") {
		values[5], err = p.sliceTagged(dir, cell)
		if err != nil {
			return reject(err)
		}
	}
	session := p.getenv("FLOW_SESSION")
	if session == "" {
		session = p.getenv("CLAUDE_CODE_SESSION_ID")
	}
	if err := appendState(dir, logical, session, values, p.now); err != nil {
		fmt.Fprintln(p.stderr, err)
		return 1
	}
	// The ledger tells a row this clone wrote from one that arrived by commit,
	// and ledgerRecordOf shapes the stored row into its line. Only a hash the
	// helper computed from the file records consent, so a typed text= approval
	// stays unrecorded, and an approval or a waiver that is not ok records
	// nothing. The target is the cell status reads back. A ledger that cannot
	// be resolved or written leaves the record unconfirmed here, which status
	// reports, so it does not fail the row already written.
	kind, hash, ok := ledgerRecordOf(values[1], values[2], evidenceOf(values[1], values[2], values[5]))
	switch values[1] {
	case "approve":
		ok = ok && guarded && values[3] == "ok"
	case "waive":
		ok = ok && values[3] == "ok"
	}
	if ok {
		entry := ledgerEntry{kind, loc.ledgerKey, cleanCell(values[2]), hash}
		_ = appendLedger(p.repo.common(), entry, p.now)
	}
	return 0
}
