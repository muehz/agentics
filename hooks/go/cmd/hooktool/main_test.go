package main

import (
	"fmt"
	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
	"github.com/muehz/agentics/hooks/go/internal/hookprovenance"
	"github.com/muehz/agentics/hooks/go/internal/hookutil/hookutiltest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHookTool(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "cmd", "hook"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"go.mod": "module example.com/fixture\n\ngo 1.27.4\n", "cmd/hook/main.go": "package main\nfunc main() {}\n"} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(name)), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := filepath.Join(root, "SHA256SUMS")
	reference := filepath.Join(root, "reference")
	rebuild(t, source, reference, hookbuild.ContractBinaries, false)
	var lines []string
	for _, name := range hookbuild.ContractBinaries {
		h, err := hookbuild.FileSHA256(filepath.Join(reference, name))
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, fmt.Sprintf("%s  bin/%s\n", h, name))
	}
	binary := filepath.Join(root, "hooktool")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build command: %v: %s", err, out)
	}
	var upperLines []string
	for _, line := range lines {
		upperLines = append(upperLines, strings.ToUpper(line[:64])+line[64:])
	}
	cases := []struct{ name, body, mod, want string }{
		{"uppercase-hash", strings.Join(upperLines, ""), "", ""},
		{"blank-interior-line", lines[0] + "\n" + strings.Join(lines[1:], ""), "", ""},
		{"malformed-before-no-entries", "broken\n", "", "malformed checksum manifest line 1"},
		{"malformed-before-carriage-return", "broken\n" + strings.Replace(lines[0], "\n", "\r\n", 1) + strings.Join(lines[1:], ""), "", "malformed checksum manifest line 1"},
		{"mismatch", strings.Repeat("0", 64) + lines[0][64:] + strings.Join(lines[1:], ""), "", "built as sha256"},
		{"matching", strings.Join(lines, ""), "", ""},
		{"missing-target", strings.Join(lines[1:], ""), "", "has no entry for bin/agx-darwin-amd64"},
		{"malformed-manifest", strings.Replace(lines[0], "\n", "\r\n", 1) + strings.Join(lines[1:], ""), "", "carries a carriage return"},
		{"malformed-one-field", strings.Join(lines, "") + "broken\n", "", "malformed checksum manifest line 7"},
		{"malformed-extra-field", strings.Join(lines, "") + lines[0][:len(lines[0])-1] + " extra\n", "", "malformed checksum manifest line 7"},
		{"malformed-hash", strings.Join(lines, "") + strings.Repeat("z", 64) + "  bin/other\n", "", "malformed checksum manifest line 7"},
		{"duplicate-first-matches", strings.Join(lines, "") + strings.Repeat("0", 64) + lines[0][64:], "", ""},
		{"duplicate-first-mismatches", strings.Repeat("0", 64) + lines[0][64:] + strings.Join(lines, ""), "", "built as sha256"},
		{"bad-pin", strings.Join(lines, ""), "module example.com/fixture\n\ngo 1.27.2\n", "but go.mod pins go1.27.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mod := "module example.com/fixture\n\ngo 1.27.4\n"
			if tc.mod != "" {
				mod = tc.mod
			}
			if err := os.WriteFile(filepath.Join(source, "go.mod"), []byte(mod), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifest, []byte(tc.body), 0644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "build", "--source", source, "--manifest", manifest, "--out", filepath.Join(root, "out-"+tc.name))
			out, err := cmd.CombinedOutput()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("matching build: %v: %s", err, out)
				}
				for _, name := range hookbuild.ContractBinaries {
					if _, err := os.Stat(filepath.Join(root, "out-"+tc.name, name)); err != nil {
						t.Fatal(err)
					}
				}
			} else if err == nil || !strings.Contains(string(out), tc.want) {
				t.Fatalf("want rejection %q, err=%v output=%s", tc.want, err, out)
			}
		})
	}
	for _, collision := range []bool{false, true} {
		name := "populated-output"
		if collision {
			name = "output-collision"
		}
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(source, "go.mod"), []byte("module example.com/fixture\n\ngo 1.27.4\n"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifest, []byte(strings.Join(lines, "")), 0644); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(root, name)
			if err := os.MkdirAll(dest, 0755); err != nil {
				t.Fatal(err)
			}
			keep := filepath.Join(dest, "bundle.tar.gz")
			if err := os.WriteFile(keep, []byte("keep"), 0644); err != nil {
				t.Fatal(err)
			}
			if collision {
				if err := os.WriteFile(filepath.Join(dest, hookbuild.ContractBinaries[0]), []byte("original"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			out, err := exec.Command(binary, "build", "--source", source, "--manifest", manifest, "--out", dest).CombinedOutput()
			if collision {
				if err == nil || !strings.Contains(string(out), "output already exists") {
					t.Fatalf("want collision rejection, err=%v output=%s", err, out)
				}
				data, e := os.ReadFile(filepath.Join(dest, hookbuild.ContractBinaries[0]))
				if e != nil || string(data) != "original" {
					t.Fatalf("collision changed: %q, %v", data, e)
				}
			} else {
				if err != nil {
					t.Fatalf("populated output build: %v: %s", err, out)
				}
				for _, target := range hookbuild.ContractBinaries {
					if _, e := os.Stat(filepath.Join(dest, target)); e != nil {
						t.Fatal(e)
					}
				}
			}
			data, e := os.ReadFile(keep)
			if e != nil || string(data) != "keep" {
				t.Fatalf("unrelated file changed: %q, %v", data, e)
			}
		})
	}
	t.Run("protected-output", func(t *testing.T) {
		protected := filepath.Join(root, "hooks", "bin")
		if err := os.MkdirAll(protected, 0755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "build", "--source", source, "--manifest", manifest, "--out", filepath.Join(protected, "nested"))
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "refusing to write hooks/bin") {
			t.Fatalf("protected output: err=%v output=%s", err, out)
		}
	})
}

