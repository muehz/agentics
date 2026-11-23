//go:build !linux && !darwin

package flow

import "errors"

func mutationLimitFileSize() (func() error, error) {
	return nil, errors.New("isolated Unix file-size limits are unavailable on this platform")
}
