package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

var scriptGuards = []string{cg, rg, sg}

// guardsAndGrouped is every per-guard hook plus the grouped event a host sends.
var guardsAndGrouped = []string{cg, rg, sg, pg}

// denyingGuards returns the guards, among the three per-guard hooks, that deny command on surface.
func denyingGuards(t *testing.T, surface surface.ID, dir, command string) []string {
	t.Helper()
	var denied []string
	for _, guard := range scriptGuards {
		code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, dir, command), surface, guard)
		switch scriptVerdict(surface, code, stdout, stderr) {
		case "deny":
			denied = append(denied, guard)
		case "pass":
		default:
			t.Fatalf("%s %s %q: neither pass nor deny: %d %q %q", surface, guard, command, code, stdout, stderr)
		}
	}
	return denied
}

// groupedDenies reports whether the grouped pre-tool event denies command on surface.
func groupedDenies(t *testing.T, surface surface.ID, dir, command string) bool {
	t.Helper()
	code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, dir, command), surface, pg)
	switch scriptVerdict(surface, code, stdout, stderr) {
	case "deny":
		return true
	case "pass":
		return false
	}
	t.Fatalf("%s grouped %q: neither pass nor deny: %d %q %q", surface, command, code, stdout, stderr)
	return false
}

// dangerousFamilies are typed commands that at least one guard denies. Each must keep being denied by the same guards
// when a script holds it, however the script wraps it.
var dangerousFamilies = []string{
	"rm -rf /", "git reset --hard HEAD~1", "git commit -m x --no-verify", "git config core.hooksPath /tmp/h",
	"git clean -fdx", "npm publish", "cat .env", "terraform apply -auto-approve",
}

// scriptShapes are the ways a script can hold one command line.
func scriptShapes(command string) map[string]string {
	return map[string]string{
		"appended": "echo ok\n" + command + "\n",
		"function": "f() { " + command + "; }\nf\n",
		"if":       "if true; then " + command + "; fi\n",
	}
}

// A guard that denies a typed command must deny the same command inside a script, in a function body and in an
// if. The set of denying guards is compared, so a guard that stops seeing script text, or a script rule that
// denies for a guard the typed form passes, both fail.
func TestDangerousFamiliesKeepTheirGuardsInsideScripts(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	slash := filepath.ToSlash(dir)
	call := "cd " + slash + " && bash ./x.sh"
	for _, command := range dangerousFamilies {
		want := denyingGuards(t, "claude-code", dir, command)
		if len(want) == 0 {
			t.Fatalf("typed %q is denied by no guard; fix the family list", command)
		}
		for shape, body := range scriptShapes(command) {
			writeScript(t, dir, "x.sh", body)
			if got := denyingGuards(t, "claude-code", dir, call); !reflect.DeepEqual(got, want) {
				t.Errorf("%q as %s: guards %v, typed form %v", command, shape, got, want)
			}
			if !groupedDenies(t, "claude-code", dir, call) {
				t.Errorf("%q as %s: the grouped event passes", command, shape)
			}
		}
		// The other two surfaces run one shape per family: the judgement is shared, the adapters differ.
		for _, surface := range []surface.ID{surface.Codex, surface.Copilot} {
			typed := denyingGuards(t, surface, dir, command)
			writeScript(t, dir, "x.sh", scriptShapes(command)["appended"])
			if got := denyingGuards(t, surface, dir, call); !reflect.DeepEqual(got, typed) || !reflect.DeepEqual(got, want) {
				t.Errorf("%s: %q appended: guards %v, typed %v, claude-code typed %v", surface, command, got, typed, want)
			}
		}
	}
	// Positive sibling: the same wrappers around a harmless line are not denied by any guard, so the rows above deny
	// for the command inside them.
	for shape, body := range scriptShapes("echo harmless") {
		writeScript(t, dir, "x.sh", body)
		if got := denyingGuards(t, "claude-code", dir, call); len(got) != 0 {
			t.Errorf("harmless %s denied by %v", shape, got)
		}
	}
}

// limitFixture is one named limit or opaque construct. build writes its files into dir and returns the command.
type limitFixture struct {
	name, want string
	build      func(t *testing.T, dir string) string
}

