package scan

import (
	"context"
	"strings"
	"testing"
)

func TestScanComposeDiscoveryCreatesServices(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "compose.yaml", "services:\n  web:\n    ports:\n      - \"4000:4000\"\n  db:\n    ports:\n      - \"5432:5432\"\n")

	got, err := Scan(context.Background(), Options{
		Root: root, MainRoot: root, DiscoverCompose: true,
		ExcludeServices: []string{"db"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "web" {
		t.Fatalf("services = %+v, want only web", got.Services)
	}
	if got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != "4000" {
		t.Fatalf("web = %+v, want compose port 4000", got.Services[0])
	}
	if got.Services[0].NameSource != "compose.yaml:services.web.name" {
		t.Fatalf("nameSource = %q, want compose name source", got.Services[0].NameSource)
	}
	if got.Services[0].MainPort.Source != "compose.yaml:services.web.ports" {
		t.Fatalf("mainPort source = %q, want compose ports source", got.Services[0].MainPort.Source)
	}
}

func TestScanComposeDiscoveryDefaultDoesNotCreate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "compose.yaml", "services:\n  web:\n    ports:\n      - \"4000:4000\"\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 0 {
		t.Fatalf("services = %+v, want none without DiscoverCompose", got.Services)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings = %+v, want none", got.Warnings)
	}
}

func TestScanComposeDiscoveryEnrichesCandidate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/web/package.json", `{"name":"web","scripts":{"dev":"vite"}}`)
	writeFile(t, root, "compose.yaml", "services:\n  web:\n    ports:\n      - \"4000:4000\"\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root, DiscoverCompose: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "web" {
		t.Fatalf("services = %+v, want one web", got.Services)
	}
	if got.Services[0].NameSource != "apps/web/package.json:name" {
		t.Fatalf("nameSource = %q, want package name", got.Services[0].NameSource)
	}
	if got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != "4000" {
		t.Fatalf("mainPort = %+v, want 4000", got.Services[0].MainPort)
	}
}

func TestScanComposeDiscoveryIgnoresServiceWithoutUniquePort(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "compose.yaml", "services:\n  web:\n    ports:\n      - \"4000:4000\"\n      - \"4001:4001\"\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root, DiscoverCompose: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 0 {
		t.Fatalf("services = %+v, want none", got.Services)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "multiple or unsupported") {
		t.Fatalf("warnings = %+v, want unsupported warning", got.Warnings)
	}
}

func TestScanComposeDiscoveryWarnsServiceWithoutPorts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "compose.yaml", "services:\n  worker:\n    image: busybox\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root, DiscoverCompose: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 0 {
		t.Fatalf("services = %+v, want none", got.Services)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "has no published ports") {
		t.Fatalf("warnings = %+v, want no-ports warning", got.Warnings)
	}
}

func TestScanComposeDiscoveryExcludeCandidate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/db/package.json", `{"name":"db","scripts":{"dev":"node index.js"}}`)
	writeFile(t, root, "apps/db/.env", "PORT=5432\n")
	writeFile(t, root, "compose.yaml", "services:\n  db:\n    ports:\n      - \"5432:5432\"\n")

	got, err := Scan(context.Background(), Options{
		Root: root, MainRoot: root, DiscoverCompose: true,
		ExcludeServices: []string{"db"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 0 {
		t.Fatalf("services = %+v, want db excluded", got.Services)
	}
	if len(got.Env) != 0 || len(got.CopyFromMain) != 0 {
		t.Fatalf("env = %+v copy = %+v, want excluded service removed from both", got.Env, got.CopyFromMain)
	}
}

func TestScanComposeDiscoveryExcludeUnknownWarns(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "compose.yaml", "services:\n  web:\n    ports:\n      - \"4000:4000\"\n")

	got, err := Scan(context.Background(), Options{
		Root: root, MainRoot: root, DiscoverCompose: true,
		ExcludeServices: []string{"db"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "web" {
		t.Fatalf("services = %+v, want web", got.Services)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "ignoring --exclude-service db") {
		t.Fatalf("warnings = %+v, want unknown exclusion warning", got.Warnings)
	}
}

func TestScanComposeDiscoveryExcludedServiceSkipsWarning(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "compose.yaml", "services:\n  db:\n    ports:\n      - \"5432:5432\"\n      - \"5433:5433\"\n")

	got, err := Scan(context.Background(), Options{
		Root: root, MainRoot: root, DiscoverCompose: true,
		ExcludeServices: []string{"db"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 0 || len(got.Warnings) != 0 {
		t.Fatalf("services = %+v warnings = %+v, want clean skip", got.Services, got.Warnings)
	}
}

func TestScanComposeDiscoveryInvalidNameGap(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "compose.yaml", "services:\n  \"123\":\n    ports:\n      - \"4000:4000\"\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root, DiscoverCompose: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 0 || len(got.Gaps) != 1 {
		t.Fatalf("services = %+v gaps = %+v, want one invalid gap", got.Services, got.Gaps)
	}
	if got.Gaps[0].Reason != "invalid" {
		t.Fatalf("gap = %+v, want invalid", got.Gaps[0])
	}
}

func TestScanComposeDiscoveryConflictingPortsGap(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "compose.yaml", "services:\n  web:\n    ports:\n      - \"4000:4000\"\n")
	writeFile(t, root, "docker-compose.yaml", "services:\n  web:\n    ports:\n      - \"4001:4001\"\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root, DiscoverCompose: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 0 || len(got.Gaps) != 1 {
		t.Fatalf("services = %+v gaps = %+v, want one ambiguous gap", got.Services, got.Gaps)
	}
	if got.Gaps[0].Reason != "ambiguous" {
		t.Fatalf("gap = %+v, want ambiguous", got.Gaps[0])
	}
}

func TestScanComposeDiscoveryCollidingNamesGap(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "compose.yaml", "services:\n  web_app:\n    ports:\n      - \"4000:4000\"\n  web-app:\n    ports:\n      - \"4000:4000\"\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root, DiscoverCompose: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 0 || len(got.Gaps) != 1 {
		t.Fatalf("services = %+v gaps = %+v, want one ambiguous gap", got.Services, got.Gaps)
	}
	if got.Gaps[0].Reason != "ambiguous" {
		t.Fatalf("gap = %+v, want ambiguous", got.Gaps[0])
	}
}

func TestScanComposeDiscoveryCollidingNamesWithCandidateGap(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/web/package.json", `{"name":"web-app","scripts":{"dev":"vite"}}`)
	writeFile(t, root, "compose.yaml", "services:\n  web_app:\n    ports:\n      - \"4000:4000\"\n  web-app:\n    ports:\n      - \"4000:4000\"\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root, DiscoverCompose: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 1 || got.Gaps[0].Reason != "ambiguous" {
		t.Fatalf("gaps = %+v, want one ambiguous gap for colliding compose names", got.Gaps)
	}
}

func TestScanComposeDiscoveryWithoutComposeFileWarns(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/web/package.json", `{"name":"web","scripts":{"dev":"vite --port 4000"}}`)

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root, DiscoverCompose: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services[0].Name != "web" {
		t.Fatalf("services = %+v, want web", got.Services)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "no compose file found") {
		t.Fatalf("warnings = %+v, want no-compose warning", got.Warnings)
	}
}
