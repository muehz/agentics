package shellwords

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
)

func chainFiles() fakeReader {
	return fakeReader{
		"/ws/ok.sh": {ID: "1", Text: "echo ok\n"}, "/ws/inner.sh": {ID: "2", Text: "echo inner\n"},
		"/ws/touch.sh": {ID: "3", Text: "touch f\nbash /ws/inner.sh\n"},
		"/ws/self.sh":  {ID: "4", Text: "echo x >> /ws/self.sh\n"}, "/ws/dollar0.sh": {ID: "5", Text: "echo x >> \"$0\"\n"},
		"/ws/bashsrc.sh": {ID: "6", Text: "tee -a \"${BASH_SOURCE[0]}\" < /dev/null\n"},
		"/ws/other.sh":   {ID: "7", Text: "echo x >> /ws/ok.sh\nbash /ws/ok.sh\n"},
		"/ws/dynamic.sh": {ID: "8", Text: "echo x >> \"$out\"\n"},
	}
}

func TestScriptPredecessorsThatKeepTheCallInspected(t *testing.T) {
	for _, p := range []string{"cd /ws", "git add -A", "git commit -m x", "git status", "mkdir -p x", "echo hi", "head -n 1 f",
		"cat f", "mytool --x", "bash /ws/inner.sh", "git -C /ws status", "echo hi > /dev/null", "echo hi 2>&1", "echo hi >&2",
		"ls 2> /dev/null", "sed -n 1p f", "curl -sS u", "go build ./...", "bash --version", "builtin echo hi",
		// A git line the guard cannot read, with no words left after the options, runs no subcommand.
		"git --version", "git --exec-path"} {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			res := parseScript(p+" && "+call, chainFiles())
			if got := firstScriptReason(res); got != "" {
				t.Errorf("%q then %q: unexpected block %q", p, call, got)
			}
			if c := scriptChild(res); c == nil {
				t.Errorf("%q then %q: the call was not expanded", p, call)
			}
		}
	}
}

func TestScriptPredecessorWritersBlock(t *testing.T) {
	writers := []string{"echo x > f", "echo x >> f", "echo x &> f", "echo x 2> f", "echo x >&f", "tee f", "cp a b", "mv a b",
		"install a b", "ln -s a b", "dd of=f", "rsync a b", "tar xf a", "unzip a", "truncate -s 0 f", "touch f", "sed -i s/a/b/ f",
		"sed --in-place s/a/b/ f", "sed -ni p f", "perl -i -pe 1 f", "perl -pi -e 1 f", "awk -i inplace 1 f", "curl -o f u", "curl --output f u",
		"curl -sSo f u", "wget u", "patch -p1", "git checkout x", "git switch x", "git restore x", "git reset x", "git pull", "git merge x",
		"git rebase x", "git stash", "git apply x", "git am x", "git read-tree x", "git cherry-pick x", "git revert x", "git clean -f",
		"git -C /ws checkout x", "git -c a=b pull", "git --git-dir=/x/.git checkout x", "git \"$V\" x", "make", "npm ci", "npx x", "pnpm i", "yarn",
		"go generate ./...",
		// A git line whose options before the subcommand the guard cannot read, with words left over, may run a
		// subcommand that writes.
		"git --exec-path checkout .", "git --attr-source HEAD checkout .", "git --super-prefix x status", "git --unknown status",
		// A redirect on a wrapper belongs to the command inside it.
		"env true > f", "sudo true > f", "bash -c 'true' > f", "builtin tee f"}
	for _, p := range writers {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			if got := firstScriptReason(parseScript(p+" && "+call, chainFiles())); !strings.HasPrefix(got, "script-predecessor-may-write") {
				t.Errorf("%q then %q: got %q", p, call, got)
			}
		}
	}
}

// wantState is the reason for a state change before a call. A compound keyword or an exec in the list leaves a relative
// call without a directory, and that reason comes first.
func wantState(predecessor, call string) string {
	if strings.HasPrefix(call, "bash ./") && (strings.HasPrefix(predecessor, "function") || strings.HasPrefix(predecessor, "exec")) {
		return "script-cwd-unproven"
	}
	return "script-predecessor-state"
}

