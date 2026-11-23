package flow

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Storage settings: committed keeps a new work folder under the work root;
// local keeps it in the Git common directory, outside every commit.
const (
	storageCommitted = "committed"
	storageLocal     = "local"
)

// checkStorage resolves the setting before a command does any other work, so
// an invalid value never writes a row; it returns 2 after reporting one.
func (p *program) checkStorage() int {
	if _, err := p.repo.storage(); err != nil {
		fmt.Fprintln(p.stderr, err)
		return 2
	}
	return 0
}

// storedFolder is where the files of a flow folder are read and written, and
// the store that holds them. Only state.tsv and the artifacts are read there:
// Git never runs in the local store, where it would answer for the main
// checkout's HEAD instead of the calling worktree's.
type storedFolder struct {
	dir, store string
}

var monthName = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(`^[0-9]{4}-[0-9]{2}$`) })

// workRootName is the work root in force as a root-relative forward-slash
// folder, the form logical folders start with.
func (p *program) workRootName() string {
	return path.Clean(strings.Trim(filepath.ToSlash(p.workRoot()), "/"))
}

// unassignedName is the logical folder that holds small-fix rows.
func (p *program) unassignedName() string { return p.workRootName() + "/unassigned" }

// mappedShape reports whether logical, a root-relative forward-slash folder,
// is a monthly folder or the unassigned folder of the work root in force; only
// these have a place in the local store.
func (p *program) mappedShape(logical string) bool {
	rest, ok := strings.CutPrefix(logical, p.workRootName()+"/")
	if !ok {
		return false
	}
	if rest == "unassigned" {
		return true
	}
	month, name, ok := strings.Cut(rest, "/")
	return ok && monthName().MatchString(month) && name != "" && !strings.Contains(name, "/")
}

// localDir returns the local-store directory of logical, or "" without a
// common directory, such as when Git cannot run.
func (p *program) localDir(logical string) string {
	return localDirIn(p.repo.common(), logical)
}

// localDirIn is localDir for the common directory common.
func localDirIn(common, logical string) string {
	if common == "" {
		return ""
	}
	return filepath.Join(common, "agentics", "work", filepath.FromSlash(logical))
}

// locateFailed reports an error from locateFlow and returns its exit status: 2
// for a refused folder, 4 for a folder no store can settle.
func (p *program) locateFailed(err error) int {
	fmt.Fprintln(p.stderr, err)
	var refused *cellError
	if errors.As(err, &refused) {
		return refused.ExitCode()
	}
	return 4
}

// holdsFlow reports whether dir holds a flow, which is when its state.tsv exists.
func holdsFlow(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "state.tsv"))
	return err == nil && info.Mode().IsRegular()
}
