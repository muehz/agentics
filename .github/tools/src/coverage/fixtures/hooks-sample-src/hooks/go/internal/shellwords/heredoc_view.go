package shellwords

import "strings"

// Redirections returns the lexical redirects of one raw invocation, including
// output descriptors omitted from Parse's input-read view. The boolean reports
// whether the existing word lexer recognized a complete segment.
func Redirections(raw string, dialect Dialect) ([]Redirection, bool) {
	segment := lexSegment(raw, dialect)
	return segment.redirects, segment.malformedMsg == ""
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
	result, blanked, _ := quotedHeredocView(command)
	if result.Complete && result.Reliable {
		result.Text = blanked
	}
	return result
}

// oddHeredocLine returns the 1-based line of the first heredoc introduction in a script where the parser's lexer may
// end the body on another line than bash, or 0: an introduction whose delimiter word is not plain. A carriage return
// is refused by the parser before this runs, so the end line is compared the way the parser strips bodies. Where the
// body starts is the comment tracker's question, which is why an operator on a line the lexer does not start a command
// on is not one of the cases here: the tracker counts that operator and finds no body for it. The lines are walked
// the way the parser strips heredoc bodies, so the first such line is where the parser's reading and bash's first part.
func oddHeredocLine(source string) int {
	if !strings.Contains(source, "<<") {
		return 0
	}
	lines := strings.SplitAfter(source, "\n")
	starts := LineStarts(source)
	for i := 0; i < len(lines); i++ {
		if i >= len(starts) || !starts[i] {
			continue
		}
		docs, _, odd := inspectHeredocsOnLine(strings.TrimSuffix(lines[i], "\n"))
		if odd {
			return i + 1
		}
		for _, doc := range docs {
			for i+1 < len(lines) {
				i++
				raw := strings.TrimSuffix(lines[i], "\n")
				if doc.stripTabs {
					raw = strings.TrimLeft(raw, "\t")
				}
				if strings.TrimSuffix(raw, "\r") == doc.delimiter {
					break
				}
			}
		}
	}
	return 0
}

// quotedHeredocView does the lexing for QuotedHeredocView and also returns the text with every quoted, terminated
// body blanked whether or not the view is reliable, for a caller that only needs positions and not an inert proof.
// bodies holds the byte span [start, end) of every body, quoted or not, delimiter line included; an unterminated body
// runs to the end of the text, as it does for bash and for the parser.
func quotedHeredocView(command string) (HeredocView, string, [][2]int) {
	var bodies [][2]int
	result := HeredocView{Text: command, Complete: true, Reliable: true}
	lines := strings.SplitAfter(command, "\n")
	starts := LineStarts(command)
	out := []byte(command)
	offset := 0
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		docs, reliable, _ := inspectHeredocsOnLine(strings.TrimSuffix(line, "\n"))
		if !reliable {
			result.Reliable = false
		}
		if i >= len(starts) || !starts[i] {
			if len(docs) != 0 {
				result.Reliable = false
			}
			docs = nil
		}
		if len(docs) > 0 && (strings.ContainsAny(line, "`()") || strings.HasSuffix(strings.TrimSuffix(line, "\n"), "\\")) {
			result.Reliable = false
		}
		offset += len(line)
		for _, doc := range docs {
			terminated := false
			bodyStart := offset
			for i+1 < len(lines) {
				i++
				body := strings.TrimSuffix(strings.TrimSuffix(lines[i], "\n"), "\r")
				if doc.stripTabs {
					body = strings.TrimLeft(body, "\t")
				}
				if body == doc.delimiter {
					terminated = true
					offset += len(lines[i])
					break
				}
				if doc.quoted {
					for j := offset; j < offset+len(lines[i]); j++ {
						if out[j] != '\n' && out[j] != '\r' {
							out[j] = ' '
						}
					}
				}
				offset += len(lines[i])
			}
			bodies = append(bodies, [2]int{bodyStart, offset})
			if !terminated {
				result.Complete = false
			}
		}
	}
	return result, string(out), bodies
}
