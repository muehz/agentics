package shellwords

import (
	"context"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/argv"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// The reference below is the construct scan as it was before the substring tests were put in front of its
// expressions: every expression runs on every line. The differential test holds the fast scan to the same hits.

func referenceScanTextConstructs(text string) []textHit {
	var hits []textHit
	seen := map[textHit]bool{}
	adder := func(line int) func(construct) {
		return func(rule construct) {
			if h := (textHit{rule, line}); !seen[h] {
				seen[h] = true
				hits = append(hits, h)
			}
		}
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		referenceScanLine(line, adder(i+1))
	}
	for i := 0; i < len(lines); i++ {
		if !strings.HasSuffix(lines[i], `\`) {
			continue
		}
		first := i
		var joined strings.Builder
		for ; i < len(lines) && strings.HasSuffix(lines[i], `\`); i++ {
			joined.WriteString(strings.TrimSuffix(lines[i], `\`))
		}
		if i < len(lines) {
			joined.WriteString(lines[i])
		}
		referenceScanLine(joined.String(), adder(first+1))
	}
	for _, re := range []*regexp.Regexp{arithmeticSpan, conditionSpan} {
		for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
			span := text[m[2]:m[3]]
			if !strings.Contains(span, "\n") {
				continue
			}
			span = strings.ReplaceAll(strings.ReplaceAll(span, "\\\n", ""), "\n", " ")
			add := adder(strings.Count(text[:m[0]], "\n") + 1)
			if re == arithmeticSpan {
				for _, name := range referenceArithmeticAssignments(span) {
					if rule := namedVariable(name); rule != constructNone {
						add(rule)
					}
				}
			}
			if subscriptSubstitution.MatchString(span) {
				add(constructArithmeticSubstitution)
			}
		}
	}
	return hits
}

func referenceArithmeticAssignments(expression string) []string {
	var names []string
	for _, m := range arithmeticAssign.FindAllStringSubmatch(expression, -1) {
		names = append(names, m[1])
	}
	return names
}

func referenceScanLine(line string, add func(rule construct)) {
	if strings.TrimSpace(line) == "" {
		return
	}
	for _, re := range []*regexp.Regexp{funcDefParens, funcDefKeyword} {
		for _, m := range re.FindAllStringSubmatch(line, -1) {
			switch name := strings.ToLower(ExecutableName(m[1])); {
			case argv.Lookup(name).Shadow:
				add(constructFunctionShadow)
			case name == "command_not_found_handle":
				add(constructCommandNotFoundHandle)
			}
		}
	}
	if promptCommand.MatchString(line) {
		add(constructPromptCommand)
	}
	for _, re := range []*regexp.Regexp{defaultAssign, descriptorVar} {
		for _, m := range re.FindAllStringSubmatch(line, -1) {
			if rule := namedVariable(m[1]); rule != constructNone {
				add(rule)
			}
		}
	}
	for _, m := range arithmeticSpan.FindAllStringSubmatch(line, -1) {
		for _, name := range referenceArithmeticAssignments(m[1]) {
			if rule := namedVariable(name); rule != constructNone {
				add(rule)
			}
		}
	}
	if referenceArithmeticSubstitution(line) {
		add(constructArithmeticSubstitution)
	}
}

func referenceArithmeticSubstitution(line string) bool {
	for _, re := range []*regexp.Regexp{arithmeticSpan, conditionSpan, letArguments} {
		for _, m := range re.FindAllStringSubmatch(line, -1) {
			if subscriptSubstitution.MatchString(m[1]) {
				return true
			}
		}
	}
	return false
}

// testStringLiterals returns every string literal in the package's own test files: the hostile shapes the other tests
// already pin, which is the densest source of constructs the scan must still find.
func testStringLiterals(t *testing.T, skip string) []string {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, name := range files {
		if name == skip {
			continue
		}
		parsed, err := goparser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil && s != "" {
					out = append(out, s)
				}
			}
			return true
		})
	}
	return out
}

// shellFilesUnder returns the text of every .sh file below root, skipping dependency and VCS directories.
func shellFilesUnder(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", ".scratch", "bin":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".sh") {
			if data, err := os.ReadFile(path); err == nil {
				out = append(out, strings.ReplaceAll(string(data), "\r", ""))
			}
		}
		return nil
	})
	return out
}

// randomScanTexts builds texts from the fragments the scan looks for, glued with and without separators so that
// near misses (one bracket, a name without its parens) are as common as hits.
func randomScanTexts(n int) []string {
	fragments := []string{
		"(", ")", "()", "((", "))", "[[", "]]", "[", "]", "{", "}", "${", "$(", "$((", "`", "$[", "=", "+=", ":=", "<", ">", "<<", "<<<",
		"let", "function", "PROMPT_COMMAND", "command_not_found_handle", "rm", "git", "cat", "ls", "f", "x", "n",
		"PATH", "IFS", "HOME", "GIT_DIR", "LD_PRELOAD", "BASH_ENV", "a", "1", "2", "\\", "'", "\"", ";", "&", "|", "#",
		" ", " ", " ", "\t", "\n", "\\\n", "echo", "if", "then", "fi", "done", "for", "in", "$", "i++", "*", "+", "-",
		// Whole shapes the scan reports, so a mutation of one is as common as a hit.
		"rm() {", "function git", "function f", "PROMPT_COMMAND=x", "${PATH:=x}", "${IFS=}", "{fd}>f", "{PATH}<f", "{PATH}>f", "{IFS}>>f",
		"((PATH=1))", "((n+=1))", "let PATH=1", "let a[$(x)]=1", "[[ a[$(x)] ]]", "((a[`x`]=1))", "ls()",
		"command_not_found_handle()", "cat () {", "a[$(x)]=1", "${a[1]:=2}", "rm ( ) {", "function\tgit",
	}
	rng := rand.New(rand.NewSource(1153))
	out := make([]string, 0, n)
	for len(out) < n {
		var b strings.Builder
		for i, count := 0, 1+rng.Intn(14); i < count; i++ {
			b.WriteString(fragments[rng.Intn(len(fragments))])
			if rng.Intn(3) == 0 {
				b.WriteByte(' ')
			}
		}
		out = append(out, b.String())
	}
	return out
}

// The fast scan must report exactly the hits of the reference scan, in the same order, on whole texts and on every
// line of them. A substring test that skips an expression it should have run drops a hit here.
func TestScanTextConstructsMatchesTheReference(t *testing.T) {
	var corpus []string
	corpus = append(corpus, testStringLiterals(t, "script_scan_diff_test.go")...)
	corpus = append(corpus, shellFilesUnder(t, hosttest.Checkout(t))...)
	if entries, err := os.ReadDir(filepath.Join("testdata", "oracle")); err == nil {
		for _, e := range entries {
			if data, err := os.ReadFile(filepath.Join("testdata", "oracle", e.Name())); err == nil {
				corpus = append(corpus, strings.ReplaceAll(string(data), "\r", ""))
			}
		}
	}
	corpus = append(corpus, benignScriptBody(8<<10))
	fixed := len(corpus)
	corpus = append(corpus, randomScanTexts(6000)...)
	if fixed < 200 {
		t.Fatalf("only %d fixture texts: the corpus walk found too little", fixed)
	}

	checked, withHits := 0, 0
	compare := func(text string) {
		want := referenceScanTextConstructs(text)
		got := scanTextConstructs(text)
		checked++
		if len(want) > 0 {
			withHits++
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("scan differs from the reference on case %d (%d bytes): got %v, want %v", checked, len(text), got, want)
		}
	}
	for _, text := range corpus {
		compare(text)
		// Each line alone, as an editor of one line would see it, so a line-level skip cannot hide behind other lines.
		if strings.Count(text, "\n") < 400 {
			for _, line := range strings.Split(text, "\n") {
				compare(line)
			}
		}
	}
	if withHits < 1500 {
		t.Fatalf("only %d of %d texts had a hit: the corpus no longer exercises the scan", withHits, checked)
	}
	t.Logf("%d texts compared, %d with hits", checked, withHits)
}

// referenceSplitFreeAssignmentAt is the question assignScan answers, asked afresh from the $ at text[i]: it walks back
// to the start of the word and of the command every time, which is quadratic on a long line.
func referenceSplitFreeAssignmentAt(text string, i int) bool {
	ws := i
	for ws > 0 && strings.IndexByte(" \t\n;&|(){}\"'`", text[ws-1]) < 0 {
		ws--
	}
	if !assignmentWord.MatchString(text[ws:i]) {
		return false
	}
	start := ws
	for start > 0 && strings.IndexByte(";&|\n({", text[start-1]) < 0 {
		start--
	}
	fields := strings.Fields(text[start:ws])
	if len(fields) > 0 && declarationBuiltins[fields[0]] {
		fields = fields[1:]
		for len(fields) > 0 && strings.HasPrefix(fields[0], "-") {
			fields = fields[1:]
		}
	}
	for _, f := range fields {
		if !assignmentWord.MatchString(f) || strings.ContainsAny(f, "\"'`\\") {
			return false
		}
	}
	return true
}

// randomAssignTexts builds texts from the words the parameter scan weighs: assignments, declaration builtins and
// their options, separators, quotes, [[ ]], case, and the white space other than a space that Fields splits on.
func randomAssignTexts(n int) []string {
	fragments := []string{
		"a=", "a+=", "A_1=", "x=$1", "y=$@", "z=${2}", "$1", "$@", "$*", "${1}", "${#}", "$#", "$$", "$?", "=", "=$1",
		"local", "export", "declare", "readonly", "typeset", "-x", "-r", "-a", "--", "-", "echo", "cat", "read", "cd",
		"[[", "]]", "case", "in", "esac", "if", "then", "do", "!", "[", "]", ";", ";;", "&", "&&", "|", "||", "\n", "(", ")", "{", "}",
		"\"", "'", "`", "\\", "$(", "$'", "<<<", "<<", "#", " ", " ", " ", " ", "\t", "\r", "\v", " ", "\u0085", "é", "\xff",
		"a=b", "a=\"", "a='", "PATH=", "IFS=", "n1=", "1a=", "a b=", "a[1]=", "a=b=$1",
	}
	rng := rand.New(rand.NewSource(1213))
	out := make([]string, 0, n)
	for len(out) < n {
		var b strings.Builder
		for i, count := 0, 1+rng.Intn(18); i < count; i++ {
			b.WriteString(fragments[rng.Intn(len(fragments))])
			if rng.Intn(2) == 0 {
				b.WriteByte(' ')
			}
		}
		out = append(out, b.String())
	}
	return out
}

// longLineShapes are the one-line scripts that made the lookback quadratic, each repeated to any length.
var longLineShapes = []string{"a=$1 ", "x=$1; ", "echo $1 ", "local x=$1 ", "a=$1$1", "export A=$1 ", "a=1}b=$1) "}

func repeatedTo(unit string, n int) string {
	return strings.Repeat(unit, n/len(unit)+1)[:n]
}

// assignScan must answer exactly what the question asked afresh does, at every $ of every text: the scan
// reads each byte once and remembers the words, and a wrong memory shows as a different answer here.
func TestAssignScanMatchesTheReference(t *testing.T) {
	corpus := testStringLiterals(t, "script_scan_diff_test.go")
	corpus = append(corpus, shellFilesUnder(t, hosttest.Checkout(t))...)
	if entries, err := os.ReadDir(filepath.Join("testdata", "oracle")); err == nil {
		for _, e := range entries {
			if data, err := os.ReadFile(filepath.Join("testdata", "oracle", e.Name())); err == nil {
				corpus = append(corpus, strings.ReplaceAll(string(data), "\r", ""))
			}
		}
	}
	fixed := len(corpus)
	if fixed < 200 {
		t.Fatalf("only %d fixture texts: the corpus walk found too little", fixed)
	}
	corpus = append(corpus, randomAssignTexts(40000)...)
	for _, shape := range longLineShapes {
		corpus = append(corpus, repeatedTo(shape, 3000))
	}
	positions, trues := 0, 0
	for n, text := range corpus {
		scan := newAssignScan(text)
		for i := 0; i < len(text); i++ {
			if text[i] != '$' {
				continue
			}
			want := referenceSplitFreeAssignmentAt(text, i)
			if got := scan.splitFreeAt(i); got != want {
				t.Fatalf("text %d (%d bytes), position %d: got %v, want %v", n, len(text), i, got, want)
			}
			positions++
			if want {
				trues++
			}
		}
	}
	if trues < 2000 || positions-trues < 2000 {
		t.Fatalf("%d positions, %d split-free: the corpus no longer exercises both answers", positions, trues)
	}
	t.Logf("%d positions compared, %d split-free", positions, trues)
}

// A one-line script of 256 KiB parses in well under a second for every shape that used to take quadratic time.
func TestLongLineScriptsParseInLinearTime(t *testing.T) {
	for _, shape := range longLineShapes {
		text := repeatedTo(shape, 256<<10)
		files := fakeReader{"/ws/s.sh": {Text: text, Bytes: len(text)}}
		start := time.Now()
		ParseScripts(context.Background(), "bash /ws/s.sh", POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws"})
		if elapsed := time.Since(start); elapsed > time.Second*raceSlowdown {
			t.Errorf("%q repeated to 256 KiB took %v, over %v", shape, elapsed, time.Second*raceSlowdown)
		}
	}
}
