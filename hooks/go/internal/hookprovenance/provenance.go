// Package hookprovenance computes and validates the provenance signals for
// the committed hook binaries in hooks/bin/: the source digest, the pinned
// toolchain, and the checksum manifest. The source digest covers the source
// the binaries are built from: go.mod, the digest-exclude list when present,
// and every non-test .go file outside bin/ and outside each directory that
// list names. A _test.go file never reaches the built binary, and a listed
// directory holds code that ships in no hook binary, so neither is part of the
// digest that vouches for one. This package is the digest's one
// implementation: hooks/go/cmd/sourcedigest prints SourceDigest and
// SourceFiles for the Makefile that stamps the binaries, and the conformance
// freshness rule checks the stamp against the same functions.
package hookprovenance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// goDirectivePattern reads go.mod's `go <version>` line. It accepts two or
// three components so the directive's own precision can set the toolchain
// pin's precision (see PinnedToolchain).
var goDirectivePattern = regexp.MustCompile(`(?m)^go[ \t]+([0-9]+(?:\.[0-9]+){1,2})[ \t]*$`)

// FaultKind names what is wrong with one line of the checksum manifest.
type FaultKind int

// The fault kinds, in precedence order: a line carrying several is reported
// with the first that applies.
const (
	// FaultCarriageReturn is a line holding a carriage return anywhere.
	FaultCarriageReturn FaultKind = iota + 1
	// FaultBlank is a line with no field: empty, or only spaces and tabs.
	FaultBlank
	// FaultFieldCount is a line with one field, or three or more.
	FaultFieldCount
	// FaultHashNotHex is a hash that is not 64 hex digits of either case.
	FaultHashNotHex
	// FaultHashNotLower is a 64-digit hex hash with an uppercase digit.
	FaultHashNotLower
	// FaultDuplicate is a path an earlier fault-free line already recorded.
	FaultDuplicate
)

// LineFault is the one fault of one manifest line. Line is 1-based. Path is
// the line's second field, or "" when it has fewer than two.
type LineFault struct {
	Line int
	Kind FaultKind
	Path string
}

// Manifest is hooks/SHA256SUMS read once, line by line: the path-to-hash
// entries the dispatchers would resolve, and every line's fault. Callers
// choose their own posture for the faults; the zero Manifest has no entries.
//
// Lines are the pieces between LF bytes. An empty piece after the final LF is
// not a line; a non-empty last piece without one is. Fields split on space and
// tab only, as run-hook.cmd's awk splits (#675), so a U+00A0 stays inside a
// path. Each line carries at most one fault, the first of carriage return,
// blank, field count other than two, hash that is not 64 hex digits of either
// case, hash with an uppercase digit, and path already recorded by an earlier
// fault-free line.
//
// Hash and Paths read every line with two or more fields, whatever its fault.
// The first entry for a path wins and its hash is lowercased, as
// hooks/run-hook.cmd and hooks/fetch-hook.ps1 resolve a duplicate, so the audit
// and the installer never disagree about which line vouches for a binary.
type Manifest struct {
	hashes map[string]string
	paths  []string
	faults []LineFault
	// carriageLine is the first line holding a carriage return, or 0.
	carriageLine int
}

// LoadManifest reads the manifest at path. It fails only when the file cannot
// be read, with the error "reading the checksum manifest: <cause>". A file
// that reads but holds nothing usable is a Manifest whose Unusable is
// non-nil, never an error, so an unreadable file cannot be mistaken for an
// empty one.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("reading the checksum manifest: %w", err)
	}
	pieces := strings.Split(string(data), "\n")
	if pieces[len(pieces)-1] == "" {
		pieces = pieces[:len(pieces)-1]
	}
	m := Manifest{hashes: map[string]string{}}
	recorded := map[string]bool{}
	for n, line := range pieces {
		fields := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' })
		entry := ""
		if len(fields) >= 2 {
			entry = fields[1]
			if _, seen := m.hashes[entry]; !seen {
				m.hashes[entry] = strings.ToLower(fields[0])
				m.paths = append(m.paths, entry)
			}
		}
		kind := lineFault(line, fields, recorded)
		if kind == 0 {
			recorded[entry] = true
			continue
		}
		if kind == FaultCarriageReturn && m.carriageLine == 0 {
			m.carriageLine = n + 1
		}
		m.faults = append(m.faults, LineFault{Line: n + 1, Kind: kind, Path: entry})
	}
	return m, nil
}

