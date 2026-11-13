package tui

import (
	"context"
	"sort"
	"strconv"

	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/domain"
)

type Discoverer interface {
	Discover(ctx context.Context, project string) (application.DiscoveryResult, error)
}

type viewLevel int

const (
	levelProjects viewLevel = iota
	levelProject
)

type projectSummary struct {
	Project   domain.Project
	Worktrees int
	Mains     int
	Errors    int
	Warnings  int
	MinPort   domain.Port
	MaxPort   domain.Port
	HasPorts  bool
}

func groupProjects(entries []application.OverviewWorktree) []projectSummary {
	byProject := map[domain.Project]*projectSummary{}
	for _, entry := range entries {
		summary, ok := byProject[entry.Project]
		if !ok {
			summary = &projectSummary{Project: entry.Project}
			byProject[entry.Project] = summary
		}
		switch entry.Kind {
		case "worktree":
			summary.Worktrees++
		case "main":
			summary.Mains++
		}
		for _, issue := range entry.Issues {
			if isErrorIssue(issue.Code) {
				summary.Errors++
			} else {
				summary.Warnings++
			}
		}
		for _, svc := range entry.Services {
			if svc.Port <= 0 {
				continue
			}
			if !summary.HasPorts || svc.Port < summary.MinPort {
				summary.MinPort = svc.Port
			}
			if !summary.HasPorts || svc.Port > summary.MaxPort {
				summary.MaxPort = svc.Port
			}
			summary.HasPorts = true
		}
	}
	out := make([]projectSummary, 0, len(byProject))
	for _, summary := range byProject {
		out = append(out, *summary)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Project < out[j].Project })
	return out
}

func discoveryRows(result application.DiscoveryResult) []application.OverviewWorktree {
	rows := make([]application.OverviewWorktree, 0, len(result.Worktrees))
	for _, wt := range result.Worktrees {
		message := "no claim; run setup"
		if wt.Detached {
			message = "no claim (detached); run setup"
		} else if wt.Branch != "" {
			message = "no claim (branch " + wt.Branch + "); run setup"
		}
		rows = append(rows, application.OverviewWorktree{
			Project: wt.Project,
			Path:    wt.Path,
			Kind:    "unregistered",
			Live:    true,
			Issues:  []application.Issue{{Code: "unregistered", Message: message}},
		})
	}
	return rows
}

func projectStatusText(summary projectSummary) string {
	switch {
	case summary.Errors > 0:
		return redStyle.Render("✖ " + strconv.Itoa(summary.Errors))
	case summary.Warnings > 0:
		return yellowStyle.Render("▲ " + strconv.Itoa(summary.Warnings))
	default:
		return greenStyle.Render("● ok")
	}
}

func projectPortsText(summary projectSummary) string {
	if !summary.HasPorts {
		return "—"
	}
	if summary.MinPort == summary.MaxPort {
		return strconv.Itoa(int(summary.MinPort))
	}
	return strconv.Itoa(int(summary.MinPort)) + "-" + strconv.Itoa(int(summary.MaxPort))
}
