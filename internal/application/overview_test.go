package application

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

type mapConfig struct {
	roots   map[string]domain.Config
	files   map[string]domain.Config
	rootErr map[string]error
	fileErr map[string]error
}

func (m mapConfig) LoadRoot(root string) (domain.Config, string, error) {
	if err, ok := m.rootErr[root]; ok {
		return domain.Config{}, "", err
	}
	if cfg, ok := m.roots[root]; ok {
		return cfg, filepath.Join(root, "portucca.json"), nil
	}
	return domain.Config{}, "", fs.ErrNotExist
}

func (m mapConfig) LoadFile(path string) (domain.Config, error) {
	if err, ok := m.fileErr[path]; ok {
		return domain.Config{}, err
	}
	if cfg, ok := m.files[path]; ok {
		return cfg, nil
	}
	return domain.Config{}, fs.ErrNotExist
}

func overviewApp(t *testing.T, reg *fakeRegistry, cfg ConfigLoader, locker Locker) (*App, *fakeEnv) {
	t.Helper()
	env := &fakeEnv{}
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	return &App{
		Locator:  fakeLocator{ctx: project.Context{Root: t.TempDir(), MainRoot: t.TempDir()}},
		Config:   cfg,
		Registry: reg,
		Locker:   locker,
		Prober:   fakeProber{},
		EnvStore: env,
		Now:      func() time.Time { return now },
	}, env
}

func overviewCfg() domain.Config {
	return domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 5173, StartPort: 5174},
		},
	}
}

func overviewRow(t *testing.T, res OverviewResult, kind, path string) OverviewWorktree {
	t.Helper()
	for _, row := range res.Worktrees {
		if row.Kind == kind && row.Path == path {
			return row
		}
	}
	t.Fatalf("row %s %s not found in %+v", kind, path, res.Worktrees)
	return OverviewWorktree{}
}

func TestOverviewEmptyRegistry(t *testing.T) {
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{}, Mains: []domain.MainReservation{}}}
	app, _ := overviewApp(t, reg, mapConfig{}, &recordingLocker{})
	got, err := app.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.RegistryVersion != 2 || got.Worktrees == nil || len(got.Worktrees) != 0 {
		t.Fatalf("overview = %+v", got)
	}
}

func TestOverviewIncludesClaimsAndMains(t *testing.T) {
	mainPath := t.TempDir()
	wtPath := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{
		Version:   2,
		Mains:     []domain.MainReservation{{Project: "app", Path: mainPath, Ports: map[domain.ServiceName]domain.Port{"web": 5173}}},
		Worktrees: []domain.Claim{{Project: "app", Path: wtPath, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5174}}}},
	}}
	cfg := overviewCfg()
	app, _ := overviewApp(t, reg, mapConfig{roots: map[string]domain.Config{mainPath: cfg}}, &recordingLocker{})
	got, err := app.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Worktrees) != 2 || got.Worktrees[0].Kind != "main" || got.Worktrees[1].Kind != "worktree" {
		t.Fatalf("rows = %+v", got.Worktrees)
	}
	if got.Worktrees[0].Issues == nil || got.Worktrees[1].Issues == nil {
		t.Fatalf("issues must be non-nil: %+v", got.Worktrees)
	}
	wt := overviewRow(t, got, "worktree", wtPath)
	if len(wt.Services) != 1 || wt.Services[0].Port != 5174 || !wt.Services[0].InBand || wt.Services[0].StartPort != 5174 {
		t.Fatalf("services = %+v", wt.Services)
	}
	if len(wt.Issues) != 0 {
		t.Fatalf("issues = %+v", wt.Issues)
	}
	if !got.GeneratedAt.Equal(time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("generatedAt = %v", got.GeneratedAt)
	}
}

