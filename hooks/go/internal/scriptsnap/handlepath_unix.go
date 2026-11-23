//go:build !windows

package scriptsnap

import "os"

// handlePath reports no handle-based name: the file behind a descriptor has no portable name here, so the caller
// resolves the path again and compares it with the handle.
func handlePath(*os.File) (string, bool, error) { return "", false, nil }
