package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// writeScript creates one fixture script. The guards read it as data; no test
// ever runs it.
func writeScript(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// scriptStdin is the payload a surface's host writes for a shell call in dir.
func scriptStdin(surface surface.ID, dir, command string) string {
	return string(hosttest.Stdin(surface, hosttest.Call{Tool: "Bash", Input: map[string]any{"command": command}, Cwd: dir}))
}

// pinScriptGuardEnv fixes every variable that could turn a guard off, switch it
// to warn, authorize a deploy, or write a log outside the test, so a row's
// verdict comes from the script content alone. HOME is moved too, because a
// user settings file can switch off the policy a row relies on.
func pinScriptGuardEnv(t *testing.T) {
	t.Helper()
	hosttest.ScrubGuardEnv(t)
	hosttest.IsolateHome(t)
	t.Setenv("COMMAND_GUARD_MODE", "block")
	t.Setenv("COMMAND_GUARD_ALLOWLIST", "")
	t.Setenv("SKIP_COMMAND_GUARD", "")
	t.Setenv("RELEASE_GUARD_MODE", "block")
	t.Setenv("SECRETS_GUARD_MODE", "block")
	t.Setenv("AGX_RELEASE_RECORD", "")
	t.Setenv("COMMAND_GUARD_LOG_DIR", t.TempDir())
	t.Setenv("RELEASE_GUARD_LOG_DIR", t.TempDir())
	t.Setenv("SECRETS_LOG_DIR", t.TempDir())
}

// scriptFixtures writes one benign script and one script per denied family
// into dir.
func scriptFixtures(t *testing.T, dir string) {
	t.Helper()
	writeScript(t, dir, "ok.sh", "#!/usr/bin/env bash\necho hello\n")
	writeScript(t, dir, "rm.sh", "echo hi\nrm -rf /\n")
	writeScript(t, dir, "reset.sh", "echo hi\ngit reset --hard HEAD~1\n")
	writeScript(t, dir, "env.sh", "echo hi\ncat .env\n")
	writeScript(t, dir, "deploy.sh", "echo hi\nterraform apply -auto-approve\n")
}

var scriptSurfaces = surface.All()

func TestScriptCallsAreJudgedByContent(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	slash := filepath.ToSlash(dir)
	scriptFixtures(t, dir)
	cd := "cd " + slash + " && "
	// want is "" for a pass. Otherwise every fragment must appear in the deny
	// reason; a content row names the family the script's own command belongs
	// to, which an uninspected script call can never produce, and refuses the
	// parse-limit category so it cannot pass on the old blanket block.
	cases := []struct {
		name, command, guard string
		want                 []string
		notWant              string
	}{
		{"benign command", cd + "bash ./ok.sh arg", "command-guardian", nil, ""},
		{"benign release", cd + "bash ./ok.sh arg", "release-guardian", nil, ""},
		{"benign secrets", cd + "bash ./ok.sh arg", "secrets-guardian", nil, ""},
		{"destructive", cd + "bash ./rm.sh", "command-guardian", []string{"blocking threat", "destructive_file_ops"}, "command_parse_limit"},
		{"reset_hard", cd + "bash ./reset.sh", "command-guardian", []string{"blocking threat", "destructive_git_ops"}, "command_parse_limit"},
		{"deploy", cd + "bash ./deploy.sh", "release-guardian", []string{"production-shaped deploy command"}, ""},
		{"credential read", cd + "bash ./env.sh", "secrets-guardian", []string{"credential file read(s) blocked"}, ""},
		{"missing script command", cd + "bash ./nope.sh", "command-guardian", []string{"script-unreadable"}, ""},
		{"missing script release", cd + "bash ./nope.sh", "release-guardian", []string{"script-unreadable"}, ""},
		{"missing script secrets", cd + "bash ./nope.sh", "secrets-guardian", []string{"script-unreadable"}, ""},
		{"unsupported option", cd + "bash -x ./ok.sh", "command-guardian", []string{"script-form-unsupported-option"}, ""},
		{"direct invocation is not read", cd + "./rm.sh", "command-guardian", nil, ""},
	}
	for _, surface := range scriptSurfaces {
		for _, tc := range cases {
			t.Run(string(surface)+"/"+tc.name, func(t *testing.T) {
				code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, dir, tc.command), surface, tc.guard)
				if tc.want == nil {
					requireOutcome(t, surface, "pass", "", code, stdout, stderr)
					return
				}
				for _, fragment := range tc.want {
					requireOutcome(t, surface, "deny", fragment, code, stdout, stderr)
				}
				if tc.notWant != "" && strings.Contains(stdout+stderr, tc.notWant) {
					t.Fatalf("deny reason names %q, so the script content was not judged: %q %q", tc.notWant, stdout, stderr)
				}
			})
		}
	}
}

