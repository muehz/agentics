package flow

import (
	"os"
	"sort"
	"strings"
)

type stateRow []string

func rowsOf(path string) []stateRow {
	countStateRead(path)
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return rowsIn(body)
}

// rowsIn parses state.tsv bytes as rowsOf does: identical rows count once, and
// rows are ordered by ts with file order as the tie-break. Every line is read,
// since a file starts with the header row or, when older, with a version line
// before it: comment lines, header rows and lines short of nine cells are
// skipped wherever they appear.
func rowsIn(body []byte) []stateRow {
	var rows []stateRow
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		cells := strings.Split(line, "\t")
		if len(cells) < 9 || cells[0] == "ts" || seen[line] {
			continue
		}
		seen[line] = true
		rows = append(rows, cells)
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
	return rows
}

func currentEvent(rows []stateRow) string {
	if len(rows) == 0 {
		return ""
	}
	return rows[len(rows)-1][3]
}

// openRows applies the current-row rule: a flow is open while it has rows and
// its current row is not done, so a row after done reopens it.
func openRows(rows []stateRow) bool {
	event := currentEvent(rows)
	return event != "" && event != "done"
}

func bindTS(path, branch string) string {
	stored := cleanCell(branch)
	var timestamp string
	for _, row := range rowsOf(path) {
		if row[3] == "bind" && row[4] == stored {
			timestamp = row[0]
		}
	}
	return timestamp
}
