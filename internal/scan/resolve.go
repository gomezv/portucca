package scan

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gomezv/portucca/internal/domain"
)

func (s *scanner) result() Detected {
	if len(s.byDir) > 1 {
		if c, ok := s.byDir["."]; ok {
			pkg := s.packages["."]
			switch {
			case !pkg.hasRun:
				delete(s.byDir, ".")
				s.warn("ignoring root .env/package.json signals in a multi-service repository; use --service to declare a root service")
			case !s.rootHasPortSignal(c, pkg):
				delete(s.byDir, ".")
				s.warn("ignoring root package.json scripts without a port in a multi-service repository; treating the root as an orchestrator (pass --service <name>:<main>:<start> to declare a root service)")
			default:
				ignoredEnv := false
				ports := c.ports[:0]
				for _, p := range c.ports {
					if p.file != "" {
						ignoredEnv = true
						continue
					}
					ports = append(ports, p)
				}
				c.ports = ports
				if ignoredEnv {
					s.warn("ignoring root .env port signals in a multi-service repository; use --service to declare a root service")
				}
			}
		}
	}
	dirs := make([]string, 0, len(s.byDir))
	for dir := range s.byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	for _, dir := range dirs {
		s.byDir[dir].resolveName(s.opts.Root, s.packages[dir])
	}
	seenNames := map[domain.ServiceName]bool{}
	for _, dir := range dirs {
		if name := s.byDir[dir].name; name != "" {
			seenNames[name] = true
		}
	}
	for name := range s.composeNames {
		if name != "" {
			seenNames[name] = true
		}
	}
	if len(s.exclude) > 0 {
		kept := make([]string, 0, len(dirs))
		for _, dir := range dirs {
			if s.exclude[s.byDir[dir].name] {
				continue
			}
			kept = append(kept, dir)
		}
		dirs = kept
	}
	nameGaps := resolveNameCollisions(dirs, s.byDir, &s.warnings)

	for _, cs := range s.compose {
		for _, dir := range dirs {
			c := s.byDir[dir]
			if c.name == cs.name {
				c.ports = append(c.ports, portSignal{rank: rankCompose, value: cs.port, source: cs.source})
			}
		}
	}

	services := make([]Service, 0, len(dirs))
	env := make([]EnvTarget, 0)
	copySignals := make([]CopyFile, 0)
	gaps := append([]Gap{}, nameGaps...)
	warnings := append([]string{}, s.warnings...)
	for _, dir := range dirs {
		c := s.byDir[dir]
		mainPort, cgaps, cwarnings := c.resolveMainPort()
		gaps = append(gaps, cgaps...)
		warnings = append(warnings, cwarnings...)
		if c.name == "" {
			gaps = append(gaps, Gap{
				Field:   "services",
				Reason:  "invalid",
				Message: "could not derive a service name for " + c.dir,
				Flags:   []string{"--service <name>:<main>:<start>"},
			})
			continue
		}
		services = append(services, Service{
			Name: c.name, Dir: c.dir, NameSource: c.nameSource, MainPort: mainPort,
		})
		env = append(env, c.portTargets(mainPort)...)
		for _, file := range c.portSignalFiles() {
			copySignals = append(copySignals, CopyFile{Service: c.name, File: file})
		}
	}

	if s.opts.DiscoverCompose {
		services, gaps = s.discoverCompose(services, gaps)
	}
	ports := map[int][]domain.ServiceName{}
	for _, svc := range services {
		if svc.MainPort == nil {
			continue
		}
		port, err := strconv.Atoi(svc.MainPort.Value)
		if err == nil {
			ports[port] = append(ports[port], svc.Name)
		}
	}
	for _, dir := range dirs {
		for _, u := range s.byDir[dir].urls {
			names := ports[u.port]
			switch len(names) {
			case 1:
				env = append(env, EnvTarget{
					Service: names[0], File: u.file, Key: u.key, Template: u.template,
					SourceFile: u.file, Source: u.source,
				})
			case 0:
			default:
				warnings = append(warnings, fmt.Sprintf("ignoring %s: port %d matches multiple services", u.source, u.port))
			}
		}
	}

	env = dedupeEnv(env, &warnings)
	copyFiles := s.copyFromMain(env, copySignals, &warnings)
	if s.opts.DiscoverCompose && !s.composeSeen {
		warnings = append(warnings, "no compose file found; compose discovery skipped")
	}
	for name := range s.exclude {
		if !seenNames[name] {
			warnings = append(warnings, "ignoring --exclude-service "+string(name)+": no matching service")
		}
	}
	sortServices(services)
	sortEnv(env)
	sort.Strings(warnings)
	sortGaps(gaps)
	return Detected{
		Project: detectProject(s.ctx, s.opts.Root), Services: services, Env: env,
		CopyFromMain: copyFiles, Warnings: warnings, Gaps: gaps,
	}
}

