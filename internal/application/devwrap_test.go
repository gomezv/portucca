package application

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/scan"
)

func scanDetected() scan.Detected {
	return scan.Detected{
		Project: &scan.Value{Value: "app", Source: "directory"},
		Services: []scan.Service{{
			Name: "web", Dir: ".", NameSource: "directory",
			MainPort: &scan.Value{Value: "8080", Source: ".env:PORT"},
		}},
		Env: []scan.EnvTarget{{Service: "web", File: ".env", Key: "PORT", SourceFile: ".env", Source: ".env:PORT"}},
	}
}

func writePackageJSON(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "package.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPreviewApplyDevWrap(t *testing.T) {
	app, dir := scanApp(t, scanDetected())
	path := writePackageJSON(t, dir, `{"scripts":{"dev":"next dev --turbopack"}}`)
	preview, err := app.PreviewDevWrap(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Changed || preview.Before != "next dev --turbopack" {
		t.Fatalf("preview = %+v", preview)
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{"scripts":{"dev":"next dev --turbopack"}}` {
		t.Fatal("preview wrote package.json")
	}
	applied, err := app.ApplyDevWrap(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Changed || !strings.Contains(applied.After, "portucca run --") {
		t.Fatalf("applied = %+v", applied)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), `"dev":"portucca run -- next dev --turbopack"`) {
		t.Fatalf("package = %s", data)
	}
}

func TestInitWrapScript(t *testing.T) {
	app, dir := scanApp(t, scanDetected())
	path := writePackageJSON(t, dir, `{"scripts":{"dev":"next dev --turbopack"}}`)
	got, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir, WrapScript: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.PackageScript == nil || !got.PackageScript.Changed {
		t.Fatalf("packageScript = %+v", got.PackageScript)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "portucca run -- next dev --turbopack") {
		t.Fatalf("package = %s", data)
	}
}

func TestInitNoWrap(t *testing.T) {
	app, dir := scanApp(t, scanDetected())
	path := writePackageJSON(t, dir, `{"scripts":{"dev":"next dev --turbopack"}}`)
	got, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got.PackageScript == nil || got.PackageScript.Changed {
		t.Fatalf("packageScript = %+v", got.PackageScript)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"dev":"next dev --turbopack"`) {
		t.Fatalf("package = %s", data)
	}
}

func TestInitWrapScriptMissing(t *testing.T) {
	app, dir := scanApp(t, scanDetected())
	_, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir, WrapScript: true})
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "package_script_missing" {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "portucca.json")); err == nil {
		t.Fatal("config was written after a wrap error")
	}
}

func TestInitWrapInvalidJSONLeavesNoConfig(t *testing.T) {
	app, dir := scanApp(t, scanDetected())
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"scripts":{"dev":"next dev"} garbage`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir, WrapScript: true})
	ae, ok := apperr.As(err)
	if !ok || ae.Code != "package_script_invalid" {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "portucca.json")); statErr == nil {
		t.Fatal("config was written after an invalid package.json")
	}
}

func TestInitPackageScriptNull(t *testing.T) {
	app, dir := scanApp(t, scanDetected())
	got, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got.PackageScript != nil {
		t.Fatalf("packageScript = %+v", got.PackageScript)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"packageScript":null`) {
		t.Fatalf("json = %s, want packageScript null", raw)
	}
}
