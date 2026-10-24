package integration

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/config"
	"github.com/gomezv/portucca/internal/envfile"
	"github.com/gomezv/portucca/internal/netprobe"
	"github.com/gomezv/portucca/internal/platform"
	"github.com/gomezv/portucca/internal/project"
	"github.com/gomezv/portucca/internal/registry"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newApp(t *testing.T, registryPath string) *application.App {
	t.Helper()
	return &application.App{
		Locator:     project.Detector{},
		Config:      config.Loader{},
		Registry:    registry.Store{Path: registryPath},
		Locker:      platform.Locker{},
		Prober:      netprobe.Prober{},
		EnvStore:    envfile.Store{},
		LockTimeout: 2 * time.Second,
	}
}

func TestFixturesAreValidConfigs(t *testing.T) {
	for _, name := range []string{"generic", "monorepo"} {
		path := filepath.Join("..", "fixtures", name, "portucca.json")
		if _, err := config.LoadFile(path); err != nil {
			t.Errorf("%s fixture: %v", name, err)
		}
	}
}

func TestSetupEnsureReallocatePrune(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-q")
	cfg := []byte(`{
  "version": 1,
  "project": "app",
  "services": {"web": {"mainPort": 18080, "startPort": 18081}},
  "env": [{"service": "web", "file": ".env.local", "key": "PORT"}],
  "bootstrap": {
    "copyFromMain": [".env"],
    "symlinks": [{"path": ".env.link", "target": ".env"}]
  }
}`)
	if err := os.WriteFile(filepath.Join(main, "portucca.json"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, main, "add", ".")
	git(t, main, "commit", "-qm", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, main, "worktree", "add", "-q", wt, "-b", "feature")
	if err := os.WriteFile(filepath.Join(main, ".env"), []byte("SHARED=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	regPath := filepath.Join(t.TempDir(), "ports.json")
	app := newApp(t, regPath)
	ctx := context.Background()
	got, err := app.Setup(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	if got.Services["web"].Port != 18081 {
		t.Fatalf("port = %d", got.Services["web"].Port)
	}
	found := false
	for _, rel := range got.Bootstrap.Copied {
		if rel == ".env" {
			found = true
		}
	}
	if !found {
		t.Fatalf("bootstrap copied = %v, want .env", got.Bootstrap.Copied)
	}
	if _, err := os.Stat(filepath.Join(wt, ".env")); err != nil {
		t.Fatalf("bootstrap did not copy .env: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(wt, ".env.link")); err != nil || target != ".env" {
		t.Fatalf("bootstrap symlink = %q, %v; want .env", target, err)
	}
	again, err := app.Ensure(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed {
		t.Fatal("second ensure changed state")
	}
	re, err := app.Reallocate(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	if re.Services["web"].Port == got.Services["web"].Port {
		t.Fatal("reallocate did not change the port")
	}
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}
	pruned, err := app.Prune(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pruned.Removed) != 1 {
		t.Fatalf("removed = %d", len(pruned.Removed))
	}
}

func TestConcurrentEnsureUsesOneClaim(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-q")
	cfg := []byte(`{"version":1,"project":"app","services":{"web":{"mainPort":18090,"startPort":18091}}}`)
	if err := os.WriteFile(filepath.Join(main, "portucca.json"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, main, "add", ".")
	git(t, main, "commit", "-qm", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, main, "worktree", "add", "-q", wt, "-b", "feature")
	regPath := filepath.Join(t.TempDir(), "ports.json")

	app := newApp(t, regPath)
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := app.Ensure(context.Background(), wt)
			done <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	reg, err := (registry.Store{Path: regPath}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Worktrees) != 1 {
		t.Fatalf("claims = %d, want 1", len(reg.Worktrees))
	}
}

func TestEnsureAdoptsExistingEnvPort(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-q")
	cfg := []byte(`{
  "version": 1,
  "project": "adopt-app",
  "services": {"web": {"mainPort": 18200, "startPort": 18201, "adoptFrom": {"file": ".env.local", "key": "PORT"}}},
  "env": [{"service": "web", "file": ".env.local", "key": "PORT"}]
}`)
	if err := os.WriteFile(filepath.Join(main, "portucca.json"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, main, "add", ".")
	git(t, main, "commit", "-qm", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, main, "worktree", "add", "-q", wt, "-b", "feature")
	if err := os.WriteFile(filepath.Join(wt, ".env.local"), []byte("PORT=18205\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	app := newApp(t, filepath.Join(t.TempDir(), "ports.json"))
	got, err := app.Ensure(context.Background(), wt)
	if err != nil {
		t.Fatal(err)
	}
	claim, ok := got.Services["web"]
	if !ok || claim.Port != 18205 || !claim.Adopted {
		t.Fatalf("claim = %+v, want adopted port 18205", claim)
	}
}

func TestDoctorIsReadOnly(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-q")
	cfg := []byte(`{
  "version": 1,
  "project": "app",
  "services": {"web": {"mainPort": 18060, "startPort": 18061}},
  "env": [{"service": "web", "file": ".env.local", "key": "PORT"}],
  "bootstrap": {"copyFromMain": [".env"], "symlinks": [{"path": ".env.link", "target": ".env"}]}
}`)
	if err := os.WriteFile(filepath.Join(main, "portucca.json"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, main, "add", ".")
	git(t, main, "commit", "-qm", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, main, "worktree", "add", "-q", wt, "-b", "feature")
	if err := os.WriteFile(filepath.Join(main, ".env"), []byte("SHARED=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	regPath := filepath.Join(t.TempDir(), "ports.json")
	app := newApp(t, regPath)
	ctx := context.Background()
	if _, err := app.Setup(ctx, wt); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := app.Doctor(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	if !got.OK {
		t.Fatalf("doctor errors = %+v", got.Errors)
	}
	if got.Registry.Claims != 1 {
		t.Fatalf("registry claims = %d, want 1", got.Registry.Claims)
	}
	after, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("doctor rewrote the registry")
	}
}
