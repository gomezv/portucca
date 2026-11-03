package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomezv/portucca/internal/config"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

func writeFallbackConfig(t *testing.T, dir, name string, port domain.Port) string {
	t.Helper()
	cfg := domain.Config{
		Version: domain.ConfigVersion,
		Project: domain.Project(name),
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: port, StartPort: port + 1},
		},
	}
	path := filepath.Join(dir, config.FileName)
	if err := config.WriteFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigForFallsBackToMain(t *testing.T) {
	main := t.TempDir()
	worktree := t.TempDir()
	mainPath := writeFallbackConfig(t, main, "shared", 18080)
	app := &App{Config: config.Loader{}}
	c := project.Context{Root: worktree, MainRoot: main}

	cfg, path, err := app.loadConfigFor(c)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Project != "shared" || path != mainPath {
		t.Fatalf("project=%q path=%q, want shared at %s", cfg.Project, path, mainPath)
	}

	worktreePath := writeFallbackConfig(t, worktree, "local", 19080)
	cfg, path, err = app.loadConfigFor(c)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Project != "local" || path != worktreePath {
		t.Fatalf("project=%q path=%q, want local at %s", cfg.Project, path, worktreePath)
	}

	app.ConfigFile = mainPath
	if _, _, err := app.loadConfigFor(c); err == nil || !strings.Contains(err.Error(), "config_invalid") {
		t.Fatalf("explicit config outside root: err=%v, want config_invalid", err)
	}

	app.ConfigFile = ""
	if err := os.Remove(worktreePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(mainPath); err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.loadConfigFor(c); err == nil || !strings.Contains(err.Error(), "config_missing") {
		t.Fatalf("missing config: err=%v, want config_missing", err)
	}
}
