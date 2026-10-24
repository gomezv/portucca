package platform

import (
	"errors"
	"path/filepath"
	"time"
)

var (
	ErrLockTimeout = errors.New("lock_timeout")
	ErrUnsupported = errors.New("unsupported_platform")
)

func Check() error {
	if !platformSupported {
		return ErrUnsupported
	}
	return nil
}

func RealPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

type Locker struct{}

func (Locker) Lock(path string, timeout time.Duration, shared bool) (func() error, error) {
	return Lock(path, timeout, shared)
}
