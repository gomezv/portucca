package application

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/config"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
	"github.com/gomezv/portucca/internal/scan"
)

func rootDet(services []scan.Service, env []scan.EnvTarget, copies []scan.CopyFile, rootEnv []scan.EnvSignal) scan.Detected {
	return scan.Detected{
		Project:  &scan.Value{Value: "app", Source: "directory"},
		Services: services, Env: env, CopyFromMain: copies, RootEnv: rootEnv,
	}
}

func hasEnvTarget(cfg domain.Config, service domain.ServiceName, file, key string) bool {
	for _, target := range cfg.Env {
		if target.Service == service && target.File == file && target.Key == key {
			return true
		}
	}
	return false
}

func hasCopy(cfg domain.Config, file string) bool {
	for _, copyFile := range cfg.Bootstrap.CopyFromMain {
		if copyFile == file {
			return true
		}
	}
	return false
}

func TestMergeRootEnvConvention(t *testing.T) {
	det := rootDet(
		[]scan.Service{
			{Name: "api", Dir: "apps/api"},
			{Name: "web", Dir: "apps/web", MainPort: &scan.Value{Value: "5173", Source: "apps/web/.env.local:WEB_PORT"}},
		},
		[]scan.EnvTarget{{Service: "web", File: "apps/web/.env.local", Key: "WEB_PORT", Source: "apps/web/.env.local:WEB_PORT"}},
		[]scan.CopyFile{{Service: "web", File: "apps/web/.env.local"}},
		[]scan.EnvSignal{{Value: 3414, File: ".env", Key: "API_PORT", Source: ".env:API_PORT", Rank: 2}},
	)
	merged, err := mergeDetected(InitRequest{}, det, nil)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Config.Services["api"].MainPort != 3414 || merged.Config.Services["api"].StartPort == 0 {
		t.Fatalf("api = %+v", merged.Config.Services["api"])
	}
	if !hasEnvTarget(merged.Config, "api", ".env", "API_PORT") || !hasCopy(merged.Config, ".env") {
		t.Fatalf("env=%+v copies=%+v", merged.Config.Env, merged.Config.Bootstrap.CopyFromMain)
	}
	if !hasEnvTarget(merged.Config, "web", "apps/web/.env.local", "WEB_PORT") {
		t.Fatalf("web local target missing: %+v", merged.Config.Env)
	}
	if len(merged.EnvCandidates) != 0 {
		t.Fatalf("candidates = %+v", merged.EnvCandidates)
	}
}

func TestMergeRootEnvLocalWins(t *testing.T) {
	det := rootDet(
		[]scan.Service{{Name: "api", Dir: "apps/api", MainPort: &scan.Value{Value: "3415", Source: "apps/api/.env:PORT"}}},
		[]scan.EnvTarget{{Service: "api", File: "apps/api/.env", Key: "PORT", Source: "apps/api/.env:PORT"}},
		[]scan.CopyFile{{Service: "api", File: "apps/api/.env"}},
		[]scan.EnvSignal{{Value: 3414, File: ".env", Key: "API_PORT", Source: ".env:API_PORT", Rank: 2}},
	)
	merged, err := mergeDetected(InitRequest{}, det, nil)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Config.Services["api"].MainPort != 3415 {
		t.Fatalf("api = %+v", merged.Config.Services["api"])
	}
	if hasEnvTarget(merged.Config, "api", ".env", "API_PORT") {
		t.Fatal("root target overrode the local target")
	}
	if !strings.Contains(strings.Join(merged.Warnings, " "), "already have env targets") {
		t.Fatalf("warnings = %+v", merged.Warnings)
	}
}

