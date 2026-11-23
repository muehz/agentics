package shellwords

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/muehz/agentics/hooks/go/internal/argv"
)

// ScriptCall is what a guard needs to judge the arguments passed to one expanded script call. Args holds the literal
// arguments in order. A call whose script consumes parameters is refused when any argument is not literal, or when a
// wrapper such as xargs appends arguments, so for such a call Args is every argument the script receives, except that
// an unquoted pattern (PatternArgs) stands for names the shell expands. For a script that consumes no parameters Args
// is informational. UnquotedParams is true when the script expands a positional parameter outside double
// quotes, so word splitting can turn one argument into several.
type ScriptCall struct {
	Path           string   `json:"path"`
	Args           []string `json:"args,omitempty"`
	ConsumesParams bool     `json:"consumesParams,omitempty"`
	UnquotedParams bool     `json:"unquotedParams,omitempty"`
	// PatternArgs is true when an argument is an unquoted glob, brace or tilde word: the script receives the expanded
	// names, which Args does not show.
	PatternArgs bool `json:"patternArgs,omitempty"`
	// OutputRedirects are the write redirects on the call and on the wrapper or script calls it runs under, filtered as
	// Invocation.OutputRedirects is. The script's invocations carry them too, but a script that runs no command leaves
	// no invocation, and the shell still opens, and may truncate, the target. Kept out of the encoded form.
	OutputRedirects []Redirection `json:"-"`
}

// CallerWords returns the words a guard classifies for this call: each argument whole and, when the script expands a
// positional parameter outside double quotes, each whitespace-separated piece as well, because word splitting turns
// one argument into several there.
func (c ScriptCall) CallerWords() []string {
	out := append([]string(nil), c.Args...)
	if c.UnquotedParams {
		for _, a := range c.Args {
			out = append(out, strings.Fields(a)...)
		}
	}
	return out
}

// forwarder is a whole word that is one positional parameter or the whole argument vector: $1, "$1", ${1}, $@, "$@".
var forwarder = regexp.MustCompile(`^"?\$(?:@|\*|[1-9]|\{[1-9@*][^}]*\})"?$`)

// forwardsParameters reports a verb CLI whose verb position holds a caller parameter: the caller picks the operation.
// A parameter after the verb is an operand and is judged by what it names. An option before the verb hides the verb
// from this rule, which only reads the word next to the command; git is covered by the operand rule as well.
func forwardsParameters(inv *Invocation) bool {
	words := stripBuiltin(inv.Words)
	if len(words) < 2 || !words[0].Literal || !argv.Lookup(strings.ToLower(ExecutableName(words[0].Text))).VerbCLI {
		return false
	}
	return !words[1].Literal && forwarder.MatchString(words[1].Raw)
}

// stdinReader finds a command that reads the script's standard input: read, mapfile, readarray and a bare cat. It
// reads the text of the script, so a word in a string can match; it is consulted only for a call that is fed. xargs
// reads its input list from the same stream.
var stdinReader = regexp.MustCompile(`(?m)(?:^|[\s;&|(){])(?:read|mapfile|readarray|xargs)(?:[\s;&|)]|$)|(?:^|[\s;&|(){])cat[ \t]*(?:$|[;&|)])`)

// argFrame is one open context of the parameter scan.
type argFrame struct {
	kind   int
	parens int  // the open ( inside a command context
	dbl    bool // inside [[ ]], where the shell does not split words
	subj   bool // between case and in, where the subject is not split
}

// declarationBuiltins take assignment words whose value is not split.
var declarationBuiltins = map[string]bool{"local": true, "export": true, "declare": true, "readonly": true, "typeset": true}

