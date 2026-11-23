package skillcompat

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// listHeader is the first line of a list file: the six column names, separated by tabs.
const listHeader = "path\ttool\tdigest\treason\trule\tnote"

// byteOrderMark is the UTF-8 encoding of U+FEFF, spelled as bytes because Go source may not hold the character.
const byteOrderMark = "\xef\xbb\xbf"

// listEntry is one line of a list file. It stands for every block of its file that has its tool and digest.
type listEntry struct {
	// line is the 1-based line of the list file the entry was read from; it is 0 for an entry built in memory.
	line int
	// path is the Markdown file, relative to the walked root, with forward slashes.
	path string
	// tool is toolBash, toolPowerShell or toolWrite.
	tool string
	// digest is bodyDigest of the block body as written.
	digest string
	// reason is the category the runner reports for the denial.
	reason string
	// rule is a heading of the hook README that states the denial as a rule.
	rule string
	// note is free text for a reader; nothing checks it.
	note string
}

// listDigest matches what bodyDigest returns.
var listDigest = regexp.MustCompile(`^[0-9a-f]{12}$`)

// blockKey is what an entry and a block are matched on.
type blockKey struct{ path, tool, digest string }

// compareKeys orders the entries of a list by path, then digest, then tool, in byte order. The digest comes before
// the tool so that the entries of one block body stay adjacent when a Bash block and a PowerShell block share it.
func compareKeys(a, b blockKey) int {
	if c := strings.Compare(a.path, b.path); c != 0 {
		return c
	}
	if c := strings.Compare(a.digest, b.digest); c != 0 {
		return c
	}
	return strings.Compare(a.tool, b.tool)
}

// listRow is one entry line of a list file before its own columns are named.
type listRow struct {
	// line is the 1-based line of the list file.
	line int
	// key holds the first three fields, which every list shares: path, tool and digest.
	key blockKey
	// rest holds the fields after the digest, in file order.
	rest []string
}

