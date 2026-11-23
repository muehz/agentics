package shellwords

import (
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
)

// lineReason parses a call to /ws/t.sh whose only line is line and returns the first refusal.
func lineReason(host pathshape.Host, line string) string {
	files := chainFiles()
	files["/ws/t.sh"] = ScriptRead{ID: "10", Text: line + "\n"}
	return firstScriptReason(parseScriptOn(host, "bash /ws/t.sh", files))
}

func wantLinesRefused(t *testing.T, host pathshape.Host, lines []string) {
	t.Helper()
	for _, line := range lines {
		if got := lineReason(host, line); !strings.HasPrefix(got, "script-writes-inspected in /ws/t.sh:1") {
			t.Errorf("%q: got %q", line, got)
		}
	}
}

func wantLinesAllowed(t *testing.T, host pathshape.Host, lines []string) {
	t.Helper()
	for _, line := range lines {
		if got := lineReason(host, line); got != "" {
			t.Errorf("%q: unexpected block %q", line, got)
		}
	}
}

// Commands inside the call itself run before the script, after the guard read it.
func TestWritesInsideTheCallItselfAreJudged(t *testing.T) {
	host := pathshape.For("linux")
	for _, command := range []string{
		`bash /ws/ok.sh "$(cp /tmp/e /ws/ok.sh)"`,
		"bash /ws/ok.sh `cp /tmp/e /ws/ok.sh`",
		`X="$(cp /tmp/e /ws/ok.sh)" bash /ws/ok.sh`,
		`bash /ws/ok.sh > "$(cp /tmp/e /ws/ok.sh)"`,
		`bash /ws/ok.sh "$(sed -i s/a/b/ /ws/ok.sh)"`,
		`bash /ws/ok.sh <(cp /tmp/e /ws/ok.sh)`,
		// Commands that run while the call runs, or before its next pass.
		`bash /ws/ok.sh | cp /tmp/e /ws/ok.sh`,
		`bash /ws/ok.sh & sleep 1; cp /tmp/e /ws/ok.sh`,
		`while true; do bash /ws/ok.sh; cp /tmp/e /ws/ok.sh; done`,
		`until false; do bash /ws/ok.sh; cp /tmp/e /ws/ok.sh; done`,
	} {
		res := parseScriptOn(host, command, chainFiles())
		refused := false
		for _, inv := range res.Invocations {
			refused = refused || !inv.Inspectable && (inv.Reason.Rule() == RuleWritesInspected || inv.Reason.Rule() == RulePredecessorMayWrite)
		}
		if !refused {
			t.Errorf("%q: no typed write to the inspected script was refused: %q", command, firstScriptReason(res))
		}
	}
	// A writer in a substitution of the call is a predecessor even when it names no inspected path.
	for _, command := range []string{`bash /ws/ok.sh "$(touch f)"`, "bash /ws/ok.sh `make`", `bash /ws/ok.sh <(git checkout x)`} {
		if got := firstScriptReason(parseScriptOn(host, command, chainFiles())); !strings.HasPrefix(got, "script-predecessor-may-write") {
			t.Errorf("%q: got %q", command, got)
		}
	}
	for _, command := range []string{`bash /ws/ok.sh "$(date)"`, `bash /ws/ok.sh && echo x > /ws/log.txt`, `bash /ws/ok.sh | cat`,
		`bash /ws/ok.sh && cp /ws/ok.sh /tmp/copy`} {
		if got := firstScriptReason(parseScriptOn(host, command, chainFiles())); got != "" {
			t.Errorf("%q: unexpected block %q", command, got)
		}
	}
}

