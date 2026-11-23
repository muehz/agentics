package flow

import (
	"strings"
	"testing"
)

// exclusionCases hold rows that fit no form beside the evidence a valid row
// would give, so each shows an invalid row counting for nothing: it names no
// commit for unaccounted: and is never the row the commit check reads.
func exclusionCases() []contractCase {
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	bound := row("2026-11-01T10:10:00Z", "execute", "bind", "topic", "ok", "bound topic to this flow", "none", "execute go")
	run := row("2026-11-01T10:20:00Z", "execute", "enter", "run", "ok", "start", "mode=solo branch=topic", "execute go")
	status := []string{"status", "--folder", contractFolder}
	absent := strings.Repeat("0", 39) + "1"
	earlier := strings.Repeat("0", 39) + "2"
	// topic is a flow committed on main and bound to topic, checked out on
	// topic with one task commit T on its parent B, then the verify rows that
	// rows builds.
	topic := func(rows func(fx *contractFixture) []string) func(t *testing.T) *contractFixture {
		return withFixtureSteps(onBranch("topic", withSteps(newState(seeded, bound, run), commitStep)),
			committedAs("T", "src/t.txt"),
			func(t *testing.T, fx *contractFixture) {
				t.Helper()
				fx.values["B"] = gitOut(t, fx.repo, "rev-parse", "HEAD~1")
				for _, r := range rows(fx) {
					rowOf(func(*contractFixture) string { return r })(t, fx)
				}
			})
	}
	verify := func(ts, evidence string) string {
		return row("2026-11-01T"+ts+":00Z", "execute", "verify", "task-1", "ok", "green", evidence, "execute go")
	}
	listed := func(name string, evidence func(fx *contractFixture) string, keys ...string) contractCase {
		return contractCase{
			name:  "exclusion/" + name,
			setup: topic(func(fx *contractFixture) []string { return []string{verify("11:00", evidence(fx))} }),
			steps: []contractStep{{args: status, stdoutCount: []lineCount{{"unaccounted:", 1}}, check: unaccountedIs(keys...)}},
		}
	}
	mismatchOf := func(sha string) string {
		return "mismatch: verify task-1 commit " + sha + " is not an ancestor of HEAD and patch-id " + pid40 + " matches no commit on this branch"
	}
	return []contractCase{
		listed("an-invalid-row-naming-the-commit-leaves-it-unaccounted",
			func(fx *contractFixture) string { return "commit=" + fx.values["T"] + " uncommitted:yes" }, "T"),
		listed("a-valid-row-naming-the-commit-accounts-for-it",
			func(fx *contractFixture) string {
				return "commit=" + fx.values["T"] + " patch-id=" + patchIDOf(fx.repo, fx.values["T"])
			}),
		// base= and head= without integrated= fit no form, so their range
		// names nothing, though base..head holds T.
		listed("an-invalid-row-with-a-range-names-none-of-its-commits",
			func(fx *contractFixture) string { return "base=" + fx.values["B"] + " head=" + fx.values["T"] }, "T"),
		listed("an-invalid-row-with-the-commits-patch-id-names-nothing",
			func(fx *contractFixture) string { return "patch-id=" + patchIDOf(fx.repo, fx.values["T"]) }, "T"),
		{
			name: "exclusion/a-latest-invalid-row-is-not-checked",
			setup: topic(func(*contractFixture) []string {
				return []string{verify("11:00", "commit="+absent+" uncommitted:yes")}
			}),
			steps: []contractStep{{args: status, stdoutCount: []lineCount{{"invalid:", 1}, {"mismatch:", 0}}}},
		},
		{
			name: "exclusion/a-latest-invalid-row-leaves-the-earlier-valid-row-checked",
			setup: topic(func(*contractFixture) []string {
				return []string{
					verify("11:00", "commit="+earlier+" patch-id="+pid40),
					verify("11:30", "commit="+absent+" uncommitted:yes"),
				}
			}),
			steps: []contractStep{{
				args:        status,
				stdoutLines: []string{mismatchOf(earlier)},
				stdoutCount: []lineCount{{"invalid:", 1}, {"mismatch:", 1}},
			}},
		},
	}
}

func TestExclusionContract(t *testing.T) {
	t.Parallel()
	for _, c := range exclusionCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			runContractCase(t, c)
		})
	}
}
