package shellwords

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// This oracle answers one question the other three do not: when a real interpreter runs a hostile-shaped script, is
// every external command it ran one the guard parsed, or was the script refused? TestScriptCwdMatchesRealBash covers
// where commands run, TestScriptHeredocEndAgainstBash and the operator rows cover where a heredoc body starts and ends,
// and TestScriptSetFlagsAgainstBash covers the set and shopt flags. Here the interpreter runs only recorder stubs that
// log a line, so no fixture can do anything but write to the log.

// oracleStubbed lists every command a fixture may name. Each is a recorder, so nothing real runs.
var oracleStubbed = []string{"git", "rm", "curl", "comm", "cat", "mktemp"}

// oracleRefused lists the fixtures the guard refuses outright; every other fixture is compared with the interpreter,
// so a refusal that appears there would silently turn a check into a skip.
var oracleRefused = map[string]bool{"alias_def.fixture": true}

// oracleLast names the command each fixture ends with when it is not rm; the interpreter must have run it, so a
// host whose interpreter stops partway cannot pass on a partial run.
var oracleLast = map[string]string{"subst_in_quotes.fixture": "curl"}

// oracleBashOnly lists the fixtures a POSIX sh such as dash rejects as a syntax error (here-strings and process
// substitution), where it runs nothing; under such an sh the comparison is skipped, not passed.
var oracleBashOnly = map[string]bool{
	"ifs_read.fixture": true, "herestring_then_cmd.fixture": true, "herestring_subst.fixture": true, "procsub_operand.fixture": true,
}

// oracleFixtures returns the checked-in fixtures and the ones that must carry CR bytes, which .gitattributes would
// normalize in a checked-in file.
func oracleFixtures(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "oracle", "*.fixture"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no oracle fixtures: %v", err)
	}
	out := map[string]string{
		"crlf.fixture":    "git status\r\nrm x\r\n",
		"lone_cr.fixture": "git status\rrm x\n",
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "\r") {
			t.Fatalf("%s holds a carriage return; the CR cases are built above, not checked in", p)
		}
		out[filepath.Base(p)] = string(raw)
	}
	return out
}

func TestScriptOracle(t *testing.T) {
	fixtures := oracleFixtures(t)
	for _, name := range []string{"bash", "sh"} {
		t.Run(name, func(t *testing.T) {
			requireOracle(t, oracleInterpreter(name), oracleStubs(oracleStubbed...)) // one skip for a host that cannot run a file
			for fixture, text := range fixtures {
				t.Run(fixture, func(t *testing.T) {
					res := parseScript(name+" /ws/t.sh", fakeReader{"/ws/t.sh": {ID: "1", Text: text}})
					parsed, refused := map[string]bool{}, false
					for _, inv := range res.Invocations {
						refused = refused || !inv.Inspectable
						if len(inv.Words) > 0 {
							parsed[ExecutableName(inv.Words[0].Text)] = true
						}
					}
					if strings.Contains(text, "\r") {
						if !refused {
							t.Fatalf("a script with CR was not refused")
						}
						return
					}
					if refused {
						if !oracleRefused[fixture] {
							t.Fatalf("the guard now refuses a fixture the oracle compares, so it no longer proves the parse")
						}
						return // the guard blocks the call, so nothing runs
					}
					// A fresh oracle per fixture keeps each fixture's stub log apart.
					o := requireOracle(t, oracleInterpreter(name), oracleStubs(oracleStubbed...))
					script := filepath.Join(t.TempDir(), "t.sh")
					if err := os.WriteFile(script, []byte(text), 0o644); err != nil {
						t.Fatal(err)
					}
					_, _, _ = o.Run(t, oracleRun{File: script}) // a failing fixture still shows what it ran
					got := o.StubLog()
					if len(got) == 0 && name == "sh" && oracleBashOnly[fixture] {
						t.Skipf("this sh rejects the bash-only syntax in %s", fixture)
					}
					if len(got) == 0 {
						t.Fatalf("%s ran no recorder, so the fixture proves nothing", name)
					}
					last := "rm"
					if c := oracleLast[fixture]; c != "" {
						last = c
					}
					if !slices.Contains(got, last) {
						t.Errorf("%s never ran the final command %q (ran %v), so the run stopped partway", name, last, got)
					}
					for _, command := range got {
						if !parsed[command] {
							t.Errorf("%s ran %q but the guard neither parsed it nor refused the script", name, command)
						}
					}
				})
			}
		})
	}
}

// TestScriptOracleRefusesCR pins the parser side of the CR gate with a reader that does not refuse CR itself.
func TestScriptOracleRefusesCR(t *testing.T) {
	for _, text := range []string{"rm x\r\n", "echo a\rrm x\n"} {
		got := scriptReasons(t, text, nil)
		if len(got) != 1 || got[0] != "script-encoding" {
			t.Errorf("%q: want one script-encoding refusal, got %v", text, got)
		}
	}
}