// pinnedFixture is a module whose hashed set and digest are fixed below. The
// list carries a comment, a blank line and no trailing newline, and names a/b
// beside an unlisted a/bc, so the pins hold only while comments and blanks are
// skipped, the last line is still read, and a listed directory excludes no
// sibling. A test file and bin/ stay out of the set as well.
var pinnedFixture = map[string]string{
	"go.mod":          "module example.com/fixture\n\ngo 1.27.4\n",
	"main.go":         "package main\n\nfunc main() {}\n",
	"main_test.go":    "package main\n",
	"a/b/excluded.go": "package b\n",
	"a/bc/kept.go":    "package bc\n",
	"skip/gone.go":    "package skip\n",
	"bin/built.go":    "package bin\n",
	"digest-exclude":  "# directories outside every hook binary\n\na/b\nskip",
}

// A change to SourceFiles or SourceDigest that moves either pin also moves
// the digest every committed binary is stamped with, marking them all stale,
// so it must be deliberate and land with rebuilt binaries.
const (
	pinnedFixtureFiles  = "a/bc/kept.go digest-exclude go.mod main.go"
	pinnedFixtureDigest = "fdb7774ef38ba65ab2b433ab6cdefe971d784fe71f3f2371c1d6922583695b23"
	// pinnedEditedListDigest is the fixture's digest once digest-exclude
	// becomes "a/b\nskip\n": the same entries, so only the list's own bytes
	// differ.
	pinnedEditedListDigest = "2ec7ef68f12b1c6e62bd0b4fbb561f31d84d6411ac87c99c881681f674ceb56a"
)

