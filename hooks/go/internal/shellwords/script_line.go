package shellwords

import (
	"sort"
	"strconv"
	"strings"
)

// firstNonBlankLine returns the first line of s that holds more than whitespace, trimmed, or "" when there is none.
func firstNonBlankLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			return t
		}
	}
	return ""
}

// scriptLines locates the line of a child invocation in the text of the script it came from.
//
// text is the script with the parts that cannot hold a command blanked, positions preserved: the bodies of quoted
// heredocs and comments. A heredoc introduction the lexer cannot vouch for still has its body blanked, and a comment
// the tracker is unsure of is still blanked, which is enough to place a line number and is why the construct scans
// read scan instead. An unquoted heredoc body is not blanked,
// because its substitutions run: a body line equal to a later command can therefore take that command's line.
//
// norm is text with every run of whitespace and every backslash-newline folded to one space, and back maps each
// byte of norm to its offset in text. A child's Raw has its continuations rewritten, so a continued command is only
// found in norm.
//
// scan is the text the construct scans read. It is text when the heredoc bodies were all found and unambiguous and
// the comment tracker is sure of every comment, and the script as written otherwise: a scan that decides a block must
// not trust a blanking it cannot vouch for, and the raw text only errs toward marking.
//
// oddHeredoc is the line of the first heredoc whose body the parser may end on another line than bash, or 0: the
// first of what the delimiter check and the comment tracker's heredoc check find. The commands the parser found after
// it cannot be trusted, so the script is refused there.
type scriptLines struct {
	text       string
	scan       string
	norm       string
	back       []int
	newlines   []int
	oddHeredoc int
}

func newScriptLines(source string) *scriptLines {
	h := readHeredocExtents(source)
	text, sure, operator := blankComments(h.blankQuoted(source), h.spans())
	s := &scriptLines{text: text, scan: text, oddHeredoc: h.oddLine()}
	if operator > 0 && (s.oddHeredoc == 0 || operator < s.oddHeredoc) {
		s.oddHeredoc = operator
	}
	if !h.complete() || !h.reliable() || !sure || s.oddHeredoc > 0 {
		s.scan = source
	}
	// Index source newlines once so locating many matches does not rescan their prefixes.
	for i := range len(text) {
		if text[i] == '\n' {
			s.newlines = append(s.newlines, i)
		}
	}
	// norm never outgrows text (a run of white space folds to one space), so neither does back.
	norm := make([]byte, 0, len(s.text))
	s.back = make([]int, 0, len(s.text))
	pending := false
	for i := 0; i < len(s.text); i++ {
		c := s.text[i]
		switch {
		case c == '\\' && i+1 < len(s.text) && s.text[i+1] == '\n':
			pending = true
			i++
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			pending = true
		default:
			if pending && len(norm) > 0 {
				norm = append(norm, ' ')
				s.back = append(s.back, i)
			}
			pending = false
			norm = append(norm, c)
			s.back = append(s.back, i)
		}
	}
	s.norm = string(norm)
	return s
}

// The contexts the comment tracker can be in. A command context is the script itself or the inside of $( ), <( ) or
// >( ); the others are the quotes and expansions that can hold one.
const (
	inCommand = iota
	inBacktick
	inDouble
	inSingle
	inANSI
	inParam
	inArith
)

// quoteFrame is one open context of the comment tracker.
type quoteFrame struct {
	kind    int
	id      int    // the order the frame was opened in; a frame opened later has the greater id while both are open
	parens  []bool // the open parens of a command context, true for one opened inside a word: a=( @( name(
	cond    bool   // a command context inside [[ ]]
	depth   int    // the open parens inside an arithmetic context
	command bool   // an arithmetic context that is the (( )) command, not the $(( )) expansion
}

// wordParen reports whether a paren opened inside a word, such as an array assignment or an extended glob, is open.
func (f *quoteFrame) wordParen() bool {
	for _, w := range f.parens {
		if w {
			return true
		}
	}
	return false
}

