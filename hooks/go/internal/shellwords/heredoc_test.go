package shellwords

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// heredocExtentsRow is one input of TestHeredocExtents, read under one delimiter grammar.
type heredocExtentsRow struct {
	name    string
	grammar delimiterGrammar
	text    string

	// at is the offset of an operator readHeredocOperator reads; 0 skips the operator check, since no row has an
	// operator at offset 0. doc and end are its expected result.
	at  int
	doc heredoc
	end int

	// Expected answers of readHeredocExtents, checked for parserDelimiters rows. strip and blank are checked only
	// when set; every parser row is also compared with the old readers.
	extents            []heredocExtent
	complete, reliable bool
	odd                int
	strip, blank       string

	// Expected walkHeredocBodies result for identifierDelimiters rows, over the doc read at at, from the line after
	// the operator's line.
	spans      [][2]int
	next       int
	terminated bool
}

func TestHeredocExtents(t *testing.T) {
	const (
		parser     = parserDelimiters
		identifier = identifierDelimiters
	)
	for _, row := range []heredocExtentsRow{
		{
			name: "bare delimiter", grammar: parser, text: "cat <<EOF\nhi $x\nEOF\necho done\n",
			at: 4, doc: heredoc{delimiter: "EOF"}, end: 9,
			extents:  []heredocExtent{{delimiter: "EOF", bodyStart: 10, bodyEnd: 20, terminated: true}},
			complete: true, reliable: true,
			strip: "cat <<EOF\necho done\n",
		},
		{
			name: "bare delimiter", grammar: identifier, text: "cat <<EOF\nhi $x\nEOF\necho done\n",
			at: 4, doc: heredoc{delimiter: "EOF"}, end: 9,
			spans: [][2]int{{10, 20}}, next: 20, terminated: true,
		},
		{
			name: "single-quoted delimiter", grammar: parser, text: "cat <<'EOF'\n$x\nEOF\n",
			at: 4, doc: heredoc{delimiter: "EOF", quoted: true}, end: 11,
			extents:  []heredocExtent{{delimiter: "EOF", quoted: true, bodyStart: 12, bodyEnd: 19, terminated: true}},
			complete: true, reliable: true,
			blank: "cat <<'EOF'\n  \nEOF\n",
		},
		{
			name: "single-quoted delimiter", grammar: identifier, text: "cat <<'EOF'\n$x\nEOF\n",
			at: 4, doc: heredoc{delimiter: "EOF", quoted: true}, end: 11,
			spans: [][2]int{{12, 19}}, next: 19, terminated: true,
		},
		{
			name: "double-quoted delimiter", grammar: parser, text: "cat <<\"EOF\"\n$x\nEOF\n",
			at: 4, doc: heredoc{delimiter: "EOF", quoted: true}, end: 11,
			extents:  []heredocExtent{{delimiter: "EOF", quoted: true, bodyStart: 12, bodyEnd: 19, terminated: true}},
			complete: true, reliable: true,
			blank: "cat <<\"EOF\"\n  \nEOF\n",
		},
		{
			name: "double-quoted delimiter", grammar: identifier, text: "cat <<\"EOF\"\n$x\nEOF\n",
			at: 4, doc: heredoc{delimiter: "EOF", quoted: true}, end: 11,
			spans: [][2]int{{12, 19}}, next: 19, terminated: true,
		},
		{
			// The parser lexer drops the backslash without marking the body quoted.
			name: "backslash delimiter", grammar: parser, text: "cat <<\\EOF\n$x\nEOF\n",
			at: 4, doc: heredoc{delimiter: "EOF"}, end: 10,
			extents:  []heredocExtent{{delimiter: "EOF", bodyStart: 11, bodyEnd: 18, terminated: true}},
			complete: true, reliable: true,
			blank: "cat <<\\EOF\n$x\nEOF\n",
		},
		{
			name: "backslash delimiter", grammar: identifier, text: "cat <<\\EOF\n$x\nEOF\n",
			at: 4, doc: heredoc{delimiter: "EOF", quoted: true}, end: 10,
			spans: [][2]int{{11, 18}}, next: 18, terminated: true,
		},
		{
			name: "tab-indented closing line", grammar: parser, text: "cat <<-EOF\n\tx\n\tEOF\necho\n",
			at: 4, doc: heredoc{delimiter: "EOF", stripTabs: true}, end: 10,
			extents:  []heredocExtent{{delimiter: "EOF", stripTabs: true, bodyStart: 11, bodyEnd: 19, terminated: true}},
			complete: true, reliable: true,
		},
		{
			name: "tab-indented closing line", grammar: identifier, text: "cat <<-EOF\n\tx\n\tEOF\necho\n",
			at: 4, doc: heredoc{delimiter: "EOF", stripTabs: true}, end: 10,
			spans: [][2]int{{11, 19}}, next: 19, terminated: true,
		},
		{
			name: "CRLF introducing and closing line", grammar: parser, text: "cat <<EOF\r\nx\r\nEOF\r\necho\r\n",
			at: 4, doc: heredoc{delimiter: "EOF"}, end: 9,
			extents:  []heredocExtent{{delimiter: "EOF", bodyStart: 11, bodyEnd: 19, terminated: true}},
			complete: true, reliable: true,
		},
		{
			// The parameter scan does not strip the carriage return, so the CRLF closing line never closes.
			name: "CRLF introducing and closing line", grammar: identifier, text: "cat <<EOF\r\nx\r\nEOF\r\necho\r\n",
			at: 4, doc: heredoc{delimiter: "EOF"}, end: 9,
			spans: [][2]int{{11, 25}}, next: 25, terminated: false,
		},
		{
			name: "unterminated body", grammar: parser, text: "cat <<EOF\nx\n",
			at: 4, doc: heredoc{delimiter: "EOF"}, end: 9,
			extents:  []heredocExtent{{delimiter: "EOF", bodyStart: 10, bodyEnd: 12}},
			complete: false, reliable: true,
		},
		{
			name: "unterminated body", grammar: identifier, text: "cat <<EOF\nx\n",
			at: 4, doc: heredoc{delimiter: "EOF"}, end: 9,
			spans: [][2]int{{10, 12}}, next: 12, terminated: false,
		},
		{
			name: "here-string", grammar: parser, text: "cat <<<x\necho\n",
			at:       4,
			complete: true, reliable: false,
		},
		{
			name: "here-string", grammar: identifier, text: "cat <<<x\necho\n",
			at: 4,
		},
		{
			name: "two heredocs on one line", grammar: parser, text: "cat <<A <<'B'\na\nA\n$b\nB\n",
			at: 4, doc: heredoc{delimiter: "A"}, end: 7,
			extents: []heredocExtent{
				{delimiter: "A", bodyStart: 14, bodyEnd: 18, terminated: true},
				{delimiter: "B", quoted: true, bodyStart: 18, bodyEnd: 23, terminated: true},
			},
			complete: true, reliable: true,
			blank: "cat <<A <<'B'\na\nA\n  \nB\n",
		},
		{
			name: "operator inside quotes", grammar: parser, text: "echo '\ncat <<EOF\n'\n",
			complete: true, reliable: false,
		},
		{
			name: "operator after a continuation", grammar: parser, text: "echo \\\ncat <<EOF\nEOF\n",
			complete: true, reliable: false,
		},
		{
			name: "operator inside $(", grammar: parser, text: "x=$(cat <<EOF\nhi\nEOF\n)\n",
			at: 8, doc: heredoc{delimiter: "EOF"}, end: 13,
			extents:  []heredocExtent{{delimiter: "EOF", bodyStart: 14, bodyEnd: 21, terminated: true}},
			complete: true, reliable: false,
		},
		{
			name: "operator inside $((", grammar: parser, text: "echo $((1<<2))\ncat\n",
			complete: true, reliable: true,
		},
		{
			name: "E-1", grammar: parser, text: "cat <<E-1\nx\nE-1\n",
			at: 4, doc: heredoc{delimiter: "E-1"}, end: 9,
			extents:  []heredocExtent{{delimiter: "E-1", bodyStart: 10, bodyEnd: 16, terminated: true}},
			complete: true, reliable: true,
		},
		{
			name: "E-1", grammar: identifier, text: "cat <<E-1\nx\nE-1\n",
			at: 4, doc: heredoc{delimiter: "E"}, end: 7,
			spans: [][2]int{{10, 16}}, next: 16, terminated: false,
		},
		{
			name: "E.F", grammar: parser, text: "cat <<E.F\nE.F\n",
			at: 4, doc: heredoc{delimiter: "E.F"}, end: 9,
			extents:  []heredocExtent{{delimiter: "E.F", bodyStart: 10, bodyEnd: 14, terminated: true}},
			complete: true, reliable: true,
		},
		{
			name: "E.F", grammar: identifier, text: "cat <<E.F\nE.F\n",
			at: 4, doc: heredoc{delimiter: "E"}, end: 7,
			spans: [][2]int{{10, 14}}, next: 14, terminated: false,
		},
		{
			name: "1", grammar: parser, text: "cat <<1\nx\n1\n",
			at: 4, doc: heredoc{delimiter: "1"}, end: 7,
			extents:  []heredocExtent{{delimiter: "1", bodyStart: 8, bodyEnd: 12, terminated: true}},
			complete: true, reliable: true,
		},
		{
			// An operand that does not start like an identifier is a shift to the parameter scan.
			name: "1", grammar: identifier, text: "cat <<1\nx\n1\n",
			at: 4,
		},
		{
			name: "\\EOF", grammar: parser, text: "cat <<\\EOF\nEOF\n",
			at: 4, doc: heredoc{delimiter: "EOF"}, end: 10,
			extents:  []heredocExtent{{delimiter: "EOF", bodyStart: 11, bodyEnd: 15, terminated: true}},
			complete: true, reliable: true,
		},
		{
			name: "\\EOF", grammar: identifier, text: "cat <<\\EOF\nEOF\n",
			at: 4, doc: heredoc{delimiter: "EOF", quoted: true}, end: 10,
			spans: [][2]int{{11, 15}}, next: 15, terminated: true,
		},
		{
			// The stripped introducing line reads the delimiter E and the unstripped one E\r: the line starts follow
			// the first reading and every other answer the second.
			name: "unclosed quote before CR", grammar: parser, text: "cat <<'E\r\nx\r\nE\r\nfoo\r\n",
			at: 4, doc: heredoc{delimiter: "E\r", quoted: true}, end: 9,
			extents:  []heredocExtent{{delimiter: "E\r", quoted: true, bodyStart: 10, bodyEnd: 21}},
			complete: false, reliable: false, odd: 1,
			strip: "cat <<'E\r\n", blank: "cat <<'E\r\n \r\n \r\n   \r\n",
		},
		{
			name: "backslash before CR", grammar: parser, text: "cat <<E\\\r\nx\r\nE\r\nfoo\r\n",
			at: 4, doc: heredoc{delimiter: "E\r"}, end: 9,
			extents:  []heredocExtent{{delimiter: "E\r", bodyStart: 10, bodyEnd: 21}},
			complete: false, reliable: false, odd: 1,
			strip: "cat <<E\\\r\n",
		},
		{
			// The parser lexer reads no delimiter from '' and introduces no body.
			name: "empty delimiter", grammar: parser, text: "cat <<''\n\nx\n",
			at:       4,
			complete: true, reliable: false, odd: 1,
		},
		{
			// A line exists only before the text's end, so the body never closes.
			name: "empty delimiter at the end", grammar: identifier, text: "cat <<''\n",
			at: 4, doc: heredoc{quoted: true}, end: 8,
			spans: [][2]int{{9, 9}}, next: 9, terminated: false,
		},
		{
			name: "empty delimiter before an empty line", grammar: identifier, text: "cat <<''\n\nx\n",
			at: 4, doc: heredoc{quoted: true}, end: 8,
			spans: [][2]int{{9, 10}}, next: 10, terminated: true,
		},
		{
			// The strip keeps the substitution of an active body and the tab-indented closing line as masked data.
			name: "active body with a tab-indented closing line", grammar: parser, text: "cat <<-E\n\t$(id)\n\tE\nnext\n",
			at: 4, doc: heredoc{delimiter: "E", stripTabs: true}, end: 8,
			extents:  []heredocExtent{{delimiter: "E", stripTabs: true, bodyStart: 9, bodyEnd: 19, terminated: true}},
			complete: true, reliable: true,
			strip: "cat <<-E\n  (id)\n  \nnext\n",
		},
		{
			// The escaped tab is part of the delimiter, which the tab strip then never matches, yet the strip drops
			// the line equal to it.
			name: "active body with a tab-led delimiter", grammar: parser, text: "cat <<-\\\tE\n\tE\n$(id)\n",
			at: 4, doc: heredoc{delimiter: "\tE", stripTabs: true}, end: 10,
			extents:  []heredocExtent{{delimiter: "\tE", stripTabs: true, bodyStart: 11, bodyEnd: 20}},
			complete: false, reliable: false, odd: 1,
			strip: "cat <<-\\\tE\n (id)\n",
		},
	} {
		t.Run(row.name+"/"+grammarName(row.grammar), func(t *testing.T) {
			if row.at > 0 {
				doc, end := readHeredocOperator(row.text, row.at, row.grammar)
				if doc != row.doc || end != row.end {
					t.Errorf("readHeredocOperator(%q, %d) = %+v, %d; want %+v, %d", row.text, row.at, doc, end, row.doc, row.end)
				}
			}
			switch row.grammar {
			case parserDelimiters:
				checkParserRow(t, row)
			case identifierDelimiters:
				checkIdentifierRow(t, row)
			}
		})
	}
}

