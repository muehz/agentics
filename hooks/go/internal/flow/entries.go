package flow

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Headings of the two sections whose items a row records one at a time. Both
// sections are left out of an artifact's approved text, so an entry is bound
// by its own hash instead.
const (
	standingOrdersHeading = "## Standing orders"
	divergencesHeading    = "## Divergences"
)

var (
	entryMarker = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^([-*+]|[0-9]+[.)]) `) })
	// The mark may wrap so that the date sits on the next line; the whitespace
	// before it goes too, so a marked entry hashes as it did before the mark.
	reconciledMark = sync.OnceValue(func() *regexp.Regexp {
		return regexp.MustCompile(`[ \t\n]*Reconciled:[ \t]*(\n[ \t]*)?[0-9]{4}-[0-9]{2}-[0-9]{2}\.$`)
	})
)

// sectionEntry is one top-level list item of a section. number counts every
// item of the section from 1, reconciled or not, so that a number written in
// a row keeps naming the same item once an earlier one is reconciled. text is
// the item as hashed, without its marker or reconciled mark, so a reader of
// the item's content sees exactly what a row records.
type sectionEntry struct {
	number     int
	hash       string
	reconciled bool
	text       string
}

// artifactEntries returns the entries of the section under heading in
// artifact in dir, and false when the artifact is not a readable regular file.
func artifactEntries(dir, artifact, heading string) ([]sectionEntry, bool) {
	file := filepath.Join(dir, artifact)
	if info, err := os.Stat(file); err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	body, err := os.ReadFile(file)
	if err != nil {
		return nil, false
	}
	return sectionEntries(body, heading), true
}

// pickEntry returns the entry numbered which, or the last entry when which is
// last, and false when the section holds no such entry.
func pickEntry(entries []sectionEntry, which string) (sectionEntry, bool) {
	if which == "last" {
		if len(entries) == 0 {
			return sectionEntry{}, false
		}
		return entries[len(entries)-1], true
	}
	n, err := strconv.Atoi(which)
	if err != nil || n < 1 || n > len(entries) {
		return sectionEntry{}, false
	}
	return entries[n-1], true
}

// divergeArtifact returns the artifact whose divergences a diverge row with
// target names: the chain artifact the target's first word names, and
// otherwise, for a task or the run, plan.md, or brief.md in a folder with no plan.
func divergeArtifact(dir, target string) string {
	first, _, _ := strings.Cut(target, " ")
	if slices.Contains(textArtifacts, first) {
		return first
	}
	if info, err := os.Stat(filepath.Join(dir, "plan.md")); err == nil && info.Mode().IsRegular() {
		return "plan.md"
	}
	return "brief.md"
}

// sectionEntries splits every occurrence of the section under heading in body
// into its top-level list items, in file order. Lines are trimmed as the
// second hash rule trims them, so a copy with other line endings or trailing
// blanks yields the same hashes.
func sectionEntries(body []byte, heading string) []sectionEntry {
	var items [][][]byte
	inSection, inItem := false, false
	for _, l := range artifactLines(body) {
		line := l.text
		// Sections are found as textHash finds them, so a heading in a fenced
		// code block opens or closes none. A fenced line that is no heading still
		// takes the item rule below.
		if l.section {
			inSection, inItem = string(line) == heading, false
			continue
		}
		switch {
		case !inSection:
		case entryMarker().Match(line):
			items = append(items, [][]byte{line})
			inItem = true
		case inItem && (len(line) == 0 || line[0] == ' ' || line[0] == '\t'):
			items[len(items)-1] = append(items[len(items)-1], line)
		default:
			inItem = false
		}
	}
	entries := make([]sectionEntry, 0, len(items))
	for i, lines := range items {
		// Trailing blank lines of an item are no part of it, so the mark can close the text.
		text := bytes.TrimRight(bytes.Join(lines, []byte{'\n'}), "\n")
		// The marker is no part of the hash, so that renumbering the list or
		// changing an item's marker keeps the entry's record. Two entries with
		// the same text after the marker therefore share a hash, and one row
		// records both.
		text = text[len(entryMarker().Find(text)):]
		stripped := reconciledMark().ReplaceAll(text, nil)
		reconciled := len(stripped) != len(text)
		sum := sha256.Sum256(append(stripped, '\n'))
		entries = append(entries, sectionEntry{number: i + 1, hash: hex.EncodeToString(sum[:]), reconciled: reconciled, text: string(stripped)})
	}
	return entries
}
