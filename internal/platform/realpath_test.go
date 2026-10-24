package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRealPathAbsoluteInput(t *testing.T) {
	dir := t.TempDir()
	got, err := RealPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("RealPath(%q) = %q, want an absolute path", dir, got)
	}
	if got != want {
		t.Fatalf("RealPath(%q) = %q, want %q", dir, got, want)
	}
}

func TestRealPathResolvesSymlinkedDirectory(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	got, err := RealPath(link)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("RealPath(%q) = %q, want %q", link, got, want)
	}
	if got == link {
		t.Fatalf("RealPath(%q) did not resolve the symlink", link)
	}
}
