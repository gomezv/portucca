package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gomezv/portucca/internal/application"
)

func overviewFixture() application.OverviewResult {
	return application.OverviewResult{
		RegistryVersion: 2,
		Worktrees: []application.OverviewWorktree{
			{
				Project: "app", Path: "/tmp/main", Kind: "main", Live: true,
				Services: []application.OverviewService{{
					Name: "web", Port: 5173, MainPort: 5173, StartPort: 5174, InBand: true,
				}},
				Issues: []application.Issue{},
			},
			{
				Project: "app", Path: "/tmp/wt", Kind: "worktree", Live: true,
				Services: []application.OverviewService{{
					Name: "web", Port: 5174, MainPort: 5173, StartPort: 5174, InBand: true,
				}},
				Issues: []application.Issue{},
			},
		},
	}
}

func sizedModel(t *testing.T, ov application.OverviewResult) Model {
	t.Helper()
	m := New(ov, "/tmp/ports.json", "", nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return next.(Model)
}

func TestModelViewShowsRows(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	view := m.View()
	if !view.AltScreen {
		t.Fatal("AltScreen = false")
	}
	for _, want := range []string{"app", "main", "worktree", "/tmp/wt", "5174", "ok"} {
		if !strings.Contains(view.Content, want) {
			t.Fatalf("view missing %q:\n%s", want, view.Content)
		}
	}
}

func TestModelDetail(t *testing.T) {
	ov := overviewFixture()
	ov.Worktrees[1].Issues = []application.Issue{{Code: "config_missing", Message: "portucca.json is missing"}}
	m := sizedModel(t, ov)
	m.table.SetCursor(1)
	m.syncDetail()
	for _, want := range []string{"main=5173", "start=5174", "in band", "config_missing"} {
		if !strings.Contains(m.detail.View(), want) {
			t.Fatalf("detail missing %q:\n%s", want, m.detail.View())
		}
	}
}

func TestModelTab(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !next.(Model).focusDetail {
		t.Fatal("focusDetail = false, want true")
	}
	if !strings.Contains(next.(Model).View().Content, "▸ details") {
		t.Fatal("focused detail title is not marked")
	}
}

func TestModelKeys(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	m.table.SetCursor(0)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if got := next.(Model).table.Cursor(); got != 1 {
		t.Fatalf("cursor after j = %d, want 1", got)
	}
	next, _ = next.(Model).Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if got := next.(Model).table.Cursor(); got != 0 {
		t.Fatalf("cursor after k = %d, want 0", got)
	}
	cmds := []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: 'c', Mod: tea.ModCtrl},
	}
	for _, key := range cmds {
		_, cmd := next.(Model).Update(key)
		if cmd == nil {
			t.Fatalf("key %s returned nil command", key.String())
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("key %s did not quit", key.String())
		}
	}
}

func TestModelRefresh(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	next := overviewFixture()
	next.Worktrees = next.Worktrees[:1]
	m.refresh = func(context.Context) (application.OverviewResult, error) { return next, nil }
	cmd := m.startRefresh()
	if cmd == nil || !m.refreshing {
		t.Fatalf("startRefresh = %v refreshing=%v", cmd, m.refreshing)
	}
	msg := cmd()
	if _, ok := msg.(refreshMsg); !ok {
		t.Fatalf("msg = %T, want refreshMsg", msg)
	}
	updated, _ := m.Update(msg)
	if len(updated.(Model).entries) != 1 || updated.(Model).refreshing {
		t.Fatalf("entries = %d refreshing=%v", len(updated.(Model).entries), updated.(Model).refreshing)
	}
}

func TestModelRefreshNoOverlap(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	m.refresh = func(context.Context) (application.OverviewResult, error) { return overviewFixture(), nil }
	if m.startRefresh() == nil {
		t.Fatal("first refresh was nil")
	}
	if m.startRefresh() != nil {
		t.Fatal("second refresh started while one was in flight")
	}
	next, _ := m.Update(refreshMsg{overview: overviewFixture()})
	mm := next.(Model)
	if mm.startRefresh() == nil {
		t.Fatal("refresh was not re-enabled after completion")
	}
}

func TestModelRefreshError(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	m.refreshing = true
	next, _ := m.Update(refreshErrMsg{err: errors.New("boom")})
	mm := next.(Model)
	if mm.refreshing || mm.err == nil || !strings.Contains(mm.View().Content, "boom") {
		t.Fatalf("refreshing=%v err=%v", mm.refreshing, mm.err)
	}
}

