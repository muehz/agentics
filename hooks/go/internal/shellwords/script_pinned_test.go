package shellwords

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

func hasRule(reasons []string, rule string) bool {
	for _, r := range reasons {
		if strings.HasPrefix(r, "script-opaque:"+rule+" in /ws/t.sh") {
			return true
		}
	}
	return false
}

func TestScriptPinnedAndShadowConstructs(t *testing.T) {
	marked := map[string]string{
		"PATH=/x cmd\n": "pinned-assignment", "export PATH=/x\n": "pinned-assignment", "env PATH=/x cmd\n": "pinned-assignment",
		"PATH=/x\n": "pinned-assignment", "GIT_DIR=/x\n": "pinned-assignment", "LD_PRELOAD=/x.so cmd\n": "pinned-assignment",
		"HOME=/x git status\n": "pinned-assignment", "TMPDIR=/x\n": "pinned-assignment",
		"IFS=,\n": "persistent-ifs", "export IFS\n": "persistent-ifs", "IFS=, mycmd\n": "ifs-prefix",
		"alias ll='ls'\n": "alias", "shopt -s expand_aliases\n": "alias",
		"rm() { :; }\n": "function-shadow", "function git { :; }\n": "function-shadow", "mktemp() { :; }\n": "function-shadow",
		"sqlite3() { :; }\n": "function-shadow", "function unlink { :; }\n": "function-shadow", "shred() { :; }\n": "function-shadow",
		"pwsh() { :; }\n": "function-shadow", "powershell() { :; }\n": "function-shadow",
		"printf -v \"$1\" x\n": "printf-v", "printf -v PATH x\n": "printf-v",
		"declare -n r=x\n": "nameref", "coproc x { :; }\n": "coproc", "(( a[$(cmd)] = 1 ))\n": "arithmetic-substitution",
		"PROMPT_COMMAND='git push'\n": "prompt-command", "command_not_found_handle() { :; }\n": "command-not-found-handle",
	}
	for body, rule := range marked {
		if got := scriptReasons(t, body, nil); !hasRule(got, rule) {
			t.Errorf("%q: want script-opaque:%s, got %v", body, rule, got)
		}
	}
	for _, body := range []string{"IFS= read -r line\n", "IFS=$'\\t' read -r a b\n", "LC_ALL=C sort </dev/null\n",
		"FLOW_WORK_ROOT=x git status\n", "printf -v out '%s' x\n", "f() { :; }\n", "x=1\n"} {
		if got := scriptReasons(t, body, nil); len(got) != 0 {
			t.Errorf("%q: unexpected marks %v", body, got)
		}
	}
}

// TestScriptPinnedRoutes covers the other ways to reach a pinned name or IFS, each of which must mark the script.
func TestScriptPinnedRoutes(t *testing.T) {
	marked := map[string]string{
		"declare -x PATH=/x\n": "pinned-assignment", "typeset PATH=/x\n": "pinned-assignment", "readonly PATH=/x\n": "pinned-assignment",
		"f() { local HOME=/x; }\n": "pinned-assignment", "export -n PATH\n": "pinned-assignment", "PATH+=:/x\n": "pinned-assignment",
		"export PATH+=:/x\n": "pinned-assignment", "unset PATH\n": "pinned-assignment", "read PATH\n": "pinned-assignment",
		"read -r -p prompt PATH\n": "pinned-assignment", "mapfile -t BASH_ENV\n": "pinned-assignment",
		"read \"$name\"\n": "pinned-assignment", "export \"$name=v\"\n": "pinned-assignment", "unset \"$name\"\n": "pinned-assignment",
		"printf -v 'a[$(cmd)]' x\n": "printf-v", "printf -v \"IFS\" x\n": "printf-v", "builtin printf -v PATH x\n": "printf-v",
		"builtin export PATH=/x\n": "pinned-assignment", "command export PATH=/x\n": "pinned-assignment",
		"PATH[0]=/x\n": "pinned-assignment", "export XDG_CONFIG_HOME=/x\n": "pinned-assignment",
		"env -u PATH cmd\n": "pinned-assignment", "PYTHONPATH=/x python3 t.py\n": "pinned-assignment",
		"read IFS\n": "persistent-ifs", "unset IFS\n": "persistent-ifs", "local IFS=,\n": "persistent-ifs",
		"IFS+=, \n": "persistent-ifs", "declare IFS=,\n": "persistent-ifs",
		"IFS=, bash -c 'x'\n": "ifs-prefix", "IFS= cat\n": "ifs-prefix",
		"typeset -n r=x\n": "nameref", "local -n r=x\n": "nameref", "declare -nx r=x\n": "nameref",
		"export PROMPT_COMMAND='x'\n": "prompt-command", "PROMPT_COMMAND+=';x'\n": "prompt-command",
		"function rm() { :; }\n": "function-shadow", "[() { :; }\n": "function-shadow", "if x; then cd() { :; }; fi\n": "function-shadow",
		"rm ()\n{ :; }\n": "function-shadow", "a() { :; }; sudo() { :; }\n": "function-shadow", ".() { :; }\n": "function-shadow",
		"let 'a[$(cmd)]=1'\n": "arithmetic-substitution", "[[ 1 -eq a[`cmd`] ]]\n": "arithmetic-substitution",
		"function command_not_found_handle { :; }\n": "command-not-found-handle",
		"shopt -s expand_aliases; alias x=y\n":       "alias",
		"f() { PATH=/x; }\n":                         "pinned-assignment", "if x; then PATH=/x; fi\n": "pinned-assignment", "{ PATH=/x; }\n": "pinned-assignment",
		"( PATH=/x )\n": "pinned-assignment", "for i in 1; do IFS=,; done\n": "persistent-ifs",
		"x=$(PATH=/x)\n": "pinned-assignment", "bash -c 'PATH=/x'\n": "pinned-assignment",
	}
	for body, rule := range marked {
		if got := scriptReasons(t, body, nil); !hasRule(got, rule) {
			t.Errorf("%q: want script-opaque:%s, got %v", body, rule, got)
		}
	}
	for _, body := range []string{"read -r line\n", "read -r -p \"$prompt\" answer\n", "unset x\n", "declare -r X=1\n", "local n=1\n",
		"export FOO=1\n", "export FOO=\"$x\"\n", "LC_ALL=C sort </dev/null\n", "A=1 B=2 git status\n", "mapfile -t lines\n",
		"printf -v 'arr[0]' x\n", "printf -v out \"%s\" x\n", "g() { :; }\n", "log_cmd() { :; }\n", "declare -a arr\n",
		"# rm() { :; }\necho ok\n", "(( n = 1 + 2 ))\n", "echo $(( a[1] + 2 ))\n", "IFS= read -r a <<< x\n",
		"export Path=/x\n", "path=/x\n", "echo PATH=x\n"} {
		if got := scriptReasons(t, body, nil); len(got) != 0 {
			t.Errorf("%q: unexpected marks %v", body, got)
		}
	}
}

