package application

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gomezv/portucca/internal/domain"
)

type OverviewService struct {
	Name      domain.ServiceName `json:"name"`
	Port      domain.Port        `json:"port"`
	Adopted   bool               `json:"adopted"`
	MainPort  domain.Port        `json:"mainPort"`
	StartPort domain.Port        `json:"startPort"`
	InBand    bool               `json:"inBand"`
}

type OverviewWorktree struct {
	Project   domain.Project    `json:"project"`
	Path      string            `json:"path"`
	Kind      string            `json:"kind"`
	Live      bool              `json:"live"`
	Services  []OverviewService `json:"services"`
	Issues    []Issue           `json:"issues"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

type OverviewResult struct {
	RegistryVersion int                `json:"registryVersion"`
	GeneratedAt     time.Time          `json:"generatedAt"`
	Worktrees       []OverviewWorktree `json:"worktrees"`
}

func (a *App) Overview(ctx context.Context) (OverviewResult, error) {
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), true)
	if err != nil {
		return OverviewResult{}, mapLockErr(err)
	}
	defer unlock()
	reg, err := a.Registry.Load()
	if err != nil {
		return OverviewResult{}, mapRegistryErr(err)
	}
	fallbacks := a.overviewFallbacks(reg)
	res := OverviewResult{
		RegistryVersion: reg.Version,
		GeneratedAt:     a.now(),
		Worktrees:       []OverviewWorktree{},
	}
	for _, main := range reg.Mains {
		var fallback *domain.Config
		if cfg, ok := fallbacks[string(main.Project)]; ok {
			fallback = &cfg
		}
		cfg, cfgErr := a.overviewConfig(main.Path, fallback)
		res.Worktrees = append(res.Worktrees, buildMainOverview(main, cfg, cfgErr))
	}
	for _, claim := range reg.Worktrees {
		var fallback *domain.Config
		if cfg, ok := fallbacks[string(claim.Project)]; ok {
			fallback = &cfg
		}
		cfg, cfgErr := a.overviewConfig(claim.Path, fallback)
		res.Worktrees = append(res.Worktrees, buildClaimOverview(claim, cfg, cfgErr))
	}
	sort.Slice(res.Worktrees, func(i, j int) bool {
		if res.Worktrees[i].Project != res.Worktrees[j].Project {
			return res.Worktrees[i].Project < res.Worktrees[j].Project
		}
		if res.Worktrees[i].Kind != res.Worktrees[j].Kind {
			return res.Worktrees[i].Kind == "main"
		}
		return res.Worktrees[i].Path < res.Worktrees[j].Path
	})
	return res, nil
}

func (a *App) overviewFallbacks(reg domain.Registry) map[string]domain.Config {
	out := map[string]domain.Config{}
	for _, main := range reg.Mains {
		key := string(main.Project)
		if _, ok := out[key]; ok {
			continue
		}
		if cfg, _, err := a.Config.LoadRoot(main.Path); err == nil {
			out[key] = cfg
		}
	}
	for _, claim := range reg.Worktrees {
		key := string(claim.Project)
		if _, ok := out[key]; ok {
			continue
		}
		local := filepath.Join(claim.Path, "portucca.json")
		if _, err := os.Stat(local); err == nil {
			if cfg, err := a.Config.LoadFile(local); err == nil {
				out[key] = cfg
				continue
			}
		}
		if cfg, _, err := a.Config.LoadRoot(claim.Path); err == nil {
			out[key] = cfg
		}
	}
	return out
}

func (a *App) overviewConfig(path string, fallback *domain.Config) (domain.Config, error) {
	local := filepath.Join(path, "portucca.json")
	if _, err := os.Stat(local); err == nil {
		return a.Config.LoadFile(local)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return domain.Config{}, err
	}
	if fallback != nil {
		return *fallback, nil
	}
	cfg, _, err := a.Config.LoadRoot(path)
	return cfg, err
}

func buildMainOverview(main domain.MainReservation, cfg domain.Config, cfgErr error) OverviewWorktree {
	row := OverviewWorktree{
		Project:   main.Project,
		Path:      main.Path,
		Kind:      "main",
		Live:      pathLive(main.Path),
		Services:  []OverviewService{},
		Issues:    append([]Issue{}, configIssues(cfgErr)...),
		UpdatedAt: main.UpdatedAt,
	}
	if !row.Live {
		row.Issues = append(row.Issues, Issue{Code: "stale_path", Message: "the main path does not exist", Hint: "run portucca prune"})
	}
	for _, name := range sortedServiceNames(main.Ports) {
		port := main.Ports[name]
		svc := OverviewService{Name: name, Port: port}
		if cfgErr == nil {
			cfgSvc, ok := cfg.Services[name]
			if !ok {
				row.Issues = append(row.Issues, Issue{
					Code: "service_not_in_config", Message: "the service is not in portucca.json",
					Hint: "update the config or run portucca prune",
				})
			} else {
				svc.MainPort = cfgSvc.MainPort
				svc.StartPort = cfgSvc.StartPort
				svc.InBand = port == cfgSvc.MainPort
				if !svc.InBand {
					row.Issues = append(row.Issues, Issue{
						Code: "main_port_mismatch", Message: "the reservation does not match mainPort",
						Hint: "run portucca ensure in the main checkout",
					})
				}
			}
		}
		row.Services = append(row.Services, svc)
	}
	sortOverviewServices(row.Services)
	return row
}

func buildClaimOverview(claim domain.Claim, cfg domain.Config, cfgErr error) OverviewWorktree {
	row := OverviewWorktree{
		Project:   claim.Project,
		Path:      claim.Path,
		Kind:      "worktree",
		Live:      pathLive(claim.Path),
		Services:  []OverviewService{},
		Issues:    append([]Issue{}, configIssues(cfgErr)...),
		UpdatedAt: claim.UpdatedAt,
	}
	if !row.Live {
		row.Issues = append(row.Issues, Issue{Code: "stale_path", Message: "the worktree path does not exist", Hint: "run portucca prune"})
	}
	for _, name := range sortedServiceNames(claim.Services) {
		sc := claim.Services[name]
		svc := OverviewService{Name: name, Port: sc.Port, Adopted: sc.Adopted}
		if cfgErr == nil {
			cfgSvc, ok := cfg.Services[name]
			if !ok {
				row.Issues = append(row.Issues, Issue{
					Code: "service_not_in_config", Message: "the service is not in portucca.json",
					Hint: "update the config or run portucca release",
				})
			} else {
				svc.MainPort = cfgSvc.MainPort
				svc.StartPort = cfgSvc.StartPort
				svc.InBand = sc.Port >= cfgSvc.StartPort
				if !svc.InBand {
					row.Issues = append(row.Issues, Issue{
						Code: "claim_out_of_band", Message: "the claim is outside the configured band",
						Hint: "run portucca reallocate",
					})
				}
			}
		}
		row.Services = append(row.Services, svc)
	}
	sortOverviewServices(row.Services)
	return row
}

func configIssues(err error) []Issue {
	if err == nil {
		return nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return []Issue{{
			Code: "config_missing", Message: "portucca.json is missing",
			Hint: "run portucca init in the main checkout",
		}}
	}
	return []Issue{{Code: "config_invalid", Message: normalizeConfigMessage(err.Error())}}
}

func normalizeConfigMessage(message string) string {
	for {
		switch {
		case strings.HasPrefix(message, "config_invalid: "):
			message = strings.TrimPrefix(message, "config_invalid: ")
		case strings.HasPrefix(message, "config_missing: "):
			message = strings.TrimPrefix(message, "config_missing: ")
		case strings.HasPrefix(message, "config_version: "):
			return "unsupported config version " + strings.TrimPrefix(message, "config_version: ")
		default:
			return message
		}
	}
}

func sortedServiceNames[T any](services map[domain.ServiceName]T) []domain.ServiceName {
	names := make([]domain.ServiceName, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

func pathLive(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sortOverviewServices(services []OverviewService) {
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
}
