package application

import (
	"context"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

type BootstrapResult struct {
	Copied    []string `json:"copied"`
	Symlinked []string `json:"symlinked"`
	Skipped   []string `json:"skipped"`
}

type SetupResult struct {
	EnsureResult
	Bootstrap BootstrapResult `json:"bootstrap"`
}

func (a *App) Setup(ctx context.Context, worktree string) (SetupResult, error) {
	c, err := a.Locator.Detect(worktree)
	if err != nil {
		return SetupResult{}, mapProjectErr(err)
	}
	cfg, cfgPath, err := a.loadConfigFor(c)
	if err != nil {
		return SetupResult{}, mapConfigErr(err)
	}
	return a.setupFrom(ctx, c, cfg, cfgPath)
}

func (a *App) setupFrom(ctx context.Context, c project.Context, cfg domain.Config, cfgPath string) (SetupResult, error) {
	if c.IsMain {
		ensure, err := a.ensureMain(c, cfg, cfgPath)
		if err != nil {
			return SetupResult{}, err
		}
		return SetupResult{
			EnsureResult: ensure,
			Bootstrap:    BootstrapResult{Copied: []string{}, Symlinked: []string{}, Skipped: []string{}},
		}, nil
	}
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), false)
	if err != nil {
		return SetupResult{}, mapLockErr(err)
	}
	defer unlock()
	boot := BootstrapResult{Copied: []string{}, Symlinked: []string{}, Skipped: []string{}}
	for _, rel := range cfg.Bootstrap.CopyFromMain {
		copied, err := a.EnvStore.CopyFromMain(c.MainRoot, c.Root, rel)
		if err != nil {
			return SetupResult{}, mapEnvErr(err)
		}
		if copied {
			boot.Copied = append(boot.Copied, rel)
		} else {
			boot.Skipped = append(boot.Skipped, rel)
		}
	}
	for _, link := range cfg.Bootstrap.Symlinks {
		created, err := a.EnvStore.EnsureSymlink(c.Root, link)
		if err != nil {
			return SetupResult{}, mapEnvErr(err)
		}
		if created {
			boot.Symlinked = append(boot.Symlinked, link.Path)
		} else {
			boot.Skipped = append(boot.Skipped, link.Path)
		}
	}
	ensure, err := a.ensureLocked(c, cfg, cfgPath)
	if err != nil {
		return SetupResult{}, err
	}
	if len(boot.Copied) > 0 || len(boot.Symlinked) > 0 {
		ensure.Changed = true
	}
	return SetupResult{EnsureResult: ensure, Bootstrap: boot}, nil
}
