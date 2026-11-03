package application

import (
	"context"
	"os"

	"github.com/gomezv/portucca/internal/allocation"
	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
)

func (a *App) Reallocate(ctx context.Context, worktree string) (EnsureResult, error) {
	c, err := a.Locator.Detect(worktree)
	if err != nil {
		return EnsureResult{}, mapProjectErr(err)
	}
	cfg, _, err := a.loadConfig(c.Root)
	if err != nil {
		return EnsureResult{}, mapConfigErr(err)
	}
	if c.IsMain {
		return EnsureResult{}, apperr.New("main_reserved", "the main checkout cannot be reallocated", "run reallocate inside a worktree")
	}
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), false)
	if err != nil {
		return EnsureResult{}, mapLockErr(err)
	}
	defer unlock()
	reg, err := a.Registry.Load()
	if err != nil {
		return EnsureResult{}, mapRegistryErr(err)
	}
	reg, _ = pruneStale(reg, os.Stat)
	own := findClaim(reg, c.Root)
	if own == nil {
		return EnsureResult{}, apperr.New("no_claim", "this worktree has no claim", "run portucca ensure first")
	}
	if own.Project != cfg.Project {
		return EnsureResult{}, apperr.New("claim_project_mismatch", "the claim belongs to another project", "fix or remove the claim")
	}
	exclude := map[domain.ServiceName]domain.Port{}
	for name, sc := range own.Services {
		exclude[name] = sc.Port
	}
	res, err := allocation.Plan(allocation.Request{
		Config:   cfg,
		Occupied: occupiedWithMainPorts(cfg, reg, c.Root),
		Exclude:  exclude,
		Prober:   a.Prober,
	})
	if err != nil {
		return EnsureResult{}, mapAllocErr(err)
	}
	if _, err := a.EnvStore.WriteValues(c.Root, desiredValues(cfg, res.Services)); err != nil {
		return EnsureResult{}, apperr.New("env_write", err.Error(), "check the env file permissions")
	}
	reg = upsertClaim(reg, domain.Claim{Project: cfg.Project, Path: c.Root, Services: res.Services, UpdatedAt: a.now()})
	if err := a.Registry.Save(reg); err != nil {
		return EnsureResult{}, mapRegistryErr(err)
	}
	return EnsureResult{
		Project: cfg.Project, WorktreePath: c.Root, Changed: true,
		Services: res.Services, Env: desiredEnv(c.Root, cfg, res.Services),
	}, nil
}
