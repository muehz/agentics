package shellwords

import (
	"slices"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
)

// refusedAsWriter reports whether a typed command leaves an invocation refused for a write or shell state rule.
func refusedAsWriter(command string) (bool, string) {
	res := parseScript(command, chainFiles())
	for _, inv := range res.Invocations {
		if !inv.Inspectable && isWriterRule(inv.Reason.Rule()) {
			return true, inv.Reason.String()
		}
	}
	return false, firstScriptReason(res)
}

// isWriterRule reports the rules that refuse a script call for a write or shell state before or around it.
func isWriterRule(r Rule) bool {
	return r == RulePredecessorMayWrite || r == RulePredecessorState || r == RuleWritesInspected
}

// Every command of a substitution in the call's own words, prefix or redirects finishes before the shell opens the
// script, not only the first.
func TestEveryCommandOfTheCallsSubstitutionsIsJudged(t *testing.T) {
	refused := []string{
		`bash /ws/ok.sh "$(true; git checkout evil)"`,
		`bash /ws/ok.sh "$(true && git pull)"`,
		`bash /ws/ok.sh "$(true; patch -p1 < p)"`,
		`bash /ws/ok.sh "$(echo | npm ci)"`,
		`bash /ws/ok.sh "$( (git checkout evil) )"`,
		`bash /ws/ok.sh "$({ git checkout evil; })"`,
		`bash /ws/ok.sh $(true; git checkout evil)`,
		`X=$(true; git checkout evil) bash /ws/ok.sh`,
		`X="$(true; git checkout evil)" bash /ws/ok.sh`,
		`bash /ws/ok.sh <<< "$(true; git checkout evil)"`,
		"bash /ws/ok.sh <<EOF\n$(git checkout evil)\nEOF",
		"bash /ws/ok.sh <<EOF\n$(true; git checkout evil)\nEOF",
		"bash /ws/ok.sh <<EOF\n`git checkout evil`\nEOF",
		`bash /ws/ok.sh <(true; git checkout evil)`,
		`bash /ws/ok.sh <(git checkout evil)`,
		`bash /ws/ok.sh >(true; git checkout evil)`,
		"bash /ws/ok.sh `true; git checkout evil`",
		"bash /ws/ok.sh \"`true; git pull`\"",
		`bash /ws/ok.sh "$(echo $(true; git pull))"`,
		`bash /ws/ok.sh "$(echo "$(true; git pull)")"`,
		`bash /ws/ok.sh > "$(true; git pull)"`,
		`bash /ws/ok.sh arg "$(true; make)" other`,
		`bash ./ok.sh "$(true; git pull)"`,
		`cd /ws && bash ./ok.sh "$(true; git pull)"`,
		`bash /ws/ok.sh "$(true; sed -i s/a/b/ f)"`,
		`bash /ws/ok.sh "$(true; curl -O u)"`,
	}
	for _, command := range refused {
		if ok, got := refusedAsWriter(command); !ok {
			t.Errorf("%q: not refused as a writer (%q)", command, got)
		}
	}
	allowed := []string{
		`bash /ws/ok.sh "$(date)"`, `bash /ws/ok.sh "$(cat f)"`, `bash /ws/ok.sh "$(git rev-parse HEAD)"`, `bash /ws/ok.sh "$(true; echo x)"`,
		`bash /ws/ok.sh "$(true; git status)"`, `bash ./ok.sh "$(true; git log -1)"`, "bash /ws/ok.sh `true; date`",
		// A substitution that is not part of the call runs in the order the list gives, after it.
		`bash /ws/ok.sh; echo "$(true; git pull)"`, `bash /ws/ok.sh && x=$(true; make)`,
	}
	for _, command := range allowed {
		if got := firstScriptReason(parseScript(command, chainFiles())); got != "" {
			t.Errorf("%q: unexpected block %q", command, got)
		}
	}
	// The same substitution before the call is a predecessor.
	if ok, got := refusedAsWriter(`x=$(true; git pull); bash /ws/ok.sh`); !ok {
		t.Errorf("a substitution before the call: %q", got)
	}
}

