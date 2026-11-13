package packagescript

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrMissing     = errors.New("package script missing")
	ErrInvalid     = errors.New("package json invalid")
	ErrUnsupported = errors.New("package script unsupported")
)

type Change struct {
	Path    string
	Before  string
	After   string
	Changed bool
}

func Preview(path string) (Change, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Change{}, ErrMissing
		}
		return Change{}, err
	}
	change, _, _, err := inspect(path, data)
	return change, err
}

func Apply(path string) (Change, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Change{}, ErrMissing
		}
		return Change{}, err
	}
	change, start, end, err := inspect(path, data)
	if err != nil {
		return Change{}, err
	}
	if !change.Changed {
		return change, nil
	}
	quoted, err := json.Marshal(change.After)
	if err != nil {
		return Change{}, ErrInvalid
	}
	next := make([]byte, 0, len(data)-(end-start)+len(quoted))
	next = append(next, data[:start]...)
	next = append(next, quoted...)
	next = append(next, data[end:]...)
	if !json.Valid(next) {
		return Change{}, ErrInvalid
	}
	if err := writeAtomic(path, next); err != nil {
		return Change{}, err
	}
	return change, nil
}

func inspect(path string, data []byte) (Change, int, int, error) {
	if !json.Valid(data) {
		return Change{}, 0, 0, ErrInvalid
	}
	scriptsRaw, scriptsStart, err := findScripts(data)
	if err != nil {
		return Change{}, 0, 0, err
	}
	devRaw, devStart, err := findDev(scriptsRaw)
	if err != nil {
		return Change{}, 0, 0, err
	}
	var before string
	if err := json.Unmarshal(devRaw, &before); err != nil {
		return Change{}, 0, 0, ErrInvalid
	}
	valueStart := scriptsStart + devStart
	valueEnd := valueStart + len(devRaw)
	change := Change{Path: path, Before: before, After: before}
	if strings.HasPrefix(strings.TrimSpace(before), "portucca run") {
		return change, valueStart, valueEnd, nil
	}
	if !simple(before) {
		return Change{}, 0, 0, ErrUnsupported
	}
	change.After = "portucca run -- " + before
	change.Changed = true
	return change, valueStart, valueEnd, nil
}

func findScripts(data []byte) ([]byte, int, error) {
	return findKey(data, "scripts")
}

func findDev(scripts []byte) ([]byte, int, error) {
	return findKey(scripts, "dev")
}

func findKey(data []byte, keyName string) ([]byte, int, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, 0, ErrInvalid
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, 0, ErrInvalid
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, 0, ErrInvalid
		}
		before := int(dec.InputOffset())
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, 0, ErrInvalid
		}
		if key != keyName {
			continue
		}
		rel := bytes.Index(data[before:], raw)
		if rel < 0 {
			return nil, 0, ErrInvalid
		}
		return raw, before + rel, nil
	}
	return nil, 0, ErrMissing
}

func simple(value string) bool {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return false
	}
	if strings.Contains(fields[0], "=") && !strings.HasPrefix(fields[0], "-") {
		return false
	}
	for _, r := range value {
		switch r {
		case '&', '|', ';', '<', '>', '(', ')', '$', '`', '"', '\'', '\\', '\n', '\r':
			return false
		}
	}
	return true
}

func writeAtomic(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".package.json.*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
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
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return nil
}

func (c Change) String() string {
	if !c.Changed {
		return fmt.Sprintf("%s: dev script already uses portucca", c.Path)
	}
	return fmt.Sprintf("%s: %s -> %s", c.Path, c.Before, c.After)
}