// A closer leaves no invocation behind, so its redirect would be lost.
func TestRedirectOnAGroupOrSubshellIsAWrite(t *testing.T) {
	host := pathshape.For("linux")
	for _, p := range []string{"{ cat /tmp/e; } > /ws/ok.sh", "( cat /tmp/e ) > /ws/ok.sh", "cat <(true) > /ws/ok.sh",
		"echo x | { cat; } > f", "f() { echo x; } >> f", "{ cat a; } 2> f", "{ cat a; } &> f"} {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			if got := firstScriptReason(parseScriptOn(host, p+" && "+call, chainFiles())); !strings.HasPrefix(got, "script-predecessor") {
				t.Errorf("%q then %q: got %q", p, call, got)
			}
		}
	}
	// A stray redirect after an inspected call has no unjudged invocation to ride on.
	if got := firstScriptReason(parseScriptOn(host, "bash /ws/inner.sh; > f; bash /ws/ok.sh", chainFiles())); !strings.HasPrefix(got, "script-predecessor-may-write") {
		t.Errorf("a redirect after a script call: got %q", got)
	}
	for _, p := range []string{"{ cat f; } > /dev/null", "{ cat f; } 2>&1", "( cat f ) >&2", "cat <(true) | cat"} {
		if got := firstScriptReason(parseScriptOn(host, p+" && bash /ws/ok.sh", chainFiles())); got != "" {
			t.Errorf("%q: unexpected block %q", p, got)
		}
	}
	wantLinesRefused(t, host, []string{"{ echo x; } >> t.sh", "( echo x ) >> t.sh", "f() { echo x; } >> t.sh", "cat <(true) > t.sh",
		"echo x | { cat; } > t.sh", "{ echo x; } >> /ws/t.sh", "{ echo x; } >> \"$0\""})
	wantLinesAllowed(t, host, []string{"{ echo x; } >> out.txt", "( echo x ) > /dev/null", "cat <(true) > out.txt", "{ echo x; } 2>&1"})
	// The call's own group redirect reaches the script it runs.
	if got := firstScriptReason(parseScriptOn(host, "{ bash /ws/ok.sh; } >> /ws/ok.sh", chainFiles())); !strings.HasPrefix(got, "script-writes-inspected") {
		t.Errorf("a group around the call: got %q", got)
	}
}

func TestDeclaredStateBlocks(t *testing.T) {
	state := []string{"declare -x BASH_ENV=/tmp/e", "typeset -x BASH_ENV=/tmp/e", "local -x BASH_ENV=/tmp/e", "declare -gx BASH_ENV=/tmp/e",
		"readonly PATH=/x", "declare PATH=/x", "export BASH_ENV", "export PROMPT_COMMAND", "declare -x PROMPT_COMMAND", "export LD_PRELOAD=/x",
		"export -f echo", "declare -f echo", "declare -fx echo", "typeset -f echo", "export -nf echo", "echo() { :; }", "ls() { :; }",
		"function g { :; }", "g () { :; }"}
	for _, p := range state {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			if got := firstScriptReason(parseScript(p+" && "+call, chainFiles())); !strings.HasPrefix(got, wantState(p, call)) {
				t.Errorf("%q then %q: got %q", p, call, got)
			}
		}
	}
	if got := firstScriptReason(parseScript("echo() { :; }; export -f echo; bash /ws/ok.sh", chainFiles())); !strings.HasPrefix(got, "script-predecessor-state") {
		t.Errorf("an exported function: got %q", got)
	}
	for _, p := range []string{"declare -a arr", "declare OTHER=1", "readonly X=1", "export OTHER=1", "arr=(a b)", "(true)", "(true) && true",
		"export -p"} {
		if got := firstScriptReason(parseScript(p+" && bash /ws/ok.sh", chainFiles())); got != "" {
			t.Errorf("%q: unexpected block %q", p, got)
		}
	}
}

