package shellwords

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// operandOutcomeFiles parses `bash /ws/t.sh` over files and reports whether any child carries the operand rule's
// refusal, with the words after rm of every rm child.
func operandOutcomeFiles(files fakeReader) (blocked bool, rmWords []string) {
	res := parseScript("bash /ws/t.sh", files)
	for _, inv := range res.Invocations {
		if !inv.Inspectable && inv.Reason.Rule() == RuleOperandNotLiteral {
			blocked = true
		}
		if len(inv.Words) > 0 && ExecutableName(inv.Words[0].Text) == "rm" {
			for _, w := range inv.Words[1:] {
				rmWords = append(rmWords, w.Text)
			}
		}
	}
	return blocked, rmWords
}

func operandOutcome(body string) (blocked bool, rmWords []string) {
	return operandOutcomeFiles(fakeReader{"/ws/t.sh": {Text: body}})
}

func TestScriptOperandBlocked(t *testing.T) {
	for _, body := range []string{
		// Unproven operands, unsafe values, several writes, conditional and loop writes, reads and arguments.
		"rm -rf \"$UNSET\"\n", "D=\"./build /\"\nrm -rf $D\n", "D='./*'\nrm -rf $D\n", "D=a\nD=b\nrm -rf \"$D\"\n",
		"if [ -z \"$1\" ]; then D=/; else D=./build; fi\nrm -rf \"$D\"\n", "for D in /; do rm -rf \"$D\"; done\n",
		"read D\nrm -rf \"$D\"\n", "git push origin \"$B\"\n", "dd of=\"$X\"\n", "chmod -R 755 \"$X\"\n",
		"tmp=\"$(mktemp -u)\"\nrm -f \"$tmp\"\n", "tmp=\"$(mktemp -p /)\"\nrm -f \"$tmp\"\n", "tmp=\"$(mktemp || echo /)\"\nrm -f \"$tmp\"\n",
		"tmp=\"$(mktemp)\"\ntmp=/\nrm -f \"$tmp\"\n", "tmp=\"$(mktemp -d)\"\nrm -rf \"$tmp/..\"\n",
		"mktemp() { echo /; }\ntmp=\"$(mktemp)\"\nrm -rf \"$tmp\"\n",
		"rm -rf \"$D\"\nD=./build\n", "tmp=\"$(mktemp)\"\nrm -f \"$tmp\"\ntrue\nrm -f \"$tmp\"\ntmp=/\n",
		"f() { rm -rf \"$1\"; }\nf /\n", "for D in a b; do rm -rf \"$D\"; done\n",
		// The assignment is not a command of the script, or not at its flat top.
		"cat <<EOF\nD=./build\nEOF\nrm -rf \"$D\"\n", "echo \"\nD=./build\n\"\nrm -rf \"$D\"\n", "(\nD=./build\n)\nrm -rf \"$D\"\n",
		"true &&\nD=./build\nrm -rf \"$D\"\n", "true && \\\nD=./build\nrm -rf \"$D\"\n",
		"if true; then\n:\nfi || {\nD=./build\n}\nrm -rf \"$D\"\n",
		"echo '\nD=./build\n'\nif true; then\nD=./build\nfi\nrm -rf \"$D\"\n",
		// A line equal to the assignment inside a string or heredoc body, with the real one conditional.
		"echo \"\nD=./build\n\"\nif [ -n \"$1\" ]; then D=./build; fi\nrm -rf \"$D\"\n",
		"cat <<EOF\nD=./build\nEOF\nif [ -n \"$1\" ]; then D=./build; fi\nrm -rf \"$D\"\n",
		"echo $'it\\'s\nD=./build\n'\nif [ -n \"$1\" ]; then D=./build; fi\nrm -rf \"$D\"\n",
		"echo a \\\nD=./build\nif [ -n \"$1\" ]; then D=./build; fi\nrm -rf \"$D\"\n",
		// Writers that are not a second bare assignment.
		"D=./build\nmapfile -t D <<< /\nrm -rf \"$D\"\n", "D=./build\nD[0]=/\nrm -rf \"$D\"\n", "D=./build\nD+=/\nrm -rf \"$D\"\n",
		"D=./build\n(( D++ ))\nrm -rf \"$D\"\n", "D=./build\n: \"${D:=/}\"\nrm -rf \"$D\"\n",
		"D=./build\nlet D++\nrm -rf \"$D\"\n", "D=./build\nread -raD\nrm -rf \"$D\"\n",
		"D=./build\nexport D=/ \nrm -rf \"$D\"\n", "D=./build\nD=/ :\nrm -rf \"$D\"\n",
		"f() { local D=/; rm -rf \"$D\"; }\nD=./build\nf\n",
		"rm -rf \"$D\"\nD=./build\nrm -rf \"$D\"\n", "_=./build\necho /\nrm -rf \"$_\"\n",
		// Accepted over-refusals.
		"D=./build; rm -rf \"$D\"\n", "D=./build\nexport D\nrm -rf \"$D\"\n", "cd \"$(dirname \"$0\")\"\nD=./build\nrm -rf \"$D\"\n",
		// The value is expanded by another shell.
		"D=./build\nbash -c 'rm -rf \"$D\"'\n",
		// Protected verbs behind options, partial operands, temps outside rm or quoted use.
		"git -C /ws push origin \"$B\"\n", "git --no-pager push origin \"$B\"\n", "git branch -D \"$B\"\n",
		"git branch -fd \"$B\"\n", "git --unknown-option push \"$B\"\n", "git \"$S\" origin \"$B\"\n", "chmod -\"$M\" 755 ./x\n",
		"chmod --recursive 755 \"$X\"\n", "rm -rf \"$D\"x\n", "rm -rf $D/sub\n",
		"tmp=$(mktemp)\ndd of=\"$tmp\"\n", "tmp=$(mktemp)\ngit push origin \"$tmp\"\n", "tmp=$(mktemp -t x.XXXXXX)\nrm -f \"$tmp\"\n", "tmp=$(mktemp)\nrm -f $tmp\n",
		"tmp=$(mktemp a;rm${IFS}-rf${IFS}/XXXXXX)\nrm -f \"$tmp\"\n", "tmp=$(mktemp /ws/x)\nrm -rf \"$tmp\"\n",
		"tmp=$(mktemp \"-uXXXXXX\")\nrm -f \"$tmp\"\n",
		// Variables the shell writes without naming them in a word.
		"REPLY=./build\nread -r <<< /\nrm -rf \"$REPLY\"\n", "REPLY=./build\nread -r <<< \"$1\"\nrm -rf \"$REPLY\"\n",
		"REPLY=./build\nread -r < f\nrm -rf \"$REPLY\"\n", "shopt -s lastpipe\nREPLY=./build\necho / | read -r\nrm -rf \"$REPLY\"\n",
		"REPLY=./build\nselect x in a; do break; done\nrm -rf \"$REPLY\"\n",
		"MAPFILE=./build\nmapfile <<< \"$1\"\nrm -rf \"$MAPFILE\"\n", "OPTARG=./build\ngetopts a: o -a /\nrm -rf \"$OPTARG\"\n",
		"OPTIND=./build\ngetopts a: o\nrm -rf \"$OPTIND\"\n", "COPROC=./build\nrm -rf \"$COPROC\"\n", "COPROC_PID=./build\nrm -rf \"$COPROC_PID\"\n",
		"LINES=./build\nrm -rf \"$LINES\"\n", "COLUMNS=./build\nrm -rf \"$COLUMNS\"\n",
		// The assignment sits inside a compound command that may run it zero or several times.
		"if [ -n \"$1\" ]; then\nD=/\nfi\nrm -rf \"$D\"\n", "while read -r x; do\nD=/\ndone < f\nrm -rf \"$D\"\n",
		"for x in a; do\nD=/\ndone\nrm -rf \"$D\"\n", "until false; do\nD=/\nbreak\ndone\nrm -rf \"$D\"\n",
		// A parameter expansion before the assignment is skipped for its braces only.
		"x=${1:-$(echo /)}\nD=./build\nrm -rf \"$D\"\n",
		// GNU getopt permutes options, so any non-literal word of chmod or git branch may itself be an option such as
		// -R or -D; the verb is protected whatever its literal words are.
		"chmod 644 \"$1\"\n", "git branch \"$1\"\n", "chmod \"$M\" 777 \"$X\"\n", "git branch \"$O\" main\n",
		"git branch --d \"$B\"\n", "git branch --de \"$B\"\n",
	} {
		if blocked, _ := operandOutcome(body); !blocked {
			t.Errorf("%q: want script-operand-not-literal", body)
		}
	}
}

