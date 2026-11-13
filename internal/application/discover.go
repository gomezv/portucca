package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/platform"
	"github.com/gomezv/portucca/internal/project"
)

type DiscoveredWorktree struct {
	Project  domain.Project `json:"project"`
	Path     string         `json:"path"`
	Branch   string         `json:"branch,omitempty"`
	MainRoot string         `json:"mainRoot"`
	Detached bool           `json:"detached,omitempty"`
	Locked   bool           `json:"locked,omitempty"`
}

type DiscoveryIssue struct {
	Project  domain.Project `json:"project"`
	MainRoot string         `json:"mainRoot"`
	Code     string         `json:"code"`
	Message  string         `json:"message"`
}

type DiscoveryResult struct {
	Worktrees []DiscoveredWorktree `json:"worktrees"`
	Issues    []DiscoveryIssue     `json:"issues"`
}

func (a *App) DiscoverWorktrees(ctx context.Context, projectFilter string) (DiscoveryResult, error) {
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), true)
	if err != nil {
		return DiscoveryResult{}, mapLockErr(err)
	}
	defer unlock()
	reg, err := a.Registry.Load()
	if err != nil {
		return DiscoveryResult{}, mapRegistryErr(err)
	}
	claimed := map[string]bool{}
	for _, claim := range reg.Worktrees {
		claimed[canonicalPath(claim.Path)] = true
	}
	mains := make([]domain.MainReservation, 0, len(reg.Mains))
	for _, main := range reg.Mains {
		if projectFilter != "" && string(main.Project) != projectFilter {
			continue
		}
		mains = append(mains, main)
	}
	sort.Slice(mains, func(i, j int) bool {
		if mains[i].Project != mains[j].Project {
			return mains[i].Project < mains[j].Project
		}
		return mains[i].Path < mains[j].Path
	})

	result := DiscoveryResult{Worktrees: []DiscoveredWorktree{}, Issues: []DiscoveryIssue{}}
	seen := map[string]bool{}
	for _, main := range mains {
		if _, err := os.Stat(main.Path); err != nil {
			result.Issues = append(result.Issues, DiscoveryIssue{
				Project: main.Project, MainRoot: main.Path, Code: "not_a_repo",
				Message: "main checkout is missing or unreadable",
			})
			continue
		}
		worktrees, err := a.lister().ListWorktrees(main.Path)
		if err != nil {
			code := "not_a_repo"
			if errors.Is(err, project.ErrGitMissing) {
				code = "git_missing"
			}
			result.Issues = append(result.Issues, DiscoveryIssue{
				Project: main.Project, MainRoot: main.Path, Code: code, Message: err.Error(),
			})
			continue
		}
		mainPath := canonicalPath(main.Path)
		for _, wt := range worktrees {
			if wt.Prunable {
				continue
			}
			if _, err := os.Stat(wt.Path); err != nil {
				continue
			}
			path := canonicalPath(wt.Path)
			if path == mainPath || claimed[path] || seen[path] {
				continue
			}
			seen[path] = true
			result.Worktrees = append(result.Worktrees, DiscoveredWorktree{
				Project: main.Project, Path: path, Branch: wt.Branch,
				MainRoot: main.Path, Detached: wt.Detached, Locked: wt.Locked,
			})
		}
	}
	sort.Slice(result.Worktrees, func(i, j int) bool {
		if result.Worktrees[i].Project != result.Worktrees[j].Project {
			return result.Worktrees[i].Project < result.Worktrees[j].Project
		}
		return result.Worktrees[i].Path < result.Worktrees[j].Path
	})
	return result, nil
}

func canonicalPath(path string) string {
	if real, err := platform.RealPath(path); err == nil {
		return real
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}
