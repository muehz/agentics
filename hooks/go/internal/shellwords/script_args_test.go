package shellwords

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

func argsOutcome(command, body string) (Result, string) {
	res := parseScript(command, fakeReader{"/ws/p.sh": {ID: "1", Text: body}})
	return res, firstScriptReason(res)
}

func TestScriptCallsRecordArgumentsAndParameterUse(t *testing.T) {
	res, reason := argsOutcome("bash /ws/p.sh a b", "echo \"$1\"\n")
	if reason != "" || len(res.ScriptCalls) != 1 {
		t.Fatalf("reason %q calls %+v", reason, res.ScriptCalls)
	}
	c := res.ScriptCalls[0]
	if c.Path != "/ws/p.sh" || !c.ConsumesParams || c.UnquotedParams || len(c.Args) != 2 || c.Args[0] != "a" || c.Args[1] != "b" {
		t.Fatalf("got %+v", c)
	}
	for body, want := range map[string][2]bool{
		"echo hi\n": {false, false}, "echo \"Usage: $0\"\n": {false, false}, "echo '$1'\n": {false, false},
		"echo $1\n": {true, true}, "rm -rf $1\n": {true, true}, "shift\n": {true, false}, "getopts ab opt\n": {true, false},
		"# it's\necho $1\n": {true, true}, "echo \"$@\"\n": {true, false}, "echo \"${1}\"\n": {true, false},
		"echo $#\n": {true, false}, "echo $@\n": {true, true}, "echo ${1:-x}\n": {true, true},
		// Quotes inside $( ) start a fresh context, and the quote state runs across lines.
		"echo \"$(clean \"$2\")\"\n": {true, false}, "echo $(clean $2)\n": {true, true}, "x=\"$(f \"$1\" \"$2\")\"\n": {true, false},
		"awk '\n{ print $1 }\n' f\n": {false, false}, "for w in $2; do :; done\n": {true, true},
		"local list=\"$2\"\nfor w in $list; do :; done\n": {true, false},
		// A context that never closes is read in the stricter way.
		"echo \"$(f \"$1\"\n": {true, true}, "cat <<E\nit's\nE\necho $1\n": {true, true}, "cat <<E\nx\n": {true, true},
		// A heredoc body is data: quotes in it change nothing, and an unquoted body expands without splitting.
		"cat <<'E'\nit's $1\nE\necho \"$2\"\n": {true, false}, "cat <<E\nit's $1\nE\necho hi\n": {true, false},
		"cat <<-E\n\tit's\n\tE\necho hi\n": {false, false}, "echo $((1<<2))\necho \"$1\"\n": {true, false},
		"read -r t <<< \"$1\"\necho hi\n": {true, false}, "cat <<'E' | f\nit's\nE\necho \"$1\"\n": {true, false}, "cat <<E1 && cat <<E2\na'\nE1\nb'\nE2\necho \"$1\"\n": {true, false},
	} {
		r, _ := argsOutcome("bash /ws/p.sh x", body)
		if len(r.ScriptCalls) != 1 || r.ScriptCalls[0].ConsumesParams != want[0] || r.ScriptCalls[0].UnquotedParams != want[1] {
			t.Errorf("%q: got %+v, want consumes=%v unquoted=%v", body, r.ScriptCalls, want[0], want[1])
		}
	}
}