// lineFault returns the one fault of line, or 0 when it has none. recorded
// holds the paths of the earlier fault-free lines.
func lineFault(line string, fields []string, recorded map[string]bool) FaultKind {
	switch {
	case strings.ContainsRune(line, '\r'):
		return FaultCarriageReturn
	case len(fields) == 0:
		return FaultBlank
	case len(fields) != 2:
		return FaultFieldCount
	case !isHexDigest(fields[0]):
		return FaultHashNotHex
	case strings.ContainsAny(fields[0], "ABCDEF"):
		return FaultHashNotLower
	case recorded[fields[1]]:
		return FaultDuplicate
	}
	return 0
}

// isHexDigest reports whether s is 64 hex digits of either case.
func isHexDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return false
		}
	}
	return true
}

// Hash returns the recorded hash for path, lowercased, and whether the
// manifest has an entry for it.
func (m Manifest) Hash(path string) (string, bool) {
	hash, ok := m.hashes[path]
	return hash, ok
}

// Paths returns each recorded path once, in order of first appearance.
func (m Manifest) Paths() []string {
	return slices.Clone(m.paths)
}

// Faults returns every faulted line, in line order.
func (m Manifest) Faults() []LineFault {
	return slices.Clone(m.faults)
}

// Unusable reports why the manifest cannot vouch for any binary: the first
// line carrying a carriage return, else "the checksum manifest has no
// entries". It is nil exactly when the manifest vouches for at least one path.
func (m Manifest) Unusable() error {
	if m.carriageLine != 0 {
		return fmt.Errorf("the checksum manifest line %d carries a carriage return", m.carriageLine)
	}
	if len(m.paths) == 0 {
		return fmt.Errorf("the checksum manifest has no entries")
	}
	return nil
}

// PinnedToolchain reads hooks/go/go.mod's `go` directive and returns it in the
// linker's spelling ("go1.26"). The directive IS the pin, and its precision is
// the pin's precision — see ToolchainSatisfiesPin — so tightening the check to
// an exact patch release is a one-line go.mod edit and no code change.
func PinnedToolchain(hooksDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(hooksDir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("reading go.mod for the toolchain pin: %w", err)
	}
	match := goDirectivePattern.FindSubmatch(data)
	if match == nil {
		return "", fmt.Errorf("hooks/go/go.mod carries no `go <version>` directive — the toolchain check has nothing to compare against")
	}
	return "go" + string(match[1]), nil
}

// ToolchainSatisfiesPin reports whether a linker-recorded Go version answers
// the go.mod pin. `go 1.26` accepts every 1.26 patch release; `go 1.26.6`
// accepts only that one. A prerelease (go1.26rc1) satisfies neither, which is
// deliberate: a release candidate is a different toolchain, and committed
// binaries should not come from one.
func ToolchainSatisfiesPin(recorded, pin string) bool {
	return recorded == pin || strings.HasPrefix(recorded, pin+".")
}

// digestExcludeFile names the exclusion list, read from the module root
// beside go.mod.
const digestExcludeFile = "digest-exclude"

