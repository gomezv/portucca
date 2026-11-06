package envfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gomezv/portucca/internal/domain"
)

var (
	ErrSourceMissing   = errors.New("bootstrap_source_missing")
	ErrSymlinkConflict = errors.New("symlink_conflict")
	keyPattern         = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type line struct {
	raw string
	key string
}

type Document struct {
	lines []line
}

func Parse(data []byte) (*Document, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	rawLines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	d := &Document{lines: make([]line, 0, len(rawLines))}
	for _, raw := range rawLines {
		d.lines = append(d.lines, line{raw: raw, key: parseKey(raw)})
	}
	return d, nil
}

func parseKey(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasPrefix(s, "#") {
		return ""
	}
	s = strings.TrimPrefix(s, "export ")
	eq := strings.IndexByte(s, '=')
	if eq < 0 {
		return ""
	}
	key := strings.TrimSpace(s[:eq])
	if !keyPattern.MatchString(key) {
		return ""
	}
	return key
}

type Entry struct {
	Key   string
	Value string
}

func (d *Document) Entries() []Entry {
	out := make([]Entry, 0, len(d.lines))
	for _, l := range d.lines {
		if l.key == "" {
			continue
		}
		s := strings.TrimSpace(l.raw)
		s = strings.TrimPrefix(s, "export ")
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			continue
		}
		value, _ := splitValue(s[eq+1:])
		out = append(out, Entry{Key: l.key, Value: value})
	}
	return out
}

func commentIndex(s string) int {
	for i := 1; i < len(s); i++ {
		if s[i] == '#' && (s[i-1] == ' ' || s[i-1] == '\t') {
			return i
		}
	}
	return -1
}

func splitValue(raw string) (value, suffix string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ""
	}
	if s[0] == '"' || s[0] == '\'' {
		quote := s[0]
		if end := strings.IndexByte(s[1:], quote); end >= 0 {
			end++
			value = s[1:end]
			rest := s[end+1:]
			if idx := commentIndex(rest); idx >= 0 {
				start := idx
				for start > 0 && (rest[start-1] == ' ' || rest[start-1] == '\t') {
					start--
				}
				return value, rest[start:]
			}
			return value, ""
		}
	}
	if idx := commentIndex(s); idx >= 0 {
		start := idx
		for start > 0 && (s[start-1] == ' ' || s[start-1] == '\t') {
			start--
		}
		return strings.TrimRight(s[:start], " \t"), s[start:]
	}
	return s, ""
}

func (d *Document) Get(key string) (string, bool) {
	for _, l := range d.lines {
		if l.key != key {
			continue
		}
		s := strings.TrimSpace(l.raw)
		s = strings.TrimPrefix(s, "export ")
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			continue
		}
		value, _ := splitValue(s[eq+1:])
		return value, true
	}
	return "", false
}

func (d *Document) Set(key, value string) bool {
	changed := false
	for i, l := range d.lines {
		if l.key != key {
			continue
		}
		prefix := ""
		trimmed := strings.TrimSpace(l.raw)
		if strings.HasPrefix(trimmed, "export ") {
			prefix = "export "
			trimmed = strings.TrimPrefix(trimmed, "export ")
		}
		suffix := ""
		if eq := strings.IndexByte(trimmed, '='); eq >= 0 {
			_, suffix = splitValue(trimmed[eq+1:])
		}
		next := prefix + key + "=" + value + suffix
		if d.lines[i].raw != next {
			d.lines[i].raw = next
			changed = true
		}
	}
	if !changed {
		for _, l := range d.lines {
			if l.key == key {
				return false
			}
		}
		d.lines = append(d.lines, line{raw: key + "=" + value, key: key})
		return true
	}
	return true
}

func (d *Document) Bytes() []byte {
	raw := make([]string, 0, len(d.lines))
	for _, l := range d.lines {
		raw = append(raw, l.raw)
	}
	return []byte(strings.Join(raw, "\n") + "\n")
}

func RenderValue(port domain.Port, template string) string {
	if template == "" {
		return strconv.Itoa(int(port))
	}
	return strings.ReplaceAll(template, "{{port}}", strconv.Itoa(int(port)))
}

func ReadValue(root string, ref domain.EnvRef) (string, bool, error) {
	data, err := os.ReadFile(filepath.Join(root, ref.File))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	doc, err := Parse(data)
	if err != nil {
		return "", false, err
	}
	value, ok := doc.Get(ref.Key)
	return value, ok, nil
}

func ReadPort(root string, ref domain.EnvRef) (domain.Port, bool, error) {
	value, ok, err := ReadValue(root, ref)
	if err != nil || !ok {
		return 0, false, err
	}
	n, err := strconv.Atoi(value)
	if err != nil || !domain.Port(n).Valid() {
		return 0, false, nil
	}
	return domain.Port(n), true, nil
}

func WriteValues(root string, values map[string]map[string]string) (bool, error) {
	changed := false
	files := make([]string, 0, len(values))
	for file := range values {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		path := filepath.Join(root, file)
		info, err := os.Lstat(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("env target %s is a symlink; portucca will not replace it", path)
		}
		mode := os.FileMode(0o600)
		data, err := os.ReadFile(path)
		if err == nil {
			if info != nil {
				mode = info.Mode().Perm()
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		doc, err := Parse(data)
		if err != nil {
			return false, err
		}
		fileChanged := false
		keys := make([]string, 0, len(values[file]))
		for key := range values[file] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if doc.Set(key, values[file][key]) {
				fileChanged = true
			}
		}
		if !fileChanged {
			continue
		}
		if err := writeAtomic(path, doc.Bytes(), mode); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".portucca-env-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
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

func CopyFromMain(mainRoot, wtRoot, rel string) (bool, error) {
	dest := filepath.Join(wtRoot, rel)
	if _, err := os.Lstat(dest); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	src := filepath.Join(mainRoot, rel)
	info, err := os.Stat(src)
	if errors.Is(err, os.ErrNotExist) {
		return false, ErrSourceMissing
	}
	if err != nil {
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("copyFromMain: %s is a directory", rel)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return false, err
	}
	if err := writeAtomic(dest, data, info.Mode().Perm()); err != nil {
		return false, err
	}
	return true, nil
}

func EnsureSymlink(wtRoot string, s domain.Symlink) (bool, error) {
	path := filepath.Join(wtRoot, s.Path)
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return false, ErrSymlinkConflict
		}
		target, err := os.Readlink(path)
		if err != nil {
			return false, err
		}
		if target != s.Target {
			return false, ErrSymlinkConflict
		}
		return false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return false, fmt.Errorf("bootstrap_error: %w", err)
	}
	if err := os.Symlink(s.Target, path); err != nil {
		return false, err
	}
	return true, nil
}

type Store struct{}

func (Store) ReadPort(root string, ref domain.EnvRef) (domain.Port, bool, error) {
	return ReadPort(root, ref)
}
func (Store) ReadValue(root string, ref domain.EnvRef) (string, bool, error) {
	return ReadValue(root, ref)
}
func (Store) WriteValues(root string, values map[string]map[string]string) (bool, error) {
	return WriteValues(root, values)
}
func (Store) CopyFromMain(mainRoot, wtRoot, rel string) (bool, error) {
	return CopyFromMain(mainRoot, wtRoot, rel)
}
func (Store) EnsureSymlink(wtRoot string, s domain.Symlink) (bool, error) {
	return EnsureSymlink(wtRoot, s)
}
