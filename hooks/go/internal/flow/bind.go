package flow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
)

var sliceID = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^S[1-9][0-9]*$`) })

// validSlices reports whether ids is one slice id, or several joined by
// commas, each named once.
func validSlices(ids string) bool {
	list := strings.Split(ids, ",")
	for i, id := range list {
		if !sliceID().MatchString(id) || slices.Contains(list[:i], id) {
			return false
		}
	}
	return true
}

// latestBind returns the latest bind row of branch in rows, or nil.
func latestBind(rows []stateRow, branch string) stateRow {
	stored := cleanCell(branch)
	var latest stateRow
	for _, row := range rows {
		if row[3] == "bind" && row[4] == stored {
			latest = row
		}
	}
	return latest
}

// boundSlices returns the slice ids the latest bind row of branch carries, as
// stored, or "" when the branch is bound with no slice or not bound.
func boundSlices(rows []stateRow, branch string) string {
	if row := latestBind(rows, branch); row != nil {
		return strings.Join(cellOf(row).list("slice"), ",")
	}
	return ""
}

func (p *program) bind(args []string) int {
	if code := p.checkStorage(); code != 0 {
		return code
	}
	folder, ids, hasSlice := "", "", false
	usageError := func() int { fmt.Fprint(p.stderr, usage); return 1 }
	for i := 0; i < len(args); i++ {
		if (args[i] != "--folder" && args[i] != "--slice") || i+1 >= len(args) {
			return usageError()
		}
		if args[i] == "--folder" {
			folder = args[i+1]
		} else {
			ids, hasSlice = args[i+1], true
		}
		i++
	}
	reject := func(field, value, reason string) int {
		fmt.Fprintln(p.stderr, invalidCell(field, value, reason))
		return 2
	}
	if hasSlice && !validSlices(ids) {
		return reject("slice", ids, "a slice id is S followed by a positive integer; pass one, or several joined by commas, each once")
	}
	top := p.repo.top()
	branch := ""
	if top != "" {
		branch = p.repo.branch()
	}
	if branch == "" {
		return reject("branch", "", "a detached HEAD has no branch to bind")
	}
	if p.repo.onDefault() {
		return reject("branch", branch, "the default branch is never bound to a flow")
	}
	if folder == "" {
		var code int
		folder, code = p.resolveInto()
		if code != 0 {
			return code
		}
	}
	// The spellings are filled even when locateFlow refuses the folder, so the
	// small-fix refusal below still comes before the store's error.
	loc, err := p.locateFlow(folder)
	// Messages name the logical folder, whatever form named it and whichever
	// store holds it.
	logical := loc.logical
	if filepath.Base(folder) == "unassigned" {
		return reject("folder", logical, "a small fix has no flow to bind")
	}
	if err != nil {
		return p.locateFailed(err)
	}
	stored := loc.stored
	state := filepath.Join(stored.dir, "state.tsv")
	if info, err := os.Stat(state); err != nil || !info.Mode().IsRegular() {
		return reject("folder", logical, "no state.tsv to bind to")
	}
	rows := rowsOf(state)
	// Work bound after the verdict reached the default branch would never be
	// judged, because the flow stays closed for lookup; a done row alone does
	// not close it this way, since a later row reopens it. The tip copy is keyed
	// as lookup keys it, so that a folder named through a symlink or another
	// drive-letter case finds it; a folder outside the repository has none. A
	// local-store flow closes only by its done row: no commit holds its
	// state.tsv, so a tip copy of the same folder is another flow's.
	if ref := p.repo.defaultRef(); ref != "" && stored.store == storageCommitted {
		if loc.inside {
			if tip := tipCopies(top, ref, []string{loc.rel})[0]; tip != nil && closedByMerge(rows, tip) {
				return reject("folder", logical, "the flow is finished: its verdict is on the default branch; start new work in a new flow")
			}
		}
	}
	evidence := "none"
	if latestBind(rows, branch) != nil {
		current := boundSlices(rows, branch)
		switch {
		case !hasSlice || current == ids:
			return 0
		case current != "":
			return reject("slice", ids, branch+" is bound to slice="+current+" in this flow")
		}
	}
	if hasSlice {
		evidence = "slice=" + ids
	}
	phase, next := "intent", "intent resume"
	if len(rows) > 0 {
		last := rows[len(rows)-1]
		if last[2] != "" {
			phase = last[2]
		}
		if last[8] != "" {
			next = last[8]
		}
	}
	return p.log([]string{phase, "bind", branch, "ok", "bound " + branch + " to this flow", evidence, next, "--folder", folder})
}
