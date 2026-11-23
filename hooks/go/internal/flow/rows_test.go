package flow

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestRowsOfFiltersSortsAndDeduplicates(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "state.tsv")
	a := row("2026-01-02T00:00:00Z", "spec", "enter", "a", "ok", "a", "none", "spec go")
	b := row("2026-01-01T00:00:00Z", "intent", "create", "b", "ok", "b", "none", "spec go")
	c := row("2026-01-02T00:00:00Z", "spec", "pause", "c", "ok", "c", "none", "spec go") + "\textra\r"
	body := strings.Join([]string{legacyVersionLine, headerRow, a, b, "short\trow", legacyVersionLine, headerRow, c, a}, "\n")
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := rowsOf(file)
	want := []stateRow{strings.Split(b, "\t"), strings.Split(a, "\t"), strings.Split(c, "\t")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %#v, want %#v", got, want)
	}
}

// A file the helper creates starts with the header row, so its first row is
// its second line; a legacy file has a version line before the header row.
// Both read every row, and a comment line anywhere is no row, even one with
// nine cells.
func TestRowsInReadsEveryRowWithOrWithoutAVersionLine(t *testing.T) {
	t.Parallel()
	first := row("2026-01-01T00:00:00Z", "intent", "create", "intent.md", "ok", "first", "intent.md", "spec go")
	second := row("2026-01-02T00:00:00Z", "intent", "challenge", "intent.md", "ok", "second", "none", "spec go")
	comment := "#" + row("2026-01-03T00:00:00Z", "spec", "create", "spec.md", "ok", "comment", "spec.md", "decompose go")
	want := []stateRow{strings.Split(first, "\t"), strings.Split(second, "\t")}
	for name, body := range map[string]string{
		"new":               headerRow + "\n" + first + "\n" + second + "\n",
		"legacy":            legacyVersionLine + "\n" + headerRow + "\n" + first + "\n" + second + "\n",
		"comment-elsewhere": headerRow + "\n" + first + "\n" + comment + "\n" + second + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := rowsIn([]byte(body)); !reflect.DeepEqual(got, want) {
				t.Fatalf("rows = %q, want %q", got, want)
			}
		})
	}
}

// Appending to a legacy file keeps its version line, and the appended row is
// read after the first one.
func TestAppendKeepsALegacyVersionLine(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	first := row("2026-01-01T00:00:00Z", "intent", "create", "intent.md", "ok", "first", "intent.md", "spec go")
	writeFile(t, repo, "f/state.tsv", legacyVersionLine+"\n"+headerRow+"\n"+first+"\n")
	appendTestState(t, repo, "f", "intent", "challenge", "intent.md", "ok", "second", "none", "spec go")
	lines := readLines(t, filepath.Join(repo, "f", "state.tsv"))
	if len(lines) != 4 || lines[0] != legacyVersionLine || lines[1] != headerRow || lines[2] != first {
		t.Fatalf("legacy file rewritten: %q", lines)
	}
	if got := rowsOf(filepath.Join(repo, "f", "state.tsv")); len(got) != 2 || got[0][6] != "first" || got[1][6] != "second" {
		t.Fatalf("rows = %q, want first and second", got)
	}
}

func TestRowsInParsesBytesAsRowsOfParsesTheFile(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	writeState(t, repo, "f",
		row("2026-01-02T00:00:00Z", "spec", "enter", "a", "ok", "a", "none", "spec go"),
		row("2026-01-01T00:00:00Z", "intent", "create", "b", "ok", "b", "none", "spec go"),
		row("2026-01-02T00:00:00Z", "spec", "enter", "a", "ok", "a", "none", "spec go"))
	file := filepath.Join(repo, "f", "state.tsv")
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	got, want := rowsIn(body), rowsOf(file)
	if len(want) != 2 || !reflect.DeepEqual(got, want) {
		t.Fatalf("rowsIn = %q, rowsOf = %q", got, want)
	}
}

func TestTipCopiesKeepsOrderAcrossMissingFiles(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeState(t, repo, "docs/specs/2026-11/a", row("2026-01-01T00:00:00Z", "intent", "create", "a", "ok", "a", "none", "spec go"))
	writeState(t, repo, "docs/specs/2026-11/c", row("2026-01-01T00:00:00Z", "intent", "create", "c", "ok", "c", "none", "spec go"))
	commitAll(t, repo, "flows")
	copies := tipCopies(repo, "main", []string{"docs/specs/2026-11/a", "docs/specs/2026-11/b", "docs/specs/2026-11/c"})
	if len(copies) != 3 || copies[1] != nil {
		t.Fatalf("copies = %q, want three with the missing one nil", copies)
	}
	for i, target := range map[int]string{0: "a", 2: "c"} {
		if rows := rowsIn(copies[i]); len(rows) != 1 || rows[0][4] != target {
			t.Errorf("copy %d rows = %q, want target %s", i, rows, target)
		}
	}
}