// A git line whose subcommand ReadGit cannot read, or that only prints, may still reach a subcommand that pushes or
// deletes, so a non-literal operand of it must be proven. A line it reads to a verb is protected only for push and
// branch, in any letter case.
func TestScriptOperandUnreadableGitPosture(t *testing.T) {
	for _, body := range []string{
		"git --unknown \"$1\"\n", "git --exec-path \"$1\"\n", "git -- push \"$1\"\n", "git -C \"$1\"\n", "git \"$S\" origin \"$B\"\n",
		"git PUSH \"$B\"\n", "git -c$X push \"$B\"\n", "git -C=x push \"$B\"\n",
	} {
		if blocked, _ := operandOutcome(body); !blocked {
			t.Errorf("%q: want script-operand-not-literal", body)
		}
	}
	for _, body := range []string{
		"git -C /ws status \"$X\"\n", "git -C push status \"$X\"\n", "git -c$X status \"$X\"\n",
	} {
		if blocked, words := operandOutcome(body); blocked || words != nil {
			t.Errorf("%q: blocked=%v words=%q, want it allowed", body, blocked, words)
		}
	}
}

func TestScriptOperandNestedScripts(t *testing.T) {
	main := "D=./build\nsource /ws/lib.sh\nrm -rf \"$D\"\n"
	if blocked, _ := operandOutcomeFiles(fakeReader{"/ws/t.sh": {Text: main}, "/ws/lib.sh": {Text: "D=/\n"}}); !blocked {
		t.Error("a sourced second write must leave the operand unproven")
	}
	if blocked, words := operandOutcomeFiles(fakeReader{"/ws/t.sh": {Text: main}, "/ws/lib.sh": {Text: "echo hi\n"}}); blocked || !slices.Equal(words, []string{"-rf", "./build"}) {
		t.Errorf("a harmless sourced file must not unprove the constant: blocked=%v words=%v", blocked, words)
	}
	// A function from a sourced file runs whenever its caller calls it, after writes the sourced file never sees.
	lib := "D=./build\nf() {\nrm -rf \"$D\"\n}\n"
	if blocked, _ := operandOutcomeFiles(fakeReader{"/ws/t.sh": {Text: "source /ws/lib.sh\nD=/\nf\n"}, "/ws/lib.sh": {Text: lib}}); !blocked {
		t.Error("a constant in a sourced file must not be proven")
	}
	// The temp proof trusts the name mktemp; a sourced function of that name is refused by the shadow marks instead.
	res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: "source /ws/lib.sh\ntmp=$(mktemp)\nrm -rf \"$tmp\"\n"},
		"/ws/lib.sh": {Text: "mktemp() { echo /; }\n"}})
	refused := false
	for _, inv := range res.Invocations {
		refused = refused || !inv.Inspectable && inv.Reason.Rule() != RuleNone
	}
	if !refused {
		t.Error("a sourced mktemp function must leave the script refused")
	}
}

