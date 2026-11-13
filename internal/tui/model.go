package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/domain"
)

const (
	refreshInterval = 5 * time.Second
	minWidth        = 80
	minHeight       = 16
)

var (
	accentColor  = lipgloss.Color("#B794F6")
	mutedColor   = lipgloss.Color("#8A8496")
	greenColor   = lipgloss.Color("#4FBF8B")
	yellowColor  = lipgloss.Color("#D9A441")
	redColor     = lipgloss.Color("#E0607E")
	surfaceColor = lipgloss.Color("#2A273F")

	accentStyle = lipgloss.NewStyle().Foreground(accentColor)
	mutedStyle  = lipgloss.NewStyle().Foreground(mutedColor)
	greenStyle  = lipgloss.NewStyle().Foreground(greenColor)
	yellowStyle = lipgloss.NewStyle().Foreground(yellowColor)
	redStyle    = lipgloss.NewStyle().Foreground(redColor)
	frameStyle  = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accentColor).
			Padding(0, 1)
)

type tickMsg time.Time

type refreshMsg struct {
	overview application.OverviewResult
}

type refreshErrMsg struct {
	err error
}

type actionDoneMsg struct {
	action  Action
	path    string
	summary string
	err     error
}

type menuState struct {
	items  []Action
	cursor int
	path   string
}

type confirmState struct {
	action Action
	path   string
}

type discoverMsg struct {
	project domain.Project
	result  application.DiscoveryResult
	err     error
}

// Model is the dashboard state.
type Model struct {
	overview    application.OverviewResult
	registry    string
	refresh     RefreshFunc
	actions     ActionRunner
	discover    Discoverer
	table       table.Model
	detail      viewport.Model
	entries     []application.OverviewWorktree
	projects    []projectSummary
	discovered  []application.OverviewWorktree
	level       viewLevel
	project     domain.Project
	width       int
	height      int
	focusDetail bool
	refreshing  bool
	discovering bool
	discoverErr error
	err         error
	lastRefresh time.Time
	detailPath  string
	menu        *menuState
	confirm     *confirmState
	running     string
	runningPath string
	notice      string
	noticeErr   error
}

func New(overview application.OverviewResult, registryPath, focusPath string, refresh RefreshFunc, actions ActionRunner, discover Discoverer) Model {
	m := Model{
		overview:    overview,
		registry:    registryPath,
		refresh:     refresh,
		actions:     actions,
		discover:    discover,
		level:       levelProjects,
		lastRefresh: time.Now(),
		table: table.New(
			table.WithColumns(projectColumns()),
			table.WithRows([]table.Row{}),
			table.WithWidth(116),
			table.WithHeight(15),
			table.WithFocused(true),
			table.WithStyles(tableStyles()),
		),
		detail: viewport.New(viewport.WithWidth(116), viewport.WithHeight(8)),
	}
	m.rebuild()
	if focusPath != "" {
		for _, entry := range m.overview.Worktrees {
			if canonical(entry.Path) == canonical(focusPath) {
				m.level = levelProject
				m.project = entry.Project
				m.rebuild()
				m.selectPath(focusPath)
				break
			}
		}
	}
	m.syncDetail()
	return m
}

