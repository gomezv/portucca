package project

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gomezv/portucca/internal/platform"
)

var (
	ErrNotRepo    = errors.New("not_a_repo")
	ErrGitMissing = errors.New("git_missing")
)

type Context struct {
	Root     string
	MainRoot string
	IsMain   bool
}

func Detect(path string) (Context, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return Context{}, ErrGitMissing
	}
	root, err := git(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return Context{}, ErrNotRepo
	}
	root, err = platform.RealPath(root)
	if err != nil {
		return Context{}, err
	}
	gitDir, err := git(path, "rev-parse", "--git-dir")
	if err != nil {
		return Context{}, ErrNotRepo
	}
	commonDir, err := git(path, "rev-parse", "--git-common-dir")
	if err != nil {
		return Context{}, ErrNotRepo
	}
	gitDir = resolve(root, gitDir)
	commonDir = resolve(root, commonDir)
	gitDir, err = platform.RealPath(gitDir)
	if err != nil {
		return Context{}, err
	}
	commonDir, err = platform.RealPath(commonDir)
	if err != nil {
		return Context{}, err
	}
	return Context{
		Root:     root,
		MainRoot: filepath.Dir(commonDir),
		IsMain:   gitDir == commonDir,
	}, nil
}

func resolve(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v: %w: %s", args, err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
