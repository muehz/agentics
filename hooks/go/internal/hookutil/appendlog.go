package hookutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// afterOwnedCheck is a test seam: it runs between ownedLogPath's refusal
// check and the os.Root open, so a test can swap the leaf for an in-root
// link right before the open to prove the post-open SameFile check below
// catches what os.Root would otherwise follow.
var afterOwnedCheck = func() {}

// appendOwnedLog appends line to leaf inside dir under the ownership rules
// every hook log shares (#586). ownedLogPath's link refusal runs first; the
// create and open then go through an os.Root, so a parent swapped for a
// link after the walk cannot carry the write out of the root. Directories
// the hook creates are 0700, the log is 0600, and an existing log with
// wider bits is tightened.
func appendOwnedLog(trusted, dir, leaf string, line []byte) error {
	if _, err := ownedLogPath(trusted, dir, leaf); err != nil {
		return err
	}
	rootDir, rel := filepath.Clean(filepath.FromSlash(dir)), "."
	trustedClean := filepath.Clean(filepath.FromSlash(trusted))
	if r, err := filepath.Rel(trustedClean, rootDir); err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		rootDir, rel = trustedClean, r
	} else if err := os.MkdirAll(rootDir, 0o700); err != nil {
		return err
	}
	afterOwnedCheck()
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return err
	}
	defer root.Close()
	if rel != "." {
		if err := root.MkdirAll(rel, 0o700); err != nil {
			return aliasOr(err)
		}
	}
	name := filepath.Join(rel, leaf)
	handle, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return aliasOr(err)
	}
	// os.Root follows a leaf link whose target stays inside the root, so the
	// opened file must be the leaf itself: a regular file, not a link that
	// was swapped in after ownedLogPath's Lstat.
	opened, statErr := handle.Stat()
	leafInfo, lstatErr := root.Lstat(name)
	if statErr != nil || lstatErr != nil || !leafInfo.Mode().IsRegular() || !os.SameFile(opened, leafInfo) {
		handle.Close()
		return ErrLogAlias
	}
	if runtime.GOOS != "windows" && opened.Mode().Perm()&0o077 != 0 {
		_ = handle.Chmod(0o600) // fchmod: never follows a link
	}
	_, writeErr := handle.Write(line)
	closeErr := handle.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// aliasOr maps os.Root's escape refusal onto ErrLogAlias. The match is on
// error text because os does not export its path-escape error, so the text
// is the only stable signal available to this package.
func aliasOr(err error) error {
	if strings.Contains(err.Error(), "path escapes from parent") {
		return ErrLogAlias
	}
	return err
}