// TestHookToolSourceDigest pins hookprovenance's hashed set and digest on a
// fixed module, folds CRLF without dropping a lone carriage return, and
// rejects every malformed digest-exclude line by number.
func TestHookToolSourceDigest(t *testing.T) {
	digest := func(dir string) string {
		t.Helper()
		d, err := hookprovenance.SourceDigest(dir)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}

	t.Run("pinned-fixture", func(t *testing.T) {
		dir := t.TempDir()
		writeFixture(t, dir, pinnedFixture)
		files, err := hookprovenance.SourceFiles(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(files, " "); got != pinnedFixtureFiles {
			t.Fatalf("SourceFiles = %q, want %s", got, pinnedFixtureFiles)
		}
		if got := digest(dir); got != pinnedFixtureDigest {
			t.Fatalf("SourceDigest = %s, want %s", got, pinnedFixtureDigest)
		}
		writeFixture(t, dir, map[string]string{"a/b/excluded.go": "package b\n// edited\n"})
		if got := digest(dir); got != pinnedFixtureDigest {
			t.Fatalf("an edit under a listed directory changed the digest: %s", got)
		}
		writeFixture(t, dir, map[string]string{"digest-exclude": "a/b\nskip\n"})
		if got := digest(dir); got != pinnedEditedListDigest {
			t.Fatalf("SourceDigest after a list edit = %s, want %s", got, pinnedEditedListDigest)
		}
	})

	t.Run("crlf-folding", func(t *testing.T) {
		dir := t.TempDir()
		writeFixture(t, dir, pinnedFixture)
		// digest-exclude stays LF: a carriage return there is a malformed
		// line, not a line ending to fold.
		for _, rel := range []string{"go.mod", "main.go", "a/bc/kept.go"} {
			writeFixture(t, dir, map[string]string{rel: strings.ReplaceAll(pinnedFixture[rel], "\n", "\r\n")})
		}
		if got := digest(dir); got != pinnedFixtureDigest {
			t.Fatalf("CRLF copy digests %s, want the LF digest %s", got, pinnedFixtureDigest)
		}
		writeFixture(t, dir, map[string]string{"main.go": pinnedFixture["main.go"] + "// standalone\rmarker\n"})
		if got := digest(dir); got == pinnedFixtureDigest {
			t.Fatal("a standalone carriage return must change the digest")
		}
	})

	t.Run("malformed-list", func(t *testing.T) {
		// Each case asserts the reason of the check it targets, because a
		// later check or the final directory stat would still reject most of
		// these lines with that check removed.
		const (
			carriage  = "carries a carriage return"
			space     = "contains whitespace"
			backslash = "contains a backslash"
			absolute  = "is absolute"
			dotdot    = "contains a .. element"
			unclean   = "is not a clean path"
			noDir     = "names no directory under the module root"
		)
		for name, tc := range map[string]struct {
			list   string
			line   int
			reason string
		}{
			"carriage-return":         {"a/b\r\n", 1, carriage},
			"carriage-return-comment": {"# note\r\na/b\n", 1, carriage},
			"space":                   {"a/b \n", 1, space},
			"tab":                     {"a\tb\n", 1, space},
			"backslash":               {`a\b` + "\n", 1, backslash},
			"drive":                   {"C:/a\n", 1, absolute},
			"absolute":                {"/a/b\n", 1, absolute},
			"dotdot":                  {"a/../a/b\n", 1, dotdot},
			"dot":                     {".\n", 1, unclean},
			"leading-dot":             {"./a/b\n", 1, unclean},
			"inner-dot":               {"a/./b\n", 1, unclean},
			"double-slash":            {"a//b\n", 1, unclean},
			"trailing-slash":          {"a/b/\n", 1, unclean},
			"nonexistent":             {"a/nope\n", 1, noDir},
			"file-not-dir":            {"a/b/x.go\n", 1, noDir},
			"bad-final-line":          {"a/b\na/nope", 2, noDir},
		} {
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				writeFixture(t, dir, map[string]string{
					"go.mod":         "module example.com/fixture\n\ngo 1.27.4\n",
					"a/b/x.go":       "package b\n",
					"digest-exclude": tc.list,
				})
				_, err := hookprovenance.ExcludeList(dir)
				if err == nil {
					t.Fatalf("ExcludeList accepts %q", tc.list)
				}
				if want := fmt.Sprintf("digest-exclude line %d ", tc.line); !strings.Contains(err.Error(), want) {
					t.Fatalf("ExcludeList error %q does not name line %d", err, tc.line)
				}
				if !strings.Contains(err.Error(), tc.reason) {
					t.Fatalf("ExcludeList error %q does not say %q", err, tc.reason)
				}
			})
		}
	})
}