func resolveNameCollisions(dirs []string, byDir map[string]*candidate, warnings *[]string) []Gap {
	groups := map[domain.ServiceName][]*candidate{}
	for _, dir := range dirs {
		c := byDir[dir]
		if c.name != "" {
			groups[c.name] = append(groups[c.name], c)
		}
	}
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		for _, c := range group {
			base := c.name
			c.name = domain.ServiceName(slugify(strings.ReplaceAll(strings.TrimPrefix(c.dir, "./"), "/", "-")))
			if c.name == "" {
				c.name = base
			}
			c.nameSource = c.dir + ":path"
		}
	}
	final := map[domain.ServiceName][]*candidate{}
	for _, dir := range dirs {
		c := byDir[dir]
		if c.name != "" {
			final[c.name] = append(final[c.name], c)
		}
	}
	var gaps []Gap
	for name, group := range final {
		if len(group) < 2 {
			continue
		}
		gaps = append(gaps, Gap{
			Field:   "services." + string(name),
			Reason:  "ambiguous",
			Message: "service name collision; pass --service to choose",
			Flags:   serviceFlag(name),
			Service: name,
		})
	}
	used := map[domain.ServiceName]bool{}
	for _, dir := range dirs {
		c := byDir[dir]
		if c.name == "" {
			continue
		}
		base := c.name
		c.name = uniqueName(base, used)
		if c.name != base {
			*warnings = append(*warnings, fmt.Sprintf("service name collision: %s renamed to %s; pass --service to choose", base, c.name))
		}
		used[c.name] = true
	}
	return gaps
}

func uniqueName(base domain.ServiceName, used map[domain.ServiceName]bool) domain.ServiceName {
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := domain.ServiceName(fmt.Sprintf("%s-%d", base, i))
		if !used[candidate] {
			return candidate
		}
	}
}

func (c *candidate) resolveName(root string, pkg packageInfo) {
	if pkg.name != "" {
		c.name = domain.ServiceName(serviceNameFromPackage(pkg.name))
		c.nameSource = pkg.source
		return
	}
	base := path.Base(c.dir)
	if c.dir == "." {
		base = filepath.Base(filepath.Clean(root))
	}
	c.name = domain.ServiceName(slugify(base))
	c.nameSource = c.dir + ":directory"
}

