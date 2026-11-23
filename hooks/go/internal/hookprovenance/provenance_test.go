package hookprovenance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// hooksModuleRel is the repo-relative path of the hook module SourceDigest
// walks. It is the same value as engineRel in
// internal/conformance/module.go; the two are independent constants
// because this package must not import the conformance tree, so a move of
// hooks/go has to update both.
const hooksModuleRel = "hooks/go"

// The fixture below is a smaller, independent stand-in for hooks/go — not a
// mirror of the one in internal/conformance/tinyhook_test.go, whose
// module is built and executed. SourceDigest only walks and hashes .go files,
// so a runnable dispatcher body is irrelevant here and the fixture is kept
// deliberately minimal. The names carry a prov prefix so the two fixtures
// cannot be mistaken for each other.
const (
	provTinyGoMod = `module example.com/tinyhook

go 1.27.1
`
	provTinyProviderGo = `package provider
`
	provTinyMainGo = `package main

func main() {}
`
)

// writeProvTinyModule materializes the fixture under root, using the
// repo-relative layout SourceDigest expects: hooks/go/go.mod and
// hooks/go/**/*.go. It takes t so a failed write fails here rather than
// surfacing later as a confusing digest mismatch.
func writeProvTinyModule(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"hooks/go/go.mod":                        provTinyGoMod,
		"hooks/go/cmd/hook/main.go":              provTinyMainGo,
		"hooks/go/internal/provider/provider.go": provTinyProviderGo,
	}
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPinnedToolchainReadsTheGoDirective(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ mod, want string }{
		{"module x\n\ngo 1.26\n", "go1.26"},
		{"module x\n\ngo 1.27.0\n", "go1.27.0"},
		{"module x\n\ngo\t1.26\n", "go1.26"},
	} {
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(c.mod), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := PinnedToolchain(dir)
		if err != nil {
			t.Fatalf("%q: %v", c.mod, err)
		}
		if got != c.want {
			t.Errorf("%q: PinnedToolchain = %q, want %q", c.mod, got, c.want)
		}
	}
	// A go.mod with no directive leaves the check nothing to compare against;
	// silently passing every binary would be worse than refusing to audit.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PinnedToolchain(dir); err == nil {
		t.Fatal("expected an error when go.mod carries no go directive")
	}
}

// The go.mod directive sets the pin's precision, so the comparison has to
// honour two and three components alike — and must never accept a version
// that merely shares a prefix.
func TestToolchainSatisfiesPin(t *testing.T) {
	cases := []struct {
		recorded, pin string
		want          bool
	}{
		{"go1.26.6", "go1.26", true},
		{"go1.26", "go1.26", true},
		{"go1.26.6", "go1.26.6", true},
		{"go1.26.7", "go1.26.6", false},
		{"go1.26.6", "go1.26.6.1", false},
		{"go1.25.6", "go1.26", false},
		{"go1.260.1", "go1.26", false},
		{"go1.26rc1", "go1.26", false},
		{"devel go1.27-abcdef", "go1.26", false},
	}
	for _, c := range cases {
		if got := ToolchainSatisfiesPin(c.recorded, c.pin); got != c.want {
			t.Errorf("ToolchainSatisfiesPin(%q, %q) = %v, want %v", c.recorded, c.pin, got, c.want)
		}
	}
}