// pendingHeredoc is a heredoc operator whose body is still to come: where it was written and the frame it was counted
// in.
type pendingHeredoc struct {
	pos   int
	frame int
}

// blankComments replaces each comment, from an unquoted # that starts a word to the end of its line, with spaces, and
// reports whether it is sure that bash reads every comment and heredoc body of text the same way.
//
// The answer decides whether a scan that marks a script may read the blanked text, so a comment is only blanked for
// sure where bash's reading does not depend on the shell version or on options: a # that starts a word at the top
// level of the script, after whitespace or one of ; & |. A # that bash may or may not take for a comment (after ( ) <
// or >, inside $( ), backticks or [[ ]], or while a paren opened inside a word is open), a quote inside backticks, a
// single quote inside ${ } inside double quotes, a case inside a nested command, an unclosed context, and a heredoc
// body that the operators on its introducing line do not account for all make the answer false. Both directions of a
// mistake matter: blanking code hides it, and reading a comment as code lets the quotes in it invert the state the
// rest of the script is read in.
//
// The heredoc bodies in bodies are data, not shell text: an apostrophe or # in one must not change the quote state
// that the code after it is read in, so they are stepped over whole.
//
// heredoc is the line of the first heredoc operator whose body bodies places where bash would not read it, or 0.
// Bash reads a body at the first newline token after the operator: a newline that ends the logical command in the
// context the operator was counted in, or in one that has since closed. A newline joined by a backslash is no token,
// and a newline inside a $( ), <( ), backtick, string or ${ } opened after the operator is part of a word, so the
// commands up to the close run first while bodies would take them for data. The invariant therefore refuses a body
// that starts after a backslash-newline, or while a frame opened after the operator's frame is still open, or while
// a string or expansion is open; and it refuses a count that does not match, a body with no operator or an operator
// with no body. A << that bash never takes for a heredoc operator is refused where it is counted: inside (( )),
// $(( )) and ${ } by the frame kind, inside [[ ]], a [ opened in a word on its line ($[ ] and a[i]= included) or a
// paren opened in a word (an array or an extended glob), and inside backticks, whose body bash looks for in the
// backtick text. A body inside a $( ) that spans lines only makes the answer unsure.
func blankComments(text string, bodies [][2]int) (blanked string, sure bool, heredoc int) {
	out := []byte(text)
	sure = true
	disagree := func(pos int) {
		sure = false
		if heredoc == 0 {
			heredoc = strings.Count(text[:max(pos, 0)], "\n") + 1
		}
	}
	frames := 1
	stack := []quoteFrame{{kind: inCommand, id: frames}}
	// start: the next byte begins a word, so a # there opens a comment. after: the byte before was ( ) < or >, where
	// whether a # opens a comment depends on what the paren or redirection belongs to.
	start, after := true, false
	var pending []pendingHeredoc // heredoc operators whose bodies are still to come
	next := 0                    // the next body in bodies
	// bracket counts the [ opened in a word on this line and not yet closed. In an assignment such as a[1<<E ]=x
	// the subscript is arithmetic and holds no heredoc, while in other words it does; the tracker does not tell the
	// two apart, so a << there is a disagreement.
	bracket := 0
	push := func(kind int) {
		frames++
		stack = append(stack, quoteFrame{kind: kind, id: frames})
	}
	// underArith reports whether an arithmetic context is open below the current one. Bash reads (( )) and $(( )) as
	// text and parses a substitution inside them when it runs, so where a heredoc body in one starts is not decided by
	// the newlines the lexer sees.
	underArith := func() bool {
		for _, f := range stack {
			if f.kind == inArith {
				return true
			}
		}
		return false
	}
	// Every context that closes, a $( ) included, is part of a word, so the byte after it continues that word; the
	// (( )) command is the one exception, and its caller says so.
	pop := func() {
		if len(stack) > 1 {
			stack = stack[:len(stack)-1]
		}
		start, after = false, false
	}
	at := func(i int, s string) bool { return i <= len(text) && strings.HasPrefix(text[i:], s) }
	// expansion opens the expansion that starts at i in any context that expands, and returns the last byte it
	// consumed, or -1 when nothing starts there.
	expansion := func(i int) int {
		switch {
		case text[i] == '`':
			push(inBacktick)
			start, after = true, false
			return i
		case at(i, "$(("):
			push(inArith)
			return i + 2
		case at(i, "$("):
			push(inCommand)
			start, after = true, true
			return i + 1
		case at(i, "${"):
			push(inParam)
			return i + 1
		case at(i, "$["):
			sure = false // the old arithmetic form; its [ counts as a bracket opened in a word
		}
		return -1
	}
	// keyword reports whether the word at i is w followed by a byte that ends a word.
	keyword := func(i int, w string) bool {
		if !at(i, w) {
			return false
		}
		j := i + len(w)
		return j == len(text) || strings.IndexByte(" \t\n;&|)", text[j]) >= 0
	}
	for i := 0; i < len(text); i++ {
		if next < len(bodies) && i >= bodies[next][0] {
			// The operators of the line before must account for exactly the bodies that follow it, and bash only reads a
			// body at a newline token that ends the command in the operator's own frame or an enclosing one. A body is
			// only sure at the top level: inside $( ) shells have differed on where it starts and ends.
			intro := bodies[next][0] - 1
			if len(pending) > 0 {
				intro = pending[0].pos
			}
			top := stack[len(stack)-1]
			if i != bodies[next][0] || top.kind != inCommand {
				disagree(intro)
			} else if len(stack) != 1 {
				sure = false
			}
			for _, p := range pending {
				if top.id > p.frame {
					disagree(intro)
				}
			}
			found := 0
			for next < len(bodies) && bodies[next][0] <= i { // the bodies of one line follow each other
				i = bodies[next][1]
				next++
				found++
			}
			if found != len(pending) {
				disagree(intro)
			}
			pending = pending[:0]
			start, after = true, false
			i--
			continue
		}
		top := &stack[len(stack)-1]
		c := text[i]
		switch top.kind {
		case inSingle:
			if c == '\'' {
				pop()
			}
			continue
		case inANSI:
			if c == '\\' {
				i++
			} else if c == '\'' {
				pop()
			}
			continue
		case inDouble:
			switch {
			case c == '\\':
				i++
			case c == '"':
				pop()
			default:
				if j := expansion(i); j >= 0 {
					i = j
				}
			}
			continue
		case inParam:
			inQuotes := false // inside double quotes, through any ${ } between, up to the nearest command
			for k := len(stack) - 2; k >= 0 && stack[k].kind != inCommand && stack[k].kind != inBacktick; k-- {
				inQuotes = inQuotes || stack[k].kind == inDouble
			}
			switch {
			case c == '\\':
				i++
			case c == '}':
				pop()
			case c == '\'' && inQuotes:
				sure = false // whether it quotes depends on the operator and the shell version
			case c == '\'':
				push(inSingle)
			case c == '"':
				push(inDouble)
			case at(i, "$'") && !inQuotes:
				push(inANSI)
				i++
			default:
				if j := expansion(i); j >= 0 {
					i = j
				}
			}
			continue
		case inArith:
			switch {
			case c == '\\':
				i++
			case c == '(':
				top.depth++
			case c == ')' && top.depth > 0:
				top.depth--
			case c == ')':
				command := top.command
				if !at(i, "))") {
					sure = false // $((cmd) ...) is a subshell in a substitution, not arithmetic
				} else {
					i++
				}
				pop()
				start, after = false, command
			case c == '\'':
				sure = false // arithmetic rejects it, and how the end of the span is then found varies
				push(inSingle)
			case c == '"':
				push(inDouble)
			default:
				if j := expansion(i); j >= 0 {
					i = j
				}
			}
			continue
		}
		// A command context: the script, a $( ) or <( ) inside it, or a backtick body.
		if top.kind == inBacktick && (c == '\'' || c == '"') {
			sure = false // whether a quote counts inside backticks depends on how the shell finds their end
		}
		switch {
		case c == '\\':
			i++
			if i < len(text) && text[i] == '\n' {
				// A backslash-newline joins the lines, so the word goes on; it is no newline token, so bash reads no
				// body here while the lexer starts one on the next line.
				if len(pending) > 0 {
					disagree(pending[0].pos)
				}
				continue
			}
			start, after = false, false
		case c == '\'':
			push(inSingle)
		case c == '"':
			push(inDouble)
		case at(i, "$'"):
			push(inANSI)
			i++
		case c == '`' && top.kind == inBacktick:
			pop()
		case (c == '<' || c == '>') && at(i+1, "("):
			push(inCommand)
			i++
			start, after = true, true
		case c == '(' && start && at(i+1, "("):
			push(inArith)
			stack[len(stack)-1].command = true
			i++
		case c == '(':
			top.parens = append(top.parens, !start)
			start, after = true, true
		case c == ')':
			switch {
			case len(top.parens) > 0:
				top.parens = top.parens[:len(top.parens)-1]
			case top.kind == inCommand && len(stack) > 1:
				pop()
				continue
			}
			start, after = true, true // an unmatched ) at the top level closes a case pattern
		case c == '#':
			if !start && !after {
				break // inside a word
			}
			if after || len(stack) > 1 || top.cond || top.wordParen() {
				sure = false
			}
			for i < len(text) && text[i] != '\n' {
				out[i] = ' '
				i++
			}
			i--
		case c == ' ' || c == '\t' || c == '\n':
			if c == '\n' && len(pending) > 0 && (next >= len(bodies) || bodies[next][0] != i+1) {
				disagree(pending[0].pos) // a heredoc operator whose body the view did not find
				pending = pending[:0]
			}
			if c == '\n' {
				bracket = 0
			}
			start, after = true, false
		case c == ';' || c == '&' || c == '|':
			start, after = true, false
		case c == '<' || c == '>':
			switch {
			case at(i, "<<<"):
				i += 2
			case at(i, "<<"):
				pending = append(pending, pendingHeredoc{pos: i, frame: top.id})
				if top.kind == inBacktick || top.cond || bracket > 0 || top.wordParen() || underArith() {
					disagree(i)
				}
				i++
			}
			start, after = true, true
		default:
			if j := expansion(i); j >= 0 {
				i = j
				continue
			}
			if start {
				switch {
				case keyword(i, "[["):
					top.cond = true
					i++
				case keyword(i, "]]"):
					top.cond = false
					i++
				case keyword(i, "case") && len(stack) > 1:
					sure = false // a case pattern's ) would close the substitution early
				}
			}
			switch {
			case c == '[' && !start: // a [ that starts a word is the test command, not a subscript
				bracket++
			case c == ']' && bracket > 0:
				bracket--
			}
			start, after = false, false
		}
	}
	return string(out), sure && len(stack) == 1 && len(pending) == 0, heredoc
}

