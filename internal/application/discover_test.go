package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

type fakeLister struct {
	byRoot map[string][]project.Worktree
	err    error
}

func (f fakeLister) ListWorktrees(root string) ([]project.Worktree, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byRoot[root], nil
}

func TestDiscoverWorktrees(t *testing.T) {
	main := t.TempDir()
	claimed := t.TempDir()
	fresh := t.TempDir()
	otherMain := t.TempDir()
	otherFresh := t.TempDir()
	missing := filepath.Join(t.TempDir(), "gone")

	reg := &fakeRegistry{reg: domain.Registry{
		Version: 2,
		Worktrees: []domain.Claim{{
			Project: "app", Path: claimed,
			Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}},
		}},
		Mains: []domain.MainReservation{
			{Project: "other", Path: otherMain, Ports: map[domain.ServiceName]domain.Port{"web": 4000}},
			{Project: "app", Path: main, Ports: map[domain.ServiceName]domain.Port{"web": 3000}},
		},
	}}
	lister := fakeLister{byRoot: map[string][]project.Worktree{
		main: {
			{Path: main, Branch: "main"},
			{Path: claimed, Branch: "claimed"},
			{Path: fresh, Branch: "fresh"},
			{Path: missing, Branch: "gone"},
			{Path: t.TempDir(), Branch: "prunable", Prunable: true},
		},
		otherMain: {{Path: otherMain, Branch: "main"}, {Path: otherFresh, Branch: "other"}},
	}}
	app := &App{Registry: reg, Locker: fakeLocker{}, Worktrees: lister}

	got, err := app.DiscoverWorktrees(context.Background(), "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Worktrees) != 1 || got.Worktrees[0].Path != canonicalPath(fresh) {
		t.Fatalf("worktrees = %+v, want only the fresh app worktree", got.Worktrees)
	}
	if got.Worktrees[0].Project != "app" || got.Worktrees[0].MainRoot != main {
		t.Fatalf("worktree = %+v", got.Worktrees[0])
	}

	all, err := app.DiscoverWorktrees(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Worktrees) != 2 || all.Worktrees[0].Project != "app" || all.Worktrees[1].Project != "other" {
		t.Fatalf("all worktrees = %+v", all.Worktrees)
	}
}

func TestDiscoverWorktreesReportsIssues(t *testing.T) {
	main := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 2,
		Mains:   []domain.MainReservation{{Project: "app", Path: main, Ports: map[domain.ServiceName]domain.Port{"web": 3000}}},
	}}
	app := &App{Registry: reg, Locker: fakeLocker{}, Worktrees: fakeLister{err: project.ErrGitMissing}}
	got, err := app.DiscoverWorktrees(context.Background(), "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Worktrees) != 0 || len(got.Issues) != 1 || got.Issues[0].Code != "git_missing" {
		t.Fatalf("result = %+v", got)
	}
}

func TestDiscoverWorktreesMissingMain(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 2,
		Mains:   []domain.MainReservation{{Project: "app", Path: missing, Ports: map[domain.ServiceName]domain.Port{"web": 3000}}},
	}}
	app := &App{Registry: reg, Locker: fakeLocker{}, Worktrees: fakeLister{}}
	got, err := app.DiscoverWorktrees(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Issues) != 1 || got.Issues[0].Code != "not_a_repo" {
		t.Fatalf("result = %+v", got)
	}
	if _, err := os.Stat(missing); err == nil {
		t.Fatal("fixture unexpectedly exists")
	}
}
