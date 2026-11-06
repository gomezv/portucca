package scan

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
)

func detectProject(ctx context.Context, root string) *Value {
	if out, err := exec.CommandContext(ctx, "git", "-C", root, "remote", "get-url", "origin").Output(); err == nil {
		if name := repoName(strings.TrimSpace(string(out))); name != "" {
			return &Value{Value: name, Source: "remote origin"}
		}
	}
	base := filepath.Base(filepath.Clean(root))
	if name := slugify(base); name != "" {
		return &Value{Value: name, Source: "directory"}
	}
	return nil
}

func repoName(remote string) string {
	remote = strings.TrimSuffix(strings.TrimSpace(remote), "/")
	remote = strings.TrimSuffix(remote, ".git")
	if i := strings.LastIndexAny(remote, "/:"); i >= 0 {
		remote = remote[i+1:]
	}
	return slugify(remote)
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 63 {
		out = strings.TrimRight(out[:63], "-")
	}
	return out
}

func serviceNameFromPackage(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return slugify(name)
}