// TestScriptCallsGroupedEvent drives all three guards through the one grouped
// pre-tool event a host sends: a benign script call must clear every guard,
// and a destructive one must still be denied.
func TestScriptCallsGroupedEvent(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	slash := filepath.ToSlash(dir)
	scriptFixtures(t, dir)
	for _, surface := range scriptSurfaces {
		t.Run(string(surface)+"/benign passes", func(t *testing.T) {
			code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, dir, "cd "+slash+" && bash ./ok.sh arg"), surface, "pre-tool")
			requireSilent(t, surface, code, stdout, stderr)
		})
		t.Run(string(surface)+"/destructive is denied", func(t *testing.T) {
			code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, dir, "cd "+slash+" && bash ./rm.sh"), surface, "pre-tool")
			reason := requireDeny(t, surface, code, stdout, stderr)
			if !strings.Contains(reason, "destructive_file_ops") {
				t.Fatalf("grouped deny does not come from the script content: %q", reason)
			}
		})
	}
}

func TestScriptCallFromThePowerShellToolFollowsTheSameRules(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	scriptFixtures(t, dir)
	// Each script is denied by the one guard whose family it belongs to and passed by the other two, so a guard that
	// stops reading script text from the PowerShell tool fails its own row.
	rows := []struct{ name, guard, want string }{
		{"ok.sh", cg, ""}, {"ok.sh", rg, ""}, {"ok.sh", sg, ""},
		{"rm.sh", cg, "destructive_file_ops"}, {"rm.sh", rg, ""}, {"rm.sh", sg, ""},
		{"deploy.sh", rg, "production-shaped deploy command"}, {"deploy.sh", cg, ""}, {"deploy.sh", sg, ""},
		{"env.sh", sg, "credential file read(s) blocked"}, {"env.sh", cg, ""}, {"env.sh", rg, ""},
	}
	for _, row := range rows {
		t.Run(row.name+"/"+row.guard, func(t *testing.T) {
			payload := string(hosttest.Stdin("claude-code", hosttest.Call{Tool: "PowerShell", Input: map[string]any{"command": "bash " + filepath.ToSlash(filepath.Join(dir, row.name))}, Cwd: dir}))
			code, stdout, stderr := invokeWithLegalArgs(t, payload, "claude-code", row.guard)
			if row.want == "" {
				requireOutcome(t, "claude-code", "pass", "", code, stdout, stderr)
				return
			}
			requireOutcome(t, "claude-code", "deny", row.want, code, stdout, stderr)
		})
	}
}

func TestRelativeScriptWithoutProvenCwdIsDenied(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	writeScript(t, dir, "ok.sh", "echo hi\n")
	code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin("claude-code", dir, "cd \"$X\" && bash ./ok.sh"), "claude-code", "command-guardian")
	requireOutcome(t, "claude-code", "deny", "script-cwd-unproven", code, stdout, stderr)
}

// An event whose cwd is relative names no directory, so a relative script call is refused rather than read against
// whatever directory the text happens to resolve to.
func TestRelativeScriptUnderARelativeEventCwdIsDenied(t *testing.T) {
	pinScriptGuardEnv(t)
	for _, surface := range scriptSurfaces {
		for _, cwd := range []string{".", "sub/dir"} {
			t.Run(string(surface)+"/"+cwd, func(t *testing.T) {
				code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, cwd, "bash ./ok.sh"), surface, "command-guardian")
				requireOutcome(t, surface, "deny", "script-cwd-unproven", code, stdout, stderr)
			})
		}
	}
}

