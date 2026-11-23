package scriptsnap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// writeIn writes name under dir and returns its path.
func writeIn(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// quarterMiB is a script just under the per-script limit; four fit in one request and a fifth does not.
var quarterMiB = []byte(strings.Repeat("# x\n", 250<<10/4))

func TestEventLimits(t *testing.T) {
	t.Run("files", func(t *testing.T) {
		dir, s := t.TempDir(), New()
		for i := 1; i <= 9; i++ {
			got := s.ReadScript(writeIn(t, dir, fmt.Sprintf("f%d.sh", i), []byte("x")))
			want := shellwords.RuleNone
			if i > 8 {
				want = shellwords.RuleTooManyFiles
			}
			if got.Limit != want {
				t.Fatalf("file %d: want %q, got %+v", i, want, got)
			}
		}
		// A path already in the snapshot is answered from it after the limit is reached.
		if got := s.ReadScript(filepath.Join(dir, "f1.sh")); got.Limit != shellwords.RuleNone {
			t.Fatalf("known path after the limit: %+v", got)
		}
	})
	t.Run("bytes", func(t *testing.T) {
		dir, s := t.TempDir(), New()
		for i := 1; i <= 5; i++ {
			got := s.ReadScript(writeIn(t, dir, fmt.Sprintf("q%d.sh", i), quarterMiB))
			switch {
			case i <= 4 && got.Limit != shellwords.RuleNone:
				t.Fatalf("file %d within the event limit refused: %+v", i, got)
			case i == 5 && (got.Limit != shellwords.RuleTooLarge || got.Detail != "exceeds the per-event limit" || got.Text != ""):
				t.Fatalf("file %d past the event limit: %+v", i, got)
			}
		}
		// A refusal for the total is never stored as the file's first read.
		if len(s.byID) != 4 {
			t.Fatalf("want 4 stored reads, got %d", len(s.byID))
		}
	})
	t.Run("bytes without identity", func(t *testing.T) {
		old := fileIDFunc
		t.Cleanup(func() { fileIDFunc = old })
		fileIDFunc = func(*os.File, os.FileInfo) (string, bool) { return "", true }
		dir, s := t.TempDir(), New()
		for i := 1; i <= 5; i++ {
			got := s.ReadScript(writeIn(t, dir, fmt.Sprintf("q%d.sh", i), quarterMiB))
			if want := map[bool]shellwords.Rule{true: shellwords.RuleNone, false: shellwords.RuleTooLarge}[i <= 4]; got.Limit != want {
				t.Fatalf("file %d: want %q, got %+v", i, want, got)
			}
		}
	})
	t.Run("refusals count", func(t *testing.T) {
		dir, s := t.TempDir(), New()
		for i := 1; i <= 8; i++ {
			if got := s.ReadScript(filepath.Join(dir, fmt.Sprintf("missing%d.sh", i))); got.Limit != shellwords.RuleUnreadable {
				t.Fatalf("missing %d: %+v", i, got)
			}
		}
		if got := s.ReadScript(writeIn(t, dir, "real.sh", []byte("x"))); got.Limit != shellwords.RuleTooManyFiles {
			t.Fatalf("a real file after eight refusals: %+v", got)
		}
	})
	t.Run("alias adds no bytes", func(t *testing.T) {
		dir, s := t.TempDir(), New()
		a := writeIn(t, dir, "a.sh", quarterMiB)
		b := filepath.Join(dir, "b.sh")
		if err := os.Link(a, b); err != nil {
			t.Skipf("hardlinks unavailable: %v", err)
		}
		// a and its alias b count once, so three more quarter-MiB files still fit.
		paths := []string{a, b, writeIn(t, dir, "c.sh", quarterMiB), writeIn(t, dir, "d.sh", quarterMiB), writeIn(t, dir, "e.sh", quarterMiB)}
		for _, p := range paths {
			if got := s.ReadScript(p); got.Limit != shellwords.RuleNone {
				t.Fatalf("%s: %+v", filepath.Base(p), got)
			}
		}
		if s.bytes != 4*len(quarterMiB) {
			t.Fatalf("counted %d bytes, want %d", s.bytes, 4*len(quarterMiB))
		}
	})
}

func TestReadBudgetIsSnapshotWide(t *testing.T) {
	dir := t.TempDir()
	a, b := writeIn(t, dir, "a.sh", []byte("echo a\n")), writeIn(t, dir, "b.sh", []byte("echo b\n"))
	var opens, starts atomic.Int32
	slowOpen := func() { opens.Add(1); time.Sleep(300 * time.Millisecond) }
	// beforeRead runs on the calling goroutine, so its count is exact when ReadScript returns.
	setHooks(t, 0, nil, slowOpen, func() { starts.Add(1) })
	oldBudget := readBudget
	t.Cleanup(func() { readBudget = oldBudget })
	readBudget = 100 * time.Millisecond

	s := New()
	start := time.Now()
	got := s.ReadScript(a)
	if took := time.Since(start); got.Limit != shellwords.RuleBudget || took > time.Second {
		t.Fatalf("first read: %+v after %v", got, took)
	}
	start = time.Now()
	got = s.ReadScript(b)
	if took := time.Since(start); got.Limit != shellwords.RuleBudget || took > raceSlowdown*150*time.Millisecond {
		t.Fatalf("second read: %+v after %v", got, took)
	}
	// The abandoned first read may not have reached its open yet, so opens is at most one.
	if n, m := starts.Load(), opens.Load(); n != 1 || m > 1 {
		t.Fatalf("a read was started after the budget was spent: %d starts, %d opens", n, m)
	}
	// The same slow reads fit the default budget.
	readBudget = oldBudget
	s = New()
	for _, p := range []string{a, b} {
		if got := s.ReadScript(p); got.Limit != shellwords.RuleNone {
			t.Fatalf("%s with the default budget: %+v", filepath.Base(p), got)
		}
	}
}
