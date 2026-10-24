package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/config"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

func TestInitWritesValidConfig(t *testing.T) {
	dir := t.TempDir()
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir}},
		Config:   config.Loader{},
		Registry: &fakeRegistry{reg: domain.Registry{Version: 1}},
		Locker:   fakeLocker{},
	}
	got, err := app.Init(context.Background(), InitRequest{
		Project:  "app",
		Services: []string{"web:8080:8081"},
		Worktree: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "app" {
		t.Fatalf("got = %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "portucca.json")); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(filepath.Join(dir, "portucca.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Services["web"].StartPort != 8081 {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestInitRefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portucca.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir}},
		Config:   config.Loader{},
		Registry: &fakeRegistry{reg: domain.Registry{Version: 1}},
		Locker:   fakeLocker{},
	}
	_, err := app.Init(context.Background(), InitRequest{
		Project:  "app",
		Services: []string{"web:8080:8081"},
		Worktree: dir,
	})
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "config_exists" {
		t.Fatalf("err = %v, want config_exists", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "{}\n" {
		t.Fatalf("config was overwritten: %q err=%v", data, err)
	}
}

func TestInitConfigOutsideRootIsUsage(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "portucca.json")
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir}},
		Config:   config.Loader{},
		Registry: &fakeRegistry{reg: domain.Registry{Version: 1}},
		Locker:   fakeLocker{},
	}
	_, err := app.Init(context.Background(), InitRequest{
		Project:    "app",
		Services:   []string{"web:8080:8081"},
		Worktree:   dir,
		ConfigPath: outside,
	})
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "usage" {
		t.Fatalf("err = %v, want usage", err)
	}
}

func TestInitValidatesFlagsBeforeConfigExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portucca.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir}},
		Config:   config.Loader{},
		Registry: &fakeRegistry{reg: domain.Registry{Version: 1}},
		Locker:   fakeLocker{},
	}
	cases := []InitRequest{
		{Project: "app", Services: []string{"web:bad:8081"}, Worktree: dir},
		{Project: "app", Services: []string{"web:8080:8081"}, Preset: "nope", Worktree: dir},
	}
	for _, req := range cases {
		_, err := app.Init(context.Background(), req)
		ae, ok := apperr.As(err)
		if !ok || ae.Code != "usage" {
			t.Fatalf("req = %+v: err = %v, want usage", req, err)
		}
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "{}\n" {
		t.Fatalf("config was overwritten: %q err=%v", data, err)
	}
}

func TestInitForceOverwritesConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portucca.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir}},
		Config:   config.Loader{},
		Registry: &fakeRegistry{reg: domain.Registry{Version: 1}},
		Locker:   fakeLocker{},
	}
	if _, err := app.Init(context.Background(), InitRequest{
		Project:  "app",
		Services: []string{"web:8080:8081"},
		Worktree: dir,
		Force:    true,
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Project != "app" || cfg.Services["web"].MainPort != 8080 {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestInitGenericPresetLeavesEnvAndBootstrapEmpty(t *testing.T) {
	dir := t.TempDir()
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir}},
		Config:   config.Loader{},
		Registry: &fakeRegistry{reg: domain.Registry{Version: 1}},
		Locker:   fakeLocker{},
	}
	if _, err := app.Init(context.Background(), InitRequest{
		Project:  "app",
		Services: []string{"web:8080:8081"},
		Preset:   "generic",
		Worktree: dir,
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(filepath.Join(dir, "portucca.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Env) != 0 || len(cfg.Bootstrap.CopyFromMain) != 0 || len(cfg.Bootstrap.Symlinks) != 0 {
		t.Fatalf("config = %+v, want no env or bootstrap", cfg)
	}
}

func TestInitNodeViteNestPresetFillsEnvAndBootstrap(t *testing.T) {
	dir := t.TempDir()
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir}},
		Config:   config.Loader{},
		Registry: &fakeRegistry{reg: domain.Registry{Version: 1}},
		Locker:   fakeLocker{},
	}
	if _, err := app.Init(context.Background(), InitRequest{
		Project:  "app",
		Services: []string{"web:3000:3001", "api:3004:3005"},
		Preset:   "node-vite-nest",
		Worktree: dir,
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFile(filepath.Join(dir, "portucca.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Env) != 3 {
		t.Fatalf("env = %+v, want three targets", cfg.Env)
	}
	if len(cfg.Bootstrap.CopyFromMain) != 1 || len(cfg.Bootstrap.Symlinks) != 1 {
		t.Fatalf("bootstrap = %+v", cfg.Bootstrap)
	}
}

func TestInitRejectsNodeViteNestWithoutRequiredServices(t *testing.T) {
	dir := t.TempDir()
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir}},
		Config:   config.Loader{},
		Registry: &fakeRegistry{reg: domain.Registry{Version: 1}},
		Locker:   fakeLocker{},
	}
	_, err := app.Init(context.Background(), InitRequest{
		Project:  "app",
		Services: []string{"web:3000:3001"},
		Preset:   "node-vite-nest",
		Worktree: dir,
	})
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "usage" {
		t.Fatalf("err = %v, want usage", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "portucca.json")); !os.IsNotExist(err) {
		t.Fatalf("invalid config was written: %v", err)
	}
}

func TestInitWarnsWhenProjectHasClaimsElsewhere(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{
		Version:   1,
		Worktrees: []domain.Claim{{Project: "app", Path: other}},
	}}
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir}},
		Config:   config.Loader{},
		Registry: reg,
		Locker:   fakeLocker{},
	}
	got, err := app.Init(context.Background(), InitRequest{
		Project:  "app",
		Services: []string{"web:8080:8081"},
		Worktree: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Warning, other) {
		t.Fatalf("warning = %q, want it to mention %s", got.Warning, other)
	}
	if reg.saves != 0 {
		t.Fatalf("init wrote the registry: saves=%d", reg.saves)
	}
}
