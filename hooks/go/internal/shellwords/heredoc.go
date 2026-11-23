package shellwords

import (
	"regexp"
	"strings"
)

// heredoc is one heredoc operator: its delimiter word, whether <<- strips tabs from the closing line, and whether a
// quote or backslash in the word marks the body quoted.
type heredoc struct {
	delimiter string
	stripTabs bool
	quoted    bool
}

// closedBy reports whether line, a body line without its newline, is the closing line of d under g.
func (d heredoc) closedBy(line string, g delimiterGrammar) bool {
	if g == parserDelimiters {
		line = strings.TrimSuffix(line, "\r")
	}
	if d.stripTabs {
		line = strings.TrimLeft(line, "\t")
	}
	return line == d.delimiter
}

// heredoc returns the operator that e's body belongs to.
func (e heredocExtent) heredoc() heredoc {
	return heredoc{delimiter: e.delimiter, stripTabs: e.stripTabs, quoted: e.quoted}
}

// plainDelimiter matches the delimiter words whose end of body the lexer and bash agree on: a word of letters, digits
// and _ . -, bare, in single or double quotes, or after one backslash. Inside double quotes bash keeps a backslash
// that the lexer drops, the $ before a quoted word starts a quote to bash and is a letter to the lexer, and other
// characters have meanings the lexer does not model.
var plainDelimiter = regexp.MustCompile(`^(?:[A-Za-z0-9_.-]+|'[A-Za-z0-9_.-]+'|"[A-Za-z0-9_.-]+"|\\[A-Za-z0-9_.-]+)$`)

// delimiterGrammar is how a heredoc delimiter word is read and how its closing line is matched. The parser and the
// parameter scan read delimiters differently on purpose: either reading imposed on the other would loosen it, so the
// grammar is the caller's choice and the body walk is shared.
type delimiterGrammar int

const (
	// parserDelimiters reads the word as the parser's lexer does: quotes and backslashes are removed, a backslash does
	// not mark the body quoted, and any byte up to a blank or an operator belongs to the word. A closing line matches
	// after one final \r is removed and, for <<-, its leading tabs.
	parserDelimiters delimiterGrammar = iota
	// identifierDelimiters reads a quoted run, or an optional backslash and a C identifier, and the backslash marks
	// the body quoted. A closing line matches after the <<- tab strip only, so a CRLF closing line never closes.
	identifierDelimiters
)

// readHeredocOperator reads the heredoc operator that starts at text[i:] (<< or <<- and its delimiter word) and
// returns it with the offset after the word, or end 0 when it opens no body: a here-string, no word, or a word the
// grammar does not accept. The caller owns the quote walk that decides an operator starts at i. Under
// identifierDelimiters a shift such as 1<<2 is no operator, because its operand does not start like an identifier.
// Under parserDelimiters the word is read on the line holding i with any \r kept, and an empty word opens no body.
func readHeredocOperator(text string, i int, g delimiterGrammar) (heredoc, int) {
	if g == identifierDelimiters {
		return readIdentifierOperator(text, i)
	}
	return readParserOperator(text, i)
}

func readIdentifierOperator(text string, i int) (heredoc, int) {
	j := i + 2
	if j >= len(text) || text[j] == '<' {
		return heredoc{}, 0
	}
	var doc heredoc
	if text[j] == '-' {
		doc.stripTabs = true
		j++
	}
	for j < len(text) && (text[j] == ' ' || text[j] == '\t') {
		j++
	}
	if j < len(text) && (text[j] == '\'' || text[j] == '"') {
		end := strings.IndexByte(text[j+1:], text[j])
		if end < 0 {
			return heredoc{}, 0
		}
		doc.delimiter, doc.quoted = text[j+1:j+1+end], true
		return doc, j + end + 2
	}
	if j < len(text) && text[j] == '\\' {
		doc.quoted = true
		j++
	}
	k := j
	for k < len(text) && (text[k] == '_' || text[k] >= 'a' && text[k] <= 'z' || text[k] >= 'A' && text[k] <= 'Z' || k > j && text[k] >= '0' && text[k] <= '9') {
		k++
	}
	if k == j {
		return heredoc{}, 0
	}
	doc.delimiter = text[j:k]
	return doc, k
}

