package hookbuild_test

import (
	"debug/buildinfo"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
	"github.com/muehz/agentics/hooks/go/internal/hosttest"
)

// TestContractBinariesNamesTheSixAgxTargets pins the contract names, in
// contract order, that run-hook.cmd and hooks/SHA256SUMS select by.
func TestContractBinariesNamesTheSixAgxTargets(t *testing.T) {
	want := []string{
		"agx-darwin-amd64",
		"agx-darwin-arm64",
		"agx-linux-amd64",
		"agx-linux-arm64",
		"agx-windows-amd64.exe",
		"agx-windows-arm64.exe",
	}
	if strings.Join(hookbuild.ContractBinaries, ",") != strings.Join(want, ",") {
		t.Fatalf("ContractBinaries = %q, want %q", hookbuild.ContractBinaries, want)
	}
	if len(hookbuild.ContractTargets) != len(want) {
		t.Fatalf("ContractTargets has %d entries, want %d", len(hookbuild.ContractTargets), len(want))
	}
	for _, name := range want {
		if _, ok := hookbuild.ContractTargets[name]; !ok {
			t.Errorf("ContractTargets has no entry for %q", name)
		}
	}
}

// TestBuildHostMatchesManifest proves a fresh build of the host's own
// contract binary reproduces the bytes hooks/SHA256SUMS commits, which is
// the whole point of hookbuild: a binary that does not come from git must
// be indistinguishable from one that does.
func TestBuildHostMatchesManifest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping a real go build in -short mode")
	}
	name := hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH)
	if name == "" {
		t.Skip("no contract target for this GOOS/GOARCH")
	}
	root := hosttest.Checkout(t)
	hooksDir := filepath.Join(root, "hooks", "go")
	outDir := t.TempDir()

	res := rebuild(t, hookbuild.Request{SourceDir: hooksDir, GoBin: "go", OutDir: outDir, Names: []string{name}})

	manifest, err := hookprovenance.LoadManifest(filepath.Join(root, "hooks", "SHA256SUMS"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	pin, err := hookprovenance.PinnedToolchain(hooksDir)
	if err != nil {
		t.Fatalf("PinnedToolchain: %v", err)
	}

	if err := res.Verify(pin, manifest); err != nil {
		t.Errorf("a fresh build must reproduce the committed manifest: %v", err)
	}
}

// TestBuildResolvesARelativeOutDirFromTheWorkingDirectory pins the call shape
// of a caller that hands Rebuild a relative directory: go build runs from the
// source directory, so a relative -o would land the binaries inside it
// instead of where every later read looks.
func TestBuildResolvesARelativeOutDirFromTheWorkingDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping a real go build in -short mode")
	}
	name := hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH)
	if name == "" {
		t.Skip("no contract target for this GOOS/GOARCH")
	}
	hooksDir := filepath.Join(hosttest.Checkout(t), "hooks", "go")
	work := t.TempDir()
	t.Chdir(work)

	rebuild(t, hookbuild.Request{SourceDir: hooksDir, GoBin: "go", OutDir: "release-assets", Names: []string{name}})
	if _, err := os.Stat(filepath.Join(work, "release-assets", name)); err != nil {
		t.Errorf("binary not under the working directory's release-assets: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hooksDir, "release-assets")); err == nil {
		t.Errorf("Rebuild wrote into hooksDir/release-assets")
		os.RemoveAll(filepath.Join(hooksDir, "release-assets"))
	}
}

// TestLauncherIdentifiersNameTheWindowsAmd64Launcher pins the launcher's
// package, platform and repository-relative output, and keeps it out of the
// six-binary contract that run-hook.cmd and hooks/SHA256SUMS select by.
func TestLauncherIdentifiersNameTheWindowsAmd64Launcher(t *testing.T) {
	if hookbuild.LauncherPackage != "./cmd/launcher" {
		t.Errorf("LauncherPackage = %q, want ./cmd/launcher", hookbuild.LauncherPackage)
	}
	if hookbuild.LauncherTarget != [2]string{"windows", "amd64"} {
		t.Errorf("LauncherTarget = %q, want windows/amd64", hookbuild.LauncherTarget)
	}
	if hookbuild.LauncherOutput != "bin/agx.exe" {
		t.Errorf("LauncherOutput = %q, want bin/agx.exe", hookbuild.LauncherOutput)
	}
	for _, name := range hookbuild.ContractBinaries {
		if name == "agx.exe" || name == hookbuild.LauncherOutput {
			t.Errorf("ContractBinaries contains the launcher %q", name)
		}
	}
	for name := range hookbuild.ContractTargets {
		if name == "agx.exe" || name == hookbuild.LauncherOutput {
			t.Errorf("ContractTargets contains the launcher %q", name)
		}
	}
}

// TestLauncherBuildArgsDifferFromBuildArgsOnlyInThePackage proves the
// launcher gets the contract binaries' exact flags and stamp.
func TestLauncherBuildArgsDifferFromBuildArgsOnlyInThePackage(t *testing.T) {
	const stamp, out = "agentics-src-0123", "/tmp/agx.exe"
	hook, launcher := hookbuild.BuildArgs(stamp, out), hookbuild.LauncherBuildArgs(stamp, out)
	if len(hook) != len(launcher) {
		t.Fatalf("LauncherBuildArgs = %q, BuildArgs = %q", launcher, hook)
	}
	last := len(hook) - 1
	if strings.Join(hook[:last], "\x00") != strings.Join(launcher[:last], "\x00") {
		t.Errorf("LauncherBuildArgs = %q differs from BuildArgs = %q before the package", launcher, hook)
	}
	if hook[last] != "./cmd/hook" || launcher[last] != hookbuild.LauncherPackage {
		t.Errorf("package arguments = %q and %q, want ./cmd/hook and %q", hook[last], launcher[last], hookbuild.LauncherPackage)
	}
}