// A CRLF checkout of identical source must digest identically to the LF
// checkout — the invariant that keeps committed binaries from reading as
// stale on Windows (core.autocrlf), observed live on windows-latest run
// 31662681572.
func TestSourceDigestIsLineEndingInvariant(t *testing.T) {
	lf := t.TempDir()
	writeProvTinyModule(t, lf)
	crlf := t.TempDir()
	writeProvTinyModule(t, crlf)
	for _, rel := range []string{"hooks/go/go.mod", "hooks/go/cmd/hook/main.go", "hooks/go/internal/provider/provider.go"} {
		path := filepath.Join(crlf, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		mangled := strings.ReplaceAll(string(data), "\n", "\r\n")
		if mangled == string(data) {
			t.Fatalf("%s gained no CRLF — the mangle is inert", rel)
		}
		if err := os.WriteFile(path, []byte(mangled), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, err := SourceDigest(filepath.Join(lf, hooksModuleRel))
	if err != nil {
		t.Fatal(err)
	}
	b, err := SourceDigest(filepath.Join(crlf, hooksModuleRel))
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("digest differs across line endings: lf=%s crlf=%s", a, b)
	}
}

// TestSourceDigestKeepsStandaloneCarriageReturns is the other half of the
// contract above, and the two are only meaningful together. Line-ending
// invariance folds the CRLF pair; it must not delete a lone CR, because
// autocrlf never introduces one, so a lone CR is content. A .go file carrying
// a literal carriage return — inside a raw string, say — must digest
// differently from the same file without it, or an edit to that content would
// leave a binary built before it looking current.
//
// The assertion is that removing the CR changes the digest. Folding pairs
// only satisfies both tests; stripping every CR passes the invariance test
// above and fails this one; hashing raw bytes passes this one and fails that
// one. Neither mutation survives the pair.
func TestSourceDigestKeepsStandaloneCarriageReturns(t *testing.T) {
	const rel = "hooks/go/internal/provider/provider.go"
	withCR := t.TempDir()
	writeProvTinyModule(t, withCR)
	without := t.TempDir()
	writeProvTinyModule(t, without)

	// A lone CR inside a raw string literal: no LF follows it, so it is
	// content and not a line ending, in either checkout.
	body := "package provider\n\nconst marker = `before\rafter`\n"
	if err := os.WriteFile(filepath.Join(withCR, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(without, filepath.FromSlash(rel)), []byte(strings.ReplaceAll(body, "\r", "")), 0o644); err != nil {
		t.Fatal(err)
	}

	kept, err := SourceDigest(filepath.Join(withCR, hooksModuleRel))
	if err != nil {
		t.Fatal(err)
	}
	dropped, err := SourceDigest(filepath.Join(without, hooksModuleRel))
	if err != nil {
		t.Fatal(err)
	}
	if kept == dropped {
		t.Fatalf("a standalone CR was stripped from the hashed content: both trees digest to %s, so an edit to a lone CR would leave a stale binary looking current", kept)
	}
}

// A _test.go file never reaches the built binary, so it must not affect the
// digest that vouches for that binary's provenance. Without this exclusion, a
// test-only edit anywhere in hooks/go stales every committed binary and fails
// the freshness check for a change that could not have altered the binary's
// bytes.
func TestSourceDigestIgnoresTestFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/x\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d0, err := SourceDigest(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "x_test.go"), []byte("package main\n\nfunc TestNothing() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d1, err := SourceDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d0 {
		t.Fatalf("adding a _test.go file changed the digest: %s -> %s", d0, d1)
	}

	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package main\n\nfunc helper() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d2, err := SourceDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d2 == d1 {
		t.Fatalf("adding a non-test .go file left the digest unchanged: %s", d2)
	}
}

// A duplicated manifest line must resolve to the FIRST entry, because that is
// what the shipped dispatcher does: hooks/run-hook.cmd uses
// `awk '$2 == file { print $1; exit }'` and hooks/fetch-hook.ps1 breaks out of
// its loop. If this reader resolved last-wins, the provenance audit and the
// installer could disagree about which line vouches for a binary — the audit
// would validate bytes the dispatcher never accepts, or pass while the
// dispatcher installs a download the audit never checked.
func TestLoadManifestFirstEntryWins(t *testing.T) {
	first := strings.Repeat("a", 64)
	second := strings.Repeat("b", 64)
	dir := t.TempDir()
	path := filepath.Join(dir, "SHA256SUMS")
	body := first + "  bin/agx-linux-amd64\n" + second + "  bin/agx-linux-amd64\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := manifest.Hash("bin/agx-linux-amd64"); got != first {
		t.Fatalf("duplicate manifest entry resolved to %q, want the first line %q", got, first)
	}
}

// A CRLF manifest must be rejected outright rather than silently parsed: a
// line-oriented reader that folds \r into whitespace would still resolve a
// field split, but the dispatchers this package must agree with (run-hook.cmd's
// awk, fetch-hook.ps1) treat \r as ordinary content, not a separator, so a
// stray one would only mean the manifest was corrupted or mis-transferred.
// The error names the 1-based line number so a bad commit is easy to find.
func TestLoadManifestCarriageReturnIsUnusable(t *testing.T) {
	hash := strings.Repeat("a", 64)
	dir := t.TempDir()
	path := filepath.Join(dir, "SHA256SUMS")
	body := hash + "  bin/agx-linux-amd64\r\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	err = manifest.Unusable()
	if err == nil {
		t.Fatal("expected an unusable manifest for a CRLF file, got none")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("error %q does not name line 1", err.Error())
	}
}

// A U+00A0 (NBSP) inside the path field is content, not a separator:
// strings.Fields would have split on it, but run-hook.cmd's awk splits only
// on space and tab, so this reader must agree and keep the NBSP as part of
// field 2's key.
func TestLoadManifestKeepsNBSPInPath(t *testing.T) {
	hash := strings.Repeat("a", 64)
	dir := t.TempDir()
	path := filepath.Join(dir, "SHA256SUMS")
	nbspPath := "bin/agx-linux amd64"
	body := hash + "  " + nbspPath + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := manifest.Hash(nbspPath); got != hash {
		t.Fatalf("entry for %q = %q, want %q", nbspPath, got, hash)
	}
	if _, present := manifest.Hash("bin/agx-linux-amd64"); present {
		t.Fatal("NBSP split the path into a separate field; bin/agx-linux-amd64 should not be a key")
	}
}

// writeModuleFiles writes each slash-relative path under dir, creating parent
// directories, so a fixture reads as one map of what the module holds.
func writeModuleFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// listFixture is a module with one shipped package, one listed directory, and
// a sibling whose name extends the listed one, so prefix matching that ignores
// path components would wrongly drop the sibling.
func listFixture(t *testing.T, list string) string {
	t.Helper()
	dir := t.TempDir()
	writeModuleFiles(t, dir, map[string]string{
		"go.mod":                 "module example.com/x\n\ngo 1.27.1\n",
		"cmd/hook/main.go":       "package main\n\nfunc main() {}\n",
		"cmd/hooktool/main.go":   "package main\n\nfunc main() {}\n",
		"cmd/hooktoolx/main.go":  "package main\n\nfunc main() {}\n",
		"internal/a/b/b.go":      "package b\n",
		"internal/a/bc/bc.go":    "package bc\n",
		"bin/ignored.go":         "package ignored\n",
		"internal/a/b/b_test.go": "package b\n",
	})
	if list != "" {
		writeModuleFiles(t, dir, map[string]string{digestExcludeFile: list})
	}
	return dir
}

// referenceDigest is the digest algorithm as it stood before the exclusion
// list: go.mod plus every non-test .go file outside bin/. A module with no list
// file must keep this digest, so existing trees and fixtures are unaffected.
func referenceDigest(t *testing.T, dir string) string {
	t.Helper()
	files := []string{"go.mod"}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == filepath.Join(dir, "bin") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	outer := sha256.New()
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(file)))
		if err != nil {
			t.Fatal(err)
		}
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		sum := sha256.Sum256(data)
		fmt.Fprintf(outer, "%s %s  -\n", file, hex.EncodeToString(sum[:]))
	}
	return hex.EncodeToString(outer.Sum(nil))
}

