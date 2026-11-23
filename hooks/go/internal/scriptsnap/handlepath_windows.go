//go:build windows

package scriptsnap

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var procGetFinalPathNameByHandle = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFinalPathNameByHandleW")

// handlePath returns the normalized DOS path of the file behind an open handle. Path resolution cannot follow a
// junction on Windows, but the handle always names the file actually opened, whatever links led to it.
func handlePath(f *os.File) (string, bool, error) {
	buf := make([]uint16, 512)
	for {
		// The volume and name flags are both zero: a DOS drive name and the normalized file name.
		n, _, callErr := procGetFinalPathNameByHandle.Call(f.Fd(), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
		switch {
		case n == 0:
			return "", false, callErr
		case int(n) > len(buf):
			buf = make([]uint16, n+1)
			continue
		}
		return stripNamespace(syscall.UTF16ToString(buf[:n])), true, nil
	}
}

// stripNamespace turns the extended-length form the system returns back into an ordinary path, so the policy and the
// shape rules see the spelling a user would write. The UNC form maps to `\\server\share`, which the shape rules refuse
// as a network path, so the mapping only keeps the refusal reason honest.
func stripNamespace(p string) string {
	switch {
	case strings.HasPrefix(p, `\\?\UNC\`):
		return `\\` + p[len(`\\?\UNC\`):]
	case strings.HasPrefix(p, `\\?\`):
		return p[len(`\\?\`):]
	}
	return p
}
