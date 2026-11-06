package cli

import (
	"fmt"
	"io"
)

const globalFlagsHelp = `Global flags:
  --json                 Print the JSON envelope
  --config <path>        Alternate portucca.json path (must be at the repo root)
  --registry <path>      Alternate claims registry path
  --worktree <path>      Worktree path to operate on (default ".")
  --lock-timeout <dur>   Registry lock timeout (default 5s)
  -h, --help             Show this command's help
  --version              Print version information
`

const generalHelp = `portucca — sticky ports per worktree

Usage:
  portucca <command> [flags]
  portucca help <command>
  portucca <command> --help

Commands:
  init        Create portucca.json for a project
  ensure      Assign or reuse the worktree ports
  setup       Bootstrap the worktree and ensure ports
  reallocate  Move this worktree to new ports
  prune       Remove claims for deleted worktrees
  doctor      Read-only diagnostics
  env         Print resolved ports and env values
  list        List claims
  run         Start the project with the resolved environment
  version     Print build metadata
  help        Show help for a command

Global flags:
  --json                 Print the JSON envelope
  --config <path>        Alternate portucca.json path (must be at the repo root)
  --registry <path>      Alternate claims registry path
  --worktree <path>      Worktree path to operate on (default ".")
  --lock-timeout <dur>   Registry lock timeout (default 5s)
  -h, --help             Show this help
  --version              Print version information

Examples:
  portucca init --project app --service web:5173:5174
  portucca setup
  portucca run -- npm run dev
  portucca help run
`

