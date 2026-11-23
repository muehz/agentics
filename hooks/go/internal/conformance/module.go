package conformance

import (
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
)

// engineRel is the repo-relative directory of the Go module that builds the
// hook binary. It is maintainer-facing source and deliberately sits OUTSIDE
// the plugin's hooks/ directory, which holds only what a consumer installs.
const engineRel = "hooks/go"

// coverageFloorPercent is the statement-coverage floor for the hooks module,
// measured with every package in the denominator. It is the Phase 1 exit
// gate of the hooks/go re-baseline: no rewrite lands against an unmeasured
// branch.
const coverageFloorPercent = 90.0

// coverageTotal reads the module-wide statement coverage out of a profile
// via `go tool cover -func`, whose last line is `total: (statements) NN.N%`.
func coverageTotal(hooksDir, goBin, profile string) (float64, error) {
	out, err := runTool(hooksDir, goBin, "tool", "cover", "-func="+profile)
	if err != nil {
		return 0, fmt.Errorf("go tool cover: %v\n%s", err, tail(out, 10))
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "total:" {
			var total float64
			if _, err := fmt.Sscanf(fields[len(fields)-1], "%f%%", &total); err != nil {
				return 0, fmt.Errorf("unparseable total line %q", line)
			}
			return total, nil
		}
	}
	return 0, fmt.Errorf("no coverage data: the module has no tests, or none of its packages were instrumented")
}

// RunModuleGates ports the exec gates of validate.yml's build-hook-binary
// job: it runs the checkout's own Go toolchain against the hooks/ module.
// Gates: no third-party dependencies, gofmt, go vet, go test, the build
// contract (all six GOOS/GOARCH targets build, into a temp dir — never
// overwriting the checkout's hooks/bin — and each one matches both the
// committed hooks/bin binary, when present, and its hooks/SHA256SUMS entry
// byte for byte), the machine format of each COMMITTED hooks/bin binary,
// and the argv/dispatch smoke against the freshly rebuilt host binary. A
// missing toolchain is an error (could not audit), never a pass. The smoke
// only runs once the rebuild comparison and the committed-binary checks are
// both clean: those checks are what vet the fresh build the smoke then
// executes, so a source/manifest disagreement or a malformed committed
// artifact must be caught first, never run past.
func RunModuleGates(repoRoot string) ([]Failure, error) {
	return runModuleGates(repoRoot, AuditOptions{})
}

// skipTestsLine is the stderr notice runModuleGates prints when skipTests is
// set, naming both gates it leaves out.
const skipTestsLine = "SKIP: hooks module-test and module-coverage-floor (--skip-tests); run hooktool audit without --skip-tests for the module tests and the 90% coverage gate"

