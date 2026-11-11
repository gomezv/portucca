package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/config"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
	"github.com/gomezv/portucca/internal/scan"
)

type InitRequest struct {
	Project         string
	Services        []string
	Preset          string
	Force           bool
	Worktree        string
	ConfigPath      string
	Scan            bool
	Compose         bool
	ExcludeServices []string
}

type InitResult struct {
	ConfigPath string                                `json:"configPath"`
	Project    domain.Project                        `json:"project"`
	Services   map[domain.ServiceName]domain.Service `json:"services"`
	Warning    string                                `json:"warning,omitempty"`
	Scan       *InitScan                             `json:"scan,omitempty"`
}

type ScanSource struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

type InitScan struct {
	Sources  []ScanSource `json:"sources"`
	Warnings []string     `json:"warnings"`
}

type mergedScan struct {
	Config   domain.Config
	Sources  []ScanSource
	Warnings []string
	Gaps     []scan.Gap
}

func (a *App) Init(ctx context.Context, req InitRequest) (InitResult, error) {
	if req.Scan && req.Preset != "" {
		return InitResult{}, apperr.New("usage", "--scan and --preset are mutually exclusive", "drop one of them")
	}
	worktree := req.Worktree
	if worktree == "" {
		worktree = "."
	}
	c, err := a.Locator.Detect(worktree)
	if err != nil {
		return InitResult{}, mapProjectErr(err)
	}
	if req.Scan {
		return a.initScan(ctx, c, req)
	}
	return a.initFromFlags(c, req)
}

