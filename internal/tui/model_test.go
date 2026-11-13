package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gomezv/portucca/internal/apperr"
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

func projectsModel(t *testing.T, ov application.OverviewResult) Model {
	t.Helper()
	m := New(ov, "/tmp/ports.json", "", nil, nil, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return next.(Model)
}

func sizedModel(t *testing.T, ov application.OverviewResult) Model {
	t.Helper()
	m := projectsModel(t, ov)
	if len(m.projects) > 0 {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = next.(Model)
	}
	return m
}

func TestModelViewShowsRows(t *testing.T) {
	m := projectsModel(t, overviewFixture())
	view := m.View()
	if !view.AltScreen {
		t.Fatal("AltScreen = false")
	}
	for _, want := range []string{"app", "1 project", "WORKTREES", "MAINS", "5173-5174", "ok"} {
		if !strings.Contains(view.Content, want) {
			t.Fatalf("view missing %q:\n%s", want, view.Content)
		}
	}
}

func TestModelViewProjectRows(t *testing.T) {
	m := sizedModel(t, overviewFixture())
	view := m.View().Content
	for _, want := range []string{"KIND", "PATH", "SERVICES", "STATUS", "/tmp/wt", "worktree", "5174", "ok"} {
		if !strings.Contains(view, want) {
			t.Fatalf("project view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "PROJECT") {
		t.Fatalf("level-2 header still has PROJECT:\n%s", view)
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
	m := New(overviewFixture(), "/home/user/.local/state/portucca/ports.json", "", nil, nil, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	m.refreshing = true
	m.err = errors.New("boom with a very long message that should be truncated")
	next, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
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

func TestModelResizeRebuildsPaths(t *testing.T) {
	ov := application.OverviewResult{RegistryVersion: 2, Worktrees: []application.OverviewWorktree{{
		Project: "app", Path: "/home/user/projects/very/long/alpha", Kind: "worktree", Live: true,
		Services: []application.OverviewService{{Name: "web", Port: 5174, MainPort: 5173, StartPort: 5174, InBand: true}},
		Issues:   []application.Issue{},
	}}}
	m := New(ov, "", "", nil, nil, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	rows := m.table.Rows()
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	cell := rows[0][1]
	if !strings.Contains(cell, "/home/user/projects/very/long/alpha") {
		t.Fatalf("path cell = %q, want the full path in the PATH column", cell)
	}
}

func TestModelMinimumSize(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 79, Height: 24}, {Width: 80, Height: 15}} {
		m := New(overviewFixture(), "", "", nil, nil, nil)
		next, _ := m.Update(size)
		if !strings.Contains(next.(Model).View().Content, "too small") {
			t.Fatalf("%dx%d did not report too small", size.Width, size.Height)
		}
	}
	m := New(overviewFixture(), "", "", nil, nil, nil)
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
	m := projectsModel(t, application.OverviewResult{RegistryVersion: 2, Worktrees: []application.OverviewWorktree{}})
	if !strings.Contains(m.View().Content, "no projects") {
		t.Fatalf("view = %s", m.View().Content)
	}
}

func TestModelTooSmall(t *testing.T) {
	m := New(overviewFixture(), "/tmp/ports.json", "", nil, nil, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 5})
	if !strings.Contains(next.(Model).View().Content, "too small") {
		t.Fatalf("view = %s", next.(Model).View().Content)
	}
}

type fakeActionCall struct {
	action   Action
	worktree string
}

type fakeActions struct {
	calls   []fakeActionCall
	summary string
	err     error
}

func (f *fakeActions) Run(_ context.Context, action Action, worktree string) (string, error) {
	f.calls = append(f.calls, fakeActionCall{action: action, worktree: worktree})
	return f.summary, f.err
}

func actionModel(t *testing.T, actions ActionRunner, row int) Model {
	t.Helper()
	m := New(overviewFixture(), "/tmp/ports.json", "", nil, actions, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	m.table.SetCursor(row)
	m.syncDetail()
	return m
}

func TestModelActionMenu(t *testing.T) {
	m := actionModel(t, &fakeActions{}, 0)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	if m.menu == nil {
		t.Fatal("menu = nil, want actions")
	}
	if len(m.menu.items) != 3 || m.menu.items[0] != ActionEnsure || m.menu.items[2] != ActionPrune {
		t.Fatalf("main menu = %v", m.menu.items)
	}
	view := m.View().Content
	if strings.Contains(view, "release") {
		t.Fatalf("main menu should not offer release:\n%s", view)
	}
	if !strings.Contains(view, "ensure") || !strings.Contains(view, "prune") {
		t.Fatalf("main menu missing actions:\n%s", view)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.menu != nil {
		t.Fatal("menu still open after esc")
	}

	m = actionModel(t, &fakeActions{}, 1)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	if len(m.menu.items) != 5 || m.menu.items[3] != ActionRelease {
		t.Fatalf("worktree menu = %v", m.menu.items)
	}
}

func TestModelActionEnsure(t *testing.T) {
	actions := &fakeActions{summary: "ensure: changed=true services=1"}
	m := actionModel(t, actions, 0)
	m.refresh = func(context.Context) (application.OverviewResult, error) {
		return overviewFixture(), nil
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || m.running == "" {
		t.Fatalf("cmd = %v running = %q", cmd, m.running)
	}
	msg := cmd()
	next, refresh := m.Update(msg)
	m = next.(Model)
	if m.running != "" || m.notice != actions.summary {
		t.Fatalf("running = %q notice = %q", m.running, m.notice)
	}
	if len(actions.calls) != 1 || actions.calls[0] != (fakeActionCall{action: ActionEnsure, worktree: "/tmp/main"}) {
		t.Fatalf("calls = %+v", actions.calls)
	}
	if refresh == nil {
		t.Fatal("action completion did not trigger a refresh")
	}
}

func TestModelActionConfirm(t *testing.T) {
	actions := &fakeActions{summary: "release: released=1"}
	m := actionModel(t, actions, 1)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	for m.menu.cursor < 3 {
		next, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = next.(Model)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.confirm == nil {
		t.Fatal("confirm = nil, want release confirmation")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = next.(Model)
	if m.confirm != nil || len(actions.calls) != 0 {
		t.Fatalf("cancel ran the action: confirm=%v calls=%+v", m.confirm, actions.calls)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	for m.menu.cursor < 3 {
		next, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = next.(Model)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(Model)
	if cmd == nil || m.confirm != nil {
		t.Fatalf("cmd = %v confirm = %v", cmd, m.confirm)
	}
	msg := cmd()
	next, _ = m.Update(msg)
	m = next.(Model)
	if len(actions.calls) != 1 || actions.calls[0].action != ActionRelease {
		t.Fatalf("calls = %+v", actions.calls)
	}
}

func TestModelActionPrune(t *testing.T) {
	ov := overviewFixture()
	ov.Worktrees[0].Issues = []application.Issue{{Code: "config_missing", Message: "portucca.json is missing"}}
	actions := &fakeActions{summary: "prune: removed=1 mains=1"}
	m := New(ov, "/tmp/ports.json", "", nil, actions, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	m.table.SetCursor(0)
	next, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	if len(m.menu.items) != 1 || m.menu.items[0] != ActionPrune {
		t.Fatalf("menu = %v, want prune only", m.menu.items)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.confirm == nil {
		t.Fatal("prune did not ask for confirmation")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(Model)
	msg := cmd()
	next, _ = m.Update(msg)
	m = next.(Model)
	if len(actions.calls) != 1 || actions.calls[0] != (fakeActionCall{action: ActionPrune, worktree: "/tmp/main"}) {
		t.Fatalf("calls = %+v", actions.calls)
	}
	if m.notice != actions.summary {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestModelActionError(t *testing.T) {
	actions := &fakeActions{err: apperr.New("no_claim", "this worktree has no claim", "run portucca ensure first")}
	m := actionModel(t, actions, 0)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	msg := cmd()
	next, _ = m.Update(msg)
	m = next.(Model)
	if m.noticeErr == nil {
		t.Fatal("noticeErr = nil, want the action error")
	}
	view := m.View().Content
	for _, want := range []string{"no_claim", "run portucca ensure first"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestModelActionRunningBlocksKeys(t *testing.T) {
	m := actionModel(t, &fakeActions{summary: "ensure: changed=false services=1"}, 0)
	m.refresh = func(context.Context) (application.OverviewResult, error) { return overviewFixture(), nil }
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || m.running == "" {
		t.Fatalf("cmd = %v running = %q", cmd, m.running)
	}
	if next, quit := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); quit != nil || next.(Model).running == "" {
		t.Fatalf("q during action = %v", quit)
	}
	if refresh := m.startRefresh(); refresh != nil {
		t.Fatal("refresh started while an action was running")
	}
}

func TestModelActionMenuUsesOpenedPath(t *testing.T) {
	actions := &fakeActions{summary: "ensure: changed=true services=1"}
	m := actionModel(t, actions, 1)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	m.table.SetCursor(0)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	msg := cmd()
	next, _ = m.Update(msg)
	_ = next
	if len(actions.calls) != 1 || actions.calls[0].worktree != "/tmp/wt" {
		t.Fatalf("calls = %+v, want the path captured when the menu opened", actions.calls)
	}
}

func TestModelActionMenuClosesWhenRowDisappears(t *testing.T) {
	actions := &fakeActions{}
	m := actionModel(t, actions, 1)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	ov := overviewFixture()
	ov.Worktrees = ov.Worktrees[:1]
	next, _ = m.Update(refreshMsg{overview: ov})
	m = next.(Model)
	if m.menu != nil {
		t.Fatal("menu stayed open after its row disappeared")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if len(actions.calls) != 0 {
		t.Fatalf("calls = %+v, want none", actions.calls)
	}
}

func TestModelActionNoticeDoesNotMaskRefreshError(t *testing.T) {
	actions := &fakeActions{summary: "ensure: changed=true services=1"}
	m := actionModel(t, actions, 0)
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	next, _ = m.Update(refreshErrMsg{err: errors.New("registry boom")})
	m = next.(Model)
	view := m.View().Content
	if !strings.Contains(view, "registry boom") {
		t.Fatalf("view hides the refresh error:\n%s", view)
	}
	if strings.Contains(view, "ensure: changed=true") {
		t.Fatalf("view still shows the stale success notice:\n%s", view)
	}
}

type fakeDiscoverer struct {
	calls  []string
	result application.DiscoveryResult
	err    error
}

func (f *fakeDiscoverer) Discover(_ context.Context, project string) (application.DiscoveryResult, error) {
	f.calls = append(f.calls, project)
	return f.result, f.err
}

func TestModelProjects(t *testing.T) {
	m := projectsModel(t, projectsFixture())
	if m.level != levelProjects {
		t.Fatalf("level = %v, want projects", m.level)
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.level != levelProject || m.project != "app" {
		t.Fatalf("level = %v project = %q", m.level, m.project)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.level != levelProjects {
		t.Fatalf("level = %v, want projects after esc", m.level)
	}
}

func TestModelFocusPath(t *testing.T) {
	m := New(projectsFixture(), "/tmp/ports.json", "/tmp/wt", nil, nil, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	if m.level != levelProject || m.project != "app" || m.selectedPath() != "/tmp/wt" {
		t.Fatalf("level = %v project = %q path = %q", m.level, m.project, m.selectedPath())
	}
}

func TestModelDiscoveryScope(t *testing.T) {
	discover := &fakeDiscoverer{result: application.DiscoveryResult{Worktrees: []application.DiscoveredWorktree{{
		Project: "app", Path: "/tmp/new", Branch: "feature", MainRoot: "/tmp/main",
	}}}}
	m := New(projectsFixture(), "/tmp/ports.json", "", nil, nil, discover)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	if len(discover.calls) != 0 {
		t.Fatalf("discovery ran at level 1: %v", discover.calls)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("open project did not start discovery")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if len(discover.calls) != 1 || discover.calls[0] != "app" {
		t.Fatalf("calls = %v", discover.calls)
	}
	found := false
	for _, entry := range m.entries {
		if entry.Kind == "unregistered" && entry.Path == "/tmp/new" {
			found = true
		}
	}
	if !found {
		t.Fatalf("entries = %+v, want the discovered row", m.entries)
	}
}

func TestModelUnregisteredOnlySetup(t *testing.T) {
	actions := &fakeActions{}
	discover := &fakeDiscoverer{result: application.DiscoveryResult{Worktrees: []application.DiscoveredWorktree{{
		Project: "app", Path: "/tmp/new", Branch: "feature", MainRoot: "/tmp/main",
	}}}}
	m := New(projectsFixture(), "/tmp/ports.json", "", nil, actions, discover)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	for i, entry := range m.entries {
		if entry.Kind == "unregistered" {
			m.table.SetCursor(i)
		}
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	if len(m.menu.items) != 1 || m.menu.items[0] != ActionSetup {
		t.Fatalf("menu = %v, want setup only", m.menu.items)
	}
}

func TestModelDiscoveryError(t *testing.T) {
	discover := &fakeDiscoverer{err: errors.New("git boom")}
	m := New(projectsFixture(), "/tmp/ports.json", "", nil, nil, discover)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.discoverErr == nil || !strings.Contains(m.View().Content, "git boom") {
		t.Fatalf("discoverErr = %v", m.discoverErr)
	}
}

func TestModelUnregisteredSetup(t *testing.T) {
	actions := &fakeActions{summary: "setup: services=1"}
	discover := &fakeDiscoverer{result: application.DiscoveryResult{Worktrees: []application.DiscoveredWorktree{{
		Project: "app", Path: "/tmp/new", Branch: "feature", MainRoot: "/tmp/main",
	}}}}
	m := New(overviewFixture(), "/tmp/ports.json", "", nil, actions, discover)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	for i, entry := range m.entries {
		if entry.Kind == "unregistered" {
			m.table.SetCursor(i)
		}
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)

	refreshed := overviewFixture()
	refreshed.Worktrees = append(refreshed.Worktrees, application.OverviewWorktree{
		Project: "app", Path: "/tmp/new", Kind: "worktree", Live: true,
		Services: []application.OverviewService{{Name: "web", Port: 5175, MainPort: 5173, StartPort: 5174, InBand: true}},
		Issues:   []application.Issue{},
	})
	next, _ = m.Update(refreshMsg{overview: refreshed})
	m = next.(Model)
	next, _ = m.Update(discoverMsg{project: "app"})
	m = next.(Model)
	found := false
	for _, entry := range m.entries {
		if entry.Path == "/tmp/new" && entry.Kind == "worktree" {
			found = true
		}
	}
	if !found {
		t.Fatalf("entries = %+v, want the unregistered row converted to worktree", m.entries)
	}
}

func TestModelProjectsSelection(t *testing.T) {
	m := sizedModel(t, projectsFixture())
	next := projectsFixture()
	next.Worktrees = next.Worktrees[2:]
	updated, _ := m.Update(refreshMsg{overview: next})
	if updated.(Model).level != levelProjects {
		t.Fatalf("level = %v, want projects after the project disappeared", updated.(Model).level)
	}
}