// Off Windows the shell resolves .. against the real directory, so a .. after a symlinked directory leaves the
// directory the spelling suggests. Every guard refuses the spelling instead of inspecting the file the cleaned path
// names while the shell runs another.
func TestScriptParentReferenceBehindASymlinkIsRefusedByEveryGuard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows resolves .. by spelling")
	}
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	ws, elsewhere := filepath.Join(dir, "ws"), filepath.Join(dir, "e", "sub")
	for _, d := range []string{filepath.Join(ws, "a"), elsewhere} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// ws/a/link and ws/link both reach e/sub. The harmless ws/a/b.sh and ws/b.sh are what the cleaned spellings name;
	// the shell runs e/b.sh.
	if err := os.Symlink(elsewhere, filepath.Join(ws, "a", "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(ws, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeScript(t, ws, "b.sh", "echo harmless\n")
	writeScript(t, filepath.Join(ws, "a"), "b.sh", "echo harmless\n")
	writeScript(t, filepath.Dir(elsewhere), "b.sh", "rm -rf /\n")
	slash := filepath.ToSlash(ws)
	cases := []struct{ command, want string }{
		{"cd " + slash + " && bash a/link/../b.sh", "script-path-not-literal"},
		{"cd " + slash + " && bash ./a/link/../b.sh", "script-path-not-literal"},
		{"bash " + slash + "/a/link/../b.sh", "script-path-not-literal"},
		{"cd " + slash + "/a/link && bash ../b.sh", "script-path-not-literal"},
		{"cd " + slash + " && cd link && bash ../b.sh", "script-path-not-literal"},
		{"cd " + slash + " && cd link && cd .. && bash ./b.sh", "script-cwd-unproven"},
	}
	for _, surface := range scriptSurfaces {
		for _, tc := range cases {
			for _, entry := range []string{"command-guardian", "release-guardian", "secrets-guardian"} {
				t.Run(string(surface)+"/"+tc.command+"/"+entry, func(t *testing.T) {
					code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, ws, tc.command), surface, entry)
					requireOutcome(t, surface, "deny", tc.want, code, stdout, stderr)
				})
			}
		}
	}
}

// A credential-named script is refused as a protected path by all three guards, from the grouped event, and from the
// legacy single-guard entry, with the same limit in each reason: the policy is part of the one shared snapshot.
func TestCredentialNamedScriptIsProtectedByEveryGuard(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	if err := os.MkdirAll(filepath.Join(home, ".aws"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeScript(t, dir, ".env.sh", "echo hi\n")
	writeScript(t, filepath.Join(home, ".aws"), "x.sh", "echo hi\n")
	slash := filepath.ToSlash(dir)
	cases := []struct{ name, command string }{
		{"dotenv in the fixture directory", "cd " + slash + " && bash ./.env.sh"},
		{"dotenv by absolute path", "bash " + slash + "/.env.sh"},
		{"aws directory in a temp home", "bash " + filepath.ToSlash(filepath.Join(home, ".aws", "x.sh"))},
		{"missing credential-named script", "bash " + slash + "/.env.missing.sh"},
	}
	entries := []string{"command-guardian", "release-guardian", "secrets-guardian", "pre-tool"}
	for _, surface := range scriptSurfaces {
		for _, tc := range cases {
			for _, entry := range entries {
				t.Run(string(surface)+"/"+tc.name+"/"+entry, func(t *testing.T) {
					code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, dir, tc.command), surface, entry)
					if entry == "pre-tool" {
						reason := requireDeny(t, surface, code, stdout, stderr)
						if !strings.Contains(reason, "script-path-protected") {
							t.Fatalf("grouped deny is not script-path-protected: %q", reason)
						}
						return
					}
					requireOutcome(t, surface, "deny", "script-path-protected", code, stdout, stderr)
				})
			}
		}
	}
}

