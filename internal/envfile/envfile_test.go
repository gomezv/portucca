package envfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomezv/portucca/internal/domain"
)

func TestDocumentMergePreservesComments(t *testing.T) {
	doc, err := Parse([]byte("# comment\nPORT=3000\nOTHER=keep\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Set("PORT", "3005") {
		t.Fatal("Set() = false, want true")
	}
	if !doc.Set("NEW", "x") {
		t.Fatal("Set(NEW) = false, want true")
	}
	got := string(doc.Bytes())
	want := "# comment\nPORT=3005\nOTHER=keep\nNEW=x\n"
	if got != want {
		t.Fatalf("Bytes() = %q, want %q", got, want)
	}
}

func TestReadPort(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte("WEB_PORT=\"5174\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, ok, err := ReadPort(dir, domain.EnvRef{File: ".env.local", Key: "WEB_PORT"})
	if err != nil || !ok || p != 5174 {
		t.Fatalf("p=%d ok=%v err=%v", p, ok, err)
	}
}

func TestWriteValuesIsAtomicAndPreservesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.local")
	if err := os.WriteFile(path, []byte("PORT=3000\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	changed, err := WriteValues(dir, map[string]map[string]string{".env.local": {"PORT": "3005"}})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", info.Mode().Perm())
	}
}

func TestCopyFromMainDoesNotOverwrite(t *testing.T) {
	mainRoot, wtRoot := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(mainRoot, ".env"), []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtRoot, ".env"), []byte("A=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	copied, err := CopyFromMain(mainRoot, wtRoot, ".env")
	if err != nil || copied {
		t.Fatalf("copied=%v err=%v, want false nil", copied, err)
	}
	data, _ := os.ReadFile(filepath.Join(wtRoot, ".env"))
	if string(data) != "A=2\n" {
		t.Fatalf("destination was overwritten: %q", data)
	}
}

func TestEnsureSymlinkConflict(t *testing.T) {
	wtRoot := t.TempDir()
	path := filepath.Join(wtRoot, "apps", "api")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, ".env"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := EnsureSymlink(wtRoot, domain.Symlink{Path: "apps/api/.env", Target: "../../.env"})
	if !errors.Is(err, ErrSymlinkConflict) {
		t.Fatalf("err = %v, want ErrSymlinkConflict", err)
	}
}

func TestGetInlineComments(t *testing.T) {
	doc, err := Parse([]byte("UNQUOTED=5174 # dev\nQUOTED=\"5175\" # qa\nHASH=\"a # b\"\nSINGLE='x # y'\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"UNQUOTED": "5174",
		"QUOTED":   "5175",
		"HASH":     "a # b",
		"SINGLE":   "x # y",
	}
	for key, expected := range want {
		got, ok := doc.Get(key)
		if !ok || got != expected {
			t.Fatalf("Get(%s) = %q, %v; want %q, true", key, got, ok, expected)
		}
	}
}

func TestReadPortInlineComment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte("WEB_PORT=5174 # dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, ok, err := ReadPort(dir, domain.EnvRef{File: ".env.local", Key: "WEB_PORT"})
	if err != nil || !ok || p != 5174 {
		t.Fatalf("p=%d ok=%v err=%v", p, ok, err)
	}
}

func TestSetPreservesInlineComments(t *testing.T) {
	doc, err := Parse([]byte("PORT=3000 # dev\nexport EXPORTED=3001 # keep\nQUOTED=\"3002\" # qa\nHASH=\"a # b\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"PORT", "EXPORTED", "QUOTED", "HASH"} {
		if !doc.Set(key, "4000") {
			t.Fatalf("Set(%s) = false, want true", key)
		}
	}
	got := string(doc.Bytes())
	want := "PORT=4000 # dev\nexport EXPORTED=4000 # keep\nQUOTED=4000 # qa\nHASH=4000\n"
	if got != want {
		t.Fatalf("Bytes() = %q, want %q", got, want)
	}
}

func TestWriteValuesRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.env")
	if err := os.WriteFile(target, []byte("PORT=3000\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".env.local")
	if err := os.Symlink("target.env", link); err != nil {
		t.Fatal(err)
	}
	changed, err := WriteValues(dir, map[string]map[string]string{".env.local": {"PORT": "3005"}})
	if err == nil || changed {
		t.Fatalf("changed=%v err=%v, want false and an error", changed, err)
	}
	if !strings.Contains(err.Error(), "symlink") || !strings.Contains(err.Error(), "will not replace") {
		t.Fatalf("err = %q, want symlink and will not replace", err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("link was replaced: info=%v err=%v", info, err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "PORT=3000\n" {
		t.Fatalf("target changed: data=%q err=%v", data, err)
	}
}

func TestWriteValuesNewFileMode(t *testing.T) {
	dir := t.TempDir()
	changed, err := WriteValues(dir, map[string]map[string]string{".env.local": {"PORT": "3005"}})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	path := filepath.Join(dir, ".env.local")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := doc.Get("PORT"); !ok || got != "3005" {
		t.Fatalf("Get(PORT) = %q, %v; want 3005, true", got, ok)
	}
}

func TestCopyFromMainMissingSource(t *testing.T) {
	mainRoot, wtRoot := t.TempDir(), t.TempDir()
	copied, err := CopyFromMain(mainRoot, wtRoot, ".env")
	if copied || !errors.Is(err, ErrSourceMissing) {
		t.Fatalf("copied=%v err=%v, want false and ErrSourceMissing", copied, err)
	}
}

func TestEnsureSymlinkCreatesRelativeTarget(t *testing.T) {
	wtRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wtRoot, "apps", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	created, err := EnsureSymlink(wtRoot, domain.Symlink{Path: "apps/api/.env", Target: "../../.env"})
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	target, err := os.Readlink(filepath.Join(wtRoot, "apps", "api", ".env"))
	if err != nil || target != "../../.env" {
		t.Fatalf("target=%q err=%v", target, err)
	}
	created, err = EnsureSymlink(wtRoot, domain.Symlink{Path: "apps/api/.env", Target: "../../.env"})
	if err != nil || created {
		t.Fatalf("second call: created=%v err=%v, want false nil", created, err)
	}
}

func TestEnsureSymlinkMissingParent(t *testing.T) {
	wtRoot := t.TempDir()
	_, err := EnsureSymlink(wtRoot, domain.Symlink{Path: "missing/.env", Target: ".env"})
	if err == nil || !strings.Contains(err.Error(), "bootstrap_error") {
		t.Fatalf("err = %v, want bootstrap_error", err)
	}
}

func TestRenderValue(t *testing.T) {
	if got := RenderValue(3005, ""); got != "3005" {
		t.Fatalf("RenderValue plain = %q, want 3005", got)
	}
	if got := RenderValue(3005, "http://localhost:{{port}}"); got != "http://localhost:3005" {
		t.Fatalf("RenderValue template = %q", got)
	}
}

func TestParseExportAndCRLF(t *testing.T) {
	doc, err := Parse([]byte("export PORT=3000\r\nOTHER=1\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := doc.Get("PORT"); !ok || got != "3000" {
		t.Fatalf("Get(PORT) = %q, %v", got, ok)
	}
	if got, ok := doc.Get("OTHER"); !ok || got != "1" {
		t.Fatalf("Get(OTHER) = %q, %v", got, ok)
	}
	if !doc.Set("PORT", "3005") {
		t.Fatal("Set(PORT) = false, want true")
	}
	if got := string(doc.Bytes()); got != "export PORT=3005\nOTHER=1\n" {
		t.Fatalf("Bytes() = %q", got)
	}
}
