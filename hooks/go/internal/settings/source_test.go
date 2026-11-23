package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSourceResolvesOncePerDirectory(t *testing.T) {
	calls := map[string]int{}
	var mu sync.Mutex
	src := NewSource(func(cwd string) Result {
		mu.Lock()
		defer mu.Unlock()
		calls[cwd]++
		return Result{Policy: Defaults(), RepoPath: cwd}
	})
	for range 3 {
		if got := src.Resolve("/a"); got.RepoPath != "/a" {
			t.Fatalf("Resolve(/a).RepoPath = %q", got.RepoPath)
		}
	}
	if got := src.Resolve("/b"); got.RepoPath != "/b" {
		t.Fatalf("Resolve(/b).RepoPath = %q", got.RepoPath)
	}
	src.Resolve("/b")
	if calls["/a"] != 1 || calls["/b"] != 1 || len(calls) != 2 {
		t.Fatalf("resolver calls = %v, want one per directory", calls)
	}
}

func TestSourceIsLazy(t *testing.T) {
	var calls atomic.Int32
	src := NewSource(func(string) Result {
		calls.Add(1)
		return Result{Policy: Defaults()}
	})
	if got := calls.Load(); got != 0 {
		t.Fatalf("NewSource called the resolver %d times, want 0", got)
	}
	src.Resolve("/a")
	if got := calls.Load(); got != 1 {
		t.Fatalf("resolver calls after one Resolve = %d, want 1", got)
	}
}

func TestSourceConcurrentCallsShareOneResult(t *testing.T) {
	var calls atomic.Int32
	src := NewSource(func(cwd string) Result {
		calls.Add(1)
		return Result{Policy: Defaults(), RepoPath: cwd}
	})
	const workers = 32
	results := make([]Result, workers)
	var start, done sync.WaitGroup
	start.Add(1)
	for i := range workers {
		done.Add(1)
		go func() {
			defer done.Done()
			start.Wait()
			results[i] = src.Resolve("/shared")
		}()
	}
	start.Done()
	done.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("resolver calls = %d, want 1", got)
	}
	for i, res := range results {
		if res.RepoPath != "/shared" {
			t.Fatalf("result %d RepoPath = %q", i, res.RepoPath)
		}
	}
}

func TestNilSourceReadsTheFiles(t *testing.T) {
	f := newFixture(t)
	t.Setenv("HOME", f.home)
	t.Setenv("USERPROFILE", f.home)
	var src *Source
	wantOnly(t, src.Resolve(f.cwd))
	write(t, f.repo, `{"version":1,"policy":{"draft_prs":false}}`)
	wantOnly(t, src.Resolve(f.cwd), DraftPRs)
	write(t, f.repo, `{"version":1,"policy":{"block_package_publish":false}}`)
	wantOnly(t, src.Resolve(f.cwd), BlockPackagePublish)
}

func TestSourceResolveEqualsResolve(t *testing.T) {
	const valid = `{"version":1,"policy":{"draft_prs":false}}`
	const invalid = `{"version":1,"oops":1}`
	worlds := []struct {
		name       string
		user, repo string
		cwd        func(f fixture) string
		chdir      bool
	}{
		{name: "no files"},
		{name: "valid user file", user: valid},
		{name: "valid repository file", repo: valid},
		{name: "both files", user: `{"version":1,"policy":{"command_reformulation":false}}`, repo: valid},
		{name: "invalid user file", user: invalid, repo: valid},
		{name: "invalid repository file", user: valid, repo: invalid},
		{name: "relative cwd", repo: valid, cwd: func(fixture) string { return filepath.Join("pkg", "inner") }},
		{name: "empty cwd", repo: valid, cwd: func(fixture) string { return "" }, chdir: true},
	}
	for _, w := range worlds {
		t.Run(w.name, func(t *testing.T) {
			f := newFixture(t)
			t.Setenv("HOME", f.home)
			t.Setenv("USERPROFILE", f.home)
			if w.user != "" {
				write(t, f.home, w.user)
			}
			if w.repo != "" {
				write(t, f.repo, w.repo)
			}
			cwd := f.cwd
			if w.cwd != nil {
				cwd = w.cwd(f)
			}
			if w.chdir {
				t.Chdir(f.cwd)
			} else if _, err := os.Stat(cwd); err != nil && filepath.IsAbs(cwd) {
				t.Fatal(err)
			}
			want := Resolve(cwd)
			if got := NewSource(Resolve).Resolve(cwd); !reflect.DeepEqual(got, want) {
				t.Errorf("source result = %+v, want %+v", got, want)
			}
			var nilSrc *Source
			if got := nilSrc.Resolve(cwd); !reflect.DeepEqual(got, want) {
				t.Errorf("nil source result = %+v, want %+v", got, want)
			}
		})
	}
}
