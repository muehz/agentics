package skillcompat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/settings"
)

// policyOffHeader is the first line of a policy-off list file: the five column names, separated by tabs.
const policyOffHeader = "path\ttool\tdigest\tkey\tnote"

// policyOffEntry is one line of a policy-off list file. It stands for every block of its file that has its tool and
// digest, and says the block is prescribed only where a repository switches the named policy setting off.
type policyOffEntry struct {
	// line is the 1-based line of the list file the entry was read from; it is 0 for an entry built in memory.
	line int
	// path is the Markdown file, relative to the walked root, with forward slashes.
	path string
	// tool is toolBash, toolPowerShell or toolWrite.
	tool string
	// digest is bodyDigest of the block body as written.
	digest string
	// key is the policy setting that, set to false, lets the block through; it is one of settings.Keys.
	key settings.Key
	// note is free text for a reader; nothing checks it.
	note string
}

// blockKey returns what the entry is matched to a block on.
func (e policyOffEntry) blockKey() blockKey {
	return blockKey{e.path, e.tool, e.digest}
}

// isPolicyKey reports whether name is a key of the hook's settings roster.
func isPolicyKey(name string) bool {
	return slices.Contains(settings.Keys, settings.Key(name))
}

// readPolicyOff returns the entries of the policy-off list file text, in file order, under the rules of readRows
// with policyOffHeader as the header. An entry's key must be one of settings.Keys, so that a list cannot name a
// setting the hook does not read; its note may be empty.
func readPolicyOff(name, text string) ([]policyOffEntry, error) {
	rows, err := readRows(name, text, policyOffHeader, func(row listRow) string {
		if !isPolicyKey(row.rest[0]) {
			return fmt.Sprintf("the key %q is not a hook settings key", row.rest[0])
		}
		return ""
	})
	if err != nil {
		return nil, err
	}
	var entries []policyOffEntry
	for _, row := range rows {
		entries = append(entries, policyOffEntry{
			line: row.line, path: row.key.path, tool: row.key.tool, digest: row.key.digest,
			key: settings.Key(row.rest[0]), note: row.rest[1],
		})
	}
	return entries, nil
}

