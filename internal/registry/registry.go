package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/gomezv/portucca/internal/domain"
)

var (
	ErrCorrupt = errors.New("registry_corrupt")
	ErrVersion = errors.New("registry_version")
)

type Store struct {
	Path string
}

func DefaultPath() (string, error) {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "portucca", "ports.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "portucca", "ports.json"), nil
}

func (s Store) Load() (domain.Registry, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.Registry{Version: domain.RegistryVersion}, nil
	}
	if err != nil {
		return domain.Registry{}, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return domain.Registry{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	raw, ok := top["worktrees"]
	if !ok {
		return domain.Registry{}, fmt.Errorf("%w: missing worktrees", ErrCorrupt)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return domain.Registry{}, fmt.Errorf("%w: null worktrees", ErrCorrupt)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var reg domain.Registry
	if err := dec.Decode(&reg); err != nil {
		return domain.Registry{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return domain.Registry{}, fmt.Errorf("%w: trailing data", ErrCorrupt)
		}
		return domain.Registry{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if reg.Version != domain.RegistryVersion {
		return domain.Registry{}, fmt.Errorf("%w: %d", ErrVersion, reg.Version)
	}
	for _, claim := range reg.Worktrees {
		if claim.Project == "" || claim.Path == "" || len(claim.Services) == 0 {
			return domain.Registry{}, fmt.Errorf("%w: incomplete claim", ErrCorrupt)
		}
		for name, sc := range claim.Services {
			if name == "" || !sc.Port.Valid() {
				return domain.Registry{}, fmt.Errorf("%w: invalid service claim", ErrCorrupt)
			}
		}
	}
	return reg, nil
}

func (s Store) Save(reg domain.Registry) error {
	reg.Version = domain.RegistryVersion
	if reg.Worktrees == nil {
		reg.Worktrees = []domain.Claim{}
	}
	claims := make([]domain.Claim, len(reg.Worktrees))
	copy(claims, reg.Worktrees)
	reg.Worktrees = claims
	for i := range reg.Worktrees {
		reg.Worktrees[i].UpdatedAt = reg.Worktrees[i].UpdatedAt.UTC()
	}
	sort.Slice(reg.Worktrees, func(i, j int) bool {
		if reg.Worktrees[i].Project != reg.Worktrees[j].Project {
			return reg.Worktrees[i].Project < reg.Worktrees[j].Project
		}
		return reg.Worktrees[i].Path < reg.Worktrees[j].Path
	})
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".ports-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.Path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

func (s Store) LockPath() string { return s.Path + ".lock" }
