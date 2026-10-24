package envfile

import (
	"errors"
	"os"
	"path/filepath"
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