// readParserOperator reads the delimiter word exactly as inspectHeredocsOnLine's word loop does, so the two agree on
// every word, including an unclosed quote that runs to the end of the line.
func readParserOperator(text string, i int) (heredoc, int) {
	line := text
	if n := strings.IndexByte(text[i:], '\n'); n >= 0 {
		line = text[:i+n]
	}
	j := i + 2
	if j < len(line) && line[j] == '<' {
		return heredoc{}, 0
	}
	var doc heredoc
	if j < len(line) && line[j] == '-' {
		doc.stripTabs = true
		j++
	}
	for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
		j++
	}
	var delimiter strings.Builder
	quoted, escaped := false, false
	for j < len(line) {
		c := line[j]
		if escaped {
			delimiter.WriteByte(c)
			escaped = false
			j++
			continue
		}
		if c == '\\' && !quoted {
			escaped = true
			j++
			continue
		}
		if c == '\'' || c == '"' {
			quote := c
			quoted = true
			j++
			for j < len(line) && line[j] != quote {
				if quote == '"' && line[j] == '\\' && j+1 < len(line) {
					j++
				}
				delimiter.WriteByte(line[j])
				j++
			}
			if j < len(line) {
				j++
			}
			continue
		}
		if strings.ContainsRune(" \t\r;|&<>", rune(c)) {
			break
		}
		delimiter.WriteByte(c)
		j++
	}
	if delimiter.Len() == 0 {
		return heredoc{}, 0
	}
	doc.delimiter, doc.quoted = delimiter.String(), quoted
	return doc, j
}

// walkHeredocBodies steps over the bodies of docs, in order, starting at from. Each span is the byte range of one
// body with its closing line and that line's newline included. A body with no closing line runs to the end of the
// text, and every doc after it gets the empty span at the end. next is the end of the last span, or from when docs
// is empty.
func walkHeredocBodies(text string, from int, docs []heredoc, g delimiterGrammar) (spans [][2]int, next int, terminated bool) {
	next, terminated = from, true
	for _, doc := range docs {
		end, _, closed := walkHeredocBody(text, next, doc, g)
		spans = append(spans, [2]int{next, end})
		next = end
		terminated = terminated && closed
	}
	return spans, next, terminated
}

// walkHeredocBody steps over one body starting at from and returns the offset after its closing line and the number
// of lines it read. A line exists only before the end of the text, so the empty remainder after a final newline is
// never a closing line, even for an empty delimiter.
func walkHeredocBody(text string, from int, doc heredoc, g delimiterGrammar) (end, lines int, terminated bool) {
	for from < len(text) {
		line, after := text[from:], len(text)
		if n := strings.IndexByte(line, '\n'); n >= 0 {
			line, after = line[:n], from+n+1
		}
		from = after
		lines++
		if doc.closedBy(line, g) {
			return from, lines, true
		}
	}
	return len(text), lines, false
}

// heredocExtent is one heredoc body as the parser reads it. introLine is the 0-based line of its operator, and the
// span [bodyStart, bodyEnd) includes the closing line and its newline.
type heredocExtent struct {
	introLine  int
	delimiter  string
	stripTabs  bool
	quoted     bool
	bodyStart  int
	bodyEnd    int
	terminated bool
}

// heredocExtents is the parser's reading of every heredoc in a text: which lines start executable text, where each
// body lies, and whether the reading can be trusted.
type heredocExtents struct {
	starts      []bool
	extents     []heredocExtent
	odd         int
	isComplete  bool
	isReliable  bool
	hasOperator bool
}

// lineReading is inspectHeredocsOnLine's answer for one line, kept so the second walk does not read the line again.
type lineReading struct {
	index         int
	docs          []heredoc
	reliable, odd bool
}