func (a *App) initFromFlags(c project.Context, req InitRequest) (InitResult, error) {
	path, err := initConfigPath(c, req)
	if err != nil {
		return InitResult{}, err
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
	warning := a.claimsWarning(cfg.Project, c.Root)
	if err := config.WriteFile(path, cfg); err != nil {
		return InitResult{}, apperr.New("config_invalid", err.Error(), "check permissions")
	}
	if err := a.registerMain(c, cfg, path); err != nil {
		return InitResult{}, err
	}
	return InitResult{ConfigPath: path, Project: cfg.Project, Services: cfg.Services, Warning: warning}, nil
}

func initConfigPath(c project.Context, req InitRequest) (string, error) {
	path := filepath.Join(c.Root, config.FileName)
	if req.ConfigPath == "" {
		return path, nil
	}
	abs, err := filepath.Abs(req.ConfigPath)
	if err != nil {
		return "", apperr.New("config_invalid", err.Error(), "check --config")
	}
	if filepath.Dir(abs) != c.Root {
		return "", apperr.New("usage", "--config must live at the repo root", "use the repository root")
	}
	return abs, nil
}

func (a *App) claimsWarning(projectName domain.Project, root string) string {
	if a.Registry == nil || a.Locker == nil {
		return ""
	}
	unlock, err := a.Locker.Lock(a.Registry.LockPath(), a.timeout(), true)
	if err != nil {
		return ""
	}
	defer unlock()
	reg, err := a.Registry.Load()
	if err != nil {
		return ""
	}
	for _, claim := range reg.Worktrees {
		if claim.Project == projectName && claim.Path != root {
			return fmt.Sprintf("project %q already has claims in %s", projectName, claim.Path)
		}
	}
	return ""
}

func (a *App) initScan(ctx context.Context, c project.Context, req InitRequest) (InitResult, error) {
	explicit, err := parseExplicitServices(req.Services)
	if err != nil {
		return InitResult{}, err
	}
	if req.Project != "" && !domain.ValidProject(req.Project) {
		return InitResult{}, apperr.New("usage", "invalid project "+req.Project, "use a lowercase slug")
	}
	path, err := initConfigPath(c, req)
	if err != nil {
		return InitResult{}, err
	}
	if _, err := os.Stat(path); err == nil && !req.Force {
		return InitResult{}, apperr.New("config_exists", "portucca.json already exists", "use --force to overwrite it")
	}
	if a.Scanner == nil {
		return InitResult{}, apperr.New("internal", "scan is not configured", "rebuild the binary")
	}
	detected, err := a.Scanner.Scan(ctx, scan.Options{
		Root: c.Root, MainRoot: c.MainRoot,
		DiscoverCompose: req.Compose, ExcludeServices: req.ExcludeServices,
	})
	if err != nil {
		var tooLarge *scan.TooLargeError
		if errors.As(err, &tooLarge) {
			ae := apperr.New("scan_too_large", "scan exceeded the entry limit", "use --service flags to configure the project manually")
			ae.Details = map[string]any{"limit": tooLarge.Limit, "visited": tooLarge.Visited}
			return InitResult{}, ae
		}
		return InitResult{}, apperr.New("internal", err.Error(), "retry or use --service flags")
	}
	merged := mergeDetected(req, detected, explicit)
	cfg := merged.Config
	if len(merged.Gaps) > 0 {
		return InitResult{}, scanIncomplete(cfg, merged)
	}
	if err := cfg.Validate(); err != nil {
		if len(req.Services) > 0 {
			return InitResult{}, apperr.New("usage", err.Error(), "check --project and --service")
		}
		if len(cfg.Services) == 0 {
			merged.Gaps = append(merged.Gaps, scan.Gap{
				Field: "services", Reason: "missing", Message: err.Error(),
				Flags: []string{"--service <name>:<main>:<start>"},
			})
		} else {
			merged.Gaps = append(merged.Gaps, scan.Gap{
				Field: "config", Reason: "invalid", Message: err.Error(),
				Flags: []string{"--project <slug>", "--service <name>:<main>:<start>"},
			})
		}
		return InitResult{}, scanIncomplete(cfg, merged)
	}
	warning := a.claimsWarning(cfg.Project, c.Root)
	if err := config.WriteFile(path, cfg); err != nil {
		return InitResult{}, apperr.New("config_invalid", err.Error(), "check permissions")
	}
	if err := a.registerMain(c, cfg, path); err != nil {
		return InitResult{}, err
	}
	return InitResult{
		ConfigPath: path, Project: cfg.Project, Services: cfg.Services, Warning: warning,
		Scan: &InitScan{Sources: merged.Sources, Warnings: merged.Warnings},
	}, nil
}

func parseExplicitServices(specs []string) (map[domain.ServiceName]domain.Service, error) {
	out := map[domain.ServiceName]domain.Service{}
	for _, spec := range specs {
		name, main, start, err := parseServiceSpec(spec)
		if err != nil {
			return nil, apperr.New("usage", err.Error(), "use --service name:main:start")
		}
		if !domain.ValidServiceName(string(name)) {
			return nil, apperr.New("usage", "invalid service name "+string(name), "use a lowercase slug")
		}
		if !main.Valid() || !start.Valid() || start <= main {
			return nil, apperr.New("usage", "invalid ports in "+spec, "start must be greater than main")
		}
		if _, ok := out[name]; ok {
			return nil, apperr.New("usage", "duplicate service "+string(name), "pass each service once")
		}
		for _, other := range out {
			if main == other.MainPort || main == other.StartPort || start == other.MainPort || start == other.StartPort {
				return nil, apperr.New("usage", "duplicate port in "+spec, "use unique ports")
			}
		}
		out[name] = domain.Service{MainPort: main, StartPort: start}
	}
	return out, nil
}

func mergeDetected(req InitRequest, det scan.Detected, explicit map[domain.ServiceName]domain.Service) mergedScan {
	services := map[domain.ServiceName]domain.Service{}
	gaps := append([]scan.Gap{}, det.Gaps...)
	warnings := append([]string{}, det.Warnings...)
	if len(explicit) > 0 {
		gaps = nil
		for name, svc := range explicit {
			services[name] = svc
		}
		dropped := make([]string, 0)
		for _, svc := range det.Services {
			if _, ok := services[svc.Name]; !ok {
				dropped = append(dropped, string(svc.Name))
			}
		}
		if len(dropped) > 0 {
			sort.Strings(dropped)
			warnings = append(warnings, "ignoring detected services: "+strings.Join(dropped, ", "))
		}
	} else {
		for _, svc := range det.Services {
			if !domain.ValidServiceName(string(svc.Name)) {
				gaps = append(gaps, scan.Gap{
					Field: "services", Reason: "invalid",
					Message: fmt.Sprintf("invalid service name %q", svc.Name),
					Flags:   []string{"--service <name>:<main>:<start>"},
					Service: svc.Name,
				})
				continue
			}
			entry := domain.Service{}
			if svc.MainPort == nil {
				if !hasPortGap(gaps, svc.Name) {
					gaps = append(gaps, scan.Gap{
						Field: "services." + string(svc.Name) + ".mainPort", Reason: "missing",
						Message: "no port declaration found",
						Flags:   []string{fmt.Sprintf("--service %s:<main>:<start>", svc.Name)},
						Service: svc.Name,
					})
				}
			} else if port, err := strconv.Atoi(svc.MainPort.Value); err != nil || !domain.Port(port).Valid() {
				gaps = append(gaps, scan.Gap{
					Field: "services." + string(svc.Name) + ".mainPort", Reason: "invalid",
					Message: "invalid detected port " + svc.MainPort.Value,
					Flags:   []string{fmt.Sprintf("--service %s:<main>:<start>", svc.Name)},
					Service: svc.Name,
				})
			} else {
				entry.MainPort = domain.Port(port)
			}
			services[svc.Name] = entry
		}
		gaps = append(gaps, deriveStartPorts(services)...)
	}

	projectName := domain.Project("")
	projectSource := ""
	switch {
	case req.Project != "":
		projectName = domain.Project(req.Project)
		projectSource = "flag"
	case det.Project != nil:
		projectName = domain.Project(det.Project.Value)
		projectSource = det.Project.Source
	default:
		gaps = append(gaps, scan.Gap{
			Field: "project", Reason: "missing", Message: "no project name could be derived",
			Flags: []string{"--project <slug>"},
		})
	}

	targets := filterEnv(det.Env, services, &warnings)
	env := make([]domain.EnvTarget, 0, len(targets))
	for _, target := range targets {
		env = append(env, domain.EnvTarget{
			Service: target.Service, File: target.File, Key: target.Key, Template: target.Template,
		})
	}
	copyFiles := filterCopy(det.CopyFromMain, services)
	cfg := domain.Config{
		Version: domain.ConfigVersion, Project: projectName, Services: services, Env: env,
		Bootstrap: domain.Bootstrap{CopyFromMain: copyFiles},
	}
	sources := buildSources(explicit, det, services, targets, copyFiles, projectName, projectSource)
	sort.Strings(warnings)
	return mergedScan{Config: cfg, Sources: sources, Warnings: warnings, Gaps: gaps}
}

func hasPortGap(gaps []scan.Gap, name domain.ServiceName) bool {
	field := "services." + string(name) + ".mainPort"
	for _, gap := range gaps {
		if gap.Field == field {
			return true
		}
	}
	return false
}

func deriveStartPorts(services map[domain.ServiceName]domain.Service) []scan.Gap {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, string(name))
	}
	sort.Strings(names)
	used := map[domain.Port]bool{}
	for _, svc := range services {
		if svc.MainPort.Valid() {
			used[svc.MainPort] = true
		}
		if svc.StartPort.Valid() {
			used[svc.StartPort] = true
		}
	}
	var gaps []scan.Gap
	for _, raw := range names {
		name := domain.ServiceName(raw)
		svc := services[name]
		if svc.StartPort.Valid() || !svc.MainPort.Valid() {
			continue
		}
		found := false
		for port := svc.MainPort + 1; port <= domain.MaxPort; port++ {
			if used[port] {
				continue
			}
			svc.StartPort = port
			services[name] = svc
			used[port] = true
			found = true
			break
		}
		if !found {
			gaps = append(gaps, scan.Gap{
				Field: "services." + raw + ".startPort", Reason: "missing",
				Message: "no free start port above mainPort",
				Flags:   []string{fmt.Sprintf("--service %s:%d:<start>", raw, svc.MainPort)},
				Service: name,
			})
		}
	}
	return gaps
}

