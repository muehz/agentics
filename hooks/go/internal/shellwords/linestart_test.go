package shellwords

import (
	"reflect"
	"testing"
)

func TestQuotedLineCannotOpenHeredoc(t *testing.T) {
	destructive := "rm -" + "rf /tmp/t"
	for _, intro := range []string{"git commit -F - <<'EOF'", "cat <<'EOF'", "tee note <<'EOF'", "cat <<EOF"} {
		command := "x=\"\n" + intro + "\n\"\n" + destructive + "\nEOF\n"
		found := false
		for _, inv := range Parse(command, POSIX).Invocations {
			if len(inv.Words) > 0 && inv.Words[0].Text == "rm" {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: rm hidden by a heredoc opened inside a quoted string", intro)
		}
	}
}

func TestLineStarts(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    []bool
	}{
		{"a\nb", []bool{true, true}},
		{"x=\"\nb\"\nc", []bool{true, false, true}},
		{"x='\nb'\nc", []bool{true, false, true}},
		{"x=$'\nb'\nc", []bool{true, false, true}},
		{"a \\\nb\nc", []bool{true, false, true}},
		{"# it's\nb", []bool{true, true}},
		{"cat <<'E'\n\"\nE\nc", []bool{true, false, false, true}},
		{"x=\"$(\necho)\"\nc", []bool{true, false, true}},
		{"x=\"", []bool{true}},
	} {
		if got := LineStarts(tc.command); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("LineStarts(%q) = %v, want %v", tc.command, got, tc.want)
		}
	}
}
