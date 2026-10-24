package application

import (
	"context"
	"os"

	"github.com/gomezv/portucca/internal/domain"
)

type PruneResult struct {
	Removed []domain.Claim `json:"removed"`
}

func (a *App) Prune(ctx context.Context) (PruneResult, error) {
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), false)
	if err != nil {
		return PruneResult{}, mapLockErr(err)
	}
	defer unlock()
	reg, err := a.Registry.Load()
	if err != nil {
		return PruneResult{}, mapRegistryErr(err)
	}
	reg, removed := pruneStale(reg, os.Stat)
	if removed == nil {
		removed = []domain.Claim{}
	}
	if len(removed) > 0 {
		if err := a.Registry.Save(reg); err != nil {
			return PruneResult{}, mapRegistryErr(err)
		}
	}
	return PruneResult{Removed: removed}, nil
}
