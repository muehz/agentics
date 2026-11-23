package shellwords

import "strings"

// LineStarts reports, per physical line of command, whether the line
// provably begins as executable shell text: outside every quote, outside a
// heredoc body, and not continuing the previous line. It is the one quote
// tracker the heredoc pre-pass and command-guardian's heredoc strip share
// (#651). Anything it cannot prove is false, and a false line never opens
// a heredoc.
func LineStarts(command string) []bool {
	lines := strings.Split(command, "\n")
	starts := make([]bool, len(lines))
	var st lineState
	for i := 0; i < len(lines); i++ {
		starts[i] = st.clean()
		line := strings.TrimSuffix(lines[i], "\r")
		var docs []heredoc
		if starts[i] {
			docs = heredocsOnLine(line)
		}
		st.scan(line)
		for _, doc := range docs {
			for i+1 < len(lines) {
				i++
				body := strings.TrimSuffix(lines[i], "\r")
				if doc.stripTabs {
					body = strings.TrimLeft(body, "\t")
				}
				if body == doc.delimiter {
					break
				}
			}
		}
	}
	return starts
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
