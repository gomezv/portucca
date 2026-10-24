package registry

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gomezv/portucca/internal/domain"
)

func TestLoadMissingReturnsEmpty(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "ports.json")}
	reg, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reg.Version != domain.RegistryVersion || len(reg.Worktrees) != 0 {
		t.Fatalf("registry = %+v", reg)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	store := Store{Path: path}
	in := domain.Registry{
		Version: domain.RegistryVersion,
		Worktrees: []domain.Claim{{
			Project: "app",
			Path:    "/worktree",
			Services: map[domain.ServiceName]domain.ServiceClaim{
				"web": {Port: 5174},
			},
			UpdatedAt: time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC),
		}},
	}
	if err := store.Save(in); err != nil {
		t.Fatal(err)
	}
	out, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Worktrees) != 1 || out.Worktrees[0].Services["web"].Port != 5174 {
		t.Fatalf("out = %+v", out)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestLoadCorruptFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"extra":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}

func TestDefaultPathUsesXDG(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg")
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := "/tmp/xdg/portucca/ports.json"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLoadVersionMismatchFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"worktrees":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("err = %v, want ErrVersion", err)
	}
}

func TestLoadIncompleteClaimFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	data := `{"version":1,"worktrees":[{"project":"app","path":"/worktree","services":{}}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}

func TestSaveSortsByProjectThenPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	store := Store{Path: path}
	in := domain.Registry{
		Version: domain.RegistryVersion,
		Worktrees: []domain.Claim{
			{Project: "beta", Path: "/b", Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5002}}},
			{Project: "alpha", Path: "/z", Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5003}}},
			{Project: "alpha", Path: "/a", Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5004}}},
		},
	}
	if err := store.Save(in); err != nil {
		t.Fatal(err)
	}
	out, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Worktrees) != 3 {
		t.Fatalf("worktrees = %+v", out.Worktrees)
	}
	got := []string{
		string(out.Worktrees[0].Project) + " " + out.Worktrees[0].Path,
		string(out.Worktrees[1].Project) + " " + out.Worktrees[1].Path,
		string(out.Worktrees[2].Project) + " " + out.Worktrees[2].Path,
	}
	want := []string{"alpha /a", "alpha /z", "beta /b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestSaveCreatesPrivateDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state", "portucca")
	store := Store{Path: filepath.Join(dir, "ports.json")}
	if err := store.Save(domain.Registry{Version: domain.RegistryVersion}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %o, want 700", info.Mode().Perm())
	}
}

func TestSaveReplacesLoosePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"worktrees":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (Store{Path: path}).Save(domain.Registry{Version: domain.RegistryVersion}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}

func TestSaveWritesVersionOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := (Store{Path: path}).Save(domain.Registry{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["version"] != float64(domain.RegistryVersion) {
		t.Fatalf("version = %v", raw["version"])
	}
}

func TestDefaultPathFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/tmp/home")
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := "/tmp/home/.local/state/portucca/ports.json"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
