package flow

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestLocatedSwitchKeepsUnassignedOverride pins the one place status departs
// from the located flow: when only the local store holds the unassigned file,
// status reports and reads that store, and runs Git at the repository root,
// although the setting would send a write to the committed store.
func TestLocatedSwitchKeepsUnassignedOverride(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	git(t, repo, "config", "agentics.flowStorage", "local")
	sha := commitFile(t, repo, "src/a.txt", "a\n", "change a")
	logOK(t, repo, unassignedFolder, "execute", "commit", "user", "ok", "user commit", "auto:"+sha, "gate go")
	if _, err := os.Stat(filepath.Join(repo, ".git", "agentics", "work", "docs", "specs", "unassigned", "state.tsv")); err != nil {
		t.Fatalf("the commit row is not in the local store: %v", err)
	}
	git(t, repo, "config", "agentics.flowStorage", "committed")
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(unassignedFolder), "state.tsv")); !os.IsNotExist(err) {
		t.Fatalf("the committed store holds the unassigned file: %v", err)
	}
	got := statusOf(t, repo, unassignedFolder)
	if !slices.Contains(got, "storage: local") {
		t.Errorf("status names another store:\n%s", strings.Join(got, "\n"))
	}
	if invalid := linesWith(got, "invalid:"); len(invalid) > 0 {
		t.Errorf("status reads the commit row as invalid: %q", invalid)
	}
}

// placeState creates or removes the state.tsv that makes dir hold a flow.
func placeState(t *testing.T, dir string, held bool) {
	t.Helper()
	if dir == "" {
		return
	}
	file := filepath.Join(dir, "state.tsv")
	if !held {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return
	}
	writeFile(t, dir, "state.tsv", "ts\tsession\tphase\tevent\ttarget\tstatus\twhy\tevidence\tnext\n")
}