func (m Model) Init() tea.Cmd {
	tick := tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
	if m.level == levelProject {
		if discover := m.startDiscovery(); discover != nil {
			return tea.Batch(tick, discover)
		}
	}
	return tick
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		path := m.selectedPath()
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		if m.level == levelProject && path != "" {
			m.selectPath(path)
		}
		m.syncDetail()
		return m, nil
	case tea.KeyPressMsg:
		if m.running != "" {
			return m, nil
		}
		if m.confirm != nil {
			switch msg.String() {
			case "y", "enter":
				pending := m.confirm
				m.confirm = nil
				return m.startAction(pending.action, pending.path)
			case "n", "esc":
				m.confirm = nil
			}
			return m, nil
		}
		if m.menu != nil {
			switch msg.String() {
			case "esc":
				m.menu = nil
			case "j", "down":
				if m.menu.cursor < len(m.menu.items)-1 {
					m.menu.cursor++
				}
			case "k", "up":
				if m.menu.cursor > 0 {
					m.menu.cursor--
				}
			case "enter":
				return m.chooseAction()
			}
			return m, nil
		}
		if m.level == levelProjects {
			switch msg.String() {
			case "enter", "l":
				return m.openProject()
			}
		} else {
			switch msg.String() {
			case "esc", "h":
				m.backToProjects()
				return m, nil
			}
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			cmd := m.startRefresh()
			return m, cmd
		case "tab":
			m.focusDetail = !m.focusDetail
			m.syncDetail()
			return m, nil
		case "a":
			if m.level == levelProject {
				m.openMenu()
			}
			return m, nil
		}
		if m.focusDetail {
			var cmd tea.Cmd
			m.detail, cmd = m.detail.Update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "j":
			m.table.MoveDown(1)
			m.syncDetail()
			return m, nil
		case "k":
			m.table.MoveUp(1)
			m.syncDetail()
			return m, nil
		}
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		m.syncDetail()
		return m, cmd
	case tickMsg:
		cmd := m.startRefresh()
		next := tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
		if cmd == nil {
			return m, next
		}
		return m, tea.Batch(cmd, next)
	case refreshMsg:
		m.refreshing = false
		m.err = nil
		m.lastRefresh = time.Now()
		path := m.selectedPath()
		project := m.project
		if m.level == levelProjects {
			project = m.selectedProject()
		}
		m.overview = msg.overview
		if m.level == levelProject && !hasOverviewProject(msg.overview.Worktrees, m.project) {
			m.level = levelProjects
			m.project = ""
			m.discovered = nil
			m.discoverErr = nil
		}
		m.rebuild()
		if m.level == levelProject {
			if path == "" || !m.selectPath(path) {
				m.table.SetCursor(0)
			}
		} else {
			m.selectProject(project)
		}
		if m.menu != nil && !m.hasPath(m.menu.path) {
			m.menu = nil
		}
		if m.confirm != nil && !m.hasPath(m.confirm.path) {
			m.confirm = nil
		}
		m.syncDetail()
		if m.level == levelProject {
			return m, m.startDiscovery()
		}
		return m, nil
	case refreshErrMsg:
		m.refreshing = false
		if m.noticeErr == nil {
			m.notice = ""
		}
		m.err = msg.err
		return m, nil
	case actionDoneMsg:
		m.running = ""
		m.runningPath = ""
		m.notice = msg.summary
		m.noticeErr = msg.err
		return m, m.startRefresh()
	case discoverMsg:
		m.discovering = false
		if m.level != levelProject || msg.project != m.project {
			return m, nil
		}
		if msg.err != nil {
			m.discoverErr = msg.err
			m.discovered = nil
		} else {
			m.discoverErr = nil
			m.discovered = discoveryRows(msg.result)
			if len(msg.result.Issues) > 0 {
				issue := msg.result.Issues[0]
				m.discoverErr = fmt.Errorf("%s: %s", issue.Code, issue.Message)
			}
		}
		path := m.selectedPath()
		m.rebuild()
		if path == "" || !m.selectPath(path) {
			m.table.SetCursor(0)
		}
		if m.menu != nil && !m.hasPath(m.menu.path) {
			m.menu = nil
		}
		if m.confirm != nil && !m.hasPath(m.confirm.path) {
			m.confirm = nil
		}
		m.syncDetail()
		return m, nil
	}
	return m, nil
}

func (m *Model) selectedProject() domain.Project {
	if len(m.projects) == 0 {
		return ""
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.projects) {
		return ""
	}
	return m.projects[idx].Project
}

func (m *Model) selectProject(project domain.Project) {
	if project == "" {
		return
	}
	for i, summary := range m.projects {
		if summary.Project == project {
			m.table.SetCursor(i)
			return
		}
	}
}

