package shellwords

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
)

func scriptChild(res Result) *Invocation {
	for i := range res.Invocations {
		if res.Invocations[i].Script != nil {
			return &res.Invocations[i]
		}
	}
	return nil
}

func firstScriptReason(res Result) string {
	for _, inv := range res.Invocations {
		if !inv.Inspectable {
			return inv.Reason.String()
		}
	}
	return ""
}

// spellingHost judges .. by spelling, as Windows does, so a test parsed under it pins the replay's own arithmetic;
// TestScriptParentReferenceFollowsTheHost covers the hosts with real symlinks.
var spellingHost = pathshape.For("windows")

func TestScriptPathResolution(t *testing.T) {
	host := spellingHost
	files := fakeReader{
		"/ws/ok.sh": {Text: "echo ok\n"}, "/ws/sub/ok.sh": {Text: "echo sub\n"}, "C:/ws/ok.sh": {Text: "echo win\n"},
		"/ws/a.sh": {Text: "cd sub\n"}, "/ws/b.sh": {Text: "bash ./ok.sh\n"}, "/ws/sub/b.sh": {Text: "bash ./ok.sh\n"},
		"C:/ok.sh": {Text: "echo root\n"}, "/ws/RUNNER~1/ok.sh": {Text: "echo short\n"},
		"/ws/c.sh": {Text: "x=1\n"},
		"/ws/s.sh": {Text: "cd \"$X\"\nbash /ws/ok.sh\n"},
	}
	cases := []struct {
		command, cwd, wantPath, wantReason string
	}{
		{"bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"cd /ws && bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"cd sub && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		// A cd that fails leaves the directory alone, so a .. over what it added climbs from elsewhere: if sub is
		// missing, `cd sub; cd ..` ends above /ws. Only an && chain, which stops at the failure, keeps that proven.
		{"cd sub; cd .. ; bash ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub && cd .. && bash ok.sh", "/ws", "/ws/ok.sh", ""},
		{"cd sub/.. && bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"cd sub/..; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub; bash ../ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub && bash ../ok.sh", "/ws", "/ws/ok.sh", ""},
		{"cd sub; bash -c 'cd .. && bash ./ok.sh'", "/ws", "", "script-cwd-unproven"},
		{"cd sub && bash -c 'cd .. && bash ./ok.sh'", "/ws", "/ws/ok.sh", ""},
		{"cd /ws/sub; cd ..; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub; cd /ws; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		// The event cwd exists, so climbing out of it is safe.
		{"cd ..; bash ./ws/ok.sh", "/ws", "/ws/ok.sh", ""},
		{"bash ../ws/ok.sh", "/ws", "/ws/ok.sh", ""},
		{"cd sub; cd ..; cd \"$X\"; bash /ws/ok.sh", "/ws", "", "script-predecessor-state"},
		{"cd C:/ws && bash ./ok.sh", "/ws", "C:/ws/ok.sh", ""},
		{"bash C:/ws/ok.sh", "", "C:/ws/ok.sh", ""},
		// The call may be wrapped, and so may the cd, without changing which directory applies.
		{"cd sub && env bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"cd sub && env A=1 bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"cd sub && nohup bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"cd sub && time bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"cd sub && command bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"command cd sub && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"builtin cd sub && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"time cd sub && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		// A cd that runs in another process or another shell does not change this shell's directory.
		{"env cd sub && bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"bash -c 'cd sub'; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"bash ./a.sh && bash ./ok.sh", "/ws", "/ws/ok.sh", ""}, // a.sh does `cd sub` in its own shell
		// A cd in an enclosing shell applies to a call made inside a nested one.
		{"bash -c 'cd sub && bash ./ok.sh'", "/ws", "/ws/sub/ok.sh", ""},
		{"cd sub; bash -c 'bash ./ok.sh'", "/ws", "/ws/sub/ok.sh", ""},
		{"cd sub && bash ./b.sh", "/ws", "/ws/sub/ok.sh", ""}, // nested relative call resolves in the caller's directory
		{"bash /ws/b.sh", "/ws", "/ws/ok.sh", ""},
		{"cd \"$X\" && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd - && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd -P sub && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd a b && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"pushd sub && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"CDPATH=x cd sub && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"exec cd sub && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"false || cd sub && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub & bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub | cat; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"if true; then cd sub; fi; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		// A cd inside ( ) or { } is refused conservatively: the lexer cannot tell a subshell from a brace group
		// for a following call, and reading the wrong file is worse than refusing.
		{"(cd sub); bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"(cd sub; bash ./ok.sh)", "/ws", "", "script-cwd-unproven"},
		{"{ cd sub; }; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		// The lexer marks only the first segment inside ( ), $( ), backticks and <( ) as grouped, so a cd later
		// inside them looks top-level and must be caught from the surrounding text.
		{"(true; cd sub); bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"(true && cd sub); bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"echo $(true; cd sub); bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"echo `true; cd sub`; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cat <(true; cd sub); bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"f() { true; cd sub; }; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"(\ntrue\ncd sub\n)\nbash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub; echo $(bash ./ok.sh)", "/ws", "", "script-cwd-unproven"},
		{"cd sub; echo $(cd ..; bash ./ok.sh)", "/ws", "", "script-cwd-unproven"},
		{"x=$(cd sub); bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"(true; cd sub); bash -c 'bash ./ok.sh'", "/ws", "", "script-cwd-unproven"},
		{"echo $(bash ./ok.sh)", "/ws", "/ws/ok.sh", ""},
		{"echo x | cd sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		// A cd skipped by a failed && is not a directory change the call can rely on.
		{"false && cd sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"true && cd sub && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		// Git Bash keeps .. at a drive root only inside one C:/-spelled operand. A directory it entered, or started in,
		// it holds as /c/..., and /c/.. is the MSYS root, which the guard cannot name.
		{"cd C:/ws/../.. && bash ./ok.sh", "/ws", "C:/ok.sh", ""},
		{"cd C:/.. && bash ./ok.sh", "/ws", "C:/ok.sh", ""},
		{"cd C:/ws && cd ../.. && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd C:/ws && bash ../../ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd .. && bash ./ok.sh", "C:/ws", "C:/ok.sh", ""},
		{"cd ../.. && bash ./ok.sh", "C:/ws", "", "script-cwd-unproven"},
		{"bash ../../ok.sh", "C:/ws", "", "script-cwd-unproven"},
		{"bash ../ok.sh", "C:\\ws", "C:/ok.sh", ""},
		// bash has no chdir builtin.
		{"chdir sub; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"cd sub && bash ./ok.sh &", "/ws", "/ws/sub/ok.sh", ""},
		{"false && cd sub; echo x && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"echo $(cd sub); bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"echo $(bash -c 'cd sub'); bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"cd sub & echo x; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub; echo x | bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		// A relative event cwd proves nothing.
		{"bash ./ok.sh", "ws", "", "script-cwd-unproven"},
		{"bash /ws/ok.sh", "ws", "/ws/ok.sh", ""},
		{"true & cd sub; bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""}, // only the cd itself must not be backgrounded
		// A line break or comment after & must not hide that the cd was backgrounded.
		{"cd sub &\nbash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub & \nbash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub & # c\nbash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		// A segment that runs no command (a bare assignment) must not hide the separator before it.
		{"cd sub & x=1; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub & x=1 && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub | x=1; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"false && cd sub; x=1 && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"false && cd sub; x=1\nbash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"false && cd sub\nx=1 && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"x=1; cd sub; bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		// An assignment inside another shell, such as an earlier script, hides nothing from this one.
		{"cd sub && bash /ws/c.sh && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		// A command that does not run in this shell (wrapped, another shell, or one that leaves no invocation) still
		// carries the separator before it, so a cd it follows may have been backgrounded, piped or skipped.
		{"cd sub & env echo hi; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub & nohup true; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub | env cat; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub | sudo tee f; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"false && cd sub; env true && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub & bash -c 'echo'; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub & bash -c ''; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub & bash -c 'x=1'; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub & command -v git; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"false && cd sub; command -v git && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub & bash ./c.sh; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"cd sub & bash -c 'bash ./ok.sh'", "/ws", "", "script-cwd-unproven"},
		{"false && cd sub; bash -c 'bash ./ok.sh'", "/ws", "", "script-cwd-unproven"},
		{"cd sub | bash -c 'bash ./ok.sh'", "/ws", "", "script-cwd-unproven"},
		{"bash -c 'cd sub & env true; bash ./ok.sh'", "/ws", "", "script-cwd-unproven"},
		{"bash -c 'false && cd sub; command -v git && bash ./ok.sh'", "/ws", "", "script-cwd-unproven"},
		// Separators are read in the text of each list, so the one that started a nested shell does not count as the
		// separator before that shell's own first command.
		{"false || bash -c 'cd sub; bash ./ok.sh'", "/ws", "/ws/sub/ok.sh", ""},
		{"true && bash -c 'cd sub; bash ./ok.sh'", "/ws", "/ws/sub/ok.sh", ""},
		{"cd sub && bash -c 'true; bash ./ok.sh'", "/ws", "/ws/sub/ok.sh", ""},
		{"cd sub && bash -c 'x=1; bash ./ok.sh'", "/ws", "/ws/sub/ok.sh", ""},
		// The & and && state of a list belongs to that list: a nested shell starts with its own.
		{"cd sub && bash -c 'true & bash ./ok.sh'", "/ws", "/ws/sub/ok.sh", ""},
		{"true && cd sub && bash -c 'true; bash ./ok.sh'", "/ws", "/ws/sub/ok.sh", ""},
		{"bash -c 'exec cd sub'; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		// time is a keyword only unquoted, unescaped and in command position, where a run of them may stand; anywhere
		// else it is an external program, and the cd runs in that process.
		{"\"time\" cd sub; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"command time cd sub; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"\\time cd sub; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"time time cd sub; bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"! time cd sub; bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"time ! cd sub; bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"time -p cd sub && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"time command cd sub; bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"bash -c 'time cd sub; bash ./ok.sh'", "/ws", "/ws/sub/ok.sh", ""},
		// bash finds builtins by exact name only: CD, Builtin and /usr/bin/cd are other programs.
		{"CD sub; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"Builtin cd sub; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"/usr/bin/cd sub; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"Command cd sub; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"\"cd\" sub && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"\"command\" cd sub && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		// Only an exact, unquoted keyword before a cd is a keyword; If, "if" or -x make the line another command.
		{"If cd sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"\"if\" cd sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"true; -x cd sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"! cd sub; bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"popd; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"for i in 1; do cd sub; done; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		// A cd in another branch, or later in a loop body, is not ordered before the call by the text, so a list
		// that holds a compound command and any directory change proves nothing.
		{"if false; then cd sub; else bash ./ok.sh; fi", "/ws", "", "script-cwd-unproven"},
		{"if false; then cd sub; elif true; then bash ./ok.sh; fi", "/ws", "", "script-cwd-unproven"},
		{"while test ! -e stop; do bash ./ok.sh; cd sub; test -e ../go && : > stop; : > ../go; done", "/ws", "", "script-cwd-unproven"},
		{"if false; then c\\d sub; else bash ./ok.sh; fi", "/ws", "", "script-cwd-unproven"},
		{"true;if false;then cd sub;else bash ./ok.sh;fi", "/ws", "", "script-cwd-unproven"},
		{"if cd sub; then bash ./ok.sh; fi", "/ws", "", "script-cwd-unproven"},
		{"if true; then bash ./ok.sh; fi", "/ws", "/ws/ok.sh", ""},
		// The check is textual, so a keyword inside a quoted -c string counts too: over-refused by design.
		{"cd sub; bash -c 'if true; then bash ./ok.sh; fi'", "/ws", "", "script-cwd-unproven"},
		{"cd sub && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		// After builtin, only a plain cd word is followed.
		{"builtin command cd sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"builtin -- cd sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"builtin builtin cd sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		// A command word that only expands at run time may be a cd.
		{"C=cd; $C sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		// So may one that brace expansion or a glob turns into cd, although the lexer keeps it as one literal word.
		{"{cd,} sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"{cd,} sub && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{": > cd; c[d] sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{": > cd; c? sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{": > cd; c*d sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"{,cd} sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"true && {cd,} sub && bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"! {cd,} sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"x=1 c{d,} sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"builtin {cd,} sub; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"{cd,} sub; bash /ws/ok.sh", "/ws", "", "script-predecessor-state"},
		// The lexer decodes ANSI-C quoting, so c$''d is the literal word cd and is followed.
		{"c$''d sub; bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		// A brace group needs a blank after {, and its cd is refused as before.
		{"{ cd sub; }; bash ./ok.sh", "/ws", "", "script-cwd-unproven"},
		{"{ true; }; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{": > cd; c? sub; bash /ws/ok.sh", "/ws", "", "script-predecessor-state"},
		// An indirect cd later in a loop body moves the next pass of the call.
		{"C=cd; while test ! -e stop; do bash ./ok.sh; $C sub; test -e ../go && : > stop; : > ../go; done", "/ws", "", "script-cwd-unproven"},
		{"while test ! -e stop; do bash ./ok.sh; c$''d sub; test -e ../go && : > stop; : > ../go; done", "/ws", "", "script-cwd-unproven"},
		{"while test ! -e stop; do bash ./ok.sh; {cd,} sub; test -e ../go && : > stop; : > ../go; done", "/ws", "", "script-cwd-unproven"},
		{"while test ! -e stop; do bash ./ok.sh; c? sub; test -e ../go && : > stop; : > ../go; done", "/ws", "", "script-cwd-unproven"},
		{"while test ! -e stop; do bash ./ok.sh; c[d] sub; test -e ../go && : > stop; : > ../go; done", "/ws", "", "script-cwd-unproven"},
		{"while test ! -e stop; do bash ./ok.sh; c*d sub; test -e ../go && : > stop; : > ../go; done", "/ws", "", "script-cwd-unproven"},
		{"while test ! -e stop; do bash ./ok.sh; `printf %s c d` sub; test -e ../go && : > stop; : > ../go; done", "/ws", "", "script-cwd-unproven"},
		// Ordinary command words around a cd still leave the directory proven.
		{"echo hi; cd sub && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"env echo hi; bash ./ok.sh", "/ws", "/ws/ok.sh", ""},
		{"./tool x; cd sub && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"git status; cd sub; bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"echo *.txt $X; cd sub && bash ./ok.sh", "/ws", "/ws/sub/ok.sh", ""},
		{"while test ! -e stop; do bash ./ok.sh; done", "/ws", "/ws/ok.sh", ""},
		// An unprovable cd before an absolute path is shell state the guard cannot vouch for, with or without a Cwd.
		{"cd \"$X\" && bash /ws/ok.sh", "", "", "script-predecessor-state"},
		{"cd sub & env true; bash /ws/ok.sh", "/ws", "", "script-predecessor-state"},
		{"cd sub & bash /ws/c.sh; bash /ws/ok.sh", "/ws", "", "script-predecessor-state"},
		// Inside an inspected script the same cd is only one more child to judge, not a refusal of the script.
		{"bash /ws/s.sh", "/ws", "/ws/ok.sh", ""},
		{"bash RUNNER~1/ok.sh", "/ws", "/ws/RUNNER~1/ok.sh", ""},
		{"bash './a`b.sh'", "/ws", "", "script-path-not-literal"},
		{"bash ./{a,b}.sh", "/ws", "", "script-path-not-literal"},
		{"bash \"$BASH_SOURCE\"", "/ws", "", "script-path-not-literal"},
		{"bash ${BASH_SOURCE[0]}", "/ws", "", "script-path-not-literal"},
		{"bash $HOME/ok.sh", "/ws", "", "script-path-not-literal"},
		{"bash ~/ok.sh", "/ws", "", "script-path-not-literal"},
		{"bash ./*.sh", "/ws", "", "script-path-not-literal"},
		{"bash \"$0\"", "/ws", "", "script-path-not-literal"},
		// A network, device or rooted-backslash spelling is never cleaned into a path the reader could open as
		// something else: the word as written is refused on every host.
		{"bash //srv/share/a.sh", "/ws", "", "script-path-not-literal"},
		{"bash '\\\\srv\\share\\a.sh'", "/ws", "", "script-path-not-literal"},
		{"bash '\\\\?\\C:\\a.sh'", "/ws", "", "script-path-not-literal"},
		{"bash '/\\srv/share/a.sh'", "/ws", "", "script-path-not-literal"},
		{"bash '\\ws\\ok.sh'", "/ws", "", "script-path-not-literal"},
		{"bash //ws/ok.sh", "", "", "script-path-not-literal"},
	}
	for _, tc := range cases {
		res := ParseScripts(context.Background(), tc.command, POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: tc.cwd, Host: host})
		switch got := firstScriptReason(res); {
		case tc.wantReason == "":
			found := false
			for _, inv := range res.Invocations {
				found = found || (inv.Script != nil && inv.Script.Path == tc.wantPath)
			}
			if !found || got != "" {
				t.Errorf("%q: want a child read from %s, got reason %q %+v", tc.command, tc.wantPath, got, res.Invocations)
			}
		case !strings.HasPrefix(got, tc.wantReason):
			t.Errorf("%q: want reason %s, got %q", tc.command, tc.wantReason, got)
		}
	}
}

// On a host where a backslash is an ordinary file name character, the file bash runs for 'a\b.sh' is not the file
// a\b.sh cleaned into a/b.sh names, so the word is refused. Where a backslash is a separator it is kept.
func TestScriptBackslashDependsOnTheHost(t *testing.T) {
	files := fakeReader{"/ws/a/b.sh": {Text: "echo ok\n"}, "C:/ws/a/b.sh": {Text: "echo ok\n"}}
	for _, tc := range []struct {
		goos, command, cwd, want string
	}{
		{"linux", "bash 'a\\b.sh'", "/ws", "script-path-not-literal"},
		{"darwin", "bash './a/b\\c.sh'", "/ws", "script-path-not-literal"},
		{"linux", "bash 'C:\\ws\\a\\b.sh'", "/ws", "script-path-not-literal"},
		{"linux", "bash a/b.sh", "/ws", ""},
		{"windows", "bash 'a\\b.sh'", "C:/ws", ""},
		{"windows", "bash 'C:\\ws\\a\\b.sh'", "", ""},
	} {
		res := ParseScripts(context.Background(), tc.command, POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: tc.cwd, Host: pathshape.For(tc.goos)})
		got := firstScriptReason(res)
		if tc.want == "" && (got != "" || scriptChild(res) == nil) || tc.want != "" && !strings.HasPrefix(got, tc.want) {
			t.Errorf("%s %q: want %q, got %q %+v", tc.goos, tc.command, tc.want, got, res.Invocations)
		}
	}
}

// Where the shell resolves .. against the real directory, a .. after a name may follow a symlink, and a cd is
// tracked by its spelling, so a .. after any cd is unproven. Where .. is resolved by spelling (Windows) the cleaned
// path is the path opened. A leading .. from the provider's own directory, which is real, stays allowed.
func TestScriptParentReferenceFollowsTheHost(t *testing.T) {
	files := fakeReader{
		"/ws/b.sh": {Text: "echo ws\n"}, "/ws/a/b.sh": {Text: "echo a\n"}, "/ws/sub/b.sh": {Text: "echo sub\n"},
		"/b.sh": {Text: "echo root\n"},
	}
	const refused = "script-path-not-literal"
	for _, tc := range []struct {
		goos, command, cwd, wantPath, wantReason string
	}{
		{"linux", "bash a/../b.sh", "/ws", "", refused},
		{"linux", "bash ./a/../b.sh", "/ws", "", refused},
		{"linux", "bash /ws/a/../b.sh", "/ws", "", refused},
		{"darwin", "bash /ws/a/../b.sh", "", "", refused},
		{"linux", "bash a/b/../../b.sh", "/ws", "", refused},
		{"linux", "bash ./sub/../b.sh", "/ws", "", refused},
		{"linux", "bash ../b.sh", "/ws/sub", "/ws/b.sh", ""},
		{"linux", "bash ../../b.sh", "/ws/sub/x", "/ws/b.sh", ""},
		{"linux", "bash ./../b.sh", "/ws/sub", "/ws/b.sh", ""},
		{"linux", "bash /../b.sh", "/ws", "/b.sh", ""},
		{"linux", "bash ./a/b.sh", "/ws", "/ws/a/b.sh", ""},
		{"linux", "cd a && bash ./b.sh", "/ws", "/ws/a/b.sh", ""},
		{"linux", "cd link && bash ../b.sh", "/ws", "", refused},
		{"linux", "cd /ws/link && bash ../b.sh", "/ws", "", refused},
		{"linux", "cd a && bash ../b.sh", "/ws", "", refused},
		{"linux", "cd .. && bash ../b.sh", "/ws/sub", "", refused},
		{"linux", "cd link && cd .. && bash ./b.sh", "/ws", "", "script-cwd-unproven"},
		{"linux", "cd link/.. && bash ./b.sh", "/ws", "", "script-cwd-unproven"},
		{"linux", "cd /ws/link/.. && bash ./b.sh", "/ws", "", "script-cwd-unproven"},
		{"linux", "cd .. && bash ./b.sh", "/ws/sub", "/ws/b.sh", ""},
		{"windows", "bash a/../b.sh", "/ws", "/ws/b.sh", ""},
		{"windows", "bash /ws/a/../b.sh", "/ws", "/ws/b.sh", ""},
		{"windows", "cd a && bash ../b.sh", "/ws", "/ws/b.sh", ""},
		{"windows", "cd link/.. && bash ./b.sh", "/ws", "/ws/b.sh", ""},
	} {
		res := ParseScripts(context.Background(), tc.command, POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: tc.cwd, Host: pathshape.For(tc.goos)})
		got := firstScriptReason(res)
		if tc.wantReason != "" {
			if !strings.HasPrefix(got, tc.wantReason) {
				t.Errorf("%s %q: want %s, got %q", tc.goos, tc.command, tc.wantReason, got)
			}
			continue
		}
		child := scriptChild(res)
		if got != "" || child == nil || child.Script.Path != tc.wantPath {
			t.Errorf("%s %q: want a child read from %s, got %q %+v", tc.goos, tc.command, tc.wantPath, got, res.Invocations)
		}
	}
}

func TestScriptPowerShellDirectoryChangeCommands(t *testing.T) {
	files := fakeReader{"/ws/ok.sh": {Text: "echo ok\n"}, "/ws/sub/ok.sh": {Text: "echo sub\n"}}
	opts := ScriptOptions{Reader: files, Context: ExecutionContext{Dialect: PowerShell, Proven: true}, Cwd: "/ws"}
	res := ParseScripts(context.Background(), "Set-Location sub; bash ./ok.sh", PowerShell, opts)
	if child := scriptChild(res); child == nil || child.Script.Path != "/ws/sub/ok.sh" {
		t.Fatalf("Set-Location did not change the directory: %+v", res.Invocations)
	}
	res = ParseScripts(context.Background(), "Push-Location sub; bash ./ok.sh", PowerShell, opts)
	if got := firstScriptReason(res); !strings.HasPrefix(got, "script-cwd-unproven") {
		t.Fatalf("Push-Location: got %q", got)
	}
	// PowerShell has no time keyword: time runs another program, and the location change stays there.
	res = ParseScripts(context.Background(), "time Set-Location sub; bash ./ok.sh", PowerShell, opts)
	if child := scriptChild(res); child == nil || child.Script.Path != "/ws/ok.sh" {
		t.Fatalf("time in PowerShell kept the location change: %+v", res.Invocations)
	}
	// A { glued to the next character is a script block in PowerShell, never brace expansion, so it hides no cd.
	res = ParseScripts(context.Background(), "Get-Date; {Write-Output hi}; bash ./ok.sh", PowerShell, opts)
	if child := scriptChild(res); child == nil || child.Script.Path != "/ws/ok.sh" {
		t.Fatalf("a PowerShell script block blocked the proof: %+v", res.Invocations)
	}
	// PowerShell keeps .. at a drive root there, unlike Git Bash.
	drive := ScriptOptions{Reader: fakeReader{"C:/ok.sh": {Text: "echo root\n"}}, Context: ExecutionContext{Dialect: PowerShell, Proven: true}, Cwd: "C:/ws"}
	res = ParseScripts(context.Background(), "Set-Location ../..; bash ./ok.sh", PowerShell, drive)
	if child := scriptChild(res); child == nil || child.Script.Path != "C:/ok.sh" {
		t.Fatalf("Set-Location .. at a drive root: %+v", res.Invocations)
	}
	// bash has no push-location, so in the POSIX dialect it is an ordinary program.
	res = ParseScripts(context.Background(), "push-location sub; bash ./ok.sh", POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws"})
	if child := scriptChild(res); child == nil || child.Script.Path != "/ws/ok.sh" {
		t.Fatalf("push-location in bash changed the directory: %+v", res.Invocations)
	}
}

// A command the parser could not take apart may still be a cd of this shell, so the call after it is unproven even
// though that command carries its own refusal.
func TestScriptUninspectableCommandIsAnUnprovableCD(t *testing.T) {
	files := fakeReader{"/ws/ok.sh": {Text: "echo ok\n"}, "/ws/sub/ok.sh": {Text: "echo sub\n"}}
	for command, want := range map[string]string{
		"command -p cd sub; bash ./ok.sh":   "script-cwd-unproven",
		"command -p cd sub; bash /ws/ok.sh": "script-predecessor-state",
	} {
		res := ParseScripts(context.Background(), command, POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws"})
		last := res.Invocations[len(res.Invocations)-1]
		if last.Words[0].Text != "bash" || !strings.HasPrefix(last.Reason.String(), want) {
			t.Errorf("%q: want the call refused as %s, got %+v", command, want, last)
		}
	}
	// A refused script call runs in a shell of its own and changes nothing here, even when its path would expand.
	for _, command := range []string{"bash ./missing.sh; bash ./ok.sh", "bash ./{a,b}.sh; bash ./ok.sh"} {
		res := ParseScripts(context.Background(), command, POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws"})
		if child := scriptChild(res); child == nil || child.Script.Path != "/ws/ok.sh" {
			t.Errorf("%q: a refused script call blocked the next one: %+v", command, res.Invocations)
		}
	}
}

// Git Bash maps /c/ws to C:/ws only on Windows, and there /c/.. is the MSYS root rather than C:/.
func TestScriptGitBashDriveRootIsNotClimbed(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("/c/ names a drive only on Windows")
	}
	files := fakeReader{"c:/ok.sh": {Text: "echo root\n"}, "C:/ok.sh": {Text: "echo root\n"}}
	for _, command := range []string{"cd /c/ws/../.. && bash ./ok.sh", "cd /c/ws && cd ../.. && bash ./ok.sh"} {
		res := ParseScripts(context.Background(), command, POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws"})
		if got := firstScriptReason(res); !strings.HasPrefix(got, "script-cwd-unproven") {
			t.Errorf("%q: got %q %+v", command, got, res.Scripts)
		}
	}
}

func TestScriptRelativePathWithoutCwdIsUnproven(t *testing.T) {
	res := ParseScripts(context.Background(), "bash ./ok.sh", POSIX, ScriptOptions{Reader: fakeReader{}, Context: provenPOSIX})
	if got := firstScriptReason(res); !strings.HasPrefix(got, "script-cwd-unproven") {
		t.Fatalf("got %q", got)
	}
}

func TestScriptAbsolutePathAfterAnUnprovenCdIsPredecessorState(t *testing.T) {
	files := fakeReader{"/ws/ok.sh": {Text: "echo ok\n"}}
	for _, command := range []string{"cd \"$X\" && bash /ws/ok.sh", "pushd sub && bash /ws/ok.sh"} {
		res := ParseScripts(context.Background(), command, POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws"})
		if got := firstScriptReason(res); !strings.HasPrefix(got, "script-predecessor-state") {
			t.Errorf("%q: got %q", command, got)
		}
	}
}

func TestScriptWrappedCallKeepsWrapperIdentity(t *testing.T) {
	files := fakeReader{"/ws/ok.sh": {Text: "echo hi\n"}}
	res := parseScript("env bash /ws/ok.sh", files)
	child := scriptChild(res)
	if child == nil {
		t.Fatalf("no script child: %+v", res.Invocations)
	}
	if len(child.Wrappers) != 2 || child.Wrappers[0].Text != "env" || child.Wrappers[1].Text != "bash" {
		t.Fatalf("wrappers %+v", child.Wrappers)
	}
	if len(child.WrapperIDs) != len(child.Wrappers) || child.WrapperIDs[0] == child.WrapperIDs[1] {
		t.Fatalf("wrapper ids %v do not match wrappers", child.WrapperIDs)
	}
}

func TestScriptCallAssignmentsReachTheChildren(t *testing.T) {
	files := fakeReader{"/ws/ok.sh": {Text: "echo hi\n"}}
	res := parseScript("FOO=1 bash /ws/ok.sh", files)
	child := scriptChild(res)
	if child == nil || len(child.Assignments) != 1 || child.Assignments[0].Text != "FOO=1" {
		t.Fatalf("assignment not propagated: %+v", res.Invocations)
	}
}

func TestScriptNestedCallRecordsChainAndOneRecordPerPath(t *testing.T) {
	files := fakeReader{
		"/ws/outer.sh": {Text: "bash /ws/inner.sh\nbash /ws/inner.sh\n"},
		"/ws/inner.sh": {Text: "echo hi\n"},
	}
	res := parseScript("bash /ws/outer.sh", files)
	var inner *Invocation
	for i := range res.Invocations {
		if s := res.Invocations[i].Script; s != nil && s.Path == "/ws/inner.sh" {
			inner = &res.Invocations[i]
		}
	}
	if inner == nil || len(inner.Script.Chain) != 1 || inner.Script.Chain[0] != "/ws/outer.sh" {
		t.Fatalf("inner child missing or wrong chain: %+v", res.Invocations)
	}
	if len(res.Scripts) != 2 {
		t.Fatalf("want one record per distinct path, got %+v", res.Scripts)
	}
}
