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
		Mains: []domain.MainReservation{{
			Project:   "app",
			Path:      "/main",
			Ports:     map[domain.ServiceName]domain.Port{"web": 5173},
			UpdatedAt: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC),
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
		t.Fatalf("worktrees = %+v", out.Worktrees)
	}
	if len(out.Mains) != 1 || out.Mains[0].Ports["web"] != 5173 {
		t.Fatalf("mains = %+v", out.Mains)
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
	if err := os.WriteFile(path, []byte(`{"version":3,"worktrees":[],"mains":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrVersion) {
		t.Fatalf("err = %v, want ErrVersion", err)
	}
}

func TestLoadVersionOneNormalizesMains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"worktrees":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := (Store{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if reg.Version != 1 || reg.Mains == nil || len(reg.Mains) != 0 {
		t.Fatalf("registry = %+v", reg)
	}
}

func TestLoadVersionTwoRequiresMains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"worktrees":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}

func TestLoadVersionOneRejectsMains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"worktrees":[],"mains":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}

func TestLoadRejectsNullMains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"worktrees":[],"mains":null}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}

func TestLoadRejectsInvalidMainReservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	raw := `{"version":2,"worktrees":[],"mains":[{"project":"","path":"/main","ports":{"web":5173}}]}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}

func TestSaveSortsMains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	in := domain.Registry{Mains: []domain.MainReservation{
		{Project: "beta", Path: "/b", Ports: map[domain.ServiceName]domain.Port{"web": 5002}},
		{Project: "alpha", Path: "/z", Ports: map[domain.ServiceName]domain.Port{"web": 5003}},
		{Project: "alpha", Path: "/a", Ports: map[domain.ServiceName]domain.Port{"web": 5004}},
	}}
	if err := (Store{Path: path}).Save(in); err != nil {
		t.Fatal(err)
	}
	out, err := (Store{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{
		string(out.Mains[0].Project) + " " + out.Mains[0].Path,
		string(out.Mains[1].Project) + " " + out.Mains[1].Path,
		string(out.Mains[2].Project) + " " + out.Mains[2].Path,
	}
	want := []string{"alpha /a", "alpha /z", "beta /b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
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

func TestSaveWritesVersionTwo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := (Store{Path: path}).Save(domain.Registry{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if got := string(raw["version"]); got != "2" {
		t.Fatalf("version = %s, want 2", got)
	}
	if _, ok := raw["mains"]; !ok {
		t.Fatal("mains missing")
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

func TestLoadMissingWorktreesFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}

func TestLoadNullWorktreesFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"worktrees":null}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}

func TestLoadEmptyWorktreesSucceeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"worktrees":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	reg, err := (Store{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if reg.Version != 1 || len(reg.Worktrees) != 0 || len(reg.Mains) != 0 {
		t.Fatalf("registry = %+v", reg)
	}
}

func TestLoadTrailingDataFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"worktrees":[]} trailing`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}

func TestLoadSecondDocumentFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	data := `{"version":1,"worktrees":[]}{"version":1,"worktrees":[]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}

func TestLoadUnknownFieldFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"worktrees":[],"extra":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := (Store{Path: path}).Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("err = %v, want ErrCorrupt", err)
	}
}

func TestSaveEmptyWritesArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	if err := (Store{Path: path}).Save(domain.Registry{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if got := string(raw["worktrees"]); got != "[]" {
		t.Fatalf("worktrees = %s, want []", got)
	}
	if got := string(raw["mains"]); got != "[]" {
		t.Fatalf("mains = %s, want []", got)
	}
}

func TestSaveDoesNotReorderCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	in := domain.Registry{
		Version: domain.RegistryVersion,
		Worktrees: []domain.Claim{
			{Project: "beta", Path: "/b", Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5002}}},
			{Project: "alpha", Path: "/a", Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5003}}},
		},
	}
	if err := (Store{Path: path}).Save(in); err != nil {
		t.Fatal(err)
	}
	if in.Worktrees[0].Project != "beta" || in.Worktrees[1].Project != "alpha" {
		t.Fatalf("caller slice reordered: %+v", in.Worktrees)
	}
}

func TestSaveNormalizesUpdatedAtUTC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ports.json")
	updated := time.Date(2026, 10, 24, 12, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60))
	in := domain.Registry{
		Version: domain.RegistryVersion,
		Worktrees: []domain.Claim{{
			Project:   "app",
			Path:      "/worktree",
			Services:  map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 5174}},
			UpdatedAt: updated,
		}},
	}
	if err := (Store{Path: path}).Save(in); err != nil {
		t.Fatal(err)
	}
	out, err := (Store{Path: path}).Load()
	if err != nil {
		t.Fatal(err)
	}
	got := out.Worktrees[0].UpdatedAt
	if got.Location() != time.UTC {
		t.Fatalf("location = %v, want UTC", got.Location())
	}
	if !got.Equal(updated) {
		t.Fatalf("updatedAt = %v, want %v", got, updated)
	}
}