// pad is a comment block of about n bytes ending with the canary, so a size limit is reached without any command in
// the padding and the canary stands for script text that must never reach an output.
func pad(n int) string {
	return strings.Repeat("# padding padding padding padding padding padding padding\n", n/58+1) + "# " + canary + "\n"
}

// callEach is a script that calls every named script by absolute path, one per line.
func callEach(dir string, names ...string) string {
	var b strings.Builder
	for _, name := range names {
		b.WriteString("bash " + filepath.ToSlash(filepath.Join(dir, name)) + "\n")
	}
	return b.String() + "# " + canary + "\n"
}

// cdTo is the qualified-context prefix every relative call uses.
func cdTo(dir string) string { return "cd " + filepath.ToSlash(dir) + " && " }

// single builds a fixture of one script x.sh called from a proven directory.
func single(body string) func(*testing.T, string) string {
	return func(t *testing.T, dir string) string {
		writeScript(t, dir, "x.sh", body+"# "+canary+"\n")
		return cdTo(dir) + "bash ./x.sh"
	}
}

// typed builds a fixture of one typed command over a harmless x.sh.
func typed(command string) func(*testing.T, string) string {
	return func(t *testing.T, dir string) string {
		writeScript(t, dir, "x.sh", "echo ok # "+canary+"\n")
		return cdTo(dir) + command
	}
}