// policyOffDir returns a new directory built as a repository whose settings file sets only key to false. The
// directory holds an empty .git directory, because the hook reads .agentics/hooks.json of the repository it finds
// itself in, and the settings file names nothing but the one key, so that every other policy default still applies
// to a run in it.
func policyOffDir(t testing.TB, key settings.Key) string {
	t.Helper()
	dir := t.TempDir()
	data, err := json.Marshal(map[string]any{
		"version": 1,
		"policy":  map[string]bool{strings.TrimPrefix(string(key), "policy."): false},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".git", ".agentics"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".agentics", "hooks.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// runPolicyOff runs b through the built hook binary twice and returns both results: on is the run under
// default policy, in an empty directory outside any repository; off is the run in a policyOffDir for key. Both
// runs send the block as sentBlock gives it, a command block with its placeholders substituted. It fails or skips
// the test as runBlock does and is safe to call from parallel tests.
func runPolicyOff(t testing.TB, b block, key settings.Key) (on, off runResult) {
	t.Helper()
	sent := sentBlock(b)
	on = runBlock(t, sent, t.TempDir(), outsideRepository)
	off = runBlock(t, sent, policyOffDir(t, key), insideRepository)
	return on, off
}

// policyOffProblems returns what the two runs of runPolicyOff refute about entry, which matches b: the block must
// be denied under default policy and allowed with the key off. A run that is a fault is reported as such and
// decides nothing. Each problem starts with the file path and the block's opening line and names the entry's list
// line and key.
func policyOffProblems(entry policyOffEntry, b block, on, off runResult) []string {
	var problems []string
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf("%s:%d: %s block %s, policy-off list line %d with the key %s: %s",
			b.path, b.line, b.tool, entry.digest, entry.line, entry.key, fmt.Sprintf(format, args...)))
	}
	switch on.verdict {
	case verdictFault:
		report("the hook gave no verdict under default policy: %s", on.message)
	case verdictAllowed:
		report("the block is allowed under default policy, so it does not depend on the key")
	}
	switch off.verdict {
	case verdictFault:
		report("the hook gave no verdict with the key set to false: %s", off.message)
	case verdictDenied:
		report("the block is still denied with the key set to false: %s", off.message)
	}
	return problems
}

// checkPolicyOff runs the block of every policy-off entry with runPolicyOff and returns the problems of
// policyOffProblems, sorted, or nothing when every entry holds. blocks are the blocks of the walked root the list
// belongs to. An entry is run once, as the first block it matches, because every block it covers has the same tool
// and body; an entry that matches no block is not run and is left for matchDenials to report. The runs are parallel
// sub-tests. The test is skipped on a platform outside the build contract and stops when a run cannot be made.
func checkPolicyOff(t *testing.T, blocks []block, entries []policyOffEntry) []string {
	t.Helper()
	runnerBinary(t)
	first := map[blockKey]block{}
	for _, b := range blocks {
		key := blockKey{b.path, b.tool, bodyDigest(b.body)}
		if _, seen := first[key]; !seen {
			first[key] = b
		}
	}
	found := make([][]string, len(entries))
	// The group returns only when every parallel sub-test in it has finished.
	finished := t.Run("policy-off", func(t *testing.T) {
		for i, entry := range entries {
			b, matches := first[entry.blockKey()]
			if !matches {
				continue
			}
			t.Run(fmt.Sprintf("%s:%d", b.path, b.line), func(t *testing.T) {
				t.Parallel()
				on, off := runPolicyOff(t, b, entry.key)
				found[i] = policyOffProblems(entry, b, on, off)
			})
		}
	})
	if !finished {
		t.FailNow()
	}
	problems := slices.Concat(found...)
	slices.Sort(problems)
	return problems
}

// checkTree is the whole check of one tree of Markdown against its two lists, and the one call a sweep makes, so
// that neither half can be left out: it walks root, runs every block under default policy, runs the block of every
// policy-off entry with its key on and off, and returns the problems of matchDenials and of checkPolicyOff
// together, sorted, or nothing when the tree and the lists agree. denials are the known-denial entries, policyOff
// the policy-off entries and headings the heading set of the hook README, from headingSet. The error is
// walkNonEmpty's: root cannot be read, holds no ".md" file, or holds a fence that never closes; nothing is run
// then. The test is skipped on a platform outside the build contract and stops when a run cannot be made.
func checkTree(
	t *testing.T, root string, denials []listEntry, policyOff []policyOffEntry, headings map[string]bool,
) ([]string, error) {
	t.Helper()
	blocks, err := walkNonEmpty(root)
	if err != nil {
		return nil, err
	}
	problems := slices.Concat(
		matchDenials(matchInput{blocks: runBlocks(t, blocks), denials: denials, headings: headings, policyOff: policyOff}),
		checkPolicyOff(t, blocks, policyOff),
	)
	slices.Sort(problems)
	return problems, nil
}

// policyKeyShape matches the whole text of a code span that is written as a policy key.
var policyKeyShape = regexp.MustCompile(`^policy\.[a-z_]+$`)

// codeSpans returns the text of every inline code span of line: what stands between a run of backticks and the
// next run of the same length, without the one space on each side that lets a span start or end with a backtick.
// A run that no later run of its length closes is ordinary text. Spans that continue on another line are not
// found.
func codeSpans(line string) []string {
	run := func(from int) int {
		end := from
		for end < len(line) && line[end] == '`' {
			end++
		}
		return end
	}
	var spans []string
	for i := 0; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		start := run(i)
		width := start - i
		i = start
		for j := start; j < len(line); {
			if line[j] != '`' {
				j++
				continue
			}
			end := run(j)
			if end-j == width {
				span := line[start:j]
				if len(span) > 2 && span[0] == ' ' && span[len(span)-1] == ' ' {
					span = span[1 : len(span)-1]
				}
				spans = append(spans, span)
				i = end
				break
			}
			j = end
		}
	}
	return spans
}

// policyKeySpans returns one problem, starting with path and the 1-based line, for every inline code span of the
// Markdown text whose whole text has the shape of a policy key (policy. followed by lower-case letters and
// underscores) and is not one of settings.Keys. A span that only contains such text, such as a file name, is not a
// key. Lines of fenced blocks, inside a blockquote or not, are skipped under fenceTracker's rule, which is the one
// extractBlocks reads blocks by, because a block may hold the settings of another tool; a fence that never closes
// skips the rest of the text. path is used only to label the problems. Line endings may be LF or CRLF.
func policyKeySpans(path, text string) []string {
	var problems []string
	var fences fenceTracker
	for i, line := range strings.Split(text, "\n") {
		kind, line := fences.next(line)
		if kind != lineOutside {
			continue
		}
		for _, span := range codeSpans(line) {
			if policyKeyShape.MatchString(span) && !isPolicyKey(span) {
				problems = append(problems, fmt.Sprintf("%s:%d: the code span %q is not a hook settings key", path, i+1, span))
			}
		}
	}
	return problems
}