func grammarName(g delimiterGrammar) string {
	if g == identifierDelimiters {
		return "identifier"
	}
	return "parser"
}

func checkParserRow(t *testing.T, row heredocExtentsRow) {
	t.Helper()
	h := readHeredocExtents(row.text)
	if !slices.Equal(h.extents, row.extents) {
		t.Errorf("extents = %+v; want %+v", h.extents, row.extents)
	}
	if h.complete() != row.complete || h.reliable() != row.reliable || h.oddLine() != row.odd {
		t.Errorf("complete, reliable, oddLine = %v, %v, %d; want %v, %v, %d",
			h.complete(), h.reliable(), h.oddLine(), row.complete, row.reliable, row.odd)
	}
	if row.strip != "" && h.stripBodies(row.text) != row.strip {
		t.Errorf("stripBodies = %q; want %q", h.stripBodies(row.text), row.strip)
	}
	if row.blank != "" && h.blankQuoted(row.text) != row.blank {
		t.Errorf("blankQuoted = %q; want %q", h.blankQuoted(row.text), row.blank)
	}
	// The body walk over the docs of the first introducing line gives the same spans as the extents.
	if len(h.extents) == 0 {
		return
	}
	var docs []heredoc
	var want [][2]int
	wantTerminated := true
	for _, e := range h.extents {
		if e.introLine == h.extents[0].introLine {
			docs = append(docs, e.heredoc())
			want = append(want, [2]int{e.bodyStart, e.bodyEnd})
			wantTerminated = wantTerminated && e.terminated
		}
	}
	spans, next, terminated := walkHeredocBodies(row.text, h.extents[0].bodyStart, docs, parserDelimiters)
	if !slices.Equal(spans, want) || next != want[len(want)-1][1] || terminated != wantTerminated {
		t.Errorf("walkHeredocBodies = %v, %d, %v; want %v", spans, next, terminated, want)
	}
}

