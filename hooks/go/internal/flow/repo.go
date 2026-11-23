package flow

import (
	"fmt"
	"path/filepath"
)

// repoView answers the Git facts a flow command needs about the repository of
// one directory. Each fact is asked of Git at most once per view, and an empty
// or failed answer is kept too, so a directory outside a repository costs one
// failed call per fact, not one per reader. Building a view starts no Git
// process; a fact is asked when first read.
//
// The view holds no fact that a flow command changes during its run: the top
// level, the common directory, the branch name, the default branch and the
// storage setting. A reading that depends on HEAD, such as rev-parse of a
// revision, a range listing or a patch-id, stays a direct call, because a
// command can move HEAD.
//
// branch and defaultRef run Git in the top level, so a caller that may be
// outside a repository checks top for "" first: with an empty top level Git
// would run in the process directory.
//
// A view is not safe for concurrent use.
type repoView struct {
	dir    string
	getenv func(string) string

	topDone, branchDone, refDone, commonDone, storageDone bool

	topText, branchText, refText, commonText, storageText string
	storageErr                                            error
}

// newRepoView returns the view of the repository holding dir. getenv supplies
// FLOW_STORAGE for the storage setting.
func newRepoView(dir string, getenv func(string) string) *repoView {
	return &repoView{dir: dir, getenv: getenv}
}

// top returns the repository's top-level directory, or "" outside a repository
// or when Git cannot run.
func (v *repoView) top() string {
	if !v.topDone {
		v.topText, v.topDone = gitText(v.dir, "rev-parse", "--show-toplevel"), true
	}
	return v.topText
}

// root returns the directory a relative flow folder resolves against: the top
// level, or the view's directory outside a repository.
func (v *repoView) root() string {
	if top := v.top(); top != "" {
		return filepath.FromSlash(top)
	}
	return v.dir
}

// branch returns the current branch name, or "" for a detached HEAD or outside
// a repository.
func (v *repoView) branch() string {
	if !v.branchDone {
		v.branchText, v.branchDone = gitText(v.top(), "symbolic-ref", "--quiet", "--short", "HEAD"), true
	}
	return v.branchText
}

// defaultRef returns the default branch: the branch origin/HEAD names, else
// main or master when that local branch exists, else "".
func (v *repoView) defaultRef() string {
	if v.refDone {
		return v.refText
	}
	v.refDone = true
	top := v.top()
	if ref := gitText(top, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); ref != "" {
		v.refText = ref
		return ref
	}
	for _, branch := range []string{"main", "master"} {
		if gitText(top, "show-ref", "--verify", "refs/heads/"+branch) != "" {
			v.refText = branch
			break
		}
	}
	return v.refText
}

// onDefault reports whether the repository is on its default branch. A
// detached HEAD counts as on it, as it has no branch of its own.
func (v *repoView) onDefault() bool {
	branch := v.branch()
	return branch == "" || isDefaultBranch(branch, v.defaultRef())
}

// common returns the Git directory every worktree of the repository shares, or
// "" outside a repository or when Git cannot run. Git may answer with a path
// relative to the view's directory, never to the process directory.
func (v *repoView) common() string {
	if v.commonDone {
		return v.commonText
	}
	v.commonDone = true
	out := gitText(v.dir, "rev-parse", "--git-common-dir")
	if out == "" {
		return ""
	}
	common := filepath.FromSlash(out)
	if !filepath.IsAbs(common) {
		common = filepath.Join(v.dir, common)
	}
	v.commonText = filepath.Clean(common)
	return v.commonText
}

// storage returns the storage setting. The Git config value comes first
// because it holds the user's own choice; FLOW_STORAGE carries the
// repository's default from a skill, so it yields to that choice.
func (v *repoView) storage() (string, error) {
	if v.storageDone {
		return v.storageText, v.storageErr
	}
	v.storageDone = true
	value, source := gitText(v.dir, "config", "--get", "agentics.flowStorage"), "Git config agentics.flowStorage"
	if value == "" {
		value, source = v.getenv("FLOW_STORAGE"), "FLOW_STORAGE"
	}
	switch value {
	case "":
		v.storageText = storageCommitted
	case storageCommitted, storageLocal:
		v.storageText = value
	default:
		v.storageErr = fmt.Errorf("flow: storage setting %q from %s is invalid; use %s or %s", value, source, storageCommitted, storageLocal)
	}
	return v.storageText, v.storageErr
}