// A script that runs no command leaves no invocation, so the write redirects on its call, and on the wrappers and
// script calls above it, are kept on the call record; descriptor duplications and input redirects are not writes.
func TestScriptCallsCarryTheWriteRedirectsTheyRunUnder(t *testing.T) {
	files := fakeReader{
		"/ws/empty.sh":   {ID: "1", Text: ""},
		"/ws/ok.sh":      {ID: "2", Text: "echo hello\n"},
		"/ws/outer.sh":   {ID: "3", Text: "bash /ws/empty.sh\n"},
		"/ws/sources.sh": {ID: "4", Text: "source /ws/empty.sh >> /ws/b.txt\n"},
	}
	targets := func(c ScriptCall) string {
		var got []string
		for _, r := range c.OutputRedirects {
			got = append(got, r.Operator+r.Target.Text)
		}
		return strings.Join(got, " ")
	}
	cases := []struct {
		command string
		want    []string // per recorded call, nested before the call that made it
	}{
		{"bash /ws/empty.sh > /ws/a.txt", []string{">/ws/a.txt"}},
		{"bash /ws/empty.sh >> /ws/a.txt 2>&1", []string{">>/ws/a.txt"}},
		{"bash /ws/empty.sh < /ws/a.txt", []string{""}},
		{"bash /ws/ok.sh", []string{""}},
		{"bash /ws/outer.sh > /ws/a.txt", []string{">/ws/a.txt", ">/ws/a.txt"}},
		{"env bash /ws/empty.sh > /ws/a.txt", []string{">/ws/a.txt"}},
		{"bash -c 'bash /ws/empty.sh > /ws/b.txt' > /ws/a.txt", []string{">/ws/b.txt >/ws/a.txt"}},
		{"bash /ws/sources.sh > /ws/a.txt", []string{">>/ws/b.txt >/ws/a.txt", ">/ws/a.txt"}},
	}
	for _, tc := range cases {
		res := parseScript(tc.command, files)
		if reason := firstScriptReason(res); reason != "" || len(res.ScriptCalls) != len(tc.want) {
			t.Errorf("%q: reason %q, calls %+v", tc.command, reason, res.ScriptCalls)
			continue
		}
		for i, want := range tc.want {
			if got := targets(res.ScriptCalls[i]); got != want {
				t.Errorf("%q call %d (%s): redirects %q, want %q", tc.command, i, res.ScriptCalls[i].Path, got, want)
			}
		}
	}
}

func TestScriptCallsAreRecordedForNestedCalls(t *testing.T) {
	files := fakeReader{"/ws/outer.sh": {ID: "1", Text: "bash /ws/inner.sh one\n"}, "/ws/inner.sh": {ID: "2", Text: "echo \"$1\"\n"}}
	res := parseScript("bash /ws/outer.sh", files)
	if len(res.ScriptCalls) != 2 || res.ScriptCalls[0].Path != "/ws/inner.sh" || res.ScriptCalls[1].Path != "/ws/outer.sh" ||
		len(res.ScriptCalls[0].Args) != 1 || res.ScriptCalls[0].Args[0] != "one" || !res.ScriptCalls[0].ConsumesParams {
		t.Fatalf("got %+v", res.ScriptCalls)
	}
	// The outer script forwards its own parameters to the inner one, which uses them.
	files["/ws/outer.sh"] = ScriptRead{ID: "1", Text: "bash /ws/inner.sh \"$@\"\n"}
	if got := firstScriptReason(parseScript("bash /ws/outer.sh x", files)); !strings.HasPrefix(got, "script-caller-argument") {
		t.Errorf("forwarded parameters into a parameter-using script: got %q", got)
	}
}

func TestPOSIXSourceRequiresProvenArgumentBinding(t *testing.T) {
	files := fakeReader{
		"/ws/entry.sh":   {Text: ". /ws/first\n"},
		"/ws/first":      {Text: "source /ws/second.inc\n"},
		"/ws/second.inc": {Text: "printf '%s' \"$1\"\n"},
		"/ws/static":     {Text: "printf '%s' sample\n"},
	}
	for _, command := range []string{
		"source /ws/second.inc",
		"bash /ws/entry.sh sample",
		". /ws/second.inc explicit",
		"sh -c '. /ws/second.inc explicit'",
	} {
		res := parseScript(command, files)
		if reason := firstScriptReason(res); !strings.HasPrefix(reason, "script-caller-argument") {
			t.Errorf("%q: reason = %q, want script-caller-argument", command, reason)
		}
	}

	for _, command := range []string{
		"bash -c '. /ws/second.inc explicit'",
		"bash -c 'source /ws/second.inc explicit'",
	} {
		res := parseScript(command, files)
		if reason := firstScriptReason(res); reason != "" || len(res.ScriptCalls) != 1 {
			t.Fatalf("%q: reason %q, calls %+v", command, reason, res.ScriptCalls)
		}
		call := res.ScriptCalls[0]
		if !call.ConsumesParams || len(call.Args) != 1 || call.Args[0] != "explicit" {
			t.Errorf("%q: call = %+v", command, call)
		}
	}

	for _, command := range []string{
		"/bin/bash -c 'source /ws/second.inc explicit'",
		"/opt/git/bash.exe -c 'source /ws/second.inc explicit'",
	} {
		if reason := firstScriptReason(parseScript(command, files)); reason != "script-predecessor-may-write" {
			t.Errorf("%q: reason = %q, want script-predecessor-may-write", command, reason)
		}
	}

	res := parseScript(". /ws/static", files)
	if reason := firstScriptReason(res); reason != "" || len(res.ScriptCalls) != 1 || res.ScriptCalls[0].ConsumesParams {
		t.Errorf("parameter-independent source: reason %q, calls %+v", reason, res.ScriptCalls)
	}
}

