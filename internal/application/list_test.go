package application

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

func TestListFiltersByWorktreeProject(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 1,
		Worktrees: []domain.Claim{
			{Project: "app", Path: root, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}}},
			{Project: "other", Path: "/other", Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 4001}}},
		},
	}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, &fakeEnv{})
	got, err := app.List(context.Background(), root, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Claims) != 1 || got.Claims[0].Project != "app" {
		t.Fatalf("claims = %+v", got.Claims)
	}
	all, err := app.List(context.Background(), root, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Claims) != 2 {
		t.Fatalf("all claims = %+v", all.Claims)
	}
}

func TestListEmptyRegistryReturnsEmptyList(t *testing.T) {
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
	got, err := app.List(context.Background(), root, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Claims == nil || len(got.Claims) != 0 {
		t.Fatalf("claims = %#v, want empty non-nil slice", got.Claims)
	}
	if reg.saves != 0 {
		t.Fatalf("list wrote the registry: saves=%d", reg.saves)
	}
}

func TestListFiltersByProjectFlagAndReportsLive(t *testing.T) {
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
		Version: 1,
		Worktrees: []domain.Claim{
			{Project: "app", Path: root, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}}},
			{Project: "other", Path: missing, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 4001}}},
		},
	}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, &fakeEnv{})
	got, err := app.List(context.Background(), root, "other", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Claims) != 1 || got.Claims[0].Project != "other" {
		t.Fatalf("claims = %+v", got.Claims)
	}
	if got.Claims[0].Live {
		t.Fatalf("claims = %+v, want live=false for a missing path", got.Claims)
	}
}
