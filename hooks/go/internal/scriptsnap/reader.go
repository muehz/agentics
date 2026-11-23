package scriptsnap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"

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
	if final != path {
		if refusal, refused := h.checkResolved(path, final); refused {
			return refusal, true
		}
	}
	if byFinal, err := os.Stat(final); err != nil || !os.SameFile(opened, byFinal) {
		return refuse(path, shellwords.RuleChanging, "path resolves to a different file than the one opened"), true
	}
	return shellwords.ScriptRead{}, false
}

func currentHooks() readHooks {
	return readHooks{afterLstat: afterLstat, afterOpen: afterOpen, beforeRead: beforeRead, fileID: fileIDFunc}
}

// readFile reads at most max bytes of one regular file. It trusts no stat size and never executes the file.
//
// The path is classified with Lstat so a final symlink is refused, then opened without blocking or following a link.
// The opened handle is compared with the Lstat result to catch a path swapped in between, but that comparison is
// best effort: a filesystem that reuses an inode for a delete and recreate passes it. Every later check therefore
// binds to the opened handle, never to the path: its kind, its identity, and, after the read, its size and
// modification time and whether the path still names it, so bytes that may mix two versions are never returned.
func readFile(path string, max int, h readHooks) shellwords.ScriptRead {
	if refusal, refused := h.checkPath(path); refused {
		return refusal
	}
	info, err := os.Lstat(path)
	if err != nil {
		return refuse(path, shellwords.RuleUnreadable, "not a readable regular file")
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
		return refuse(path, shellwords.RuleTooLarge, "exceeds the per-script limit")
	}
	if reason := contentGate(data); reason != "" {
		return refuse(path, shellwords.RuleEncoding, reason)
	}
	sum := sha256.Sum256(data)
	return shellwords.ScriptRead{Path: path, ID: id, Text: string(data), SHA256: hex.EncodeToString(sum[:]), Bytes: len(data)}
}

// contentGate admits only plain LF text: valid UTF-8, no NUL, no CR, no byte-order mark.
func contentGate(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}), bytes.HasPrefix(b, []byte{0xFF, 0xFE}), bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return "byte-order mark"
	case bytes.IndexByte(b, 0) >= 0:
		return "NUL byte"
	case bytes.IndexByte(b, '\r') >= 0:
		return "carriage return"
	case !utf8.Valid(b):
		return "invalid UTF-8"
	}
	return ""
}