func TestScriptNonLiteralArgumentsBlockOnlyWhenParametersAreConsumed(t *testing.T) {
	if _, got := argsOutcome("bash /ws/p.sh \"$X\"", "echo \"$1\"\n"); !strings.HasPrefix(got, "script-caller-argument") {
		t.Errorf("consuming script: got %q", got)
	}
	if _, got := argsOutcome("bash /ws/p.sh $(cmd)", "echo \"$1\"\n"); !strings.HasPrefix(got, "script-caller-argument") {
		t.Errorf("substitution argument: got %q", got)
	}
	if _, got := argsOutcome("bash /ws/p.sh \"$X\"", "echo hi\n"); got != "" {
		t.Errorf("non-consuming script: got %q", got)
	}
	if res, got := argsOutcome("bash /ws/p.sh \"$X\"", "echo hi\n"); got != "" || len(res.ScriptCalls) != 1 || res.ScriptCalls[0].ConsumesParams {
		t.Errorf("non-consuming script: got %q %+v", got, res.ScriptCalls)
	}
}

func TestScriptParameterForwardingBlocks(t *testing.T) {
	for body, blocked := range map[string]bool{
		"git \"$@\"\n": true, "git \"$1\" log\n": true, "gh \"$1\" pr\n": true, "npm \"$@\"\n": true, "docker $1 ps\n": true,
		"sudo git \"$1\" log\n": true, "builtin git \"$@\"\n": true,
		"git show \"$1\"\n": false, "git -C . log\n": false, "echo \"$@\"\n": false, "git -C \"$1\" status\n": false,
		"gh pr view \"$1\"\n": false, "npm run \"$1\"\n": false,
		"tofu \"$1\" plan\n": true, "terragrunt \"$@\"\n": true, "helmfile $1 sync\n": true, "pulumi \"$1\" up\n": true,
		"vercel \"${1}\"\n": true, "nuget \"$1\" push\n": true,
		"tofu plan \"$1\"\n": false, "pulumi --cwd \"$1\" up\n": false,
	} {
		_, got := argsOutcome("bash /ws/p.sh x", body)
		// The operand rule reads the same git forms, so either name is a refusal of the form.
		refused := strings.HasPrefix(got, "script-caller-argument") || strings.HasPrefix(got, "script-operand-not-literal") && strings.HasPrefix(body, "git ")
		if blocked != refused {
			t.Errorf("%q: blocked=%v, got reason %q", body, blocked, got)
		}
		if blocked && !strings.HasPrefix(body, "git ") && !strings.HasPrefix(got, "script-caller-argument") {
			t.Errorf("%q: got reason %q", body, got)
		}
	}
	// The refusal names the child's own line and never its text.
	if _, got := argsOutcome("bash /ws/p.sh x", "echo hi\nnpm \"$@\"\n"); got != "script-caller-argument in /ws/p.sh:2" {
		t.Errorf("got %q", got)
	}
}