func TestOverviewConfigMissing(t *testing.T) {
	wtPath := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{{Project: "app", Path: wtPath, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5174}}}}}}
	app, _ := overviewApp(t, reg, mapConfig{}, &recordingLocker{})
	got, err := app.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssueCode(overviewRow(t, got, "worktree", wtPath).Issues, "config_missing") {
		t.Fatalf("issues = %+v", got.Worktrees[0].Issues)
	}
}

func TestOverviewConfigInvalid(t *testing.T) {
	wtPath := t.TempDir()
	local := filepath.Join(wtPath, "portucca.json")
	if err := os.WriteFile(local, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{{Project: "app", Path: wtPath, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5174}}}}}}
	app, _ := overviewApp(t, reg, mapConfig{fileErr: map[string]error{local: errors.New("config_invalid: bad json")}}, &recordingLocker{})
	got, err := app.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row := overviewRow(t, got, "worktree", wtPath)
	if !hasIssueCode(row.Issues, "config_invalid") {
		t.Fatalf("issues = %+v", row.Issues)
	}
	for _, issue := range row.Issues {
		if strings.Contains(issue.Message, "config_invalid:") {
			t.Fatalf("issue repeats the code: %+v", issue)
		}
	}
}

func TestOverviewConfigInvalidRoot(t *testing.T) {
	wtPath := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{{Project: "app", Path: wtPath, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5174}}}}}}
	app, _ := overviewApp(t, reg, mapConfig{rootErr: map[string]error{wtPath: errors.New("bad json")}}, &recordingLocker{})
	got, err := app.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssueCode(overviewRow(t, got, "worktree", wtPath).Issues, "config_invalid") {
		t.Fatalf("issues = %+v", got.Worktrees[0].Issues)
	}
}

func TestOverviewConfigVersion(t *testing.T) {
	wtPath := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{{Project: "app", Path: wtPath, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5174}}}}}}
	app, _ := overviewApp(t, reg, mapConfig{rootErr: map[string]error{wtPath: errors.New("config_version: 2")}}, &recordingLocker{})
	got, err := app.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row := overviewRow(t, got, "worktree", wtPath)
	if !hasIssueCode(row.Issues, "config_invalid") {
		t.Fatalf("issues = %+v, want config_invalid", row.Issues)
	}
	for _, issue := range row.Issues {
		if strings.Contains(issue.Message, "config_version:") {
			t.Fatalf("issue repeats the prefix: %+v", issue)
		}
	}
}

func TestOverviewServiceNotInConfig(t *testing.T) {
	wtPath := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{{Project: "app", Path: wtPath, Services: map[domain.ServiceName]domain.ServiceClaim{"api": {Port: 5000}}}}}}
	app, _ := overviewApp(t, reg, mapConfig{roots: map[string]domain.Config{wtPath: overviewCfg()}}, &recordingLocker{})
	got, err := app.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssueCode(overviewRow(t, got, "worktree", wtPath).Issues, "service_not_in_config") {
		t.Fatalf("issues = %+v", got.Worktrees[0].Issues)
	}
}

func TestOverviewClaimOutOfBand(t *testing.T) {
	wtPath := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{{Project: "app", Path: wtPath, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5000}}}}}}
	app, _ := overviewApp(t, reg, mapConfig{roots: map[string]domain.Config{wtPath: overviewCfg()}}, &recordingLocker{})
	got, err := app.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssueCode(overviewRow(t, got, "worktree", wtPath).Issues, "claim_out_of_band") {
		t.Fatalf("issues = %+v", got.Worktrees[0].Issues)
	}
}

func TestOverviewMainPortMismatch(t *testing.T) {
	mainPath := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Mains: []domain.MainReservation{{Project: "app", Path: mainPath, Ports: map[domain.ServiceName]domain.Port{"web": 5173}}}}}
	cfg := overviewCfg()
	web := cfg.Services["web"]
	web.MainPort = 6000
	web.StartPort = 6001
	cfg.Services["web"] = web
	app, _ := overviewApp(t, reg, mapConfig{roots: map[string]domain.Config{mainPath: cfg}}, &recordingLocker{})
	got, err := app.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !hasIssueCode(overviewRow(t, got, "main", mainPath).Issues, "main_port_mismatch") {
		t.Fatalf("issues = %+v", got.Worktrees[0].Issues)
	}
}

