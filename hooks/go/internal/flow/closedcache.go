package flow

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The closed-flow cache keeps lookup's cost flat as closed work folders
// accumulate. It names each flow found closed, in either store, by the
// absolute path, size and modification time of its state.tsv; a file whose
// size and time are unchanged is skipped unread. Any append to state.tsv
// changes both, so a flow reopened by a later row is read again. A committed
// flow recorded as closed by the
// verdict at the default branch's tip stays closed while its state.tsv is
// unchanged, even if the default branch later moves. It lives in the Git
// directory, which Git neither commits nor lists, and is only ever a hint: a
// missing or unreadable cache means every file is read.

// fileStamp is the size and modification time a cache line records.
type fileStamp struct {
	size, mtime int64
}

// closedCache is the cache of one repository, or a cache with no file when no
// common directory resolves.
type closedCache struct {
	file    string
	entries map[string]fileStamp
	added   []string
}

func closedCacheFile(common string) string {
	return filepath.Join(common, "agentics", "flow", "closed.tsv")
}

// loadClosedCache reads the cache in the common Git directory common. A line is
// dead when it does not parse, when a later line names the same path, since
// the last line for a path wins, or when its path no longer exists, as after a
// worktree is removed. Dead lines are not loaded, and when they make up half
// the lines or more, the file is compacted to the live lines.
func loadClosedCache(common string) *closedCache {
	cache := &closedCache{entries: make(map[string]fileStamp)}
	if common == "" {
		return cache
	}
	cache.file = closedCacheFile(common)
	body, err := os.ReadFile(cache.file)
	if err != nil {
		return cache
	}
	type parsed struct {
		line, path string
		stamp      fileStamp
	}
	var lines []parsed
	last := make(map[string]int)
	total := 0
	for _, line := range strings.Split(string(body), "\n") {
		if line == "" {
			continue
		}
		total++
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || fields[0] == "" {
			continue
		}
		size, sizeErr := strconv.ParseInt(fields[1], 10, 64)
		mtime, timeErr := strconv.ParseInt(fields[2], 10, 64)
		if sizeErr != nil || timeErr != nil {
			continue
		}
		last[fields[0]] = len(lines)
		lines = append(lines, parsed{line, fields[0], fileStamp{size, mtime}})
	}
	var live []string
	for i, entry := range lines {
		if last[entry.path] != i {
			continue
		}
		// A stat costs no Git process and no state.tsv read, so checking every
		// line keeps the cost lookup bounds flat.
		if _, err := os.Stat(entry.path); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		cache.entries[entry.path] = entry.stamp
		live = append(live, entry.line+"\n")
	}
	if dead := total - len(live); dead > 0 && 2*dead >= total {
		cache.compact(live)
	}
	return cache
}

// compact replaces the cache file with live by writing a temporary file beside
// it and renaming that over it, so that a reader sees the old file or the new
// one, never a part. A line another lookup appends between this read and the
// rename is lost; that is accepted, since the next lookup reads that file
// again and records it anew. Any failure keeps the old file, which is only a
// cache.
func (c *closedCache) compact(live []string) {
	tmp, err := os.CreateTemp(filepath.Dir(c.file), "closed-*.tmp")
	if err != nil {
		return
	}
	writeErr := writeStateBytes(tmp, []byte(strings.Join(live, "")))
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil || os.Rename(tmp.Name(), c.file) != nil {
		_ = os.Remove(tmp.Name())
	}
}

// stampOf returns the cache key of the state.tsv at path: its absolute path
// and its current stamp, and false when it cannot be taken.
func stampOf(path string) (string, fileStamp, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fileStamp{}, false
	}
	info, err := os.Stat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return "", fileStamp{}, false
	}
	return filepath.Clean(abs), fileStamp{info.Size(), info.ModTime().UnixNano()}, true
}

// closed reports whether the cache records the file at abs with stamp.
func (c *closedCache) closed(abs string, stamp fileStamp) bool {
	if c.file == "" {
		return false
	}
	have, ok := c.entries[abs]
	return ok && have == stamp
}

// record queues abs with stamp for save. The stamp is the one taken before the
// file was read, so a write during the read leaves a stamp that no longer
// matches and the file is read again.
func (c *closedCache) record(abs string, stamp fileStamp) {
	// A separator inside the path would split the line, which a later load
	// would then misread.
	if c.file == "" || strings.ContainsAny(abs, "\t\r\n") {
		return
	}
	c.entries[abs] = stamp
	c.added = append(c.added, abs+"\t"+strconv.FormatInt(stamp.size, 10)+"\t"+strconv.FormatInt(stamp.mtime, 10)+"\n")
}

// save appends the recorded lines with one write on an append handle, as
// appendLedger does, so that concurrent lookups never interleave a line. A
// failure is ignored: the next lookup reads the files again.
func (c *closedCache) save() {
	if len(c.added) == 0 {
		return
	}
	lines := strings.Join(c.added, "")
	c.added = nil
	if err := os.MkdirAll(filepath.Dir(c.file), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(c.file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return
	}
	_ = writeStateBytes(f, []byte(lines))
	_ = f.Close()
}
