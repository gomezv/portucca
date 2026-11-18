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
	"github.com/gomezv/portucca/internal/scan"
)

func applyProposal(t *testing.T, dir string) InitProposal {
	t.Helper()
	cfg := domain.Config{
		Version:  domain.ConfigVersion,
		Project:  "app",
		Services: map[domain.ServiceName]domain.Service{"web": {MainPort: 8080, StartPort: 8081}},
		Env:      []domain.EnvTarget{{Service: "web", File: ".env", Key: "PORT"}},
	}
	return InitProposal{
		Root: dir, MainRoot: dir, IsMain: true,
		ConfigPath: filepath.Join(dir, "portucca.json"),
		Config:     cfg, Project: "app", Services: cfg.Services,
		Sources: []ScanSource{{Field: "project", Value: "app", Source: "directory"}},
		Gaps:    []InitGap{},
	}
}

func applyApp(t *testing.T, dir string) (*App, *fakeRegistry) {
	t.Helper()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{}, Mains: []domain.MainReservation{}}}
	return &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir, MainRoot: dir, IsMain: true}},
		Config:   config.Loader{},
		Registry: reg,
		Locker:   fakeLocker{},
	}, reg
}

func TestApplyInitWritesConfigAndReservation(t *testing.T) {
	dir := t.TempDir()
	app, reg := applyApp(t, dir)
	got, err := app.ApplyInit(context.Background(), applyProposal(t, dir), ApplyInitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(got.ConfigPath); err != nil {
		t.Fatal(err)
	}
	if len(reg.reg.Mains) != 1 || reg.reg.Mains[0].Path != dir || reg.reg.Mains[0].Ports["web"] != 8080 {
		t.Fatalf("mains = %+v", reg.reg.Mains)
	}
	if got.Scan == nil || len(got.Scan.Sources) != 1 {
		t.Fatalf("scan = %+v", got.Scan)
	}
}

func TestApplyInitRefusesExistingWithoutForce(t *testing.T) {
	dir := t.TempDir()
	app, _ := applyApp(t, dir)
	path := filepath.Join(dir, "portucca.json")
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := app.ApplyInit(context.Background(), applyProposal(t, dir), ApplyInitOptions{})
	if ae, _ := apperr.As(err); ae == nil || ae.Code != "config_exists" {
		t.Fatalf("err = %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{"version":1}` {
		t.Fatalf("config overwritten: %s", data)
	}
}

func TestApplyInitForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	app, _ := applyApp(t, dir)
	path := filepath.Join(dir, "portucca.json")
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ApplyInit(context.Background(), applyProposal(t, dir), ApplyInitOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"web"`) {
		t.Fatalf("config = %s", data)
	}
}

func TestApplyInitWrapFailsBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	app, _ := applyApp(t, dir)
	_, err := app.ApplyInit(context.Background(), applyProposal(t, dir), ApplyInitOptions{WrapScript: true})
	if ae, _ := apperr.As(err); ae == nil || ae.Code != "package_script_missing" {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "portucca.json")); statErr == nil {
		t.Fatal("config written after a wrap error")
	}
}

func TestApplyInitWrapApplies(t *testing.T) {
	dir := t.TempDir()
	app, _ := applyApp(t, dir)
	path := writePackageJSON(t, dir, `{"scripts":{"dev":"next dev"}}`)
	got, err := app.ApplyInit(context.Background(), applyProposal(t, dir), ApplyInitOptions{WrapScript: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.PackageScript == nil || !got.PackageScript.Changed {
		t.Fatalf("packageScript = %+v", got.PackageScript)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"dev":"portucca run -- next dev"`) {
		t.Fatalf("package = %s", data)
	}
}

func TestApplyInitNoWrapPreviews(t *testing.T) {
	dir := t.TempDir()
	app, _ := applyApp(t, dir)
	path := writePackageJSON(t, dir, `{"scripts":{"dev":"next dev"}}`)
	got, err := app.ApplyInit(context.Background(), applyProposal(t, dir), ApplyInitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.PackageScript == nil || got.PackageScript.Changed {
		t.Fatalf("packageScript = %+v", got.PackageScript)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"dev":"next dev"`) {
		t.Fatalf("package = %s", data)
	}
}

type countingScanner struct{ calls int }

func (c *countingScanner) Scan(context.Context, scan.Options) (scan.Detected, error) {
	c.calls++
	return scan.Detected{}, nil
}

func TestInitScanServiceValidationPrecedesConfigExists(t *testing.T) {
	dir := t.TempDir()
	app, _ := applyApp(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "portucca.json"), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir, Services: []string{"bogus"}})
	if ae, _ := apperr.As(err); ae == nil || ae.Code != "usage" {
		t.Fatalf("err = %v, want usage", err)
	}
}

func TestInitScanChecksExistingConfigBeforeScan(t *testing.T) {
	dir := t.TempDir()
	app, _ := applyApp(t, dir)
	scanner := &countingScanner{}
	app.Scanner = scanner
	if err := os.WriteFile(filepath.Join(dir, "portucca.json"), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir})
	if ae, _ := apperr.As(err); ae == nil || ae.Code != "config_exists" {
		t.Fatalf("err = %v", err)
	}
	if scanner.calls != 0 {
		t.Fatalf("scanner called %d times", scanner.calls)
	}
}
