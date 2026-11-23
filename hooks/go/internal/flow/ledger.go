package flow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The ledger records, in one clone, the rows written there that a read must
// tell from rows that arrived by commit: approvals written through the content
// guard (approve), standing orders (order), rule divergences (rule), commit
// rows (commit) and finding waivers (waive). A state.tsv row can be copied or
// committed into another clone; the ledger cannot, because it lives in the Git
// directory, which Git neither commits nor lists.

// ledgerEntry is one ledger line without its timestamp, the part a lookup compares.
type ledgerEntry struct {
	kind, folder, target, hash string
}

var errNoLedger = errors.New("flow: no ledger outside a Git repository")

// ledgerRecordOf returns the kind and hash of the ledger line a state row of the
// given event, target, and evidence cell records, and ok false when the row
// records nothing:
//
//   - approve gives the hash of approved(), of either rule, and nothing when
//     that hash is empty;
//   - order with an entry= pair gives that hash;
//   - diverge whose target is two words ending in rule, with an entry= pair,
//     gives rule and that hash;
//   - commit with exactly one commit= value gives that value as written;
//   - waive whose first word by strings.Fields is finding gives -; and
//   - every other row gives ok false.
//
// It shapes the row only. The writer adds two conditions of its own, that an
// approval is the guarded one whose hash the helper computed from the file and
// that an approval or a waiver has status ok, and a reader asks only of valid
// rows. A waive target is split here with strings.Fields and by a reader at an
// ASCII space, which agree on every valid row because parseWaive refuses a
// first word that holds another space character.
func ledgerRecordOf(event, target string, cell evidenceCell) (kind, hash string, ok bool) {
	switch event {
	case "approve":
		if _, hash := cell.approved(); hash != "" {
			return "approve", hash, true
		}
	case "order":
		if hash := cell.first("entry"); hash != "" {
			return "order", hash, true
		}
	case "diverge":
		words := strings.Fields(target)
		if hash := cell.first("entry"); hash != "" && len(words) == 2 && words[1] == "rule" {
			return "rule", hash, true
		}
	case "commit":
		if shas := cell.values("commit"); len(shas) == 1 && shas[0] != "" {
			return "commit", shas[0], true
		}
	case "waive":
		if words := strings.Fields(target); len(words) > 0 && words[0] == "finding" {
			return "waive", "-", true
		}
	}
	return "", "", false
}

func ledgerFile(common string) string {
	return filepath.Join(common, "agentics", "flow", "ledger.tsv")
}

// resolvedPath follows symlinks in path, and keeps path as given when it
// cannot, such as for a folder not created yet.
func resolvedPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

// appendLedger appends entry to the ledger in the common Git directory common
// with one write on an append handle, as appendState appends a row.
func appendLedger(common string, entry ledgerEntry, now func() time.Time) error {
	if common == "" {
		return errNoLedger
	}
	fields := []string{now().UTC().Format("2006-01-02T15:04:05Z"), entry.kind, entry.folder, entry.target, entry.hash}
	for _, field := range fields {
		// A separator inside a field would split it into other fields or another
		// line, which a lookup could then match.
		if field == "" || strings.ContainsAny(field, "\t\r\n") {
			return errors.New("flow: a ledger field is empty or holds a separator")
		}
	}
	file := ledgerFile(common)
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return err
	}
	writeErr := writeStateBytes(f, []byte(strings.Join(fields, "\t")+"\n"))
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// readLedger returns the entries of the ledger in the common Git directory common,
// each with the latest timestamp among its lines, so that a lookup can tell a
// record written after a revoke row from one written before it. It is empty
// outside a repository, without Git, or when no line was written. A line
// without five fields is skipped.
func readLedger(common string) map[ledgerEntry]string {
	entries := make(map[ledgerEntry]string)
	if common == "" {
		return entries
	}
	body, err := os.ReadFile(ledgerFile(common))
	if err != nil {
		return entries
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 5 {
			continue
		}
		// The timestamp has one fixed width, so the later of two sorts last.
		entry := ledgerEntry{fields[1], fields[2], fields[3], fields[4]}
		if fields[0] > entries[entry] {
			entries[entry] = fields[0]
		}
	}
	return entries
}