// A script word spelled as a network path or with a backslash is refused as written by every guard: cleaning it into a
// local path would inspect a different file than the one the shell opens.
func TestScriptPathSpellingsThatCleanIntoAnotherFileAreRefused(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	slash := filepath.ToSlash(dir)
	writeScript(t, dir, "b.sh", "echo hi\n")
	if err := os.Mkdir(filepath.Join(dir, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeScript(t, filepath.Join(dir, "a"), "b.sh", "echo hi\n")
	commands := []string{"bash //srv/share/a.sh", `bash '\\srv\share\a.sh'`}
	if runtime.GOOS != "windows" {
		commands = append(commands, `bash 'a\b.sh'`)
	}
	for _, surface := range scriptSurfaces {
		for _, command := range commands {
			for _, entry := range []string{"command-guardian", "release-guardian", "secrets-guardian"} {
				t.Run(string(surface)+"/"+command+"/"+entry, func(t *testing.T) {
					code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, dir, "cd "+slash+" && "+command), surface, entry)
					requireOutcome(t, surface, "deny", "script-path-not-literal", code, stdout, stderr)
				})
			}
		}
	}
}

// readGuardLog returns the guard.log the command guard wrote under dir.
func readGuardLog(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "guard.log"))
	if err != nil {
		t.Fatalf("guard.log not written: %v", err)
	}
	return string(data)
}

// A deny names the rule, the script path and the line, and nothing the script says: not the reason, not the
// diagnostics table, not guard.log. Only the matched text of the script is held back; the typed command is not
// script content, so the same fields may still show it.
func TestScriptDenyReasonNamesPathAndLineButNotContent(t *testing.T) {
	pinScriptGuardEnv(t)
	logDir := t.TempDir()
	t.Setenv("COMMAND_GUARD_LOG_DIR", logDir)
	dir := t.TempDir()
	writeScript(t, dir, "rm.sh", "echo one\necho two\nrm -rf /\n")
	command := "cd " + filepath.ToSlash(dir) + " && bash ./rm.sh"
	code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin("claude-code", dir, command), "claude-code", "command-guardian")
	requireOutcome(t, "claude-code", "deny", "rm.sh:3", code, stdout, stderr)
	log := readGuardLog(t, logDir)
	// The host adapter discards the threat table on a deny, so the table itself is pinned in the commandguard tests;
	// here the stream the host reads (stderr on Claude Code) and the log file are checked.
	for name, text := range map[string]string{"deny output": stdout, "stderr": stderr, "guard.log": log} {
		if strings.Contains(text, "rm -rf") {
			t.Fatalf("%s leaks script text: %q", name, text)
		}
	}
	if !strings.Contains(log, "script content redacted (") || !strings.Contains(log, "rm.sh:3)") {
		t.Fatalf("guard.log does not carry the redaction label: %q", log)
	}
}

// A rule that exists only in the text bank, with no parsed counterpart, still fires from script text, and is
// denied with the redacted label instead of the matched text.
func TestScriptTextBankRuleFiresFromScriptContent(t *testing.T) {
	pinScriptGuardEnv(t)
	logDir := t.TempDir()
	t.Setenv("COMMAND_GUARD_LOG_DIR", logDir)
	dir := t.TempDir()
	writeScript(t, dir, "wipe.sh", "echo one\nClear-Disk -Number 1\n")
	command := "cd " + filepath.ToSlash(dir) + " && bash ./wipe.sh"
	code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin("claude-code", dir, command), "claude-code", "command-guardian")
	requireOutcome(t, "claude-code", "deny", "destructive_file_ops", code, stdout, stderr)
	requireOutcome(t, "claude-code", "deny", "wipe.sh:2", code, stdout, stderr)
	log := readGuardLog(t, logDir)
	for name, text := range map[string]string{"deny output": stdout, "diagnostics stream": stderr, "guard.log": log} {
		if strings.Contains(strings.ToLower(text), "clear-disk") {
			t.Fatalf("%s leaks script text: %q", name, text)
		}
	}
}

// callerArgFixtures writes the scripts the caller-argument rows call: one per way a script can use its parameters.
func callerArgFixtures(t *testing.T, dir string) {
	t.Helper()
	writeScript(t, dir, "commit.sh", "git commit -m msg \"$1\"\n")
	writeScript(t, dir, "cat.sh", "cat \"$1\"\n")
	writeScript(t, dir, "unq.sh", "cat $1\n")
	writeScript(t, dir, "none.sh", "echo hi\n")
	writeScript(t, dir, "assign.sh", "msg=$1\necho \"$msg\"\n")
}

