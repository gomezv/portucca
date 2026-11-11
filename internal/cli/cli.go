package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"runtime"
	"time"

	"github.com/gomezv/portucca/internal/apperr"
	"github.com/gomezv/portucca/internal/application"
	"github.com/gomezv/portucca/internal/buildinfo"
	"github.com/gomezv/portucca/internal/config"
	"github.com/gomezv/portucca/internal/domain"
	"github.com/gomezv/portucca/internal/envfile"
	"github.com/gomezv/portucca/internal/netprobe"
	"github.com/gomezv/portucca/internal/platform"
	"github.com/gomezv/portucca/internal/project"
	"github.com/gomezv/portucca/internal/registry"
	"github.com/gomezv/portucca/internal/scan"
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
	"release":    true,
	"prune":      true,
	"doctor":     true,
	"env":        true,
	"list":       true,
	"run":        true,
	"version":    true,
	"help":       true,
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printGeneralHelp(stdout)
		return 0
	}
	if args[0] == "help" {
		if len(args) > 1 {
			target := args[1]
			if target == "--help" || target == "-h" {
				printCommandHelp(stdout, "help")
				return 0
			}
			if target == "help" || knownCommands[target] {
				printCommandHelp(stdout, target)
				return 0
			}
			writeHuman(stderr, "help", nil, apperr.New("usage", "unknown command: "+target, "run portucca help to list commands"))
			return 2
		}
		printGeneralHelp(stdout)
		return 0
	}
	if args[0] == "--help" || args[0] == "-h" {
		printGeneralHelp(stdout)
		return 0
	}
	cmd := args[0]
	if cmd == "--version" {
		cmd = "version"
	}
	data, jsonMode, err := execute(cmd, args[1:])
	if errors.Is(err, errHelp) {
		if !printCommandHelp(stdout, cmd) {
			printGeneralHelp(stdout)
		}
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

func hasRunSeparator(args, rest []string) bool {
	consumed := len(args) - len(rest)
	return consumed > 0 && args[consumed-1] == "--"
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
	var initScan bool
	var initCompose bool
	var excludeServices stringList
	var services stringList
	var runService string
	var releaseService string
	switch cmd {
	case "list":
		fs.BoolVar(&all, "all", false, "")
		fs.StringVar(&projectFlag, "project", "", "")
	case "run":
		fs.StringVar(&runService, "service", "", "")
	case "release":
		fs.StringVar(&releaseService, "service", "", "")
	case "init":
		fs.StringVar(&initProject, "project", "", "")
		fs.StringVar(&initPreset, "preset", "", "")
		fs.BoolVar(&initForce, "force", false, "")
		fs.BoolVar(&initScan, "scan", false, "")
		fs.BoolVar(&initCompose, "compose", false, "")
		fs.Var(&excludeServices, "exclude-service", "")
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
	if cmd == "run" && !hasRunSeparator(args, fs.Args()) {
		return nil, false, apperr.New("usage", "run: missing -- before the command", "use -- <command> [args...]")
	}
	if !knownCommands[cmd] {
		return nil, g.json, apperr.New("usage", "unknown command: "+cmd, "")
	}
	if cmd == "run" {
		if g.json {
			return nil, false, apperr.New("usage", "run does not support --json", "")
		}
		if fs.NArg() == 0 {
			return nil, false, apperr.New("usage", "run: missing command", "use -- <command> [args...]")
		}
	} else if fs.NArg() > 0 {
		return nil, g.json, apperr.New("usage", "unexpected argument: "+fs.Arg(0), "")
	}
	if cmd == "list" && all && projectFlag != "" {
		return nil, g.json, apperr.New("usage", "--all and --project are mutually exclusive", "")
	}
	if cmd == "init" {
		if initScan && initPreset != "" {
			return nil, g.json, apperr.New("usage", "--scan and --preset are mutually exclusive", "drop one of them")
		}
		if initCompose && !initScan {
			return nil, g.json, apperr.New("usage", "--compose requires --scan", "add --scan or drop --compose")
		}
		if len(excludeServices) > 0 && !initCompose {
			return nil, g.json, apperr.New("usage", "--exclude-service requires --compose", "add --compose or drop --exclude-service")
		}
		if len(excludeServices) > 0 && len(services) > 0 {
			return nil, g.json, apperr.New("usage", "--exclude-service cannot be combined with --service", "drop one of them")
		}
		for _, name := range excludeServices {
			if !domain.ValidServiceName(string(scan.NormalizeServiceName(name))) {
				return nil, g.json, apperr.New("usage", "invalid --exclude-service "+name, "use a lowercase service name")
			}
		}
		if !initScan && (initProject == "" || len(services) == 0) {
			return nil, g.json, apperr.New("usage", "--project and at least one --service are required", "")
		}
		if initScan && initProject != "" && !domain.ValidProject(initProject) {
			return nil, g.json, apperr.New("usage", "invalid project "+initProject, "use a lowercase slug")
		}
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
		Executor:    platform.Executor{},
		Scanner:     scan.Scanner{},
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
	case "release":
		data, err := app.Release(ctx, g.worktree, domain.ServiceName(releaseService))
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
			Scan: initScan, Compose: initCompose, ExcludeServices: excludeServices,
			Worktree: g.worktree, ConfigPath: g.config,
		})
		return data, g.json, err
	case "run":
		err := app.Run(ctx, application.RunRequest{
			Worktree: g.worktree,
			Service:  domain.ServiceName(runService),
			Command:  fs.Args(),
		})
		return nil, g.json, err
	}
	return nil, g.json, apperr.New("usage", "unknown command: "+cmd, "")
}