// bash 5.3 assigns `export -p NAME=v` and `readonly -p NAME=v`, and `export -p NAME` and `export -pf f` still export,
// so -p does not make export or readonly print-only. declare, typeset and local with -p assign nothing in bash 5.3; a
// value given to them is refused anyway, since that is not assumed of every bash.
func TestPrintFlagDoesNotExemptAnAssignment(t *testing.T) {
	refused := []string{"export -p BASH_ENV=/tmp/e", "export -p PATH=/tmp/e", "readonly -p PATH=/tmp/e", "export -pf echo",
		"export -fp echo", "export -p -f echo", "export -p -- PATH=/x", "readonly -p -- PATH=/x", "export -px PATH=/x", "export -p PATH",
		"export -p BASH_ENV", "export -pn PATH=/x", "readonly -p BASH_ENV", "readonly -pf echo", "builtin export -p BASH_ENV=/tmp/e",
		"declare -p PATH=/x", "typeset -p BASH_ENV=/x", "declare -px BASH_ENV=/x", "declare -p -- PATH=/x", "declare -p \"$n\"",
		"declare -gp PATH=/x", "declare -fx echo", "readonly -n BASH_ENV"}
	for _, p := range refused {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			if got := firstScriptReason(parseScript(p+"; "+call, chainFiles())); !strings.HasPrefix(got, "script-predecessor-state") {
				t.Errorf("%q then %q: got %q", p, call, got)
			}
		}
	}
	// local is only valid in a function, and a typed function definition is refused on its own, so it is judged directly.
	for _, args := range [][]string{{"-p", "PATH=/x"}, {"-px", "BASH_ENV=/x"}, {"-x", "PATH"}} {
		if !declaresState("local", literalWords(args)) {
			t.Errorf("local %v: not state", args)
		}
	}
	for _, args := range [][]string{{"-p"}, {"-p", "PATH"}, {"-pf", "echo"}} {
		if declaresState("local", literalWords(args)) {
			t.Errorf("local %v: state", args)
		}
	}
	allowed := []string{"declare -p", "export -p", "readonly -p", "declare -p PATH", "typeset -p BASH_ENV", "declare -pf echo", "typeset -fp echo",
		"declare -p -f", "declare -gp PATH", "declare -ip PATH", "declare -p -- PATH", "export -pn PATH", "export -p OTHER=1", "readonly -p X=1",
		"declare -p OTHER=1", "export -p -- OTHER=1"}
	for _, p := range allowed {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			if got := firstScriptReason(parseScript(p+"; "+call, chainFiles())); got != "" {
				t.Errorf("%q then %q: unexpected block %q", p, call, got)
			}
		}
	}
}

// A name reference makes a later plain assignment land on another variable, and some builtins assign a variable named
// by an operand. bash 5.3: `declare -n r; r=PATH; r=/x` assigns PATH; read, printf -v, mapfile, getopts and let assign.
func TestBuiltinsThatAssignByNameAreState(t *testing.T) {
	refused := []string{"declare -n r=PATH", "declare -n r", "typeset -n r=BASH_ENV", "declare -gn r=X", "declare -n -- r",
		"read PATH < f", "read -r -a PATH < f", "read \"$v\" < f", "printf -v PATH x", "printf -vPATH x", "printf -v \"$v\" x",
		"printf \"$o\" PATH x", "mapfile PATH < f", "readarray -t BASH_ENV < f", "getopts a PATH", "let PATH=1", "let \"$e\"",
		"unset PATH", "unset -v GIT_DIR", "hash -p /tmp/e bash", "enable -n echo", "enable -f x.so y", "read PROMPT_COMMAND < f"}
	for _, p := range refused {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			if got := firstScriptReason(parseScript(p+"; "+call, chainFiles())); !strings.HasPrefix(got, "script-predecessor-state") {
				t.Errorf("%q then %q: got %q", p, call, got)
			}
		}
	}
	if !declaresState("local", literalWords([]string{"-n", "r=PATH"})) {
		t.Error("local -n: not state")
	}
	allowed := []string{"declare -pn r", "export -n PATH", "read -r x < f", "read -p 'name: ' x < f", "mapfile -t lines < f",
		"printf '%s\\n' x", "printf -v out '%s' x", "getopts ab opt", "let i=1", "unset x", "unset -f mytool", "hash", "hash -r", "enable",
		"enable -a"}
	for _, p := range allowed {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			if got := firstScriptReason(parseScript(p+"; "+call, chainFiles())); got != "" {
				t.Errorf("%q then %q: unexpected block %q", p, call, got)
			}
		}
	}
}

