package cli

import (
	"context"
	"testing"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/tui"
)

type fakeTUIApp struct {
	ensure     application.EnsureResult
	setup      application.SetupResult
	reallocate application.EnsureResult
	release    application.ReleaseResult
	prune      application.PruneResult
	err        error

	worktrees []string
	service   domain.ServiceName
}

func (f *fakeTUIApp) Ensure(_ context.Context, worktree string) (application.EnsureResult, error) {
	f.worktrees = append(f.worktrees, worktree)
	return f.ensure, f.err
}

func (f *fakeTUIApp) Setup(_ context.Context, worktree string) (application.SetupResult, error) {
	f.worktrees = append(f.worktrees, worktree)
	return f.setup, f.err
}

func (f *fakeTUIApp) Reallocate(_ context.Context, worktree string) (application.EnsureResult, error) {
	f.worktrees = append(f.worktrees, worktree)
	return f.reallocate, f.err
}

func (f *fakeTUIApp) Release(_ context.Context, worktree string, service domain.ServiceName) (application.ReleaseResult, error) {
	f.worktrees = append(f.worktrees, worktree)
	f.service = service
	return f.release, f.err
}

func (f *fakeTUIApp) Prune(_ context.Context, worktree string) (application.PruneResult, error) {
	f.worktrees = append(f.worktrees, worktree)
	return f.prune, f.err
}

func TestTUIActionsSummaries(t *testing.T) {
	services := map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}}
	cases := []struct {
		name   string
		action tui.Action
		app    *fakeTUIApp
		want   string
	}{
		{
			name: "ensure", action: tui.ActionEnsure,
			app:  &fakeTUIApp{ensure: application.EnsureResult{Changed: true, Services: services}},
			want: "ensure: changed=true services=1",
		},
		{
			name: "setup", action: tui.ActionSetup,
			app: &fakeTUIApp{setup: application.SetupResult{
				EnsureResult: application.EnsureResult{Services: services},
				Bootstrap:    application.BootstrapResult{Copied: []string{"a"}, Symlinked: []string{"b", "c"}},
			}},
			want: "setup: copied=1 symlinked=2 skipped=0 services=1",
		},
		{
			name: "reallocate", action: tui.ActionReallocate,
			app:  &fakeTUIApp{reallocate: application.EnsureResult{Changed: true, Services: services}},
			want: "reallocate: services=1 changed=true",
		},
		{
			name: "release", action: tui.ActionRelease,
			app:  &fakeTUIApp{release: application.ReleaseResult{Released: map[domain.ServiceName]domain.Port{"web": 3001}}},
			want: "release: released=1",
		},
		{
			name: "prune", action: tui.ActionPrune,
			app: &fakeTUIApp{prune: application.PruneResult{
				Removed:      []domain.Claim{{Project: "app"}},
				RemovedMains: []domain.MainReservation{{Project: "app"}},
			}},
			want: "prune: removed=1 mains=1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tuiActions{app: tc.app}.Run(context.Background(), tc.action, "/repo/worktree")
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("summary = %q, want %q", got, tc.want)
			}
			if len(tc.app.worktrees) != 1 || tc.app.worktrees[0] != "/repo/worktree" {
				t.Fatalf("worktrees = %+v", tc.app.worktrees)
			}
		})
	}
}

func TestTUIActionsReleaseWholeClaim(t *testing.T) {
	app := &fakeTUIApp{}
	if _, err := (tuiActions{app: app}).Run(context.Background(), tui.ActionRelease, "/repo/worktree"); err != nil {
		t.Fatal(err)
	}
	if app.service != "" {
		t.Fatalf("release service = %q, want the whole claim", app.service)
	}
	if len(app.worktrees) != 1 || app.worktrees[0] != "/repo/worktree" {
		t.Fatalf("release calls = %+v, want one call", app.worktrees)
	}
}

func TestTUIActionsKeepsError(t *testing.T) {
	want := apperr.New("no_claim", "this worktree has no claim", "run portucca ensure first")
	app := &fakeTUIApp{err: want}
	_, err := (tuiActions{app: app}).Run(context.Background(), tui.ActionEnsure, "/repo/worktree")
	ae, ok := apperr.As(err)
	if !ok || ae != want {
		t.Fatalf("err = %v, want the original apperr", err)
	}
}

func TestNewTUIActionsClearsConfigFile(t *testing.T) {
	runner := newTUIActions(&application.App{ConfigFile: "/repo/alt.json"})
	actions, ok := runner.(tuiActions)
	if !ok {
		t.Fatalf("runner = %T", runner)
	}
	app, ok := actions.app.(*application.App)
	if !ok {
		t.Fatalf("app = %T", actions.app)
	}
	if app.ConfigFile != "" {
		t.Fatalf("ConfigFile = %q, want empty", app.ConfigFile)
	}
}