func checkIdentifierRow(t *testing.T, row heredocExtentsRow) {
	t.Helper()
	if row.end == 0 {
		return
	}
	from := strings.IndexByte(row.text, '\n') + 1
	spans, next, terminated := walkHeredocBodies(row.text, from, []heredoc{row.doc}, identifierDelimiters)
	if !slices.Equal(spans, row.spans) || next != row.next || terminated != row.terminated {
		t.Errorf("walkHeredocBodies = %v, %d, %v; want %v, %d, %v", spans, next, terminated, row.spans, row.next, row.terminated)
	}
}

func TestHeredocExtentsAreLinear(t *testing.T) {
	const operators = 10000
	line := "cat" + strings.Repeat(" <<A", operators)
	line += strings.Repeat(" ", 100000-len(line))
	many := line + "\n" + strings.Repeat("A\n", operators)
	noNewline := "cat" + strings.Repeat(" <<A", 100000/4)
	docs := make([]heredoc, operators)
	for i := range docs {
		docs[i] = heredoc{delimiter: "A"}
	}
	within := func(name string, run func()) {
		t.Helper()
		start := time.Now()
		run()
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("%s took %v; want at most 1s", name, elapsed)
		}
	}
	within("readHeredocExtents over many operators", func() {
		h := readHeredocExtents(many)
		if len(h.extents) != operators || !h.complete() {
			t.Errorf("extents = %d, complete = %v; want %d, true", len(h.extents), h.complete(), operators)
		}
	})
	within("readHeredocExtents over one line", func() {
		h := readHeredocExtents(noNewline)
		if len(h.extents) != 100000/4 || h.complete() {
			t.Errorf("extents = %d, complete = %v; want %d, false", len(h.extents), h.complete(), 100000/4)
		}
	})
	for _, g := range []delimiterGrammar{parserDelimiters, identifierDelimiters} {
		within("walkHeredocBodies over many operators/"+grammarName(g), func() {
			if _, next, terminated := walkHeredocBodies(many, len(line)+1, docs, g); !terminated || next != len(many) {
				t.Errorf("walkHeredocBodies = %d, %v; want %d, true", next, terminated, len(many))
			}
		})
		within("walkHeredocBodies over one line/"+grammarName(g), func() {
			if _, next, terminated := walkHeredocBodies(noNewline, 0, docs, g); terminated || next != len(noNewline) {
				t.Errorf("walkHeredocBodies = %d, %v; want %d, false", next, terminated, len(noNewline))
			}
		})
	}
}