func (c *candidate) resolveMainPort() (*Value, []Gap, []string) {
	if len(c.ports) == 0 {
		return nil, nil, nil
	}
	best := c.ports[0].rank
	for _, p := range c.ports {
		if p.rank < best {
			best = p.rank
		}
	}
	at := make([]portSignal, 0, len(c.ports))
	for _, p := range c.ports {
		if p.rank == best {
			at = append(at, p)
		}
	}
	sort.Slice(at, func(i, j int) bool { return at[i].source < at[j].source })
	for _, p := range at {
		if p.invalid {
			return nil, []Gap{{
				Field:   "services." + string(c.name) + ".mainPort",
				Reason:  "invalid",
				Message: p.invalidReason,
				Flags:   serviceFlag(c.name),
				Service: c.name,
			}}, nil
		}
	}
	values := map[int][]string{}
	for _, p := range at {
		values[p.value] = append(values[p.value], p.source)
	}
	if len(values) > 1 {
		sources := make([]string, 0, len(at))
		for _, srcs := range values {
			sources = append(sources, srcs...)
		}
		sort.Strings(sources)
		return nil, []Gap{{
			Field:   "services." + string(c.name) + ".mainPort",
			Reason:  "ambiguous",
			Message: "conflicting values in " + strings.Join(sources, ", "),
			Flags:   serviceFlag(c.name),
			Service: c.name,
		}}, nil
	}
	chosen := at[0]
	warnings := make([]string, 0)
	for _, p := range c.ports {
		if p.rank == best || p.invalid {
			continue
		}
		if p.value != chosen.value {
			warnings = append(warnings, fmt.Sprintf("ignoring %s=%d because %s wins", p.source, p.value, chosen.source))
		}
	}
	if best == rankEnvExample {
		warnings = append(warnings, fmt.Sprintf(
			"using example file %s; create %s or pass --service to override",
			chosen.file, path.Join(c.dir, ".env"),
		))
	}
	return &Value{Value: strconv.Itoa(chosen.value), Source: chosen.source}, nil, warnings
}

func (s *scanner) rootHasPortSignal(c *candidate, pkg packageInfo) bool {
	for _, p := range c.ports {
		if p.file == "" {
			return true
		}
	}
	if len(s.compose) == 0 {
		return false
	}
	name := domain.ServiceName(slugify(filepath.Base(filepath.Clean(s.opts.Root))))
	if pkg.name != "" {
		name = domain.ServiceName(serviceNameFromPackage(pkg.name))
	}
	for _, cs := range s.compose {
		if cs.name == name {
			return true
		}
	}
	return false
}

func (c *candidate) portTargets(mainPort *Value) []EnvTarget {
	if mainPort == nil {
		return nil
	}
	targets := make([]EnvTarget, 0, len(c.ports))
	for _, p := range c.ports {
		if p.invalid || p.file == "" || p.key == "" {
			continue
		}
		file := p.file
		if p.rank == rankEnvExample {
			if p.source != mainPort.Source {
				continue
			}
			file = path.Join(c.dir, ".env")
		} else if strconv.Itoa(p.value) != mainPort.Value {
			continue
		}
		targets = append(targets, EnvTarget{
			Service: c.name, File: file, Key: p.key, SourceFile: p.file, Source: p.source,
		})
	}
	return targets
}

func (c *candidate) portSignalFiles() []string {
	files := make([]string, 0, len(c.ports))
	for _, p := range c.ports {
		if p.invalid || p.file == "" || p.key == "" || p.rank == rankEnvExample {
			continue
		}
		files = append(files, p.file)
	}
	return files
}

func serviceFlag(name domain.ServiceName) []string {
	return []string{fmt.Sprintf("--service %s:<main>:<start>", name)}
}

func (s *scanner) copyFromMain(env []EnvTarget, extra []CopyFile, warnings *[]string) []CopyFile {
	seen := map[CopyFile]bool{}
	for _, target := range env {
		seen[CopyFile{Service: target.Service, File: target.SourceFile}] = true
	}
	for _, file := range extra {
		seen[file] = true
	}
	out := make([]CopyFile, 0, len(seen))
	for file := range seen {
		if file.File == "" || isExampleEnv(path.Base(file.File)) {
			continue
		}
		info, err := os.Lstat(filepath.Join(s.opts.MainRoot, filepath.FromSlash(file.File)))
		if err == nil && info.Mode().IsRegular() {
			out = append(out, file)
			continue
		}
		if s.opts.Root != s.opts.MainRoot {
			*warnings = append(*warnings, file.File+" is not present in the main checkout; it will not be copied")
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Service < out[j].Service
	})
	return out
}

