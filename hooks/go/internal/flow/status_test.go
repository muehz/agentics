package flow

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const folderX = "docs/specs/2026-10/feature-x"

func TestStatusReadFailureDoesNotReportASummary(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("requires a regular procfs file that rejects reads")
	}
	repo := newRepo(t)
	folder := filepath.Join(repo, "unreadable")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(folder, "state.tsv")
	// This write-only procfs interface rejects reads even for a root test process.
	if err := os.Symlink("/proc/self/clear_refs", file); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(file); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("fixture must pass the regular-file check: %v, %v", info, err)
	}
	if _, err := os.ReadFile(file); err == nil {
		t.Fatal("fixture must fail reading the regular file")
	}
	res := runFlow(t, repo, nil, "status", "--folder", folder)
	mustExit(t, res, 1, "state read failure")
	if res.stdout != "" || !strings.HasPrefix(res.stderr, "flow: cannot read ") || !strings.Contains(res.stderr, "state.tsv") {
		t.Fatalf("read failure reported as success: stdout=%q stderr=%q", res.stdout, res.stderr)
	}
}

func statusOf(t *testing.T, repo, folder string, extra ...string) []string {
	t.Helper()
	args := append([]string{"status", "--folder", folder}, extra...)
	res := runFlow(t, repo, nil, args...)
	mustExit(t, res, 0, "status")
	if res.stderr != "" {
		t.Fatalf("status stderr = %q", res.stderr)
	}
	return strings.Split(strings.TrimRight(res.stdout, "\n"), "\n")
}

func hasLine(lines []string, prefix string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			return true
		}
	}
	return false
}

func lineWith(lines []string, prefix string) string {
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}

func TestStatusResumesFromStateAlone(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	logOK(t, repo, folderX, "intent", "create", "intent.md", "ok", "capture the problem", "intent.md", "spec define the solution")
	logOK(t, repo, folderX, "intent", "challenge", "intent.md", "ok", "direction challenged", "none", "spec define the solution")
	got := statusOf(t, repo, folderX)
	want := []string{
		"flow: " + folderX,
		"storage: committed",
		"phase: intent (challenge, ok)",
		"next: spec define the solution",
		"approvals: none",
		"orders: none",
		"divergences: none",
		"blocks: none",
		"slice: not bound",
		"unaccounted: not computed (this branch is not bound to the flow)",
		"sessions: 1",
	}
	if len(got) != 12 {
		t.Fatalf("want 12 lines, got %d: %q", len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("line %d = %q, want %q", i, got[i], w)
		}
	}
	if !strings.HasPrefix(got[11], "last: ") || !strings.HasSuffix(got[11], " test-session") {
		t.Fatalf("want a final last: line, got %q", got)
	}
}

func TestStatusStaleApprovalForEveryArtifact(t *testing.T) {
	t.Parallel()
	for _, artifact := range []string{"intent.md", "spec.md", "plan.md"} {
		t.Run(artifact, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\none\n")
			approveGuarded(t, repo, folderX, "decompose", artifact, "approved", "execute go")
			if got := lineWith(statusOf(t, repo, folderX), "approvals:"); !strings.HasSuffix(got, " current") {
				t.Fatalf("fresh approval should read current, got %q", got)
			}
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\none\ntwo\n")
			if got := lineWith(statusOf(t, repo, folderX), "approvals:"); !strings.HasSuffix(got, " stale") {
				t.Fatalf("an edited artifact should read stale, got %q", got)
			}
		})
	}
}

const planNext = "Review it, then approve it again; execute will not start until you do."
const phaseNeutralNext = "Review it, then approve it again; a phase that needs this approval will not start until you do."
const stalePointerText = "approved content not found in the last 20 commits touching the file"

func staleLinePattern(artifact string) *regexp.Regexp {
	return regexp.MustCompile(`^stale: ` + regexp.QuoteMeta(folderX+"/"+artifact) +
		` differs from the content approved (\d{4}-\d\d-\d\d) \(approved text=([0-9a-f]{64}), current text=([0-9a-f]{64})\)\. ` +
		regexp.QuoteMeta(stalePointerText) + `\. (.+)$`)
}

func TestStaleLineFieldsAndPosition(t *testing.T) {
	t.Parallel()
	for _, artifact := range []string{"intent.md", "spec.md", "plan.md"} {
		t.Run(artifact, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\none\n")
			approveGuarded(t, repo, folderX, "decompose", artifact, "approved", "execute go")
			if hasLine(statusOf(t, repo, folderX), "stale:") {
				t.Fatalf("a current approval must not print a stale: line")
			}
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\none\ntwo\n")
			got := statusOf(t, repo, folderX)
			line := lineWith(got, "stale:")
			m := staleLinePattern(artifact).FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("stale: line has the wrong shape: %q", line)
			}
			if m[2] == m[3] {
				t.Errorf("approved and current text hashes should differ, got %q", line)
			}
			if !strings.Contains(lineWith(got, "approvals:"), " approved "+m[1]+" stale") {
				t.Errorf("stale: date %s should match the approvals: line %q", m[1], lineWith(got, "approvals:"))
			}
			ai, si, bi := slices.Index(got, lineWith(got, "approvals:")), slices.Index(got, line), slices.Index(got, "blocks: none")
			if !(ai < si && si < bi) || si != ai+1 {
				t.Errorf("stale: line must sit right after the approvals: lines and before blocks:, got %q", got)
			}
			low := strings.ToLower(line)
			for _, w := range []string{"format", "whitespace", "unchanged", "meaning"} {
				if strings.Contains(low, w) {
					t.Errorf("stale: line must not say why content differs (%q): %q", w, line)
				}
			}
		})
	}
}

func TestStaleLineNextAction(t *testing.T) {
	t.Parallel()
	for artifact, want := range map[string]string{
		"plan.md": planNext, "spec.md": phaseNeutralNext, "intent.md": phaseNeutralNext, "notes.md": phaseNeutralNext,
	} {
		t.Run(artifact, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\none\n")
			approveGuarded(t, repo, folderX, "decompose", artifact, "approved", "execute go")
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\none\ntwo\n")
			line := lineWith(statusOf(t, repo, folderX), "stale:")
			if !strings.HasSuffix(line, ". "+want) {
				t.Fatalf("want next action %q, got %q", want, line)
			}
		})
	}
}

func TestStaleLineMissingArtifactIsUnavailable(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "s\n")
	approveGuarded(t, repo, folderX, "spec", "spec.md", "approved", "decompose go")
	if err := os.Remove(filepath.Join(repo, filepath.FromSlash(folderX+"/spec.md"))); err != nil {
		t.Fatal(err)
	}
	line := lineWith(statusOf(t, repo, folderX), "stale:")
	if !strings.Contains(line, ", current text=unavailable)") {
		t.Fatalf("want current text=unavailable, got %q", line)
	}
}

func TestStaleLineNoneForApprovalsNone(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	logOK(t, repo, folderX, "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	if got := statusOf(t, repo, folderX); hasLine(got, "stale:") {
		t.Fatalf("approvals: none must not print a stale: line, got %q", got)
	}
}

func TestStaleLineUnrecordedEvidence(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "s\n")
	writeState(t, repo, folderX,
		row("2026-10-14T11:00:00Z", "spec", "approve", "spec.md", "ok", "approved", "none", "decompose go"),
	)
	line := lineWith(statusOf(t, repo, folderX), "stale:")
	if !strings.Contains(line, "(approved blob=unrecorded, current blob=") {
		t.Fatalf("want approved blob=unrecorded, got %q", line)
	}
	writeState(t, repo, folderX,
		row("2026-10-14T11:00:00Z", "spec", "approve", "spec.md", "ok", "approved", "blob=", "decompose go"),
	)
	line = lineWith(statusOf(t, repo, folderX), "stale:")
	if !strings.Contains(line, "(approved blob=unrecorded, current blob=") {
		t.Fatalf("an empty blob= should read approved blob=unrecorded, got %q", line)
	}
}

// An approval with no recorded hash has nothing to look for, so status prints
// the not-found pointer without searching history. This test is not parallel
// because statusCommandRun sets GIT_TRACE in the process environment, which
// the Git children of the command inherit.
func TestStaleLineUnrecordedEvidenceRunsNoSearch(t *testing.T) {
	for _, evidence := range []string{"none", "blob="} {
		t.Run(evidence, func(t *testing.T) {
			repo := newRepo(t)
			writeFile(t, repo, folderX+"/spec.md", "s\n")
			commitAll(t, repo, "spec")
			writeState(t, repo, folderX,
				row("2026-10-14T11:00:00Z", "spec", "approve", "spec.md", "ok", "approved", evidence, "decompose go"),
			)
			trace := filepath.Join(t.TempDir(), "trace.txt")
			line := staleLineOf(t, repo, folderX, "spec.md", "GIT_TRACE="+trace)
			if !strings.Contains(line, "(approved blob=unrecorded, current blob=") {
				t.Fatalf("want approved blob=unrecorded, got %q", line)
			}
			wantPointer(t, line, stalePointerText)
			body, err := os.ReadFile(trace)
			if err != nil {
				t.Fatalf("the trace shows whether Git ran: %v", err)
			}
			if strings.Contains(string(body), "--show-prefix") {
				t.Fatalf("an approval with no recorded hash must not search history:\n%s", body)
			}
		})
	}
}

func TestStaleLinesFollowApprovalOrder(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	for _, a := range []string{"spec.md", "intent.md"} {
		writeFile(t, repo, folderX+"/"+a, "one\n")
		approveGuarded(t, repo, folderX, "decompose", a, "approved", "execute go")
		writeFile(t, repo, folderX+"/"+a, "two\n")
	}
	var stale []string
	lastAppr, firstStale, lastStale := -1, -1, -1
	got := statusOf(t, repo, folderX)
	for i, l := range got {
		if strings.HasPrefix(l, "approvals:") {
			lastAppr = i
		}
		if strings.HasPrefix(l, "stale:") {
			stale = append(stale, l)
			if firstStale < 0 {
				firstStale = i
			}
			lastStale = i
		}
	}
	if lastAppr < 0 || lastAppr > firstStale || lastStale+3 >= len(got) || !slices.Equal(got[lastStale+1:lastStale+4], []string{"orders: none", "divergences: none", "blocks: none"}) {
		t.Fatalf("every approvals: line must precede the first stale: line, and the last stale: line must sit directly before the orders:, divergences: and blocks: lines, got %q", got)
	}
	if len(stale) != 2 || !strings.HasPrefix(stale[0], "stale: "+folderX+"/intent.md ") || !strings.HasPrefix(stale[1], "stale: "+folderX+"/spec.md ") {
		t.Fatalf("want intent.md then spec.md stale lines, got %q", stale)
	}
}

func TestStatusThroughDecomposeNamesPhaseNextAndApprovals(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/intent.md", "# Intent\nthe problem\n")
	writeFile(t, repo, folderX+"/spec.md", "# Spec\nthe solution\n")
	writeFile(t, repo, folderX+"/plan.md", "# Plan\nthe tasks\n")
	logOK(t, repo, folderX, "intent", "create", "intent.md", "ok", "capture the problem", "intent.md", "spec define the solution")
	approveGuarded(t, repo, folderX, "intent", "intent.md", "intent approved", "spec define the solution")
	logOK(t, repo, folderX, "spec", "create", "spec.md", "ok", "define the solution", "spec.md", "decompose plan the work")
	approveGuarded(t, repo, folderX, "spec", "spec.md", "spec approved", "decompose plan the work")
	logOK(t, repo, folderX, "decompose", "create", "plan.md", "ok", "plan the work", "plan.md", "decompose approve the plan")
	approveGuarded(t, repo, folderX, "decompose", "plan.md", "plan approved", "execute run the plan")
	got := statusOf(t, repo, folderX)
	for _, want := range []string{"phase: decompose (approve, ok)", "next: execute run the plan"} {
		if !slices.Contains(got, want) {
			t.Errorf("want the line %q in %q", want, got)
		}
	}
	var approvals []string
	for _, l := range got {
		if strings.HasPrefix(l, "approvals:") {
			approvals = append(approvals, l)
		}
	}
	if len(approvals) != 3 {
		t.Fatalf("want 3 approvals: lines, got %q", approvals)
	}
	for i, artifact := range []string{"intent.md", "plan.md", "spec.md"} {
		if !strings.HasPrefix(approvals[i], "approvals: "+artifact+" approved ") || !strings.HasSuffix(approvals[i], " current") {
			t.Errorf("approvals line %d = %q, want %s approved ... current", i, approvals[i], artifact)
		}
	}
	if hasLine(got, "mismatch:") {
		t.Errorf("want no mismatch: line, got %q", got)
	}
}

func TestStatusRevokeRemovesAnApproval(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "s\n")
	approveGuarded(t, repo, folderX, "spec", "spec.md", "approved", "decompose go")
	logOK(t, repo, folderX, "spec", "revoke", "spec.md", "ok", "spec align changed it", "none", "spec go")
	if got := lineWith(statusOf(t, repo, folderX), "approvals:"); got != "approvals: none" {
		t.Fatalf("a revoked approval should not be listed, got %q", got)
	}
}

func TestStatusPlanApprovalIgnoresDivergencesAndStandingOrders(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	plan := "# Plan\n\n## Standing orders\n\n1. never touch x\n\n## Tasks\n\n### Task 1\n\nbody\n\n## Divergences\n\nnone yet\n"
	writeFile(t, repo, folderX+"/plan.md", plan)
	approveBlob(t, repo, folderX, "decompose", "plan.md", "execute go")

	// execute appends a divergence and a standing order: the approval holds.
	grown := strings.Replace(plan, "1. never touch x\n", "1. never touch x\n2. no new deps\n", 1)
	grown = strings.Replace(grown, "none yet\n", "none yet\n\n- task-1: files changed\n", 1)
	writeFile(t, repo, folderX+"/plan.md", grown)
	if got := lineWith(statusOf(t, repo, folderX), "approvals:"); !strings.HasSuffix(got, " unconfirmed here") {
		t.Fatalf("appending to Divergences or Standing orders must keep the hash matching, got %q", got)
	}

	// An edit to a task is a rewrite of what was approved.
	writeFile(t, repo, folderX+"/plan.md", strings.Replace(grown, "body\n", "body changed\n", 1))
	if got := lineWith(statusOf(t, repo, folderX), "approvals:"); !strings.HasSuffix(got, " stale") {
		t.Fatalf("a task edit must make the approval stale, got %q", got)
	}
}

// A CRLF plan keeps its CR bytes in the hash, and its two appended sections
// still drop out, so execute's additions leave the approval's hash matching.
func TestStatusCRLFPlanApprovalIgnoresDivergencesAndStandingOrders(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	plan := strings.ReplaceAll("# Plan\n\n## Standing orders\n\n1. never touch x\n\n## Tasks\n\n### Task 1\n\nbody\n\n## Divergences\n\nnone yet\n", "\n", "\r\n")
	writeFile(t, repo, folderX+"/plan.md", plan)
	approveBlob(t, repo, folderX, "decompose", "plan.md", "execute go")

	grown := strings.Replace(plan, "1. never touch x\r\n", "1. never touch x\r\n2. no new deps\r\n", 1)
	grown = strings.Replace(grown, "none yet\r\n", "none yet\r\n\r\n- task-1: files changed\r\n", 1)
	writeFile(t, repo, folderX+"/plan.md", grown)
	if got := lineWith(statusOf(t, repo, folderX), "approvals:"); !strings.HasSuffix(got, " unconfirmed here") {
		t.Fatalf("appending to Divergences or Standing orders in a CRLF plan must keep the hash matching, got %q", got)
	}

	// The same text with LF endings is a different file: the hash covers the bytes.
	writeFile(t, repo, folderX+"/plan.md", strings.ReplaceAll(grown, "\r\n", "\n"))
	if got := lineWith(statusOf(t, repo, folderX), "approvals:"); !strings.HasSuffix(got, " stale") {
		t.Fatalf("a CRLF to LF rewrite must make the approval stale, got %q", got)
	}
}

