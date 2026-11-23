package shellwords

import (
	"bufio"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"
)

// setVocabulary and shoptVocabulary are the words the generated set and shopt commands are built from: option
// clusters in both directions, -o names that are safe, unsafe or unknown, and words that end the options.
var (
	setVocabulary = []string{"-a", "+a", "-k", "+k", "-e", "+e", "-x", "+x", "-u", "-ea", "-ex", "-eu", "-eo", "-o", "+o", "allexport",
		"keyword", "pipefail", "errexit", "-ak", "-ka", "-B", "-h", "-C", "-f", "-E", "-T", "-P", "-H", "--", "-", "x", "posix"}
	shoptVocabulary = []string{"-s", "-u", "-o", "-so", "-os", "-uo", "-q", "-p", "allexport", "keyword", "pipefail", "nullglob", "extglob",
		"expand_aliases", "-su", "errexit"}
)

// oracleCombos returns every sequence of one and two words and a fixed random sample of three-word sequences.
func oracleCombos(vocab []string, sample int, seed int64) [][]string {
	var out [][]string
	for _, a := range vocab {
		out = append(out, []string{a})
		for _, b := range vocab {
			out = append(out, []string{a, b})
		}
	}
	r := rand.New(rand.NewSource(seed))
	for range sample {
		out = append(out, []string{vocab[r.Intn(len(vocab))], vocab[r.Intn(len(vocab))], vocab[r.Intn(len(vocab))]})
	}
	return out
}

// TestSetAndShoptMatchRealBash runs generated set and shopt commands in one real bash and checks that every command
// that leaves allexport (a in $-) or keyword (k in $-) on is refused before a script call. Each command runs, prints
// $- and resets the flags it may have changed, so one process covers all of them. noexec and onecmd are left out of
// the vocabulary because they stop the commands that report and reset.
func TestSetAndShoptMatchRealBash(t *testing.T) {
	bash := requireOracle(t)
	type row struct{ builtin, words string }
	var rows []row
	for _, c := range oracleCombos(setVocabulary, 1500, 1) {
		rows = append(rows, row{"set", strings.Join(c, " ")})
	}
	for _, c := range oracleCombos(shoptVocabulary, 800, 2) {
		rows = append(rows, row{"shopt", strings.Join(c, " ")})
	}
	var script strings.Builder
	script.WriteString("exec 2>/dev/null\n")
	for i, r := range rows {
		script.WriteString(r.builtin + " " + r.words + " >/dev/null || :\n")
		script.WriteString("echo \"" + strconv.Itoa(i) + " $-\"\n")
		script.WriteString("set +aekuxCfmETPH -Bh +o pipefail; shopt -u nullglob extglob expand_aliases\n")
	}
	// The script goes on stdin: thousands of lines exceed the Windows command-line limit.
	out, stderr, err := bash.Run(t, oracleRun{Stdin: script.String(), Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("bash failed: %v\n%s", err, stderr)
	}
	flags := map[int]string{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if n, f, ok := strings.Cut(strings.TrimSpace(sc.Text()), " "); ok {
			if i, err := strconv.Atoi(n); err == nil {
				flags[i] = f
			}
		}
	}
	if len(flags) != len(rows) {
		t.Fatalf("bash reported %d of %d rows", len(flags), len(rows))
	}
	exporting := 0
	for i, r := range rows {
		f, ok := flags[i]
		if !ok || !strings.ContainsAny(f, "ak") {
			continue
		}
		exporting++
		command := r.builtin + " " + r.words + "; bash /ws/ok.sh"
		if got := firstScriptReason(parseScript(command, chainFiles())); !strings.HasPrefix(got, "script-predecessor-state") {
			t.Errorf("%q leaves $- = %q in bash, got %q", command, f, got)
		}
	}
	if exporting == 0 {
		t.Fatal("no generated command turned allexport or keyword on; the oracle did not run as intended")
	}
}
