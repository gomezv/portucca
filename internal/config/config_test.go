package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomezv/portucca/internal/domain"
)

func TestLoadRootRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portucca.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"project":"app","services":{"web":{"mainPort":8080,"startPort":8081}},"extra":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadRoot(dir)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %v, want unknown field", err)
	}
}

func TestLoadRootValid(t *testing.T) {
	dir := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 8080, StartPort: 8081},
		},
		Env: []domain.EnvTarget{{Service: "web", File: ".env.local", Key: "PORT"}},
	}
	if err := WriteFile(filepath.Join(dir, "portucca.json"), cfg); err != nil {
		t.Fatal(err)
	}
	got, path, err := LoadRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "portucca.json") {
		t.Fatalf("path = %q", path)
	}
	if got.Project != "app" || got.Services["web"].StartPort != 8081 {
		t.Fatalf("config = %+v", got)
	}
}

func TestLoadFileReportsConfigVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portucca.json")
	data := []byte(`{"version":2,"project":"app","services":{"web":{"mainPort":8080,"startPort":8081}}}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "config_version") {
		t.Fatalf("LoadFile error = %v, want config_version", err)
	}

	_, _, err = LoadRoot(dir)
	if err == nil || !strings.Contains(err.Error(), "config_version") {
		t.Fatalf("LoadRoot error = %v, want config_version", err)
	}
}

func TestApplyPresetNodeViteNestRequiresWebAndAPI(t *testing.T) {
	services := map[domain.ServiceName]domain.Service{
		"web": {MainPort: 5173, StartPort: 5174},
	}
	if _, err := ApplyPreset("node-vite-nest", "app", services); err == nil {
		t.Fatal("want an error for a missing api service")
	}
}

func TestApplyPresetNodeViteNest(t *testing.T) {
	services := map[domain.ServiceName]domain.Service{
		"web": {MainPort: 5173, StartPort: 5174},
		"api": {MainPort: 3004, StartPort: 3005},
	}
	cfg, err := ApplyPreset("node-vite-nest", "app", services)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Env) != 3 || len(cfg.Bootstrap.Symlinks) != 1 {
		t.Fatalf("env=%d symlinks=%d", len(cfg.Env), len(cfg.Bootstrap.Symlinks))
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPresetNext(t *testing.T) {
	services := map[domain.ServiceName]domain.Service{
		"web": {MainPort: 3000, StartPort: 3001},
	}
	cfg, err := ApplyPreset("next", "app", services)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Env) != 2 || len(cfg.Bootstrap.CopyFromMain) != 1 || len(cfg.Bootstrap.Symlinks) != 0 {
		t.Fatalf("env=%d copy=%d symlinks=%d", len(cfg.Env), len(cfg.Bootstrap.CopyFromMain), len(cfg.Bootstrap.Symlinks))
	}
	if cfg.Env[0].File != ".env" || cfg.Env[0].Key != "PORT" {
		t.Fatalf("env[0] = %+v", cfg.Env[0])
	}
	if cfg.Env[1].Key != "NEXT_PUBLIC_URL" || cfg.Env[1].Template != "http://localhost:{{port}}" {
		t.Fatalf("env[1] = %+v", cfg.Env[1])
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPresetNextRequiresOnlyWeb(t *testing.T) {
	if _, err := ApplyPreset("next", "app", map[domain.ServiceName]domain.Service{}); err == nil {
		t.Fatal("want an error for a missing web service")
	}
	services := map[domain.ServiceName]domain.Service{
		"web": {MainPort: 3000, StartPort: 3001},
		"api": {MainPort: 3004, StartPort: 3005},
	}
	if _, err := ApplyPreset("next", "app", services); err == nil {
		t.Fatal("want an error for an extra api service")
	}
}
