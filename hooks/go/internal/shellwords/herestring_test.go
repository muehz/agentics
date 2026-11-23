package shellwords

import "testing"

func TestHereStringIsNotAHeredoc(t *testing.T) {
	for _, line := range []string{
		`cat <<< "x"`, `cat <<< x`, `cat <<<"x"`, `read -r a <<< "$1" || true`, `cat <<< "x" <<< "y"`, `cat <<< $(echo x)`,
	} {
		if spans := readHeredocExtents(line).spans(); len(spans) != 0 {
			t.Errorf("%q: here-string read as heredoc with bodies %v", line, spans)
		}
	}
}

func TestHereStringDoesNotHideTheNextLines(t *testing.T) {
	for _, command := range []string{"cat <<< \"x\"\nrm -rf /", "cat <<< x\nrm -rf /", "read -r a <<< \"$1\" || true\nrm -rf /\n"} {
		if got := stripHeredocBodies(command); got != command {
			t.Errorf("%q: stripHeredocBodies changed the text: %q", command, got)
		}
		starts := LineStarts(command)
		if len(starts) < 2 || !starts[1] {
			t.Errorf("%q: the second line is not an executable line start: %v", command, starts)
		}
		found := false
		for _, inv := range Parse(command, POSIX).Invocations {
			found = found || (len(inv.Words) > 0 && ExecutableName(inv.Words[0].Text) == "rm")
		}
		if !found {
			t.Errorf("%q: the rm invocation after the here-string was not parsed", command)
		}
	}
}

// A real heredoc after a here-string still has its body treated as data, and the command after the terminator is
// still parsed.
func TestHeredocAfterAHereStringStillStripsItsBody(t *testing.T) {
	command := "cat <<< \"x\"\ncat <<EOF\nrm -rf /tmp/data\nEOF\ngit status\n"
	stripped := stripHeredocBodies(command)
	if stripped == command {
		t.Fatalf("the heredoc body was not stripped: %q", stripped)
	}
	var exes []string
	for _, inv := range Parse(command, POSIX).Invocations {
		if len(inv.Words) > 0 {
			exes = append(exes, ExecutableName(inv.Words[0].Text))
		}
	}
	for _, e := range exes {
		if e == "rm" {
			t.Errorf("heredoc body parsed as a command: %v", exes)
		}
	}
	if len(exes) == 0 || exes[len(exes)-1] != "git" {
		t.Errorf("command after the heredoc terminator was not parsed: %v", exes)
	}
}
