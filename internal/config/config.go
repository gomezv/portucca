package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gomezv/portucca/internal/domain"
)

const FileName = "portucca.json"

func LoadRoot(root string) (domain.Config, string, error) {
	path := filepath.Join(root, FileName)
	cfg, err := LoadFile(path)
	return cfg, path, err
}

func LoadFile(path string) (domain.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.Config{}, fmt.Errorf("config_missing: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var cfg domain.Config
	if err := dec.Decode(&cfg); err != nil {
		return domain.Config{}, fmt.Errorf("config_invalid: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return domain.Config{}, fmt.Errorf("config_invalid: %w", err)
	}
	return cfg, nil
}

func WriteFile(path string, cfg domain.Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".portucca-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o644); err != nil {
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
	return os.Rename(name, path)
}

type Loader struct{}

func (Loader) LoadRoot(root string) (domain.Config, string, error) { return LoadRoot(root) }
func (Loader) LoadFile(path string) (domain.Config, error)         { return LoadFile(path) }