func TestTipCopiesReadsPastAMissingPathWithASpace(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeState(t, repo, "docs/specs/2026-11/c", row("2026-01-01T00:00:00Z", "intent", "create", "c", "ok", "c", "none", "spec go"))
	commitAll(t, repo, "flows")
	copies := tipCopies(repo, "main", []string{"docs/specs/2026-11/b x", "docs/specs/2026-11/c"})
	if len(copies) != 2 || copies[0] != nil {
		t.Fatalf("copies = %q, want two with the missing one nil", copies)
	}
	if rows := rowsIn(copies[1]); len(rows) != 1 || rows[0][4] != "c" {
		t.Fatalf("copy after the missing path rows = %q, want target c", rows)
	}
}

func TestRowsOfKeepsFileOrderForEqualTimestamps(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	var lines []string
	var want []string
	for i := range 40 {
		lines = append(lines, row("2026-02-01T00:00:00Z", "spec", "pause", strconv.Itoa(i), "ok", "later", "none", "spec go"))
		lines = append(lines, row("2026-01-01T00:00:00Z", "spec", "pause", "earlier-"+strconv.Itoa(i), "ok", "earlier", "none", "spec go"))
		want = append(want, "earlier-"+strconv.Itoa(i))
	}
	for i := range 40 {
		want = append(want, strconv.Itoa(i))
	}
	writeState(t, repo, "f", lines...)
	var got []string
	for _, row := range rowsOf(filepath.Join(repo, "f", "state.tsv")) {
		got = append(got, row[4])
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("timestamp tie order = %q, want %q", got, want)
	}
}

// Every line is read: a file with no version line and no header row still
// loses none of its first rows.
func TestRowsOfReadsTheFirstLinesAsRows(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	valid := row("2026-01-01T00:00:00Z", "intent", "create", "kept", "ok", "why", "none", "spec go")
	writeFile(t, repo, "state.tsv", strings.Replace(valid, "kept", "first", 1)+"\n"+strings.Replace(valid, "kept", "second", 1)+"\n"+valid+"\n")
	got := rowsOf(filepath.Join(repo, "state.tsv"))
	if len(got) != 3 || got[0][4] != "first" || got[1][4] != "second" || got[2][4] != "kept" {
		t.Fatalf("rows = %q, want first, second and kept", got)
	}
}

