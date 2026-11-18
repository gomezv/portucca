package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
	"github.com/gomezv/portucca/internal/scan"
)

func analyzeProposal(t *testing.T, app *App, dir string) InitProposal {
	t.Helper()
	got, err := app.AnalyzeInit(context.Background(), InitRequest{Scan: true, Worktree: dir})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestInspectInitPath(t *testing.T) {
	dir := t.TempDir()
	app := &App{Locator: fakeLocator{ctx: project.Context{Root: dir, MainRoot: dir, IsMain: true}}}
	got, err := app.InspectInitPath(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Root != dir || got.MainRoot != dir || !got.IsMain {
		t.Fatalf("inspect = %+v", got)
	}
	app.Locator = fakeLocator{err: project.ErrNotRepo}
	if _, err := app.InspectInitPath(context.Background(), dir); err == nil {
		t.Fatal("want not_a_repo")
	} else if ae, _ := apperr.As(err); ae == nil || ae.Code != "not_a_repo" {
		t.Fatalf("err = %v", err)
	}
}

func TestAnalyzeInitDoesNotWrite(t *testing.T) {
	det := scan.Detected{
		Project: &scan.Value{Value: "app", Source: "directory"},
		Services: []scan.Service{{Name: "web", Dir: ".", NameSource: "directory",
			MainPort: &scan.Value{Value: "8080", Source: ".env:PORT"}}},
	}
	app, dir := scanApp(t, det)
	app.Locator = fakeLocator{ctx: project.Context{Root: dir, MainRoot: dir, IsMain: true}}
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{}, Mains: []domain.MainReservation{}}}
	app.Registry = reg

	got := analyzeProposal(t, app, dir)
	if got.Project != "app" || got.Services["web"].MainPort != 8080 || got.Services["web"].StartPort != 8081 {
		t.Fatalf("proposal = %+v", got)
	}
	if !got.IsMain || got.MainRoot != dir || got.ConfigPath != filepath.Join(dir, "portucca.json") {
		t.Fatalf("proposal meta = %+v", got)
	}
	if got.ConfigExists {
		t.Fatal("ConfigExists = true")
	}
	if len(got.Gaps) != 0 {
		t.Fatalf("gaps = %+v", got.Gaps)
	}
	if _, err := os.Stat(got.ConfigPath); !os.IsNotExist(err) {
		t.Fatalf("config written: %v", err)
	}
	if reg.saves != 0 {
		t.Fatalf("registry saves = %d", reg.saves)
	}
}

func TestAnalyzeInitReportsFillableGap(t *testing.T) {
	det := scan.Detected{
		Project:  &scan.Value{Value: "app", Source: "directory"},
		Services: []scan.Service{{Name: "api", Dir: "apps/api"}},
	}
	app, dir := scanApp(t, det)
	got := analyzeProposal(t, app, dir)
	if len(got.Gaps) != 1 {
		t.Fatalf("gaps = %+v", got.Gaps)
	}
	g := got.Gaps[0]
	if g.Field != "services.api.mainPort" || g.Reason != "missing" || len(g.Flags) == 0 {
		t.Fatalf("gap = %+v", g)
	}
	_, err := os.Stat(filepath.Join(dir, "portucca.json"))
	if !os.IsNotExist(err) {
		t.Fatalf("config written: %v", err)
	}
}

func TestAnalyzeInitExistingConfig(t *testing.T) {
	app, dir := scanApp(t, scan.Detected{
		Project:  &scan.Value{Value: "app", Source: "directory"},
		Services: []scan.Service{{Name: "web", MainPort: &scan.Value{Value: "8080", Source: ".env:PORT"}}},
	})
	if err := os.WriteFile(filepath.Join(dir, "portucca.json"), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got := analyzeProposal(t, app, dir)
	if !got.ConfigExists {
		t.Fatal("ConfigExists = false")
	}
}

func TestAnalyzeInitWorktree(t *testing.T) {
	app, dir := scanApp(t, scan.Detected{Project: &scan.Value{Value: "app", Source: "directory"}})
	app.Locator = fakeLocator{ctx: project.Context{Root: dir, MainRoot: "/tmp/main", IsMain: false}}
	got := analyzeProposal(t, app, dir)
	if got.IsMain || got.MainRoot != "/tmp/main" {
		t.Fatalf("proposal meta = %+v", got)
	}
}

func TestAnalyzeInitExplicitReplacesDetected(t *testing.T) {
	det := scan.Detected{
		Project: &scan.Value{Value: "app", Source: "directory"},
		Services: []scan.Service{
			{Name: "web", MainPort: &scan.Value{Value: "5173", Source: "package.json:scripts.dev"}},
			{Name: "api", MainPort: &scan.Value{Value: "3004", Source: "apps/api/.env:PORT"}},
		},
	}
	app, dir := scanApp(t, det)
	got, err := app.AnalyzeInit(context.Background(), InitRequest{
		Scan: true, Worktree: dir, Services: []string{"web:9000:9001"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services["web"].MainPort != 9000 {
		t.Fatalf("services = %+v", got.Services)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "ignoring detected services: api") {
		t.Fatalf("warnings = %+v", got.Warnings)
	}
}
