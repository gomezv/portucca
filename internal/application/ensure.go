package application

import (
	"context"
	"fmt"
	"os"

	"github.com/gomezv/portucca/internal/allocation"
	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/envfile"
	"github.com/gomezv/portucca/internal/platform"
	"github.com/gomezv/portucca/internal/project"
	"github.com/gomezv/portucca/internal/registry"
)

type EnsureResult struct {
	Project      domain.Project                             `json:"project"`
	WorktreePath string                                     `json:"worktreePath"`
	ConfigPath   string                                     `json:"configPath"`
	IsMain       bool                                       `json:"isMain"`
	Changed      bool                                       `json:"changed"`
	Services     map[domain.ServiceName]domain.ServiceClaim `json:"services"`
	Env          []EnvValue                                 `json:"env"`
}

func (a *App) Ensure(ctx context.Context, worktree string) (EnsureResult, error) {
	c, err := a.Locator.Detect(worktree)
	if err != nil {
		return EnsureResult{}, mapProjectErr(err)
	}
	cfg, cfgPath, err := a.loadConfigFor(c)
	if err != nil {
		return EnsureResult{}, mapConfigErr(err)
	}
	if c.IsMain {
		return a.ensureMain(c, cfg, cfgPath)
	}
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), false)
	if err != nil {
		return EnsureResult{}, mapLockErr(err)
	}
	defer unlock()
	return a.ensureLocked(c, cfg, cfgPath)
}

func (a *App) ensureMain(c project.Context, cfg domain.Config, cfgPath string) (EnsureResult, error) {
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), false)
	if err != nil {
		return EnsureResult{}, mapLockErr(err)
	}
	defer unlock()
	reg, err := a.Registry.Load()
	if err != nil {
		return EnsureResult{}, mapRegistryErr(err)
	}
	reg, removedClaims, removedMains := pruneStale(reg, os.Stat)
	reg, reserved := upsertMainReservation(reg, cfg, c.Root, a.now())
	changed := reserved || len(removedClaims) > 0 || len(removedMains) > 0
	if changed {
		if err := a.Registry.Save(reg); err != nil {
			return EnsureResult{}, mapRegistryErr(err)
		}
	}
	res := mainEnsureResult(c, cfg, cfgPath)
	res.Changed = changed
	return res, nil
}

func (a *App) registerMain(c project.Context, cfg domain.Config, cfgPath string) error {
	if !c.IsMain {
		return nil
	}
	if _, err := a.ensureMain(c, cfg, cfgPath); err != nil {
		if ae, ok := apperr.As(err); ok {
			hint := ae.Hint
			if hint == "" {
				hint = "config was written; run portucca ensure in the main checkout"
			}
			return apperr.New(ae.Code, ae.Message, hint)
		}
		return err
	}
	return nil
}

func (a *App) ensureLocked(c project.Context, cfg domain.Config, cfgPath string) (EnsureResult, error) {
	reg, err := a.Registry.Load()
	if err != nil {
		return EnsureResult{}, mapRegistryErr(err)
	}
	reg, removedClaims, removedMains := pruneStale(reg, os.Stat)
	own := findClaim(reg, c.Root)
	if own != nil && own.Project != cfg.Project {
		return EnsureResult{}, apperr.New(
			"claim_project_mismatch",
			fmt.Sprintf("the claim for %s belongs to project %q, not %q", c.Root, own.Project, cfg.Project),
			"check the registry with portucca doctor and fix or remove the claim",
		)
	}
	existing := map[domain.ServiceName]domain.ServiceClaim{}
	if own != nil {
		for name, sc := range own.Services {
			existing[name] = sc
		}
	}
	res, err := allocation.Plan(allocation.Request{
		Config:   cfg,
		Existing: existing,
		Occupied: occupiedWithReservations(cfg, reg, c.Root),
		Prober:   a.Prober,
		ReadPort: func(ref domain.EnvRef) (domain.Port, bool, error) {
			return a.EnvStore.ReadPort(c.Root, ref)
		},
	})
	if err != nil {
		return EnsureResult{}, mapAllocErr(err)
	}
	values := desiredValues(cfg, res.Services)
	envChanged, err := a.EnvStore.WriteValues(c.Root, values)
	if err != nil {
		return EnsureResult{}, apperr.New("env_write", err.Error(), "check the env file permissions")
	}
	changed := res.Changed || envChanged || len(removedClaims) > 0 || len(removedMains) > 0 || own == nil || claimDiffers(own, res.Services)
	if changed {
		reg = upsertClaim(reg, domain.Claim{
			Project: cfg.Project, Path: c.Root, Services: res.Services, UpdatedAt: a.now(),
		})
		if err := a.Registry.Save(reg); err != nil {
			return EnsureResult{}, mapRegistryErr(err)
		}
	}
	return EnsureResult{
		Project: cfg.Project, WorktreePath: c.Root, ConfigPath: cfgPath, Changed: changed,
		Services: res.Services, Env: desiredEnvFrom(c.Root, values),
	}, nil
}

