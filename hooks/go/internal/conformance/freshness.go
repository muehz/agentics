package conformance

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
)

var digestStampPattern = regexp.MustCompile(`agentics-src-[0-9a-f]{64}`)

// RunFreshness is the staleness guard for the committed hook binaries in
// hooks/bin/ and the committed agx launcher bin/agx.exe. It is a PROVENANCE
// check: it reads bytes and executes nothing, so it covers all six platform
// binaries equally — including the five this host cannot run, and on a host
// with no contract target at all.
//
// It first applies the bin-launchers rule (checkBinLaunchers): bin/ holds
// exactly agx and agx.exe, bin/agx has no CR byte, and .gitattributes marks
// bin/agx.exe binary. Those findings are returned even when the source
// digest later cannot be computed.
//
// Three verdicts per committed hook binary; the launcher gets the first two
// with Path bin/agx.exe, and no manifest verdict because nothing downloads it:
//
//   - source — the binary must embed the digest of the checked-out source the
//     binaries are built from (stamped at link time by hooks/go/Makefile from
//     cmd/sourcedigest, which prints hookprovenance.SourceDigest). The digest
//     skips _test.go files and the directories hooks/go/digest-exclude names,
//     whose code ships in no hook binary, so an edit confined to them leaves
//     the binaries current. A binary built from any other revision of the
//     covered source cannot carry the current digest.
//
//   - toolchain — the Go version the linker recorded in the binary's build
//     info must satisfy the version hooks/go/go.mod pins. This is the one
//     drift the digest cannot see: identical source built by a different
//     toolchain digests identically, and could still behave differently.
//
//   - manifest — hooks/SHA256SUMS must record the binary's SHA-256.
//     run-hook.cmd checks a downloaded binary against that file before it
//     runs one, so a stale entry makes the dispatcher discard the very
//     bytes the release ships.
//
// A binary that is not committed is not a finding; the manifest is checked
// either way.
func RunFreshness(repoRoot string) ([]Failure, error) {
	return runFreshness(repoRoot, false)
}

func runFreshness(repoRoot string, sourceOnly bool) ([]Failure, error) {
	// The bin-launchers checks read only files, so they run first and their
	// findings survive a source digest that cannot be computed.
	failures := checkBinLaunchers(repoRoot, sourceOnly)
	// Two directories, two audiences: the Go module that BUILDS the binary is
	// maintainer-facing and lives at the repo root, while the binaries it
	// produces are part of the plugin a consumer installs and stay in hooks/.
	hooksDir := filepath.Join(repoRoot, engineRel)
	binDir := filepath.Join(repoRoot, "hooks", "bin")

	// The manifest is the anchor whether or not binaries are committed:
	// run-hook.cmd verifies every download against it.
	manifestFailures, manifest := manifestFindings(filepath.Join(repoRoot, "hooks", "SHA256SUMS"))
	failures = append(failures, manifestFailures...)

	if sourceOnly {
		return failures, nil
	}

	// The digest is computed once, on the first committed binary, so a
	// checkout with none never needs a readable module.
	var digest, pin string
	provenance := func() error {
		if digest != "" {
			return nil
		}
		d, err := hookprovenance.SourceDigest(hooksDir)
		if err != nil {
			return fmt.Errorf("freshness: could not compute the source digest: %w", err)
		}
		p, err := hookprovenance.PinnedToolchain(hooksDir)
		if err != nil {
			return fmt.Errorf("freshness: %w", err)
		}
		digest, pin = d, p
		return nil
	}
	for _, name := range hookbuild.ContractBinaries {
		rel := "hooks/bin/" + name
		path := filepath.Join(binDir, name)
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			// Not committed: the dispatcher fetches it and checks it against
			// the manifest above.
			continue
		}
		if err != nil {
			failures = append(failures, Failure{Rule: "freshness-manifest", Path: rel, Message: fmt.Sprintf("unreadable: %v", err)})
			continue
		}
		if err := provenance(); err != nil {
			return failures, err
		}
		failures = append(failures, checkSourceStamp(rel, data, digest)...)
		failures = append(failures, checkToolchainStamp(rel, path, pin)...)
		// A missing entry is checkManifestRoster's finding; report it once.
		if _, ok := manifest.Hash("bin/" + name); ok {
			failures = append(failures, checkManifestEntry(name, data, manifest)...)
		}
	}

	// The launcher gets the source and toolchain verdicts but no manifest
	// verdict: nothing downloads it, so hooks/SHA256SUMS does not list it. A
	// missing or unreadable launcher is bin-launchers' finding.
	launcherPath := filepath.Join(repoRoot, filepath.FromSlash(hookbuild.LauncherOutput))
	if data, err := os.ReadFile(launcherPath); err == nil {
		if err := provenance(); err != nil {
			return failures, err
		}
		failures = append(failures, checkSourceStamp(hookbuild.LauncherOutput, data, digest)...)
		failures = append(failures, checkToolchainStamp(hookbuild.LauncherOutput, launcherPath, pin)...)
	}
	return failures, nil
}

