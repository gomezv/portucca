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

type fakeScanner struct {
	det scan.Detected
	err error
}

func (f fakeScanner) Scan(context.Context, scan.Options) (scan.Detected, error) {
	return f.det, f.err
}

func scanApp(t *testing.T, det scan.Detected) (*App, string) {
	t.Helper()
	dir := t.TempDir()
	return &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir, MainRoot: dir}},
		Config:   config.Loader{},
		Registry: &fakeRegistry{reg: domain.Registry{Version: 1}},
		Locker:   fakeLocker{},
		Scanner:  fakeScanner{det: det},
	}, dir
}

func TestInitScanWritesDetectedConfig(t *testing.T) {
	det := scan.Detected{
		Project: &scan.Value{Value: "app", Source: "directory"},
		Services: []scan.Service{{
			Name: "web", Dir: ".", NameSource: "directory",
			MainPort: &scan.Value{Value: "8080", Source: ".env:PORT"},
		}},
		Env: []scan.EnvTarget{{
			Service: "web", File: ".env", Key: "PORT", SourceFile: ".env", Source: ".env:PORT",
		}},
		CopyFromMain: []scan.CopyFile{{Service: "web", File: ".env"}},
	}
	app, dir := scanApp(t, det)
	got, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "app" || got.Services["web"].MainPort != 8080 || got.Services["web"].StartPort != 8081 {
		t.Fatalf("result = %+v", got)
	}
	if got.Scan == nil || len(got.Scan.Sources) == 0 {
		t.Fatalf("scan = %+v, want sources", got.Scan)
	}
	if !hasSource(got.Scan.Sources, "services.web.name", "web", "directory") {
		t.Fatalf("sources = %+v, want services.web.name from directory", got.Scan.Sources)
	}
	cfg, err := config.LoadFile(filepath.Join(dir, "portucca.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Env) != 1 || len(cfg.Bootstrap.CopyFromMain) != 1 {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestInitScanIncompleteDoesNotWrite(t *testing.T) {
	det := scan.Detected{
		Project:  &scan.Value{Value: "app", Source: "directory"},
		Services: []scan.Service{{Name: "api", Dir: "apps/api"}},
	}
	app, dir := scanApp(t, det)
	_, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir})
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "scan_incomplete" {
		t.Fatalf("err = %v, want scan_incomplete", err)
	}
	gaps, ok := ae.Details["gaps"].([]scan.Gap)
	if !ok || len(gaps) == 0 {
		t.Fatalf("details = %+v, want gaps", ae.Details)
	}
	if _, err := os.Stat(filepath.Join(dir, "portucca.json")); !os.IsNotExist(err) {
		t.Fatalf("config was written: %v", err)
	}
}

func TestInitScanExplicitOverrides(t *testing.T) {
	det := scan.Detected{
		Project: &scan.Value{Value: "app", Source: "directory"},
		Services: []scan.Service{
			{Name: "web", Dir: "apps/web", MainPort: &scan.Value{Value: "5173", Source: "package.json:scripts.dev"}},
			{Name: "api", Dir: "apps/api", MainPort: &scan.Value{Value: "3004", Source: "apps/api/.env:PORT"}},
		},
	}
	app, dir := scanApp(t, det)
	got, err := app.Init(context.Background(), InitRequest{
		Scan: true, Worktree: dir, Services: []string{"web:9000:9001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services["web"].MainPort != 9000 {
		t.Fatalf("services = %+v, want only the explicit web", got.Services)
	}
	if !strings.Contains(strings.Join(got.Scan.Warnings, " "), "ignoring detected services: api") {
		t.Fatalf("warnings = %+v, want the dropped-service warning", got.Scan.Warnings)
	}
}

func TestInitScanTooLarge(t *testing.T) {
	app, dir := scanApp(t, scan.Detected{})
	app.Scanner = fakeScanner{err: &scan.TooLargeError{Limit: 1, Visited: 2}}
	_, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir})
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "scan_too_large" {
		t.Fatalf("err = %v, want scan_too_large", err)
	}
}

func TestInitScanRejectsPreset(t *testing.T) {
	app, dir := scanApp(t, scan.Detected{})
	_, err := app.Init(context.Background(), InitRequest{Scan: true, Preset: "generic", Worktree: dir})
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "usage" {
		t.Fatalf("err = %v, want usage", err)
	}
}

func TestInitScanInvalidExplicitProject(t *testing.T) {
	app, dir := scanApp(t, scan.Detected{})
	_, err := app.Init(context.Background(), InitRequest{Scan: true, Project: "Bad", Worktree: dir})
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "usage" {
		t.Fatalf("err = %v, want usage", err)
	}
}