var limitFixtures = []limitFixture{
	{"script-unreadable", "script-unreadable", typed("bash ./nope.sh")},
	// COM0 is a device name on every host the reader refuses, but only a non-Windows host can create the file without
	// risking a device open, so the fixture runs there.
	{"script-unreadable COM0", "script-unreadable", func(t *testing.T, dir string) string {
		if runtime.GOOS == "windows" {
			t.Skip("creating COM0 on Windows may open a device")
		}
		writeScript(t, dir, "COM0.sh", "echo ok # "+canary+"\n")
		return cdTo(dir) + "bash ./COM0.sh"
	}},
	// CRLF and a lone CR end lines differently in the guard and in bash, so both are refused by name. The bodies are
	// written whole: single() would append an LF line.
	{"script-encoding crlf", "script-encoding", func(t *testing.T, dir string) string {
		writeScript(t, dir, "x.sh", "echo ok\r\necho two\r\n")
		return cdTo(dir) + "bash ./x.sh"
	}},
	{"script-encoding lone cr", "script-encoding", func(t *testing.T, dir string) string {
		writeScript(t, dir, "x.sh", "echo ok\recho two\n")
		return cdTo(dir) + "bash ./x.sh"
	}},
	{"script-extension txt", "script-extension", func(t *testing.T, dir string) string {
		writeScript(t, dir, "x.txt", "echo ok\n")
		return cdTo(dir) + "bash ./x.txt"
	}},
	{"script-extension none", "script-extension", func(t *testing.T, dir string) string {
		writeScript(t, dir, "run", "echo ok\n")
		return cdTo(dir) + "bash ./run"
	}},
	// A caller argument that is not a literal cannot be judged, so a script that uses its parameters is refused.
	{"script-caller-argument non-literal", "script-caller-argument", func(t *testing.T, dir string) string {
		writeScript(t, dir, "x.sh", "echo \"$1\" # "+canary+"\n")
		return cdTo(dir) + `bash ./x.sh "$X"`
	}},
	{"script-too-large", "script-too-large", func(t *testing.T, dir string) string {
		writeScript(t, dir, "x.sh", pad(300*1024))
		return cdTo(dir) + "bash ./x.sh"
	}},
	{"script-too-large across files", "script-too-large", func(t *testing.T, dir string) string {
		names := []string{"b1.sh", "b2.sh", "b3.sh", "b4.sh", "b5.sh"}
		for _, name := range names {
			writeScript(t, dir, name, pad(250*1024))
		}
		writeScript(t, dir, "x.sh", callEach(dir, names...))
		return cdTo(dir) + "bash ./x.sh"
	}},
	{"script-too-many-files", "script-too-many-files", func(t *testing.T, dir string) string {
		var names []string
		// x.sh and eight callees make nine files, one past the per-event cap of eight.
		for i := 1; i <= 8; i++ {
			name := "f" + strconv.Itoa(i) + ".sh"
			writeScript(t, dir, name, "echo "+strconv.Itoa(i)+"\n")
			names = append(names, name)
		}
		writeScript(t, dir, "x.sh", callEach(dir, names...))
		return cdTo(dir) + "bash ./x.sh"
	}},
	// Four nested scripts: past the chain bound of three but inside the wrapper-depth bound, so only the chain bound
	// can name it.
	{"script-depth", "script-depth", func(t *testing.T, dir string) string {
		writeScript(t, dir, "d4.sh", "echo leaf # "+canary+"\n")
		for i := 3; i >= 1; i-- {
			writeScript(t, dir, "d"+strconv.Itoa(i)+".sh", callEach(dir, "d"+strconv.Itoa(i+1)+".sh"))
		}
		return cdTo(dir) + "bash ./d1.sh"
	}},
	{"script-cycle mutual", "script-cycle", func(t *testing.T, dir string) string {
		writeScript(t, dir, "a.sh", callEach(dir, "b.sh"))
		writeScript(t, dir, "b.sh", callEach(dir, "a.sh"))
		return cdTo(dir) + "bash ./a.sh"
	}},
	{"script-cycle self", "script-cycle", func(t *testing.T, dir string) string {
		writeScript(t, dir, "a.sh", callEach(dir, "a.sh"))
		return cdTo(dir) + "bash ./a.sh"
	}},
	{"script-path-protected", "script-path-protected", func(t *testing.T, dir string) string {
		writeScript(t, dir, ".env.sh", "echo ok # "+canary+"\n")
		return cdTo(dir) + "bash ./.env.sh"
	}},
	{"script-path-not-literal", "script-path-not-literal", typed(`bash "$(pwd)/x.sh"`)},
	{"script-path-not-literal network", "script-path-not-literal", typed("bash //srv/share/a.sh")},
	{"script-cwd-unproven", "script-cwd-unproven", func(t *testing.T, dir string) string {
		writeScript(t, dir, "x.sh", "echo ok # "+canary+"\n")
		return `cd "$UNSET" && bash ./x.sh`
	}},
	{"script-form-unsupported-option", "script-form-unsupported-option", typed("bash -x ./x.sh")},
	{"script-operand-not-literal", "script-operand-not-literal", single("rm -rf \"$UNSET\"\n")},
	{"script-caller-argument", "script-caller-argument", func(t *testing.T, dir string) string {
		writeScript(t, dir, "x.sh", "git \"$@\" # "+canary+"\n")
		return cdTo(dir) + "bash ./x.sh status"
	}},
	{"script-stdin-fed", "script-stdin-fed", func(t *testing.T, dir string) string {
		writeScript(t, dir, "x.sh", "read line\necho ok # "+canary+"\n")
		return "echo hi | bash " + filepath.ToSlash(filepath.Join(dir, "x.sh"))
	}},
	{"script-env-pinned", "script-env-pinned", typed("PATH=/x bash ./x.sh")},
	{"script-predecessor-may-write", "script-predecessor-may-write", typed("make && bash ./x.sh")},
	{"script-predecessor-state", "script-predecessor-state", typed("export PATH=/x && bash ./x.sh")},
	{"script-writes-inspected", "script-writes-inspected", typed("bash ./x.sh && echo more >> ./x.sh")},
	{"script-opaque:eval", "script-opaque:eval", single("eval \"$UNSET\"\n")},
	{"script-opaque:source", "script-opaque:source", single("source \"$HOME/lib.sh\"\n")},
	{"script-opaque:trap", "script-opaque:trap", single("trap 'echo bye' EXIT\n")},
	{"script-opaque:command-position-expansion", "script-opaque:command-position-expansion", single("$UNSET arg\n")},
	{"script-opaque:dynamic-c", "script-opaque:dynamic-c", single("bash -c \"$UNSET\"\n")},
	{"script-opaque:stdin-to-shell", "script-opaque:stdin-to-shell", single("echo ls | bash\n")},
	{"script-opaque:other-shell", "script-opaque:other-shell", single("zsh -c 'echo ok'\n")},
	{"script-opaque:pinned-assignment", "script-opaque:pinned-assignment", single("PATH=/x\necho ok\n")},
	{"script-opaque:function-shadow", "script-opaque:function-shadow", single("rm() { :; }\necho ok\n")},
	{"script-opaque:heredoc-delimiter", "script-opaque:heredoc-delimiter", single("cat <<\"E\\X\"\nbody\nE\\X\n")},
	// A script past the prescan caps (10,001 lines, 65 nesting levels) is refused before any parse, so the verdict
	// needs no clock and the three guards agree.
	{"script-budget lines", "script-budget", func(t *testing.T, dir string) string {
		writeScript(t, dir, "x.sh", strings.Repeat("echo ok\n", 10001)+"# "+canary+"\n")
		return cdTo(dir) + "bash ./x.sh"
	}},
	{"script-budget nesting", "script-budget", single(strings.Repeat("{ ", 65) + "echo ok" + strings.Repeat("; }", 65) + "\n")},
}

