package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func walkOnly(root string, max int) error {
	s := &scanner{opts: Options{Root: root, MainRoot: root, MaxEntries: max}}
	return s.walk(context.Background(), func(string, string) {})
}

func TestScanTooLarge(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := walkOnly(root, 1)
	var tooLarge *TooLargeError
	if !errors.As(err, &tooLarge) || tooLarge.Limit != 1 || tooLarge.Visited != 2 {
		t.Fatalf("err = %v, want TooLargeError with limit 1 and visited 2", err)
	}
}

func TestScanSkipsIgnoredDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "a.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := walkOnly(root, 1); err != nil {
		t.Fatalf("walk = %v, want node_modules skipped", err)
	}
}
