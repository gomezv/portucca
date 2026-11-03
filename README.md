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

## How it works

1. `portucca.json` lives at the repository root and is committed with the project so every
   worktree created from that branch sees it. Portucca never creates commits and never writes
   to the git index. The file declares the project name, services, port ranges, env targets,
   and optional bootstrap files.
2. `portucca init` writes that file. `portucca setup` prepares a worktree: it bootstraps files
   from the main checkout and then runs `ensure`.
3. `ensure` assigns one port per service, stores the claim in a machine-wide registry keyed by
   the worktree path, and writes the values to the configured env files. The claim survives
   restarts and is released on the next `ensure` or `prune` after the worktree directory is
   deleted.
4. The main checkout keeps its reserved `mainPort` values and never receives claims or writes.
5. `doctor`, `env`, and `list` are read-only. The registry lives in
   `$XDG_STATE_HOME/portucca/ports.json` (or `~/.local/state/portucca/ports.json`) and is
   protected by an exclusive `flock`.

## Installation

> Portucca is not released yet. The `install.sh` and module commands below apply from
> `v0.1.0`; until then, build from source.

### From source (current development builds)

Requires Go 1.27 or later.

```bash
git clone https://github.com/gomezv/portucca.git
cd portucca
go install ./cmd/portucca
```

`go install` writes the binary to `$(go env GOBIN)` or `$(go env GOPATH)/bin`. Add that
directory to `PATH`. You can also build a local binary:

```bash
go build -o ~/.local/bin/portucca ./cmd/portucca
```

### Install script (from `v0.1.0`)

```bash
curl -fsSL https://raw.githubusercontent.com/gomezv/portucca/main/install.sh | bash
```

Pin a version:

```bash
curl -fsSL https://raw.githubusercontent.com/gomezv/portucca/main/install.sh | PORTUCCA_VERSION=v0.1.0 bash
```

The script needs `curl`, `tar`, `install`, and either `sha256sum` or `shasum`. It detects
Linux or macOS and `amd64` or `arm64`, verifies the checksum, and installs to
`${PORTUCCA_INSTALL_DIR:-$HOME/.local/bin}`.

### Module install (from `v0.1.0`)

```bash
go install github.com/gomezv/portucca/cmd/portucca@latest
```

### Requirements

Linux or macOS on `amd64` or `arm64`. Windows has no runtime support in v1.

## Quickstart

The first part is a **one-time setup** in the repository. The second part is what you repeat
for each worktree; no commit is involved there.

### One-time setup

From the repository root:

```bash
portucca init --project app --service web:5173:5174
git add portucca.json
git commit -m "chore: add portucca config"
```

`portucca init` writes the file; `git add` and `git commit` are ordinary Git, not portucca
commands. Commit it once so worktrees created from this branch include it. You do not repeat
this for each worktree.

### For each worktree

```bash
git worktree add ../app-feature -b feature
cd ../app-feature
portucca setup
```

No commit is needed here. `portucca` reads `portucca.json` from the worktree root, writes the
env files in the worktree working tree, and never runs `git commit`. Add those env files to
`.gitignore` if your project does not track them.

If the worktree is created from an old branch that does not include the config commit, either
create it from this branch or copy `portucca.json` to its root.

`setup` prints the resolved ports:

```text
project: app
worktree: /path/app-feature (main=false) changed=true
web: 5174 adopted=false
bootstrap: copied=[] symlinked=[] skipped=[]
```

Read the ports as JSON or inspect the registry:

```bash
portucca env --json
portucca list
```

`setup` does not install dependencies or run builds; start your stack with the port that
portucca assigned.

### Node/Vite/Nest projects

`--preset node-vite-nest` fills in the common web+API layout:

```bash
portucca init --project app --preset node-vite-nest \
  --service web:5173:5174 --service api:3004:3005
```

The preset expects the `apps/web` and `apps/api` directories, a `.env` file in the main
checkout, and writes:

- `apps/web/.env.local`: `WEB_PORT` and `API_PROXY_TARGET`
- `apps/api/.env.local`: `PORT`
- `apps/api/.env` as a symlink to the worktree `.env`

If your layout differs, use `--preset generic` and declare the `env` and `bootstrap` entries
yourself, or edit `portucca.json` after `init`.

## Commands