// walkPolicyKeySpans returns the problems of policyKeySpans for every file under root whose name ends in ".md", in
// byte order of the files' paths relative to root, which are the paths the problems name. It is an error when no
// such file is found, so that a scan pointed at a wrong or emptied directory fails instead of passing with nothing
// checked.
func walkPolicyKeySpans(root string) ([]string, error) {
	paths, err := markdownFiles(root)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no .md file under %s", root)
	}
	var problems []string
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		problems = append(problems, policyKeySpans(path, string(data))...)
	}
	return problems, nil
}

// policyFixture returns the text of a file under testdata/policy.
func policyFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "policy", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// policyOffText returns a policy-off list file: the header and one line per row, each row being its five fields.
func policyOffText(rows ...[]string) string {
	var text strings.Builder
	text.WriteString(policyOffHeader + "\n")
	for _, row := range rows {
		text.WriteString(strings.Join(row, "\t") + "\n")
	}
	return text.String()
}

// requireProblem fails the test unless got is one problem that contains every text.
func requireProblem(t *testing.T, got []string, texts ...string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("got %d problems, want 1:\n%s", len(got), strings.Join(got, "\n"))
	}
	for _, text := range texts {
		if !strings.Contains(got[0], text) {
			t.Errorf("problem %q does not contain %q", got[0], text)
		}
	}
}

