package flow

import (
	"bytes"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// openFlows returns the lookup filter over flow folders: false for a closed
// flow. A flow is closed when its current row is done, or, for a committed
// flow, when the copy of its state.tsv at the default branch's tip holds its
// final verdict. The working file alone never closes a flow by verdict, because
// the verdict counts only once the work it judges has reached the default
// branch; a local-store flow closes only by its done row, since no commit holds
// its state.tsv. A flow the closed-flow cache records with its state.tsv
// unchanged is closed unread. Each listed flow is decided once, so the filter
// may be asked about it again at no cost. common is the repository's common
// Git directory, which holds the cache.
func (p *program) openFlows(top, common string, flows []storedFolder) func(folder string) bool {
	cache := loadClosedCache(common)
	open := p.openSet(top, flows, cache)
	return func(folder string) bool {
		key := relativeFlow(top, folder)
		if verdict, known := open[key]; known {
			return verdict
		}
		// The branch diff can name a state.tsv deeper than allFlows lists; it is
		// decided on its own rather than widening the batch for every lookup.
		return p.openSet(top, []storedFolder{{folder, storageCommitted}}, cache)[key]
	}
}

// openSet decides each flow, keyed by its directory relative to top, and
// records in cache each flow it finds closed. One git cat-file --batch process
// reads every default-tip copy, and a flow cache records is neither read nor
// asked for, so that neither the number of Git processes nor the number of
// state.tsv reads grows with the number of closed work folders.
func (p *program) openSet(top string, flows []storedFolder, cache *closedCache) map[string]bool {
	defer cache.save()
	open := make(map[string]bool, len(flows))
	working := make(map[string][]stateRow)
	type stamped struct {
		abs   string
		stamp fileStamp
		ok    bool
	}
	stamps := make(map[string]stamped)
	var keys []string
	for _, flow := range flows {
		key := relativeFlow(top, flow.dir)
		file := filepath.Join(flow.dir, "state.tsv")
		abs, stamp, ok := stampOf(file)
		if ok && cache.closed(abs, stamp) {
			open[key] = false
			continue
		}
		stamps[key] = stamped{abs, stamp, ok}
		rows := rowsOf(file)
		open[key] = openRows(rows)
		if open[key] && flow.store != storageLocal {
			working[key] = rows
			keys = append(keys, key)
		} else if !open[key] && ok && currentEvent(rows) == "done" {
			// No rows, from a failed read or a header-only file, leave the flow
			// closed for this lookup only: a read that failed on an unchanged
			// file would otherwise hide an open flow until the file changed.
			cache.record(abs, stamp)
		}
	}
	ref := p.repo.defaultRef()
	if ref == "" || len(keys) == 0 {
		return open
	}
	tips := tipCopies(top, ref, keys)
	for i, key := range keys {
		if tips[i] != nil && closedByMerge(working[key], tips[i]) {
			open[key] = false
			if s := stamps[key]; s.ok {
				cache.record(s.abs, s.stamp)
			}
		}
	}
	return open
}

// tipCopies returns the state.tsv bytes of each repository-relative folder at
// ref, in the order given, with nil where ref holds no such file.
func tipCopies(top, ref string, keys []string) [][]byte {
	var input strings.Builder
	for _, key := range keys {
		input.WriteString(ref + ":" + key + "/state.tsv\n")
	}
	out, err := gitBytes(top, []byte(input.String()), "cat-file", "--batch")
	copies := make([][]byte, len(keys))
	if err != nil {
		return copies
	}
	for i := range keys {
		end := bytes.IndexByte(out, '\n')
		if end < 0 {
			break
		}
		header := strings.Fields(string(out[:end]))
		out = out[end+1:]
		// A name that resolves to nothing is answered "<name> missing" or
		// "<name> ambiguous" with no body, and the name may itself hold spaces.
		if len(header) > 0 && (header[len(header)-1] == "missing" || header[len(header)-1] == "ambiguous") {
			continue
		}
		if len(header) < 3 {
			break
		}
		size, err := strconv.Atoi(header[len(header)-1])
		if err != nil || size < 0 || size >= len(out) {
			break
		}
		if header[len(header)-2] == "blob" {
			copies[i] = out[:size]
		}
		out = out[size+1:]
	}
	return copies
}

// closedByMerge reports whether the default-tip copy tip holds the flow's final
// verdict. The working rows say whether the plan is sliced: a sliced flow needs
// the latest branch verdict of every slice its latest note slices row lists to
// be ok, and the latest verify final row to be ok when that row says final=yes;
// any other flow needs its
// latest branch row to be an ok verdict for the whole plan. A slice verdict
// alone therefore never closes a flow with no note slices row, even one bound
// to a slice.
func closedByMerge(working []stateRow, tip []byte) bool {
	var note stateRow
	for _, row := range working {
		if row[3] == "note" && row[4] == "slices" {
			note = row
		}
	}
	rows := rowsIn(tip)
	if note == nil {
		var latest stateRow
		for _, row := range rows {
			if row[3] == "branch" {
				latest = row
			}
		}
		// An empty slice= still names a slice verdict, only an unreadable one.
		return latest != nil && latest[5] == "ok" && !cellOf(latest).has("slice")
	}
	want := cellOf(note).list("slices")
	needFinal := slices.Contains(cellOf(note).list("final"), "yes")
	// Rows are in ts order, so the last verdict per slice and the last final
	// review win, as the latest branch row does for a whole-plan flow: a later
	// failed verdict takes back an earlier ok one.
	merged := make(map[string]bool)
	finalOK := false
	for _, row := range rows {
		switch {
		case row[3] == "branch":
			for _, slice := range cellOf(row).list("slice") {
				merged[slice] = row[5] == "ok"
			}
		case row[3] == "verify" && row[4] == "final":
			finalOK = row[5] == "ok"
		}
	}
	if len(want) == 0 || (needFinal && !finalOK) {
		return false
	}
	for _, slice := range want {
		if !merged[slice] {
			return false
		}
	}
	return true
}