func mustDigest(t *testing.T, dir string) string {
	t.Helper()
	d, err := SourceDigest(dir)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestSourceDigestWithoutListKeepsTheReferenceDigest(t *testing.T) {
	dir := listFixture(t, "")
	if got, want := mustDigest(t, dir), referenceDigest(t, dir); got != want {
		t.Fatalf("a module with no %s digests to %s, want the pre-list digest %s", digestExcludeFile, got, want)
	}
}

// A present list file is hashed even when empty, so creating it is a visible
// change to what the stamp vouches for.
func TestSourceDigestHashesAnEmptyList(t *testing.T) {
	dir := listFixture(t, "")
	before := mustDigest(t, dir)
	writeModuleFiles(t, dir, map[string]string{digestExcludeFile: ""})
	if after := mustDigest(t, dir); after == before {
		t.Fatalf("adding an empty %s left the digest unchanged: %s", digestExcludeFile, after)
	}
}

func TestSourceDigestSkipsListedDirectories(t *testing.T) {
	dir := listFixture(t, "cmd/hooktool\ninternal/a/b\n")
	before := mustDigest(t, dir)
	writeModuleFiles(t, dir, map[string]string{
		"cmd/hooktool/main.go":  "package main\n\nfunc main() { println() }\n",
		"cmd/hooktool/extra.go": "package main\n",
		"internal/a/b/c/c.go":   "package c\n",
	})
	if after := mustDigest(t, dir); after != before {
		t.Fatalf("an edit under a listed directory changed the digest: %s -> %s", before, after)
	}
	writeModuleFiles(t, dir, map[string]string{"internal/a/bc/bc.go": "package bc\n\nvar X int\n"})
	if after := mustDigest(t, dir); after == before {
		t.Fatal("an edit to internal/a/bc, a sibling of listed internal/a/b, left the digest unchanged")
	}
}

func TestSourceDigestCoversTheList(t *testing.T) {
	dir := listFixture(t, "cmd/hooktool\n")
	before := mustDigest(t, dir)
	writeModuleFiles(t, dir, map[string]string{digestExcludeFile: "# shipped elsewhere\ncmd/hooktool\n"})
	if after := mustDigest(t, dir); after == before {
		t.Fatalf("an edit to %s left the digest unchanged: %s", digestExcludeFile, after)
	}
}

func TestSourceFilesListsTheHashedSet(t *testing.T) {
	dir := listFixture(t, "cmd/hooktool\ninternal/a/b\n")
	got, err := SourceFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"cmd/hook/main.go",
		"cmd/hooktoolx/main.go",
		digestExcludeFile,
		"go.mod",
		"internal/a/bc/bc.go",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("SourceFiles = %q, want %q", got, want)
	}
}

