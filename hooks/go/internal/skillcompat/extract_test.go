// Package skillcompat tests that the commands and file contents the skills show in fenced Markdown blocks pass the
// hooks this module builds. It holds only test files: the digest that vouches for the committed hook binaries covers
// every non-test Go file in the module, so a non-test file here would change that digest without changing a binary.
package skillcompat

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The tools a block is run as. A command block is the command of a shell tool; any other block is the content of a
// file write.
const (
	toolBash       = "Bash"
	toolPowerShell = "PowerShell"
	toolWrite      = "Write"
)

// block is one fenced code block of a Markdown file.
type block struct {
	// path is the file the block came from, as the caller named it.
	path string
	// line is the 1-based number of the opening fence line.
	line int
	// tool is toolBash, toolPowerShell or toolWrite, chosen from language.
	tool string
	// language is the first word of the info string, lower-cased; it is empty when the fence has no info string.
	language string
	// body is the block's lines with the opening fence's blockquote markers and indentation removed, each followed
	// by one LF.
	body string
}

// fenceOpen matches a line that can open a block: any blockquote markers, each with the indentation before it and
// at most one space after it; indentation; three or more of one fence character; and the info string.
var fenceOpen = regexp.MustCompile("^((?:[ \t]*> ?)*)([ \t]*)(`{3,}|~{3,})(.*)$")

// lineKind is what a line of a Markdown text is with respect to its fenced blocks.
type lineKind int

const (
	// lineOutside: the line belongs to no fenced block.
	lineOutside lineKind = iota
	// lineOpens: the line is the opening fence of a block.
	lineOpens
	// lineBody: the line is inside a block.
	lineBody
	// lineCloses: the line is the closing fence of a block.
	lineCloses
)

// fenceTracker follows the fenced blocks of a Markdown text, one line at a time. Every reader of Markdown in this
// package asks it which lines are fenced, so that they cannot disagree about where a block starts and ends. The
// zero value starts outside any block.
type fenceTracker struct {
	// open reports whether the last line given to next left a block open.
	open bool
	// quote is the number of blockquote markers before the opening fence.
	quote int
	// indent is the width of the indentation between the last marker, or the line start, and the opening fence.
	indent int
	// fence is the opening fence: three or more of one fence character.
	fence string
	// info is the text after the opening fence.
	info string
	// quoteEnded reports whether the last line given to next ended a block behind markers that had no closing
	// fence yet, because the line left the quote.
	quoteEnded bool
}

// next classifies line, the next line of the text without its LF, and returns its text: for lineBody the line as
// the block holds it, for lineOutside the line without a trailing CR, and nothing for a fence line.
//
// A block opens at a line of three or more backticks or tildes, which may be indented and may stand behind one or
// more blockquote markers (">", each optionally followed by one space). It closes at the first later line that,
// after the markers, holds only a fence of the same character and at least the same length, so a longer fence can
// hold a shorter one and a tilde fence can hold a backtick one. From each line of a block behind markers, up to as
// many markers as the opening line has are removed, each with the indentation before it and one space after it; a
// further marker is the block's text. The opening fence's indentation is then removed from the line as far as the
// line has it.
//
// A line of a block behind markers belongs to the block when it carries all of the opening line's markers, or when
// nothing but spaces and tabs is left after the markers it does carry: such a blank line is an empty line of the
// body whether it has every marker, some or none, because authors leave the markers off an empty line. Any other
// line has left the quote, and a quote cannot hold a block past its own end. The block ends there without a closing
// fence, quoteEnded is set, and the line is classified again as a line outside any block, so it is lineOutside or,
// when it is a fence itself, lineOpens. Without this a quoted fence that was never closed would take in the
// unquoted text after it up to the next fence line, and hide a real block there. extractBlocks reports the ended
// block as an error; a reader that only skips fenced lines has no error to give and reads on from that line as
// unfenced text.
func (f *fenceTracker) next(line string) (lineKind, string) {
	line = strings.TrimSuffix(line, "\r")
	quoteEnded := false
	if f.open {
		rest, found := trimQuote(line, f.quote)
		if found == f.quote || strings.Trim(rest, " \t") == "" {
			f.quoteEnded = false
			if closesFence(rest, f.fence) {
				f.open = false
				return lineCloses, ""
			}
			return lineBody, trimIndent(rest, f.indent)
		}
		quoteEnded = true
	}
	*f = fenceTracker{quoteEnded: quoteEnded}
	m := fenceOpen.FindStringSubmatch(line)
	// A backtick in the info string means the line is inline code such as ```x```, not a fence.
	if m == nil || (m[3][0] == '`' && strings.Contains(m[4], "`")) {
		return lineOutside, line
	}
	f.open, f.quote, f.indent, f.fence, f.info = true, strings.Count(m[1], ">"), len(m[2]), m[3], m[4]
	return lineOpens, ""
}

