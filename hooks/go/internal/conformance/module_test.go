package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
)

// The module-gate bite checks run the owning gate against a tiny hook module
// (see tinyhook_test.go), mutating one precondition per scenario. They are
// skipped under -short because several owners exec the Go toolchain.

func skipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: module-gate scenarios exec the Go toolchain repeatedly")
	}
}

func TestModuleGatesPassOnPristineTinyRepo(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	failures, err := RunModuleGates(root)
	assertGreen(t, failures, err)
}

func TestSourceOnlyGeneratedLauncher(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	mod := filepath.Join(root, engineRel)
	digest, err := hookprovenance.SourceDigest(mod)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := hookprovenance.PinnedToolchain(mod)
	if err != nil {
		t.Fatal(err)
	}
	removeRel(t, root, "bin/agx.exe")
	found, built, err := checkBuildContract(mod, filepath.Join(root, "hooks/bin"), filepath.Join(root, manifestRel), "go", true)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(built)
	assertGreen(t, found, nil)
	launcher := filepath.Join(built, "agx.exe")
	assertFindingAt(t, checkGeneratedLauncher(filepath.Join(built, "agx-windows-arm64.exe"), pin, digest), "module-build-contract", "bin/agx.exe")
	assertFindingAt(t, checkGeneratedLauncher(launcher, "go1.0", digest), "module-rebuild-toolchain", "hooks/go/go.mod")
	assertFindingAt(t, checkGeneratedLauncher(launcher, pin, strings.Repeat("0", 64)), "module-build-contract", "bin/agx.exe")
	// Losing the stamp symbol still produces a valid PE from the pinned
	// compiler. The build contract itself must catch that regression.
	writeRel(t, root, "hooks/go/cmd/launcher/main.go", "package main\n\nfunc main() {}\n")
	found, unstamped, err := checkBuildContract(mod, filepath.Join(root, "hooks/bin"), filepath.Join(root, manifestRel), "go", true)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(unstamped)
	assertFindingAt(t, found, "module-build-contract", "bin/agx.exe")
}

func TestSourceOnlyGeneratedRuntimeStamps(t *testing.T) {
	root := cloneTinyRepo(t)
	mod := filepath.Join(root, engineRel)
	// Renaming the linker's target preserves behavior but prevents -X from
	// stamping any of the six images. Publish a matching fixture manifest.
	writeRel(t, root, "hooks/go/cmd/hook/main.go", strings.ReplaceAll(tinyMainGo, "sourceDigest", "runtimeStamp"))
	digest, err := hookprovenance.SourceDigest(mod)
	if err != nil {
		t.Fatal(err)
	}
	if err := stampTinyBinariesRaw(root, "agentics-src-"+digest); err != nil {
		t.Fatal(err)
	}
	removeRel(t, root, "hooks/bin")
	removeRel(t, root, "bin/agx.exe")
	found, built, err := checkBuildContract(mod, filepath.Join(root, "hooks/bin"), filepath.Join(root, manifestRel), "go", true)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(built)
	for _, name := range hookbuild.ContractBinaries {
		assertFindingAt(t, found, "module-build-contract", "hooks/bin/"+name)
	}
	for _, f := range found {
		if f.Rule != "module-build-contract" || !strings.Contains(f.Message, "source digest") {
			t.Fatalf("only the missing embedded stamps should fail: %+v", f)
		}
	}
}

func TestModuleGatesBiteThirdPartyDependency(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	directive := tinyGoDirective(t)
	mutateTinyFile(t, root, "hooks/go/go.mod", directive, directive+"\n\nrequire example.com/dep v1.0.0")
	failures, err := checkNoThirdParty(filepath.Join(root, engineRel))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "module-no-third-party")
}

