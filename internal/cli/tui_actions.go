package cli

import (
	"context"
	"fmt"

	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/tui"
)

type tuiApp interface {
	Ensure(context.Context, string) (application.EnsureResult, error)
	Setup(context.Context, string) (application.SetupResult, error)
	Reallocate(context.Context, string) (application.EnsureResult, error)
	Release(context.Context, string, domain.ServiceName) (application.ReleaseResult, error)
	Prune(context.Context, string) (application.PruneResult, error)
}

type tuiActions struct {
	app tuiApp
}

func newTUIActions(app *application.App) tui.ActionRunner {
	copied := *app
	copied.ConfigFile = ""
	return tuiActions{app: &copied}
}

func (a tuiActions) Run(ctx context.Context, action tui.Action, worktree string) (string, error) {
	switch action {
	case tui.ActionEnsure:
		res, err := a.app.Ensure(ctx, worktree)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("ensure: changed=%t services=%d", res.Changed, len(res.Services)), nil
	case tui.ActionSetup:
		res, err := a.app.Setup(ctx, worktree)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("setup: copied=%d symlinked=%d skipped=%d services=%d",
			len(res.Bootstrap.Copied), len(res.Bootstrap.Symlinked), len(res.Bootstrap.Skipped), len(res.Services)), nil
	case tui.ActionReallocate:
		res, err := a.app.Reallocate(ctx, worktree)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("reallocate: services=%d changed=%t", len(res.Services), res.Changed), nil
	case tui.ActionRelease:
		res, err := a.app.Release(ctx, worktree, domain.ServiceName(""))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("release: released=%d", len(res.Released)), nil
	case tui.ActionPrune:
		res, err := a.app.Prune(ctx, worktree)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("prune: removed=%d mains=%d", len(res.Removed), len(res.RemovedMains)), nil
	default:
		return "", fmt.Errorf("unknown action %q", action)
	}
}

type tuiWrapper struct {
	app *application.App
}

func newTUIWrapper(app *application.App) tui.DevWrapper {
	copied := *app
	copied.ConfigFile = ""
	return tuiWrapper{app: &copied}
}

func (w tuiWrapper) Preview(ctx context.Context, worktree string) (application.DevScriptChange, error) {
	return w.app.PreviewDevWrap(ctx, worktree)
}

func (w tuiWrapper) Apply(ctx context.Context, worktree string) (application.DevScriptChange, error) {
	return w.app.ApplyDevWrap(ctx, worktree)
}

type tuiRunner struct {
	app *application.App
}

func newTUIRunner(app *application.App) tui.RunPreparer {
	copied := *app
	copied.ConfigFile = ""
	return tuiRunner{app: &copied}
}

func (r tuiRunner) Prepare(ctx context.Context, worktree string, command []string) (application.RunPlan, error) {
	return r.app.PrepareRun(ctx, application.RunRequest{Worktree: worktree, Command: command})
}
