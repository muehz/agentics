package hookutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrLogAlias is the sentinel an append refuses with when a link or a
// non-regular guard.log would redirect the write. The error GuardLog.Append
// returns carries the guard's own sentence and still satisfies errors.Is
// against it.
var ErrLogAlias = errors.New("log path is redirected through a link")

// ownedLogPath returns the path of leaf inside dir that a hook may append
// to, or ErrLogAlias when a link could redirect the write. When dir sits
// inside trusted (a repository root or the process working directory),
// every path component from trusted down to dir must be a real directory:
// a symlink or a Windows junction/mount point (ModeIrregular) is refused,
// and a component that does not exist yet is fine because MkdirAll makes
// a real directory. The leaf itself may never be anything but a regular
// file, wherever dir is. A dir outside trusted keeps its parents
// unchecked: that configuration is the operator's, not the repository's.
//
// The parent walk is a point-in-time check: it refuses a link that exists
// when it runs, not one swapped in between this call and the caller's
// MkdirAll/OpenFile. The leaf check is the Lstat here plus the SameFile
// comparison appendOwnedLog makes after it opens the file, which refuses a
// leaf swapped for a link in between. appendOwnedLog closes the parent race
// on every platform with a handle-scoped design (os.Root), and it is the
// only caller (#586, closing the residual note on #500 F2).
func ownedLogPath(trusted, dir, leaf string) (string, error) {
	trustedClean := filepath.Clean(filepath.FromSlash(trusted))
	dirClean := filepath.Clean(filepath.FromSlash(dir))
	if rel, err := filepath.Rel(trustedClean, dirClean); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		current := trustedClean
		for _, component := range strings.Split(rel, string(filepath.Separator)) {
			if component == "." || component == "" {
				continue
			}
			current = filepath.Join(current, component)
			info, err := os.Lstat(current)
			if err != nil {
				break // not created yet: MkdirAll makes a real directory
			}
			if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				return "", ErrLogAlias
			}
		}
	}
	path := dirClean + string(filepath.Separator) + leaf
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return "", ErrLogAlias
	}
	return path, nil
}