// trimQuote removes up to markers blockquote markers from the start of line, each with the indentation before it
// and one space after it, and returns the rest with the number of markers it found. It stops at the first place
// that holds no marker, and leaves the indentation there.
func trimQuote(line string, markers int) (rest string, found int) {
	for found < markers {
		after, ok := strings.CutPrefix(strings.TrimLeft(line, " \t"), ">")
		if !ok {
			break
		}
		line = strings.TrimPrefix(after, " ")
		found++
	}
	return line, found
}

// placeholder matches a token an author writes for a value the reader supplies, such as <file>. The token starts
// with a letter or a dot and does not end in white space, which keeps shell redirections such as "< <(cmd)", "<<EOF"
// and "<in >out" out of the match.
var placeholder = regexp.MustCompile(`<[A-Za-z.](?:[^<>\r\n]*[^<>\s])?>`)

// toolForLanguage returns the tool a block of the given lower-cased language is run as. Languages that name no
// shell, and the empty language, are file content.
func toolForLanguage(language string) string {
	switch language {
	case "bash", "sh", "shell":
		return toolBash
	case "powershell", "pwsh":
		return toolPowerShell
	default:
		return toolWrite
	}
}

// extractBlocks returns the fenced code blocks of the Markdown text, in order. path is used only to label the
// blocks and the error. Where a block opens and closes, and what its lines hold, is fenceTracker's rule; a block
// inside a blockquote is returned with the body a reader sees, without the quote markers. Mermaid blocks are
// diagrams, not commands or file content, and are not returned. A fence that never closes is an error naming path
// and the opening line, and so is a fence behind blockquote markers that is not closed before its quote ends, which
// also names the line the quote ended at. Line endings may be LF or CRLF.
func extractBlocks(path, text string) ([]block, error) {
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var (
		blocks []block
		fences fenceTracker
		cur    block
		body   strings.Builder
	)
	for i, line := range lines {
		kind, content := fences.next(line)
		if fences.quoteEnded {
			return nil, fmt.Errorf("%s:%d: the quoted fence was not closed before its quote ended at line %d",
				path, cur.line, i+1)
		}
		switch kind {
		case lineOpens:
			cur = block{path: path, line: i + 1}
			if words := strings.Fields(fences.info); len(words) > 0 {
				cur.language = strings.ToLower(words[0])
			}
			cur.tool = toolForLanguage(cur.language)
			body.Reset()
		case lineBody:
			body.WriteString(content)
			body.WriteByte('\n')
		case lineCloses:
			if cur.language != "mermaid" {
				cur.body = body.String()
				blocks = append(blocks, cur)
			}
		}
	}
	if fences.open {
		return nil, fmt.Errorf("%s:%d: fenced block is never closed", path, cur.line)
	}
	return blocks, nil
}

// sentBlock returns b as it is sent to the hook. A command block has its placeholders substituted, because an
// author's <file> would otherwise read as a redirection; file content is sent as written, because a token in angle
// brackets there is the content itself, such as an HTML tag. Every run of a block goes through it, so the runs
// under default policy and with a policy key off send the same text.
func sentBlock(b block) block {
	if b.tool != toolWrite {
		b.body = substitutePlaceholders(b.body)
	}
	return b
}

// closesFence reports whether line, already without any blockquote markers, holds only a fence of the same
// character as fence and at least its length.
func closesFence(line, fence string) bool {
	trimmed := strings.Trim(line, " \t")
	return len(trimmed) >= len(fence) && strings.Trim(trimmed, fence[:1]) == ""
}

// trimIndent removes up to width leading spaces and tabs from line. A line indented less than the opening fence
// keeps its text.
func trimIndent(line string, width int) string {
	n := 0
	for n < width && n < len(line) && (line[n] == ' ' || line[n] == '\t') {
		n++
	}
	return line[n:]
}

