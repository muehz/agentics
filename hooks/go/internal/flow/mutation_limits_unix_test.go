//go:build linux || darwin

package flow

import (
	"os/signal"
	"syscall"
)

// mutationLimitFileSize is called only inside the owned writer child. Its
// restore function runs before coverage writes its metadata on test exit.
func mutationLimitFileSize() (func() error, error) {
	var previous syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &previous); err != nil {
		return nil, err
	}
	limited := previous
	limited.Cur = 0
	signal.Ignore(syscall.SIGXFSZ)
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limited); err != nil {
		signal.Reset(syscall.SIGXFSZ)
		return nil, err
	}
	return func() error {
		err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &previous)
		signal.Reset(syscall.SIGXFSZ)
		return err
	}, nil
}