func TestStatusIntentAndSpecApprovalIgnoreDivergences(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ artifact, phase string }{{"intent.md", "intent"}, {"spec.md", "spec"}} {
		t.Run(tc.artifact, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			body := "# Doc\n\n## Constraints\n\nkeep it small\n\n## Divergences\n\nnone yet\n"
			writeFile(t, repo, folderX+"/"+tc.artifact, body)
			approveBlob(t, repo, folderX, tc.phase, tc.artifact, "decompose go")

			grown := strings.Replace(body, "none yet\n", "none yet\n\n- **2026-10-22, wave 1, success criterion** — differs. Reason: x.\n", 1)
			writeFile(t, repo, folderX+"/"+tc.artifact, grown)
			if got := lineWith(statusOf(t, repo, folderX), "approvals:"); !strings.HasSuffix(got, " unconfirmed here") {
				t.Fatalf("appending to Divergences must keep the hash matching, got %q", got)
			}

			writeFile(t, repo, folderX+"/"+tc.artifact, grown+"\n## Standing orders\n\n1. x\n")
			if got := lineWith(statusOf(t, repo, folderX), "approvals:"); !strings.HasSuffix(got, " stale") {
				t.Fatalf("Standing orders is a plan-only exclusion, got %q", got)
			}

			writeFile(t, repo, folderX+"/"+tc.artifact, strings.Replace(grown, "keep it small", "keep it large", 1))
			if got := lineWith(statusOf(t, repo, folderX), "approvals:"); !strings.HasSuffix(got, " stale") {
				t.Fatalf("a body edit must make the approval stale, got %q", got)
			}
		})
	}
}

func TestStatusIntentAndSpecCRLFDivergences(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ artifact, phase string }{{"intent.md", "intent"}, {"spec.md", "spec"}} {
		t.Run(tc.artifact, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			body := "# Doc\r\n\r\n## Constraints\r\n\r\nkeep it small\r\n\r\n## Divergences\r\n\r\nnone yet\r\n"
			writeFile(t, repo, folderX+"/"+tc.artifact, body)
			approveBlob(t, repo, folderX, tc.phase, tc.artifact, "decompose go")
			writeFile(t, repo, folderX+"/"+tc.artifact, body+"\r\n- **2026-10-22, wave 1, success criterion** — differs. Reason: x.\r\n")
			if got := lineWith(statusOf(t, repo, folderX), "approvals:"); !strings.HasSuffix(got, " unconfirmed here") {
				t.Fatalf("a CRLF Divergences heading must still be excluded, keeping the hash matching, got %q", got)
			}
		})
	}
}

// Appending the first Divergences section to an approved file adds a blank
// separator line before the new heading. That line must not change the hash,
// including when the approved file already ended in blank lines.
func TestStatusIntentAndSpecNewDivergencesSectionKeepsApproval(t *testing.T) {
	t.Parallel()
	entry := "- **2026-10-22, execute wave 1, spec.md success-criteria** — differs. Reason: x.\n"
	for _, eol := range []string{"\n", "\r\n"} {
		name := "LF"
		if eol == "\r\n" {
			name = "CRLF"
		}
		for _, tail := range []struct{ name, text string }{
			{"final newline", "keep it small\n"},
			{"trailing blank lines", "keep it small\n\n\n"},
		} {
			for _, tc := range []struct{ artifact, phase string }{{"intent.md", "intent"}, {"spec.md", "spec"}} {
				t.Run(tc.artifact+" "+name+" "+tail.name, func(t *testing.T) {
					t.Parallel()
					repo := newRepo(t)
					body := strings.ReplaceAll("# Doc\n\n## Constraints\n\n"+tail.text, "\n", eol)
					writeFile(t, repo, folderX+"/"+tc.artifact, body)
					approveBlob(t, repo, folderX, tc.phase, tc.artifact, "decompose go")

					section := strings.ReplaceAll("\n## Divergences\n\n"+entry, "\n", eol)
					writeFile(t, repo, folderX+"/"+tc.artifact, body+section)
					if got := lineWith(statusOf(t, repo, folderX), "approvals:"); !strings.HasSuffix(got, " unconfirmed here") {
						t.Fatalf("appending a first Divergences section must keep the hash matching, got %q", got)
					}

					edited := strings.Replace(body, "keep it small", "keep it large", 1)
					writeFile(t, repo, folderX+"/"+tc.artifact, edited+section)
					if got := lineWith(statusOf(t, repo, folderX), "approvals:"); !strings.HasSuffix(got, " stale") {
						t.Fatalf("a body edit must make the approval stale, got %q", got)
					}
				})
			}
		}
	}
}

// plan.md keeps the blank line before ## Divergences in its approved text, so
// its hash does not change with the intent and spec rule.
func TestStatusPlanApprovalKeepsBlankLineBeforeDivergences(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	kept := "# Plan\n\n## Tasks\n\nbody\n\n"
	writeFile(t, repo, folderX+"/plan.md", kept+"## Divergences\n\nnone yet\n")
	approveBlob(t, repo, folderX, "decompose", "plan.md", "execute go")

	var blob string
	for _, l := range readLines(t, filepath.Join(repo, folderX, "state.tsv")) {
		for _, c := range cells(l) {
			if strings.HasPrefix(c, "blob=") {
				blob = strings.TrimPrefix(c, "blob=")
			}
		}
	}
	writeFile(t, repo, "kept.txt", kept)
	want := gitOut(t, repo, "hash-object", "--path="+folderX+"/plan.md", "--", "kept.txt")
	if blob != want {
		t.Fatalf("plan.md must hash its kept lines including the blank line before Divergences: blob %q, want %q", blob, want)
	}
}

func TestStatusSurvivesARebaseReplayAndAUnionMerge(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	r1 := row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	r2 := row("2026-10-14T11:00:00Z", "spec", "create", "spec.md", "ok", "spec", "spec.md", "decompose go")
	r3 := row("2026-10-14T12:00:00Z", "decompose", "create", "plan.md", "ok", "plan", "plan.md", "execute go")
	writeState(t, repo, folderX, r1, r2, r3)
	clean := statusOf(t, repo, folderX)

	body := headerRow + "\n" + r1 + "\n" + r3 + "\n" + r2 + "\n" + r3 + "\n" + r1 + "\n" +
		headerRow + "\n" + r2 + "\n"
	writeFile(t, repo, folderX+"/state.tsv", body)
	messy := statusOf(t, repo, folderX)
	if strings.Join(clean, "\n") != strings.Join(messy, "\n") {
		t.Fatalf("a replayed or interleaved file changed the summary:\nclean: %q\nmessy: %q", clean, messy)
	}
	if !strings.HasPrefix(messy[2], "phase: decompose (create, ok)") {
		t.Fatalf("the current row is the greatest ts, got %q", messy[2])
	}
}

func TestStatusReportsAnotherHeaderRow(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	logOK(t, repo, folderX, "intent", "create", "intent.md", "ok", "why", "intent.md", "spec go")
	p := filepath.Join(repo, folderX, "state.tsv")
	body, _ := os.ReadFile(p)
	other := strings.Replace(headerRow, "evidence", "proof", 1)
	if err := os.WriteFile(p, []byte(strings.Replace(string(body), headerRow, other, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	got := statusOf(t, repo, folderX)
	if got[len(got)-1] != "mismatch: header row is '"+other+"', expected '"+headerRow+"'" {
		t.Fatalf("want a header mismatch, got %q", got)
	}
}

// A comment line, such as the version line of an older file, is not the
// header row, and a comment line elsewhere is no row.
func TestStatusSkipsCommentLines(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	r1 := row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	// The comment has nine cells and a second session, so the session count
	// would be two if it were read as a row.
	comment := "#" + strings.Join([]string{"2026-10-14T12:00:00Z", "s2", "spec", "create", "spec.md", "ok", "spec", "spec.md", "decompose go"}, "\t")
	writeFile(t, repo, folderX+"/state.tsv", legacyVersionLine+"\n"+headerRow+"\n"+r1+"\n"+comment+"\n")
	got := statusOf(t, repo, folderX)
	if !strings.HasPrefix(got[2], "phase: intent (create, ok)") || !slices.Contains(got, "sessions: 1") {
		t.Fatalf("the comment line was read as a row: %q", got)
	}
	for _, line := range got {
		if strings.HasPrefix(line, "mismatch:") {
			t.Fatalf("a version line before the header row raised %q", line)
		}
	}
}

func TestStatusWithoutAStateFileExitsThree(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	mustExit(t, runFlow(t, repo, nil, "status", "--folder", "docs/specs/2026-10/none"), 3, "status with no file")
}

func TestStatusIsSilentForASmallFix(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "fix/zero-limit")
	logOK(t, repo, "docs/specs/unassigned", "execute", "skip", "chain fix/zero-limit", "ok", "bug fix: one concern", "base=abc", "execute go")
	res := runFlow(t, repo, nil, "status")
	mustExit(t, res, 0, "status for a small fix")
	if strings.TrimSpace(res.stdout) != "small fix: fix/zero-limit" {
		t.Fatalf("want one small fix line, got %q", res.stdout)
	}
	// A later pivot row ends the small fix: status then has no flow to report.
	logOK(t, repo, "docs/specs/unassigned", "execute", "pivot", "fix fix/zero-limit", "ok", "moved into the chain", "base=abc", "intent go")
	mustExit(t, runFlow(t, repo, nil, "status"), 3, "after the pivot")
}

func TestStatusOnlyAnOkApproveRowApproves(t *testing.T) {
	t.Parallel()
	approveOK := func(hash string) string {
		return row("2026-10-14T11:00:00Z", "spec", "approve", "spec.md", "ok", "approved", "blob="+hash, "decompose go")
	}
	declined := func(status string) string {
		return row("2026-10-14T11:30:00Z", "spec", "approve", "spec.md", status, "user said no", "none", "spec go")
	}
	earlier := func(status string) string {
		return row("2026-10-14T10:30:00Z", "spec", "approve", "spec.md", status, "user said no", "none", "spec go")
	}

	for _, status := range []string{"declined", "failed", "cancelled", "blocked", "pending"} {
		t.Run(status+" alone", func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			writeFile(t, repo, folderX+"/spec.md", "# Spec\n")
			writeState(t, repo, folderX, declined(status))
			if got := lineWith(statusOf(t, repo, folderX), "approvals:"); got != "approvals: none" {
				t.Fatalf("a %s approve row must not read as an approval, got %q", status, got)
			}
		})
	}

	// A fixture row has no ledger line, so the approval it leaves reads
	// unconfirmed here rather than current.
	t.Run("declined after an ok row leaves the approval", func(t *testing.T) {
		t.Parallel()
		repo := newRepo(t)
		writeFile(t, repo, folderX+"/spec.md", "# Spec\n")
		hash := gitOut(t, repo, "hash-object", "--", folderX+"/spec.md")
		writeState(t, repo, folderX, approveOK(hash), declined("declined"))
		want := "approvals: spec.md approved 2026-10-14 unconfirmed here"
		if got := lineWith(statusOf(t, repo, folderX), "approvals:"); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("declined before an ok row changes nothing", func(t *testing.T) {
		t.Parallel()
		repo := newRepo(t)
		writeFile(t, repo, folderX+"/spec.md", "# Spec\n")
		hash := gitOut(t, repo, "hash-object", "--", folderX+"/spec.md")
		writeState(t, repo, folderX, earlier("declined"), approveOK(hash))
		want := "approvals: spec.md approved 2026-10-14 unconfirmed here"
		if got := lineWith(statusOf(t, repo, folderX), "approvals:"); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("a declined row does not hide a revoke", func(t *testing.T) {
		t.Parallel()
		repo := newRepo(t)
		writeFile(t, repo, folderX+"/spec.md", "# Spec\n")
		hash := gitOut(t, repo, "hash-object", "--", folderX+"/spec.md")
		writeState(t, repo, folderX, approveOK(hash),
			row("2026-10-14T11:10:00Z", "spec", "revoke", "spec.md", "ok", "rework", "none", "spec go"),
			declined("declined"))
		if got := lineWith(statusOf(t, repo, folderX), "approvals:"); got != "approvals: none" {
			t.Fatalf("got %q, want none", got)
		}
	})
}

// staleLineOf runs status through the command and returns its stale: line for
// artifact.
func staleLineOf(t *testing.T, dir, folder, artifact string, env ...string) string {
	t.Helper()
	res := statusCommandRun(t, dir, env, "status", "--folder", folder)
	mustExit(t, res, 0, "status")
	if res.stderr != "" {
		t.Fatalf("status stderr: %q", res.stderr)
	}
	for _, l := range strings.Split(res.stdout, "\n") {
		if strings.HasPrefix(l, "stale: ") && strings.Contains(l, "/"+artifact+" differs ") {
			return strings.TrimRight(l, "\r")
		}
	}
	t.Fatalf("no stale: line for %s in:\n%s", artifact, res.stdout)
	return ""
}

// wantPointer fails unless line carries exactly pointer between the blob
// parenthesis and the next action.
func wantPointer(t *testing.T, line, pointer string) {
	t.Helper()
	if !strings.Contains(line, "). "+pointer+". Review it, then approve it again;") {
		t.Fatalf("want pointer %q, got %q", pointer, line)
	}
}

// foundPointer is the pointer flow status prints for a revision it found: the
// short commit and the diff command against rel.
func foundPointer(t *testing.T, repo, sha, rel string) string {
	t.Helper()
	short := gitOut(t, repo, "rev-parse", "--short", sha)
	return "compare with commit " + short + " (from the repository root): git diff " + short + " -- " + rel
}

func approveAuto(t *testing.T, repo, folder, artifact string) {
	t.Helper()
	approveGuarded(t, repo, folder, "decompose", artifact, "approved", "execute go")
}

// guardHash is the hash an auto approval of artifact passes to --expect-blob:
// the one flow status prints for the four chain artifacts, and the
// second-rule hash of any other file. A folder without a state.tsv gets the
// empty file flow log would create, because status reads only an existing one.
func guardHash(t *testing.T, repo, folder, artifact string) string {
	t.Helper()
	if !slices.Contains(textArtifacts, artifact) {
		hash, ok := fileTextHash(filepath.Join(repo, filepath.FromSlash(folder)), artifact)
		if !ok {
			t.Fatalf("no readable %s in %s", artifact, folder)
		}
		return hash
	}
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(folder), "state.tsv")); err != nil {
		writeFile(t, repo, folder+"/state.tsv", stateHeader)
	}
	return statusTextHash(t, repo, folder, artifact)
}

// approveGuarded writes an ok auto approval of artifact through the content
// guard, which stores the artifact's text= hash.
func approveGuarded(t *testing.T, repo, folder, phase, artifact, why, next string) {
	t.Helper()
	args := []string{"log", phase, "approve", artifact, "ok", why, "auto", next, "--expect-blob", guardHash(t, repo, folder, artifact), "--folder", folder}
	mustExit(t, runFlow(t, repo, nil, args...), 0, "approve "+artifact)
}

// approveBlob replaces the folder's state.tsv with one ok approval of artifact
// under the first rule, the blob= row an approval wrote before the second
// rule existed.
func approveBlob(t *testing.T, repo, folder, phase, artifact, next string) {
	t.Helper()
	hash := firstRuleHash(repo, folder, artifact)
	if hash == "" {
		t.Fatalf("no first-rule hash for %s", artifact)
	}
	writeState(t, repo, folder, row("2026-10-14T11:00:00Z", phase, "approve", artifact, "ok", "approved", "blob="+hash, next))
}

func assertRecordedStaleLine(t *testing.T, repo, folder, artifact, keptCurrent, pointer, line string) {
	t.Helper()
	var approved, date string
	for _, raw := range readLines(t, filepath.Join(repo, folder, "state.tsv")) {
		fields := cells(raw)
		if len(fields) >= 9 && fields[3] == "approve" && fields[4] == artifact && fields[5] == "ok" {
			date, _, _ = strings.Cut(fields[0], "T")
			approved = fields[7]
		}
	}
	if approved == "" {
		t.Fatal("missing fixture approval")
	}
	current := "text=" + textHash(artifact, []byte(keptCurrent))
	if !strings.HasPrefix(approved, "text=") {
		current = "blob=" + hashExpected(t, repo, folder+"/"+artifact, keptCurrent)
	}
	next := phaseNeutralNext
	if artifact == "plan.md" {
		next = planNext
	}
	want := "stale: " + folder + "/" + artifact + " differs from the content approved " + date + " (approved " + approved + ", current " + current + "). " + pointer + ". " + next
	if line != want {
		t.Fatalf("stale line = %q, want %q", line, want)
	}
}

func TestStalePointerFoundAtDepthOneCommand(t *testing.T) {
	t.Parallel()
	for _, artifact := range []string{"intent.md", "spec.md", "plan.md", "notes.md"} {
		t.Run(artifact, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\none\n")
			approveBlob(t, repo, folderX, "decompose", artifact, "execute go")
			commitAll(t, repo, "approved")
			sha := gitOut(t, repo, "rev-parse", "HEAD")
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\none\ntwo\n")
			wantPointer(t, staleLineOf(t, repo, folderX, artifact), foundPointer(t, repo, sha, folderX+"/"+artifact))
		})
	}
}

func TestStalePointerFoundAtDepthThreeCommand(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "v1\n")
	approveGuarded(t, repo, folderX, "spec", "spec.md", "approved", "decompose go")
	commitAll(t, repo, "v1")
	sha := gitOut(t, repo, "rev-parse", "HEAD")
	for _, v := range []string{"v2\n", "v3\n"} {
		writeFile(t, repo, folderX+"/spec.md", v)
		commitAll(t, repo, strings.TrimSpace(v))
	}
	writeFile(t, repo, folderX+"/spec.md", "v4\n")
	wantPointer(t, staleLineOf(t, repo, folderX, "spec.md"), foundPointer(t, repo, sha, folderX+"/spec.md"))
}

// The approved text was never committed, but older text of the file was.
func TestStalePointerUncommittedApprovalIsNotFoundCommand(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "v1\n")
	commitAll(t, repo, "v1")
	writeFile(t, repo, folderX+"/spec.md", "v2\n")
	approveGuarded(t, repo, folderX, "spec", "spec.md", "approved", "decompose go")
	writeFile(t, repo, folderX+"/spec.md", "v3\n")
	wantPointer(t, staleLineOf(t, repo, folderX, "spec.md"), stalePointerText)
}