func TestScriptStdinFedBlocks(t *testing.T) {
	for command, blocked := range map[string]bool{
		"echo a | bash /ws/p.sh": true, "bash /ws/p.sh <<< x": true, "bash /ws/p.sh < in.txt": true, "bash /ws/p.sh": false,
		"echo a | { bash /ws/p.sh; }": true, "env bash /ws/p.sh < in.txt": true, "bash -c 'bash /ws/p.sh' < in.txt": true,
		"echo a | env FOO=1 bash /ws/p.sh": true, "bash /ws/p.sh | cat": false, "bash /ws/p.sh > out.txt": false,
		"bash /ws/p.sh <<EOF\nx\nEOF": true,
	} {
		_, got := argsOutcome(command, "read x\n")
		if blocked != strings.HasPrefix(got, "script-stdin-fed") {
			t.Errorf("%q: blocked=%v, got %q", command, blocked, got)
		}
	}
	for body, blocked := range map[string]bool{
		"mapfile -t a\n": true, "readarray a\n": true, "cat\n": true, "echo hi\ncat | tee f\n": true, "while read -r l; do :; done\n": true,
		"cat f\n": false, "cat <<EOF\nx\nEOF\n": false, "echo hi\n": false, "threadread=1\n": false,
	} {
		_, got := argsOutcome("echo a | bash /ws/p.sh", body)
		if blocked != strings.HasPrefix(got, "script-stdin-fed") {
			t.Errorf("%q: blocked=%v, got %q", body, blocked, got)
		}
	}
}

func TestScriptStdinReachesNestedScripts(t *testing.T) {
	files := fakeReader{"/ws/outer.sh": {ID: "1", Text: "bash /ws/inner.sh\n"}, "/ws/inner.sh": {ID: "2", Text: "read x\n"},
		"/ws/outer2.sh": {ID: "3", Text: "bash /ws/inner.sh < /ws/in.txt\n"}}
	if got := firstScriptReason(parseScript("echo a | bash /ws/outer.sh", files)); !strings.HasPrefix(got, "script-stdin-fed") {
		t.Errorf("a nested reader behind a fed call: got %q", got)
	}
	if got := firstScriptReason(parseScript("bash /ws/outer.sh", files)); got != "" {
		t.Errorf("no feed: got %q", got)
	}
	if got := firstScriptReason(parseScript("echo a | bash /ws/outer2.sh", files)); got != "" {
		t.Errorf("the nested call has an input of its own: got %q", got)
	}
}

func TestScriptPinnedNamesOnTheCall(t *testing.T) {
	for command, want := range map[string]string{
		"PATH=/x bash /ws/p.sh": "script-env-pinned", "env GIT_DIR=/x bash /ws/p.sh": "script-env-pinned",
		"LD_PRELOAD=/x.so bash /ws/p.sh": "script-env-pinned", "IFS=, bash /ws/p.sh": "script-env-pinned",
		"env IFS=, bash /ws/p.sh": "script-env-pinned", "env -i bash /ws/p.sh": "script-env-pinned",
		"env -u PATH bash /ws/p.sh": "script-env-pinned", "nohup env GIT_DIR=/x bash /ws/p.sh": "script-env-pinned",
		"sudo env PATH=/x bash /ws/p.sh": "script-env-pinned", "PATH=/x bash -c 'bash /ws/p.sh'": "script-env-pinned",
		"env A=1 env BASH_ENV=/x bash /ws/p.sh": "script-env-pinned", "FOO=1 PATH=/x bash /ws/p.sh": "script-env-pinned",
		// A bare assignment lasts for the rest of the shell: it is shell state, like an export.
		"PATH=/x; bash /ws/p.sh": "script-predecessor-state", "PATH=/x && bash /ws/p.sh": "script-predecessor-state",
		"IFS=,; bash /ws/p.sh": "script-predecessor-state", "CDPATH=; bash /ws/p.sh": "script-predecessor-state",
		"PS4=x; set -x; bash /ws/p.sh": "script-predecessor-state", "export PATH=/x; bash /ws/p.sh": "script-predecessor-state",
		"FLOW_WORK_ROOT=x bash /ws/p.sh": "", "env FOO=1 bash /ws/p.sh": "", "FOO=1; bash /ws/p.sh": "", "env -u FOO bash /ws/p.sh": "",
		"FOO=1 bash /ws/p.sh": "", "env FOO=1 nohup bash /ws/p.sh": "", "PATH=/x echo hi; bash /ws/p.sh": "script-predecessor-state",
	} {
		_, got := argsOutcome(command, "echo hi\n")
		if want == "" && got != "" || want != "" && !strings.HasPrefix(got, want) {
			t.Errorf("%q: want %q, got %q", command, want, got)
		}
	}
}

