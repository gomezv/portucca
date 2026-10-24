package application

import (
	"context"
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
	cfg, _, err := a.loadConfig(c.Root)
	if err != nil {
		return SetupResult{}, mapConfigErr(err)
	}
	if c.IsMain {
		ensure, err := a.Ensure(ctx, worktree)
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
		copied, err := a.Env.CopyFromMain(c.MainRoot, c.Root, rel)
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
		created, err := a.Env.EnsureSymlink(c.Root, link)
		if err != nil {
			return SetupResult{}, mapEnvErr(err)
		}
		if created {
			boot.Symlinked = append(boot.Symlinked, link.Path)
		} else {
			boot.Skipped = append(boot.Skipped, link.Path)
		}
	}
	ensure, err := a.ensureLocked(c, cfg)
	if err != nil {
		return SetupResult{}, err
	}
	return SetupResult{EnsureResult: ensure, Bootstrap: boot}, nil
}