func TestOverviewStalePath(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Worktrees: []domain.Claim{{Project: "app", Path: gone, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5174}}}}}}
	app, _ := overviewApp(t, reg, mapConfig{roots: map[string]domain.Config{gone: overviewCfg()}}, &recordingLocker{})
	got, err := app.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row := overviewRow(t, got, "worktree", gone)
	if row.Live || !hasIssueCode(row.Issues, "stale_path") {
		t.Fatalf("row = %+v", row)
	}
}

func TestOverviewLocalConfigOverride(t *testing.T) {
	mainPath := t.TempDir()
	wtPath := t.TempDir()
	local := filepath.Join(wtPath, "portucca.json")
	if err := os.WriteFile(local, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	localCfg := overviewCfg()
	web := localCfg.Services["web"]
	web.StartPort = 6000
	localCfg.Services["web"] = web
	reg := &fakeRegistry{reg: domain.Registry{
		Version:   2,
		Mains:     []domain.MainReservation{{Project: "app", Path: mainPath, Ports: map[domain.ServiceName]domain.Port{"web": 5173}}},
		Worktrees: []domain.Claim{{Project: "app", Path: wtPath, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5175}}}},
	}}
	app, _ := overviewApp(t, reg, mapConfig{
		roots: map[string]domain.Config{mainPath: overviewCfg()},
		files: map[string]domain.Config{local: localCfg},
	}, &recordingLocker{})
	got, err := app.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	row := overviewRow(t, got, "worktree", wtPath)
	if len(row.Services) != 1 || row.Services[0].StartPort != 6000 || !hasIssueCode(row.Issues, "claim_out_of_band") {
		t.Fatalf("row = %+v", row)
	}
}

func TestOverviewSortsMainsFirst(t *testing.T) {
	alphaMain := t.TempDir()
	betaMain := t.TempDir()
	alphaWT := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 2,
		Mains: []domain.MainReservation{
			{Project: "beta", Path: betaMain, Ports: map[domain.ServiceName]domain.Port{"web": 5173}},
			{Project: "alpha", Path: alphaMain, Ports: map[domain.ServiceName]domain.Port{"web": 5173}},
		},
		Worktrees: []domain.Claim{{Project: "alpha", Path: alphaWT, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5174}}}},
	}}
	app, _ := overviewApp(t, reg, mapConfig{roots: map[string]domain.Config{
		alphaMain: overviewCfg(), betaMain: overviewCfg(), alphaWT: overviewCfg(),
	}}, &recordingLocker{})
	got, err := app.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha/main", "alpha/worktree", "beta/main"}
	for i, row := range got.Worktrees {
		if got := string(row.Project) + "/" + row.Kind; got != want[i] {
			t.Fatalf("row %d = %s, want %s", i, got, want[i])
		}
	}
}

func TestOverviewDoesNotWrite(t *testing.T) {
	mainPath := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{Version: 2, Mains: []domain.MainReservation{{Project: "app", Path: mainPath, Ports: map[domain.ServiceName]domain.Port{"web": 5173}}}}}
	locker := &recordingLocker{}
	app, env := overviewApp(t, reg, mapConfig{roots: map[string]domain.Config{mainPath: overviewCfg()}}, locker)
	if _, err := app.Overview(context.Background()); err != nil {
		t.Fatal(err)
	}
	if reg.saves != 0 || len(env.files) != 0 {
		t.Fatalf("overview wrote: saves=%d env=%+v", reg.saves, env.files)
	}
	if len(locker.shared) != 1 || !locker.shared[0] {
		t.Fatalf("lock modes = %+v, want shared", locker.shared)
	}
}