func dedupeEnv(env []EnvTarget, warnings *[]string) []EnvTarget {
	sortEnv(env)
	out := make([]EnvTarget, 0, len(env))
	seen := map[string]bool{}
	for _, target := range env {
		key := target.File + "\x00" + target.Key
		if seen[key] {
			*warnings = append(*warnings, fmt.Sprintf("ignoring duplicate env target %s:%s", target.File, target.Key))
			continue
		}
		seen[key] = true
		out = append(out, target)
	}
	return out
}

func sortServices(services []Service) {
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
}

func sortEnv(env []EnvTarget) {
	sort.Slice(env, func(i, j int) bool {
		if env[i].File != env[j].File {
			return env[i].File < env[j].File
		}
		return env[i].Key < env[j].Key
	})
}

func sortGaps(gaps []Gap) {
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].Field != gaps[j].Field {
			return gaps[i].Field < gaps[j].Field
		}
		if gaps[i].Reason != gaps[j].Reason {
			return gaps[i].Reason < gaps[j].Reason
		}
		return gaps[i].Message < gaps[j].Message
	})
}

func (s *scanner) discoverCompose(services []Service, gaps []Gap) ([]Service, []Gap) {
	existing := make(map[domain.ServiceName]bool, len(services))
	for _, svc := range services {
		existing[svc.Name] = true
	}
	groups := map[domain.ServiceName][]composeSignal{}
	order := make([]string, 0)
	for _, cs := range s.compose {
		if s.exclude[cs.name] {
			continue
		}
		if _, ok := groups[cs.name]; !ok {
			order = append(order, string(cs.name))
		}
		groups[cs.name] = append(groups[cs.name], cs)
	}
	sort.Strings(order)
	for _, raw := range order {
		name := domain.ServiceName(raw)
		if !domain.ValidServiceName(raw) {
			field := "services"
			if raw != "" {
				field = "services." + raw
			}
			gaps = append(gaps, Gap{
				Field: field, Reason: "invalid",
				Message: "compose service name does not normalize to a valid service name",
				Flags:   []string{"--service <name>:<main>:<start>"},
				Service: name,
			})
			continue
		}
		signals := groups[name]
		sort.Slice(signals, func(i, j int) bool { return signals[i].source < signals[j].source })
		rawNames := map[string]bool{}
		for _, cs := range signals {
			rawNames[cs.rawName] = true
		}
		if len(rawNames) > 1 {
			names := make([]string, 0, len(rawNames))
			for rawName := range rawNames {
				names = append(names, rawName)
			}
			sort.Strings(names)
			gaps = append(gaps, Gap{
				Field: "services." + raw + ".name", Reason: "ambiguous",
				Message: "compose services " + strings.Join(names, ", ") + " normalize to the same name",
				Flags:   []string{"--exclude-service " + raw, "--service <name>:<main>:<start>"},
				Service: name,
			})
			continue
		}
		if existing[name] {
			continue
		}
		ports := map[int]string{}
		for _, cs := range signals {
			ports[cs.port] = cs.source
		}
		if len(ports) > 1 {
			sources := make([]string, 0, len(ports))
			for _, source := range ports {
				sources = append(sources, source)
			}
			sort.Strings(sources)
			gaps = append(gaps, Gap{
				Field: "services." + raw + ".mainPort", Reason: "ambiguous",
				Message: "conflicting compose ports in " + strings.Join(sources, ", "),
				Flags:   []string{"--exclude-service " + raw, "--service " + raw + ":<main>:<start>"},
				Service: name,
			})
			continue
		}
		signal := signals[0]
		services = append(services, Service{
			Name: name, Dir: signal.dir, NameSource: signal.nameSource,
			MainPort: &Value{Value: strconv.Itoa(signal.port), Source: signal.source},
		})
		existing[name] = true
	}
	return services, gaps
}
