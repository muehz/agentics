package shellwords

import (
	"strings"
	"testing"
)

func TestQuotedHeredocView(t *testing.T) {
	for _, tc := range []struct {
		name, command               string
		complete, reliable, removed bool
	}{
		{"complete", "cat <<'EOF'\nbody\nEOF\necho later\n", true, true, true},
		{"crlf", "cat <<'EOF'\r\nbody\r\nEOF\r\necho later\r\n", true, true, true},
		{"tabs", "cat <<-'EOF'\n\tbody\n\tEOF\n", true, true, true},
		{"multiple", "cat <<'A' <<'B'\nbody\nA\nbody\nB\necho later", true, true, true},
		{"sequential", "cat <<'A'\nbody\nA\ncat <<'B'\nbody\nB\n", true, true, true},
		{"unterminated", "cat <<'EOF'\nbody\n", false, true, false},
		{"unquoted", "cat <<EOF\nbody $(echo later)\nEOF\n", true, true, false},
		{"quoted fake", "echo \"cat <<'EOF'\"\necho body", true, true, false},
		{"comment fake", "echo x # cat <<'EOF'\necho body", true, true, false},
		{"substitution", "echo \"$(cat <<'EOF'\nbody\nEOF\n)\"", true, false, false},
		{"process substitution", "cat <(cat <<'EOF'\nbody\nEOF\n)", true, false, false},
		{"unsupported intro", "cat <<\nbody", true, false, false},
		// A here-string opens no heredoc: the line is complete, but stays unreliable so callers remain conservative.
		{"here string", "cat <<< body", true, false, false},
		{"here string then heredoc", "cat <<< x\ncat <<'EOF'\nbody\nEOF\necho later\n", true, false, false}, // an unreliable line removes nothing
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := QuotedHeredocView(tc.command)
			if view.Complete != tc.complete || view.Reliable != tc.reliable {
				t.Fatalf("view = %+v", view)
			}
			if tc.removed == strings.Contains(view.Text, "body") {
				t.Fatalf("unexpected body view %q", view.Text)
			}
			if len(view.Text) != len(tc.command) {
				t.Fatal("source offsets changed")
			}
			if strings.Contains(tc.command, "echo later") && !strings.Contains(view.Text, "echo later") {
				t.Fatal("later executable evidence erased")
			}
			if (!tc.complete || !tc.reliable) && view.Text != tc.command {
				t.Fatal("uncertain view changed input")
			}
		})
	}
}

func TestDataRedirections(t *testing.T) {
	redirects, complete := (Invocation{Raw: `cat > "$TMPDIR/draft.md" <<'EOF' 3>&1`, Dialect: POSIX}).OwnRedirects()
	if !complete || len(redirects) != 3 {
		t.Fatalf("redirects=%+v complete=%v", redirects, complete)
	}
	if redirects[0].Operator != ">" || redirects[0].Target.Text != "$TMPDIR/draft.md" || redirects[0].Target.Literal {
		t.Fatalf("output=%+v", redirects[0])
	}
	if redirects[1].Operator != "<<" || !redirects[1].Target.Quoted {
		t.Fatalf("heredoc=%+v", redirects[1])
	}
	if redirects[2].Operator != ">&" || redirects[2].FD != 3 {
		t.Fatalf("descriptor=%+v", redirects[2])
	}
	if _, complete := (Invocation{Raw: "cat >", Dialect: POSIX}).OwnRedirects(); complete {
		t.Fatal("missing target is complete")
	}
	parsed := Parse("Set-Content x -Value y > out.txt", PowerShell)
	if len(parsed.Invocations) != 1 {
		t.Fatalf("invocations = %+v", parsed.Invocations)
	}
	redirects, complete = parsed.Invocations[0].OwnRedirects()
	if !complete || len(redirects) != 1 || redirects[0].Operator != ">" || redirects[0].Target.Text != "out.txt" {
		t.Fatalf("PowerShell redirects=%+v complete=%v", redirects, complete)
	}
}
