package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/buildinfo"
	"github.com/gomezv/portucca/internal/config"
	"github.com/gomezv/portucca/internal/envfile"
	"github.com/gomezv/portucca/internal/netprobe"
	"github.com/gomezv/portucca/internal/platform"
	"github.com/gomezv/portucca/internal/project"
	"github.com/gomezv/portucca/internal/registry"
)

type globalFlags struct {
	json        bool
	config      string
	registry    string
	worktree    string
	lockTimeout time.Duration
}

var errHelp = errors.New("help")

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "portucca — sticky ports per worktree")
		fmt.Fprintln(stdout, "usage: portucca <init|ensure|setup|reallocate|prune|doctor|env|list|version> [flags]")
		return 0
	}
	if args[0] == "version" || args[0] == "--version" {
		data := map[string]any{
			"version": buildinfo.Version,
			"commit":  buildinfo.Commit,
			"os":      runtime.GOOS,
			"arch":    runtime.GOARCH,
		}
		if hasJSON(args[1:]) {
			writeJSON(stdout, "version", data, nil)
		} else {
			fmt.Fprintf(stdout, "portucca %s (%s)\n", buildinfo.Version, buildinfo.Commit)
		}
		return 0
	}
	cmd := args[0]
	data, err := execute(cmd, args[1:])
	if errors.Is(err, errHelp) {
		fmt.Fprintln(stdout, "usage: portucca <init|ensure|setup|reallocate|prune|doctor|env|list|version> [flags]")
		return 0
	}
	if err != nil {
		code := 1
		if ae, ok := apperr.As(err); ok && ae.Code == "usage" {
			code = 2
		}
		if hasJSON(args[1:]) {
			writeJSON(stdout, cmd, nil, err)
		} else {
			writeHuman(stderr, cmd, nil, err)
		}
		return code
	}
	if d, ok := data.(application.DoctorResult); ok && !d.OK {
		if hasJSON(args[1:]) {
			writeJSON(stdout, cmd, data, nil)
		} else {
			writeHuman(stdout, cmd, data, nil)
		}
		return 1
	}
	if hasJSON(args[1:]) {
		writeJSON(stdout, cmd, data, nil)
	} else {
		writeHuman(stdout, cmd, data, nil)
	}
	return 0
}

func hasJSON(args []string) bool {
	for _, a := range args {
		if a == "--json" {
			return true
		}
	}
	return false
}

func execute(cmd string, args []string) (any, error) {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var g globalFlags
	fs.BoolVar(&g.json, "json", false, "")
	fs.StringVar(&g.config, "config", "", "")
	fs.StringVar(&g.registry, "registry", "", "")
	fs.StringVar(&g.worktree, "worktree", ".", "")
	fs.DurationVar(&g.lockTimeout, "lock-timeout", 5*time.Second, "")
	var projectFlag string
	var all bool
	var initProject string
	var initPreset string
	var initForce bool
	var services stringList
	switch cmd {
	case "list":
		fs.BoolVar(&all, "all", false, "")
		fs.StringVar(&projectFlag, "project", "", "")
	case "init":
		fs.StringVar(&initProject, "project", "", "")
		fs.StringVar(&initPreset, "preset", "", "")
		fs.BoolVar(&initForce, "force", false, "")
		fs.Var(&services, "service", "")
	default:
		// ensure, setup, reallocate, prune, doctor, env use only global flags.
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, errHelp
		}
		return nil, apperr.New("usage", err.Error(), "revisa las flags")
	}
	if fs.NArg() > 0 {
		return nil, apperr.New("usage", "unexpected argument: "+fs.Arg(0), "")
	}
	if cmd == "list" && all && projectFlag != "" {
		return nil, apperr.New("usage", "--all and --project are mutually exclusive", "")
	}
	regPath := g.registry
	if regPath == "" {
		p, err := registry.DefaultPath()
		if err != nil {
			return nil, apperr.New("internal", err.Error(), "define HOME o usa --registry")
		}
		regPath = p
	}
	app := &application.App{
		Locator:     project.Detector{},
		Config:      config.Loader{},
		Registry:    registry.Store{Path: regPath},
		Locker:      platform.Locker{},
		Prober:      netprobe.Prober{},
		EnvStore:    envfile.Store{},
		ConfigFile:  g.config,
		LockTimeout: g.lockTimeout,
	}
	ctx := context.Background()
	switch cmd {
	case "ensure":
		return app.Ensure(ctx, g.worktree)
	case "setup":
		return app.Setup(ctx, g.worktree)
	case "reallocate":
		return app.Reallocate(ctx, g.worktree)
	case "prune":
		return app.Prune(ctx)
	case "doctor":
		return app.Doctor(ctx, g.worktree)
	case "env":
		return app.Env(ctx, g.worktree)
	case "list":
		return app.List(ctx, g.worktree, projectFlag, all)
	case "init":
		if initProject == "" || len(services) == 0 {
			return nil, apperr.New("usage", "--project and at least one --service are required", "")
		}
		return app.Init(ctx, application.InitRequest{
			Project: initProject, Services: services, Preset: initPreset, Force: initForce,
			Worktree: g.worktree, ConfigPath: g.config,
		})
	default:
		return nil, apperr.New("usage", "unknown command: "+cmd, "")
	}
}