func hasOverviewProject(entries []application.OverviewWorktree, project domain.Project) bool {
	for _, entry := range entries {
		if entry.Project == project {
			return true
		}
	}
	return false
}

func (m *Model) openProject() (tea.Model, tea.Cmd) {
	project := m.selectedProject()
	if project == "" {
		return *m, nil
	}
	m.level = levelProject
	m.project = project
	m.discovered = nil
	m.discoverErr = nil
	m.discovering = false
	m.detailPath = ""
	m.rebuild()
	m.table.SetCursor(0)
	m.syncDetail()
	return *m, m.startDiscovery()
}

func (m *Model) backToProjects() {
	project := m.project
	m.level = levelProjects
	m.project = ""
	m.discovered = nil
	m.discoverErr = nil
	m.discovering = false
	m.detailPath = ""
	m.rebuild()
	m.selectProject(project)
	m.syncDetail()
}

func (m *Model) startDiscovery() tea.Cmd {
	if m.discover == nil || m.level != levelProject || m.discovering {
		return nil
	}
	m.discovering = true
	project := m.project
	discover := m.discover
	return func() tea.Msg {
		result, err := discover.Discover(context.Background(), string(project))
		return discoverMsg{project: project, result: result, err: err}
	}
}

func (m *Model) selectedEntry() (application.OverviewWorktree, bool) {
	if len(m.entries) == 0 {
		return application.OverviewWorktree{}, false
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.entries) {
		return application.OverviewWorktree{}, false
	}
	return m.entries[idx], true
}

func (m *Model) openMenu() {
	if m.actions == nil {
		return
	}
	w, ok := m.selectedEntry()
	if !ok {
		return
	}
	items := actionsFor(w)
	if len(items) == 0 {
		return
	}
	m.menu = &menuState{items: items, path: w.Path}
}

func (m *Model) hasPath(path string) bool {
	target := canonical(path)
	for _, w := range m.entries {
		if canonical(w.Path) == target {
			return true
		}
	}
	return false
}

func (m *Model) chooseAction() (Model, tea.Cmd) {
	if m.menu == nil || m.menu.cursor < 0 || m.menu.cursor >= len(m.menu.items) {
		m.menu = nil
		return *m, nil
	}
	action := m.menu.items[m.menu.cursor]
	path := m.menu.path
	m.menu = nil
	if actionNeedsConfirm(action) {
		m.confirm = &confirmState{action: action, path: path}
		return *m, nil
	}
	return m.startAction(action, path)
}

func (m *Model) startAction(action Action, path string) (Model, tea.Cmd) {
	if path == "" || m.actions == nil {
		return *m, nil
	}
	m.running = actionLabel(action)
	m.runningPath = path
	m.notice = ""
	m.noticeErr = nil
	runner := m.actions
	return *m, func() tea.Msg {
		summary, err := runner.Run(context.Background(), action, path)
		return actionDoneMsg{action: action, path: path, summary: summary, err: err}
	}
}

func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		v := tea.NewView("loading…")
		v.AltScreen = true
		return v
	}
	if m.width < minWidth || m.height < minHeight {
		v := tea.NewView("terminal too small (minimum 80x16)")
		v.AltScreen = true
		return v
	}
	if m.running != "" {
		return m.modalView(m.runningView())
	}
	if m.confirm != nil {
		return m.modalView(m.confirmView())
	}
	if m.menu != nil {
		return m.modalView(m.menuView())
	}
	width := m.contentWidth()
	body := padHeight(lipgloss.NewStyle().Width(width).Render(m.table.View()), m.table.Height()+2)
	if m.level == levelProjects && len(m.projects) == 0 {
		body = padHeight(lipgloss.NewStyle().Width(width).Render(
			mutedStyle.Render("  no projects\n  run portucca ensure in a worktree"),
		), m.table.Height()+2)
	}
	if m.level == levelProject && len(m.entries) == 0 {
		body = padHeight(lipgloss.NewStyle().Width(width).Render(
			mutedStyle.Render("  no claims or main reservations\n  run portucca ensure in a worktree"),
		), m.table.Height()+2)
	}
	inner := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeader(width),
		body,
		m.detail.View(),
		m.footerText(width),
	)
	content := frameStyle.Render(inner)
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) modalView(content string) tea.View {
	width := m.contentWidth()
	if width > 72 {
		width = 72
	}
	if width < 40 {
		width = 40
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(1, 2).
		Width(width).
		Render(content)
	v := tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box))
	v.AltScreen = true
	return v
}

