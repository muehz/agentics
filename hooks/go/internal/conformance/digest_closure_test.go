package conformance

import (
	"go/build"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// shippedCommands are the module-relative packages whose binaries the source
// digest vouches for: the contract hook binaries and the agx launcher.
var shippedCommands = []string{"cmd/hook", "cmd/launcher"}

// TestDigestExcludeMatchesClosure pins hooks/go/digest-exclude to the
// complement of the shipped binaries' import closure in both directions. A
// listed directory that a binary imports would let an edit to shipped code
// leave the digest unchanged, so the committed binaries would read as fresh
// while stale. A non-closure directory left unlisted would make an edit to
// code no binary contains stale every binary for nothing.
func TestDigestExcludeMatchesClosure(t *testing.T) {
	moduleRoot := filepath.Join(hosttest.Checkout(t), "hooks", "go")
	list, err := hookprovenance.ExcludeList(moduleRoot)
	if err != nil {
		t.Fatalf("ExcludeList: %v", err)
	}
	for _, entry := range list {
		info, err := os.Stat(filepath.Join(moduleRoot, filepath.FromSlash(entry)))
		if err != nil || !info.IsDir() {
			t.Errorf("digest-exclude entry %q names no existing directory", entry)
		}
	}
	closure := shippedImportClosure(t, moduleRoot)
	for _, cmd := range shippedCommands {
		if !closure[cmd] {
			t.Fatalf("import closure %v lacks the command %s itself", sortedKeys(closure), cmd)
		}
	}
	sourceDirs := nonTestSourceDirs(t, moduleRoot)

	for _, failure := range digestExcludeFailures(list, closure, sourceDirs) {
		t.Error(failure)
	}

	// The negative cases feed altered copies of the list to the comparison, so
	// the committed digest-exclude is never edited to prove either direction.
	t.Run("unlisted non-closure dir fails", func(t *testing.T) {
		without := slices.DeleteFunc(slices.Clone(list), func(e string) bool { return e == "internal/hookbuild" })
		if len(without) == len(list) {
			t.Fatalf("digest-exclude %v does not list internal/hookbuild", list)
		}
		failures := digestExcludeFailures(without, closure, sourceDirs)
		if !slices.Contains(failures, "unlisted non-closure dir internal/hookbuild") {
			t.Errorf("failures = %q, want one naming unlisted non-closure dir internal/hookbuild", failures)
		}
	})
	t.Run("listed closure dir fails", func(t *testing.T) {
		with := append(slices.Clone(list), "internal/settings")
		failures := digestExcludeFailures(with, closure, sourceDirs)
		if !slices.Contains(failures, "excludes shipped dir internal/settings") {
			t.Errorf("failures = %q, want one naming excludes shipped dir internal/settings", failures)
		}
	})
}

// digestExcludeFailures compares the list with the shipped closure and the
// directories holding a non-test .go file. A closure directory at or under an
// entry is "excludes shipped dir"; a source directory outside the closure and
// at or under no entry is "unlisted non-closure dir". Failures come back
// sorted so a report is stable.
func digestExcludeFailures(list []string, closure, sourceDirs map[string]bool) []string {
	var failures []string
	for _, dir := range sortedKeys(closure) {
		if underAnyEntry(dir, list) {
			failures = append(failures, "excludes shipped dir "+dir)
		}
	}
	for _, dir := range sortedKeys(sourceDirs) {
		if !closure[dir] && !underAnyEntry(dir, list) {
			failures = append(failures, "unlisted non-closure dir "+dir)
		}
	}
	slices.Sort(failures)
	return failures
}

// underAnyEntry matches by whole path component, the way the digest prunes:
// entry a/b covers a/b and a/b/c but never a/bc.
func underAnyEntry(dir string, list []string) bool {
	for _, entry := range list {
		if dir == entry || strings.HasPrefix(dir, entry+"/") {
			return true
		}
	}
	return false
}

// shippedImportClosure returns the module-relative directories reachable
// from shippedCommands through in-module imports, unioned over every contract
// target. Build constraints select different files per GOOS/GOARCH, so one
// host's view would miss a platform-only import. Cgo is off because the
// Makefile builds the binaries without it.
func shippedImportClosure(t *testing.T, moduleRoot string) map[string]bool {
	t.Helper()
	modulePath := readModulePath(t, moduleRoot)
	closure := map[string]bool{}
	for name, target := range hookbuild.ContractTargets {
		ctx := build.Default
		ctx.GOOS, ctx.GOARCH = target[0], target[1]
		ctx.CgoEnabled = false
		seen := map[string]bool{}
		queue := slices.Clone(shippedCommands)
		for len(queue) > 0 {
			rel := queue[0]
			queue = queue[1:]
			if seen[rel] {
				continue
			}
			seen[rel] = true
			pkg, err := ctx.ImportDir(filepath.Join(moduleRoot, filepath.FromSlash(rel)), 0)
			if err != nil {
				t.Fatalf("%s: importing %s: %v", name, rel, err)
			}
			for _, imp := range pkg.Imports {
				if imp == modulePath {
					queue = append(queue, ".")
				} else if sub, ok := strings.CutPrefix(imp, modulePath+"/"); ok {
					queue = append(queue, sub)
				}
			}
		}
		for rel := range seen {
			closure[rel] = true
		}
	}
	return closure
}

// readModulePath returns go.mod's module path, the prefix that marks an
// import as in-module.
func readModulePath(t *testing.T, moduleRoot string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	for line := range strings.Lines(string(data)) {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(path)
		}
	}
	t.Fatal("go.mod has no module line")
	return ""
}

// nonTestSourceDirs returns every module-relative directory holding a non-test
// .go file. It skips bin/ (build outputs) and, as the go tool does, testdata
// and directories whose names start with _ or ., since no package there can
// be imported.
func nonTestSourceDirs(t *testing.T, moduleRoot string) map[string]bool {
	t.Helper()
	dirs := map[string]bool{}
	err := filepath.WalkDir(moduleRoot, func(walked string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(moduleRoot, walked)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			name := d.Name()
			if rel != "." && (rel == "bin" || name == "testdata" || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
			dirs[filepath.ToSlash(filepath.Dir(filepath.FromSlash(rel)))] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", moduleRoot, err)
	}
	return dirs
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
