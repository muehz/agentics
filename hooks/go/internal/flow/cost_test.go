package flow

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// countCost installs a counter for the rest of the test. TestCost is not
// parallel, so no other test's work reaches the counter.
func countCost(t *testing.T) *costCounter {
	t.Helper()
	c := &costCounter{gitStarts: map[string]int{}, stateReads: map[string]int{}}
	cost = c
	t.Cleanup(func() { cost = nil })
	return c
}

func (c *costCounter) reset() {
	clear(c.gitStarts)
	clear(c.stateReads)
}

func (c *costCounter) reads() int {
	n := 0
	for _, count := range c.stateReads {
		n += count
	}
	return n
}

// readsOf counts the reads of folder's state.tsv, whatever separators the
// reader joined its path with.
func (c *costCounter) readsOf(folder string) int {
	n := 0
	for path, count := range c.stateReads {
		if strings.HasSuffix(filepath.ToSlash(path), "/"+folder+"/state.tsv") {
			n += count
		}
	}
	return n
}

const costOpen = "docs/specs/2026-10/open-work"

func closedFolder(i int) string { return fmt.Sprintf("docs/specs/2026-09/closed-%03d", i) }

// costRepo returns a repository on branch topic holding one open flow bound to
// topic and closed flows, every other one closed by its done row and the rest
// by an ok branch verdict at the default branch's tip.
func costRepo(t *testing.T, closed int) string {
	t.Helper()
	repo := newRepo(t)
	writeState(t, repo, costOpen,
		row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "seed", "intent.md", "spec go"),
		row("2026-10-14T10:01:00Z", "intent", "bind", "topic", "ok", "bound", "none", "spec go"))
	for i := range closed {
		last := row("2026-09-02T10:00:00Z", "gate", "done", "flow", "ok", "closed", "none", "none")
		if i%2 == 1 {
			last = row("2026-09-02T10:00:00Z", "gate", "branch", "feat/old", "ok", "merged", "none", "none")
		}
		writeState(t, repo, closedFolder(i),
			row("2026-09-01T10:00:00Z", "intent", "create", "intent.md", "ok", "seed", "intent.md", "spec go"), last)
	}
	commitAll(t, repo, "flows")
	git(t, repo, "switch", "-q", "-c", "topic")
	return repo
}

const costStore = ".git/agentics/work/"

// localCostRepo returns a repository under the local setting whose local
// store holds one open flow and closed flows closed by their done row. With
// bind set, the open flow is bound to it and the repository is on branch bind;
// otherwise the open flow is unbound and the repository stays on the default
// branch, where only an unbound, unapproved local flow is found.
func localCostRepo(t *testing.T, closed int, bind string) string {
	t.Helper()
	repo := newRepo(t)
	git(t, repo, "config", "agentics.flowStorage", "local")
	rows := []string{row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "seed", "intent.md", "spec go")}
	if bind != "" {
		rows = append(rows, row("2026-10-14T10:01:00Z", "intent", "bind", bind, "ok", "bound", "none", "spec go"))
		git(t, repo, "switch", "-q", "-c", bind)
	}
	writeState(t, repo, costStore+costOpen, rows...)
	for i := range closed {
		writeState(t, repo, costStore+closedFolder(i),
			row("2026-09-01T10:00:00Z", "intent", "create", "intent.md", "ok", "seed", "intent.md", "spec go"),
			row("2026-09-02T10:00:00Z", "gate", "done", "flow", "ok", "closed", "none", "none"))
	}
	return repo
}

// whereTwice runs where once to record the closed flows, then counts a second
// call alone.
func whereTwice(t *testing.T, repo string, c *costCounter) {
	t.Helper()
	wantFolder(t, whereOf(t, repo), costOpen)
	c.reset()
	wantFolder(t, whereOf(t, repo), costOpen)
}