func (m Model) menuView() string {
	lines := []string{accentStyle.Bold(true).Render("actions")}
	if w, ok := m.selectedEntry(); ok {
		lines[0] += mutedStyle.Render(" · " + string(w.Project) + " · " + w.Kind)
		lines = append(lines, mutedStyle.Render(w.Path), "")
	}
	for i, action := range m.menu.items {
		prefix, style := "  ", mutedStyle
		if i == m.menu.cursor {
			prefix, style = "› ", accentStyle.Bold(true)
		}
		lines = append(lines, style.Render(prefix+actionLabel(action))+"  "+mutedStyle.Render(actionDescription(action)))
	}
	lines = append(lines, "", mutedStyle.Render("enter run · esc cancel"))
	return strings.Join(lines, "\n")
}

func (m Model) confirmView() string {
	lines := []string{accentStyle.Bold(true).Render("confirm " + actionLabel(m.confirm.action))}
	if w, ok := m.selectedEntry(); ok {
		lines = append(lines, mutedStyle.Render(w.Path))
	}
	lines = append(lines, "", actionConfirmText(m.confirm.action, m.confirm.path), "",
		mutedStyle.Render("y confirm · n cancel"))
	return strings.Join(lines, "\n")
}

func (m Model) runningView() string {
	return strings.Join([]string{
		accentStyle.Bold(true).Render("running " + m.running),
		mutedStyle.Render(m.runningPath),
		"",
		yellowStyle.Render("wait…"),
	}, "\n")
}

func actionErrText(err error) string {
	if ae, ok := apperr.As(err); ok {
		if ae.Hint != "" {
			return ae.Code + ": " + ae.Message + " — " + ae.Hint
		}
		return ae.Code + ": " + ae.Message
	}
	return err.Error()
}

func (m *Model) startRefresh() tea.Cmd {
	if m.refreshing || m.running != "" || m.refresh == nil {
		return nil
	}
	m.refreshing = true
	fn := m.refresh
	return func() tea.Msg {
		overview, err := fn(context.Background())
		if err != nil {
			return refreshErrMsg{err: err}
		}
		return refreshMsg{overview: overview}
	}
}

func (m *Model) resize() {
	if m.width < minWidth || m.height < minHeight {
		return
	}
	width := m.contentWidth()
	usable := m.height - 5
	tableHeight := usable * 3 / 5
	if tableHeight < 3 {
		tableHeight = 3
	}
	detailHeight := usable - tableHeight
	if detailHeight < 3 {
		detailHeight = 3
		tableHeight = usable - detailHeight
		if tableHeight < 3 {
			tableHeight = 3
		}
	}
	m.table.SetWidth(width)
	m.table.SetHeight(tableHeight)
	if m.level == levelProjects {
		m.table.SetColumns(projectColumns())
	} else {
		m.table.SetColumns(columns(width))
	}
	m.detail.SetWidth(width)
	m.detail.SetHeight(detailHeight)
	m.rebuild()
	m.syncDetail()
}