func TestScriptPredecessorStateBlocks(t *testing.T) {
	state := []string{"alias a=b", "function f { :; }", "export PATH=/x", "export GIT_DIR=/x", "export \"$n=v\"", "exec x", "set -- a",
		"trap 'x' DEBUG", "trap 'x' EXIT", "PROMPT_COMMAND=x", "export PROMPT_COMMAND=x", "builtin trap x EXIT"}
	for _, p := range state {
		for _, call := range []string{"bash /ws/ok.sh", "bash ./ok.sh"} {
			if got := firstScriptReason(parseScript(p+" && "+call, chainFiles())); !strings.HasPrefix(got, wantState(p, call)) {
				t.Errorf("%q then %q: got %q", p, call, got)
			}
		}
	}
	if got := firstScriptReason(parseScript("PROMPT_COMMAND=x; bash /ws/ok.sh", chainFiles())); !strings.HasPrefix(got, "script-predecessor-state") {
		t.Errorf("a bare assignment: got %q", got)
	}
	if got := firstScriptReason(parseScript("export OTHER=1 && bash /ws/ok.sh", chainFiles())); got != "" {
		t.Errorf("an export of a name that changes nothing: got %q", got)
	}
}

// A typed source, . or eval ran text in this shell that the cwd proof cannot follow: a relative call after one is
// unproven for its directory, and an absolute call, which needs no directory, is refused for the state itself.
func TestScriptCallAfterTypedSourceAndEval(t *testing.T) {
	for _, p := range []string{"source x", "source ./x", ". x", "eval x", "eval \"$X\""} {
		if got := firstScriptReason(parseScript(p+" && bash ./ok.sh", chainFiles())); !strings.HasPrefix(got, "script-cwd-unproven") {
			t.Errorf("%q then a relative call: got %q", p, got)
		}
		if got := firstScriptReason(parseScript(p+" && bash /ws/ok.sh", chainFiles())); !strings.HasPrefix(got, "script-predecessor-state") {
			t.Errorf("%q then an absolute call: got %q", p, got)
		}
	}
}

func TestScriptCallAfterUnprovenDirectoryChange(t *testing.T) {
	for command, want := range map[string]string{
		"cd \"$X\" && bash /ws/ok.sh":      "script-predecessor-state",
		"pushd sub && bash /ws/ok.sh":      "script-predecessor-state",
		"popd && bash /ws/ok.sh":           "script-predecessor-state",
		"cd \"$X\" && bash ./ok.sh":        "script-cwd-unproven",
		"pushd sub && bash ./ok.sh":        "script-cwd-unproven",
		"cd /ws && bash /ws/ok.sh":         "",
		"cd /ws && cd /ws && bash ./ok.sh": "",
	} {
		got := firstScriptReason(parseScript(command, chainFiles()))
		if want == "" && got != "" || want != "" && !strings.HasPrefix(got, want) {
			t.Errorf("%q: got %q, want %q", command, got, want)
		}
	}
}

func TestPredecessorRulesIgnoreScriptContent(t *testing.T) {
	// The touch sits inside touch.sh, so it is script content, not a predecessor of the nested call.
	res := parseScript("bash /ws/touch.sh", chainFiles())
	if got := firstScriptReason(res); got != "" {
		t.Fatalf("got %q", got)
	}
	if len(res.Scripts) != 2 {
		t.Fatalf("the nested call was not expanded: %+v", res.Scripts)
	}
}

// An earlier typed call to an inspected script is a predecessor that keeps proof; a typed writer after the same call
// is not.
func TestEarlierInspectedScriptCallIsAPredecessor(t *testing.T) {
	res := parseScript("bash /ws/inner.sh && bash /ws/ok.sh", chainFiles())
	if got := firstScriptReason(res); got != "" || len(res.Scripts) != 2 {
		t.Fatalf("got %q with %d scripts", got, len(res.Scripts))
	}
	if got := firstScriptReason(parseScript("bash /ws/inner.sh && touch f && bash /ws/ok.sh", chainFiles())); !strings.HasPrefix(got, "script-predecessor-may-write") {
		t.Fatalf("got %q", got)
	}
}