// Each fixture is denied by all three guards on all three surfaces, with the limit's name in the reason and no script
// text in any output. It covers the limits and constructs listed above; script-context-unqualified needs a tool
// environment and is a guard-regressions.json case, and script-budget is covered only by the prescan caps (a stalled
// read is a snapshot-level test and the parse ceiling is judged per guard). The sibling test passes the shapes just
// inside the limits, which shows the denials come from the named construct.
func TestNamedLimitFixtures(t *testing.T) {
	pinScriptGuardEnv(t)
	// A subtest's own TempDir is named after the subtest, and a deny shows the script path, so the limit's name would
	// appear in the reason whether or not the limit fired. The fixtures live in numbered directories instead.
	parent := t.TempDir()
	for i, fixture := range limitFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			dir := filepath.Join(parent, "d"+strconv.Itoa(i))
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			command := fixture.build(t, dir)
			for _, surface := range scriptSurfaces {
				for _, guard := range guardsAndGrouped {
					code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, dir, command), surface, guard)
					if strings.Contains(stdout+stderr, canary) {
						t.Errorf("%s %s leaks script text: %q %q", surface, guard, stdout, stderr)
					}
					if guard == pg {
						reason := requireDeny(t, surface, code, stdout, stderr)
						if !strings.Contains(reason, fixture.want) {
							t.Errorf("%s grouped: reason %q lacks %q", surface, reason, fixture.want)
						}
						continue
					}
					if scriptVerdict(surface, code, stdout, stderr) != "deny" {
						t.Errorf("%s %s: not denied: %d %q %q", surface, guard, code, stdout, stderr)
						continue
					}
					requireOutcome(t, surface, "deny", fixture.want, code, stdout, stderr)
				}
			}
		})
	}
}

