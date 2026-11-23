package commandguard

import (
	"bytes"
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/hookspec/hookspectest"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func TestPowerShellFileDataPrecision(t *testing.T) {
	for _, body := range []string{"gh stack submit", "don't; gh stack submit; 'literal"} {
		c := tableCase{tool: "PowerShell", command: "@'\n" + body + "\n'@ | Set-Content -LiteralPath draft.txt", want: "pass"}
		assertCaseSemantics(t, c, recordCase(t, c))
	}
	safe := "@'\ngh stack submit\n'@ | Set-Content -LiteralPath draft.txt"
	for _, command := range []string{
		strings.Replace(safe, "Set-Content -LiteralPath draft.txt", "Invoke-Expression", 1),
		safe + "; gh pr ready 1",
		strings.Replace(safe, "draft.txt", "Env:EXAMPLE", 1),
		strings.Replace(safe, "draft.txt", "NUL.txt", 1),
		strings.Replace(safe, "draft.txt", "draft.txt -Unknown", 1),
		"$body = " + safe,
		"Invoke-Expression (" + safe + ")",
		"@'\ngh stack submit\n'@; Write-Output 'collision\n'@ | Set-Content -LiteralPath draft.txt",
	} {
		t.Run(command, func(t *testing.T) {
			c := tableCase{tool: "PowerShell", command: command, want: "deny"}
			got := recordCase(t, c)
			assertCaseSemantics(t, c, got)
			if !strings.Contains(got.Outcome.Payload(), ": unsafe_publication - ") {
				t.Fatalf("lost publication policy: %s", got.Outcome.Payload())
			}
		})
	}
	for _, input := range []map[string]any{
		{"command": safe, "shell": "pwsh"},
		{"command": safe, "value": map[string]any{"command": "different"}},
	} {
		c := tableCase{tool: "PowerShell", input: mustEncodeCompact(t, input), want: "deny"}
		got := recordCase(t, c)
		assertCaseSemantics(t, c, got)
		if !strings.Contains(got.Outcome.Payload(), ": unsafe_publication - ") {
			t.Fatalf("unknown context lost original publication: %s", got.Outcome.Payload())
		}
	}
}

func TestPowerShellFileDataCancelled(t *testing.T) {
	hosttest.ScrubGuardEnv(t)
	for _, stage := range []string{"body", "suffix"} {
		t.Run(stage, func(t *testing.T) {
			command := "@'\ngh stack submit\n" + strings.Repeat("x", 6*64*1024) + "\n'@ | Set-Content -LiteralPath draft.txt" + strings.Repeat(" ", 6*64*1024)
			baseline := &hookspectest.CancelContext{Context: context.Background()}
			shellwords.ParseContext(baseline, command, shellwords.PowerShell)
			if stage == "suffix" {
				shellwords.PowerShellLiteralData(baseline, command)
			}
			ctx := &hookspectest.CancelContext{Context: context.Background(), At: baseline.Calls + 4}
			event := buildEvent(t, "PowerShell", commandInput(t, command), t.TempDir())
			var diag bytes.Buffer
			out := Run(hookspec.Request{Context: ctx, Event: event, Diag: &diag})
			// The parse-limit threat the cancellation adds is fixed (no settings
			// key governs it), and a fixed threat names the reason alone whenever
			// one is present, while the publication threat is governed by a
			// key. The reason therefore omits it, and the diagnostic table,
			// which lists every threat, is where the original evidence shows.
			if ctx.Calls < ctx.At || out.Kind() != "deny" || !strings.Contains(diag.String(), "unsafe_publication") {
				t.Fatalf("cancellation lost original executable evidence: checks=%d/%d outcome=%s %s\n%s", ctx.Calls, ctx.At, out.Kind(), out.Payload(), diag.String())
			}
		})
	}
}

func TestSafeHeredocVariableOutput(t *testing.T) {
	assertGhOutcome(t, "Bash", "cat > \"$TMPDIR/draft.md\" <<'EOF'\ngh stack submit\nEOF\n", "pass", "")
}

