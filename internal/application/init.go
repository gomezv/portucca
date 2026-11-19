package application

import (
	"context"
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
	Env             []EnvBinding
	WrapScript      bool
}

type EnvBinding struct {
	Service domain.ServiceName
	File    string
	Key     string
}

type InitResult struct {
	ConfigPath    string                                `json:"configPath"`
	Project       domain.Project                        `json:"project"`
	Services      map[domain.ServiceName]domain.Service `json:"services"`
	Warning       string                                `json:"warning,omitempty"`
	Scan          *InitScan                             `json:"scan,omitempty"`
	PackageScript *DevScriptChange                      `json:"packageScript"`
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
	Config        domain.Config
	Sources       []ScanSource
	Warnings      []string
	Gaps          []scan.Gap
	EnvCandidates []EnvCandidate
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
	if !req.Scan {
		return a.initFromFlags(c, req)
	}
	if _, err := parseExplicitServices(req.Services); err != nil {
		return InitResult{}, err
	}
	if req.Project != "" && !domain.ValidProject(req.Project) {
		return InitResult{}, apperr.New("usage", "invalid project "+req.Project, "use a lowercase slug")
	}
	path, err := initConfigPath(c, req)
	if err != nil {
		return InitResult{}, err
	}
	if _, statErr := os.Stat(path); statErr == nil && !req.Force {
		return InitResult{}, apperr.New("config_exists", "portucca.json already exists", "use --force to overwrite it")
	}
	proposal, merged, err := a.analyzeInit(ctx, c, req)
	if err != nil {
		return InitResult{}, err
	}
	if len(merged.Gaps) > 0 {
		return InitResult{}, scanIncomplete(proposal.Config, merged)
	}
	return a.ApplyInit(ctx, proposal, ApplyInitOptions{Force: req.Force, WrapScript: req.WrapScript})
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

func mergeDetected(req InitRequest, det scan.Detected, explicit map[domain.ServiceName]domain.Service) (mergedScan, error) {
	det.Services = append([]scan.Service(nil), det.Services...)
	services := map[domain.ServiceName]domain.Service{}
	gaps := append([]scan.Gap{}, det.Gaps...)
	warnings := append([]string{}, det.Warnings...)
	usedRoot := map[string]bool{}
	conventions := map[domain.ServiceName]scan.EnvSignal{}
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
		for i := range det.Services {
			svc := &det.Services[i]
			if svc.MainPort != nil || hasPortGap(gaps, svc.Name) {
				continue
			}
			if sig, ok := conventionRootEnv(*svc, det.RootEnv, usedRoot); ok {
				svc.MainPort = &scan.Value{Value: strconv.Itoa(sig.Value), Source: sig.Source}
				usedRoot[rootEnvKey(sig)] = true
				conventions[svc.Name] = sig
			}
		}
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
						Message: missingPortMessage(svc),
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
	copySignals := append([]scan.CopyFile{}, det.CopyFromMain...)
	withTarget := map[domain.ServiceName]bool{}
	for _, target := range targets {
		withTarget[target.Service] = true
	}

	convNames := make([]string, 0, len(conventions))
	for name := range conventions {
		convNames = append(convNames, string(name))
	}
	sort.Strings(convNames)
	for _, raw := range convNames {
		name := domain.ServiceName(raw)
		sig := conventions[name]
		if withTarget[name] {
			warnings = append(warnings, "ignoring root env "+sig.Source+": service "+raw+" already has an env target")
			continue
		}
		targets = append(targets, rootEnvTarget(name, sig))
		copySignals = append(copySignals, scan.CopyFile{Service: name, File: sig.File})
		withTarget[name] = true
	}

	for _, raw := range sortedInitServiceNames(services) {
		name := domain.ServiceName(raw)
		svc := services[name]
		if withTarget[name] || !svc.MainPort.Valid() {
			continue
		}
		matches := make([]scan.EnvSignal, 0)
		for _, sig := range det.RootEnv {
			if !usedRoot[rootEnvKey(sig)] && sig.Value == int(svc.MainPort) {
				matches = append(matches, sig)
			}
		}
		if len(matches) != 1 {
			continue
		}
		sig := matches[0]
		targets = append(targets, rootEnvTarget(name, sig))
		copySignals = append(copySignals, scan.CopyFile{Service: name, File: sig.File})
		usedRoot[rootEnvKey(sig)] = true
		withTarget[name] = true
	}

	for _, binding := range req.Env {
		name := binding.Service
		if _, ok := services[name]; !ok {
			return mergedScan{}, apperr.New("usage", "unknown env service "+string(name), "use a service from the proposal")
		}
		if withTarget[name] {
			return mergedScan{}, apperr.New("usage", "service "+string(name)+" already has an env target", "choose a service without a target")
		}
		sig, ok := findRootEnv(det.RootEnv, usedRoot, binding.File, binding.Key)
		if !ok {
			return mergedScan{}, apperr.New("usage", "env binding "+binding.File+":"+binding.Key+" is not a remaining root candidate", "pick a candidate from the scan")
		}
		targets = append(targets, rootEnvTarget(name, sig))
		copySignals = append(copySignals, scan.CopyFile{Service: name, File: sig.File})
		usedRoot[rootEnvKey(sig)] = true
		withTarget[name] = true
	}

	candidates := make([]EnvCandidate, 0)
	for _, sig := range det.RootEnv {
		if usedRoot[rootEnvKey(sig)] {
			continue
		}
		matched := rootEnvMatches(sig, services)
		suggested := make([]domain.ServiceName, 0, len(matched))
		for _, name := range matched {
			if !withTarget[name] {
				suggested = append(suggested, name)
			}
		}
		if len(matched) > 0 && len(suggested) == 0 {
			warnings = append(warnings, "ignoring root env "+sig.Source+": matching services already have env targets")
			continue
		}
		candidates = append(candidates, EnvCandidate{
			Value: sig.Value, File: sig.File, Key: sig.Key, Source: sig.Source, Suggested: suggested,
		})
		warnings = append(warnings, "root env "+sig.Source+" has no unique service target; use the TUI to map it")
	}

	targets = filterEnv(targets, services, &warnings)
	env := make([]domain.EnvTarget, 0, len(targets))
	for _, target := range targets {
		env = append(env, domain.EnvTarget{
			Service: target.Service, File: target.File, Key: target.Key, Template: target.Template,
		})
	}
	copyFiles := filterCopy(copySignals, services)
	cfg := domain.Config{
		Version: domain.ConfigVersion, Project: projectName, Services: services, Env: env,
		Bootstrap: domain.Bootstrap{CopyFromMain: copyFiles},
	}
	sources := buildSources(explicit, det, services, targets, copyFiles, projectName, projectSource)
	sort.Strings(warnings)
	return mergedScan{Config: cfg, Sources: sources, Warnings: warnings, Gaps: gaps, EnvCandidates: candidates}, nil
}

func conventionPortKey(name domain.ServiceName) string {
	key := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(string(name)))
	return key + "_PORT"
}

