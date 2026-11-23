// Package hookbuild builds the six contract hook binaries, and the
// windows/amd64 agx launcher committed as bin/agx.exe, with the exact flags
// hooks/go/Makefile uses, so a binary that does not come from git is the same
// bytes hooks/SHA256SUMS (or, for the launcher, the committed bin/agx.exe)
// records. `hooktool build` and the module gate both build through it; CI
// itself uses the Makefile.
package hookbuild

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
)

// ContractBinaries is the hooks/go/Makefile output contract — run-hook.cmd
// selects a binary by GOOS/GOARCH name, so the six names are load-bearing.
var ContractBinaries = []string{
	"agx-darwin-amd64",
	"agx-darwin-arm64",
	"agx-linux-amd64",
	"agx-linux-arm64",
	"agx-windows-amd64.exe",
	"agx-windows-arm64.exe",
}

// ContractTargets maps each contract name to its GOOS/GOARCH pair.
var ContractTargets = map[string][2]string{
	"agx-darwin-amd64":      {"darwin", "amd64"},
	"agx-darwin-arm64":      {"darwin", "arm64"},
	"agx-linux-amd64":       {"linux", "amd64"},
	"agx-linux-arm64":       {"linux", "arm64"},
	"agx-windows-amd64.exe": {"windows", "amd64"},
	"agx-windows-arm64.exe": {"windows", "arm64"},
}

// BuildOpts and Ldflags restate hooks/go/Makefile's BUILDOPTS and LDFLAGS.
// The rebuild the module gate compares against must use the exact flag set
// that produced the committed bytes, and -buildvcs=false is what makes the
// comparison possible at all (the Makefile explains why). The two copies
// are pinned to each other by TestBuildArgsMatchMakefile.
var BuildOpts = []string{"-trimpath", "-buildvcs=false"}

// ArchTuningEnv pins the microarchitecture-tuning knobs `go build` otherwise
// reads from the environment (GOAMD64 for amd64, GOARM64 for arm64) to Go's
// own defaults, restating hooks/go/Makefile's same pin. Two checkouts with
// identical source, Go version, and flags still produce different bytes
// under e.g. GOAMD64=v3 vs. the default v1 — a value neither the module
// gate nor the digest/manifest verdicts observe — so an inherited host
// setting would misreport a valid checkout as module-rebuild-mismatch. The
// values are irrelevant to a target whose GOARCH they do not tune; go build
// ignores them silently.
var ArchTuningEnv = []string{"GOAMD64=v1", "GOARM64=v8.0"}

// Ldflags returns the linker flags for a build stamped with the given
// source-digest value ("agentics-src-<hex>").
func Ldflags(stamp string) string {
	return "-s -w -X main.sourceDigest=" + stamp
}

// BuildArgs returns the `go build` argument list for one contract target
// written to out.
func BuildArgs(stamp, out string) []string {
	args := append([]string{"build"}, BuildOpts...)
	return append(args, "-ldflags", Ldflags(stamp), "-o", out, "./cmd/hook")
}

// LauncherPackage is the package the agx launcher builds from. The launcher
// is not a contract binary: run-hook.cmd never selects it and
// hooks/SHA256SUMS does not list it.
const LauncherPackage = "./cmd/launcher"

// LauncherTarget is the one GOOS/GOARCH pair the launcher is built for.
var LauncherTarget = [2]string{"windows", "amd64"}

// LauncherOutput is the launcher's committed path, relative to the
// repository root rather than to hooks/.
const LauncherOutput = "bin/agx.exe"

// LauncherBuildArgs returns the `go build` argument list for the launcher
// written to out. It differs from BuildArgs(stamp, out) only in the package.
func LauncherBuildArgs(stamp, out string) []string {
	args := BuildArgs(stamp, out)
	args[len(args)-1] = LauncherPackage
	return args
}

// HostBinary returns the contract binary name for a GOOS/GOARCH pair, or ""
// when the platform has no contract target.
func HostBinary(goos, goarch string) string {
	for name, target := range ContractTargets {
		if target[0] == goos && target[1] == goarch {
			return name
		}
	}
	return ""
}

// Request names what Rebuild builds. SourceDir is the module root; GoBin the
// go command; OutDir the directory the binaries are written to, which may be
// relative to the caller's working directory. Digest is the source digest to
// stamp; empty means Rebuild computes it from SourceDir. Names are contract
// binary names, built in order. Launcher also builds the agx launcher.
type Request struct {
	SourceDir, GoBin, OutDir, Digest string
	Names                            []string
	Launcher                         bool
}

// Target is one binary Rebuild attempted. Path is the absolute file it was
// written to. Output is the go command's raw combined output. Err is nil when
// the build succeeded, else "hookbuild: go build <goos>/<goarch>: <cause>"
// followed by a newline and Output. Target carries no hash or build
// information: each caller measures the file itself, because each words its
// own failures.
type Target struct {
	Name, Path, Output string
	Err                error
}

// Result holds one Target per requested name, in request order, and the
// launcher's Target, which is nil unless the request set Launcher.
type Result struct {
	Targets  []Target
	Launcher *Target
}