var assignmentWord = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\+?=`)

// atCommandPosition reports whether the word at text[i:] starts a command: it follows a separator, the start of the
// text, or a keyword that introduces a command.
func atCommandPosition(text string, i int) bool {
	j := i
	for j > 0 && (text[j-1] == ' ' || text[j-1] == '\t') {
		j--
	}
	if j == 0 || strings.IndexByte(";&|\n({", text[j-1]) >= 0 {
		return true
	}
	k := j
	for k > 0 && (text[k-1] >= 'a' && text[k-1] <= 'z' || text[k-1] == '!') {
		k--
	}
	switch text[k:j] {
	case "if", "elif", "while", "until", "then", "do", "else", "!":
		return k == 0 || strings.IndexByte(" \t;&|\n({", text[k-1]) >= 0
	}
	return false
}

// assignScan answers, for the successive $ of one text, whether it sits in the value of an assignment word that bash
// does not split, in time linear in the text. The question looks back from a $ to the start of its word and then to the
// start of its command, and asking it afresh for every $ makes a long line of assignments quadratic. The scan instead
// keeps what it learned about the words of the command, so each byte is read once. It must be asked about the $
// of the text, in increasing order.
type assignScan struct {
	text string
	// wordPos has read text[:wordPos] to find the start of the word that ends at the next $: one past the last byte
	// that ends a word.
	wordPos, wordStart int
	// fieldPos has read text[:fieldPos] as the words of the command that contains it. cmdStart is where that command
	// begins; the words are those of text[cmdStart:fieldPos].
	fieldPos, cmdStart int
	words              int  // the words that started in the command so far, the open one included
	inWord             bool // a word is open at fieldPos
	wordFrom           int  // where the open word starts
	wordBad            bool // the open word holds a quote, backtick or backslash
	allowed            bool // every closed word so far is an assignment, a declaration builtin or one of its options
	declared           bool // the first word is a declaration builtin
	options            bool // still among the options that may follow a declaration builtin
	// Whether the word that starts at tokenAt, and the one that starts at wordAt, begin like an assignment.
	tokenAt, wordAt   int
	tokenOK, wordHead bool
}

func newAssignScan(text string) *assignScan {
	return &assignScan{text: text, allowed: true, tokenAt: -1, wordAt: -1}
}

// assignPrefix reports whether text[from:end] begins like NAME= or NAME+=, as assignmentWord does.
func assignPrefix(text string, from, end int) bool {
	j := from
	if j >= end || !(text[j] == '_' || text[j] >= 'a' && text[j] <= 'z' || text[j] >= 'A' && text[j] <= 'Z') {
		return false
	}
	for j < end && (text[j] == '_' || text[j] >= 'a' && text[j] <= 'z' || text[j] >= 'A' && text[j] <= 'Z' || text[j] >= '0' && text[j] <= '9') {
		j++
	}
	if j < end && text[j] == '+' {
		j++
	}
	return j < end && text[j] == '='
}

// wordOK reports whether the word text[from:end] is acceptable given the words before it, and the state after it.
func (s *assignScan) wordOK(from, end int, bad bool) (ok, declared, options bool) {
	if s.words == 1 && end-from <= 8 && declarationBuiltins[s.text[from:end]] {
		return true, true, true
	}
	if s.declared && s.options && s.text[from] == '-' {
		return true, s.declared, true
	}
	if s.wordAt != from {
		s.wordAt, s.wordHead = from, assignPrefix(s.text, from, end)
	}
	return s.wordHead && !bad, s.declared, false
}

// advance reads text[fieldPos:to], which ends at a byte that is not part of a rune.
func (s *assignScan) advance(to int) {
	for s.fieldPos < to {
		c := s.text[s.fieldPos]
		size := 1
		space := false
		switch {
		case c < 0x80:
			space = c == ' ' || c >= '\t' && c <= '\r'
		default:
			var r rune
			r, size = utf8.DecodeRuneInString(s.text[s.fieldPos:])
			space = unicode.IsSpace(r)
		}
		if size == 1 && strings.IndexByte(";&|\n({", c) >= 0 {
			s.fieldPos++
			s.cmdStart, s.words, s.inWord, s.allowed, s.declared, s.options = s.fieldPos, 0, false, true, false, false
			continue
		}
		if space {
			if s.inWord {
				ok, declared, options := s.wordOK(s.wordFrom, s.fieldPos, s.wordBad)
				s.allowed, s.declared, s.options, s.inWord = s.allowed && ok, declared, options, false
			}
		} else {
			if !s.inWord {
				s.inWord, s.wordFrom, s.wordBad = true, s.fieldPos, false
				s.words++
			}
			if size == 1 && strings.IndexByte("\"'`\\", c) >= 0 {
				s.wordBad = true
			}
		}
		s.fieldPos += size
	}
}

// splitFreeAt reports whether the $ at text[i] sits in the value of an assignment word that bash does not split:
// NAME=value as a statement, a prefix assignment, or an argument of a declaration builtin. The words before it in the
// same command must all be assignments, or a declaration builtin and its options; anything else, including a quote
// that may hide a separator, reads as a splitting position.
func (s *assignScan) splitFreeAt(i int) bool {
	for ; s.wordPos < i; s.wordPos++ {
		if strings.IndexByte(" \t\n;&|(){}\"'`", s.text[s.wordPos]) >= 0 {
			s.wordStart = s.wordPos + 1
		}
	}
	ws := s.wordStart
	if s.tokenAt != ws {
		s.tokenAt, s.tokenOK = ws, assignPrefix(s.text, ws, i)
	}
	if !s.tokenOK {
		return false
	}
	s.advance(ws)
	if !s.allowed {
		return false
	}
	if s.inWord {
		ok, _, _ := s.wordOK(s.wordFrom, ws, s.wordBad)
		return ok
	}
	return true
}

const (
	argCommand = iota
	argDouble
	argSingle
	argANSI
	argBacktick
)

