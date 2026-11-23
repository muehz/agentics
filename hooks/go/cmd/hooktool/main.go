// Command hooktool builds and verifies hook binaries from an explicit source tree.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/muehz/agentics/hooks/go/internal/conformance"
	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "stage-dev" {
		return stageDev(args[1:])
	}
	if len(args) > 0 && args[0] == "audit" {
		flags := flag.NewFlagSet("audit", flag.ContinueOnError)
		repo := flags.String("repo", "", "absolute repository root")
		sourceOnly := flags.Bool("source-only", true, "audit a Git checkout that distributes source and builds native artifacts locally")
		skipTests := flags.Bool("skip-tests", false, "skip the module-test and module-coverage-floor gates; every other gate still runs")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || !filepath.IsAbs(*repo) {
			return errors.New("repo must be an absolute path")
		}
		result, auditErr := conformance.Audit(*repo, conformance.AuditOptions{SkipTests: *skipTests, SourceOnly: *sourceOnly})
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			return err
		}
		return auditErr
	}
	if len(args) == 0 || args[0] != "build" {
		return errors.New("expected build command")
	}
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	source := flags.String("source", "", "absolute module root")
	manifestPath := flags.String("manifest", "", "absolute checksum manifest")
	out := flags.String("out", "", "absolute output directory (caller must ensure exclusive ownership during publication and rollback)")
	launcher := flags.String("launcher", "", "absolute path to a committed bin/agx.exe; when set, the launcher is rebuilt, compared with it, and published as <out>/bin/agx.exe")
	buildLauncher := flags.Bool("build-launcher", false, "build and publish <out>/bin/agx.exe without a reference")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*source) || !filepath.IsAbs(*manifestPath) || !filepath.IsAbs(*out) {
		return errors.New("source, manifest and out must be absolute paths")
	}
	if *buildLauncher && *launcher != "" {
		return errors.New("--build-launcher and --launcher are mutually exclusive")
	}
	if *launcher != "" && !filepath.IsAbs(*launcher) {
		return errors.New("source, manifest, out and launcher must be absolute paths")
	}
	withLauncher := *buildLauncher || *launcher != ""
	publication, err := hookbuild.OpenPublication(*out, withLauncher)
	if err != nil {
		return err
	}
	pin, err := hookprovenance.PinnedToolchain(*source)
	if err != nil {
		return err
	}
	manifest, err := manifestPosture(*manifestPath)
	if err != nil {
		return err
	}
	scratch, err := publication.Scratch()
	if err != nil {
		return err
	}
	defer publication.Close()
	// Digest stays empty so that a source-digest failure keeps its
	// "hookbuild: <cause>" text.
	result, err := hookbuild.Rebuild(hookbuild.Request{
		SourceDir: *source,
		GoBin:     "go",
		OutDir:    scratch,
		Names:     hookbuild.ContractBinaries,
		Launcher:  withLauncher,
	})
	if err != nil {
		return err
	}
	if err := result.Verify(pin, manifest); err != nil {
		return err
	}
	if *launcher != "" {
		if err := sameBytes(result.Launcher.Path, *launcher); err != nil {
			return fmt.Errorf("%s built from %s does not match %s: %w", hookbuild.LauncherOutput, *source, *launcher, err)
		}
	}
	// The caller owns the output directory exclusively while Commit publishes
	// and, on a failure, rolls back.
	return publication.Commit(result)
}

// manifestPosture vets the checksum manifest before a build. A carriage return,
// a line that is not two fields, and a hash that is not 64 hex digits refuse
// the build in line order; blank lines, uppercase hashes and duplicate paths
// are tolerated.
func manifestPosture(path string) (hookprovenance.Manifest, error) {
	manifest, err := hookprovenance.LoadManifest(path)
	if err != nil {
		return hookprovenance.Manifest{}, err
	}
	for _, fault := range manifest.Faults() {
		switch fault.Kind {
		case hookprovenance.FaultCarriageReturn:
			return hookprovenance.Manifest{}, fmt.Errorf("the checksum manifest line %d carries a carriage return", fault.Line)
		case hookprovenance.FaultFieldCount, hookprovenance.FaultHashNotHex:
			return hookprovenance.Manifest{}, fmt.Errorf("malformed checksum manifest line %d", fault.Line)
		}
	}
	if err := manifest.Unusable(); err != nil {
		return hookprovenance.Manifest{}, err
	}
	for _, name := range hookbuild.ContractBinaries {
		if _, ok := manifest.Hash("bin/" + name); !ok {
			return hookprovenance.Manifest{}, fmt.Errorf("hookbuild: hooks/SHA256SUMS has no entry for bin/%s", name)
		}
	}
	return manifest, nil
}

// sameBytes reports an error unless the files at built and committed hold
// identical bytes.
func sameBytes(built, committed string) error {
	want, err := os.ReadFile(committed)
	if err != nil {
		return err
	}
	have, err := os.ReadFile(built)
	if err != nil {
		return err
	}
	if !bytes.Equal(have, want) {
		return errors.New("the bytes differ")
	}
	return nil
}