func TestSafeDataConsumerBarrier(t *testing.T) {
	assertGhOutcome(t, "Bash", "cat > p.sh <<'EOF'\ngh stack submit\nEOF\nrg --pre sh x p.sh", "deny", "unsafe_publication")
}

func TestSafeDataShellContext(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		context    map[string]any
	}{
		{"native bash", "pass", map[string]any{"value": map[string]any{"shell": "bash"}}},
		{"native pwsh", "deny", map[string]any{"value": map[string]any{"shell": "pwsh"}}},
		{"native cmd", "deny", map[string]any{"value": map[string]any{"shell": "cmd.exe"}}},
		{"null shell", "deny", map[string]any{"value": map[string]any{"shell": nil}}},
		{"competing shells", "deny", map[string]any{"shell": "bash", "value": map[string]any{"shell": "bash"}}},
		{"malformed value", "deny", map[string]any{"value": nil}},
		{"second value", "deny", map[string]any{"value": map[string]any{"value": map[string]any{"shell": "bash"}}}},
	} {
		for _, prefix := range []string{"cat > draft.md", "git commit -F -"} {
			t.Run(tc.name+"/"+prefix, func(t *testing.T) {
				input := map[string]any{"command": prefix + " <<'EOF'\ngh stack submit\nEOF\n"}
				for key, value := range tc.context {
					input[key] = value
				}
				out, _, _ := runGo(t, tableCase{input: mustEncodeCompact(t, input)})
				if out.Kind() != tc.want {
					t.Fatalf("kind=%s want=%s payload=%s", out.Kind(), tc.want, out.Payload())
				}
				if tc.want == "deny" && !strings.Contains(out.Payload(), ": unsafe_publication - ") {
					t.Fatalf("wrong denial: %s", out.Payload())
				}
			})
		}
	}
}

func TestSafeGitPagerBarrier(t *testing.T) {
	assertGhOutcomeEnv(t, map[string]string{"GIT_PAGER": "unproven-pager"}, "Bash", "cat > p.sh <<'EOF'\ngh stack submit\nEOF\ngit grep gh -- p.sh", "deny", "unsafe_publication")
}

func TestSafeGitPagerContext(t *testing.T) {
	for _, tc := range []struct {
		name, host, command, want string
		unset                     bool
		overrides                 map[string]any
	}{
		{name: "unset pager", unset: true, want: "deny"},
		{name: "host pager", host: "unproven-pager", want: "deny"},
		{name: "inherited empty", want: "pass"},
		{name: "tool empty", host: "unproven-pager", overrides: map[string]any{"env": map[string]any{"GIT_PAGER": ""}}, want: "pass"},
		{name: "wrapped tool empty", host: "unproven-pager", overrides: map[string]any{"value": map[string]any{"env": map[string]any{"GIT_PAGER": ""}}}, want: "pass"},
		{name: "wrapped tool pager overrides empty", overrides: map[string]any{"value": map[string]any{"env": map[string]any{"GIT_PAGER": "unproven-pager"}}}, want: "deny"},
		{name: "wrapped environment pager overrides empty", overrides: map[string]any{"value": map[string]any{"environment": map[string]any{"GIT_PAGER": "unproven-pager"}}}, want: "deny"},
		{name: "wrapped null env", overrides: map[string]any{"value": map[string]any{"env": nil}}, want: "deny"},
		{name: "competing argument layers", overrides: map[string]any{"env": map[string]any{"GIT_PAGER": ""}, "value": map[string]any{"env": map[string]any{"GIT_PAGER": "unproven-pager"}}}, want: "deny"},
		{name: "malformed value envelope", overrides: map[string]any{"value": nil}, want: "deny"},
		{name: "ambiguous second value envelope", overrides: map[string]any{"value": map[string]any{"value": map[string]any{"env": map[string]any{"GIT_PAGER": ""}}}}, want: "deny"},
		{name: "tool pager overrides empty", overrides: map[string]any{"env": map[string]any{"GIT_PAGER": "unproven-pager"}}, want: "deny"},
		{name: "environment pager overrides empty", overrides: map[string]any{"environment": map[string]any{"GIT_PAGER": "unproven-pager"}}, want: "deny"},
		{name: "null override", overrides: map[string]any{"env": nil}, want: "deny"},
		{name: "null pager", overrides: map[string]any{"env": map[string]any{"GIT_PAGER": nil}}, want: "deny"},
		{name: "nonstring pager", overrides: map[string]any{"env": map[string]any{"GIT_PAGER": 0}}, want: "deny"},
		{name: "malformed container", overrides: map[string]any{"env": "GIT_PAGER="}, want: "deny"},
		{name: "competing containers", overrides: map[string]any{"env": map[string]any{"GIT_PAGER": ""}, "environment": map[string]any{}}, want: "deny"},
		{name: "ambiguous case", overrides: map[string]any{"env": map[string]any{"git_pager": ""}}, want: "deny"},
		{name: "override cannot establish inheritance", overrides: map[string]any{"env": map[string]any{"TERM": "dumb"}}, want: "deny"},
		{name: "explicit no-pager", host: "unproven-pager", command: "git --no-pager grep gh -- p.sh", want: "pass"},
		{name: "explicit short no-pager", unset: true, command: "git -P grep gh -- p.sh", want: "pass"},
		{name: "explicit flag independent of malformed env", overrides: map[string]any{"env": nil}, command: "git --no-pager grep gh -- p.sh", want: "pass"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var env map[string]string
			if !tc.unset {
				env = map[string]string{"GIT_PAGER": tc.host}
			}
			command := tc.command
			if command == "" {
				command = "git grep gh -- p.sh"
			}
			input := map[string]any{"command": "cat > p.sh <<'EOF'\ngh stack submit\nEOF\n" + command}
			for key, value := range tc.overrides {
				input[key] = value
			}
			out, _, _ := runGo(t, tableCase{input: mustEncodeCompact(t, input), env: env})
			if out.Kind() != tc.want {
				t.Fatalf("kind=%s want=%s payload=%s", out.Kind(), tc.want, out.Payload())
			}
			if tc.want == "deny" && !strings.Contains(out.Payload(), ": unsafe_publication - ") {
				t.Fatalf("wrong denial: %s", out.Payload())
			}
		})
	}
}

