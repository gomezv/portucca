package application

import (
	"context"
	"errors"
	"testing"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

type bootstrapEnv struct {
	*fakeEnv
	copied    []string
	symlinked []string
}

func (b *bootstrapEnv) CopyFromMain(_, _, rel string) (bool, error) {
	b.copied = append(b.copied, rel)
	return true, nil
}
func (b *bootstrapEnv) EnsureSymlink(_ string, s domain.Symlink) (bool, error) {
	b.symlinked = append(b.symlinked, s.Path)
	return true, nil
}

func TestSetupBootstrapsThenEnsures(t *testing.T) {
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
		Env: []domain.EnvTarget{{Service: "web", File: ".env.local", Key: "PORT"}},
		Bootstrap: domain.Bootstrap{
			CopyFromMain: []string{".env"},
			Symlinks:     []domain.Symlink{{Path: "apps/api/.env", Target: "../../.env"}},
		},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	env := &bootstrapEnv{fakeEnv: &fakeEnv{}}
	app := testApp(fakeLocator{ctx: project.Context{Root: "/wt", MainRoot: "/repo", IsMain: false}}, cfg, reg, env)
	got, err := app.Setup(context.Background(), "/wt")
	if err != nil {
		t.Fatal(err)
	}
	if len(env.copied) != 1 || len(env.symlinked) != 1 {
		t.Fatalf("bootstrap copied=%v symlinked=%v", env.copied, env.symlinked)
	}
	if got.Services["web"].Port != 3001 {
		t.Fatalf("services = %+v", got.Services)
	}
}

func TestSetupBootstrapMarksChanged(t *testing.T) {
	root := t.TempDir()
	cfg := bootstrapConfig()
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 1,
		Worktrees: []domain.Claim{{
			Project:  "app",
			Path:     root,
			Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}},
		}},
	}}
	env := &bootstrapEnv{fakeEnv: &fakeEnv{
		files: map[string]map[string]string{".env.local": {"PORT": "3001"}},
	}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: "/repo", IsMain: false}}, cfg, reg, env)
	got, err := app.Setup(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Bootstrap.Copied) != 1 || !got.Changed {
		t.Fatalf("got = %+v, want a copied bootstrap and changed true", got)
	}
	if reg.saves != 0 {
		t.Fatalf("registry saved without changes: %d", reg.saves)
	}
}

type failingCopyEnv struct {
	*fakeEnv
}

func (failingCopyEnv) CopyFromMain(string, string, string) (bool, error) {
	return false, errors.New("disk full")
}

type failingWriteEnv struct {
	*bootstrapEnv
}

func (failingWriteEnv) WriteValues(string, map[string]map[string]string) (bool, error) {
	return false, errors.New("disk full")
}

func bootstrapConfig() domain.Config {
	return domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
		Env: []domain.EnvTarget{{Service: "web", File: ".env.local", Key: "PORT"}},
		Bootstrap: domain.Bootstrap{
			CopyFromMain: []string{".env"},
		},
	}
}

func errorCode(t *testing.T, err error) string {
	t.Helper()
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("expected apperr, got %v", err)
	}
	return e.Code
}