// allexport (set -a) exports every later assignment and keyword (set -k) moves NAME=value words anywhere in a command
// into its environment (bash 5.3: `set -a; : ${ZY:=2}; ((ZW=3))` exports both). A default-value expansion and an
// arithmetic assignment set a pinned name without an assignment word, and an unquoted heredoc body is expanded too.
func TestShellOptionsAndExpansionsThatAssignState(t *testing.T) {
	refused := []string{"set -a", "set -o allexport", "set -oallexport", "set -eo allexport", "set -ka", "set -k", "set -o keyword",
		"set -ek", "set \"$o\"", "set -o \"$n\"", "builtin set -a", ": ${BASH_ENV:=/tmp/e}", ": \"${LD_PRELOAD:=/tmp/x.so}\"",
		": ${PATH=/tmp/e}", "echo ${GIT_DIR:=/x}", "X=${BASH_ENV:=/x}", "echo x > \"${BASH_ENV:=/x}\"", "((BASH_ENV=1))",
		"(( BASH_ENV = 1 ))", "echo $((PATH=1))", "((PATH+=1))", "((PROMPT_COMMAND=1))", "x=$((LD_PRELOAD=1))",
		"set -a; : ${BASH_ENV:=/tmp/e}", "set -o allexport; : \"${LD_PRELOAD:=/tmp/x.so}\"", "cat <<EOF\n${BASH_ENV:=/x}\nEOF\n:",
		// A + option does not end the options, and shopt -o is the set -o namespace (bash 5.3 turns allexport on in each).
		"set +x -a", "set +o pipefail -a", "set +o pipefail -o allexport", "set -o errexit -o allexport", "set -ea", "set -e -a",
		"shopt -so allexport", "shopt -o -s allexport", "shopt -s -o allexport", "shopt -so keyword", "shopt -os allexport",
		"shopt -s expand_aliases", "shopt -s cdable_vars", "shopt -s \"$n\"", "shopt -s nullglob -o allexport", "shopt -su nullglob",
		"shopt -u interactive_comments", "set +o interactive-comments", "set -o posix", "set -P", "set -o physical", "set -r",
		"set -o \"$n\" -e", "set -o nosuchoption", "set -Z", "set x", "set -", "set -o -a", "set -oo pipefail allexport", "shopt -z",
		// The variable an indirect default names is built at run time; $[ ] is arithmetic; so are [[ ]] and subscripts.
		"x=BASH_ENV; : ${!x:=/tmp/e}", ": ${!x=/tmp/e}", ": ${!x[0]:=/tmp/e}", "echo $[ BASH_ENV=1 ]", "echo $[PATH+=1]",
		"[[ 1 -eq BASH_ENV=1 ]]", "a[BASH_ENV=1]=x", "echo ${a[PATH=1]}", "declare -i n", "typeset -ai n=1",
		"shopt -so allexport; x=BASH_ENV; : ${!x:=/tmp/e}", "set +x -a; x=BASH_ENV; : ${!x:=/tmp/e}",
		"shopt -so allexport; echo $[ BASH_ENV=1 ]"}
	for _, p := range refused {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			got := firstScriptReason(parseScript(p+"; "+call, chainFiles()))
			// [[ and a subscript already leave a relative call without a proven directory, which is checked first.
			if !strings.HasPrefix(got, "script-predecessor-state") && !(call == "bash ./ok.sh" && strings.HasPrefix(got, "script-cwd-unproven")) {
				t.Errorf("%q then %q: got %q", p, call, got)
			}
		}
	}
	allowed := []string{": ${FOO:=bar}", "((n=1))", "(( n = 1 ))", "set -e", "set -u", "set -o pipefail", "set -euo pipefail", "set -x",
		"set +a", "set +o allexport", "set -o", ": ${PATH:-x}", "echo ${BASH_ENV:-none}", "((n==1))", "echo $((1+2))", "echo $((n=PATH))",
		"cat <<'EOF'\nhello\nEOF\n:", "set -euxo pipefail", "set -eu", "set -ex", "set -o nounset -o errexit", "set +o", "set",
		"set -Ceufo pipefail", "set +e", "set -o pipefail +x", "set -oo pipefail errexit", "set -o interactive-comments", "set +P", "set +o posix",
		"shopt -s nullglob", "shopt -s extglob globstar dotglob", "shopt -u nullglob", "shopt -u expand_aliases", "shopt", "shopt -p",
		"shopt -q extglob", "shopt -o allexport", "shopt -po", "shopt -uo allexport", "shopt -so pipefail", "shopt -s lastpipe inherit_errexit",
		": ${!x}", "echo ${!x:-d}", "echo $[1+2]", "echo $[n=1]", "echo ${a[i+1]}", "declare -a arr"}
	// The directory proof already refuses a call after any [[ ]] test or subscript assignment, so the text rule for
	// them is judged directly.
	for _, text := range []string{"[[ $x == y ]]", "[[ x = y ]]", "a[0]=x", "a[i+1]=x", "[[ n -eq m=1 ]]"} {
		if textAssignsState(text) {
			t.Errorf("%q: counted as an assignment of a pinned name", text)
		}
	}
	for _, text := range []string{"[[ 1 -eq BASH_ENV=1 ]]", "a[BASH_ENV=1]=x", "echo $[PATH=1]", "x=BASH_ENV; : ${!x:=v}"} {
		if !textAssignsState(text) {
			t.Errorf("%q: not counted as an assignment of a pinned name", text)
		}
	}
	for _, p := range allowed {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			if got := firstScriptReason(parseScript(p+"; "+call, chainFiles())); got != "" {
				t.Errorf("%q then %q: unexpected block %q", p, call, got)
			}
		}
	}
}

