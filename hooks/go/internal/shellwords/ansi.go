package shellwords

import (
	"strconv"
	"strings"
)

// ansiCEnd skips a dollar-single-quoted span, including escaped quotes.
// It returns len(s) for an unfinished span so callers cannot inspect its data
// as executable shell syntax.
func ansiCEnd(s string, start int) int {
	end, _ := ansiCEndClosed(s, start)
	return end
}

// ansiCEndClosed skips a dollar-single-quoted span, including escaped quotes,
// and reports whether a closing single quote terminated the span.
func ansiCEndClosed(s string, start int) (int, bool) {
	for i := start + 2; i < len(s); i++ {
		if s[i] == '\\' {
			i++
		} else if s[i] == '\'' {
			return i + 1, true
		}
	}
	return len(s), false
}

// decodeANSI decodes the ASCII subset used by literal command names and
// arguments. Locale-dependent Unicode, NUL truncation, and control escapes
// remain explicit parsing limits instead of guessed shell values. A limit is reported as the refusal of the matching
// ANSI form kind, and the zero Refusal means the span decoded.
func decodeANSI(raw string) (string, Refusal) {
	var out strings.Builder
	for i := 2; i < len(raw); i++ {
		c := raw[i]
		if c == '\'' {
			return out.String(), Refusal{}
		}
		if c != '\\' {
			out.WriteByte(c)
			continue
		}
		i++
		if i >= len(raw) {
			break
		}
		c = raw[i]
		switch c {
		case 'a':
			out.WriteByte('\a')
		case 'b':
			out.WriteByte('\b')
		case 'e', 'E':
			out.WriteByte(27)
		case 'f':
			out.WriteByte('\f')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'v':
			out.WriteByte('\v')
		case '\\', '\'', '"', '?':
			out.WriteByte(c)
		case 'c':
			return "", formRefusal(formANSIControl)
		case 'x', 'u', 'U', '0', '1', '2', '3', '4', '5', '6', '7':
			base, width, start := 16, 2, i+1
			if c >= '0' && c <= '7' {
				base, width, start = 8, 3, i
			} else if c == 'u' {
				width = 4
			} else if c == 'U' {
				width = 8
			}
			end := start
			for end < len(raw) && end-start < width {
				if _, err := strconv.ParseUint(raw[end:end+1], base, 8); err != nil {
					break
				}
				end++
			}
			if end == start {
				out.WriteByte('\\')
				out.WriteByte(c)
				continue
			}
			n, _ := strconv.ParseUint(raw[start:end], base, 32)
			if n == 0 || n > 127 {
				return "", formRefusal(formANSIByte)
			}
			out.WriteByte(byte(n))
			i = end - 1
		default:
			out.WriteByte('\\')
			out.WriteByte(c)
		}
	}
	return "", formRefusal(formANSIUnterminated)
}
