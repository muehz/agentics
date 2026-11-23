//go:build unix

package scriptsnap

import (
	"os"
	"syscall"
)

// openNoBlock opens path for reading without waiting for a writer and without following a final symlink: a FIFO that
// replaced the file after the Lstat would otherwise hold the open call, and every waiter on the entry, indefinitely,
// and a symlink swapped in would be read as the file it points to.
func openNoBlock(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
}
