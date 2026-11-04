package integration

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/envfile"
)

func TestSetupUsesMainConfigWhenWorktreeHasNoConfig(t *testing.T) {
	main := t.TempDir()
	git(t, main, "init", "-q")
	if err := os.WriteFile(filepath.Join(main, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, main, "add", "README.md")
	git(t, main, "commit", "-qm", "init")

	wt := filepath.Join(t.TempDir(), "wt")
	git(t, main, "worktree", "add", "-q", wt, "-b", "feature")

	cfg := []byte(`{
  "version": 1,
  "project": "fallback",
  "services": {"web": {"mainPort": 18080, "startPort": 18081}},
  "env": [{"service": "web", "file": ".env.local", "key": "PORT"}]
}`)
	if err := os.WriteFile(filepath.Join(main, "portucca.json"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}

	app := newApp(t, filepath.Join(t.TempDir(), "ports.json"))
	res, err := app.Setup(context.Background(), wt)
	if err != nil {
		t.Fatal(err)
	}
	mainReal, err := filepath.EvalSymlinks(main)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(mainReal, "portucca.json"); res.ConfigPath != want {
		t.Fatalf("configPath = %q, want %q", res.ConfigPath, want)
	}
	if res.Services["web"].Port < 18081 {
		t.Fatalf("port = %d, want >= 18081", res.Services["web"].Port)
	}
	value, ok, err := envfile.ReadValue(wt, domain.EnvRef{File: ".env.local", Key: "PORT"})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != strconv.Itoa(int(res.Services["web"].Port)) {
		t.Fatalf("env PORT = %q (ok=%v), want %d", value, ok, res.Services["web"].Port)
	}
}
