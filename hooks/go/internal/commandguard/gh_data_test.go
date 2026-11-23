package commandguard

import (
	"strings"
	"testing"
)

// ghGatedSpellings is every gated gh verb in a form that denies when executed.
// The verbs are spelled by concatenation so the installed guard does not
// deny the commands that edit this file.
func ghGatedSpellings() []string {
	return []string{
		"gh pr " + "create --title x",
		"gh pr " + "ready 1",
		"gh stack " + "submit",
		"gh stack " + "link --open",
	}
}

func assertGhOutcome(t *testing.T, tool, command, want, category string) {
	t.Helper()
	assertGhOutcomeEnv(t, nil, tool, command, want, category)
}

// assertGhOutcomeEnv is assertGhOutcome with environment variables set after
// the guard scrub, for cases that depend on one such as GIT_PAGER.
func assertGhOutcomeEnv(t *testing.T, env map[string]string, tool, command, want, category string) {
	t.Helper()
	outcome, diag, _ := runGo(t, tableCase{name: "gh-data", tool: tool, command: command, env: env})
	if outcome.Kind() != want {
		t.Fatalf("%s %q: kind = %q, want %q (%s)", tool, command, outcome.Kind(), want, outcome.Payload())
	}
	// The gated gh verbs are all governed by a settings key, and a fixed
	// threat such as command_parse_limit names the reason alone whenever one
	// is present (changing a setting would not unblock the call). The
	// payload then omits the governed publication threat, so the diagnostic
	// table, which lists every threat, is checked as well.
	if category != "" && !strings.Contains(outcome.Payload(), ": "+category+" - ") && !strings.Contains(diag, category) {
		t.Fatalf("%s %q: payload %q does not name %s", tool, command, outcome.Payload(), category)
	}
}

// TestGhDataPositionsPass: a gated verb in a data position is prose, not an
// executed command.
func TestGhDataPositionsPass(t *testing.T) {
	for _, verb := range ghGatedSpellings() {
		for _, c := range []struct{ tool, command string }{
			{"Bash", `echo "` + verb + `"`},
			{"Bash", `echo ` + verb},
			{"Bash", `printf '%s\n' "` + verb + `"`},
			{"Bash", `echo "=== ` + verb + ` rule"; sed -n 880,920p commandguard.go`},
			{"Bash", `echo "` + verb + `" | gh issue comment 1 --body-file -`},
			{"Bash", `echo "` + verb + `" | gh pr comment 1 --body-file -`},
			{"Bash", `echo "` + verb + `" | gh api repos/o/r/issues/1/comments -F body=@-`},
			{"Bash", `echo "` + verb + `" | cat | head -1`},
			{"PowerShell", `Write-Output "` + verb + `"`},
			{"PowerShell", `Write-Host "` + verb + `"`},
			{"PowerShell", `echo "` + verb + `"`},
		} {
			t.Run(c.tool+"/"+c.command, func(t *testing.T) { assertGhOutcome(t, c.tool, c.command, "pass", "") })
		}
	}
}