// Rebuild builds each name in req.Names, in order (nil builds none), into
// <OutDir>/<name>, and with req.Launcher the launcher into <OutDir>/agx.exe,
// built with the Makefile's exact flag set and stamped with the source digest.
// It attempts every target: one failing build is recorded in its Target and the
// rest still run, so a caller sees every broken platform at once.
//
// Its error covers only what stops every target, checked in this order: a
// name outside ContractTargets ("hookbuild: <name> is not a contract
// binary"), checked for every name before anything runs; the source digest
// when req.Digest is empty; then resolving and creating OutDir. The last two
// read "hookbuild: <cause>". With an error the Result is empty and nothing
// was built.
func Rebuild(req Request) (Result, error) {
	for _, name := range req.Names {
		if _, ok := ContractTargets[name]; !ok {
			return Result{}, fmt.Errorf("hookbuild: %s is not a contract binary", name)
		}
	}
	digest := req.Digest
	if digest == "" {
		var err error
		if digest, err = hookprovenance.SourceDigest(req.SourceDir); err != nil {
			return Result{}, fmt.Errorf("hookbuild: %w", err)
		}
	}
	// go build runs from SourceDir, so a relative -o would resolve there
	// rather than against the caller's working directory.
	outDir, err := filepath.Abs(req.OutDir)
	if err != nil {
		return Result{}, fmt.Errorf("hookbuild: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("hookbuild: %w", err)
	}
	stamp := "agentics-src-" + digest
	build := func(name string, target [2]string, args func(stamp, out string) []string) Target {
		out := filepath.Join(outDir, name)
		output, err := goBuild(req.SourceDir, req.GoBin, target, args(stamp, out))
		return Target{Name: name, Path: out, Output: string(output), Err: err}
	}
	var res Result
	for _, name := range req.Names {
		res.Targets = append(res.Targets, build(name, ContractTargets[name], BuildArgs))
	}
	if req.Launcher {
		launcher := build(filepath.Base(LauncherOutput), LauncherTarget, LauncherBuildArgs)
		res.Launcher = &launcher
	}
	return res, nil
}

// Verify reads a Result fail-fast and returns the first of: the first failed
// contract target's Err; the launcher's Err; "hookbuild: nothing was
// rebuilt" when there are no targets; the buildinfo.ReadFile error for the
// first target, unwrapped; "the hook binaries were built with <v>, but go.mod
// pins <pin>; build with that toolchain" when that target's Go version does
// not satisfy pin (see hookprovenance.ToolchainSatisfiesPin); then, per target
// in order, "hookbuild: hooks/SHA256SUMS has no entry for bin/<name>", a
// FileSHA256 error, or "hookbuild: <name> built as sha256 <h>, but
// hooks/SHA256SUMS records <want>". Every target came from one go command, so
// the first target's toolchain stands for all of them and the launcher. Verify
// does not judge the manifest's own faults; the caller applies its posture for
// them first.
func (r Result) Verify(pin string, m hookprovenance.Manifest) error {
	for _, target := range r.Targets {
		if target.Err != nil {
			return target.Err
		}
	}
	if r.Launcher != nil && r.Launcher.Err != nil {
		return r.Launcher.Err
	}
	if len(r.Targets) == 0 {
		return errors.New("hookbuild: nothing was rebuilt")
	}
	info, err := buildinfo.ReadFile(r.Targets[0].Path)
	if err != nil {
		return err
	}
	if !hookprovenance.ToolchainSatisfiesPin(info.GoVersion, pin) {
		return fmt.Errorf("the hook binaries were built with %s, but go.mod pins %s; build with that toolchain", info.GoVersion, pin)
	}
	for _, target := range r.Targets {
		want, ok := m.Hash("bin/" + target.Name)
		if !ok {
			return fmt.Errorf("hookbuild: hooks/SHA256SUMS has no entry for bin/%s", target.Name)
		}
		have, err := FileSHA256(target.Path)
		if err != nil {
			return err
		}
		if have != want {
			return fmt.Errorf("hookbuild: %s built as sha256 %s, but hooks/SHA256SUMS records %s", target.Name, have, want)
		}
	}
	return nil
}

// ManifestRoster compares the manifest's paths with the six contract
// binaries. missing lists each absent "bin/<name>" in ContractBinaries order;
// extra lists every other recorded path, sorted. Each is nil when empty. A
// missing entry leaves run-hook.cmd unable to verify that platform's download,
// and an extra one is a hand edit build-all never writes.
func ManifestRoster(m hookprovenance.Manifest) (missing, extra []string) {
	contract := make(map[string]bool, len(ContractBinaries))
	for _, name := range ContractBinaries {
		path := "bin/" + name
		contract[path] = true
		if _, ok := m.Hash(path); !ok {
			missing = append(missing, path)
		}
	}
	for _, path := range m.Paths() {
		if !contract[path] {
			extra = append(extra, path)
		}
	}
	slices.Sort(extra)
	return missing, extra
}

// goBuild runs goBin with args from hooksDir for target and returns its
// combined output. cgo is off and the tuning knobs are pinned last, so an
// inherited host setting cannot change the bytes.
func goBuild(hooksDir, goBin string, target [2]string, args []string) ([]byte, error) {
	env := append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target[0], "GOARCH="+target[1])
	env = append(env, ArchTuningEnv...)
	cmd := exec.Command(goBin, args...)
	cmd.Dir = hooksDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("hookbuild: go build %s/%s: %w\n%s", target[0], target[1], err, output)
	}
	return output, nil
}

// FileSHA256 returns the lowercase hex SHA-256 digest of the file at path.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("hookbuild: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hookbuild: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
