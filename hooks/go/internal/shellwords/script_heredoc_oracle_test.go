package shellwords

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// heredocOpener is one way the command that holds a heredoc operator can be opened around the operator or continued
// past the end of its line. open is text written before the operator's command (a before-opener) or appended to the
// operator line (an after-opener). extra holds the lines the opener adds: after the operator line for an after-opener,
// after the heredoc for a before-opener. <M> in a line is replaced by a marker that bash runs only where the line is code.
type heredocOpener struct {
	name  string
	open  string
	extra []string
}

// beforeOpeners wrap the operator's command: the operator is counted inside them. Each ends the construct after the
// heredoc, so a sound lexer agrees with bash on every line.
func beforeOpeners() []heredocOpener {
	return []heredocOpener{
		{name: "none"},
		{name: "comsub", open: "x=$(", extra: []string{"); <M>"}},
		{name: "quoted-comsub", open: "x=\"$(", extra: []string{")\"; <M>"}},
		{name: "param-comsub", open: "x=${zz:-$(", extra: []string{")}; <M>"}},
		// cat reads the substitution to its end, so the child has run its markers before the fixture goes on.
		{name: "procsub", open: "cat <(", extra: []string{"); <M>"}},
		{name: "backtick", open: "x=`", extra: []string{"`; <M>"}},
		{name: "array", open: "a=(1 ", extra: []string{"); <M>"}},
		{name: "arith-comsub", open: "(( zz = $(", extra: []string{") )); <M>"}},
		{name: "test-comsub", open: "[ -n \"$(", extra: []string{")\" ]; <M>"}},
		{name: "cond-comsub", open: "[[ -n $(", extra: []string{") ]]; <M>"}},
		{name: "subshell", open: "( ", extra: []string{"); <M>"}},
		{name: "group", open: "{ ", extra: []string{"}; <M>"}},
		{name: "then", open: "if :; then ", extra: []string{"fi; <M>"}},
		{name: "do", open: "for zi in 1; do ", extra: []string{"done; <M>"}},
		{name: "while", open: "while :; do ", extra: []string{"break; done; <M>"}},
		{name: "case", open: "case x in x) ", extra: []string{";; esac; <M>"}},
		{name: "function", open: "zf() { ", extra: []string{"}; zf; <M>"}},
		{name: "and", open: ": && "},
		{name: "pipe", open: ": | "},
		// The operator sits on a line that continues the one before: a string left open, or a backslash-newline.
		{name: "open-string", open: "x=\"a\nb\" "},
		{name: "continued", open: ": \\\n"},
	}
}

// afterOpeners continue the operator's command past the end of its physical line, or end it there. Bash starts the
// body after the command's last line; a lexer that starts it on the next physical line reads the lines between as
// data, so a marker on those lines is the probe.
func afterOpeners() []heredocOpener {
	return []heredocOpener{
		{name: "none"},
		{name: "semicolon", open: ";"},
		{name: "backslash-redirect", open: " \\", extra: []string{">/dev/null; <M>"}},
		{name: "backslash-pipe", open: " \\", extra: []string{"| cat; <M>"}},
		{name: "backslash-word", open: " \\", extra: []string{"zz; <M>"}},
		{name: "backslash-only", open: " \\", extra: []string{"; <M>"}},
		{name: "comsub-arg", open: " $(", extra: []string{"<M>", "); <M>"}},
		{name: "comsub-assign", open: "; x=$(", extra: []string{"<M>", "); <M>"}},
		{name: "param-comsub", open: " ${zz:-$(", extra: []string{"<M>", ")}; <M>"}},
		{name: "procsub", open: " <(", extra: []string{"<M>", "); <M>"}},
		{name: "backtick", open: " `", extra: []string{"<M>", "`; <M>"}},
		{name: "subshell", open: "; (", extra: []string{"<M>", "); <M>"}},
		{name: "group", open: "; {", extra: []string{"<M>", "}; <M>"}},
		{name: "array", open: "; a=(", extra: []string{"1", "); <M>"}},
		{name: "arith", open: "; ((", extra: []string{"1", ")); <M>"}},
		{name: "arith-comsub", open: "; (( zz = $(", extra: []string{"<M>", ") )); <M>"}},
		{name: "test-comsub", open: "; [ -n \"$(", extra: []string{"<M>", ")\" ]; <M>"}},
		{name: "cond-comsub", open: "; [[ -n $(", extra: []string{"<M>", ") ]]; <M>"}},
		{name: "cond", open: "; [[ a &&", extra: []string{"b ]]; <M>"}},
		{name: "case", open: "; case x in", extra: []string{"x) <M>;;", "esac; <M>"}},
		{name: "then", open: "; if :; then", extra: []string{"<M>", "fi; <M>"}},
		{name: "do", open: "; for zi in 1; do", extra: []string{"<M>", "done; <M>"}},
		{name: "pipe", open: " |", extra: []string{"cat; <M>"}},
		// Only &&: after || the marker that follows the body is the right operand, which a successful cat skips.
		{name: "and", open: " &&", extra: []string{"<M>"}},
		{name: "double-quote", open: " \"", extra: []string{"<M>", "\"; <M>"}},
		{name: "single-quote", open: " '", extra: []string{"<M>", "'; <M>"}},
		{name: "ansi-quote", open: " $'", extra: []string{"<M>", "'; <M>"}},
		{name: "param", open: " ${zz:-", extra: []string{"<M>", "}; <M>"}},
	}
}