// The proofs must cost time in proportion to the script, not to the square of its assignments: a long run of
// assignments with no protected verb, with one protected use, and with a protected use of each of many.
func TestScriptOperandScalesLinearly(t *testing.T) {
	var assigns, uses strings.Builder
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&assigns, "V%d=1\n", i)
	}
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&uses, "rm -rf \"$V%d\"\n", i)
	}
	for name, body := range map[string]string{"no protected verb": assigns.String(),
		"one use": assigns.String() + "rm -rf \"$V1\"\n", "many uses": assigns.String() + uses.String()} {
		start := time.Now()
		res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: body}})
		if d := time.Since(start); d > raceSlowdown*400*time.Millisecond {
			t.Errorf("%s: parse took %v", name, d)
		}
		// A script refused at the parse deadline skips the proofs, so the timing above would prove nothing.
		if len(res.Scripts) != 1 {
			t.Errorf("%s: the script was not inspected: %q", name, firstScriptReason(res))
		}
	}
}

func TestScriptOperandAllowedAndRewritten(t *testing.T) {
	for body, want := range map[string][]string{
		"D=/\nrm -rf $D\n":                                                     {"-rf", "/"},
		"D=./build\nrm -rf \"$D\"\n":                                           {"-rf", "./build"},
		"D=./build # note\nrm -rf \"$D\"\n":                                    {"-rf", "./build"},
		"D=./build\nf() {\nrm -rf \"$D\"\n}\nf\n":                              {"-rf", "./build"},
		"D=./build\nif [ -n \"$1\" ]; then\nrm -rf \"$D\"\nfi\n":               {"-rf", "./build"},
		"set -eu\nD=\"./build\"\nrm -rf \"${D}\" ${D}\n":                       {"-rf", "./build", "./build"},
		"tmp=\"$(mktemp)\"\nrm -f \"$tmp\"\n":                                  {"-f", "/tmp/agentics-script-temp"},
		"tmp=\"$(mktemp -d \"${TMPDIR:-/tmp}/x.XXXXXX\")\"\nrm -rf \"$tmp\"\n": {"-rf", "/tmp/agentics-script-temp"},
		"tmp=$(mktemp -d \"$1/x.XXXXXX\")\nrm -rf \"$tmp\"\n":                  {"-rf", "/tmp/agentics-script-temp"},
		"tmp=$(mktemp)\ncd /\nrm -rf \"$tmp\"\n":                               {"-rf", "/tmp/agentics-script-temp"},
		"git show \"$1\"\n":                                                    nil,
		"rm -f ./literal.txt\n":                                                {"-f", "./literal.txt"},
		"D=./build\nsudo rm -rf \"$D\"\n":                                      {"-rf", "./build"},
		"ROOT=${1:-.}\n: \"${VAR:?}\"\nD=./build\nrm -rf \"$D\"\n":             {"-rf", "./build"},
		"git -C /ws status \"$X\"\n":                                           nil,
		"git -C push status \"$X\"\n":                                          nil,
	} {
		blocked, words := operandOutcome(body)
		if blocked || !slices.Equal(words, want) {
			t.Errorf("%q: blocked=%v words=%q, want %q", body, blocked, words, want)
		}
	}
}

