package scan

import (
	"context"
	"testing"
)

func TestResolveAmbiguousPort(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "apps/api/.env.development", "PORT=3004\n")
	writeFile(t, root, "apps/api/.env.production", "PORT=3005\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 1 {
		t.Fatalf("gaps = %+v, want one ambiguity", got.Gaps)
	}
	if got.Gaps[0].Reason != "ambiguous" || got.Gaps[0].Field != "services.api.mainPort" {
		t.Fatalf("gap = %+v, want services.api.mainPort ambiguous", got.Gaps[0])
	}
}

func TestScanReportsPersistentNameCollision(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a/b/package.json", `{"name":"web","scripts":{"dev":"vite --port 5173"}}`)
	writeFile(t, root, "a-b/package.json", `{"name":"web","scripts":{"dev":"vite --port 5174"}}`)

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 1 || got.Gaps[0].Reason != "ambiguous" {
		t.Fatalf("gaps = %+v, want one name-collision gap", got.Gaps)
	}
	if len(got.Services) != 2 || got.Services[0].Name == got.Services[1].Name {
		t.Fatalf("services = %+v, want unique names", got.Services)
	}
}

func TestScanRejectsOutOfRangeScriptPort(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"vite --port 80"}}`)

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 1 || got.Gaps[0].Reason != "invalid" || got.Gaps[0].Field != "services.api.mainPort" {
		t.Fatalf("gaps = %+v, want one invalid services.api.mainPort gap", got.Gaps)
	}
}