// The prefix of a call made by a script is judged by the script text rules; the call layer leaves it alone.
func TestScriptPinnedPrefixOnNestedCallStaysWithTheTextRules(t *testing.T) {
	files := fakeReader{"/ws/outer.sh": {ID: "1", Text: "PATH=/x bash /ws/inner.sh\n"}, "/ws/inner.sh": {ID: "2", Text: "echo hi\n"}}
	if got := firstScriptReason(parseScript("bash /ws/outer.sh", files)); !strings.HasPrefix(got, "script-opaque:") {
		t.Errorf("got %q", got)
	}
}

func TestScriptCallsOfTheRepositoryHelper(t *testing.T) {
	text, err := os.ReadFile(filepath.Join(hosttest.Checkout(t), "hooks", "go", "internal", "shellwords", "testdata", "flow.sh"))
	if err != nil {
		t.Skip("the helper is not part of this checkout")
	}
	files := fakeReader{"/ws/hooks/go/internal/shellwords/testdata/flow.sh": {ID: "1", Text: strings.ReplaceAll(string(text), "\r", "")}}
	for _, command := range []string{
		"bash /ws/hooks/go/internal/shellwords/testdata/flow.sh status",
		"bash /ws/hooks/go/internal/shellwords/testdata/flow.sh log execute verify task-1 ok \"free text with / and --force words\" next",
		"bash /ws/hooks/go/internal/shellwords/testdata/flow.sh log execute verify task-1 ok 'a b' -- x",
	} {
		res := parseScript(command, files)
		if got := firstScriptReason(res); got != "" {
			t.Fatalf("%q: got %q", command, got)
		}
		c := res.ScriptCalls[len(res.ScriptCalls)-1]
		if !c.ConsumesParams || c.UnquotedParams {
			t.Errorf("%q: the helper uses its parameters and expands none of them unquoted: %+v", command, c)
		}
	}
	if res := parseScript("bash /ws/hooks/go/internal/shellwords/testdata/flow.sh log execute verify task-1 ok \"free text with / and --force words\"", files); len(res.ScriptCalls) == 0 ||
		res.ScriptCalls[len(res.ScriptCalls)-1].Args[5] != "free text with / and --force words" {
		t.Errorf("the free-text argument must be recorded whole: %+v", res.ScriptCalls)
	}
}

func TestScriptLoopsOverParametersWithoutADollar(t *testing.T) {
	for body, want := range map[string]bool{
		"for a; do :; done\n": true, "for a do :; done\n": true, "for a\ndo :; done\n": true, "select a; do :; done\n": true,
		"for a in x y; do :; done\n": false, "for ((i=0;i<3;i++)); do :; done\n": false, "echo for a\n": false, "forget a; do\n": false,
	} {
		r, _ := argsOutcome("bash /ws/p.sh x", body)
		if len(r.ScriptCalls) != 1 || r.ScriptCalls[0].ConsumesParams != want || r.ScriptCalls[0].UnquotedParams {
			t.Errorf("%q: got %+v, want consumes=%v unquoted=false", body, r.ScriptCalls, want)
		}
	}
}

func TestScriptArgumentsAppendedByXargsAreCallerArguments(t *testing.T) {
	for command, blocked := range map[string]bool{
		"echo a | xargs bash /ws/p.sh": true, "xargs -n 1 bash /ws/p.sh < /ws/list": true, "git diff --name-only | xargs -n 1 bash /ws/p.sh": true,
		"bash /ws/p.sh a": false,
	} {
		_, got := argsOutcome(command, "echo \"$1\"\n")
		if blocked != strings.HasPrefix(got, "script-caller-argument") {
			t.Errorf("%q: blocked=%v, got %q", command, blocked, got)
		}
	}
	if _, got := argsOutcome("echo a | xargs bash /ws/p.sh", "echo hi\n"); got != "" {
		t.Errorf("a script that uses no parameters ignores appended arguments: %q", got)
	}
}

