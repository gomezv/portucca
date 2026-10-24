package application

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

func TestDoctorDetectsMainPortClaimed(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
			"api": {MainPort: 3004, StartPort: 3005},
		},
		Env: []domain.EnvTarget{{Service: "web", File: ".env.local", Key: "PORT"}},
	}
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 1,
		Worktrees: []domain.Claim{{
			Project: "app", Path: root,
			Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3004}},
		}},
	}}
	env := &fakeEnv{values: map[string]string{"PORT": "3004"}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, env)
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got.OK {
		t.Fatal("Doctor reported OK, want errors")
	}
	found := false
	for _, issue := range got.Errors {
		if issue.Code == "main_port_claimed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors = %+v, want main_port_claimed", got.Errors)
	}
}

func TestDoctorReportsStaleClaimWithoutWriting(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(t.TempDir(), "gone")
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	reg := &fakeRegistry{reg: domain.Registry{
		Version:   1,
		Worktrees: []domain.Claim{{Project: "app", Path: missing}},
	}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, &fakeEnv{})
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got.OK {
		t.Fatal("Doctor reported OK, want stale_claim error")
	}
	if len(got.Claims) != 1 || got.Claims[0].Live {
		t.Fatalf("claims = %+v", got.Claims)
	}
	if got.Registry.Claims != 1 || got.Registry.Version != 1 {
		t.Fatalf("registry = %+v", got.Registry)
	}
	if reg.saves != 0 {
		t.Fatalf("doctor wrote the registry: saves=%d", reg.saves)
	}
}

func TestDoctorReportsMissingConfigAndKeepsRegistry(t *testing.T) {
	root := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{
		Version:   1,
		Worktrees: []domain.Claim{{Project: "app", Path: root}},
	}}
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: root, MainRoot: root}},
		Config:   fakeConfig{err: errors.New("config_missing: no portucca.json")},
		Registry: reg,
		Locker:   fakeLocker{},
		Prober:   fakeProber{},
		Env:      &fakeEnv{},
	}
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got.OK {
		t.Fatal("Doctor reported OK, want config_missing error")
	}
	found := false
	for _, issue := range got.Errors {
		if issue.Code == "config_missing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors = %+v, want config_missing", got.Errors)
	}
	if got.Registry.Claims != 1 || got.Registry.Version != 1 || len(got.Claims) != 1 {
		t.Fatalf("registry = %+v claims = %+v", got.Registry, got.Claims)
	}
	if reg.saves != 0 {
		t.Fatalf("doctor wrote the registry: saves=%d", reg.saves)
	}
}
