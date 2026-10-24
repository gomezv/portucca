package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

func TestDoctorDetectsMainPortClaimed(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
			"api": {MainPort: 3004, StartPort: 3005},
		},
		Env: []domain.EnvTarget{{Service: "web", File: ".env.local", Key: "PORT"}},
	}
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 1,
		Worktrees: []domain.Claim{{
			Project: "app", Path: root,
			Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3004}},
		}},
	}}
	env := &fakeEnv{values: map[string]string{"PORT": "3004"}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, env)
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got.OK {
		t.Fatal("Doctor reported OK, want errors")
	}
	found := false
	for _, issue := range got.Errors {
		if issue.Code == "main_port_claimed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors = %+v, want main_port_claimed", got.Errors)
	}
}

func TestDoctorReportsStaleClaimWithoutWriting(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(t.TempDir(), "gone")
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	reg := &fakeRegistry{reg: domain.Registry{
		Version:   1,
		Worktrees: []domain.Claim{{Project: "app", Path: missing}},
	}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, &fakeEnv{})
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got.OK {
		t.Fatal("Doctor reported OK, want stale_claim error")
	}
	if len(got.Claims) != 1 || got.Claims[0].Live {
		t.Fatalf("claims = %+v", got.Claims)
	}
	if got.Registry.Claims != 1 || got.Registry.Version != 1 {
		t.Fatalf("registry = %+v", got.Registry)
	}
	if reg.saves != 0 {
		t.Fatalf("doctor wrote the registry: saves=%d", reg.saves)
	}
}

func TestDoctorReportsMissingConfigAndKeepsRegistry(t *testing.T) {
	root := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{
		Version:   1,
		Worktrees: []domain.Claim{{Project: "app", Path: root}},
	}}
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: root, MainRoot: root}},
		Config:   fakeConfig{err: errors.New("config_missing: no portucca.json")},
		Registry: reg,
		Locker:   fakeLocker{},
		Prober:   fakeProber{},
		EnvStore: &fakeEnv{},
	}
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got.OK {
		t.Fatal("Doctor reported OK, want config_missing error")
	}
	found := false
	for _, issue := range got.Errors {
		if issue.Code == "config_missing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors = %+v, want config_missing", got.Errors)
	}
	if got.Registry.Claims != 1 || got.Registry.Version != 1 || len(got.Claims) != 1 {
		t.Fatalf("registry = %+v claims = %+v", got.Registry, got.Claims)
	}
	if got.Config.Valid {
		t.Fatalf("config = %+v, want invalid", got.Config)
	}
	if reg.saves != 0 {
		t.Fatalf("doctor wrote the registry: saves=%d", reg.saves)
	}
}

func TestDoctorReportsConfigState(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
			"api": {MainPort: 3004, StartPort: 3005},
		},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, &fakeEnv{})
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Config.Valid || got.Config.Version != 1 {
		t.Fatalf("config = %+v", got.Config)
	}
	web, ok := got.Config.Services["web"]
	if !ok || web.MainPort != 3000 || web.StartPort != 3001 {
		t.Fatalf("services = %+v", got.Config.Services)
	}
	api, ok := got.Config.Services["api"]
	if !ok || api.MainPort != 3004 || api.StartPort != 3005 {
		t.Fatalf("services = %+v", got.Config.Services)
	}
}

type rootConfigLoader struct {
	rootCfg   domain.Config
	claimPath string
	claimCfg  domain.Config
}

func (l rootConfigLoader) LoadRoot(root string) (domain.Config, string, error) {
	if root != l.claimPath {
		return domain.Config{}, "", errors.New("config_missing: unexpected root")
	}
	return l.claimCfg, filepath.Join(root, "portucca.json"), nil
}

func (l rootConfigLoader) LoadFile(string) (domain.Config, error) { return l.rootCfg, nil }