// runModuleGates runs RunModuleGates' gates. With skipTests set it leaves out
// module-test and module-coverage-floor on every host and prints
// skipTestsLine to stderr instead; every other gate still runs.
func runModuleGates(repoRoot string, options AuditOptions) ([]Failure, error) {
	if options.SourceOnly {
		// No command from the audited module may run before its index clears.
		if found, err := checkTrackedNative(repoRoot); err != nil || len(found) != 0 {
			return found, err
		}
	}
	var failures []Failure
	hooksDir := filepath.Join(repoRoot, engineRel)
	if info, err := os.Stat(hooksDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("hooks module gates: %s is not a directory", hooksDir)
	}
	// The committed binaries are NOT in the module: they ship with the plugin.
	binDir := filepath.Join(repoRoot, "hooks", "bin")
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("hooks module gates: the Go toolchain is required (go not found on PATH)")
	}
	gofmtBin, err := exec.LookPath("gofmt")
	if err != nil {
		return nil, fmt.Errorf("hooks module gates: gofmt is required and was not found on PATH")
	}

	// Gate: no third-party dependencies. A third-party import would put
	// unvetted code inside a hook that runs on every tool call. Parsing go.mod
	// finds dependencies without a network-capable toolchain call.
	if f, err := checkNoThirdParty(hooksDir); err != nil {
		return failures, err
	} else {
		failures = append(failures, f...)
	}

	if f, err := checkGofmt(hooksDir, gofmtBin); err != nil {
		return failures, err
	} else {
		failures = append(failures, f...)
	}
	failures = append(failures, checkVet(hooksDir, goBin)...)
	if options.SkipTests {
		fmt.Fprintln(os.Stderr, skipTestsLine)
	} else if f, err := checkHostTests(hooksDir, goBin, runtime.GOOS); err != nil {
		return failures, err
	} else {
		failures = append(failures, f...)
	}

	buildContractFailures, buildDir, err := checkBuildContract(hooksDir, binDir, filepath.Join(repoRoot, "hooks", "SHA256SUMS"), goBin, options.SourceOnly)
	if err != nil {
		return failures, err
	}
	defer os.RemoveAll(buildDir)
	failures = append(failures, buildContractFailures...)
	var committedBinaryFailures []Failure
	if !options.SourceOnly {
		committedBinaryFailures = checkCommittedBinaries(binDir)
	} else {
		committedBinaryFailures = checkBinRoster(binDir)
	}
	failures = append(failures, committedBinaryFailures...)

	// Gate: the argv/dispatch smoke, against the freshly rebuilt HOST binary
	// (already vetted by the rebuild comparison and the manifest check
	// above). The twelve provider x hook pairs run with no hook arguments
	// and empty stdin. The expectations encode each surface's fail posture:
	// claude-code fails CLOSED at 1 with no body; copilot fails OPEN at 0
	// with no body; codex fails open at 0 and emits `{}`; antigravity fails
	// open at 0 and emits `{"decision":"allow"}`. Argv validation must exit
	// 1 — never 2, Claude Code's blocking signal.
	//
	// This runs a binary this checkout just built, so it must never execute
	// one the preceding gates already proved unsound: a source/manifest
	// disagreement caught by the rebuild or manifest comparison, or a
	// committed artifact that failed the machine-format/roster check, would
	// otherwise still get code execution here before conformance reports the
	// failure.
	if len(buildContractFailures) == 0 && len(committedBinaryFailures) == 0 {
		hostName := hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH)
		if hostName == "" {
			// No contract target for this host — the module gate cannot
			// exercise dispatch here. Callers on contract platforms (all of
			// CI) do.
			failures = append(failures, Failure{
				Rule: "module-argv-contract", Path: "hooks/bin",
				Message: fmt.Sprintf("host %s/%s has no contract binary — the dispatch smoke cannot run here", runtime.GOOS, runtime.GOARCH),
			})
		} else {
			failures = append(failures, argvSmoke(filepath.Join(buildDir, hostName))...)
		}
	}

	return failures, nil
}

// checkNoThirdParty parses hooks/go/go.mod for require directives.
func checkNoThirdParty(hooksDir string) ([]Failure, error) {
	data, err := os.ReadFile(filepath.Join(hooksDir, "go.mod"))
	if err != nil {
		return nil, fmt.Errorf("hooks module gates: reading go.mod: %w", err)
	}
	var deps []string
	inBlock := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		switch {
		case inBlock:
			if line == ")" {
				inBlock = false
			} else if line != "" {
				deps = append(deps, line)
			}
		case line == "require (":
			inBlock = true
		case strings.HasPrefix(line, "require "):
			deps = append(deps, strings.TrimPrefix(line, "require "))
		}
	}
	if len(deps) > 0 {
		return []Failure{{
			Rule: "module-no-third-party", Path: "hooks/go/go.mod",
			Message: fmt.Sprintf("the hook binary must stay dependency-free; go.mod requires: %v", deps),
		}}, nil
	}
	return nil, nil
}

// checkGofmt reports files that gofmt would rewrite.
func checkGofmt(hooksDir, gofmtBin string) ([]Failure, error) {
	out, err := runTool(hooksDir, gofmtBin, "-l", ".")
	if err != nil {
		return nil, fmt.Errorf("hooks module gates: gofmt -l failed: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	return []Failure{{
		Rule: "module-gofmt", Path: "hooks",
		Message: fmt.Sprintf("gofmt would rewrite:\n%s", strings.TrimSpace(out)),
	}}, nil
}

// checkVet runs go vet over every package in the hooks module.
func checkVet(hooksDir, goBin string) []Failure {
	out, err := runTool(hooksDir, goBin, "vet", "./...")
	if err == nil {
		return nil
	}
	return []Failure{{
		Rule: "module-vet", Path: "hooks",
		Message: fmt.Sprintf("go vet ./... failed:\n%s", tail(out, 30)),
	}}
}

