package application

import (
	"context"
	"strings"
	"testing"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

func TestEnvReturnsClaim(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
		Env: []domain.EnvTarget{{Service: "web", File: ".env.local", Key: "PORT"}},
	}
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 1,
		Worktrees: []domain.Claim{{
			Project: "app", Path: root,
			Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3005}},
		}},
	}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, &fakeEnv{})
	got, err := app.Env(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsMain || got.Services["web"].Port != 3005 {
		t.Fatalf("got = %+v", got)
	}
	if len(got.Env) != 1 || got.Env[0].Value != "3005" {
		t.Fatalf("env = %+v", got.Env)
	}
}

func TestEnvMainReturnsReservedPorts(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
		Env: []domain.EnvTarget{{Service: "web", File: ".env.local", Key: "PORT"}},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root, IsMain: true}}, cfg, reg, &fakeEnv{})
	got, err := app.Env(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsMain || got.Services["web"].Port != 3000 {
		t.Fatalf("got = %+v", got)
	}
	if len(got.Env) != 1 || got.Env[0].Value != "3000" {
		t.Fatalf("env = %+v", got.Env)
	}
	if reg.saves != 0 {
		t.Fatalf("env wrote the registry: saves=%d", reg.saves)
	}
}

func TestEnvWithoutClaimReturnsNoClaim(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, &fakeEnv{})
	_, err := app.Env(context.Background(), root)
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "no_claim" {
		t.Fatalf("err = %v, want no_claim", err)
	}
	if !strings.Contains(ae.Hint, "ensure") {
		t.Fatalf("hint = %q, want the ensure hint", ae.Hint)
	}
}
