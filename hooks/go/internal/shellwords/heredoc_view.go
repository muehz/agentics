package shellwords

// redirections returns the lexical redirects of one raw invocation, including
// output descriptors omitted from Parse's input-read view. The boolean reports
// whether the existing word lexer recognized a complete segment.
func redirections(raw string, dialect Dialect) ([]Redirection, bool) {
	segment := lexSegment(raw, dialect)
	return segment.redirects, !segment.refusal.Refused()
}

// OwnRedirects returns the redirects written in the invocation's own text, read in its own dialect.
// The redirects of a wrapper, script call or group the invocation runs
// under are not in its text; OutputRedirects carries those that write a file.
//
// A false result means the text is not readable: the lexer did not recognize a complete segment, and the redirects
// returned are partial. It is never the same as "no redirects". Each caller keeps its posture for it: a proof that
// needs the redirects (the deletion and inert-data proofs, the execution context) treats it as "not proven", and a
// reader that only searches for a write target still judges the partial redirects it was given.
func (inv Invocation) OwnRedirects() ([]Redirection, bool) {
	return redirections(inv.Raw, inv.Dialect)
}

// HeredocView describes a lexical data view, not whether its consumer is safe.
// Text blanks only quoted, terminated bodies, preserving source byte positions.
// Complete reports termination; Reliable reports unambiguous introductions and
// source association. Callers must retain the original text when either is false.
type HeredocView struct {
	Text     string
	Complete bool
	Reliable bool
}

// QuotedHeredocView reuses the parser's heredoc lexer without changing Parse.
// Nested substitutions and continued introductions have ambiguous ownership in
// this bounded view and therefore cannot authorize data exemptions.
func QuotedHeredocView(command string) HeredocView {
	h := readHeredocExtents(command)
	result := HeredocView{Text: command, Complete: h.complete(), Reliable: h.reliable()}
	if result.Complete && result.Reliable {
		result.Text = h.blankQuoted(command)
	}
	return result
}
