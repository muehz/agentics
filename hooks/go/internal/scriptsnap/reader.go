package scriptsnap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func refuse(path string, limit shellwords.Rule, detail string) shellwords.ScriptRead {
	return shellwords.ScriptRead{Path: path, Limit: limit, Detail: detail}
}

// Test hooks. They are read once on the caller's goroutine and passed by value, so a test that resets them cannot
// race with a reader that outlived its ceiling.
var (
	afterLstat = func() {} // runs between the Lstat and the open
	afterOpen  = func() {} // runs between the open and the read
	beforeRead = func() {} // runs on the calling goroutine before the read starts
)

// fileIDFunc is replaceable so a test can stand in for a filesystem with a missing or constant file index.
var fileIDFunc = fileID

type readHooks struct {
	afterLstat, afterOpen, beforeRead func()
	fileID                            func(*os.File, os.FileInfo) (string, bool)
	policy                            func(path string) string // nil allows every path
}

// checkPath refuses a path that must not be read: a hazardous spelling, a drive path on a host that has no drives,
// or a path the policy protects. It runs on the lexical path and on the symlink-resolved one, so a link into a
// protected directory is refused whether or not the final file exists.
func (h readHooks) checkPath(path string) (shellwords.ScriptRead, bool) {
	if problem := pathshape.LocalFileProblem(path); problem != pathshape.NoProblem {
		return refuse(path, shellwords.RuleUnreadable, string(problem)), true
	}
	if pathshape.Current().DrivePathOffHost(path) {
		return refuse(path, shellwords.RuleUnreadable, "drive path on a non-Windows host"), true
	}
	if h.policy != nil && h.policy(path) != "" {
		return refuse(path, shellwords.RulePathProtected, protectedDetail), true
	}
	resolved, ok := resolveExisting(path)
	if !ok || resolved == path {
		return shellwords.ScriptRead{}, false
	}
	return h.checkResolved(path, resolved)
}

// checkResolved applies the spelling and policy checks to the symlink-resolved form of path.
func (h readHooks) checkResolved(path, resolved string) (shellwords.ScriptRead, bool) {
	if problem := pathshape.LocalFileProblem(resolved); problem != pathshape.NoProblem {
		return refuse(path, shellwords.RuleUnreadable, string(problem)), true
	}
	if h.policy != nil && h.policy(resolved) != "" {
		return refuse(path, shellwords.RulePathProtected, protectedDetail), true
	}
	return shellwords.ScriptRead{}, false
}

const protectedDetail = "protected or credential path"

// checkOpened judges the file behind an open handle against the path policy and the path shape rules.
func (h readHooks) checkOpened(path string, f *os.File, opened os.FileInfo) (shellwords.ScriptRead, bool) {
	named, byHandle, err := handlePath(f)
	if err != nil {
		return refuse(path, shellwords.RuleUnreadable, "path of the open file unavailable"), true
	}
	if byHandle {
		return h.checkResolved(path, named)
	}
	final, err := filepath.EvalSymlinks(path)
	if err != nil {
		return refuse(path, shellwords.RuleChanging, "path no longer resolves"), true
	}
	if refusal, refused := h.checkResolved(path, final); refused {
		return refusal, true
	}
	if byFinal, err := os.Stat(final); err != nil || !os.SameFile(opened, byFinal) {
		return refuse(path, shellwords.RuleChanging, "path resolves to a different file than the one opened"), true
	}
	return shellwords.ScriptRead{}, false
}

func currentHooks() readHooks {
	return readHooks{afterLstat: afterLstat, afterOpen: afterOpen, beforeRead: beforeRead, fileID: fileIDFunc}
}