func TestRowLookups(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	file := filepath.Join(repo, "f", "state.tsv")
	if got := rowsOf(file); len(got) != 0 {
		t.Fatalf("missing rows = %q", got)
	}
	if openRows(rowsOf(file)) || currentEvent(rowsOf(file)) != "" || bindTS(file, "topic") != "" {
		t.Fatal("missing state has lookup results")
	}
	writeState(t, repo, "f",
		row("2026-01-02T00:00:00Z", "intent", "bind", "'+topic", "ok", "bound", "none", "spec go"),
		row("2026-01-01T00:00:00Z", "intent", "bind", "'+topic", "ok", "bound", "none", "spec go"),
		row("2026-01-03T00:00:00Z", "spec", "pause", "'+topic", "pending", "waiting", "none", "spec go"))
	if !openRows(rowsOf(file)) || currentEvent(rowsOf(file)) != "pause" || bindTS(file, "+topic") != "2026-01-02T00:00:00Z" {
		t.Fatal("active/bind lookups do not follow sorted rows and guarded branch")
	}
	if bindTS(file, "other") != "" {
		t.Fatal("bind matched a different branch")
	}
	writeState(t, repo, "f", row("2026-01-04T00:00:00Z", "gate", "done", "branch", "ok", "done", "none", "gate go"))
	if openRows(rowsOf(file)) || currentEvent(rowsOf(file)) != "done" || bindTS(file, "+topic") != "" {
		t.Fatal("done state is active")
	}
	if err := os.Mkdir(filepath.Join(repo, "directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if openRows(rowsOf(filepath.Join(repo, "directory"))) {
		t.Fatal("directory is active")
	}
}

const unionFolder = "docs/specs/2026-10/feature-a"

func unionRepo(t *testing.T) string {
	t.Helper()
	const unionRule = "docs/specs/**/state.tsv merge=union"
	attrs, err := os.ReadFile(filepath.Join("..", "..", "..", "..", ".gitattributes"))
	// Mutation testing runs the package in a copy of the Go module alone, where the
	// repository's .gitattributes is absent; the rule itself is what the merge needs.
	if errors.Is(err, fs.ErrNotExist) {
		attrs, err = []byte(unionRule+"\n"), nil
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(attrs), unionRule) {
		t.Fatal("state.tsv union merge attribute missing")
	}
	repo := newRepo(t)
	writeFile(t, repo, ".gitattributes", string(attrs))
	commitAll(t, repo, "attributes")
	return repo
}

func appendTestState(t *testing.T, repo, folder string, values ...string) {
	t.Helper()
	if len(values) != 7 {
		t.Fatal("test row needs seven values")
	}
	if err := appendState(filepath.Join(repo, filepath.FromSlash(folder)), folder, "s1", [7]string(values), fixedStateClock); err != nil {
		t.Fatal(err)
	}
}

func mergedRows(t *testing.T, repo string) []string {
	t.Helper()
	var out []string
	for _, line := range readLines(t, filepath.Join(repo, unionFolder, "state.tsv")) {
		if line == headerRow {
			out = append(out, line)
			continue
		}
		c := cells(line)
		if len(c) != 9 {
			t.Fatalf("merged row has %d cells: %q", len(c), line)
		}
		out = append(out, strings.Join(c[2:], "\t"))
	}
	return out
}

func TestStateTSVUnionMergeKeepsBothSidesRows(t *testing.T) {
	t.Parallel()
	repo := unionRepo(t)
	appendTestState(t, repo, unionFolder, "intent", "create", "intent.md", "ok", "base", "intent.md", "spec go")
	commitAll(t, repo, "base")
	git(t, repo, "switch", "-q", "-c", "left")
	appendTestState(t, repo, unionFolder, "spec", "enter", "spec.md", "ok", "left row", "none", "spec go")
	commitAll(t, repo, "left")
	git(t, repo, "switch", "-q", "main")
	git(t, repo, "switch", "-q", "-c", "right")
	appendTestState(t, repo, unionFolder, "spec", "pause", "spec.md", "ok", "right row", "none", "spec go")
	commitAll(t, repo, "right")
	git(t, repo, "merge", "-q", "left", "-m", "merge left")
	want := []string{headerRow, "intent\tcreate\tintent.md\tok\tbase\tintent.md\tspec go", "spec\tpause\tspec.md\tok\tright row\tnone\tspec go", "spec\tenter\tspec.md\tok\tleft row\tnone\tspec go"}
	if got := mergedRows(t, repo); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged rows = %q, want %q", got, want)
	}
	if got := rowsOf(filepath.Join(repo, unionFolder, "state.tsv")); len(got) != 3 {
		t.Fatalf("merged reader kept %d rows", len(got))
	}
	mustExit(t, runFlow(t, repo, nil, "status", "--folder", unionFolder), 0, "status after union merge")
}

func TestStateTSVUnionMergeOfTwoCreatedFilesKeepsEveryRow(t *testing.T) {
	t.Parallel()
	repo := unionRepo(t)
	git(t, repo, "switch", "-q", "-c", "left")
	appendTestState(t, repo, unionFolder, "intent", "create", "intent.md", "ok", "left row", "intent.md", "spec go")
	appendTestState(t, repo, unionFolder, "intent", "challenge", "intent.md", "ok", "left second", "none", "spec go")
	commitAll(t, repo, "left")
	git(t, repo, "switch", "-q", "main")
	git(t, repo, "switch", "-q", "-c", "right")
	appendTestState(t, repo, unionFolder, "intent", "create", "intent.md", "ok", "right row", "intent.md", "spec go")
	commitAll(t, repo, "right")
	git(t, repo, "merge", "-q", "left", "-m", "merge left")
	want := []string{headerRow, "intent\tcreate\tintent.md\tok\tright row\tintent.md\tspec go", "intent\tcreate\tintent.md\tok\tleft row\tintent.md\tspec go", "intent\tchallenge\tintent.md\tok\tleft second\tnone\tspec go"}
	if got := mergedRows(t, repo); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged rows = %q, want %q", got, want)
	}
	if got := rowsOf(filepath.Join(repo, unionFolder, "state.tsv")); len(got) != 3 {
		t.Fatalf("merged reader kept %d rows", len(got))
	}
	mustExit(t, runFlow(t, repo, nil, "status", "--folder", unionFolder), 0, "status after union merge")
}