func (m *Model) rebuild() {
	if m.level == levelProjects {
		m.projects = groupProjects(m.overview.Worktrees)
		m.entries = nil
		m.table.SetRows([]table.Row{})
		m.table.SetColumns(projectColumns())
		rows := make([]table.Row, 0, len(m.projects))
		for _, summary := range m.projects {
			rows = append(rows, table.Row{
				string(summary.Project),
				strconv.Itoa(summary.Worktrees),
				strconv.Itoa(summary.Mains),
				projectStatusText(summary),
				projectPortsText(summary),
			})
		}
		m.table.SetRows(rows)
		if len(rows) == 0 || m.table.Cursor() < 0 || m.table.Cursor() >= len(rows) {
			m.table.SetCursor(0)
		}
		return
	}
	entries := make([]application.OverviewWorktree, 0, len(m.overview.Worktrees)+len(m.discovered))
	for _, w := range m.overview.Worktrees {
		if w.Project == m.project {
			entries = append(entries, w)
		}
	}
	entries = append(entries, m.discovered...)
	m.entries = entries
	m.table.SetRows([]table.Row{})
	m.table.SetColumns(columns(m.contentWidth()))
	pathWidth := 13
	if cols := m.table.Columns(); len(cols) > 1 {
		pathWidth = cols[1].Width
	}
	rows := make([]table.Row, 0, len(entries))
	for _, w := range entries {
		rows = append(rows, table.Row{
			w.Kind,
			pathText(w.Path, pathWidth),
			servicesText(w.Services),
			statusText(w),
		})
	}
	m.table.SetRows(rows)
	if len(rows) == 0 || m.table.Cursor() < 0 || m.table.Cursor() >= len(rows) {
		m.table.SetCursor(0)
	}
}

func (m *Model) selectedPath() string {
	if len(m.entries) == 0 {
		return ""
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.entries) {
		return ""
	}
	return m.entries[idx].Path
}

func (m *Model) selectPath(path string) bool {
	if path == "" {
		return false
	}
	target := canonical(path)
	for i, w := range m.entries {
		if canonical(w.Path) == target {
			m.table.SetCursor(i)
			return true
		}
	}
	return false
}

func (m *Model) syncDetail() {
	if m.level == levelProjects {
		m.syncProjectDetail()
		return
	}
	if len(m.entries) == 0 {
		m.detail.SetContent("")
		return
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.entries) {
		idx = 0
	}
	w := m.entries[idx]
	width := m.contentWidth()
	if w.Path != m.detailPath {
		m.detailPath = w.Path
		m.detail.GotoTop()
	}
	title := "details"
	titleStyle := mutedStyle
	if m.focusDetail {
		title = "▸ details"
		titleStyle = accentStyle.Bold(true)
	}
	lines := []string{
		titleStyle.Render(title) + mutedStyle.Render(" · "+string(w.Project)+" · "+w.Kind),
		mutedStyle.Render(strings.Repeat("─", max(1, width))),
		kv("project", string(w.Project)),
		kv("kind", w.Kind),
		kv("path", w.Path),
		kv("live", boolText(w.Live)),
	}
	if !w.UpdatedAt.IsZero() {
		lines = append(lines, kv("updated", w.UpdatedAt.Format(time.RFC3339)))
	}
	lines = append(lines, "", mutedStyle.Render("services"))
	if len(w.Services) == 0 {
		lines = append(lines, "  (none)")
	}
	for _, svc := range w.Services {
		badge := greenStyle.Render("● in band")
		if !svc.InBand {
			badge = yellowStyle.Render("▲ out of band")
		}
		if svc.MainPort == 0 && svc.StartPort == 0 {
			lines = append(lines, fmt.Sprintf("  %-8s port=%d  %s", svc.Name, svc.Port, mutedStyle.Render("no band")))
			continue
		}
		lines = append(lines, fmt.Sprintf(
			"  %-8s port=%d  main=%d  start=%d  %s",
			svc.Name, svc.Port, svc.MainPort, svc.StartPort, badge,
		))
	}
	lines = append(lines, "", mutedStyle.Render("issues"))
	if len(w.Issues) == 0 {
		lines = append(lines, "  none")
	}
	for _, issue := range w.Issues {
		icon := yellowStyle.Render("▲")
		if isErrorIssue(issue.Code) {
			icon = redStyle.Render("✖")
		}
		lines = append(lines, "  "+icon+" "+accentStyle.Render(issue.Code)+"  "+issue.Message)
	}
	m.detail.SetContent(strings.Join(lines, "\n"))
}