func rootEnvKey(sig scan.EnvSignal) string {
	return sig.File + "\x00" + sig.Key
}

func conventionRootEnv(svc scan.Service, signals []scan.EnvSignal, used map[string]bool) (scan.EnvSignal, bool) {
	key := conventionPortKey(svc.Name)
	var best scan.EnvSignal
	found := false
	for _, sig := range signals {
		if used[rootEnvKey(sig)] || strings.ToUpper(sig.Key) != key {
			continue
		}
		if !found || sig.Rank < best.Rank {
			best, found = sig, true
			continue
		}
		if sig.Rank == best.Rank {
			return scan.EnvSignal{}, false
		}
	}
	return best, found
}

func rootEnvTarget(name domain.ServiceName, sig scan.EnvSignal) scan.EnvTarget {
	return scan.EnvTarget{Service: name, File: sig.File, Key: sig.Key, Source: sig.Source}
}

func findRootEnv(signals []scan.EnvSignal, used map[string]bool, file, key string) (scan.EnvSignal, bool) {
	for _, sig := range signals {
		if !used[rootEnvKey(sig)] && sig.File == file && sig.Key == key {
			return sig, true
		}
	}
	return scan.EnvSignal{}, false
}

func rootEnvMatches(sig scan.EnvSignal, services map[domain.ServiceName]domain.Service) []domain.ServiceName {
	set := map[domain.ServiceName]bool{}
	for name, svc := range services {
		if svc.MainPort.Valid() && int(svc.MainPort) == sig.Value {
			set[name] = true
		}
		if strings.ToUpper(sig.Key) == conventionPortKey(name) {
			set[name] = true
		}
	}
	out := make([]domain.ServiceName, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedInitServiceNames(services map[domain.ServiceName]domain.Service) []string {
	out := make([]string, 0, len(services))
	for name := range services {
		out = append(out, string(name))
	}
	sort.Strings(out)
	return out
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

func missingPortMessage(svc scan.Service) string {
	where := "the repository root"
	if svc.Dir != "" && svc.Dir != "." {
		where = "directory " + svc.Dir
	}
	checked := "no package.json, env files, or Compose signals were found"
	if len(svc.Checked) > 0 {
		checked = "checked " + joinEnglish(svc.Checked)
	}
	return fmt.Sprintf(
		"no port declaration found for service %s at %s; %s; framework defaults are not assumed; add PORT=<port> to an env file or pass --service %s:<main>:<start> (example: --service %s:3000:3001)",
		svc.Name, where, checked, svc.Name, svc.Name,
	)
}

func joinEnglish(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}
