# portucca

One stable port per worktree. Sticky, auditable, no `EADDRINUSE`.

Portucca allocates development ports across the worktrees of a git repository (and across
projects sharing the same machine) in a **sticky** way (the same worktree always gets the same
ports) and **fail-fast** (on a conflict, the process fails with an explicit message instead of
silently moving to another port).

## What it solves

| Problem | What portucca does |
|---|---|
| Two worktrees request the same port | Allocates ports per worktree with a global claims registry and locking (`flock`) |
| The port changes on every start | The claim follows the worktree **directory**: if the stack stops and starts again, the URL stays the same |
| A new worktree does not start | Declarative bootstrap: copies files from the main checkout and creates symlinks |
| Ports already exist in env files | `adoptFrom` registers the existing port without binding or tearing down the stack |
| Two projects on the same machine collide | A single registry with claims tagged by project |
| The main checkout receives writes | The main checkout has reserved ports and never enters the registry |

Portucca is language-agnostic: it works with N services per project and assumes neither a
framework nor a specific `.env` layout.

## What it does not solve (v1)

- Databases per worktree
- Process supervision, per-subdomain proxies, or certificates
- Containers / devcontainers
- Worktree creation: `git worktree`, Orca, or another tool creates them; portucca prepares an
  existing worktree
- Dependency installation or builds: `setup` does not run project commands
- TUI and plugins (deferred to v2)

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/gomezv/portucca/main/install.sh | bash
```

The script detects Linux or macOS and the architecture (`amd64` or `arm64`), verifies the
checksum, and installs to `${PORTUCCA_INSTALL_DIR:-$HOME/.local/bin}`. To pin a version:

```bash
curl -fsSL https://raw.githubusercontent.com/gomezv/portucca/main/install.sh | PORTUCCA_VERSION=v0.1.0 bash
```

With Go 1.27 or later you can also install from the module:

```bash
go install github.com/gomezv/portucca/cmd/portucca@latest
```

Requirements: Linux or macOS on `amd64` or `arm64`. Windows has no runtime support in v1.

## Usage

```bash
portucca init --project app --service web:5173:5174 --service api:3004:3005
portucca setup    # prepares the current worktree: bootstrap + ensure
portucca ensure   # allocates or recalls the ports; repairs the environment
portucca env      # prints the resolved values without writing anything
```

`init` writes `portucca.json` at the repository root. The `--service` format is
`name:main_port:first_worktree_port`; `--preset node-vite-nest` fills in the environment
targets for the web+API layout.

A minimal `portucca.json`:

```json
{
  "version": 1,
  "project": "app",
  "services": {
    "web": { "mainPort": 5173, "startPort": 5174 },
    "api": { "mainPort": 3004, "startPort": 3005 }
  },
  "env": [
    { "service": "web", "file": "apps/web/.env.local", "key": "WEB_PORT" },
    { "service": "api", "file": "apps/api/.env.local", "key": "PORT" }
  ]
}
```

After creating a worktree, run `portucca setup` inside it. The other commands are
`reallocate` (explicitly changes the worktree ports), `prune` (removes claims for deleted
worktrees), `doctor` (read-only diagnostics), `list`, and `version`.

## Integration

All commands accept `--json` and return a stable envelope:

```json
{ "schemaVersion": 1, "command": "ensure", "ok": true, "data": {} }
```

Exit codes are `0` (success), `1` (actionable error), and `2` (invalid usage). A tool that
creates worktrees can call `portucca setup --json` and read the ports without parsing
human-readable text.

## Status

The v1 CLI is implemented. The installation described here applies from the first `v0.1.0`
release. See [`CHANGELOG.md`](CHANGELOG.md) for the release history.

## Contributing

Issues and pull requests are welcome. Open an issue for bugs or proposals and read
[`CONTRIBUTING.md`](CONTRIBUTING.md).

## License

MIT. See [`LICENSE`](LICENSE).
