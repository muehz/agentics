package hookbuild_test

import (
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
)

// writeModule writes files, keyed by slash path, under a new module root.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "source")
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const (
	fixtureMod      = "module example.com/fixture\n\ngo 1.27.4\n"
	fixtureHook     = "package main\nfunc main() {}\n"
	fixtureLauncher = "package main\nvar sourceDigest = \"unset\"\nfunc main() { println(sourceDigest) }\n"
)

// TestRebuildAttemptsEveryTarget proves one failing platform does not stop
// the others: both windows targets fail, every other target and the launcher
// still leave a binary, and Verify reports the first failure in contract
// order.
func TestRebuildAttemptsEveryTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping a real go build in -short mode")
	}
	source := writeModule(t, map[string]string{
		"go.mod":                   fixtureMod,
		"cmd/hook/main.go":         fixtureHook,
		"cmd/hook/main_windows.go": "package main\nvar broken int = \"not an int\"\n",
		"cmd/launcher/main.go":     fixtureLauncher,
	})
	outDir := filepath.Join(t.TempDir(), "out")
	res, err := hookbuild.Rebuild(hookbuild.Request{
		SourceDir: source, GoBin: "go", OutDir: outDir,
		Names: hookbuild.ContractBinaries, Launcher: true,
	})
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if len(res.Targets) != len(hookbuild.ContractBinaries) {
		t.Fatalf("Rebuild returned %d targets, want %d", len(res.Targets), len(hookbuild.ContractBinaries))
	}
	for i, target := range res.Targets {
		name := hookbuild.ContractBinaries[i]
		if target.Name != name || target.Path != filepath.Join(outDir, name) {
			t.Errorf("target %d = %q at %q, want %q at %q", i, target.Name, target.Path, name, filepath.Join(outDir, name))
		}
		goos := hookbuild.ContractTargets[name]
		if goos[0] == "windows" {
			prefix := fmt.Sprintf("hookbuild: go build windows/%s: ", goos[1])
			if target.Err == nil || !strings.HasPrefix(target.Err.Error(), prefix) {
				t.Errorf("%s: Err = %v, want a failure starting %q", name, target.Err, prefix)
				continue
			}
			if target.Output == "" || !strings.HasSuffix(target.Err.Error(), "\n"+target.Output) {
				t.Errorf("%s: Err does not end with the raw output %q", name, target.Output)
			}
			continue
		}
		if target.Err != nil {
			t.Errorf("%s: Err = %v, want none", name, target.Err)
		}
		if _, err := os.Stat(target.Path); err != nil {
			t.Errorf("%s: no binary at Path: %v", name, err)
		}
	}
	if res.Launcher == nil {
		t.Fatal("Launcher is nil although the request set Launcher")
	}
	if res.Launcher.Name != "agx.exe" || res.Launcher.Path != filepath.Join(outDir, "agx.exe") {
		t.Errorf("launcher = %q at %q, want agx.exe at %q", res.Launcher.Name, res.Launcher.Path, filepath.Join(outDir, "agx.exe"))
	}
	if res.Launcher.Err != nil {
		t.Errorf("launcher Err = %v, want none", res.Launcher.Err)
	}
	if _, err := os.Stat(res.Launcher.Path); err != nil {
		t.Errorf("no launcher at Path: %v", err)
	}
	if err := res.Verify(runtime.Version(), hookprovenance.Manifest{}); err != res.Targets[4].Err {
		t.Errorf("Verify = %v, want the windows/amd64 failure %v", err, res.Targets[4].Err)
	}
}

// TestRebuildRefusesAnUnknownName pins that a name outside the contract stops
// the rebuild before anything runs or is created.
func TestRebuildRefusesAnUnknownName(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "out")
	res, err := hookbuild.Rebuild(hookbuild.Request{
		SourceDir: t.TempDir(), GoBin: filepath.Join(t.TempDir(), "no-such-go"), OutDir: outDir,
		Names: []string{"agx-linux-amd64", "nope"},
	})
	if err == nil || err.Error() != "hookbuild: nope is not a contract binary" {
		t.Fatalf("Rebuild error = %v, want the unknown-name refusal", err)
	}
	if res.Targets != nil || res.Launcher != nil {
		t.Errorf("Rebuild returned %+v with its error, want an empty Result", res)
	}
	if _, err := os.Stat(outDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("OutDir exists after a refused rebuild: %v", err)
	}
}

