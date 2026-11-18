package application

import (
	"context"
	"os"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/config"
	"github.com/gomezv/portucca/internal/project"
)

type ApplyInitOptions struct {
	Force      bool
	WrapScript bool
}

func (a *App) ApplyInit(ctx context.Context, proposal InitProposal, opts ApplyInitOptions) (InitResult, error) {
	if err := ctx.Err(); err != nil {
		return InitResult{}, err
	}
	if err := proposal.Config.Validate(); err != nil {
		return InitResult{}, apperr.New("config_invalid", err.Error(), "check the proposed config")
	}
	if _, err := os.Stat(proposal.ConfigPath); err == nil && !opts.Force {
		return InitResult{}, apperr.New("config_exists", "portucca.json already exists", "use --force to overwrite it")
	}
	var preview *DevScriptChange
	if changed, err := a.previewDevWrapAt(proposal.Root); err == nil {
		preview = &changed
	} else if opts.WrapScript {
		return InitResult{}, err
	}
	warning := a.claimsWarning(proposal.Config.Project, proposal.Root)
	if err := config.WriteFile(proposal.ConfigPath, proposal.Config); err != nil {
		return InitResult{}, apperr.New("config_invalid", err.Error(), "check permissions")
	}
	c := project.Context{Root: proposal.Root, MainRoot: proposal.MainRoot, IsMain: proposal.IsMain}
	if err := a.registerMain(c, proposal.Config, proposal.ConfigPath); err != nil {
		return InitResult{}, err
	}
	result := InitResult{
		ConfigPath: proposal.ConfigPath,
		Project:    proposal.Config.Project,
		Services:   proposal.Config.Services,
		Warning:    warning,
		Scan:       &InitScan{Sources: proposal.Sources, Warnings: proposal.Warnings},
	}
	if opts.WrapScript && preview != nil && preview.Changed {
		applied, err := a.applyDevWrapAt(proposal.Root)
		if err != nil {
			return InitResult{}, err
		}
		result.PackageScript = &applied
		return result, nil
	}
	if preview != nil {
		preview.Changed = false
		result.PackageScript = preview
	}
	return result, nil
}