// The parser unwraps these wrappers, so the operand rule sees rm as the command word.
func TestScriptOperandSeesThroughWrappers(t *testing.T) {
	for _, prefix := range []string{"command ", "xargs ", "env ", "\\", "sudo ", "nohup ", "time ", "timeout 5 ", "builtin "} {
		body := prefix + "rm -rf \"$UNSET\"\n"
		if blocked, _ := operandOutcome(body); !blocked {
			t.Errorf("%q: want script-operand-not-literal", body)
		}
	}
}

// Proven options are rewritten too, so the guards judge the literal form the shell runs.
func TestScriptOperandRewritesOptionWords(t *testing.T) {
	for body, want := range map[string][]string{
		"M=-R\nX=/\nchmod \"$M\" 777 \"$X\"\n": {"chmod", "-R", "777", "/"},
		"O=-D\ngit branch \"$O\" main\n":       {"git", "branch", "-D", "main"},
	} {
		res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: body}})
		var got []string
		for _, inv := range res.Invocations {
			if !inv.Inspectable {
				t.Errorf("%q: refused with %q", body, inv.Reason)
			}
			if len(inv.Words) > 0 && inv.Words[0].Text == want[0] {
				for _, w := range inv.Words {
					got = append(got, w.Text)
				}
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%q: words = %q, want %q", body, got, want)
		}
	}
}

// A child already refused by an earlier script rule keeps that rule's reason.
func TestScriptOperandKeepsEarlierRefusal(t *testing.T) {
	res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: "IFS=/ rm -rf \"$D\"\n"}})
	for _, inv := range res.Invocations {
		if len(inv.Words) > 0 && inv.Words[0].Text == "rm" && (inv.Reason.Rule() != RuleOpaque || inv.Reason.Construct() != "ifs-prefix") {
			t.Errorf("reason = %q, want the ifs-prefix refusal", inv.Reason)
		}
	}
}

func TestScriptOperandReasonCarriesLocation(t *testing.T) {
	res := parseScript("bash /ws/t.sh", fakeReader{"/ws/t.sh": {Text: "echo ok\nrm -rf \"$UNSET\"\n"}})
	for _, inv := range res.Invocations {
		if !inv.Inspectable {
			if inv.Reason.String() != "script-operand-not-literal in /ws/t.sh:2" {
				t.Errorf("reason = %q", inv.Reason)
			}
			return
		}
	}
	t.Error("rm with an unproven operand was not refused")
}
