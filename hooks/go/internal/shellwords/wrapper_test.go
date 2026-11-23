package shellwords

import (
	"context"
	"strings"
	"testing"
	"time"
)

// readingWant is a wrapper reading in a form a table row can state. An empty child or assign requires a nil slice,
// an empty cmd a nil command string, and an empty refusal no refusal at all.
type readingWant struct {
	child, assign, cmd string
	powerShell         bool
	incomplete         bool
	inspectionOnly     bool
	refusal            string
}

func optionRefused(exe, w string) readingWant {
	return readingWant{refusal: exe + " option " + w + " is outside the literal wrapper contract"}
}

func valueRefused(exe, w string) readingWant {
	return readingWant{refusal: exe + " " + w + " requires a literal value"}
}

func noChild(exe string) readingWant { return readingWant{refusal: exe + " has no child executable"} }

func childNotLiteral(exe string) readingWant {
	return readingWant{refusal: exe + " child executable is not literal"}
}

func wordTexts(words []Word) string {
	texts := make([]string, len(words))
	for i, w := range words {
		texts[i] = w.Text
	}
	return strings.Join(texts, " ")
}

func checkReading(t *testing.T, got wrapperReading, parent Dialect, want readingWant) {
	t.Helper()
	if (want.child == "") != (got.child == nil) || wordTexts(got.child) != want.child {
		t.Errorf("child = %q (nil %t), want %q", wordTexts(got.child), got.child == nil, want.child)
	}
	if (want.assign == "") != (got.assignments == nil) || wordTexts(got.assignments) != want.assign {
		t.Errorf("assignments = %q (nil %t), want %q", wordTexts(got.assignments), got.assignments == nil, want.assign)
	}
	if want.cmd == "" {
		if got.commandString != nil {
			t.Errorf("command string = %q, want none", got.commandString.Text)
		}
	} else if got.commandString == nil || got.commandString.Text != want.cmd {
		t.Errorf("command string = %v, want %q", got.commandString, want.cmd)
	}
	dialect := parent
	if want.cmd != "" {
		dialect = POSIX
		if want.powerShell {
			dialect = PowerShell
		}
	}
	if got.dialect != dialect {
		t.Errorf("dialect = %d, want %d", got.dialect, dialect)
	}
	if got.complete == want.incomplete {
		t.Errorf("complete = %t, want %t", got.complete, !want.incomplete)
	}
	if got.inspectionOnly != want.inspectionOnly {
		t.Errorf("inspectionOnly = %t, want %t", got.inspectionOnly, want.inspectionOnly)
	}
	if got.refusal.Refused() != (want.refusal != "") || got.refusal.String() != want.refusal {
		t.Errorf("refusal = %q, want %q", got.refusal.String(), want.refusal)
	}
}