func TestStalePointerNeverCommittedPathIsNotFoundCommand(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "v1\n")
	approveGuarded(t, repo, folderX, "spec", "spec.md", "approved", "decompose go")
	git(t, repo, "add", folderX+"/state.tsv")
	git(t, repo, "commit", "-q", "-m", "state only")
	writeFile(t, repo, folderX+"/spec.md", "v2\n")
	wantPointer(t, staleLineOf(t, repo, folderX, "spec.md"), stalePointerText)
}

// The approval hash leaves out ## Divergences, so the committed revision only
// matches by its filtered hash.
func TestStalePointerMatchesTheFilteredHashCommand(t *testing.T) {
	t.Parallel()
	for _, artifact := range []string{"spec.md", "plan.md"} {
		t.Run(artifact, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\none\n\n## Divergences\n\nd1\n")
			approveGuarded(t, repo, folderX, "decompose", artifact, "approved", "execute go")
			commitAll(t, repo, "approved")
			sha := gitOut(t, repo, "rev-parse", "HEAD")
			raw := gitOut(t, repo, "hash-object", "--", folderX+"/"+artifact)
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\nchanged\n\n## Divergences\n\nd1\n")
			line := staleLineOf(t, repo, folderX, artifact)
			if strings.Contains(line, "approved blob="+raw) {
				t.Fatalf("the approved blob should be the filtered hash, not the raw blob %s: %q", raw, line)
			}
			wantPointer(t, line, foundPointer(t, repo, sha, folderX+"/"+artifact))
		})
	}
}

// A CRLF revision of each filtered artifact, and of a plain file, hashes the
// same in both shells: each finds the commit and prints the same stale: line.
func TestStalePointerCRLFRevisionCommand(t *testing.T) {
	t.Parallel()
	for _, artifact := range []string{"intent.md", "spec.md", "plan.md", "notes.md"} {
		t.Run(artifact, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\r\none\r\n\r\n## Divergences\r\n\r\nd\r\n")
			approveBlob(t, repo, folderX, "decompose", artifact, "execute go")
			commitAll(t, repo, "crlf")
			sha := gitOut(t, repo, "rev-parse", "HEAD")
			writeFile(t, repo, folderX+"/"+artifact, "## Goal\r\ntwo\r\n")
			want := foundPointer(t, repo, sha, folderX+"/"+artifact)
			b := staleLineOf(t, repo, folderX, artifact)
			wantPointer(t, b, want)
			assertRecordedStaleLine(t, repo, folderX, artifact, "## Goal\r\ntwo\r\n", want, b)
		})
	}
}

// The newest revision with the approved hash wins when two revisions share it,
// and a deletion in between leaves no revision to read, so the search goes on.
func TestStalePointerNewestMatchAndDeletedRevisionCommand(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "body\n## Divergences\nfirst\n")
	approveBlob(t, repo, folderX, "decompose", "spec.md", "execute go")
	commitAll(t, repo, "approved")
	writeFile(t, repo, folderX+"/spec.md", "body\n## Divergences\nsecond\n")
	commitAll(t, repo, "same approved content")
	newest := gitOut(t, repo, "rev-parse", "HEAD")
	git(t, repo, "rm", "-q", folderX+"/spec.md")
	commitAll(t, repo, "deleted")
	writeFile(t, repo, folderX+"/spec.md", "changed\n")
	wantPointer(t, staleLineOf(t, repo, folderX, "spec.md"), foundPointer(t, repo, newest, folderX+"/spec.md"))
}

// The history hash applies the Git attributes of the path, so a revision
// committed raw still matches the hash recorded through the clean filter. The
// body has no final newline, so the last record is unterminated.
func TestStalePointerHistoryHashAppliesAttributesCommand(t *testing.T) {
	t.Parallel()
	for _, artifact := range []string{"plan.md", "intent.md", "spec.md", "notes.md"} {
		t.Run(artifact, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			git(t, repo, "config", "filter.approval.clean", "sed s/raw/clean/g")
			git(t, repo, "config", "filter.approval.required", "true")
			writeFile(t, repo, ".gitattributes", "*.md filter=approval\n")
			writeFile(t, repo, folderX+"/"+artifact, "raw")
			approveBlob(t, repo, folderX, "decompose", artifact, "execute go")
			// Keep the committed blob raw, so only the history hash's attribute
			// lookup can reproduce the approved clean form.
			git(t, repo, "-c", "filter.approval.clean=cat", "add", "-A")
			git(t, repo, "commit", "-q", "-m", "approved")
			sha := gitOut(t, repo, "rev-parse", "HEAD")
			writeFile(t, repo, folderX+"/"+artifact, "changed")
			wantPointer(t, staleLineOf(t, repo, folderX, artifact), foundPointer(t, repo, sha, folderX+"/"+artifact))
		})
	}
}

// A Git failure leaves hashing working, so only the search falls back.
func TestStalePointerGitFailureIsNotFoundCommand(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "v1\n")
	approveGuarded(t, repo, folderX, "spec", "spec.md", "approved", "decompose go")
	commitAll(t, repo, "v1")
	sha := gitOut(t, repo, "rev-parse", "HEAD")
	writeFile(t, repo, folderX+"/spec.md", "v2\n")
	wantPointer(t, staleLineOf(t, repo, folderX, "spec.md"), foundPointer(t, repo, sha, folderX+"/spec.md"))
	line := staleLineOf(t, repo, folderX, "spec.md", "GIT_DIR="+filepath.Join(t.TempDir(), "missing"))
	wantPointer(t, line, stalePointerText)
	if !regexp.MustCompile(`, current text=[0-9a-f]{64}\)`).MatchString(line) {
		t.Fatalf("the current text hash should still be a hash: %q", line)
	}
	// The first rule hashes through Git as well, so its current hash
	// must survive the same failure.
	blobRepo := newRepo(t)
	writeFile(t, blobRepo, folderX+"/spec.md", "v1\n")
	approveBlob(t, blobRepo, folderX, "spec", "spec.md", "decompose go")
	commitAll(t, blobRepo, "v1")
	blobSha := gitOut(t, blobRepo, "rev-parse", "HEAD")
	writeFile(t, blobRepo, folderX+"/spec.md", "v2\n")
	wantPointer(t, staleLineOf(t, blobRepo, folderX, "spec.md"), foundPointer(t, blobRepo, blobSha, folderX+"/spec.md"))
	line = staleLineOf(t, blobRepo, folderX, "spec.md", "GIT_DIR="+filepath.Join(t.TempDir(), "missing"))
	wantPointer(t, line, stalePointerText)
	if !regexp.MustCompile(`, current blob=([0-9a-f]{40}|[0-9a-f]{64})\)`).MatchString(line) {
		t.Fatalf("the current blob hash should still be a hash: %q", line)
	}
}

// The printed path is repository-root relative whatever form --folder takes.
func TestStalePointerPathIsRootRelativeForEveryFolderFormCommand(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "v1\n")
	approveGuarded(t, repo, folderX, "spec", "spec.md", "approved", "decompose go")
	commitAll(t, repo, "v1")
	sha := gitOut(t, repo, "rev-parse", "HEAD")
	writeFile(t, repo, folderX+"/spec.md", "v2\n")
	want := foundPointer(t, repo, sha, folderX+"/spec.md")
	sub := filepath.Join(repo, "docs")
	// The first rule hashes the current file through Git, whose paths depend
	// on the folder form, so a blob= approval in a second repository covers
	// that hash.
	blobRepo := newRepo(t)
	writeFile(t, blobRepo, folderX+"/spec.md", "v1\n")
	approveBlob(t, blobRepo, folderX, "spec", "spec.md", "decompose go")
	commitAll(t, blobRepo, "v1")
	blobSha := gitOut(t, blobRepo, "rev-parse", "HEAD")
	writeFile(t, blobRepo, folderX+"/spec.md", "v2\n")
	blobRootLine := staleLineOf(t, blobRepo, folderX, "spec.md")
	wantPointer(t, blobRootLine, foundPointer(t, blobRepo, blobSha, folderX+"/spec.md"))
	for _, form := range []struct{ dir, folder string }{
		{repo, folderX},
		{repo, filepath.ToSlash(filepath.Join(repo, folderX))},
		{sub, folderX},
		{filepath.Join(repo, folderX), filepath.ToSlash(filepath.Join(repo, folderX))},
	} {
		// The subtests share one repository, so they run in order.
		t.Run(form.folder, func(t *testing.T) {
			wantPointer(t, staleLineOf(t, form.dir, form.folder, "spec.md"), want)
			blobDir := strings.Replace(form.dir, repo, blobRepo, 1)
			blobFolder := strings.Replace(form.folder, filepath.ToSlash(repo), filepath.ToSlash(blobRepo), 1)
			if got := staleLineOf(t, blobDir, blobFolder, "spec.md"); got != blobRootLine {
				t.Fatalf("stale line for folder form %+v:\n got %q\nwant %q", form, got, blobRootLine)
			}
		})
	}
}

// The search reads only: the index, the work tree and HEAD stay as they were,
// and a current approval runs no search at all.
func TestStalePointerSearchChangesNothingCommand(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "v1\n")
	approveGuarded(t, repo, folderX, "spec", "spec.md", "approved", "decompose go")
	commitAll(t, repo, "v1")
	trace := filepath.Join(t.TempDir(), "trace.txt")
	res := statusCommandRun(t, repo, []string{"GIT_TRACE=" + trace}, "status", "--folder", folderX)
	mustExit(t, res, 0, "status")
	body, _ := os.ReadFile(trace)
	if strings.Contains(string(body), "--show-prefix") {
		t.Fatalf("a current approval must not search history:\n%s", body)
	}
	writeFile(t, repo, folderX+"/spec.md", "v2\n")
	// Snapshot the index after git status, which may itself refresh a
	// racily clean index.
	porcelain, head := gitOut(t, repo, "status", "--porcelain", "--untracked-files=all"), gitOut(t, repo, "rev-parse", "HEAD")
	index := filepath.Join(repo, ".git", "index")
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	res = statusCommandRun(t, repo, []string{"GIT_TRACE=" + trace}, "status", "--folder", folderX)
	mustExit(t, res, 0, "status")
	if body, _ = os.ReadFile(trace); !strings.Contains(string(body), "--show-prefix") {
		t.Fatalf("a stale approval should search history:\n%s", body)
	}
	after, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("the search changed the index")
	}
	if p, h := gitOut(t, repo, "status", "--porcelain", "--untracked-files=all"), gitOut(t, repo, "rev-parse", "HEAD"); p != porcelain || h != head {
		t.Fatalf("the search changed the repository: %q %s, was %q %s", p, h, porcelain, head)
	}
}

// boundRepo makes 21 commits touching spec.md, approves the content of the
// approveAt-th commit (1 is the oldest), then edits the file. It returns the
// repository and the approved commit.
func boundRepo(t *testing.T, approveAt int) (string, string) {
	t.Helper()
	repo := newRepo(t)
	sha := ""
	for i := 1; i <= 21; i++ {
		writeFile(t, repo, folderX+"/spec.md", "v"+strconv.Itoa(i)+"\n")
		if i == approveAt {
			approveAuto(t, repo, folderX, "spec.md")
		}
		commitAll(t, repo, "v"+strconv.Itoa(i))
		if i == approveAt {
			sha = gitOut(t, repo, "rev-parse", "HEAD")
		}
	}
	writeFile(t, repo, folderX+"/spec.md", "v22\n")
	return repo, sha
}

func TestStalePointerBoundCommand(t *testing.T) {
	t.Parallel()
	t.Run("20th newest is found", func(t *testing.T) {
		repo, sha := boundRepo(t, 2)
		wantPointer(t, staleLineOf(t, repo, folderX, "spec.md"), foundPointer(t, repo, sha, folderX+"/spec.md"))
	})
	t.Run("21st newest is not found", func(t *testing.T) {
		repo, _ := boundRepo(t, 1)
		wantPointer(t, staleLineOf(t, repo, folderX, "spec.md"), stalePointerText)
	})
}

