package project

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gomezv/portucca/internal/platform"
)

type Worktree struct {
	Path     string
	Head     string
	Branch   string
	Detached bool
	Locked   bool
	Prunable bool
}

type Lister struct{}

func (Lister) ListWorktrees(root string) ([]Worktree, error) {
	return ListWorktrees(root)
}

func ListWorktrees(root string) ([]Worktree, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, ErrGitMissing
	}
	out, err := git(root, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, ErrNotRepo
	}
	worktrees := parseWorktrees(out)
	for i := range worktrees {
		if real, err := platform.RealPath(worktrees[i].Path); err == nil {
			worktrees[i].Path = real
		} else if abs, err := filepath.Abs(worktrees[i].Path); err == nil {
			worktrees[i].Path = abs
		}
	}
	return worktrees, nil
}

func parseWorktrees(out string) []Worktree {
	var worktrees []Worktree
	for _, block := range strings.Split(out, "\n\n") {
		var wt Worktree
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				wt.Path = strings.TrimPrefix(line, "worktree ")
			case strings.HasPrefix(line, "HEAD "):
				wt.Head = strings.TrimPrefix(line, "HEAD ")
			case strings.HasPrefix(line, "branch refs/heads/"):
				wt.Branch = strings.TrimPrefix(line, "branch refs/heads/")
			case line == "detached":
				wt.Detached = true
			case strings.HasPrefix(line, "locked"):
				wt.Locked = true
			case strings.HasPrefix(line, "prunable"):
				wt.Prunable = true
			}
		}
		if wt.Path != "" {
			worktrees = append(worktrees, wt)
		}
	}
	return worktrees
}
