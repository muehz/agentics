package flow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func portabilityHeader() []byte {
	return []byte(stateHeader)
}

func preparePortabilityHeader(t *testing.T, folder string) string {
	t.Helper()
	source := filepath.Join(folder, "prepared-header")
	if err := os.WriteFile(source, portabilityHeader(), 0o666); err != nil {
		t.Fatal(err)
	}
	return source
}

func deniedPortabilityLink(t *testing.T, source, destination string) func(string, string) error {
	t.Helper()
	return func(old, new string) error {
		if old != source || new != destination {
			t.Fatalf("link paths = %q, %q; want %q, %q", old, new, source, destination)
		}
		return &os.LinkError{Op: "link", Old: old, New: new, Err: errors.ErrUnsupported}
	}
}

func assertPortabilityBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(want) {
		t.Fatalf("file %q bytes = %q, error = %v; want %q", path, got, err, want)
	}
}

func assertPortabilityAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination %q exists or cannot be checked: %v", path, err)
	}
}

func assertPortabilityPathError(t *testing.T, got, want error) {
	t.Helper()
	var gotPath, wantPath *os.PathError
	if !errors.As(want, &wantPath) {
		t.Fatalf("fixture did not produce a filesystem path error: %v", want)
	}
	if !errors.As(got, &gotPath) || gotPath.Path != wantPath.Path || !errors.Is(got, wantPath.Err) {
		t.Fatalf("error = %v; want path %q and underlying cause %v", got, wantPath.Path, wantPath.Err)
	}
}

func TestStatePortabilityDeniedLinkPublishesHeaderWithOrdinaryPermissions(t *testing.T) {
	folder := t.TempDir()
	source := preparePortabilityHeader(t, folder)
	destination := filepath.Join(folder, "state.tsv")
	control := filepath.Join(folder, "ordinary-create")
	f, err := os.OpenFile(control, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := publishStateUsing(source, destination, portabilityHeader(), deniedPortabilityLink(t, source, destination)); err != nil {
		t.Fatalf("ordinary creation is available but header publication failed: %v", err)
	}
	assertPortabilityBytes(t, destination, portabilityHeader())
	assertPortabilityBytes(t, source, portabilityHeader())
	want, err := os.Stat(control)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode().Perm() != want.Mode().Perm() {
		t.Fatalf("state permissions = %o, ordinary create = %o", got.Mode().Perm(), want.Mode().Perm())
	}
}

func TestStatePortabilityWriteOnlyPreparedSource(t *testing.T) {
	folder := t.TempDir()
	source := preparePortabilityHeader(t, folder)
	if err := os.Chmod(source, 0o222); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(source, 0o666); err != nil {
			t.Error(err)
		}
	})
	destination := filepath.Join(folder, "state.tsv")
	if err := publishStateUsing(source, destination, portabilityHeader(), deniedPortabilityLink(t, source, destination)); err != nil {
		t.Fatalf("publication required reading a prepared write-only source: %v", err)
	}
	assertPortabilityBytes(t, destination, portabilityHeader())
}

func TestStatePortabilityDeniedLinkPreservesWinner(t *testing.T) {
	for _, duringLink := range []bool{false, true} {
		name := "existing"
		if duringLink {
			name = "racing"
		}
		t.Run(name, func(t *testing.T) {
			folder := t.TempDir()
			source := preparePortabilityHeader(t, folder)
			destination := filepath.Join(folder, "state.tsv")
			winner := append(portabilityHeader(), []byte("winner row without final newline")...)
			createWinner := func() {
				if err := os.WriteFile(destination, winner, 0o666); err != nil {
					t.Fatal(err)
				}
			}
			if !duringLink {
				createWinner()
			}
			denied := deniedPortabilityLink(t, source, destination)
			link := func(old, new string) error {
				if duringLink {
					createWinner()
				}
				return denied(old, new)
			}
			if err := publishStateUsing(source, destination, portabilityHeader(), link); err != nil {
				t.Fatalf("losing creator failed: %v", err)
			}
			assertPortabilityBytes(t, destination, winner)
			assertPortabilityBytes(t, source, portabilityHeader())
		})
	}
}

func TestStatePortabilitySourceMetadataFailureDoesNotCreateDestination(t *testing.T) {
	for _, blockedParent := range []bool{false, true} {
		name := "missing"
		if blockedParent {
			name = "non-directory-parent"
		}
		t.Run(name, func(t *testing.T) {
			folder := t.TempDir()
			parent := filepath.Join(folder, "source-parent")
			if blockedParent {
				if err := os.WriteFile(parent, []byte("regular file"), 0o666); err != nil {
					t.Fatal(err)
				}
			}
			source := filepath.Join(parent, "prepared-header")
			destination := filepath.Join(folder, "state.tsv")
			_, want := os.Lstat(source)
			got := publishStateUsing(source, destination, portabilityHeader(), deniedPortabilityLink(t, source, destination))
			assertPortabilityAbsent(t, destination)
			assertPortabilityPathError(t, got, want)
		})
	}
}

func TestStatePortabilityDirectorySourceDoesNotCreateDestination(t *testing.T) {
	folder := t.TempDir()
	source := filepath.Join(folder, "directory-source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(folder, "state.tsv")
	err := publishStateUsing(source, destination, portabilityHeader(), deniedPortabilityLink(t, source, destination))
	assertPortabilityAbsent(t, destination)
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("nonregular prepared source error = %v; want invalid-source error", err)
	}
}

func TestStatePortabilityFinalCreationFailureIsReturned(t *testing.T) {
	for _, blockedParent := range []bool{false, true} {
		name := "missing-parent"
		if blockedParent {
			name = "non-directory-parent"
		}
		t.Run(name, func(t *testing.T) {
			folder := t.TempDir()
			source := preparePortabilityHeader(t, folder)
			parent := filepath.Join(folder, "destination-parent")
			if blockedParent {
				if err := os.WriteFile(parent, []byte("regular file"), 0o666); err != nil {
					t.Fatal(err)
				}
			}
			destination := filepath.Join(parent, "state.tsv")
			f, want := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
			if want == nil {
				f.Close()
				t.Fatal("fixture unexpectedly permits destination creation")
			}
			got := publishStateUsing(source, destination, portabilityHeader(), deniedPortabilityLink(t, source, destination))
			assertPortabilityPathError(t, got, want)
			assertPortabilityBytes(t, source, portabilityHeader())
			if blockedParent {
				assertPortabilityBytes(t, parent, []byte("regular file"))
			} else {
				assertPortabilityAbsent(t, parent)
			}
		})
	}
}

func TestStatePortabilityHardlinkSuccessAndExistingDestination(t *testing.T) {
	folder := t.TempDir()
	source := preparePortabilityHeader(t, folder)
	destination := filepath.Join(folder, "state.tsv")
	if err := publishStateUsing(source, destination, portabilityHeader(), os.Link); err != nil {
		t.Fatal(err)
	}
	assertPortabilityBytes(t, destination, portabilityHeader())
	sourceInfo, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	destinationInfo, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(sourceInfo, destinationInfo) {
		t.Fatal("successful hardlink publication stopped linking the prepared header")
	}
	loser := filepath.Join(folder, "losing-header")
	if err := os.WriteFile(loser, []byte("losing contents"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := publishStateUsing(loser, destination, portabilityHeader(), os.Link); err != nil {
		t.Fatalf("existing destination link error was not accepted: %v", err)
	}
	assertPortabilityBytes(t, destination, portabilityHeader())
	assertPortabilityBytes(t, loser, []byte("losing contents"))
}
