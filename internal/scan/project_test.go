package scan

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"My App":       "my-app",
		"@acme/Web_UI": "acme-web-ui",
		"portucca-app": "portucca-app",
		"___":          "",
		"123app":       "123app",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Fatalf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
	if got := serviceNameFromPackage("@acme/Web_UI"); got != "web-ui" {
		t.Fatalf("serviceNameFromPackage = %q, want web-ui", got)
	}
}

func TestDetectProjectFromDirectory(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "my-app")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	got := detectProject(context.Background(), root)
	if got == nil || got.Value != "my-app" || got.Source != "directory" {
		t.Fatalf("detectProject = %+v, want my-app from directory", got)
	}
}

func TestDetectProjectFromRemote(t *testing.T) {
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("remote", "add", "origin", "git@github.com:acme/My-App.git")
	got := detectProject(context.Background(), root)
	if got == nil || got.Value != "my-app" || got.Source != "remote origin" {
		t.Fatalf("detectProject = %+v, want my-app from remote origin", got)
	}
}