// locate returns the 1-based line of raw at or after byte offset from, and the offset to continue from. Line 0
// means raw cannot be located. Children arrive in source order, so the moving offset is what tells repeated identical
// lines apart; it advances one byte past the match, not past all of it, because a substitution child such as the
// command in `echo $(cmd)` is found inside the text of the child that precedes it.
//
// The first non-blank line of raw is looked for as written, then, for a continued command, in the folded view; each
// first from the offset and then from the start.
func (s *scriptLines) locate(from int, raw string) (line, next int) {
	needle := firstNonBlankLine(raw)
	if needle == "" {
		return 0, from
	}
	if from < 0 || from > len(s.text) {
		from = 0
	}
	folded := strings.Join(strings.Fields(raw), " ")
	for _, start := range []int{from, 0} {
		if idx := strings.Index(s.text[start:], needle); idx >= 0 {
			return s.lineAt(start + idx)
		}
		nstart := sort.Search(len(s.back), func(k int) bool { return s.back[k] >= start })
		if idx := strings.Index(s.norm[nstart:], folded); idx >= 0 {
			return s.lineAt(s.back[nstart+idx])
		}
	}
	return 0, from
}

func (s *scriptLines) lineAt(offset int) (line, next int) {
	return sort.SearchInts(s.newlines, offset) + 1, offset + 1
}

