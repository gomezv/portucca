package application

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
)

func noPrune(context.Context) (PruneResult, error) {
	return PruneResult{Removed: []domain.Claim{}, RemovedMains: []domain.MainReservation{}}, nil
}

func TestWatchOnce(t *testing.T) {
	result := DiscoveryResult{Worktrees: []DiscoveredWorktree{{Project: "app", Path: "/repo/wt"}}}
	calls := 0
	discover := func(context.Context) (DiscoveryResult, error) {
		calls++
		if calls == 1 {
			return result, nil
		}
		return DiscoveryResult{}, nil
	}
	var prepared []string
	setup := func(_ context.Context, path string) (SetupResult, error) {
		prepared = append(prepared, path)
		return SetupResult{EnsureResult: EnsureResult{Services: map[domain.ServiceName]domain.ServiceClaim{"web": {Port: 3002}}}}, nil
	}
	var out bytes.Buffer
	first, err := watchLoop(context.Background(), WatchRequest{Once: true, Out: &out}, discover, setup, noPrune, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Setups != 1 || first.Failed != 0 || len(prepared) != 1 {
		t.Fatalf("first = %+v prepared=%v", first, prepared)
	}
	out.Reset()
	second, err := watchLoop(context.Background(), WatchRequest{Once: true, Out: &out}, discover, setup, noPrune, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Setups != 0 || strings.Contains(out.String(), "setup ok") {
		t.Fatalf("second = %+v out=%q", second, out.String())
	}
}

func TestWatchFailure(t *testing.T) {
	result := DiscoveryResult{Worktrees: []DiscoveredWorktree{
		{Project: "app", Path: "/repo/bad"},
		{Project: "app", Path: "/repo/good"},
	}}
	discover := func(context.Context) (DiscoveryResult, error) { return result, nil }
	setup := func(_ context.Context, path string) (SetupResult, error) {
		if path == "/repo/bad" {
			return SetupResult{}, apperr.New("config_invalid", "config is invalid", "fix portucca.json")
		}
		return SetupResult{}, nil
	}
	var out bytes.Buffer
	got, err := watchCycle(context.Background(), discover, setup, noPrune, &out, map[string]time.Time{}, time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Setups != 1 || got.Failed != 1 {
		t.Fatalf("result = %+v", got)
	}
	for _, want := range []string{"config_invalid", "fix portucca.json"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("log missing %q: %s", want, out.String())
		}
	}
}

func TestWatchRetry(t *testing.T) {
	result := DiscoveryResult{Worktrees: []DiscoveredWorktree{{Project: "app", Path: "/repo/wt"}}}
	discover := func(context.Context) (DiscoveryResult, error) { return result, nil }
	setups := 0
	setup := func(context.Context, string) (SetupResult, error) {
		setups++
		return SetupResult{}, errors.New("boom")
	}
	failures := map[string]time.Time{}
	now := time.Unix(1000, 0)
	if _, err := watchCycle(context.Background(), discover, setup, noPrune, &bytes.Buffer{}, failures, time.Minute, now); err != nil {
		t.Fatal(err)
	}
	if _, err := watchCycle(context.Background(), discover, setup, noPrune, &bytes.Buffer{}, failures, time.Minute, now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if setups != 1 {
		t.Fatalf("setups = %d, want the retry skipped", setups)
	}
	if _, err := watchCycle(context.Background(), discover, setup, noPrune, &bytes.Buffer{}, failures, time.Minute, now.Add(61*time.Second)); err != nil {
		t.Fatal(err)
	}
	if setups != 2 {
		t.Fatalf("setups = %d, want a retry", setups)
	}
}

func TestWatchLoopCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	discover := func(context.Context) (DiscoveryResult, error) { return DiscoveryResult{}, nil }
	setup := func(context.Context, string) (SetupResult, error) { return SetupResult{}, nil }
	got, err := watchLoop(ctx, WatchRequest{Interval: time.Second, Retry: time.Minute}, discover, setup, noPrune, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Setups != 0 || got.Failed != 0 {
		t.Fatalf("result = %+v", got)
	}
}

func TestWatchCycleLogsDiscoveryIssues(t *testing.T) {
	discover := func(context.Context) (DiscoveryResult, error) {
		return DiscoveryResult{Issues: []DiscoveryIssue{{Project: "app", MainRoot: "/repo/main", Code: "not_a_repo", Message: "gone"}}}, nil
	}
	setup := func(context.Context, string) (SetupResult, error) {
		t.Fatal("setup should not run without worktrees")
		return SetupResult{}, nil
	}
	var out bytes.Buffer
	got, err := watchCycle(context.Background(), discover, setup, noPrune, &out, map[string]time.Time{}, time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Setups != 0 || !strings.Contains(out.String(), "not_a_repo") || !strings.Contains(out.String(), "gone") {
		t.Fatalf("result = %+v out=%q", got, out.String())
	}
}

func TestWatchPruneStale(t *testing.T) {
	discover := func(context.Context) (DiscoveryResult, error) { return DiscoveryResult{}, nil }
	setup := func(context.Context, string) (SetupResult, error) {
		t.Fatal("setup should not run with no discovered worktrees")
		return SetupResult{}, nil
	}
	prune := func(context.Context) (PruneResult, error) {
		return PruneResult{Removed: []domain.Claim{{Project: "app", Path: "/repo/gone"}}}, nil
	}
	var out bytes.Buffer
	got, err := watchCycle(context.Background(), discover, setup, prune, &out, map[string]time.Time{}, time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Pruned != 1 || !strings.Contains(out.String(), "app /repo/gone: pruned stale claim (path missing)") {
		t.Fatalf("result = %+v out=%q", got, out.String())
	}
}

func TestPruneStaleClaimsIgnoresOtherErrors(t *testing.T) {
	reg := &fakeRegistry{reg: domain.Registry{
		Version:   2,
		Worktrees: []domain.Claim{{Project: "app", Path: "/repo/live"}},
	}}
	app := &App{Registry: reg, Locker: fakeLocker{}}
	got, err := app.PruneStaleClaims(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = got
	// Use the filter directly with a non-ENOENT error.
	reg.reg.Worktrees = []domain.Claim{{Project: "app", Path: "/repo/live"}}
	filtered, removed := pruneStaleClaims(reg.reg, func(string) (os.FileInfo, error) {
		return nil, fs.ErrPermission
	})
	if len(removed) != 0 || len(filtered.Worktrees) != 1 {
		t.Fatalf("removed = %+v worktrees = %+v", removed, filtered.Worktrees)
	}
}

func TestWatchPruneErrorPropagates(t *testing.T) {
	discover := func(context.Context) (DiscoveryResult, error) {
		t.Fatal("discovery should not run after a prune error")
		return DiscoveryResult{}, nil
	}
	setup := func(context.Context, string) (SetupResult, error) {
		t.Fatal("setup should not run after a prune error")
		return SetupResult{}, nil
	}
	prune := func(context.Context) (PruneResult, error) {
		return PruneResult{}, apperr.New("lock_timeout", "lock timed out", "retry")
	}
	_, err := watchCycle(context.Background(), discover, setup, prune, &bytes.Buffer{}, map[string]time.Time{}, time.Minute, time.Now())
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "lock_timeout" {
		t.Fatalf("err = %v, want lock_timeout", err)
	}
}

func TestPruneStaleClaimsKeepsMains(t *testing.T) {
	missingMain := filepath.Join(t.TempDir(), "gone-main")
	missing := filepath.Join(t.TempDir(), "gone")
	reg := &fakeRegistry{reg: domain.Registry{
		Version:   2,
		Worktrees: []domain.Claim{{Project: "app", Path: missing}},
		Mains:     []domain.MainReservation{{Project: "app", Path: missingMain}},
	}}
	app := &App{Registry: reg, Locker: fakeLocker{}}
	got, err := app.PruneStaleClaims(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Removed) != 1 || len(got.RemovedMains) != 0 {
		t.Fatalf("result = %+v", got)
	}
	if reg.reg.Mains[0].Path != missingMain {
		t.Fatalf("mains = %+v, want the missing main kept", reg.reg.Mains)
	}
	if len(reg.reg.Worktrees) != 0 || len(reg.reg.Mains) != 1 || reg.saves != 1 {
		t.Fatalf("registry = %+v saves=%d", reg.reg, reg.saves)
	}
}
