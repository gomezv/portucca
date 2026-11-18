package tui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/domain"
)

type InitFlow interface {
	Inspect(ctx context.Context, path string) (application.InitPath, error)
	Analyze(ctx context.Context, path string, services []string) (application.InitProposal, error)
	Apply(ctx context.Context, proposal application.InitProposal, force bool) (application.InitResult, error)
}

type InitOptions struct {
	DefaultPath string
	Flow        InitFlow
}

type initStage int

const (
	initPathStage initStage = iota
	initBusyStage
	initGapStage
	initPreviewStage
	initBlockedStage
)

type initPortGap struct {
	name string
}

type initPorts struct {
	main  int
	start int
}

type initState struct {
	stage      initStage
	path       string
	input      string
	proposal   application.InitProposal
	gaps       []initPortGap
	gapIndex   int
	mainInput  string
	startInput string
	filled     map[string]initPorts
	err        error
}

type initInspectedMsg struct {
	path string
	info application.InitPath
	err  error
}

type initAnalyzedMsg struct {
	path     string
	services []string
	proposal application.InitProposal
	err      error
}

type initAppliedMsg struct {
	proposal application.InitProposal
	force    bool
	result   application.InitResult
	err      error
}

type initWrapPreviewMsg struct {
	path   string
	change application.DevScriptChange
	err    error
}

func (m *Model) startInit() (Model, tea.Cmd) {
	if m.initFlow == nil {
		return *m, nil
	}
	m.init = &initState{stage: initPathStage, path: m.initDefault, input: m.initDefault}
	m.notice = ""
	m.noticeErr = nil
	return *m, nil
}

func (m *Model) inspectInit(path string) tea.Cmd {
	flow := m.initFlow
	return func() tea.Msg {
		info, err := flow.Inspect(context.Background(), path)
		return initInspectedMsg{path: path, info: info, err: err}
	}
}

func (m *Model) analyzeInit(services []string) tea.Cmd {
	flow, path := m.initFlow, m.init.path
	return func() tea.Msg {
		proposal, err := flow.Analyze(context.Background(), path, services)
		return initAnalyzedMsg{path: path, services: services, proposal: proposal, err: err}
	}
}

func (m *Model) startInitApply() tea.Cmd {
	s := m.init
	if s == nil {
		return nil
	}
	proposal := s.proposal
	force := proposal.ConfigExists
	s.stage = initBusyStage
	flow := m.initFlow
	return func() tea.Msg {
		result, err := flow.Apply(context.Background(), proposal, force)
		return initAppliedMsg{proposal: proposal, force: force, result: result, err: err}
	}
}

func (m *Model) startInitWrapPreview(path string) tea.Cmd {
	if m.wrapper == nil {
		return nil
	}
	m.running = "wrap dev"
	m.runningPath = path
	wrapper := m.wrapper
	return func() tea.Msg {
		change, err := wrapper.Preview(context.Background(), path)
		return initWrapPreviewMsg{path: path, change: change, err: err}
	}
}

func (m *Model) updateInit(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := m.init
	if s == nil {
		return *m, nil
	}
	if s.stage == initBusyStage {
		return *m, nil
	}
	if msg.String() == "esc" {
		m.init = nil
		return *m, nil
	}
	switch s.stage {
	case initPathStage:
		return m.updateInitPath(msg)
	case initGapStage:
		return m.updateInitGap(msg)
	case initPreviewStage:
		switch msg.String() {
		case "y", "enter":
			return *m, m.startInitApply()
		case "n":
			m.init = nil
		}
		return *m, nil
	}
	return *m, nil
}

func (m *Model) updateInitPath(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := m.init
	if msg.String() == "enter" {
		path := strings.TrimSpace(s.input)
		if path == "" {
			s.err = fmt.Errorf("path is required")
			return *m, nil
		}
		s.path = path
		s.err = nil
		s.stage = initBusyStage
		return *m, m.inspectInit(path)
	}
	s.input = editInitInput(s.input, msg)
	return *m, nil
}

func (m *Model) updateInitGap(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := m.init
	if msg.String() == "enter" {
		return m.submitInitGap()
	}
	if s.startInput == "" {
		s.mainInput = editInitInput(s.mainInput, msg)
	} else {
		s.startInput = editInitInput(s.startInput, msg)
	}
	return *m, nil
}

