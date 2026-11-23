package hookbuild_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
)

// absent fails the test unless nothing exists at path.
func absent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("%s exists or is unreadable (err = %v), want it absent", path, err)
	}
}

// publishable writes the six contract files and the launcher into the
// publication's scratch directory and returns a Result over them, built
// without any go build.
func publishable(t *testing.T, pub *hookbuild.Publication) hookbuild.Result {
	t.Helper()
	scratch, err := pub.Scratch()
	if err != nil {
		t.Fatalf("Scratch: %v", err)
	}
	var res hookbuild.Result
	for _, name := range hookbuild.ContractBinaries {
		path := filepath.Join(scratch, name)
		if err := os.WriteFile(path, []byte("bytes of "+name), 0o755); err != nil {
			t.Fatal(err)
		}
		res.Targets = append(res.Targets, hookbuild.Target{Name: name, Path: path})
	}
	launcher := filepath.Join(scratch, "agx.exe")
	if err := os.WriteFile(launcher, []byte("bytes of the launcher"), 0o755); err != nil {
		t.Fatal(err)
	}
	res.Launcher = &hookbuild.Target{Name: "agx.exe", Path: launcher}
	return res
}

func TestPublicationRefusesHooksBin(t *testing.T) {
	cases := []struct {
		name        string
		out         string
		parent      string
		launcher    bool
		wantRefusal bool
	}{
		{"hooks/bin", "hooks/bin", "hooks", false, true},
		{"hooks/bin with the launcher", "hooks/bin", "hooks", true, true},
		{"under hooks/bin", "hooks/bin/nested", "hooks/bin", false, true},
		{"under hooks/bin with the launcher", "hooks/bin/nested", "hooks/bin", true, true},
		{"mixed case", "Hooks/BIN", "Hooks", false, true},
		{"mixed case with the launcher", "Hooks/BIN", "Hooks", true, true},
		{"mixed case under hooks/bin", "HOOKS/Bin/Nested", "HOOKS/Bin", false, true},
		{"hooks with the launcher", "hooks", ".", true, true},
		{"mixed case hooks with the launcher", "HOOKS", ".", true, true},
		{"hooks without the launcher", "hooks", ".", false, false},
		{"another directory with the launcher", "other", ".", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(tc.parent)), 0o755); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(root, filepath.FromSlash(tc.out))
			pub, err := hookbuild.OpenPublication(out, tc.launcher)
			if !tc.wantRefusal {
				if err != nil {
					t.Fatalf("OpenPublication = %v, want it accepted", err)
				}
				if closeErr := pub.Close(); closeErr != nil {
					t.Fatal(closeErr)
				}
				return
			}
			if err == nil || err.Error() != "refusing to write hooks/bin" {
				t.Fatalf("OpenPublication = %v, want %q", err, "refusing to write hooks/bin")
			}
			absent(t, filepath.Join(out, "bin"))
			for _, name := range hookbuild.ContractBinaries {
				absent(t, filepath.Join(out, name))
			}
		})
	}
}

func TestPublicationRefusesHooksBinThroughAParentSymlink(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "hooks", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(bin, link); err != nil {
		t.Skipf("this host cannot create symlinks: %v", err)
	}
	for _, launcher := range []bool{false, true} {
		_, err := hookbuild.OpenPublication(filepath.Join(link, "out"), launcher)
		if err == nil || err.Error() != "refusing to write hooks/bin" {
			t.Errorf("launcher=%v: OpenPublication = %v, want %q", launcher, err, "refusing to write hooks/bin")
		}
	}
}

func TestPublicationResolvesTheParent(t *testing.T) {
	out := filepath.Join(t.TempDir(), "missing", "out")
	_, err := hookbuild.OpenPublication(out, false)
	if err == nil || !strings.HasPrefix(err.Error(), "output parent: ") {
		t.Fatalf("OpenPublication = %v, want an %q error", err, "output parent: ")
	}
}