// readRows returns the entry lines of the list file text, in file order, applying the rules every list shares.
// name is used only to label errors, each of which starts with name and the 1-based line number. The first line
// must be header exactly, and header fixes the number of tab-separated fields of every later line. A line needs a
// non-empty path, a known tool and a digest of 12 lower-case hexadecimal characters; check then judges the line's
// remaining fields and returns the fault, or "" for none. Lines must be strictly ascending by compareKeys, so a
// file has one valid order and no two entries share path, tool and digest. Line endings may be LF or CRLF; a byte
// order mark is rejected, because it would otherwise read as part of the first column name.
func readRows(name, text, header string, check func(row listRow) string) ([]listRow, error) {
	fail := func(line int, format string, args ...any) ([]listRow, error) {
		return nil, fmt.Errorf("%s:%d: %s", name, line, fmt.Sprintf(format, args...))
	}
	if strings.HasPrefix(text, byteOrderMark) {
		return fail(1, "the file starts with a byte order mark")
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 || strings.TrimSuffix(lines[0], "\r") != header {
		return fail(1, "the header is not %q", header)
	}
	width := strings.Count(header, "\t") + 1

	var rows []listRow
	for i, line := range lines[1:] {
		number := i + 2
		fields := strings.Split(strings.TrimSuffix(line, "\r"), "\t")
		if len(fields) != width {
			return fail(number, "%d fields, want %d separated by tabs", len(fields), width)
		}
		row := listRow{line: number, key: blockKey{path: fields[0], tool: fields[1], digest: fields[2]}, rest: fields[3:]}
		switch {
		case row.key.path == "":
			return fail(number, "the path is empty")
		case row.key.tool != toolBash && row.key.tool != toolPowerShell && row.key.tool != toolWrite:
			return fail(number, "unknown tool %q, want %s, %s or %s", row.key.tool, toolBash, toolPowerShell, toolWrite)
		case !listDigest.MatchString(row.key.digest):
			return fail(number, "the digest %q is not 12 lower-case hexadecimal characters", row.key.digest)
		}
		if fault := check(row); fault != "" {
			return fail(number, "%s", fault)
		}
		if len(rows) > 0 {
			switch c := compareKeys(rows[len(rows)-1].key, row.key); {
			case c == 0:
				return fail(number, "duplicate entry for %s block %s of %s", row.key.tool, row.key.digest, row.key.path)
			case c > 0:
				return fail(number, "entries are not sorted by path, then digest, then tool")
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// readList returns the entries of the known-denial list file text, in file order, under the rules of readRows with
// listHeader as the header. An entry also needs a non-empty reason and rule; its note may be empty.
func readList(name, text string) ([]listEntry, error) {
	rows, err := readRows(name, text, listHeader, func(row listRow) string {
		switch {
		case row.rest[0] == "":
			return "the reason is empty"
		case row.rest[1] == "":
			return "the rule is empty"
		}
		return ""
	})
	if err != nil {
		return nil, err
	}
	var entries []listEntry
	for _, row := range rows {
		entries = append(entries, listEntry{
			line: row.line, path: row.key.path, tool: row.key.tool, digest: row.key.digest,
			reason: row.rest[0], rule: row.rest[1], note: row.rest[2],
		})
	}
	return entries, nil
}

// markdownFiles returns the path of every file under root whose name ends in ".md", relative to root with forward
// slashes, in byte order, so that a walk does not depend on the host's directory order or path separator.
func markdownFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	return paths, nil
}

// headingSet returns the headings of the Markdown text that a list entry may name as its rule: the text of every
// line that starts with "## " or "### ", without the marker and without surrounding white space. Deeper headings
// and the title are left out because the hook README states its rules in sections of these two levels. Lines of
// fenced blocks are skipped under fenceTracker's rule, because a line of a block is an example, not a section a
// rule is stated in; a fence that never closes skips the rest of the text.
func headingSet(text string) map[string]bool {
	headings := map[string]bool{}
	var fences fenceTracker
	for _, line := range strings.Split(text, "\n") {
		kind, line := fences.next(line)
		if kind != lineOutside {
			continue
		}
		for _, marker := range []string{"## ", "### "} {
			if rest, ok := strings.CutPrefix(line, marker); ok {
				if heading := strings.TrimSpace(rest); heading != "" {
					headings[heading] = true
				}
			}
		}
	}
	return headings
}

// walkBlocks returns the fenced blocks of every file under root whose name ends in ".md", and the number of such
// files. Each block's path is its file's path relative to root with forward slashes, which is the path a list
// entry names. Files are read in byte order of those paths and blocks keep their order in the file, so the result
// does not depend on the host's directory order or path separator. A fence that never closes is an error.
func walkBlocks(root string) (blocks []block, files int, err error) {
	paths, err := markdownFiles(root)
	if err != nil {
		return nil, 0, err
	}
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return nil, 0, err
		}
		found, err := extractBlocks(path, string(data))
		if err != nil {
			return nil, 0, err
		}
		blocks = append(blocks, found...)
	}
	return blocks, len(paths), nil
}

// walkNonEmpty is walkBlocks for a root that must hold Markdown: it is an error when no ".md" file is found, so
// that a sweep pointed at a wrong or emptied directory fails instead of passing with nothing checked.
func walkNonEmpty(root string) ([]block, error) {
	blocks, files, err := walkBlocks(root)
	if err != nil {
		return nil, err
	}
	if files == 0 {
		return nil, fmt.Errorf("no .md file under %s", root)
	}
	return blocks, nil
}

// ranBlock is a block and what the hook decided about it.
type ranBlock struct {
	// block is the block as written: its body is the one the digest is taken from, before any placeholder is
	// substituted.
	block block
	// result is the hook's answer to the block, run after substitution for a command block.
	result runResult
}

// runBlocks runs every block through the built hook binary, each in its own empty directory outside any
// repository, and returns the results in the order of blocks. A block is sent as sentBlock gives it, a command
// block with its placeholders substituted; the returned block keeps the body as written. The runs are parallel
// sub-tests, each writing only its own element of the result. The test is skipped on a platform outside the build contract and stops when a run
// cannot be made.
func runBlocks(t *testing.T, blocks []block) []ranBlock {
	t.Helper()
	runnerBinary(t)
	ran := make([]ranBlock, len(blocks))
	// The group returns only when every parallel sub-test in it has finished.
	finished := t.Run("run", func(t *testing.T) {
		for i, b := range blocks {
			t.Run(fmt.Sprintf("%s:%d", b.path, b.line), func(t *testing.T) {
				t.Parallel()
				ran[i] = ranBlock{block: b, result: runBlock(t, sentBlock(b), t.TempDir(), outsideRepository)}
			})
		}
	})
	if !finished {
		t.FailNow()
	}
	return ran
}

// matchInput is what matchDenials compares. It is a struct so that a further list of entries is a new field rather
// than a new parameter at every call.
type matchInput struct {
	// blocks are the blocks of the walked tree with their results.
	blocks []ranBlock
	// denials are the entries of the known-denial list.
	denials []listEntry
	// headings is the heading set of the hook README, from headingSet.
	headings map[string]bool
	// policyOff are the entries of the policy-off list; it may be empty.
	policyOff []policyOffEntry
}

// matchDenials compares the blocks with the known-denial list and returns the problems, sorted, or nothing when
// they agree. A block and an entry match when path, tool and digest are equal, so one entry covers every such block
// of its file. The problems are: a denied block that no entry of either list matches; an allowed block that a
// known-denial entry matches; an entry of either list that matches no block; a known-denial entry whose reason is
// not the category the hook reported; a known-denial entry whose rule is not in the heading set; a known-denial
// entry that is also on the policy-off list, because a block is denied either always or only under a policy
// setting; and every block whose run is a fault, which is reported once with the fault's message and otherwise
// neither confirms nor refutes its entry. A policy-off entry only accounts for a denial here: whether its block is
// denied under default policy and allowed with its key off is checkPolicyOff's to report. A block problem starts
// with the file path and the block's opening line; an entry problem starts with the file path and names the
// entry's list line, tool and digest.
func matchDenials(in matchInput) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	listed := map[blockKey]listEntry{}
	for _, entry := range in.denials {
		listed[blockKey{entry.path, entry.tool, entry.digest}] = entry
	}
	policyOff := map[blockKey]policyOffEntry{}
	for _, entry := range in.policyOff {
		policyOff[entry.blockKey()] = entry
	}
	matched := map[blockKey]bool{}
	for _, ran := range in.blocks {
		b, result := ran.block, ran.result
		key := blockKey{b.path, b.tool, bodyDigest(b.body)}
		entry, isListed := listed[key]
		matched[key] = true
		switch result.verdict {
		case verdictFault:
			report("%s:%d: %s block %s: the hook gave no verdict: %s", b.path, b.line, b.tool, key.digest, result.message)
		case verdictDenied:
			_, isPolicyOff := policyOff[key]
			switch {
			case !isListed && isPolicyOff:
			case !isListed:
				report("%s:%d: %s block %s is denied and no entry lists it: %s",
					b.path, b.line, b.tool, key.digest, result.message)
			case entry.reason != result.category:
				report("%s:%d: %s block %s: list line %d gives the reason %q but the hook reported %q: %s",
					b.path, b.line, b.tool, key.digest, entry.line, entry.reason, result.category, result.message)
			}
		case verdictAllowed:
			if isListed {
				report("%s:%d: %s block %s is allowed but list line %d lists it as denied",
					b.path, b.line, b.tool, key.digest, entry.line)
			}
		}
	}
	for _, entry := range in.denials {
		if !matched[blockKey{entry.path, entry.tool, entry.digest}] {
			report("%s: list line %d: the entry for %s block %s matches no block",
				entry.path, entry.line, entry.tool, entry.digest)
		}
		if !in.headings[entry.rule] {
			report("%s: list line %d: the entry for %s block %s names the rule %q, which is not a heading of the hook README",
				entry.path, entry.line, entry.tool, entry.digest, entry.rule)
		}
		if other, both := policyOff[blockKey{entry.path, entry.tool, entry.digest}]; both {
			report("%s: list line %d: the entry for %s block %s is also on the policy-off list, at line %d with the key %s",
				entry.path, entry.line, entry.tool, entry.digest, other.line, other.key)
		}
	}
	for _, entry := range in.policyOff {
		if !matched[entry.blockKey()] {
			report("%s: policy-off list line %d: the entry for %s block %s with the key %s matches no block",
				entry.path, entry.line, entry.tool, entry.digest, entry.key)
		}
	}
	slices.Sort(problems)
	return problems
}

// listFixture returns the text of a file under testdata/list.
func listFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "list", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// listText returns a list file: the header and one line per row, each row being its six fields.
func listText(rows ...[]string) string {
	var text strings.Builder
	text.WriteString(listHeader + "\n")
	for _, row := range rows {
		text.WriteString(strings.Join(row, "\t") + "\n")
	}
	return text.String()
}

func TestListReader(t *testing.T) {
	const (
		reset  = "f80c35de4baf"
		status = "ecb33f773d47"
		rule   = "Command-guardian threat categories"
	)
	row := func(path, tool, digest string) []string {
		return []string{path, tool, digest, "destructive_git_ops", rule, "note"}
	}
	with := func(index int, value string) []string {
		fields := row("demo/guide.md", toolBash, reset)
		fields[index] = value
		return fields
	}
	good := listFixture(t, "good.tsv")
	fixtureEntry := listEntry{
		line: 2, path: "demo/guide.md", tool: toolBash, digest: reset, reason: "destructive_git_ops", rule: rule,
		note: "the guide shows the command it warns against",
	}

	for _, tc := range []struct {
		name string
		text string
		want []listEntry
	}{
		{name: "fixture", text: good, want: []listEntry{fixtureEntry}},
		{name: "CRLF", text: strings.ReplaceAll(good, "\n", "\r\n"), want: []listEntry{fixtureEntry}},
		{name: "no final line ending", text: strings.TrimSuffix(good, "\n"), want: []listEntry{fixtureEntry}},
		{name: "header only", text: listHeader + "\n"},
		{
			name: "same path and digest, different tool",
			text: listText(
				row("demo/guide.md", toolBash, reset),
				row("demo/guide.md", toolPowerShell, reset),
				row("demo/guide.md", toolWrite, reset),
			),
			want: []listEntry{
				{line: 2, path: "demo/guide.md", tool: toolBash, digest: reset, reason: "destructive_git_ops", rule: rule, note: "note"},
				{line: 3, path: "demo/guide.md", tool: toolPowerShell, digest: reset, reason: "destructive_git_ops", rule: rule, note: "note"},
				{line: 4, path: "demo/guide.md", tool: toolWrite, digest: reset, reason: "destructive_git_ops", rule: rule, note: "note"},
			},
		},
		{
			// The path decides before the digest, and the digest before the tool.
			name: "sort keys in order",
			text: listText(
				row("a.md", toolWrite, reset),
				row("b.md", toolWrite, status),
				row("b.md", toolBash, reset),
			),
			want: []listEntry{
				{line: 2, path: "a.md", tool: toolWrite, digest: reset, reason: "destructive_git_ops", rule: rule, note: "note"},
				{line: 3, path: "b.md", tool: toolWrite, digest: status, reason: "destructive_git_ops", rule: rule, note: "note"},
				{line: 4, path: "b.md", tool: toolBash, digest: reset, reason: "destructive_git_ops", rule: rule, note: "note"},
			},
		},
		{
			name: "empty note",
			text: listText(with(5, "")),
			want: []listEntry{
				{line: 2, path: "demo/guide.md", tool: toolBash, digest: reset, reason: "destructive_git_ops", rule: rule},
			},
		},
	} {
		t.Run("accepts "+tc.name, func(t *testing.T) {
			got, err := readList("list.tsv", tc.text)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name string
		text string
		// line is the line the error must name, and about a word of its message.
		line  int
		about string
	}{
		{name: "byte order mark", text: byteOrderMark + good, line: 1, about: "byte order mark"},
		{name: "empty file", text: "", line: 1, about: "header"},
		{
			name: "header with a renamed column", text: strings.Replace(good, "reason", "category", 1),
			line: 1, about: "header",
		},
		{name: "header with a seventh column", text: listHeader + "\textra\n", line: 1, about: "header"},
		{name: "header with a trailing space", text: listHeader + " \n", line: 1, about: "header"},
		{name: "no header", text: strings.TrimPrefix(good, listHeader+"\n"), line: 1, about: "header"},
		{name: "unsorted digests", text: listFixture(t, "unsorted.tsv"), line: 3, about: "sorted"},
		{
			name: "unsorted paths",
			text: listText(row("b.md", toolBash, reset), row("a.md", toolBash, reset)),
			line: 3, about: "sorted",
		},
		{
			name: "unsorted tools",
			text: listText(row("a.md", toolPowerShell, reset), row("a.md", toolBash, reset)),
			line: 3, about: "sorted",
		},
		{name: "duplicate", text: listFixture(t, "duplicate.tsv"), line: 4, about: "duplicate"},
		{name: "five fields", text: listFixture(t, "malformed.tsv"), line: 3, about: "fields"},
		{
			name: "seven fields", text: listText(append(row("a.md", toolBash, reset), "extra")),
			line: 2, about: "fields",
		},
		{
			name: "blank line",
			text: listHeader + "\n" + strings.Join(row("a.md", toolBash, reset), "\t") + "\n\n" +
				strings.Join(row("b.md", toolBash, reset), "\t") + "\n",
			line: 3, about: "fields",
		},
		{name: "unknown tool", text: listText(with(1, "Read")), line: 2, about: "tool"},
		{name: "tool in lower case", text: listText(with(1, "bash")), line: 2, about: "tool"},
		{name: "short digest", text: listText(with(2, reset[:11])), line: 2, about: "digest"},
		{name: "long digest", text: listText(with(2, reset+"0")), line: 2, about: "digest"},
		{name: "upper-case digest", text: listText(with(2, strings.ToUpper(reset))), line: 2, about: "digest"},
		{name: "digest that is not hexadecimal", text: listText(with(2, "g80c35de4baf")), line: 2, about: "digest"},
		{name: "empty digest", text: listText(with(2, "")), line: 2, about: "digest"},
		{name: "empty path", text: listText(with(0, "")), line: 2, about: "path"},
		{name: "empty reason", text: listText(with(3, "")), line: 2, about: "reason"},
		{name: "empty rule", text: listText(with(4, "")), line: 2, about: "rule"},
		{
			name: "error on a later line",
			text: listText(row("a.md", toolBash, reset), row("b.md", toolBash, reset), with(4, "")),
			line: 4, about: "rule",
		},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			got, err := readList("list.tsv", tc.text)
			if err == nil {
				t.Fatalf("got %+v and no error", got)
			}
			if want := fmt.Sprintf("list.tsv:%d: ", tc.line); !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error %q does not start with %q", err, want)
			}
			if !strings.Contains(err.Error(), tc.about) {
				t.Errorf("error %q does not mention %q", err, tc.about)
			}
		})
	}
}

