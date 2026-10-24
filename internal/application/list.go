package application

import (
	"context"
	"os"
	"sort"
	"time"

	"github.com/gomezv/portucca/internal/domain"
)

type ListClaim struct {
	Project   domain.Project                             `json:"project"`
	Path      string                                     `json:"path"`
	Live      bool                                       `json:"live"`
	Services  map[domain.ServiceName]domain.ServiceClaim `json:"services"`
	UpdatedAt time.Time                                  `json:"updatedAt"`
}

type ListResult struct {
	Claims []ListClaim `json:"claims"`
}

func (a *App) List(ctx context.Context, worktree, project string, all bool) (ListResult, error) {
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), true)
	if err != nil {
		return ListResult{}, mapLockErr(err)
	}
	defer unlock()
	if !all && project == "" && a.Locator != nil && a.Config != nil {
		if c, err := a.Locator.Detect(worktree); err == nil {
			if cfg, _, err := a.loadConfig(c.Root); err == nil {
				project = string(cfg.Project)
			}
		}
	}
	reg, err := a.Registry.Load()
	if err != nil {
		return ListResult{}, mapRegistryErr(err)
	}
	out := ListResult{Claims: []ListClaim{}}
	for _, claim := range reg.Worktrees {
		if !all && project != "" && string(claim.Project) != project {
			continue
		}
		_, statErr := os.Stat(claim.Path)
		out.Claims = append(out.Claims, ListClaim{
			Project: claim.Project, Path: claim.Path, Live: statErr == nil,
			Services: claim.Services, UpdatedAt: claim.UpdatedAt,
		})
	}
	sort.Slice(out.Claims, func(i, j int) bool {
		if out.Claims[i].Project != out.Claims[j].Project {
			return out.Claims[i].Project < out.Claims[j].Project
		}
		return out.Claims[i].Path < out.Claims[j].Path
	})
	return out, nil
}