func (m *Model) submitInitGap() (tea.Model, tea.Cmd) {
	s := m.init
	if s.gapIndex >= len(s.gaps) {
		return m.analyzeInitGaps()
	}
	if s.startInput == "" {
		main, err := parseInitPort(s.mainInput)
		if err != nil {
			s.err = err
			return *m, nil
		}
		s.startInput = strconv.Itoa(main + 1)
		s.err = nil
		return *m, nil
	}
	main, err := parseInitPort(s.mainInput)
	if err != nil {
		s.err = err
		return *m, nil
	}
	start, err := parseInitPort(s.startInput)
	if err != nil {
		s.err = err
		return *m, nil
	}
	if start <= main {
		s.err = fmt.Errorf("start port must be greater than main port")
		return *m, nil
	}
	if s.filled == nil {
		s.filled = map[string]initPorts{}
	}
	s.filled[s.gaps[s.gapIndex].name] = initPorts{main: main, start: start}
	s.gapIndex++
	s.mainInput = ""
	s.startInput = ""
	s.err = nil
	if s.gapIndex < len(s.gaps) {
		return *m, nil
	}
	return m.analyzeInitGaps()
}

func (m *Model) analyzeInitGaps() (tea.Model, tea.Cmd) {
	s := m.init
	s.err = nil
	s.stage = initBusyStage
	return *m, m.analyzeInit(initServiceSpecs(s.proposal, s.filled))
}

func (m *Model) handleInitAnalyzed(msg initAnalyzedMsg) {
	s := m.init
	if s == nil {
		return
	}
	if msg.err != nil {
		s.err = msg.err
		if len(msg.services) == 0 {
			s.stage = initPathStage
			return
		}
		s.stage = initGapStage
		if s.gapIndex >= len(s.gaps) && len(s.gaps) > 0 {
			s.gapIndex = len(s.gaps) - 1
			if filled, ok := s.filled[s.gaps[s.gapIndex].name]; ok {
				s.mainInput = strconv.Itoa(filled.main)
				s.startInput = strconv.Itoa(filled.start)
			}
		}
		return
	}
	s.proposal = msg.proposal
	s.err = nil
	fill, blocked := fillableInitGaps(msg.proposal)
	s.gaps = nil
	if len(blocked) > 0 {
		s.stage = initBlockedStage
		return
	}
	if len(fill) > 0 {
		s.gaps = fill
		s.gapIndex = 0
		s.filled = map[string]initPorts{}
		s.mainInput = ""
		s.startInput = ""
		s.stage = initGapStage
		return
	}
	s.stage = initPreviewStage
}

func fillableInitGaps(p application.InitProposal) ([]initPortGap, []application.InitGap) {
	var fill []initPortGap
	var blockedGaps []application.InitGap
	for _, g := range p.Gaps {
		name, ok := initPortGapName(g)
		if !ok {
			blockedGaps = append(blockedGaps, g)
			continue
		}
		if _, exists := p.Services[domain.ServiceName(name)]; !exists {
			blockedGaps = append(blockedGaps, g)
			continue
		}
		fill = append(fill, initPortGap{name: name})
	}
	return fill, blockedGaps
}

func initPortGapName(g application.InitGap) (string, bool) {
	const prefix, suffix = "services.", ".mainPort"
	if g.Reason != "missing" || !strings.HasPrefix(g.Field, prefix) || !strings.HasSuffix(g.Field, suffix) {
		return "", false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(g.Field, prefix), suffix)
	return name, name != ""
}

func initServiceSpecs(p application.InitProposal, filled map[string]initPorts) []string {
	names := make([]string, 0, len(p.Services))
	for name := range p.Services {
		names = append(names, string(name))
	}
	sort.Strings(names)
	specs := make([]string, 0, len(names))
	for _, name := range names {
		svc := p.Services[domain.ServiceName(name)]
		ports := initPorts{main: int(svc.MainPort), start: int(svc.StartPort)}
		if f, ok := filled[name]; ok {
			ports = f
		}
		specs = append(specs, fmt.Sprintf("%s:%d:%d", name, ports.main, ports.start))
	}
	return specs
}

func parseInitPort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < domain.MinPort || port > domain.MaxPort {
		return 0, fmt.Errorf("port must be between %d and %d", domain.MinPort, domain.MaxPort)
	}
	return port, nil
}

func editInitInput(input string, msg tea.KeyPressMsg) string {
	if msg.String() == "backspace" {
		if input == "" {
			return ""
		}
		r := []rune(input)
		return string(r[:len(r)-1])
	}
	if msg.Text != "" {
		return input + msg.Text
	}
	return input
}

const (
	initPreviewMaxServices = 8
	initPreviewMaxEnv      = 4
	initPreviewMaxWarnings = 2
)

func initServiceSource(p application.InitProposal, name domain.ServiceName) string {
	field := "services." + string(name) + ".mainPort"
	for _, s := range p.Sources {
		if s.Field == field && s.Source != "" {
			return s.Source
		}
	}
	return "proposal"
}