| Command | What it does |
|---|---|
| `init --project <slug> --service <name:main:start>...` | Writes `portucca.json`. Add `--preset` (`generic` or `node-vite-nest`) and `--force` to overwrite. |
| `setup` | Bootstraps the worktree and runs `ensure`. |
| `ensure` | Assigns or reuses the worktree ports, repairs env files, and writes the registry. |
| `reallocate` | Moves the worktree to new ports. This is the only command that changes a URL. |
| `prune` | Removes claims for worktree directories that no longer exist. |
| `doctor` | Read-only diagnostics. Exits `1` when `data.ok` is false. |
| `env` | Prints the resolved ports and env values for the current worktree. |
| `list` | Lists claims. Defaults to the current project; use `--all` or `--project <name>`. |
| `version` | Prints version, commit, OS, architecture, and schema version. |

Global flags: `--json`, `--config <path>`, `--registry <path>`, `--worktree <path>`,
`--lock-timeout <duration>`, `--help`, and `--version`.

Examples:

```bash
portucca ensure --json
portucca reallocate --json
portucca prune
portucca doctor --json
portucca list --all
portucca version --json
```

`--all` and `--project` are mutually exclusive (exit `2`). The project name comes from
`portucca.json`; there is no global `--project` flag outside `list`.

## Configuration

A complete `portucca.json`:

```json
{
  "version": 1,
  "project": "app",
  "services": {
    "web": {
      "mainPort": 5173,
      "startPort": 5174,
      "adoptFrom": { "file": "apps/web/.env.local", "key": "WEB_PORT" }
    },
    "api": {
      "mainPort": 3004,
      "startPort": 3005,
      "adoptFrom": { "file": "apps/api/.env.local", "key": "PORT" }
    }
  },
  "env": [
    { "service": "web", "file": "apps/web/.env.local", "key": "WEB_PORT" },
    { "service": "api", "file": "apps/api/.env.local", "key": "PORT" },
    {
      "service": "api",
      "file": "apps/web/.env.local",
      "key": "API_PROXY_TARGET",
      "template": "http://localhost:{{port}}"
    }
  ],
  "bootstrap": {
    "copyFromMain": [".env"],
    "symlinks": [
      { "path": "apps/api/.env", "target": "../../.env" }
    ]
  }
}
```

- `mainPort` is reserved for the main checkout; `startPort` is the first port a worktree can
  receive.
- `adoptFrom` registers a port that already exists in an env file. Portucca does not bind it
  or restart the stack. It must reference an `env` target of the same service without a
  template.
- `env` writes the assigned port. Without `template`, it writes the number; with a template,
  it replaces `{{port}}`. Portucca preserves other keys and comments and creates new files
  with mode `0600`.
- `bootstrap.copyFromMain` copies a file from the main checkout only when the destination is
  missing. `bootstrap.symlinks` creates a symlink only when the path is missing; a regular
  file or a different symlink fails with `symlink_conflict`. `target` is relative to the
  link's directory.
- Paths are relative to the repository root. Absolute paths and `..` escapes are rejected.
- The file is strict JSON: unknown fields fail with `config_invalid`.

Commit `portucca.json` once; worktrees created from that branch read the same file.

## Troubleshooting

Start with the read-only diagnostic from the worktree you are debugging:

```bash
portucca doctor
portucca doctor --json
```

The main checkout intentionally has no env files, so `doctor` reports missing env targets
there; run it from a worktree for a meaningful result.

- `config_missing`: run `portucca init` at the repository root. In a worktree, it usually
  means the worktree was created from a commit that predates `portucca.json`; create it from
  the config commit or copy the file to the worktree root.
- `duplicate_claim`: another live worktree holds the port. Inspect `portucca list --all`, then
  run `portucca reallocate` in this worktree or free the other one.
- `port_exhausted`: widen the service band (`startPort`) or remove stale claims with
  `portucca prune`.
- `lock_timeout`: another portucca process is holding the registry lock. Retry, or raise
  `--lock-timeout`.
- `env_write`: the target is a symlink or its parent directory is missing. Portucca will not
  write through a symlink; fix the target manually.
- `no_claim`: run `portucca ensure` or `portucca setup` in the worktree first.
- `unsupported_platform`: Windows is not supported at runtime in v1.

## Integration

All commands accept `--json` and return a stable envelope:

```json
{ "schemaVersion": 1, "command": "ensure", "ok": true, "data": {} }
```

Exit codes are `0` (success), `1` (actionable error), and `2` (invalid usage). A tool that
creates worktrees can call `portucca setup --json` and read the ports without parsing
human-readable text.

## Status

The v1 CLI is implemented but not released yet. Until `v0.1.0`, install from source. See
[`CHANGELOG.md`](CHANGELOG.md) for the release history.

## Contributing

Issues and pull requests are welcome. Open an issue for bugs or proposals and read
[`CONTRIBUTING.md`](CONTRIBUTING.md).

## License

MIT. See [`LICENSE`](LICENSE).
