package hosttest

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
)

func writeBinary(t *testing.T, hooksDir, name string) string {
	t.Helper()
	path := filepath.Join(hooksDir, "bin", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBinaryPathPresentForContractPlatformWithFile(t *testing.T) {
	hooksDir := t.TempDir()
	want := writeBinary(t, hooksDir, "agx-linux-amd64")

	path, state := binaryPath(hooksDir, "linux", "amd64")
	if state != Present {
		t.Fatalf("state = %v, want Present", state)
	}
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestBinaryPathMissingForContractPlatformWithoutFile(t *testing.T) {
	hooksDir := t.TempDir()

	path, state := binaryPath(hooksDir, "windows", "arm64")
	if state != Missing {
		t.Fatalf("state = %v, want Missing", state)
	}
	if want := filepath.Join(hooksDir, "bin", "agx-windows-arm64.exe"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestBinaryPathDirectoryAtBinaryPathIsMissing(t *testing.T) {
	hooksDir := t.TempDir()
	path := filepath.Join(hooksDir, "bin", "agx-darwin-arm64")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}

	got, state := binaryPath(hooksDir, "darwin", "arm64")
	if state != Missing {
		t.Fatalf("state = %v, want Missing for a directory", state)
	}
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
}

func TestBinaryPathOffContractPlatformHasNoPath(t *testing.T) {
	hooksDir := t.TempDir()
	// A file at a plausible name must not make an off-contract platform present.
	writeBinary(t, hooksDir, "agx-freebsd-amd64")

	path, state := binaryPath(hooksDir, "freebsd", "amd64")
	if state != OffContract {
		t.Fatalf("state = %v, want OffContract", state)
	}
	if path != "" {
		t.Fatalf("path = %q, want empty", path)
	}
}

func TestBinaryPathReadsEveryContractNameAsOnContract(t *testing.T) {
	for name, target := range hookbuild.ContractTargets {
		t.Run(name, func(t *testing.T) {
			hooksDir := t.TempDir()
			if _, state := binaryPath(hooksDir, target[0], target[1]); state != Missing {
				t.Fatalf("without a file: state = %v, want Missing", state)
			}
			want := writeBinary(t, hooksDir, name)
			path, state := binaryPath(hooksDir, target[0], target[1])
			if state != Present || path != want {
				t.Fatalf("with a file: got (%q, %v), want (%q, Present)", path, state, want)
			}
		})
	}
	if len(hookbuild.ContractBinaries) != 6 {
		t.Fatalf("ContractBinaries has %d names, want 6", len(hookbuild.ContractBinaries))
	}
	for _, name := range hookbuild.ContractBinaries {
		if _, ok := hookbuild.ContractTargets[name]; !ok {
			t.Errorf("contract binary %q is not covered above", name)
		}
	}
}

func TestBinaryMessages(t *testing.T) {
	if got, want := offContractMessage("freebsd", "amd64"), "no hook binary is committed for freebsd/amd64"; got != want {
		t.Fatalf("off-contract message = %q, want %q", got, want)
	}
	if got, want := missingMessage("linux", "arm64", "/x/agx-linux-arm64"),
		"the committed hook binary for linux/arm64 is missing: /x/agx-linux-arm64"; got != want {
		t.Fatalf("missing message = %q, want %q", got, want)
	}
}

func TestCommittedBinaryPathUsesHostPlatform(t *testing.T) {
	hooksDir := t.TempDir()
	wantName := hookbuild.HostBinary(runtime.GOOS, runtime.GOARCH)

	path, state := CommittedBinaryPath(hooksDir)
	if wantName == "" {
		if state != OffContract || path != "" {
			t.Fatalf("off-contract host: got (%q, %v)", path, state)
		}
		return
	}
	if state != Missing {
		t.Fatalf("state = %v, want Missing", state)
	}
	if want := filepath.Join(hooksDir, "bin", wantName); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}