// A writer that runs with the call, or before its next pass, changes the file the shell opens.
func TestWritersThatRunWithOrAfterTheCallAreJudged(t *testing.T) {
	refused := []string{
		"while :; do bash /ws/ok.sh; git pull; done",
		"until false; do bash /ws/ok.sh; patch -p1 < p; done",
		"for i in 1 2; do bash /ws/ok.sh; make; done",
		"while :; do\n bash /ws/ok.sh\n git checkout evil\ndone",
		"while read x; do bash /ws/ok.sh && git pull; done < f",
		"while :; do bash -c 'bash /ws/ok.sh; git pull'; done",
		"while :; do bash /ws/ok.sh; bash -c 'git pull'; done",
		"bash /ws/ok.sh & git pull",
		"bash /ws/ok.sh & sleep 1; git pull",
		"bash /ws/ok.sh & echo a; git checkout evil",
		"bash -c 'bash /ws/ok.sh' & git pull",
		"{ bash /ws/ok.sh; } & git pull",
		"bash /ws/ok.sh | git checkout evil",
		"bash /ws/ok.sh | patch -p1",
		"bash /ws/ok.sh | { git pull; }",
		"bash /ws/ok.sh | cat | npm ci",
		"bash /ws/ok.sh | ( make )",
		"bash /ws/ok.sh & git --attr-source HEAD checkout .",
	}
	for _, command := range refused {
		if ok, got := refusedAsWriter(command); !ok {
			t.Errorf("%q: not refused as a writer (%q)", command, got)
		}
	}
	allowed := []string{
		"bash /ws/ok.sh && git status", "bash /ws/ok.sh & echo hi", "bash /ws/ok.sh <<EOF\nhello\nEOF", "bash /ws/ok.sh <<EOF\nhello\nEOF\ngit status", "bash /ws/ok.sh | cat", "while :; do bash /ws/ok.sh; git status; done",
		"echo hi & bash /ws/ok.sh",
	}
	for _, command := range allowed {
		if got := firstScriptReason(parseScript(command, chainFiles())); got != "" {
			t.Errorf("%q: unexpected block %q", command, got)
		}
	}
	// A writer after a call in sequence is ordered after it, so it stays allowed.
	for _, command := range []string{"bash /ws/ok.sh; git pull", "bash /ws/ok.sh <<< hi; git pull", "bash /ws/ok.sh && git checkout main"} {
		if ok, got := refusedAsWriter(command); ok {
			t.Errorf("%q: refused as %q", command, got)
		}
	}
}

func TestIndexOnlyGitAndNonStateDeclarations(t *testing.T) {
	for _, p := range []string{"git restore --staged .", "git rm -r --cached .", "declare -p PATH", "declare -p", "export -p", "export -n PATH", "typeset -p BASH_ENV",
		"bash -c 'f() { :; }'", "bash -c 'g () { :; }'"} {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			if got := firstScriptReason(parseScript(p+" && "+call, chainFiles())); got != "" {
				t.Errorf("%q then %q: unexpected block %q", p, call, got)
			}
		}
	}
	for _, p := range []string{"git restore --staged --worktree .", "git restore -S -W .", "export -n PATH=/x", "f() { :; }"} {
		if got := firstScriptReason(parseScript(p+" && bash /ws/ok.sh", chainFiles())); !strings.HasPrefix(got, "script-predecessor-") {
			t.Errorf("%q: got %q", p, got)
		}
	}
}

// pipeFiles adds the execute helper at its repository path to the chain fixture.
func pipeFiles() fakeReader {
	files := chainFiles()
	files["/ws/hooks/go/internal/shellwords/testdata/flow.sh"] = ScriptRead{ID: "13", Text: "echo status\n"}
	return files
}

// wantRefusedAsWriter asserts that a command other than the script call itself is refused for a write or shell state
// rule, so a row cannot pass on a refusal of the call for another reason.
func wantRefusedAsWriter(t *testing.T, host pathshape.Host, commands []string) {
	t.Helper()
	for _, command := range commands {
		res := parseScriptOn(host, command, chainFiles())
		refused := false
		for _, inv := range res.Invocations {
			if inv.Script == nil && !inv.Inspectable && ExecutableName(inv.Words[0].Text) != "bash" &&
				isWriterRule(inv.Reason.Rule()) {
				refused = true
			}
		}
		if !refused {
			t.Errorf("%q: no writer refused (%q)", command, firstScriptReason(res))
		}
	}
}

// wantInspectedAndAllowed asserts that nothing is refused and that the call was expanded.
func wantInspectedAndAllowed(t *testing.T, host pathshape.Host, commands []string) {
	t.Helper()
	for _, command := range commands {
		res := parseScriptOn(host, command, pipeFiles())
		if got := firstScriptReason(res); got != "" {
			t.Errorf("%q: unexpected block %q", command, got)
		}
		if scriptChild(res) == nil {
			t.Errorf("%q: the call was not expanded", command)
		}
	}
}