// A parameter-consuming script is judged together with the literal arguments the caller passes it: each guard checks
// them, as whole words, against the operands it protects. The same words pass a script that ignores its parameters.
// A branch name such as main is not a command-guardian concern here: that guard has no classifier for a bare ref, and
// none is added for caller arguments.
func TestScriptCallerArgumentsAreJudgedByEachGuard(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	slash := filepath.ToSlash(dir)
	callerArgFixtures(t, dir)
	cd := "cd " + slash + " && "
	const label = "script-caller-argument"
	cases := []struct {
		name, command, guard string
		deny                 bool
	}{
		{"no-verify", "bash ./commit.sh --no-verify", "command-guardian", true},
		{"force", "bash ./commit.sh --force", "command-guardian", true},
		{"plain message", "bash ./commit.sh msg2", "command-guardian", false},
		{"short no-verify", "bash ./commit.sh -n", "command-guardian", true},
		{"short cluster no-verify", "bash ./commit.sh -an", "command-guardian", true},
		{"abbreviated no-verify", "bash ./commit.sh --no-verif", "command-guardian", true},
		{"message flag", "bash ./commit.sh -m", "command-guardian", false},
		{"short flag without n", "bash ./commit.sh -a", "command-guardian", false},
		{"force with lease", "bash ./commit.sh --force-with-lease", "command-guardian", false},
		{"current directory", "bash ./cat.sh .", "command-guardian", false},
		{"parent directory", "bash ./cat.sh ..", "command-guardian", false},
		{"double slash", "bash ./cat.sh //", "command-guardian", true},
		{"home slash", "bash ./cat.sh '~/'", "command-guardian", true},
		{"drive root", "bash ./cat.sh C:/", "command-guardian", true},
		{"branch name", "bash ./commit.sh main", "command-guardian", false},
		{"root", "bash ./cat.sh /", "command-guardian", true},
		{"home", "bash ./cat.sh '~'", "command-guardian", true},
		{"git component", "bash ./cat.sh .git/config", "command-guardian", true},
		{"env component", "bash ./cat.sh sub/.env.local", "command-guardian", true},
		{"ordinary path", "bash ./cat.sh src/a.go", "command-guardian", false},
		{"script ignores parameters", "bash ./none.sh --force", "command-guardian", false},
		{"script ignores parameters root", "bash ./none.sh /", "command-guardian", false},
		{"quoted prose is one word", "bash ./cat.sh 'use / and --force here'", "command-guardian", false},
		{"unquoted use splits prose", "bash ./unq.sh 'use / and here'", "command-guardian", true},
		{"unquoted pattern", "bash ./cat.sh *.txt", "command-guardian", true},
		{"quoted pattern, quoted use", "bash ./cat.sh '*.txt'", "command-guardian", false},
		{"quoted pattern, unquoted use", "bash ./unq.sh '.e*'", "command-guardian", true},
		{"assignment right side", "bash ./assign.sh 'use / and here'", "command-guardian", false},

		{"dotenv", "bash ./cat.sh .env", "secrets-guardian", true},
		{"aws credentials", "bash ./cat.sh '~/.aws/credentials'", "secrets-guardian", true},
		{"ordinary source file", "bash ./cat.sh src/a.go", "secrets-guardian", false},
		{"credential, script ignores parameters", "bash ./none.sh .env", "secrets-guardian", false},
		{"quoted prose mentioning dotenv", "bash ./cat.sh 'moved .env to backup'", "secrets-guardian", false},
		{"unquoted use splits prose", "bash ./unq.sh 'moved .env to backup'", "secrets-guardian", true},
		{"unquoted pattern", "bash ./cat.sh .e*", "secrets-guardian", true},
		{"quoted pattern, unquoted use", "bash ./unq.sh '.e*'", "secrets-guardian", true},
		{"nested dotenv", "bash ./cat.sh sub/.env", "secrets-guardian", true},

		// release-guardian judges deploy command shapes only, so no argument can be a caller-argument finding.
		{"release has no operand classifier", "bash ./cat.sh .env", "release-guardian", false},
		{"release ignores force", "bash ./commit.sh --force", "release-guardian", false},
	}
	for _, surface := range scriptSurfaces {
		for _, tc := range cases {
			t.Run(string(surface)+"/"+tc.guard+"/"+tc.name, func(t *testing.T) {
				code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, dir, cd+tc.command), surface, tc.guard)
				if !tc.deny {
					requireOutcome(t, surface, "pass", "", code, stdout, stderr)
					return
				}
				requireOutcome(t, surface, "deny", label, code, stdout, stderr)
				if tc.guard == "secrets-guardian" && !strings.Contains(stdout+stderr, "cat.sh") && !strings.Contains(stdout+stderr, "unq.sh") {
					t.Fatalf("deny reason does not name the script: %q %q", stdout, stderr)
				}
				for _, leaked := range []string{"backup", "credentials", "no-verify"} {
					if strings.Contains(stdout+stderr, leaked) {
						t.Fatalf("deny reason repeats the caller argument %q: %q %q", leaked, stdout, stderr)
					}
				}
			})
		}
	}
}