// readHeredocExtents reads text under parserDelimiters. An introducing line has two readings, which differ only when
// its last delimiter word runs into a final \r: the line starts strip that \r, and every other answer keeps it and
// takes its start lines from the first reading. Two walks reproduce both, and the second reuses the first's reading
// of a start line without a \r, so each walk reads every line once, as an introducing line or as a body line.
func readHeredocExtents(text string) heredocExtents {
	h := heredocExtents{isComplete: true, isReliable: true, hasOperator: strings.Contains(text, "<<")}
	h.starts = make([]bool, strings.Count(text, "\n")+1)
	var st lineState
	var readings []lineReading
	walkIntroLines(text, func(index int, raw string) []heredoc {
		h.starts[index] = st.clean()
		line := strings.TrimSuffix(raw, "\r")
		var docs []heredoc
		if h.starts[index] {
			r := lineReading{index: index}
			r.docs, r.reliable, r.odd = inspectHeredocsOnLine(line)
			docs = r.docs
			if len(line) == len(raw) {
				readings = append(readings, r)
			}
		}
		st.scan(line)
		return docs
	}, nil)
	walkIntroLines(text, func(index int, raw string) []heredoc {
		for len(readings) > 0 && readings[0].index < index {
			readings = readings[1:]
		}
		var r lineReading
		if len(readings) > 0 && readings[0].index == index {
			r = readings[0]
		} else {
			r.docs, r.reliable, r.odd = inspectHeredocsOnLine(raw)
		}
		if !r.reliable {
			h.isReliable = false
		}
		if !h.starts[index] {
			if len(r.docs) != 0 {
				h.isReliable = false
			}
			return nil
		}
		if r.odd && h.odd == 0 {
			h.odd = index + 1
		}
		if len(r.docs) > 0 && (strings.ContainsAny(raw, "`()") || strings.HasSuffix(raw, "\\")) {
			h.isReliable = false
		}
		return r.docs
	}, func(e heredocExtent) {
		h.extents = append(h.extents, e)
		if !e.terminated {
			h.isComplete = false
		}
	})
	return h
}

// walkIntroLines calls visit for every line outside a heredoc body with its 0-based index and its bytes without the
// newline, and steps over the bodies of the heredocs visit returns. The lines are those of strings.Split on \n, so a
// text ending in a newline has an empty last line; a body with no closing line takes every remaining line.
func walkIntroLines(text string, visit func(index int, raw string) []heredoc, body func(heredocExtent)) {
	lines := strings.Count(text, "\n") + 1
	pos := 0
	for index := 0; index < lines; {
		end := len(text)
		if n := strings.IndexByte(text[pos:], '\n'); n >= 0 {
			end = pos + n
		}
		docs := visit(index, text[pos:end])
		introLine := index
		index++
		pos = min(end+1, len(text))
		for _, doc := range docs {
			bodyEnd, read, terminated := walkHeredocBody(text, pos, doc, parserDelimiters)
			if body != nil {
				body(heredocExtent{
					introLine: introLine, delimiter: doc.delimiter, stripTabs: doc.stripTabs, quoted: doc.quoted,
					bodyStart: pos, bodyEnd: bodyEnd, terminated: terminated,
				})
			}
			pos = bodyEnd
			if terminated {
				index += read
			} else {
				index = lines
			}
		}
	}
}

// lineStarts is LineStarts' answer.
func (h heredocExtents) lineStarts() []bool { return h.starts }

// oddLine is the script line view's odd heredoc line: the 1-based start line of the first introduction whose delimiter word is not
// plain, or 0.
func (h heredocExtents) oddLine() int {
	if !h.hasOperator {
		return 0
	}
	return h.odd
}

// complete reports that every body has a closing line.
func (h heredocExtents) complete() bool { return h.isComplete }

// reliable reports that every introduction is unambiguous and on a line that starts executable text.
func (h heredocExtents) reliable() bool { return h.isReliable }

// spans returns the byte span of every body, quoted or not, closing line included.
func (h heredocExtents) spans() [][2]int {
	if len(h.extents) == 0 {
		return nil
	}
	spans := make([][2]int, len(h.extents))
	for i, e := range h.extents {
		spans[i] = [2]int{e.bodyStart, e.bodyEnd}
	}
	return spans
}

// blankQuoted returns text, the text these extents were read from, with every byte of a quoted body other than a
// line ending replaced by a space, closing line excepted, so byte positions are preserved.
func (h heredocExtents) blankQuoted(text string) string {
	out := []byte(text)
	for _, e := range h.extents {
		if !e.quoted {
			continue
		}
		for j := e.bodyStart; j < e.contentEnd(text); j++ {
			if out[j] != '\n' && out[j] != '\r' {
				out[j] = ' '
			}
		}
	}
	return string(out)
}

