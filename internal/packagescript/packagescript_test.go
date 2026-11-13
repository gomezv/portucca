package packagescript

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePackage(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "package.json")
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	return path
}

const formatted = `{
  "name": "app",
  "scripts": {
    "dev": "next dev --turbopack",
    "build": "next build"
  },
  "dependencies": {
    "next": "16.0.0"
  }
}
`

func TestPreviewAndApply(t *testing.T) {
	path := writePackage(t, formatted)
	change, err := Preview(path)
	if err != nil {
		t.Fatal(err)
	}
	if !change.Changed || change.Before != "next dev --turbopack" || change.After != "portucca run -- next dev --turbopack" {
		t.Fatalf("change = %+v", change)
	}
	data, _ := os.ReadFile(path)
	if string(data) != formatted {
		t.Fatal("Preview wrote the file")
	}
	applied, err := Apply(path)
	if err != nil {
		t.Fatal(err)
	}
	if applied != change {
		t.Fatalf("applied = %+v, want %+v", applied, change)
	}
	data, _ = os.ReadFile(path)
	if !json.Valid(data) {
		t.Fatal("result is not valid JSON")
	}
	want := strings.Replace(formatted, `"dev": "next dev --turbopack"`, `"dev": "portucca run -- next dev --turbopack"`, 1)
	if string(data) != want {
		t.Fatalf("result = %s, want %s", data, want)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestAlreadyWrapped(t *testing.T) {
	path := writePackage(t, `{"scripts":{"dev":"portucca run -- next dev"}}`)
	change, err := Apply(path)
	if err != nil {
		t.Fatal(err)
	}
	if change.Changed || change.Before != "portucca run -- next dev" {
		t.Fatalf("change = %+v", change)
	}
}

func TestErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "package.json")
	if _, err := Preview(missing); !errors.Is(err, ErrMissing) {
		t.Fatalf("missing = %v", err)
	}
	cases := []struct {
		name    string
		content string
		want    error
	}{
		{"invalid-json", `{`, ErrInvalid},
		{"no-scripts", `{"name":"app"}`, ErrMissing},
		{"no-dev", `{"scripts":{"build":"next build"}}`, ErrMissing},
		{"dev-not-string", `{"scripts":{"dev":42}}`, ErrInvalid},
		{"unsupported", `{"scripts":{"dev":"next dev && echo x"}}`, ErrUnsupported},
		{"env-assignment", `{"scripts":{"dev":"NODE_ENV=development next dev"}}`, ErrUnsupported},
		{"invalid-after-scripts", `{"scripts":{"dev":"next dev"} garbage`, ErrInvalid},
		{"missing-outer-brace", `{"scripts":{"dev":"next dev"}`, ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writePackage(t, tc.content)
			if _, err := Apply(path); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			data, _ := os.ReadFile(path)
			if string(data) != tc.content {
				t.Fatalf("file changed on error: %s", data)
			}
		})
	}
}