func TestSetupMainReturnsEmptyBootstrapActions(t *testing.T) {
	cfg := bootstrapConfig()
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	env := &bootstrapEnv{fakeEnv: &fakeEnv{}}
	app := testApp(fakeLocator{ctx: project.Context{Root: "/repo", MainRoot: "/repo", IsMain: true}}, cfg, reg, env)
	got, err := app.Setup(context.Background(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsMain || got.Services["web"].Port != 3000 {
		t.Fatalf("got = %+v", got)
	}
	if got.Bootstrap.Copied == nil || got.Bootstrap.Symlinked == nil || got.Bootstrap.Skipped == nil {
		t.Fatalf("bootstrap actions must be empty, not nil: %+v", got.Bootstrap)
	}
	if len(got.Bootstrap.Copied) != 0 || len(got.Bootstrap.Symlinked) != 0 || len(got.Bootstrap.Skipped) != 0 {
		t.Fatalf("main bootstrapped: %+v", got.Bootstrap)
	}
	if len(env.copied) != 0 || len(env.symlinked) != 0 || reg.saves != 0 {
		t.Fatalf("main wrote: copied=%v symlinked=%v saves=%d", env.copied, env.symlinked, reg.saves)
	}
}

func TestSetupBootstrapFailureLeavesRegistryUntouched(t *testing.T) {
	cfg := bootstrapConfig()
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	env := failingCopyEnv{fakeEnv: &fakeEnv{}}
	app := testApp(fakeLocator{ctx: project.Context{Root: "/wt", MainRoot: "/repo", IsMain: false}}, cfg, reg, &env)
	_, err := app.Setup(context.Background(), "/wt")
	if err == nil {
		t.Fatal("expected bootstrap error")
	}
	if code := errorCode(t, err); code != "bootstrap_error" {
		t.Fatalf("code = %q", code)
	}
	if reg.saves != 0 || len(env.files) != 0 {
		t.Fatalf("registry touched: saves=%d env=%+v", reg.saves, env.files)
	}
}

func TestSetupEnsureFailureKeepsBootstrapDone(t *testing.T) {
	cfg := bootstrapConfig()
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	env := &bootstrapEnv{fakeEnv: &fakeEnv{}}
	app := testApp(fakeLocator{ctx: project.Context{Root: "/wt", MainRoot: "/repo", IsMain: false}}, cfg, reg, &failingWriteEnv{bootstrapEnv: env})
	_, err := app.Setup(context.Background(), "/wt")
	if err == nil {
		t.Fatal("expected ensure error")
	}
	if code := errorCode(t, err); code != "env_write" {
		t.Fatalf("code = %q", code)
	}
	if len(env.copied) != 1 {
		t.Fatalf("bootstrap copy lost: %v", env.copied)
	}
	if reg.saves != 0 {
		t.Fatalf("registry saved after env failure: %d", reg.saves)
	}
}

func TestReallocateMainReserved(t *testing.T) {
	cfg := bootstrapConfig()
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	env := &fakeEnv{}
	app := testApp(fakeLocator{ctx: project.Context{Root: "/repo", MainRoot: "/repo", IsMain: true}}, cfg, reg, env)
	_, err := app.Reallocate(context.Background(), "/repo")
	if err == nil {
		t.Fatal("expected main_reserved")
	}
	if code := errorCode(t, err); code != "main_reserved" {
		t.Fatalf("code = %q", code)
	}
	if reg.saves != 0 || len(env.files) != 0 {
		t.Fatalf("main wrote: saves=%d env=%+v", reg.saves, env.files)
	}
}

func TestReallocateRequiresClaim(t *testing.T) {
	root := t.TempDir()
	cfg := bootstrapConfig()
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	env := &fakeEnv{}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: "/repo", IsMain: false}}, cfg, reg, env)
	_, err := app.Reallocate(context.Background(), root)
	if err == nil {
		t.Fatal("expected no_claim")
	}
	e, ok := apperr.As(err)
	if !ok || e.Code != "no_claim" || e.Hint != "run portucca ensure first" {
		t.Fatalf("err = %+v", e)
	}
	if reg.saves != 0 || len(env.files) != 0 {
		t.Fatalf("wrote without claim: saves=%d env=%+v", reg.saves, env.files)
	}
}

func TestReallocateExcludesCurrentPorts(t *testing.T) {
	root := t.TempDir()
	cfg := bootstrapConfig()
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 1,
		Worktrees: []domain.Claim{{
			Project:  "app",
			Path:     root,
			Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}},
		}},
	}}
	env := &fakeEnv{files: map[string]map[string]string{".env.local": {"PORT": "3001"}}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: "/repo", IsMain: false}}, cfg, reg, env)
	got, err := app.Reallocate(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Services["web"].Port != 3002 || !got.Changed {
		t.Fatalf("got = %+v", got)
	}
	if env.files[".env.local"]["PORT"] != "3002" {
		t.Fatalf("env = %+v", env.files)
	}
	if len(reg.reg.Worktrees) != 1 || reg.reg.Worktrees[0].Services["web"].Port != 3002 || reg.saves != 1 {
		t.Fatalf("registry = %+v saves=%d", reg.reg, reg.saves)
	}
}

func TestReallocateEnvFailureKeepsOldClaim(t *testing.T) {
	root := t.TempDir()
	cfg := bootstrapConfig()
	old := domain.Claim{
		Project:  "app",
		Path:     root,
		Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1, Worktrees: []domain.Claim{old}}}
	env := &failingWriteEnv{bootstrapEnv: &bootstrapEnv{fakeEnv: &fakeEnv{}}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: "/repo", IsMain: false}}, cfg, reg, env)
	_, err := app.Reallocate(context.Background(), root)
	if err == nil {
		t.Fatal("expected env_write")
	}
	if code := errorCode(t, err); code != "env_write" {
		t.Fatalf("code = %q", code)
	}
	if reg.saves != 0 {
		t.Fatalf("registry saved after env failure: %d", reg.saves)
	}
	if len(reg.reg.Worktrees) != 1 || reg.reg.Worktrees[0].Services["web"].Port != 3001 {
		t.Fatalf("old claim lost: %+v", reg.reg)
	}
}
