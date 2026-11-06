package scan

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, rel, data string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyPaths(files []CopyFile) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, file.File)
	}
	return out
}

func TestScanDetectsMonorepo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/web/package.json", `{"name":"@acme/web","scripts":{"dev":"vite --port 5173"}}`)
	writeFile(t, root, "apps/web/.env.local", "API_URL=http://localhost:3004\n")
	writeFile(t, root, "apps/api/package.json", `{"name":"@acme/api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "apps/api/.env", "PORT=3004\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 0 {
		t.Fatalf("gaps = %+v, want none", got.Gaps)
	}
	if len(got.Services) != 2 {
		t.Fatalf("services = %+v, want web and api", got.Services)
	}
	byName := map[string]Service{}
	for _, svc := range got.Services {
		byName[string(svc.Name)] = svc
	}
	if byName["web"].MainPort == nil || byName["web"].MainPort.Value != "5173" {
		t.Fatalf("web = %+v, want mainPort 5173", byName["web"])
	}
	if byName["api"].MainPort == nil || byName["api"].MainPort.Value != "3004" {
		t.Fatalf("api = %+v, want mainPort 3004", byName["api"])
	}
	if len(got.Env) != 2 {
		t.Fatalf("env = %+v, want two targets", got.Env)
	}
	if got.Env[0].File == "" || got.Env[1].File == "" {
		t.Fatalf("env = %+v, want file paths", got.Env)
	}
	if len(got.CopyFromMain) != 2 {
		t.Fatalf("copyFromMain = %+v, want both env files", got.CopyFromMain)
	}
}

func TestScanPrecedenceWarnsButDoesNotFail(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "apps/api/.env", "PORT=3004\n")
	writeFile(t, root, "apps/api/.env.local", "PORT=3005\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 0 {
		t.Fatalf("gaps = %+v, want none", got.Gaps)
	}
	if got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != "3005" {
		t.Fatalf("service = %+v, want .env.local to win", got.Services[0])
	}
	if len(got.Warnings) == 0 || !strings.Contains(got.Warnings[0], "ignoring apps/api/.env") {
		t.Fatalf("warnings = %+v, want the lower-rank warning", got.Warnings)
	}
}

func TestScanIgnoresSymlinkEnv(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/real.env", "PORT=3004\n")
	if err := os.Symlink("real.env", filepath.Join(root, "apps/api/.env")); err != nil {
		t.Fatal(err)
	}
	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 0 {
		t.Fatalf("services = %+v, want no service from a symlink", got.Services)
	}
	if len(got.Warnings) == 0 || !strings.Contains(strings.Join(got.Warnings, " "), "ignoring non-regular env file") {
		t.Fatalf("warnings = %+v, want the symlink warning", got.Warnings)
	}
}

func TestScanComposeJSONEnrichesOnlyExistingServices(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "docker-compose.json", `{"services":{"api":{"ports":["4000:4000"]},"db":{"ports":["5432:5432"]}}}`)

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "api" {
		t.Fatalf("services = %+v, want only api", got.Services)
	}
	if got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != "4000" {
		t.Fatalf("api = %+v, want compose port 4000", got.Services[0])
	}
}

func TestScanIgnoresRootEnvInMonorepo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env", "PORT=9999\n")
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "apps/api/.env", "PORT=3004\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "api" {
		t.Fatalf("services = %+v, want only api", got.Services)
	}
	if len(got.Warnings) == 0 || !strings.Contains(strings.Join(got.Warnings, " "), "ignoring root") {
		t.Fatalf("warnings = %+v, want the root warning", got.Warnings)
	}
}

func TestScanIgnoresOutOfRangeComposePort(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "docker-compose.json", `{"services":{"api":{"ports":["80:80"]}}}`)

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "api" {
		t.Fatalf("services = %+v, want api", got.Services)
	}
	if got.Services[0].MainPort != nil {
		t.Fatalf("api = %+v, want nil mainPort", got.Services[0])
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "outside 1024..65535") {
		t.Fatalf("warnings = %+v, want outside range warning", got.Warnings)
	}
}

func TestScanTargetsAllPortKeys(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "apps/api/.env", "PORT=19004\nAPI_PORT=19004\n")
	writeFile(t, root, "apps/api/.env.local", "PORT=19004\nAPI_PORT=19004\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 0 {
		t.Fatalf("gaps = %+v, want none", got.Gaps)
	}
	if len(got.Services) != 1 || got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != "19004" {
		t.Fatalf("services = %+v, want api mainPort 19004", got.Services)
	}
	targets := map[string]string{}
	for _, target := range got.Env {
		targets[target.File+":"+target.Key] = target.SourceFile
	}
	for _, want := range []string{
		"apps/api/.env:PORT",
		"apps/api/.env:API_PORT",
		"apps/api/.env.local:PORT",
		"apps/api/.env.local:API_PORT",
	} {
		if _, ok := targets[want]; !ok {
			t.Fatalf("env = %+v, want target %s", got.Env, want)
		}
	}
	if len(targets) != 4 {
		t.Fatalf("env = %+v, want four targets", got.Env)
	}
	wantCopy := []string{"apps/api/.env", "apps/api/.env.local"}
	if strings.Join(copyPaths(got.CopyFromMain), ",") != strings.Join(wantCopy, ",") {
		t.Fatalf("copyFromMain = %+v, want %+v", got.CopyFromMain, wantCopy)
	}
}

func TestScanDoesNotRewriteDatabasePort(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "apps/api/.env.local", "PORT=3004\n")
	writeFile(t, root, "apps/api/.env", "DB_PORT=5432\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 0 {
		t.Fatalf("gaps = %+v, want none", got.Gaps)
	}
	if len(got.Services) != 1 || got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != "3004" {
		t.Fatalf("services = %+v, want api mainPort 3004", got.Services)
	}
	if len(got.Env) != 1 {
		t.Fatalf("env = %+v, want only the PORT target", got.Env)
	}
	target := got.Env[0]
	if target.File != "apps/api/.env.local" || target.Key != "PORT" {
		t.Fatalf("target = %+v, want apps/api/.env.local:PORT", target)
	}
	for _, target := range got.Env {
		if target.Key == "DB_PORT" {
			t.Fatalf("env = %+v, want no DB_PORT target", got.Env)
		}
	}
	wantCopy := []string{"apps/api/.env", "apps/api/.env.local"}
	if strings.Join(copyPaths(got.CopyFromMain), ",") != strings.Join(wantCopy, ",") {
		t.Fatalf("copyFromMain = %+v, want %+v", got.CopyFromMain, wantCopy)
	}
}

func TestScanUsesEnvExampleFallback(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "apps/api/.env.example", "PORT=3004\nSECRET=value\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 0 {
		t.Fatalf("gaps = %+v, want none", got.Gaps)
	}
	if len(got.Services) != 1 || got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != "3004" {
		t.Fatalf("services = %+v, want api mainPort 3004", got.Services)
	}
	if len(got.Env) != 1 {
		t.Fatalf("env = %+v, want one fallback target", got.Env)
	}
	target := got.Env[0]
	if target.File != "apps/api/.env" || target.Key != "PORT" || target.SourceFile != "apps/api/.env.example" {
		t.Fatalf("target = %+v, want apps/api/.env:PORT from the example", target)
	}
	if len(got.CopyFromMain) != 0 {
		t.Fatalf("copyFromMain = %+v, want no example files", got.CopyFromMain)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "using example file") {
		t.Fatalf("warnings = %+v, want the example warning", got.Warnings)
	}
}

func TestScanMaxFileBytes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/.env", strings.Repeat("X", int(DefaultMaxFileBytes)+1))

	got, err := Scan(context.Background(), Options{
		Root: root, MainRoot: root, MaxFileBytes: DefaultMaxFileBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 0 {
		t.Fatalf("services = %+v, want none from an oversized file", got.Services)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "larger than") {
		t.Fatalf("warnings = %+v, want the size warning", got.Warnings)
	}
}

func TestScanDuplicatePortKeys(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "apps/api/.env", "PORT=3004\nPORT=3005\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 1 || got.Gaps[0].Reason != "ambiguous" || got.Gaps[0].Field != "services.api.mainPort" {
		t.Fatalf("gaps = %+v, want one ambiguous services.api.mainPort gap", got.Gaps)
	}
}

func TestScanSingleRootService(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{"name":"app","scripts":{"dev":"vite --port 5173"}}`)

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 0 {
		t.Fatalf("gaps = %+v, want none", got.Gaps)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "app" {
		t.Fatalf("services = %+v, want app", got.Services)
	}
	if got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != "5173" {
		t.Fatalf("service = %+v, want mainPort 5173", got.Services[0])
	}
}

