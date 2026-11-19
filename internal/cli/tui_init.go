package cli

import (
	"context"
	"os"

	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/tui"
)

type tuiInitApp interface {
	InspectInitPath(context.Context, string) (application.InitPath, error)
	AnalyzeInit(context.Context, application.InitRequest) (application.InitProposal, error)
	ApplyInit(context.Context, application.InitProposal, application.ApplyInitOptions) (application.InitResult, error)
}

type tuiInit struct{ app tuiInitApp }

func newTUIInit(app *application.App) tui.InitFlow {
	copied := *app
	copied.ConfigFile = ""
	return tuiInit{app: &copied}
}

func (i tuiInit) Inspect(ctx context.Context, path string) (application.InitPath, error) {
	return i.app.InspectInitPath(ctx, path)
}

func (i tuiInit) Analyze(ctx context.Context, path string, services []string, env []application.EnvBinding) (application.InitProposal, error) {
	return i.app.AnalyzeInit(ctx, application.InitRequest{Scan: true, Worktree: path, Services: services, Env: env})
}

func (i tuiInit) Apply(ctx context.Context, proposal application.InitProposal, force bool) (application.InitResult, error) {
	return i.app.ApplyInit(ctx, proposal, application.ApplyInitOptions{Force: force})
}

func tuiInitOptions(app *application.App, worktree string, worktreeSet bool) tui.InitOptions {
	def := worktree
	if def == "" {
		def = "."
	}
	if !worktreeSet {
		if cwd, err := os.Getwd(); err == nil {
			def = cwd
		}
	}
	return tui.InitOptions{DefaultPath: def, Flow: newTUIInit(app)}
}
