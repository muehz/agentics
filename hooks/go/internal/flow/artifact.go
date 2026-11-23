package flow

import "bytes"

// artifactLine is one line of an artifact body as artifactLines reads it.
type artifactLine struct {
	text            []byte
	fenced, section bool
}

// artifactLines reads body once into the lines that hashing and entry reading
// share. Lines split on LF, so a body with a final newline ends in one empty
// line. text is the line without its CR, trailing blanks, or tabs. fenced is
// set for a line inside a fenced code block or delimiting one, as codeFence.step
// reads text. section is set for a line that is not fenced and starts "## "
// as written, before the trim, so "## " followed only by blanks is a section
// whose text is "##" while an indented "## x" is none.
func artifactLines(body []byte) []artifactLine {
	raw := bytes.Split(body, []byte{'\n'})
	lines := make([]artifactLine, len(raw))
	var fence codeFence
	for i, line := range raw {
		text := bytes.TrimRight(bytes.TrimSuffix(line, []byte{'\r'}), " \t")
		fenced := fence.step(text)
		lines[i] = artifactLine{
			text:    text,
			fenced:  fenced,
			section: !fenced && bytes.HasPrefix(line, []byte("## ")),
		}
	}
	return lines
}

// codeFence follows the fenced code blocks of a Markdown file line by line.
// The zero value is outside any block. Indentation before a fence is not
// limited, so that a block inside a list item counts too; a block left open
// runs to the end of the file, as Markdown renders it.
type codeFence struct {
	char byte
	size int
}

// step reads the next line, with trailing blanks removed, and reports whether
// it lies inside a fenced code block or delimits one.
func (f *codeFence) step(line []byte) bool {
	text := bytes.TrimLeft(line, " \t")
	run := 0
	for run < len(text) && (text[0] == '`' || text[0] == '~') && text[run] == text[0] {
		run++
	}
	if f.size > 0 {
		// Only a run of the opening character, at least as long and with
		// nothing after it, closes the block.
		if run >= f.size && text[0] == f.char && run == len(text) {
			*f = codeFence{}
		}
		return true
	}
	// A backtick fence's info string cannot hold a backtick, or the line would be inline code.
	if run < 3 || (text[0] == '`' && bytes.IndexByte(text[run:], '`') >= 0) {
		return false
	}
	*f = codeFence{text[0], run}
	return true
}
