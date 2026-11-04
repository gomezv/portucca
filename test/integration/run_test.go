package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/envfile"
	"github.com/gomezv/portucca/internal/platform"
	"github.com/gomezv/portucca/internal/registry"
)

type fakeExecutor struct {
	path string
	args []string
	env  []string
	err  error
}

func (f *fakeExecutor) Exec(path string, argv, env []string) error {
	f.path, f.args, f.env = path, argv, env
	return f.err
}

const runConfig = `{
  "version": 1,
  "project": "app",
  "services": {"web": {"mainPort": 18080, "startPort": 18081}},
  "env": [{"service": "web", "file": ".env.local", "key": "PORT"}]
}`

const runConflictConfig = `{
  "version": 1,
  "project": "app",
  "services": {
    "web": {"mainPort": 18080, "startPort": 18081},
    "api": {"mainPort": 18090, "startPort": 18091}
  },
  "env": [
    {"service": "web", "file": ".env.local", "key": "PORT"},
    {"service": "api", "file": ".env", "key": "PORT"}
  ]
}`

const runSameServiceConflictConfig = `{
  "version": 1,
  "project": "app",
  "services": {"web": {"mainPort": 18080, "startPort": 18081}},
  "env": [
    {"service": "web", "file": ".env.local", "key": "PORT"},
    {"service": "web", "file": ".env.prod", "key": "PORT", "template": "http://localhost:{{port}}"}
  ]
}`