// TestGhDataRelaxationFailsClosed: every shape that could execute the text
// keeps denying. The first seven are shapes the blanking relaxation must
// never grant.
func TestGhDataRelaxationFailsClosed(t *testing.T) {
	create := "gh pr " + "create"
	for i, c := range []struct{ tool, command string }{
		{"Bash", `eval "` + create + `"`},
		{"Bash", `ssh host ` + create},
		{"Bash", `find . -exec ` + create + ` \;`},
		{"Bash", `watch "` + create + `"`},
		{"Bash", `alias p='` + create + `'`},
		{"Bash", `source <(echo "` + create + `")`},
		{"Bash", `GH=gh; $GH pr ` + "create"},
		{"Bash", `echo "` + create + `" | bash`},
		{"Bash", `bash -c "echo ` + create + `"`},
		{"Bash", `$(echo ` + create + `)`},
		{"Bash", `echo "$(` + create + `)"`},
		{"Bash", `echo "` + create + `"; bash -c "` + create + `"`},
		{"Bash", `echo "` + create + `"; ` + create},
		{"Bash", `printf -v c "` + create + `"; eval "$c"`},
		{"Bash", `printf -v c "` + create + `"; fish -c "$c"`},
		{"Bash", `printf -vc "` + create + `"; fish -c "$c"`},
		{"Bash", `echo "` + create + `" > p.sh; . p.sh`},
		{"Bash", `echo "` + create + `" | gh extension exec runner`},
		{"Bash", `echo "` + create + `" | gh alias set --shell p -`},
		{"Bash", `sudo echo "` + create + `"`},
		{"Bash", "cat <<'EOF' | sh\n" + create + "\nEOF"},
		{"Bash", "echo \"" + create + "\" \\\n  && true"},
		{"Bash", `echo "` + create + `" | fish`},
		{"Bash", `echo "` + create + `" | cat | fish`},
		{"Bash", `(echo a; echo "` + create + `") | fish`},
		{"Bash", `echo "` + create + `" > p.fish; fish p.fish`},
		{"Bash", `echo "` + create + `" 2>&1`},
		// The walk finds the echo's Raw inside the assignment first and blanks
		// that copy; the real echo keeps its text (an over-deny, never a bypass).
		{"Bash", `v='echo "` + create + `"'; echo "` + create + `"`},
		{"PowerShell", `iex "` + create + `"`},
		{"PowerShell", `& ` + create},
		{"PowerShell", `Write-Output "` + create + `" | Out-File p.ps1`},
		{"Bash", `fish -c x; echo "` + create + `"`},
		{"Bash", `csh -c x; echo "` + create + `"`},
		{"Bash", `tcsh -c x; echo "` + create + `"`},
	} {
		category := ""
		if i < 7 {
			category = "unsafe_publication"
		}
		t.Run(c.tool+"/"+c.command, func(t *testing.T) { assertGhOutcome(t, c.tool, c.command, "deny", category) })
	}
}

// TestGhDataFlagPositionsPass: a commit message, a gh body/title/notes/subject
// value, and a search pattern are data.
func TestGhDataFlagPositionsPass(t *testing.T) {
	for _, verb := range ghGatedSpellings() {
		for _, c := range []struct{ tool, command string }{
			{"Bash", `git commit -m "note: ` + verb + ` later"`},
			{"Bash", `git -C repo commit -am "` + verb + `"`},
			{"Bash", `git commit --message="` + verb + `"`},
			// Both lines read to the commit verb, so the message stays data.
			{"Bash", `git --config-env=core.editor=E commit -m "` + verb + `"`},
			{"Bash", `git -C=x commit -m "` + verb + `"`},
			{"Bash", `gh issue comment 1 --body "run ` + verb + `"`},
			{"Bash", `gh issue create --title "` + verb + `" -b x`},
			{"Bash", `gh pr comment 1 -b "` + verb + `"`},
			{"Bash", `gh release edit v1 --notes="` + verb + `"`},
			{"Bash", `grep -rn "` + verb + `" docs/`},
			{"Bash", `rg -n -e "` + verb + `" hooks/`},
			{"PowerShell", `Select-String -Pattern "` + verb + `" -Path docs\x.md`},
			{"PowerShell", `git commit -m "` + verb + `"`},
		} {
			t.Run(c.tool+"/"+c.command, func(t *testing.T) { assertGhOutcome(t, c.tool, c.command, "pass", "") })
		}
	}
}

// TestGhDataFlagPositionsStillGate: blanking a value never hides the executed
// invocation it belongs to, or one beside it.
func TestGhDataFlagPositionsStillGate(t *testing.T) {
	create := "gh pr " + "create"
	for _, command := range []string{
		create + ` --title "` + create + `"`,
		create + ` --body "x --draft"`,
		`git commit -m "x" && ` + create,
		`git -c core.editor="` + create + `" commit`,
		`grep -rn x docs/; ` + create,
		`grep -h "` + create + `" notes.txt | fish`,
		`rg "` + create + `" notes.txt > p.fish`,
		// A git line the grammar cannot read to its verb may not run the
		// commit it seems to name, so its message is not data.
		`git --unknown commit -m "` + create + `"`,
		`git --help commit -m "` + create + `"`,
		`git -- commit -m "` + create + `"`,
		// The separated value is read as an option, so commit is never found
		// and the message was never blanked.
		`git --config-env core.editor=E commit -m "` + create + `"`,
	} {
		t.Run(command, func(t *testing.T) { assertGhOutcome(t, "Bash", command, "deny", "unsafe_publication") })
	}
}