// TestScriptPinnedLocationAndAttribution checks that a bare assignment is attributed to the script it appears in, on
// its own line, and counted once however deep the script call is nested.
func TestScriptPinnedLocationAndAttribution(t *testing.T) {
	files := fakeReader{
		"/ws/t.sh":     {Text: "echo ok\nbash /ws/inner.sh\nX=1\nHOME=/y\n"},
		"/ws/inner.sh": {Text: "echo in\n\nPATH=/x\n"},
	}
	res := parseScript("bash /ws/t.sh", files)
	var got []string
	for _, inv := range res.Invocations {
		if !inv.Inspectable {
			got = append(got, inv.Reason.String())
		}
	}
	want := []string{
		"script-opaque:pinned-assignment in /ws/inner.sh:3 (via /ws/t.sh)",
		"script-opaque:pinned-assignment in /ws/t.sh:4",
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, inv := range res.Invocations {
		if !inv.Inspectable && (inv.Raw != "" || len(inv.Wrappers) != len(inv.WrapperIDs)) {
			t.Errorf("synthetic child leaks text or misaligns wrappers: raw %q wrappers %d ids %d", inv.Raw, len(inv.Wrappers), len(inv.WrapperIDs))
		}
	}
	if len(res.BareAssignments) != 3 {
		t.Errorf("bare assignments must stay recorded for the guards, got %d", len(res.BareAssignments))
	}
}

// TestScriptRepositoryHelperHasNoOpaqueConstruct parses the shipped execute helper as a script call and fails when any
// rule marks it, so a new rule cannot silently block the helper the workflow depends on. Only reasons and locations
// are printed, never the script text.
func TestScriptRepositoryHelperHasNoOpaqueConstruct(t *testing.T) {
	path := filepath.Join(hosttest.Checkout(t), "hooks", "go", "internal", "shellwords", "testdata", "flow.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("helper not readable: %v", err)
	}
	text := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	res := parseScript("bash /ws/hooks/go/internal/shellwords/testdata/flow.sh status", fakeReader{"/ws/hooks/go/internal/shellwords/testdata/flow.sh": {Text: text}})
	if len(res.Scripts) == 0 {
		t.Fatalf("the helper was not read as a script")
	}
	for _, inv := range res.Invocations {
		if !inv.Inspectable {
			t.Errorf("helper refused: %s", inv.Reason)
		}
	}
}

// TestScriptPinnedPrefixOnNestedCall checks that a pinned prefix on a script call made from a script is reported once,
// at the line of the call in the calling script, and not once for each command of the nested script.
func TestScriptPinnedPrefixOnNestedCall(t *testing.T) {
	files := fakeReader{
		"/ws/inner.sh": {Text: "echo in\necho again\n"},
		"/ws/lib.sh":   {Text: "echo lib\necho more\n"},
	}
	for body, rule := range map[string]string{
		"echo a\nPATH=/x bash /ws/inner.sh\n":     "pinned-assignment",
		"echo a\nPATH=/x source /ws/lib.sh\n":     "pinned-assignment",
		"echo a\nenv PATH=/x bash /ws/inner.sh\n": "pinned-assignment",
		"echo a\nPATH=/x env bash /ws/inner.sh\n": "pinned-assignment",
		"echo a\nenv -i bash /ws/inner.sh\n":      "pinned-assignment",
		"echo a\nenv -u HOME bash /ws/inner.sh\n": "pinned-assignment",
		"echo a\nIFS=, bash /ws/inner.sh\n":       "ifs-prefix",
	} {
		got := scriptReasons(t, body, files)
		if len(got) != 1 || !strings.HasPrefix(got[0], "script-opaque:"+rule+" in /ws/t.sh:2") || strings.Contains(got[0], "via") {
			t.Errorf("%q: want one %s at /ws/t.sh:2, got %v", body, rule, got)
		}
	}
	if got := scriptReasons(t, "echo a\nPATH=/x git status\n", nil); len(got) != 1 || !strings.HasPrefix(got[0], "script-opaque:pinned-assignment in /ws/t.sh:2") {
		t.Errorf("a prefix on a plain command: got %v", got)
	}
	if got := scriptReasons(t, "FOO=1 bash /ws/inner.sh\n", files); len(got) != 0 {
		t.Errorf("an unpinned prefix on a nested call: unexpected marks %v", got)
	}
}

func TestScriptCommentsAndDataAreNotShadows(t *testing.T) {
	for _, body := range []string{"# rm() { :; }\necho ok\n", "echo done # see function git\n", "cat <<'EOF'\nrm() { :; }\nEOF\necho ok\n"} {
		if got := scriptReasons(t, body, nil); len(got) != 0 {
			t.Errorf("%q: unexpected marks %v", body, got)
		}
	}
	got := scriptReasons(t, "echo done # see function git\nrm() { :; }\n", nil)
	if len(got) != 1 || !strings.HasPrefix(got[0], "script-opaque:function-shadow in /ws/t.sh:2") {
		t.Errorf("a real definition after a comment: got %v", got)
	}
}

// TestScriptAssignmentRoutes covers each way besides a plain assignment to set a variable, for a pinned name and for IFS.
func TestScriptAssignmentRoutes(t *testing.T) {
	routes := []string{
		"for %s in /x; do :; done\n", "select %s in a b; do :; done\n", "getopts a %s\n", "let %s=0\n", "let 'x=1' '%s = 0'\n",
		"(( %s = 0 ))\n", "(( %s += 1 ))\n", "for (( %s=0; %s<1; %s++ )); do :; done\n", "read -pt %s <<< /x\n", "read -a%s <<< /x\n",
		"read -rd '' %s <<< /x\n", ": \"${%s:=/x}\"\n", ": ${%s=v}\n", ": \"${%s[0]:=/x}\"\n", "exec {%s}>/dev/null\n",
		"wait -n -p %s\n", "wait -p%s\n", "mapfile -tn1 %s\n",
	}
	for name, rule := range map[string]string{"PATH": "pinned-assignment", "TMPDIR": "pinned-assignment", "IFS": "persistent-ifs"} {
		for _, route := range routes {
			body := strings.ReplaceAll(route, "%s", name)
			if got := scriptReasons(t, body, nil); !hasRule(got, rule) {
				t.Errorf("%q: want script-opaque:%s, got %v", body, rule, got)
			}
		}
	}
	for _, name := range []string{"PATH", "IFS"} {
		if got := scriptReasons(t, "printf -v"+name+" /x\n", nil); !hasRule(got, "printf-v") {
			t.Errorf("attached printf -v%s: got %v", name, got)
		}
	}
	for _, body := range []string{"for i in /x; do :; done\n", "select i in a; do :; done\n", "getopts a opt\n", "let n=0\n", "(( n = 0 ))\n",
		"(( n == 0 ))\n", "(( n <= 3 ))\n", "read -pt line <<< /x\n", "read -ap arr <<< /x\n", ": \"${x:=/x}\"\n", "echo ${HOME}>/dev/null\n",
		"exec {fd}>/dev/null\n", "wait -n -p pid\n", "printf -vout /x\n", "mapfile -tn1 lines\n", "echo \"${PATH:-/x}\"\n"} {
		if got := scriptReasons(t, body, nil); len(got) != 0 {
			t.Errorf("%q: unexpected marks %v", body, got)
		}
	}
}

// TestScriptCommandResolutionRoutes covers the ways to change what a command name runs or to run a string as a command
// without naming a pinned variable.
func TestScriptCommandResolutionRoutes(t *testing.T) {
	marked := map[string]string{
		"hash -p /x/m mktemp\n": "pinned-assignment", "hash -p/x/m mktemp\n": "pinned-assignment", "hash -rp /x/m mktemp\n": "pinned-assignment", "BASH_CMDS[mktemp]=/x/m\n": "pinned-assignment",
		"BASH_ALIASES[ls]=x\n": "pinned-assignment", "enable -n cd\n": "pinned-assignment", "enable\n": "pinned-assignment",
		"mapfile -C 'cmd' -c 1\n": "eval", "readarray -C cmd\n": "eval", "mapfile -tCcmd\n": "eval", "builtin mapfile -C cmd\n": "eval",
		"env -i cmd\n": "pinned-assignment", "mktemp.exe() { :; }\n": "function-shadow", "git.exe() { :; }\n": "function-shadow",
	}
	for body, rule := range marked {
		if got := scriptReasons(t, body, nil); !hasRule(got, rule) {
			t.Errorf("%q: want script-opaque:%s, got %v", body, rule, got)
		}
	}
	for _, body := range []string{"hash -r\n", "hash\n", "hash git\n", "if ! hash jq 2>/dev/null; then :; fi\n", "mapfile -t lines\n", "mapfile -n 3 -c 1 lines\n", "env FOO=1 cmd\n"} {
		if got := scriptReasons(t, body, nil); len(got) != 0 {
			t.Errorf("%q: unexpected marks %v", body, got)
		}
	}
}

// TestScriptArithmeticSubscriptsOnly checks that only a subscript attached to a name inside an arithmetic context counts.
func TestScriptArithmeticSubscriptsOnly(t *testing.T) {
	for _, body := range []string{
		"if [ \"$n\" -eq 0 ] && [ -z \"$(git status --porcelain)\" ]; then :; fi\n",
		"(( n > 0 )) && [ -n \"$(git status --porcelain)\" ]\n",
		"[ \"$n\" -eq 0 ] && echo \"${a[1]} $(date)\"\n",
	} {
		if got := scriptReasons(t, body, nil); len(got) != 0 {
			t.Errorf("%q: unexpected marks %v", body, got)
		}
	}
	for _, body := range []string{"(( a[$(cmd)] = 1 ))\n", "let 'a[$(cmd)]=1'\n", "[[ 1 -eq a[`cmd`] ]]\n", "x=$(( a[$(cmd)] + 1 ))\n"} {
		if got := scriptReasons(t, body, nil); !hasRule(got, "arithmetic-substitution") {
			t.Errorf("%q: want arithmetic-substitution, got %v", body, got)
		}
	}
}

// TestScriptHeredocApostropheDoesNotHideCode checks that an apostrophe inside a heredoc body cannot make a later
// " #" on a code line look like the start of a comment.
func TestScriptHeredocApostropheDoesNotHideCode(t *testing.T) {
	heredoc := "cat <<EOF\nit's\nEOF\n"
	for body, rule := range map[string]string{
		heredoc + "echo ' #'; mktemp() { echo /; }\n":               "function-shadow",
		heredoc + "echo ' #'; (( PATH = 0 ))\n":                     "pinned-assignment",
		heredoc + "echo ' #'; PATH=/x\n":                            "pinned-assignment",
		"cat <<-EOF\n\tit's\n\tEOF\necho ' #'; rm() { :; }\n":       "function-shadow",
		"cat <<EOF $(date)\nit's\nEOF\necho ' #'; rm() { :; }\n":    "function-shadow",
		"cat <<EOF\nit's\necho ' #'; rm() { :; }\n":                 "function-shadow",
		"cat <<EOF\nit's\nEOF\ncat <<'X'\nrm() {\nX\nrm() { :; }\n": "function-shadow",
	} {
		got := scriptReasons(t, body, nil)
		if len(got) == 0 || !hasRule(got, rule) {
			t.Errorf("%q: want %s, got %v", body, rule, got)
		}
	}
	if got := scriptReasons(t, heredoc+"echo ' #'; PATH=/x\n", nil); len(got) != 1 || !strings.HasSuffix(got[0], "/ws/t.sh:4") {
		t.Errorf("a bare assignment after a heredoc must carry its line, got %v", got)
	}
	for _, body := range []string{heredoc + "echo done # see function git\n", "cat <<'EOF'\nit's\nrm() { :; }\nEOF\n"} {
		if got := scriptReasons(t, body, nil); len(got) != 0 {
			t.Errorf("%q: unexpected marks %v", body, got)
		}
	}
}

// TestScriptQuotedHashLineIsScanned checks that a line beginning with # inside a string that spans lines is not taken
// for a comment, so the code after the closing quote on that line is scanned, both when the heredoc view is reliable
// and when the here-string earlier in the script makes the scan read the script as written.
func TestScriptQuotedHashLineIsScanned(t *testing.T) {
	for _, quote := range []string{`"`, `'`} {
		body := "x=" + quote + "\n#" + quote + "; mktemp() { echo /; }\n"
		for _, prefix := range []string{"", "read -r a <<< x\n"} {
			if prefix != "" && newScriptLines(prefix+body).scan != prefix+body {
				t.Fatalf("the here-string must make the scan read the script as written")
			}
			if got := scriptReasons(t, prefix+body, nil); !hasRule(got, "function-shadow") {
				t.Errorf("%q: want function-shadow, got %v", prefix+body, got)
			}
		}
	}
}

// scanRow is one script of the hiding matrix. rule is the construct the script must be marked with, or "" when it
// must not be marked; line, when set, is the line the mark must carry. over marks a row whose construct bash never
// runs, where a mark is an accepted over-refusal and not a hiding the oracle can confirm.
type scanRow struct {
	body string
	rule string
	line int
	over bool
}

// shadowScanMatrix holds the ways a quote, comment or heredoc state error could hide a function definition or an
// assignment from the text scan, each followed by the construct, and the comments and data that must not be marked.
func shadowScanMatrix() []scanRow {
	heredocs := []string{
		"cat <<EOF\nit's\nEOF\n", "cat <<EOF\n# it's\nEOF\n", "cat <<-EOF\n\tit's\n\tEOF\n", "cat <<EOF $(echo)\nit's\nEOF\n",
		"cat <<'A' <<B\nit's\nA\nit's\nB\n", "f() {\ncat <<EOF\nit's\nEOF\n}\n", "cat <<\"EOF\"\nit's\nEOF\n",
		"cat <<\\EOF\nit's\nEOF\n", "cat <<< \"it's\"\n", "cat <<< it\\'s\n", "case x in x) cat <<EOF\nit's\nEOF\n;; esac\n",
		"x=$(cat <<EOF\nit's\nEOF\n)\n", "x=$(\ncat <<EOF\nit's\nEOF\n)\n", "cat <<EOF\n\"\nEOF\n", "x=\"$(cat <<EOF\nit's\nEOF\n)\"\n",
		"cat <<EOF | cat\nit's\nEOF\n", "cat <<EOF; echo \"it's\"\nit's\nEOF\n", "cat <<EOF\n$(echo ')')\nEOF\n",
	}
	var rows []scanRow
	for _, h := range heredocs {
		next := strings.Count(h, "\n") + 1
		rows = append(rows,
			scanRow{body: h + "echo ' #'; mktemp() { echo /; }\n", rule: "function-shadow", line: next},
			scanRow{body: h + "echo \" #\"; rm() { :; }\n", rule: "function-shadow", line: next},
			scanRow{body: h + "echo ' #'; (( PATH = 0 ))\n", rule: "pinned-assignment", line: next},
			scanRow{body: h + "echo ' #'; PATH=/x\n", rule: "pinned-assignment", line: next},
		)
	}
	for _, body := range []string{
		// A string that spans lines, with a # line, an apostrophe or a heredoc-like line inside it.
		"x=\"\n#\"; mktemp() { echo /; }\n", "x='\n#'; mktemp() { echo /; }\n",
		"read -r a <<< x\nx=\"\n#\"; mktemp() { echo /; }\n", "read -r a <<< x\nx='\n#'; mktemp() { echo /; }\n",
		"x=\"\n# it's\n\"; rm() { :; }\n", "x='\n# say \"hi\n'; rm() { :; }\n", "x=\"a\n#b\nc\n#d\"; rm() { :; }\n",
		"x=\"\ncat <<EOF\nit's\nEOF\n#\"; rm() { :; }\n", "x='\ncat <<EOF\n#\nEOF\n'; rm() { :; }\n",
		"x=\"\ncat <<'E'\n\"; rm() { :; }\nE\n",
		// Command substitutions, alone and inside double quotes, where the quoting starts afresh.
		"x=\"$(echo \" # \")\"; rm() { :; }\n", "x=\"$(echo ' # ')\"; rm() { :; }\n", "x=$(echo \")#\"); rm() { :; }\n",
		"x=$(echo a # )\n); rm() { :; }\n", "x=$(case a in a) echo \" # \";; esac); rm() { :; }\n",
		"echo $(echo $(echo ' # ')); rm() { :; }\n", "echo \"$(echo \"$(echo \" # \")\")\"; rm() { :; }\n",
		// Arithmetic, where # is never a comment.
		"echo $(( 1 << 2 )); rm() { :; }\n",
		// Parameter expansions.
		"echo ${x#*/}; rm() { :; }\n", "echo ${#x} #c\nrm() { :; }\n", "echo \"${x:-\" # \"}\"; rm() { :; }\n",
		"echo \"${x:-' # '}\"; rm() { :; }\n", "echo ${x:-'}'}; rm() { :; }\n", "echo ${x:-' # '}; rm() { :; }\n",
		"echo $# ; rm() { :; }\n",
		// A backslash-newline before #, and an escaped #.
		"echo a\\\n#x; rm() { :; }\n", "echo \\#x; rm() { :; }\n",
		// Backticks.
		"x=`echo # `; rm() { :; }\n", "x=\"`echo \" # \"`\"; rm() { :; }\n", "x=`echo '#'`; rm() { :; }\n",
		// One quote inside the other, ANSI-C strings and escaped quotes.
		"echo \"it's # here\"; rm() { :; }\n", "echo 'say \"#\"'; rm() { :; }\n", "echo $'it\\'s #'; rm() { :; }\n",
		"x=$'\\''\necho ' #'; rm() { :; }\n", "echo \"$'\" #c\nrm() { :; }\n", "echo it\\'s # c\nrm() { :; }\n",
		"x='a'\\''b # c'; rm() { :; }\n", "echo \\\"; x=\" # \"; rm() { :; }\n", "echo \"a\\\" # \"; rm() { :; }\n",
		// A # after an operator, inside [[ ]], after an extended glob or an array paren.
		"echo a&&#x\nrm() { :; }\n", "echo a;#x\nrm() { :; }\n", "[[ x =~ (#) ]]; rm() { :; }\n",
		"[[ x == @(#|y) ]]; rm() { :; }\n", "[[ x =~ a|#b ]]; rm() { :; }\n", "shopt -s extglob\necho @(a|#b); rm() { :; }\n",
		"a=(#x\n); rm() { :; }\n", "f() { # note\nrm() { :; }\n}; f\n",
		// A definition split by a backslash-newline.
		"mk\\\ntemp() { :; }\n", "rm \\\n() { :; }\n", "function \\\ngit { :; }\n", "rm(\\\n) { :; }\n",
		// Heredoc introductions that bash does not honor, and one in a comment.
		"echo \"<<'E'\"\nrm() { :; }\nE\n", "echo \"${x:-\"<<'E'\"}\"\nrm() { :; }\nE\n", "echo x #<<'E'\nrm() { :; }\nE\n",
		"cat <<EOF # it's\nit's\nEOF\necho ' #'; rm() { :; }\n", "let x=1<<E\nit's\nE\necho ' #'; rm() { :; }\n",
		// A comment where its quotes would invert the state if it were read as code.
		"case x in a) # it's\n;; esac\necho ' #'; rm() { :; }\n", "x=$(\n# it's\necho a\n)\necho ' #'; rm() { :; }\n",
		"(( 1 ))#'\nx=' #'; rm() { :; }\n", "echo \"${x:-${y:-' # '}}\"; rm() { :; }\n",
	} {
		rows = append(rows, scanRow{body: body, rule: "function-shadow"})
	}
	for _, body := range []string{"echo $(( #x )); rm() { :; }\n", "(( #x )); rm() { :; }\n"} {
		rows = append(rows, scanRow{body: body, rule: "function-shadow", over: true})
	}
	// Arithmetic that spans lines, plainly or through a backslash-newline.
	for _, body := range []string{"((\nPATH = 0\n))\n", "x=$((\nPATH = 0\n))\n", "(( PA\\\nTH = 0 ))\n"} {
		rows = append(rows, scanRow{body: body, rule: "pinned-assignment"})
	}
	// The oracle cannot see a subscript substitution run, so these rows only bind the parser.
	for _, body := range []string{"((\na[$(:)] = 1\n))\n", "[[ 1 -eq\na[`:`] ]]\n"} {
		rows = append(rows, scanRow{body: body, rule: "arithmetic-substitution", line: 1, over: true})
	}
	rows = append(rows, scanRow{body: "cat <<'EOF'\nrm() { :; }\nEOF\nrm() { :; }\n", rule: "function-shadow", line: 4})
	for _, body := range []string{
		"echo done # see function git\n", "echo a;# x; rm() { :; }\n", " echo a \\\n#x; rm() { :; }\n",
		"echo it\\' # '; rm() { :; }\n", "echo 'a\\' # '; rm() { :; }\n", "cat <<'EOF'\nit's\nrm() { :; }\nEOF\n",
		"echo \"$x\" # rm() { :; }\n", "x=$(echo) # function git\n", "echo \"${x#*/}\" # rm() { :; }\n",
		"f() { # function git\n:; }\n", "echo ${#x} # rm() { :; }\n", "# rm() { :; }\necho ok\n",
		"cat <<EOF\nit's\nEOF\necho done # see function git\n", "x=\"a\" # rm() { :; }\n", "cat <<'EOF'\nrm() { :; }\nEOF\n",
		"x=\"\nmultiline\n\" # function git\n", "echo $(echo a)#'\nx=' #'; rm() { :; }\n",
		"case x in a) # it's\n;; esac\necho done # see function git\n",
	} {
		rows = append(rows, scanRow{body: body})
	}
	return rows
}

// TestBlankCommentsIsUnsureWhereShellsDiffer checks the cases the comment tracker must not vouch for although the bash
// on the test host reads them one way: heredoc bodies that do not match the operators before them, and constructs
// whose reading has varied between shell versions.
func TestBlankCommentsIsUnsureWhereShellsDiffer(t *testing.T) {
	body := func(text, line string) [][2]int {
		i := strings.Index(text, line)
		return [][2]int{{i, i + len(line)}}
	}
	for _, c := range []struct {
		text   string
		bodies [][2]int
	}{
		{"cat <<E\nx\nE\necho ok\n", nil},                                // an operator with no body
		{"cat\nx\nE\necho ok\n", body("cat\nx\nE\necho ok\n", "x\nE\n")}, // a body with no operator
		{"cat <<E <<F\nx\nE\necho ok\n", body("cat <<E <<F\nx\nE\necho ok\n", "x\nE\n")},
		{"cat <<E\necho a\nb\nF\n", body("cat <<E\necho a\nb\nF\n", "b\nF\n")}, // a body one line later than its operator
		{"x=\"<<E\nx\nE\n\"\n", body("x=\"<<E\nx\nE\n\"\n", "x\nE\n")},         // a body inside a string
		{"x=$(cat <<E\nx\nE\n)\n", body("x=$(cat <<E\nx\nE\n)\n", "x\nE\n")},   // a body inside $( )
		{"x=$(case a in a) echo;; esac # c\n)\n", nil},                         // case inside $( )
		{"x=`echo '#'`\n", nil}, {"x=`echo # c`\n", nil}, // quotes and comments in backticks
		{"echo \"${x:-${y:-'}'}}\"\n", nil}, {"echo $(( 'a' ))\n", nil}, {"echo $[ 1 ]\n", nil}, // quotes in ${ } and arithmetic
		{"x=$(\n# c\necho a\n)\n", nil}, {"[[ a == b ]] #c\n[[ x =~ a|#b ]]\n", nil}, {"a=(x #c\n)\n", nil},
		{"(( 1 ))#c\n", nil}, {"echo a >#c\n", nil}, {"x='\n", nil},
	} {
		if _, sure, _ := blankComments(c.text, c.bodies); sure {
			t.Errorf("%q: the tracker must not be sure", c.text)
		}
	}
	for _, text := range []string{"echo a # c\n", "x=\"$(echo \" # \")\" # c\n", "echo ${#x} ${x#y} $# # c\n", "f() { # c\n:; }\n",
		"case x in a) # c\n;; esac\n", "echo a;# c\n", "x=$'\\'' # c\n", "[[ a == b ]] # c\n", "echo \"${x:-\"}\"}\" # c\n"} {
		if _, sure, _ := blankComments(text, nil); !sure {
			t.Errorf("%q: the tracker should be sure", text)
		}
	}
	if _, sure, _ := blankComments("cat <<'E' <<F\na\nE\nb\nF\necho ok # c\n", [][2]int{{14, 18}, {18, 22}}); !sure {
		t.Errorf("two bodies after one line of two operators should be sure")
	}
	// Where the tracker and the bodies disagree about a heredoc, the line is reported; a body inside $( ) is only unsure.
	for _, c := range []struct {
		text, body string
		line       int
	}{
		{"(( a << E ))\nx\nE\n", "x\nE\n", 1}, {"echo $[ 1 << E ]\nx\nE\n", "x\nE\n", 1}, {"echo ${x:1<<E }\nx\nE\n", "x\nE\n", 1},
		{"x=\"\na\" <<E\nx\nE\n", "", 2}, {"x=`cat <<E `\nx\nE\n", "x\nE\n", 1}, {"[[ a =~ a<<E ]]\nx\nE\n", "x\nE\n", 1},
		{"echo a\ncat <<E; x=\"\nx\nE\n\"\n", "x\nE\n", 2},
		// The body may only start at a newline that ends the command in the operator's frame or an enclosing one: not
		// after a backslash-newline, not while a frame opened after the operator is open, and not for an operator
		// counted inside a paren opened in a word.
		{"cat <<E \\\n;\nx\nE\n", ";\nx\nE\n", 1}, {"cat <<E $(\n:\n)\nx\nE\n", ":\n)\nx\nE\n", 1},
		{"cat <<E <(\n:\n)\nx\nE\n", ":\n)\nx\nE\n", 1}, {"cat <<E ${zz:-$(\n:\n)}\nx\nE\n", ":\n)}\nx\nE\n", 1},
		{"x=$(: <<E) $(\n:\n)\nx\nE\n", ":\n)\nx\nE\n", 1}, {"a=(1 <<E )\nx\nE\n", "x\nE\n", 1},
		{"x=$(cat <<E $(\n:\n)\nx\nE\n)\n", ":\n)\nx\nE\n", 1}, {"echo a\ncat <<E \\\n;\nx\nE\n", ";\nx\nE\n", 2},
		// An operator inside a substitution that an arithmetic command holds as text.
		{"(( zz = $(cat <<E\nx\nE\n) ))\n", "x\nE\n", 1}, {"x=$(( $(cat <<E\nx\nE\n) ))\n", "x\nE\n", 1},
		// A frame that closed before the newline, a plain paren or a [[ ]] that stays open, and a list that goes on
		// after the body all read the body where the lexer does.
		{"x=$(: <<E)\nx\nE\n", "x\nE\n", 0}, {"cat <<E; (\nx\nE\n)\n", "x\nE\n", 0}, {"cat <<E; [[ a &&\nx\nE\nb ]]\n", "x\nE\n", 0},
		{"( cat <<E\nx\nE\n)\n", "x\nE\n", 0}, {"cat <<E |\nx\nE\ncat\n", "x\nE\n", 0},
		{"[ -n \"$(cat <<E\nx\nE\n)\" ]\n", "x\nE\n", 0}, {"[[ -n $(cat <<E\nx\nE\n) ]]\n", "x\nE\n", 0},
		{"x=$(cat <<E\nx\nE\n)\n", "x\nE\n", 0}, {"cat <<E\nx\nE\n", "x\nE\n", 0}, {"cat <<E\nx\n", "x\n", 0},
	} {
		var bodies [][2]int
		if c.body != "" {
			bodies = body(c.text, c.body)
		}
		if _, _, line := blankComments(c.text, bodies); line != c.line {
			t.Errorf("%q: heredoc disagreement at line %d, want %d", c.text, line, c.line)
		}
	}
}

// TestScriptShadowScanMatrix checks every row of the hiding matrix against the parser.
func TestScriptShadowScanMatrix(t *testing.T) {
	for _, row := range shadowScanMatrix() {
		got := scriptReasons(t, row.body, nil)
		switch {
		case row.rule == "" && len(got) != 0:
			t.Errorf("%q: unexpected marks %v", row.body, got)
		case row.rule != "" && !hasRule(got, row.rule):
			t.Errorf("%q: want %s, got %v", row.body, row.rule, got)
		case row.line > 0 && !hasRuleAt(got, row.rule, row.line):
			t.Errorf("%q: want %s at line %d, got %v", row.body, row.rule, row.line, got)
		}
	}
}

func hasRuleAt(reasons []string, rule string, line int) bool {
	want := "script-opaque:" + rule + " in /ws/t.sh:" + strconv.Itoa(line)
	for _, r := range reasons {
		if r == want || strings.HasPrefix(r, want+" ") {
			return true
		}
	}
	return false
}

// TestScriptShadowScanMatrixAgainstBash sources each matrix row with real bash and checks that a function of a
// guarded name defined, or PATH changed, is always marked, and that every row the matrix calls an attack really is
// one. The rows only define functions, print and set variables in the oracle's own shell.
func TestScriptShadowScanMatrixAgainstBash(t *testing.T) {
	o := requireOracle(t, oracleBash44())
	dir := t.TempDir()
	// cat stands in for the program of that name, so the rows that feed a heredoc to it run no external command.
	probe := "cat() { :; }; o=$PATH; source ./t.sh >/dev/null 2>&1 </dev/null; r=; for n in rm mktemp git; do declare -F $n >/dev/null && r=F; done; " +
		"[ \"$PATH\" = \"$o\" ] || r=${r}P; echo \"R:$r\""
	effect := func(body string) string {
		s := runProbe(t, o, dir, body, probe)
		if i := strings.LastIndex(s, "R:"); i >= 0 {
			return strings.TrimSpace(s[i+2:])
		}
		return "?"
	}
	// A bash that cannot source a file from the temp directory (a WSL launcher, say) is not an oracle.
	if got := effect("rm() { :; }\n"); got != "F" {
		t.Skipf("bash did not source a fixture from the temp directory (got %q)", got)
	}
	for _, row := range shadowScanMatrix() {
		got := effect(row.body)
		ran := strings.Contains(got, "F") || strings.Contains(got, "P")
		marked := len(scriptReasons(t, row.body, nil)) > 0
		if ran && !marked {
			t.Errorf("%q: bash ran the construct (%s) and the script is not marked", row.body, got)
		}
		if row.rule != "" && !row.over && !ran {
			t.Errorf("%q: listed as an attack but bash did not run the construct", row.body)
		}
	}
}

// runProbe writes body to t.sh in dir and runs probe there with the oracle, returning its standard output. The run is
// bounded at five seconds, and a failed run is not an error: a probe that exits non-zero, as one that sources a fixture
// ending in `exit` does, still leaves the output the caller reads.
func runProbe(t *testing.T, o *oracleShell, dir, body, probe string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "t.sh"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, _ := o.Run(t, oracleRun{Dir: dir, Command: probe, Timeout: 5 * time.Second})
	return stdout
}

// heredocEndRow is a heredoc introduction, written after `done ` on the first line of a fixture, and the body lines
// that follow it.
type heredocEndRow struct {
	intro string
	body  []string
}

// heredocEndRows covers delimiter forms on which the parser's heredoc lexer and bash may end a body on different lines.
// Every body line is either a no-op command or a word no program on PATH is named after, so the lines bash runs as
// code after its own end of the body do nothing.
func heredocEndRows() []heredocEndRow {
	var candidates []string
	for _, c := range []string{`E\X`, `EX`, `$E`, `E$X`, `E X`, `E\ X`, "\tE", "\t\"E\"", `E`} {
		candidates = append(candidates, ": d", c)
	}
	candidates = append(candidates, ": d")
	var rows []heredocEndRow
	for _, intro := range []string{`<<E`, `<<'E'`, `<<"E"`, `<<\E`, `<<"E\X"`, `<<E"\X"`, `<<'E\X'`, `<<$'E'`, `<<$"E"`, `<<E$X`,
		`<<E\ X`, `<<-E`, `<<-"E"`, `<<-"E\X"`, `<< E`} {
		rows = append(rows, heredocEndRow{intro: intro, body: candidates})
	}
	// A carriage return on the introduction or on the end line but not on both.
	rows = append(rows, heredocEndRow{intro: "<<E", body: []string{": d", "E\r", ": d", "E", ": d"}},
		heredocEndRow{intro: "<<E\r", body: []string{": d", "E", ": d", "E\r", ": d"}},
		// An introduction the lexer does not see, on a line continued from the one before.
		heredocEndRow{intro: "\\\n<<'E'", body: []string{": it's", "E", ": d"}})
	return rows
}

// heredocFixture prints the body of the row's heredoc line by line and then @END, so the lines bash printed before
// @END are the lines it took as the body.
func heredocFixture(row heredocEndRow) string {
	return "while IFS= read -r l; do printf '%s\\n' \"$l\"; done " + row.intro + "\n" + strings.Join(row.body, "\n") + "\necho @END\n"
}

// lexerBodyLines returns how many lines the parser's heredoc lexer takes as the body of the first heredoc in text,
// read from the heredoc extents: the lines of the first body, less its closing line, or -1 when text has no heredoc.
func lexerBodyLines(text string) int {
	extents := readHeredocExtents(text).extents
	if len(extents) == 0 {
		return -1
	}
	first := extents[0]
	n := physicalLines(text[first.bodyStart:first.bodyEnd])
	if first.terminated {
		n--
	}
	return n
}

// physicalLines counts the lines of s as strings.Split on \n does, except that the empty remainder after a final
// newline is not a line.
func physicalLines(s string) int {
	n := strings.Count(s, "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// TestScriptHeredocEndAgainstBash checks, for each delimiter form, that whenever real bash ends a heredoc body on a
// different line than the parser's lexer, the script is refused at the introduction.
func TestScriptHeredocEndAgainstBash(t *testing.T) {
	o := requireOracle(t)
	dir := t.TempDir()
	bashBodyLines := func(text string) int {
		n := 0
		for _, line := range strings.Split(strings.TrimRight(runProbe(t, o, dir, text, "source ./t.sh 2>/dev/null </dev/null"), "\n"), "\n") {
			if line == "@END" {
				return n
			}
			n++
		}
		return n
	}
	if got := bashBodyLines(heredocFixture(heredocEndRow{intro: "<<E", body: []string{": a", "E"}})); got != 1 {
		t.Skipf("bash did not run a fixture from the temp directory (counted %d body lines)", got)
	}
	differed := 0
	for _, row := range heredocEndRows() {
		text := heredocFixture(row)
		if strings.Contains(text, "\r") {
			// The lexer ignores a carriage return that bash compares, so the parser refuses the script before the
			// lexer is asked where a body ends.
			if got := scriptReasons(t, text, nil); len(got) != 1 || got[0] != "script-encoding" {
				t.Errorf("%q: a carriage return must be refused as script-encoding, got %v", row.intro, got)
			}
			continue
		}
		bash, lexer := bashBodyLines(text), lexerBodyLines(text)
		if bash == lexer {
			continue
		}
		differed++
		if got := scriptReasons(t, text, nil); !hasRuleAt(got, "heredoc-delimiter", strings.Count(row.intro, "\n")+1) {
			t.Errorf("%q: bash ends the body after %d lines, the lexer after %d, and the script is not refused at its introduction: %v",
				row.intro, bash, lexer, got)
		}
	}
	if differed == 0 {
		t.Errorf("no row showed the lexer and bash disagreeing, so the oracle proves nothing")
	}
}

// operatorRow is a line, or lines, holding a << that bash may or may not take for a heredoc operator, and the word that
// would end the body. over marks a row whose refusal is an accepted cost although bash and the lexer agree.
type operatorRow struct {
	shape, delim string
	over         bool
}

func operatorRows() []operatorRow {
	return []operatorRow{
		{shape: "(( zzx = 1 << E ))", delim: "E"}, {shape: "(( zzx = 1 <<E ))", delim: "E"}, {shape: "(( y = 1 << 2 ))", delim: "2"},
		{shape: "for (( i = 1 << E; i < 0; i++ )); do :; done", delim: "E"}, {shape: "echo $[ 1 << E ]", delim: "E"},
		{shape: "x=`: <<E `", delim: "E"}, {shape: "let \"x = 1 << E\"", delim: "E"}, {shape: "let x=1<<E", delim: "E"},
		{shape: "[[ $(( 1 << E )) -eq 0 ]]", delim: "E"}, {shape: "echo $(( 1 << E ))", delim: "E"}, {shape: "echo $((1<<E))", delim: "E"},
		{shape: "arr[1<<E]=x", delim: "E"}, {shape: "x=${y:1<<E}", delim: "E"}, {shape: "x=${y:0:1<<E}", delim: "E"},
		{shape: "x=${y:1<<E }", delim: "E"}, {shape: "case x in a) (( 1 << E )) ;; esac", delim: "E"},
		{shape: "case x in x) (( 1 << E )) ;; esac", delim: "E"}, {shape: "(( 1 # << E ))", delim: "E"},
		{shape: "[[ a =~ a<<E ]]", delim: "E"}, {shape: "x=\"\na\" <<E", delim: "E"}, {shape: "x='\na' <<E", delim: "E"},
		{shape: "printf '%s\\n' \"a<<E\"", delim: "E"}, {shape: "arr[1<<E ]=x", delim: "E"}, {shape: "arr[1 << E]=x", delim: "E"},
		{shape: "echo $[ 1 ] <<E", delim: "E"}, {shape: "x=`echo a` <<E", delim: "E"}, {shape: "echo a<<E", delim: "E"}, {shape: ": <<E", delim: "E"},
		{shape: "exec 3<<E", delim: "E"}, {shape: ": <(: <<E)", delim: "E"}, {shape: "x=$(: <<E)", delim: "E"},
		{shape: ": <<< E", delim: "E"}, {shape: "echo \"a << E\"", delim: "E"}, {shape: "echo 'a << E'", delim: "E"},
		{shape: ": <<'E'", delim: "E"}, {shape: ": $(( 1 )) <<E", delim: "E"}, {shape: "x=$(( $(echo 1) << E ))", delim: "E"},
		{shape: "(( zzx = 1 <<\nE ))", delim: "E", over: true}, {shape: "[[ a &&\nb =~ a<<E ]]", delim: "E"}, {shape: "(( zzx =\n1 << E ))", delim: "E"},
		// An operator inside array parens, and inside a substitution that closes on the operator's line.
		{shape: "arr=(1 <<E )", delim: "E"}, {shape: "arr=(1 <<E)", delim: "E"}, {shape: "x=$(: <<E )", delim: "E"},
	}
}

// operatorFixture writes the row's shape and then four lines: a marker, the delimiter, and two more markers. A marker
// line prints its own number, so the first number bash prints is the first line after the shape that it runs as code.
func operatorFixture(row operatorRow) string {
	return row.shape + "\necho L1\n" + row.delim + "\necho L3\necho L4\n"
}

// lexerFirstCodeLine returns the line, counted from 1 after the shape's last line, on which the parser's heredoc lexer
// resumes reading commands once it has taken the bodies the shape's last line introduces; 5 when it never resumes. It
// reads the heredoc extents introduced on that line.
func lexerFirstCodeLine(text string, shapeLines int) int {
	k := shapeLines - 1
	h := readHeredocExtents(text)
	if starts := h.lineStarts(); k >= len(starts) || !starts[k] {
		return 1
	}
	last := -1
	for _, e := range h.extents {
		if e.introLine != k {
			continue
		}
		if !e.terminated {
			return 5
		}
		last = e.bodyEnd
	}
	if last < 0 {
		return 1
	}
	return physicalLines(text[:last]) - k
}

// TestScriptHeredocOperatorAgainstBash checks, for each shape, that whenever real bash and the parser's lexer resume
// running commands on different lines after it, the script is refused where the << is, and that a shape on which they
// agree is not refused unless the row accepts the cost.
func TestScriptHeredocOperatorAgainstBash(t *testing.T) {
	o := requireOracle(t, oracleBash44())
	dir := t.TempDir()
	bashFirstCodeLine := func(text string) int {
		for _, line := range strings.Split(runProbe(t, o, dir, text, "source ./t.sh 2>/dev/null </dev/null"), "\n") {
			if len(line) == 2 && line[0] == 'L' && line[1] >= '1' && line[1] <= '4' {
				return int(line[1] - '0')
			}
		}
		return 5
	}
	if got := bashFirstCodeLine(operatorFixture(operatorRow{shape: ": <<E", delim: "E"})); got != 3 {
		t.Skipf("bash did not run a fixture from the temp directory (first code line %d)", got)
	}
	differed := 0
	for _, row := range operatorRows() {
		text := operatorFixture(row)
		shapeLines := strings.Count(row.shape, "\n") + 1
		bash, lexer := bashFirstCodeLine(text), lexerFirstCodeLine(text, shapeLines)
		got := scriptReasons(t, text, nil)
		switch {
		case bash != lexer:
			differed++
			if !hasRuleAt(got, "heredoc-delimiter", shapeLines) {
				t.Errorf("%q: bash runs line %d first, the lexer resumes at %d, and the script is not refused at line %d: %v",
					row.shape, bash, lexer, shapeLines, got)
			}
		case !row.over && hasRule(got, "heredoc-delimiter"):
			t.Errorf("%q: bash and the lexer agree (line %d) but the script is refused: %v", row.shape, bash, got)
		}
	}
	if differed == 0 {
		t.Errorf("no row showed the lexer and bash disagreeing, so the oracle proves nothing")
	}
}

// TestScriptOddHeredocDelimiterIsRefused checks that a script whose heredoc delimiter the lexer may read differently
// from bash is refused at the introduction line, so a command bash runs after the body cannot pass as body data, and
// that the plain forms stay allowed.
func TestScriptOddHeredocDelimiterIsRefused(t *testing.T) {
	for body, line := range map[string]int{
		"cat <<\"E\\X\"\ndata\nE\\X\nrm -rf /tmp/zz\n": 1, "cat <<E\"\\X\"\nx\nE\\X\n": 1, "cat <<$'E'\nx\nE\n": 1,
		"cat <<$\"E\"\nx\nE\n": 1, "cat <<E$X\nx\nE$X\n": 1, "cat <<E\\ X\nx\nE X\n": 1, "cat <<''\nx\n\n": 1,
		"cat <<E'F'\nx\nEF\n": 1, "echo a\nf() {\ncat <<\"E\\X\"\nx\nE\\X\n}\n": 3,
		"cat <<-\"E\\X\"\n\tx\n\tE\\X\n": 1, "cat <<E\\\nx\n": 1,
		// An introduction on a continued line, which the lexer does not take for one, with quotes that would otherwise
		// make it read the command after the body as part of a string.
		"cat \\\n<<'E'\n: 'a\nE\nrm -rf /tmp/zz\n: '\n": 2,
		// A << that bash reads as a shift or as text, where the lexer reads a heredoc and drops the next lines.
		"(( zzx = 1 << E ))\nrm -rf /tmp/zz\nE\n": 1, "(( zzx = 1 <<E ))\nrm -rf /tmp/zz\nE\n": 1, "(( y = 1 << 2 ))\nrm -rf /tmp/zz\n2\n": 1,
		"for (( i = 1 << E; i < 0; i++ )); do :; done\nrm -rf /tmp/zz\nE\n": 1, "echo $[ 1 << E ]\nrm -rf /tmp/zz\nE\n": 1,
		"x=`cat <<E `\nrm -rf /tmp/zz\nE\n": 1, "x=${y:1<<E }\nrm -rf /tmp/zz\nE\n": 1, "arr[1<<E ]=x\nrm -rf /tmp/zz\nE\n": 1,
		"case x in x) (( 1 << E )) ;; esac\nrm -rf /tmp/zz\nE\n": 1, "(( zzx =\n1 << E ))\nrm -rf /tmp/zz\nE\n": 2,
		"x=$(( $(echo 1) << E ))\nrm -rf /tmp/zz\nE\n": 1, "echo a\nf() {\n(( z = 1 << E ))\nrm -rf /tmp/zz\nE\n}\n": 3,
		// A << that ends a string spanning lines, which bash reads as a heredoc and the lexer does not.
		"x=\"\na\" <<E\n: 'a\nE\nrm -rf /tmp/zz\n: '\n": 2,
		// A [[ ]] test that spans lines, and the first of two disagreements wins.
		"[[ a &&\nb =~ a<<E ]]\nrm -rf /tmp/zz\nE\n": 2, "(( z = 1 << E ))\nx\nE\ncat <<\"E\\X\"\nx\nE\\X\n": 1,
		// The command that holds the operator goes on past its line, so bash runs the lines up to its end before it
		// reads the body, while the lexer takes them for the body: an operator inside array parens, a backslash-newline
		// after the operator, and a substitution opened after the operator and closed on a later line.
		"arr=(1 <<E )\nrm -rf /tmp/zz\nE\n": 1, "cat <<E \\\n>/dev/null; rm -rf /tmp/zz\nE\n": 1,
		"cat <<E \\\n| cat; rm -rf /tmp/zz\nE\n": 1, "echo a <<E \\\nb; rm -rf /tmp/zz\nE\n": 1,
		"cat <<E $(\nrm -rf /tmp/zz\n)\nx\nE\n": 1, "cat <<E ${zz:-$(\nrm -rf /tmp/zz\n)}\nx\nE\n": 1,
		"cat <<E <(\nrm -rf /tmp/zz\n)\nx\nE\n": 1, "cat <<E; x=$(\nrm -rf /tmp/zz\n)\nx\nE\n": 1,
		"x=$(: <<E) $(\nrm -rf /tmp/zz\n)\nx\nE\n": 1, "echo a\nf() {\ncat <<E $(\nrm -rf /tmp/zz\n)\nx\nE\n}\n": 3,
		"x=$(cat <<E \\\n; rm -rf /tmp/zz\nE\n)\n": 1,
		// An arithmetic command holds its text, substitution included, and parses the substitution when it runs, so the
		// body of a heredoc inside is not found where the lexer finds it.
		"(( zz = $(cat <<E; (\nrm -rf /tmp/zz\n)\nx\nE\n) ))\n": 1, "x=$(( $(cat <<E; (\nrm -rf /tmp/zz\n)\nx\nE\n) ))\n": 1,
	} {
		if got := scriptReasons(t, body, nil); !hasRuleAt(got, "heredoc-delimiter", line) {
			t.Errorf("%q: want heredoc-delimiter at line %d, got %v", body, line, got)
		}
	}
	for _, body := range []string{"cat <<E\nx\nE\n", "cat <<'E'\nx\nE\n", "cat <<\"E\"\nx\nE\n", "cat <<\\E\nx\nE\n", "cat <<-E\n\tx\n\tE\n",
		"cat << END_OF-file.1\nx\nEND_OF-file.1\n", "read -r a <<< x\n", "cat <<E | cat\nx\nE\n", "echo $((1<<3))\n",
		"echo \"a << b\"\n", "x=$(( 1 << 2 ))\n", "(( a = 1 ))\ncat <<E\nx\nE\n", "cat <<E > \"${f}\"\nx\nE\n",
		"for (( i=0; i<2; i++ )); do :; done\ncat <<E\nx\nE\n", "[ -f x ] && cat <<E\nx\nE\n", "ls *.[ch] <<E\nx\nE\n",
		"x=$(cat <<E\nx\nE\n)\n", "cat <<E\nx\n", "echo ${a[0]} <<E\nx\nE\n", "exec 3<<E\nx\nE\n", "echo $[ 1 ]\n",
		"echo a[\ncat <<E\nx\nE\n",
		// The command ends at the operator line's newline although something stays open around it: bash reads the body
		// there, as the lexer does.
		"( cat <<E\nx\nE\n)\n", "if :; then cat <<E\nx\nE\nfi\n", "cat <<E; [[ a &&\nb ]]\nx\nE\n", "x=$(: <<E )\nx\nE\n",
		"cat <<E |\nx\nE\ncat\n", "cat <<E &&\nx\nE\necho a\n", "x=\"$(cat <<E\nx\nE\n)\"\n", "cat <(cat <<E\nx\nE\n)\n",
		"[ -n \"$(cat <<E\nx\nE\n)\" ]\n", "[[ -n $(cat <<E\nx\nE\n) ]]\n"} {
		if got := scriptReasons(t, body, nil); hasRule(got, "heredoc-delimiter") {
			t.Errorf("%q: unexpected heredoc-delimiter: %v", body, got)
		}
	}
	// The data view must not vouch for an odd delimiter, and the text scan must read such a script as written.
	if QuotedHeredocView("cat <<\"E\\X\"\nx\nE\\X\n").Reliable || !QuotedHeredocView("cat <<'E'\nx\nE\n").Reliable {
		t.Errorf("the heredoc view must be unreliable for an odd delimiter and only for one")
	}
	for _, body := range []string{"cat <<\"E\\X\"\nx\nE\\X\n# c\n", "cat \\\n<<'E'\nx\nE\n# c\n"} {
		if newScriptLines(body).scan != body {
			t.Errorf("%q: the scan must read a script with an odd heredoc as written", body)
		}
	}
}

// TestScriptCallPrefixStandInHasCallerWrappers checks that the stand-in for a call prefix carries the wrappers of the
// call, not the extra shell wrapper of the script it calls.
func TestScriptCallPrefixStandInHasCallerWrappers(t *testing.T) {
	files := fakeReader{"/ws/t.sh": {Text: "echo a\nPATH=/x bash /ws/inner.sh\nenv PATH=/x bash /ws/inner.sh\n"}, "/ws/inner.sh": {Text: "echo in\n"}}
	res := parseScript("bash /ws/t.sh", files)
	stand := 0
	for _, inv := range res.Invocations {
		if len(inv.Wrappers) != len(inv.WrapperIDs) {
			t.Errorf("wrappers and ids misaligned: %d vs %d", len(inv.Wrappers), len(inv.WrapperIDs))
		}
		if !inv.Inspectable && strings.HasPrefix(inv.Reason.String(), "script-opaque:pinned-assignment in /ws/t.sh:") {
			stand++
			if len(inv.Wrappers) != 1 || inv.Wrappers[0].Text != "bash" || len(inv.WrapperIDs) != 1 {
				t.Errorf("stand-in wrappers %+v ids %v, want only the caller's bash", inv.Wrappers, inv.WrapperIDs)
			}
		}
	}
	if stand != 2 {
		t.Errorf("want 2 stand-ins, got %d", stand)
	}
}

// TestOptionScanRowsReadOptions pins what the builtin option walk produces for read's letter sets: a cluster that ends
// on a value letter, an attached and a separate variable name, a value letter that ends the line, and `--`.
func TestOptionScanRowsReadOptions(t *testing.T) {
	literals := func(texts ...string) []Word {
		words := make([]Word, len(texts))
		for i, text := range texts {
			words[i] = Word{Text: text, Raw: text, Literal: true}
		}
		return words
	}
	got := scanOptionsArgv(literals("-pt", "x"), readValues, readTargets)
	if got.seen != "p" || len(got.operands) != 1 || got.operands[0].Text != "x" || len(got.targets) != 0 {
		t.Errorf("read -pt x: %#v, want seen %q, operand x, no target", got, "p")
	}
	got = scanOptionsArgv(literals("-aPATH"), readValues, readTargets)
	if got.seen != "a" || len(got.targets) != 1 || got.targets[0] != (Word{Text: "PATH", Literal: true}) {
		t.Errorf("read -aPATH: %#v, want seen a and target PATH", got)
	}
	got = scanOptionsArgv(literals("-a", "PATH"), readValues, readTargets)
	if got.seen != "a" || len(got.targets) != 1 || got.targets[0] != (Word{Text: "PATH", Raw: "PATH", Literal: true}) {
		t.Errorf("read -a PATH: %#v, want seen a and the next word as target", got)
	}
	got = scanOptionsArgv(literals("-a"), readValues, readTargets)
	if len(got.targets) != 1 || got.targets[0] != (Word{Literal: true}) {
		t.Errorf("read -a: %#v, want one empty literal target", got)
	}
	got = scanOptionsArgv(literals("--", "-a", "x"), readValues, readTargets)
	if got.seen != "" || len(got.operands) != 2 {
		t.Errorf("read -- -a x: %#v, want both later words as operands", got)
	}
}
