package flow

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

func (p *program) workRoot() string {
	if root := p.getenv("FLOW_WORK_ROOT"); root != "" {
		return root
	}
	return "docs/specs"
}

// isDefaultBranch reports whether branch is the one defaultRef names, which may
// be the remote-tracking form origin/<branch>.
func isDefaultBranch(branch, defaultRef string) bool {
	return branch == strings.TrimPrefix(defaultRef, "origin/")
}

// The shell prefixes ROOT with the repository even when ROOT starts with '/'.
func (p *program) rootPath(top string) string { return filepath.FromSlash(top + "/" + p.workRoot()) }

// listedFlow is a flow lookup lists: its logical folder, the name messages
// print and a later locateFlow maps, and the directory that holds its files.
type listedFlow struct {
	logical string
	storedFolder
}

// allFlows lists the flows of both stores, the local one in the common Git
// directory common. A local-store folder outside the mapped shape is left out,
// since locateFlow would not map its logical folder back to it.
func (p *program) allFlows(top, common string) []listedFlow {
	var flows []listedFlow
	for _, dir := range monthlyFlows(p.rootPath(top)) {
		flows = append(flows, listedFlow{relativeFlow(top, dir), storedFolder{dir, storageCommitted}})
	}
	if store := localDirIn(common, p.workRootName()); store != "" {
		for _, dir := range monthlyFlows(store) {
			logical := p.workRootName() + "/" + filepath.Base(filepath.Dir(dir)) + "/" + filepath.Base(dir)
			if p.mappedShape(logical) {
				flows = append(flows, listedFlow{logical, storedFolder{dir, storageLocal}})
			}
		}
	}
	return flows
}

// monthlyFlows returns the folders two levels below root that hold a flow.
func monthlyFlows(root string) []string {
	var flows []string
	months, _ := os.ReadDir(root)
	for _, month := range months {
		if strings.HasPrefix(month.Name(), ".") {
			continue
		}
		names, _ := os.ReadDir(filepath.Join(root, month.Name()))
		for _, name := range names {
			if strings.HasPrefix(name.Name(), ".") {
				continue
			}
			folder := root + "/" + month.Name() + "/" + name.Name()
			if holdsFlow(folder) {
				flows = append(flows, folder)
			}
		}
	}
	return flows
}

func relativeFlow(top, folder string) string {
	return strings.TrimPrefix(filepath.ToSlash(folder), filepath.ToSlash(top)+"/")
}

// lookupFolder returns the repository-relative form of a folder lookup found.
// A slash-rooted work root keeps its slash in what where prints, as the shell
// prints it, but locateFlow would take that form as an absolute path outside
// the repository.
func lookupFolder(folder string) string { return strings.TrimLeft(folder, "/") }

// pickOne returns the one logical folder of folders, 1 for none, or 4 after
// listing them all. A folder both stores hold is reported as locateFlow
// reports it, with exit 4, rather than listed once for each store. Every
// locateFlow error exits 4: lookup lists logical folders, which the refusal of
// a folder inside the local store never names.
func (p *program) pickOne(folders []string) (string, int) {
	var unique []string
	seen := make(map[string]bool)
	for _, folder := range folders {
		name := lookupFolder(folder)
		if seen[name] {
			continue
		}
		seen[name] = true
		if _, err := p.locateFlow(name); err != nil {
			fmt.Fprintln(p.stderr, err)
			return "", 4
		}
		unique = append(unique, folder)
	}
	if len(unique) == 1 {
		return unique[0], 0
	}
	if len(unique) > 1 {
		fmt.Fprintln(p.stderr, "flow: more than one flow matches; ask which one:")
		for _, folder := range unique {
			fmt.Fprintln(p.stderr, "  "+folder)
		}
		return "", 4
	}
	return "", 1
}

