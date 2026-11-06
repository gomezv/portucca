package scan

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/envfile"
)

const (
	rankEnvLocal = iota + 1
	rankEnv
	rankEnvOther
	rankEnvExample
	rankScript
	rankCompose
)

func (s *scanner) candidate(dir string) *candidate {
	dir = path.Clean(strings.ReplaceAll(dir, "\\", "/"))
	if dir == "" {
		dir = "."
	}
	c := s.byDir[dir]
	if c == nil {
		c = &candidate{dir: dir}
		s.byDir[dir] = c
	}
	return c
}

func (s *scanner) readPackage(file, rel string) {
	data, err := s.readFile(file)
	if err != nil {
		s.warn(fmt.Sprintf("ignoring %s: %v", rel, err))
		return
	}
	var pkg struct {
		Name    string            `json:"name"`
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		s.warn(fmt.Sprintf("ignoring %s: %v", rel, err))
		return
	}
	dir := path.Dir(rel)
	hasRun := false
	var signals []portSignal
	for _, script := range []string{"dev", "start", "serve"} {
		spec, ok := pkg.Scripts[script]
		if !ok {
			continue
		}
		hasRun = true
		if port, ok := parseScriptPort(spec); ok {
			sig := portSignal{
				rank: rankScript, source: rel + ":scripts." + script,
			}
			if !domain.Port(port).Valid() {
				sig.invalid = true
				sig.invalidReason = fmt.Sprintf("%s has invalid port %q", sig.source, strconv.Itoa(port))
			} else {
				sig.value = port
			}
			signals = append(signals, sig)
			break
		}
	}
	if pkg.Name != "" || hasRun {
		s.packages[dir] = packageInfo{name: pkg.Name, source: rel + ":name", hasRun: hasRun}
	}
	if !hasRun && len(signals) == 0 {
		return
	}
	c := s.candidate(dir)
	c.ports = append(c.ports, signals...)
}

func (s *scanner) readEnv(file, rel string) {
	data, err := s.readFile(file)
	if err != nil {
		s.warn(fmt.Sprintf("ignoring %s: %v", rel, err))
		return
	}
	doc, err := envfile.Parse(data)
	if err != nil {
		s.warn(fmt.Sprintf("ignoring %s: %v", rel, err))
		return
	}
	name := path.Base(rel)
	rank := envRank(name)
	example := rank == rankEnvExample
	var ports []portSignal
	var urls []urlSignal
	for _, entry := range doc.Entries() {
		if isPortKey(entry.Key) {
			port, err := strconv.Atoi(entry.Value)
			sig := portSignal{
				rank: rank, source: rel + ":" + entry.Key, file: rel, key: entry.Key,
			}
			if err != nil || !domain.Port(port).Valid() {
				sig.invalid = true
				sig.invalidReason = fmt.Sprintf("%s has invalid port %q", sig.source, entry.Value)
			} else {
				sig.value = port
			}
			ports = append(ports, sig)
		}
		if example {
			continue
		}
		if port, template, ok := parseLoopbackURL(entry.Value); ok {
			urls = append(urls, urlSignal{
				rank: rank, port: port, source: rel + ":" + entry.Key,
				file: rel, key: entry.Key, template: template,
			})
		} else if (strings.HasPrefix(entry.Value, "http://") || strings.HasPrefix(entry.Value, "https://")) && strings.ContainsAny(entry.Value, "@?#") {
			s.warn(fmt.Sprintf("ignoring %s: URL templates with userinfo, query or fragment are not supported", rel+":"+entry.Key))
		}
	}
	if len(ports) == 0 && len(urls) == 0 {
		return
	}
	c := s.candidate(path.Dir(rel))
	c.ports = append(c.ports, ports...)
	c.urls = append(c.urls, urls...)
}

