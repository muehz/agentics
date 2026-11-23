package shellwords

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var updateParseGolden = flag.Bool("update-parse-golden", false, "rewrite testdata/parse_golden.json from the current Parse")

const parseGoldenPath = "testdata/parse_golden.json"

// parseGoldenExtra adds shapes the corpus lacks: nesting, heredocs,
// redirections of every operator, wrappers, PowerShell, and the empty cases.
var parseGoldenExtra = []string{
	"", " ", ";", "echo \"$(echo \"$(rm -rf /)\")\"", "echo \"`rm -rf /`\"",
	"cat <<'EOF'\n$(rm -rf /)\nEOF", "cat <<EOF\n$(rm -rf /)\nEOF", "cat <<<word",
	"a < in > out >> app 2>&1 <> rw >| clob &> both", "sudo env FOO=1 bash -c 'git push --force'",
	"FOO=1 BAR=2 cmd", "a | b && c || d & e; f", "Get-Content .env | Select-String KEY",
}

type parseGoldenCase struct {
	Dialect Dialect `json:"dialect"`
	Command string  `json:"command"`
	Result  Result  `json:"result"`
}

// readCorpusCommands is swapped out in tests to prove TestParseGolden never
// touches the corpus reader: the golden file at parseGoldenPath is its own
// fixed command list, and -update-parse-golden is the only path that rebuilds
// it from the corpus (on unchanged code).
var readCorpusCommands = corpusCommands

func corpusCommands(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../../cmd/hook/testdata/corpus/entries.json")
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			if s, ok := v["command"].(string); ok {
				out = append(out, s)
			}
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(v[k])
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(doc)
	if len(out) == 0 {
		t.Fatal("corpus yielded no commands")
	}
	return append(out, parseGoldenExtra...)
}

// TestParseGolden re-parses the (command, dialect) pairs recorded in
// testdata/parse_golden.json and checks Parse's output against the stored
// Result. The golden file is its own fixed command list — TestParseGolden
// never opens the corpus (hooks/go/cmd/hook/testdata/corpus/entries.json)
// unless -update-parse-golden is set, in which case it rebuilds the golden
// from the live corpus plus parseGoldenExtra (on unchanged code).
//
// The comparison marshals both the freshly-parsed Result and the
// stored-then-unmarshalled Result through the same json.Marshal call before
// comparing strings. That keeps the comparison faithful: unmarshalling can
// turn a stored `null` into a nil slice and a stored `[]` into an empty
// non-nil slice, but re-marshalling either side through the identical Go
// struct shape reproduces the same bytes it started from, so the only way
// the two JSON strings can differ is a genuine change in what Parse returns
// for that case.
func TestParseGolden(t *testing.T) {
	if *updateParseGolden {
		var cases []parseGoldenCase
		for _, command := range readCorpusCommands(t) {
			for _, dialect := range []Dialect{POSIX, PowerShell} {
				cases = append(cases, parseGoldenCase{Dialect: dialect, Command: command, Result: Parse(command, dialect)})
			}
		}
		raw, err := json.MarshalIndent(cases, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(parseGoldenPath, append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(parseGoldenPath)
	if err != nil {
		t.Fatalf("%v — record it with -update-parse-golden on unchanged code", err)
	}
	var want []parseGoldenCase
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 {
		t.Fatal("golden holds no cases")
	}
	for i, c := range want {
		got := parseGoldenCase{Dialect: c.Dialect, Command: c.Command, Result: Parse(c.Command, c.Dialect)}
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(c)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("case %d (%q): Parse output changed\n got %s\nwant %s", i, c.Command, gotJSON, wantJSON)
		}
		if len(got.Result.Invocations) == 0 && !reflect.ValueOf(got.Result.Invocations).IsNil() {
			t.Errorf("Parse(%q) returned an empty non-nil Invocations; it must stay nil", c.Command)
		}
	}
}

// TestParseGoldenIgnoresCorpusGrowth proves TestParseGolden reads only the
// fixed golden file, not the live corpus: it swaps readCorpusCommands for a
// stub that fails the test if called, then runs TestParseGolden and expects
// it to still pass.
func TestParseGoldenIgnoresCorpusGrowth(t *testing.T) {
	saved := readCorpusCommands
	readCorpusCommands = func(t *testing.T) []string { t.Fatal("TestParseGolden must not read the corpus"); return nil }
	t.Cleanup(func() { readCorpusCommands = saved })
	TestParseGolden(t)
}

// TestParseReservationIsBounded pins the up-front capacity at reserveCap for
// input shaped to inflate it: one quoted word holding a long run of spaces,
// and a long run of assignment-only segments that yield no invocation. The
// returned slices carry the reserved capacity, so a reservation that scaled
// with the input would show here.
func TestParseReservationIsBounded(t *testing.T) {
	spaces := Parse("echo '"+strings.Repeat(" ", 1<<16)+"'", POSIX)
	if len(spaces.Invocations) != 1 {
		t.Fatalf("spaces: got %d invocations, want 1", len(spaces.Invocations))
	}
	if got := cap(spaces.Invocations[0].Words); got > reserveCap {
		t.Errorf("spaces: Words capacity %d exceeds reserveCap %d", got, reserveCap)
	}
	segments := Parse(strings.Repeat("FOO=1;", 1<<12)+"echo x", POSIX)
	if len(segments.Invocations) != 1 {
		t.Fatalf("segments: got %d invocations, want 1", len(segments.Invocations))
	}
	if got := cap(segments.Invocations); got > reserveCap {
		t.Errorf("segments: Invocations capacity %d exceeds reserveCap %d", got, reserveCap)
	}
}
