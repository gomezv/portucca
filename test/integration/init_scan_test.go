package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/config"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/envfile"
)

func writeScanFile(t *testing.T, root, rel, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scanRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q")
	writeScanFile(t, root, "apps/web/package.json", `{"name":"@app/web","scripts":{"dev":"vite --port 5173"}}`)
	writeScanFile(t, root, "apps/web/.env.local", "API_URL=http://localhost:3004\n")
	writeScanFile(t, root, "apps/api/package.json", `{"name":"@app/api","scripts":{"dev":"nest start"}}`)
	writeScanFile(t, root, "apps/api/.env", "PORT=3004\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-qm", "init")
	return root
}

func TestInitScanGeneratesUsableConfig(t *testing.T) {
	main := scanRepo(t)
	app := newApp(t, filepath.Join(t.TempDir(), "ports.json"))
	ctx := context.Background()

	got, err := app.Init(ctx, application.InitRequest{Scan: true, Worktree: main})
	if err != nil {
		t.Fatal(err)
	}
	if got.Project == "" {
		t.Fatal("project is empty")
	}
	cfg, err := config.LoadFile(filepath.Join(main, "portucca.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Services["web"].MainPort != 5173 || cfg.Services["api"].MainPort != 3004 {
		t.Fatalf("services = %+v", cfg.Services)
	}
	if cfg.Services["web"].StartPort != 5174 || cfg.Services["api"].StartPort != 3005 {
		t.Fatalf("startPorts = %+v", cfg.Services)
	}
	if len(cfg.Bootstrap.CopyFromMain) != 2 {
		t.Fatalf("bootstrap = %+v, want both env files", cfg.Bootstrap)
	}

	git(t, main, "add", "portucca.json")
	git(t, main, "commit", "-qm", "config")
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, main, "worktree", "add", "-q", wt, "-b", "feature")
	setup, err := app.Setup(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	wantAPI := fmt.Sprintf("%d", setup.Services["api"].Port)
	apiPort, ok, err := envfile.ReadValue(wt, domain.EnvRef{File: "apps/api/.env", Key: "PORT"})
	if err != nil || !ok || apiPort != wantAPI {
		t.Fatalf("api PORT=%q ok=%v err=%v, want %s", apiPort, ok, err, wantAPI)
	}
	apiURL, ok, err := envfile.ReadValue(wt, domain.EnvRef{File: "apps/web/.env.local", Key: "API_URL"})
	if err != nil || !ok || apiURL != "http://localhost:"+wantAPI {
		t.Fatalf("API_URL=%q ok=%v err=%v, want http://localhost:%s", apiURL, ok, err, wantAPI)
	}
}

func TestInitScanDeterministic(t *testing.T) {
	main := scanRepo(t)
	app := newApp(t, filepath.Join(t.TempDir(), "ports.json"))
	ctx := context.Background()
	if _, err := app.Init(ctx, application.InitRequest{Scan: true, Worktree: main}); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(main, "portucca.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Init(ctx, application.InitRequest{Scan: true, Force: true, Worktree: main}); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(main, "portucca.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("config changed between runs:\n%s\n---\n%s", first, second)
	}
}

func TestInitScanIncompleteWritesNothing(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-q")
	app := newApp(t, filepath.Join(t.TempDir(), "ports.json"))
	_, err := app.Init(context.Background(), application.InitRequest{Scan: true, Worktree: main})
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "scan_incomplete" {
		t.Fatalf("err = %v, want scan_incomplete", err)
	}
	if _, err := os.Stat(filepath.Join(main, "portucca.json")); !os.IsNotExist(err) {
		t.Fatalf("config was written: %v", err)
	}
}

func TestInitScanUsesComposeYAML(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-q")
	writeScanFile(t, main, "apps/api/package.json", `{"name":"@app/api","scripts":{"dev":"nest start"}}`)
	writeScanFile(t, main, "compose.yaml", "services:\n  api:\n    ports:\n      - \"4000:4000\"\n")
	git(t, main, "add", ".")
	git(t, main, "commit", "-qm", "init")

	app := newApp(t, filepath.Join(t.TempDir(), "ports.json"))
	got, err := app.Init(context.Background(), application.InitRequest{Scan: true, Worktree: main})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(filepath.Join(main, "portucca.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Services["api"].MainPort != 4000 {
		t.Fatalf("api mainPort = %d, want 4000", cfg.Services["api"].MainPort)
	}
	if got.Scan == nil {
		t.Fatal("scan result is nil")
	}
	found := false
	for _, source := range got.Scan.Sources {
		if source.Field == "services.api.mainPort" && source.Value == "4000" && source.Source == "compose.yaml:services.api.ports" {
			found = true
		}
	}
	if !found {
		t.Fatalf("sources = %+v, want compose.yaml mainPort source", got.Scan.Sources)
	}
}

func TestInitScanDiscoversComposeServices(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-q")
	writeScanFile(t, main, "compose.yaml", "services:\n  web:\n    ports:\n      - \"4000:4000\"\n  db:\n    ports:\n      - \"5432:5432\"\n")
	git(t, main, "add", ".")
	git(t, main, "commit", "-qm", "init")

	app := newApp(t, filepath.Join(t.TempDir(), "ports.json"))
	got, err := app.Init(context.Background(), application.InitRequest{
		Scan: true, Compose: true, ExcludeServices: []string{"db"}, Worktree: main,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(filepath.Join(main, "portucca.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("services = %+v, want only web", cfg.Services)
	}
	web, ok := cfg.Services["web"]
	if !ok || web.MainPort != 4000 || web.StartPort != 4001 {
		t.Fatalf("services = %+v, want web 4000/4001", cfg.Services)
	}
	if _, ok := cfg.Services["db"]; ok {
		t.Fatalf("services = %+v, want db excluded", cfg.Services)
	}
	if got.Scan == nil {
		t.Fatal("scan result is nil")
	}
	foundName := false
	foundPort := false
	for _, source := range got.Scan.Sources {
		if source.Field == "services.web.name" && source.Value == "web" && source.Source == "compose.yaml:services.web.name" {
			foundName = true
		}
		if source.Field == "services.web.mainPort" && source.Value == "4000" && source.Source == "compose.yaml:services.web.ports" {
			foundPort = true
		}
	}
	if !foundName || !foundPort {
		t.Fatalf("sources = %+v, want compose name and port sources", got.Scan.Sources)
	}
}