func TestSafeDataTildeBarrier(t *testing.T) {
	assertGhOutcome(t, "Bash", "cat > p.sh <<'EOF'\ngh stack submit\nEOF\nrg --no-config ~ x p.sh", "deny", "unsafe_publication")
}

func TestSafeDataLiteralTilde(t *testing.T) {
	for _, word := range []string{`'~'`, `"~"`, `\~`, `prefix~`} {
		t.Run(word, func(t *testing.T) {
			assertGhOutcome(t, "Bash", "cat > p.sh <<'EOF'\ngh stack submit\nEOF\nrg --no-config "+word+" x p.sh", "pass", "")
		})
	}
}

func TestSafeDataExpansionBarrier(t *testing.T) {
	env := map[string]string{"GIT_PAGER": ""}
	for _, search := range []string{
		"rg --no-config x *",
		"rg --no-config {--pre=sh,--no-heading} x p.sh",
		"rg --no-config x \"prefix\"*",
		`rg --no-config x 'prefix'*`,
		`rg --no-config x \$'a\'{one,two}`,
		"rg --no-config x -- *",
		"rg --no-config -e * p.sh",
		"printf '%s' *",
		"gh issue create --title example --body example *",
		"cat *",
		"tee {one,two}",
		"mkdir -p drafts*",
		"cd drafts?",
		"grep -e [ab] p.sh",
		"git grep -e {one,two} -- p.sh",
		"cat > draft*",
	} {
		t.Run(search, func(t *testing.T) {
			assertGhOutcomeEnv(t, env, "Bash", "cat > p.sh <<'EOF'\ngh stack submit\nEOF\n"+search, "deny", "unsafe_publication")
		})
	}
}