func TestModelTick(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	m.refresh = func(context.Context) (application.OverviewResult, error) { return overviewFixture(), nil }
	next, cmd := m.Update(tickMsg(time.Now()))
	if cmd == nil || !next.(Model).refreshing {
		t.Fatalf("tick cmd=%v refreshing=%v", cmd, next.(Model).refreshing)
	}
}

func TestModelPreservesSelection(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	m.table.SetCursor(1)
	next, _ := m.Update(refreshMsg{overview: overviewFixture()})
	if got := next.(Model).table.Cursor(); got != 1 {
		t.Fatalf("cursor = %d, want 1", got)
	}
}

func TestModelArrowKeys(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	m.table.SetCursor(0)
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if got := next.(Model).table.Cursor(); got != 1 {
		t.Fatalf("cursor after down = %d, want 1", got)
	}
	next, _ = next.(Model).Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if got := next.(Model).table.Cursor(); got != 0 {
		t.Fatalf("cursor after up = %d, want 0", got)
	}
}

func TestModelSelectionFallback(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	m.table.SetCursor(1)
	next := overviewFixture()
	next.Worktrees = next.Worktrees[:1]
	updated, _ := m.Update(refreshMsg{overview: next})
	if got := updated.(Model).table.Cursor(); got != 0 {
		t.Fatalf("cursor after selected path disappears = %d, want 0", got)
	}
}

func TestModelEightyColumns(t *testing.T) {
	m := New(overviewFixture(), "/home/user/.local/state/portucca/ports.json", "", nil)
	m.refreshing = true
	m.err = errors.New("boom with a very long message that should be truncated")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := next.(Model).View().Content
	if !strings.Contains(view, "STATUS") || !strings.Contains(view, "web=5174") {
		t.Fatalf("view at 80x24:\n%s", view)
	}
	if !strings.Contains(view, "quit") {
		t.Fatalf("footer missing at 80x24:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 80 {
			t.Fatalf("line exceeds 80 columns (%d): %q", lipgloss.Width(line), line)
		}
	}
}

func TestModelRefreshKey(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	m.refresh = func(context.Context) (application.OverviewResult, error) { return overviewFixture(), nil }
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r did not start a refresh")
	}
	if !next.(Model).refreshing {
		t.Fatal("refreshing = false after r")
	}
}

func TestModelDetailScrollReset(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	m.table.SetCursor(0)
	m.syncDetail()
	m.detail.SetYOffset(1)
	m.table.SetCursor(1)
	m.syncDetail()
	if got := m.detail.YOffset(); got != 0 {
		t.Fatalf("YOffset = %d, want 0 after selection change", got)
	}
}

func TestPathTextKeepsTail(t *testing.T) {
	got := pathText("/home/user/projects/very/long/alpha", 13)
	if !strings.Contains(got, "alpha") {
		t.Fatalf("pathText = %q, want the leaf", got)
	}
	if lipgloss.Width(got) > 13 {
		t.Fatalf("pathText width = %d, want <= 13", lipgloss.Width(got))
	}
}

func TestModelMinimumSize(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 79, Height: 24}, {Width: 80, Height: 15}} {
		m := New(overviewFixture(), "", "", nil)
		next, _ := m.Update(size)
		if !strings.Contains(next.(Model).View().Content, "too small") {
			t.Fatalf("%dx%d did not report too small", size.Width, size.Height)
		}
	}
	m := New(overviewFixture(), "", "", nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	view := next.(Model).View().Content
	lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
	if len(lines) != 16 {
		t.Fatalf("lines = %d, want 16", len(lines))
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 80 {
			t.Fatalf("line exceeds 80 columns: %q", line)
		}
	}
}

func TestModelEmpty(t *testing.T) {
	m := sizedModel(t, application.OverviewResult{RegistryVersion: 2, Worktrees: []application.OverviewWorktree{}})
	if !strings.Contains(m.View().Content, "no claims") {
		t.Fatalf("view = %s", m.View().Content)
	}
}

func TestModelTooSmall(t *testing.T) {
	m := New(overviewFixture(), "/tmp/ports.json", "", nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 5})
	if !strings.Contains(next.(Model).View().Content, "too small") {
		t.Fatalf("view = %s", next.(Model).View().Content)
	}
}
