package tui

import (
	"testing"

	"github.com/gomezv/portucca/internal/application"
)

func projectsFixture() application.OverviewResult {
	ov := overviewFixture()
	ov.Worktrees = append(ov.Worktrees, application.OverviewWorktree{
		Project: "other", Path: "/tmp/other-main", Kind: "main", Live: true,
		Services: []application.OverviewService{{Name: "api", Port: 4000, MainPort: 4000, StartPort: 4001, InBand: true}},
		Issues:   []application.Issue{},
	})
	return ov
}

func TestGroupProjects(t *testing.T) {
	got := groupProjects(projectsFixture().Worktrees)
	if len(got) != 2 {
		t.Fatalf("projects = %+v", got)
	}
	if got[0].Project != "app" || got[0].Worktrees != 1 || got[0].Mains != 1 || got[0].MinPort != 5173 || got[0].MaxPort != 5174 {
		t.Fatalf("app = %+v", got[0])
	}
	if got[1].Project != "other" || got[1].Mains != 1 || got[1].MinPort != 4000 {
		t.Fatalf("other = %+v", got[1])
	}
}

func TestDiscoveryRows(t *testing.T) {
	rows := discoveryRows(application.DiscoveryResult{Worktrees: []application.DiscoveredWorktree{{
		Project: "app", Path: "/tmp/new", Branch: "feature", MainRoot: "/tmp/main",
	}}})
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	row := rows[0]
	if row.Kind != "unregistered" || !row.Live || row.Project != "app" {
		t.Fatalf("row = %+v", row)
	}
	if len(row.Issues) != 1 || row.Issues[0].Code != "unregistered" {
		t.Fatalf("issues = %+v", row.Issues)
	}
}