func TestSafeDataLiteralExpansionCharacters(t *testing.T) {
	for _, search := range []string{
		`rg --no-config x '*'`,
		`rg --no-config x "*"`,
		`rg --no-config x \*`,
		`rg --no-config x prefix"*"`,
		`rg --no-config x $'escaped\'*'`,
		`rg --no-config x '{--pre=sh,p.sh}'`,
		`rg --no-config x \{one,two\}`,
		`rg --no-config -e '*' -- p.sh`,
		`printf '%s' '*' '{one,two}'`,
		`cat "$TMPDIR/draft.md"`,
		`cat ${TMPDIR}/draft.md`,
		`tee "${TMPDIR}/draft.md"`,
		`mkdir -p "${TMPDIR}/drafts" && cd "$TMPDIR/drafts"`,
		`cat > "${TMPDIR}/draft.md"`,
		`cat > ${TMPDIR}/draft.md`,
	} {
		t.Run(search, func(t *testing.T) {
			assertGhOutcome(t, "Bash", "cat > p.sh <<'EOF'\ngh stack submit\nEOF\n"+search, "pass", "")
		})
	}
}

func TestSafeSearchOptionArity(t *testing.T) {
	env := map[string]string{"GIT_PAGER": ""}
	for _, tc := range []struct{ search, want string }{
		{"rg -r --no-config x p.sh", "deny"},
		{"rg -E --no-config x p.sh", "deny"},
		{"rg -r --no-config --no-config x p.sh", "pass"},
		{"rg -r replacement --no-config x p.sh", "pass"},
		{"rg -E utf-8 --no-config x p.sh", "pass"},
		{"rg -E utf-8 x p.sh", "deny"},
		{"rg --no-config -r --no-config x p.sh", "pass"},
		{"rg -r replacement -- --no-config p.sh", "deny"},
		{"grep -E x p.sh", "pass"},
		{"grep -r x p.sh", "pass"},
		{"git grep -E x -- p.sh", "pass"},
		{"grep --no-config x p.sh", "deny"},
		{"git grep --no-config x p.sh", "deny"},
		{"rg -g --no-config x p.sh", "deny"},
		{"rg --no-config=x x p.sh", "deny"},
	} {
		t.Run(tc.search, func(t *testing.T) {
			category := ""
			if tc.want == "deny" {
				category = "unsafe_publication"
			}
			assertGhOutcomeEnv(t, env, "Bash", "cat > p.sh <<'EOF'\ngh stack submit\nEOF\n"+tc.search, tc.want, category)
		})
	}
}

func TestSafeCommandData(t *testing.T) {
	env := map[string]string{"GIT_PAGER": ""}
	for _, command := range []string{
		"cat <<'EOF' > draft.md\ngh stack submit\nEOF\n",
		"cat > draft.md <<'EOF'\ngh stack submit\nEOF\n",
		"mkdir -p drafts && cd drafts && cat > first.md <<'EOF'\ngh stack submit\nEOF\ncat > second.md <<'TWO'\ngh pr ready 1\nTWO\n",
		"tee draft.md <<'EOF'\ngh stack submit\nEOF\n",
		"cat <<'ONE' <<'TWO'\ngh pr create\nONE\ngh stack submit\nTWO\n",
		"printf '%s' 'gh stack submit'",
		"printf '%s' 'gh stack submit' > draft.md",
		"printf '%s' 'DROP TABLE examples;' > draft.md",
		"gh issue create --title example --body 'gh stack submit'",
		"gh issue create --title example --body-file 'gh stack submit'",
		"gh issue create --title example --body-file - <<'EOF'\ngh stack submit\nEOF\n",
		`git grep -n "gh pr create" -- skills/ops`,
		`rg --no-config -n "gh pr create" skills/ops`,
	} {
		t.Run(command, func(t *testing.T) { assertGhOutcomeEnv(t, env, "Bash", command, "pass", "") })
	}
}