// scanParams reports whether text uses a positional parameter outside single quotes (consumes) and whether it expands
// one outside double quotes, where word splitting applies (unquoted). Quote state runs across lines, and the quotes
// inside $( ) start a fresh context, so "$(clean "$2")" counts as quoted. A comment hides its words. When a context
// is still open at the end, or a heredoc has no closing line, the scan cannot vouch for its reading, and the answer is
// the stricter one. A heredoc body is data: its quotes change nothing, and a parameter in an unquoted body is expanded
// without word splitting.
func scanParams(text string) (consumes, unquoted bool) {
	stack := []argFrame{{kind: argCommand}}
	wordStart := true
	var pending []heredoc
	assign := newAssignScan(text)
	for i := 0; i < len(text); i++ {
		c := text[i]
		top := &stack[len(stack)-1]
		if c == '\n' && len(pending) > 0 && top.kind == argCommand {
			next, used, ok := heredocBodiesUse(text, i+1, pending)
			if !ok {
				return true, true
			}
			consumes = consumes || used
			pending, i, wordStart = nil, next-1, true
			continue
		}
		if c == '<' && i+1 < len(text) && text[i+1] == '<' && top.kind == argCommand {
			if i+2 < len(text) && text[i+2] == '<' {
				i, wordStart = i+2, false // a here-string
				continue
			}
			if doc, end := readHeredocOperator(text, i, identifierDelimiters); end > 0 {
				pending = append(pending, doc)
				i, wordStart = end-1, false
				continue
			}
		}
		switch top.kind {
		case argSingle:
			if c == '\'' {
				stack = stack[:len(stack)-1]
			}
			continue
		case argANSI:
			if c == '\\' {
				i++
			} else if c == '\'' {
				stack = stack[:len(stack)-1]
			}
			continue
		}
		quoted := top.kind == argDouble
		if wordStart && top.kind == argCommand {
			switch {
			case hasWordAt(text[i:], "[[") && atCommandPosition(text, i):
				top.dbl = true
			case hasWordAt(text[i:], "]]"):
				top.dbl = false
			case hasWordAt(text[i:], "case") && atCommandPosition(text, i):
				top.subj = true
			case hasWordAt(text[i:], "in"):
				top.subj = false
			}
		}
		next := byte(0)
		if i+1 < len(text) {
			next = text[i+1]
		}
		switch {
		case c == '\\':
			i++
			wordStart = false
			continue
		case c == '\'' && !quoted:
			stack = append(stack, argFrame{kind: argSingle})
		case c == '"':
			if quoted {
				stack = stack[:len(stack)-1]
			} else {
				stack = append(stack, argFrame{kind: argDouble})
			}
		case c == '`':
			if top.kind == argBacktick {
				stack = stack[:len(stack)-1]
			} else {
				stack = append(stack, argFrame{kind: argBacktick})
			}
		case c == '$' && next == '\'' && !quoted:
			stack = append(stack, argFrame{kind: argANSI})
			i++
		case c == '$' && next == '(':
			stack = append(stack, argFrame{kind: argCommand})
			i++
			wordStart = true
			continue
		case c == '$':
			use, splits := paramAt(text[i+1:])
			consumes = consumes || use
			unquoted = unquoted || use && splits && !quoted && !top.dbl && !top.subj && !assign.splitFreeAt(i)
		case !quoted && c == '#' && wordStart:
			for i < len(text) && text[i] != '\n' {
				i++
			}
			wordStart = true
			continue
		case !quoted && top.kind == argCommand && c == '(':
			top.parens++
		case !quoted && top.kind == argCommand && c == ')':
			if top.parens > 0 {
				top.parens--
			} else if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case wordStart && !quoted && (hasWordAt(text[i:], "shift") || hasWordAt(text[i:], "getopts") || loopsOverParams(text[i:])):
			consumes = true
		}
		wordStart = !quoted && strings.IndexByte(" \t\n\r;&|({", c) >= 0
	}
	if len(stack) > 1 || len(pending) > 0 {
		return true, true
	}
	return consumes, unquoted
}

// heredocBodiesUse steps over the bodies of docs, which start at from, as the parameter scan reads them, and returns
// the offset after the last body. used reports a positional parameter on a body line of an unquoted doc; a closing line
// is not a body line. ok is false when a body has no closing line, and the scan then gives its strict answer.
func heredocBodiesUse(text string, from int, docs []heredoc) (next int, used, ok bool) {
	spans, next, ok := walkHeredocBodies(text, from, docs, identifierDelimiters)
	for k, doc := range docs {
		if doc.quoted {
			continue
		}
		e := heredocExtent{bodyStart: spans[k][0], bodyEnd: spans[k][1], terminated: true}
		content := e.contentEnd(text)
		// When some body did not close, a span may end without a closing line. The walk stops at the first line equal
		// to the delimiter, so a span closed exactly when its last line is one.
		if last := strings.TrimSuffix(text[content:e.bodyEnd], "\n"); !ok && !doc.closedBy(last, identifierDelimiters) {
			content = e.bodyEnd
		}
		for _, line := range strings.Split(text[e.bodyStart:content], "\n") {
			used = used || lineUsesParam(line)
		}
	}
	return next, used, ok
}