func TestPublicationRefusesAnOccupiedOutput(t *testing.T) {
	occupy := func(t *testing.T, dest, name string) string {
		t.Helper()
		path := filepath.Join(dest, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("occupant of "+name), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	names := append(slices.Clone(hookbuild.ContractBinaries), hookbuild.LauncherOutput)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "out")
			path := occupy(t, dest, name)
			pub, err := hookbuild.OpenPublication(dest, true)
			want := "output already exists: " + name
			if err == nil || err.Error() != want {
				t.Fatalf("OpenPublication = %v, want %q", err, want)
			}
			if pub != nil {
				t.Error("OpenPublication returned a Publication with its error")
			}
			if got, readErr := os.ReadFile(path); readErr != nil || string(got) != "occupant of "+name {
				t.Errorf("occupant = %q, %v, want its bytes kept", got, readErr)
			}
			entries, err := os.ReadDir(dest)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".hooktool-") {
					t.Errorf("OpenPublication created %s", entry.Name())
				}
			}
		})
	}

	t.Run("the six are checked before the launcher", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out")
		occupy(t, dest, hookbuild.LauncherOutput)
		occupy(t, dest, hookbuild.ContractBinaries[5])
		_, err := hookbuild.OpenPublication(dest, true)
		want := "output already exists: " + hookbuild.ContractBinaries[5]
		if err == nil || err.Error() != want {
			t.Fatalf("OpenPublication = %v, want %q", err, want)
		}
	})

	t.Run("an occupied launcher path does not matter without the launcher", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out")
		occupy(t, dest, hookbuild.LauncherOutput)
		pub, err := hookbuild.OpenPublication(dest, false)
		if err != nil {
			t.Fatalf("OpenPublication = %v, want it accepted", err)
		}
		if err := pub.Close(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a fresh output stays absent", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out")
		pub, err := hookbuild.OpenPublication(dest, true)
		if err != nil {
			t.Fatalf("OpenPublication = %v", err)
		}
		absent(t, dest)
		if err := pub.Close(); err != nil {
			t.Fatalf("Close before any Scratch = %v, want nil", err)
		}
		absent(t, dest)
	})
}

func TestPublicationScratchIsStable(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	pub, err := hookbuild.OpenPublication(dest, false)
	if err != nil {
		t.Fatal(err)
	}
	first, err := pub.Scratch()
	if err != nil {
		t.Fatal(err)
	}
	second, err := pub.Scratch()
	if err != nil || second != first {
		t.Fatalf("second Scratch = %q, %v, want %q", second, err, first)
	}
	if filepath.Dir(first) != dest || !strings.HasPrefix(filepath.Base(first), ".hooktool-") {
		t.Errorf("Scratch = %q, want a .hooktool- directory inside %s", first, dest)
	}
	if err := pub.Close(); err != nil {
		t.Fatal(err)
	}
	absent(t, first)
	if info, err := os.Stat(dest); err != nil || !info.IsDir() {
		t.Errorf("Close removed the destination: %v", err)
	}
}

func TestPublicationRollsBack(t *testing.T) {
	for _, launcher := range []bool{true, false} {
		name := "with the launcher"
		if !launcher {
			name = "without the launcher"
		}
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "out")
			pub, err := hookbuild.OpenPublication(dest, launcher)
			if err != nil {
				t.Fatal(err)
			}
			res := publishable(t, pub)
			if !launcher {
				res.Launcher = nil
			}
			unrelated := filepath.Join(dest, "unrelated.txt")
			if err := os.WriteFile(unrelated, []byte("keep"), 0o644); err != nil {
				t.Fatal(err)
			}
			broken := res.Targets[1]
			if broken.Name != "agx-darwin-arm64" {
				t.Fatalf("second target = %s", broken.Name)
			}
			if err := os.Remove(broken.Path); err != nil {
				t.Fatal(err)
			}

			err = pub.Commit(res)
			if err == nil || !strings.HasPrefix(err.Error(), "publishing agx-darwin-arm64: ") {
				t.Fatalf("Commit = %v, want a %q error", err, "publishing agx-darwin-arm64: ")
			}
			if strings.Contains(err.Error(), "removing ") {
				t.Errorf("Commit = %v, want a clean rollback", err)
			}
			for _, name := range hookbuild.ContractBinaries {
				absent(t, filepath.Join(dest, name))
			}
			absent(t, filepath.Join(dest, filepath.FromSlash(hookbuild.LauncherOutput)))
			if got, err := os.ReadFile(unrelated); err != nil || string(got) != "keep" {
				t.Errorf("unrelated file = %q, %v, want it intact", got, err)
			}

			if err := os.WriteFile(broken.Path, []byte("bytes of "+broken.Name), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := pub.Commit(res); err != nil {
				t.Fatalf("second Commit = %v, want success", err)
			}
			want := append([]string(nil), hookbuild.ContractBinaries...)
			if launcher {
				want = append(want, hookbuild.LauncherOutput)
			}
			for _, name := range want {
				got, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(name)))
				if err != nil {
					t.Fatalf("published %s: %v", name, err)
				}
				if !strings.HasPrefix(string(got), "bytes of ") {
					t.Errorf("%s holds %q", name, got)
				}
			}
			if !launcher {
				absent(t, filepath.Join(dest, filepath.FromSlash(hookbuild.LauncherOutput)))
			}

			if err := pub.Close(); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(dest)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".hooktool-") {
					t.Errorf("Close left %s", entry.Name())
				}
			}
			if got, err := os.ReadFile(unrelated); err != nil || string(got) != "keep" {
				t.Errorf("unrelated file after Close = %q, %v", got, err)
			}
			for _, name := range want {
				if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(name))); err != nil {
					t.Errorf("Close removed %s: %v", name, err)
				}
			}
		})
	}
}

