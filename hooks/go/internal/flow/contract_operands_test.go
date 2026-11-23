package flow

import (
	"strings"
	"testing"
)

// headTargets holds one accepted waive target per head, each with the fewest
// arguments its head takes. It is written out rather than built from
// waiveHeads, so a head dropped from or miscounted in the table fails here.
var headTargets = []string{
	"checklist",
	"proportionality",
	"coverage",
	"no-code",
	"controller-rule",
	"independence g1",
	"quality-review g1",
	"verification task-1",
	"deepen seams",
	"aggregate-review",
	"final-review",
	"union-rule",
	"stage-1",
	"verification-preflight",
	"wave-pauses",
	"diagnose chore/issue-1141",
	"test-first",
	"delete-untested",
	"refactor",
	"root-cause",
	"prototype-code",
	"chain",
	"direction-blockers",
	"brief-questions",
	"direction-review",
	"spec-step seams",
	"docs-refresh",
	"review",
	"converge",
	"tests",
	"work-folder",
	"security-depth",
	"finding audit src/a.go:3",
	"artifact",
	"migrations",
	"flags",
	"rollback",
	"ci",
	"on-call",
	"comms",
	"window",
	"rule docs-first",
}

// operandsCases are waive and demote rows through flow log: the targets the
// emitting skills write, every head once, and the targets outside the list.
func operandsCases() []contractCase {
	accepted := func(name, event, target string) contractCase {
		return acceptedLog("operands/"+name, event, target, "ok", "none")
	}
	refused := func(name, event, target, reason string) contractCase {
		return refusedLog("operands/"+name, event, target, "ok", "none", "target: '"+target+"' -- "+reason)
	}
	cases := []contractCase{
		accepted("aggregate-review-alone", "waive", "aggregate-review"),
		accepted("aggregate-review-with-a-wave", "waive", "aggregate-review wave-2"),
		accepted("quality-review-with-a-group", "waive", "quality-review g1"),
		accepted("verification-with-two-tasks", "waive", "verification task-1 task-2"),
		accepted("no-code-alone", "waive", "no-code"),
		accepted("no-code-with-a-task", "waive", "no-code task-1"),
		accepted("finding-with-a-lane-and-locator", "waive", "finding audit src/a.go:3"),
		accepted("ci-with-a-check", "waive", "ci:required-checks"),
		accepted("ci-alone", "waive", "ci"),
		accepted("rule-with-a-name", "waive", "rule docs-first"),
		accepted("spec-step-with-a-step", "waive", "spec-step seams"),
		accepted("test-first", "waive", "test-first"),
		accepted("root-cause", "waive", "root-cause"),
		accepted("prototype-code", "waive", "prototype-code"),
		accepted("diagnose-with-a-branch", "waive", "diagnose chore/issue-1141"),
		accepted("demote-a-numbered-wave", "demote", "wave-2"),
		accepted("demote-a-lettered-wave", "demote", "wave-C"),

		refused("a-spaced-head", "waive", "aggregate review wave-5", "head aggregate is not in the Operands list of flow-state.md"),
		refused("a-wave-named-head", "waive", "wave-3-aggregate", "head wave-3-aggregate is not in the Operands list of flow-state.md"),
		refused("a-made-up-head", "waive", "made-up-gate", "head made-up-gate is not in the Operands list of flow-state.md"),
		refused("finding-without-a-locator", "waive", "finding audit", "finding takes 2 arguments, got 1"),
		refused("quality-review-without-a-group", "waive", "quality-review", "quality-review takes 1 arguments, got 0"),
		refused("checklist-with-an-argument", "waive", "checklist extra", "checklist takes 0 arguments, got 1"),
		refused("a-check-with-capitals", "waive", "ci:Bad", "check Bad is not a-z 0-9 -"),
		refused("rule-without-a-name", "waive", "rule", "rule takes 1 arguments, got 0"),
		refused("diagnose-without-a-branch", "waive", "diagnose", "diagnose takes 1 arguments, got 0"),
		refused("diagnose-with-two-arguments", "waive", "diagnose a b", "diagnose takes 1 arguments, got 2"),
		refused("verification-without-a-task", "waive", "verification", "verification takes 1 or more arguments, got 0"),
		refused("aggregate-review-with-two-arguments", "waive", "aggregate-review a b", "aggregate-review takes 0 or 1 arguments, got 2"),
		refused("a-check-on-a-head-that-takes-none", "waive", "checklist:x", ":<check> is allowed only on a release item"),
		refused("an-empty-check", "waive", "ci:", "check is empty"),
		refused("a-check-with-an-argument", "waive", "ci:x extra", "ci takes 0 arguments, got 1"),
		refused("an-empty-argument", "waive", "no-code  task-1", "an argument is empty"),
		refused("demote-a-bare-number", "demote", "2", "demote names wave-<id>"),
		refused("demote-a-wave-without-an-id", "demote", "wave-", "demote names wave-<id>"),
	}
	for _, target := range headTargets {
		head, _, _ := strings.Cut(target, " ")
		cases = append(cases, accepted("head/"+head, "waive", target))
	}
	return cases
}

func TestOperandsContract(t *testing.T) {
	t.Parallel()
	for _, c := range operandsCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			runContractCase(t, c)
		})
	}
}

// TestWaiveHeadsMatchHeadTargets keeps the accepted-per-head cases and the
// table in step, so a head added to one is not left untested by the other.
func TestWaiveHeadsMatchHeadTargets(t *testing.T) {
	t.Parallel()
	var tested []string
	for _, target := range headTargets {
		head, _, _ := strings.Cut(target, " ")
		tested = append(tested, head)
	}
	var listed []string
	for _, head := range waiveHeads {
		listed = append(listed, head.name)
	}
	if got, want := strings.Join(listed, " "), strings.Join(tested, " "); got != want {
		t.Fatalf("waiveHeads lists %q, headTargets tests %q", got, want)
	}
}