func readCache(t *testing.T, file string) string {
	t.Helper()
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func writeCache(t *testing.T, file, body string) {
	t.Helper()
	if err := os.WriteFile(file, []byte(body), 0o666); err != nil {
		t.Fatal(err)
	}
}

func TestCost(t *testing.T) {
	t.Run("closed folders add no work once recorded", func(t *testing.T) {
		none, many := costRepo(t, 0), costRepo(t, 200)
		c := countCost(t)
		whereTwice(t, none, c)
		noneStarts, noneReads := maps.Clone(c.gitStarts), c.reads()
		// The top level, and the common directory, which the local store, the
		// closed-flow cache and the match's locate share.
		if noneStarts["rev-parse"] != 2 {
			t.Errorf("rev-parse processes for one lookup: %d, want 2 (%v)", noneStarts["rev-parse"], noneStarts)
		}
		whereTwice(t, many, c)
		if !maps.Equal(c.gitStarts, noneStarts) {
			t.Errorf("Git processes with 200 closed folders %v, with none %v", c.gitStarts, noneStarts)
		}
		if c.reads() != noneReads {
			t.Errorf("state.tsv reads with 200 closed folders %d, with none %d", c.reads(), noneReads)
		}
		cache := filepath.Join(newRepoView(many, os.Getenv).common(), "agentics", "flow", "closed.tsv")
		if info, err := os.Stat(cache); err != nil || info.Size() == 0 {
			t.Fatalf("no closed-flow cache at %s: %v", cache, err)
		}
		if listed := gitOut(t, many, "status", "--porcelain", "--untracked-files=all", "--ignored"); strings.Contains(listed, "closed.tsv") {
			t.Fatalf("git status lists the cache:\n%s", listed)
		}
	})

	for _, c := range []struct{ name, bind string }{
		{"on a feature branch", "topic"},
		{"on the default branch", ""},
	} {
		t.Run("closed local flows add no work once recorded "+c.name, func(t *testing.T) {
			none, many := localCostRepo(t, 0, c.bind), localCostRepo(t, 200, c.bind)
			counter := countCost(t)
			whereTwice(t, none, counter)
			noneStarts, noneReads := maps.Clone(counter.gitStarts), counter.reads()
			whereTwice(t, many, counter)
			if !maps.Equal(counter.gitStarts, noneStarts) {
				t.Errorf("Git processes with 200 closed local flows %v, with none %v", counter.gitStarts, noneStarts)
			}
			if counter.reads() != noneReads {
				t.Errorf("state.tsv reads with 200 closed local flows %d, with none %d", counter.reads(), noneReads)
			}
		})
	}

	t.Run("a reopened local flow is read again", func(t *testing.T) {
		repo := localCostRepo(t, 2, "topic")
		c := countCost(t)
		whereTwice(t, repo, c)
		if n := c.readsOf(closedFolder(0)) + c.readsOf(closedFolder(1)); n != 0 {
			t.Fatalf("recorded closed local flows read %d times", n)
		}
		reopened := filepath.Join(repo, filepath.FromSlash(costStore+closedFolder(0)), "state.tsv")
		f, err := os.OpenFile(reopened, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := f.WriteString(row("2026-09-03T10:00:00Z", "intent", "note", "reopen", "ok", "more", "none", "spec go") + "\n")
		if err := f.Close(); err != nil || writeErr != nil {
			t.Fatal(err, writeErr)
		}
		for range 2 {
			c.reset()
			wantFolder(t, whereOf(t, repo), costOpen)
			if c.readsOf(closedFolder(0)) == 0 || c.readsOf(closedFolder(1)) != 0 {
				t.Fatalf("after reopening %s: %v", closedFolder(0), c.stateReads)
			}
		}
	})

	t.Run("one process reads every default-tip copy", func(t *testing.T) {
		repo := newRepo(t)
		for i := range 10 {
			bind := ""
			if i == 0 {
				bind = "topic"
			}
			lookupSeed(t, repo, fmt.Sprintf("docs/specs/2026-10/candidate-%d", i), bind)
		}
		commitAll(t, repo, "flows")
		git(t, repo, "switch", "-q", "-c", "topic")
		c := countCost(t)
		wantFolder(t, whereOf(t, repo), "docs/specs/2026-10/candidate-0")
		if c.gitStarts["cat-file"] != 1 {
			t.Fatalf("cat-file processes for ten candidate flows: %d, want 1 (%v)", c.gitStarts["cat-file"], c.gitStarts)
		}
	})

	t.Run("a changed closed file is read again", func(t *testing.T) {
		repo := costRepo(t, 2)
		c := countCost(t)
		whereTwice(t, repo, c)
		if n := c.readsOf(closedFolder(0)) + c.readsOf(closedFolder(1)); n != 0 {
			t.Fatalf("recorded closed flows read %d times", n)
		}

		touched := filepath.Join(repo, filepath.FromSlash(closedFolder(0)), "state.tsv")
		info, err := os.Stat(touched)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(touched, info.ModTime().Add(time.Hour), info.ModTime().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		c.reset()
		wantFolder(t, whereOf(t, repo), costOpen)
		if c.readsOf(closedFolder(0)) != 1 || c.readsOf(closedFolder(1)) != 0 {
			t.Fatalf("after a time change: %v", c.stateReads)
		}

		grown := filepath.Join(repo, filepath.FromSlash(closedFolder(1)), "state.tsv")
		info, err = os.Stat(grown)
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(grown, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := f.WriteString("\n")
		if err := f.Close(); err != nil || writeErr != nil {
			t.Fatal(err, writeErr)
		}
		if err := os.Chtimes(grown, info.ModTime(), info.ModTime()); err != nil {
			t.Fatal(err)
		}
		c.reset()
		wantFolder(t, whereOf(t, repo), costOpen)
		if c.readsOf(closedFolder(1)) != 1 || c.readsOf(closedFolder(0)) != 0 {
			t.Fatalf("after a size change: %v", c.stateReads)
		}

		c.reset()
		wantFolder(t, whereOf(t, repo), costOpen)
		if n := c.readsOf(closedFolder(0)) + c.readsOf(closedFolder(1)); n != 0 {
			t.Fatalf("re-recorded closed flows read %d times", n)
		}
	})

	t.Run("a header-only flow is never recorded closed", func(t *testing.T) {
		repo := newRepo(t)
		writeState(t, repo, costOpen,
			row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "seed", "intent.md", "spec go"),
			row("2026-10-14T10:01:00Z", "intent", "bind", "topic", "ok", "bound", "none", "spec go"))
		const fresh = "docs/specs/2026-10/fresh"
		writeFile(t, repo, fresh+"/state.tsv", headerRow+"\n")
		commitAll(t, repo, "flows")
		git(t, repo, "switch", "-q", "-c", "topic")
		c := countCost(t)
		whereTwice(t, repo, c)
		if c.readsOf(fresh) == 0 {
			t.Fatalf("header-only flow skipped unread on the second lookup: %v", c.stateReads)
		}
		body, _ := os.ReadFile(closedCacheFile(newRepoView(repo, os.Getenv).common()))
		if strings.Contains(filepath.ToSlash(string(body)), "/"+fresh+"/") {
			t.Fatalf("cache records the header-only flow:\n%s", body)
		}
	})

	t.Run("a cache mostly of dead lines is compacted", func(t *testing.T) {
		repo := costRepo(t, 2)
		c := countCost(t)
		whereTwice(t, repo, c)
		file := closedCacheFile(newRepoView(repo, os.Getenv).common())
		live := readCache(t, file)
		lines := strings.SplitAfter(live, "\n")
		if len(lines) != 3 || lines[2] != "" {
			t.Fatalf("want two cache lines, got:\n%s", live)
		}
		first := strings.Split(lines[0], "\t")[0]
		removed := filepath.Join(repo, "removed-worktree", "docs", "state.tsv")
		writeCache(t, file, first+"\t1\t1\n"+removed+"\t5\t5\n"+live)

		c.reset()
		wantFolder(t, whereOf(t, repo), costOpen)
		if got := readCache(t, file); got != live {
			t.Fatalf("compacted cache:\n%s\nwant:\n%s", got, live)
		}
		if n := c.readsOf(closedFolder(0)) + c.readsOf(closedFolder(1)); n != 0 {
			t.Fatalf("closed flows read %d times during compaction", n)
		}
		c.reset()
		wantFolder(t, whereOf(t, repo), costOpen)
		if n := c.readsOf(closedFolder(0)) + c.readsOf(closedFolder(1)); n != 0 {
			t.Fatalf("closed flows read %d times after compaction", n)
		}
	})

	t.Run("a cache with few dead lines is left alone", func(t *testing.T) {
		repo := costRepo(t, 2)
		c := countCost(t)
		whereTwice(t, repo, c)
		file := closedCacheFile(newRepoView(repo, os.Getenv).common())
		live := readCache(t, file)
		dirty := "not a cache line\n" + live
		writeCache(t, file, dirty)
		wantFolder(t, whereOf(t, repo), costOpen)
		if got := readCache(t, file); got != dirty {
			t.Fatalf("cache with one dead line in three rewritten:\n%s", got)
		}

		gone := filepath.Join(repo, "removed-worktree", "docs", "state.tsv")
		writeCache(t, file, dirty+gone+"\t5\t5\n")
		wantFolder(t, whereOf(t, repo), costOpen)
		if got := readCache(t, file); got != live {
			t.Fatalf("cache with two dead lines in four not compacted:\n%s", got)
		}
	})

	t.Run("--folder reads a recorded flow", func(t *testing.T) {
		repo := costRepo(t, 1)
		c := countCost(t)
		whereTwice(t, repo, c)
		if c.readsOf(closedFolder(0)) != 0 {
			t.Fatalf("recorded closed flow read: %v", c.stateReads)
		}
		c.reset()
		res := runFlow(t, repo, nil, "status", "--folder", closedFolder(0))
		mustExit(t, res, 0, "status --folder")
		if !strings.HasPrefix(res.stdout, "flow: "+closedFolder(0)+"\n") || c.readsOf(closedFolder(0)) == 0 {
			t.Fatalf("status --folder did not read %s: %v\n%s", closedFolder(0), c.stateReads, res.stdout)
		}
	})
}