func filterEnv(det []scan.EnvTarget, services map[domain.ServiceName]domain.Service, warnings *[]string) []scan.EnvTarget {
	out := make([]scan.EnvTarget, 0, len(det))
	seen := map[string]bool{}
	for _, target := range det {
		if _, ok := services[target.Service]; !ok {
			continue
		}
		key := target.File + "\x00" + target.Key
		if seen[key] {
			*warnings = append(*warnings, "ignoring duplicate env target "+target.File+":"+target.Key)
			continue
		}
		seen[key] = true
		out = append(out, target)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func filterCopy(files []scan.CopyFile, services map[domain.ServiceName]domain.Service) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(files))
	for _, file := range files {
		if _, ok := services[file.Service]; !ok {
			continue
		}
		if seen[file.File] {
			continue
		}
		seen[file.File] = true
		out = append(out, file.File)
	}
	sort.Strings(out)
	return out
}

func buildSources(explicit map[domain.ServiceName]domain.Service, det scan.Detected, services map[domain.ServiceName]domain.Service, env []scan.EnvTarget, copyFiles []string, projectName domain.Project, projectSource string) []ScanSource {
	sources := make([]ScanSource, 0)
	if projectSource != "" {
		sources = append(sources, ScanSource{Field: "project", Value: string(projectName), Source: projectSource})
	}
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, string(name))
	}
	sort.Strings(names)
	detectedByName := map[domain.ServiceName]scan.Service{}
	for _, svc := range det.Services {
		detectedByName[svc.Name] = svc
	}
	for _, raw := range names {
		name := domain.ServiceName(raw)
		svc := services[name]
		detected, isDetected := detectedByName[name]
		if _, ok := explicit[name]; ok {
			sources = append(sources,
				ScanSource{Field: "services." + raw + ".name", Value: raw, Source: "flag"},
				ScanSource{Field: "services." + raw + ".mainPort", Value: strconv.Itoa(int(svc.MainPort)), Source: "flag"},
				ScanSource{Field: "services." + raw + ".startPort", Value: strconv.Itoa(int(svc.StartPort)), Source: "flag"},
			)
			continue
		}
		if isDetected && detected.NameSource != "" {
			sources = append(sources, ScanSource{
				Field: "services." + raw + ".name", Value: raw, Source: detected.NameSource,
			})
		}
		if isDetected && detected.MainPort != nil {
			sources = append(sources, ScanSource{
				Field: "services." + raw + ".mainPort", Value: detected.MainPort.Value, Source: detected.MainPort.Source,
			})
		}
		if svc.StartPort.Valid() {
			sources = append(sources, ScanSource{
				Field: "services." + raw + ".startPort", Value: strconv.Itoa(int(svc.StartPort)), Source: "derived",
			})
		}
	}
	for i, target := range env {
		sources = append(sources, ScanSource{
			Field: fmt.Sprintf("env[%d]", i), Value: target.File + ":" + target.Key, Source: target.Source,
		})
	}
	for i, file := range copyFiles {
		sources = append(sources, ScanSource{
			Field: fmt.Sprintf("bootstrap.copyFromMain[%d]", i), Value: file, Source: "file exists in main",
		})
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Field != sources[j].Field {
			return sources[i].Field < sources[j].Field
		}
		return sources[i].Value < sources[j].Value
	})
	return sources
}

func scanIncomplete(cfg domain.Config, merged mergedScan) error {
	err := apperr.New(
		"scan_incomplete",
		"scan could not determine all required values",
		"add the missing flags or use --json to inspect the proposal",
	)
	err.Details = map[string]any{
		"proposal": cfg,
		"sources":  merged.Sources,
		"gaps":     merged.Gaps,
		"warnings": merged.Warnings,
	}
	return err
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