func (m *Model) syncProjectDetail() {
	if len(m.projects) == 0 {
		m.detail.SetContent("")
		return
	}
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(m.projects) {
		idx = 0
	}
	summary := m.projects[idx]
	width := m.contentWidth()
	title := "project details"
	titleStyle := mutedStyle
	if m.focusDetail {
		title = "▸ project details"
		titleStyle = accentStyle.Bold(true)
	}
	lines := []string{
		titleStyle.Render(title) + mutedStyle.Render(" · "+string(summary.Project)),
		mutedStyle.Render(strings.Repeat("─", max(1, width))),
		kv("project", string(summary.Project)),
		kv("worktrees", strconv.Itoa(summary.Worktrees)),
		kv("mains", strconv.Itoa(summary.Mains)),
		kv("ports", projectPortsText(summary)),
		kv("errors", strconv.Itoa(summary.Errors)),
		kv("warnings", strconv.Itoa(summary.Warnings)),
		"",
		mutedStyle.Render("enter open · j/k move · q quit"),
	}
	m.detail.SetContent(strings.Join(lines, "\n"))
}

func projectColumns() []table.Column {
	const projectW, worktreesW, mainsW, statusW, portsW = 16, 11, 7, 10, 12
	return []table.Column{
		{Title: "PROJECT", Width: projectW},
		{Title: "WORKTREES", Width: worktreesW},
		{Title: "MAINS", Width: mainsW},
		{Title: "STATUS", Width: statusW},
		{Title: "PORTS", Width: portsW},
	}
}

func columns(width int) []table.Column {
	const kindW, servicesW, statusW = 9, 22, 10
	pathW := width - kindW - servicesW - statusW - 10
	if pathW < 13 {
		pathW = 13
	}
	if pathW > 80 {
		pathW = 80
	}
	return []table.Column{
		{Title: "KIND", Width: kindW},
		{Title: "PATH", Width: pathW},
		{Title: "SERVICES", Width: servicesW},
		{Title: "STATUS", Width: statusW},
	}
}

func servicesText(services []application.OverviewService) string {
	if len(services) == 0 {
		return "—"
	}
	names := make([]string, 0, len(services))
	for _, svc := range services {
		names = append(names, string(svc.Name)+"="+fmt.Sprint(svc.Port))
	}
	sort.Strings(names)
	return strings.Join(names, " ")
}

func statusText(w application.OverviewWorktree) string {
	errors, warnings := 0, 0
	for _, issue := range w.Issues {
		if isErrorIssue(issue.Code) {
			errors++
		} else {
			warnings++
		}
	}
	switch {
	case errors > 0:
		return redStyle.Render(fmt.Sprintf("✖ %d", errors))
	case warnings > 0:
		return yellowStyle.Render(fmt.Sprintf("▲ %d", warnings))
	default:
		return greenStyle.Render("● ok")
	}
}

func isErrorIssue(code string) bool {
	switch code {
	case "stale_path", "config_invalid", "claim_out_of_band", "main_port_mismatch":
		return true
	default:
		return false
	}
}

func shortPath(path string) string {
	path = homePath(path)
	runes := []rune(path)
	if len(runes) > 42 {
		return "…" + string(runes[len(runes)-41:])
	}
	return path
}

func homePath(path string) string {
	if path == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" &&
		(path == home || strings.HasPrefix(path, home+string(os.PathSeparator))) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func pathText(path string, width int) string {
	path = homePath(path)
	if width <= 1 {
		return "…"
	}
	pathWidth := ansi.StringWidth(path)
	if pathWidth <= width {
		return path
	}
	return ansi.TruncateLeft(path, pathWidth-(width-1), "…")
}

func canonical(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}

func truncateWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, "…")
}

func padHeight(s string, height int) string {
	if height <= 0 {
		return s
	}
	if current := lipgloss.Height(s); current < height {
		return s + strings.Repeat("\n", height-current)
	}
	return s
}

