package application

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/envfile"
)

type RunRequest struct {
	Worktree string
	Service  domain.ServiceName
	Command  []string
}

type RunPlan struct {
	Path string
	Args []string
	Env  []string
}

type Executor interface {
	Exec(path string, argv, env []string) error
}

func (a *App) PrepareRun(ctx context.Context, req RunRequest) (RunPlan, error) {
	if len(req.Command) == 0 {
		return RunPlan{}, apperr.New("usage", "run: missing command", "use -- <command> [args...]")
	}
	c, err := a.Locator.Detect(req.Worktree)
	if err != nil {
		return RunPlan{}, mapProjectErr(err)
	}
	cfg, _, err := a.loadConfig(c.Root)
	if err != nil {
		return RunPlan{}, mapConfigErr(err)
	}
	if req.Service != "" {
		if _, ok := cfg.Services[req.Service]; !ok {
			return RunPlan{}, apperr.New(
				"usage",
				fmt.Sprintf("unknown service %q", req.Service),
				"use --service <name>",
			)
		}
	}
	path, err := exec.LookPath(req.Command[0])
	if err != nil {
		return RunPlan{}, apperr.New(
			"command_not_found",
			fmt.Sprintf("command not found: %s", req.Command[0]),
			"check the command",
		)
	}
	setup, err := a.Setup(ctx, req.Worktree)
	if err != nil {
		return RunPlan{}, err
	}
	values, err := runEnvValues(cfg, setup.Services, req.Service)
	if err != nil {
		return RunPlan{}, err
	}
	return RunPlan{Path: path, Args: req.Command, Env: mergeEnv(os.Environ(), values)}, nil
}

func (a *App) Run(ctx context.Context, req RunRequest) error {
	plan, err := a.PrepareRun(ctx, req)
	if err != nil {
		return err
	}
	if a.Executor == nil {
		return apperr.New("internal", "executor is not configured", "")
	}
	return a.Executor.Exec(plan.Path, plan.Args, plan.Env)
}

func runEnvValues(cfg domain.Config, services map[domain.ServiceName]domain.ServiceClaim, filter domain.ServiceName) ([]string, error) {
	values := map[string]string{}
	for _, target := range cfg.Env {
		if filter != "" && target.Service != filter {
			continue
		}
		claim, ok := services[target.Service]
		if !ok {
			continue
		}
		value := envfile.RenderValue(claim.Port, target.Template)
		if prev, ok := values[target.Key]; ok && prev != value {
			return nil, apperr.New("env_conflict",
				fmt.Sprintf("env key %q has conflicting values", target.Key),
				"use --service <name>")
		}
		values[target.Key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out, nil
}

func mergeEnv(base, overrides []string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	seen := map[string]bool{}
	for _, kv := range overrides {
		key, _, _ := strings.Cut(kv, "=")
		seen[key] = true
		out = append(out, kv)
	}
	for _, kv := range base {
		key, _, ok := strings.Cut(kv, "=")
		if !ok || seen[key] {
			continue
		}
		out = append(out, kv)
	}
	return out
}
