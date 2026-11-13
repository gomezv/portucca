package application

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/gomezv/portucca/internal/apperr"
)

type WatchRequest struct {
	Interval time.Duration
	Retry    time.Duration
	Once     bool
	Out      io.Writer
}

type WatchResult struct {
	Setups int `json:"setups"`
	Failed int `json:"failed"`
	Pruned int `json:"pruned"`
}

type watchDiscover func(context.Context) (DiscoveryResult, error)
type watchSetup func(context.Context, string) (SetupResult, error)
type watchPrune func(context.Context) (PruneResult, error)

func (a *App) Watch(ctx context.Context, req WatchRequest) (WatchResult, error) {
	if req.Out == nil {
		req.Out = io.Discard
	}
	discover := func(ctx context.Context) (DiscoveryResult, error) {
		return a.DiscoverWorktrees(ctx, "")
	}
	setup := func(ctx context.Context, path string) (SetupResult, error) {
		return a.Setup(ctx, path)
	}
	prune := func(ctx context.Context) (PruneResult, error) {
		return a.PruneStaleClaims(ctx)
	}
	return watchLoop(ctx, req, discover, setup, prune, nil)
}

func watchLoop(ctx context.Context, req WatchRequest, discover watchDiscover, setup watchSetup, prune watchPrune, ticks <-chan time.Time) (WatchResult, error) {
	failures := map[string]time.Time{}
	if req.Once {
		return watchCycle(ctx, discover, setup, prune, req.Out, failures, req.Retry, time.Now())
	}
	var total WatchResult
	var ticker *time.Ticker
	ch := ticks
	if ch == nil {
		ticker = time.NewTicker(req.Interval)
		ch = ticker.C
		defer ticker.Stop()
	}
	for {
		result, err := watchCycle(ctx, discover, setup, prune, req.Out, failures, req.Retry, time.Now())
		if err != nil {
			return total, err
		}
		total.Setups += result.Setups
		total.Failed += result.Failed
		total.Pruned += result.Pruned
		select {
		case <-ctx.Done():
			return total, nil
		case <-ch:
		}
	}
}

func watchCycle(ctx context.Context, discover watchDiscover, setup watchSetup, prune watchPrune, out io.Writer, failures map[string]time.Time, retry time.Duration, now time.Time) (WatchResult, error) {
	pruned, err := prune(ctx)
	if err != nil {
		return WatchResult{}, err
	}
	var total WatchResult
	for _, claim := range pruned.Removed {
		total.Pruned++
		fmt.Fprintf(out, "%s %s: pruned stale claim (path missing)\n", claim.Project, claim.Path)
	}
	result, err := discover(ctx)
	if err != nil {
		return WatchResult{}, err
	}
	for _, issue := range result.Issues {
		fmt.Fprintf(out, "%s %s: error %s: %s\n", issue.Project, issue.MainRoot, issue.Code, issue.Message)
	}
	for _, wt := range result.Worktrees {
		if when, ok := failures[wt.Path]; ok && retry > 0 && now.Sub(when) < retry {
			continue
		}
		prepared, err := setup(ctx, wt.Path)
		if err != nil {
			total.Failed++
			failures[wt.Path] = now
			fmt.Fprintf(out, "%s %s: error %s\n", wt.Project, wt.Path, watchErrorText(err))
			continue
		}
		total.Setups++
		delete(failures, wt.Path)
		fmt.Fprintf(out, "%s %s: setup ok (%d services)\n", wt.Project, wt.Path, len(prepared.Services))
	}
	return total, nil
}

func watchErrorText(err error) string {
	if ae, ok := apperr.As(err); ok {
		if ae.Hint != "" {
			return ae.Code + ": " + ae.Message + " (" + ae.Hint + ")"
		}
		return ae.Code + ": " + ae.Message
	}
	return err.Error()
}