func TestInitScanProjectWithoutServicesIsIncomplete(t *testing.T) {
	app, dir := scanApp(t, scan.Detected{})
	_, err := app.Init(context.Background(), InitRequest{Scan: true, Project: "app", Worktree: dir})
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "scan_incomplete" {
		t.Fatalf("err = %v, want scan_incomplete", err)
	}
	gaps, ok := ae.Details["gaps"].([]scan.Gap)
	if !ok {
		t.Fatalf("details = %+v, want gaps", ae.Details)
	}
	found := false
	for _, gap := range gaps {
		if gap.Field == "services" && gap.Reason == "missing" && len(gap.Flags) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("gaps = %+v, want a services gap with flags", gaps)
	}
	if _, err := os.Stat(filepath.Join(dir, "portucca.json")); !os.IsNotExist(err) {
		t.Fatalf("config was written: %v", err)
	}
}

func TestInitScanGapsSuggestFlags(t *testing.T) {
	t.Run("missing project", func(t *testing.T) {
		app, dir := scanApp(t, scan.Detected{})
		_, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir})
		gaps := incompleteGaps(t, err)
		gap, ok := findGap(gaps, "project")
		if !ok || !containsString(gap.Flags, "--project <slug>") {
			t.Fatalf("gaps = %+v, want project flag", gaps)
		}
	})
	t.Run("invalid service name", func(t *testing.T) {
		app, dir := scanApp(t, scan.Detected{
			Project: &scan.Value{Value: "app", Source: "directory"},
			Services: []scan.Service{{
				Name: "Bad_Name", Dir: "bad", NameSource: "directory",
				MainPort: &scan.Value{Value: "8080", Source: "package.json:scripts.dev"},
			}},
		})
		_, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir})
		gaps := incompleteGaps(t, err)
		gap, ok := findGap(gaps, "services")
		if !ok || !containsString(gap.Flags, "--service <name>:<main>:<start>") {
			t.Fatalf("gaps = %+v, want service flag", gaps)
		}
	})
	t.Run("invalid config", func(t *testing.T) {
		app, dir := scanApp(t, scan.Detected{
			Project: &scan.Value{Value: "app", Source: "directory"},
			Services: []scan.Service{
				{Name: "a", Dir: "a", NameSource: "directory", MainPort: &scan.Value{Value: "8080", Source: "package.json:scripts.dev"}},
				{Name: "b", Dir: "b", NameSource: "directory", MainPort: &scan.Value{Value: "8080", Source: "package.json:scripts.dev"}},
			},
		})
		_, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir})
		gaps := incompleteGaps(t, err)
		gap, ok := findGap(gaps, "config")
		if !ok || !containsString(gap.Flags, "--project <slug>") || !containsString(gap.Flags, "--service <name>:<main>:<start>") {
			t.Fatalf("gaps = %+v, want config flags", gaps)
		}
	})
}

func incompleteGaps(t *testing.T, err error) []scan.Gap {
	t.Helper()
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "scan_incomplete" {
		t.Fatalf("err = %v, want scan_incomplete", err)
	}
	gaps, ok := ae.Details["gaps"].([]scan.Gap)
	if !ok {
		t.Fatalf("details = %+v, want gaps", ae.Details)
	}
	return gaps
}

func findGap(gaps []scan.Gap, field string) (scan.Gap, bool) {
	for _, gap := range gaps {
		if gap.Field == field {
			return gap, true
		}
	}
	return scan.Gap{}, false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestDeriveStartPortSkipsUsed(t *testing.T) {
	det := scan.Detected{
		Project: &scan.Value{Value: "app", Source: "directory"},
		Services: []scan.Service{
			{
				Name: "api", Dir: "apps/api", NameSource: "directory",
				MainPort: &scan.Value{Value: "8080", Source: "package.json:scripts.dev"},
			},
			{
				Name: "web", Dir: "apps/web", NameSource: "directory",
				MainPort: &scan.Value{Value: "8081", Source: "package.json:scripts.dev"},
			},
		},
	}
	merged := mergeDetected(InitRequest{}, det, nil)
	if len(merged.Gaps) != 0 {
		t.Fatalf("gaps = %+v, want none", merged.Gaps)
	}
	if got := merged.Config.Services["api"].StartPort; got != 8082 {
		t.Fatalf("api startPort = %d, want 8082", got)
	}
	if got := merged.Config.Services["web"].StartPort; got != 8083 {
		t.Fatalf("web startPort = %d, want 8083", got)
	}
}

func hasSource(sources []ScanSource, field, value, source string) bool {
	for _, item := range sources {
		if item.Field == field && item.Value == value && item.Source == source {
			return true
		}
	}
	return false
}
