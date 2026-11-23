package commandguard

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

const commitHeredocMarker = "MARKER_BODY_LINE"

// commitHeredocScan builds the combined scan body for a Bash command and
// strips it with the command's own parse, as the guard does.
func commitHeredocScan(t *testing.T, command string) string {
	t.Helper()
	doc, err := encodeCompact(map[string]string{"command": command})
	if err != nil {
		t.Fatal(err)
	}
	invocations := shellwords.Parse(command, shellwords.POSIX).Invocations
	return stripHeredocsFor("Bash "+doc, command, invocations, true)
}

func TestCommitHeredocStrip(t *testing.T) {
	body := commitHeredocMarker + "\n"
	for _, tc := range []struct {
		name, command string
		stripped      bool
	}{
		{"stdin -F -", "git commit -F - <<'EOF'\n" + body + "EOF", true},
		{"stdin --file=-", "git commit --file=- <<'EOF'\n" + body + "EOF", true},
		{"stdin --file -", "git commit --file - <<'EOF'\n" + body + "EOF", true},
		{"stdin after flags", "git commit -s --no-verify -F - <<'EOF'\n" + body + "EOF", true},
		{"stdin after &&", "git add -A && git commit -F - <<'EOF'\n" + body + "EOF", true},
		{"message substitution", "git commit -m \"$(cat <<'EOF'\n" + body + "EOF\n)\"", true},
		{"message= substitution", "git commit --message=\"$(cat <<'EOF'\n" + body + "EOF\n)\"", true},
		{"unquoted delimiter", "git commit -F - <<EOF\n" + body + "EOF", false},
		{"sudo wrapper", "sudo git commit -F - <<'EOF'\n" + body + "EOF", false},
		{"env prefix", "env X=1 git commit -F - <<'EOF'\n" + body + "EOF", false},
		{"bash -c wrapper", "bash -c \"x; git commit -F - <<'EOF'\n" + body + "EOF\n\"", false},
		{"comment before intro", "echo hi # ; git commit -F - <<'EOF'\n" + body + "EOF", false},
		{"text after delimiter", "git commit -F - <<'EOF' | sh\n" + body + "EOF", false},
		{"substitution not closed by )\"", "git commit -m \"$(cat <<'EOF'\n" + body + "EOF\n; true)\"", false},
		{"substitution in command position", "$(cat <<'EOF'\n" + body + "EOF\n)", false},
		{"unterminated stdin body", "git commit -F - <<'EOF'\n" + body, false},
		{"fake intro in echo string beside a real commit", "echo \"a; git commit -F - <<'EOF'\n" + body + "EOF\n\"; git commit -F - <<'X'\nmsg\nX", false},
		{"same-line cat and commit heredocs", "cat <<'A' > p.sh; git commit -F - <<'B'\n" + body + "A\nmsg\nB", false},
		{"hash earlier on the intro line", "x#y; git commit -F - <<'EOF'\n" + body + "EOF", false},
		{"unrelated stdin commit cannot pay for a fake intro", "git commit -F -; echo \"x; git commit -F - <<'EOF'\n\"; " + body + "EOF", false},
		{"stdin commit from a file cannot pay for a fake intro", "git commit -F - < m.txt; x=\"\ngit commit -F - <<'EOF'\n\"; " + body + "EOF", false},
		{"delimiter must match the real commit's heredoc", "git commit -F - <<'A'\nmsg\nA\necho \"x; git commit -F - <<'EOF'\n\"; " + body + "EOF", false},
		{"literal backslash-n in the message still strips", "git commit -F - <<'EOF'\n" + commitHeredocMarker + "a\\nb\nEOF", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := commitHeredocScan(t, tc.command)
			if present := strings.Contains(got, commitHeredocMarker); present == tc.stripped {
				t.Fatalf("stripped=%v, want %v: %q", !present, tc.stripped, got)
			}
		})
	}
}

