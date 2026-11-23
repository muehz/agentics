// Package hookutiltest holds test-only helpers shared by hook packages. It is
// separate from hookutil so a non-test package never imports testing.
package hookutiltest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// SymlinkOrSkip creates a symlink or skips the test on a host that refuses
// one (Windows without the privilege).
func SymlinkOrSkip(t testing.TB, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
}

// JunctionOrSkip creates a Windows directory junction (mklink /J) or skips
// when the host is not Windows or cmd.exe refuses. cmd.exe resolves from
// PATH normally even though this host sets
// NoDefaultCurrentDirectoryInExePath=1.
func JunctionOrSkip(t testing.TB, target, link string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("junctions only exist on Windows")
	}
	cmd := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", filepath.FromSlash(link), filepath.FromSlash(target))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("mklink /J unavailable: %v\n%s", err, out)
	}
}