// The repository's own flow helper takes free text as one quoted argument per field. Words such as --force or .env
// inside that text are part of one whole argument, so the call passes; the same word as an argument of its own to a
// script that forwards it into a commit is denied.
func TestScriptCallerArgumentsLetFlowHelperFreeTextPass(t *testing.T) {
	pinScriptGuardEnv(t)
	root := filepath.ToSlash(hosttest.Checkout(t))
	folder := filepath.ToSlash(t.TempDir())
	command := "bash hooks/go/internal/shellwords/testdata/flow.sh log --folder " + folder +
		" chain dispatch task-1 unfinished 'why with / and --force and .env words' 'evidence' 'next'"
	for _, guard := range []string{"command-guardian", "secrets-guardian", "release-guardian"} {
		code, stdout, stderr := invokeWithLegalArgs(t, string(hosttest.Stdin("claude-code", hosttest.Call{Tool: "Bash", Input: map[string]any{"command": command}, Cwd: root})), "claude-code", guard)
		requireOutcome(t, "claude-code", "pass", "", code, stdout, stderr)
	}
}

// find-polluter.sh takes a path to watch and a test pattern and tests the path with -e, so ordinary arguments pass.
// Its own documented example watches .git, which is a protected component, so that call is denied. This is the
// deliberate conservative match: the guard cannot tell a test for existence from a deletion.
func TestScriptCallerArgumentsFindPolluter(t *testing.T) {
	pinScriptGuardEnv(t)
	root := filepath.ToSlash(hosttest.Checkout(t))
	run := func(args string) (int, string, string) {
		return invokeWithLegalArgs(t, string(hosttest.Stdin("claude-code", hosttest.Call{Tool: "Bash", Input: map[string]any{"command": "bash skills/diagnose/references/find-polluter.sh " + args}, Cwd: root})), "claude-code", "command-guardian")
	}
	code, stdout, stderr := run("tmp/out.txt 'src/a.test.ts'")
	requireOutcome(t, "claude-code", "pass", "", code, stdout, stderr)
	code, stdout, stderr = run("'.git' 'src/a.test.ts'")
	requireOutcome(t, "claude-code", "deny", "script-caller-argument", code, stdout, stderr)
}

// The record carries the fixed label and the script path, as a content finding does.
func TestScriptCallerArgumentRecordCarriesLabelAndPath(t *testing.T) {
	pinScriptGuardEnv(t)
	logDir := t.TempDir()
	t.Setenv("COMMAND_GUARD_LOG_DIR", logDir)
	dir := t.TempDir()
	callerArgFixtures(t, dir)
	command := "cd " + filepath.ToSlash(dir) + " && bash ./commit.sh --no-verify"
	code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin("claude-code", dir, command), "claude-code", "command-guardian")
	requireOutcome(t, "claude-code", "deny", "script-caller-argument", code, stdout, stderr)
	log := readGuardLog(t, logDir)
	if !strings.Contains(log, "script content redacted (script-caller-argument in ") || !strings.Contains(log, "commit.sh") {
		t.Fatalf("guard.log %q", log)
	}
}

