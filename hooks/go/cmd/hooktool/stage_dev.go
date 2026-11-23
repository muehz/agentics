package main

import (
	"debug/buildinfo"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
)

// stageDev creates an install overlay in a new, private directory outside the
// checkout. Its manifest describes unpublished source and is never a release input.
func stageDev(args []string) (err error) {
	flags := flag.NewFlagSet("stage-dev", flag.ContinueOnError)
	repo := flags.String("repo", "", "absolute local checkout")
	out := flags.String("out", "", "new directory under a privately owned output parent")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !filepath.IsAbs(*repo) || !filepath.IsAbs(*out) {
		return errors.New("stage-dev requires absolute repo and out paths")
	}
	root, err := filepath.EvalSymlinks(*repo)
	if err != nil {
		return err
	}
	checkout, err := os.Stat(root)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(*out))
	if err != nil {
		return err
	}
	dest := filepath.Join(parent, filepath.Base(*out))
	// EvalSymlinks can preserve different spellings of the same directory on
	// a case-insensitive volume. Compare the existing parents by identity.
	for dir := dest; ; dir = filepath.Dir(dir) {
		info, err := os.Stat(dir)
		if err != nil && !(dir == dest && os.IsNotExist(err)) {
			return err
		}
		if err == nil && os.SameFile(checkout, info) {
			return errors.New("stage-dev output must be outside the checkout")
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	source := filepath.Join(root, "hooks", "go")
	pin, err := hookprovenance.PinnedToolchain(source)
	if err != nil {
		return err
	}
	if runtime.Version() != pin {
		return fmt.Errorf("stage-dev requires local %s; hooktool was built with %s", pin, runtime.Version())
	}
	if err := os.Mkdir(dest, 0700); err != nil {
		return fmt.Errorf("creating new stage-dev output: %w", err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(dest))
		}
	}()
	// A standalone invocation has the same offline contract as plugin:setup.
	for key, value := range map[string]string{
		"GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off", "GOVCS": "*:off",
		"GONOPROXY": "none", "GOWORK": "off", "GOENV": "off", "GOFLAGS": "-mod=readonly",
	} {
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	bin := filepath.Join(dest, "hooks", "bin")
	binaries, err := hookbuild.Rebuild(hookbuild.Request{SourceDir: source, GoBin: "go", OutDir: bin, Names: hookbuild.ContractBinaries})
	if err != nil {
		return err
	}
	var manifest strings.Builder
	for _, target := range binaries.Targets {
		if target.Err != nil {
			return target.Err
		}
		if err := checkDevBinary(target.Path, hookbuild.ContractTargets[target.Name], pin); err != nil {
			return err
		}
		digest, err := hookbuild.FileSHA256(target.Path)
		if err != nil {
			return err
		}
		fmt.Fprintf(&manifest, "%s  bin/%s\n", digest, target.Name)
	}
	launcherDir := filepath.Dir(filepath.Join(dest, filepath.FromSlash(hookbuild.LauncherOutput)))
	result, err := hookbuild.Rebuild(hookbuild.Request{SourceDir: source, GoBin: "go", OutDir: launcherDir, Launcher: true})
	if err != nil {
		return err
	}
	if result.Launcher.Err != nil {
		return result.Launcher.Err
	}
	if err := checkDevBinary(result.Launcher.Path, hookbuild.LauncherTarget, pin); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, "hooks", "SHA256SUMS"), []byte(manifest.String()), 0644)
}

func checkDevBinary(file string, target [2]string, pin string) error {
	info, err := buildinfo.ReadFile(file)
	if err != nil {
		return err
	}
	var goos, arch string
	for _, setting := range info.Settings {
		switch setting.Key {
		case "GOOS":
			goos = setting.Value
		case "GOARCH":
			arch = setting.Value
		}
	}
	if info.GoVersion != pin || goos != target[0] || arch != target[1] {
		return fmt.Errorf("%s was built for %s/%s with %s; expected %s/%s with %s", file, goos, arch, info.GoVersion, target[0], target[1], pin)
	}
	switch goos {
	case "darwin":
		f, err := macho.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		if f.Type == macho.TypeExec && (arch == "amd64" && f.Cpu == macho.CpuAmd64 || arch == "arm64" && f.Cpu == macho.CpuArm64) {
			return nil
		}
	case "linux":
		f, err := elf.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		if f.Type == elf.ET_EXEC && (arch == "amd64" && f.Machine == elf.EM_X86_64 || arch == "arm64" && f.Machine == elf.EM_AARCH64) {
			return nil
		}
	case "windows":
		f, err := pe.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		if f.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE != 0 && (arch == "amd64" && f.Machine == pe.IMAGE_FILE_MACHINE_AMD64 || arch == "arm64" && f.Machine == pe.IMAGE_FILE_MACHINE_ARM64) {
			return nil
		}
	}
	return fmt.Errorf("%s executable header does not match %s/%s", file, goos, arch)
}