// Positive siblings for the limit table: the shape that sits just inside each limit passes in all three guards.
func TestNamedLimitSiblingsPass(t *testing.T) {
	pinScriptGuardEnv(t)
	siblings := map[string]func(t *testing.T, dir string) string{
		"depth 3": func(t *testing.T, dir string) string {
			writeScript(t, dir, "d3.sh", "echo leaf\n")
			writeScript(t, dir, "d2.sh", callEach(dir, "d3.sh"))
			writeScript(t, dir, "d1.sh", callEach(dir, "d2.sh"))
			return cdTo(dir) + "bash ./d1.sh"
		},
		"8 files": func(t *testing.T, dir string) string {
			var names []string
			for i := 1; i <= 7; i++ {
				name := "f" + strconv.Itoa(i) + ".sh"
				writeScript(t, dir, name, "echo "+strconv.Itoa(i)+"\n")
				names = append(names, name)
			}
			writeScript(t, dir, "x.sh", callEach(dir, names...))
			return cdTo(dir) + "bash ./x.sh"
		},
		"two files of 250 KiB": func(t *testing.T, dir string) string {
			writeScript(t, dir, "b1.sh", pad(250*1024))
			writeScript(t, dir, "b2.sh", pad(250*1024))
			writeScript(t, dir, "x.sh", callEach(dir, "b1.sh", "b2.sh"))
			return cdTo(dir) + "bash ./x.sh"
		},
		"proven cd": typed("bash ./x.sh"),
		"allowed option": func(t *testing.T, dir string) string {
			writeScript(t, dir, "x.sh", "echo ok\n")
			return cdTo(dir) + "bash -e ./x.sh"
		},
		"writes elsewhere": func(t *testing.T, dir string) string {
			writeScript(t, dir, "x.sh", "echo ok\n")
			return cdTo(dir) + "bash ./x.sh && echo more >> ./log.txt"
		},
		"caller argument to a forwarder": func(t *testing.T, dir string) string {
			writeScript(t, dir, "x.sh", "git status \"$@\"\n")
			return cdTo(dir) + "bash ./x.sh --short"
		},
		"literal source": func(t *testing.T, dir string) string {
			writeScript(t, dir, "lib.sh", "echo lib\n")
			writeScript(t, dir, "x.sh", "source ./lib.sh\n")
			return cdTo(dir) + "bash ./x.sh"
		},
		"COM10 name": func(t *testing.T, dir string) string {
			writeScript(t, dir, "COM10.sh", "echo ok\n")
			return cdTo(dir) + "bash ./COM10.sh"
		},
		"literal bash -c": single("bash -c 'echo ok'\n"),
		"plain heredoc":   single("cat <<EOF\nhello\nEOF\n"),
		"10000 lines": func(t *testing.T, dir string) string {
			writeScript(t, dir, "x.sh", strings.Repeat("echo ok\n", 10000))
			return cdTo(dir) + "bash ./x.sh"
		},
		"literal operand": single("rm -f ./out.txt\n"),
		"own function":    single("greet() { echo hi; }\ngreet\n"),
		"plain variable":  single("name=world\necho \"hello $name\"\n"),
	}
	for name, build := range siblings {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			command := build(t, dir)
			for _, surface := range scriptSurfaces {
				if got := denyingGuards(t, surface, dir, command); len(got) != 0 {
					t.Errorf("%s denied by %v", surface, got)
				}
				if groupedDenies(t, surface, dir, command) {
					t.Errorf("%s grouped event denies", surface)
				}
			}
		})
	}
}

// The helpers the workflow depends on run through the guards, alone and after each predecessor an agent commonly
// puts in front of a script call. The source-digest helper is a frozen copy, so the entry outlives the original.
func TestHeadlineHelpersPass(t *testing.T) {
	pinScriptGuardEnv(t)
	root := filepath.ToSlash(hosttest.Checkout(t))
	helpers := []string{
		"bash hooks/go/internal/shellwords/testdata/flow.sh status",
		"bash hooks/go/cmd/hook/testdata/corpus/scripts/source-digest.sh hooks/go",
		"bash skills/diagnose/references/find-polluter.sh tmp/out.txt 'src/a.test.ts'",
		"sh skills/diagnose/references/find-polluter.sh tmp/out.txt 'src/a.test.ts'",
		"sh skills/scaffold/scripts/planning-protection.sh --parent docs/specs --gitignore .gitignore",
	}
	predecessors := []string{
		"", `cd "` + root + `"`, "git add -A", "git commit -m x", "git status", "mkdir -p out", "echo hi", "head -n 1 README.md",
	}
	for _, helper := range helpers {
		for _, predecessor := range predecessors {
			command := helper
			if predecessor != "" {
				command = predecessor + " && " + helper
			}
			t.Run(command, func(t *testing.T) {
				for _, surface := range scriptSurfaces {
					for _, guard := range guardsAndGrouped {
						code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, root, command), surface, guard)
						requireOutcome(t, surface, "pass", "", code, stdout, stderr)
					}
				}
			})
		}
	}
}