// readFile reads at most max bytes of one regular file into Raw and Text without dialect-specific content filtering,
// peeking at most 4 KiB for oversized files and symlink targets. It trusts no stat size and never executes the file.
//
// The path is classified with Lstat after lexical and resolved policy checks. A final symlink is refused as unreadable
// while recording IsSymlink and, when its final target is a stable regular file that clears the path policy, a 4 KiB
// ResolvedHeader peek. A regular file is opened without blocking or following a link and compared with the Lstat
// result to catch a swap in between; because inode reuse can pass that comparison, every subsequent check binds to the
// opened handle—its kind, its identity, and, after the read or header peek, its size, modification time, and whether
// the path still names it—so bytes or headers that may mix two versions are never returned.
func readFile(path string, max int, h readHooks) shellwords.ScriptRead {
	if refusal, refused := h.checkPath(path); refused {
		return refusal
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return shellwords.ScriptRead{
			Path:     path,
			Limit:    shellwords.RuleUnreadable,
			Detail:   "not a readable regular file",
			NotExist: true,
		}
	}
	if err != nil {
		return refuse(path, shellwords.RuleUnreadable, "not a readable regular file")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		r := refuse(path, shellwords.RuleUnreadable, "not a regular file")
		r.IsSymlink = true
		header, ok, protectedRefusal := peekSymlinkHeader(path, h)
		if protectedRefusal != nil {
			return *protectedRefusal
		}
		if ok {
			r.ResolvedHeader = header
			r.HasResolvedHeader = true
		}
		return r
	}
	if !info.Mode().IsRegular() {
		return refuse(path, shellwords.RuleUnreadable, "not a regular file")
	}
	// On Windows a path-based FileInfo resolves its file identity lazily, by reopening the path at the first
	// comparison. Comparing it with itself fixes the identity now, so a later swap cannot be read back as the file the
	// Lstat saw.
	if !os.SameFile(info, info) {
		return refuse(path, shellwords.RuleUnreadable, "no stable file identity")
	}
	h.afterLstat()
	f, err := openNoBlock(path)
	if err != nil {
		return refuse(path, shellwords.RuleUnreadable, "open failed")
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return refuse(path, shellwords.RuleUnreadable, "stat failed")
	}
	// Not redundant with the Lstat check: when the filesystem reuses an inode for a delete and recreate, a swapped-in
	// FIFO passes the SameFile comparison below, and this check is the only thing that refuses it.
	if !before.Mode().IsRegular() {
		return refuse(path, shellwords.RuleUnreadable, "not a regular file")
	}
	if !os.SameFile(info, before) {
		return refuse(path, shellwords.RuleChanging, "file changed while it was opened")
	}
	id, ok := h.fileID(f, before)
	if !ok {
		return refuse(path, shellwords.RuleUnreadable, "no stable file identity")
	}
	h.afterOpen()
	// A directory link swapped after the first check can leave the handle on a file now reached through a protected
	// or hazardous path. Where the system names the file behind a handle (Windows), that name is judged: it is the
	// file that will be read, and it sees through junctions, which path resolution cannot follow there. Elsewhere
	// the path is resolved again and must still name the opened file.
	if refusal, refused := h.checkOpened(path, f, before); refused {
		return refusal
	}
	if before.Size() > int64(max) {
		head, err := io.ReadAll(io.LimitReader(f, headerPeekBytes))
		if err != nil {
			return refuse(path, shellwords.RuleUnreadable, "read failed")
		}
		after, err := f.Stat()
		byPath, pathErr := os.Stat(path)
		if err != nil || pathErr != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !os.SameFile(before, byPath) {
			return refuse(path, shellwords.RuleChanging, "file changed while it was read")
		}
		return shellwords.ScriptRead{
			Path:     path,
			Limit:    shellwords.RuleTooLarge,
			Detail:   "exceeds the per-script limit",
			TooLarge: true,
			Header:   inspectHeader(head),
		}
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil {
		return refuse(path, shellwords.RuleUnreadable, "read failed")
	}
	after, err := f.Stat()
	byPath, pathErr := os.Stat(path)
	if err != nil || pathErr != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !os.SameFile(before, byPath) {
		return refuse(path, shellwords.RuleChanging, "file changed while it was read")
	}
	if len(data) > max {
		return shellwords.ScriptRead{
			Path:     path,
			Limit:    shellwords.RuleTooLarge,
			Detail:   "exceeds the per-script limit",
			TooLarge: true,
			Header:   inspectHeader(data),
		}
	}
	sum := sha256.Sum256(data)
	raw := string(data)
	return shellwords.ScriptRead{
		Path:   path,
		ID:     id,
		Raw:    raw,
		Text:   raw,
		SHA256: hex.EncodeToString(sum[:]),
		Bytes:  len(data),
		Header: inspectHeader(data),
	}
}

func peekSymlinkHeader(path string, h readHooks) (shellwords.ScriptHeader, bool, *shellwords.ScriptRead) {
	resolved, ok := resolveExisting(path)
	if !ok {
		return shellwords.ScriptHeader{}, false, nil
	}
	if refusal, refused := h.checkResolved(path, resolved); refused {
		if refusal.Limit == shellwords.RulePathProtected {
			refusal.Path = path
			return shellwords.ScriptHeader{}, false, &refusal
		}
		return shellwords.ScriptHeader{}, false, nil
	}
	targetInfo, err := os.Lstat(resolved)
	if err != nil || !targetInfo.Mode().IsRegular() || !os.SameFile(targetInfo, targetInfo) {
		return shellwords.ScriptHeader{}, false, nil
	}
	h.afterLstat()
	f, err := openNoBlock(resolved)
	if err != nil {
		return shellwords.ScriptHeader{}, false, nil
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() || !os.SameFile(targetInfo, before) {
		return shellwords.ScriptHeader{}, false, nil
	}
	h.afterOpen()
	if refusal, refused := h.checkOpened(resolved, f, before); refused {
		if refusal.Limit == shellwords.RulePathProtected {
			refusal.Path = path
			return shellwords.ScriptHeader{}, false, &refusal
		}
		return shellwords.ScriptHeader{}, false, nil
	}
	head, err := io.ReadAll(io.LimitReader(f, headerPeekBytes))
	if err != nil {
		return shellwords.ScriptHeader{}, false, nil
	}
	after, err := f.Stat()
	byTarget, tErr := os.Stat(resolved)
	byLink, lErr := os.Stat(path)
	if err != nil || tErr != nil || lErr != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !os.SameFile(before, byTarget) || !os.SameFile(before, byLink) {
		return shellwords.ScriptHeader{}, false, nil
	}
	return inspectHeader(head), true, nil
}