func TestScanKeepsRootServiceInMonorepo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{"name":"root","scripts":{"dev":"vite --port 4000"}}`)
	writeFile(t, root, ".env", "PORT=9999\n")
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "apps/api/.env", "PORT=3004\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 0 {
		t.Fatalf("gaps = %+v, want none", got.Gaps)
	}
	if len(got.Services) != 2 {
		t.Fatalf("services = %+v, want api and root", got.Services)
	}
	byName := map[string]Service{}
	for _, svc := range got.Services {
		byName[string(svc.Name)] = svc
	}
	if byName["root"].MainPort == nil || byName["root"].MainPort.Value != "4000" {
		t.Fatalf("root = %+v, want script port 4000", byName["root"])
	}
	if byName["api"].MainPort == nil || byName["api"].MainPort.Value != "3004" {
		t.Fatalf("api = %+v, want port 3004", byName["api"])
	}
	for _, target := range got.Env {
		if target.File == ".env" {
			t.Fatalf("env = %+v, want no root .env target", got.Env)
		}
	}
	if len(got.CopyFromMain) != 1 || copyPaths(got.CopyFromMain)[0] != "apps/api/.env" {
		t.Fatalf("copyFromMain = %+v, want only apps/api/.env", got.CopyFromMain)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "ignoring root .env") {
		t.Fatalf("warnings = %+v, want the root .env warning", got.Warnings)
	}
}

func TestScanDropsRootOrchestrator(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{"name":"root","scripts":{"dev":"turbo run dev"}}`)
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "apps/api/.env", "PORT=3004\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 0 {
		t.Fatalf("gaps = %+v, want none", got.Gaps)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "api" {
		t.Fatalf("services = %+v, want only api", got.Services)
	}
	if got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != "3004" {
		t.Fatalf("api = %+v, want mainPort 3004", got.Services[0])
	}
	warnings := strings.Join(got.Warnings, " ")
	if !strings.Contains(warnings, "orchestrator") && !strings.Contains(warnings, "root") {
		t.Fatalf("warnings = %+v, want an orchestrator or root warning", got.Warnings)
	}
	for _, target := range got.Env {
		if target.File == ".env" {
			t.Fatalf("env = %+v, want no root .env target", got.Env)
		}
	}
	if len(got.CopyFromMain) != 1 || copyPaths(got.CopyFromMain)[0] != "apps/api/.env" {
		t.Fatalf("copyFromMain = %+v, want only apps/api/.env", got.CopyFromMain)
	}
}