func TestPredecessorIsRefusedBeforeTheScriptIsRead(t *testing.T) {
	res := parseScript("touch f && bash /ws/ok.sh", chainFiles())
	if len(res.Scripts) != 0 {
		t.Fatalf("a script behind a refused predecessor was read: %+v", res.Scripts)
	}
}

func TestScriptWritesInspected(t *testing.T) {
	for _, call := range []string{"bash /ws/self.sh", "bash /ws/dollar0.sh", "bash /ws/bashsrc.sh", "bash /ws/other.sh"} {
		got := firstScriptReason(parseScript(call, chainFiles()))
		if !strings.HasPrefix(got, "script-writes-inspected in /ws/") || !strings.Contains(got, ".sh:1") && !strings.Contains(got, ".sh:2") {
			t.Errorf("%q: got %q", call, got)
		}
	}
	if got := firstScriptReason(parseScript("bash /ws/dynamic.sh", chainFiles())); got != "" {
		t.Errorf("a non-literal write target must be allowed, got %q", got)
	}
}

func TestScriptWriteTargetForms(t *testing.T) {
	host := pathshape.For("linux")
	selfWrites := []string{
		"echo x > /ws/t.sh", "echo x >> /ws/t.sh", "echo x &> /ws/t.sh", "echo x 2> /ws/t.sh", "echo x >&/ws/t.sh",
		"echo x <> /ws/t.sh", "echo x >> ./t.sh", "echo x >> t.sh", "echo x >> sub/../t.sh", "echo x >> ../t.sh", "echo x >> $0", "echo x >> \"$0\"",
		"echo x >> ${0}", "echo x >> $BASH_SOURCE", "echo x >> ${BASH_SOURCE}", "echo x >> ${BASH_SOURCE[0]}", "echo x >> \"${BASH_SOURCE[@]}\"",
		"tee /ws/t.sh", "tee -a /ws/t.sh", "tee -a -- /ws/t.sh", "echo x | tee -a \"$0\"", "cp a /ws/t.sh", "cp a b /ws/t.sh", "cp -t /ws a /ws/t.sh",
		"cp -f /tmp/e/t.sh /ws/", "cp --target-directory=/ws /tmp/t.sh", "cp -t /ws /tmp/t.sh", "mv a /ws/t.sh", "install a /ws/t.sh",
		"install -m 755 /tmp/t.sh /ws/", "ln -sf a /ws/t.sh", "truncate -s 0 /ws/t.sh", "touch /ws/t.sh", "dd if=a of=/ws/t.sh",
		"sed -i s/a/b/ /ws/t.sh", "sed -i.bak s/a/b/ t.sh", "sed --in-place=.bak s/a/b/ /ws/t.sh", "sed -ni p /ws/t.sh",
		"perl -i -pe 1 /ws/t.sh", "perl -pi -e 1 t.sh", "awk -i inplace 1 /ws/t.sh", "gawk -i inplace 1 t.sh", "curl -o /ws/t.sh u",
		"curl --output=/ws/t.sh u", "curl -sSo /ws/t.sh u", "wget -O /ws/t.sh u", "wget --output-document=t.sh u", "rsync a /ws/t.sh",
		"echo x >> \"$d/t.sh\"", "echo x >> \"$(dirname \"$0\")/t.sh\"", "builtin tee /ws/t.sh", "sudo tee /ws/t.sh", "env tee /ws/t.sh",
		// A cleaned spelling and the shell's name for the file can differ off Windows, so these count as a match.
		"echo x >> /ws/sub/../t.sh", "echo x >> //ws/x", "echo x >> \\\\srv\\share\\x", "echo x >> 'a\\b'",
		"git checkout -- t.sh", "git -C /ws restore t.sh",
	}
	for _, line := range selfWrites {
		files := chainFiles()
		files["/ws/t.sh"] = ScriptRead{ID: "10", Text: line + "\n"}
		got := firstScriptReason(parseScriptOn(host, "bash /ws/t.sh", files))
		if !strings.HasPrefix(got, "script-writes-inspected in /ws/t.sh:1") {
			t.Errorf("%q: got %q", line, got)
		}
	}
	allowed := []string{
		"echo x > /dev/null", "echo x 2>&1", "echo x >&2", "echo x >> \"$out\"", "echo x > out.txt", "echo x > /ws/log.txt", "echo x >> sub/log.txt",
		"echo x > ../log.txt", "tee out.txt", "tee -a /tmp/log", "cp /ws/t.sh /tmp/backup", "cp /ws/t.sh backup", "cp -t /tmp /ws/t.sh",
		"cp \"$src\" \"$dst\"", "mv /ws/t.sh /tmp/old", "install -m 755 /ws/t.sh /tmp/bin/x", "touch out.txt", "truncate -s 0 out.txt",
		"dd if=/ws/t.sh of=/tmp/x", "sed -i s/a/b/ out.txt", "sed -n p /ws/t.sh", "perl -ne print /ws/t.sh", "awk 1 /ws/t.sh", "cat /ws/t.sh",
		"curl -o /tmp/x u", "echo x >> /ws/other/t.shx", "echo x >> /ws/t.sh.bak", "echo x >> \"${0}.bak\"", "echo x >> \"${BASH_SOURCE%/*}/out\"",
		"echo x >> \"$d/out.txt\"", "echo hi",
	}
	for _, line := range allowed {
		files := chainFiles()
		files["/ws/t.sh"] = ScriptRead{ID: "10", Text: line + "\n"}
		if got := firstScriptReason(parseScriptOn(host, "bash /ws/t.sh", files)); got != "" {
			t.Errorf("%q: unexpected block %q", line, got)
		}
	}
}