// The reapproval chain walks approve, commit, edit, then re-approves with the
// text hash status prints, which the stale: line repeats.
func TestStalePointerReapprovalChainCommand(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "## Goal\none\n")
	approveAuto(t, repo, folderX, "spec.md")
	commitAll(t, repo, "approved")
	sha := gitOut(t, repo, "rev-parse", "HEAD")
	writeFile(t, repo, folderX+"/spec.md", "## Goal\none\ntwo\n")
	line := staleLineOf(t, repo, folderX, "spec.md")
	wantPointer(t, line, foundPointer(t, repo, sha, folderX+"/spec.md"))
	hash := statusTextHash(t, repo, folderX, "spec.md")
	if !strings.Contains(line, ", current text="+hash+")") {
		t.Fatalf("the stale: line should print the current text hash %s: %q", hash, line)
	}
	state := filepath.Join(repo, filepath.FromSlash(folderX+"/state.tsv"))
	rows := len(readLines(t, state))
	reapprove := func(hash string) result {
		return runFlow(t, repo, nil, "log", "spec", "approve", "spec.md", "ok", "reapproved", "auto", "decompose go", "--expect-blob", hash, "--folder", folderX)
	}
	wrong := hash[:len(hash)-1] + "z"
	mustExit(t, reapprove(wrong), 2, "guarded approve with a wrong hash")
	if n := len(readLines(t, state)); n != rows {
		t.Fatalf("a rejected guarded approve wrote a row: %d rows, was %d", n, rows)
	}
	mustExit(t, reapprove(hash), 0, "guarded approve with the printed hash")
	if n := len(readLines(t, state)); n != rows+1 {
		t.Fatalf("want one new row, got %d rows, was %d", n, rows)
	}
	res := runFlow(t, repo, nil, "status", "--folder", folderX)
	mustExit(t, res, 0, "status")
	if !strings.Contains(res.stdout, "approvals: spec.md approved ") || !strings.Contains(res.stdout, " current\n") || strings.Contains(res.stdout, "stale:") {
		t.Fatalf("the re-approval should read current:\n%s", res.stdout)
	}
}

// The full stale: lines are byte-identical across shells, found and not found,
// for every target kind, LF and CRLF.
func TestPowerShellStalePointerParityCommand(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	for _, a := range []string{"plan.md", "spec.md", "intent.md", "notes.md"} {
		writeFile(t, repo, folderX+"/"+a, "## Goal\none\n\n## Divergences\n\nd\n")
		writeFile(t, repo, folderX+"/crlf-"+a, "## Goal\r\none\r\n")
	}
	for _, a := range []string{"plan.md", "spec.md", "intent.md", "notes.md"} {
		approveGuarded(t, repo, folderX, "decompose", a, "approved", "execute go")
		approveGuarded(t, repo, folderX, "decompose", "crlf-"+a, "approved", "execute go")
	}
	commitAll(t, repo, "approved")
	writeFile(t, repo, folderX+"/uncommitted.md", "u1\n")
	approveGuarded(t, repo, folderX, "decompose", "uncommitted.md", "approved", "execute go")
	for _, a := range []string{"plan.md", "spec.md", "intent.md", "notes.md"} {
		writeFile(t, repo, folderX+"/"+a, "## Goal\ntwo\n")
		writeFile(t, repo, folderX+"/crlf-"+a, "## Goal\r\ntwo\r\n")
	}
	writeFile(t, repo, folderX+"/uncommitted.md", "u2\n")
	b := statusCommandRun(t, repo, nil, "status", "--folder", folderX)
	mustExit(t, b, 0, "original-case")
	if n := strings.Count(b.stdout, "(from the repository root): git diff "); n != 8 {
		t.Fatalf("want 8 found pointers from flow.sh, got %d:\n%s", n, b.stdout)
	}
	if n := strings.Count(b.stdout, stalePointerText); n != 1 {
		t.Fatalf("want 1 not-found pointer from flow.sh, got %d:\n%s", n, b.stdout)
	}
	for _, artifact := range []string{"plan.md", "spec.md", "intent.md", "notes.md", "crlf-plan.md", "crlf-spec.md", "crlf-intent.md", "crlf-notes.md", "uncommitted.md"} {
		body := "## Goal\ntwo\n"
		pointer := foundPointer(t, repo, "HEAD", folderX+"/"+artifact)
		if strings.HasPrefix(artifact, "crlf-") {
			body = "## Goal\r\ntwo\r\n"
		}
		if artifact == "uncommitted.md" {
			body = "u2\n"
			pointer = stalePointerText
		}
		line := lineWith(strings.Split(b.stdout, "\n"), "stale: "+folderX+"/"+artifact+" ")
		assertRecordedStaleLine(t, repo, folderX, artifact, body, pointer, line)
	}
}

// quotedPointerCaseCommand approves and commits an artifact under a folder whose name
// needs quoting, edits it, and checks the printed git diff command: the path
// token is exactly wantToken, and Git from the repository root shows the patch.
func quotedPointerCaseCommand(t *testing.T, name, wantToken string) {
	t.Helper()
	repo := newRepo(t)
	folder := "docs/specs/2026-10/" + name
	if err := os.MkdirAll(filepath.Join(repo, filepath.FromSlash(folder)), 0o755); err != nil {
		t.Skipf("the filesystem refuses the folder name %q: %v", name, err)
	}
	writeFile(t, repo, folder+"/spec.md", "## Goal\none\n")
	approveGuarded(t, repo, folder, "spec", "spec.md", "approved", "decompose go")
	commitAll(t, repo, "approved")
	sha := gitOut(t, repo, "rev-parse", "HEAD")
	short := gitOut(t, repo, "rev-parse", "--short", sha)
	writeFile(t, repo, folder+"/spec.md", "## Goal\none\ntwo\n")
	line := staleLineOf(t, repo, folder, "spec.md")
	prefix := "compare with commit " + short + " (from the repository root): "
	cmd := "git diff " + short + " -- " + wantToken
	wantPointer(t, line, prefix+cmd)
	assertRecordedStaleLine(t, repo, folder, "spec.md", "## Goal\none\ntwo\n", prefix+cmd, line)
	out := git(t, repo, "diff", short, "--", folder+"/spec.md")
	if !strings.Contains(string(out), "+two") {
		t.Fatalf("the printed command %q did not show the patch:\n%s", cmd, out)
	}
}

func TestStalePointerQuotedSpaceCommand(t *testing.T) {
	t.Parallel()
	quotedPointerCaseCommand(t, "work item", "'docs/specs/2026-10/work item/spec.md'")
}

func TestStalePointerQuotedEmbeddedSingleQuoteCommand(t *testing.T) {
	t.Parallel()
	quotedPointerCaseCommand(t, "it's here", `'docs/specs/2026-10/it'\''s here/spec.md'`)
}

// A path of only the safe characters is printed as it always was. Windows
// refuses a colon in a folder name, so only the row without one runs there.
func TestStalePointerQuotedSafeCharactersStayBareCommand(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ label, name string }{
		{"with a colon", "a@b%c+d=e:f,g-h_1.2"},
		{"without a colon", "a@b%c+d=ef,g-h_1.2"},
	} {
		t.Run(c.label, func(t *testing.T) {
			t.Parallel()
			quotedPointerCaseCommand(t, c.name, "docs/specs/2026-10/"+c.name+"/spec.md")
		})
	}
}

// Git children inherit process variables; isolate each command's explicit Git settings.
func statusCommandRun(t *testing.T, dir string, env []string, args ...string) result {
	t.Helper()
	for _, setting := range env {
		key, value, _ := strings.Cut(setting, "=")
		if strings.HasPrefix(key, "GIT_") {
			old, exists := os.LookupEnv(key)
			if err := os.Setenv(key, value); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if exists {
					_ = os.Setenv(key, old)
				} else {
					_ = os.Unsetenv(key)
				}
			}()
		}
	}
	return runFlow(t, dir, env, args...)
}

