package application

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
)

type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type DoctorClaim struct {
	Project  domain.Project                             `json:"project"`
	Path     string                                     `json:"path"`
	Live     bool                                       `json:"live"`
	Services map[domain.ServiceName]domain.ServiceClaim `json:"services"`
	Issues   []Issue                                    `json:"issues"`
}

type DoctorRegistry struct {
	Path        string `json:"path"`
	Version     int    `json:"version"`
	Permissions string `json:"permissions"`
	Claims      int    `json:"claims"`
}

type DoctorResult struct {
	OK       bool           `json:"ok"`
	Project  domain.Project `json:"project,omitempty"`
	Registry DoctorRegistry `json:"registry"`
	Claims   []DoctorClaim  `json:"claims"`
	Errors   []Issue        `json:"errors"`
	Warnings []Issue        `json:"warnings"`
}

func (a *App) Doctor(ctx context.Context, worktree string) (DoctorResult, error) {
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), true)
	if err != nil {
		return DoctorResult{}, mapLockErr(err)
	}
	defer unlock()
	result := DoctorResult{OK: true, Claims: []DoctorClaim{}, Errors: []Issue{}, Warnings: []Issue{}}
	regPath := strings.TrimSuffix(a.Registry.LockPath(), ".lock")
	result.Registry = DoctorRegistry{Path: regPath}
	reg, regErr := a.Registry.Load()
	if regErr != nil {
		result.Errors = append(result.Errors, registryIssue(regErr))
		result.OK = false
		reg = domain.Registry{Version: domain.RegistryVersion}
	} else {
		result.Registry.Version = reg.Version
		result.Registry.Claims = len(reg.Worktrees)
		if info, err := os.Stat(regPath); err == nil {
			result.Registry.Permissions = fmt.Sprintf("%04o", info.Mode().Perm())
		}
	}
	var cfg domain.Config
	var haveConfig bool
	var c project.Context
	var detectErr error
	c, detectErr = a.Locator.Detect(worktree)
	if detectErr != nil {
		result.Warnings = append(result.Warnings, Issue{Code: "not_a_repo", Message: detectErr.Error()})
	} else {
		loaded, _, loadErr := a.loadConfig(c.Root)
		if loadErr != nil {
			result.Errors = append(result.Errors, configIssue(loadErr))
			result.OK = false
		} else {
			cfg, haveConfig = loaded, true
			result.Project = cfg.Project
		}
	}
	byPort := map[domain.Port]string{}
	for _, claim := range reg.Worktrees {
		dc := DoctorClaim{Project: claim.Project, Path: claim.Path, Services: claim.Services, Issues: []Issue{}}
		if _, err := os.Stat(claim.Path); errors.Is(err, fs.ErrNotExist) {
			dc.Live = false
			dc.Issues = append(dc.Issues, Issue{Code: "stale_claim", Message: "el worktree no existe"})
		} else {
			dc.Live = true
			claimCfg, _, cfgErr := a.loadConfig(claim.Path)
			if cfgErr != nil {
				result.Warnings = append(result.Warnings, configIssue(cfgErr))
			}
			for name, sc := range claim.Services {
				if other, ok := byPort[sc.Port]; ok && other != claim.Path {
					result.Errors = append(result.Errors, Issue{Code: "duplicate_claim", Message: "puerto reclamado por dos worktrees"})
					result.OK = false
				}
				byPort[sc.Port] = claim.Path
				if a.Prober != nil && !a.Prober.IsFree(sc.Port) && claim.Path != c.Root {
					result.Warnings = append(result.Warnings, Issue{Code: "port_busy", Message: fmt.Sprintf("puerto %d ocupado en %s", sc.Port, claim.Path)})
				}
				if cfgErr == nil {
					if claimCfg.Project != claim.Project {
						dc.Issues = append(dc.Issues, Issue{Code: "claim_project_mismatch", Message: "el proyecto del claim no coincide con portucca.json"})
					}
					svc, ok := claimCfg.Services[name]
					if !ok || sc.Port < svc.StartPort {
						dc.Issues = append(dc.Issues, Issue{Code: "claim_out_of_band", Message: "el puerto quedó fuera de la banda"})
					}
					for _, mainSvc := range claimCfg.Services {
						if sc.Port == mainSvc.MainPort {
							dc.Issues = append(dc.Issues, Issue{Code: "main_port_claimed", Message: "el claim usa un puerto reservado del main"})
							break
						}
					}
				}
			}
		}
		if len(dc.Issues) > 0 {
			result.Errors = append(result.Errors, dc.Issues...)
			result.OK = false
		}
		result.Claims = append(result.Claims, dc)
	}
	if haveConfig && a.Prober != nil {
		for _, svc := range cfg.Services {
			if !a.Prober.IsFree(svc.MainPort) {
				result.Warnings = append(result.Warnings, Issue{Code: "port_busy", Message: fmt.Sprintf("puerto reservado %d ocupado", svc.MainPort)})
			}
		}
	}
	if haveConfig && a.Env != nil {
		if own := findClaim(reg, c.Root); own != nil {
			desired := desiredValues(cfg, own.Services)
			for _, target := range cfg.Env {
				expected := desired[target.File][target.Key]
				got, ok, err := a.Env.ReadValue(c.Root, domain.EnvRef{File: target.File, Key: target.Key})
				if err != nil || !ok || got != expected {
					result.Warnings = append(result.Warnings, Issue{Code: "env_out_of_sync", Message: filepath.Join(target.File, target.Key)})
				}
			}
			for _, link := range cfg.Bootstrap.Symlinks {
				path := filepath.Join(c.Root, link.Path)
				info, err := os.Lstat(path)
				if errors.Is(err, fs.ErrNotExist) {
					result.Warnings = append(result.Warnings, Issue{Code: "symlink_missing", Message: link.Path})
					continue
				}
				if err != nil {
					result.Warnings = append(result.Warnings, Issue{Code: "symlink_out_of_sync", Message: link.Path})
					continue
				}
				if info.Mode()&os.ModeSymlink == 0 {
					result.Warnings = append(result.Warnings, Issue{Code: "symlink_out_of_sync", Message: link.Path})
					continue
				}
				if target, err := os.Readlink(path); err != nil || target != link.Target {
					result.Warnings = append(result.Warnings, Issue{Code: "symlink_out_of_sync", Message: link.Path})
				}
			}
			for _, rel := range cfg.Bootstrap.CopyFromMain {
				if _, err := os.Lstat(filepath.Join(c.Root, rel)); errors.Is(err, fs.ErrNotExist) {
					result.Warnings = append(result.Warnings, Issue{Code: "bootstrap_pending", Message: rel})
				}
			}
		}
	}
	sort.Slice(result.Claims, func(i, j int) bool {
		if result.Claims[i].Project != result.Claims[j].Project {
			return result.Claims[i].Project < result.Claims[j].Project
		}
		return result.Claims[i].Path < result.Claims[j].Path
	})
	return result, nil
}

func configIssue(err error) Issue {
	mapped := mapConfigErr(err)
	if ae, ok := apperr.As(mapped); ok {
		return Issue{Code: ae.Code, Message: ae.Message, Hint: ae.Hint}
	}
	return Issue{Code: "config_invalid", Message: err.Error()}
}

func registryIssue(err error) Issue {
	mapped := mapRegistryErr(err)
	if ae, ok := apperr.As(mapped); ok {
		return Issue{Code: ae.Code, Message: ae.Message, Hint: ae.Hint}
	}
	return Issue{Code: "registry_corrupt", Message: err.Error()}
}