func TestScriptPatternArguments(t *testing.T) {
	for command, want := range map[string]bool{
		"bash /ws/p.sh .e*": true, "bash /ws/p.sh {.env,x}": true, "bash /ws/p.sh ~/.aws/credentials": true, "bash /ws/p.sh f?.txt": true,
		"bash /ws/p.sh '*.txt'": false, "bash /ws/p.sh \"{a,b}\"": false, "bash /ws/p.sh a/b.txt": false, "bash /ws/p.sh a~b": false,
	} {
		res, _ := argsOutcome(command, "echo \"$1\"\n")
		if len(res.ScriptCalls) != 1 || res.ScriptCalls[0].PatternArgs != want {
			t.Errorf("%q: got %+v, want patternArgs=%v", command, res.ScriptCalls, want)
		}
	}
}

func TestScriptStdinReadersFollowedByASeparatorOrXargs(t *testing.T) {
	for _, body := range []string{"while read; do :; done\n", "read;\n", "xargs echo\n", "ls | xargs rm\n"} {
		if _, got := argsOutcome("echo a | bash /ws/p.sh", body); !strings.HasPrefix(got, "script-stdin-fed") {
			t.Errorf("%q: got %q", body, got)
		}
	}
}

func TestScriptPatternArgumentsQuotedIntoUnquotedUse(t *testing.T) {
	for body, want := range map[string]bool{
		"cat $1\n": true, "cat \"$1\"\n": false, "cat \"$1\" $2\n": true, "echo hi\n": false, "msg=$1\n": false,
	} {
		res, _ := argsOutcome("bash /ws/p.sh '.e*'", body)
		if len(res.ScriptCalls) != 1 || res.ScriptCalls[0].PatternArgs != want {
			t.Errorf("%q: got %+v, want patternArgs=%v", body, res.ScriptCalls, want)
		}
	}
}

func TestScriptCallerWords(t *testing.T) {
	call := ScriptCall{Args: []string{"a b", "c"}}
	if got := strings.Join(call.CallerWords(), "|"); got != "a b|c" {
		t.Errorf("quoted use: %q", got)
	}
	call.UnquotedParams = true
	if got := strings.Join(call.CallerWords(), "|"); got != "a b|c|a|b|c" {
		t.Errorf("unquoted use: %q", got)
	}
	if len(call.Args) != 2 {
		t.Errorf("CallerWords changed the call: %+v", call)
	}
}

// Positions where bash does not word-split an expansion do not count as unquoted use; every other position does.
func TestScanParamsNonSplittingPositions(t *testing.T) {
	for body, want := range map[string]bool{
		"msg=$1\n": false, "local name=$1\n": false, "export NAME=$2\n": false, "declare -r x=$1 y=$2\n": false,
		"x=a$1\n": false, "x=1 y=$2\n": false, "x=${1:-d}\n": false, "x+=$1\n": false,
		"[[ $1 == x ]]\n": false, "if [[ -z $1 || -n $2 ]]; then :; fi\n": false, "[[ x =~ $1 ]]\n": false,
		"case $1 in\n  a) :;; esac\n": false, "case \"$1\" in\n  a) :;; esac\n": false,
		// Everything else still splits.
		"echo x=$1\n": true, "cat $1\n": true, "echo msg=$1\n": true, "x=1 cat $1\n": true, "local x; cat $1\n": true,
		"[[ -n a ]] && cat $1\n": true, "[[ -n a ]]; cat $1\n": true, "echo case $1 in\n": true, "echo \"; local\" x=$1\n": true,
		"x=$(cat $1)\n": true, "for w in $1; do :; done\n": true, "echo [[ $1\n": true, "case x in\n  a) cat $1;; esac\n": true,
		"x=\"a;b\" y; echo $1\n": true,
	} {
		r, _ := argsOutcome("bash /ws/p.sh x", body)
		if len(r.ScriptCalls) != 1 || !r.ScriptCalls[0].ConsumesParams || r.ScriptCalls[0].UnquotedParams != want {
			t.Errorf("%q: got %+v, want unquoted=%v", body, r.ScriptCalls, want)
		}
	}
}