func TestStatusStaleLineParity(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	for _, a := range []string{"plan.md", "spec.md", "intent.md", "notes.md"} {
		writeFile(t, repo, folderX+"/"+a, "## Goal\none\n")
		approveGuarded(t, repo, folderX, "decompose", a, "approved", "execute go")
		writeFile(t, repo, folderX+"/"+a, "## Goal\none\ntwo\n")
	}
	writeFile(t, repo, folderX+"/gone.md", "x\n")
	approveGuarded(t, repo, folderX, "decompose", "gone.md", "approved", "execute go")
	if err := os.Remove(filepath.Join(repo, filepath.FromSlash(folderX+"/gone.md"))); err != nil {
		t.Fatal(err)
	}
	// Approve rows whose evidence is not blob=<hash>, appended as raw rows.
	statePath := filepath.Join(repo, filepath.FromSlash(folderX+"/state.tsv"))
	f, err := os.OpenFile(statePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range []string{
		row("2026-10-14T11:00:00Z", "decompose", "approve", "none-evidence.md", "ok", "approved", "none", "execute go"),
		row("2026-10-14T11:01:00Z", "decompose", "approve", "empty-blob.md", "ok", "approved", "blob=", "execute go"),
	} {
		if _, err := f.WriteString(r + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	b := runFlow(t, repo, nil, "status", "--folder", folderX)
	mustExit(t, b, 0, "original-case")
	if n := strings.Count(b.stdout, "\nstale: "); n != 7 {
		t.Fatalf("want 7 stale: lines from flow.sh, got %d:\n%s", n, b.stdout)
	}
	if n := strings.Count(b.stdout, "(approved blob=unrecorded, current blob=unavailable)"); n != 2 {
		t.Fatalf("want 2 unrecorded stale: lines from flow.sh, got %d:\n%s", n, b.stdout)
	}
}

func TestStatusParityDeclinedApprovals(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/plan.md", "# Plan\n\n## Tasks\n\nbody\n")
	hash := gitOut(t, repo, "hash-object", "--", folderX+"/plan.md")
	for name, rows := range map[string][]string{
		"declined alone": {
			row("2026-10-14T12:00:00Z", "decompose", "create", "plan.md", "ok", "plan", "plan.md", "execute go"),
			row("2026-10-14T12:10:00Z", "decompose", "approve", "plan.md", "declined", "not yet", "none", "decompose go"),
			row("2026-10-14T12:20:00Z", "spec", "approve", "spec.md", "declined", "not yet", "none", "spec go"),
			row("2026-10-14T12:30:00Z", "intent", "approve", "intent.md", "failed", "no hash", "none", "spec go"),
			row("2026-10-14T13:00:00Z", "gate", "branch", "feat/x", "ok", "PR opened", "#12", "done"),
		},
		"declined then ok": {
			row("2026-10-14T12:00:00Z", "decompose", "create", "plan.md", "ok", "plan", "plan.md", "execute go"),
			row("2026-10-14T12:10:00Z", "decompose", "approve", "plan.md", "declined", "not yet", "none", "decompose go"),
			row("2026-10-14T12:30:00Z", "decompose", "approve", "plan.md", "ok", "approved", "blob="+hash, "execute go"),
			row("2026-10-14T13:00:00Z", "gate", "branch", "feat/x", "ok", "PR opened", "#12", "done"),
		},
		"ok then declined": {
			row("2026-10-14T12:00:00Z", "decompose", "create", "plan.md", "ok", "plan", "plan.md", "execute go"),
			row("2026-10-14T12:30:00Z", "decompose", "approve", "plan.md", "ok", "approved", "blob="+hash, "execute go"),
			row("2026-10-14T12:40:00Z", "decompose", "approve", "plan.md", "declined", "not yet", "none", "decompose go"),
			row("2026-10-14T13:00:00Z", "gate", "branch", "feat/x", "ok", "PR opened", "#12", "done"),
		},
	} {
		// The subtests share one repository and rewrite its state.tsv, so they run in order.
		t.Run(name, func(t *testing.T) {
			writeState(t, repo, folderX, rows...)
			b := runFlow(t, repo, nil, "status", "--folder", folderX, "--metrics")
			mustExit(t, b, 0, "original-case")
			if name != "declined alone" && !strings.Contains(b.stdout, "approvals: plan.md approved 2026-10-14 unconfirmed here\n") {
				t.Fatalf("missing the plan approval, unconfirmed here for a fixture row: %s", b.stdout)
			}
			if name == "declined alone" && !strings.Contains(b.stdout, "approvals: none") {
				t.Fatalf("flow.sh lists an approval for declined rows:\n%s", b.stdout)
			}
			wantVerdict := "plan-to-verdict: 1800s"
			if name == "declined alone" {
				wantVerdict = "plan-to-verdict: n/a"
			}
			for _, line := range []string{"time-to-intent: n/a", "intent-to-spec: n/a", "spec-rework: 0", wantVerdict, "reviews: none", "review-time: latency=0s blocked=0s", "escapes: 0"} {
				if !strings.Contains(b.stdout, line+"\n") {
					t.Fatalf("declined metrics missing %q: %s", line, b.stdout)
				}
			}
		})
	}
}

func rowFor(session, ts, phase, event, target, status, why, evidence, next string) string {
	return strings.Join([]string{ts, session, phase, event, target, status, why, evidence, next}, "\t")
}

func TestStatusParityLetterCase(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	head := gitOut(t, repo, "rev-parse", "HEAD")
	writeState(t, repo, folderX,
		rowFor("ses", "2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"),
		rowFor("SES", "2026-10-14T10:01:00Z", "execute", "block", "Task-1", "blocked", "waiting", "none", "execute go"),
		rowFor("ses", "2026-10-14T10:02:00Z", "execute", "unblock", "task-1", "ok", "free", "none", "execute go"),
		rowFor("ses", "2026-10-14T10:03:00Z", "spec", "approve", "Spec.md", "ok", "approved", "blob=aaaa", "decompose go"),
		rowFor("ses", "2026-10-14T10:04:00Z", "spec", "revoke", "spec.md", "ok", "rework", "none", "spec go"),
		rowFor("ses", "2026-10-14T10:05:00Z", "execute", "BLOCK", "Zed", "blocked", "waiting", "none", "execute go"),
		rowFor("ses", "2026-10-14T10:06:00Z", "execute", "enter", "run", "ok", "start", "MODE=x mode=pair mode=solo branch=p branch=q", "execute go"),
		rowFor("ses", "2026-10-14T10:07:00Z", "execute", "verify", "Task-1", "ok", "v", "commit=0000000000000000000000000000000000000001 patch-id="+pid40, "execute go"),
		rowFor("ses", "2026-10-14T10:08:00Z", "execute", "verify", "task-1", "ok", "v", "commit="+head+" patch-id="+pid40, "execute go"),
		// A verify cell refuses a repeated key and an uppercase key, so the
		// last-wins and case-sensitive-key readings are kept on diverge rows,
		// whose cells status does not parse.
		rowFor("ses", "2026-10-14T10:08:20Z", "execute", "diverge", "task-2", "ok", "d", "commit=0000000000000000000000000000000000000002 commit=0000000000000000000000000000000000000003 patch-id=cc patch-id=dd", "execute go"),
		rowFor("ses", "2026-10-14T10:08:40Z", "execute", "diverge", "task-3", "ok", "d", "COMMIT=0000000000000000000000000000000000000004 patch-id=ee", "execute go"),
		rowFor("ses", "2026-10-14T10:09:00Z", "execute", "verify", "task-2", "ok", "v", "commit=0000000000000000000000000000000000000002 commit=0000000000000000000000000000000000000003 patch-id=cc patch-id=dd", "execute go"),
		rowFor("ses", "2026-10-14T10:10:00Z", "execute", "verify", "task-3", "ok", "v", "COMMIT=0000000000000000000000000000000000000004 patch-id=ee", "execute go"),
	)
	commitAll(t, repo, "flow")
	git(t, repo, "switch", "-q", "-c", "q")
	b := runFlow(t, repo, nil, "status", "--folder", folderX)
	mustExit(t, b, 0, "original-case")
	for _, want := range []string{
		"blocks: Task-1 blocked since 2026-10-14", "mode: solo (q)",
		"mismatch: verify Task-1 commit 0000000000000000000000000000000000000001 is not an ancestor of HEAD and patch-id " + pid40 + " matches no commit on this branch",
		"mismatch: diverge task-2 commit 0000000000000000000000000000000000000003 is not an ancestor of HEAD and patch-id dd matches no commit on this branch",
		"invalid: 2026-10-14T10:09:00Z ses verify task-2: key commit repeats",
		"invalid: 2026-10-14T10:10:00Z ses verify task-3: COMMIT=0000000000000000000000000000000000000004 is not a key=value pair",
		"sessions: 2", "approvals: Spec.md approved 2026-10-14 stale", "phase: execute (verify, ok)", "next: execute go", "last: 2026-10-14T10:10:00Z ses",
	} {
		if !strings.Contains(b.stdout, want) {
			t.Fatalf("flow.sh output lacks %q, so the fixture does not test the case rule:\n%s", want, b.stdout)
		}
	}
	// The invalid verify rows are no evidence, so neither prints a mismatch:
	// line; the diverge task-3 row names no commit= key, so it prints none.
	if strings.Count(b.stdout, "invalid:") != 2 || strings.Contains(b.stdout, "mismatch: verify task-") || strings.Contains(b.stdout, "mismatch: diverge task-3") {
		t.Fatalf("invalid rows reached the commit check: %s", b.stdout)
	}
	if strings.Count(b.stdout, "mismatch:") != 2 {
		t.Fatalf("case-sensitive audit count: %s", b.stdout)
	}
}

func TestStatusParitySummary(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, ".gitattributes", "docs/specs/**/state.tsv merge=union\n")
	writeFile(t, repo, folderX+"/spec.md", "# Spec\n")
	writeFile(t, repo, folderX+"/plan.md", "# Plan\n\n## Tasks\n\nbody\n")
	hash := gitOut(t, repo, "hash-object", "--", folderX+"/spec.md")
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"),
		row("2026-10-14T10:10:00Z", "intent", "approve", "intent.md", "ok", "approved", "blob=aaaa", "spec go"),
		row("2026-10-14T11:00:00Z", "spec", "approve", "spec.md", "ok", "approved", "blob="+hash, "decompose go"),
		row("2026-10-14T12:00:00Z", "decompose", "create", "plan.md", "ok", "plan", "plan.md", "execute go"),
		row("2026-10-14T12:30:00Z", "spec", "revoke", "spec.md", "ok", "rework", "none", "spec go"),
		row("2026-10-14T12:40:00Z", "spec", "approve", "spec.md", "ok", "re-approved", "blob="+hash, "decompose go"),
		row("2026-10-14T13:00:00Z", "decompose", "approve", "plan.md", "ok", "approved", "blob=dddd", "execute go"),
		row("2026-10-14T13:05:00Z", "execute", "enter", "run", "ok", "start", "mode=pair branch=feat/x", "execute go"),
		row("2026-10-14T13:10:00Z", "execute", "block", "task-3", "blocked", "waiting", "none", "execute go"),
		row("2026-10-14T13:20:00Z", "execute", "verify", "task-1", "ok", "v", "commit=0000000000000000000000000000000000000009 patch-id=eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "execute go"),
		row("2026-10-14T13:00:00Z", "decompose", "approve", "plan.md", "ok", "approved", "blob=dddd", "execute go"),
		row("2026-10-14T13:30:00Z", "intent", "bind", "main", "ok", "bound", "none", "execute go"),
	)
	commitAll(t, repo, "flow")
	git(t, repo, "switch", "-q", "-c", "feat/x")

	got := statusOf(t, repo, folderX)
	planHash := gitOut(t, repo, "hash-object", "--", folderX+"/plan.md")
	want := []string{
		"flow: " + folderX, "storage: committed", "phase: intent (bind, ok)", "next: execute go",
		textHashLine("spec.md", "# Spec\n"),
		textHashLine("plan.md", "# Plan\n\n## Tasks\n\nbody\n"),
		"approvals: intent.md approved 2026-10-14 stale",
		"approvals: plan.md approved 2026-10-14 stale",
		"approvals: spec.md approved 2026-10-14 unconfirmed here",
		"stale: " + folderX + "/intent.md differs from the content approved 2026-10-14 (approved blob=aaaa, current blob=unavailable). " + stalePointerText + ". " + phaseNeutralNext,
		"stale: " + folderX + "/plan.md differs from the content approved 2026-10-14 (approved blob=dddd, current blob=" + planHash + "). " + stalePointerText + ". " + planNext,
		"unconfirmed: " + folderX + "/spec.md was approved 2026-10-14" + unconfirmedTail,
		"orders: none", "divergences: none",
		"blocks: task-3 blocked since 2026-10-14", "mode: pair (feat/x)", "slice: not bound", "unaccounted: not computed (this branch is not bound to the flow)", "sessions: 1", "last: 2026-10-14T13:30:00Z s1",
		"mismatch: verify task-1 commit 0000000000000000000000000000000000000009 is not an ancestor of HEAD and patch-id eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee matches no commit on this branch",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	withMetrics := statusOf(t, repo, folderX, "--metrics")
	expectedMetrics := []string{"time-to-intent: 600s", "intent-to-spec: 3000s", "spec-rework: 2", "plan-to-verdict: n/a", "reviews: none", "review-time: latency=0s blocked=0s", "escapes: 0"}
	expected := append(append(append([]string{}, want[:len(want)-1]...), expectedMetrics...), want[len(want)-1])
	if !slices.Equal(withMetrics, expected) {
		t.Fatalf("complete parity metrics=%q, want %q", withMetrics, expected)
	}
	git(t, repo, "switch", "-q", "-c", "feat/y")
	seedFlow(t, repo, flowB, "")
	commitAll(t, repo, "b on branch")
	git(t, repo, "switch", "-q", "-c", "fix/q")
	logOK(t, repo, "docs/specs/unassigned", "execute", "skip", "chain fix/q", "ok", "bug fix: one concern", "base=abc", "execute go")
	res := runFlow(t, repo, nil, "status")
	mustExit(t, res, 0, "small-fix status")
	if res.stdout != "small fix: fix/q\n" || res.stderr != "" {
		t.Fatalf("small fix: %+v", res)
	}
}

// sliceLine returns the status line that starts with slice:, and fails unless
// there is exactly one and it sits between the blocks: or mode: line and the
// unaccounted: line.
func sliceLine(t *testing.T, lines []string) string {
	t.Helper()
	at := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, "slice:") })
	count := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "slice:") {
			count++
		}
	}
	if count != 1 || at < 1 || at+1 >= len(lines) || !strings.HasPrefix(lines[at+1], "unaccounted:") ||
		!(strings.HasPrefix(lines[at-1], "mode:") || strings.HasPrefix(lines[at-1], "blocks:")) {
		t.Fatalf("want one slice: line after the blocks: or mode: line and before unaccounted:, got %q", lines)
	}
	return lines[at]
}

func TestStatusSliceFollowsTheLatestBindRow(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "feat/s")
	seedFlow(t, repo, flowA, "")
	logOK(t, repo, flowA, "execute", "enter", "run", "ok", "start", "mode=solo branch=feat/s", "execute go")
	commitAll(t, repo, "a")
	if got := sliceLine(t, statusOf(t, repo, flowA)); got != "slice: not bound" {
		t.Fatalf("unbound feature branch: %q", got)
	}
	mustExit(t, bindAt(t, repo, "--folder", flowA), 0, "plain bind")
	if got := sliceLine(t, statusOf(t, repo, flowA)); got != "slice: all (feat/s)" {
		t.Fatalf("plain bind: %q", got)
	}
	mustExit(t, bindAt(t, repo, "--folder", flowA, "--slice", "S1,S2"), 0, "re-bind with slices")
	if got := sliceLine(t, statusOf(t, repo, flowA)); got != "slice: S1,S2 (feat/s)" {
		t.Fatalf("re-bind with slices: %q", got)
	}
	// Another branch's bind row says nothing about this branch.
	logOK(t, repo, flowA, "execute", "bind", "feat/other", "ok", "bound", "slice=S9", "execute go")
	if got := sliceLine(t, statusOf(t, repo, flowA)); got != "slice: S1,S2 (feat/s)" {
		t.Fatalf("another branch's bind row: %q", got)
	}
	// A later hand-written bind row for this branch decides.
	logOK(t, repo, flowA, "execute", "bind", "feat/s", "ok", "bound", "slice=S3", "execute go")
	if got := sliceLine(t, statusOf(t, repo, flowA)); got != "slice: S3 (feat/s)" {
		t.Fatalf("latest bind row: %q", got)
	}
}

func TestStatusSliceAfterBindSlice(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "feat/t")
	seedFlow(t, repo, flowA, "")
	commitAll(t, repo, "a")
	mustExit(t, bindAt(t, repo, "--folder", flowA, "--slice", "S2"), 0, "bind S2")
	if got := sliceLine(t, statusOf(t, repo, flowA)); got != "slice: S2 (feat/t)" {
		t.Fatalf("bind --slice S2: %q", got)
	}
}

func TestStatusSliceNotBoundWithoutAFeatureBranch(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	// Bind refuses the default branch, so only a hand-written row binds it. That
	// row still leaves the slice: line not bound, while unaccounted: counts the
	// branch as bound and computes its range.
	seedFlow(t, repo, flowA, "")
	logOK(t, repo, flowA, "execute", "bind", "main", "ok", "bound", "slice=S1", "execute go")
	commitAll(t, repo, "a")
	got := statusOf(t, repo, flowA)
	if line := sliceLine(t, got); line != "slice: not bound" {
		t.Fatalf("default branch with a bind row: %q", line)
	}
	if line := lineWith(got, "unaccounted:"); line != "unaccounted: none" {
		t.Fatalf("default branch with a bind row: want unaccounted: computed, got %q", line)
	}
	// The bind row stays in the state file, and a detached HEAD has no branch
	// for it to name.
	git(t, repo, "checkout", "-q", "--detach")
	if line := sliceLine(t, statusOf(t, repo, flowA)); line != "slice: not bound" {
		t.Fatalf("detached HEAD: %q", line)
	}
}

func TestStatusRelativeFolderInOpenSession(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	for _, f := range []string{"sub/fa", "sub/fb"} {
		writeFile(t, repo, f+"/spec.md", "# Spec\n")
	}
	hash := gitOut(t, repo, "hash-object", "--", "sub/fa/spec.md")
	head := gitOut(t, repo, "rev-parse", "HEAD")
	writeState(t, repo, "sub/fa",
		row("2026-10-14T10:00:00Z", "spec", "approve", "spec.md", "ok", "approved", "blob="+hash, "decompose go"),
		row("2026-10-14T10:01:00Z", "execute", "verify", "task-1", "ok", "v", "commit="+head+" patch-id=bb", "execute go"),
	)
	commitAll(t, repo, "flow")
	sub := filepath.Join(repo, "sub")

	b := runFlow(t, sub, nil, "status", "--folder", "sub/fa")
	mustExit(t, b, 0, "original-case")
	if !strings.Contains(b.stdout, "spec.md approved 2026-10-14 unconfirmed here\n") || !strings.Contains(b.stdout, "\nunconfirmed: sub/fa/spec.md was approved 2026-10-14"+unconfirmedTail+"\n") {
		t.Fatalf("the fixture approval's hash should match, unconfirmed here:\n%s", b.stdout)
	}
}

func TestStatusEmptyRowsAndHeaderBytes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ body, header string }{
		{headerRow + "\n", headerRow},
		{legacyVersionLine + "\n" + headerRow + "\n", headerRow},
		{legacyVersionLine + "\nnot a header\nshort\trow\n", "not a header"},
		{"", ""},
		{legacyVersionLine + "\n", ""},
		{legacyVersionLine + "\r\n" + headerRow + "\r\n", headerRow + "\r"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFile(t, dir, "rel/state.tsv", tc.body)
			got := runFlow(t, dir, nil, "status", "--folder", "rel")
			mustExit(t, got, 0, "empty status")
			want := "flow: rel\nstorage: committed\nphase: none (no rows)\nnext: none\napprovals: none\norders: none\ndivergences: none\nblocks: none\nslice: not bound\nunaccounted: not computed (this branch is not bound to the flow)\nsessions: 0\n"
			if tc.header != headerRow {
				want += "mismatch: header row is '" + tc.header + "', expected '" + headerRow + "'\n"
			}
			if got.stdout != want || got.stderr != "" {
				t.Fatalf("status = %+v, want stdout %q", got, want)
			}
		})
	}
}

func TestStatusUsageAndMissingState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, args := range [][]string{{"--folder"}, {"--bogus"}, {"--folder", "rel", "extra"}, {"--metrics", "extra"}} {
		res := runFlow(t, dir, nil, append([]string{"status"}, args...)...)
		if res.code != 1 || res.stdout != "" || res.stderr != usage {
			t.Fatalf("args %q: %+v", args, res)
		}
	}
	for _, directory := range []bool{false, true} {
		if directory {
			if err := os.MkdirAll(filepath.Join(dir, "rel", "state.tsv"), 0755); err != nil {
				t.Fatal(err)
			}
		}
		res := runFlow(t, dir, nil, "status", "--folder", "rel")
		if res.code != 3 || res.stdout != "" || res.stderr != "flow: no state.tsv in rel\n" {
			t.Fatalf("missing regular state: %+v", res)
		}
	}
	res := runFlow(t, dir, nil, "status")
	if res.code != 3 || res.stdout != "" || res.stderr != "flow: no flow found; ask which flow, or pass --folder\n" {
		t.Fatalf("no repository: %+v", res)
	}
}

func TestStatusLookupAndLastFolderOption(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "feat/status")
	seedFlow(t, repo, flowA, "feat/status")
	seedFlow(t, repo, flowB, "")
	res := runFlow(t, repo, nil, "status")
	mustExit(t, res, 0, "bound status")
	if !strings.HasPrefix(res.stdout, "flow: "+flowA+"\n") || res.stderr != "" {
		t.Fatalf("resolved folder: %+v", res)
	}
	got := statusOf(t, repo, "missing", "--folder", flowA)
	if got[0] != "flow: "+flowA {
		t.Fatalf("last folder must win: %q", got)
	}
	git(t, repo, "switch", "-q", "-c", "feat/ambiguous")
	seedFlow(t, repo, (flowA + "-third"), "")
	res = runFlow(t, repo, nil, "status")
	assertLookupAmbiguous(t, res, flowA+"-third", flowA, flowB)
}