// writeFixture writes each slash-relative path under dir with its body,
// creating parent directories.
func writeFixture(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		dest := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// rebuild builds names, and with launcher the agx launcher, from source into
// outDir, and fails the test on the error and on any target that failed.
// Rebuild reports a failed target in its Result, and each target's error
// carries the go command's output.
func rebuild(t *testing.T, source, outDir string, names []string, launcher bool) {
	t.Helper()
	res, err := hookbuild.Rebuild(hookbuild.Request{SourceDir: source, GoBin: "go", OutDir: outDir, Names: names, Launcher: launcher})
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	for _, target := range res.Targets {
		if target.Err != nil {
			t.Fatalf("rebuild %s: %v", target.Name, target.Err)
		}
	}
	if res.Launcher != nil && res.Launcher.Err != nil {
		t.Fatalf("rebuild launcher: %v", res.Launcher.Err)
	}
}

// launcherFixture writes a module that has both cmd/hook and cmd/launcher,
// builds its six contract binaries and its launcher as references, and returns
// the module root, a manifest of the six, and the reference launcher.
func launcherFixture(t *testing.T, root string) (source, manifest, launcher string) {
	t.Helper()
	source = filepath.Join(root, "launcher-source")
	for name, body := range map[string]string{
		"go.mod":               "module example.com/fixture\n\ngo 1.27.4\n",
		"cmd/hook/main.go":     "package main\nfunc main() {}\n",
		"cmd/launcher/main.go": "package main\nvar sourceDigest = \"unset\"\nfunc main() { println(sourceDigest) }\n",
	} {
		path := filepath.Join(source, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	reference := filepath.Join(root, "launcher-reference")
	rebuild(t, source, reference, hookbuild.ContractBinaries, false)
	launcher = filepath.Join(reference, "bin", "agx.exe")
	rebuild(t, source, filepath.Dir(launcher), nil, true)
	var lines []string
	for _, name := range hookbuild.ContractBinaries {
		h, err := hookbuild.FileSHA256(filepath.Join(reference, name))
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, fmt.Sprintf("%s  bin/%s\n", h, name))
	}
	manifest = filepath.Join(root, "launcher-SHA256SUMS")
	if err := os.WriteFile(manifest, []byte(strings.Join(lines, "")), 0644); err != nil {
		t.Fatal(err)
	}
	return source, manifest, launcher
}

// assertNoOutputs fails when any of the six contract binaries or the launcher
// exists under dest.
func assertNoOutputs(t *testing.T, dest string) {
	t.Helper()
	for _, name := range append(append([]string{}, hookbuild.ContractBinaries...), filepath.FromSlash(hookbuild.LauncherOutput)) {
		if _, err := os.Lstat(filepath.Join(dest, name)); !os.IsNotExist(err) {
			t.Errorf("output %s exists: %v", name, err)
		}
	}
}

func TestHookToolLauncherOutputParent(t *testing.T) {
	root := t.TempDir()
	for _, kind := range []string{"linked", "file"} {
		t.Run(kind, func(t *testing.T) {
			dest := t.TempDir()
			bin := filepath.Join(dest, "bin")
			target := t.TempDir()
			if kind == "file" {
				if err := os.WriteFile(bin, []byte("keep"), 0644); err != nil {
					t.Fatal(err)
				}
			} else if runtime.GOOS == "windows" {
				hookutiltest.JunctionOrSkip(t, target, bin)
			} else {
				hookutiltest.SymlinkOrSkip(t, target, bin)
			}
			for _, mode := range [][]string{{"--build-launcher"}, {"--launcher", filepath.Join(root, "reference")}} {
				// Output admission precedes the source build. No toolchain or built artifact is needed.
				args := []string{"build", "--source", root, "--manifest", filepath.Join(root, "manifest"), "--out", dest}
				if err := run(append(args, mode...)); err == nil || !strings.Contains(err.Error(), "launcher output parent must be a directory") {
					t.Errorf("%s: want launcher output parent refusal, got %v", mode[0], err)
				}
			}
			if entries, err := os.ReadDir(target); err != nil || len(entries) != 0 {
				t.Fatalf("unexpected contents in link target: %v, %v", entries, err)
			}
		})
	}
}

func TestHookToolLauncher(t *testing.T) {
	root := t.TempDir()
	source, manifest, reference := launcherFixture(t, root)
	want, err := os.ReadFile(reference)
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "hooktool")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build command: %v: %s", err, out)
	}
	build := func(dest string, extra ...string) ([]byte, error) {
		args := append([]string{"build", "--source", source, "--manifest", manifest, "--out", dest}, extra...)
		return exec.Command(binary, args...).CombinedOutput()
	}

	t.Run("matching-launcher", func(t *testing.T) {
		dest := filepath.Join(root, "out-matching")
		if err := os.MkdirAll(filepath.Join(dest, "bin"), 0755); err != nil {
			t.Fatal(err)
		}
		if out, err := build(dest, "--launcher", reference); err != nil {
			t.Fatalf("build: %v: %s", err, out)
		}
		got, err := os.ReadFile(filepath.Join(dest, "bin", "agx.exe"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Error("published bin/agx.exe differs from the reference launcher")
		}
		for _, name := range hookbuild.ContractBinaries {
			if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
				t.Error(err)
			}
		}
	})

	t.Run("source-only", func(t *testing.T) {
		dest := filepath.Join(root, "out-source-only")
		if out, err := build(dest, "--build-launcher"); err != nil {
			t.Fatalf("build without a reference: %v: %s", err, out)
		}
		got, err := os.ReadFile(filepath.Join(dest, "bin", "agx.exe"))
		if err != nil || string(got) != string(want) {
			t.Fatalf("published launcher differs from the independently built launcher: %v", err)
		}
		for _, name := range hookbuild.ContractBinaries {
			if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
				t.Error(err)
			}
		}
	})

	t.Run("conflicting-launcher-modes", func(t *testing.T) {
		dest := filepath.Join(root, "out-conflict")
		out, err := build(dest, "--build-launcher", "--launcher", reference)
		if err == nil || !strings.Contains(string(out), "mutually exclusive") {
			t.Fatalf("want mutually exclusive modes: %v: %s", err, out)
		}
		assertNoOutputs(t, dest)
	})

	t.Run("differing-launcher", func(t *testing.T) {
		other := filepath.Join(root, "other-agx.exe")
		if err := os.WriteFile(other, append([]byte("not "), want...), 0644); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(root, "out-differing")
		out, err := build(dest, "--launcher", other)
		if err == nil || !strings.Contains(string(out), "bin/agx.exe") || !strings.Contains(string(out), other) {
			t.Fatalf("want a launcher mismatch naming bin/agx.exe and %s, err=%v output=%s", other, err, out)
		}
		assertNoOutputs(t, dest)
	})

	t.Run("relative-launcher", func(t *testing.T) {
		out, err := build(filepath.Join(root, "out-relative"), "--launcher", filepath.Join("bin", "agx.exe"))
		if err == nil || !strings.Contains(string(out), "must be absolute paths") {
			t.Fatalf("want an absolute-path rejection, err=%v output=%s", err, out)
		}
	})

	t.Run("protected-launcher-output", func(t *testing.T) {
		dest := filepath.Join(root, "plugin", "hooks")
		if err := os.MkdirAll(dest, 0755); err != nil {
			t.Fatal(err)
		}
		out, err := build(dest, "--launcher", reference)
		if err == nil || !strings.Contains(string(out), "refusing to write hooks/bin") {
			t.Fatalf("want hooks/bin refusal, err=%v output=%s", err, out)
		}
		assertNoOutputs(t, dest)
	})

	t.Run("launcher-collision", func(t *testing.T) {
		dest := filepath.Join(root, "out-collision")
		existing := filepath.Join(dest, "bin", "agx.exe")
		if err := os.MkdirAll(filepath.Dir(existing), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(existing, []byte("original"), 0644); err != nil {
			t.Fatal(err)
		}
		out, err := build(dest, "--launcher", reference)
		if err == nil || !strings.Contains(string(out), "output already exists: bin/agx.exe") {
			t.Fatalf("want a launcher collision rejection, err=%v output=%s", err, out)
		}
		if data, e := os.ReadFile(existing); e != nil || string(data) != "original" {
			t.Fatalf("collision changed: %q, %v", data, e)
		}
		for _, name := range hookbuild.ContractBinaries {
			if _, err := os.Lstat(filepath.Join(dest, name)); !os.IsNotExist(err) {
				t.Errorf("output %s exists: %v", name, err)
			}
		}
	})
}