func TestSafeCommandDataExecutionTwins(t *testing.T) {
	for _, command := range []string{
		"cat <<'EOF' | bash\ngh stack submit\nEOF\n",
		"cat <<EOF > draft.md\n$(gh stack submit)\nEOF\n",
		"cat > draft.md <<'EOF'\nplain text\nEOF\ngh stack submit",
		"cat > p.sh <<'EOF'\ngh stack submit\nEOF\nrg --pre sh x p.sh",
		"cat > p.sh <<'EOF'\ngh stack submit\nEOF\nrg --pre=sh x p.sh",
		"cat > p.sh <<'EOF'\ngh stack submit\nEOF\nrg --hostname-bin sh x p.sh",
		"cat > p.sh <<'EOF'\ngh stack submit\nEOF\ngit grep --open-files-in-pager=sh x p.sh",
		"cat > p.sh <<'EOF'\ngh stack submit\nEOF\ngit grep --textconv x p.sh",
		"cat > p.sh <<'EOF'\ngh stack submit\nEOF\nrg x p.sh",
		"cat > p.sh <<'EOF'\ngh stack submit\nEOF\nrg -e --no-config x p.sh",
		"cat > p.sh <<'EOF'\ngh stack submit\nEOF\nrg -- --no-config p.sh",
		"cat > p.sh <<'EOF'\ngh stack submit\nEOF\nrg --no-config --pre=sh x p.sh",
		"cat > p.sh <<'EOF'\ngh stack submit\nEOF\nrg --no-config --pre-glob '*.sh' x p.sh",
		"cat > p.sh <<'EOF'\ngh stack submit\nEOF\ngit grep -Osh x p.sh",
		"cat > p.sh <<'EOF'\ngh stack submit\nEOF\ngit -c core.pager=sh grep x p.sh",
		"cat >&3 <<'EOF'\ngh stack submit\nEOF\n",
		"cat 3> pipe <<'EOF'\ngh stack submit\nEOF\n",
		"cat > /dev/fd/3 <<'EOF'\ngh stack submit\nEOF\n",
		"tee /dev/fd/3 <<'EOF'\ngh stack submit\nEOF\n",
		"./cat > draft.md <<'EOF'\ngh stack submit\nEOF\n",
		"printf '%s' 'gh stack submit' >&3",
		"printf '%10n' var 'gh stack submit' > draft.md",
		"printf -v var '%s' 'gh stack submit' > draft.md",
		"cat > >(sh) <<'EOF'\ngh stack submit\nEOF\n",
		"gh issue create --editor --body-file - <<'EOF'\ngh stack submit\nEOF\n",
		"gh issue create --unknown --body-file - <<'EOF'\ngh stack submit\nEOF\n",
	} {
		t.Run(command, func(t *testing.T) { assertGhOutcome(t, "Bash", command, "deny", "unsafe_publication") })
	}
}

func TestSafeDataRetainsGenericExecutableEvidence(t *testing.T) {
	for _, command := range []string{
		"cat > p.sh <<'EOF'\nDROP TABLE examples;\nEOF\nrg --pre sh x p.sh",
		"cat >&3 <<'EOF'\nDROP TABLE examples;\nEOF\n",
		"cat <<'EOF'\nDROP TABLE examples;\n",
		"printf '%s' 'DROP TABLE examples;' | unknown-reader",
	} {
		t.Run(command, func(t *testing.T) { assertGhOutcome(t, "Bash", command, "deny", "database_destruction") })
	}
}

func TestSafeDataDialectAndSourceFallback(t *testing.T) {
	command := "cat > draft.md <<'EOF'\ngh stack submit\nEOF\n"
	for _, shell := range []string{"pwsh", "cmd.exe", "fish", ""} {
		input := mustEncodeCompact(t, map[string]string{"command": command, "shell": shell})
		out, _, _ := runGo(t, tableCase{input: input})
		if out.Kind() != "deny" {
			t.Fatalf("shell=%q outcome=%v", shell, out)
		}
	}
	for _, command := range []string{
		`gh pr create --body 'x --draft'`,
		`gh pr create --body-file 'gh pr create --draft'`,
		`printf '%s' 'gh stack submit' > draft.md; gh stack submit`,
	} {
		assertGhOutcome(t, "Bash", command, "deny", "unsafe_publication")
	}
	input := mustEncodeCompact(t, map[string]string{"command": "printf '%s' 'safe'", "description": "gh stack submit"})
	out, _, _ := runGo(t, tableCase{input: input})
	if out.Kind() != "pass" {
		t.Fatalf("unrelated metadata scanned: %v", out)
	}
	input = mustEncodeCompact(t, map[string]string{"description": "gh stack submit"})
	out, _, _ = runGo(t, tableCase{input: input})
	if out.Kind() != "deny" || !strings.Contains(out.Payload(), "unsafe_publication") {
		t.Fatalf("no-command fallback lost: %v", out)
	}
}