func TestModuleGatesBiteGofmt(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	// Valid Go, but not gofmt-clean (spaces around =).
	misformatted := "package main\n\nvar sloppy    = 2\n"
	if err := os.WriteFile(filepath.Join(root, engineRel, "cmd", "hook", "sloppy.go"), []byte(misformatted), 0o644); err != nil {
		t.Fatal(err)
	}
	failures, err := checkGofmt(filepath.Join(root, engineRel), "gofmt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "module-gofmt")
}

func TestModuleGatesBiteVet(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	bait := "package main\n\nimport \"fmt\"\n\nfunc vetBait() {\n\tfmt.Printf(\"%d\\n\", \"not an int\")\n}\n"
	if err := os.WriteFile(filepath.Join(root, engineRel, "cmd", "hook", "vetbait.go"), []byte(bait), 0o644); err != nil {
		t.Fatal(err)
	}
	failures := checkVet(filepath.Join(root, engineRel), "go")
	assertRuleFired(t, failures, "module-vet")
}

func TestModuleGatesBiteTest(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	failing := "package main\n\nimport \"testing\"\n\nfunc TestAlwaysFails(t *testing.T) {\n\tt.Fatal(\"deliberate\")\n}\n"
	if err := os.WriteFile(filepath.Join(root, engineRel, "cmd", "hook", "fail_test.go"), []byte(failing), 0o644); err != nil {
		t.Fatal(err)
	}
	failures, err := checkTestsAndCoverage(filepath.Join(root, engineRel), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "module-test")
}

// TestModuleGatesSkipTests breaks a test, gofmt, and one committed binary in
// the same checkout. The failing test sits in internal/provider, which the
// test gate runs on every host, so the unskipped run reports module-test
// everywhere. With the skip, module-test and module-coverage-floor go silent
// while gofmt and the rebuild comparison, which runs after the skipped
// gates, still report.
func TestModuleGatesSkipTests(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	provider := filepath.Join(root, engineRel, "internal", "provider")
	failing := "package provider\n\nimport \"testing\"\n\nfunc TestAlwaysFails(t *testing.T) {\n\tt.Fatal(\"deliberate\")\n}\n"
	if err := os.WriteFile(filepath.Join(provider, "fail_test.go"), []byte(failing), 0o644); err != nil {
		t.Fatal(err)
	}
	// A test file stays out of the source digest, so the binaries stay fresh.
	if err := os.WriteFile(filepath.Join(provider, "sloppy_test.go"), []byte("package provider\n\nvar sloppy    = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const tampered = "agx-linux-amd64"
	binPath := filepath.Join(root, "hooks", "bin", tampered)
	data, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binPath, append(data, 0), 0o755); err != nil {
		t.Fatal(err)
	}

	skipped, err := runModuleGates(root, AuditOptions{SkipTests: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range skipped {
		if f.Rule == "module-test" || f.Rule == "module-coverage-floor" {
			t.Fatalf("a skipped gate reported: %v", f)
		}
	}
	assertRuleFired(t, skipped, "module-gofmt")
	mismatch := false
	for _, f := range skipped {
		if f.Rule == "module-rebuild-mismatch" && f.Path == "hooks/bin/"+tampered {
			mismatch = true
		}
	}
	if !mismatch {
		t.Fatalf("the rebuild comparison must still run with tests skipped, got: %v", skipped)
	}

	full, err := RunModuleGates(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, full, "module-test")
}

func TestCoverageGateRunsTestsOnEveryInvocation(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "module")
	if err := os.Mkdir(module, 0o755); err != nil {
		t.Fatal(err)
	}
	// Outside the module, the observation file does not invalidate Go's test cache.
	marker := filepath.Join(root, "executions")
	files := map[string]string{
		"go.mod":   tinyGoModFor(t),
		"value.go": "package tinyhook\nfunc Value() int { return 1 }\n",
		"value_test.go": fmt.Sprintf(`package tinyhook
import ("os"; "testing")
func TestValue(t *testing.T) {
 if Value() != 1 { t.Fatal("wrong value") }
 f, err := os.OpenFile(%q, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
 if err != nil { t.Fatal(err) }
 _, writeErr := f.WriteString("ran\n")
 closeErr := f.Close()
 if writeErr != nil { t.Fatal(writeErr) }
 if closeErr != nil { t.Fatal(closeErr) }
}
`, marker),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(module, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		failures, err := checkTestsAndCoverage(module, "go")
		assertGreen(t, failures, err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ran\nran\n" {
		t.Fatalf("coverage gate reused a cached test result: executions=%q, want two runs", got)
	}
}

func TestModuleGatesBiteBuildContract(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	targetOS := "windows"
	name := "winbreak.go"
	if runtime.GOOS == targetOS {
		targetOS = "darwin"
		name = "darwinbreak.go"
	}
	broken := "//go:build " + targetOS + "\n\npackage main\n\nvar broken = undefinedSymbol\n"
	if err := os.WriteFile(filepath.Join(root, engineRel, "cmd", "hook", name), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	failures, buildDir, err := checkBuildContract(filepath.Join(root, engineRel), filepath.Join(root, "hooks", "bin"), filepath.Join(root, "hooks", "SHA256SUMS"), "go", false)
	defer os.RemoveAll(buildDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "module-build-contract")
}

// TestModuleGatesBiteLauncherBuild breaks only the launcher's windows build:
// the six hook binaries still build, so the finding can only come from the
// launcher rebuild.
func TestModuleGatesBiteLauncherBuild(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	broken := "//go:build windows\n\npackage main\n\nvar broken = undefinedSymbol\n"
	if err := os.WriteFile(filepath.Join(root, engineRel, "cmd", "launcher", "winbreak.go"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	failures, buildDir, err := checkBuildContract(filepath.Join(root, engineRel), filepath.Join(root, "hooks", "bin"), filepath.Join(root, "hooks", "SHA256SUMS"), "go", false)
	defer os.RemoveAll(buildDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, f := range failures {
		if f.Rule == "module-build-contract" && strings.Contains(f.Message, "agx launcher") {
			return
		}
	}
	t.Fatalf("want a module-build-contract finding for the launcher, got %v", failures)
}

// TestModuleGatesBiteRebuildMismatch is the #607 scenario. The impostor
// carries the correct source stamp, the pinned toolchain in its build
// info, and a refreshed manifest line, and it was not built with the
// Makefile's flags: linked with its symbol table (no -s -w), so its bytes
// differ from what the checkout builds. Every freshness verdict accepts it.
// Only the rebuild comparison can reject it.
func TestModuleGatesBiteRebuildMismatch(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	hooksDir := filepath.Join(root, engineRel)
	binDir := filepath.Join(root, "hooks", "bin")
	digest, err := hookprovenance.SourceDigest(hooksDir)
	if err != nil {
		t.Fatal(err)
	}
	const name = "agx-linux-amd64"
	target := hookbuild.ContractTargets[name]
	out, err := runToolEnv(hooksDir,
		append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target[0], "GOARCH="+target[1]),
		"go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-X main.sourceDigest=agentics-src-"+digest,
		"-o", filepath.Join(binDir, name), "./cmd/hook")
	if err != nil {
		t.Fatalf("building the impostor: %v\n%s", err, out)
	}
	if err := writeTinyManifest(root); err != nil {
		t.Fatal(err)
	}
	// The provenance verdicts cannot see the substitution: that is the gap.
	failures, err := RunFreshness(root)
	assertGreen(t, failures, err)

	failures, buildDir, err := checkBuildContract(hooksDir, binDir, filepath.Join(root, "hooks", "SHA256SUMS"), "go", false)
	defer os.RemoveAll(buildDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "module-rebuild-mismatch")
	// The impostor's own manifest line was refreshed by writeTinyManifest, so
	// module-rebuild-manifest also fires here; that is expected (checked
	// below) and not this rule's finding.
	assertRuleFired(t, failures, "module-rebuild-manifest")
	for _, f := range failures {
		if f.Rule != "module-rebuild-mismatch" {
			continue
		}
		if f.Path != "hooks/bin/"+name {
			t.Fatalf("rebuild mismatch fired on %q; only the substituted %s differs from the source", f.Path, name)
		}
	}
}

// TestModuleGatesBiteRebuildToolchainDrift pins an older release than the
// toolchain on PATH. `go build` keeps the newer local toolchain (GOTOOLCHAIN
// only ever upgrades), so the fresh artifact's build info no longer answers
// the pin. That must surface as a toolchain mismatch, never as a byte
// difference, because a wrong release differs in every byte and would
// otherwise read as tampering.
func TestModuleGatesBiteRebuildToolchainDrift(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	mutateTinyFile(t, root, "hooks/go/go.mod", tinyGoDirective(t), "go 1.24")
	failures, buildDir, err := checkBuildContract(filepath.Join(root, engineRel), filepath.Join(root, "hooks", "bin"), filepath.Join(root, "hooks", "SHA256SUMS"), "go", false)
	defer os.RemoveAll(buildDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "module-rebuild-toolchain")
	for _, f := range failures {
		// go.mod moved the source digest too, so a manifest comparison run
		// under drift would fire; it must be skipped like the byte compare.
		if f.Rule == "module-rebuild-mismatch" || f.Rule == "module-rebuild-manifest" {
			t.Fatalf("a toolchain mismatch must be reported as such, not as a byte difference: %v", f)
		}
	}
}

// TestCheckBuildContractIgnoresHostArchTuning is the #611 review scenario:
// a CI or maintainer host with a non-default GOAMD64/GOARM64 in its
// environment must not make checkBuildContract's rebuild disagree with the
// pinned-default committed binaries. Without hookbuild.ArchTuningEnv overriding the
// inherited os.Environ() values, this reproduces module-rebuild-mismatch on
// a perfectly valid checkout — see the bug this guards against.
func TestCheckBuildContractIgnoresHostArchTuning(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	t.Setenv("GOAMD64", "v3")
	t.Setenv("GOARM64", "v9.0")
	failures, buildDir, err := checkBuildContract(filepath.Join(root, engineRel), filepath.Join(root, "hooks", "bin"), filepath.Join(root, "hooks", "SHA256SUMS"), "go", false)
	defer os.RemoveAll(buildDir)
	assertGreen(t, failures, err)
}

func TestModuleGatesBiteBinaryFormat(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	// A committed binary that is not its platform's format: overwrite the
	// linux-amd64 artifact with text.
	if err := os.WriteFile(filepath.Join(root, "hooks", "bin", "agx-linux-amd64"), []byte("#!/bin/sh\necho not a binary at all, but padded to pass the size floor........\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	failures := checkCommittedBinaries(filepath.Join(root, "hooks", "bin"))
	assertRuleFired(t, failures, "module-binary-format")
}

// TestModuleGatesPassWithoutCommittedBinaries is the transitional scenario:
// hooks/bin/* binaries will stop being committed, leaving hooks/SHA256SUMS
// as the only thing tying a fetched binary to the source. With no committed
// binaries at all, RunModuleGates must find nothing to report — the rebuild
// comparison has nothing to compare against, and the manifest still agrees
// with the fresh build.
func TestModuleGatesPassWithoutCommittedBinaries(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	if err := os.RemoveAll(filepath.Join(root, "hooks", "bin")); err != nil {
		t.Fatal(err)
	}
	failures, err := RunModuleGates(root)
	assertGreen(t, failures, err)
}

// TestModuleGatesBiteManifestMismatch is the other #607-shaped gap once
// hooks/bin holds no committed binaries: with nothing to rebuild-compare
// against, hooks/SHA256SUMS is the only thing left tying a fetched binary
// to this source, so a source change that the manifest was never
// regenerated for must still be caught. It also proves the ordering: the
// smoke would fire module-argv-contract if run against this fresh build
// (the same mutation TestModuleGatesBiteArgvContract uses), so its absence
// here proves the manifest disagreement gated it off before it could run.
func TestModuleGatesBiteManifestMismatch(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	if err := os.RemoveAll(filepath.Join(root, "hooks", "bin")); err != nil {
		t.Fatal(err)
	}
	mutateTinyFile(t, root, "hooks/go/cmd/hook/main.go", "fmt.Fprintln(stdout, \"{}\")", "// silent codex")

	failures, err := RunModuleGates(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "module-rebuild-manifest")
	for _, f := range failures {
		if f.Rule == "module-argv-contract" {
			t.Fatalf("module-argv-contract fired (%v): the smoke ran despite the source/manifest disagreement", f)
		}
	}
}

func TestModuleGatesBiteStrayBinary(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	// A dispatchable stray: run-hook.cmd would resolve this name on a
	// hypothetical Plan 9 host, but no contract check covers it. The dotfile
	// alongside it must NOT fire — OS metadata appears locally and is
	// git-ignored.
	stray := filepath.Join(root, "hooks", "bin", "agx-plan9-amd64")
	if err := os.WriteFile(stray, []byte("stray"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hooks", "bin", ".DS_Store"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	failures := checkCommittedBinaries(filepath.Join(root, "hooks", "bin"))
	assertRuleFired(t, failures, "module-bin-roster")
	for _, f := range failures {
		if f.Rule == "module-bin-roster" && f.Path != "hooks/bin/agx-plan9-amd64" {
			t.Fatalf("module-bin-roster fired on %q; only the stray binary should fire (dotfiles are exempt)", f.Path)
		}
	}
}

func TestModuleGatesBiteArgvContract(t *testing.T) {
	skipIfShort(t)
	t.Run("codex", func(t *testing.T) {
		root := cloneTinyRepo(t)
		// Break the codex body contract in the source, rebuild only the HOST
		// binary from it (stamp irrelevant to this gate): codex must ALWAYS
		// emit `{}`, and exit 0 alone hides its absence.
		mutateTinyFile(t, root, "hooks/go/cmd/hook/main.go", "fmt.Fprintln(stdout, \"{}\")", "// silent codex")
		rebuildTinyHost(t, root, "agentics-src-deadbeef")
		failures := argvSmoke(filepath.Join(root, "hooks", "bin", hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH)))
		assertRuleFired(t, failures, "module-argv-contract")
	})
	t.Run("antigravity", func(t *testing.T) {
		root := cloneTinyRepo(t)
		// Break the antigravity body contract: antigravity must emit
		// `{"decision":"allow"}` on empty stdin, never silence.
		mutateTinyFile(t, root, "hooks/go/cmd/hook/main.go", `fmt.Fprintln(stdout, "{\"decision\":\"allow\"}")`, "// silent antigravity")
		rebuildTinyHost(t, root, "agentics-src-deadbeef")
		failures := argvSmoke(filepath.Join(root, "hooks", "bin", hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH)))
		assertRuleFired(t, failures, "module-argv-contract")
	})
}

// TestRunModuleGatesSkipsArgvSmokeOnRebuildMismatch is the ordering half of
// the #611 review: a substituted committed host binary must be caught by the
// rebuild comparison BEFORE the argv/dispatch smoke ever executes a fresh
// build. The substitute here always exits 42 regardless of argv — nothing
// the hooks module's own source would ever produce — so if the smoke ran at
// all here it could only run against a fresh build, never this substitute;
// its absence from the failures proves the smoke was skipped, not merely
// that it passed. The source/manifest-disagreement half of the ordering
// (the smoke must also be skipped when a fresh build disagrees with
// hooks/SHA256SUMS) is covered by TestModuleGatesBiteManifestMismatch.
func TestRunModuleGatesSkipsArgvSmokeOnRebuildMismatch(t *testing.T) {
	skipIfShort(t)
	root := cloneTinyRepo(t)
	hostName := hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH)
	if hostName == "" {
		t.Skip("host has no contract binary target")
	}
	hostBin := filepath.Join(root, "hooks", "bin", hostName)
	buildAlwaysExit42(t, hostBin)

	failures, err := RunModuleGates(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRuleFired(t, failures, "module-rebuild-mismatch")
	for _, f := range failures {
		if f.Rule == "module-argv-contract" {
			t.Fatalf("module-argv-contract fired (%v): the substituted binary was executed before the rebuild mismatch on it was resolved", f)
		}
	}
}

// buildAlwaysExit42 compiles a standalone program — unrelated to the hooks
// module's source — that exits 42 for any argv or stdin, and writes it to
// dest. It stands in for a substituted host binary: a valid executable for
// the host's GOOS/GOARCH whose behavior a real hooks binary would never
// produce, so running it is detectable.
func buildAlwaysExit42(t *testing.T, dest string) {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module example.com/impostor\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	main := "package main\n\nimport \"os\"\n\nfunc main() { os.Exit(42) }\n"
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runToolEnv(src, append(os.Environ(), "CGO_ENABLED=0"), "go", "build", "-o", dest, ".")
	if err != nil {
		t.Fatalf("building the impostor binary: %v\n%s", err, out)
	}
}

// TestMachineFormatAcceptsEveryContractTarget cross-builds a tiny program for
// every ContractTargets entry and proves checkMachineFormat accepts each
// one's real machine format — the widened six-binary contract only holds if
// the format checker recognizes every name in it, not just the ones this
// host happens to run.
func TestMachineFormatAcceptsEveryContractTarget(t *testing.T) {
	skipIfShort(t)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module example.com/machineformat\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	for name, target := range hookbuild.ContractTargets {
		dest := filepath.Join(out, name)
		env := append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target[0], "GOARCH="+target[1])
		combined, err := runToolEnv(src, env, "go", "build", "-o", dest, ".")
		if err != nil {
			t.Fatalf("building %s (%s/%s): %v\n%s", name, target[0], target[1], err, combined)
		}
		if msg := checkMachineFormat(dest, name); msg != "" {
			t.Errorf("checkMachineFormat(%s) = %q, want accepted", name, msg)
		}
	}
}

func TestModuleGatesErrorWithoutHooksDir(t *testing.T) {
	if _, err := RunModuleGates(t.TempDir()); err == nil {
		t.Fatal("expected an error for a repo with no hooks/ module")
	}
}

func TestModuleTestArgsRunFreshWithCoverageAndSkipNothing(t *testing.T) {
	args := strings.Join(moduleTestArgs("p.out"), " ")
	if !strings.Contains(args, "-count=1") || !strings.Contains(args, "-coverprofile=p.out") {
		t.Fatalf("the module gate must run fresh tests with a coverage profile: %s", args)
	}
	if strings.Contains(args, "-skip") || strings.Contains(args, "-run") {
		t.Fatalf("the module gate must execute every test in the module: %s", args)
	}
}