func TestStatusGitExecutableLoss(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "spec\n")
	approveGuarded(t, repo, folderX, "spec", "spec.md", "approved", "decompose go")
	before, err := os.ReadFile(filepath.Join(repo, folderX, "state.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	res := runFlow(t, repo, nil, "status", "--folder", folderX)
	mustExit(t, res, 0, "explicit state without Git")
	// The text= hash runs no Git, so the hash still matches; without Git the
	// ledger cannot be found, so the approval reads unconfirmed here.
	if !strings.Contains(res.stdout, "\napprovals: spec.md approved ") || !strings.Contains(res.stdout, " unconfirmed here\n") || !strings.Contains(res.stdout, "\nunconfirmed: "+folderX+"/spec.md was approved ") || strings.Contains(res.stdout, "stale:") || res.stderr != "" {
		t.Fatalf("missing Git: %+v", res)
	}
	res = runFlow(t, repo, nil, "status")
	if res.code != 3 || res.stdout != "" || res.stderr != "flow: no flow found; ask which flow, or pass --folder\n" {
		t.Fatalf("missing Git lookup: %+v", res)
	}
	after, err := os.ReadFile(filepath.Join(repo, folderX, "state.tsv"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("status changed state: %v", err)
	}
}

// A blob= approval hashes with Git, so without Git its current hash is
// unavailable and the search for the approved commit finds nothing, even
// though the approved text is committed.
func TestStatusGitExecutableLossBlobApproval(t *testing.T) {
	repo := newRepo(t)
	writeFile(t, repo, folderX+"/spec.md", "spec\n")
	approveBlob(t, repo, folderX, "spec", "spec.md", "decompose go")
	commitAll(t, repo, "approved")
	hash := firstRuleHash(repo, folderX, "spec.md")
	t.Setenv("PATH", t.TempDir())
	res := runFlow(t, repo, nil, "status", "--folder", folderX)
	mustExit(t, res, 0, "explicit state without Git")
	lines := strings.Split(strings.TrimRight(res.stdout, "\n"), "\n")
	want := "stale: " + folderX + "/spec.md differs from the content approved 2026-10-14 (approved blob=" + hash + ", current blob=unavailable). " + stalePointerText + ". " + phaseNeutralNext
	if !slices.Contains(lines, "approvals: spec.md approved 2026-10-14 stale") || !slices.Contains(lines, want) || res.stderr != "" {
		t.Fatalf("missing Git: want the approval stale with the line %q: %+v", want, res)
	}
}

func TestStatusByteOrderAndExactCase(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeState(t, repo, folderX,
		rowFor("ses", "2026-10-14T10:00:00Z", "spec", "approve", "z.md", "ok", "approved", "none", "spec go"),
		rowFor("SES", "2026-10-14T10:00:00Z", "spec", "approve", "a.md", "ok", "approved", "blob=", "spec go"),
		rowFor("ses", "2026-10-14T10:00:00Z", "spec", "approve", "Z.md", "ok", "approved", "blob=aaaa", "spec go"),
		rowFor("ses", "2026-10-14T10:00:00Z", "spec", "REVOKE", "Z.md", "ok", "case", "none", "spec go"),
		rowFor("ses", "2026-10-14T10:00:00Z", "spec", "APPROVE", "A.md", "ok", "case", "none", "spec go"),
		rowFor("ses", "2026-10-14T10:00:00Z", "spec", "approve", "B.md", "OK", "case", "none", "spec go"),
	)
	got := statusOf(t, repo, folderX)
	want := []string{"flow: " + folderX, "storage: committed", "phase: spec (approve, OK)", "next: spec go"}
	for _, target := range []string{"Z.md", "a.md", "z.md"} {
		want = append(want, "approvals: "+target+" approved 2026-10-14 stale")
	}
	for _, target := range []string{"Z.md", "a.md", "z.md"} {
		approved := "blob=unrecorded"
		if target == "Z.md" {
			approved = "blob=aaaa"
		}
		want = append(want, "stale: "+folderX+"/"+target+" differs from the content approved 2026-10-14 (approved "+approved+", current blob=unavailable). "+stalePointerText+". "+phaseNeutralNext)
	}
	want = append(want, "orders: none", "divergences: none", "blocks: none", "slice: not bound", "unaccounted: not computed (this branch is not bound to the flow)", "sessions: 2", "last: 2026-10-14T10:00:00Z ses")
	if !slices.Equal(got, want) {
		t.Fatalf("byte/case ordering: %q, want %q", got, want)
	}
}

func TestStatusSparseRowsKeepShellFieldAssignment(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeState(t, dir, "rel", row("2026-10-14T10:00:00Z", "", "pause", "spec.md", "pending", "why", "none", "spec go"))
	want := []string{"flow: rel", "storage: committed", "phase: pause (pending, spec go)", "next: ", "approvals: none", "orders: none", "divergences: none", "blocks: none", "slice: not bound", "unaccounted: not computed (this branch is not bound to the flow)", "sessions: 1", "last: 2026-10-14T10:00:00Z s1"}
	if got := statusOf(t, dir, "rel"); !slices.Equal(got, want) {
		t.Fatalf("sparse status = %q, want %q", got, want)
	}
	writeState(t, dir, "rel", row("2026-10-14T10:00:00Z", "spec", "approve", "", "ok", "why", "blob=aaaa", "spec go"))
	wantApproval := "approvals: 2026-10-14T10:00:00Z approved blob=aaaa stale"
	if got := lineWith(statusOf(t, dir, "rel"), "approvals:"); got != wantApproval {
		t.Fatalf("empty target = %q, want %q", got, wantApproval)
	}
}

func TestStatusBlocksModeAndMetrics(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"),
		row("2026-10-14T10:10:00Z", "intent", "approve", "intent.md", "ok", "approved", "blob=aaaa", "spec go"),
		row("2026-10-14T11:00:00Z", "spec", "approve", "spec.md", "ok", "approved", "blob=bbbb", "decompose go"),
		row("2026-10-14T12:00:00Z", "decompose", "create", "plan.md", "ok", "plan", "plan.md", "execute go"),
		row("2026-10-14T12:30:00Z", "spec", "revoke", "spec.md", "ok", "rework", "none", "spec go"),
		row("2026-10-14T12:40:00Z", "spec", "approve", "spec.md", "ok", "re-approved", "blob=cccc", "decompose go"),
		row("2026-10-14T13:00:00Z", "decompose", "approve", "plan.md", "ok", "approved", "blob=dddd", "execute go"),
		row("2026-10-14T13:05:00Z", "execute", "enter", "run", "ok", "start", "mode=auto branch=feat/x", "execute go"),
		row("2026-10-14T13:10:00Z", "execute", "block", "task-3", "blocked", "waiting", "none", "execute go"),
		row("2026-10-14T15:00:00Z", "gate", "branch", "feat/x", "ok", "PR opened", "#12", "done"),
	)
	git(t, repo, "switch", "-q", "-c", "feat/x")
	got := statusOf(t, repo, folderX, "--metrics")
	if l := lineWith(got, "blocks:"); !strings.HasPrefix(l, "blocks: task-3 blocked since 2026-10-14") {
		t.Fatalf("want an open block, got %q", l)
	}
	if l := lineWith(got, "mode:"); l != "mode: auto (feat/x)" {
		t.Fatalf("mode line %q", l)
	}
	want := map[string]string{
		"time-to-intent:":  "time-to-intent: 600s",
		"intent-to-spec:":  "intent-to-spec: 3000s",
		"spec-rework:":     "spec-rework: 2",
		"plan-to-verdict:": "plan-to-verdict: 7200s",
	}
	for prefix, line := range want {
		if l := lineWith(got, prefix); l != line {
			t.Errorf("%s got %q, want %q", prefix, l, line)
		}
	}
	// A trail with no plan reads n/a for the measures that need one.
	repo2 := newRepo(t)
	writeState(t, repo2, folderX, row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"))
	g2 := statusOf(t, repo2, folderX, "--metrics")
	for _, p := range []string{"time-to-intent: n/a", "intent-to-spec: n/a", "spec-rework: n/a", "plan-to-verdict: n/a"} {
		if !hasLine(g2, p) {
			t.Errorf("missing %q in %q", p, g2)
		}
	}
}

// TestStatusSurvivesARebaseReplayAndAUnionMerge pins the set semantics: an
// identical row repeated, rows out of ts order, and a repeated header pair
// change nothing a reader sees.

func TestStatusCommitRows(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "feat/orig")
	writeFile(t, repo, "src/a.txt", "one\n")
	commitAll(t, repo, "task commit")
	head := gitOut(t, repo, "rev-parse", "HEAD")
	id := patchIDOf(repo, head)

	// 1. An ancestor of HEAD passes.
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "execute", "verify", "task-1", "ok", "v", "commit="+head+" patch-id="+id, "execute go"))
	if hasLine(statusOf(t, repo, folderX), "mismatch:") {
		t.Fatal("an ancestor commit must not be a mismatch")
	}

	// 2. A commit that is no ancestor but whose patch-id matches a branch
	//    commit (a rebase moved it) passes. The branch is cut from main, so
	//    its commit is main..HEAD.
	git(t, repo, "switch", "-q", "main")
	git(t, repo, "switch", "-q", "-c", "feat/y")
	git(t, repo, "cherry-pick", head)
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "execute", "verify", "task-1", "ok", "v", "commit=0000000000000000000000000000000000000009 patch-id="+id, "execute go"))
	if hasLine(statusOf(t, repo, folderX), "mismatch:") {
		t.Fatal("a rebased commit with a matching patch-id must not be a mismatch")
	}

	// 3. A commit that is no ancestor and whose patch-id matches nothing is a
	//    mismatch, for a verify row and for a diverge row alike.
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "execute", "verify", "task-1", "ok", "v", "commit=0000000000000000000000000000000000000009 patch-id=deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "execute go"),
		row("2026-10-14T10:05:00Z", "execute", "diverge", "task-2 files", "ok", "d", "commit=0000000000000000000000000000000000000008 patch-id=cafebabe", "execute go"))
	got := statusOf(t, repo, folderX)
	if n := strings.Count(strings.Join(got, "\n"), "mismatch:"); n != 2 {
		t.Fatalf("want two mismatch lines, got %d: %q", n, got)
	}

	// 4. Rows older than the latest gate verdict are history: a squash merge of
	//    an earlier slice leaves their commits off this branch.
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "execute", "verify", "task-1", "ok", "v", "commit=0000000000000000000000000000000000000009 patch-id=deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "execute go"),
		row("2026-10-14T11:00:00Z", "gate", "branch", "feat/old", "ok", "PR", "#1", "done"))
	if hasLine(statusOf(t, repo, folderX), "mismatch:") {
		t.Fatal("rows older than the latest verdict must not be checked")
	}

	// 5. A wave row carries no patch-id and is checked by ancestry alone: its
	//    mismatch line has no patch-id part.
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "execute", "verify", "wave-1", "ok", "v", "commit=0000000000000000000000000000000000000009", "execute go"))
	const wave = "mismatch: verify wave-1 commit 0000000000000000000000000000000000000009 is not an ancestor of HEAD"
	if got := lineWith(statusOf(t, repo, folderX), "mismatch:"); got != wave {
		t.Fatalf("want %q, got %q", wave, got)
	}

	// 6. A wave row whose commit is an ancestor of HEAD passes.
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "execute", "verify", "wave-1", "ok", "v", "commit="+gitOut(t, repo, "rev-parse", "HEAD"), "execute go"))
	if hasLine(statusOf(t, repo, folderX), "mismatch:") {
		t.Fatal("a wave row naming an ancestor of HEAD must not be a mismatch")
	}
}

// Rows share a ts when they land in the same second. File order breaks that
// tie, so a commit row written after the verdict row in the same second is
// newer than the verdict and is audited, while one written before it is
// history. Both shells print the same.

func TestStatusAuditsACommitRowAtTheVerdictTimestampByFileOrder(t *testing.T) {
	t.Parallel()
	const ts = "2026-10-14T11:00:00Z"
	verdict := row(ts, "gate", "branch", "feat/old", "ok", "PR", "#1", "done")
	wrong := row(ts, "execute", "verify", "task-1", "ok", "v", "commit=0000000000000000000000000000000000000009 patch-id=deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "execute go")
	const mismatch = "mismatch: verify task-1 commit 0000000000000000000000000000000000000009 is not an ancestor of HEAD and patch-id deadbeefdeadbeefdeadbeefdeadbeefdeadbeef matches no commit on this branch"
	for name, tc := range map[string]struct {
		rows      []string
		wantAudit bool
	}{
		"a row after the verdict at the same ts is audited":      {[]string{verdict, wrong}, true},
		"a row before the verdict at the same ts is not audited": {[]string{wrong, verdict}, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			writeState(t, repo, folderX, tc.rows...)
			args := []string{"status", "--folder", folderX}
			b := runFlow(t, repo, nil, args...)
			mustExit(t, b, 0, "bash status")
			lines := strings.Split(strings.TrimRight(b.stdout, "\n"), "\n")
			if tc.wantAudit {
				if got := lineWith(lines, "mismatch:"); got != mismatch {
					t.Fatalf("want %q, got %q in %q", mismatch, got, lines)
				}
			} else if hasLine(lines, "mismatch:") {
				t.Fatalf("a row before the verdict is history, got %q", lines)
			}
		})
	}
}

func TestStatusRebindRowRepairsAMismatch(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, "src/a.txt", "one\n")
	commitAll(t, repo, "task commit")
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "execute", "verify", "task-1", "ok", "v", "commit=0000000000000000000000000000000000000009 patch-id=deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "execute go"))
	if !hasLine(statusOf(t, repo, folderX), "mismatch:") {
		t.Fatal("setup: want a mismatch before the rebind")
	}
	logOK(t, repo, folderX, "execute", "verify", "task-1", "ok", "rebound after rebase", "auto", "execute go")
	if hasLine(statusOf(t, repo, folderX), "mismatch:") {
		t.Fatal("a new verify row naming the new commit must repair the mismatch")
	}
}

func TestStatusCountsAReplayedRowOnce(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	plan := row("2026-10-14T12:00:00Z", "decompose", "create", "plan.md", "ok", "plan", "plan.md", "execute go")
	rework := row("2026-10-14T12:40:00Z", "spec", "approve", "spec.md", "ok", "re-approved", "blob=cccc", "decompose go")
	body := headerRow + "\n" + plan + "\n" + rework + "\n" + rework + "\n"
	writeFile(t, repo, folderX+"/state.tsv", body)
	if got := lineWith(statusOf(t, repo, folderX, "--metrics"), "spec-rework:"); got != "spec-rework: 1" {
		t.Fatalf("a replayed row must count once, got %q", got)
	}
}

// TestStatusOnlyAnOkApproveRowApproves pins that an approve row with any other
// status records a refusal or an unfinished step and approves nothing.