// firstInvocation parses command and returns its only invocation.
func firstInvocation(t *testing.T, command string, dialect shellwords.Dialect) shellwords.Invocation {
	t.Helper()
	invocations := shellwords.Parse(command, dialect).Invocations
	if len(invocations) != 1 {
		t.Fatalf("Parse(%q) gave %d invocations, want 1", command, len(invocations))
	}
	return invocations[0]
}

// TestSafeDataReadingPins fixes how the data readers treat the words their
// grammar leaves open: a lone dash, a second `--`, a flag or short option
// joined with `=`, and a non-literal value or operand. Each row is the answer
// the readers give for the line, so a reading that changed it would show here.
func TestSafeDataReadingPins(t *testing.T) {
	create := "gh pr " + "create --title t --body b"
	for _, tc := range []struct {
		command string
		ok      bool
		marks   []int
	}{
		{"echo -", true, []int{1}},
		{"grep x -", true, []int{1, 2}},
		{"cat - f", true, nil},
		{"echo -- --", true, []int{2}},
		{"grep x -- --", true, []int{1, 3}},
		{"cat -- --", true, nil},
		{"cat -A=x f", false, nil},
		{"cat --number=x f", false, nil},
		{"mkdir -m=x d", false, nil},
		{create + " --draft=false", false, nil},
		{"gh issue comment 1 -b=x", false, nil},
		{"gh issue create --title=x --body b", true, []int{3, 5}},
		{"mkdir -m $M d", false, nil},
		{"mkdir -- $D", true, nil},
		{"rg --no-config x p.sh", true, []int{2, 3}},
		{"rg -g --no-config x p.sh", false, nil},
		{"rg --no-config=x x p.sh", false, nil},
		{"rg -g --no-config --no-config x p.sh", true, []int{4, 5}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			ok, marks := inertDataInvocation(firstInvocation(t, tc.command, shellwords.POSIX), false)
			got := sortedMarks(marks)
			if ok != tc.ok || !reflect.DeepEqual(got, tc.marks) && (len(got) != 0 || len(tc.marks) != 0) {
				t.Fatalf("inertDataInvocation = (%v, %v), want (%v, %v)", ok, got, tc.ok, tc.marks)
			}
		})
	}
}

// TestGhDataWordsPins fixes which words of one invocation the gh rules treat as
// data: a long prose option joined with `=` is marked whole, a short one is
// not, a non-literal pattern option suppresses the first-operand mark, and
// Select-String names its parameters in any letter case.
func TestGhDataWordsPins(t *testing.T) {
	for _, tc := range []struct {
		command    string
		powerShell bool
		want       []int
	}{
		{"gh issue edit 1 --title=x -b=y -t z", false, []int{4, 7}},
		{"gh issue edit 1 --body --title t", false, []int{5}},
		{"gh issue edit 1 --body", false, nil},
		{"grep -e$X foo", false, nil},
		{"grep -e foo bar", false, []int{2}},
		{"grep -epat f", false, []int{1}},
		{"grep -A 3 pat f", false, []int{3}},
		{"grep -- pat f", false, []int{2}},
		{"echo a b", false, []int{1, 2}},
		{"select-string -PATTERN x", true, []int{2}},
		{"Select-String y -Pattern", true, nil},
		{"Select-String -path p x", true, []int{3}},
		{"Select-String -Pattern=x y", true, []int{2}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			dialect := shellwords.POSIX
			if tc.powerShell {
				dialect = shellwords.PowerShell
			}
			data, _ := ghDataWords(firstInvocation(t, tc.command, dialect))
			var got []int
			for i, marked := range data {
				if marked {
					got = append(got, i)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ghDataWords marks %v, want %v", got, tc.want)
			}
		})
	}
}

func sortedMarks(marks map[int]bool) []int {
	var out []int
	for i, marked := range marks {
		if marked {
			out = append(out, i)
		}
	}
	sort.Ints(out)
	return out
}