// checkHostTests leaves the command integration test executable to Unix hosts.
// Defender can quarantine that executable before its security fixtures run.
// A partial Windows run cannot establish the module-wide coverage floor.
func checkHostTests(hooksDir, goBin, goos string) ([]Failure, error) {
	if goos != "windows" {
		return checkTestsAndCoverage(hooksDir, goBin)
	}
	listed, err := runTool(hooksDir, goBin, "list", "./...")
	if err != nil {
		return nil, fmt.Errorf("hooks module gates: list test packages: %v\n%s", err, listed)
	}
	var packages []string
	for _, pkg := range strings.Fields(listed) {
		if !strings.HasSuffix(pkg, "/cmd/hook") {
			packages = append(packages, pkg)
		}
	}
	fmt.Fprintln(os.Stderr, "SKIP: hooks cmd/hook integration tests and module-wide coverage on native Windows; run the complete suite and 90% coverage gate on Linux/macOS")
	if len(packages) == 0 {
		return nil, nil
	}
	args := append([]string{"test", "-count=1"}, packages...)
	out, testErr := runToolEnv(hooksDir, append(os.Environ(), "CGO_ENABLED=0", "AGX_HOOK_AUDIT_GATE=1"), goBin, args...)
	if testErr != nil {
		return []Failure{{
			Rule: "module-test", Path: "hooks",
			Message: fmt.Sprintf("native Windows hook package tests failed:\n%s", tail(out, 40)),
		}}, nil
	}
	return nil, nil
}

// moduleTestArgs is the go test command line for the module gate. An audit
// must execute tests, not reuse an earlier result, and it skips none of them.
func moduleTestArgs(profile string) []string {
	return []string{"test", "./...", "-count=1", "-coverpkg=./...", "-coverprofile=" + profile}
}

// checkTestsAndCoverage runs the module tests once and enforces the statement
// coverage floor over every package in the module.
func checkTestsAndCoverage(hooksDir, goBin string) ([]Failure, error) {
	profile, err := os.CreateTemp("", "agentics-hooks-cover.*.out")
	if err != nil {
		return nil, fmt.Errorf("hooks module gates: %w", err)
	}
	profile.Close()
	defer os.Remove(profile.Name())

	out, err := runToolEnv(hooksDir, append(os.Environ(), "CGO_ENABLED=0", "AGX_HOOK_AUDIT_GATE=1"), goBin, moduleTestArgs(profile.Name())...)
	if err != nil {
		return []Failure{{
			Rule: "module-test", Path: "hooks",
			Message: fmt.Sprintf("go test ./... failed:\n%s", tail(out, 40)),
		}}, nil
	}
	total, err := coverageTotal(hooksDir, goBin, profile.Name())
	if err != nil {
		return []Failure{{
			Rule: "module-coverage-floor", Path: "hooks",
			Message: fmt.Sprintf("could not read the module's coverage total: %v", err),
		}}, nil
	}
	if total < coverageFloorPercent {
		return []Failure{{
			Rule: "module-coverage-floor", Path: "hooks",
			Message: fmt.Sprintf("statement coverage is %.1f%%, under the %.0f%% floor; add cases for the uncovered branches (go tool cover -func) rather than lowering the floor", total, coverageFloorPercent),
		}}, nil
	}
	return nil, nil
}