// substitutePlaceholders replaces every placeholder token in body, such as <file> or <.gitignore, when changed>,
// with the word "placeholder", so that a command an author wrote for a reader to fill in parses as a command. Shell
// redirections and braced tokens such as {owner} are left unchanged. It is for command blocks only; sentBlock
// decides which blocks those are.
func substitutePlaceholders(body string) string {
	return placeholder.ReplaceAllString(body, "placeholder")
}

// bodyDigest returns the first 12 lower-case hexadecimal characters of the SHA-256 of body's lines, each followed by
// one LF, with CRLF read as LF. It identifies a block by its text as written, so callers take it before
// substitutePlaceholders.
func bodyDigest(body string) string {
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	if normalized != "" && !strings.HasSuffix(normalized, "\n") {
		normalized += "\n"
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])[:12]
}

// readFixture returns the fixture's slash-separated path and its text.
func readFixture(t *testing.T, name string) (string, string) {
	t.Helper()
	path := "testdata/extract/" + name
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	return path, string(data)
}

func TestExtractBlocks(t *testing.T) {
	path, text := readFixture(t, "blocks.md")
	want := []block{
		{line: 3, tool: toolBash, language: "bash", body: "git status\n"},
		{line: 7, tool: toolBash, language: "sh", body: "git log --oneline\n"},
		{line: 11, tool: toolBash, language: "shell", body: "git diff --stat\n"},
		{line: 15, tool: toolPowerShell, language: "powershell", body: "Get-ChildItem\n"},
		{line: 19, tool: toolPowerShell, language: "pwsh", body: "Get-Location\n"},
		{line: 23, tool: toolBash, language: "bash", body: "git branch\n"},
		{line: 27, tool: toolPowerShell, language: "powershell", body: "Get-Date\n"},
		{line: 36, tool: toolWrite, language: "json", body: "{ \"name\": \"fixture\" }\n"},
		{line: 40, tool: toolWrite, language: "", body: "plain text\n"},
		{line: 44, tool: toolBash, language: "bash", body: "git fetch\n"},
		{line: 48, tool: toolWrite, language: "markdown", body: "```bash\ngit status\n```\n"},
		{line: 56, tool: toolBash, language: "bash", body: "git add --all\n  git commit\n"},
		{line: 61, tool: toolBash, language: "bash", body: "git stash list\n\ngit remote\n"},
		// A fence of the other character is a line of the body, not the closing fence.
		{line: 67, tool: toolWrite, language: "text", body: "~~~\n"},
		{line: 71, tool: toolWrite, language: "text", body: "```\n"},
		// A closing fence may be longer than the opening one.
		{line: 75, tool: toolBash, language: "bash", body: "git tag\n"},
		{line: 79, tool: toolBash, language: "bash", body: "git show\n"},
		// A quote line without text is an empty line of the body.
		{line: 85, tool: toolWrite, language: "text", body: "  [ ] First\n\n  [ ] Second\n"},
		// Behind two markers: both in each spelling, then empty lines that carry one marker and none.
		{line: 93, tool: toolBash, language: "bash", body: "git describe\ngit rev-parse HEAD\n\n\ngit shortlog\n"},
	}

	for _, eol := range []struct{ name, text string }{
		{"LF", text},
		{"CRLF", strings.ReplaceAll(text, "\n", "\r\n")},
	} {
		t.Run(eol.name, func(t *testing.T) {
			got, err := extractBlocks(path, eol.text)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(want) {
				t.Fatalf("got %d blocks, want %d: %+v", len(got), len(want), got)
			}
			for i, w := range want {
				w.path = path
				if got[i] != w {
					t.Errorf("block %d:\n got %+v\nwant %+v", i, got[i], w)
				}
			}
		})
	}
}

func TestExtractInlineCodeIsNotAFence(t *testing.T) {
	got, err := extractBlocks("inline.md", "```git status```\n\ntext\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want no blocks", got)
	}
}