func literalWords(texts []string) []Word {
	out := make([]Word, len(texts))
	for i, s := range texts {
		out[i] = Word{Text: s, Raw: s, Literal: true}
	}
	return out
}

func TestSelfReferenceIdioms(t *testing.T) {
	host := pathshape.For("linux")
	wantLinesRefused(t, host, []string{
		`echo x >> "${BASH_SOURCE[0]:-$0}"`, `echo x >> "${BASH_SOURCE:-$0}"`, `echo x >> "${0:-x}"`, `echo x >> "${F:-$0}"`,
		`echo x >> "$(realpath "$0")"`, `echo x >> "$(readlink -f "$0")"`, `echo x >> "$(dirname "$0")/$(basename "$0")"`,
		`echo x >> "$(basename "$0")"`, `tee -a "$(realpath "${BASH_SOURCE[0]}")"`,
	})
	wantLinesAllowed(t, host, []string{`echo x >> "${0}.bak"`, `echo x >> "$(dirname "$0")/out"`, `echo x >> "$(dirname "$0")"`,
		`echo x >> "${BASH_SOURCE%/*}/out"`, `echo x >> "${F:-out}"`})
}

func TestDownloadsToTheWorkingDirectoryAreWrites(t *testing.T) {
	host := pathshape.For("linux")
	for _, p := range []string{"curl -O u", "curl -sSLO u", "curl --remote-name u", "curl --remote-name-all u", "curl -J -O u"} {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			if got := firstScriptReason(parseScriptOn(host, p+" && "+call, chainFiles())); !strings.HasPrefix(got, "script-predecessor-may-write") {
				t.Errorf("%q then %q: got %q", p, call, got)
			}
		}
	}
	wantLinesRefused(t, host, []string{"curl -O http://h/t.sh", "curl -sSLO http://h/p/t.sh", "curl --remote-name http://h/p/t.sh?x=1#y",
		"curl --remote-name-all http://h/t.sh", "wget http://h/t.sh", "wget -q http://h/a/t.sh", "wget -Ot.sh u", "curl -ot.sh u",
		"wget -O t.sh u", "curl -o t.sh u"})
	wantLinesAllowed(t, host, []string{"curl -O http://h/x.tgz", "wget http://h/x.tgz", "curl -sS http://h/t.sh", "wget -O out.txt http://h/t.sh",
		"curl -o out.txt http://h/t.sh"})
}