func TestListRuleHeadings(t *testing.T) {
	text := "# Title\n\n## Overview\ntext\n### Threat categories \r\n#### Too deep\n##Unspaced\n ## Indented\n" +
		"## \ntext ## Inline\n### Limits\n" +
		"```markdown\n## Inside a fence\n```\n## After a fence\n" +
		"> ~~~\n> text\n> ## Inside a quoted fence\n\n> ~~~\n### After a quoted fence\r\n" +
		// The quote ends before its fence closes, so the block ends with it and the next line is read again.
		"> ```\n> ## Inside a quoted fence that is not closed\n## After a quote that ended\n" +
		"```\n## Inside a fence that never closes\n"
	got := headingSet(text)
	want := map[string]bool{
		"Overview": true, "Threat categories": true, "Limits": true, "After a fence": true, "After a quoted fence": true,
		"After a quote that ended": true,
	}
	if len(got) != len(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	for heading := range want {
		if !got[heading] {
			t.Errorf("heading %q is missing from %v", heading, got)
		}
	}
}

// writeTree writes files, a map of slash-separated relative path to text, under a new directory and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, text := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestWalkBlocks(t *testing.T) {
	t.Run("fixture tree", func(t *testing.T) {
		got, files, err := walkBlocks(filepath.Join("testdata", "list", "tree"))
		if err != nil {
			t.Fatal(err)
		}
		want := []block{
			{path: "demo/guide.md", line: 5, tool: toolBash, language: "bash", body: "git reset --hard HEAD~1\n"},
			{path: "demo/guide.md", line: 11, tool: toolBash, language: "bash", body: "git status\n"},
			{path: "demo/guide.md", line: 17, tool: toolBash, language: "bash", body: "git add <file>\n"},
			{path: "demo/guide.md", line: 23, tool: toolWrite, language: "json", body: "{ \"name\": \"fixture\" }\n"},
		}
		if files != 1 || !slices.Equal(got, want) {
			t.Errorf("%d files, blocks %+v\nwant 1 file, blocks %+v", files, got, want)
		}
	})

	t.Run("order and file selection", func(t *testing.T) {
		fence := func(command string) string { return "```bash\n" + command + "\n```\n" }
		root := writeTree(t, map[string]string{
			"b.md":           fence("git branch"),
			"a/z.md":         fence("git fetch") + "\n" + fence("git log"),
			"a.md":           fence("git status"),
			"a/deep/c.md":    "no block here\n",
			"a/notes.txt":    "```bash\nnever closed\n",
			"a/md":           fence("git diff"),
			"a/guide.md.bak": fence("git show"),
		})
		got, files, err := walkBlocks(root)
		if err != nil {
			t.Fatal(err)
		}
		// Byte order of the slash-separated paths: "." sorts before "/".
		want := []block{
			{path: "a.md", line: 1, tool: toolBash, language: "bash", body: "git status\n"},
			{path: "a/z.md", line: 1, tool: toolBash, language: "bash", body: "git fetch\n"},
			{path: "a/z.md", line: 5, tool: toolBash, language: "bash", body: "git log\n"},
			{path: "b.md", line: 1, tool: toolBash, language: "bash", body: "git branch\n"},
		}
		if files != 4 || !slices.Equal(got, want) {
			t.Errorf("%d files, blocks %+v\nwant 4 files, blocks %+v", files, got, want)
		}
	})

	t.Run("unclosed fence", func(t *testing.T) {
		got, _, err := walkBlocks(filepath.Join("testdata", "extract"))
		if err == nil {
			t.Fatalf("got %+v and no error", got)
		}
		if want := "unclosed.md:3:"; !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	})

	t.Run("missing root", func(t *testing.T) {
		if _, _, err := walkBlocks(filepath.Join(t.TempDir(), "absent")); err == nil {
			t.Error("no error for a root that does not exist")
		}
	})
}

