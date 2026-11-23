package application

import (
	"context"
	"testing"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

func TestReallocateKeepsWorktreeID(t *testing.T) {
	root := t.TempDir()
	cfg := worktreeIDConfig()
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 2,
		Worktrees: []domain.Claim{{
			Project: "app", Path: root,
			Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}},
		}},
		Mains: []domain.MainReservation{},
	}}
	want := domain.WorktreeID("app", root)
	env := &fakeEnv{files: map[string]map[string]string{".env": {"DATABASE_NAME": want}}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: "/repo", IsMain: false}}, cfg, reg, env)
	got, err := app.Reallocate(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Services["web"].Port == 3001 {
		t.Fatalf("port = %d, want a new port", got.Services["web"].Port)
	}
	if env.files[".env"]["DATABASE_NAME"] != want {
		t.Fatalf("DATABASE_NAME = %q, want %q", env.files[".env"]["DATABASE_NAME"], want)
	}
}