// jsonUnescape undoes the escapes of the compact JSON copy that the text scans run over.
var jsonUnescape = strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\n`, "\n", `\t`, "\t")

// ScriptLabelFor returns "path:line" for the inspected script a match most likely came from, and the label holds no
// script content, so a guard can print it where it would otherwise print the matched text. It only chooses which
// label to show; whether a match is script content at all is the caller's decision. The path is rendered like every
// other script path in a message, so control and bidirectional characters in it cannot reach the output.
//
// It matches in three passes: against child invocations with a located line, whose raw text or unquoted words equal
// the match or sit inside it; then against the script texts; and last against a match a rule composed from a child's
// words (such as the executable and one operand) that never appears in the text as written. A composed match on a
// child with no located line yields the path alone. Each pass also tries the match with JSON escapes undone, because
// the text scans of a command run over a JSON-encoded copy.
func (r Result) ScriptLabelFor(match string) (string, bool) {
	needles := matchNeedles(match)
	for _, needle := range needles {
		for i := range r.Invocations {
			inv := &r.Invocations[i]
			if inv.Script == nil || inv.Script.Line == 0 {
				continue
			}
			words := make([]string, len(inv.Words))
			for j, w := range inv.Words {
				words[j] = w.Text
			}
			for _, spelling := range []string{strings.TrimSpace(inv.Raw), strings.Join(words, " ")} {
				if spelling != "" && (spelling == needle || strings.Contains(needle, spelling)) {
					return inv.Script.Location(), true
				}
			}
		}
		for _, rec := range r.Scripts {
			if idx := strings.Index(rec.Text, needle); idx >= 0 {
				return safeScriptPath(rec.Path) + ":" + strconv.Itoa(strings.Count(rec.Text[:idx], "\n")+1), true
			}
		}
	}
	for _, needle := range needles {
		for i := range r.Invocations {
			if inv := &r.Invocations[i]; inv.Script != nil && ComposedFromWords(needle, inv.Words) {
				return inv.Script.Location(), true
			}
		}
	}
	return "", false
}

// Location renders path and line without the calling chain, or the path alone when the line is unknown. The path
// goes through the same rendering as every script path in a message.
func (o *ScriptOrigin) Location() string {
	if o.Line > 0 {
		return safeScriptPath(o.Path) + ":" + strconv.Itoa(o.Line)
	}
	return safeScriptPath(o.Path)
}

// SafePath renders the script's path for a message.
func (r ScriptRecord) SafePath() string { return safeScriptPath(r.Path) }

// ComposedFromWords reports whether needle is some of words joined by single spaces, in order, with the executable
// possibly reduced to its lower-case name: how the rules spell "rm " plus a target. Each word may stand for itself
// in its raw or its unquoted spelling.
func ComposedFromWords(needle string, words []Word) bool {
	rest := needle
	for i, w := range words {
		if rest == "" {
			break
		}
		spellings := []string{w.Raw, w.Text}
		if i == 0 {
			spellings = append(spellings, ExecutableName(w.Text))
		}
		for _, s := range spellings {
			if s == "" || len(rest) < len(s) || !strings.EqualFold(rest[:len(s)], s) {
				continue
			}
			if tail := rest[len(s):]; tail == "" || tail[0] == ' ' {
				rest = strings.TrimPrefix(tail, " ")
				break
			}
		}
	}
	return rest == "" && needle != ""
}

// matchNeedles returns the first non-blank line of match, as given and with JSON escapes undone.
func matchNeedles(match string) []string {
	var out []string
	for _, m := range []string{match, jsonUnescape.Replace(match)} {
		if s := firstNonBlankLine(m); s != "" && (len(out) == 0 || out[len(out)-1] != s) {
			out = append(out, s)
		}
	}
	return out
}