func mapProjectErr(err error) error {
	switch {
	case errorsIs(err, project.ErrNotRepo):
		return apperr.New("not_a_repo", err.Error(), "run portucca inside a git repository")
	case errorsIs(err, project.ErrGitMissing):
		return apperr.New("git_missing", err.Error(), "install git and retry")
	default:
		return apperr.New("internal", err.Error(), "")
	}
}

func mapConfigErr(err error) error {
	text := err.Error()
	switch {
	case contains(text, "config_missing"):
		return apperr.New("config_missing", text, "run portucca init in the main checkout or add portucca.json")
	case contains(text, "config_version"):
		return apperr.New("config_version", text, "upgrade portucca or fix the version")
	default:
		return apperr.New("config_invalid", text, "check portucca.json")
	}
}

func mapLockErr(err error) error {
	if errorsIs(err, platform.ErrLockTimeout) {
		return apperr.New("lock_timeout", err.Error(), "another portucca process is working; retry")
	}
	if errorsIs(err, platform.ErrUnsupported) {
		return apperr.New("unsupported_platform", err.Error(), "this platform is not supported yet")
	}
	return apperr.New("internal", err.Error(), "")
}

func mapRegistryErr(err error) error {
	switch {
	case errorsIs(err, registry.ErrCorrupt):
		return apperr.New("registry_corrupt", err.Error(), "move the registry aside and retry")
	case errorsIs(err, registry.ErrVersion):
		return apperr.New("registry_version", err.Error(), "upgrade portucca or fix the registry")
	default:
		return apperr.New("internal", err.Error(), "")
	}
}

func mapAllocErr(err error) error {
	var exhausted *allocation.PortExhaustedError
	if errorsAs(err, &exhausted) {
		return apperr.New("port_exhausted", err.Error(), "widen the project band or free ports")
	}
	var duplicate *allocation.DuplicateClaimError
	if errorsAs(err, &duplicate) {
		return apperr.New("duplicate_claim", err.Error(), "check the registry with portucca doctor")
	}
	var adoption *allocation.AdoptionConflictError
	if errorsAs(err, &adoption) {
		return apperr.New("adoption_conflict", err.Error(), "fix the existing ports in the env files")
	}
	var outOfBand *allocation.OutOfBandError
	if errorsAs(err, &outOfBand) {
		return apperr.New("claim_out_of_band", err.Error(), "run portucca reallocate")
	}
	return apperr.New("internal", err.Error(), "")
}

func mapEnvErr(err error) error {
	switch {
	case errorsIs(err, envfile.ErrSourceMissing):
		return apperr.New("bootstrap_source_missing", err.Error(), "fix bootstrap.copyFromMain")
	case errorsIs(err, envfile.ErrSymlinkConflict):
		return apperr.New("symlink_conflict", err.Error(), "check the existing file or symlink")
	default:
		return apperr.New("bootstrap_error", err.Error(), "check bootstrap and permissions")
	}
}
