package cli

import (
	"context"
	"testing"

	"github.com/gomezv/portucca/internal/application"
)

type fakeTUIInitApp struct {
	inspectPath string
	analyzeReq  application.InitRequest
	applyForce  bool
}

func (f *fakeTUIInitApp) InspectInitPath(_ context.Context, path string) (application.InitPath, error) {
	f.inspectPath = path
	return application.InitPath{Root: path, IsMain: true}, nil
}

func (f *fakeTUIInitApp) AnalyzeInit(_ context.Context, req application.InitRequest) (application.InitProposal, error) {
	f.analyzeReq = req
	return application.InitProposal{Root: req.Worktree}, nil
}

func (f *fakeTUIInitApp) ApplyInit(_ context.Context, p application.InitProposal, opts application.ApplyInitOptions) (application.InitResult, error) {
	f.applyForce = opts.Force
	return application.InitResult{}, nil
}

func TestTUIInitAdapter(t *testing.T) {
	fake := &fakeTUIInitApp{}
	adapter := tuiInit{app: fake}
	if _, err := adapter.Inspect(context.Background(), "/repo"); err != nil {
		t.Fatal(err)
	}
	if fake.inspectPath != "/repo" {
		t.Fatalf("inspect path = %q", fake.inspectPath)
	}
	if _, err := adapter.Analyze(context.Background(), "/repo", []string{"api:1:2"}, []application.EnvBinding{{Service: "api", File: ".env", Key: "API_PORT"}}); err != nil {
		t.Fatal(err)
	}
	if !fake.analyzeReq.Scan || fake.analyzeReq.Worktree != "/repo" || len(fake.analyzeReq.Services) != 1 || len(fake.analyzeReq.Env) != 1 {
		t.Fatalf("analyze req = %+v", fake.analyzeReq)
	}
	if _, err := adapter.Apply(context.Background(), application.InitProposal{}, true); err != nil {
		t.Fatal(err)
	}
	if !fake.applyForce {
		t.Fatal("force not forwarded")
	}
}

func TestNewTUIInitClearsConfigFile(t *testing.T) {
	app := &application.App{ConfigFile: "/custom/portucca.json"}
	adapter := newTUIInit(app).(tuiInit)
	if adapter.app == app {
		t.Fatal("adapter reused the original App")
	}
	if got := adapter.app.(*application.App).ConfigFile; got != "" {
		t.Fatalf("ConfigFile = %q", got)
	}
}

func TestTUIInitOptions(t *testing.T) {
	opts := tuiInitOptions(&application.App{}, "/tmp/wt", true)
	if opts.DefaultPath != "/tmp/wt" || opts.Flow == nil {
		t.Fatalf("opts = %+v", opts)
	}
	opts = tuiInitOptions(&application.App{}, "", false)
	if opts.DefaultPath == "" {
		t.Fatal("default path is empty")
	}
}
