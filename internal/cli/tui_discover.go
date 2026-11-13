package cli

import (
	"context"

	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/tui"
)

type tuiDiscoverer struct {
	app *application.App
}

func newTUIDiscoverer(app *application.App) tui.Discoverer {
	return tuiDiscoverer{app: app}
}

func (d tuiDiscoverer) Discover(ctx context.Context, project string) (application.DiscoveryResult, error) {
	return d.app.DiscoverWorktrees(ctx, project)
}