// unassignedRows returns the rows of the unassigned folder whose committed
// directory is committed, and its local-store directory, or "" without one.
// Each store may hold an unassigned/state.tsv, whichever setting wrote it, so
// the rows of both are read in time order, with the committed file first among
// rows of the same second: a pivot in one file ends a skip in the other.
func (p *program) unassignedRows(committed string) ([]stateRow, string) {
	rows := rowsOf(filepath.Join(committed, "state.tsv"))
	local := p.localDir(p.unassignedName())
	if local == "" {
		return rows, ""
	}
	// Sorting only a merge leaves a read of the committed file alone as it was.
	if localRows := rowsOf(filepath.Join(local, "state.tsv")); len(localRows) > 0 {
		rows = append(rows, localRows...)
		sort.SliceStable(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
	}
	return rows, local
}

// smallFixOpen reports whether a small fix is in progress on branch, by the
// rows of both unassigned files.
func (p *program) smallFixOpen(top, branch string) bool {
	rows, _ := p.unassignedRows(filepath.Join(p.rootPath(top), "unassigned"))
	open := false
	for _, row := range rows {
		if row[3] == "skip" && row[4] == "chain "+branch {
			open = true
		}
		if row[3] == "pivot" && row[4] == "fix "+branch {
			open = false
		}
	}
	return open
}

func pathSet(text string) map[string]bool {
	paths := make(map[string]bool)
	for _, path := range strings.Split(text, "\n") {
		if path != "" {
			paths[path] = true
		}
	}
	return paths
}

// lookupOutcome is the case flow lookup settles on. Each command maps it to
// its own exit status.
type lookupOutcome int

const (
	lookupNone lookupOutcome = iota
	lookupFound
	lookupSmallFix
	lookupAmbiguous
)

// lookup is what flow lookup found. lookupNone also covers no repository and
// a detached HEAD. lookupAmbiguous means the diagnostic is already on stderr,
// so a caller prints nothing more. For lookupFound, printed is the folder as
// where prints it, which keeps a slash-rooted work root's leading slash, and
// folder is the logical form locateFlow takes. branch is set only for
// lookupSmallFix, and names the branch whose small fix is in progress.
type lookup struct {
	outcome lookupOutcome
	printed string
	folder  string
	branch  string
}

// settled maps a pickOne answer to the lookup it settles, or reports that no
// flow matched, so the caller goes on to its next step.
func settled(folder string, code int) (lookup, bool) {
	switch code {
	case 0:
		return lookup{outcome: lookupFound, printed: folder, folder: lookupFolder(folder)}, true
	case 4:
		return lookup{outcome: lookupAmbiguous}, true
	}
	return lookup{}, false
}

// lookupFlow finds the flow a command with no folder acts on. A flow bound to
// the branch wins; then an open small fix on the branch; then, on the default
// branch, the one changed committed flow or the one unbound, unapproved
// local flow, and elsewhere the one flow the branch changed since it forked.
// It never writes workflow state; it may append to, or compact, the
// closed-flow cache in the Git directory.
func (p *program) lookupFlow() lookup {
	top := p.repo.top()
	if top == "" {
		return lookup{}
	}
	// A detached HEAD matches nothing.
	branch := p.repo.branch()
	if branch == "" {
		return lookup{}
	}
	common := p.repo.common()
	flows := p.allFlows(top, common)
	stored := make([]storedFolder, len(flows))
	for i, flow := range flows {
		stored[i] = flow.storedFolder
	}
	openDir := p.openFlows(top, common, stored)
	open := func(flow listedFlow) bool { return openDir(flow.dir) }
	var matches []string
	best := ""
	for _, flow := range flows {
		// A closed flow is skipped before the later-bind comparison, so that its bind never hides an open flow.
		if !open(flow) {
			continue
		}
		stamp := bindTS(filepath.Join(flow.dir, "state.tsv"), branch)
		if stamp > best {
			best, matches = stamp, nil
		}
		if stamp != "" && stamp == best {
			matches = append(matches, flow.logical)
		}
	}
	if found, ok := settled(p.pickOne(matches)); ok {
		return found
	}
	if p.smallFixOpen(top, branch) {
		return lookup{outcome: lookupSmallFix, branch: branch}
	}
	if p.repo.onDefault() {
		if found, ok := settled(p.pickOne(p.changedOnDefault(top, flows, open))); ok {
			return found
		}
		found, _ := settled(p.pickOne(unboundLocal(flows, open)))
		return found
	}

	tip := p.repo.defaultRef()
	if tip == "" {
		return lookup{}
	}
	fork := gitText(top, "merge-base", tip, "HEAD")
	if fork == "" {
		fork = tip
	}
	pathspec := p.workRoot() + "/*/*/state.tsv"
	untracked := gitText(top, "ls-files", "--others", "--exclude-standard", "--", pathspec)
	tipPaths := pathSet(gitText(top, "diff", "--name-only", "--diff-filter=AM", tip, "--", pathspec) + "\n" + untracked)
	forkPaths := pathSet(gitText(top, "diff", "--name-only", "--diff-filter=AM", fork, "--", pathspec) + "\n" + untracked)
	var changed []string
	for path := range tipPaths {
		if forkPaths[path] && !strings.Contains(path, "/temp/") {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	matches = nil
	for _, path := range changed {
		if openDir(filepath.Dir(filepath.FromSlash(top + "/" + path))) {
			matches = append(matches, filepath.ToSlash(filepath.Dir(filepath.FromSlash(path))))
		}
	}
	found, _ := settled(p.pickOne(matches))
	return found
}

// changedOnDefault returns the logical folders of the open committed flows
// whose state.tsv differs from HEAD in the index or the working tree, or is
// untracked. On the default branch no branch diff can single out a flow, so
// only work not yet committed does.
func (p *program) changedOnDefault(top string, flows []listedFlow, open func(listedFlow) bool) []string {
	// --no-optional-locks keeps status from refreshing the index, so that a
	// lookup never takes index.lock while another Git command runs.
	listed := statusPaths(gitText(top, "--no-optional-locks", "status", "--porcelain", "-z", "--untracked-files=all", "--", p.workRoot()))
	var matches []string
	for _, flow := range flows {
		if flow.store == storageCommitted && listed[flow.logical+"/state.tsv"] && open(flow) {
			matches = append(matches, flow.logical)
		}
	}
	return matches
}

// unboundLocal returns the one open local-store flow that has no bind row,
// when it has no approval yet, and otherwise none. No Git state can tell a local
// flow's work apart on the default branch, so only a flow nobody has bound or
// approved is taken as the one being started; with two candidates, or once an
// approval exists, the caller must name the folder.
func unboundLocal(flows []listedFlow, open func(listedFlow) bool) []string {
	found, approved, count := "", false, 0
	for _, flow := range flows {
		if flow.store != storageLocal || !open(flow) {
			continue
		}
		rows := rowsOf(filepath.Join(flow.dir, "state.tsv"))
		if slices.ContainsFunc(rows, func(row stateRow) bool { return row[3] == "bind" }) {
			continue
		}
		count++
		found = flow.logical
		approved = slices.ContainsFunc(rows, func(row stateRow) bool { return row[3] == "approve" && row[5] == "ok" })
	}
	if count != 1 || approved {
		return nil
	}
	return []string{found}
}

// statusPaths returns the paths of git status --porcelain -z output. -z keeps
// paths unquoted; a rename or copy record carries its source path as the next
// field, which is skipped because only the destination exists in the tree.
func statusPaths(out string) map[string]bool {
	listed := make(map[string]bool)
	records := strings.Split(out, "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 4 {
			continue
		}
		listed[record[3:]] = true
		if strings.ContainsAny(record[:2], "RC") {
			i++
		}
	}
	return listed
}

// resolveInto supplies the folder used by commands with no --folder: the
// logical folder of the flow lookup found, or the logical unassigned folder
// during a small fix, so that locateFlow maps it to the store that holds it,
// or to the store the setting names.
func (p *program) resolveInto() (string, int) {
	found := p.lookupFlow()
	switch found.outcome {
	case lookupFound:
		return found.folder, 0
	case lookupSmallFix:
		return p.unassignedName(), 0
	case lookupAmbiguous:
		return "", 4
	}
	fmt.Fprintln(p.stderr, "flow: no flow found; ask which flow, or pass --folder")
	return "", 3
}
