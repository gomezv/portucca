package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/domain"
)

type fakeInitFlow struct {
	inspectCalls int
	inspectPath  string
	info         application.InitPath
	inspectErr   error
	analyzeCalls int
	analyzePath  string
	services     []string
	proposal     application.InitProposal
	analyzeErr   error
	applyCalls   int
	force        bool
	result       application.InitResult
	applyErr     error
}

func (f *fakeInitFlow) Inspect(_ context.Context, path string) (application.InitPath, error) {
	f.inspectCalls++
	f.inspectPath = path
	return f.info, f.inspectErr
}

func (f *fakeInitFlow) Analyze(_ context.Context, path string, services []string) (application.InitProposal, error) {
	f.analyzeCalls++
	f.analyzePath = path
	f.services = append([]string{}, services...)
	return f.proposal, f.analyzeErr
}

func (f *fakeInitFlow) Apply(_ context.Context, proposal application.InitProposal, force bool) (application.InitResult, error) {
	f.applyCalls++
	f.force = force
	return f.result, f.applyErr
}

func initModel(t *testing.T, flow InitFlow, def string) Model {
	t.Helper()
	m := New(overviewFixture(), "/tmp/ports.json", "", nil, nil, nil, nil, nil,
		InitOptions{DefaultPath: def, Flow: flow})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return next.(Model)
}

func TestModelInitOpensPath(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", MainRoot: "/repo", IsMain: true}}
	m := initModel(t, flow, "/repo")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = next.(Model)
	if m.init == nil || m.init.stage != initPathStage {
		t.Fatalf("init = %+v", m.init)
	}
	if m.init.input != "/repo" {
		t.Fatalf("input = %q", m.init.input)
	}
}

func TestModelInitNoFlow(t *testing.T) {
	m := initModel(t, nil, "/repo")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if next.(Model).init != nil {
		t.Fatal("init opened without a flow")
	}
}

func TestModelInitInspecting(t *testing.T) {
	flow := &fakeInitFlow{
		info:     application.InitPath{Root: "/repo", MainRoot: "/repo", IsMain: true},
		proposal: application.InitProposal{Root: "/repo", IsMain: true, ConfigPath: "/repo/portucca.json"},
	}
	m := initModel(t, flow, "/repo")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || m.init.stage != initBusyStage {
		t.Fatalf("cmd = %v stage = %v", cmd, m.init.stage)
	}
	msg := cmd()
	if _, ok := msg.(initInspectedMsg); !ok {
		t.Fatalf("msg = %T", msg)
	}
	next, cmd = m.Update(msg)
	m = next.(Model)
	if cmd == nil || flow.inspectCalls != 1 {
		t.Fatalf("cmd = %v inspectCalls = %d", cmd, flow.inspectCalls)
	}
	if _, ok := cmd().(initAnalyzedMsg); !ok {
		t.Fatal("inspect did not trigger analyze")
	}
}

func TestModelInitWorktreeRejected(t *testing.T) {
	flow := &fakeInitFlow{
		info: application.InitPath{Root: "/repo/wt", MainRoot: "/repo", IsMain: false},
	}
	m := initModel(t, flow, "/repo/wt")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.init == nil || m.init.err == nil {
		t.Fatal("worktree did not produce a hint")
	}
	if !strings.Contains(m.init.err.Error(), "/repo") {
		t.Fatalf("err = %v", m.init.err)
	}
	if flow.analyzeCalls != 0 {
		t.Fatalf("analyze called %d times", flow.analyzeCalls)
	}
}

