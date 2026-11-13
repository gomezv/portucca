package application

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/envfile"
	"github.com/gomezv/portucca/internal/project"
)

type EnvValue struct {
	File  string `json:"file"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

func mainServices(cfg domain.Config) map[domain.ServiceName]domain.ServiceClaim {
	out := map[domain.ServiceName]domain.ServiceClaim{}
	for name, svc := range cfg.Services {
		out[name] = domain.ServiceClaim{Port: svc.MainPort}
	}
	return out
}

func mainEnsureResult(c project.Context, cfg domain.Config, cfgPath string) EnsureResult {
	services := mainServices(cfg)
	return EnsureResult{
		Project: cfg.Project, WorktreePath: c.Root, ConfigPath: cfgPath, IsMain: true,
		Services: services, Env: desiredEnv(c.Root, cfg, services),
	}
}

func desiredValues(cfg domain.Config, services map[domain.ServiceName]domain.ServiceClaim) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, target := range cfg.Env {
		claim, ok := services[target.Service]
		if !ok {
			continue
		}
		value := envfile.RenderValue(claim.Port, target.Template)
		if out[target.File] == nil {
			out[target.File] = map[string]string{}
		}
		out[target.File][target.Key] = value
	}
	return out
}

func desiredEnv(root string, cfg domain.Config, services map[domain.ServiceName]domain.ServiceClaim) []EnvValue {
	return desiredEnvFrom(root, desiredValues(cfg, services))
}

func desiredEnvFrom(root string, values map[string]map[string]string) []EnvValue {
	out := make([]EnvValue, 0, len(values))
	for file, kv := range values {
		for key, value := range kv {
			out = append(out, EnvValue{File: filepath.Join(root, file), Key: key, Value: value})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func findClaim(reg domain.Registry, path string) *domain.Claim {
	for i := range reg.Worktrees {
		if reg.Worktrees[i].Path == path {
			return &reg.Worktrees[i]
		}
	}
	return nil
}

func upsertClaim(reg domain.Registry, claim domain.Claim) domain.Registry {
	for i := range reg.Worktrees {
		if reg.Worktrees[i].Path == claim.Path {
			reg.Worktrees[i] = claim
			return reg
		}
	}
	reg.Worktrees = append(reg.Worktrees, claim)
	return reg
}

func isStalePath(stat func(string) (fs.FileInfo, error), path string) bool {
	_, err := stat(path)
	return errors.Is(err, fs.ErrNotExist)
}

func pruneStale(reg domain.Registry, stat func(string) (fs.FileInfo, error)) (domain.Registry, []domain.Claim, []domain.MainReservation) {
	kept := make([]domain.Claim, 0, len(reg.Worktrees))
	var removed []domain.Claim
	for _, claim := range reg.Worktrees {
		if isStalePath(stat, claim.Path) {
			removed = append(removed, claim)
			continue
		}
		kept = append(kept, claim)
	}
	reg.Worktrees = kept
	keptMains := make([]domain.MainReservation, 0, len(reg.Mains))
	var removedMains []domain.MainReservation
	for _, main := range reg.Mains {
		if isStalePath(stat, main.Path) {
			removedMains = append(removedMains, main)
			continue
		}
		keptMains = append(keptMains, main)
	}
	reg.Mains = keptMains
	return reg, removed, removedMains
}

func occupiedFrom(reg domain.Registry, exceptPath string) map[domain.Port]string {
	out := map[domain.Port]string{}
	for _, claim := range reg.Worktrees {
		if claim.Path == exceptPath {
			continue
		}
		for _, sc := range claim.Services {
			out[sc.Port] = claim.Path
		}
	}
	return out
}

func reservedMainPorts(cfg domain.Config) map[domain.Port]string {
	out := make(map[domain.Port]string, len(cfg.Services))
	for _, svc := range cfg.Services {
		out[svc.MainPort] = "main"
	}
	return out
}

func mainReservationPorts(reg domain.Registry) map[domain.Port]string {
	out := map[domain.Port]string{}
	for _, main := range reg.Mains {
		for _, port := range main.Ports {
			out[port] = "main:" + main.Path
		}
	}
	return out
}

func occupiedWithReservations(cfg domain.Config, reg domain.Registry, exceptPath string) map[domain.Port]string {
	out := occupiedFrom(reg, exceptPath)
	for port, marker := range mainReservationPorts(reg) {
		out[port] = marker
	}
	for port, marker := range reservedMainPorts(cfg) {
		out[port] = marker
	}
	return out
}

func upsertMainReservation(reg domain.Registry, cfg domain.Config, path string, now time.Time) (domain.Registry, bool) {
	ports := make(map[domain.ServiceName]domain.Port, len(cfg.Services))
	for name, svc := range cfg.Services {
		ports[name] = svc.MainPort
	}
	for i, main := range reg.Mains {
		if main.Path != path {
			continue
		}
		if main.Project == cfg.Project && portsEqual(main.Ports, ports) {
			return reg, false
		}
		reg.Mains[i] = domain.MainReservation{
			Project: cfg.Project, Path: path, Ports: ports, UpdatedAt: now,
		}
		return reg, true
	}
	reg.Mains = append(reg.Mains, domain.MainReservation{
		Project: cfg.Project, Path: path, Ports: ports, UpdatedAt: now,
	})
	return reg, true
}

func portsEqual(a, b map[domain.ServiceName]domain.Port) bool {
	if len(a) != len(b) {
		return false
	}
	for name, port := range a {
		if b[name] != port {
			return false
		}
	}
	return true
}

func claimDiffers(claim *domain.Claim, services map[domain.ServiceName]domain.ServiceClaim) bool {
	if claim == nil || len(claim.Services) != len(services) {
		return true
	}
	for name, want := range services {
		got, ok := claim.Services[name]
		if !ok || got != want {
			return true
		}
	}
	return false
}

func (a *App) loadConfig(root string) (domain.Config, string, error) {
	if a.ConfigFile == "" {
		return a.Config.LoadRoot(root)
	}
	abs, err := filepath.Abs(a.ConfigFile)
	if err != nil {
		return domain.Config{}, "", err
	}
	if filepath.Dir(abs) != root {
		return domain.Config{}, abs, fmt.Errorf("config_invalid: --config must live at the repo root")
	}
	cfg, err := a.Config.LoadFile(abs)
	return cfg, abs, err
}

// loadConfigFor loads the worktree config. When the worktree has no config and is not the
// main checkout, it falls back to the main checkout's config, enabling a shared local config.
func (a *App) loadConfigFor(c project.Context) (domain.Config, string, error) {
	if a.ConfigFile != "" {
		return a.loadConfig(c.Root)
	}
	cfg, path, err := a.Config.LoadRoot(c.Root)
	if err == nil || c.IsMain || !errors.Is(err, fs.ErrNotExist) {
		return cfg, path, err
	}
	return a.Config.LoadRoot(c.MainRoot)
}

func errorsIs(err, target error) bool     { return errors.Is(err, target) }
func errorsAs(err error, target any) bool { return errors.As(err, target) }
func contains(s, sub string) bool         { return strings.Contains(s, sub) }
