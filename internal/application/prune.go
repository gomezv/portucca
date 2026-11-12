package application

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/gomezv/portucca/internal/domain"
)

type PruneResult struct {
	Removed      []domain.Claim           `json:"removed"`
	RemovedMains []domain.MainReservation `json:"removedMains"`
}

func (a *App) Prune(ctx context.Context, worktree string) (PruneResult, error) {
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
	if root := a.mainWithoutConfig(worktree); root != "" {
		kept := reg.Mains[:0]
		for _, main := range reg.Mains {
			if main.Path == root {
				removedMains = append(removedMains, main)
				continue
			}
			kept = append(kept, main)
		}
		reg.Mains = kept
	}
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

func (a *App) mainWithoutConfig(worktree string) string {
	if a.Locator == nil || a.Config == nil {
		return ""
	}
	if worktree == "" {
		worktree = "."
	}
	c, err := a.Locator.Detect(worktree)
	if err != nil || !c.IsMain {
		return ""
	}
	if _, _, err := a.loadConfigFor(c); err != nil && errors.Is(err, fs.ErrNotExist) {
		return c.Root
	}
	return ""
}
