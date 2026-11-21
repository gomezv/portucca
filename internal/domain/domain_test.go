package domain

import (
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{
		Version: 1,
		Project: "app",
		Services: map[ServiceName]Service{
			"web": {MainPort: 5173, StartPort: 5174},
			"api": {MainPort: 3004, StartPort: 3005},
		},
		Env: []EnvTarget{
			{Service: "web", File: "apps/web/.env.local", Key: "WEB_PORT"},
			{Service: "api", File: "apps/api/.env.local", Key: "PORT"},
			{Service: "api", File: "apps/web/.env.local", Key: "API_PROXY_TARGET", Template: "http://localhost:{{port}}"},
		},
		Bootstrap: Bootstrap{
			CopyFromMain: []string{".env"},
			Symlinks:     []Symlink{{Path: "apps/api/.env", Target: "../../.env"}},
		},
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"valid", func(c *Config) {}, ""},
		{"version", func(c *Config) { c.Version = 2 }, "config_version"},
		{"project", func(c *Config) { c.Project = "App" }, "project"},
		{"no services", func(c *Config) { c.Services = nil }, "services"},
		{"bad service name", func(c *Config) {
			c.Services = map[ServiceName]Service{"Web": {MainPort: 5173, StartPort: 5174}}
		}, "service"},
		{"service name with dash and digit", func(c *Config) {
			c.Services["web-2"] = Service{MainPort: 6000, StartPort: 6001}
		}, ""},
		{"start below main", func(c *Config) {
			c.Services["web"] = Service{MainPort: 5174, StartPort: 5173}
		}, "startPort"},
		{"duplicate port", func(c *Config) {
			c.Services["api"] = Service{MainPort: 5173, StartPort: 3005}
		}, "duplicate port"},
		{"env unknown service", func(c *Config) {
			c.Env = append(c.Env, EnvTarget{Service: "worker", File: ".env.local", Key: "PORT"})
		}, "unknown service"},
		{"env duplicate key", func(c *Config) {
			c.Env = append(c.Env, EnvTarget{Service: "web", File: "apps/web/.env.local", Key: "WEB_PORT"})
		}, "duplicate env target"},
		{"env invalid key", func(c *Config) {
			c.Env = append(c.Env, EnvTarget{Service: "web", File: "apps/web/.env.local", Key: "API-PORT"})
		}, "invalid key"},
		{"env escapes repo", func(c *Config) {
			c.Env = append(c.Env, EnvTarget{Service: "web", File: "../outside", Key: "X"})
		}, "escapes"},
		{"bad template", func(c *Config) {
			c.Env[2].Template = "http://localhost:{{port}}{{port}}"
		}, "template"},
		{"env worktree template", func(c *Config) {
			c.Env = append(c.Env, EnvTarget{Service: "api", File: ".env.local", Key: "DATABASE_NAME", Template: "{{worktree}}"})
		}, ""},
		{"env mixed template", func(c *Config) {
			c.Env = append(c.Env, EnvTarget{Service: "api", File: ".env.local", Key: "DATABASE_NAME", Template: "{{port}}{{worktree}}"})
		}, "template"},
		{"adoptFrom without env", func(c *Config) {
			c.Services["web"] = Service{MainPort: 5173, StartPort: 5174, AdoptFrom: &EnvRef{File: "x", Key: "Y"}}
		}, "adoptFrom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(&c)
			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestConfigServiceNamesAreSorted(t *testing.T) {
	c := validConfig()
	got := c.ServiceNames()
	if len(got) != 2 || got[0] != "api" || got[1] != "web" {
		t.Fatalf("ServiceNames() = %v, want [api web]", got)
	}
}

func TestWorktreeID(t *testing.T) {
	tests := []struct{ name, project, root, want string }{
		{"spaces", "app", "/repo/Plan 2", "app_plan_2_ec6b82ba"},
		{"same basename other root", "app", "/other/Plan 2", "app_plan_2_1bada369"},
		{"project dash", "my-app", "/repo/wt", "my_app_wt_cc994380"},
		{"leading digit", "2app", "/repo/wt", "p_2app_wt_cc994380"},
		{"unicode", "app", "/repo/Ñandú", "app_and_7e85336a"},
		{"empty basename", "app", "", "app_wt_e3b0c442"},
	}
	for _, tt := range tests {
		if got := WorktreeID(Project(tt.project), tt.root); got != tt.want {
			t.Fatalf("%s: WorktreeID = %q, want %q", tt.name, got, tt.want)
		}
	}
	long := WorktreeID("app", "/repo/"+strings.Repeat("a", 80))
	wantLong := "app_" + strings.Repeat("a", 50) + "_ab07126d"
	if long != wantLong || len(long) != 63 {
		t.Fatalf("long ID = %q (%d bytes), want %q (63)", long, len(long), wantLong)
	}
	first := WorktreeID("app", "/repo/Plan 2")
	second := WorktreeID("app", "/repo/Plan 2")
	if first != second {
		t.Fatal("WorktreeID is not deterministic")
	}
}

func TestValidateTemplate(t *testing.T) {
	for _, ok := range []string{"http://localhost:{{port}}", "db_{{worktree}}"} {
		if err := validateTemplate("t", ok); err != nil {
			t.Fatalf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"{{port}}{{port}}", "{{worktree}}{{worktree}}",
		"{{port}}{{worktree}}", "{{unknown}}"} {
		if err := validateTemplate("t", bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestValidProjectAndServiceName(t *testing.T) {
	if !ValidProject("app-1") || ValidProject("App") || ValidProject("") {
		t.Fatal("ValidProject is wrong")
	}
	if !ValidServiceName("web") || ValidServiceName("Web") || ValidServiceName("1web") {
		t.Fatal("ValidServiceName is wrong")
	}
}