func tableStyles() table.Styles {
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		Foreground(accentColor).
		Bold(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(mutedColor)
	styles.Selected = styles.Selected.
		Background(surfaceColor).
		Foreground(lipgloss.Color("#FFFFFF")).
		Bold(true)
	return styles
}

func (m Model) contentWidth() int {
	width := m.width - 4
	if width < 1 {
		return 1
	}
	return width
}

func (m Model) renderHeader(width int) string {
	projectCount := 0
	worktrees, mains, issues := 0, 0, 0
	if m.level == levelProjects {
		projectCount = len(m.projects)
		for _, w := range m.overview.Worktrees {
			issues += len(w.Issues)
			switch w.Kind {
			case "main":
				mains++
			case "worktree":
				worktrees++
			}
		}
	} else {
		projectCount = 1
		for _, w := range m.entries {
			issues += len(w.Issues)
			switch w.Kind {
			case "main":
				mains++
			case "worktree":
				worktrees++
			}
		}
	}
	counts := fmt.Sprintf("%s · %s · %s · %s",
		countLabel(projectCount, "project", "projects"),
		countLabel(worktrees, "worktree", "worktrees"),
		countLabel(mains, "main", "mains"),
		countLabel(issues, "issue", "issues"),
	)
	title := "portucca tui"
	if m.level == levelProject {
		title += " / " + string(m.project)
	}
	left := accentStyle.Bold(true).Render(title) + "  " + mutedStyle.Render(counts)
	line1 := joinSides(left, mutedStyle.Render("refreshed "+m.lastRefresh.Format("15:04:05")), width)
	status := greenStyle.Render("● live")
	switch {
	case m.discovering:
		status = yellowStyle.Render("● discovering…")
	case m.discoverErr != nil:
		status = redStyle.Render("✖ " + m.discoverErr.Error())
	case m.running != "":
		status = yellowStyle.Render("● " + m.running + "…")
	case m.noticeErr != nil:
		status = redStyle.Render("✖ " + actionErrText(m.noticeErr))
	case m.err != nil:
		status = redStyle.Render("✖ " + m.err.Error())
	case m.notice != "":
		status = greenStyle.Render("● " + m.notice)
	case m.refreshing:
		status = yellowStyle.Render("● refreshing…")
	}
	line2 := joinSides(mutedStyle.Render("registry "+shortPath(m.registry)), status, width)
	return line1 + "\n" + line2
}

func joinSides(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	rightWidth := ansi.StringWidth(right)
	if rightWidth >= width {
		return truncateWidth(right, width)
	}
	left = truncateWidth(left, width-rightWidth-1)
	leftWidth := ansi.StringWidth(left)
	return left + strings.Repeat(" ", width-leftWidth-rightWidth) + right
}

func countLabel(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func (m Model) footerText(width int) string {
	key := accentStyle.Bold(true)
	desc := mutedStyle
	parts := []string{
		key.Render("↑↓ jk") + desc.Render(" move"),
		key.Render("tab") + desc.Render(" details"),
	}
	if m.level == levelProjects {
		parts = append(parts, key.Render("enter")+desc.Render(" open"))
	} else {
		parts = append(parts, key.Render("h/esc")+desc.Render(" back"))
		if m.actions != nil && len(m.entries) > 0 {
			parts = append(parts, key.Render("a")+desc.Render(" actions"))
		}
	}
	parts = append(parts,
		key.Render("r")+desc.Render(" refresh"),
		key.Render("q")+desc.Render(" quit"),
	)
	return truncateWidth(strings.Join(parts, desc.Render("  ·  ")), width)
}

func kv(key, value string) string {
	return mutedStyle.Render(fmt.Sprintf("%-9s", key)) + " " + value
}

func boolText(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// Run starts the Bubble Tea program. It returns when the user quits.
func Run(in io.Reader, out io.Writer, overview application.OverviewResult, registryPath, focusPath string, refresh RefreshFunc, actions ActionRunner, discover Discoverer) error {
	p := tea.NewProgram(New(overview, registryPath, focusPath, refresh, actions, discover), tea.WithInput(in), tea.WithOutput(out))
	_, err := p.Run()
	return err
}