// The grouped pre-tool event reaches the same caller-argument verdicts.
func TestScriptCallerArgumentsGroupedEvent(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	callerArgFixtures(t, dir)
	cd := "cd " + filepath.ToSlash(dir) + " && "
	for _, surface := range scriptSurfaces {
		for _, tc := range []struct {
			name, command string
			deny          bool
		}{
			{"short no-verify", "bash ./commit.sh -n", true}, {"cluster", "bash ./commit.sh -an", true},
			{"abbreviation", "bash ./commit.sh --no-verif", true}, {"message flag", "bash ./commit.sh -m", false},
			{"plain", "bash ./commit.sh msg2", false}, {"dotenv", "bash ./cat.sh .env", true},
		} {
			t.Run(string(surface)+"/"+tc.name, func(t *testing.T) {
				code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, dir, cd+tc.command), surface, "pre-tool")
				if !tc.deny {
					requireSilent(t, surface, code, stdout, stderr)
					return
				}
				if reason := requireDeny(t, surface, code, stdout, stderr); !strings.Contains(reason, "script-caller-argument") {
					t.Fatalf("reason %q", reason)
				}
			})
		}
	}
}

// The caller's redirect on a script call writes the file whatever the script runs, and truncates it when the script
// runs nothing, so command-guardian denies it as the settings edit the typed redirect is, and the other two guards,
// which own no rule for it, pass.
func TestScriptCallRedirectOntoTheSettingsFileIsASettingsEdit(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	slash := filepath.ToSlash(dir)
	settingsFile := ".agentics/" + "hooks.json"
	writeScript(t, dir, "ok.sh", "#!/usr/bin/env bash\necho hello\n")
	writeScript(t, dir, "empty.sh", "")
	writeScript(t, dir, "outer.sh", "bash "+slash+"/ok.sh\n")
	writeScript(t, dir, "nested-write.sh", "bash "+slash+"/empty.sh > "+settingsFile+"\n")
	writeScript(t, dir, "sources.sh", "source "+slash+"/ok.sh > "+settingsFile+"\n")
	cd := "cd " + slash + " && "
	denied := []string{
		cd + "bash ./ok.sh > " + settingsFile,
		cd + "bash ./ok.sh >> " + settingsFile,
		cd + "bash ./empty.sh > " + settingsFile,
		cd + "bash ./empty.sh >> " + settingsFile,
		cd + "bash ./outer.sh > " + settingsFile,
		cd + "bash ./nested-write.sh",
		cd + "bash ./sources.sh",
		cd + "echo hello > " + settingsFile,
	}
	for _, surface := range scriptSurfaces {
		for _, command := range denied {
			t.Run(string(surface)+"/"+command, func(t *testing.T) {
				if got := denyingGuards(t, surface, dir, command); len(got) != 1 || got[0] != cg {
					t.Fatalf("denied by %v, want only %s", got, cg)
				}
				code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin(surface, dir, command), surface, cg)
				requireOutcome(t, surface, "deny", "hook_settings_edit", code, stdout, stderr)
				if !groupedDenies(t, surface, dir, command) {
					t.Fatal("the grouped event passes")
				}
			})
		}
	}
	// The same calls aimed at another file pass every guard, so the rows above deny for the target.
	for _, command := range []string{cd + "bash ./ok.sh > out.txt", cd + "bash ./empty.sh >> out.txt"} {
		t.Run("claude-code/"+command, func(t *testing.T) {
			if got := denyingGuards(t, "claude-code", dir, command); len(got) != 0 {
				t.Fatalf("denied by %v", got)
			}
		})
	}
}

// A path the allowlist covers is skipped for a caller argument as it is for a typed command.
func TestScriptCallerArgumentHonorsTheSecretsAllowlist(t *testing.T) {
	pinScriptGuardEnv(t)
	dir := t.TempDir()
	callerArgFixtures(t, dir)
	cd := "cd " + filepath.ToSlash(dir) + " && "
	code, stdout, stderr := invokeWithLegalArgs(t, scriptStdin("claude-code", dir, cd+"bash ./cat.sh fixtures/.env"), "claude-code", "secrets-guardian")
	requireOutcome(t, "claude-code", "deny", "script-caller-argument", code, stdout, stderr)
	t.Setenv("SECRETS_GUARD_ALLOWLIST", "fixtures/")
	code, stdout, stderr = invokeWithLegalArgs(t, scriptStdin("claude-code", dir, cd+"bash ./cat.sh fixtures/.env"), "claude-code", "secrets-guardian")
	requireOutcome(t, "claude-code", "pass", "", code, stdout, stderr)
}
