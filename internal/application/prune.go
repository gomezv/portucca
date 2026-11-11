package application

import (
	"context"
	"os"

	"github.com/gomezv/portucca/internal/domain"
)

type PruneResult struct {
	Removed      []domain.Claim           `json:"removed"`
	RemovedMains []domain.MainReservation `json:"removedMains"`
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
	reg, removed, removedMains := pruneStale(reg, os.Stat)
	if removed == nil {
		removed = []domain.Claim{}
	}
	if removedMains == nil {
		removedMains = []domain.MainReservation{}
	}
	if len(removed) > 0 || len(removedMains) > 0 {
		if err := a.Registry.Save(reg); err != nil {
			return PruneResult{}, mapRegistryErr(err)
		}
	}
	return PruneResult{Removed: removed, RemovedMains: removedMains}, nil
}