// Only the call's own pipeline runs while the call does. A pipeline elsewhere in the list is ordered by its separator.
func TestOnlyTheCallsOwnPipelineIsJudged(t *testing.T) {
	host := pathshape.For("linux")
	wantInspectedAndAllowed(t, host, []string{
		"bash ./ok.sh | tee run.log", "bash ./ok.sh 2>&1 | tee run.log", "bash ./ok.sh |& tee log",
		"bash hooks/go/internal/shellwords/testdata/flow.sh status | tee /tmp/s.txt", "bash ./ok.sh && go test ./... 2>&1 | tee test.log",
		"bash ./ok.sh; echo x | tee log", "bash ./ok.sh; ls | sort > out.txt", "bash ./ok.sh; git log | head > out.txt",
		"bash ./ok.sh 2>&1 | tail", "bash ./ok.sh | grep x | wc -l", "bash ./ok.sh | cat > out.txt", "bash ./ok.sh | tee -a log | grep x",
		"time bash ./ok.sh | tee log", "! bash ./ok.sh | tee log", "{ bash ./ok.sh; } | tee log", "( bash ./ok.sh ) 2>&1 | tee log",
		"bash ./ok.sh | sed -n p > out.txt", "bash ./ok.sh && make | tee log", "bash ./ok.sh; git pull | cat",
		"bash hooks/go/internal/shellwords/testdata/flow.sh status 2>&1 | tail -20",
		"bash hooks/go/internal/shellwords/testdata/flow.sh log x && git add f && git commit -m y",
		"{ bash ./ok.sh; bash /ws/inner.sh; } > all.log", "bash ./ok.sh && git add -A && git commit -m x",
		`bash ./ok.sh "$(date)"`, `bash ./ok.sh "$(cat f)"`, `bash ./ok.sh "$(git rev-parse HEAD)"`, "FOO=$(pwd) bash ./ok.sh",
		"bash ./ok.sh > out.log", "bash ./ok.sh & wait",
	})
	// A writer of the call's own pipeline that does not list its targets, wherever the call sits in the pipeline's
	// first member: a group, a compound command, a command word that holds it in a substitution.
	wantRefusedAsWriter(t, host, []string{
		"bash ./ok.sh | git pull", "bash ./ok.sh |& git pull", "bash ./ok.sh |& make", "bash ./ok.sh | tee log | git pull",
		"bash ./ok.sh 2>&1 | patch -p1", "time bash ./ok.sh | git pull", "! bash ./ok.sh | git pull", "{ bash ./ok.sh; } | git pull",
		"{ bash ./ok.sh; } 2>&1 | git pull", "( bash ./ok.sh ) 2>&1 | git pull", "{ bash ./ok.sh; } > log | git pull",
		"while :; do bash /ws/ok.sh; done | git pull", "if true; then bash ./ok.sh; fi | git pull",
		"case x in x) bash ./ok.sh;; esac | git pull", "for i in 1; do bash /ws/ok.sh; done 2>&1 | make",
		"if true; then while :; do bash /ws/ok.sh; done; fi | git pull",
		`echo "$(bash ./ok.sh)" | git pull`, "cat <(bash ./ok.sh) | git pull", "bash -c 'bash ./ok.sh' | git pull",
		"bash ./ok.sh | bash -c 'git pull'", "bash ./ok.sh | xargs git pull", `bash ./ok.sh | tee "$(git pull)"`,
		"bash ./ok.sh | tee `make`", "bash ./ok.sh | while read l; do git pull; done", "bash ./ok.sh | while read l; do :; done | git pull",
		"bash ./ok.sh | if true; then make; fi", "bash ./ok.sh | { cat; git pull; }", "bash ./ok.sh | ( cat; npm ci )",
		"bash ./ok.sh |& { git pull; }", "bash ./ok.sh |& ( make )", "bash ./ok.sh | curl -O u", "bash ./ok.sh | curl -sSLO u",
		"bash ./ok.sh | wget u", "bash ./ok.sh | tar xf -", "bash ./ok.sh | unzip a", "bash ./ok.sh | go generate ./...",
		"bash ./ok.sh | npx x", "bash ./ok.sh | git checkout x", "bash ./ok.sh | git --exec-path checkout .", "bash ./ok.sh |\ngit pull", "bash ./ok.sh | tee log & git pull",
		// The typed commands of a member that also holds a call run with the first call.
		"bash ./ok.sh | bash -c 'bash /ws/inner.sh; git pull'", "bash ./ok.sh & bash -c 'bash /ws/inner.sh; git pull'",
		"while :; do bash /ws/ok.sh; bash -c 'bash /ws/inner.sh; git pull'; done",
		`bash /ws/ok.sh "$(bash -c 'bash /ws/inner.sh; git pull')"`,
		// Writers that name an inspected script are caught by the write-target check in every position.
		"bash ./ok.sh | tee ./ok.sh", "bash ./ok.sh | tee /ws/ok.sh", "bash ./ok.sh |& tee ok.sh", "bash ./ok.sh | cat > ./ok.sh",
		"bash ./ok.sh | cp /tmp/e ./ok.sh", "bash ./ok.sh | sed -i s/a/b/ ok.sh", "bash ./ok.sh | wget -O ok.sh u",
	})
	// A writer before the call is a predecessor, and the call carries the refusal.
	for _, command := range []string{"git pull | bash ./ok.sh", "echo x | tee log | bash ./ok.sh"} {
		if got := firstScriptReason(parseScriptOn(host, command, chainFiles())); !strings.HasPrefix(got, "script-predecessor-may-write") {
			t.Errorf("%q: got %q", command, got)
		}
	}
}