func TestExtractBlockquotedBlocks(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want []block
	}{
		{
			name: "marker without a space",
			text: ">```bash\n>git status\n>```\n",
			want: []block{{line: 1, tool: toolBash, language: "bash", body: "git status\n"}},
		},
		{
			name: "indented marker and indented fence",
			text: "- item\n\n  >   ```bash\n  >   git status\n  >     git log\n  >   ```\n",
			want: []block{{line: 3, tool: toolBash, language: "bash", body: "git status\n  git log\n"}},
		},
		{
			// Only as many markers as the opening line has are quote markers; a further one is the block's text.
			name: "a body line that starts with a marker of its own",
			text: "> ```text\n> > quoted\n> ```\n",
			want: []block{{line: 1, tool: toolWrite, language: "text", body: "> quoted\n"}},
		},
		{
			name: "a marker in a block that is not quoted is the block's text",
			text: "```text\n> quoted\n```\n",
			want: []block{{line: 1, tool: toolWrite, language: "text", body: "> quoted\n"}},
		},
		{
			name: "empty lines without the markers",
			text: "> > ```bash\n> > git status\n>\n\n \t\n>> git log\n> > ```\n",
			want: []block{{line: 1, tool: toolBash, language: "bash", body: "git status\n\n\n \t\ngit log\n"}},
		},
		{
			name: "a quote that ends after its block is closed",
			text: "> ```bash\n> git status\n> ```\ntext\n\n```bash\ngit log\n```\n",
			want: []block{
				{line: 1, tool: toolBash, language: "bash", body: "git status\n"},
				{line: 6, tool: toolBash, language: "bash", body: "git log\n"},
			},
		},
		{
			name: "a quoted fence of another character does not close the block",
			text: "> ```text\n> ~~~\n> ```\n",
			want: []block{{line: 1, tool: toolWrite, language: "text", body: "~~~\n"}},
		},
		{
			name: "a quoted line of inline code is not a fence",
			text: "> ```git status```\n",
		},
		{
			name: "CRLF",
			text: "> ```bash\r\n> git status\r\n>\r\n> ```\r\n",
			want: []block{{line: 1, tool: toolBash, language: "bash", body: "git status\n\n"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractBlocks("", tc.text)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got  %+v\nwant %+v", got, tc.want)
			}
		})
	}

	t.Run("a quoted fence that is open at the end of the text", func(t *testing.T) {
		got, err := extractBlocks("quoted.md", "text\n\n> ```bash\n> git status\n>\n")
		if err == nil || !strings.Contains(err.Error(), "quoted.md:3:") || !strings.Contains(err.Error(), "never closed") {
			t.Errorf("got %+v and the error %v, want an error naming quoted.md:3: and a fence that is never closed", got, err)
		}
	})
}