var commandHelpText = map[string]string{
	"init": `portucca init — create portucca.json

Usage:
  portucca init [--scan] [--project <slug>] [--service <name:main:start>]... [flags]

Flags:
  --scan                 Detect services, ports and env from the repository
  --project <slug>       Project slug (required without --scan)
  --service <spec>       Service as name:mainPort:startPort (repeatable; required without --scan)
  --preset <name>        generic (default), node-vite-nest, or next
  --force                Overwrite an existing portucca.json

What it does:
  Writes portucca.json at the repository root after validating the project,
  services, ports and preset. With --scan, detects signals from package.json,
  .env* and docker-compose.json/compose.json; explicit flags override detected values.
  It does not touch the claims registry.

Examples:
  portucca init --project app --service web:5173:5174
  portucca init --scan
  portucca init --scan --project app --service api:3004:3005

Notes:
  - Without --scan, --project and at least one --service are required.
  - --scan and --preset are mutually exclusive.
  - If the scan cannot determine every required value, it writes nothing and
    reports the gaps with the flags to add.
  - mainPort is reserved for the main checkout.
  - startPort is the first candidate for worktrees; when --scan derives it, it
    is the smallest free port above mainPort.
  - The node-vite-nest preset expects the web+api layout and fills env and
    bootstrap for apps/web, apps/api and .env.
  - The next preset expects one service named web and fills PORT and
    NEXT_PUBLIC_URL in .env, copying .env from the main checkout.
  - A worktree without portucca.json uses the main checkout's config; a
    worktree-local config overrides it. Run init in the main checkout to share
    a local, gitignored config.
`,
	"ensure": `portucca ensure — assign or reuse the worktree ports

Usage:
  portucca ensure [flags]

What it does:
  - loads the registry and prunes claims for deleted worktrees
  - reuses the current claim when it is valid
  - assigns missing services from startPort, skipping live claims, main
    ports and ports occupied by other processes
  - writes the configured env files and updates the registry atomically
  - is idempotent: a second run with no changes reports changed=false

Examples:
  portucca ensure
  portucca ensure --json
  portucca ensure --worktree ../feature

Notes:
  - Run setup instead for a new worktree; setup also bootstraps files.
  - Run reallocate when you want different ports.
`,
	"setup": `portucca setup — prepare a worktree

Usage:
  portucca setup [flags]

What it does:
  1. bootstraps files from the main checkout:
     - copyFromMain copies only when the destination is missing
     - symlinks creates only when the path is missing
  2. runs ensure: ports, env files and registry

Examples:
  portucca setup
  portucca setup --json
  portucca setup --worktree ../feature

Notes:
  - Run this once in a new worktree.
  - It does not install dependencies, run builds or start processes.
  - In the main checkout it returns the reserved mainPort values and writes
    nothing.
`,
	"reallocate": `portucca reallocate — move this worktree to new ports

Usage:
  portucca reallocate [flags]

What it does:
  - releases the current claim and assigns the next free ports in the band
  - updates the env files and the registry

Examples:
  portucca reallocate
  portucca reallocate --json

Notes:
  - This is the only command that changes an existing worktree URL.
  - It fails in the main checkout with main_reserved.
`,
	"prune": `portucca prune — remove claims for deleted worktrees

Usage:
  portucca prune [flags]

What it does:
  - loads the registry and drops claims whose worktree directory no longer
    exists
  - reports the number of removed claims

Examples:
  portucca prune
  portucca prune --json
`,
	"doctor": `portucca doctor — read-only diagnostics

Usage:
  portucca doctor [flags]

What it checks:
  - config validity, registry state, claims, env files, symlinks and conflicts
  - reports ok, errors, warnings and the current worktree state

Examples:
  portucca doctor
  portucca doctor --json
  portucca doctor --worktree ../feature

Notes:
  - Exits 1 when data.ok is false; exits 0 with warnings only.
  - Run it from a worktree for meaningful env checks; the main checkout has no
    env writes.
`,
	"env": `portucca env — print the resolved environment

Usage:
  portucca env [flags]

What it does:
  - prints project, worktree, services and resolved env values
  - is read-only: it does not write env files or the registry

Examples:
  portucca env
  portucca env --json
  portucca env --worktree ../feature

Notes:
  - Requires an existing claim; run ensure or setup first.
`,
	"list": `portucca list — list claims

Usage:
  portucca list [--all | --project <name>] [flags]

Flags:
  --all                 List claims from every project
  --project <name>      List claims for one project

Examples:
  portucca list
  portucca list --all
  portucca list --project app --json

Notes:
  - Defaults to the current project when a config exists.
  - --all and --project are mutually exclusive (exit 2).
`,
	"run": `portucca run — start the project with the resolved environment

Usage:
  portucca run [--service <name>] -- <command> [args...]

Flags:
  --service <name>      Inject only this service's env values

What it does:
  1. runs setup (bootstrap + ensure)
  2. injects the resolved env values into the process
  3. replaces itself with the command (exec); stdout, stderr, signals and the
     exit code pass through

Examples:
  portucca run -- npm run dev
  portucca run --service web -- npm run dev
  portucca run -- sh -c 'python manage.py runserver 0.0.0.0:$PORT'

Notes:
  - The command and args after -- are literal; there is no shell.
  - Without --service, all env targets are injected. If two services define the
    same key with different values, use --service.
  - Use run when the framework reads the port from the process environment
    (Next.js, Vite, Nest).
  - --worktree selects the worktree whose claim and env are used; the command
    runs in the current directory, not in that worktree.
  - --json is not supported by run.
  - It does not install dependencies, build or supervise processes.
`,
	"version": `portucca version — print build metadata

Usage:
  portucca version [--json]

What it does:
  - prints version, commit, OS, architecture and schema version

Examples:
  portucca version
  portucca version --json
`,
	"help": `portucca help — show help

Usage:
  portucca help [command]
  portucca <command> --help

Examples:
  portucca help
  portucca help run
  portucca run --help

Notes:
  - Without a command, prints the general help and command list.
  - An unknown command exits 2.
`,
}

func printGeneralHelp(w io.Writer) {
	fmt.Fprint(w, generalHelp)
}

func printCommandHelp(w io.Writer, cmd string) bool {
	text, ok := commandHelpText[cmd]
	if !ok {
		return false
	}
	fmt.Fprint(w, text)
	if cmd != "help" {
		fmt.Fprint(w, "\n"+globalFlagsHelp)
	}
	return true
}
