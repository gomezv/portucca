package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListWorktrees(t *testing.T) {
	main := t.TempDir()
	run(t, main, "init", "-q")
	if err := os.WriteFile(filepath.Join(main, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, main, "add", "README.md")
	run(t, main, "commit", "-qm", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	run(t, main, "worktree", "add", "-q", wt, "-b", "feature")

	got, err := ListWorktrees(main)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("worktrees = %+v, want main and feature", got)
	}
	if got[0].Path != canonicalPath(main) {
		t.Fatalf("main path = %q, want %q", got[0].Path, canonicalPath(main))
	}
	if got[1].Path != canonicalPath(wt) || got[1].Branch != "feature" || got[1].Detached {
		t.Fatalf("feature = %+v", got[1])
	}
}

func canonicalPath(path string) string {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return real
}

func TestListWorktreesFlags(t *testing.T) {
	main := t.TempDir()
	run(t, main, "init", "-q")
	if err := os.WriteFile(filepath.Join(main, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, main, "add", "README.md")
	run(t, main, "commit", "-qm", "init")
	locked := filepath.Join(t.TempDir(), "locked")
	detached := filepath.Join(t.TempDir(), "detached")
	prunable := filepath.Join(t.TempDir(), "prunable")
	run(t, main, "worktree", "add", "-q", locked, "-b", "locked")
	run(t, main, "worktree", "add", "-q", "--detach", detached)
	run(t, main, "worktree", "add", "-q", prunable, "-b", "prunable")
	run(t, main, "worktree", "lock", locked)
	if err := os.RemoveAll(prunable); err != nil {
		t.Fatal(err)
	}

	got, err := ListWorktrees(main)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]Worktree{}
	for _, wt := range got {
		found[wt.Path] = wt
	}
	if !found[canonicalPath(locked)].Locked {
		t.Fatalf("locked worktree = %+v", found[canonicalPath(locked)])
	}
	if !found[canonicalPath(detached)].Detached {
		t.Fatalf("detached worktree = %+v", found[canonicalPath(detached)])
	}
	if !found[canonicalPath(prunable)].Prunable {
		t.Fatalf("prunable worktree = %+v", found[canonicalPath(prunable)])
	}
}