func TestWalkFindsNoFile(t *testing.T) {
	root := writeTree(t, map[string]string{"a/notes.txt": "text\n"})
	blocks, files, err := walkBlocks(root)
	if err != nil || files != 0 || len(blocks) != 0 {
		t.Fatalf("walkBlocks: %d files, %d blocks, error %v; want none of each", files, len(blocks), err)
	}
	if _, err := walkNonEmpty(root); err == nil || !strings.Contains(err.Error(), root) {
		t.Errorf("walkNonEmpty: error %v, want one naming %s", err, root)
	}

	// A file without a block still counts: the condition is about files, not blocks.
	root = writeTree(t, map[string]string{"a/guide.md": "text\n"})
	if blocks, err := walkNonEmpty(root); err != nil || len(blocks) != 0 {
		t.Errorf("walkNonEmpty of a tree with one file and no block: %d blocks, error %v", len(blocks), err)
	}
	if _, err := walkNonEmpty(filepath.Join("testdata", "extract")); err == nil {
		t.Error("walkNonEmpty hides the error of an unclosed fence")
	}
}

func TestMatchReportsEachProblem(t *testing.T) {
	const (
		rule     = "Command-guardian threat categories"
		blocking = "[commands] 1 blocking threat(s) detected: destructive_git_ops - Use git stash instead."
	)
	denied := ranBlock{
		block:  block{path: "a/guide.md", line: 7, tool: toolBash, language: "bash", body: "git reset --hard HEAD~1\n"},
		result: runResult{verdict: verdictDenied, category: "destructive_git_ops", message: blocking, exitCode: 2},
	}
	allowed := ranBlock{
		block: block{path: "a/guide.md", line: 12, tool: toolBash, language: "bash", body: "git status\n"},
	}
	entryFor := func(b block) listEntry {
		return listEntry{
			line: 2, path: b.path, tool: b.tool, digest: bodyDigest(b.body), reason: "destructive_git_ops", rule: rule,
		}
	}
	deniedDigest, allowedDigest := bodyDigest(denied.block.body), bodyDigest(allowed.block.body)
	headings := map[string]bool{rule: true}
	at := func(r ranBlock, line int) ranBlock {
		r.block.line = line
		return r
	}

	for _, tc := range []struct {
		name string
		in   matchInput
		// want holds, for each expected problem in order, the texts it must contain.
		want [][]string
	}{
		{
			name: "a listed denial and an allowed block",
			in:   matchInput{blocks: []ranBlock{denied, allowed}, denials: []listEntry{entryFor(denied.block)}, headings: headings},
		},
		{
			name: "denied with no entry",
			in:   matchInput{blocks: []ranBlock{denied, allowed}, headings: headings},
			want: [][]string{{"a/guide.md:7: ", toolBash, deniedDigest, "no entry", blocking}},
		},
		{
			name: "entry for an allowed block",
			in: matchInput{
				blocks:   []ranBlock{denied, allowed},
				denials:  []listEntry{entryFor(allowed.block), entryFor(denied.block)},
				headings: headings,
			},
			want: [][]string{{"a/guide.md:12: ", toolBash, allowedDigest, "is allowed"}},
		},
		{
			name: "entry that matches no block",
			in: matchInput{
				blocks:   []ranBlock{allowed},
				denials:  []listEntry{entryFor(denied.block)},
				headings: headings,
			},
			want: [][]string{{"a/guide.md: ", toolBash, deniedDigest, "matches no block"}},
		},
		{
			name: "entry with another reason",
			in: matchInput{
				blocks: []ranBlock{denied, allowed},
				denials: []listEntry{func() listEntry {
					entry := entryFor(denied.block)
					entry.reason = categoryCommands
					return entry
				}()},
				headings: headings,
			},
			want: [][]string{{"a/guide.md:7: ", toolBash, deniedDigest, `"commands"`, `"destructive_git_ops"`, blocking}},
		},
		{
			name: "entry whose rule is no heading",
			in: matchInput{
				blocks:   []ranBlock{denied, allowed},
				denials:  []listEntry{entryFor(denied.block)},
				headings: map[string]bool{"Command-guardian overview": true},
			},
			want: [][]string{{"a/guide.md: ", toolBash, deniedDigest, `"` + rule + `"`, "heading"}},
		},
		{
			name: "fault without an entry",
			in: matchInput{
				blocks: []ranBlock{{
					block:  denied.block,
					result: runResult{verdict: verdictFault, message: "exit status 1: agx: unknown provider", exitCode: 1},
				}},
				headings: headings,
			},
			want: [][]string{{"a/guide.md:7: ", toolBash, deniedDigest, "no verdict", "exit status 1: agx: unknown provider"}},
		},
		{
			// The entry is neither confirmed nor refuted by a run that gave no verdict, so only the fault is reported.
			name: "fault with an entry",
			in: matchInput{
				blocks: []ranBlock{{
					block:  denied.block,
					result: runResult{verdict: verdictFault, message: "the hook did not exit within 2m0s", exitCode: -1},
				}},
				denials:  []listEntry{entryFor(denied.block)},
				headings: headings,
			},
			want: [][]string{{"a/guide.md:7: ", "no verdict", "the hook did not exit within 2m0s"}},
		},
		{
			name: "one entry covers two blocks",
			in: matchInput{
				blocks:   []ranBlock{denied, allowed, at(denied, 30)},
				denials:  []listEntry{entryFor(denied.block)},
				headings: headings,
			},
		},
		{
			name: "an entry covers only its tool",
			in: matchInput{
				blocks: []ranBlock{denied, func() ranBlock {
					r := at(denied, 30)
					r.block.tool, r.block.language = toolPowerShell, "powershell"
					return r
				}()},
				denials:  []listEntry{entryFor(denied.block)},
				headings: headings,
			},
			want: [][]string{{"a/guide.md:30: ", toolPowerShell, deniedDigest, "no entry"}},
		},
		{
			name: "an entry covers only its file",
			in: matchInput{
				blocks: []ranBlock{denied, func() ranBlock {
					r := denied
					r.block.path = "b/guide.md"
					return r
				}()},
				denials:  []listEntry{entryFor(denied.block)},
				headings: headings,
			},
			want: [][]string{{"b/guide.md:7: ", "no entry"}},
		},
		{
			name: "problems are sorted",
			in: matchInput{
				blocks: []ranBlock{at(denied, 9), at(denied, 3), allowed},
				denials: []listEntry{func() listEntry {
					entry := entryFor(allowed.block)
					entry.rule = "No such heading"
					return entry
				}()},
				headings: headings,
			},
			want: [][]string{
				{"a/guide.md: ", allowedDigest, "heading"},
				{"a/guide.md:12: ", "is allowed"},
				{"a/guide.md:3: ", "no entry"},
				{"a/guide.md:9: ", "no entry"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := matchDenials(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d problems, want %d:\n%s", len(got), len(tc.want), strings.Join(got, "\n"))
			}
			for i, texts := range tc.want {
				for _, text := range texts {
					if !strings.Contains(got[i], text) {
						t.Errorf("problem %d %q does not contain %q", i, got[i], text)
					}
				}
			}
		})
	}
}

func TestMatchFixtureTree(t *testing.T) {
	const (
		commandRule = "Command-guardian threat categories"
		secretRule  = "Secrets-guardian overview"
	)
	blocks, err := walkNonEmpty(filepath.Join("testdata", "list", "tree"))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 4 {
		t.Fatalf("the fixture tree holds %d blocks, want 4: %+v", len(blocks), blocks)
	}

	// The secrets check refuses a write that holds a key-shaped assignment with a literal value, and an installed
	// hook applies the same rule to this checkout, so no committed file can hold the line. It is put together here
	// and joins the fixture's blocks as file content.
	name := "to" + "ken"
	secret := block{
		path: "demo/guide.md", line: 27, tool: toolWrite, language: "ini",
		body: name + " = \"" + "canon" + "ical" + "\"\n",
	}
	blocks = append(blocks, secret)

	ran := runBlocks(t, blocks)
	for i, want := range []struct {
		verdict  verdict
		category string
	}{
		{verdictDenied, "destructive_git_ops"},
		{verdictAllowed, ""},
		{verdictAllowed, ""},
		{verdictAllowed, ""},
		{verdictDenied, categorySecrets},
	} {
		got := ran[i].result
		if got.verdict != want.verdict || got.category != want.category {
			t.Errorf("%s:%d: got %s %q, want %s %q: %s\n%s", ran[i].block.path, ran[i].block.line,
				got.verdict, got.category, want.verdict, want.category, got.message, got.output)
		}
	}
	if ran[2].block.body != "git add <file>\n" {
		t.Errorf("the block kept for matching is %q, want the body as written", ran[2].block.body)
	}

	listed, err := readList("good.tsv", listFixture(t, "good.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	denials := append(slices.Clone(listed), listEntry{
		path: secret.path, tool: toolWrite, digest: bodyDigest(secret.body), reason: categorySecrets, rule: secretRule,
	})
	headings := headingSet("## " + commandRule + "\n\n## " + secretRule + "\n")

	if got := matchDenials(matchInput{blocks: ran, denials: denials, headings: headings}); len(got) != 0 {
		t.Errorf("the fixture tree and its list disagree:\n%s", strings.Join(got, "\n"))
	}

	got := matchDenials(matchInput{blocks: ran, denials: denials[1:], headings: headings})
	if len(got) != 1 || !strings.Contains(got[0], "demo/guide.md:5: ") || !strings.Contains(got[0], "no entry") ||
		!strings.Contains(got[0], ran[0].result.message) {
		t.Errorf("without the entry of the denied command, got:\n%s\nwant one problem at line 5 with the hook's message",
			strings.Join(got, "\n"))
	}

	extra := append(slices.Clone(denials), listEntry{
		path: "demo/guide.md", tool: toolBash, digest: bodyDigest("git status\n"), reason: "destructive_git_ops",
		rule: commandRule,
	})
	got = matchDenials(matchInput{blocks: ran, denials: extra, headings: headings})
	if len(got) != 1 || !strings.Contains(got[0], "demo/guide.md:11: ") || !strings.Contains(got[0], "is allowed") {
		t.Errorf("with an entry for the allowed command, got:\n%s\nwant one problem at line 11", strings.Join(got, "\n"))
	}
}