func TestDoctorReadsPerClaimConfigWithOverride(t *testing.T) {
	root := t.TempDir()
	claimPath := t.TempDir()
	rootCfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	claimCfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 1,
		Worktrees: []domain.Claim{{
			Project: "app", Path: claimPath,
			Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3000}},
		}},
	}}
	app := &App{
		Locator:    fakeLocator{ctx: project.Context{Root: root, MainRoot: root}},
		Config:     rootConfigLoader{rootCfg: rootCfg, claimPath: claimPath, claimCfg: claimCfg},
		Registry:   reg,
		Locker:     fakeLocker{},
		Prober:     fakeProber{},
		EnvStore:   &fakeEnv{},
		ConfigFile: filepath.Join(root, "portucca.json"),
	}
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range got.Warnings {
		if warning.Code == "config_invalid" || warning.Code == "config_missing" {
			t.Fatalf("false config warning: %+v", got.Warnings)
		}
	}
	found := false
	for _, issue := range got.Errors {
		if issue.Code == "main_port_claimed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors = %+v, want main_port_claimed from the claim config", got.Errors)
	}
}

func TestDoctorReportsBootstrapIssuesWithoutClaim(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
		Bootstrap: domain.Bootstrap{
			Symlinks:     []domain.Symlink{{Path: "apps/api/.env", Target: "../../.env"}},
			CopyFromMain: []string{".env"},
		},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, nil)
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, warning := range got.Warnings {
		codes[warning.Code] = true
	}
	if !codes["symlink_missing"] || !codes["bootstrap_pending"] {
		t.Fatalf("warnings = %+v, want symlink_missing and bootstrap_pending", got.Warnings)
	}
}

func TestDoctorReportsMissingEnvWithoutClaim(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
		Env: []domain.EnvTarget{{Service: "web", File: ".env.local", Key: "PORT"}},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, &fakeEnv{})
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasWarning(got.Warnings, "env_missing") {
		t.Fatalf("warnings = %+v, want env_missing", got.Warnings)
	}
}

func TestDoctorReportsEnvSymlinkWithoutClaim(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(".env.shared", filepath.Join(root, ".env.local")); err != nil {
		t.Fatal(err)
	}
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
		Env: []domain.EnvTarget{{Service: "web", File: ".env.local", Key: "PORT"}},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, &fakeEnv{})
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasWarning(got.Warnings, "env_symlink") {
		t.Fatalf("warnings = %+v, want env_symlink", got.Warnings)
	}
}

func TestDoctorReportsBootstrapSourceMissing(t *testing.T) {
	root := t.TempDir()
	mainRoot := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
		Bootstrap: domain.Bootstrap{CopyFromMain: []string{".env"}},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: mainRoot}}, cfg, reg, &fakeEnv{})
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasWarning(got.Warnings, "bootstrap_source_missing") {
		t.Fatalf("warnings = %+v, want bootstrap_source_missing", got.Warnings)
	}
}

func hasWarning(warnings []Issue, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}

func TestDoctorDuplicateClaimMessageIncludesPortAndPaths(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 1,
		Worktrees: []domain.Claim{
			{Project: "app", Path: root, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}}},
			{Project: "app", Path: other, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}}},
		},
	}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}}, cfg, reg, &fakeEnv{})
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	var duplicate *Issue
	for i := range got.Errors {
		if got.Errors[i].Code == "duplicate_claim" {
			duplicate = &got.Errors[i]
		}
	}
	if duplicate == nil {
		t.Fatalf("errors = %+v, want duplicate_claim", got.Errors)
	}
	for _, want := range []string{"3001", root, other} {
		if !strings.Contains(duplicate.Message, want) {
			t.Fatalf("message = %q, want it to contain %q", duplicate.Message, want)
		}
	}
}

type recordingLocker struct {
	shared []bool
}

func (l *recordingLocker) Lock(_ string, _ time.Duration, shared bool) (func() error, error) {
	l.shared = append(l.shared, shared)
	return func() error { return nil }, nil
}

func TestDoctorTakesSharedLock(t *testing.T) {
	root := t.TempDir()
	locker := &recordingLocker{}
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: root, MainRoot: root}},
		Config:   fakeConfig{cfg: cfg},
		Registry: &fakeRegistry{reg: domain.Registry{Version: 1}},
		Locker:   locker,
		Prober:   fakeProber{},
		EnvStore: &fakeEnv{},
	}
	if _, err := app.Doctor(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if len(locker.shared) != 1 || !locker.shared[0] {
		t.Fatalf("lock flags = %v, want one shared lock", locker.shared)
	}
}

func TestDoctorReportsProjectErrorCode(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root}, err: project.ErrGitMissing}, cfg, reg, &fakeEnv{})
	got, err := app.Doctor(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Warnings) != 1 || got.Warnings[0].Code != "git_missing" {
		t.Fatalf("warnings = %+v, want git_missing", got.Warnings)
	}
}