// TestReadWrapper pins every wrapper's reading: each accepted option form, each refusal with its exact text, "--", a
// non-literal option, a missing and a non-literal value, no child, and a non-literal child.
func TestReadWrapper(t *testing.T) {
	ok := func(child string) readingWant { return readingWant{child: child} }
	shellContract := func(exe string) readingWant {
		return readingWant{refusal: exe + " requires exactly -c or -lc and a literal command string"}
	}
	psContract := func(exe string) readingWant {
		return readingWant{refusal: exe + " requires optional -NoProfile/-NonInteractive, then -Command and one literal token"}
	}
	envName := func(w string) readingWant { return readingWant{refusal: "env " + w + " requires a literal name"} }
	duration := readingWant{refusal: "timeout requires a literal duration before the command"}
	xargsNoChild := readingWant{refusal: "xargs has no literal child executable"}
	inspect := readingWant{inspectionOnly: true}
	incomplete := func(w readingWant) readingWant { w.incomplete = true; return w }

	for _, tc := range []struct {
		source  string
		dialect Dialect
		want    readingWant
	}{
		{"sh -c 'echo hi'", POSIX, readingWant{cmd: "echo hi"}},
		{"bash -lc 'echo hi'", POSIX, readingWant{cmd: "echo hi"}},
		{"bash -c 'echo hi'", PowerShell, readingWant{cmd: "echo hi"}},
		{`bash -c "$X"`, POSIX, readingWant{cmd: "$X"}},
		{"bash -c", POSIX, shellContract("bash")},
		{"sh -x 'echo hi'", POSIX, shellContract("sh")},
		{"bash $F 'echo hi'", POSIX, shellContract("bash")},
		{"bash", PowerShell, shellContract("bash")},

		{"pwsh -Command 'Get-Item x'", POSIX, readingWant{cmd: "Get-Item x", powerShell: true}},
		{"powershell -noprofile -NonInteractive -command x", PowerShell, readingWant{cmd: "x", powerShell: true}},
		{"pwsh -NoProfile -Command", POSIX, psContract("pwsh")},
		{"pwsh -Command a b", POSIX, psContract("pwsh")},
		{"powershell -File x", POSIX, psContract("powershell")},
		{"pwsh $C x", POSIX, psContract("pwsh")},
		{"pwsh", POSIX, psContract("pwsh")},

		{"env printf ok", POSIX, ok("printf ok")},
		{"env A=1 B=2 printf ok", POSIX, readingWant{child: "printf ok", assign: "A=1 B=2"}},
		{"env -i --ignore-environment A=1 printf", PowerShell, readingWant{child: "printf", assign: "A=1"}},
		{"env -u NAME --unset NAME --unset=NAME -uNAME printf", POSIX, ok("printf")},
		{"env -- printf", POSIX, ok("printf")},
		{"env A=1 -- printf", POSIX, readingWant{child: "printf", assign: "A=1"}},
		{"env A=1 --", POSIX, withAssignment("A=1", noChild("env"))},
		{"env A=1", POSIX, noChild("env")},
		{"env", POSIX, noChild("env")},
		{"env -x printf", POSIX, optionRefused("env", "-x")},
		{"env --unset= printf", POSIX, optionRefused("env", "--unset=")},
		{"env -u", POSIX, envName("-u")},
		{"env --unset", POSIX, envName("--unset")},
		{"env -u $X printf", POSIX, envName("-u")},
		{"env -u '' printf", POSIX, envName("-u")},
		{"env A=1 $CMD", POSIX, withAssignment("A=1", childNotLiteral("env"))},

		{"command -v git", POSIX, inspect},
		{"command -V git", POSIX, inspect},
		{"command git status", POSIX, ok("git status")},
		{"command -- git", POSIX, ok("git")},
		{"command -p git", POSIX, optionRefused("command", "-p")},
		{"command -- -v", POSIX, optionRefused("command", "-v")},
		{"command", POSIX, noChild("command")},
		{"command --", POSIX, noChild("command")},
		{"command $X", POSIX, childNotLiteral("command")},

		{"nohup git push", POSIX, ok("git push")},
		{"nohup -- git", POSIX, ok("git")},
		{"exec -- git", POSIX, ok("git")},
		{"nohup -x git", POSIX, optionRefused("nohup", "-x")},
		{"exec -- -a git", POSIX, optionRefused("exec", "-a")},
		{"exec", POSIX, noChild("exec")},
		{"nohup $X", POSIX, childNotLiteral("nohup")},

		{"time git", POSIX, ok("git")},
		{"time -p git", POSIX, ok("git")},
		{"time -p -- git", POSIX, ok("git")},
		{"time -- git", POSIX, ok("git")},
		{"time -v git", POSIX, optionRefused("time", "-v")},
		{"time -- -p git", POSIX, optionRefused("time", "-p")},
		{"time", POSIX, noChild("time")},
		{"time $X", POSIX, childNotLiteral("time")},

		{"xargs rm", POSIX, incomplete(ok("rm"))},
		{"xargs -0 -r -n 1 -P 4 -I X rm X", POSIX, incomplete(ok("rm X"))},
		{"xargs -- rm", POSIX, incomplete(ok("rm"))},
		{"xargs --", POSIX, incomplete(noChild("xargs"))},
		{"xargs $X", POSIX, incomplete(childNotLiteral("xargs"))},
		{"xargs -n", POSIX, valueRefused("xargs", "-n")},
		{"xargs -P $N rm", POSIX, valueRefused("xargs", "-P")},
		{"xargs -I '' rm", POSIX, valueRefused("xargs", "-I")},
		{"xargs -x rm", POSIX, optionRefused("xargs", "-x")},
		{"xargs", POSIX, xargsNoChild},

		{"timeout 5 git", POSIX, ok("git")},
		{"timeout --foreground --preserve-status -v --verbose -s KILL --signal TERM -k 1 --kill-after 2 --signal=TERM --kill-after=3 1.5m git", POSIX, ok("git")},
		{"timeout 10s git", POSIX, ok("git")},
		{"timeout -s", POSIX, valueRefused("timeout", "-s")},
		{"timeout -k $K 5 git", POSIX, valueRefused("timeout", "-k")},
		{"timeout -x 5 git", POSIX, optionRefused("timeout", "-x")},
		{"timeout git push", POSIX, duration},
		{"timeout 5", POSIX, duration},
		{"timeout $D git", POSIX, duration},
		{"timeout", POSIX, duration},
		{"timeout 5 -x", POSIX, readingWant{refusal: "timeout option -x after the duration is outside the literal wrapper contract"}},
		{"timeout 5 $X", POSIX, childNotLiteral("timeout")},

		{"nice git", POSIX, ok("git")},
		{"nice -n 5 --adjustment=3 -10 git", POSIX, ok("git")},
		{"nice -n", POSIX, valueRefused("nice", "-n")},
		{"nice -n $N git", POSIX, valueRefused("nice", "-n")},
		{"nice --adjustment= git", POSIX, optionRefused("nice", "--adjustment=")},
		{"nice -1a git", POSIX, optionRefused("nice", "-1a")},
		{"nice", POSIX, noChild("nice")},
		{"nice $X", POSIX, childNotLiteral("nice")},

		{"flock /tmp/l git", POSIX, ok("git")},
		{"flock -s -x -e -n --nb -o -F --verbose -w 5 --timeout 3 -E 2 /tmp/l git", POSIX, ok("git")},
		{"flock -w", POSIX, valueRefused("flock", "-w")},
		{"flock -E $C /tmp/l git", POSIX, valueRefused("flock", "-E")},
		{"flock -z /tmp/l git", POSIX, optionRefused("flock", "-z")},
		{"flock /tmp/l", POSIX, noChild("flock")},
		{"flock $L git", POSIX, noChild("flock")},
		{"flock", POSIX, noChild("flock")},
		{"flock /tmp/l -c git", POSIX, readingWant{refusal: "flock option -c after the lock file is outside the literal wrapper contract"}},
		{"flock /tmp/l $X", POSIX, childNotLiteral("flock")},

		{"sudo git", POSIX, ok("git")},
		{"sudo -E -n -u root -g wheel git", POSIX, ok("git")},
		{"sudo -- git", POSIX, ok("git")},
		{"sudo -E -- -x", POSIX, ok("-x")},
		{"sudo --", POSIX, noChild("sudo")},
		{"sudo -u", POSIX, valueRefused("sudo", "-u")},
		{"sudo -g $G git", POSIX, valueRefused("sudo", "-g")},
		{"sudo -u '' git", POSIX, valueRefused("sudo", "-u")},
		{"sudo -S git", POSIX, optionRefused("sudo", "-S")},
		{"sudo", POSIX, noChild("sudo")},
		{"sudo $X", POSIX, childNotLiteral("sudo")},

		{"doas git", POSIX, ok("git")},
		{"doas -n -L -u root -C /etc/doas.conf git", POSIX, ok("git")},
		{"doas -- git", POSIX, optionRefused("doas", "--")},
		{"doas -C", POSIX, valueRefused("doas", "-C")},
		{"doas -u $U git", POSIX, valueRefused("doas", "-u")},
		{"doas -s git", POSIX, optionRefused("doas", "-s")},
		{"doas", POSIX, noChild("doas")},
		{"doas $X", POSIX, childNotLiteral("doas")},

		{"ionice git", POSIX, ok("git")},
		{"ionice -t -c 2 -n 7 -p 1 --class= --classdata=3 git", POSIX, ok("git")},
		{"ionice -- git", POSIX, optionRefused("ionice", "--")},
		{"ionice -c", POSIX, valueRefused("ionice", "-c")},
		{"ionice -n $N git", POSIX, valueRefused("ionice", "-n")},
		{"ionice -x git", POSIX, optionRefused("ionice", "-x")},
		{"ionice", POSIX, noChild("ionice")},
		{"ionice $X", POSIX, childNotLiteral("ionice")},

		{"stdbuf git", POSIX, ok("git")},
		{"stdbuf -i 0 -o L -e 0 git", POSIX, ok("git")},
		{"stdbuf -iL -oL -e0 --input= --output=L --error=0 git", POSIX, ok("git")},
		{"stdbuf -- git", POSIX, optionRefused("stdbuf", "--")},
		{"stdbuf -o", POSIX, valueRefused("stdbuf", "-o")},
		{"stdbuf -e $E git", POSIX, valueRefused("stdbuf", "-e")},
		{"stdbuf -x git", POSIX, optionRefused("stdbuf", "-x")},
		{"stdbuf", POSIX, noChild("stdbuf")},
		{"stdbuf $X", POSIX, childNotLiteral("stdbuf")},

		{"setsid git", POSIX, ok("git")},
		{"setsid -c --ctty -f --fork -w --wait git", POSIX, ok("git")},
		{"setsid -- git", POSIX, optionRefused("setsid", "--")},
		{"setsid -x git", POSIX, optionRefused("setsid", "-x")},
		{"setsid", POSIX, noChild("setsid")},
		{"setsid $X", POSIX, childNotLiteral("setsid")},
	} {
		t.Run(tc.source, func(t *testing.T) {
			words := lexSegment(tc.source, tc.dialect).words
			exe := strings.ToLower(ExecutableName(words[0].Text))
			got := readWrapper(context.Background(), exe, words, tc.dialect, 1)
			checkReading(t, got, tc.dialect, tc.want)
			if exe != "env" && got.effects != nil {
				t.Errorf("%s reported environment effects %#v", exe, got.effects)
			}
		})
	}

	// An option the wrapper accepts when spelled literally refuses as unknown when it is not literal. A "--" that is
	// not literal still ends the options of env, sudo, and xargs, which match it on its text, and is an unknown
	// option to the wrappers that test it for a literal.
	for _, tc := range []struct {
		exe, option string
		want        readingWant
	}{
		{"env", "-i", optionRefused("env", "-i")},
		{"env", "--unset=N", optionRefused("env", "--unset=N")},
		{"env", "-uN", optionRefused("env", "-uN")},
		{"sudo", "-E", optionRefused("sudo", "-E")},
		{"doas", "-n", optionRefused("doas", "-n")},
		{"ionice", "--class=2", optionRefused("ionice", "--class=2")},
		{"stdbuf", "-oL", optionRefused("stdbuf", "-oL")},
		{"setsid", "-w", optionRefused("setsid", "-w")},
		{"xargs", "-0", optionRefused("xargs", "-0")},
		{"timeout", "--signal=TERM", optionRefused("timeout", "--signal=TERM")},
		{"nice", "-10", optionRefused("nice", "-10")},
		{"nice", "--adjustment=3", optionRefused("nice", "--adjustment=3")},
		{"flock", "-s", optionRefused("flock", "-s")},
		{"time", "-p", optionRefused("time", "-p")},
		{"command", "--", optionRefused("command", "--")},
		{"nohup", "--", optionRefused("nohup", "--")},
		{"exec", "--", optionRefused("exec", "--")},
		{"time", "--", optionRefused("time", "--")},
		{"env", "--", ok("git")},
		{"sudo", "--", ok("git")},
		{"xargs", "--", incomplete(ok("git"))},
	} {
		t.Run(tc.exe+" non-literal "+tc.option, func(t *testing.T) {
			words := lexSegment(tc.exe+" "+tc.option+" git", POSIX).words
			if literal := readWrapper(context.Background(), tc.exe, words, POSIX, 1); literal.refusal.Refused() && tc.exe != "timeout" && tc.exe != "flock" {
				t.Fatalf("literal %s %s refused: %s", tc.exe, tc.option, literal.refusal)
			}
			words[1].Literal = false
			checkReading(t, readWrapper(context.Background(), tc.exe, words, POSIX, 1), POSIX, tc.want)
		})
	}

	t.Run("head spellings", func(t *testing.T) {
		for _, source := range []string{"/usr/bin/sudo git", "SUDO git", "sudo.exe git"} {
			words := lexSegment(source, POSIX).words
			checkReading(t, readWrapper(context.Background(), "sudo", words, POSIX, 1), POSIX, ok("git"))
		}
	})

	t.Run("unknown wrapper panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("readWrapper read git as a wrapper")
			}
		}()
		readWrapper(context.Background(), "git", lexSegment("git status", POSIX).words, POSIX, 1)
	})
}

