package application

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/packagescript"
)

type DevScriptChange struct {
	Path    string `json:"path"`
	Before  string `json:"before"`
	After   string `json:"after"`
	Changed bool   `json:"changed"`
}

func (a *App) PreviewDevWrap(ctx context.Context, worktree string) (DevScriptChange, error) {
	c, err := a.Locator.Detect(worktree)
	if err != nil {
		return DevScriptChange{}, mapProjectErr(err)
	}
	return a.previewDevWrapAt(c.Root)
}

func (a *App) ApplyDevWrap(ctx context.Context, worktree string) (DevScriptChange, error) {
	c, err := a.Locator.Detect(worktree)
	if err != nil {
		return DevScriptChange{}, mapProjectErr(err)
	}
	return a.applyDevWrapAt(c.Root)
}

func (a *App) previewDevWrapAt(root string) (DevScriptChange, error) {
	change, err := packagescript.Preview(filepath.Join(root, "package.json"))
	if err != nil {
		return DevScriptChange{}, mapPackageScriptErr(err)
	}
	return devScriptChange(change), nil
}

func (a *App) applyDevWrapAt(root string) (DevScriptChange, error) {
	change, err := packagescript.Apply(filepath.Join(root, "package.json"))
	if err != nil {
		return DevScriptChange{}, mapPackageScriptErr(err)
	}
	return devScriptChange(change), nil
}

func devScriptChange(change packagescript.Change) DevScriptChange {
	return DevScriptChange{
		Path: change.Path, Before: change.Before, After: change.After, Changed: change.Changed,
	}
}

func mapPackageScriptErr(err error) error {
	switch {
	case errors.Is(err, packagescript.ErrMissing):
		return apperr.New("package_script_missing", "package.json or scripts.dev was not found", "add a dev script or skip the wrap")
	case errors.Is(err, packagescript.ErrInvalid):
		return apperr.New("package_script_invalid", "package.json is not valid", "fix package.json and retry")
	case errors.Is(err, packagescript.ErrUnsupported):
		return apperr.New("package_script_unsupported", "scripts.dev cannot be wrapped automatically", "wrap the command manually or skip the wrap")
	default:
		return apperr.New("package_script_write", err.Error(), "check permissions and retry")
	}
}