func TestStatusMetricsSkipADeclinedApproveRow(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"),
		row("2026-10-14T10:10:00Z", "intent", "approve", "intent.md", "declined", "not yet", "none", "intent go"),
		row("2026-10-14T10:30:00Z", "intent", "approve", "intent.md", "ok", "approved", "blob=aaaa", "spec go"),
		row("2026-10-14T11:00:00Z", "spec", "approve", "spec.md", "declined", "not yet", "none", "spec go"),
		row("2026-10-14T11:20:00Z", "spec", "approve", "spec.md", "ok", "approved", "blob=bbbb", "decompose go"),
		row("2026-10-14T12:00:00Z", "decompose", "create", "plan.md", "ok", "plan", "plan.md", "execute go"),
		row("2026-10-14T12:10:00Z", "decompose", "approve", "plan.md", "declined", "not yet", "none", "decompose go"),
		row("2026-10-14T12:20:00Z", "spec", "approve", "spec.md", "declined", "not yet", "none", "spec go"),
		row("2026-10-14T13:00:00Z", "decompose", "approve", "plan.md", "ok", "approved", "blob=dddd", "execute go"),
		row("2026-10-14T15:00:00Z", "gate", "branch", "feat/x", "ok", "PR opened", "#12", "done"),
	)
	got := statusOf(t, repo, folderX, "--metrics")
	want := map[string]string{
		"time-to-intent:":  "time-to-intent: 1800s",
		"intent-to-spec:":  "intent-to-spec: 3000s",
		"spec-rework:":     "spec-rework: 0",
		"plan-to-verdict:": "plan-to-verdict: 7200s",
	}
	for prefix, line := range want {
		if l := lineWith(got, prefix); l != line {
			t.Errorf("%s got %q, want %q", prefix, l, line)
		}
	}
}

func TestStatusMetricsReportReviewsAndEscapes(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	quality := row("2026-10-14T12:10:00Z", "execute", "review", "group-a", "failed", "findings", "kind=quality dispatched=2026-10-14T12:00:00Z round=1 blocked=100", "execute go")
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"),
		row("2026-10-14T10:30:00Z", "decompose", "review", "plan", "ok", "cleared", "kind=plan dispatched=2026-10-14T10:00:00Z round=1 blocked=1800", "execute go"),
		quality,
		quality, // a rebase replay: counted once
		row("2026-10-14T12:20:00Z", "execute", "review", "group-a", "ok", "cleared", "kind=quality dispatched=2026-10-14T12:15:00Z round=2 blocked=0", "execute go"),
		row("2026-10-14T13:00:00Z", "execute", "review", "final", "ok", "cleared", "kind=integration dispatched=2026-10-14T13:05:00Z round=1 blocked=0", "execute go"),
		row("2026-10-14T14:00:00Z", "gate", "escape", "task-2", "failed", "missed nil check", "found-by=gate severity=important review=quality", "execute go"),
	)
	got := statusOf(t, repo, folderX, "--metrics")
	want := []string{
		"reviews: plan=1 quality=2 integration=1",
		"review-time: latency=2700s blocked=1900s",
		"escapes: 1",
	}
	assertAfterPlanToVerdict(t, got, want)

	// A spec review is counted under its own kind, and its optional pairs change no measure.
	logOK(t, repo, folderX, "spec", "review", "spec.md", "ok", "cleared", "kind=spec dispatched=2099-01-01T00:00:00Z round=1 blocked=0 rev=2 by=independent commit=0123abc", "decompose go")
	assertAfterPlanToVerdict(t, statusOf(t, repo, folderX, "--metrics"), []string{
		"reviews: plan=1 quality=2 integration=1 spec=1",
		"review-time: latency=2700s blocked=1900s",
		"escapes: 1",
	})

	repo2 := newRepo(t)
	writeState(t, repo2, folderX, row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"))
	assertAfterPlanToVerdict(t, statusOf(t, repo2, folderX, "--metrics"), []string{
		"reviews: none",
		"review-time: latency=0s blocked=0s",
		"escapes: 0",
	})
}

// assertAfterPlanToVerdict checks that want follows the plan-to-verdict line.

func assertAfterPlanToVerdict(t *testing.T, got, want []string) {
	t.Helper()
	at := -1
	for i, l := range got {
		if strings.HasPrefix(l, "plan-to-verdict:") {
			at = i
		}
	}
	if at < 0 || at+len(want) >= len(got) {
		t.Fatalf("no plan-to-verdict line followed by %d lines in %q", len(want), got)
	}
	for i, w := range want {
		if l := got[at+1+i]; l != w {
			t.Errorf("line %d after plan-to-verdict is %q, want %q", i+1, l, w)
		}
	}
}

func TestRebaseReplayOfARepeatedRowLeavesTheFlowIntact(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, ".gitattributes", "docs/specs/**/state.tsv merge=union\n")
	r1 := row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	r2 := row("2026-10-14T11:00:00Z", "spec", "approve", "spec.md", "ok", "approved", "blob=abc", "decompose go")
	rParent := row("2026-10-14T12:00:00Z", "decompose", "enter", "plan.md", "ok", "parent row", "none", "decompose go")
	rChild := row("2026-10-14T10:30:00Z", "decompose", "create", "plan.md", "ok", "child row", "plan.md", "execute go")
	writeState(t, repo, flowA, r1)
	commitAll(t, repo, "base")

	git(t, repo, "switch", "-q", "-c", "parent")
	writeState(t, repo, flowA, r1, r2, rParent)
	commitAll(t, repo, "parent adds r2 and its own row")

	git(t, repo, "switch", "-q", "main")
	git(t, repo, "switch", "-q", "-c", "child")
	writeState(t, repo, flowA, r1, rChild, r2)
	commitAll(t, repo, "child adds its own row and the same r2")

	git(t, repo, "rebase", "-q", "parent")
	if got := git(t, repo, "log", "--format=%s", "parent..child"); strings.TrimSpace(got) != "child adds its own row and the same r2" {
		t.Fatalf("the rebase must keep the child commit, got log %q", got)
	}

	file := filepath.Join(repo, flowA, "state.tsv")
	occurrences := 0
	for _, line := range readLines(t, file) {
		if line == r2 {
			occurrences++
		}
	}
	if occurrences != 2 {
		t.Fatalf("the rebase must leave r2 twice in state.tsv, found %d:\n%s", occurrences, strings.Join(readLines(t, file), "\n"))
	}

	// The same rows, once each and in ts order, in a file no rebase touched.
	// Only --metrics counts rows one by one: spec-rework is the number of
	// spec.md approve rows after the plan was created, and r2 is that one row.
	ref := newRepo(t)
	writeState(t, ref, flowA, r1, rChild, r2, rParent)
	for _, extra := range [][]string{nil, {"--metrics"}} {
		want := statusOf(t, ref, flowA, extra...)
		got := statusOf(t, repo, flowA, extra...)
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Fatalf("a replayed row changed the summary (flags %v):\nreplayed:   %q\nunreplayed: %q", extra, got, want)
		}
	}
	if !hasLine(statusOf(t, repo, flowA, "--metrics"), "spec-rework: 1") {
		t.Fatalf("r2 must count once for spec-rework: %q", statusOf(t, repo, flowA, "--metrics"))
	}
}

// TestWhereSquashMergedBranchWithABindAndNoDoneRowResolvesToNothing covers a
// branch whose flow was never closed with a done row. Its bind row names the
// branch, so it matches only that branch. A new branch cut from the default
// branch after the squash merge has no bind, no changed state file, and sees
// an active flow that is bound, which lookup step 3 must exclude.

func TestStatusReviewMetricsReferenceCorners(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go"),
		row("2026-10-14T10:30:00Z", "decompose", "review", "plan", "ok", "cleared", "kind=plan dispatched=2026-10-14T10:00:00Z round=1 blocked=1800", "execute go"),
		row("2026-10-14T11:00:00Z", "execute", "review", "task-1", "ok", "cleared", "kind=packet dispatched=2026-10-14T10:50:00Z round=1 blocked=5", "execute go"),
		row("2026-10-14T12:10:00Z", "execute", "review", "group-a", "failed", "findings", "kind=quality dispatched=2026-10-14T12:00:00Z round=1 blocked=100", "execute go"),
		row("2026-10-14T12:10:00Z", "execute", "review", "group-a", "failed", "findings", "kind=quality dispatched=2026-10-14T12:00:00Z round=1 blocked=100", "execute go"),
		row("2026-10-14T13:00:00Z", "execute", "review", "final", "ok", "cleared", "kind=integration dispatched=2026-10-14T13:05:00Z round=1 blocked=0", "execute go"),
		row("2026-10-14T14:00:00Z", "gate", "escape", "task-2", "failed", "missed nil check", "found-by=gate severity=important review=quality", "execute go"),
		row("2026-10-14T15:00:00Z", "execute", "review", "odd", "ok", "cleared", "kind=quality dispatched=2026-13-40T25:61:61Z round=1 blocked=7", "execute go"),
		row("2027-01-15T10:00:00Z", "execute", "review", "early", "ok", "cleared", "kind=packet dispatched=2026-12-40T09:00:00Z round=1 blocked=3", "execute go"),
	)
	got := statusOf(t, repo, folderX, "--metrics")
	assertAfterPlanToVerdict(t, got, []string{"reviews: plan=1 packet=2 quality=2 integration=1", "review-time: latency=525000s blocked=1915s", "escapes: 1"})
	huge := "kind=quality dispatched=2026-10-14T12:00:00Z round=1 blocked=9223372036854775807"
	writeState(t, repo, folderX, row("2026-10-14T12:10:00Z", "execute", "review", "a", "ok", "cleared", huge, "execute go"), row("2026-10-14T12:20:00Z", "execute", "review", "b", "ok", "cleared", huge, "execute go"))
	got = statusOf(t, repo, folderX, "--metrics")
	if !slices.Contains(got, "reviews: quality=2") || !strings.HasPrefix(lineWith(got, "review-time:"), "review-time: latency=1800s blocked=") {
		t.Fatalf("overflowing sum lost metrics: %q", got)
	}
}

func TestStatusAuditLatestRowsAndEveryVerdictKind(t *testing.T) {
	t.Parallel()
	const ts = "2026-10-14T12:00:00Z"
	wrong := row(ts, "execute", "verify", "task-1", "ok", "verify", "commit=0000000000000000000000000000000000000009 patch-id="+pid40, "execute go")
	for _, event := range []string{"branch", "release", "change"} {
		t.Run(event, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			verdict := row(ts, "gate", event, "target", "ok", "verdict", "none", "execute go")
			writeState(t, repo, folderX, wrong, verdict)
			if hasLine(statusOf(t, repo, folderX), "mismatch:") {
				t.Fatal("pre-verdict row must be history")
			}
			writeState(t, repo, folderX, verdict, wrong)
			if !hasLine(statusOf(t, repo, folderX), "mismatch: verify task-1 commit 0000000000000000000000000000000000000009 ") {
				t.Fatal("post-verdict row must be audited")
			}
			writeState(t, repo, folderX, verdict, wrong, row(ts, "execute", "verify", "task-1", "failed", "supersedes", "cause=repair", "execute go"))
			if hasLine(statusOf(t, repo, folderX), "mismatch:") {
				t.Fatal("latest event/target without commit evidence must replace earlier audit")
			}
		})
	}
}

func TestStatusBlocksAndModeLastMatch(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "-c", "two")
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "execute", "block", "z", "failed", "block", "none", "execute go"),
		row("2026-10-14T10:01:00Z", "execute", "block", "a", "blocked", "block", "none", "execute go"),
		row("2026-10-14T10:02:00Z", "execute", "unblock", "z", "failed", "unblock", "none", "execute go"),
		row("2026-10-14T10:03:00Z", "execute", "enter", "other", "ok", "not mode", "mode=auto branch=two", "execute go"),
		row("2026-10-14T10:04:00Z", "execute", "enter", "run", "ok", "mode", "MODE=x mode=auto mode=pair branch=one branch=two", "execute go"))
	got := statusOf(t, repo, folderX)
	if lineWith(got, "blocks:") != "blocks: a blocked since 2026-10-14" || lineWith(got, "mode:") != "mode: pair (two)" || strings.Count(strings.Join(got, "\n"), "blocks:") != 1 {
		t.Fatalf("block/mode reduction: %q", got)
	}
	writeState(t, repo, folderX, row("2026-10-14T10:00:00Z", "execute", "enter", "run", "ok", "mode", "other=value branch=two", "execute go"))
	if lineWith(statusOf(t, repo, folderX), "mode:") != "mode: unknown (two)" {
		t.Fatal("mode fallbacks missing")
	}
	writeState(t, repo, folderX, row("2026-10-14T10:00:00Z", "execute", "enter", "run", "ok", "mode", "mode=pair", "execute go"))
	if hasLine(statusOf(t, repo, folderX), "mode:") {
		t.Fatal("a run row that names no branch must print no mode line")
	}
}

func TestStatusMetricsFirstEventsAndStrictReworkTime(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "first", "none", "execute go"),
		row("2026-10-14T10:01:00Z", "intent", "create", "intent.md", "ok", "later", "none", "execute go"),
		row("2026-10-14T10:02:00Z", "intent", "approve", "intent.md", "ok", "first", "none", "execute go"),
		row("2026-10-14T10:03:00Z", "intent", "approve", "intent.md", "ok", "later", "none", "execute go"),
		row("2026-10-14T10:04:00Z", "spec", "approve", "spec.md", "ok", "first", "none", "execute go"),
		row("2026-10-14T10:05:00Z", "spec", "approve", "spec.md", "ok", "later", "none", "execute go"),
		row("2026-10-14T10:06:00Z", "decompose", "create", "plan.md", "ok", "first", "none", "execute go"),
		row("2026-10-14T10:06:00Z", "spec", "revoke", "spec.md", "ok", "equal time", "none", "execute go"),
		row("2026-10-14T10:07:00Z", "spec", "approve", "spec.md", "declined", "ignored", "none", "execute go"),
		row("2026-10-14T10:07:30Z", "spec", "revoke", "spec.md", "ok", "between plan creations", "none", "execute go"),
		row("2026-10-14T10:08:00Z", "decompose", "create", "plan.md", "ok", "later", "none", "execute go"),
		row("2026-10-14T10:09:00Z", "decompose", "approve", "plan.md", "ok", "first", "none", "execute go"),
		row("2026-10-14T10:10:00Z", "decompose", "approve", "plan.md", "ok", "later", "none", "execute go"),
		row("2026-10-14T10:11:00Z", "gate", "branch", "x", "ok", "first", "none", "done"),
		row("2026-10-14T10:12:00Z", "gate", "branch", "x", "ok", "later", "none", "done"))
	got := statusOf(t, repo, folderX, "--metrics")
	for _, want := range []string{"time-to-intent: 120s", "intent-to-spec: 120s", "spec-rework: 1", "plan-to-verdict: 120s"} {
		if !slices.Contains(got, want) {
			t.Fatalf("first/strict metrics lacks %s: %q", want, got)
		}
	}
}

func TestStatusReviewCalendarAndNumericOutput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, start, end, want string }{
		{"seconds", "2026-10-14T10:00:59Z", "2026-10-14T10:02:00Z", "61"},
		{"year boundary", "2019-12-31T23:59:00Z", "2020-01-01T00:01:00Z", "120"},
		{"leap day", "2024-02-28T23:59:00Z", "2024-03-01T00:01:00Z", "86520"},
		{"nonleap century", "2100-02-28T23:59:00Z", "2100-03-01T00:01:00Z", "120"},
		{"leap century", "2000-02-28T23:59:00Z", "2000-03-01T00:01:00Z", "86520"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			writeState(t, repo, folderX, row(tc.end, "execute", "review", "calendar", "ok", "review", "kind=quality dispatched="+tc.start+" round=1 blocked=0", "execute go"))
			want := "review-time: latency=" + tc.want + "s blocked=0s"
			if got := lineWith(statusOf(t, repo, folderX, "--metrics"), "review-time:"); got != want {
				t.Fatalf("calendar=%q, want %q", got, want)
			}
		})
	}
	repo := newRepo(t)
	writeState(t, repo, folderX, row("2026-10-14T10:00:00Z", "execute", "review", "numbers", "ok", "review", "kind=quality dispatched=2026-10-14T10:00:00Z round=1 blocked=2147483648", "execute go"))
	if got := lineWith(statusOf(t, repo, folderX, "--metrics"), "review-time:"); got != "review-time: latency=0s blocked=2147483648s" {
		t.Fatalf("reference numeric rendering: %q", got)
	}
}

