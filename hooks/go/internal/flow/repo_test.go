package flow

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// repoViewCase is a repository state a view is read in. build returns the
// directory the view runs in; for the no-repository case it is a plain
// directory, and the test runs from it too, because a command with an empty
// top level runs Git in the process directory.
type repoViewCase struct {
	name  string
	build func(t *testing.T) string
}

func repoViewCases() []repoViewCase {
	return []repoViewCase{
		{"no-repository", func(t *testing.T) string {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Chdir(dir)
			return dir
		}},
		{"detached-head", func(t *testing.T) string {
			repo := newRepo(t)
			git(t, repo, "checkout", "-q", "--detach")
			return repo
		}},
		{"feature-branch", func(t *testing.T) string {
			repo := newRepo(t)
			git(t, repo, "checkout", "-q", "-b", "feature")
			return repo
		}},
		{"main-with-origin-head", func(t *testing.T) string {
			repo := newRepo(t)
			git(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
			git(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
			return repo
		}},
		{"master-without-remote", func(t *testing.T) string {
			repo := newRepo(t)
			git(t, repo, "branch", "-m", "master")
			return repo
		}},
		{"linked-worktree", func(t *testing.T) string {
			repo := newRepo(t)
			linked := filepath.Join(t.TempDir(), "linked")
			git(t, repo, "worktree", "add", "-q", "-b", "side", linked)
			resolved, err := filepath.EvalSymlinks(linked)
			if err != nil {
				t.Fatal(err)
			}
			return resolved
		}},
		{"repository-subdirectory", func(t *testing.T) string {
			repo := newRepo(t)
			sub := filepath.Join(repo, "deep", "down")
			if err := os.MkdirAll(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			return sub
		}},
	}
}

func errText(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

// repoFacts is what a repository view answers, as literal values.
type repoFacts struct {
	top, root, branch, defaultRef, common string
	onDefault                             bool
}

// linkedCommon reads the common directory of a linked worktree from the
// pointer file Git leaves in it, which names the worktree's own directory
// inside the common one.
func linkedCommon(t *testing.T, worktree string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(worktree, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	pointer, ok := strings.CutPrefix(strings.TrimSpace(string(body)), "gitdir: ")
	if !ok {
		t.Fatalf("not a worktree pointer: %q", body)
	}
	return filepath.Dir(filepath.Dir(filepath.FromSlash(pointer)))
}

// TestRepoViewFacts pins each method of the view as a literal value per
// repository state, and the storage reading per setting, with the error text.
func TestRepoViewFacts(t *testing.T) {
	// plain is a repository on main with no remote: its own .git is the common
	// directory.
	plain := func(top, branch, ref string, onDefault bool) repoFacts {
		return repoFacts{top: filepath.ToSlash(top), root: top, branch: branch, defaultRef: ref,
			common: filepath.Join(top, ".git"), onDefault: onDefault}
	}
	want := map[string]func(t *testing.T, dir string) repoFacts{
		"no-repository": func(t *testing.T, dir string) repoFacts {
			return repoFacts{root: dir, onDefault: true}
		},
		"detached-head": func(t *testing.T, dir string) repoFacts { return plain(dir, "", "main", true) },
		"feature-branch": func(t *testing.T, dir string) repoFacts {
			return plain(dir, "feature", "main", false)
		},
		"main-with-origin-head": func(t *testing.T, dir string) repoFacts {
			return plain(dir, "main", "origin/main", true)
		},
		"master-without-remote": func(t *testing.T, dir string) repoFacts {
			return plain(dir, "master", "master", true)
		},
		"linked-worktree": func(t *testing.T, dir string) repoFacts {
			return repoFacts{top: filepath.ToSlash(dir), root: dir, branch: "side", defaultRef: "main",
				common: linkedCommon(t, dir)}
		},
		"repository-subdirectory": func(t *testing.T, dir string) repoFacts {
			top := filepath.Dir(filepath.Dir(dir))
			return plain(top, "main", "main", true)
		},
	}
	for _, c := range repoViewCases() {
		t.Run(c.name, func(t *testing.T) {
			wantFacts, ok := want[c.name]
			if !ok {
				t.Fatalf("no expected facts for %s", c.name)
			}
			dir := c.build(t)
			w := wantFacts(t, dir)
			view := newRepoView(dir, func(string) string { return "" })
			got := repoFacts{top: view.top(), root: view.root(), branch: view.branch(), defaultRef: view.defaultRef(),
				common: view.common(), onDefault: view.onDefault()}
			if got != w {
				t.Errorf("facts:\n got  %+v\n want %+v", got, w)
			}
			if setting, err := view.storage(); setting != storageCommitted || err != nil {
				t.Errorf("storage() = %q, %v; want %q, nil", setting, err, storageCommitted)
			}
		})
	}

	for _, c := range []struct {
		name, config, env string
		want, wantErr     string
	}{
		{name: "none", want: "committed"},
		{name: "environment", env: "local", want: "local"},
		{name: "config", config: "local", want: "local"},
		{name: "both", config: "committed", env: "local", want: "committed"},
		{name: "invalid-config", config: "shared",
			wantErr: `flow: storage setting "shared" from Git config agentics.flowStorage is invalid; use committed or local`},
		{name: "invalid-environment", env: "Local",
			wantErr: `flow: storage setting "Local" from FLOW_STORAGE is invalid; use committed or local`},
		{name: "invalid-in-each", config: "shared", env: "Local",
			wantErr: `flow: storage setting "shared" from Git config agentics.flowStorage is invalid; use committed or local`},
	} {
		t.Run("storage-"+c.name, func(t *testing.T) {
			repo := newRepo(t)
			if c.config != "" {
				git(t, repo, "config", "agentics.flowStorage", c.config)
			}
			getenv := func(key string) string {
				if key == "FLOW_STORAGE" {
					return c.env
				}
				return ""
			}
			got, err := newRepoView(repo, getenv).storage()
			if got != c.want || (c.wantErr == "" && err != nil) || (c.wantErr != "" && errText(err) != c.wantErr) {
				t.Fatalf("storage() = %q, %v; want %q, %q", got, err, c.want, c.wantErr)
			}
		})
	}
}

func TestRepoViewAsksOnce(t *testing.T) {
	for _, c := range repoViewCases() {
		t.Run(c.name, func(t *testing.T) {
			dir := c.build(t)
			trace := filepath.Join(t.TempDir(), "trace.txt")
			t.Setenv("GIT_TRACE", trace)
			view := newRepoView(dir, func(string) string { return "" })
			if body, _ := os.ReadFile(trace); len(body) != 0 {
				t.Fatalf("building the view started Git:\n%s", body)
			}
			for range 2 {
				view.top()
				view.root()
				view.branch()
				view.defaultRef()
				view.onDefault()
				view.common()
				view.storage()
			}
			body, err := os.ReadFile(trace)
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]int{}
			for _, line := range strings.Split(string(body), "\n") {
				if _, command, ok := strings.Cut(line, "built-in: git "); ok {
					seen[strings.TrimSpace(command)]++
				}
			}
			if len(seen) == 0 {
				t.Fatalf("the trace names no Git command:\n%s", body)
			}
			for command, n := range seen {
				if n != 1 {
					t.Errorf("git %s ran %d times, want once", command, n)
				}
			}
			for _, want := range []string{"rev-parse --show-toplevel", "rev-parse --git-common-dir", "config --get agentics.flowStorage"} {
				if seen[want] != 1 {
					t.Errorf("git %s ran %d times, want once; trace:\n%s", want, seen[want], body)
				}
			}
		})
	}
}

// TestCommandsAskRepositoryFactsOnce runs each command with no --folder on a
// flow bound to the branch, so each reaches lookup, and counts the Git calls
// for the facts the repository view holds. bind --slice on a flow bound with
// no slice goes on to log, so its trace covers both commands. It is not
// parallel, because statusCommandRun sets GIT_TRACE in the process
// environment, which the command's Git children inherit.
func TestCommandsAskRepositoryFactsOnce(t *testing.T) {
	const folder = "docs/specs/2026-10/bound-work"
	cases := []struct {
		name string
		args []string
		// wrote is a cell the last row holds after the command, for the
		// commands that write one, so the trace is known to cover log.
		wrote string
	}{
		{"where", []string{"where"}, ""},
		{"status", []string{"status"}, ""},
		{"path", []string{"path"}, ""},
		{"log", []string{"log", "execute", "note", "coverage", "ok", "noted", "none", "execute go"}, "noted"},
		{"bind", []string{"bind", "--slice", "S1"}, "slice=S1"},
		{"audit", []string{"audit", "--base", "main"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := newRepo(t)
			writeState(t, repo, folder,
				row("2026-10-14T10:00:00Z", "intent", "create", "intent.md", "ok", "seed", "intent.md", "spec go"),
				row("2026-10-14T10:01:00Z", "intent", "bind", "topic", "ok", "bound", "none", "spec go"))
			commitAll(t, repo, "flow")
			git(t, repo, "switch", "-q", "-c", "topic")
			trace := filepath.Join(t.TempDir(), "trace.txt")
			res := statusCommandRun(t, repo, []string{"GIT_TRACE=" + trace}, c.args...)
			mustExit(t, res, 0, c.name)
			if c.wrote != "" {
				rows := dataLines(readLines(t, filepath.Join(repo, filepath.FromSlash(folder), "state.tsv")))
				if last := rows[len(rows)-1]; !slices.Contains(cells(last), c.wrote) {
					t.Fatalf("%s wrote no row holding %q; last row %q", c.name, c.wrote, last)
				}
			}
			body, err := os.ReadFile(trace)
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]int{}
			for _, line := range strings.Split(string(body), "\n") {
				if _, command, ok := strings.Cut(line, "built-in: git "); ok {
					seen[strings.TrimSpace(command)]++
				}
			}
			if seen["rev-parse --show-toplevel"] != 1 {
				t.Errorf("git rev-parse --show-toplevel ran %d times, want once", seen["rev-parse --show-toplevel"])
			}
			for _, command := range []string{
				"rev-parse --git-common-dir",
				"config --get agentics.flowStorage",
				"symbolic-ref --quiet --short HEAD",
				"symbolic-ref --quiet --short refs/remotes/origin/HEAD",
			} {
				if seen[command] > 1 {
					t.Errorf("git %s ran %d times, want at most once", command, seen[command])
				}
			}
			if c.name == "bind" && seen["config --get agentics.flowStorage"] != 1 {
				t.Errorf("git config --get agentics.flowStorage ran %d times across bind and its log, want once", seen["config --get agentics.flowStorage"])
			}
			if t.Failed() {
				t.Logf("trace:\n%s", body)
			}
		})
	}
}