// The git line names its subcommand through ReadGit. A line it reads to a verb lists targets for that verb, compared
// exactly, unless a global option is not literal: its text may be the verb. A line it cannot read keeps the lenient
// walk, so the targets of `git -- checkout .` are not dropped.
func TestScriptGitWriteTargets(t *testing.T) {
	for _, tc := range []struct {
		line        string
		files, dirs []string
	}{
		{"git checkout .", []string{"."}, []string{"."}},
		{"git checkout -- t.sh", []string{"t.sh"}, nil},
		{"git -C /ws restore t.sh", []string{"t.sh"}, nil},
		{"git -C \"$D\" checkout .", []string{"."}, []string{"."}},
		{"git rm a b/", []string{"a", "b/"}, []string{"b/"}},
		{"git -C checkout checkout .", []string{"checkout", "."}, []string{"."}},
		{"git --exec-path=x checkout .", []string{"."}, []string{"."}},
		{"git --super-prefix x checkout .", []string{"."}, []string{"."}},
		{"git -- checkout .", []string{"."}, []string{"."}},
		{"git CHECKOUT .", nil, nil},
		{"git restore --staged x", nil, nil},
		{"git rm --cached x", nil, nil},
		{"git \"$V\" checkout .", nil, nil},
		{"git -c$X checkout .", nil, nil},
		{"git --git-dir=$X checkout .", nil, nil},
	} {
		texts := func(words []Word) []string {
			var out []string
			for _, w := range words {
				out = append(out, w.Text)
			}
			return out
		}
		found := false
		for _, inv := range Parse(tc.line, POSIX).Invocations {
			if len(inv.Words) == 0 || ExecutableName(inv.Words[0].Text) != "git" {
				continue
			}
			found = true
			files, dirs := commandWriteTargets(&inv)
			if !slices.Equal(texts(files), tc.files) || !slices.Equal(texts(dirs), tc.dirs) {
				t.Errorf("%q: files %q dirs %q, want files %q dirs %q", tc.line, texts(files), texts(dirs), tc.files, tc.dirs)
			}
		}
		if !found {
			t.Errorf("%q: no git invocation was parsed", tc.line)
		}
	}
}