// TestGhRulesKeepCatBodyBesideCommitHeredoc: a command holding both an
// eligible commit heredoc and a cat/tee heredoc must let the gh rules keep
// seeing the cat/tee body — text a cat/tee heredoc writes to a file can be
// executed later in the same command (bash p.sh) — while the commit message
// body is still safe to strip. The full bank strip (catTee=true) removes
// both bodies; the gh-only strip (catTee=false) removes only the commit body.
func TestGhRulesKeepCatBodyBesideCommitHeredoc(t *testing.T) {
	const commitMarker = "COMMIT_MARKER"
	gatedVerb := "gh pr " + "create"
	command := "git commit -F - <<'A'\n" + commitMarker + "\nA\n" +
		"cat > p.sh <<'B'\n" + gatedVerb + "\nB\n" +
		"bash p.sh"

	doc, err := encodeCompact(map[string]string{"command": command})
	if err != nil {
		t.Fatal(err)
	}
	combined := "Bash " + doc
	invocations := shellwords.Parse(command, shellwords.POSIX).Invocations

	ghOnly := stripHeredocsFor(combined, command, invocations, false)
	if !strings.Contains(ghOnly, gatedVerb) {
		t.Fatalf("gh-only strip removed the cat/tee body: %q", ghOnly)
	}
	if strings.Contains(ghOnly, commitMarker) {
		t.Fatalf("gh-only strip kept the commit body: %q", ghOnly)
	}

	full := stripHeredocsFor(combined, command, invocations, true)
	if strings.Contains(full, gatedVerb) {
		t.Fatalf("full strip kept the cat/tee body: %q", full)
	}
	if strings.Contains(full, commitMarker) {
		t.Fatalf("full strip kept the commit body: %q", full)
	}

	// bash p.sh is itself an uninspectable wrapper, so command_parse_limit
	// also fires and names the deny reason (it is reported first); the
	// diagnostic table is what proves the gh rule independently caught the
	// gated verb the cat/tee body carries.
	outcome, diag, _ := runGo(t, tableCase{command: command})
	if outcome.Kind() != "deny" {
		t.Fatalf("kind = %s, want deny (%s)", outcome.Kind(), outcome.Payload())
	}
	if !strings.Contains(diag, "unsafe_publication") {
		t.Fatalf("diagnostic table lacks the unsafe_publication finding:\n%s", diag)
	}
}

func TestGhCommandFromStrippedFailsClosed(t *testing.T) {
	const fallback = "original command"
	for _, tc := range []struct{ name, body, want string }{
		{"decodes the command", `Bash {"command":"git commit -F - <<'EOF'\nEOF"}`, "git commit -F - <<'EOF'\nEOF"},
		{"wrong tool prefix", `Shell {"command":"x"}`, fallback},
		{"not JSON", `Bash {"command":"x"`, fallback},
		{"extra key", `Bash {"command":"x","other":"y"}`, fallback},
		{"non-string command", `Bash {"command":1}`, fallback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ghCommandFromStripped("Bash", tc.body, fallback); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestQuotedFakeHeredocIntroDenies is the #651 regression: a heredoc intro
// that only appears inside a quoted shell string (x="...") must never be
// treated as real — neither to be "paid for" by an unrelated real commit
// heredoc, nor to open a cat/tee exemption on its own — because the line it
// sits on never provably starts as executable shell text. Before the fix,
// stripHeredocsFor split the JSON-encoded scan text on the literal \n escape
// with no quote tracking at all, so the fake intro line matched the same
// text-only rules as a real one and its "body" (which holds the destructive
// command) was stripped out of the scan.
func TestQuotedFakeHeredocIntroDenies(t *testing.T) {
	destructive := "rm -" + "rf /"
	for _, tc := range []struct{ name, command string }{
		{
			"commit paid by a real commit",
			"git commit -F - <<'EOF'\nmsg\nEOF\nx=\"\ngit commit -F - <<'EOF'\n\"\n" + destructive + "\nEOF\n",
		},
		{
			"cat",
			"x=\"\ncat <<'EOF'\n\"\n" + destructive + "\nEOF\n",
		},
		{
			"tee",
			"x=\"\ntee note <<'EOF'\n\"\n" + destructive + "\nEOF\n",
		},
		{
			"cat unquoted delimiter",
			"x=\"\ncat <<EOF\n\"\n" + destructive + "\nEOF\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := commitHeredocScan(t, tc.command); !strings.Contains(got, "rm -") {
				t.Fatalf("strip removed the destructive line: %q", got)
			}

			c := tableCase{tool: "Bash", command: tc.command, want: "deny"}
			assertCaseSemantics(t, c, recordCase(t, c))
		})
	}
}