func (s *scanner) readCompose(file, rel string) {
	data, err := s.readFile(file)
	if err != nil {
		s.warn(fmt.Sprintf("ignoring %s: %v", rel, err))
		return
	}
	var compose struct {
		Services map[string]struct {
			Ports []json.RawMessage `json:"ports"`
		} `json:"services"`
	}
	if err := json.Unmarshal(data, &compose); err != nil {
		s.warn(fmt.Sprintf("ignoring %s: %v", rel, err))
		return
	}
	names := make([]string, 0, len(compose.Services))
	for name := range compose.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		svc := compose.Services[name]
		port, ok := publishedPort(svc.Ports)
		if !ok {
			if len(svc.Ports) > 0 {
				s.warn(fmt.Sprintf("ignoring %s: services.%s has multiple or unsupported published ports", rel, name))
			}
			continue
		}
		if !domain.Port(port).Valid() {
			s.warn(fmt.Sprintf("ignoring %s:services.%s.ports: port %d is outside 1024..65535", rel, name, port))
			continue
		}
		s.compose = append(s.compose, composeSignal{
			name: domain.ServiceName(slugify(name)), port: port,
			source: fmt.Sprintf("%s:services.%s.ports", rel, name),
		})
	}
}

func (s *scanner) readFile(file string) ([]byte, error) {
	info, err := os.Lstat(file)
	if err != nil {
		return nil, err
	}
	if info.Size() > s.opts.MaxFileBytes {
		return nil, fmt.Errorf("file is larger than %d bytes", s.opts.MaxFileBytes)
	}
	return os.ReadFile(file)
}

func isExampleEnv(name string) bool {
	if !strings.HasPrefix(name, ".env") {
		return false
	}
	for _, part := range strings.Split(strings.TrimPrefix(name, ".env"), ".") {
		switch part {
		case "example", "sample", "template", "dist":
			return true
		}
	}
	return false
}

func envRank(name string) int {
	switch {
	case name == ".env.local":
		return rankEnvLocal
	case name == ".env":
		return rankEnv
	case isExampleEnv(name):
		return rankEnvExample
	default:
		return rankEnvOther
	}
}

func isPortKey(key string) bool {
	upper := strings.ToUpper(key)
	return upper == "PORT" || strings.HasSuffix(upper, "_PORT")
}

var scriptPort = regexp.MustCompile(`(?:^|\s)(?:--port|-p)(?:=|\s+)(\d{1,5})(?:\s|$)`)

func parseScriptPort(script string) (int, bool) {
	m := scriptPort.FindStringSubmatch(script)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

var loopbackURL = regexp.MustCompile(`^(https?)://(localhost|127\.0\.0\.1):(\d{1,5})(/[^\s]*)?$`)

func parseLoopbackURL(value string) (int, string, bool) {
	if strings.ContainsAny(value, "@?#") {
		return 0, "", false
	}
	m := loopbackURL.FindStringSubmatch(value)
	if m == nil {
		return 0, "", false
	}
	port, err := strconv.Atoi(m[3])
	if err != nil || !domain.Port(port).Valid() {
		return 0, "", false
	}
	template := m[1] + "://" + m[2] + ":{{port}}" + m[4]
	return port, template, true
}

func publishedPort(ports []json.RawMessage) (int, bool) {
	var out []int
	for _, raw := range ports {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			if port, ok := parseHostPort(str); ok {
				out = append(out, port)
			}
			continue
		}
		var obj struct {
			Published json.RawMessage `json:"published"`
		}
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		var n int
		if err := json.Unmarshal(obj.Published, &n); err == nil {
			out = append(out, n)
			continue
		}
		var s string
		if err := json.Unmarshal(obj.Published, &s); err == nil {
			if n, err := strconv.Atoi(s); err == nil {
				out = append(out, n)
			}
		}
	}
	if len(out) != 1 {
		return 0, false
	}
	return out[0], true
}

func parseHostPort(value string) (int, bool) {
	value = strings.TrimSuffix(value, "/tcp")
	value = strings.TrimSuffix(value, "/udp")
	parts := strings.Split(value, ":")
	if len(parts) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return 0, false
	}
	return n, true
}

func (s *scanner) detectFile(file, rel string) {
	switch {
	case path.Base(rel) == "package.json":
		s.readPackage(file, rel)
	case isEnvName(path.Base(rel)):
		s.readEnv(file, rel)
	case path.Base(rel) == "docker-compose.json" || path.Base(rel) == "compose.json":
		s.readCompose(file, rel)
	}
}