// heredocPrefixes are the commands that hold the operator.
func heredocPrefixes() []string {
	return []string{"cat <<E", "cat <<'E'", "cat <<-E", "echo zz <<E", "cat <<E >/dev/null", "cat <<E | cat", "x=1 cat <<E"}
}

// heredocFixtureText is one generated fixture: its text, the 1-based line of the operator, and the names it was built
// from.
type heredocFixtureText struct {
	name          string
	before, after string
	text          string
	intro         int
}

// generateHeredocFixtures crosses every pair of the three dimensions (prefix, before-opener, after-opener), holding
// the third at its first value, and replaces each <M> with a marker that prints its own line number on stderr. The
// marker lines are: those the openers add, the line bash takes as the first body line when the lexer is right, and
// one after the delimiter.
func generateHeredocFixtures() []heredocFixtureText {
	prefixes, befores, afters := heredocPrefixes(), beforeOpeners(), afterOpeners()
	seen := map[string]bool{}
	var out []heredocFixtureText
	add := func(prefix string, before, after heredocOpener) {
		lines := []string{before.open + prefix + after.open}
		lines = append(lines, after.extra...)
		lines = append(lines, "<M>", "E", "<M>")
		lines = append(lines, before.extra...)
		lines = append(lines, "<M>")
		text := strings.Join(lines, "\n") + "\n"
		if seen[text] {
			return
		}
		seen[text] = true
		var b strings.Builder
		for n, l := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
			b.WriteString(strings.ReplaceAll(l, "<M>", "echo L"+strconv.Itoa(n+1)+" >&2") + "\n")
		}
		out = append(out, heredocFixtureText{name: prefix + " / " + before.name + " / " + after.name, before: before.name,
			after: after.name, text: b.String(), intro: strings.Count(before.open, "\n") + 1})
	}
	for _, p := range prefixes {
		for _, a := range afters {
			add(p, befores[0], a)
		}
		for _, b := range befores {
			add(p, b, afters[0])
		}
	}
	for _, b := range befores {
		for _, a := range afters {
			add(prefixes[0], b, a)
		}
	}
	return out
}

// benignHeredocFixtures are ordinary heredoc idioms that must stay allowed. Each line is a no-op, a stubbed command or
// a marker, so bash runs nothing of consequence.
func benignHeredocFixtures() []string {
	return []string{
		"cat <<EOF\ndata\nEOF\necho L4 >&2\n",
		"cat <<'EOF'\n$(data)\nEOF\necho L4 >&2\n",
		"x=$(cat <<EOF\ndata\nEOF\n)\necho L5 >&2\n",
		"x=\"$(cat <<EOF\ndata\nEOF\n)\"\necho L5 >&2\n",
		"if [ -z \"$zz\" ]; then\ncat <<EOF\ndata\nEOF\nfi\necho L6 >&2\n",
		"[ -z \"$zz\" ] && cat <<EOF\ndata\nEOF\necho L4 >&2\n",
		"zf() {\ncat <<EOF\ndata\nEOF\n}\nzf\necho L7 >&2\n",
		"cat <<EOF >\"$zf\"\ndata\nEOF\necho L4 >&2\n",
		"sudo tee /dev/null <<EOF\ndata\nEOF\necho L4 >&2\n",
		"git commit -F - <<'EOF'\nmsg\nEOF\necho L4 >&2\n",
		"read -r -d '' zv <<'EOF'\ndata\nEOF\necho L4 >&2\n",
		"cat <<-EOF\n\tdata\n\tEOF\necho L4 >&2\n",
		"cat <<EOF | cat\ndata\nEOF\necho L4 >&2\n",
		"cat <<EOF; echo L4 >&2\ndata\nEOF\necho L5 >&2\n",
		"cat <<A <<B\na\nA\nb\nB\necho L6 >&2\n",
		"while IFS= read -r zl; do :; done <<EOF\ndata\nEOF\necho L4 >&2\n",
		"cat <<EOF\n$zz $(echo zz)\nEOF\necho L4 >&2\n",
		"cat <<EOF # note\ndata\nEOF\necho L4 >&2\n",
		// A construct left open at the operator line's newline, closed after the body: bash reads the body at that
		// newline all the same, so the tracker must not refuse a body that starts inside [[ ]], a paren, a pipe or a list.
		"cat <<EOF; [[ a &&\ndata\nEOF\nb ]]; echo L4 >&2\n",
		"cat <<EOF; (\ndata\nEOF\n: ); echo L4 >&2\n",
		"cat <<EOF |\ndata\nEOF\ncat; echo L4 >&2\n",
		"cat <<EOF &&\ndata\nEOF\necho L4 >&2\n",
		"x=$(cat <<EOF )\ndata\nEOF\necho L4 >&2\n",
	}
}