// TestReadWrapperEnvEffectsAndRefusalFromOneWalk checks that one env walk yields the strict reading, refusal
// included, and the lenient effects that go on past it, polling the context as the effects reading always has.
func TestReadWrapperEnvEffectsAndRefusalFromOneWalk(t *testing.T) {
	type effect struct {
		kind  EnvironmentEffectKind
		text  string
		order int
	}
	for _, tc := range []struct {
		source  string
		want    readingWant
		effects []effect
		polls   int
	}{
		{"env -u $X cmd", readingWant{refusal: "env -u requires a literal name"}, []effect{{EnvironmentUnset, "$X", 1}}, 2},
		{"env -u$X cmd", optionRefused("env", "-u$X"), []effect{{EnvironmentUnset, "$X", 1}}, 2},
		{"env --unset= cmd", optionRefused("env", "--unset="), []effect{{EnvironmentUnset, "", 1}}, 2},
		{"env A=1 --", withAssignment("A=1", noChild("env")), []effect{{EnvironmentAssignment, "A=1", 1}}, 2},
		{"env -u", readingWant{refusal: "env -u requires a literal name"}, []effect{{EnvironmentUnset, "", 1}}, 2},
		{"env -x A=1 cmd", optionRefused("env", "-x"), nil, 1},
		{"env --unset= A=1 -u N B=2", optionRefused("env", "--unset="), []effect{{EnvironmentUnset, "", 1}, {EnvironmentAssignment, "A=1", 2}, {EnvironmentUnset, "N", 3}, {EnvironmentAssignment, "B=2", 5}}, 5},
		{"env -i -u N --unset=M -uK A=1 cmd", readingWant{child: "cmd", assign: "A=1"}, []effect{{EnvironmentResetAll, "", 1}, {EnvironmentUnset, "N", 2}, {EnvironmentUnset, "M", 4}, {EnvironmentUnset, "K", 5}, {EnvironmentAssignment, "A=1", 6}}, 6},
		{"env", noChild("env"), nil, 1},
	} {
		t.Run(tc.source, func(t *testing.T) {
			words := lexSegment(tc.source, POSIX).words
			probe := &callCancelContext{Context: context.Background()}
			got := readWrapper(probe, "env", words, POSIX, 7)
			checkReading(t, got, POSIX, tc.want)
			if len(got.effects) != len(tc.effects) || (tc.effects == nil) != (got.effects == nil) {
				t.Fatalf("effects = %#v, want %v", got.effects, tc.effects)
			}
			for i, e := range got.effects {
				w := tc.effects[i]
				if e.Kind != w.kind || e.Word.Text != w.text || e.Order != w.order || e.Occurrence != 7 || e.Dialect != POSIX {
					t.Errorf("effect %d = %#v, want %v", i, e, w)
				}
			}
			// The count is the one the effects reading has always made on these words: one poll per effect word,
			// one for the word that stops the effects, or one more when the words run out.
			if probe.calls != tc.polls {
				t.Errorf("polls = %d, want %d", probe.calls, tc.polls)
			}
		})
	}

	t.Run("canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got := readWrapper(ctx, "env", lexSegment("env A=1 -u $X cmd", POSIX).words, POSIX, 1)
		if got.effects != nil {
			t.Fatalf("canceled walk kept effects %#v", got.effects)
		}
		checkReading(t, got, POSIX, readingWant{refusal: "env -u requires a literal name"})
	})

	t.Run("canceled mid-walk", func(t *testing.T) {
		probe := &callCancelContext{Context: context.Background(), at: 2}
		got := readWrapper(probe, "env", lexSegment("env A=1 B=2 C=3 cmd", POSIX).words, POSIX, 1)
		if got.effects != nil || probe.calls != 2 {
			t.Fatalf("effects = %#v after %d polls, want nil after 2", got.effects, probe.calls)
		}
		checkReading(t, got, POSIX, readingWant{child: "cmd", assign: "A=1 B=2 C=3"})
	})
}