func buildPortucca(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "portucca")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/portucca")
	cmd.Dir = "../.."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func runRepo(t *testing.T, cfg string) (wt, regPath string) {
	t.Helper()
	main := t.TempDir()
	git(t, main, "init", "-q")
	if err := os.WriteFile(filepath.Join(main, "portucca.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, main, "add", ".")
	git(t, main, "commit", "-qm", "init")
	wt = filepath.Join(t.TempDir(), "wt")
	git(t, main, "worktree", "add", "-q", wt, "-b", "feature")
	regPath = filepath.Join(t.TempDir(), "ports.json")
	return wt, regPath
}

func invokeRun(t *testing.T, bin, dir, regPath string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	full := append([]string{"run", "--registry", regPath}, args...)
	cmd := exec.Command(bin, full...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err == nil {
		return out.String(), errb.String(), 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return out.String(), errb.String(), exitErr.ExitCode()
	}
	t.Fatalf("portucca %v: %v", full, err)
	return "", "", -1
}

func readRunEnv(t *testing.T, root, key string) string {
	t.Helper()
	value, ok, err := envfile.ReadValue(root, domain.EnvRef{File: ".env.local", Key: key})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("%s not found in .env.local", key)
	}
	return value
}

func TestRunBinaryEndToEnd(t *testing.T) {
	bin := buildPortucca(t)
	wt, regPath := runRepo(t, runConfig)

	t.Run("injects env", func(t *testing.T) {
		stdout, stderr, code := invokeRun(t, bin, wt, regPath, "--", "sh", "-c", `printf "%s" "$PORT"`)
		if code != 0 {
			t.Fatalf("exit = %d, stderr = %s", code, stderr)
		}
		if want := readRunEnv(t, wt, "PORT"); stdout != want {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("propagates exit code", func(t *testing.T) {
		_, stderr, code := invokeRun(t, bin, wt, regPath, "--", "sh", "-c", "exit 7")
		if code != 7 {
			t.Fatalf("exit = %d, want 7 (stderr = %s)", code, stderr)
		}
	})

	t.Run("rejects json", func(t *testing.T) {
		stdout, stderr, code := invokeRun(t, bin, wt, regPath, "--json", "--", "sh", "-c", "true")
		if code != 2 {
			t.Fatalf("exit = %d, want 2", code)
		}
		if !strings.Contains(stderr, "error:") {
			t.Fatalf("stderr = %q, want a human error", stderr)
		}
		if strings.Contains(stdout, "schemaVersion") || strings.Contains(stderr, "schemaVersion") {
			t.Fatalf("stdout = %q, stderr = %q, want no JSON envelope", stdout, stderr)
		}
	})

	t.Run("requires separator", func(t *testing.T) {
		_, stderr, code := invokeRun(t, bin, wt, regPath, "sh", "-c", "true")
		if code != 2 {
			t.Fatalf("exit = %d, want 2", code)
		}
		if !strings.Contains(stderr, "run: missing -- before the command") {
			t.Fatalf("stderr = %q, want the missing separator message", stderr)
		}
	})

	t.Run("rejects a separator after the command", func(t *testing.T) {
		_, stderr, code := invokeRun(t, bin, wt, regPath, "sh", "-c", "true", "--")
		if code != 2 {
			t.Fatalf("exit = %d, want 2", code)
		}
		if !strings.Contains(stderr, "run: missing -- before the command") {
			t.Fatalf("stderr = %q, want the missing separator message", stderr)
		}
	})

	t.Run("rejects unknown service", func(t *testing.T) {
		_, stderr, code := invokeRun(t, bin, wt, regPath, "--service", "nope", "--", "sh", "-c", "true")
		if code != 2 {
			t.Fatalf("exit = %d, want 2", code)
		}
		if !strings.Contains(stderr, "unknown service") {
			t.Fatalf("stderr = %q, want unknown service", stderr)
		}
	})
}

func TestAppRunUsesExecutor(t *testing.T) {
	wt, regPath := runRepo(t, runConfig)
	app := newApp(t, regPath)
	executor := &fakeExecutor{}
	app.Executor = executor

	err := app.Run(context.Background(), application.RunRequest{
		Worktree: wt,
		Command:  []string{"sh", "-c", "true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantPath, err := exec.LookPath("sh")
	if err != nil {
		t.Fatal(err)
	}
	if executor.path != wantPath {
		t.Fatalf("path = %q, want %q", executor.path, wantPath)
	}
	if !reflect.DeepEqual(executor.args, []string{"sh", "-c", "true"}) {
		t.Fatalf("args = %v", executor.args)
	}
	want := "PORT=" + readRunEnv(t, wt, "PORT")
	for _, kv := range executor.env {
		if kv == want {
			return
		}
	}
	t.Fatalf("env is missing %q", want)
}

func TestRunBinaryEnvConflict(t *testing.T) {
	bin := buildPortucca(t)
	wt, regPath := runRepo(t, runConflictConfig)

	app := newApp(t, regPath)
	app.Executor = &fakeExecutor{}
	err := app.Run(context.Background(), application.RunRequest{
		Worktree: wt,
		Command:  []string{"sh", "-c", "true"},
	})
	if e, ok := apperr.As(err); !ok || e.Code != "env_conflict" {
		t.Fatalf("err = %v, want env_conflict", err)
	}

	resolvedWT, err := platform.RealPath(wt)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := (registry.Store{Path: regPath}).Load()
	if err != nil {
		t.Fatal(err)
	}
	webPort := domain.Port(0)
	for _, claim := range reg.Worktrees {
		if claim.Path == resolvedWT {
			webPort = claim.Services["web"].Port
		}
	}
	if webPort == 0 {
		t.Fatalf("no claim for %s", resolvedWT)
	}

	stdout, stderr, code := invokeRun(t, bin, wt, regPath, "--service", "web", "--", "sh", "-c", `printf "%s" "$PORT"`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if want := fmt.Sprintf("%d", webPort); stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestRunBinaryEnvConflictSameService(t *testing.T) {
	bin := buildPortucca(t)
	wt, regPath := runRepo(t, runSameServiceConflictConfig)

	app := newApp(t, regPath)
	app.Executor = &fakeExecutor{}
	err := app.Run(context.Background(), application.RunRequest{
		Worktree: wt,
		Command:  []string{"sh", "-c", "true"},
	})
	e, ok := apperr.As(err)
	if !ok || e.Code != "env_conflict" {
		t.Fatalf("err = %v, want env_conflict", err)
	}
	if !strings.Contains(e.Hint, "portucca.json") || strings.Contains(e.Hint, "--service") {
		t.Fatalf("hint = %q, want portucca.json without --service", e.Hint)
	}

	_, stderr, code := invokeRun(t, bin, wt, regPath, "--", "sh", "-c", "true")
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (stderr = %s)", code, stderr)
	}
	if !strings.Contains(stderr, "portucca.json") || strings.Contains(stderr, "--service") {
		t.Fatalf("stderr = %q, want portucca.json without --service", stderr)
	}
}