// heredocOracleResult is what the oracle found for one fixture.
type heredocOracleResult struct {
	ran    map[int]bool // the marker lines bash ran
	syntax bool         // bash reported a syntax error
}

// heredocMarkerProbe stubs the commands the fixtures name so no external program runs, then sources the fixture with
// its stderr on the oracle's stdout: the markers print there, the fixture's own output is dropped. The cat stub reads
// every file it is given, so a process substitution's child has finished when the stub returns.
const heredocMarkerProbe = "zdrain() { while IFS= read -r zl; do :; done; }; " +
	"cat() { if [ $# -eq 0 ]; then zdrain; else local zf; for zf in \"$@\"; do zdrain <\"$zf\"; done 2>/dev/null; fi; }; " +
	"tee() { zdrain; }; sudo() { zdrain; }; git() { zdrain; }; " +
	"source ./t.sh 2>&1 >/dev/null </dev/null"

var heredocConditionalSyntaxDiagnostic = regexp.MustCompile(`^\./t\.sh: line [0-9]+: conditional binary operator expected$`)

func parseHeredocOracleOutput(out string) heredocOracleResult {
	r := heredocOracleResult{ran: map[int]bool{}}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "L") {
			if n, err := strconv.Atoi(line[1:]); err == nil {
				r.ran[n] = true
			}
		}
		if strings.Contains(line, "syntax error") || strings.Contains(line, "unexpected EOF") || heredocConditionalSyntaxDiagnostic.MatchString(line) {
			r.syntax = true
		}
	}
	return r
}

// runHeredocOracle sources every fixture with real bash, a few at a time, each runner in its own directory.
func runHeredocOracle(t *testing.T, o *oracleShell, texts []string) []heredocOracleResult {
	t.Helper()
	results := make([]heredocOracleResult, len(texts))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < 6; w++ {
		dir := t.TempDir()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = parseHeredocOracleOutput(runProbe(t, o, dir, texts[i], heredocMarkerProbe))
			}
		}()
	}
	for i := range texts {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}

// lexerCodeLines reports, for each 1-based line of text, whether the parser's heredoc lexer reads it as command text
// rather than as part of a heredoc body (delimiter line included).
func lexerCodeLines(text string) map[int]bool {
	bodies := readHeredocExtents(text).spans()
	code := map[int]bool{}
	offset := 0
	for n, line := range strings.SplitAfter(text, "\n") {
		inBody := false
		for _, b := range bodies {
			inBody = inBody || (offset >= b[0] && offset < b[1])
		}
		code[n+1] = !inBody
		offset += len(line)
	}
	return code
}

// markerLines returns the 1-based lines of text that hold a marker.
func markerLines(text string) []int {
	var out []int
	for n, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "echo L"+strconv.Itoa(n+1)+" >&2") {
			out = append(out, n+1)
		}
	}
	return out
}

// acceptedHeredocOverRefusal reports whether a generated combination on which bash and the lexer agree may be refused
// all the same: the cost of a rule that is simpler than the shapes it covers, taken where bash's own reading depends on
// the context or the version.
func acceptedHeredocOverRefusal(before, after string) bool {
	substitution := map[string]bool{"comsub": true, "quoted-comsub": true, "param-comsub": true, "procsub": true,
		"test-comsub": true, "cond-comsub": true}
	reparsed := after == "arith" || after == "arith-comsub" || after == "cond-comsub" || strings.HasPrefix(after, "backslash-")
	switch {
	case before == "backtick":
		// Bash looks for the body inside the backtick text; with the body outside, it runs the next line as code.
		// The tracker refuses every << inside backticks rather than tell the two apart.
		return true
	case before == "array" || before == "arith-comsub":
		// A << inside an array assignment, or inside a substitution that an arithmetic command holds as text, is
		// refused where it is counted; where bash reads its body depends on the rest of the command.
		return true
	case before == "open-string" || before == "continued":
		// The operator is on a line the lexer does not start a command on, so it never finds the body; where the bash
		// on this host then reads the same lines as code, it is by accident rather than by agreement.
		return true
	case substitution[before] && reparsed:
		// At the top level bash runs the continuation before the body; inside a substitution the bash on this host
		// does not. A reading that differs by context is not one the lexer may rely on.
		return true
	}
	return false
}

