//go:build windows

package scriptsnap

import (
	"fmt"
	"os"
	"syscall"
)

// fileID identifies a file independently of the path used to reach it: the volume serial number and file index of
// an open handle, so a hardlink and its original agree. Filesystems that cannot supply an index report zero or all
// ones; that gives an empty identity, which skips alias detection for the file rather than refusing it.
func fileID(f *os.File, _ os.FileInfo) (string, bool) {
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info); err != nil {
		return "", false
	}
	return windowsID(info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), true
}

func windowsID(volume, high, low uint32) string {
	if (high == 0 && low == 0) || (high == 0xFFFFFFFF && low == 0xFFFFFFFF) {
		return ""
	}
	return fmt.Sprintf("%d:%d:%d", volume, high, low)
}