func TestPublicationSecondCommitRemovesNothingOfTheFirst(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	pub, err := hookbuild.OpenPublication(dest, true)
	if err != nil {
		t.Fatal(err)
	}
	res := publishable(t, pub)
	if err := pub.Commit(res); err != nil {
		t.Fatal(err)
	}
	err = pub.Commit(res)
	if err == nil || !strings.HasPrefix(err.Error(), "publishing bin/agx.exe: ") {
		t.Fatalf("second Commit = %v, want a %q error", err, "publishing bin/agx.exe: ")
	}
	for _, name := range append(slices.Clone(hookbuild.ContractBinaries), hookbuild.LauncherOutput) {
		got, readErr := os.ReadFile(filepath.Join(dest, filepath.FromSlash(name)))
		if readErr != nil || !strings.HasPrefix(string(got), "bytes of ") {
			t.Errorf("%s = %q, %v, want the first Commit's file", name, got, readErr)
		}
	}
}

func TestPublicationLauncherFailureLeavesNothing(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	pub, err := hookbuild.OpenPublication(dest, true)
	if err != nil {
		t.Fatal(err)
	}
	res := publishable(t, pub)
	if err := os.Remove(res.Launcher.Path); err != nil {
		t.Fatal(err)
	}
	err = pub.Commit(res)
	if err == nil || !strings.HasPrefix(err.Error(), "publishing bin/agx.exe: ") {
		t.Fatalf("Commit = %v, want a %q error", err, "publishing bin/agx.exe: ")
	}
	for _, name := range append(slices.Clone(hookbuild.ContractBinaries), hookbuild.LauncherOutput) {
		absent(t, filepath.Join(dest, filepath.FromSlash(name)))
	}
}

func TestPublicationRefusesAResultThatDoesNotFit(t *testing.T) {
	t.Run("a target that is not a contract binary", func(t *testing.T) {
		for _, bad := range []string{"agx-plan9-386", filepath.Join("..", "escaped"), "bin/agx.exe", ""} {
			dest := filepath.Join(t.TempDir(), "out")
			pub, err := hookbuild.OpenPublication(dest, true)
			if err != nil {
				t.Fatal(err)
			}
			res := publishable(t, pub)
			res.Targets = append(res.Targets, hookbuild.Target{Name: bad, Path: res.Targets[0].Path})
			want := "hookbuild: " + bad + " is not a contract binary"
			if err := pub.Commit(res); err == nil || err.Error() != want {
				t.Errorf("Commit with target %q = %v, want %q", bad, err, want)
			}
			absent(t, filepath.Join(dest, filepath.FromSlash(hookbuild.LauncherOutput)))
			for _, name := range hookbuild.ContractBinaries {
				absent(t, filepath.Join(dest, name))
			}
			absent(t, filepath.Join(filepath.Dir(dest), "escaped"))
		}
	})

	t.Run("a launcher the publication was not opened for", func(t *testing.T) {
		hooks := filepath.Join(t.TempDir(), "hooks")
		if err := os.MkdirAll(hooks, 0o755); err != nil {
			t.Fatal(err)
		}
		pub, err := hookbuild.OpenPublication(hooks, false)
		if err != nil {
			t.Fatal(err)
		}
		res := publishable(t, pub)
		want := "hookbuild: the result has a launcher, but the publication was opened without one"
		if err := pub.Commit(res); err == nil || err.Error() != want {
			t.Fatalf("Commit = %v, want %q", err, want)
		}
		absent(t, filepath.Join(hooks, "bin"))
		for _, name := range hookbuild.ContractBinaries {
			absent(t, filepath.Join(hooks, name))
		}
	})

	t.Run("no launcher where the publication expects one", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out")
		pub, err := hookbuild.OpenPublication(dest, true)
		if err != nil {
			t.Fatal(err)
		}
		res := publishable(t, pub)
		res.Launcher = nil
		want := "hookbuild: the result has no launcher, but the publication was opened with one"
		if err := pub.Commit(res); err == nil || err.Error() != want {
			t.Fatalf("Commit = %v, want %q", err, want)
		}
		for _, name := range hookbuild.ContractBinaries {
			absent(t, filepath.Join(dest, name))
		}
	})
}
