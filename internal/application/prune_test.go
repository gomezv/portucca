package application

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

type pruneConfig struct{ errs map[string]error }

func (p pruneConfig) LoadRoot(root string) (domain.Config, string, error) {
	return domain.Config{}, filepath.Join(root, "portucca.json"), p.errs[root]
}

func (p pruneConfig) LoadFile(path string) (domain.Config, error) {
	return domain.Config{}, p.errs[path]
}

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
	got, err := app.Prune(context.Background(), "")
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

func TestPruneRemovesDeadMainReservations(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live-main")
	missing := filepath.Join(dir, "missing-main")
	if err := os.Mkdir(live, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 2,
		Mains: []domain.MainReservation{
			{Project: "app", Path: live, Ports: map[domain.ServiceName]domain.Port{"web": 3000}},
			{Project: "app", Path: missing, Ports: map[domain.ServiceName]domain.Port{"web": 3002}},
		},
	}}
	app := &App{Registry: reg, Locker: fakeLocker{}}
	got, err := app.Prune(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RemovedMains) != 1 || got.RemovedMains[0].Path != missing {
		t.Fatalf("removedMains = %+v", got.RemovedMains)
	}
	if len(reg.reg.Mains) != 1 || reg.reg.Mains[0].Path != live || reg.saves != 1 {
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
	got, err := app.Prune(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Removed == nil || len(got.Removed) != 0 {
		t.Fatalf("removed = %#v, want empty non-nil slice", got.Removed)
	}
	if got.RemovedMains == nil || len(got.RemovedMains) != 0 {
		t.Fatalf("removedMains = %#v, want empty non-nil slice", got.RemovedMains)
	}
	if reg.saves != 0 || len(reg.reg.Worktrees) != 1 {
		t.Fatalf("registry = %+v saves=%d", reg.reg, reg.saves)
	}
}

func TestPruneRemovesCurrentMainWithoutConfig(t *testing.T) {
	dir := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 2,
		Mains: []domain.MainReservation{
			{Project: "app", Path: dir, Ports: map[domain.ServiceName]domain.Port{"web": 18120}},
		},
	}}
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir, MainRoot: dir, IsMain: true}},
		Config:   pruneConfig{errs: map[string]error{dir: fs.ErrNotExist}},
		Registry: reg,
		Locker:   fakeLocker{},
	}
	got, err := app.Prune(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RemovedMains) != 1 || got.RemovedMains[0].Path != dir {
		t.Fatalf("removedMains = %+v", got.RemovedMains)
	}
	if len(reg.reg.Mains) != 0 || reg.saves != 1 {
		t.Fatalf("registry = %+v saves=%d", reg.reg, reg.saves)
	}
}

func TestPruneKeepsCurrentMainWithConfig(t *testing.T) {
	dir := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 2,
		Mains: []domain.MainReservation{
			{Project: "app", Path: dir, Ports: map[domain.ServiceName]domain.Port{"web": 18120}},
		},
	}}
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir, MainRoot: dir, IsMain: true}},
		Config:   pruneConfig{},
		Registry: reg,
		Locker:   fakeLocker{},
	}
	got, err := app.Prune(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RemovedMains) != 0 || reg.saves != 0 {
		t.Fatalf("removedMains = %+v saves=%d", got.RemovedMains, reg.saves)
	}
	if len(reg.reg.Mains) != 1 || reg.reg.Mains[0].Path != dir {
		t.Fatalf("registry = %+v", reg.reg)
	}
}