// stripBodies returns text, the text these extents were read from, without its heredoc bodies. An unquoted body that
// holds a substitution is kept as masked data so its commands stay inspectable, without the lines that equal the
// delimiter before any tab strip. A tab-indented closing line therefore stays as data, while
// a line equal to a delimiter that itself starts with a tab is dropped even where it does not close the body.
func (h heredocExtents) stripBodies(text string) string {
	if !strings.Contains(text, "<<") {
		return text
	}
	var out strings.Builder
	pos := 0
	for _, e := range h.extents {
		out.WriteString(text[pos:e.bodyStart])
		pos = e.bodyEnd
		body := text[e.bodyStart:e.bodyEnd]
		if e.quoted || !strings.Contains(body, "$(") && !strings.Contains(body, "`") {
			continue
		}
		var data strings.Builder
		for line := range strings.Lines(body) {
			if strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r") != e.delimiter {
				data.WriteString(line)
			}
		}
		out.WriteString(maskHeredocData(data.String()))
	}
	out.WriteString(text[pos:])
	return out.String()
}

// contentEnd is where the closing line starts, or bodyEnd for a body with no closing line.
func (e heredocExtent) contentEnd(text string) int {
	if !e.terminated {
		return e.bodyEnd
	}
	last := e.bodyEnd
	if last > e.bodyStart && text[last-1] == '\n' {
		last--
	}
	return e.bodyStart + strings.LastIndexByte(text[e.bodyStart:last], '\n') + 1
}

