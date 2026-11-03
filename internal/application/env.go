package application

import (
	"context"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
)

type EnvResult struct {
	Project      domain.Project                             `json:"project"`
	WorktreePath string                                     `json:"worktreePath"`
	IsMain       bool                                       `json:"isMain"`
	Services     map[domain.ServiceName]domain.ServiceClaim `json:"services"`
	Env          []EnvValue                                 `json:"env"`
}

func (a *App) Env(ctx context.Context, worktree string) (EnvResult, error) {
	c, err := a.Locator.Detect(worktree)
	if err != nil {
		return EnvResult{}, mapProjectErr(err)
	}
	cfg, _, err := a.loadConfigFor(c)
	if err != nil {
		return EnvResult{}, mapConfigErr(err)
	}
	if c.IsMain {
		services := mainServices(cfg)
		return EnvResult{Project: cfg.Project, WorktreePath: c.Root, IsMain: true, Services: services, Env: desiredEnv(c.Root, cfg, services)}, nil
	}
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), true)
	if err != nil {
		return EnvResult{}, mapLockErr(err)
	}
	defer unlock()
	reg, err := a.Registry.Load()
	if err != nil {
		return EnvResult{}, mapRegistryErr(err)
	}
	own := findClaim(reg, c.Root)
	if own == nil {
		return EnvResult{}, apperr.New("no_claim", "this worktree has no claim", "run portucca ensure first")
	}
	return EnvResult{Project: cfg.Project, WorktreePath: c.Root, Services: own.Services, Env: desiredEnv(c.Root, cfg, own.Services)}, nil
}
