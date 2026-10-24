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
	cfg, _, err := a.loadConfig(c.Root)
	if err != nil {
		return EnsureResult{}, mapConfigErr(err)
	}
	if c.IsMain {
		services := mainServices(cfg)
		return EnsureResult{
			Project: cfg.Project, WorktreePath: c.Root, IsMain: true,
			Services: services, Env: desiredEnv(c.Root, cfg, services),
		}, nil
	}
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), false)
	if err != nil {
		return EnsureResult{}, mapLockErr(err)
	}
	defer unlock()
	return a.ensureLocked(c, cfg)
}

func (a *App) ensureLocked(c project.Context, cfg domain.Config) (EnsureResult, error) {
	reg, err := a.Registry.Load()
	if err != nil {
		return EnsureResult{}, mapRegistryErr(err)
	}
	reg, removed := pruneStale(reg, os.Stat)
	own := findClaim(reg, c.Root)
	if own != nil && own.Project != cfg.Project {
		return EnsureResult{}, apperr.New(
			"claim_project_mismatch",
			fmt.Sprintf("el claim de %s pertenece al proyecto %q, no a %q", c.Root, own.Project, cfg.Project),
			"revisa el registro con portucca doctor y corrige o elimina el claim",
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
		Occupied: occupiedFrom(reg, c.Root),
		Prober:   a.Prober,
		ReadPort: func(ref domain.EnvRef) (domain.Port, bool, error) {
			return a.Env.ReadPort(c.Root, ref)
		},
	})
	if err != nil {
		return EnsureResult{}, mapAllocErr(err)
	}
	envChanged, err := a.Env.WriteValues(c.Root, desiredValues(cfg, res.Services))
	if err != nil {
		return EnsureResult{}, apperr.New("env_write", err.Error(), "revisa los permisos de los archivos de entorno")
	}
	changed := res.Changed || envChanged || len(removed) > 0 || own == nil || claimDiffers(own, res.Services)
	if changed {
		reg = upsertClaim(reg, domain.Claim{
			Project: cfg.Project, Path: c.Root, Services: res.Services, UpdatedAt: a.now(),
		})
		if err := a.Registry.Save(reg); err != nil {
			return EnsureResult{}, mapRegistryErr(err)
		}
	}
	return EnsureResult{
		Project: cfg.Project, WorktreePath: c.Root, Changed: changed,
		Services: res.Services, Env: desiredEnv(c.Root, cfg, res.Services),
	}, nil
}

func mapProjectErr(err error) error {
	switch {
	case errorsIs(err, project.ErrNotRepo):
		return apperr.New("not_a_repo", err.Error(), "ejecuta portucca dentro de un repositorio git")
	case errorsIs(err, project.ErrGitMissing):
		return apperr.New("git_missing", err.Error(), "instala git y vuelve a intentar")
	default:
		return apperr.New("internal", err.Error(), "")
	}
}

func mapConfigErr(err error) error {
	text := err.Error()
	switch {
	case contains(text, "config_missing"):
		return apperr.New("config_missing", text, "ejecuta portucca init")
	case contains(text, "config_version"):
		return apperr.New("config_version", text, "actualiza portucca o corrige la versión")
	default:
		return apperr.New("config_invalid", text, "revisa portucca.json")
	}
}

func mapLockErr(err error) error {
	if errorsIs(err, platform.ErrLockTimeout) {
		return apperr.New("lock_timeout", err.Error(), "otro proceso portucca está trabajando; reintenta")
	}
	if errorsIs(err, platform.ErrUnsupported) {
		return apperr.New("unsupported_platform", err.Error(), "esta plataforma todavía no está soportada")
	}
	return apperr.New("internal", err.Error(), "")
}

func mapRegistryErr(err error) error {
	switch {
	case errorsIs(err, registry.ErrCorrupt):
		return apperr.New("registry_corrupt", err.Error(), "mueve el registro a un lado y vuelve a intentar")
	case errorsIs(err, registry.ErrVersion):
		return apperr.New("registry_version", err.Error(), "actualiza portucca o corrige el registro")
	default:
		return apperr.New("internal", err.Error(), "")
	}
}

func mapAllocErr(err error) error {
	var exhausted *allocation.PortExhaustedError
	if errorsAs(err, &exhausted) {
		return apperr.New("port_exhausted", err.Error(), "amplía la banda del proyecto o libera puertos")
	}
	var duplicate *allocation.DuplicateClaimError
	if errorsAs(err, &duplicate) {
		return apperr.New("duplicate_claim", err.Error(), "revisa el registro con portucca doctor")
	}
	var adoption *allocation.AdoptionConflictError
	if errorsAs(err, &adoption) {
		return apperr.New("adoption_conflict", err.Error(), "corrige los puertos existentes en los archivos de entorno")
	}
	var outOfBand *allocation.OutOfBandError
	if errorsAs(err, &outOfBand) {
		return apperr.New("claim_out_of_band", err.Error(), "ejecuta portucca reallocate")
	}
	return apperr.New("internal", err.Error(), "")
}

func mapEnvErr(err error) error {
	switch {
	case errorsIs(err, envfile.ErrSourceMissing):
		return apperr.New("bootstrap_source_missing", err.Error(), "corrige bootstrap.copyFromMain")
	case errorsIs(err, envfile.ErrSymlinkConflict):
		return apperr.New("symlink_conflict", err.Error(), "revisa el archivo o symlink existente")
	default:
		return apperr.New("bootstrap_error", err.Error(), "revisa bootstrap y los permisos")
	}
}