// checkBuildContract cross-compiles every supported hook artifact into a
// temporary directory with the Makefile's exact flag set, verifies its
// machine format, and then compares it byte for byte with the committed
// hooks/bin binary, when one is present, and with its hooks/SHA256SUMS
// entry; a missing committed binary is skipped. It also rebuilds the agx
// launcher and compares it with the committed bin/agx.exe, reporting a
// difference as module-rebuild-mismatch with Path bin/agx.exe. The source stamp, the
// toolchain build info, and the manifest entry that RunFreshness checks are
// all data the artifact carries; only a rebuild proves the committed bytes
// — or a bytes-to-be-fetched manifest entry — came from this source. A
// toolchain that does not satisfy the go.mod pin builds different bytes
// from identical source, so it is reported once as its own rule and the
// comparison is skipped for that run. The caller owns buildDir's cleanup:
// the argv/dispatch smoke runs the fresh host build out of it after this
// function returns, so it must not be removed here. buildDir is "" when err
// is non-nil.
func checkBuildContract(hooksDir, binDir, manifestPath, goBin string, sourceOnly bool) ([]Failure, string, error) {
	digest, err := hookprovenance.SourceDigest(hooksDir)
	if err != nil {
		return nil, "", fmt.Errorf("hooks module gates: %w", err)
	}
	pin, err := hookprovenance.PinnedToolchain(hooksDir)
	if err != nil {
		return nil, "", fmt.Errorf("hooks module gates: %w", err)
	}
	buildDir, err := os.MkdirTemp("", "agentics-module-gates.")
	if err != nil {
		return nil, "", fmt.Errorf("hooks module gates: %w", err)
	}

	// An unreadable manifest, or one that vouches for nothing, is
	// freshness-manifest's finding (RunFreshness), not repeated here; the
	// rebuild-manifest comparison below simply does not run.
	manifest, manifestErr := hookprovenance.LoadManifest(manifestPath)
	if manifestErr == nil {
		manifestErr = manifest.Unusable()
	}

	result, err := hookbuild.Rebuild(hookbuild.Request{
		SourceDir: hooksDir, GoBin: goBin, OutDir: buildDir, Digest: digest,
		Names: hookbuild.ContractBinaries, Launcher: true,
	})
	if err != nil {
		os.RemoveAll(buildDir)
		return nil, "", fmt.Errorf("hooks module gates: %w", err)
	}

	var failures []Failure
	toolchainChecked, compare := false, true
	for _, built := range result.Targets {
		name, fresh := built.Name, built.Path
		target := hookbuild.ContractTargets[name]
		if built.Err != nil {
			failures = append(failures, Failure{
				Rule: "module-build-contract", Path: "hooks",
				Message: fmt.Sprintf("go build for %s/%s (%s) failed:\n%s", target[0], target[1], name, tail(built.Output, 30)),
			})
			continue
		}
		if msg := checkMachineFormat(fresh, name); msg != "" {
			failures = append(failures, Failure{
				Rule: "module-build-contract", Path: "hooks",
				Message: fmt.Sprintf("fresh build %s: %s", name, msg),
			})
			continue
		}
		if sourceOnly {
			failures = append(failures, checkGeneratedStamp(fresh, "hooks/bin/"+name, digest)...)
		}
		// One toolchain builds all six targets, so the first good artifact
		// answers for the run.
		if !toolchainChecked {
			toolchainChecked = true
			if f := checkRebuildToolchain(fresh, pin); f != nil {
				failures = append(failures, *f)
				compare = false
			}
		}
		if compare {
			if manifestErr == nil {
				failures = append(failures, checkRebuildManifest(name, fresh, manifest)...)
			}
			if !sourceOnly {
				failures = append(failures, checkRebuildMatches("hooks/bin/"+name, fresh, filepath.Join(binDir, name))...)
			}
		}
	}

	// The launcher is rebuilt the way hooks/go/Makefile builds bin/agx.exe and
	// compared with the committed copy under the same toolchain gate. It has
	// no manifest entry, so only the byte comparison applies.
	if launcher := result.Launcher; launcher.Err != nil {
		failures = append(failures, Failure{
			Rule: "module-build-contract", Path: "hooks",
			Message: fmt.Sprintf("building the agx launcher failed:\n%s", tail(launcher.Err.Error(), 30)),
		})
	} else if sourceOnly {
		failures = append(failures, checkGeneratedLauncher(launcher.Path, pin, digest)...)
	} else if compare {
		failures = append(failures, checkRebuildMatches(hookbuild.LauncherOutput, launcher.Path, launcherPath(hooksDir))...)
	}
	return failures, buildDir, nil
}

func checkGeneratedLauncher(path, pin, digest string) []Failure {
	const rel = hookbuild.LauncherOutput
	if msg := checkMachineFormat(path, launcherFormat); msg != "" {
		return []Failure{{Rule: "module-build-contract", Path: rel, Message: msg}}
	}
	if failure := checkRebuildToolchain(path, pin); failure != nil {
		return []Failure{*failure}
	}
	return checkGeneratedStamp(path, rel, digest)
}

func checkGeneratedStamp(path, rel, digest string) []Failure {
	data, err := os.ReadFile(path)
	if err != nil {
		return []Failure{{Rule: "module-build-contract", Path: rel, Message: err.Error()}}
	}
	if !bytes.Equal(digestStampPattern.Find(data), []byte("agentics-src-"+digest)) {
		return []Failure{{Rule: "module-build-contract", Path: rel, Message: "fresh artifact does not embed the current source digest"}}
	}
	return nil
}

