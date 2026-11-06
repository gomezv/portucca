package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/config"
	"github.com/gomezv/portucca/internal/envfile"
	"github.com/gomezv/portucca/internal/netprobe"
	"github.com/gomezv/portucca/internal/platform"
	"github.com/gomezv/portucca/internal/project"
	"github.com/gomezv/portucca/internal/registry"
	"github.com/gomezv/portucca/internal/scan"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)...)
	env := make([]string, 0, len(os.Environ())+8)
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE":
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = append(env,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
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
		Scanner:     scan.Scanner{},
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
	envLocal := filepath.Join(wt, ".env.local")
	if data, err := os.ReadFile(envLocal); err != nil {
		t.Fatalf("env file: %v", err)
	} else if want := fmt.Sprintf("PORT=%d", got.Services["web"].Port); !strings.Contains(string(data), want) {
		t.Fatalf(".env.local = %q, want %s", data, want)
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
	beforeEnsure, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	again, err := app.Ensure(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed {
		t.Fatal("second ensure changed state")
	}
	afterEnsure, err := os.ReadFile(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeEnsure, afterEnsure) {
		t.Fatal("second ensure rewrote the registry")
	}
	re, err := app.Reallocate(ctx, wt)
	if err != nil {
		t.Fatal(err)
	}
	if re.Services["web"].Port == got.Services["web"].Port {
		t.Fatal("reallocate did not change the port")
	}
	if data, err := os.ReadFile(envLocal); err != nil {
		t.Fatalf("env file after reallocate: %v", err)
	} else if want := fmt.Sprintf("PORT=%d", re.Services["web"].Port); !strings.Contains(string(data), want) {
		t.Fatalf(".env.local = %q, want %s", data, want)
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
	type outcome struct {
		result application.EnsureResult
		err    error
	}
	start := make(chan struct{})
	done := make(chan outcome, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			result, err := app.Ensure(context.Background(), wt)
			done <- outcome{result: result, err: err}
		}()
	}
	close(start)
	results := make([]outcome, 0, 2)
	for i := 0; i < 2; i++ {
		out := <-done
		if out.err != nil {
			t.Fatal(out.err)
		}
		results = append(results, out)
	}
	first, second := results[0].result.Services["web"], results[1].result.Services["web"]
	if first.Port != second.Port {
		t.Fatalf("concurrent ensures returned ports %d and %d", first.Port, second.Port)
	}
	reg, err := (registry.Store{Path: regPath}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Worktrees) != 1 {
		t.Fatalf("claims = %d, want 1", len(reg.Worktrees))
	}
	if got := reg.Worktrees[0].Services["web"].Port; got != first.Port {
		t.Fatalf("claim port = %d, want %d", got, first.Port)
	}
}

func TestEnsureRespectsHeldLock(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-q")
	cfg := []byte(`{"version":1,"project":"app","services":{"web":{"mainPort":18110,"startPort":18111}}}`)
	if err := os.WriteFile(filepath.Join(main, "portucca.json"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, main, "add", ".")
	git(t, main, "commit", "-qm", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, main, "worktree", "add", "-q", wt, "-b", "feature")
	regPath := filepath.Join(t.TempDir(), "ports.json")

	unlock, err := platform.Lock(regPath+".lock", time.Second, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unlock() })

	app := newApp(t, regPath)
	app.LockTimeout = 100 * time.Millisecond
	if _, err := app.Ensure(context.Background(), wt); err == nil {
		t.Fatal("ensure succeeded while the registry lock was held")
	} else if e, ok := apperr.As(err); !ok || e.Code != "lock_timeout" {
		t.Fatalf("error = %v, want lock_timeout", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	got, err := app.Ensure(context.Background(), wt)
	if err != nil {
		t.Fatalf("ensure after unlock: %v", err)
	}
	if _, ok := got.Services["web"]; !ok {
		t.Fatalf("services = %+v, want web", got.Services)
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