// TestHookToolWithoutLauncherIgnoresIt runs the build on a module with no
// cmd/launcher: without --launcher the launcher is neither built nor required,
// and no bin directory appears under the output.
func TestHookToolWithoutLauncherIgnoresIt(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "cmd", "hook"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"go.mod": "module example.com/fixture\n\ngo 1.27.4\n", "cmd/hook/main.go": "package main\nfunc main() {}\n"} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(name)), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	reference := filepath.Join(root, "reference")
	rebuild(t, source, reference, hookbuild.ContractBinaries, false)
	var lines []string
	for _, name := range hookbuild.ContractBinaries {
		h, err := hookbuild.FileSHA256(filepath.Join(reference, name))
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, fmt.Sprintf("%s  bin/%s\n", h, name))
	}
	manifest := filepath.Join(root, "SHA256SUMS")
	if err := os.WriteFile(manifest, []byte(strings.Join(lines, "")), 0644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "out")
	if out, err := exec.Command("go", "run", ".", "build", "--source", source, "--manifest", manifest, "--out", dest).CombinedOutput(); err != nil {
		t.Fatalf("build without --launcher: %v: %s", err, out)
	}
	for _, name := range hookbuild.ContractBinaries {
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Lstat(filepath.Join(dest, "bin")); !os.IsNotExist(err) {
		t.Errorf("a build without --launcher created %s: %v", filepath.Join(dest, "bin"), err)
	}
}