func TestModelInitInspectError(t *testing.T) {
	flow := &fakeInitFlow{inspectErr: apperr.New("not_a_repo", "not a git repository", "run inside a git repository")}
	m := initModel(t, flow, "/repo")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.init == nil || m.init.err == nil {
		t.Fatal("inspect error not shown")
	}
	view := m.View().Content
	for _, want := range []string{"not_a_repo", "run inside a git repository"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if flow.analyzeCalls != 0 {
		t.Fatalf("analyze called %d times", flow.analyzeCalls)
	}
}

func TestModelInitCancel(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", MainRoot: "/repo", IsMain: true}}
	m := initModel(t, flow, "/repo")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = next.(Model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if next.(Model).init != nil {
		t.Fatal("esc did not close init")
	}
	if flow.inspectCalls != 0 || flow.analyzeCalls != 0 || flow.applyCalls != 0 {
		t.Fatalf("cancel called the flow: %+v", flow)
	}
}

func TestModelInitFooter(t *testing.T) {
	m := initModel(t, &fakeInitFlow{}, "/repo")
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "i init") {
		t.Fatalf("footer missing init:\n%s", m.View().Content)
	}
}

func TestModelInitFooterHiddenWithoutFlow(t *testing.T) {
	m := initModel(t, nil, "/repo")
	if view := ansi.Strip(m.View().Content); strings.Contains(view, "i init") {
		t.Fatalf("footer shows init without a flow:\n%s", view)
	}
}

func TestModelInitIgnoredAtProjectLevel(t *testing.T) {
	flow := &fakeInitFlow{}
	m := New(overviewFixture(), "/tmp/ports.json", "/tmp/main", nil, nil, nil, nil, nil,
		InitOptions{DefaultPath: "/repo", Flow: flow})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	if m.level != levelProject {
		t.Fatalf("level = %v, want project", m.level)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if next.(Model).init != nil {
		t.Fatal("i opened init at project level")
	}
	if flow.inspectCalls != 0 || flow.analyzeCalls != 0 {
		t.Fatalf("i called the flow at project level: %+v", flow)
	}
}

func gapProposal() application.InitProposal {
	cfg := domain.Config{
		Version: domain.ConfigVersion, Project: "shoucca",
		Services: map[domain.ServiceName]domain.Service{
			"api":      {},
			"checador": {MainPort: 5414, StartPort: 5415},
			"web":      {MainPort: 5413, StartPort: 5416},
		},
		Env: []domain.EnvTarget{
			{Service: "checador", File: "apps/checador/.env.local", Key: "CHECADOR_PORT"},
			{Service: "web", File: "apps/web/.env.local", Key: "WEB_PORT"},
		},
	}
	return application.InitProposal{
		Root: "/repo", MainRoot: "/repo", IsMain: true, ConfigPath: "/repo/portucca.json",
		Config: cfg, Project: "shoucca", Services: cfg.Services,
		Gaps: []application.InitGap{{
			Field: "services.api.mainPort", Reason: "missing", Message: "no port for api",
			Flags: []string{"--service api:<main>:<start>"},
		}},
	}
}

func completeProposal() application.InitProposal {
	p := gapProposal()
	p.Services["api"] = domain.Service{MainPort: 3414, StartPort: 3415}
	p.Config.Services = p.Services
	p.Gaps = nil
	p.Sources = []application.ScanSource{
		{Field: "services.api.mainPort", Value: "3414", Source: "flag"},
		{Field: "services.web.mainPort", Value: "5413", Source: "apps/web/.env.local:WEB_PORT"},
	}
	return p
}

func initToAnalyzed(t *testing.T, flow *fakeInitFlow) Model {
	t.Helper()
	m := initModel(t, flow, "/repo")
	next, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	next, cmd = m.Update(cmd())
	m = next.(Model)
	next, _ = m.Update(cmd())
	return next.(Model)
}

func TestModelInitGapFill(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", MainRoot: "/repo", IsMain: true}}
	flow.proposal = gapProposal()
	m := initToAnalyzed(t, flow)
	if m.init.stage != initGapStage || len(m.init.gaps) != 1 {
		t.Fatalf("state = %+v", m.init)
	}
	m.init.mainInput = "3414"
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.init.startInput != "3415" {
		t.Fatalf("start prefill = %q", m.init.startInput)
	}
	flow.proposal = completeProposal()
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("gap submit did not analyze")
	}
	want := []string{"api:3414:3415", "checador:5414:5415", "web:5413:5416"}
	msg := cmd()
	if strings.Join(flow.services, ",") != strings.Join(want, ",") {
		t.Fatalf("services = %v", flow.services)
	}
	next, _ = m.Update(msg)
	m = next.(Model)
	if m.init.stage != initPreviewStage {
		t.Fatalf("stage = %v, want preview", m.init.stage)
	}
}

func TestModelInitGapInvalid(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", IsMain: true}}
	flow.proposal = gapProposal()
	m := initToAnalyzed(t, flow)
	m.init.mainInput = "80"
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil || m.init.err == nil || m.init.stage != initGapStage {
		t.Fatalf("invalid port advanced: err=%v stage=%v cmd=%v", m.init.err, m.init.stage, cmd)
	}
}

func TestModelInitGapBlocked(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", IsMain: true}}
	p := gapProposal()
	p.Gaps = []application.InitGap{{Field: "project", Reason: "missing", Message: "no project name", Flags: []string{"--project <slug>"}}}
	flow.proposal = p
	m := initToAnalyzed(t, flow)
	if m.init.stage != initBlockedStage {
		t.Fatalf("stage = %v", m.init.stage)
	}
	view := m.View().Content
	for _, want := range []string{"no project name", "--project"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestModelInitMixedGap(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", MainRoot: "/repo", IsMain: true}}
	p := gapProposal()
	p.Gaps = []application.InitGap{
		{Field: "services.api.mainPort", Reason: "missing", Message: "no port for api", Flags: []string{"--service api:<main>:<start>"}},
		{Field: "services.web.mainPort", Reason: "ambiguous", Message: "two ports", Flags: []string{"--service web:<main>:<start>"}},
	}
	flow.proposal = p
	m := initToAnalyzed(t, flow)
	if m.init.stage != initBlockedStage {
		t.Fatalf("stage = %v, want blocked", m.init.stage)
	}
	view := m.View().Content
	for _, want := range []string{"two ports", "--service web"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if flow.analyzeCalls != 1 {
		t.Fatalf("analyzeCalls = %d, want 1", flow.analyzeCalls)
	}
}

func TestModelInitPreview(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", IsMain: true}}
	flow.proposal = completeProposal()
	m := initToAnalyzed(t, flow)
	view := m.View().Content
	for _, want := range []string{"shoucca", "api 3414 3415 flag", "WEB_PORT", "apps/web/.env.local"} {
		if !strings.Contains(view, want) {
			t.Fatalf("preview missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "CHECADOR_PORT=") {
		t.Fatal("preview printed env values")
	}
}

func TestModelInitPreviewTruncates(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", IsMain: true}}
	p := completeProposal()
	for i := 0; i < 7; i++ {
		name := domain.ServiceName(fmt.Sprintf("svc%02d", i))
		p.Services[name] = domain.Service{MainPort: domain.Port(6000 + i*2), StartPort: domain.Port(6001 + i*2)}
	}
	p.Config.Services = p.Services
	flow.proposal = p
	m := initToAnalyzed(t, flow)
	if !strings.Contains(m.View().Content, "+2 more services") {
		t.Fatalf("preview missing truncation:\n%s", m.View().Content)
	}
}

func TestModelInitOverwritePreview(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", IsMain: true}}
	p := completeProposal()
	p.ConfigExists = true
	flow.proposal = p
	m := initToAnalyzed(t, flow)
	if !strings.Contains(m.View().Content, "overwrite") {
		t.Fatalf("preview missing overwrite warning:\n%s", m.View().Content)
	}
}

func runMessages(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			if c != nil {
				out = append(out, c())
			}
		}
		return out
	}
	return []tea.Msg{msg}
}

func previewModel(t *testing.T, flow *fakeInitFlow) Model {
	t.Helper()
	m := initToAnalyzed(t, flow)
	if m.init.stage != initPreviewStage {
		t.Fatalf("stage = %v", m.init.stage)
	}
	return m
}

func TestModelInitApply(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", IsMain: true}}
	flow.proposal = completeProposal()
	flow.result = application.InitResult{Project: "shoucca", Services: flow.proposal.Services}
	m := previewModel(t, flow)
	m.refresh = func(context.Context) (application.OverviewResult, error) { return overviewFixture(), nil }

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("apply did not start")
	}
	var applied initAppliedMsg
	for _, msg := range runMessages(t, cmd) {
		if a, ok := msg.(initAppliedMsg); ok {
			applied = a
		}
	}
	if applied.proposal.Project != "shoucca" {
		t.Fatalf("applied = %+v", applied)
	}
	next, cmd = m.Update(applied)
	m = next.(Model)
	if flow.applyCalls != 1 || flow.force {
		t.Fatalf("applyCalls = %d force = %v", flow.applyCalls, flow.force)
	}
	if m.init != nil {
		t.Fatal("init flow still open after apply")
	}
	if !strings.Contains(m.notice, "init: project shoucca") {
		t.Fatalf("notice = %q", m.notice)
	}
	refreshed := false
	for _, msg := range runMessages(t, cmd) {
		if _, ok := msg.(refreshMsg); ok {
			refreshed = true
		}
	}
	if !refreshed {
		t.Fatal("apply did not trigger a refresh")
	}
}

func TestModelInitForce(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", IsMain: true}}
	p := completeProposal()
	p.ConfigExists = true
	flow.proposal = p
	flow.result = application.InitResult{Project: "shoucca", Services: p.Services}
	m := previewModel(t, flow)
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(Model)
	var applied initAppliedMsg
	for _, msg := range runMessages(t, cmd) {
		if a, ok := msg.(initAppliedMsg); ok {
			applied = a
		}
	}
	_, _ = m.Update(applied)
	if flow.applyCalls != 1 || !flow.force {
		t.Fatalf("applyCalls = %d force = %v", flow.applyCalls, flow.force)
	}
}

func TestModelInitApplyError(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", IsMain: true}}
	flow.proposal = completeProposal()
	flow.applyErr = apperr.New("lock_timeout", "lock is busy", "retry")
	m := previewModel(t, flow)
	m.refresh = func(context.Context) (application.OverviewResult, error) { return overviewFixture(), nil }
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(Model)
	var applied initAppliedMsg
	for _, msg := range runMessages(t, cmd) {
		if a, ok := msg.(initAppliedMsg); ok {
			applied = a
		}
	}
	next, cmd = m.Update(applied)
	m = next.(Model)
	if m.init != nil || m.noticeErr == nil {
		t.Fatalf("init=%v noticeErr=%v", m.init, m.noticeErr)
	}
	view := m.View().Content
	for _, want := range []string{"lock_timeout", "retry"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if cmd == nil {
		t.Fatal("apply error did not refresh")
	}
}

func TestModelInitWrap(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", IsMain: true}}
	flow.proposal = completeProposal()
	flow.result = application.InitResult{Project: "shoucca", Services: flow.proposal.Services}
	wrapper := &fakeWrapper{change: application.DevScriptChange{
		Path: "/repo/package.json", Before: "next dev", After: "portucca run -- next dev", Changed: true,
	}}
	m := previewModel(t, flow)
	m.wrapper = wrapper
	m.refresh = func(context.Context) (application.OverviewResult, error) { return overviewFixture(), nil }

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(Model)
	var applied initAppliedMsg
	for _, msg := range runMessages(t, cmd) {
		if a, ok := msg.(initAppliedMsg); ok {
			applied = a
		}
	}
	next, cmd = m.Update(applied)
	m = next.(Model)
	var preview initWrapPreviewMsg
	for _, msg := range runMessages(t, cmd) {
		if p, ok := msg.(initWrapPreviewMsg); ok {
			preview = p
		}
	}
	next, _ = m.Update(preview)
	m = next.(Model)
	if m.wrapChange == nil || !strings.Contains(m.View().Content, "portucca run -- next dev") {
		t.Fatalf("wrap preview missing: %+v", m.wrapChange)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.notice != "wrap: dev script updated" {
		t.Fatalf("notice = %q", m.notice)
	}
	if len(wrapper.calls) < 2 || wrapper.calls[len(wrapper.calls)-1] != "apply" {
		t.Fatalf("wrapper calls = %v", wrapper.calls)
	}
}

func TestModelInitWrapCancel(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", IsMain: true}}
	flow.proposal = completeProposal()
	flow.result = application.InitResult{Project: "shoucca", Services: flow.proposal.Services}
	wrapper := &fakeWrapper{change: application.DevScriptChange{Changed: true, Before: "next dev", After: "portucca run -- next dev"}}
	m := previewModel(t, flow)
	m.wrapper = wrapper
	m.refresh = func(context.Context) (application.OverviewResult, error) { return overviewFixture(), nil }
	m.init = nil
	m.notice = "init: project shoucca services=3"
	m.wrapChange = &application.DevScriptChange{Changed: true, Before: "next dev", After: "portucca run -- next dev"}
	m.wrapPath = "/repo"
	next, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = next.(Model)
	if m.wrapChange != nil {
		t.Fatal("wrap modal still open")
	}
	if m.notice != "init: project shoucca services=3" {
		t.Fatalf("notice = %q", m.notice)
	}
	if len(wrapper.calls) != 0 {
		t.Fatalf("wrapper calls = %v", wrapper.calls)
	}
}

func TestModelInitWrapUnchanged(t *testing.T) {
	flow := &fakeInitFlow{info: application.InitPath{Root: "/repo", IsMain: true}}
	flow.proposal = completeProposal()
	flow.result = application.InitResult{Project: "shoucca", Services: flow.proposal.Services}
	wrapper := &fakeWrapper{change: application.DevScriptChange{Changed: false}}
	m := previewModel(t, flow)
	m.wrapper = wrapper
	m.notice = "init: project shoucca services=3"
	m.running = "wrap dev"
	next, _ := m.Update(initWrapPreviewMsg{path: "/repo", change: application.DevScriptChange{Changed: false}})
	m = next.(Model)
	if m.wrapChange != nil || m.running != "" {
		t.Fatalf("wrapChange=%v running=%q", m.wrapChange, m.running)
	}
	if m.notice != "init: project shoucca services=3" {
		t.Fatalf("notice = %q", m.notice)
	}
}
