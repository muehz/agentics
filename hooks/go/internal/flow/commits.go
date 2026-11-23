package flow

import (
	"bytes"
	"strings"
)

// isMergeCommit reports whether the commit sha has two or more parents. A
// merge has no single diff, so it has no patch-id to record.
func isMergeCommit(dir, sha string) bool {
	return len(strings.Fields(gitText(dir, "rev-list", "--parents", "-n", "1", sha))) > 2
}

// changedPaths returns the repository-relative paths the non-merge commit sha
// changes, and false when git cannot answer. NUL-separated output keeps paths
// that git would otherwise quote, and --root lets a root commit answer too.
func changedPaths(dir, sha string) ([]string, bool) {
	out, err := gitBytes(dir, nil, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", "--root", sha)
	if err != nil {
		return nil, false
	}
	var paths []string
	for _, path := range bytes.Split(out, []byte{0}) {
		if len(path) > 0 {
			paths = append(paths, string(path))
		}
	}
	return paths, true
}

// rangeCommit is one non-merge commit of a branch's range: its full sha, the
// repository-relative paths it changes, and its patch-id, empty for a commit
// that changes nothing.
type rangeCommit struct {
	sha     string
	paths   []string
	patchID string
}

// rangeCommits returns the non-merge commits of tip that base cannot reach,
// oldest first, with their paths and patch-ids. One listing reads every
// commit with its paths and one patch-id pass reads every patch, so the cost
// stays at three git processes however long the branch is. Renames are listed
// as a deletion and an addition, so that a file moved into the work folder
// does not make its commit look artifact-only. A git failure, such as an
// unborn HEAD, yields no commits, and a failed patch pass yields commits
// without patch-ids; checkedRangeCommits reports either failure instead.
func rangeCommits(dir, tip, base string) []rangeCommit {
	commits, _ := checkedRangeCommits(dir, tip, base)
	return commits
}

// checkedRangeCommits is rangeCommits that also returns the error of a failed
// Git call, with the commits read before it: none when the listing failed,
// and every commit without its patch-id when the patch pass failed.
func checkedRangeCommits(dir, tip, base string) ([]rangeCommit, error) {
	revs := []string{tip, "--not", base, "--"}
	listing, err := gitBytes(dir, nil, append([]string{"log", "--no-merges", "--reverse", "--root", "--no-renames", "--no-show-signature", "-z", "--format=%x00%H", "--name-only"}, revs...)...)
	if err != nil {
		return nil, err
	}
	// Each commit starts with an empty token, which no path can be, followed
	// by its sha; the first path after the sha carries the newline git puts
	// between the header and the path list.
	var commits []rangeCommit
	tokens := bytes.Split(listing, []byte{0})
	for i := 0; i < len(tokens); i++ {
		if len(tokens[i]) > 0 || i+1 >= len(tokens) {
			continue
		}
		i++
		commit := rangeCommit{sha: string(tokens[i])}
		for i+1 < len(tokens) && len(tokens[i+1]) > 0 {
			i++
			path := tokens[i]
			if len(commit.paths) == 0 {
				path = bytes.TrimPrefix(path, []byte{'\n'})
			}
			commit.paths = append(commit.paths, string(path))
		}
		commits = append(commits, commit)
	}
	if len(commits) == 0 {
		return nil, nil
	}
	// The patches are read with the diff options patchIDOf reads the one whose
	// patch-id a row records with, so equal changes give equal ids. The
	// header is pinned because patch-id finds each commit by its "commit <sha>"
	// line, which a user's format.pretty would otherwise replace.
	args := append([]string{"log", "--no-merges", "--no-show-signature", "--format=" + patchHeader, "-p"}, patchDiffOptions...)
	patches, err := gitBytes(dir, nil, append(args, revs...)...)
	if err != nil {
		return commits, err
	}
	ids, err := gitBytes(dir, patches, "patch-id", "--stable")
	if err != nil {
		return commits, err
	}
	byCommit := make(map[string]string)
	for _, line := range strings.Split(string(ids), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 {
			byCommit[fields[1]] = fields[0]
		}
	}
	for i := range commits {
		commits[i].patchID = byCommit[commits[i].sha]
	}
	return commits, nil
}

// unaccountedCommits returns the shas of commits, in their order, that change
// something outside workFolder and that no row names. An empty commit changes
// nothing and no row can name it, since auto evidence refuses one, so it is
// never returned. Every row it is given counts, of any event and at any
// position, since a commit stays accounted for once any record names it; the
// caller passes only rows that fit a form, because an invalid row is no
// record, and of commit rows only those this clone's ledger attests, because a
// commit row that arrived by merge or was written by hand records nothing this
// clone did. A commit= or integrated= value names a commit when it is at least
// four hex characters and starts that commit's sha and no other's; a patch-id=
// value names the commits with that patch-id; and a row's base= and head= pair
// names the commits of base..head, which inRange lists. A no-commit verify
// cell quotes a failing command and its output, so the words of that quote
// name no commit, even ones that read as these pairs. inRange is called only
// while some commit is still unnamed.
func unaccountedCommits(commits []rangeCommit, rows []trailRow, workFolder string, inRange func(base, head string) map[string]bool) []string {
	return unnamedCommits(commits, rows, func(commit rangeCommit) bool {
		return len(commit.paths) > 0 && !onlyUnder(commit.paths, workFolder)
	}, inRange)
}

// unnamedCommits returns the shas of the commits, in their order, that keep
// reports and that no row names, by the naming rules unaccountedCommits
// documents. A prefix is matched against every commit, kept or not, so a
// value that starts two of them names neither. A quoted no-commit cell has no
// pairs, so it names no commit. Every word of any other cell is read, a bare
// base or head among them, which clears that value: skipping a bare word
// would let an earlier value form a range the cell does not state.
func unnamedCommits(commits []rangeCommit, rows []trailRow, keep func(rangeCommit) bool, inRange func(base, head string) map[string]bool) []string {
	var prefixes []string
	var ranges [][2]string
	patchIDs := make(map[string]bool)
	for _, row := range rows {
		var base, head string
		for _, token := range row.cell.pairs() {
			key, value := token.key, token.value
			switch key {
			case "commit", "integrated":
				if hexAtLeast4(value) {
					prefixes = append(prefixes, strings.ToLower(value))
				}
			case "patch-id":
				patchIDs[value] = true
			case "base":
				base = value
			case "head":
				head = value
			}
		}
		// Only hex values reach git, so a cell cannot pass it an option.
		if hexAtLeast4(base) && hexAtLeast4(head) {
			ranges = append(ranges, [2]string{base, head})
		}
	}
	named := make(map[string]bool)
	for _, value := range prefixes {
		match := ""
		for _, commit := range commits {
			if strings.HasPrefix(commit.sha, value) {
				if match != "" {
					match = ""
					break
				}
				match = commit.sha
			}
		}
		if match != "" {
			named[match] = true
		}
	}
	var open []string
	for _, commit := range commits {
		if !keep(commit) || named[commit.sha] || (commit.patchID != "" && patchIDs[commit.patchID]) {
			continue
		}
		open = append(open, commit.sha)
	}
	for _, r := range ranges {
		if len(open) == 0 {
			break
		}
		held := inRange(r[0], r[1])
		kept := open[:0]
		for _, sha := range open {
			if !held[sha] {
				kept = append(kept, sha)
			}
		}
		open = kept
	}
	return open
}

// hexAtLeast4 reports whether value is at least four hex digits, the shortest
// abbreviation git accepts.
func hexAtLeast4(value string) bool {
	if len(value) < 4 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// onlyUnder reports whether every path in paths lies inside folder, a
// repository-relative path with forward slashes. No paths is not "only under":
// a commit that changes nothing is empty, not an artifact commit.
func onlyUnder(paths []string, folder string) bool {
	if len(paths) == 0 || folder == "" {
		return false
	}
	prefix := strings.TrimSuffix(folder, "/") + "/"
	for _, path := range paths {
		if !strings.HasPrefix(path, prefix) {
			return false
		}
	}
	return true
}
