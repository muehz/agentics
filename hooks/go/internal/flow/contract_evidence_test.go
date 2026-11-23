package flow

import (
	"strings"
	"testing"
)

// evidenceCases are verify rows through flow log: the refusals, and the forms
// that name no commit, so that commit resolution on write cannot refuse them.
func evidenceCases() []contractCase {
	accepted := func(name, target, status, evidence string) contractCase {
		return acceptedLog("evidence/"+name, "verify", target, status, evidence)
	}
	refused := func(name, target, status, evidence, reason string) contractCase {
		return refusedLog("evidence/"+name, "verify", target, status, evidence, "evidence: '"+evidence+"' -- "+reason)
	}
	patchID := strings.Repeat("b", 40)
	sha := strings.Repeat("a", 40)
	isolated := "base=" + sha + " head=" + sha + " integrated=" + sha + " patch-id=" + patchID
	const (
		taskOK = "task ok evidence is one of: commit=<sha> patch-id=<id>; commit=<sha> on a merge commit; base=<sha> head=<sha> integrated=<sha> patch-id=<id>"
		fixOK  = "fix ok evidence is one of: commit=<sha> patch-id=<id>; commit=<sha> on a merge commit"
	)
	return []contractCase{
		refused("a-file-path-is-refused", "task-1", "ok", "docs/specs/2026-11/feature-x/plan.md", "docs/specs/2026-11/feature-x/plan.md is not a key=value pair"),
		refused("none-is-refused", "task-1", "ok", "none", "none is not a key=value pair"),
		refused("uncommitted-text-is-refused", "task-1", "ok", "uncommitted: tests pass in the worktree", "uncommitted: is not a key=value pair"),
		refused("prose-is-refused", "task-1", "ok", "tests pass", "tests is not a key=value pair"),
		refused("an-absolute-path-is-refused", "task-1", "ok", "C:/work/repo/out.txt", "C:/work/repo/out.txt is not a key=value pair"),
		refused("a-bare-sha-is-refused", "task-1", "ok", "abc1234", "abc1234 is not a key=value pair"),
		refused("a-repeated-key-is-refused", "task-1", "ok", "commit=abc1234 commit=abc1234", "key commit repeats"),
		refused("an-unknown-key-is-refused", "task-1", "ok", "commit=abc1234 note=x", "key note belongs to no task form"),
		refused("a-short-patch-id-is-refused", "task-1", "ok", "patch-id=abc", "patch-id=abc is not a patch-id of 40 or 64 lowercase hex"),
		refused("a-commit-that-is-no-hex-is-refused", "task-1", "ok", "commit=xyz", "commit=xyz is not a commit id of 7 to 64 lowercase hex"),
		refused("a-slice-that-is-no-slice-list-is-refused", "task-1", "failed", "cause=repair slice=S1,not-a-slice", "slice=S1,not-a-slice is not a comma-separated list of S<n> ids"),
		refused("a-slice-list-with-an-empty-id-is-refused", "task-1", "failed", "cause=repair slice=S1,,S2", "slice=S1,,S2 is not a comma-separated list of S<n> ids"),
		refused("a-patch-id-alone-is-refused", "task-1", "ok", "patch-id="+patchID, taskOK),
		refused("the-failure-form-on-ok-is-refused", "task-1", "ok", "cause=repair", taskOK),
		refused("the-branch-form-on-fix-ok-is-refused", "fix", "ok", "branch=b patch-id="+patchID, fixOK),
		refused("the-branch-form-on-a-task-is-refused", "task-1", "failed", "branch=b patch-id="+patchID, "key branch belongs to no task form"),
		refused("the-isolated-form-on-a-wave-is-refused", "wave-1", "ok", isolated, "key base belongs to no integrated form"),
		refused("the-isolated-form-on-a-red-task-is-refused", "red-task-1", "failed", isolated, "key base belongs to no red form"),
		refused("the-failure-form-on-a-wave-is-refused", "wave-1", "failed", "cause=repair", "integrated failed evidence is one of: commit=<sha>; commit=<sha> patch-id=<id>"),

		accepted("the-failure-form-is-accepted", "task-1", "failed", "cause=repair"),
		accepted("the-failure-form-on-fix-is-accepted", "fix", "failed", "cause=context"),
		accepted("a-slice-list-is-accepted", "task-1", "failed", "cause=repair slice=S1,S2"),
		accepted("the-branch-form-on-a-failed-fix-is-accepted", "fix", "failed", "branch=b patch-id="+patchID),
		accepted("the-no-commit-form-on-a-red-task-is-accepted", "red-task-1", "failed", "no-commit exit=1 cmd=go test out=FAIL"),
	}
}

func TestEvidenceContract(t *testing.T) {
	t.Parallel()
	for _, c := range evidenceCases() {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			runContractCase(t, c)
		})
	}
}