func TestMergeRootEnvValueFallback(t *testing.T) {
	det := rootDet(
		[]scan.Service{{Name: "api", Dir: "apps/api"}},
		nil, nil,
		[]scan.EnvSignal{{Value: 3414, File: ".env", Key: "SERVER_PORT", Source: ".env:SERVER_PORT", Rank: 2}},
	)
	merged, err := mergeDetected(InitRequest{Services: []string{"api:3414:3415"}}, det, map[domain.ServiceName]domain.Service{
		"api": {MainPort: 3414, StartPort: 3415},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasEnvTarget(merged.Config, "api", ".env", "SERVER_PORT") || !hasCopy(merged.Config, ".env") {
		t.Fatalf("env=%+v copies=%+v", merged.Config.Env, merged.Config.Bootstrap.CopyFromMain)
	}
	if len(merged.EnvCandidates) != 0 {
		t.Fatalf("candidates = %+v", merged.EnvCandidates)
	}
}

func TestMergeRootEnvAmbiguous(t *testing.T) {
	det := rootDet(
		[]scan.Service{{Name: "api", Dir: "apps/api"}},
		nil, nil,
		[]scan.EnvSignal{
			{Value: 3414, File: ".env", Key: "A_PORT", Source: ".env:A_PORT", Rank: 2},
			{Value: 3414, File: ".env", Key: "B_PORT", Source: ".env:B_PORT", Rank: 2},
		},
	)
	merged, err := mergeDetected(InitRequest{Services: []string{"api:3414:3415"}}, det, map[domain.ServiceName]domain.Service{
		"api": {MainPort: 3414, StartPort: 3415},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.EnvCandidates) != 2 {
		t.Fatalf("candidates = %+v", merged.EnvCandidates)
	}
	for _, candidate := range merged.EnvCandidates {
		if len(candidate.Suggested) != 1 || candidate.Suggested[0] != "api" {
			t.Fatalf("candidate = %+v", candidate)
		}
	}
	if hasEnvTarget(merged.Config, "api", ".env", "A_PORT") || hasEnvTarget(merged.Config, "api", ".env", "B_PORT") {
		t.Fatal("ambiguous candidate was bound")
	}
	if !strings.Contains(strings.Join(merged.Warnings, " "), "no unique service target") {
		t.Fatalf("warnings = %+v", merged.Warnings)
	}
}

func TestMergeRootEnvExplicitBinding(t *testing.T) {
	det := rootDet(
		[]scan.Service{{Name: "api", Dir: "apps/api"}},
		nil, nil,
		[]scan.EnvSignal{{Value: 9999, File: ".env", Key: "SERVER_PORT", Source: ".env:SERVER_PORT", Rank: 2}},
	)
	merged, err := mergeDetected(InitRequest{
		Services: []string{"api:3414:3415"},
		Env:      []EnvBinding{{Service: "api", File: ".env", Key: "SERVER_PORT"}},
	}, det, map[domain.ServiceName]domain.Service{"api": {MainPort: 3414, StartPort: 3415}})
	if err != nil {
		t.Fatal(err)
	}
	if !hasEnvTarget(merged.Config, "api", ".env", "SERVER_PORT") || !hasCopy(merged.Config, ".env") {
		t.Fatalf("env=%+v copies=%+v", merged.Config.Env, merged.Config.Bootstrap.CopyFromMain)
	}
	if len(merged.EnvCandidates) != 0 {
		t.Fatalf("candidates = %+v", merged.EnvCandidates)
	}
}

func TestMergeRootEnvInvalidBinding(t *testing.T) {
	base := rootDet(
		[]scan.Service{{Name: "api", Dir: "apps/api"}},
		nil, nil,
		[]scan.EnvSignal{{Value: 3414, File: ".env", Key: "SERVER_PORT", Source: ".env:SERVER_PORT", Rank: 2}},
	)
	explicit := map[domain.ServiceName]domain.Service{"api": {MainPort: 3414, StartPort: 3415}}
	cases := []InitRequest{
		{Services: []string{"api:3414:3415"}, Env: []EnvBinding{{Service: "web", File: ".env", Key: "SERVER_PORT"}}},
		{Services: []string{"api:3414:3415"}, Env: []EnvBinding{{Service: "api", File: ".env", Key: "MISSING"}}},
	}
	for _, req := range cases {
		_, err := mergeDetected(req, base, explicit)
		if ae, _ := apperr.As(err); ae == nil || ae.Code != "usage" {
			t.Fatalf("err = %v", err)
		}
	}

	local := rootDet(
		[]scan.Service{{Name: "api", Dir: "apps/api", MainPort: &scan.Value{Value: "3415", Source: "apps/api/.env:PORT"}}},
		[]scan.EnvTarget{{Service: "api", File: "apps/api/.env", Key: "PORT", Source: "apps/api/.env:PORT"}},
		nil,
		[]scan.EnvSignal{{Value: 3414, File: ".env", Key: "API_PORT", Source: ".env:API_PORT", Rank: 2}},
	)
	_, err := mergeDetected(InitRequest{Env: []EnvBinding{{Service: "api", File: ".env", Key: "API_PORT"}}}, local, nil)
	if ae, _ := apperr.As(err); ae == nil || ae.Code != "usage" {
		t.Fatalf("local target err = %v", err)
	}
}

func TestInitScanRootEnvConvention(t *testing.T) {
	det := rootDet(
		[]scan.Service{{Name: "api", Dir: "apps/api"}},
		nil, nil,
		[]scan.EnvSignal{{Value: 3414, File: ".env", Key: "API_PORT", Source: ".env:API_PORT", Rank: 2}},
	)
	app, dir := scanApp(t, det)
	app.Locator = fakeLocator{ctx: project.Context{Root: dir, MainRoot: dir, IsMain: true}}
	got, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigPath == "" {
		t.Fatal("config not written")
	}
	cfg, err := config.LoadFile(filepath.Join(dir, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !hasEnvTarget(cfg, "api", ".env", "API_PORT") || !hasCopy(cfg, ".env") {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestMergeRootEnvRankPrefersLocal(t *testing.T) {
	det := rootDet(
		[]scan.Service{{Name: "api", Dir: "apps/api"}},
		nil, nil,
		[]scan.EnvSignal{
			{Value: 3414, File: ".env", Key: "API_PORT", Source: ".env:API_PORT", Rank: 2},
			{Value: 3415, File: ".env.local", Key: "API_PORT", Source: ".env.local:API_PORT", Rank: 1},
		},
	)
	merged, err := mergeDetected(InitRequest{}, det, nil)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Config.Services["api"].MainPort != 3415 {
		t.Fatalf("api = %+v", merged.Config.Services["api"])
	}
	if !hasEnvTarget(merged.Config, "api", ".env.local", "API_PORT") || !hasCopy(merged.Config, ".env.local") {
		t.Fatalf("env=%+v copies=%+v", merged.Config.Env, merged.Config.Bootstrap.CopyFromMain)
	}
}

func TestMergeRootEnvCopyDedupe(t *testing.T) {
	det := rootDet(
		[]scan.Service{{Name: "api", Dir: "apps/api"}, {Name: "web", Dir: "apps/web"}},
		nil, nil,
		[]scan.EnvSignal{
			{Value: 3414, File: ".env", Key: "API_PORT", Source: ".env:API_PORT", Rank: 2},
			{Value: 5413, File: ".env", Key: "WEB_PORT", Source: ".env:WEB_PORT", Rank: 2},
		},
	)
	merged, err := mergeDetected(InitRequest{}, det, nil)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, file := range merged.Config.Bootstrap.CopyFromMain {
		if file == ".env" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("copyFromMain = %+v", merged.Config.Bootstrap.CopyFromMain)
	}
}

func TestMergeRootEnvConsumedBinding(t *testing.T) {
	det := rootDet(
		[]scan.Service{{Name: "api", Dir: "apps/api"}},
		nil, nil,
		[]scan.EnvSignal{{Value: 3414, File: ".env", Key: "API_PORT", Source: ".env:API_PORT", Rank: 2}},
	)
	_, err := mergeDetected(InitRequest{Env: []EnvBinding{{Service: "api", File: ".env", Key: "API_PORT"}}}, det, nil)
	if ae, _ := apperr.As(err); ae == nil || ae.Code != "usage" {
		t.Fatalf("err = %v", err)
	}
}

func TestInitScanRootEnvCandidateWarning(t *testing.T) {
	det := rootDet(
		[]scan.Service{{Name: "api", Dir: "apps/api"}},
		nil, nil,
		[]scan.EnvSignal{
			{Value: 3414, File: ".env", Key: "A_PORT", Source: ".env:A_PORT", Rank: 2},
			{Value: 3414, File: ".env", Key: "B_PORT", Source: ".env:B_PORT", Rank: 2},
		},
	)
	app, dir := scanApp(t, det)
	app.Locator = fakeLocator{ctx: project.Context{Root: dir, MainRoot: dir, IsMain: true}}
	got, err := app.Init(context.Background(), InitRequest{Scan: true, Worktree: dir, Services: []string{"api:3414:3415"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Scan == nil || !strings.Contains(strings.Join(got.Scan.Warnings, " "), "no unique service target") {
		t.Fatalf("warnings = %+v", got.Scan)
	}
}
