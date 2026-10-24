package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

type fakeLocator struct {
	ctx project.Context
	err error
}

func (f fakeLocator) Detect(string) (project.Context, error) { return f.ctx, f.err }

type fakeConfig struct {
	cfg domain.Config
	err error
}

func (f fakeConfig) LoadRoot(string) (domain.Config, string, error) {
	return f.cfg, "/repo/portucca.json", f.err
}
func (f fakeConfig) LoadFile(string) (domain.Config, error) { return f.cfg, f.err }

type fakeRegistry struct {
	reg   domain.Registry
	saves int
}

func (f *fakeRegistry) Load() (domain.Registry, error) { return f.reg, nil }
func (f *fakeRegistry) Save(reg domain.Registry) error {
	f.reg = reg
	f.saves++
	return nil
}
func (f *fakeRegistry) LockPath() string { return "/tmp/ports.json.lock" }

type fakeLocker struct{}

func (fakeLocker) Lock(string, time.Duration, bool) (func() error, error) {
	return func() error { return nil }, nil
}

type fakeProber struct{}

func (fakeProber) IsFree(domain.Port) bool { return true }

type fakeEnv struct {
	files  map[string]map[string]string
	reads  map[string]domain.Port
	values map[string]string
}

func (f *fakeEnv) ReadPort(_ string, ref domain.EnvRef) (domain.Port, bool, error) {
	p, ok := f.reads[ref.Key]
	return p, ok, nil
}
func (f *fakeEnv) ReadValue(_ string, ref domain.EnvRef) (string, bool, error) {
	v, ok := f.values[ref.Key]
	return v, ok, nil
}
func (f *fakeEnv) WriteValues(_ string, values map[string]map[string]string) (bool, error) {
	changed := false
	if f.files == nil {
		f.files = map[string]map[string]string{}
	}
	for file, kv := range values {
		if f.files[file] == nil {
			f.files[file] = map[string]string{}
		}
		for k, v := range kv {
			if f.files[file][k] != v {
				f.files[file][k] = v
				changed = true
			}
		}
	}
	return changed, nil
}
func (f *fakeEnv) CopyFromMain(string, string, string) (bool, error)  { return false, nil }
func (f *fakeEnv) EnsureSymlink(string, domain.Symlink) (bool, error) { return false, nil }

func testApp(locator fakeLocator, cfg domain.Config, reg *fakeRegistry, env EnvStore) *App {
	return &App{
		Locator:  locator,
		Config:   fakeConfig{cfg: cfg},
		Registry: reg,
		Locker:   fakeLocker{},
		Prober:   fakeProber{},
		EnvStore: env,
		Now:      func() time.Time { return time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC) },
	}
}

func TestEnsureAllocatesInWorktree(t *testing.T) {
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
		Env: []domain.EnvTarget{{Service: "web", File: ".env.local", Key: "PORT"}},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	env := &fakeEnv{}
	app := testApp(fakeLocator{ctx: project.Context{Root: "/wt", MainRoot: "/repo", IsMain: false}}, cfg, reg, env)
	got, err := app.Ensure(context.Background(), "/wt")
	if err != nil {
		t.Fatal(err)
	}
	if got.IsMain || !got.Changed || got.Services["web"].Port != 3001 {
		t.Fatalf("got = %+v", got)
	}
	if len(reg.reg.Worktrees) != 1 || env.files[".env.local"]["PORT"] != "3001" {
		t.Fatalf("registry=%+v env=%+v", reg.reg, env.files)
	}
}

func TestEnsureMainDoesNotWrite(t *testing.T) {
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	env := &fakeEnv{}
	app := testApp(fakeLocator{ctx: project.Context{Root: "/repo", MainRoot: "/repo", IsMain: true}}, cfg, reg, env)
	got, err := app.Ensure(context.Background(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsMain || got.Changed || got.Services["web"].Port != 3000 {
		t.Fatalf("got = %+v", got)
	}
	if reg.saves != 0 || len(env.files) != 0 {
		t.Fatalf("main wrote: saves=%d env=%+v", reg.saves, env.files)
	}
}

func TestEnsureDoesNotAdoptMainPort(t *testing.T) {
	ref := domain.EnvRef{File: ".env.local", Key: "PORT"}
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 5173, StartPort: 5174},
			"api": {MainPort: 3004, StartPort: 3005, AdoptFrom: &ref},
		},
		Env: []domain.EnvTarget{{Service: "api", File: ".env.local", Key: "PORT"}},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	env := &fakeEnv{reads: map[string]domain.Port{"PORT": 5173}}
	app := testApp(fakeLocator{ctx: project.Context{Root: "/wt", MainRoot: "/repo", IsMain: false}}, cfg, reg, env)
	got, err := app.Ensure(context.Background(), "/wt")
	if err != nil {
		t.Fatal(err)
	}
	api := got.Services["api"]
	if api.Port != 3005 || api.Adopted {
		t.Fatalf("api = %+v, want port 3005 without adoption", api)
	}
	if got.Services["web"].Port != 5174 {
		t.Fatalf("web = %+v, want port 5174", got.Services["web"])
	}
	if env.files[".env.local"]["PORT"] != "3005" {
		t.Fatalf("env = %+v, want PORT=3005", env.files)
	}
}

func TestEnsureEnvSerializesAsEmptyArray(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
	}
	reg := &fakeRegistry{reg: domain.Registry{Version: 1}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root, IsMain: false}}, cfg, reg, &fakeEnv{})
	got, err := app.Ensure(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Env == nil {
		t.Fatal("Env is nil, want an empty slice")
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"env":[]`) {
		t.Fatalf("json = %s, want \"env\":[]", data)
	}
}

func TestEnsureIdempotent(t *testing.T) {
	root := t.TempDir()
	cfg := domain.Config{
		Version: 1,
		Project: "app",
		Services: map[domain.ServiceName]domain.Service{
			"web": {MainPort: 3000, StartPort: 3001},
		},
		Env: []domain.EnvTarget{{Service: "web", File: ".env.local", Key: "PORT"}},
	}
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 1,
		Worktrees: []domain.Claim{{
			Project: "app", Path: root,
			Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}},
		}},
	}}
	env := &fakeEnv{files: map[string]map[string]string{".env.local": {"PORT": "3001"}}}
	app := testApp(fakeLocator{ctx: project.Context{Root: root, MainRoot: root, IsMain: false}}, cfg, reg, env)
	got, err := app.Ensure(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Changed || reg.saves != 0 {
		t.Fatalf("idempotent run changed: %+v saves=%d", got, reg.saves)
	}
}
