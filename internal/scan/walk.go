package scan

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gomezv/portucca/internal/domain"
)

const (
	DefaultMaxEntries   = 50000
	DefaultMaxFileBytes = 1 << 20
)

type Value struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

type Service struct {
	Name       domain.ServiceName
	Dir        string
	NameSource string
	MainPort   *Value
}

type EnvTarget struct {
	Service    domain.ServiceName
	File       string
	Key        string
	Template   string
	SourceFile string
	Source     string
}

type CopyFile struct {
	Service domain.ServiceName
	File    string
}

type Gap struct {
	Field   string             `json:"field"`
	Reason  string             `json:"reason"`
	Message string             `json:"message"`
	Flags   []string           `json:"flags,omitempty"`
	Service domain.ServiceName `json:"-"`
}

type Detected struct {
	Project      *Value
	Services     []Service
	Env          []EnvTarget
	CopyFromMain []CopyFile
	Warnings     []string
	Gaps         []Gap
}

type Options struct {
	Root         string
	MainRoot     string
	MaxEntries   int
	MaxFileBytes int64
}

type TooLargeError struct {
	Limit   int
	Visited int
}

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("scan visited %d entries, limit is %d", e.Visited, e.Limit)
}

type packageInfo struct {
	name   string
	source string
	hasRun bool
}

type composeSignal struct {
	name   domain.ServiceName
	port   int
	source string
}

type portSignal struct {
	rank          int
	value         int
	source        string
	file          string
	key           string
	invalid       bool
	invalidReason string
}

type urlSignal struct {
	rank     int
	port     int
	source   string
	file     string
	key      string
	template string
}

type candidate struct {
	dir        string
	name       domain.ServiceName
	nameSource string
	ports      []portSignal
	urls       []urlSignal
}

type scanner struct {
	opts     Options
	ctx      context.Context
	byDir    map[string]*candidate
	packages map[string]packageInfo
	compose  []composeSignal
	warnings []string
	visited  int
}

func skipDir(name string) bool {
	switch name {
	case "node_modules", "vendor", "dist", "build", "coverage", "venv", "__pycache__", "bin", "obj", "target":
		return true
	}
	return strings.HasPrefix(name, ".")
}

func isEnvName(name string) bool {
	return name == ".env" || strings.HasPrefix(name, ".env.")
}

func (s *scanner) walk(ctx context.Context, onFile func(path, rel string)) error {
	return filepath.WalkDir(s.opts.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if path != s.opts.Root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		s.visited++
		if s.visited > s.opts.MaxEntries {
			return &TooLargeError{Limit: s.opts.MaxEntries, Visited: s.visited}
		}
		if !d.Type().IsRegular() {
			if isEnvName(d.Name()) {
				s.warn(fmt.Sprintf("ignoring non-regular env file %s", s.rel(path)))
			}
			return nil
		}
		onFile(path, s.rel(path))
		return nil
	})
}

func (s *scanner) rel(path string) string {
	rel, err := filepath.Rel(s.opts.Root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func (s *scanner) warn(msg string) {
	s.warnings = append(s.warnings, msg)
}
