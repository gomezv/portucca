package application

import (
	"context"
	"time"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
	"github.com/gomezv/portucca/internal/scan"
)

type Locator interface {
	Detect(path string) (project.Context, error)
}

type ConfigLoader interface {
	LoadRoot(root string) (domain.Config, string, error)
	LoadFile(path string) (domain.Config, error)
}

type RegistryStore interface {
	Load() (domain.Registry, error)
	Save(domain.Registry) error
	LockPath() string
}

type Locker interface {
	Lock(path string, timeout time.Duration, shared bool) (func() error, error)
}

type Prober interface {
	IsFree(domain.Port) bool
}

type EnvStore interface {
	ReadPort(root string, ref domain.EnvRef) (domain.Port, bool, error)
	ReadValue(root string, ref domain.EnvRef) (string, bool, error)
	WriteValues(root string, values map[string]map[string]string) (bool, error)
	CopyFromMain(mainRoot, wtRoot, rel string) (bool, error)
	EnsureSymlink(wtRoot string, s domain.Symlink) (bool, error)
}

type ScanRunner interface {
	Scan(ctx context.Context, opts scan.Options) (scan.Detected, error)
}

type App struct {
	Locator     Locator
	Config      ConfigLoader
	Registry    RegistryStore
	Locker      Locker
	Prober      Prober
	EnvStore    EnvStore
	Executor    Executor
	Scanner     ScanRunner
	ConfigFile  string
	Now         func() time.Time
	LockTimeout time.Duration
}

func (a *App) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now().UTC()
}

func (a *App) timeout() time.Duration {
	if a.LockTimeout > 0 {
		return a.LockTimeout
	}
	return 5 * time.Second
}
