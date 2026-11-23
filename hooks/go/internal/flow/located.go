package flow

import (
	"fmt"
	"path/filepath"
	"strings"
)

// locatedFlow is one work folder resolved once into every spelling the
// commands need.
//
//   - given is the folder as the caller named it.
//   - root is the directory a relative folder resolves against, the
//     repository view's root.
//   - dir is the folder's directory: given when absolute, else below root.
//   - logical is the name messages print: the repository-relative path with
//     forward slashes inside the repository, else given.
//   - ledgerKey is the folder field of a ledger line: the repository-relative
//     path inside the repository, else dir made absolute with its symlinks
//     resolved, in forward slashes.
//   - rel and inside are the repository-relative path and whether the folder
//     lies inside the repository, compared with symlinks resolved.
//   - unassigned is true when the folder is relative, inside the repository,
//     and the work root's unassigned folder.
//   - stored is where a write goes: the store that holds the flow, or the one
//     the setting names for a new flow and for the unassigned folder.
//   - gitDir is where Git runs for the flow: the repository root for a flow in
//     the local store, where Git would answer for the main checkout's HEAD, and
//     the stored directory otherwise, which is dir for a committed-store flow.
//
// The spellings are filled even when locateFlow returns an error, so a caller
// that reports a folder before its store keeps its order; stored and gitDir
// are then zero. The error is a refusal of a folder inside the local store
// (exit 2 through locateFailed) or of a folder both stores hold (exit 4).
type locatedFlow struct {
	given, root, dir   string
	logical, ledgerKey string
	rel                string
	inside, unassigned bool
	stored             storedFolder
	gitDir             string
}

// locateFlow resolves folder against the repository view. A relative folder of
// the mapped shape inside the repository is found in whichever store holds
// it, and the setting decides only where a new one goes; one held by both
// stores is an error, since neither copy can be preferred. The unassigned
// folder is not one flow but a log of small fixes that each clone may keep in
// either store, so its writes go to the store the setting names. Any other
// folder is used as given, except one inside the local store, which is refused
// in favour of its logical folder. Without a common directory, such as when
// Git cannot run, a new folder goes under the work root.
func (p *program) locateFlow(folder string) (locatedFlow, error) {
	top, root := p.repo.top(), p.repo.root()
	loc := locatedFlow{given: folder, root: root, dir: flowFolder(root, folder)}
	loc.rel, loc.inside = repoRelative(top, root, folder)
	loc.logical, loc.ledgerKey = folder, loc.rel
	if loc.inside {
		loc.logical = loc.rel
	} else {
		path := loc.dir
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		loc.ledgerKey = filepath.ToSlash(filepath.Clean(resolvedPath(path)))
	}
	loc.unassigned = loc.inside && !filepath.IsAbs(folder) && loc.rel == p.unassignedName()
	stored, err := p.storeOf(loc)
	if err != nil {
		return loc, err
	}
	loc.stored, loc.gitDir = stored, stored.dir
	if stored.store == storageLocal {
		loc.gitDir = top
	}
	return loc, nil
}

// storeOf returns the store a write to the located folder goes to.
func (p *program) storeOf(loc locatedFlow) (storedFolder, error) {
	common := p.repo.common()
	if err := insideStore(common, loc.root, loc.given); err != nil {
		return storedFolder{}, err
	}
	given := storedFolder{loc.dir, storageCommitted}
	if filepath.IsAbs(loc.given) || !loc.inside || !p.mappedShape(loc.rel) {
		return given, nil
	}
	localDir := localDirIn(common, loc.rel)
	if localDir == "" {
		return given, nil
	}
	local := storedFolder{localDir, storageLocal}
	bySetting := func() storedFolder {
		if setting, _ := p.repo.storage(); setting == storageLocal {
			return local
		}
		return given
	}
	if loc.unassigned {
		return bySetting(), nil
	}
	inLocal, inCommitted := holdsFlow(local.dir), holdsFlow(given.dir)
	switch {
	case inLocal && inCommitted:
		return storedFolder{}, fmt.Errorf("flow: %s is held by both stores, %s at %s and %s at %s; keep one and remove the other",
			loc.rel, storageCommitted, given.dir, storageLocal, local.dir)
	case inLocal:
		return local, nil
	case inCommitted:
		return given, nil
	}
	return bySetting(), nil
}

// flowFolder returns the directory of a flow folder: an absolute folder as
// given, a relative one below root. locateFlow passes the repository view's
// root, so that a relative folder names the same directory from anywhere in
// the repository.
func flowFolder(root, folder string) string {
	if filepath.IsAbs(folder) {
		return folder
	}
	return filepath.Join(root, folder)
}

// repoRelative returns the path of a flow folder relative to the repository
// root top with forward slashes, and false when the folder lies outside it.
// Both paths are compared with their symlinks resolved, so that a folder named
// through a link into the repository counts as inside.
func repoRelative(top, root, folder string) (string, bool) {
	if top == "" {
		return "", false
	}
	rel, err := filepath.Rel(resolvedPath(filepath.FromSlash(top)), resolvedPath(flowFolder(root, folder)))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// insideStore refuses a folder that names a directory inside the local store
// of common, such as one flow path printed. Used as given, it would read as a
// committed flow, key the ledger by its store path and run Git inside the
// store. Both the folder as given and its symlink-resolved form are compared,
// because a folder not created yet cannot be resolved.
func insideStore(common, root, folder string) error {
	if common == "" {
		return nil
	}
	target := flowFolder(root, folder)
	if abs, err := filepath.Abs(target); err == nil {
		target = abs
	}
	pairs := [][2]string{
		{localDirIn(common, ""), filepath.Clean(target)},
		{localDirIn(resolvedPath(common), ""), resolvedPath(target)},
	}
	for _, pair := range pairs {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		advice := "pass --folder " + filepath.ToSlash(rel)
		if rel == "." {
			advice = "pass the logical folder with --folder"
		}
		return invalidCell("folder", folder, "this is inside the clone-local store; "+advice)
	}
	return nil
}