func TestExtractQuotedFenceEndsWithItsQuote(t *testing.T) {
	const reason = "the quoted fence was not closed before its quote ended"
	for _, tc := range []struct {
		name string
		text string
		// opens and ends are the lines the error must name: the opening fence, and the line that left the quote.
		opens, ends int
	}{
		{
			// Without the error the text block would run to the last line and hide the command block.
			name:  "unquoted text and a real block after the quote",
			text:  "> ```text\n>   [ ] One\n\nThen run:\n\n```bash\ngit status\n```\n",
			opens: 1, ends: 4,
		},
		{name: "text directly after the quote", text: "text\n\n> ```bash\n> git status\ntext\n", opens: 3, ends: 5},
		{name: "a fence without the marker", text: "> ```bash\n> git status\n```\ntext\n", opens: 1, ends: 3},
		{name: "one marker of two", text: "> > ```bash\n> > git status\n> git log\n> > ```\n", opens: 1, ends: 3},
		{name: "a closing fence behind one marker of two", text: "> > ```bash\n> > git status\n> ```\n", opens: 1, ends: 3},
		{name: "CRLF", text: "> ```text\r\n> One\r\n\r\nThen run:\r\n", opens: 1, ends: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractBlocks("quoted.md", tc.text)
			if err == nil {
				t.Fatalf("got %+v and no error", got)
			}
			if got != nil {
				t.Errorf("got the blocks %+v beside the error", got)
			}
			for _, want := range []string{fmt.Sprintf("quoted.md:%d: ", tc.opens), reason, fmt.Sprintf("line %d", tc.ends)} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestFenceTrackerClassifiesLines(t *testing.T) {
	type step struct {
		line string
		kind lineKind
		text string
		// quoteEnded is the tracker's flag after the line.
		quoteEnded bool
	}
	for _, tc := range []struct {
		name  string
		steps []step
	}{
		{
			name: "a quoted block that is closed",
			steps: []step{
				{line: "> ```text", kind: lineOpens},
				{line: "> one", kind: lineBody, text: "one"},
				{line: "", kind: lineBody},
				{line: "> ```", kind: lineCloses},
				{line: "text", kind: lineOutside, text: "text"},
			},
		},
		{
			// The readers that only skip fenced lines see everything from the line that left the quote as unfenced.
			name: "the lines after a quote that ended are outside a fence",
			steps: []step{
				{line: "> ```text", kind: lineOpens},
				{line: ">   [ ] One", kind: lineBody, text: "  [ ] One"},
				{line: "", kind: lineBody},
				{line: "Then run:", kind: lineOutside, text: "Then run:", quoteEnded: true},
				{line: "## Heading", kind: lineOutside, text: "## Heading"},
				{line: "```bash", kind: lineOpens},
				{line: "git status", kind: lineBody, text: "git status"},
				{line: "```", kind: lineCloses},
			},
		},
		{
			name: "a fence line that leaves the quote opens a block of its own",
			steps: []step{
				{line: "> ```text", kind: lineOpens},
				{line: "```bash", kind: lineOpens, quoteEnded: true},
				{line: "> git status", kind: lineBody, text: "> git status"},
				{line: "```", kind: lineCloses},
			},
		},
		{
			name: "one marker of two leaves the inner quote",
			steps: []step{
				{line: "> > ```text", kind: lineOpens},
				{line: ">", kind: lineBody},
				{line: "> one\r", kind: lineOutside, text: "> one", quoteEnded: true},
				{line: "> > two", kind: lineOutside, text: "> > two"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var fences fenceTracker
			for i, s := range tc.steps {
				kind, text := fences.next(s.line)
				if kind != s.kind || text != s.text || fences.quoteEnded != s.quoteEnded {
					t.Errorf("line %d %q: got kind %d, text %q, quoteEnded %v; want kind %d, text %q, quoteEnded %v",
						i+1, s.line, kind, text, fences.quoteEnded, s.kind, s.text, s.quoteEnded)
				}
			}
		})
	}
}

func TestSentBlockSubstitutesOnlyCommands(t *testing.T) {
	const body = "git add <file>\n<p>text</p>\n"
	for _, tc := range []struct{ tool, want string }{
		{toolBash, "git add placeholder\nplaceholdertext</p>\n"},
		{toolPowerShell, "git add placeholder\nplaceholdertext</p>\n"},
		{toolWrite, body},
	} {
		written := block{path: "a/guide.md", line: 7, tool: tc.tool, language: "x", body: body}
		want := written
		want.body = tc.want
		if got := sentBlock(written); got != want {
			t.Errorf("%s: sentBlock gives %+v, want %+v", tc.tool, got, want)
		}
	}
}

func TestExtractUnclosedFence(t *testing.T) {
	path, text := readFixture(t, "unclosed.md")
	got, err := extractBlocks(path, text)
	if err == nil {
		t.Fatalf("got %+v and no error, want an error", got)
	}
	if want := path + ":3:"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not name %q", err, want)
	}
}

func TestSubstitutePlaceholders(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"git add <file>", "git add placeholder"},
		{"ls <work-folder>/plan.md", "ls placeholder/plan.md"},
		{"git add <.gitignore, when changed>", "git add placeholder"},
		{"git show <the tree id the collection reported>", "git show placeholder"},
		{"git diff <base> <head>\ncat <file>\n", "git diff placeholder placeholder\ncat placeholder\n"},
		{"while read -r f; do :; done < <(git ls-files)", "while read -r f; do :; done < <(git ls-files)"},
		{"cat <<EOF\ntext\nEOF\n", "cat <<EOF\ntext\nEOF\n"},
		{"make build 2>&1", "make build 2>&1"},
		{"sort <in >out", "sort <in >out"},
		{"gh api repos/{owner}/{repo}", "gh api repos/{owner}/{repo}"},
	} {
		if got := substitutePlaceholders(tc.in); got != tc.want {
			t.Errorf("substitutePlaceholders(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDigestNormalizesLineEndings(t *testing.T) {
	// The first 12 hexadecimal characters of the SHA-256 of "git status\ngit log\n", computed with sha256sum.
	const want = "cc1525e36663"
	for _, body := range []string{
		"git status\ngit log\n",
		"git status\r\ngit log\r\n",
		"git status\ngit log",
	} {
		if got := bodyDigest(body); got != want {
			t.Errorf("bodyDigest(%q) = %q, want %q", body, got, want)
		}
	}
	if got := bodyDigest("git status\n"); got == want {
		t.Errorf("bodyDigest of a different body is also %q", got)
	}
}