func (m Model) initView() string {
	s := m.init
	if s == nil {
		return ""
	}
	switch s.stage {
	case initBusyStage:
		return m.initBusyView()
	case initGapStage:
		return m.initGapView()
	case initBlockedStage:
		return m.initBlockedView()
	case initPreviewStage:
		return m.initPreviewView()
	}
	lines := []string{accentStyle.Bold(true).Render("init project"), mutedStyle.Render(s.path), "", "> " + s.input + "_", ""}
	if s.err != nil {
		lines = append(lines, redStyle.Render(actionErrText(s.err)), "")
	}
	lines = append(lines, mutedStyle.Render("enter inspect · esc cancel"))
	return strings.Join(lines, "\n")
}

func (m Model) initBusyView() string {
	s := m.init
	return strings.Join([]string{
		accentStyle.Bold(true).Render("init project"),
		mutedStyle.Render(s.path),
		"",
		yellowStyle.Render("wait…"),
	}, "\n")
}

func (m Model) initGapView() string {
	s := m.init
	input, field := s.mainInput, "mainPort"
	if s.startInput != "" {
		input, field = s.startInput, "startPort"
	}
	lines := []string{accentStyle.Bold(true).Render("init ports"), mutedStyle.Render(s.path), ""}
	if s.gapIndex < len(s.gaps) {
		lines = append(lines, fmt.Sprintf("%s for %s", field, s.gaps[s.gapIndex].name), "")
	}
	lines = append(lines, "> "+input+"_", "")
	if s.err != nil {
		lines = append(lines, redStyle.Render(actionErrText(s.err)), "")
	}
	lines = append(lines, mutedStyle.Render("enter next · esc cancel"))
	return strings.Join(lines, "\n")
}

func (m Model) initBlockedView() string {
	s := m.init
	lines := []string{accentStyle.Bold(true).Render("init blocked"), mutedStyle.Render(s.path), ""}
	for _, gap := range s.proposal.Gaps {
		lines = append(lines, redStyle.Render(gap.Message))
		if len(gap.Flags) > 0 {
			lines = append(lines, "  "+greenStyle.Render(strings.Join(gap.Flags, " · ")))
		}
	}
	lines = append(lines, "", mutedStyle.Render("esc cancel"))
	return strings.Join(lines, "\n")
}

func (m Model) initPreviewView() string {
	s := m.init
	p := s.proposal
	lines := []string{
		accentStyle.Bold(true).Render("init preview"),
		kv("project", string(p.Project)),
		kv("config", p.ConfigPath),
		kv("main", boolText(p.IsMain)),
		"",
		mutedStyle.Render("services"),
	}
	names := make([]string, 0, len(p.Services))
	for name := range p.Services {
		names = append(names, string(name))
	}
	sort.Strings(names)
	for i, name := range names {
		if i == initPreviewMaxServices {
			lines = append(lines, mutedStyle.Render(fmt.Sprintf("+%d more services", len(names)-initPreviewMaxServices)))
			break
		}
		svc := p.Services[domain.ServiceName(name)]
		lines = append(lines, fmt.Sprintf("  %s %d %d %s", name, svc.MainPort, svc.StartPort, initServiceSource(p, domain.ServiceName(name))))
	}
	lines = append(lines, "", mutedStyle.Render("env"))
	for i, target := range p.Config.Env {
		if i == initPreviewMaxEnv {
			lines = append(lines, mutedStyle.Render(fmt.Sprintf("+%d more env targets", len(p.Config.Env)-initPreviewMaxEnv)))
			break
		}
		lines = append(lines, fmt.Sprintf("  env: %s:%s", target.File, target.Key))
	}
	lines = append(lines, "", fmt.Sprintf("bootstrap: %d files", len(p.Config.Bootstrap.CopyFromMain)+len(p.Config.Bootstrap.Symlinks)))
	if len(p.Warnings) > 0 {
		lines = append(lines, "")
		for i, warning := range p.Warnings {
			if i == initPreviewMaxWarnings {
				lines = append(lines, yellowStyle.Render(fmt.Sprintf("+%d more warnings", len(p.Warnings)-initPreviewMaxWarnings)))
				break
			}
			lines = append(lines, yellowStyle.Render("▲ "+warning))
		}
	}
	if p.ConfigExists {
		lines = append(lines, "", redStyle.Render("overwrite existing portucca.json (force)"))
	}
	lines = append(lines, "", mutedStyle.Render("y apply · n cancel"))
	return strings.Join(lines, "\n")
}