// ExcludeList reads and validates dir/digest-exclude and returns its entries
// in file order. A missing file is an empty list. The file splits on LF; a
// blank line and a line whose first byte is # are ignored. Every other line
// is an entry and must be a clean, relative, slash-separated path naming an
// existing directory under dir, with no whitespace, backslash, colon, or ..
// element. Any other line, and any line carrying a carriage return, is an
// error naming its line number, so a typo can never silently stop excluding a
// directory.
func ExcludeList(dir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, digestExcludeFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", digestExcludeFile, err)
	}
	var entries []string
	for n, line := range strings.Split(string(data), "\n") {
		if strings.ContainsRune(line, '\r') {
			return nil, fmt.Errorf("%s line %d carries a carriage return", digestExcludeFile, n+1)
		}
		if line == "" || line[0] == '#' {
			continue
		}
		if problem := excludeEntryProblem(dir, line); problem != "" {
			return nil, fmt.Errorf("%s line %d %q %s", digestExcludeFile, n+1, line, problem)
		}
		entries = append(entries, line)
	}
	return entries, nil
}

// excludeEntryProblem returns why line is not a valid entry, or "" when it is.
// The checks run in a fixed order, so a line with several problems is always
// rejected for the same one.
func excludeEntryProblem(dir, line string) string {
	switch {
	case strings.ContainsAny(line, " \t"):
		return "contains whitespace"
	case strings.ContainsRune(line, '\\'):
		return "contains a backslash; separate with /"
	case strings.HasPrefix(line, "/") || strings.ContainsRune(line, ':'):
		return "is absolute; name a directory relative to the module root"
	case slices.Contains(strings.Split(line, "/"), ".."):
		return "contains a .. element"
	case line == "." || line != path.Clean(line):
		return "is not a clean path (no ., //, leading ./ or trailing /)"
	}
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(line)))
	if err != nil || !info.IsDir() {
		return "names no directory under the module root"
	}
	return ""
}

// SourceFiles returns the files SourceDigest hashes, as slash-separated paths
// relative to dir sorted byte-wise: go.mod, digest-exclude when present, and
// every non-test .go file outside bin/ and outside every directory the list
// names. A malformed list returns the ExcludeList error.
func SourceFiles(dir string) ([]string, error) {
	entries, err := ExcludeList(dir)
	if err != nil {
		return nil, err
	}
	pruned := map[string]bool{filepath.Join(dir, "bin"): true}
	for _, entry := range entries {
		pruned[filepath.Join(dir, filepath.FromSlash(entry))] = true
	}
	files := []string{"go.mod"}
	if _, err := os.Stat(filepath.Join(dir, digestExcludeFile)); err == nil {
		files = append(files, digestExcludeFile)
	}
	err = filepath.WalkDir(dir, func(walked string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// bin/ holds the committed build outputs, never source, and a
			// listed directory ships in no hook binary. Pruning by whole
			// directory path keeps a listed a/b from excluding a sibling a/bc.
			if pruned[walked] {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
			rel, relErr := filepath.Rel(dir, walked)
			if relErr != nil {
				return relErr
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files) // byte-wise, independent of any locale's collation
	return files, nil
}

// SourceDigest computes the deterministic digest of the files SourceFiles
// returns: the SHA-256 of one "<path> <content sha256>  -" line per file, in
// SourceFiles order, with each file's CRLF pairs folded to LF before hashing.
func SourceDigest(dir string) (string, error) {
	files, err := SourceFiles(dir)
	if err != nil {
		return "", fmt.Errorf("source digest: %w", err)
	}

	outer := sha256.New()
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file)))
		if err != nil {
			return "", fmt.Errorf("source digest: %w", err)
		}
		// CRLF pairs fold to LF so a CRLF checkout digests the same as an LF
		// one. A Windows checkout with core.autocrlf materializes the same
		// source as CRLF, and without the fold every committed binary reads
		// as stale there (observed on windows-latest run 31662681572).
		//
		// Only the CRLF pair is folded; a lone CR stays hashed. autocrlf
		// converts only between CRLF and LF and never introduces a lone CR,
		// so folding pairs cancels the conversion exactly, while a CR inside
		// a raw string literal is content whose removal must change the
		// digest.
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		sum := sha256.Sum256(data)
		fmt.Fprintf(outer, "%s %s  -\n", file, hex.EncodeToString(sum[:]))
	}
	return hex.EncodeToString(outer.Sum(nil)), nil
}