// TestBuildLauncherStampsAWindowsAmd64Binary builds the real launcher and
// checks it carries this module's source stamp and the launcher platform.
func TestBuildLauncherStampsAWindowsAmd64Binary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping a real go build in -short mode")
	}
	hooksDir := filepath.Join(hosttest.Checkout(t), "hooks", "go")
	work := t.TempDir()
	t.Chdir(work)
	t.Setenv("GOAMD64", "v3")
	rebuild(t, hookbuild.Request{SourceDir: hooksDir, GoBin: "go", OutDir: "scratch", Launcher: true})
	out := filepath.Join(work, "scratch", "agx.exe")
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("launcher not under the working directory: %v", err)
	}
	digest, err := hookprovenance.SourceDigest(hooksDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "agentics-src-"+digest) {
		t.Errorf("launcher does not carry the stamp agentics-src-%s", digest)
	}
	info, err := buildinfo.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Path != "github.com/muehz/agentics/hooks/go/cmd/launcher" {
		t.Errorf("built package %q, want cmd/launcher", info.Path)
	}
	settings := map[string]string{}
	for _, s := range info.Settings {
		settings[s.Key] = s.Value
	}
	for key, want := range map[string]string{"GOOS": "windows", "GOARCH": "amd64", "CGO_ENABLED": "0", "GOAMD64": "v1", "-trimpath": "true"} {
		if settings[key] != want {
			t.Errorf("build setting %s = %q, want %q", key, settings[key], want)
		}
	}
}

// makeVars expands the simple `NAME := value` assignments of a Makefile, so a
// test can resolve a target named through a variable.
func makeVars(lines []string) func(string) string {
	vars := map[string]string{}
	var expand func(string) string
	expand = func(s string) string {
		for name, value := range vars {
			s = strings.ReplaceAll(s, "$("+name+")", value)
		}
		return s
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, ":=")
		if !ok || strings.ContainsAny(strings.TrimSpace(name), " \t") || strings.HasPrefix(strings.TrimSpace(name), "export") {
			continue
		}
		vars[strings.TrimSpace(name)] = expand(strings.TrimSpace(value))
	}
	return expand
}

// TestMakefileBuildsTheLauncher pins hooks/go/Makefile's launcher rule to
// the platform, package and output the Go identifiers name, and requires
// build-all to produce it.
func TestMakefileBuildsTheLauncher(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(hosttest.Checkout(t), "hooks", "go", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\\n", " ")
	lines := strings.Split(text, "\n")
	expand := makeVars(lines)
	const recipe = "\tGOOS=windows GOARCH=amd64 $(GO) build $(BUILDOPTS) -o $@ ./cmd/launcher"
	target := ""
	for i, line := range lines {
		if line != recipe {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			if !strings.HasPrefix(lines[j], "\t") {
				head, _, ok := strings.Cut(lines[j], ":")
				if !ok {
					t.Fatalf("the launcher recipe follows %q, not a rule", lines[j])
				}
				if target != "" {
					t.Fatal("the launcher recipe appears in more than one rule")
				}
				target = strings.TrimSpace(head)
				break
			}
		}
	}
	if target == "" {
		t.Fatalf("hooks/go/Makefile has no rule with the recipe %q", strings.TrimPrefix(recipe, "\t"))
	}
	if got := expand(target); got != "../../"+hookbuild.LauncherOutput {
		t.Errorf("the launcher rule's target %s expands to %q, want ../../%s", target, got, hookbuild.LauncherOutput)
	}
	listed := false
	for _, line := range lines {
		if prereqs, ok := strings.CutPrefix(line, "build-all:"); ok {
			for _, p := range strings.Fields(prereqs) {
				if expand(p) == "../../"+hookbuild.LauncherOutput {
					listed = true
				}
			}
		}
	}
	if !listed {
		t.Errorf("build-all does not list the launcher target %s", target)
	}
}

// TestBuildArgsMatchMakefile pins the Go restatement of the Makefile's
// build flags to the Makefile text. The conformance rebuild comparison in
// checkBuildContract is only meaningful while the two agree: a flag added
// to one side and not the other makes every committed binary read as
// tampered after the next `make build-all`.
func TestBuildArgsMatchMakefile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(hosttest.Checkout(t), "hooks", "go", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	// Join backslash line continuations before splitting into lines, so a
	// value the Makefile wraps across multiple lines (as BINARIES does, and
	// LDFLAGS/BUILDOPTS may grow to) is not silently truncated at the first
	// line.
	text := strings.ReplaceAll(string(data), "\\\r\n", " ")
	text = strings.ReplaceAll(text, "\\\n", " ")

	want := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		for _, key := range []string{"LDFLAGS", "BUILDOPTS"} {
			if strings.HasPrefix(line, key) {
				if _, value, ok := strings.Cut(line, ":="); ok {
					want[key] = strings.Join(strings.Fields(value), " ")
				}
			}
		}
	}
	const digest = "0123456789abcdef"
	if got, exp := hookbuild.Ldflags("agentics-src-"+digest), strings.Join(strings.Fields(strings.ReplaceAll(want["LDFLAGS"], "$(SOURCE_DIGEST)", digest)), " "); got != exp {
		t.Errorf("Ldflags = %q, but hooks/go/Makefile LDFLAGS renders %q", got, exp)
	}
	if got, exp := strings.Join(strings.Fields(strings.Join(hookbuild.BuildOpts, " ")+` -ldflags="$(LDFLAGS)"`), " "), want["BUILDOPTS"]; got != exp {
		t.Errorf("BuildOpts render %q, but hooks/go/Makefile BUILDOPTS is %q", got, exp)
	}
}
