package scan

import (
	"context"
	"strings"
	"testing"
)

func TestScanComposeYAMLShortSyntax(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "compose.yaml", "services:\n  api:\n    ports:\n      - \"4000:4000\"\n  db:\n    ports:\n      - \"5432:5432\"\n")

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
	if got.Services[0].MainPort.Source != "compose.yaml:services.api.ports" {
		t.Fatalf("source = %q, want compose.yaml:services.api.ports", got.Services[0].MainPort.Source)
	}
}

func TestScanComposeYAMLFileNames(t *testing.T) {
	for _, name := range []string{
		"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
			writeFile(t, root, name, "services:\n  api:\n    ports:\n      - \"4000:4000\"\n")

			got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Services) != 1 || got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != "4000" {
				t.Fatalf("services = %+v, want api mainPort 4000", got.Services)
			}
		})
	}
}

func TestScanComposeYAMLLongSyntax(t *testing.T) {
	cases := []struct {
		name  string
		ports string
		want  string
	}{
		{"int", "- target: 4000\n        published: 4001\n", "4001"},
		{"string", "- target: 4000\n        published: \"4002\"\n", "4002"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
			writeFile(t, root, "compose.yaml", "services:\n  api:\n    ports:\n      "+tc.ports)

			got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Services) != 1 || got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != tc.want {
				t.Fatalf("services = %+v, want api mainPort %s", got.Services, tc.want)
			}
		})
	}
}

func TestScanComposeYAMLUnsupportedPortsWarn(t *testing.T) {
	cases := []struct {
		name  string
		ports string
	}{
		{"multiple", "- \"4000:4000\"\n      - \"4001:4001\"\n"},
		{"range", "- \"4000-4005:4000-4005\"\n"},
		{"interpolation", "- \"${PORT}:4000\"\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
			writeFile(t, root, "compose.yaml", "services:\n  api:\n    ports:\n      "+tc.ports)

			got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Services) != 1 || got.Services[0].MainPort != nil {
				t.Fatalf("services = %+v, want api without mainPort", got.Services)
			}
			if !strings.Contains(strings.Join(got.Warnings, " "), "multiple or unsupported") {
				t.Fatalf("warnings = %+v, want unsupported warning", got.Warnings)
			}
		})
	}
}

func TestScanComposeYAMLOutOfRangeWarns(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "compose.yaml", "services:\n  api:\n    ports:\n      - \"80:80\"\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services[0].MainPort != nil {
		t.Fatalf("services = %+v, want api without mainPort", got.Services)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "outside 1024..65535") {
		t.Fatalf("warnings = %+v, want out-of-range warning", got.Warnings)
	}
}

func TestScanComposeYAMLInvalidWarnsAndContinues(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "apps/api/.env", "PORT=3004\n")
	writeFile(t, root, "compose.yaml", "services: [\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != "3004" {
		t.Fatalf("services = %+v, want api mainPort 3004", got.Services)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "ignoring compose.yaml") {
		t.Fatalf("warnings = %+v, want invalid yaml warning", got.Warnings)
	}
}

func TestScanComposeYAMLExposeDoesNotEnrich(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "compose.yaml", "services:\n  api:\n    expose:\n      - \"4000\"\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services[0].MainPort != nil {
		t.Fatalf("services = %+v, want api without mainPort", got.Services)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings = %+v, want none", got.Warnings)
	}
}

func TestScanComposeYAMLReadsOnlyPorts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "compose.yaml", "version: \"3.9\"\nservices:\n  api:\n    build: .\n    environment:\n      PORT: 9999\n    ports:\n      - \"4000:4000\"\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 || got.Services[0].MainPort == nil || got.Services[0].MainPort.Value != "4000" {
		t.Fatalf("services = %+v, want api mainPort 4000", got.Services)
	}
}

func TestScanComposeYAMLAndJSONConflict(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "apps/api/package.json", `{"name":"api","scripts":{"dev":"nest start"}}`)
	writeFile(t, root, "docker-compose.json", `{"services":{"api":{"ports":["4000:4000"]}}}`)
	writeFile(t, root, "compose.yaml", "services:\n  api:\n    ports:\n      - \"4001:4001\"\n")

	got, err := Scan(context.Background(), Options{Root: root, MainRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Gaps) != 1 || got.Gaps[0].Field != "services.api.mainPort" || got.Gaps[0].Reason != "ambiguous" {
		t.Fatalf("gaps = %+v, want ambiguous mainPort", got.Gaps)
	}
	if len(got.Services) != 1 || got.Services[0].MainPort != nil {
		t.Fatalf("services = %+v, want api without mainPort", got.Services)
	}
}