// TestLocateFlowFields pins every spelling of a located flow, and its two
// refusals, as literal values relative to the temporary repository, one row
// per way of naming the folder and per store state that changes an answer.
// Each row builds its own program, since a repository view keeps its first
// storage answer.
func TestLocateFlowFields(t *testing.T) {
	repo := newRepo(t)
	local := filepath.Join(repo, ".git", "agentics", "work")
	sub := filepath.Join(repo, "deep", "down")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bare, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	linkErr := os.Symlink(repo, filepath.Join(repo, "link"))

	const flow = "docs/specs/2026-11/located-work"
	const unassigned = "docs/specs/unassigned"
	const plansFlow = "plans/2026-11/located-work"
	at := func(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }
	// An outside folder named through a link is keyed by the directory it
	// resolves to, so the link needs a real target to resolve.
	realOutside := at(outside, "real")
	if err := os.MkdirAll(at(realOutside, flow), 0o755); err != nil {
		t.Fatal(err)
	}
	outsideLinkErr := os.Symlink(realOutside, at(outside, "alias"))
	// top is the repository root as Git prints it, which is what a flow in the
	// local store runs Git in.
	top := filepath.ToSlash(repo)
	committed, kept := at(repo, flow), at(local, flow)
	committedU, keptU := at(repo, unassigned), at(local, unassigned)
	away, bareFlow := at(outside, flow), at(bare, flow)
	through := at(repo, "link/"+flow)
	storeLogical := ".git/agentics/work/" + flow

	// inside is a flow folder that lies inside the repository.
	inside := func(given, dir, logical string, stored storedFolder, gitDir string) locatedFlow {
		return locatedFlow{given: given, root: repo, dir: dir, logical: logical, ledgerKey: logical, rel: logical,
			inside: true, stored: stored, gitDir: gitDir}
	}
	// outsideRepo is a folder that lies outside the repository, so it keeps its
	// own name, is keyed by its absolute path and is read where it is.
	outsideRepo := func(root, given, dir string) locatedFlow {
		return locatedFlow{given: given, root: root, dir: dir, logical: given, ledgerKey: filepath.ToSlash(dir),
			stored: storedFolder{dir, storageCommitted}, gitDir: dir}
	}
	markUnassigned := func(loc locatedFlow) locatedFlow {
		loc.unassigned = true
		return loc
	}
	refused := locatedFlow{given: kept, root: repo, dir: kept, logical: storeLogical, ledgerKey: storeLogical, rel: storeLogical, inside: true}

	rows := []struct {
		name        string
		dir         string
		folder      string
		held        string // which stores hold state.tsv: "", "committed", "local" or "both"
		heldAt      string // the logical folder that holds it; flow when empty
		workRoot    string
		setting     string
		want        locatedFlow
		wantErr     string
		wantExit    int
		link        bool
		outsideLink bool
	}{
		{name: "relative new flow, committed setting", dir: repo, folder: flow, setting: storageCommitted,
			want: inside(flow, committed, flow, storedFolder{committed, storageCommitted}, committed)},
		{name: "relative new flow, local setting", dir: repo, folder: flow, setting: storageLocal,
			want: inside(flow, committed, flow, storedFolder{kept, storageLocal}, top)},
		{name: "relative held by the committed store, local setting", dir: repo, folder: flow, held: "committed", setting: storageLocal,
			want: inside(flow, committed, flow, storedFolder{committed, storageCommitted}, committed)},
		{name: "relative held by the local store, committed setting", dir: repo, folder: flow, held: "local", setting: storageCommitted,
			want: inside(flow, committed, flow, storedFolder{kept, storageLocal}, top)},
		{name: "relative held by both stores", dir: repo, folder: flow, held: "both", setting: storageCommitted, wantExit: 4,
			want: locatedFlow{given: flow, root: repo, dir: committed, logical: flow, ledgerKey: flow, rel: flow, inside: true},
			wantErr: "flow: " + flow + " is held by both stores, committed at " + committed + " and local at " + kept +
				"; keep one and remove the other"},
		{name: "relative from a repository subdirectory", dir: sub, folder: flow, setting: storageCommitted,
			want: inside(flow, committed, flow, storedFolder{committed, storageCommitted}, committed)},
		{name: "absolute inside, local setting", dir: repo, folder: committed, setting: storageLocal,
			want: inside(committed, committed, flow, storedFolder{committed, storageCommitted}, committed)},
		{name: "absolute inside from a subdirectory", dir: sub, folder: committed, setting: storageCommitted,
			want: inside(committed, committed, flow, storedFolder{committed, storageCommitted}, committed)},
		{name: "absolute outside through a symlink", dir: repo, folder: at(outside, "alias/"+flow), setting: storageCommitted, outsideLink: true,
			want: locatedFlow{given: at(outside, "alias/"+flow), root: repo, dir: at(outside, "alias/"+flow), logical: at(outside, "alias/"+flow),
				ledgerKey: filepath.ToSlash(at(realOutside, flow)), stored: storedFolder{at(outside, "alias/"+flow), storageCommitted}, gitDir: at(outside, "alias/"+flow)}},
		{name: "absolute outside the repository", dir: repo, folder: away, setting: storageLocal,
			want: outsideRepo(repo, away, away)},
		{name: "relative through a symlink into the repository", dir: repo, folder: "link/" + flow, setting: storageCommitted, link: true,
			want: inside("link/"+flow, through, flow, storedFolder{through, storageCommitted}, through)},
		{name: "inside the local store", dir: repo, folder: kept, setting: storageCommitted, wantExit: 2, want: refused,
			wantErr: "flow: invalid folder: '" + kept + "' -- this is inside the clone-local store; pass --folder " + flow},
		{name: "unassigned, committed setting", dir: repo, folder: unassigned, setting: storageCommitted,
			want: markUnassigned(inside(unassigned, committedU, unassigned, storedFolder{committedU, storageCommitted}, committedU))},
		{name: "unassigned, local setting", dir: repo, folder: unassigned, setting: storageLocal,
			want: markUnassigned(inside(unassigned, committedU, unassigned, storedFolder{keptU, storageLocal}, top))},
		{name: "unassigned held by the local store, committed setting", dir: repo, folder: unassigned, held: "local", heldAt: unassigned, setting: storageCommitted,
			want: markUnassigned(inside(unassigned, committedU, unassigned, storedFolder{committedU, storageCommitted}, committedU))},
		{name: "work root plans", dir: repo, folder: plansFlow, workRoot: "/plans", setting: storageLocal,
			want: inside(plansFlow, at(repo, plansFlow), plansFlow, storedFolder{at(local, plansFlow), storageLocal}, top)},
		{name: "outside a repository", dir: bare, folder: flow, setting: storageLocal,
			want: outsideRepo(bare, flow, bareFlow)},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			if row.outsideLink && outsideLinkErr != nil {
				t.Skipf("cannot make a symlink here: %v", outsideLinkErr)
			}
			if row.link && linkErr != nil {
				t.Skipf("cannot make a symlink here: %v", linkErr)
			}
			heldAt := row.heldAt
			if heldAt == "" {
				heldAt = flow
			}
			placeState(t, at(repo, heldAt), row.held == "committed" || row.held == "both")
			placeState(t, at(local, heldAt), row.held == "local" || row.held == "both")
			defer placeState(t, at(repo, heldAt), false)
			defer placeState(t, at(local, heldAt), false)

			getenv := func(key string) string {
				switch key {
				case "FLOW_WORK_ROOT":
					return row.workRoot
				case "FLOW_STORAGE":
					return row.setting
				}
				return ""
			}
			var stderr bytes.Buffer
			p := program{dir: row.dir, getenv: getenv, stderr: &stderr, repo: newRepoView(row.dir, getenv)}
			got, err := p.locateFlow(row.folder)
			if got != row.want {
				t.Errorf("fields:\n got  %+v\n want %+v", got, row.want)
			}
			if row.wantErr == "" {
				if err != nil {
					t.Fatalf("error %q, want none", err)
				}
				return
			}
			if errText(err) != row.wantErr {
				t.Fatalf("error %q, want %q", errText(err), row.wantErr)
			}
			if code := p.locateFailed(err); code != row.wantExit {
				t.Errorf("exit %d, want %d", code, row.wantExit)
			}
			if stderr.String() != row.wantErr+"\n" {
				t.Errorf("stderr %q, want %q", stderr.String(), row.wantErr+"\n")
			}
		})
	}
}
