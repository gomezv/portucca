package tui

import (
	"context"
	"fmt"

	"github.com/gomezv/portucca/internal/application"
)

type Action string

const (
	ActionEnsure     Action = "ensure"
	ActionSetup      Action = "setup"
	ActionReallocate Action = "reallocate"
	ActionRelease    Action = "release"
	ActionPrune      Action = "prune"
)

type ActionRunner interface {
	Run(ctx context.Context, action Action, worktree string) (string, error)
}

type RefreshFunc func(context.Context) (application.OverviewResult, error)

func actionsFor(w application.OverviewWorktree) []Action {
	if !w.Live {
		return []Action{ActionPrune}
	}
	if w.Kind == "unregistered" {
		return []Action{ActionSetup}
	}
	if hasConfigIssue(w) {
		return []Action{ActionPrune}
	}
	switch w.Kind {
	case "main":
		return []Action{ActionEnsure, ActionSetup, ActionPrune}
	case "worktree":
		return []Action{ActionEnsure, ActionSetup, ActionReallocate, ActionRelease, ActionPrune}
	default:
		return []Action{ActionPrune}
	}
}

func hasConfigIssue(w application.OverviewWorktree) bool {
	for _, issue := range w.Issues {
		if issue.Code == "config_missing" || issue.Code == "config_invalid" {
			return true
		}
	}
	return false
}

func actionNeedsConfirm(action Action) bool {
	return action != ActionEnsure
}

func actionLabel(action Action) string {
	return string(action)
}

func actionDescription(action Action) string {
	switch action {
	case ActionEnsure:
		return "reuse or assign ports and write env"
	case ActionSetup:
		return "bootstrap the worktree and run ensure"
	case ActionReallocate:
		return "assign new ports (changes URLs)"
	case ActionRelease:
		return "release the whole claim"
	case ActionPrune:
		return "remove stale claims and main reservations"
	default:
		return ""
	}
}

func actionConfirmText(action Action, path string) string {
	switch action {
	case ActionSetup:
		return fmt.Sprintf("Run setup on %s? It copies bootstrap files and assigns ports.", path)
	case ActionReallocate:
		return fmt.Sprintf("Reallocate %s? The URLs will change.", path)
	case ActionRelease:
		return fmt.Sprintf("Release the claim for %s? Its ports become available to other worktrees.", path)
	case ActionPrune:
		return fmt.Sprintf("Prune stale entries using %s as the current checkout? Claims and main reservations may be removed.", path)
	default:
		return ""
	}
}