// launcherPath returns the committed launcher's path from hooksChild, a
// directory directly under <repo>/hooks (hooks/go or hooks/bin). The
// launcher sits at <repo>/bin/agx.exe, outside the plugin's hooks/ directory.
func launcherPath(hooksChild string) string {
	return filepath.Join(hooksChild, "..", "..", filepath.FromSlash(hookbuild.LauncherOutput))
}

// checkRebuildManifest compares a fresh artifact with the hooks/SHA256SUMS
// entry run-hook.cmd verifies a download against. With no committed
// binaries this is the only thing tying the manifest to the source.
func checkRebuildManifest(name, fresh string, manifest hookprovenance.Manifest) []Failure {
	want, ok := manifest.Hash("bin/" + name)
	if !ok {
		// freshness reports a missing entry; this rule only speaks to a
		// present one that disagrees with the source.
		return nil
	}
	have, err := hookbuild.FileSHA256(fresh)
	if err != nil {
		return []Failure{{
			Rule: "module-rebuild-manifest", Path: manifestRel,
			Message: fmt.Sprintf("hashing the fresh build of %s: %v", name, err),
		}}
	}
	if have == want {
		return nil
	}
	return []Failure{{
		Rule: "module-rebuild-manifest", Path: manifestRel,
		Message: fmt.Sprintf("this source builds %s as sha256 %s, but hooks/SHA256SUMS records %s — a fetched binary would be discarded; rebuild with the pinned toolchain (make -C hooks/go build-all) and commit the manifest", name, have, want),
	}}
}

// checkRebuildToolchain reads the Go version the linker recorded in the
// fresh artifact and compares it with the go.mod pin, the same way
// checkToolchainStamp does for a committed binary. Returns nil when the
// pin is satisfied.
func checkRebuildToolchain(fresh, pin string) *Failure {
	info, err := buildinfo.ReadFile(fresh)
	if err != nil {
		return &Failure{
			Rule: "module-rebuild-toolchain", Path: "hooks/go/go.mod",
			Message: fmt.Sprintf("the fresh build carries no Go build information (%v); the rebuild comparison cannot run", err),
		}
	}
	if hookprovenance.ToolchainSatisfiesPin(info.GoVersion, pin) {
		return nil
	}
	return &Failure{
		Rule: "module-rebuild-toolchain", Path: "hooks/go/go.mod",
		Message: fmt.Sprintf("the fresh build was linked by %s, but hooks/go/go.mod pins %s — a different Go release builds different bytes from identical source, so the rebuild comparison would fail for the wrong reason and was skipped; run with GOTOOLCHAIN=auto or install %s", info.GoVersion, pin, pin),
	}
}

// checkRebuildMatches compares a fresh artifact with the committed binary
// at the repo-relative path rel. A committed file that does not exist is not
// this rule's finding: a missing hook binary is fetched and verified against
// the manifest, and a missing launcher is bin-launchers' finding.
func checkRebuildMatches(rel, fresh, committed string) []Failure {
	committedData, err := os.ReadFile(committed)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return []Failure{{Rule: "module-rebuild-mismatch", Path: rel, Message: fmt.Sprintf("unreadable: %v", err)}}
	}
	freshData, err := os.ReadFile(fresh)
	if err != nil {
		return []Failure{{Rule: "module-rebuild-mismatch", Path: rel, Message: fmt.Sprintf("fresh build unreadable: %v", err)}}
	}
	if bytes.Equal(freshData, committedData) {
		return nil
	}
	committedSum, freshSum := sha256.Sum256(committedData), sha256.Sum256(freshData)
	return []Failure{{
		Rule: "module-rebuild-mismatch", Path: rel,
		Message: fmt.Sprintf("the committed binary (sha256 %s) is not what this source builds with hooks/go/Makefile's flags (sha256 %s) — the source stamp, the toolchain, and the manifest are data the artifact carries, and a binary nobody built from this checkout can satisfy all three; rebuild with the pinned toolchain and commit the rebuilt binaries and manifest (make -C hooks/go build-all), and if a rebuild still differs do not run these hooks", hex.EncodeToString(committedSum[:]), hex.EncodeToString(freshSum[:])),
	}}
}