func TestExcludeListIgnoresCommentsAndBlankLines(t *testing.T) {
	dir := listFixture(t, "# the verifier ships separately\n\ncmd/hooktool\n#internal/a/b\n\ninternal/a/b")
	got, err := ExcludeList(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cmd/hooktool", "internal/a/b"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ExcludeList = %q, want %q", got, want)
	}
}

func TestExcludeListMissingFileIsEmpty(t *testing.T) {
	got, err := ExcludeList(listFixture(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("ExcludeList without a list file = %q, want none", got)
	}
}

// Each malformed line must fail, and the error must name its line, so a typo
// can never silently stop excluding a directory or start excluding another.
func TestExcludeListRejectsMalformedLines(t *testing.T) {
	cases := map[string]string{
		"absolute":            "/cmd/hooktool",
		"drive letter":        "C:/cmd/hooktool",
		"dotdot element":      "cmd/../cmd/hooktool",
		"dotdot alone":        "..",
		"backslash":           `cmd\hooktool`,
		"nonexistent dir":     "cmd/nosuch",
		"names a file":        "go.mod",
		"dot":                 ".",
		"trailing slash":      "cmd/hooktool/",
		"leading dot slash":   "./cmd/hooktool",
		"double slash":        "cmd//hooktool",
		"carriage return":     "cmd/hooktool\r",
		"trailing space":      "cmd/hooktool ",
		"leading tab":         "\tcmd/hooktool",
		"inner dot element":   "cmd/./hooktool",
		"indented comment":    " # note",
		"whitespace only":     "  ",
		"cr on a blank line":  "\r",
		"cr in a comment":     "# note\r",
		"cr mid entry":        "cmd/hook\rtool",
		"dotdot inside a dir": "internal/a/b/..",
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			dir := listFixture(t, "cmd/hooktool\n"+bad+"\n")
			_, err := ExcludeList(dir)
			if err == nil {
				t.Fatalf("ExcludeList accepted line %q", bad)
			}
			if !strings.Contains(err.Error(), "line 2") {
				t.Fatalf("error %q does not name line 2", err)
			}
			if _, err := SourceFiles(dir); err == nil {
				t.Fatalf("SourceFiles accepted line %q", bad)
			}
			if _, err := SourceDigest(dir); err == nil {
				t.Fatalf("SourceDigest accepted line %q", bad)
			}
		})
	}
}
