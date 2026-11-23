//go:build unix

package scriptsnap

import (
	"fmt"
	"os"
	"syscall"
)

// fileID identifies a file independently of the path used to reach it: the device and inode of an open handle, so a
// hardlink and its original agree. A filesystem that reports no inode gives an empty identity, which skips alias
// detection for that file rather than refusing it.
func fileID(_ *os.File, info os.FileInfo) (string, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	return unixID(uint64(st.Dev), uint64(st.Ino)), true
}

func unixID(dev, ino uint64) string {
	if ino == 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d", dev, ino)
}
