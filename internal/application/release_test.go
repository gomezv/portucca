package application

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

func releaseApp(t *testing.T, root string, isMain bool, reg *fakeRegistry) (*App, *fakeEnv) {
	t.Helper()
	env := &fakeEnv{}
	cfg := domain.Config{Version: 1, Project: "app", Services: map[domain.ServiceName]domain.Service{
		"web": {MainPort: 3000, StartPort: 3001},
	}}
	return testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root, IsMain: isMain}}, cfg, reg, env), env
}

func TestReleaseWholeClaim(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 2,
		Worktrees: []domain.Claim{
			{Project: "app", Path: root, Services: map[domain.ServiceName]domain.ServiceClaim{
				"web": {Port: 3001}, "api": {Port: 3005},
			}},
			{Project: "other", Path: other, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 4001}}},
		},
		Mains: []domain.MainReservation{{Project: "other", Path: other, Ports: map[domain.ServiceName]domain.Port{"web": 4000}}},
	}}
	app, env := releaseApp(t, root, false, reg)
	got, err := app.Release(context.Background(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Released) != 2 || got.Released["web"] != 3001 || got.Released["api"] != 3005 {
		t.Fatalf("released = %+v", got.Released)
	}
	if got.Project != "app" || got.WorktreePath != root {
		t.Fatalf("result = %+v", got)
	}
	if len(reg.reg.Worktrees) != 1 || reg.reg.Worktrees[0].Path != other || len(reg.reg.Mains) != 1 {
		t.Fatalf("registry = %+v", reg.reg)
	}
	if len(env.files) != 0 {
		t.Fatalf("release wrote env: %+v", env.files)
	}
}

func TestReleaseOneService(t *testing.T) {
	root := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{{
		Project: "app", Path: root,
		Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}, "api": {Port: 3005}},
	}}}}
	app, _ := releaseApp(t, root, false, reg)
	got, err := app.Release(context.Background(), root, "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Released) != 1 || got.Released["web"] != 3001 {
		t.Fatalf("released = %+v", got.Released)
	}
	if len(reg.reg.Worktrees) != 1 || len(reg.reg.Worktrees[0].Services) != 1 || reg.reg.Worktrees[0].Services["api"].Port != 3005 {
		t.Fatalf("claim = %+v", reg.reg.Worktrees)
	}
}

func TestReleaseLastServiceRemovesClaim(t *testing.T) {
	root := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{{
		Project: "app", Path: root,
		Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}},
	}}}}
	app, _ := releaseApp(t, root, false, reg)
	if _, err := app.Release(context.Background(), root, "web"); err != nil {
		t.Fatal(err)
	}
	if len(reg.reg.Worktrees) != 0 {
		t.Fatalf("worktrees = %+v", reg.reg.Worktrees)
	}
}

func TestReleaseNoClaim(t *testing.T) {
	root := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{}, Mains: []domain.MainReservation{}}}
	app, _ := releaseApp(t, root, false, reg)
	_, err := app.Release(context.Background(), root, "")
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "no_claim" {
		t.Fatalf("err = %v, want no_claim", err)
	}
}

func TestReleaseMainReserved(t *testing.T) {
	root := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{}, Mains: []domain.MainReservation{}}}
	app, _ := releaseApp(t, root, true, reg)
	_, err := app.Release(context.Background(), root, "")
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "main_reserved" {
		t.Fatalf("err = %v, want main_reserved", err)
	}
}

func TestReleaseServiceNotClaimed(t *testing.T) {
	root := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{{
		Project: "app", Path: root,
		Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}},
	}}}}
	app, _ := releaseApp(t, root, false, reg)
	_, err := app.Release(context.Background(), root, "api")
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "service_not_claimed" {
		t.Fatalf("err = %v, want service_not_claimed", err)
	}
}

func TestReleaseDoesNotPrune(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(t.TempDir(), "gone")
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{
		{Project: "app", Path: root, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}}},
		{Project: "other", Path: missing, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3002}}},
	}}}
	app, _ := releaseApp(t, root, false, reg)
	if _, err := app.Release(context.Background(), root, ""); err != nil {
		t.Fatal(err)
	}
	if len(reg.reg.Worktrees) != 1 || reg.reg.Worktrees[0].Path != missing {
		t.Fatalf("worktrees = %+v, want stale claim untouched", reg.reg.Worktrees)
	}
}

func TestReleaseDoesNotNeedConfig(t *testing.T) {
	root := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{{
		Project: "app", Path: root,
		Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}},
	}}}}
	app, _ := releaseApp(t, root, false, reg)
	app.Config = fakeConfig{err: errors.New("config missing")}
	if _, err := app.Release(context.Background(), root, ""); err != nil {
		t.Fatal(err)
	}
	if len(reg.reg.Worktrees) != 0 {
		t.Fatalf("worktrees = %+v", reg.reg.Worktrees)
	}
}