func TestMoreWriteForms(t *testing.T) {
	host := pathshape.For("linux")
	wantLinesRefused(t, host, []string{
		"patch t.sh < p", "patch -p1 /ws/t.sh", "git checkout -- t.sh", "git restore t.sh", "git -C /ws checkout -- t.sh",
		"git checkout .", "git restore ./", "git restore --staged --worktree t.sh", "ln -t /ws /tmp/e/t.sh", "ln -sf /tmp/e/t.sh /ws/", "ln -sf /tmp/e/x /ws/t.sh",
		"cp -r /tmp/x/. .", "rsync -a /tmp/x/ ./", "cp /tmp/x/* .", "cp /tmp/x/* /ws", "cp -r /tmp/x/. /ws/", "mv /tmp/x/* /ws",
		"cp \"$src\"/* /ws", "tar -x -C /ws -f a", "tar xf a", "tar --extract --directory=/ws -f a", "unzip -d /ws a.zip", "unzip a.zip",
		"echo x >> ~/t.sh", "echo x >> $HOME/t.sh",
	})
	wantLinesAllowed(t, host, []string{"cp -r /tmp/x /tmp/y", "cp /tmp/x/* /tmp", "cp -r /tmp/x/. /tmp/y", "rsync -a /tmp/x/ /tmp/y/", "tar -tf a",
		"tar -x -C /tmp/out -f a", "unzip -d /tmp/out a.zip", "unzip -l a.zip", "ln -s /ws/t.sh l", "git checkout main", "git checkout -b topic", "git reset t.sh", "git restore --staged t.sh", "git restore --staged .", "git rm -r --cached .",
		"git status", "patch -p1", "echo x >> ~/log", "cp -r /tmp/x /ws/sub"})
}

// The compared form: a / boundary, so s/t.sh is not the end of /ws/t.sh.
func TestWriteSuffixKeepsAComponentBoundary(t *testing.T) {
	host := pathshape.For("linux")
	wantLinesAllowed(t, host, []string{"echo x >> s/t.sh", "echo x >> ws/t.shx", "echo x >> xt.sh"})
	wantLinesRefused(t, host, []string{"echo x >> ws/t.sh"})
}

func TestWriteHostSpellings(t *testing.T) {
	host := pathshape.For("darwin")
	files := fakeReader{"/ws/t.sh": {ID: "1", Text: "echo x >> /WS/T.SH\n"}}
	if got := firstScriptReason(parseScriptOn(host, "bash /ws/t.sh", files)); !strings.HasPrefix(got, "script-writes-inspected") {
		t.Errorf("darwin folds case: got %q", got)
	}
	host = pathshape.For("windows")
	for _, line := range []string{`echo x >> 't.sh.'`, `echo x >> 't.sh '`, `echo x >> 'T~1.SH'`, `echo x >> 't.sh:s'`, `echo x >> 'C:/ws/t.sh::$DATA'`} {
		files := fakeReader{"C:/ws/t.sh": {ID: "1", Text: line + "\n"}}
		if got := firstScriptReason(parseScriptOn(host, "bash C:/ws/t.sh", files)); !strings.HasPrefix(got, "script-writes-inspected") {
			t.Errorf("windows %q: got %q", line, got)
		}
	}
	files = fakeReader{"C:/ws/t.sh": {ID: "1", Text: "echo x >> out.txt\n"}}
	if got := firstScriptReason(parseScriptOn(host, "bash C:/ws/t.sh", files)); got != "" {
		t.Errorf("windows plain target: got %q", got)
	}
}

func TestFunctionDefinitionOpener(t *testing.T) {
	for raw, want := range map[string]bool{"f": true, " name ": true, "function g": true, "echo hi": true,
		"then": false, " do ": false, "if": false, "elif": false, "while": false, "until": false, "!": false, "for": false, "time": false,
		"arr=": false, "arr+=": false, "ls @": false, "": false, "  ": false} {
		if got := funcDefBefore(raw); got != want {
			t.Errorf("%q: got %v", raw, got)
		}
	}
}