func TestStatusLinuxReferenceLargeMetrics(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		blocked []string
		want    string
	}{
		{"above signed 32 bit", []string{"2147483648"}, "2147483648"},
		{"above unsigned 32 bit", []string{"4294967296"}, "4294967296"},
		{"rounded signed maximum", []string{"9223372036854775807"}, "9223372036854775808"},
		{"rounded unsigned maximum", []string{"18446744073709551615"}, "18446744073709551615"},
		{"overflowing signed sum", []string{"9223372036854775807", "9223372036854775807"}, "18446744073709551615"},
		{"beyond integer rendering", []string{"18446744073709555712"}, "1.84467e+19"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			var rows []string
			for i, blocked := range tc.blocked {
				rows = append(rows, row("2026-10-14T12:00:00Z", "execute", "review", "task-"+strconv.Itoa(i), "ok", "review", "kind=quality dispatched=2026-10-14T12:00:00Z round=1 blocked="+blocked, "execute go"))
			}
			writeState(t, repo, folderX, rows...)
			want := "review-time: latency=0s blocked=" + tc.want + "s"
			if got := lineWith(statusOf(t, repo, folderX, "--metrics"), "review-time:"); got != want {
				t.Fatalf("large metric = %q, want %q", got, want)
			}
		})
	}
}

func TestStatusAuditExcludesMergeOnlyPatch(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeFile(t, repo, "shared.txt", "base\n")
	commitAll(t, repo, "shared base")
	git(t, repo, "switch", "-q", "-c", "side")
	writeFile(t, repo, "shared.txt", "side\n")
	commitAll(t, repo, "side")
	git(t, repo, "switch", "-q", "-c", "topic", "main")
	writeFile(t, repo, "shared.txt", "topic\n")
	commitAll(t, repo, "topic")
	merge := runCmd(t, repo, "git", []string{"merge", "--no-ff", "-m", "combine", "side"}, baseEnv())
	mustExit(t, merge, 1, "conflicting merge fixture")
	writeFile(t, repo, "shared.txt", "resolved\n")
	commitAll(t, repo, "resolved merge")
	merged := gitOut(t, repo, "rev-parse", "HEAD")
	id := patchIDOf(repo, merged)
	if id == "" {
		t.Fatal("resolved merge fixture must expose a patch")
	}
	for _, sha := range strings.Fields(gitOut(t, repo, "rev-list", "--no-merges", "main..HEAD")) {
		if id == patchIDOf(repo, sha) {
			t.Fatal("merge patch must differ from individual branch patches")
		}
	}
	writeState(t, repo, folderX, row("2026-10-14T10:00:00Z", "execute", "verify", "merge-only", "ok", "verify", "commit=0000000000000000000000000000000000000009 patch-id="+id, "execute go"))
	if got := lineWith(statusOf(t, repo, folderX), "mismatch:"); !strings.Contains(got, "merge-only commit 0000000000000000000000000000000000000009 ") {
		t.Fatalf("merge-only patch incorrectly accepted: %q", got)
	}
}

func TestStatusBlockSortAndLatestDate(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "execute", "block", "z", "blocked", "block", "none", "execute go"),
		row("2026-10-14T10:01:00Z", "execute", "block", "a", "blocked", "block", "none", "execute go"),
		row("2026-10-14T10:02:00Z", "execute", "block", "B", "blocked", "block", "none", "execute go"),
		row("2026-10-15T10:00:00Z", "execute", "block", "a", "failed", "latest block", "none", "execute go"))
	var blocks []string
	for _, line := range statusOf(t, repo, folderX) {
		if strings.HasPrefix(line, "blocks:") {
			blocks = append(blocks, line)
		}
	}
	want := []string{"blocks: B blocked since 2026-10-14", "blocks: a blocked since 2026-10-15", "blocks: z blocked since 2026-10-14"}
	if !slices.Equal(blocks, want) {
		t.Fatalf("sorted latest blocks=%q, want %q", blocks, want)
	}
}

func TestStatusReviewEvidenceSubstringSemantics(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeState(t, repo, folderX,
		row("2026-10-14T10:01:00Z", "execute", "review", "one", "failed", "review", "kind=plan kind=quality dispatched=bad dispatched=2026-10-14T10:00:00Z blocked=7junk blocked=9", "execute go"),
		row("2026-10-14T10:02:00Z", "execute", "review", "two", "pending", "review", "KIND=quality xkind=packet dispatched=2026-10-14T10:00:00z blocked=3", "execute go"))
	assertAfterPlanToVerdict(t, statusOf(t, repo, folderX, "--metrics"), []string{"reviews: plan=1 packet=1", "review-time: latency=60s blocked=10s", "escapes: 0"})
	writeState(t, repo, folderX,
		row("2026-10-14T10:00:00Z", "execute", "enter", "run", "ok", "mode", "mode=pair branch=one", "execute go"),
		row("2026-10-14T10:01:00Z", "execute", "enter", "run", "ok", "clear mode", "", "execute go"))
	if hasLine(statusOf(t, repo, folderX), "mode:") {
		t.Fatal("latest empty mode evidence must clear prior mode")
	}
}

func TestStatusAuditMarkerSubstringsAndSort(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	writeState(t, repo, folderX,
		// The marker substrings sit on diverge rows, since a verify cell that
		// holds them fits no form and is no evidence.
		row("2026-10-14T10:00:00Z", "execute", "diverge", "z", "ok", "v", "prefixcommit=missing patch-id=bad", "execute go"),
		row("2026-10-14T10:01:00Z", "execute", "diverge", "a", "ok", "v", "commit=missing patch-id=bad", "execute go"),
		row("2026-10-14T10:02:00Z", "execute", "diverge", "B", "ok", "v", "commit=old commit= patch-id=bad", "execute go"),
		// A verify row whose target sorts between the diverge targets: lines
		// sort by event before target, so it comes last, not before z.
		row("2026-10-14T10:03:00Z", "execute", "verify", "m", "ok", "v", "commit=0000000000000000000000000000000000000009 patch-id="+pid40, "execute go"))
	var mismatches []string
	for _, line := range statusOf(t, repo, folderX) {
		if strings.HasPrefix(line, "mismatch:") {
			mismatches = append(mismatches, line)
		}
	}
	want := []string{
		"mismatch: diverge B commit  is not an ancestor of HEAD and patch-id bad matches no commit on this branch",
		"mismatch: diverge a commit missing is not an ancestor of HEAD and patch-id bad matches no commit on this branch",
		"mismatch: diverge z commit missing is not an ancestor of HEAD and patch-id bad matches no commit on this branch",
		"mismatch: verify m commit 0000000000000000000000000000000000000009 is not an ancestor of HEAD and patch-id " + pid40 + " matches no commit on this branch",
	}
	if !slices.Equal(mismatches, want) {
		t.Fatalf("marker/sort audit=%q, want %q", mismatches, want)
	}
}

// TestStatusAttestsCommitRowsAndWaivers pins that a commit row accounts for a
// commit on the unaccounted: line only when this clone's ledger holds it, and
// that each valid finding waiver prints a waivers: line saying whether this
// clone's ledger holds it.
func TestStatusAttestsCommitRowsAndWaivers(t *testing.T) {
	t.Parallel()
	seeded := row("2026-11-01T10:00:00Z", "intent", "create", "intent.md", "ok", "capture", "intent.md", "spec go")
	bound := row("2026-11-01T10:10:00Z", "execute", "bind", "topic", "ok", "bound topic to this flow", "none", "execute go")
	run := row("2026-11-01T10:20:00Z", "execute", "enter", "run", "ok", "start", "mode=solo branch=topic", "execute go")
	status := []string{"status", "--folder", contractFolder}
	topic := func(steps ...fixtureStep) func(t *testing.T) *contractFixture {
		return withFixtureSteps(onBranch("topic", withSteps(newState(seeded, bound, run), commitStep)), steps...)
	}
	// handRow writes, without flow log, a row naming the commit recorded under
	// key by its full sha and its real patch-id, so that only attestation can
	// keep it from accounting for the commit.
	handRow := func(event, target, key string) fixtureStep {
		return rowOf(func(fx *contractFixture) string {
			sha := fx.values[key]
			return row("2026-11-01T11:00:00Z", "execute", event, target, "ok", "by hand", "commit="+sha+" patch-id="+patchIDOf(fx.repo, sha), "execute go")
		})
	}
	logCommit := func(key string) contractStep {
		return contractStep{argsOf: func(fx *contractFixture) []string {
			return contractLog("execute", "commit", "user", "ok", "user commit", "auto:"+fx.values[key], contractNext)
		}}
	}
	// ledgerLine appends a ledger line by hand, as another flow's record would read.
	ledgerLine := func(kind, folder, target, key string) fixtureStep {
		return func(t *testing.T, fx *contractFixture) {
			t.Helper()
			file := filepath.Join(fx.repo, ".git", "agentics", "flow", "ledger.tsv")
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			line := strings.Join([]string{"2026-11-01T10:30:00Z", kind, folder, target, fx.values[key]}, "\t") + "\n"
			f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if _, err := f.WriteString(line); err != nil {
				t.Fatal(err)
			}
		}
	}
	const audit = "finding audit src/a.go:3"
	const review = "finding review src/b.go:4"
	// waiversAre expects the waivers: lines to be want, in order, after the
	// invalid: lines and directly before the sessions: line. The word today
	// stands for the date of the row flow log wrote.
	waiversAre := func(want ...string) func(t *testing.T, fx *contractFixture, res result) {
		return func(t *testing.T, fx *contractFixture, res result) {
			t.Helper()
			lines := strings.Split(strings.TrimRight(res.stdout, "\n"), "\n")
			var got []string
			first, last := -1, -1
			for i, line := range lines {
				if strings.HasPrefix(line, "waivers:") {
					got = append(got, line)
					if first < 0 {
						first = i
					}
					last = i
				}
			}
			today := ""
			for _, line := range dataLines(readLines(t, fx.statePath())) {
				if c := cells(line); len(c) == 9 && c[3] == "waive" && c[4] == audit {
					today, _, _ = strings.Cut(c[0], "T")
				}
			}
			expected := make([]string, len(want))
			for i, w := range want {
				expected[i] = strings.ReplaceAll(w, "today", today)
			}
			if !slices.Equal(got, expected) {
				t.Fatalf("waivers: lines %q, want %q in %q", got, expected, res.stdout)
			}
			if len(got) == 0 {
				return
			}
			if !strings.HasPrefix(lines[last+1], "sessions:") || !(strings.HasPrefix(lines[first-1], "unaccounted:") || strings.HasPrefix(lines[first-1], "invalid:")) {
				t.Errorf("waivers: lines must follow the unaccounted: and invalid: lines and precede sessions: in %q", res.stdout)
			}
		}
	}
	cases := []contractCase{
		{
			name:  "a-hand-written-commit-row-accounts-for-nothing",
			setup: topic(committedAs("one", "src/one.txt"), handRow("commit", "user", "one")),
			steps: []contractStep{{args: status, check: unaccountedIs("one")}},
		},
		{
			name:  "a-commit-row-flow-log-wrote-accounts-for-its-commit",
			setup: topic(committedAs("one", "src/one.txt")),
			steps: []contractStep{logCommit("one"), {args: status, check: unaccountedIs()}},
		},
		{
			name:  "a-hand-written-verify-row-still-accounts-for-its-commit",
			setup: topic(committedAs("one", "src/one.txt"), handRow("verify", "task-1", "one")),
			steps: []contractStep{{args: status, check: unaccountedIs()}},
		},
		{
			name:  "an-attested-commit-row-does-not-attest-another-in-the-same-folder",
			setup: topic(committedAs("one", "src/one.txt"), committedAs("two", "src/two.txt")),
			steps: []contractStep{
				logCommit("one"),
				{before: func(t *testing.T, fx *contractFixture) { handRow("commit", "user", "two")(t, fx) }, args: status, check: unaccountedIs("two")},
			},
		},
		{
			name:  "a-ledger-line-for-another-folder-does-not-attest",
			setup: topic(committedAs("one", "src/one.txt"), ledgerLine("commit", "docs/specs/2026-11/feature-other", "user", "one"), handRow("commit", "user", "one")),
			steps: []contractStep{{args: status, check: unaccountedIs("one")}},
		},
		{
			name:  "a-ledger-line-of-another-kind-does-not-attest",
			setup: topic(committedAs("one", "src/one.txt"), ledgerLine("approve", contractFolder, "user", "one"), handRow("commit", "user", "one")),
			steps: []contractStep{{args: status, check: unaccountedIs("one")}},
		},
		{
			name: "an-abbreviated-commit-value-stays-unattested",
			setup: topic(committedAs("one", "src/one.txt"), ledgerLine("commit", contractFolder, "user", "one"),
				rowOf(func(fx *contractFixture) string {
					return row("2026-11-01T11:00:00Z", "execute", "commit", "user", "ok", "by hand", "commit="+fx.values["one"][:7], "execute go")
				})),
			steps: []contractStep{{args: status, check: unaccountedIs("one")}},
		},
		{
			name:  "a-matching-ledger-line-attests-a-hand-written-row",
			setup: topic(committedAs("one", "src/one.txt"), ledgerLine("commit", contractFolder, "user", "one"), handRow("commit", "user", "one")),
			steps: []contractStep{{args: status, check: unaccountedIs()}},
		},
		{
			name:  "each-valid-finding-waiver-says-whether-this-clone-attests-it",
			setup: topic(),
			steps: []contractStep{
				{args: contractLog("execute", "waive", audit, "ok", contractWhy, "none", contractNext)},
				{
					// The hand-written row shares the logged row's ts, so only file
					// order puts it second.
					before: func(t *testing.T, fx *contractFixture) {
						t.Helper()
						logged := cells(dataLines(readLines(t, fx.statePath()))[3])
						appendStep(contractFolder, row(logged[0], "execute", "waive", review, "ok", "by hand", "none", "execute go"))(t, fx.repo)
					},
					args:        status,
					stdoutCount: []lineCount{{"waivers:", 2}},
					check:       waiversAre("waivers: "+audit+" today attested", "waivers: "+review+" today unattested"),
				},
			},
		},
		{
			name: "an-invalid-finding-waiver-prints-no-waivers-line",
			setup: topic(rowOf(func(*contractFixture) string {
				return row("2026-11-02T09:00:00Z", "execute", "waive", "finding audit", "ok", "by hand", "none", "execute go")
			})),
			steps: []contractStep{{args: status, stdoutCount: []lineCount{{"invalid:", 1}, {"waivers:", 0}}}},
		},
		{
			name: "a-waiver-of-another-head-prints-no-waivers-line",
			setup: topic(rowOf(func(*contractFixture) string {
				return row("2026-11-02T09:00:00Z", "execute", "waive", "review", "ok", "by hand", "none", "execute go")
			})),
			steps: []contractStep{{args: status, stdoutCount: []lineCount{{"invalid:", 0}, {"waivers:", 0}}}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			runContractCase(t, c)
		})
	}
}