// TestScriptHeredocContinuationAgainstBash generates every pairwise combination of a command holding a heredoc
// operator with the ways the command can be opened around the operator or continued past its line, runs each in real
// bash, and checks the one property the lexer must keep: on every marker line bash and the lexer agree whether the line
// is code, or the script is refused at the operator's line. A fixture on which they agree must not be refused unless
// the combination is an accepted over-refusal, and the benign idioms must never be refused.
func TestScriptHeredocContinuationAgainstBash(t *testing.T) {
	o := requireOracle(t, oracleBash44())
	if probe := runHeredocOracle(t, o, []string{"echo L1 >&2\n"}); !probe[0].ran[1] {
		t.Skipf("bash did not run a fixture from the temp directory")
	}
	fixtures := generateHeredocFixtures()
	benign := benignHeredocFixtures()
	texts := make([]string, 0, len(fixtures)+len(benign))
	for _, f := range fixtures {
		texts = append(texts, f.text)
	}
	texts = append(texts, benign...)
	results := runHeredocOracle(t, o, texts)
	var refused, dangerous, mismatched, agreed, rejected, over int
	var overNames, mismatchNames []string
	for i, f := range fixtures {
		res := results[i]
		code := lexerCodeLines(f.text)
		reasons := scriptReasons(t, f.text, nil)
		isRefused := hasRule(reasons, "heredoc-delimiter")
		if isRefused {
			refused++
		}
		// A marker bash ran on a line the lexer takes for data is the hiding this test exists for. The other direction,
		// a code line bash did not run, is harmless in itself but still a reading the lexer may not rely on, unless bash
		// rejected the fixture, when the lines it did not run prove nothing.
		hid, missed := false, false
		for _, n := range markerLines(f.text) {
			switch {
			case res.ran[n] && !code[n]:
				hid = true
			case !res.ran[n] && code[n] && !res.syntax:
				missed = true
			}
		}
		switch {
		case hid || missed:
			if hid {
				dangerous++
			} else {
				mismatched++
				mismatchNames = append(mismatchNames, f.name)
			}
			if !isRefused {
				t.Errorf("%s:\n%s\nbash ran %v, the lexer reads %v as code (hidden code %v), and the script is not refused: %v",
					f.name, f.text, sortedKeys(res.ran), trueKeys(code), hid, reasons)
			} else if !hasRuleAt(reasons, "heredoc-delimiter", f.intro) {
				t.Errorf("%s: refused, but not at the operator's line %d: %v", f.name, f.intro, reasons)
			}
		case res.syntax && len(res.ran) == 0:
			rejected++
		default:
			agreed++
			if isRefused {
				over++
				overNames = append(overNames, f.name)
				if !acceptedHeredocOverRefusal(f.before, f.after) {
					t.Errorf("%s:\n%s\nbash and the lexer agree, yet the script is refused: %v", f.name, f.text, reasons)
				}
			}
		}
	}
	for i, text := range benign {
		res := results[len(fixtures)+i]
		code := lexerCodeLines(text)
		for _, n := range markerLines(text) {
			if res.ran[n] != code[n] {
				t.Errorf("benign %q: bash ran %v, lexer code %v", text, sortedKeys(res.ran), trueKeys(code))
			}
		}
		if reasons := scriptReasons(t, text, nil); hasRule(reasons, "heredoc-delimiter") {
			t.Errorf("benign %q refused: %v", text, reasons)
		}
	}
	if dangerous == 0 {
		t.Errorf("no fixture showed bash running a line the lexer takes for data, so the oracle proves nothing")
	}
	t.Logf("fixtures %d: hidden code %d, other disagreements %d, agreed %d, rejected by bash %d, refused %d, over-refused %d, benign %d",
		len(fixtures), dangerous, mismatched, agreed, rejected, refused, over, len(benign))
	if over > 0 {
		t.Logf("over-refused: %s", strings.Join(overNames, "; "))
	}
	if mismatched > 0 {
		t.Logf("code lines bash did not run: %s", strings.Join(mismatchNames, "; "))
	}
}

func sortedKeys(m map[int]bool) []int {
	var out []int
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

func trueKeys(m map[int]bool) string {
	var out []string
	for _, k := range sortedKeys(m) {
		if m[k] {
			out = append(out, fmt.Sprint(k))
		}
	}
	return strings.Join(out, ",")
}
