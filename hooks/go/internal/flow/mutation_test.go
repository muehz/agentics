package flow

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMutationStatusAccountsForUppercaseCommitEvidence(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	parent := gitOut(t, repo, "rev-parse", "HEAD")
	git(t, repo, "switch", "-q", "-c", "topic")
	writeFile(t, repo, "src/change.txt", "source change\n")
	git(t, repo, "add", "src/change.txt")
	tree := gitOut(t, repo, "write-tree")
	var sha, body string
	// The hash must contain both boundary letters without depending on the clock
	// or starting additional Git processes to search for a suitable commit.
	for i := 0; i < 65536; i++ {
		body = fmt.Sprintf("tree %s\nparent %s\nauthor t <t@example.com> 1700000000 +0000\ncommitter t <t@example.com> 1700000000 +0000\n\nchange %d\n", tree, parent, i)
		sha = fmt.Sprintf("%x", sha1.Sum([]byte(fmt.Sprintf("commit %d\x00%s", len(body), body))))
		if strings.Contains(sha, "a") && strings.Contains(sha, "f") {
			break
		}
	}
	if !strings.Contains(sha, "a") || !strings.Contains(sha, "f") {
		t.Fatal("no commit hash with both hexadecimal boundary letters")
	}
	out, err := gitBytes(repo, []byte(body), "hash-object", "-t", "commit", "-w", "--stdin")
	if err != nil || strings.TrimSpace(string(out)) != sha {
		t.Fatalf("write commit: hash %q, error %v; want %s", out, err, sha)
	}
	git(t, repo, "update-ref", "refs/heads/topic", sha)
	const folder = "docs/specs/2026-11/mutation-evidence"
	bound := row("2026-11-01T10:00:00Z", "execute", "bind", "topic", "ok", "bound", "none", "execute go")
	writeState(t, repo, folder, bound)
	before := runFlow(t, repo, nil, "status", "--folder", folder)
	mustExit(t, before, 0, "status before evidence")
	if !strings.Contains(before.stdout, "unaccounted: 1 "+sha[:7]+"\n") {
		t.Fatalf("source commit is not listed before evidence:\n%s", before.stdout)
	}
	writeState(t, repo, folder, bound,
		row("2026-11-01T11:00:00Z", "execute", "review", "task-1", "ok", "reviewed", "kind=quality base=main head=topic", "execute go"))
	invalidRange := runFlow(t, repo, nil, "status", "--folder", folder)
	mustExit(t, invalidRange, 0, "status after non-hexadecimal range evidence")
	if !strings.Contains(invalidRange.stdout, "unaccounted: 1 "+sha[:7]+"\n") {
		t.Fatalf("non-hexadecimal range evidence accounted for a source commit:\n%s", invalidRange.stdout)
	}
	writeState(t, repo, folder, bound,
		row("2026-11-01T11:00:00Z", "execute", "review", "task-1", "ok", "reviewed", "kind=quality commit="+strings.ToUpper(sha), "execute go"))
	after := runFlow(t, repo, nil, "status", "--folder", folder)
	mustExit(t, after, 0, "status after uppercase evidence")
	if !strings.Contains(after.stdout, "unaccounted: none\n") {
		t.Fatalf("uppercase hexadecimal evidence did not account for its commit:\n%s", after.stdout)
	}
}

