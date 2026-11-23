package hosttest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/hookbuild"
)

// BinaryState is what the checkout holds for the committed hook binary of one
// platform.
type BinaryState int

const (
	// OffContract: no hook binary is committed for the platform, so there is
	// nothing to run. A file that happens to sit at a plausible name does not
	// change this, because nothing vouches for it.
	OffContract BinaryState = iota
	// Missing: the platform is on the build contract but no regular file is at
	// the binary's path, which is a defect in the checkout.
	Missing
	// Present: the platform is on the build contract and a regular file is at
	// the binary's path.
	Present
)

func (s BinaryState) String() string {
	switch s {
	case OffContract:
		return "OffContract"
	case Missing:
		return "Missing"
	case Present:
		return "Present"
	}
	return fmt.Sprintf("BinaryState(%d)", int(s))
}

// CommittedBinaryPath returns where the committed hook binary for the running
// platform lives under hooksDir (the checkout's hooks directory), and what
// the checkout holds there. It returns the empty path with OffContract when
// the platform has no contract binary.
func CommittedBinaryPath(hooksDir string) (string, BinaryState) {
	return binaryPath(hooksDir, runtime.GOOS, runtime.GOARCH)
}

// binaryPath is CommittedBinaryPath for a named platform.
func binaryPath(hooksDir, goos, goarch string) (string, BinaryState) {
	name := hookbuild.HostBinary(goos, goarch)
	if name == "" {
		return "", OffContract
	}
	path := filepath.Join(hooksDir, "bin", name)
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return path, Missing
	}
	return path, Present
}

// CommittedBinary returns the committed hook binary for the running platform.
// It skips the test when the platform has no contract binary and fails it when
// a contract platform's binary is missing, so a test cannot pass without
// running the binary it exists to run.
func CommittedBinary(t testing.TB) string {
	t.Helper()
	path, state := CommittedBinaryPath(filepath.Join(Checkout(t), "hooks"))
	switch state {
	case OffContract:
		t.Skip(offContractMessage(runtime.GOOS, runtime.GOARCH))
	case Missing:
		t.Fatal(missingMessage(runtime.GOOS, runtime.GOARCH, path))
	}
	return path
}

func offContractMessage(goos, goarch string) string {
	return fmt.Sprintf("no hook binary is committed for %s/%s", goos, goarch)
}

func missingMessage(goos, goarch, path string) string {
	return fmt.Sprintf("the committed hook binary for %s/%s is missing: %s", goos, goarch, path)
}
