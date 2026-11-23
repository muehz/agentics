package shellwords

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// diskReader reads the oracle's own fixture files as data.
type diskReader struct{}

func (diskReader) ReadScript(p string) ScriptRead {
	data, err := os.ReadFile(filepath.FromSlash(p))
	if err != nil {
		return ScriptRead{Path: p, Limit: RuleUnreadable, Detail: "no such fixture"}
	}
	return ScriptRead{Path: p, Text: string(data), Bytes: len(data)}
}

// oracleCommands are run by real bash from the ws fixture directory. Besides the fixture scripts, a row runs shell
// builtins and keywords and the standard utilities found on PATH: true, false, echo, cat, env, nohup, time and git
// (only for `command -v`), plus whatever program named cd an `env cd` or an external `time cd` finds. Rows whose
// first word would run an arbitrary program of that name (chdir, push-location, If) stay in the parser table only.
// Besides the log, only the loop rows and the glob rows write: the empty markers go and stop that end a loop, and an
// empty file named cd that a glob can match. They are removed after every row. A row runs ok.sh once, except the
// loops, which run it in two directories; the log and the parser's reads are compared in order.
var oracleCommands = []string{
	"bash ./ok.sh",
	"bash ok.sh",
	"cd sub && bash ./ok.sh",
	"cd sub; cd .. ; bash ok.sh",
	"cd .. && bash ./ok.sh",
	"cd sub && env bash ./ok.sh",
	"cd sub && env A=1 bash ./ok.sh",
	"cd sub && nohup bash ./ok.sh",
	"cd sub && time bash ./ok.sh",
	"cd sub && command bash ./ok.sh",
	"command cd sub && bash ./ok.sh",
	"builtin cd sub && bash ./ok.sh",
	"time cd sub && bash ./ok.sh",
	"env cd sub; bash ./ok.sh",
	"bash -c 'cd sub'; bash ./ok.sh",
	"bash ./a.sh && bash ./ok.sh",
	"bash -c 'cd sub && bash ./ok.sh'",
	"cd sub; bash -c 'bash ./ok.sh'",
	"cd sub && bash ./b.sh",
	"bash ./b.sh",
	"exec cd sub && bash ./ok.sh",
	"false || cd sub && bash ./ok.sh",
	"cd sub & bash ./ok.sh",
	"cd sub | cat; bash ./ok.sh",
	"if true; then cd sub; fi; bash ./ok.sh",
	"if cd sub; then bash ./ok.sh; fi",
	"if false; then cd sub; fi; bash ./ok.sh",
	"while false; do cd sub; done; bash ./ok.sh",
	"true || cd sub; bash ./ok.sh",
	"(true; cd sub); bash -c 'bash ./ok.sh'",
	"(cd sub); bash ./ok.sh",
	"(cd sub; bash ./ok.sh)",
	"{ cd sub; }; bash ./ok.sh",
	"(true; cd sub); bash ./ok.sh",
	"(true && cd sub); bash ./ok.sh",
	"echo $(true; cd sub); bash ./ok.sh",
	"echo `true; cd sub`; bash ./ok.sh",
	"f() { true; cd sub; }; f; bash ./ok.sh",
	"cd sub; echo $(bash ./ok.sh)",
	"x=$(cd sub); bash ./ok.sh",
	"echo $(bash ./ok.sh)",
	"echo x | cd sub; bash ./ok.sh",
	"false && cd sub; bash ./ok.sh",
	"true && cd sub && bash ./ok.sh",
	"\"time\" cd sub; bash ./ok.sh",
	"command time cd sub; bash ./ok.sh",
	"\\time cd sub; bash ./ok.sh",
	"time time cd sub; bash ./ok.sh",
	"! time cd sub; bash ./ok.sh",
	"time ! cd sub; bash ./ok.sh",
	"time -p cd sub && bash ./ok.sh",
	"time command cd sub; bash ./ok.sh",
	"bash -c 'time cd sub; bash ./ok.sh'",
	"command -p cd sub; bash ./ok.sh",
	"! cd sub; bash ./ok.sh",
	"if false; then cd sub; else bash ./ok.sh; fi",
	"if false; then cd sub; elif true; then bash ./ok.sh; fi",
	"while test ! -e stop; do bash ./ok.sh; cd sub; test -e ../go && : > stop; : > ../go; done",
	"if false; then c\\d sub; else bash ./ok.sh; fi",
	"true;if false;then cd sub;else bash ./ok.sh;fi",
	"if true; then bash ./ok.sh; fi",
	"builtin command cd sub; bash ./ok.sh",
	"builtin -- cd sub; bash ./ok.sh",
	"builtin builtin cd sub; bash ./ok.sh",
	"C=cd; $C sub; bash ./ok.sh",
	// Brace expansion, and a glob once a file named cd exists, turn a literal-looking command word into cd.
	"{cd,} sub; bash ./ok.sh",
	"{cd,} sub && bash ./ok.sh",
	": > cd; c[d] sub; bash ./ok.sh",
	": > cd; c? sub; bash ./ok.sh",
	": > cd; c*d sub; bash ./ok.sh",
	"c$''d sub; bash ./ok.sh",
	"{,cd} sub; bash ./ok.sh",
	"true && {cd,} sub && bash ./ok.sh",
	"! {cd,} sub; bash ./ok.sh",
	"x=1 c{d,} sub; bash ./ok.sh",
	"builtin {cd,} sub; bash ./ok.sh",
	"{ true; }; bash ./ok.sh",
	// An indirect cd after the call in a loop body moves the next pass of the call.
	"C=cd; while test ! -e stop; do bash ./ok.sh; $C sub; test -e ../go && : > stop; : > ../go; done",
	"while test ! -e stop; do bash ./ok.sh; c$''d sub; test -e ../go && : > stop; : > ../go; done",
	"while test ! -e stop; do bash ./ok.sh; {cd,} sub; test -e ../go && : > stop; : > ../go; done",
	"while test ! -e stop; do bash ./ok.sh; `printf %s c d` sub; test -e ../go && : > stop; : > ../go; done",
	// Ordinary command words around a cd still leave the directory proven.
	"echo hi; cd sub && bash ./ok.sh",
	"env echo hi; bash ./ok.sh",
	"echo *.txt $X; cd sub && bash ./ok.sh",
	"bash ./{a,b}.sh; bash ./ok.sh",
	"\"cd\" sub && bash ./ok.sh",
	"\"command\" cd sub && bash ./ok.sh",
	"\"builtin\" cd sub && bash ./ok.sh",
	"cd sub && bash ./ok.sh &",
	"false && cd sub; echo x && bash ./ok.sh",
	"cd sub & echo x; bash ./ok.sh",
	"cd sub; echo x | bash ./ok.sh",
	"true & cd sub; bash ./ok.sh",
	"cd sub &\nbash ./ok.sh",
	"cd sub & # c\nbash ./ok.sh",
	"cd sub & x=1; bash ./ok.sh",
	"cd sub & x=1 && bash ./ok.sh",
	"cd sub | x=1; bash ./ok.sh",
	"false && cd sub; x=1 && bash ./ok.sh",
	"false && cd sub; x=1\nbash ./ok.sh",
	"x=1; cd sub; bash ./ok.sh",
	"cd sub && bash ./c.sh && bash ./ok.sh",
	"popd; bash ./ok.sh",
	"for i in 1; do cd sub; done; bash ./ok.sh",
	// Commands that do not run in this shell still carry the separator before them.
	"cd sub & env echo hi; bash ./ok.sh",
	"cd sub & nohup true; bash ./ok.sh",
	"cd sub | env cat; bash ./ok.sh",
	"false && cd sub; env true && bash ./ok.sh",
	"cd sub & bash -c 'echo'; bash ./ok.sh",
	"cd sub & bash -c ''; bash ./ok.sh",
	"cd sub & bash -c 'x=1'; bash ./ok.sh",
	"cd sub & command -v git; bash ./ok.sh",
	"false && cd sub; command -v git && bash ./ok.sh",
	"cd sub & bash ./c.sh; bash ./ok.sh",
	"cd sub & bash ./empty.sh; bash ./ok.sh",
	"cd sub & command true; bash ./ok.sh",
	"cd sub && env true & bash ./ok.sh",
	"cd sub; env true & bash ./ok.sh",
	"cd sub & bash -c 'bash ./ok.sh'",
	"false && cd sub; bash -c 'bash ./ok.sh'",
	"cd sub | bash -c 'bash ./ok.sh'",
	"bash -c 'cd sub & env true; bash ./ok.sh'",
	"bash -c 'false && cd sub; command -v git && bash ./ok.sh'",
	"bash -c 'cd sub & bash ./ok.sh'",
	"false || bash -c 'cd sub; bash ./ok.sh'",
	"true && bash -c 'cd sub; bash ./ok.sh'",
	"cd sub && bash -c 'true; bash ./ok.sh'",
	"cd sub && bash -c 'x=1; bash ./ok.sh'",
	"cd sub && bash -c 'true & bash ./ok.sh'",
	"true && cd sub && bash -c 'true; bash ./ok.sh'",
	"bash -c 'exec cd sub'; bash ./ok.sh",
	"cd sub; time cd ..; bash ./ok.sh",
	"cd sub; command cd ..; bash ./ok.sh",
	"cd sub && bash ./b.sh & wait",
	"cd sub && true & wait; bash ./ok.sh",
	"cd sub; bash ./b.sh",
	"cd sub; bash ../b.sh",
	"cd sub; bash -c 'cd .. && bash ./ok.sh'",
	// A cd that fails leaves the directory alone, so a later .. climbs from where the shell really is.
	"cd nope; cd ..; bash ./ok.sh",
	"cd nope; bash ../ok.sh",
	"cd nope/..; cd ..; bash ./ok.sh",
	"cd nope/../sub; bash ./ok.sh",
	"cd nope; bash -c 'cd .. && bash ./ok.sh'",
	"cd nope && cd .. && bash ./ok.sh",
	"cd sub && cd .. && bash ./ok.sh",
	"cd sub/.. && bash ./ok.sh",
	"cd ..; bash ./ws/ok.sh",
	"bash ../ok.sh",
	// A sourced file runs in the calling shell, so its cd moves the calls after the source.
	"bash ./s.sh",
	"bash ./d.sh",
	"bash ./g.sh",
	"bash ./h.sh",
	"bash ./e.sh",
	// A call inside a sourced file is proven from the directory the source ran in.
	"bash ./cdb.sh",
	"bash ./srcin.sh",
	// A source operand without a slash is looked up in PATH first, so a same-named file there is what runs.
	"PATH=\"$PWD/pbin:$PATH\" bash ./pb.sh",
}