// inspectHeredocsOnLine returns the heredocs a line introduces. reliable is false when the line's shape leaves the
// introductions in doubt. odd reports an introduction whose delimiter word is not a plain one, where the lexer may end
// the body on another line than bash; such a line is never reliable. The heredocs returned do not depend on either
// flag, so how commands are split is the same for every caller.
func inspectHeredocsOnLine(line string) (docs []heredoc, reliable, odd bool) {
	reliable = true
	inSingle, inDouble := false, false
	// quotedSubst holds one paren depth per $( opened inside double quotes.
	// Inside such a frame the shell is unquoted again, so a heredoc intro
	// there is recognized exactly as in an unquoted $( … ).
	var quotedSubst []int
	wordStart := true
	for i := 0; i+1 < len(line); i++ {
		c := line[i]
		if inSingle {
			if c == '\'' {
				inSingle = false
			}
			continue
		}
		if !inDouble && c == '$' && line[i+1] == '\'' {
			i = ansiCEnd(line, i) - 1
			wordStart = false
			continue
		}
		if c == '\\' {
			wordStart = false
			i++
			continue
		}
		// A span that closes as `))` on this line with no command substitution
		// inside is arithmetic, so a << in it is a shift, never a heredoc:
		// skip to the matching close. Anything else — an unclosed span, a
		// close with anything but another `)` immediately before it, or a
		// nested $( or backtick — is read by bash as $(( cmd ) ), a command
		// substitution wrapping a subshell, not arithmetic (this is exactly
		// bash's own rule for telling the two apart), so a heredoc inside it
		// must still be recognized: fall through to the existing
		// quotedSubst/unquoted handling below instead of skipping.
		if c == '$' && line[i+1] == '(' && i+2 < len(line) && line[i+2] == '(' {
			depth := 0
			j := i + 1
			for ; j < len(line); j++ {
				if line[j] == '(' {
					depth++
				} else if line[j] == ')' {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			if j < len(line) && line[j-1] == ')' {
				span := line[i+3 : j-1]
				if !strings.Contains(span, "$(") && !strings.ContainsRune(span, '`') {
					i = j
					wordStart = false
					continue
				}
			}
		}
		if inDouble && c == '$' && line[i+1] == '(' {
			quotedSubst = append(quotedSubst, 1)
			inDouble = false
			wordStart = true
			i++
			continue
		}
		if !inDouble && len(quotedSubst) > 0 && (c == '(' || c == ')') {
			top := len(quotedSubst) - 1
			if c == '(' {
				quotedSubst[top]++
			} else {
				quotedSubst[top]--
			}
			if quotedSubst[top] == 0 {
				quotedSubst = quotedSubst[:top]
				inDouble = true
				wordStart = false
				continue
			}
			wordStart = true
			continue
		}
		if c == '"' {
			inDouble = !inDouble
			wordStart = false
			continue
		}
		if !inDouble && c == '\'' {
			inSingle = true
			wordStart = false
			continue
		}
		// A shell comment cannot introduce a heredoc on a later executable line.
		if !inDouble && c == '#' && wordStart {
			break
		}
		if !inDouble {
			wordStart = strings.ContainsRune(" \t\r;|&()<>", rune(c))
		}
		// A here-string (<<<) feeds one word to a command and opens no body. Consume all three characters: leaving the
		// second and third to the loop reads them as a << introducer whose delimiter is the word after the here-string,
		// and every following line is then swallowed as heredoc data. The span stays unreliable so callers that need
		// proof of the line's shape keep treating it conservatively.
		if !inDouble && c == '<' && line[i+1] == '<' && i+2 < len(line) && line[i+2] == '<' {
			reliable = false
			wordStart = false
			i += 2
			continue
		}
		if inDouble || c != '<' || line[i+1] != '<' {
			continue
		}
		j := i + 2
		stripTabs := false
		if j < len(line) && line[j] == '-' {
			stripTabs = true
			j++
		}
		for j < len(line) && (line[j] == ' ' || line[j] == '\t') {
			j++
		}
		var delimiter strings.Builder
		quoted, escaped := false, false
		word := j
		for j < len(line) {
			c = line[j]
			if escaped {
				delimiter.WriteByte(c)
				escaped = false
				j++
				continue
			}
			if c == '\\' && !quoted {
				escaped = true
				j++
				continue
			}
			if c == '\'' || c == '"' {
				quote := c
				quoted = true
				j++
				for j < len(line) && line[j] != quote {
					if quote == '"' && line[j] == '\\' && j+1 < len(line) {
						j++
					}
					delimiter.WriteByte(line[j])
					j++
				}
				if j < len(line) {
					j++
				} else {
					reliable = false
				}
				continue
			}
			if strings.ContainsRune(" \t\r;|&<>", rune(c)) {
				break
			}
			delimiter.WriteByte(c)
			j++
		}
		if !plainDelimiter.MatchString(line[word:j]) {
			odd, reliable = true, false
		}
		if delimiter.Len() > 0 {
			docs = append(docs, heredoc{delimiter: delimiter.String(), stripTabs: stripTabs, quoted: quoted})
		} else {
			reliable = false
		}
		if escaped {
			reliable = false
		}
		i = j - 1
		wordStart = false
	}
	return docs, reliable, odd
}

type lineState struct {
	single, double, ansi, continued, backtick bool
	subst                                     []int // paren depth per $( opened inside double quotes
}

func (s *lineState) clean() bool {
	return !s.single && !s.double && !s.ansi && !s.continued && !s.backtick && len(s.subst) == 0
}

// scan advances the state across one line. A # at word start outside any
// quote ends the scan; a trailing unescaped backslash sets continued.
func (s *lineState) scan(line string) {
	s.continued = false
	wordStart := true
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case s.single:
			if c == '\'' {
				s.single = false
			}
		case s.ansi:
			if c == '\\' {
				i++
			} else if c == '\'' {
				s.ansi = false
			}
		case s.double && len(s.subst) == 0:
			switch {
			case c == '\\':
				i++
			case c == '"':
				s.double = false
			case c == '$' && i+1 < len(line) && line[i+1] == '(':
				s.subst = append(s.subst, 1)
				i++
			case c == '`':
				s.backtick = !s.backtick
			}
		default:
			switch {
			case c == '\\':
				if i == len(line)-1 {
					s.continued = true
				}
				i++
			case c == '#' && wordStart && len(s.subst) == 0:
				return
			case c == '\'' && i > 0 && line[i-1] == '$':
				s.ansi = true
			case c == '\'':
				s.single = true
			case c == '"' && len(s.subst) > 0:
				j := strings.IndexByte(line[i+1:], '"')
				if j < 0 {
					s.double, s.subst = true, s.subst[:0] // unclosed: fail closed
					return
				}
				i += j + 1
			case c == '"':
				s.double = true
			case c == '`':
				s.backtick = !s.backtick
			case len(s.subst) > 0 && c == '(':
				s.subst[len(s.subst)-1]++
			case len(s.subst) > 0 && c == ')':
				s.subst[len(s.subst)-1]--
				if s.subst[len(s.subst)-1] == 0 {
					s.subst = s.subst[:len(s.subst)-1]
					s.double = true // back inside the enclosing double quotes
				}
			}
			wordStart = strings.ContainsRune(" \t;|&()<>", rune(c))
		}
	}
}