func TestPolicyOffReader(t *testing.T) {
	const (
		ready  = "0c9aac19f7a8"
		status = "ecb33f773d47"
		key    = string(settings.DraftPRs)
	)
	row := func(path, tool, digest string) []string {
		return []string{path, tool, digest, key, "note"}
	}
	with := func(index int, value string) []string {
		fields := row("demo/guide.md", toolBash, ready)
		fields[index] = value
		return fields
	}
	good := policyFixture(t, "policy-off.tsv")
	fixtureEntry := policyOffEntry{
		line: 2, path: "demo/guide.md", tool: toolBash, digest: ready, key: settings.DraftPRs,
		note: "the guide marks a pull request ready where the repository allows it",
	}

	accepted := []struct {
		name string
		text string
		want []policyOffEntry
	}{
		{name: "fixture", text: good, want: []policyOffEntry{fixtureEntry}},
		{name: "CRLF", text: strings.ReplaceAll(good, "\n", "\r\n"), want: []policyOffEntry{fixtureEntry}},
		{name: "no final line ending", text: strings.TrimSuffix(good, "\n"), want: []policyOffEntry{fixtureEntry}},
		{name: "header only", text: policyOffHeader + "\n"},
		{
			// The path decides before the digest, and the digest before the tool.
			name: "sort keys in order",
			text: policyOffText(
				row("a.md", toolWrite, status),
				row("b.md", toolWrite, ready),
				row("b.md", toolBash, status),
				row("b.md", toolPowerShell, status),
			),
			want: []policyOffEntry{
				{line: 2, path: "a.md", tool: toolWrite, digest: status, key: settings.DraftPRs, note: "note"},
				{line: 3, path: "b.md", tool: toolWrite, digest: ready, key: settings.DraftPRs, note: "note"},
				{line: 4, path: "b.md", tool: toolBash, digest: status, key: settings.DraftPRs, note: "note"},
				{line: 5, path: "b.md", tool: toolPowerShell, digest: status, key: settings.DraftPRs, note: "note"},
			},
		},
		{
			name: "empty note",
			text: policyOffText(with(4, "")),
			want: []policyOffEntry{{line: 2, path: "demo/guide.md", tool: toolBash, digest: ready, key: settings.DraftPRs}},
		},
	}
	for _, name := range settings.Keys {
		accepted = append(accepted, struct {
			name string
			text string
			want []policyOffEntry
		}{
			name: "key " + string(name),
			text: policyOffText(with(3, string(name))),
			want: []policyOffEntry{{line: 2, path: "demo/guide.md", tool: toolBash, digest: ready, key: name, note: "note"}},
		})
	}
	for _, tc := range accepted {
		t.Run("accepts "+tc.name, func(t *testing.T) {
			got, err := readPolicyOff("policy-off.tsv", tc.text)
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
		{name: "header of the known-denial list", text: listHeader + "\n", line: 1, about: "header"},
		{name: "header with a renamed column", text: strings.Replace(good, "key", "setting", 1), line: 1, about: "header"},
		{name: "no header", text: strings.TrimPrefix(good, policyOffHeader+"\n"), line: 1, about: "header"},
		{
			name: "unsorted paths",
			text: policyOffText(row("b.md", toolBash, ready), row("a.md", toolBash, ready)),
			line: 3, about: "sorted",
		},
		{
			name: "unsorted digests",
			text: policyOffText(row("a.md", toolBash, status), row("a.md", toolBash, ready)),
			line: 3, about: "sorted",
		},
		{
			name: "unsorted tools",
			text: policyOffText(row("a.md", toolPowerShell, ready), row("a.md", toolBash, ready)),
			line: 3, about: "sorted",
		},
		{
			name: "duplicate",
			text: policyOffText(row("a.md", toolBash, ready), row("b.md", toolBash, ready), row("b.md", toolBash, ready)),
			line: 4, about: "duplicate",
		},
		{
			// Two keys for one block is a duplicate: a block depends on one setting.
			name: "duplicate with another key",
			text: policyOffText(
				row("a.md", toolBash, ready),
				[]string{"a.md", toolBash, ready, string(settings.BlockPackagePublish), "note"},
			),
			line: 3, about: "duplicate",
		},
		{name: "four fields", text: policyOffText(row("a.md", toolBash, ready)[:4]), line: 2, about: "fields"},
		{
			name: "six fields", text: policyOffText(append(row("a.md", toolBash, ready), "extra")),
			line: 2, about: "fields",
		},
		{
			name: "line of the known-denial list",
			text: policyOffText([]string{"a.md", toolBash, ready, "unsafe_publication", "Rule", "note"}),
			line: 2, about: "fields",
		},
		{name: "unknown tool", text: policyOffText(with(1, "Read")), line: 2, about: "tool"},
		{name: "short digest", text: policyOffText(with(2, ready[:11])), line: 2, about: "digest"},
		{name: "upper-case digest", text: policyOffText(with(2, strings.ToUpper(ready))), line: 2, about: "digest"},
		{name: "empty path", text: policyOffText(with(0, "")), line: 2, about: "path"},
		{name: "unknown key", text: policyOffText(with(3, "policy.not_a_key")), line: 2, about: `"policy.not_a_key"`},
		{name: "key without its prefix", text: policyOffText(with(3, "draft_prs")), line: 2, about: `"draft_prs"`},
		{name: "key in upper case", text: policyOffText(with(3, "policy.DRAFT_PRS")), line: 2, about: "key"},
		{name: "empty key", text: policyOffText(with(3, "")), line: 2, about: "key"},
		{
			name: "unknown key on a later line",
			text: policyOffText(row("a.md", toolBash, ready), row("b.md", toolBash, ready), with(3, "policy.not_a_key")),
			line: 4, about: "key",
		},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			got, err := readPolicyOff("policy-off.tsv", tc.text)
			if err == nil {
				t.Fatalf("got %+v and no error", got)
			}
			if want := fmt.Sprintf("policy-off.tsv:%d: ", tc.line); !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error %q does not start with %q", err, want)
			}
			if !strings.Contains(err.Error(), tc.about) {
				t.Errorf("error %q does not mention %q", err, tc.about)
			}
		})
	}
}