func TestMutationFailedCacheCompactionRemovesTemporaryFile(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	const kept = "existing directory contents\n"
	writeFile(t, repo, "cache/keep.txt", kept)
	destination := filepath.Join(repo, "cache")
	cache := closedCache{file: destination}
	cache.compact([]string{"replacement cache\n"})
	leftovers, err := filepath.Glob(filepath.Join(repo, "closed-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("failed cache compaction left temporary files: %v", leftovers)
	}
	body, err := os.ReadFile(filepath.Join(destination, "keep.txt"))
	if err != nil || string(body) != kept {
		t.Fatalf("failed cache compaction changed the destination: bytes %q, error %v", body, err)
	}
}

func TestMutationStatusKeepsSafeBoundaryLettersUnquoted(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	const folder = "docs/specs/2026-11/AZaz09"
	writeFile(t, repo, folder+"/spec.md", "approved content\n")
	hash := firstRuleHash(repo, folder, "spec.md")
	if hash == "" {
		t.Fatal("empty approved hash")
	}
	commitAll(t, repo, "approved")
	short := gitOut(t, repo, "rev-parse", "--short", "HEAD")
	writeState(t, repo, folder,
		row("2026-11-01T10:00:00Z", "spec", "approve", "spec.md", "ok", "approved", "blob="+hash, "execute go"))
	writeFile(t, repo, folder+"/spec.md", "changed content\n")
	result := runFlow(t, repo, nil, "status", "--folder", folder)
	mustExit(t, result, 0, "status with safe boundary letters in folder")
	want := "compare with commit " + short + " (from the repository root): git diff " + short + " -- " + folder + "/spec.md"
	if !strings.Contains(result.stdout, want) {
		t.Fatalf("safe boundary letters were quoted in the stale pointer:\n%s\nwant %s", result.stdout, want)
	}
}

func TestMutationStateFallbackPreservesWriteFailure(t *testing.T) {
	if os.Getenv("AGX_MUTATION_LIMITED_WRITER") == "1" {
		restore, err := mutationLimitFileSize()
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := restore(); err != nil {
				t.Errorf("restore child file-size limit before coverage reporting: %v", err)
			}
		}()
		dir := t.TempDir()
		source := filepath.Join(dir, "source")
		destination := filepath.Join(dir, "state.tsv")
		if err := os.WriteFile(source, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		err = publishStateUsing(source, destination, []byte(stateHeader), func(_, _ string) error { return os.ErrPermission })
		var writeErr *os.PathError
		if !errors.As(err, &writeErr) || writeErr.Op != "write" || writeErr.Path != destination {
			t.Fatalf("fallback publication discarded write failure: %v", err)
		}
		info, statErr := os.Stat(destination)
		if statErr != nil || info.Size() != 0 {
			t.Fatalf("limited writer did not leave an empty destination: info %v, error %v", info, statErr)
		}
		return
	}
	t.Parallel()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("requires Unix RLIMIT_FSIZE in an isolated child; unavailable on Windows")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestMutationStateFallbackPreservesWriteFailure$")
	cmd.Env = baseEnv("AGX_MUTATION_LIMITED_WRITER=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("limited writer child: %v\n%s", err, out)
	}
}

func TestMutationStatusListsCommitsWithoutCommonAncestor(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "switch", "-q", "--orphan", "topic")
	writeFile(t, repo, "src/change.txt", "unrelated source\n")
	commitAll(t, repo, "unrelated root")
	sha := gitOut(t, repo, "rev-parse", "HEAD")
	const folder = "docs/specs/2026-11/unrelated-history"
	writeState(t, repo, folder,
		row("2026-11-01T10:00:00Z", "execute", "bind", "topic", "ok", "bound", "none", "execute go"))
	result := runFlow(t, repo, nil, "status", "--folder", folder)
	mustExit(t, result, 0, "status without common ancestor")
	if !strings.Contains(result.stdout, "unaccounted: 1 "+sha[:7]+"\n") {
		t.Fatalf("status lost the source commit without a common ancestor:\n%s", result.stdout)
	}
}

func TestMutationStatusRetiresVerdictOwnEvidence(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	const folder = "docs/specs/2026-11/verdict-evidence"
	writeState(t, repo, folder,
		row("2026-11-01T10:00:00Z", "gate", "branch", "topic", "ok", "closed", "commit=01234567", "none"))
	result := runFlow(t, repo, nil, "status", "--folder", folder)
	mustExit(t, result, 0, "status after closing verdict")
	if strings.Contains(result.stdout, "mismatch:") {
		t.Fatalf("closing verdict did not retire its own evidence:\n%s", result.stdout)
	}
}

func TestMutationStatusEvidenceBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, event, evidence, timestamp, reviews, latency string
	}{
		{"unknown-kind-ending-in-z", "review", "kind=qualityz", "2026-11-01T10:01:00Z", "none", "0"},
		{"empty-kind-before-valid-kind", "review", "kind= kind=quality", "2026-11-01T10:01:00Z", "quality=1", "0"},
		{"timestamp-at-start-and-exact-end", "review", "dispatched=2026-11-01T10:00:00Z", "2026-11-01T10:01:00Z", "none", "60"},
		{"truncated-timestamp-at-end", "review", "dispatched=2026", "2026-11-01T10:01:00Z", "none", "0"},
		{"empty-commit-value-at-end", "diverge", "commit=", "2026-11-01T10:01:00Z", "none", "0"},
		{"single-sign-timestamp", "review", "kind=quality dispatched=2026-11-01T00:00:00Z", "-", "quality=1", "0"},
		{"signed-year-prefix", "review", "dispatched=0000-11-01T00:00:00Z", "+001-11-01T00:00:00Z", "none", "31536000"},
		{"fractional-hour-prefix", "review", "kind=quality dispatched=2026-11-01T00:00:00Z", "2026-11-01T.5:00:00Z", "quality=1", "1800"},
		{"numeric-prefix-before-nonnumeric", "review", "dispatched=2026-11-01T00:00:00Z", "2026-11-01T1x:00:00Z", "none", "3600"},
		{"field-width-stops-scientific-prefix", "review", "dispatched=2026-01-01T00:00:00Z", "2026-1e+3-T00:00:00Z", "none", "172800"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepo(t)
			const folder = "docs/specs/2026-11/evidence-boundaries"
			writeState(t, repo, folder,
				row(tc.timestamp, "execute", tc.event, "task-1", "ok", "recorded", tc.evidence, "execute go"))
			result := runFlow(t, repo, nil, "status", "--folder", folder, "--metrics")
			mustExit(t, result, 0, "status for evidence boundary")
			for _, want := range []string{"reviews: " + tc.reviews, "review-time: latency=" + tc.latency + "s blocked=0s"} {
				if !strings.Contains(result.stdout, want+"\n") {
					t.Fatalf("evidence boundary %s: missing %q:\n%s", tc.name, want, result.stdout)
				}
			}
		})
	}
}

func TestMutationWhereContinuesPastEmptyDefaultTip(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	const first = "docs/specs/2026-11/a-empty-tip"
	const second = "docs/specs/2026-11/b-closed-tip"
	writeFile(t, repo, first+"/state.tsv", "")
	writeState(t, repo, second,
		row("2026-11-01T10:00:00Z", "gate", "branch", "old", "ok", "closed", "none", "none"))
	commitAll(t, repo, "default tip copies")
	git(t, repo, "switch", "-q", "-c", "topic")
	bound := row("2026-11-01T11:00:00Z", "execute", "bind", "topic", "ok", "bound", "none", "execute go")
	writeState(t, repo, first, bound)
	writeState(t, repo, second, bound)
	result := whereOf(t, repo)
	mustExit(t, result, 0, "lookup after an empty default-tip blob")
	if result.stdout != first+"\n" {
		t.Fatalf("empty default-tip blob hid the later closed flow: %q, want %s", result.stdout, first)
	}
}

func TestMutationUnassignedEqualTimesKeepStoreOrder(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	const folder = "docs/specs/unassigned"
	writeState(t, repo, folder,
		row("2026-11-01T10:00:00Z", "execute", "skip", "chain main", "ok", "committed", "none", "execute committed"))
	writeState(t, repo, ".git/agentics/work/"+folder,
		row("2026-11-01T10:00:00Z", "execute", "pivot", "fix main", "ok", "local", "none", "execute local"))
	result := runFlow(t, repo, nil, "status", "--folder", folder)
	mustExit(t, result, 0, "unassigned status with equal timestamps")
	if !strings.Contains(result.stdout, "next: execute local\n") {
		t.Fatalf("equal timestamps did not preserve committed-before-local order:\n%s", result.stdout)
	}
}

func TestMutationNumericOutputBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value float64
		want  string
	}{
		{-9223372036854775808, "-9223372036854775808"},
		{math.Inf(-1), "-inf"},
	} {
		if got := statusNumericText(tc.value); got != tc.want {
			t.Fatalf("numeric output for %v = %q, want %q", tc.value, got, tc.want)
		}
	}
}

func TestMutationStatusKeepsLargeFiniteYearRounding(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	const folder = "docs/specs/2026-11/numeric-epoch"
	writeState(t, repo, folder,
		row("3e11-01-01T00:00:00Z", "intent", "create", "intent.md", "ok", "captured", "none", "intent approve"),
		row("3e11-01-07T00:00:00Z", "intent", "approve", "intent.md", "ok", "approved", "none", "spec go"))
	result := runFlow(t, repo, nil, "status", "--folder", folder, "--metrics")
	mustExit(t, result, 0, "status with large finite year fields")
	// At this magnitude the reference's epoch origin affects float rounding
	// before subtraction, even though the ideal calendar interval is unchanged.
	if !strings.Contains(result.stdout, "time-to-intent: 520192s\n") {
		t.Fatalf("large finite year interval changed reference rounding:\n%s", result.stdout)
	}
}

func TestMutationStatusPathsConsumesSingleLetterRename(t *testing.T) {
	t.Parallel()
	got := statusPaths("R  x\x00XXXab/foo/state.tsv\x00")
	if len(got) != 1 || !got["x"] || got["ab/foo/state.tsv"] {
		t.Fatalf("porcelain rename lost its destination or exposed its source: %v", got)
	}
}
