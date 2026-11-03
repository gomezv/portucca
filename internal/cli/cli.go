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

var knownCommands = map[string]bool{
	"init":       true,
	"ensure":     true,
	"setup":      true,
	"reallocate": true,
	"prune":      true,
	"doctor":     true,
	"env":        true,
	"list":       true,
	"version":    true,
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsage(stdout)
		return 0
	}
	cmd := args[0]
	if cmd == "--version" {
		cmd = "version"
	}
	data, jsonMode, err := execute(cmd, args[1:])
	if errors.Is(err, errHelp) {
		printUsage(stdout)
		return 0
	}
	if err != nil {
		code := 1
		if ae, ok := apperr.As(err); ok && ae.Code == "usage" {
			code = 2
		}
		if jsonMode {
			writeJSON(stdout, cmd, nil, err)
		} else {
			writeHuman(stderr, cmd, nil, err)
		}
		return code
	}
	if d, ok := data.(application.DoctorResult); ok && !d.OK {
		if jsonMode {
			writeJSON(stdout, cmd, data, nil)
		} else {
			writeHuman(stdout, cmd, data, nil)
		}
		return 1
	}
	if jsonMode {
		writeJSON(stdout, cmd, data, nil)
	} else {
		writeHuman(stdout, cmd, data, nil)
	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "portucca — sticky ports per worktree")
	fmt.Fprintln(w, "usage: portucca <init|ensure|setup|reallocate|prune|doctor|env|list|version> [flags]")
}

func execute(cmd string, args []string) (any, bool, error) {
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
	case "version":
		var versionFlag bool
		fs.BoolVar(&versionFlag, "version", false, "")
	default:
		// ensure, setup, reallocate, prune, doctor, env use only global flags.
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, g.json, errHelp
		}
		return nil, g.json, apperr.New("usage", err.Error(), "check the flags")
	}
	if !knownCommands[cmd] {
		return nil, g.json, apperr.New("usage", "unknown command: "+cmd, "")
	}
	if fs.NArg() > 0 {
		return nil, g.json, apperr.New("usage", "unexpected argument: "+fs.Arg(0), "")
	}
	if cmd == "list" && all && projectFlag != "" {
		return nil, g.json, apperr.New("usage", "--all and --project are mutually exclusive", "")
	}
	if cmd == "init" && (initProject == "" || len(services) == 0) {
		return nil, g.json, apperr.New("usage", "--project and at least one --service are required", "")
	}
	if err := platform.Check(); err != nil {
		return nil, g.json, apperr.New("unsupported_platform", err.Error(), "this platform is not supported yet")
	}
	if cmd == "version" {
		return versionInfo{
			Version: buildinfo.Version,
			Commit:  buildinfo.Commit,
			OS:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		}, g.json, nil
	}
	regPath := g.registry
	if regPath == "" {
		p, err := registry.DefaultPath()
		if err != nil {
			return nil, g.json, apperr.New("internal", err.Error(), "set HOME or use --registry")
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
		data, err := app.Ensure(ctx, g.worktree)
		return data, g.json, err
	case "setup":
		data, err := app.Setup(ctx, g.worktree)
		return data, g.json, err
	case "reallocate":
		data, err := app.Reallocate(ctx, g.worktree)
		return data, g.json, err
	case "prune":
		data, err := app.Prune(ctx)
		return data, g.json, err
	case "doctor":
		data, err := app.Doctor(ctx, g.worktree)
		return data, g.json, err
	case "env":
		data, err := app.Env(ctx, g.worktree)
		return data, g.json, err
	case "list":
		data, err := app.List(ctx, g.worktree, projectFlag, all)
		return data, g.json, err
	case "init":
		data, err := app.Init(ctx, application.InitRequest{
			Project: initProject, Services: services, Preset: initPreset, Force: initForce,
			Worktree: g.worktree, ConfigPath: g.config,
		})
		return data, g.json, err
	}
	return nil, g.json, apperr.New("usage", "unknown command: "+cmd, "")
}