func TestPolicyOffSettingsFile(t *testing.T) {
	dir := policyOffDir(t, settings.BlockPackagePublish)
	if info, err := os.Stat(filepath.Join(dir, ".git")); err != nil || !info.IsDir() {
		t.Errorf("%s holds no .git directory: %v", dir, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".agentics", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"policy":{"block_package_publish":false},"version":1}`; string(data) != want {
		t.Errorf("settings file %s, want %s", data, want)
	}
}

func TestPolicyOffFixtureEntry(t *testing.T) {
	entries, err := readPolicyOff("policy-off.tsv", policyFixture(t, "policy-off.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := walkNonEmpty(filepath.Join("testdata", "policy", "tree"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || len(blocks) != 2 {
		t.Fatalf("the fixture holds %d entries and %d blocks, want 1 and 2", len(entries), len(blocks))
	}
	entry, ready := entries[0], blocks[0]
	want := block{path: "demo/guide.md", line: 5, tool: toolBash, language: "bash", body: "gh pr ready 1\n"}
	if ready != want {
		t.Fatalf("the first fixture block is %+v, want %+v", ready, want)
	}
	if entry.blockKey() != (blockKey{ready.path, ready.tool, bodyDigest(ready.body)}) || entry.key != settings.DraftPRs {
		t.Fatalf("the fixture entry %+v does not name the block %+v with %s", entry, ready, settings.DraftPRs)
	}

	on := runBlock(t, ready, t.TempDir(), outsideRepository)
	requireDenied(t, on, "unsafe_publication")
	dir := policyOffDir(t, entry.key)
	if off := runBlock(t, ready, dir, insideRepository); off.verdict != verdictAllowed {
		t.Errorf("with %s off: got %s, want allowed: exit %d, %s\n%s", entry.key, off.verdict, off.exitCode, off.message, off.output)
	}

	// The settings file is what lets the block through, not the directory being a repository.
	if err := os.Remove(filepath.Join(dir, ".agentics", "hooks.json")); err != nil {
		t.Fatal(err)
	}
	requireDenied(t, runBlock(t, ready, dir, insideRepository), "unsafe_publication")

	if got := checkPolicyOff(t, blocks, entries); len(got) != 0 {
		t.Errorf("the fixture tree and its policy-off list disagree:\n%s", strings.Join(got, "\n"))
	}
}

func TestPolicyOffReportsAWrongEntry(t *testing.T) {
	entryFor := func(b block) policyOffEntry {
		return policyOffEntry{line: 4, path: b.path, tool: b.tool, digest: bodyDigest(b.body), key: settings.DraftPRs}
	}

	t.Run("allowed under default policy", func(t *testing.T) {
		status := block{path: "a/guide.md", line: 12, tool: toolBash, language: "bash", body: "git status\n"}
		got := checkPolicyOff(t, []block{status}, []policyOffEntry{entryFor(status)})
		requireProblem(t, got, "a/guide.md:12: ", toolBash, bodyDigest(status.body), "list line 4", "policy.draft_prs",
			"allowed under default policy")
	})

	t.Run("still denied with the key off", func(t *testing.T) {
		// The rule that denies this command is not one a policy setting governs.
		reset := block{path: "a/guide.md", line: 7, tool: toolBash, language: "bash", body: "git reset --hard HEAD~1\n"}
		got := checkPolicyOff(t, []block{reset}, []policyOffEntry{entryFor(reset)})
		requireProblem(t, got, "a/guide.md:7: ", toolBash, bodyDigest(reset.body), "list line 4", "policy.draft_prs",
			"still denied", "destructive_git_ops")
	})

	t.Run("an entry is run as the first block it matches", func(t *testing.T) {
		status := block{path: "a/guide.md", line: 12, tool: toolBash, language: "bash", body: "git status\n"}
		later, other := status, status
		later.line = 30
		other.path = "b/guide.md"
		got := checkPolicyOff(t, []block{other, status, later}, []policyOffEntry{entryFor(status)})
		requireProblem(t, got, "a/guide.md:12: ")
	})

	t.Run("an entry that matches no block is not run", func(t *testing.T) {
		status := block{path: "a/guide.md", line: 12, tool: toolBash, language: "bash", body: "git status\n"}
		if got := checkPolicyOff(t, nil, []policyOffEntry{entryFor(status)}); len(got) != 0 {
			t.Errorf("got problems for an entry without a block:\n%s", strings.Join(got, "\n"))
		}
	})

	t.Run("a command block is run with its placeholders substituted", func(t *testing.T) {
		// The hook denies the command as written and allows it with the placeholder substituted, so the entry is
		// wrong only because of the substitution.
		add := block{path: "a/guide.md", line: 17, tool: toolBash, language: "bash", body: "git add <file>\n"}
		if got := runBlock(t, add, t.TempDir(), outsideRepository); got.verdict != verdictDenied {
			t.Fatalf("the block as written: got %s, want denied, without which the check below proves nothing", got.verdict)
		}
		got := checkPolicyOff(t, []block{add}, []policyOffEntry{entryFor(add)})
		requireProblem(t, got, "a/guide.md:17: ", "allowed under default policy")
	})
}

func TestCheckTree(t *testing.T) {
	const rule = "Command-guardian threat categories"
	headings := map[string]bool{rule: true}
	listTree := filepath.Join("testdata", "list", "tree")
	policyTree := filepath.Join("testdata", "policy", "tree")
	denials, err := readList("good.tsv", listFixture(t, "good.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	policyOff, err := readPolicyOff("policy-off.tsv", policyFixture(t, "policy-off.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	check := func(t *testing.T, root string, denials []listEntry, policyOff []policyOffEntry) []string {
		t.Helper()
		got, err := checkTree(t, root, denials, policyOff, headings)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	entryFor := func(body string) policyOffEntry {
		return policyOffEntry{line: 4, path: "demo/guide.md", tool: toolBash, digest: bodyDigest(body), key: settings.DraftPRs}
	}

	t.Run("a tree and its known-denial list", func(t *testing.T) {
		if got := check(t, listTree, denials, nil); len(got) != 0 {
			t.Errorf("got problems:\n%s", strings.Join(got, "\n"))
		}
	})

	t.Run("a tree and its policy-off list", func(t *testing.T) {
		if got := check(t, policyTree, nil, policyOff); len(got) != 0 {
			t.Errorf("got problems:\n%s", strings.Join(got, "\n"))
		}
	})

	t.Run("a denied block that no list names", func(t *testing.T) {
		requireProblem(t, check(t, policyTree, nil, nil), "demo/guide.md:5: ", "no entry", "unsafe_publication")
	})

	t.Run("a policy-off entry whose key does not let the block through", func(t *testing.T) {
		// The entry accounts for the denial, so only the run with the key off can refute it.
		got := check(t, listTree, nil, []policyOffEntry{entryFor("git reset --hard HEAD~1\n")})
		requireProblem(t, got, "demo/guide.md:5: ", "list line 4", "still denied", "destructive_git_ops")
	})

	t.Run("the problems of both halves are sorted together", func(t *testing.T) {
		got := check(t, listTree, nil, []policyOffEntry{entryFor("git status\n")})
		if len(got) != 2 || !strings.HasPrefix(got[0], "demo/guide.md:11: ") || !strings.HasPrefix(got[1], "demo/guide.md:5: ") ||
			!strings.Contains(got[0], "allowed under default policy") || !strings.Contains(got[1], "no entry") {
			t.Errorf("got:\n%s\nwant the policy-off problem at line 11, then the unlisted denial at line 5", strings.Join(got, "\n"))
		}
	})

	t.Run("a root without Markdown", func(t *testing.T) {
		root := writeTree(t, map[string]string{"a/notes.txt": "text\n"})
		if got, err := checkTree(t, root, nil, nil, headings); err == nil || !strings.Contains(err.Error(), root) {
			t.Errorf("got %v and the error %v, want an error naming %s", got, err, root)
		}
	})

	t.Run("a fence that never closes", func(t *testing.T) {
		got, err := checkTree(t, filepath.Join("testdata", "extract"), nil, nil, headings)
		if err == nil || !strings.Contains(err.Error(), "unclosed.md:3:") {
			t.Errorf("got %v and the error %v, want an error naming unclosed.md:3:", got, err)
		}
	})
}

func TestPolicyOffProblems(t *testing.T) {
	b := block{path: "a/guide.md", line: 7, tool: toolBash, language: "bash", body: "gh pr ready 1\n"}
	entry := policyOffEntry{line: 3, path: b.path, tool: b.tool, digest: bodyDigest(b.body), key: settings.DraftPRs}
	denied := runResult{verdict: verdictDenied, category: "unsafe_publication", message: "[commands] denied", exitCode: 2}
	fault := runResult{verdict: verdictFault, message: "exit status 1: agx: unknown provider", exitCode: 1}
	allowed := runResult{}

	for _, tc := range []struct {
		name    string
		on, off runResult
		// want holds, for each expected problem in order, the texts it must contain.
		want [][]string
	}{
		{name: "denied, then allowed", on: denied, off: allowed},
		{name: "allowed twice", on: allowed, off: allowed, want: [][]string{{"allowed under default policy"}}},
		{name: "denied twice", on: denied, off: denied, want: [][]string{{"still denied", "[commands] denied"}}},
		{
			name: "allowed, then denied", on: allowed, off: denied,
			want: [][]string{{"allowed under default policy"}, {"still denied"}},
		},
		{
			name: "fault under default policy", on: fault, off: allowed,
			want: [][]string{{"no verdict under default policy", "unknown provider"}},
		},
		{
			name: "fault with the key off", on: denied, off: fault,
			want: [][]string{{"no verdict with the key set to false", "unknown provider"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := policyOffProblems(entry, b, tc.on, tc.off)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d problems, want %d:\n%s", len(got), len(tc.want), strings.Join(got, "\n"))
			}
			for i, texts := range tc.want {
				for _, text := range append(texts, "a/guide.md:7: ", "list line 3", "policy.draft_prs") {
					if !strings.Contains(got[i], text) {
						t.Errorf("problem %d %q does not contain %q", i, got[i], text)
					}
				}
			}
		})
	}
}

func TestPolicyOffAndKnownDenialLists(t *testing.T) {
	const (
		rule     = "Command-guardian threat categories"
		blocking = "[commands] 1 blocking threat(s) detected: unsafe_publication - Do not mark the pull request ready."
	)
	denied := ranBlock{
		block:  block{path: "a/guide.md", line: 7, tool: toolBash, language: "bash", body: "gh pr ready 1\n"},
		result: runResult{verdict: verdictDenied, category: "unsafe_publication", message: blocking, exitCode: 2},
	}
	allowed := ranBlock{
		block: block{path: "a/guide.md", line: 12, tool: toolBash, language: "bash", body: "git status\n"},
	}
	digest := bodyDigest(denied.block.body)
	policyOff := policyOffEntry{line: 3, path: "a/guide.md", tool: toolBash, digest: digest, key: settings.DraftPRs}
	denial := listEntry{line: 2, path: "a/guide.md", tool: toolBash, digest: digest, reason: "unsafe_publication", rule: rule}
	headings := map[string]bool{rule: true}
	blocks := []ranBlock{denied, allowed}

	t.Run("a policy-off entry accounts for a denied block", func(t *testing.T) {
		in := matchInput{blocks: blocks, headings: headings, policyOff: []policyOffEntry{policyOff}}
		if got := matchDenials(in); len(got) != 0 {
			t.Errorf("got problems:\n%s", strings.Join(got, "\n"))
		}
		// Without the entry the same block is a problem, so the entry is what accounts for it.
		in.policyOff = nil
		requireProblem(t, matchDenials(in), "a/guide.md:7: ", "no entry")
	})

	t.Run("a policy-off entry covers only its tool, file and body", func(t *testing.T) {
		for _, change := range []func(*policyOffEntry){
			func(e *policyOffEntry) { e.tool = toolPowerShell },
			func(e *policyOffEntry) { e.path = "b/guide.md" },
			func(e *policyOffEntry) { e.digest = bodyDigest("gh pr ready 2\n") },
		} {
			entry := policyOff
			change(&entry)
			got := matchDenials(matchInput{blocks: blocks, headings: headings, policyOff: []policyOffEntry{entry}})
			if len(got) != 2 || !strings.Contains(strings.Join(got, "\n"), "a/guide.md:7: Bash block "+digest+" is denied and no entry") {
				t.Errorf("with the entry %+v, got:\n%s\nwant the denied block and the unmatched entry", entry, strings.Join(got, "\n"))
			}
		}
	})

	t.Run("an entry on both lists", func(t *testing.T) {
		got := matchDenials(matchInput{
			blocks: blocks, denials: []listEntry{denial}, headings: headings, policyOff: []policyOffEntry{policyOff},
		})
		requireProblem(t, got, "a/guide.md: ", "list line 2", toolBash, digest, "also on the policy-off list", "line 3",
			"policy.draft_prs")
	})

	t.Run("a policy-off entry that matches no block", func(t *testing.T) {
		got := matchDenials(matchInput{blocks: []ranBlock{allowed}, headings: headings, policyOff: []policyOffEntry{policyOff}})
		requireProblem(t, got, "a/guide.md: ", "policy-off list line 3", toolBash, digest, "policy.draft_prs", "matches no block")
	})

	t.Run("one policy-off entry covers two blocks", func(t *testing.T) {
		again := denied
		again.block.line = 30
		in := matchInput{blocks: []ranBlock{denied, allowed, again}, headings: headings, policyOff: []policyOffEntry{policyOff}}
		if got := matchDenials(in); len(got) != 0 {
			t.Errorf("got problems:\n%s", strings.Join(got, "\n"))
		}
	})
}

func TestPolicyKeySpans(t *testing.T) {
	t.Run("fixture tree", func(t *testing.T) {
		got, err := walkPolicyKeySpans(filepath.Join("testdata", "policy", "tree"))
		if err != nil {
			t.Fatal(err)
		}
		requireProblem(t, got, "demo/guide.md:9: ", `"policy.not_a_key"`)
	})

	t.Run("a root without Markdown", func(t *testing.T) {
		root := writeTree(t, map[string]string{"a/notes.txt": "`policy.not_a_key`\n"})
		if got, err := walkPolicyKeySpans(root); err == nil || !strings.Contains(err.Error(), root) {
			t.Errorf("got %v and the error %v, want an error naming %s", got, err, root)
		}
		if _, err := walkPolicyKeySpans(filepath.Join(root, "absent")); err == nil {
			t.Error("no error for a root that does not exist")
		}
	})

	t.Run("files in byte order", func(t *testing.T) {
		root := writeTree(t, map[string]string{
			"b.md":   "`policy.b`\n",
			"a/z.md": "text\n\n`policy.z`\n",
			"a.md":   "`policy.a`\n",
		})
		got, err := walkPolicyKeySpans(root)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"a.md:1: ", "a/z.md:3: ", "b.md:1: "}
		if len(got) != len(want) {
			t.Fatalf("got %d problems, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
		}
		for i, prefix := range want {
			if !strings.HasPrefix(got[i], prefix) {
				t.Errorf("problem %d %q does not start with %q", i, got[i], prefix)
			}
		}
	})

	var known strings.Builder
	for _, key := range settings.Keys {
		known.WriteString("`" + string(key) + "` ")
	}
	for _, tc := range []struct {
		name string
		text string
		// want holds, for each expected problem in order, its line and the span it names.
		want []struct {
			line int
			span string
		}
	}{
		{name: "every settings key", text: known.String() + "\n"},
		{name: "unknown key", text: "text\nSet `policy.not_a_key` to false.\n", want: []struct {
			line int
			span string
		}{{2, "policy.not_a_key"}}},
		{name: "a file name that contains a key shape", text: "See `references/checks/checks-tier-policy.md`.\n"},
		{name: "a key inside a longer span", text: "`policy.not_a_key = false` and `the policy.not_a_key`\n"},
		{name: "a key followed by a member", text: "`policy.not_a_key.leaf` and `policy.not_a_key: false`\n"},
		{name: "upper case and digits are not the key shape", text: "`policy.Not_A_Key` `policy.key2` `policy.`\n"},
		{name: "outside a code span", text: "policy.not_a_key and \"policy.not_a_key\"\n"},
		{name: "unclosed backtick", text: "`policy.not_a_key\n"},
		{name: "two on one line", text: "`policy.one`, `policy.draft_prs`, `policy.two`\n", want: []struct {
			line int
			span string
		}{{1, "policy.one"}, {1, "policy.two"}}},
		{name: "double backticks", text: "``policy.not_a_key`` and `` policy.spaced ``\n", want: []struct {
			line int
			span string
		}{{1, "policy.not_a_key"}, {1, "policy.spaced"}}},
		{name: "a span after an unclosed run", text: "`` `policy.not_a_key`\n", want: []struct {
			line int
			span string
		}{{1, "policy.not_a_key"}}},
		{name: "CRLF", text: "text\r\n`policy.not_a_key`\r\n", want: []struct {
			line int
			span string
		}{{2, "policy.not_a_key"}}},
		{name: "inside a fenced block", text: "```python\n# `policy.check_tool`\n```\n"},
		{name: "inside a tilde fence", text: "~~~\n`policy.check_tool`\n~~~\n"},
		{name: "inside an indented fence", text: "- item\n\n  ```yaml\n  key: `policy.check_tool`\n  ```\n"},
		{
			name: "a shorter fence does not close a longer one",
			text: "````markdown\n```\n`policy.check_tool`\n```\n````\n",
		},
		{name: "after a fenced block", text: "```\ntext\n```\n`policy.not_a_key`\n", want: []struct {
			line int
			span string
		}{{4, "policy.not_a_key"}}},
		{name: "inline code of three backticks is not a fence", text: "```policy.not_a_key```\n`policy.other`\n", want: []struct {
			line int
			span string
		}{{1, "policy.not_a_key"}, {2, "policy.other"}}},
		{name: "a fence that never closes", text: "```\n`policy.check_tool`\n"},
		{name: "inside a quoted fence", text: "> ```yaml\n> key: `policy.check_tool`\n>\n> `policy.other_tool`\n> ```\n"},
		{name: "in a quote after a quoted fence", text: "> ```\n> text\n> ```\n> `policy.not_a_key`\n", want: []struct {
			line int
			span string
		}{{4, "policy.not_a_key"}}},
		{
			name: "a fence of the other character does not close a fence",
			text: "```\n~~~\n`policy.check_tool`\n```\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := policyKeySpans("a/guide.md", tc.text)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d problems, want %d:\n%s", len(got), len(tc.want), strings.Join(got, "\n"))
			}
			for i, want := range tc.want {
				prefix := fmt.Sprintf("a/guide.md:%d: ", want.line)
				if !strings.HasPrefix(got[i], prefix) || !strings.Contains(got[i], `"`+want.span+`"`) {
					t.Errorf("problem %d %q, want one at %s naming %q", i, got[i], prefix, want.span)
				}
			}
		})
	}
}