// rebuild runs Rebuild and fails the test on its error and on any target or
// launcher that failed to build. Rebuild reports a failed target in the Result,
// and each target's error already carries the go command's output.
func rebuild(t *testing.T, req hookbuild.Request) hookbuild.Result {
	t.Helper()
	res, err := hookbuild.Rebuild(req)
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	for _, target := range res.Targets {
		if target.Err != nil {
			t.Fatalf("Rebuild %s: %v", target.Name, target.Err)
		}
	}
	if res.Launcher != nil && res.Launcher.Err != nil {
		t.Fatalf("Rebuild launcher: %v", res.Launcher.Err)
	}
	return res
}

// copyFile copies src to dst.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// loadManifest writes lines as a checksum manifest and reads it back.
func loadManifest(t *testing.T, lines ...string) hookprovenance.Manifest {
	t.Helper()
	path := filepath.Join(t.TempDir(), "SHA256SUMS")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := hookprovenance.LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestVerifyOrder has one row per Verify error, in the order Verify checks.
// Rows past the build-information read copy this test binary into the target
// paths, because a file the test writes carries no build information.
func TestVerifyOrder(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exeHash, err := hookbuild.FileSHA256(exe)
	if err != nil {
		t.Fatal(err)
	}
	pin := runtime.Version()
	first, second := hookbuild.ContractBinaries[0], hookbuild.ContractBinaries[1]
	zeros := strings.Repeat("0", 64)
	entry := func(hash, name string) string { return hash + "  bin/" + name + "\n" }

	// built returns two targets whose files are copies of this test binary.
	built := func(t *testing.T) []hookbuild.Target {
		dir := t.TempDir()
		targets := []hookbuild.Target{
			{Name: first, Path: filepath.Join(dir, first)},
			{Name: second, Path: filepath.Join(dir, second)},
		}
		for _, target := range targets {
			copyFile(t, exe, target.Path)
		}
		return targets
	}
	targetErr := errors.New("hookbuild: go build darwin/arm64: exit status 1\nboom")
	otherErr := errors.New("hookbuild: go build windows/amd64: exit status 1\nbang")
	launcherErr := errors.New("hookbuild: go build windows/amd64: exit status 1\nlauncher")

	tests := []struct {
		name string
		// setup returns the Result, pin, manifest, and the exact error Verify
		// must return: wantErr by value when non-nil, else wantText.
		setup func(t *testing.T) (res hookbuild.Result, pin string, m hookprovenance.Manifest, wantErr error, wantText string)
	}{
		{"failed contract target", func(t *testing.T) (hookbuild.Result, string, hookprovenance.Manifest, error, string) {
			res := hookbuild.Result{
				Targets:  []hookbuild.Target{{Name: first}, {Name: second, Err: targetErr}, {Name: "agx-windows-amd64.exe", Err: otherErr}},
				Launcher: &hookbuild.Target{Name: "agx.exe", Err: launcherErr},
			}
			return res, pin, hookprovenance.Manifest{}, targetErr, ""
		}},
		{"failed launcher", func(t *testing.T) (hookbuild.Result, string, hookprovenance.Manifest, error, string) {
			res := hookbuild.Result{
				Targets:  []hookbuild.Target{{Name: first, Path: filepath.Join(t.TempDir(), first)}},
				Launcher: &hookbuild.Target{Name: "agx.exe", Err: launcherErr},
			}
			return res, pin, hookprovenance.Manifest{}, launcherErr, ""
		}},
		{"no targets", func(t *testing.T) (hookbuild.Result, string, hookprovenance.Manifest, error, string) {
			return hookbuild.Result{}, pin, hookprovenance.Manifest{}, nil, "hookbuild: nothing was rebuilt"
		}},
		{"build information unreadable", func(t *testing.T) (hookbuild.Result, string, hookprovenance.Manifest, error, string) {
			path := filepath.Join(t.TempDir(), first)
			if err := os.WriteFile(path, []byte("not a binary"), 0o644); err != nil {
				t.Fatal(err)
			}
			_, readErr := buildinfo.ReadFile(path)
			if readErr == nil {
				t.Fatal("buildinfo.ReadFile read a plain file")
			}
			res := hookbuild.Result{Targets: []hookbuild.Target{{Name: first, Path: path}}}
			return res, pin, hookprovenance.Manifest{}, nil, readErr.Error()
		}},
		{"toolchain off the pin", func(t *testing.T) (hookbuild.Result, string, hookprovenance.Manifest, error, string) {
			res := hookbuild.Result{Targets: built(t)}
			m := loadManifest(t, entry(exeHash, first), entry(exeHash, second))
			return res, "go1.0", m, nil, fmt.Sprintf("the hook binaries were built with %s, but go.mod pins go1.0; build with that toolchain", runtime.Version())
		}},
		{"no manifest entry", func(t *testing.T) (hookbuild.Result, string, hookprovenance.Manifest, error, string) {
			res := hookbuild.Result{Targets: built(t)}
			m := loadManifest(t, entry(exeHash, first))
			return res, pin, m, nil, "hookbuild: hooks/SHA256SUMS has no entry for bin/" + second
		}},
		{"file unreadable", func(t *testing.T) (hookbuild.Result, string, hookprovenance.Manifest, error, string) {
			targets := built(t)
			if err := os.Remove(targets[1].Path); err != nil {
				t.Fatal(err)
			}
			_, hashErr := hookbuild.FileSHA256(targets[1].Path)
			if hashErr == nil {
				t.Fatal("FileSHA256 hashed a removed file")
			}
			m := loadManifest(t, entry(exeHash, first), entry(exeHash, second))
			return hookbuild.Result{Targets: targets}, pin, m, nil, hashErr.Error()
		}},
		{"hash mismatch", func(t *testing.T) (hookbuild.Result, string, hookprovenance.Manifest, error, string) {
			m := loadManifest(t, entry(exeHash, first), entry(zeros, second))
			return hookbuild.Result{Targets: built(t)}, pin, m, nil,
				fmt.Sprintf("hookbuild: %s built as sha256 %s, but hooks/SHA256SUMS records %s", second, exeHash, zeros)
		}},
		{"verified", func(t *testing.T) (hookbuild.Result, string, hookprovenance.Manifest, error, string) {
			m := loadManifest(t, entry(exeHash, first), entry(exeHash, second))
			return hookbuild.Result{Targets: built(t)}, pin, m, nil, ""
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, pin, m, wantErr, wantText := tc.setup(t)
			err := res.Verify(pin, m)
			switch {
			case wantErr != nil:
				if err != wantErr {
					t.Errorf("Verify = %v, want the value %v", err, wantErr)
				}
			case wantText != "":
				if err == nil || err.Error() != wantText {
					t.Errorf("Verify = %v, want %q", err, wantText)
				}
			default:
				if err != nil {
					t.Errorf("Verify = %v, want nil", err)
				}
			}
		})
	}
}

// TestManifestRoster pins the roster reading: missing contract paths in
// contract order, extra paths sorted, and nil for an empty list.
func TestManifestRoster(t *testing.T) {
	hash := strings.Repeat("a", 64)
	line := func(path string) string { return hash + "  " + path + "\n" }
	contract := func(skip ...string) []string {
		var lines []string
		for _, name := range hookbuild.ContractBinaries {
			if !slices.Contains(skip, name) {
				lines = append(lines, line("bin/"+name))
			}
		}
		return lines
	}
	tests := []struct {
		name               string
		lines              []string
		missing, extraWant []string
	}{
		{"complete", contract(), nil, nil},
		{"missing", contract("agx-windows-arm64.exe", "agx-darwin-amd64"), []string{"bin/agx-darwin-amd64", "bin/agx-windows-arm64.exe"}, nil},
		{"extra", append(contract(), line("bin/zz"), line("bin/agx.exe")), nil, []string{"bin/agx.exe", "bin/zz"}},
		{"both", append(contract("agx-linux-arm64"), line("bin/zz"), line("README")), []string{"bin/agx-linux-arm64"}, []string{"README", "bin/zz"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			missing, extra := hookbuild.ManifestRoster(loadManifest(t, tc.lines...))
			if !slices.Equal(missing, tc.missing) || (missing == nil) != (tc.missing == nil) {
				t.Errorf("missing = %#v, want %#v", missing, tc.missing)
			}
			if !slices.Equal(extra, tc.extraWant) || (extra == nil) != (tc.extraWant == nil) {
				t.Errorf("extra = %#v, want %#v", extra, tc.extraWant)
			}
		})
	}
}