func withAssignment(assign string, w readingWant) readingWant {
	w.assign = assign
	return w
}

// TestReadWrapperIsLinear bounds the walk over hostile input: a long run of words must be read in one pass.
func TestReadWrapperIsLinear(t *testing.T) {
	const count = 10000
	for _, tc := range []struct {
		exe, source string
	}{
		{"env", "env " + strings.Repeat("ASSIGNED=v ", count) + "git"},
		{"sudo", "sudo " + strings.Repeat("-E ", count) + "git"},
	} {
		t.Run(tc.exe, func(t *testing.T) {
			words := lexSegment(tc.source, POSIX).words
			if len(words) != count+2 {
				t.Fatalf("lexed %d words", len(words))
			}
			start := time.Now()
			got := readWrapper(context.Background(), tc.exe, words, POSIX, 1)
			elapsed := time.Since(start)
			if got.refusal.Refused() || wordTexts(got.child) != "git" {
				t.Fatalf("reading = %#v", got)
			}
			if tc.exe == "env" && (len(got.effects) != count || len(got.assignments) != count) {
				t.Fatalf("effects %d assignments %d", len(got.effects), len(got.assignments))
			}
			if elapsed > time.Second {
				t.Fatalf("read %d bytes in %s", len(tc.source), elapsed)
			}
		})
	}
}
