package application

import (
	"context"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
)

type ReleaseResult struct {
	Project      domain.Project                     `json:"project"`
	WorktreePath string                             `json:"worktreePath"`
	Released     map[domain.ServiceName]domain.Port `json:"released"`
}

func (a *App) Release(ctx context.Context, worktree string, service domain.ServiceName) (ReleaseResult, error) {
	c, err := a.Locator.Detect(worktree)
	if err != nil {
		return ReleaseResult{}, mapProjectErr(err)
	}
	if c.IsMain {
		return ReleaseResult{}, apperr.New("main_reserved", "the main checkout has no claim to release", "run release inside a worktree")
	}
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), false)
	if err != nil {
		return ReleaseResult{}, mapLockErr(err)
	}
	defer unlock()
	reg, err := a.Registry.Load()
	if err != nil {
		return ReleaseResult{}, mapRegistryErr(err)
	}
	index := -1
	for i := range reg.Worktrees {
		if reg.Worktrees[i].Path == c.Root {
			index = i
			break
		}
	}
	if index < 0 {
		return ReleaseResult{}, apperr.New("no_claim", "this worktree has no claim", "run portucca list to inspect the registry")
	}
	own := &reg.Worktrees[index]
	project := own.Project
	next := make(map[domain.ServiceName]domain.ServiceClaim, len(own.Services))
	for name, sc := range own.Services {
		next[name] = sc
	}
	released := map[domain.ServiceName]domain.Port{}
	if service != "" {
		sc, ok := next[service]
		if !ok {
			return ReleaseResult{}, apperr.New("service_not_claimed", "the service is not claimed for this worktree", "run portucca list to inspect the claim")
		}
		released[service] = sc.Port
		delete(next, service)
	} else {
		for name, sc := range next {
			released[name] = sc.Port
		}
		next = map[domain.ServiceName]domain.ServiceClaim{}
	}
	if len(next) == 0 {
		reg.Worktrees = append(reg.Worktrees[:index], reg.Worktrees[index+1:]...)
	} else {
		reg.Worktrees[index].Services = next
		reg.Worktrees[index].UpdatedAt = a.now()
	}
	if err := a.Registry.Save(reg); err != nil {
		return ReleaseResult{}, mapRegistryErr(err)
	}
	return ReleaseResult{Project: project, WorktreePath: c.Root, Released: released}, nil
}
