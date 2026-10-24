package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gomezv/portucca/internal/domain"
)

func TestPruneRemovesMissingPaths(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live")
	missing := filepath.Join(dir, "missing")
	if err := os.Mkdir(live, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 1,
		Worktrees: []domain.Claim{
			{Project: "app", Path: live, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}}},
			{Project: "app", Path: missing, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3002}}},
		},
	}}
	app := &App{Registry: reg, Locker: fakeLocker{}}
	got, err := app.Prune(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Removed) != 1 || got.Removed[0].Path != missing {
		t.Fatalf("removed = %+v", got.Removed)
	}
	if len(reg.reg.Worktrees) != 1 || reg.saves != 1 {
		t.Fatalf("registry = %+v saves=%d", reg.reg, reg.saves)
	}
}

func TestPruneWithoutStaleClaimsDoesNotSave(t *testing.T) {
	dir := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 1,
		Worktrees: []domain.Claim{
			{Project: "app", Path: dir, Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3001}}},
		},
	}}
	app := &App{Registry: reg, Locker: fakeLocker{}}
	got, err := app.Prune(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Removed == nil || len(got.Removed) != 0 {
		t.Fatalf("removed = %#v, want empty non-nil slice", got.Removed)
	}
	if reg.saves != 0 || len(reg.reg.Worktrees) != 1 {
		t.Fatalf("registry = %+v saves=%d", reg.reg, reg.saves)
	}
}
