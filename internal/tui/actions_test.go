package tui

import (
	"slices"
	"testing"

	"github.com/gomezv/portucca/internal/application"
)

func TestActionsFor(t *testing.T) {
	main := application.OverviewWorktree{Kind: "main", Live: true}
	wt := application.OverviewWorktree{Kind: "worktree", Live: true}
	dead := application.OverviewWorktree{Kind: "main", Live: false}
	missing := application.OverviewWorktree{Kind: "worktree", Live: true, Issues: []application.Issue{{Code: "config_missing"}}}
	invalid := application.OverviewWorktree{Kind: "main", Live: true, Issues: []application.Issue{{Code: "config_invalid"}}}

	if got, want := actionsFor(main), []Action{ActionEnsure, ActionSetup, ActionPrune, ActionRun, ActionWrap}; !slices.Equal(got, want) {
		t.Fatalf("main actions = %v, want %v", got, want)
	}
	if got, want := actionsFor(wt), []Action{ActionEnsure, ActionSetup, ActionReallocate, ActionRelease, ActionPrune, ActionRun, ActionWrap}; !slices.Equal(got, want) {
		t.Fatalf("worktree actions = %v, want %v", got, want)
	}
	if got, want := actionsFor(dead), []Action{ActionPrune}; !slices.Equal(got, want) {
		t.Fatalf("dead actions = %v, want %v", got, want)
	}
	if got, want := actionsFor(missing), []Action{ActionPrune}; !slices.Equal(got, want) {
		t.Fatalf("missing-config actions = %v, want %v", got, want)
	}
	if got, want := actionsFor(invalid), []Action{ActionPrune}; !slices.Equal(got, want) {
		t.Fatalf("invalid-config actions = %v, want %v", got, want)
	}
}

func TestActionConfirm(t *testing.T) {
	for _, action := range []Action{ActionSetup, ActionReallocate, ActionRelease, ActionPrune} {
		if !actionNeedsConfirm(action) {
			t.Fatalf("%s needs confirmation", action)
		}
	}
	for _, action := range []Action{ActionEnsure, ActionRun, ActionWrap} {
		if actionNeedsConfirm(action) {
			t.Fatalf("%s must not ask for confirmation", action)
		}
	}
}
