package application

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/envfile"
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
	values := desiredValues(cfg, services)
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

func pruneStale(reg domain.Registry, stat func(string) (fs.FileInfo, error)) (domain.Registry, []domain.Claim) {
	kept := make([]domain.Claim, 0, len(reg.Worktrees))
	var removed []domain.Claim
	for _, claim := range reg.Worktrees {
		if _, err := stat(claim.Path); errors.Is(err, fs.ErrNotExist) {
			removed = append(removed, claim)
			continue
		}
		kept = append(kept, claim)
	}
	reg.Worktrees = kept
	return reg, removed
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

func occupiedWithMainPorts(cfg domain.Config, reg domain.Registry, exceptPath string) map[domain.Port]string {
	out := occupiedFrom(reg, exceptPath)
	for port, marker := range reservedMainPorts(cfg) {
		out[port] = marker
	}
	return out
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

func errorsIs(err, target error) bool     { return errors.Is(err, target) }
func errorsAs(err error, target any) bool { return errors.As(err, target) }
func contains(s, sub string) bool         { return strings.Contains(s, sub) }
