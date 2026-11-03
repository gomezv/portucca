package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/config"
	"github.com/gomezv/portucca/internal/domain"
)

type InitRequest struct {
	Project    string
	Services   []string
	Preset     string
	Force      bool
	Worktree   string
	ConfigPath string
}

type InitResult struct {
	ConfigPath string                                `json:"configPath"`
	Project    domain.Project                        `json:"project"`
	Services   map[domain.ServiceName]domain.Service `json:"services"`
	Warning    string                                `json:"warning,omitempty"`
}

func (a *App) Init(ctx context.Context, req InitRequest) (InitResult, error) {
	worktree := req.Worktree
	if worktree == "" {
		worktree = "."
	}
	c, err := a.Locator.Detect(worktree)
	if err != nil {
		return InitResult{}, mapProjectErr(err)
	}
	path := filepath.Join(c.Root, config.FileName)
	if req.ConfigPath != "" {
		abs, err := filepath.Abs(req.ConfigPath)
		if err != nil {
			return InitResult{}, apperr.New("config_invalid", err.Error(), "check --config")
		}
		if filepath.Dir(abs) != c.Root {
			return InitResult{}, apperr.New("usage", "--config must live at the repo root", "use the repository root")
		}
		path = abs
	}
	services := map[domain.ServiceName]domain.Service{}
	for _, spec := range req.Services {
		name, main, start, err := parseServiceSpec(spec)
		if err != nil {
			return InitResult{}, apperr.New("usage", err.Error(), "use --service name:main:start")
		}
		services[name] = domain.Service{MainPort: main, StartPort: start}
	}
	preset := req.Preset
	if preset == "" {
		preset = "generic"
	}
	cfg, err := config.ApplyPreset(preset, domain.Project(req.Project), services)
	if err != nil {
		return InitResult{}, apperr.New("usage", err.Error(), "check --project, --service, and --preset")
	}
	if _, err := os.Stat(path); err == nil && !req.Force {
		return InitResult{}, apperr.New("config_exists", "portucca.json already exists", "use --force to overwrite it")
	}
	warning := ""
	if a.Registry != nil && a.Locker != nil {
		if unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), true); err == nil {
			if reg, err := a.Registry.Load(); err == nil {
				for _, claim := range reg.Worktrees {
					if claim.Project == cfg.Project && claim.Path != c.Root {
						warning = fmt.Sprintf("project %q already has claims in %s", cfg.Project, claim.Path)
						break
					}
				}
			}
			_ = unlock()
		}
	}
	if err := config.WriteFile(path, cfg); err != nil {
		return InitResult{}, apperr.New("config_invalid", err.Error(), "check permissions")
	}
	return InitResult{ConfigPath: path, Project: cfg.Project, Services: cfg.Services, Warning: warning}, nil
}

func parseServiceSpec(spec string) (domain.ServiceName, domain.Port, domain.Port, error) {
	parts := strings.Split(spec, ":")
	if len(parts) != 3 {
		return "", 0, 0, fmt.Errorf("invalid service %q", spec)
	}
	main, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, 0, fmt.Errorf("invalid main port in %q", spec)
	}
	start, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", 0, 0, fmt.Errorf("invalid start port in %q", spec)
	}
	return domain.ServiceName(parts[0]), domain.Port(main), domain.Port(start), nil
}