// lineUsesParam reports a positional parameter or the count after any $ in line.
func lineUsesParam(line string) bool {
	for k := strings.IndexByte(line, '$'); k >= 0; {
		if use, _ := paramAt(line[k+1:]); use {
			return true
		}
		n := strings.IndexByte(line[k+1:], '$')
		if n < 0 {
			break
		}
		k += 1 + n
	}
	return false
}

// hasWordAt reports whether text starts with word followed by something that ends a word.
func hasWordAt(text, word string) bool {
	return strings.HasPrefix(text, word) && (len(text) == len(word) || strings.IndexByte(" \t\n\r;&|)}", text[len(word)]) >= 0)
}

// paramAt reads what follows a $: use is true for a positional parameter or the count, splits for the forms whose
// unquoted expansion is split into words.
func paramAt(rest string) (use, splits bool) {
	if rest == "" {
		return false, false
	}
	switch c := rest[0]; {
	case c >= '1' && c <= '9', c == '@', c == '*':
		return true, true
	case c == '#':
		return true, false
	case c == '{' && len(rest) > 1:
		switch d := rest[1]; {
		case d >= '1' && d <= '9', d == '@', d == '*':
			return true, true
		case d == '!':
			return true, false
		case d == '#' && len(rest) > 2 && (rest[2] == '}' || rest[2] == '@' || rest[2] == '*' || rest[2] >= '1' && rest[2] <= '9'):
			return true, false
		}
	}
	return false, false
}

// rawRedirectsStdin reports whether a command's text redirects standard input: <, <>, a heredoc or a here-string.
func rawRedirectsStdin(raw string, dialect Dialect) bool {
	redirects, _ := redirections(raw, dialect)
	for _, r := range redirects {
		switch r.Operator {
		case "<", "<>", "<<", "<<-", "<<<":
			return true
		}
	}
	return false
}

// enterCall notes the prefix assignments and the stdin feed of a wrapper or script call for the script call it may
// reach, and returns the function that takes them back. The prefix of every wrapper around a call acts on the call,
// and a feed given to a wrapper reaches the shell it starts.
func (p *parser) enterCall(assignments []Word, fed bool) func() {
	n, was := len(p.script.callAssignments), p.script.wrapperFed
	p.script.callAssignments = append(p.script.callAssignments, assignments...)
	p.script.wrapperFed = p.script.wrapperFed || fed
	return func() {
		p.script.callAssignments = p.script.callAssignments[:n]
		p.script.wrapperFed = was
	}
}

// ownBefore returns the separator the current segment has in its own list. The one parseSegment is given is inherited
// by the first command of a script or a -c string from the call that started it, so it says nothing about whether the
// command itself follows a pipe.
func (p *parser) ownBefore(before Separator) Separator {
	if n := len(p.lists); n > 0 && len(p.lists[n-1].segments) > 0 {
		return p.lists[n-1].segments[len(p.lists[n-1].segments)-1].before
	}
	return before
}

// markCallerArguments refuses, among the commands from the list own, each that hands a caller parameter to a verb CLI
// as its verb. The label is the child's own location.
func (p *parser) markCallerArguments(own []int) {
	for _, i := range own {
		inv := &p.result.Invocations[i]
		if inv.Inspectable && forwardsParameters(inv) {
			inv.Inspectable = false
			inv.Reason = ruleRefusal(RuleCallerArgument, inv.Script)
		}
	}
}

// loopsOverParams reports a `for NAME` or `select NAME` loop with no `in` list at the start of text: it runs over the
// positional parameters without writing a $.
func loopsOverParams(text string) bool {
	rest := ""
	switch {
	case hasWordAt(text, "for"):
		rest = text[len("for"):]
	case hasWordAt(text, "select"):
		rest = text[len("select"):]
	default:
		return false
	}
	rest = strings.TrimLeft(rest, " \t")
	n := 0
	for n < len(rest) && (rest[n] == '_' || rest[n] >= 'a' && rest[n] <= 'z' || rest[n] >= 'A' && rest[n] <= 'Z' || n > 0 && rest[n] >= '0' && rest[n] <= '9') {
		n++
	}
	if n == 0 {
		return false
	}
	rest = strings.TrimLeft(rest[n:], " \t")
	return strings.HasPrefix(rest, ";") || hasWordAt(strings.TrimLeft(rest, " \t\r\n"), "do")
}
