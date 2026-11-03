package application

import (
	"context"
	"reflect"
	"testing"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

func runEnvFixture() (domain.Config, map[domain.ServiceName]domain.ServiceClaim) {
	cfg := domain.Config{Version: 1, Project: "app", Env: []domain.EnvTarget{
		{Service: "web", File: ".env", Key: "PORT"},
		{Service: "web", File: ".env", Key: "URL", Template: "http://localhost:{{port}}"},
		{Service: "api", File: ".env", Key: "API_PORT"},
	}}
	services := map[domain.ServiceName]domain.ServiceClaim{
		"web": {Port: 3002},
		"api": {Port: 3004},
	}
	return cfg, services
}

func TestRunEnvValuesAllServices(t *testing.T) {
	cfg, services := runEnvFixture()
	got, err := runEnvValues(cfg, services, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"API_PORT=3004", "PORT=3002", "URL=http://localhost:3002"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRunEnvValuesServiceFilter(t *testing.T) {
	cfg, services := runEnvFixture()
	got, err := runEnvValues(cfg, services, "web")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"PORT=3002", "URL=http://localhost:3002"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRunEnvValuesConflict(t *testing.T) {
	cfg := domain.Config{Version: 1, Project: "app", Env: []domain.EnvTarget{
		{Service: "web", File: ".env", Key: "PORT"},
		{Service: "api", File: ".env", Key: "PORT"},
	}}
	services := map[domain.ServiceName]domain.ServiceClaim{
		"web": {Port: 3002},
		"api": {Port: 3004},
	}
	_, err := runEnvValues(cfg, services, "")
	if e, ok := apperr.As(err); !ok || e.Code != "env_conflict" {
		t.Fatalf("err = %v, want env_conflict", err)
	}
}

func TestMergeEnvOverrides(t *testing.T) {
	got := mergeEnv([]string{"PORT=1", "KEEP=yes"}, []string{"PORT=2"})
	want := []string{"PORT=2", "KEEP=yes"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPrepareRunCommandNotFound(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{Version: 1, Project: "app",
		Services: map[domain.ServiceName]domain.Service{"web": {MainPort: 3000, StartPort: 3001}}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root}}, cfg, &fakeRegistry{}, &fakeEnv{})
	_, err := app.PrepareRun(context.Background(), RunRequest{
		Worktree: root,
		Command:  []string{"definitely-not-a-command-xyz"},
	})
	e, ok := apperr.As(err)
	if !ok || e.Code != "command_not_found" {
		t.Fatalf("err = %v, want command_not_found", err)
	}
}
