package project

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestDetectMainAndWorktree(t *testing.T) {
	main := t.TempDir()
	run(t, main, "init", "-q")
	if err := os.WriteFile(filepath.Join(main, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, main, "add", "README.md")
	run(t, main, "commit", "-qm", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	run(t, main, "worktree", "add", "-q", wt, "-b", "feature")

	gotMain, err := Detect(main)
	if err != nil {
		t.Fatal(err)
	}
	if !gotMain.IsMain {
		t.Fatalf("main.IsMain = false")
	}
	gotWT, err := Detect(wt)
	if err != nil {
		t.Fatal(err)
	}
	if gotWT.IsMain {
		t.Fatalf("worktree.IsMain = true")
	}
	if gotWT.MainRoot != gotMain.Root {
		t.Fatalf("MainRoot = %q, want %q", gotWT.MainRoot, gotMain.Root)
	}
}

func TestDetectNotARepo(t *testing.T) {
	_, err := Detect(t.TempDir())
	if !errors.Is(err, ErrNotRepo) {
		t.Fatalf("err = %v, want ErrNotRepo", err)
	}
}