func TestScriptWriteToAnotherScriptAcrossNesting(t *testing.T) {
	host := pathshape.For("linux")
	files := chainFiles()
	files["/ws/outer.sh"] = ScriptRead{ID: "11", Text: "bash /ws/writer.sh\n"}
	files["/ws/writer.sh"] = ScriptRead{ID: "12", Text: "echo x >> /ws/outer.sh\n"}
	got := firstScriptReason(parseScriptOn(host, "bash /ws/outer.sh", files))
	if !strings.HasPrefix(got, "script-writes-inspected in /ws/writer.sh:1 (via /ws/outer.sh)") {
		t.Fatalf("got %q", got)
	}
	// The same script text only writes to a file that is not inspected.
	files["/ws/writer.sh"] = ScriptRead{ID: "12", Text: "echo x >> /ws/notes.txt\n"}
	if got := firstScriptReason(parseScriptOn(host, "bash /ws/outer.sh", files)); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestScriptCallRedirectWritesToTheScript(t *testing.T) {
	host := pathshape.For("linux")
	// bash reads a script while it runs, so output appended to the file it is running becomes part of it.
	for _, command := range []string{"bash /ws/ok.sh >> /ws/ok.sh", "bash /ws/ok.sh > ./ok.sh", "bash /ws/ok.sh &>> /ws/ok.sh"} {
		if got := firstScriptReason(parseScriptOn(host, command, chainFiles())); !strings.HasPrefix(got, "script-writes-inspected") {
			t.Errorf("%q: got %q", command, got)
		}
	}
	if got := firstScriptReason(parseScriptOn(host, "bash /ws/ok.sh >> /ws/log.txt 2>&1", chainFiles())); got != "" {
		t.Errorf("a log redirect: got %q", got)
	}
}

// aliasReader answers a second path to the same file with the first path that reached it, as the snapshot does.
type aliasReader struct {
	files fakeReader
	first map[string]string
}

func (a *aliasReader) ReadScript(path string) ScriptRead {
	r := a.files.ReadScript(path)
	if r.ID == "" {
		return r
	}
	if first, ok := a.first[r.ID]; ok {
		r.Path = first
		return r
	}
	a.first[r.ID] = path
	return r
}

func TestScriptWriteToAHardlinkAliasOfAnInspectedScript(t *testing.T) {
	host := pathshape.For("linux")
	text := "echo x >> ./b.sh\n"
	reader := func() *aliasReader {
		return &aliasReader{first: map[string]string{}, files: fakeReader{"/ws/a.sh": {ID: "9", Text: text}, "/ws/b.sh": {ID: "9", Text: text}}}
	}
	res := ParseScripts(t.Context(), "bash ./a.sh && bash ./b.sh", POSIX, ScriptOptions{Reader: reader(), Context: provenPOSIX, Cwd: "/ws", Host: host})
	if got := firstScriptReason(res); !strings.HasPrefix(got, "script-writes-inspected") {
		t.Fatalf("an append to the second spelling of an inspected file: got %q", got)
	}
	if len(res.Scripts) != 1 || res.Scripts[0].Path != "/ws/a.sh" || res.Scripts[0].ID != "9" {
		t.Fatalf("the alias should be one record with the file identity: %+v", res.Scripts)
	}
	// With the second spelling never asked for, nothing ties ./b.sh to the inspected file.
	res = ParseScripts(t.Context(), "bash ./a.sh", POSIX, ScriptOptions{Reader: reader(), Context: provenPOSIX, Cwd: "/ws", Host: host})
	if got := firstScriptReason(res); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestScriptRecordCarriesTheFileIdentity(t *testing.T) {
	res := parseScript("bash /ws/ok.sh", chainFiles())
	if len(res.Scripts) != 1 || res.Scripts[0].ID != "1" {
		t.Fatalf("got %+v", res.Scripts)
	}
}

func TestScriptWriteTargetsFollowTheHostPathRules(t *testing.T) {
	files := fakeReader{"/ws/t.sh": {ID: "1", Text: "echo x >> sub/../log.txt\n"}}
	// Off Windows the shell resolves .. against the real directory, so the cleaned spelling may name another file.
	host := pathshape.For("linux")
	if got := firstScriptReason(parseScriptOn(host, "bash /ws/t.sh", files)); !strings.HasPrefix(got, "script-writes-inspected") {
		t.Errorf("linux: got %q", got)
	}
	// On Windows the spelling decides, and it does not reach an inspected script.
	host = pathshape.For("windows")
	if got := firstScriptReason(parseScriptOn(host, "bash /ws/t.sh", files)); got != "" {
		t.Errorf("windows: got %q", got)
	}
	// Windows compares paths without regard to case.
	files = fakeReader{"/ws/t.sh": {ID: "1", Text: "echo x >> /WS/T.SH\n"}}
	if got := firstScriptReason(parseScriptOn(host, "bash /ws/t.sh", files)); !strings.HasPrefix(got, "script-writes-inspected") {
		t.Errorf("windows case: got %q", got)
	}
	host = pathshape.For("linux")
	if got := firstScriptReason(parseScriptOn(host, "bash /ws/t.sh", files)); got != "" {
		t.Errorf("linux case: got %q", got)
	}
}

func TestScriptWriteToDriveSpelling(t *testing.T) {
	host := pathshape.For("windows")
	files := fakeReader{"C:/ws/t.sh": {ID: "1", Text: "echo x >> 'C:\\ws\\t.sh'\n"}}
	if got := firstScriptReason(parseScriptOn(host, "bash C:/ws/t.sh", files)); !strings.HasPrefix(got, "script-writes-inspected") {
		t.Errorf("got %q", got)
	}
}

// A redirect the lexer cannot take apart is refused whole, so it can neither pass as a predecessor nor hide a write.
func TestRedirectFormsTheParserRefusesStayRefused(t *testing.T) {
	for _, p := range []string{"echo x >| f"} {
		if got := firstScriptReason(parseScript(p+" && bash /ws/ok.sh", chainFiles())); got == "" {
			t.Errorf("%q then a call: passed", p)
		}
		files := chainFiles()
		files["/ws/t.sh"] = ScriptRead{ID: "10", Text: p + "\n"}
		if got := firstScriptReason(parseScript("bash /ws/t.sh", files)); got == "" {
			t.Errorf("%q in a script: passed", p)
		}
	}
}

// A command that is already refused keeps its own reason; the write check does not replace it.
func TestScriptWriteCheckKeepsAnEarlierRefusal(t *testing.T) {
	host := pathshape.For("linux")
	files := chainFiles()
	files["/ws/t.sh"] = ScriptRead{ID: "10", Text: "eval \"$x\" >> /ws/t.sh\n"}
	if got := firstScriptReason(parseScriptOn(host, "bash /ws/t.sh", files)); !strings.HasPrefix(got, "script-opaque:eval") {
		t.Fatalf("got %q", got)
	}
}

func TestScriptWriteTargetsAreComparedInOneForm(t *testing.T) {
	host := pathshape.For("windows")
	for _, line := range []string{"echo x >> /c/ws/t.sh", "echo x >> T.SH", "echo x >> ../WS/T.SH", "echo x >> 'C:\\WS\\T.SH'"} {
		files := fakeReader{"C:/ws/t.sh": {ID: "1", Text: line + "\n"}}
		if got := firstScriptReason(parseScriptOn(host, "bash C:/ws/t.sh", files)); !strings.HasPrefix(got, "script-writes-inspected") {
			t.Errorf("windows %q: got %q", line, got)
		}
	}
	host = pathshape.For("linux")
	files := fakeReader{"/ws/t.sh": {ID: "1", Text: "echo x >> T.SH\n"}}
	if got := firstScriptReason(parseScriptOn(host, "bash /ws/t.sh", files)); got != "" {
		t.Errorf("linux keeps case: got %q", got)
	}
}

// A refused call leaves no inspected script behind, and no spelling of its path either.
func TestRefusedScriptCallLeavesNoSpelling(t *testing.T) {
	waitForNoLeakedForks(t)
	files := fakeReader{"/ws/slow.sh": {ID: "1", Text: "true\n"}}
	p := newParser(t.Context(), POSIX, ScriptOptions{Reader: files, Context: provenPOSIX, Cwd: "/ws"})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	p.script.parseHook = func(*parser) { <-release }
	p.script.deadline = time.Now().Add(50 * time.Millisecond)
	p.parseCommand("bash /ws/slow.sh", POSIX, nil, true, Start, -1, 0)
	if last := p.result.Invocations[len(p.result.Invocations)-1]; last.Reason.String() != "script-budget" {
		t.Fatalf("the call was not abandoned: %+v", last)
	}
	if len(p.result.Scripts) != 0 || len(p.scriptSpellings) != 0 {
		t.Fatalf("scripts %+v spellings %+v", p.result.Scripts, p.scriptSpellings)
	}
}
