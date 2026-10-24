package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gomezv/portucca/internal/domain"
)

//go:embed presets/node-vite-nest.json
var nodeViteNestJSON []byte

type presetFile struct {
	Services  []domain.ServiceName `json:"services"`
	Env       []domain.EnvTarget   `json:"env"`
	Bootstrap domain.Bootstrap     `json:"bootstrap"`
}

var presetNames = map[string]bool{"generic": true, "node-vite-nest": true}

func PresetNames() []string {
	names := make([]string, 0, len(presetNames))
	for name := range presetNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ApplyPreset(name string, project domain.Project, services map[domain.ServiceName]domain.Service) (domain.Config, error) {
	cfg := domain.Config{
		Version:  domain.ConfigVersion,
		Project:  project,
		Services: services,
	}
	switch name {
	case "generic":
		return cfg, cfg.Validate()
	case "node-vite-nest":
		var p presetFile
		if err := json.Unmarshal(nodeViteNestJSON, &p); err != nil {
			return domain.Config{}, err
		}
		if len(services) != len(p.Services) {
			return domain.Config{}, fmt.Errorf("preset %s requires services %v", name, p.Services)
		}
		for _, s := range p.Services {
			if _, ok := services[s]; !ok {
				return domain.Config{}, fmt.Errorf("preset %s requires service %s", name, s)
			}
		}
		cfg.Env = p.Env
		cfg.Bootstrap = p.Bootstrap
		return cfg, cfg.Validate()
	default:
		return domain.Config{}, fmt.Errorf("unknown preset %q", name)
	}
}
