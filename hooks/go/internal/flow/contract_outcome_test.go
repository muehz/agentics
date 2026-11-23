package flow

import (
	"strings"
	"testing"
)

// TestOutcomeProof runs the end-to-end step of checkpoint evidence
// validation in one fixture: flow log refuses the evidence cells seen in real
// transcripts and a waive head outside the Operands list, flow status reports
// a hand-written invalid row and leaves its commit unaccounted, and a later
// valid row supersedes it and accounts for the commit.
func TestOutcomeProof(t *testing.T) {
	t.Parallel()
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	bound := row("2026-11-01T10:10:00Z", "execute", "bind", "topic", "ok", "bound topic to this flow", "none", "execute go")
	run := row("2026-11-01T10:20:00Z", "execute", "enter", "run", "ok", "start", "mode=solo branch=topic", "execute go")
	status := []string{"status", "--folder", contractFolder}
	absent := strings.Repeat("0", 39) + "1"
	notAPair := func(token string) string { return token + " is not a key=value pair" }
	handInvalid := "invalid: 2026-11-01T11:00:00Z s1 verify task-1: " + notAPair("uncommitted:yes")

	// rowsKept fails when a refused call changed the three setup rows.
	rowsKept := func(t *testing.T, fx *contractFixture, _ result) {
		t.Helper()
		if got := contractRows(t, fx); len(got) != 3 {
			t.Errorf("a refused call changed state.tsv: %d rows, want the 3 setup rows: %q", len(got), got)
		}
	}
	refusedVerify := func(cell, reason string) contractStep {
		return contractStep{
			args:        contractLog("execute", "verify", "task-1", "ok", "x", cell, "execute next"),
			code:        2,
			stderrLines: []string{"flow: invalid evidence: '" + cell + "' -- " + reason},
			check:       rowsKept,
		}
	}

	steps := []contractStep{
		refusedVerify("docs/specs/2026-11/feature-x/plan.md", notAPair("docs/specs/2026-11/feature-x/plan.md")),
		refusedVerify("none", notAPair("none")),
		refusedVerify("uncommitted: tests pass in the worktree", notAPair("uncommitted:")),
		refusedVerify("tests pass C:/work/repo/out.txt", notAPair("tests")),
		refusedVerify("abc1234", notAPair("abc1234")),
		refusedVerify("commit="+absent+" patch-id="+pid40, "commit "+absent+" does not resolve to a commit"),
		{
			args:        contractLog("execute", "waive", "wave-3-aggregate", "ok", "x", "none", "execute next"),
			code:        2,
			stderrLines: []string{"flow: invalid target: 'wave-3-aggregate' -- head wave-3-aggregate is not in the Operands list of flow-state.md"},
			check:       rowsKept,
		},
		{
			before: func(t *testing.T, fx *contractFixture) {
				t.Helper()
				evidence := "commit=" + fx.values["T"] + " uncommitted:yes"
				appendStep(contractFolder, row("2026-11-01T11:00:00Z", "execute", "verify", "task-1", "ok", "green", evidence, "execute go"))(t, fx.repo)
			},
			args:        status,
			stdoutCount: []lineCount{{"invalid:", 1}, {"unaccounted:", 1}},
			check: func(t *testing.T, fx *contractFixture, res result) {
				t.Helper()
				invalidLinesAre(keepText, handInvalid)(t, fx, res)
				unaccountedIs("T")(t, fx, res)
			},
		},
		{
			argsOf: func(fx *contractFixture) []string {
				return contractLog("execute", "verify", "task-1", "ok", "x", "auto:"+fx.values["T"], "execute next")
			},
			check: func(t *testing.T, fx *contractFixture, _ result) {
				t.Helper()
				got := contractRows(t, fx)
				want := cellsRow("execute", "verify", "task-1", "ok", "x",
					"commit="+fx.values["T"]+" patch-id="+patchIDOf(fx.repo, fx.values["T"]), "execute next")
				if len(got) != 5 || got[4] != want {
					t.Errorf("rows after the auto call:\n got %q\nwant the setup rows, the hand-written row and %q", got, want)
				}
			},
		},
		{
			args:        status,
			stdoutCount: []lineCount{{"invalid:", 1}, {"unaccounted:", 1}},
			check: func(t *testing.T, fx *contractFixture, res result) {
				t.Helper()
				lines := dataLines(readLines(t, fx.statePath()))
				ts := cells(lines[len(lines)-1])[0]
				invalidLinesAre(keepText, handInvalid+" (superseded "+ts+")")(t, fx, res)
				unaccountedIs()(t, fx, res)
			},
		},
	}
	runContractCase(t, contractCase{
		name: "outcome/proof",
		setup: withFixtureSteps(onBranch("topic", withSteps(newState(seeded, bound, run), commitStep)),
			committedAs("T", "src/t.txt")),
		steps: steps,
	})
}

// keepText is an expand function for invalidLinesAre whose lines hold no placeholders.
func keepText(_ *contractFixture, text string) string { return text }