// A mention of a dangerous command that is data, not a command, does not deny where the parsed rules judge the script:
// a comment, an echo argument and a heredoc body are not invocations. A rule that exists only in the text bank scans
// the script's text instead, so the same mention denies. That is a known over-refusal, the same one a typed command
// gets, and the rows pin it so a change in either direction is noticed.
func TestScriptMentionsOfDangerousCommands(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	rows := []struct {
		name, body string
		denied     bool
	}{
		{"parsed rule in a comment", "# rm -rf /\necho ok\n", false},
		{"parsed rule in a trailing comment", "echo ok # rm -rf /\n", false},
		{"parsed rule in a quoted heredoc", "cat <<'EOF'\nrm -rf /\nEOF\nrm -f ./out.txt\n", false},
		{"parsed rule as an echo argument", "echo 'rm -rf /'\n", false},
		{"release and secrets families in comments", "# terraform apply -auto-approve\n# cat .env\necho ok\n", false},
		{"text-bank rule in a comment", "# Clear-Disk -Number 1\necho ok\n", true},
		{"text-bank rule as an echo argument", "echo Clear-Disk -Number 1\n", true},
		{"text-bank rule in a quoted heredoc", "cat <<'EOF'\nClear-Disk -Number 1\nEOF\n", true},
		{"the command itself", "rm -rf /\n", true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			writeScript(t, dir, "x.sh", row.body)
			command := cdTo(dir) + "bash ./x.sh"
			var want []string
			if row.denied {
				want = []string{cg}
			}
			if got := denyingGuards(t, "claude-code", dir, command); !reflect.DeepEqual(got, want) {
				t.Errorf("denied by %v, want %v", got, want)
			}
		})
	}
}

// Helpers whose commands the guards cannot read stay blocked, and the reason names the construct. The helpers are
// frozen copies, so the entries outlive the originals.
func TestUninspectableHelpersStayBlocked(t *testing.T) {
	pinScriptGuardEnv(t)
	root := filepath.ToSlash(hosttest.Checkout(t))
	for _, tc := range []struct{ command, want string }{
		{"bash hooks/go/cmd/hook/testdata/corpus/scripts/verify-stage.sh", "script-opaque:source"},
		{"bash hooks/go/cmd/hook/testdata/corpus/scripts/plugin.sh --help", "script-opaque:source"},
	} {
		for _, guard := range scriptGuards {
			code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin("claude-code", root, tc.command), "claude-code", guard)
			requireOutcome(t, "claude-code", "deny", tc.want, code, stdout, stderr)
		}
	}
}

// countingScripts counts the reads a guard asks of the shared reader.
type countingScripts struct {
	inner shellwords.ScriptReader
	reads *atomic.Int64
}

func (c countingScripts) ReadScript(path string) shellwords.ScriptRead {
	c.reads.Add(1)
	return c.inner.ReadScript(path)
}

// readsFor runs one guard entry with a counting reader wrapped around the production snapshot and returns the count.
func readsFor(t *testing.T, surface surface.ID, dir, command, entry string) int64 {
	t.Helper()
	deps := productionHookDependencies()
	lookup := deps.lookup
	var reads atomic.Int64
	deps.lookup = func(name string) (hookspec.Handler, bool) {
		handler, ok := lookup(name)
		if !ok {
			return nil, false
		}
		return func(req hookspec.Request) hookspec.Outcome {
			if req.Scripts != nil {
				req.Scripts = countingScripts{inner: req.Scripts, reads: &reads}
			}
			return handler(req)
		}, true
	}
	invokeWithLegalArgsAndDependencies(t, deps, scriptStdin(surface, dir, command), surface, entry)
	return reads.Load()
}

// A direct invocation, an interpreter other than a shell, and bash -c are never opened: no guard asks the reader for
// a file. The shell-script call is the control that proves the counter is wired into the same path.
func TestDirectInvocationIsNeverRead(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	writeScript(t, dir, "x.sh", "echo ok\n")
	writeScript(t, dir, "x.py", "print('ok')\n")
	cd := cdTo(dir)
	for _, entry := range guardsAndGrouped {
		for _, command := range []string{cd + "./x.sh", cd + "python x.py", cd + "bash -c 'echo x'"} {
			if n := readsFor(t, "claude-code", dir, command, entry); n != 0 {
				t.Errorf("%s %q read %d scripts", entry, command, n)
			}
		}
		if n := readsFor(t, "claude-code", dir, cd+"bash ./x.sh", entry); n == 0 {
			t.Errorf("%s: control call read nothing, so the counter is not wired", entry)
		}
	}
}

