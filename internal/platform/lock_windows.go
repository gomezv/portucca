//go:build windows

package platform

import "time"

func Lock(path string, timeout time.Duration, shared bool) (func() error, error) {
	return nil, ErrUnsupported
}