// A plain wait in the list's own shell returns after every background job, the call included.
func TestWaitEndsTheBackgroundWindow(t *testing.T) {
	wantInspectedAndAllowed(t, pathshape.Current(), []string{
		"bash ./ok.sh & wait; touch done.flag", "bash ./ok.sh & wait && git pull", "bash ./ok.sh & wait\ngit pull",
		"bash ./ok.sh & sleep 1 & wait; make", "bash ./ok.sh & wait; git pull & wait; make", "bash ./ok.sh & wait",
		"bash ./ok.sh & \"wait\"; git pull", "bash ./ok.sh & wait 2>/dev/null; git pull",
	})
	// wait that may not run, may not wait for the call, may not be the builtin, or is itself sent away.
	wantRefusedAsWriter(t, pathshape.Current(), []string{
		"bash ./ok.sh & wait & git pull", "bash ./ok.sh & wait | cat; git pull", "bash ./ok.sh & ( wait ); git pull",
		"bash ./ok.sh & true | wait; git pull", "bash ./ok.sh & wait -n; git pull", "bash ./ok.sh & wait $!; git pull",
		"bash ./ok.sh & false && wait; git pull", "bash ./ok.sh & false || wait; git pull", "bash ./ok.sh & alias wait=true; wait; git pull",
		"bash ./ok.sh & wait() { :; }; wait; git pull", "bash ./ok.sh & source x; wait; git pull", "bash ./ok.sh & env wait; git pull",
		"bash ./ok.sh & { wait; }; git pull", "bash ./ok.sh & if true; then wait; fi; git pull", "bash ./ok.sh & wait > f; git pull",
		"bash ./ok.sh & X=1 wait; git pull", "bash ./ok.sh & echo \"$(wait)\"; git pull", "bash ./ok.sh & wait; sleep 1 & git pull",
		"bash ./ok.sh & ( true; wait ); git pull", "bash ./ok.sh & { true; wait; }; git pull", "bash ./ok.sh & echo \"$(true; wait)\"; git pull",
		"bash ./ok.sh & true |& wait; git pull", "bash ./ok.sh & wait |& cat; git pull", "bash ./ok.sh & cat <(true; wait); git pull",
		// wait does not wait for a disowned job (bash 5.3: `sleep 2 & disown; wait` returns at once).
		"bash ./ok.sh & disown; wait; git pull", "bash ./ok.sh & disown -a; wait; git pull", "bash ./ok.sh & disown; wait && git pull",
		"bash ./ok.sh & disown %1; wait; git pull", "bash ./ok.sh & disown -h; wait; git pull", "bash ./ok.sh & builtin disown; wait; git pull",
		// A command word built at run time may be disown.
		"bash ./ok.sh & $d; wait; git pull", "bash ./ok.sh & \"$d\" -a; wait; git pull", "bash ./ok.sh & builtin $d; wait; git pull",
		// A & followed by a group still sends the call to the background.
		"bash ./ok.sh & { git pull; }", "bash ./ok.sh & ( git pull )", "bash ./ok.sh & (git pull)", "bash ./ok.sh &\n{ git pull; }",
		// A & at the end of a group or subshell is seen by the command after its closer.
		"{ bash ./ok.sh & }; git pull", "( bash ./ok.sh & ); git pull", "{ bash ./ok.sh & wait; }; git pull",
	})
}

