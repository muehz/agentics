package conformance

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
)

// launcherTreeFiles is a plain tree that satisfies bin-launchers with dummy
// bytes; the rule reads names, CR bytes and one attribute line, never the
// launcher's machine format.
var launcherTreeFiles = map[string]string{
	"bin/agx":        "#!/bin/sh\nexec true\n",
	"bin/agx.exe":    "MZ dummy launcher",
	".gitattributes": "* text=auto eol=lf\n# bin/agx.exe binary is only a comment here\nbin/agx.exe binary linguist-generated\n",
}

func writeLauncherTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// launcherPaths returns the sorted Paths of the bin-launchers findings and
// fails on a finding of any other rule.
func launcherPaths(t *testing.T, failures []Failure) []string {
	t.Helper()
	paths := []string{}
	for _, f := range failures {
		if f.Rule != "bin-launchers" {
			t.Fatalf("unexpected rule %q: %v", f.Rule, f)
		}
		paths = append(paths, f.Path)
	}
	sort.Strings(paths)
	return paths
}

func TestBinLaunchersRoster(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, root string)
		want   []string
	}{
		{name: "pristine", mutate: func(*testing.T, string) {}, want: []string{}},
		{name: "stray file beside a dotfile", mutate: func(t *testing.T, root string) {
			writeRel(t, root, "bin/agx.cmd", "@echo off\r\n")
			writeRel(t, root, "bin/.DS_Store", "junk")
		}, want: []string{"bin/agx.cmd"}},
		{name: "stray directory", mutate: func(t *testing.T, root string) {
			writeRel(t, root, "bin/extra/agx", "x")
		}, want: []string{"bin/extra"}},
		{name: "missing bin/agx", mutate: func(t *testing.T, root string) {
			removeRel(t, root, "bin/agx")
		}, want: []string{"bin/agx"}},
		{name: "missing bin/agx.exe", mutate: func(t *testing.T, root string) {
			removeRel(t, root, "bin/agx.exe")
		}, want: []string{"bin/agx.exe"}},
		{name: "missing bin", mutate: func(t *testing.T, root string) {
			removeRel(t, root, "bin")
		}, want: []string{"bin/agx", "bin/agx.exe"}},
		{name: "CRLF in bin/agx", mutate: func(t *testing.T, root string) {
			writeRel(t, root, "bin/agx", "#!/bin/sh\r\nexec true\r\n")
		}, want: []string{"bin/agx"}},
		{name: "attribute line removed", mutate: func(t *testing.T, root string) {
			writeRel(t, root, ".gitattributes", "* text=auto eol=lf\n# bin/agx.exe binary\nbin/agx.exe -diff\nhooks/bin/agx.exe binary\n")
		}, want: []string{".gitattributes"}},
		{name: ".gitattributes removed", mutate: func(t *testing.T, root string) {
			removeRel(t, root, ".gitattributes")
		}, want: []string{".gitattributes"}},
		{name: "CRLF .gitattributes still marks the launcher", mutate: func(t *testing.T, root string) {
			writeRel(t, root, ".gitattributes", "bin/agx.exe\tbinary\r\n")
		}, want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeLauncherTree(t, launcherTreeFiles)
			tc.mutate(t, root)
			if got := launcherPaths(t, checkBinLaunchers(root, false)); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("bin-launchers paths = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestBinLaunchersRunInsideFreshness proves RunFreshness carries the rule and
// keeps its findings when the source digest later cannot be computed: this
// tree has a committed hook binary but no hooks/go module.
func TestBinLaunchersRunInsideFreshness(t *testing.T) {
	files := map[string]string{"hooks/bin/agx-linux-amd64": "not built"}
	for rel, content := range launcherTreeFiles {
		files[rel] = content
	}
	root := writeLauncherTree(t, files)
	writeRel(t, root, "bin/agx.cmd", "@echo off\r\n")
	failures, err := RunFreshness(root)
	if err == nil {
		t.Fatal("RunFreshness must report the missing module as an operational error")
	}
	for _, f := range failures {
		if f.Rule == "bin-launchers" && f.Path == "bin/agx.cmd" {
			return
		}
	}
	t.Fatalf("the bin-launchers finding was lost with the digest error: %v", failures)
}

func TestFreshnessPristineTinyRepoKeepsSixManifestEntries(t *testing.T) {
	skipFreshnessScenario(t)
	root := cloneTinyRepo(t)
	failures, err := RunFreshness(root)
	assertGreen(t, failures, err)
	data, err := os.ReadFile(filepath.Join(root, "hooks", "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != len(hookbuild.ContractBinaries) {
		t.Fatalf("the manifest lists exactly the six contract binaries: %q", data)
	}
	for _, line := range lines {
		if strings.HasSuffix(line, " bin/agx.exe") {
			t.Fatalf("the manifest must not list the launcher: %q", line)
		}
	}
}

func TestFreshnessBitesStaleLauncherStamp(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	fakeStaleStamp(t, filepath.Join(root, "bin", "agx.exe"))

	failures, err := RunFreshness(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFindingAt(t, failures, "freshness-stale-digest", "bin/agx.exe")

	failures = auditFailures(t, root)
	assertFindingAt(t, failures, "freshness-stale-digest", "bin/agx.exe")
	for _, f := range failures {
		if f.Rule == "module-rebuild-mismatch" && f.Path == "bin/agx.exe" {
			t.Fatalf("a stale launcher stamp already explains its rebuild mismatch: %v", f)
		}
	}
}

func TestFreshnessAndMachineFormatBiteTextLauncher(t *testing.T) {
	skipFreshnessScenario(t)
	root := cloneTinyRepo(t)
	writeRel(t, root, "bin/agx.exe", "not an executable, padded well past the size floor of sixty-four bytes..........")

	failures, err := RunFreshness(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFindingAt(t, failures, "freshness-no-toolchain", "bin/agx.exe")
	assertFindingAt(t, checkCommittedBinaries(filepath.Join(root, "hooks", "bin")), "module-binary-format", "bin/agx.exe")
}

func TestAuditKeepsLauncherRebuildMismatchWithFreshDigest(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	victim := filepath.Join(root, "bin", "agx.exe")
	data, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victim, append(data, 0x00), 0o755); err != nil {
		t.Fatal(err)
	}

	failures := auditFailures(t, root)
	for _, f := range failures {
		if f.Rule == "freshness-stale-digest" {
			t.Fatalf("tampering committed bytes directly must not move the source digest: %v", f)
		}
	}
	assertFindingAt(t, failures, "module-rebuild-mismatch", "bin/agx.exe")
}

func assertFindingAt(t *testing.T, failures []Failure, rule, path string) {
	t.Helper()
	for _, f := range failures {
		if f.Rule == rule && f.Path == path {
			return
		}
	}
	t.Fatalf("want %s at %s, got %v", rule, path, failures)
}

func writeRel(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func removeRel(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
}
