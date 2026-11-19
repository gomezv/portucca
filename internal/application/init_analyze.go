package application

import (
	"context"
	"errors"
	"os"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/project"
	"github.com/gomezv/portucca/internal/scan"
)

type InitPath struct {
	Root     string
	MainRoot string
	IsMain   bool
}

type InitGap struct {
	Field   string
	Reason  string
	Message string
	Flags   []string
}

type EnvCandidate struct {
	Value     int
	File      string
	Key       string
	Source    string
	Suggested []domain.ServiceName
}

type InitProposal struct {
	Root          string
	MainRoot      string
	IsMain        bool
	ConfigPath    string
	ConfigExists  bool
	Config        domain.Config
	Project       domain.Project
	Services      map[domain.ServiceName]domain.Service
	Sources       []ScanSource
	Warnings      []string
	Gaps          []InitGap
	EnvCandidates []EnvCandidate
}

func (a *App) InspectInitPath(ctx context.Context, path string) (InitPath, error) {
	if err := ctx.Err(); err != nil {
		return InitPath{}, err
	}
	c, err := a.Locator.Detect(path)
	if err != nil {
		return InitPath{}, mapProjectErr(err)
	}
	return InitPath{Root: c.Root, MainRoot: c.MainRoot, IsMain: c.IsMain}, nil
}

func (a *App) AnalyzeInit(ctx context.Context, req InitRequest) (InitProposal, error) {
	if !req.Scan || req.Preset != "" {
		return InitProposal{}, apperr.New("usage", "analyze requires scan mode", "use --scan without --preset")
	}
	worktree := req.Worktree
	if worktree == "" {
		worktree = "."
	}
	c, err := a.Locator.Detect(worktree)
	if err != nil {
		return InitProposal{}, mapProjectErr(err)
	}
	proposal, _, err := a.analyzeInit(ctx, c, req)
	return proposal, err
}

func (a *App) analyzeInit(ctx context.Context, c project.Context, req InitRequest) (InitProposal, mergedScan, error) {
	explicit, err := parseExplicitServices(req.Services)
	if err != nil {
		return InitProposal{}, mergedScan{}, err
	}
	if req.Project != "" && !domain.ValidProject(req.Project) {
		return InitProposal{}, mergedScan{}, apperr.New("usage", "invalid project "+req.Project, "use a lowercase slug")
	}
	path, err := initConfigPath(c, req)
	if err != nil {
		return InitProposal{}, mergedScan{}, err
	}
	if a.Scanner == nil {
		return InitProposal{}, mergedScan{}, apperr.New("internal", "scan is not configured", "rebuild the binary")
	}
	detected, err := a.Scanner.Scan(ctx, scan.Options{
		Root: c.Root, MainRoot: c.MainRoot,
		DiscoverCompose: req.Compose, ExcludeServices: req.ExcludeServices,
	})
	if err != nil {
		var tooLarge *scan.TooLargeError
		if errors.As(err, &tooLarge) {
			ae := apperr.New("scan_too_large", "scan exceeded the entry limit", "use --service flags to configure the project manually")
			ae.Details = map[string]any{"limit": tooLarge.Limit, "visited": tooLarge.Visited}
			return InitProposal{}, mergedScan{}, ae
		}
		return InitProposal{}, mergedScan{}, apperr.New("internal", err.Error(), "retry or use --service flags")
	}
	merged, err := mergeDetected(req, detected, explicit)
	if err != nil {
		return InitProposal{}, mergedScan{}, err
	}
	if len(merged.Gaps) == 0 {
		if verr := merged.Config.Validate(); verr != nil {
			if len(req.Services) > 0 {
				return InitProposal{}, mergedScan{}, apperr.New("usage", verr.Error(), "check --project and --service")
			}
			if len(merged.Config.Services) == 0 {
				merged.Gaps = append(merged.Gaps, scan.Gap{
					Field: "services", Reason: "missing", Message: verr.Error(),
					Flags: []string{"--service <name>:<main>:<start>"},
				})
			} else {
				merged.Gaps = append(merged.Gaps, scan.Gap{
					Field: "config", Reason: "invalid", Message: verr.Error(),
					Flags: []string{"--project <slug>", "--service <name>:<main>:<start>"},
				})
			}
		}
	}
	exists := false
	if _, statErr := os.Stat(path); statErr == nil {
		exists = true
	}
	proposal := InitProposal{
		Root: c.Root, MainRoot: c.MainRoot, IsMain: c.IsMain,
		ConfigPath: path, ConfigExists: exists,
		Config: merged.Config, Project: merged.Config.Project, Services: merged.Config.Services,
		Sources: merged.Sources, Warnings: merged.Warnings, Gaps: toInitGaps(merged.Gaps),
		EnvCandidates: merged.EnvCandidates,
	}
	return proposal, merged, nil
}

func toInitGaps(gaps []scan.Gap) []InitGap {
	out := make([]InitGap, 0, len(gaps))
	for _, g := range gaps {
		out = append(out, InitGap{Field: g.Field, Reason: g.Reason, Message: g.Message, Flags: g.Flags})
	}
	return out
}
