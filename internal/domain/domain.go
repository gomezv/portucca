package domain

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	ConfigVersion   = 1
	RegistryVersion = 1
	MinPort         = 1024
	MaxPort         = 65535
	DefaultMaxSlots = 1000
)

type Project string
type ServiceName string
type Port int

func (p Port) Valid() bool { return p >= MinPort && p <= MaxPort }

var (
	projectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	servicePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	keyPattern     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func ValidProject(s string) bool     { return projectPattern.MatchString(s) }
func ValidServiceName(s string) bool { return servicePattern.MatchString(s) }

type EnvRef struct {
	File string `json:"file"`
	Key  string `json:"key"`
}

type Service struct {
	MainPort  Port    `json:"mainPort"`
	StartPort Port    `json:"startPort"`
	AdoptFrom *EnvRef `json:"adoptFrom,omitempty"`
}

type EnvTarget struct {
	Service  ServiceName `json:"service"`
	File     string      `json:"file"`
	Key      string      `json:"key"`
	Template string      `json:"template,omitempty"`
}

type Symlink struct {
	Path   string `json:"path"`
	Target string `json:"target"`
}

type Bootstrap struct {
	CopyFromMain []string  `json:"copyFromMain,omitempty"`
	Symlinks     []Symlink `json:"symlinks,omitempty"`
}

type Config struct {
	Version   int                     `json:"version"`
	Project   Project                 `json:"project"`
	Services  map[ServiceName]Service `json:"services"`
	Env       []EnvTarget             `json:"env,omitempty"`
	Bootstrap Bootstrap               `json:"bootstrap,omitempty"`
}

type ServiceClaim struct {
	Port    Port `json:"port"`
	Adopted bool `json:"adopted"`
}

type Claim struct {
	Project   Project                      `json:"project"`
	Path      string                       `json:"path"`
	Services  map[ServiceName]ServiceClaim `json:"services"`
	UpdatedAt time.Time                    `json:"updatedAt"`
}

type Registry struct {
	Version   int     `json:"version"`
	Worktrees []Claim `json:"worktrees"`
}

func (c Config) ServiceNames() []ServiceName {
	names := make([]ServiceName, 0, len(c.Services))
	for name := range c.Services {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

func (c Config) Validate() error {
	if c.Version != ConfigVersion {
		return fmt.Errorf("config_version: %d", c.Version)
	}
	if !projectPattern.MatchString(string(c.Project)) {
		return fmt.Errorf("project: invalid slug %q", c.Project)
	}
	if len(c.Services) == 0 {
		return fmt.Errorf("services: at least one service is required")
	}
	names := c.ServiceNames()
	ports := map[Port]ServiceName{}
	for _, name := range names {
		svc := c.Services[name]
		if !servicePattern.MatchString(string(name)) {
			return fmt.Errorf("service %q: invalid name", name)
		}
		if !svc.MainPort.Valid() || !svc.StartPort.Valid() {
			return fmt.Errorf("service %s: ports must be in %d..%d", name, MinPort, MaxPort)
		}
		for _, p := range []Port{svc.MainPort, svc.StartPort} {
			if other, ok := ports[p]; ok {
				return fmt.Errorf("duplicate port %d in %s and %s", p, other, name)
			}
			ports[p] = name
		}
	}
	for _, name := range names {
		svc := c.Services[name]
		if svc.StartPort <= svc.MainPort {
			return fmt.Errorf("service %s: startPort must be greater than mainPort", name)
		}
		if svc.AdoptFrom != nil {
			if err := validateEnvRef(fmt.Sprintf("services.%s.adoptFrom", name), *svc.AdoptFrom); err != nil {
				return err
			}
		}
	}
	seen := map[string]bool{}
	for i, e := range c.Env {
		field := fmt.Sprintf("env[%d]", i)
		if _, ok := c.Services[e.Service]; !ok {
			return fmt.Errorf("%s: unknown service %q", field, e.Service)
		}
		if err := validateRelPath(field+".file", e.File); err != nil {
			return err
		}
		if !keyPattern.MatchString(e.Key) {
			return fmt.Errorf("%s.key: invalid key %q", field, e.Key)
		}
		if err := validateTemplate(field+".template", e.Template); err != nil {
			return err
		}
		key := e.File + "\x00" + e.Key
		if seen[key] {
			return fmt.Errorf("%s: duplicate env target %s:%s", field, e.File, e.Key)
		}
		seen[key] = true
	}
	for name, svc := range c.Services {
		if svc.AdoptFrom == nil {
			continue
		}
		found := false
		for _, e := range c.Env {
			if e.Service == name && e.File == svc.AdoptFrom.File && e.Key == svc.AdoptFrom.Key && e.Template == "" {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("services.%s.adoptFrom: no matching env target", name)
		}
	}
	for i, p := range c.Bootstrap.CopyFromMain {
		if err := validateRelPath(fmt.Sprintf("bootstrap.copyFromMain[%d]", i), p); err != nil {
			return err
		}
	}
	for i, s := range c.Bootstrap.Symlinks {
		field := fmt.Sprintf("bootstrap.symlinks[%d]", i)
		if err := validateRelPath(field+".path", s.Path); err != nil {
			return err
		}
		if s.Target == "" || path.IsAbs(s.Target) {
			return fmt.Errorf("%s.target: must be a non-empty relative path", field)
		}
	}
	return nil
}

func validateEnvRef(field string, r EnvRef) error {
	if err := validateRelPath(field+".file", r.File); err != nil {
		return err
	}
	if !keyPattern.MatchString(r.Key) {
		return fmt.Errorf("%s.key: invalid key %q", field, r.Key)
	}
	return nil
}

func validateRelPath(field, p string) error {
	if p == "" {
		return fmt.Errorf("%s: empty path", field)
	}
	if path.IsAbs(p) {
		return fmt.Errorf("%s: absolute path not allowed", field)
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%s: path escapes the repo", field)
	}
	return nil
}

func validateTemplate(field, t string) error {
	if t == "" {
		return nil
	}
	if strings.Count(t, "{{port}}") != 1 {
		return fmt.Errorf("%s: exactly one {{port}} placeholder is required", field)
	}
	if strings.Contains(strings.ReplaceAll(t, "{{port}}", ""), "{{") {
		return fmt.Errorf("%s: unknown template placeholder", field)
	}
	return nil
}