// checkCommittedBinaries verifies the machine format and exact roster of the
// hook artifacts committed under hooks/bin, and the machine format (PE32+,
// x86-64) of the agx launcher committed at bin/agx.exe, reported with Path
// bin/agx.exe. binDir is <repo>/hooks/bin, which locates the launcher.
// hooks/bin is transitional — the binaries are moving to a download the
// manifest verifies — so a missing directory, or a missing individual binary
// inside a present one, is not a finding of this rule. A missing launcher is
// bin-launchers' finding, and bin/'s roster is that rule's too.
func checkCommittedBinaries(binDir string) []Failure {
	var failures []Failure
	launcher := launcherPath(binDir)
	if _, err := os.Stat(launcher); err == nil {
		if msg := checkMachineFormat(launcher, launcherFormat); msg != "" {
			failures = append(failures, Failure{Rule: "module-binary-format", Path: hookbuild.LauncherOutput, Message: msg})
		}
	}
	if _, err := os.Stat(binDir); os.IsNotExist(err) {
		return failures
	}
	for _, name := range hookbuild.ContractBinaries {
		committed := filepath.Join(binDir, name)
		if _, err := os.Stat(committed); os.IsNotExist(err) {
			continue
		}
		if msg := checkMachineFormat(committed, name); msg != "" {
			failures = append(failures, Failure{
				Rule: "module-binary-format", Path: "hooks/bin/" + name,
				Message: msg,
			})
		}
	}

	return append(failures, checkBinRoster(binDir)...)
}

func checkBinRoster(binDir string) []Failure {
	var failures []Failure
	entries, err := os.ReadDir(binDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return append(failures, Failure{
			Rule: "module-bin-roster", Path: "hooks/bin",
			Message: fmt.Sprintf("could not read the committed binary directory: %v", err),
		})
	}
	contract := make(map[string]struct{}, len(hookbuild.ContractBinaries))
	for _, name := range hookbuild.ContractBinaries {
		contract[name] = struct{}{}
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if _, ok := contract[name]; !ok {
			failures = append(failures, Failure{
				Rule: "module-bin-roster", Path: "hooks/bin/" + name,
				Message: "hooks/bin/ may hold only the six Makefile contract binaries; delete this stray entry — run-hook.cmd resolves binary names dynamically, so a stray agx-<os>-<arch> would dispatch with no freshness, digest, or format check",
			})
		}
	}
	return failures
}

// checkMachineFormat verifies a binary's magic bytes match its contract
// platform. Returns "" on success, a violation message otherwise.
func checkMachineFormat(path, name string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("unreadable: %v", err)
	}
	if len(data) < 64 {
		return fmt.Sprintf("too small to be an executable (%d bytes)", len(data))
	}
	switch name {
	case "agx-darwin-amd64":
		// Mach-O 64-bit little-endian magic, cputype x86_64.
		if !bytes.Equal(data[:4], []byte{0xCF, 0xFA, 0xED, 0xFE}) {
			return "not a 64-bit Mach-O binary"
		}
		if binary.LittleEndian.Uint32(data[4:8]) != 0x01000007 {
			return "Mach-O binary is not x86_64"
		}
	case "agx-darwin-arm64":
		// Mach-O 64-bit little-endian magic, cputype ARM64.
		if !bytes.Equal(data[:4], []byte{0xCF, 0xFA, 0xED, 0xFE}) {
			return "not a 64-bit Mach-O binary"
		}
		if binary.LittleEndian.Uint32(data[4:8]) != 0x0100000C {
			return "Mach-O binary is not arm64"
		}
	case "agx-linux-amd64", "agx-linux-arm64":
		if !bytes.Equal(data[:4], []byte{0x7F, 'E', 'L', 'F'}) {
			return "not an ELF binary"
		}
		machine := binary.LittleEndian.Uint16(data[18:20])
		want := uint16(0x3E) // EM_X86_64
		wantName := "x86-64"
		if name == "agx-linux-arm64" {
			want = 0xB7 // EM_AARCH64
			wantName = "aarch64"
		}
		if machine != want {
			return fmt.Sprintf("ELF machine 0x%X is not %s", machine, wantName)
		}
	case "agx-windows-amd64.exe":
		if !bytes.Equal(data[:2], []byte{'M', 'Z'}) {
			return "not a PE binary (missing MZ header)"
		}
		peOffset := binary.LittleEndian.Uint32(data[0x3C:0x40])
		if int(peOffset)+26 > len(data) || !bytes.Equal(data[peOffset:peOffset+4], []byte{'P', 'E', 0, 0}) {
			return "not a PE binary (missing PE signature)"
		}
		if binary.LittleEndian.Uint16(data[peOffset+4:peOffset+6]) != 0x8664 {
			return "PE binary is not x86-64"
		}
		if binary.LittleEndian.Uint16(data[peOffset+24:peOffset+26]) != 0x20B {
			return "PE binary is not PE32+"
		}
	case "agx-windows-arm64.exe":
		if !bytes.Equal(data[:2], []byte{'M', 'Z'}) {
			return "not a PE binary (missing MZ header)"
		}
		peOffset := binary.LittleEndian.Uint32(data[0x3C:0x40])
		if int(peOffset)+26 > len(data) || !bytes.Equal(data[peOffset:peOffset+4], []byte{'P', 'E', 0, 0}) {
			return "not a PE binary (missing PE signature)"
		}
		if binary.LittleEndian.Uint16(data[peOffset+4:peOffset+6]) != 0xAA64 {
			return "PE binary is not arm64"
		}
		if binary.LittleEndian.Uint16(data[peOffset+24:peOffset+26]) != 0x20B {
			return "PE binary is not PE32+"
		}
	default:
		return "unknown contract name"
	}
	return ""
}