// realWorkflowRows are common agent command lines. An empty want passes; otherwise every guard denies with the name.
var realWorkflowRows = []struct{ command, want string }{
	{"bash scripts/x.sh", ""},
	{"cd repo && bash ./ci.sh", ""},
	{"bash ./a.sh && git commit -m wip", ""},
	{"set -e; bash ./x.sh", ""},
	{"export CI=1 && bash ./x.sh", ""},
	{"bash ./x.sh | tee log", ""},
	{"FOO=1 bash ./x.sh", ""},
	{"cd {DIR} && bash ./x.sh", ""},
	{"bash ./x.sh > out.txt 2>&1", ""},
	{"npm test && bash ./deploy.sh", "script-predecessor-may-write"},
	{"git pull && bash ./x.sh", "script-predecessor-may-write"},
	{"make && bash ./run.sh", "script-predecessor-may-write"},
	{"bash -x ./x.sh", "script-form-unsupported-option"},
	{`bash "$(pwd)/x.sh"`, "script-path-not-literal"},
	{`cd {DIR} && bash ./x.sh && git commit -m "fix for x"`, "script-cwd-unproven"},
	{"for f in a b; do bash ./x.sh; done", "script-cwd-unproven"},
	// Known over-refusal: the flat replay sees a pipe after the cd and treats the cd as possibly run in a pipeline
	// subshell, though here the pipe joins only the echo and the call and a failed cd would skip both. Only an
	// absolute path works there.
	{"cd {DIR} && echo hi | bash ./x.sh", "script-cwd-unproven"},
}

// TestRealWorkflowTable runs the table on every surface and guard. Every guard agrees on every row.
func TestRealWorkflowTable(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	for _, name := range []string{"x.sh", "a.sh", "deploy.sh", "run.sh"} {
		writeScript(t, dir, name, "echo ok\n")
	}
	for _, sub := range []string{"scripts", "repo"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		writeScript(t, filepath.Join(dir, sub), map[string]string{"scripts": "x.sh", "repo": "ci.sh"}[sub], "echo ok\n")
	}
	for _, row := range realWorkflowRows {
		command := strings.ReplaceAll(row.command, "{DIR}", filepath.ToSlash(dir))
		t.Run(row.command, func(t *testing.T) {
			for _, surface := range scriptSurfaces {
				for _, guard := range guardsAndGrouped {
					code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, dir, command), surface, guard)
					if row.want == "" {
						requireOutcome(t, surface, "pass", "", code, stdout, stderr)
					} else if guard == pg {
						if reason := requireDeny(t, surface, code, stdout, stderr); !strings.Contains(reason, row.want) {
							t.Errorf("%s grouped: reason %q lacks %q", surface, reason, row.want)
						}
					} else {
						requireOutcome(t, surface, "deny", row.want, code, stdout, stderr)
					}
				}
			}
		})
	}
}

// Operands of a destructive command that a script builds from a variable, a loop or a function argument. A constant
// is substituted and judged as the typed command would be, so only command-guardian, which owns the destructive
// family, denies it. An operand the parser cannot prove literal is refused by name before any guard's rules run, so
// all three guards deny it.
func TestScriptOperandShapesAreJudgedByEveryGuard(t *testing.T) {
	pinScriptGuardEnv(t)
	parent := t.TempDir()
	rows := []struct {
		name, body, want string
		denying          []string
	}{
		{"constant variable", "D=/\nrm -rf $D\n", "destructive_file_ops", []string{cg}},
		{"loop variable", "for d in /; do rm -rf \"$d\"; done\n", "script-operand-not-literal", scriptGuards},
		{"function argument", "f() { rm -rf \"$1\"; }\nf /\n", "script-operand-not-literal", scriptGuards},
		{"harmless constant", "D=./out\nrm -f $D\n", "", nil},
	}
	for i, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			dir := filepath.Join(parent, "d"+strconv.Itoa(i))
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			writeScript(t, dir, "x.sh", row.body)
			command := cdTo(dir) + "bash ./x.sh"
			if got := denyingGuards(t, "claude-code", dir, command); !reflect.DeepEqual(got, row.denying) {
				t.Errorf("denied by %v, want %v", got, row.denying)
			}
			for _, guard := range row.denying {
				code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin("claude-code", dir, command), "claude-code", guard)
				requireOutcome(t, "claude-code", "deny", row.want, code, stdout, stderr)
			}
		})
	}
}