func TestPruneKeepsForeignMainWithoutConfig(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 2,
		Mains: []domain.MainReservation{
			{Project: "other", Path: other, Ports: map[domain.ServiceName]domain.Port{"web": 18122}},
		},
	}}
	app := &App{
		Locator:  fakeLocator{ctx: project.Context{Root: dir, MainRoot: dir, IsMain: true}},
		Config:   pruneConfig{errs: map[string]error{dir: fs.ErrNotExist}},
		Registry: reg,
		Locker:   fakeLocker{},
	}
	got, err := app.Prune(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RemovedMains) != 0 || reg.saves != 0 {
		t.Fatalf("removedMains = %+v saves=%d", got.RemovedMains, reg.saves)
	}
	if len(reg.reg.Mains) != 1 || reg.reg.Mains[0].Path != other {
		t.Fatalf("registry = %+v, want the foreign main kept", reg.reg)
	}
}

func TestPruneUsesExplicitConfig(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		dir := t.TempDir()
		alt := filepath.Join(dir, "alt.json")
		reg := &fakeRegistry{reg: domain.Registry{
			Version: 2,
			Mains: []domain.MainReservation{
				{Project: "app", Path: dir, Ports: map[domain.ServiceName]domain.Port{"web": 18120}},
			},
		}}
		app := &App{
			Locator:    fakeLocator{ctx: project.Context{Root: dir, MainRoot: dir, IsMain: true}},
			Config:     pruneConfig{},
			ConfigFile: alt,
			Registry:   reg,
			Locker:     fakeLocker{},
		}
		got, err := app.Prune(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.RemovedMains) != 0 || len(reg.reg.Mains) != 1 {
			t.Fatalf("removedMains = %+v registry = %+v", got.RemovedMains, reg.reg)
		}
	})

	t.Run("missing", func(t *testing.T) {
		dir := t.TempDir()
		alt := filepath.Join(dir, "alt.json")
		reg := &fakeRegistry{reg: domain.Registry{
			Version: 2,
			Mains: []domain.MainReservation{
				{Project: "app", Path: dir, Ports: map[domain.ServiceName]domain.Port{"web": 18120}},
			},
		}}
		app := &App{
			Locator:    fakeLocator{ctx: project.Context{Root: dir, MainRoot: dir, IsMain: true}},
			Config:     pruneConfig{errs: map[string]error{alt: fs.ErrNotExist}},
			ConfigFile: alt,
			Registry:   reg,
			Locker:     fakeLocker{},
		}
		got, err := app.Prune(context.Background(), dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.RemovedMains) != 1 || len(reg.reg.Mains) != 0 {
			t.Fatalf("removedMains = %+v registry = %+v", got.RemovedMains, reg.reg)
		}
	})
}

func TestPruneDetectionFailureStillPrunesDeadPaths(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 2,
		Mains: []domain.MainReservation{
			{Project: "app", Path: missing, Ports: map[domain.ServiceName]domain.Port{"web": 18120}},
		},
	}}
	app := &App{
		Locator:  fakeLocator{err: errors.New("boom")},
		Config:   pruneConfig{},
		Registry: reg,
		Locker:   fakeLocker{},
	}
	got, err := app.Prune(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RemovedMains) != 1 || got.RemovedMains[0].Path != missing {
		t.Fatalf("removedMains = %+v", got.RemovedMains)
	}
}

func TestPruneInWorktreeKeepsMainReservation(t *testing.T) {
	worktree := t.TempDir()
	main := t.TempDir()
	reg := &fakeRegistry{reg: domain.Registry{
		Version: 2,
		Mains: []domain.MainReservation{
			{Project: "app", Path: main, Ports: map[domain.ServiceName]domain.Port{"web": 18120}},
		},
	}}
	app := &App{
		Locator: fakeLocator{ctx: project.Context{Root: worktree, MainRoot: main, IsMain: false}},
		Config: pruneConfig{errs: map[string]error{
			worktree: fs.ErrNotExist,
			main:     fs.ErrNotExist,
		}},
		Registry: reg,
		Locker:   fakeLocker{},
	}
	got, err := app.Prune(context.Background(), worktree)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.RemovedMains) != 0 || len(reg.reg.Mains) != 1 {
		t.Fatalf("removedMains = %+v registry = %+v", got.RemovedMains, reg.reg)
	}
}