// argvSmoke runs the 12-pair dispatch truth table and the 4 negative argv
// cases against hostBin, the freshly rebuilt host binary.
func argvSmoke(hostBin string) []Failure {
	var failures []Failure
	hostName := filepath.Base(hostBin)

	type expectation struct {
		exit   int
		stdout string
	}
	perProvider := map[string]expectation{
		"claude-code": {exit: 1, stdout: ""},
		"copilot":     {exit: 0, stdout: ""},
		"codex":       {exit: 0, stdout: "{}"},
		"antigravity": {exit: 0, stdout: `{"decision":"allow"}`},
	}
	for _, provider := range []string{"claude-code", "copilot", "codex", "antigravity"} {
		want := perProvider[provider]
		for _, hook := range []string{"command-guardian", "secrets-guardian", "release-guardian"} {
			exit, stdout := runSmoke(hostBin, provider, hook)
			// Trim trailing newlines before comparing the response body because
			// the contract concerns payload bytes, not line termination.
			got := strings.TrimRight(stdout, "\n")
			if exit != want.exit || got != want.stdout {
				failures = append(failures, Failure{
					Rule: "module-argv-contract", Path: "hooks/bin/" + hostName,
					Message: fmt.Sprintf("%s %s exited %d with stdout %q, want exit %d stdout %q", provider, hook, exit, got, want.exit, want.stdout),
				})
			}
		}
	}
	for _, bad := range [][]string{
		{"bogus-provider", "command-guardian"},
		{"claude-code", "bogus-hook"},
		{"claude-code"},
		{},
	} {
		exit, _ := runSmoke(hostBin, bad...)
		if exit != 1 {
			failures = append(failures, Failure{
				Rule: "module-argv-contract", Path: "hooks/bin/" + hostName,
				Message: fmt.Sprintf("argv %q exited %d, want 1 (usage) — argv validation must never read as a guardrail deny", strings.Join(bad, " "), exit),
			})
		}
	}
	return failures
}

// runSmoke invokes a hook binary with empty stdin, discarding stderr.
func runSmoke(bin string, args ...string) (int, string) {
	cmd := exec.Command(bin, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if cmd.ProcessState == nil {
		// The binary did not start (unreadable, wrong format, ...): report
		// -1 so the caller's expectation comparison fails with a real code.
		_ = err
		return -1, stdout.String()
	}
	return cmd.ProcessState.ExitCode(), stdout.String()
}

// runTool runs a command in dir, returning combined output; nonzero exit
// comes back as the error.
func runTool(dir, bin string, args ...string) (string, error) {
	return runToolEnv(dir, append(os.Environ(), "CGO_ENABLED=0"), bin, args...)
}

func runToolEnv(dir string, env []string, bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// tail returns the last n lines of s.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
