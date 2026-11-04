package application

import (
	"context"
	"testing"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

type countingConfig struct {
	fakeConfig
	loads int
}

func (c *countingConfig) LoadRoot(root string) (domain.Config, string, error) {
	c.loads++
	return c.fakeConfig.LoadRoot(root)
}

func (c *countingConfig) LoadFile(path string) (domain.Config, error) {
	c.loads++
	return c.fakeConfig.LoadFile(path)
}

func TestSetupMainLoadsConfigOnce(t *testing.T) {
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	loader := &countingConfig{fakeConfig: fakeConfig{cfg: cfg}}
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: "/repo", MainRoot: "/repo", IsMain: true}},
		Config:   loader,
		Registry: &fakeRegistry{reg: domain.Registry{Version: 1}},
		Locker:   fakeLocker{},
		Prober:   fakeProber{},
		EnvStore: &fakeEnv{},
	}
	if _, err := app.Setup(context.Background(), "/repo"); err != nil {
		t.Fatal(err)
	}
	if loader.loads != 1 {
		t.Fatalf("config loads = %d, want 1", loader.loads)
	}
}

func TestPrepareRunLoadsConfigOnce(t *testing.T) {
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
		Env: []domain.EnvTarget{{Service: "web", File: ".env.local", Key: "PORT"}},
	}
	loader := &countingConfig{fakeConfig: fakeConfig{cfg: cfg}}
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: "/wt", MainRoot: "/repo"}},
		Config:   loader,
		Registry: &fakeRegistry{reg: domain.Registry{Version: 1}},
		Locker:   fakeLocker{},
		Prober:   fakeProber{},
		EnvStore: &fakeEnv{},
	}
	if _, err := app.PrepareRun(context.Background(), RunRequest{
		Worktree: "/wt",
		Command:  []string{"sh", "-c", "true"},
	}); err != nil {
		t.Fatal(err)
	}
	if loader.loads != 1 {
		t.Fatalf("config loads = %d, want 1", loader.loads)
	}
}