// manifestFindings reads the manifest once and returns its findings with the
// Manifest the per-binary hash checks use. A manifest that cannot be read, or
// that vouches for nothing, yields the single failure and the zero Manifest,
// so no binary is checked against hashes the dispatcher would not trust.
// Otherwise the roster findings come first, then one per faulted line in line
// order, as the legacy reading reports them.
func manifestFindings(path string) ([]Failure, hookprovenance.Manifest) {
	manifest, err := hookprovenance.LoadManifest(path)
	if err == nil {
		err = manifest.Unusable()
	}
	if err != nil {
		return []Failure{{Rule: "freshness-manifest", Path: manifestRel,
			Message: fmt.Sprintf("%v — run-hook.cmd cannot verify a downloaded binary without it (make -C hooks/go build-all)", err)}}, hookprovenance.Manifest{}
	}
	entries := make(map[string]string, len(manifest.Paths()))
	for _, p := range manifest.Paths() {
		entries[p], _ = manifest.Hash(p)
	}
	failures := checkManifestRoster(entries)
	for _, fault := range manifest.Faults() {
		var message string
		switch fault.Kind {
		case hookprovenance.FaultBlank, hookprovenance.FaultFieldCount:
			message = fmt.Sprintf("malformed line %d — each line is \"<sha256>  bin/<name>\" (make -C hooks/go build-all)", fault.Line)
		case hookprovenance.FaultHashNotHex, hookprovenance.FaultHashNotLower:
			message = fmt.Sprintf("bad hash on line %d — each hash is 64 lowercase hex characters (make -C hooks/go build-all)", fault.Line)
		case hookprovenance.FaultDuplicate:
			message = fmt.Sprintf("duplicate entry %s — run-hook.cmd reads only the first (make -C hooks/go build-all)", fault.Path)
		default:
			continue
		}
		failures = append(failures, Failure{Rule: "freshness-manifest", Path: manifestRel, Message: message})
	}
	return failures, manifest
}

// checkManifestRoster requires exactly the six contract entries: a missing
// one leaves run-hook.cmd unable to verify that platform's download, and an
// extra one is a hand edit build-all never writes.
func checkManifestRoster(manifest map[string]string) []Failure {
	var failures []Failure
	contract := make(map[string]struct{}, len(hookbuild.ContractBinaries))
	for _, name := range hookbuild.ContractBinaries {
		contract["bin/"+name] = struct{}{}
		if _, ok := manifest["bin/"+name]; !ok {
			failures = append(failures, Failure{
				Rule: "freshness-manifest", Path: manifestRel,
				Message: fmt.Sprintf("no entry for bin/%s — run-hook.cmd cannot verify a downloaded %s (make -C hooks/go build-all)", name, name),
			})
		}
	}
	var extra []string
	for key := range manifest {
		if _, ok := contract[key]; !ok {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		failures = append(failures, Failure{
			Rule: "freshness-manifest", Path: manifestRel,
			Message: fmt.Sprintf("unexpected entry %s — the manifest lists only the six contract binaries (make -C hooks/go build-all)", key),
		})
	}
	return failures
}

// manifestRel is the repo-relative checksum manifest run-hook.cmd reads. It
// sits beside the dispatcher rather than in bin/ so a checkout that ships no
// binaries still carries it.
const manifestRel = "hooks/SHA256SUMS"

// checkManifestEntry compares a committed binary's SHA-256 with the manifest
// entry the dispatcher would check a download against.
func checkManifestEntry(name string, data []byte, manifest hookprovenance.Manifest) []Failure {
	key := "bin/" + name
	want, ok := manifest.Hash(key)
	if !ok {
		return []Failure{{
			Rule: "freshness-manifest", Path: manifestRel,
			Message: fmt.Sprintf("no entry for %s — run-hook.cmd would refuse to download it (make -C hooks/go build-all)", key),
		}}
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		return []Failure{{
			Rule: "freshness-manifest", Path: manifestRel,
			Message: fmt.Sprintf("STALE: records %s for %s, but the committed binary hashes to %s — run-hook.cmd would discard the release's copy (make -C hooks/go build-all)", want, key, got),
		}}
	}
	return nil
}

// checkSourceStamp scans a committed binary's BYTES for the
// "agentics-src-<digest>" stamp the Makefile links in. Reading rather than
// executing is what lets the guard verify the three platforms this host
// cannot run.
func checkSourceStamp(rel string, data []byte, digest string) []Failure {
	embedded := digestStampPattern.Find(data)
	if embedded == nil {
		return []Failure{{
			Rule: "freshness-no-digest", Path: rel,
			Message: "embeds no source digest — it predates digest stamping; rebuild and commit the rebuilt binaries and manifest (make -C hooks/go build-all)",
		}}
	}
	if string(embedded) != "agentics-src-"+digest {
		return []Failure{{
			Rule: "freshness-stale-digest", Path: rel,
			Message: fmt.Sprintf("STALE: embedded source digest %s does not match the checkout's %s — rebuild and commit the rebuilt binaries and manifest (make -C hooks/go build-all)", strings.TrimPrefix(string(embedded), "agentics-src-"), digest),
		}}
	}
	return nil
}

// checkToolchainStamp compares the Go version the LINKER recorded against the
// go.mod pin. The recorded version is read from the binary's build info, not
// from an -X stamp: the linker writes it itself, for every GOOS/GOARCH, and a
// build flag cannot forge it.
func checkToolchainStamp(rel, path, pin string) []Failure {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return []Failure{{
			Rule: "freshness-no-toolchain", Path: rel,
			Message: fmt.Sprintf("carries no Go build information (%v) — `go build` writes it into every artifact, so this file did not come from one; rebuild and commit the rebuilt binaries and manifest (make -C hooks/go build-all)", err),
		}}
	}
	if hookprovenance.ToolchainSatisfiesPin(info.GoVersion, pin) {
		return nil
	}
	return []Failure{{
		Rule: "freshness-toolchain-drift", Path: rel,
		Message: fmt.Sprintf("built by %s, but hooks/go/go.mod pins %s — identical source built by a different toolchain carries the SAME source digest, so this is the only signal of that drift; rebuild with the pinned toolchain and commit the rebuilt binaries and manifest (make -C hooks/go clean build-all)", info.GoVersion, pin),
	}}
}

// copyTree copies a file or directory tree preserving file modes.
func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dst, info.Mode())
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, mode.Perm())
}
