// Package asciifold lowers exactly the runes Go's (?i) folds into ASCII
// letters: A-Z, U+017F (LATIN SMALL LETTER LONG S) and U+212A (KELVIN SIGN).
// Contains on the folded text is therefore false only when a case-insensitive
// regexp requiring that literal cannot match -- the invariant every guard
// prefilter needs.
package asciifold

import "unicode/utf8"

// String returns s with ASCII A-Z lowered and the two non-ASCII runes that
// fold into ASCII letters replaced by their ASCII fold. Every other byte is
// copied unchanged, invalid UTF-8 included. It returns s itself when no
// byte changes.
func String(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || c >= utf8.RuneSelf {
			return fold(s, i)
		}
	}
	return s
}

func fold(s string, start int) string {
	out := make([]byte, 0, len(s))
	out = append(out, s[:start]...)
	changed := false
	for i := start; i < len(s); {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			out = append(out, c+('a'-'A'))
			changed = true
			i++
		case c < utf8.RuneSelf:
			out = append(out, c)
			i++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			switch r {
			case 0x017F:
				out = append(out, 's')
				changed = true
			case 0x212A:
				out = append(out, 'k')
				changed = true
			default:
				out = append(out, s[i:i+size]...)
			}
			i += size
		}
	}
	if !changed {
		return s
	}
	return string(out)
}