// A loop keyword counts only where bash reads it as one: unquoted, first in a command. Then only a call inside the
// loop may run again after a later writer.
func TestLoopKeywordsComeFromCommandPosition(t *testing.T) {
	wantInspectedAndAllowed(t, pathshape.Current(), []string{
		"bash ./ok.sh && echo 'for you' && touch f", "bash ./ok.sh && git commit -m 'wait for it' && make",
		"bash ./ok.sh && make # for", "bash ./ok.sh && make # while true", "bash ./ok.sh && echo done && make",
		"bash ./ok.sh && echo while for until select && make", "bash ./ok.sh && echo \"while\" && make", "bash /ws/ok.sh; echo \"for\" | cat; make",
		"bash ./ok.sh && cat <<EOF\nwhile true\nEOF\nmake", "bash ./ok.sh && case x in for) echo;; esac && make",
		"bash ./ok.sh; for f in a b; do make; done",
		"bash ./ok.sh; while false; do :; done; git pull",
		"while :; do bash /ws/ok.sh; done; make", "if true; then while :; do bash /ws/ok.sh; done; fi; make",
		"bash ./ok.sh && git commit -m 'done' && git pull",
	})
	wantRefusedAsWriter(t, pathshape.Current(), []string{
		"for i in 1 2; do bash /ws/ok.sh && make; done", "while bash /ws/ok.sh; do git pull; done", "until bash /ws/ok.sh; do make; done",
		"select x in a; do bash /ws/ok.sh; make; done", "for ((i=0;i<2;i++)); do bash /ws/ok.sh; make; done",
		"for((i=0;i<2;i++)); do bash /ws/ok.sh; make; done", "while :; do if true; then bash /ws/ok.sh; fi; git pull; done",
		"while :; do case x in done) bash /ws/ok.sh;; esac; git pull; done", "time while :; do bash /ws/ok.sh; make; done",
		"! while :; do bash /ws/ok.sh; make; done", "{ while :; do bash /ws/ok.sh; make; done; }", "( while :; do bash /ws/ok.sh; make; done )",
		"while :; do (bash /ws/ok.sh); make; done", "while :; do echo \"$(bash /ws/ok.sh)\"; make; done",
		"while :; do bash -c 'bash /ws/ok.sh; make'; done", "while :; do bash /ws/ok.sh; bash -c 'make'; done",
		"while :\ndo\nbash /ws/ok.sh\nmake\ndone", "while :; do while :; do bash /ws/ok.sh; done; make; done",
		"if while bash /ws/ok.sh; do git pull; done; then :; fi", "while :; do bash /ws/ok.sh; git pull; done # for",
		// A case pattern spelled like a closer closes nothing.
		"while :; do case x in a) :;; done) bash /ws/ok.sh;; esac; git pull; done",
	})
}

func TestLeadingKeywords(t *testing.T) {
	for raw, want := range map[string]string{
		"while :": "while", " do bash x": "", "done": "done", " done > f": "done", "for i in a": "for", "for": "for",
		"if while x": "if while", "time -p while x": "while", "! until x": "until", "then if x": "if", "else case x in": "case",
		"echo for": "", "'while' x": "", `\while x`: "", "X=1 while x": "", "fi": "fi", "esac": "esac", "select x in a": "select",
		"-p while": "", "": "", "do done": "done", "while\twhile x": "while while",
	} {
		if got := strings.Join(leadingKeywords(raw), " "); got != want {
			t.Errorf("%q: got %q, want %q", raw, got, want)
		}
	}
}

// |& stays a pipe followed by a background separator for the other guards; the splitter records the edge as well.
func TestStderrPipeIsRecordedBesideTheBackgroundMark(t *testing.T) {
	edge := 0
	segs := splitSegments("a |& b", POSIX, &edge)
	if len(segs) != 2 || segs[1].before != Background || segs[1].stderrPipeEdge == 0 || segs[1].stderrPipeEdge != segs[0].pipelineOutputEdge ||
		segs[1].afterBackground {
		t.Fatalf("got %+v", segs)
	}
	for _, command := range []string{"a |& { b; }", "a |& ( b )", "a & { b; }", "a & (b)", "a & b"} {
		edge = 0
		segs = splitSegments(command, POSIX, &edge)
		if len(segs) != 2 || !segs[1].afterBackground || segs[1].stderrPipeEdge != 0 {
			t.Errorf("%q: got %+v", command, segs)
		}
	}
	edge = 0
	if segs = splitSegments("a || b; c | d", POSIX, &edge); slices.ContainsFunc(segs, func(s rawSegment) bool { return s.afterBackground || s.stderrPipeEdge != 0 }) {
		t.Errorf("no & at all: got %+v", segs)
	}
}
