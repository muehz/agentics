package flow

import (
	"strings"
	"testing"
)

// withArtifact is a setup that runs inner, writes an artifact into the flow folder and records its text hash as the value "text".
func withArtifact(name, body string, inner func(t *testing.T) *contractFixture) func(t *testing.T) *contractFixture {
	return func(t *testing.T) *contractFixture {
		t.Helper()
		fx := inner(t)
		writeFile(t, fx.repo, fx.folder+"/"+name, body)
		fx.values["text"] = textHash(name, []byte(body))
		return fx
	}
}

// withHead is a setup that runs inner and records HEAD as the value "head" and its patch id as "head-patch".
func withHead(inner func(t *testing.T) *contractFixture) func(t *testing.T) *contractFixture {
	return func(t *testing.T) *contractFixture {
		t.Helper()
		fx := inner(t)
		fx.values["head"] = gitOut(t, fx.repo, "rev-parse", "HEAD")
		fx.values["head-patch"] = patchIDOf(fx.repo, fx.values["head"])
		return fx
	}
}

func formsCases() []contractCase {
	const why, next = contractWhy, contractNext
	accepted := func(name, event, target, status, evidence string) contractCase {
		return acceptedLog("forms/"+name, event, target, status, evidence)
	}
	refused := func(name, event, target, status, evidence, message string) contractCase {
		return refusedLog("forms/"+name, event, target, status, evidence, message)
	}
	status := []string{"status", "--folder", contractFolder}
	// autoRow is the stored form of a row logged with evidence auto at the fixture's HEAD.
	autoRow := func(fx *contractFixture, event, target, status string) string {
		return cellsRow("execute", event, target, status, why, "commit="+fx.values["head"]+" patch-id="+fx.values["head-patch"], next)
	}

	const (
		escape      = "found-by=person severity=critical review=quality"
		escapeShape = " -- escape evidence is: found-by severity review, and optionally: word"
		review      = "kind=spec dispatched=2026-11-10T03:59:13Z round=1 blocked=0"
		reviewShape = " -- review evidence is: kind dispatched round blocked, and optionally: rev by commit"
		reviewKeys  = "kind dispatched round blocked rev by commit"
		sha         = "0123456789abcdef0123456789abcdef01234567"
		absent      = "0000000000000000000000000000000000000009"
		noCommit    = "no-commit exit=1 cmd=go test ./internal/flow -run TestRed out=FAIL: TestRed"
		noCommitIs  = " -- no-commit evidence is: no-commit exit=<n> cmd=<command> out=<excerpt>"
	)
	// sized is a valid no-commit cell of exactly n characters.
	sized := func(n int) string {
		const head = "no-commit exit=1 cmd=go test ./... out="
		return head + strings.Repeat("x", n-len(head))
	}
	quoted := func(evidence string) string { return "evidence: '" + evidence + "'" }
	withExit := func(exit string) string { return strings.Replace(noCommit, "exit=1", "exit="+exit, 1) }
	const badExit = " -- exit must be a non-zero integer exit status"

	cases := []contractCase{
		accepted("escape-with-three-pairs-is-accepted", "escape", "task-1", "failed", escape),
		accepted("escape-with-a-word-is-accepted", "escape", "task-1", "failed", escape+" word=blocker"),
		accepted("escape-takes-the-word-among-the-other-pairs", "escape", "task-1", "failed", "word=must-fix found-by=ci severity=important review=plan"),
		refused("escape-with-a-fifth-pair-is-refused", "escape", "task-1", "failed", escape+" word=a word=b", quoted(escape+" word=a word=b")+escapeShape),
		refused("escape-with-the-word-twice-is-refused", "escape", "task-1", "failed", "found-by=ci severity=critical word=a word=b", quoted("found-by=ci severity=critical word=a word=b")+" -- key word repeats"),
		refused("escape-with-a-word-in-place-of-a-required-pair-is-refused", "escape", "task-1", "failed", "found-by=ci severity=critical word=a", quoted("found-by=ci severity=critical word=a")+escapeShape),
		refused("escape-with-another-fourth-pair-is-refused", "escape", "task-1", "failed", escape+" note=a", quoted(escape+" note=a")+" -- unknown key note; expected: found-by severity review word"),
		refused("escape-with-another-severity-is-refused", "escape", "task-1", "failed", "found-by=person severity=minor review=quality word=minor", quoted("found-by=person severity=minor review=quality word=minor")+" -- severity must be one of: critical important"),
		refused("escape-of-a-spec-review-is-refused", "escape", "task-1", "failed", "found-by=person severity=critical review=spec", quoted("found-by=person severity=critical review=spec")+" -- review must be one of: plan packet quality integration"),

		accepted("review-of-kind-spec-is-accepted", "review", "spec.md", "ok", review),
		accepted("review-with-the-optional-pairs-is-accepted", "review", "spec.md", "failed", review+" rev=2 by=independent commit="+sha),
		accepted("review-takes-the-optional-pairs-in-any-order", "review", "plan.md", "ok", "commit=0123abc by=self kind=plan dispatched=2026-11-10T03:59:13Z rev=10 round=1 blocked=0"),
		accepted("review-takes-one-optional-pair", "review", "spec.md", "ok", review+" by=self"),
		refused("review-of-another-kind-is-refused", "review", "spec.md", "ok", strings.Replace(review, "spec", "intent", 1), quoted(strings.Replace(review, "spec", "intent", 1))+" -- kind must be one of: plan packet quality integration spec"),
		refused("review-with-an-optional-pair-in-place-of-a-required-one-is-refused", "review", "spec.md", "ok", "kind=spec dispatched=2026-11-10T03:59:13Z round=1 rev=1", quoted("kind=spec dispatched=2026-11-10T03:59:13Z round=1 rev=1")+reviewShape),
		refused("review-with-revision-zero-is-refused", "review", "spec.md", "ok", review+" rev=0", quoted(review+" rev=0")+" -- rev must be a positive integer"),
		refused("review-with-a-revision-that-is-no-number-is-refused", "review", "spec.md", "ok", review+" rev=two", quoted(review+" rev=two")+" -- rev must be a positive integer"),
		refused("review-by-another-reviewer-is-refused", "review", "spec.md", "ok", review+" by=author", quoted(review+" by=author")+" -- by must be one of: independent self"),
		refused("review-with-a-ref-name-as-its-commit-is-refused", "review", "spec.md", "ok", review+" commit=HEAD", quoted(review+" commit=HEAD")+" -- commit must be a commit id"),
		refused("review-with-a-six-character-commit-is-refused", "review", "spec.md", "ok", review+" commit=0123ab", quoted(review+" commit=0123ab")+" -- commit must be a commit id"),
		refused("review-with-the-revision-twice-is-refused", "review", "spec.md", "ok", review+" rev=1 rev=2", quoted(review+" rev=1 rev=2")+" -- key rev repeats"),
		refused("review-with-another-pair-is-refused", "review", "spec.md", "ok", review+" note=a", quoted(review+" note=a")+" -- unknown key note; expected: "+reviewKeys),
		refused("review-with-eight-pairs-is-refused", "review", "spec.md", "ok", review+" rev=1 by=self commit="+sha+" rev=2", quoted(review+" rev=1 by=self commit="+sha+" rev=2")+reviewShape),
		{
			name:  "forms/metrics-count-a-spec-review",
			setup: emptyRepo,
			steps: []contractStep{
				{args: contractLog("spec", "review", "spec.md", "failed", why, review+" rev=1 by=independent", next)},
				{args: contractLog("spec", "review", "spec.md", "ok", why, strings.Replace(review, "round=1", "round=2", 1)+" rev=2 by=self", next)},
				{args: contractLog("decompose", "review", "plan.md", "ok", why, strings.Replace(review, "spec", "plan", 1), next)},
				{args: append(status[:len(status):len(status)], "--metrics"), stdoutLines: []string{"reviews: plan=1 spec=2"}},
			},
		},
		{
			name: "forms/review-commit-is-not-a-checked-commit",
			setup: newState(
				row("2026-11-01T10:00:00Z", "spec", "review", "spec.md", "ok", why, review+" commit="+absent+" patch-id=deadbeef", next)),
			steps: []contractStep{{args: status, stdoutCount: []lineCount{{"mismatch:", 0}}}},
		},

		accepted("verify-red-task-without-a-commit-is-accepted", "verify", "red-task-1", "failed", noCommit),
		accepted("verify-red-fix-ends-the-command-at-the-first-out", "verify", "red-fix", "failed", "no-commit exit=127 cmd=sh -c 'run --log out.txt' out=sh: run: not found out=1"),
		accepted("verify-red-fix-takes-a-cell-of-500-characters", "verify", "red-fix", "failed", sized(500)),
		{
			name:  "forms/verify-red-fix-without-a-commit-raises-no-mismatch",
			setup: emptyRepo,
			steps: []contractStep{
				{args: contractLog("execute", "verify", "red-fix", "failed", why, "no-commit exit=1 cmd=git log --grep commit="+absent+" patch-id=deadbeef out=no match", next)},
				{args: status, stdoutCount: []lineCount{{"mismatch:", 0}}},
			},
			rows: []string{cellsRow("execute", "verify", "red-fix", "failed", why, "no-commit exit=1 cmd=git log --grep commit="+absent+" patch-id=deadbeef out=no match", next)},
		},
		accepted("verify-red-task-takes-a-negative-exit", "verify", "red-task-1", "failed", withExit("-1")),
		accepted("verify-red-fix-takes-a-large-negative-exit", "verify", "red-fix", "failed", withExit("-1073741819")),
		accepted("verify-red-fix-quotes-an-entry-pair-in-its-output", "verify", "red-fix", "failed", "no-commit exit=1 cmd=go test out=want entry=abc"),
		accepted("verify-red-fix-takes-an-empty-excerpt", "verify", "red-fix", "failed", "no-commit exit=1 cmd=go test out="),
		refused("verify-red-fix-with-an-empty-command-is-refused", "verify", "red-fix", "failed", "no-commit exit=1 cmd= out=x", quoted("no-commit exit=1 cmd= out=x")+noCommitIs),
		refused("verify-red-fix-with-exit-zero-is-refused", "verify", "red-fix", "failed", withExit("0"), quoted(withExit("0"))+badExit),
		refused("verify-red-task-with-exit-zero-is-refused", "verify", "red-task-1", "failed", withExit("0"), quoted(withExit("0"))+badExit),
		refused("verify-red-fix-with-exit-double-zero-is-refused", "verify", "red-fix", "failed", withExit("00"), quoted(withExit("00"))+badExit),
		refused("verify-red-fix-with-a-plus-sign-is-refused", "verify", "red-fix", "failed", withExit("+1"), quoted(withExit("+1"))+badExit),
		refused("verify-red-fix-with-a-trailing-letter-is-refused", "verify", "red-fix", "failed", withExit("1x"), quoted(withExit("1x"))+badExit),
		refused("verify-red-fix-with-an-empty-exit-is-refused", "verify", "red-fix", "failed", withExit(""), quoted(withExit(""))+badExit),
		refused("verify-red-fix-with-an-exit-that-is-no-number-is-refused", "verify", "red-fix", "failed", withExit("failed"), quoted(withExit("failed"))+badExit),
		refused("verify-red-fix-without-an-out-pair-is-refused", "verify", "red-fix", "failed", "no-commit exit=1 cmd=go test ./internal/flow", quoted("no-commit exit=1 cmd=go test ./internal/flow")+noCommitIs),
		refused("verify-red-fix-without-a-command-is-refused", "verify", "red-fix", "failed", "no-commit exit=1 out=FAIL", quoted("no-commit exit=1 out=FAIL")+noCommitIs),
		refused("verify-red-fix-without-an-exit-is-refused", "verify", "red-fix", "failed", "no-commit cmd=go test out=FAIL", quoted("no-commit cmd=go test out=FAIL")+noCommitIs),
		refused("verify-red-fix-with-the-bare-word-is-refused", "verify", "red-fix", "failed", "no-commit", quoted("no-commit")+noCommitIs),
		refused("verify-red-fix-with-a-cell-of-501-characters-is-refused", "verify", "red-fix", "failed", sized(501), quoted(sized(501))+" -- a no-commit cell is at most 500 characters"),
		{
			name:  "forms/verify-red-fix-still-takes-a-commit",
			setup: withHead(emptyRepo),
			steps: []contractStep{{args: contractLog("execute", "verify", "red-fix", "failed", why, "auto", next)}},
			check: rowsAre(func(fx *contractFixture) []string { return []string{autoRow(fx, "verify", "red-fix", "failed")} }),
		},
		refused("verify-of-another-target-refuses-the-no-commit-form", "verify", "task-1", "failed", "no-commit exit=0", quoted("no-commit exit=0")+" -- no-commit is not a key=value pair"),

		accepted("approve-declined-is-accepted", "approve", "intent.md", "declined", "none"),
		refused("approve-declined-with-a-pointer-is-refused", "approve", "intent.md", "declined", "intent.md", "evidence: 'intent.md' -- a declined approve row has evidence none"),
		refused("approve-declined-with-a-hash-is-refused", "approve", "intent.md", "declined", "blob=0123abc", "evidence: 'blob=0123abc' -- a declined approve row has evidence none"),
		{
			name:  "forms/approve-declined-with-auto-is-refused",
			setup: withArtifact("intent.md", "# intent\n", emptyRepo),
			steps: []contractStep{{
				args:        contractLog("intent", "approve", "intent.md", "declined", why, "auto", next),
				code:        2,
				stderrLines: []string{"flow: invalid evidence: 'auto' -- a declined approve row has evidence none"},
			}},
			check: noStateFile,
		},
		{
			name:  "forms/approve-declined-approves-nothing",
			setup: emptyRepo,
			steps: []contractStep{
				{args: contractLog("intent", "approve", "intent.md", "declined", why, "none", next)},
				{args: status, stdoutLines: []string{"approvals: none"}},
			},
		},
		{
			name:  "forms/approve-declined-keeps-an-earlier-approval",
			setup: withArtifact("intent.md", "# intent\n", emptyRepo),
			steps: []contractStep{
				{argsOf: func(fx *contractFixture) []string {
					return append(contractLog("intent", "approve", "intent.md", "ok", why, "auto", next), "--expect-blob", fx.values["text"])
				}},
				{args: contractLog("intent", "approve", "intent.md", "declined", why, "none", next)},
				{args: status, stdoutHas: []string{" current\n"}, stdoutCount: []lineCount{{"approvals: intent.md approved ", 1}, {"approvals:", 1}}},
			},
			check: rowsAre(func(fx *contractFixture) []string {
				return []string{
					cellsRow("intent", "approve", "intent.md", "ok", why, "text="+fx.values["text"], next),
					cellsRow("intent", "approve", "intent.md", "declined", why, "none", next),
				}
			}),
		},

		accepted("challenge-with-a-clear-scan-is-accepted", "challenge", "intent.md", "ok", "scan=clear"),
		{
			name:  "forms/verify-pending-is-accepted",
			setup: withHead(emptyRepo),
			steps: []contractStep{{args: contractLog("execute", "verify", "task-1", "pending", why, "auto", next)}},
			check: rowsAre(func(fx *contractFixture) []string { return []string{autoRow(fx, "verify", "task-1", "pending")} }),
		},
	}
	return cases
}