// TestScriptCwdMatchesRealBash runs each command with real bash and checks that whenever the parser resolves an
// ok.sh call to a path, it is the file bash actually ran. A refusal is always acceptable.
func TestScriptCwdMatchesRealBash(t *testing.T) {
	o := requireOracle(t)
	root := t.TempDir()
	ws := filepath.Join(root, "ws")
	sub := filepath.Join(ws, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	logLine := func(name string) string { return "printf '%s\\n' " + name + " >> \"$ORACLE_LOG\"\n" }
	// logged maps each ok.sh, as the parser spells it, to the name it logs when bash runs it.
	logged := map[string]string{
		filepath.ToSlash(filepath.Join(root, "ok.sh")): "root",
		filepath.ToSlash(filepath.Join(ws, "ok.sh")):   "ws",
		filepath.ToSlash(filepath.Join(sub, "ok.sh")):  "sub",
	}
	fixtures := map[string]string{
		filepath.Join(root, "ok.sh"):  logLine("root"),
		filepath.Join(ws, "ok.sh"):    logLine("ws"),
		filepath.Join(sub, "ok.sh"):   logLine("sub"),
		filepath.Join(ws, "a.sh"):     "cd sub\n",
		filepath.Join(ws, "b.sh"):     "bash ./ok.sh\n",
		filepath.Join(sub, "b.sh"):    "bash ./ok.sh\n",
		filepath.Join(ws, "c.sh"):     "x=1\n",
		filepath.Join(sub, "c.sh"):    "x=1\n",
		filepath.Join(ws, "empty.sh"): "",
		// s.sh and d.sh source a.sh, so its cd sub runs in their shell before their call.
		filepath.Join(ws, "s.sh"): "source ./a.sh\nbash ./ok.sh\n",
		filepath.Join(ws, "d.sh"): ". ./a.sh\nbash ./ok.sh\n",
		// g.sh sources a file whose cd never runs; h.sh and e.sh cd through a source and an eval the parser cannot read.
		filepath.Join(ws, "f.sh"): "cd sub &\nwait\n",
		filepath.Join(ws, "g.sh"): "source ./f.sh\nbash ./ok.sh\n",
		filepath.Join(ws, "h.sh"): "d=.\nsource \"$d/a.sh\"\nbash ./ok.sh\n",
		filepath.Join(ws, "e.sh"): "eval 'cd sub'\nbash ./ok.sh\n",
		// A call inside a sourced file resolves from the directory the source ran in.
		filepath.Join(ws, "cdb.sh"):   "cd sub && source ../b.sh\n",
		filepath.Join(ws, "srcin.sh"): "source ./n2.sh\n",
		filepath.Join(ws, "n2.sh"):    "cd sub\nbash ./ok.sh\n",
		// pb.sh sources a bare name. bash finds it in PATH (pbin) before the working directory, so the pbin copy runs.
		filepath.Join(ws, "pb.sh"):          "source pathed.sh\n",
		filepath.Join(ws, "pathed.sh"):      "bash ./ok.sh\n",
		filepath.Join(ws, "pbin/pathed.sh"): "bash ./sub/ok.sh\n",
	}
	if err := os.MkdirAll(filepath.Join(ws, "pbin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range fixtures {
		if err := os.WriteFile(name, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(t *testing.T, command string) []string {
		logPath := filepath.Join(t.TempDir(), "log")
		start := time.Now()
		// Several rows fail on purpose, so the exit status is not checked; only the log matters.
		_, _, _ = o.Run(t, oracleRun{Dir: ws, Command: command + "\nwait", Env: []string{"ORACLE_LOG=" + filepath.ToSlash(logPath)}})
		// Run cannot tell a timeout from an ordinary failure, and a killed row would look like a row that ran nothing.
		if elapsed := time.Since(start); elapsed >= oracleDefaultTimeout {
			t.Errorf("the run of %q took %v and reached its %v timeout", command, elapsed, oracleDefaultTimeout)
		}
		for _, marker := range []string{filepath.Join(root, "go"), filepath.Join(ws, "go"), filepath.Join(ws, "stop"), filepath.Join(sub, "stop"), filepath.Join(ws, "cd")} {
			_ = os.Remove(marker)
		}
		data, err := os.ReadFile(logPath)
		if err != nil {
			return nil
		}
		return strings.Fields(string(data))
	}
	// A bash that cannot run a fixture from the temp directory (a WSL launcher, say) is not an oracle.
	if got := run(t, "bash ./ok.sh"); len(got) != 1 || got[0] != "ws" {
		t.Skipf("bash at %s did not run a fixture script from the temp directory (logged %q)", o.path, got)
	}
	cwd := filepath.ToSlash(ws)
	proven := map[string]int{}
	major, minor := o.Version()
	// Older bash runs a `time` that follows `time` or `!` as the external program, so the cd happens in a child.
	oldBashRows := map[string]bool{"time time cd sub; bash ./ok.sh": true, "! time cd sub; bash ./ok.sh": true}
	for _, command := range oracleCommands {
		t.Run(command, func(t *testing.T) {
			if oldBashRows[command] && bashOlderThan44(major, minor) {
				t.Skip(oldBashSkipMessage(major, minor))
			}
			res := ParseScripts(context.Background(), command, POSIX, ScriptOptions{Reader: diskReader{}, Context: provenPOSIX, Cwd: cwd})
			var resolved []string
			for _, rec := range res.Scripts {
				if path.Base(rec.Path) != "ok.sh" {
					continue
				}
				name, ok := logged[rec.Path]
				if !ok {
					t.Fatalf("parser read %s, which is not a fixture", rec.Path)
				}
				resolved = append(resolved, name)
			}
			if len(resolved) == 0 {
				return
			}
			ran := run(t, command)
			if len(ran) == 0 {
				t.Logf("bash ran no ok.sh, so there is nothing to compare (parser read %v)", resolved)
				return
			}
			if strings.Join(ran, " ") != strings.Join(resolved, " ") {
				t.Errorf("parser read ok.sh in %v, bash ran ok.sh in %v", resolved, ran)
			}
			for _, r := range resolved {
				proven[r]++
			}
		})
	}
	t.Logf("proven %v", proven)
	// The comparison is vacuous if the parser proves nothing.
	if proven["ws"] == 0 || proven["sub"] == 0 || proven["root"] == 0 {
		t.Errorf("the parser proved too few rows to compare: %v", proven)
	}
}
