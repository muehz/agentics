//go:build windows

package scriptsnap

import "os"

// openNoBlock opens path for reading. Opening an ordinary path never waits for a peer on Windows, and a directory
// opens without blocking and is then refused by its file kind. Named pipes live in a separate namespace, which the
// path shape check refuses before this call. os.Open shares read and write access, so a concurrent writer is not
// locked out.
func openNoBlock(path string) (*os.File, error) { return os.Open(path) }
