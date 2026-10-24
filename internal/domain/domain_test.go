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
