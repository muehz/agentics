package flow

import (
	"bytes"
	"strings"
	"testing"
)

func TestArtifactLines(t *testing.T) {
	line := func(text string, fenced, section bool) artifactLine {
		return artifactLine{text: []byte(text), fenced: fenced, section: section}
	}
	hundredKB := make([]artifactLine, 0, 10001)
	for range 10000 {
		hundredKB = append(hundredKB, line("`````````", true, false))
	}
	hundredKB = append(hundredKB, line("", false, false))

	tests := []struct {
		name string
		body string
		want []artifactLine
	}{
		{"crlf", "## x\r\ny\r\n", []artifactLine{line("## x", false, true), line("y", false, false), line("", false, false)}},
		{"trailing blanks and tabs", "a \t\nb\t \n", []artifactLine{line("a", false, false), line("b", false, false), line("", false, false)}},
		{"backtick and tilde fences opened and closed", "```go\nx\n```\ny\n~~~\nz\n~~~\nw", []artifactLine{
			line("```go", true, false), line("x", true, false), line("```", true, false), line("y", false, false),
			line("~~~", true, false), line("z", true, false), line("~~~", true, false), line("w", false, false),
		}},
		{"longer closing run", "```\nx\n````\ny", []artifactLine{
			line("```", true, false), line("x", true, false), line("````", true, false), line("y", false, false),
		}},
		{"closing run followed by text stays open", "```\nx\n``` y\nz", []artifactLine{
			line("```", true, false), line("x", true, false), line("``` y", true, false), line("z", true, false),
		}},
		{"backtick in a backtick info string opens no fence", "``` a`b\n## x", []artifactLine{
			line("``` a`b", false, false), line("## x", false, true),
		}},
		{"unclosed fence runs to the end", "```\na\n## b", []artifactLine{
			line("```", true, false), line("a", true, false), line("## b", true, false),
		}},
		{"heading inside a fence is no section", "```\n## x\n```\n## y", []artifactLine{
			line("```", true, false), line("## x", true, false), line("```", true, false), line("## y", false, true),
		}},
		{"heading followed only by blanks", "## \t \n", []artifactLine{line("##", false, true), line("", false, false)}},
		{"indented heading is no section", "  ## x", []artifactLine{line("  ## x", false, false)}},
		{"no final newline", "a\n## b", []artifactLine{line("a", false, false), line("## b", false, true)}},
		{"about 100 KB of fence lines in one pass", strings.Repeat("`````````\n", 10000), hundredKB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := artifactLines([]byte(tt.body))
			if len(got) != len(tt.want) {
				t.Fatalf("artifactLines gave %d lines, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if !bytes.Equal(got[i].text, tt.want[i].text) || got[i].fenced != tt.want[i].fenced || got[i].section != tt.want[i].section {
					t.Errorf("line %d = {%q fenced=%t section=%t}, want {%q fenced=%t section=%t}", i,
						got[i].text, got[i].fenced, got[i].section, tt.want[i].text, tt.want[i].fenced, tt.want[i].section)
				}
			}
		})
	}
}
