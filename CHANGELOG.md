# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `env[].template` accepts `{{worktree}}`, a stable per-worktree identifier
  (`<project>_<basename>_<hash8>`) for database or schema names. `ensure` and `setup` write
  it, `reallocate` preserves it, `env` and `run` expose it, and `doctor` detects drift. The
  main checkout omits those targets entirely, and portucca still never creates or migrates
  databases.

## [0.7.0] - 2026-11-19

### Added

- `portucca tui` can initialize a project with `i`: it scans a main checkout, fills missing
  `mainPort`s inline, previews the config, and offers the dev-script wrap after writing.
- `init --scan` maps root `.env*` port keys to services by `<SERVICE>_PORT`, then by the
  final `mainPort` value; the TUI init flow auto-binds unique matches and offers a selector
  for ambiguous ones, adding the root env file to `bootstrap.copyFromMain`.

### Fixed

- The TUI and `doctor` now flag a claim whose path exists but is no longer a git worktree
  (`stale_worktree`/`stale_claim`) and point to `portucca release`; they no longer show
  `● ok` for an orphaned path.

## [0.6.0] - 2026-11-13

### Added

- `portucca init --scan` can wrap `scripts.dev` as `portucca run -- <original>` after
  confirmation (`--wrap-script`/`--no-wrap`), so plain `npm run dev` uses the assigned port.
- `portucca tui` can run a prompted command in the selected row and wrap its dev script with
  confirmation.
- `portucca tui` now navigates by project and discovers worktrees that exist in git but have
  no claim, offering `setup` for them.
- `portucca watch` discovers worktrees without claims under registered mains and runs
  `setup` on them; `--once`, `--interval`, and `--retry` control the loop. It also prunes
  claims for worktrees whose directories no longer exist.
- `portucca tui` can run lifecycle actions on the selected row: `ensure`, `setup`,
  `reallocate`, `release`, and `prune`, with confirmation for state-changing operations.

### Changed

- `portucca prune` now also removes the current checkout's main reservation when its config
  file is missing.
- `portucca init --scan` gaps for a missing service port now list the files the scan
  inspected, state that framework defaults are not assumed, and show how to declare the port.

### Fixed

- The TUI no longer repeats config error codes (`config_invalid:`, `config_version:`,
  `config_missing:`) in issue messages.

## [0.5.0] - 2026-11-12

### Added

- `portucca tui` opens a read-only dashboard of every project in the registry: worktrees,
  main reservations, claims, ports, bands, and lightweight issues. It refreshes every 5
  seconds or with `r`, never writes state, and requires an interactive terminal.
- Release artifacts for Linux and macOS on `amd64` and `arm64`.

## [0.4.0] - 2026-11-11

### Added

- `portucca release [--service <name>]` releases the worktree claim, or one service, without
  deleting the worktree or modifying env files.

### Changed

- The registry is now version 2 and records the main checkout's reserved ports (`mains`).
  `init`, `ensure`, `setup`, and `run` in the main checkout publish those reservations, so a
  worktree of another project no longer takes a stopped main's ports. Version 1 registries
  are accepted and upgraded on the next write; run `ensure` once per main checkout after
  upgrading.

### Fixed

- `portucca init --scan --compose` no longer treats a root `package.json` without a port as a
  service when Compose discovers services. `--exclude-service` filters detection only and
  does not reserve the excluded service's published ports.

## [0.3.0] - 2026-11-07

### Added

- `portucca init --scan` reads Compose YAML (`compose.yaml`, `compose.yml`,
  `docker-compose.yaml`, `docker-compose.yml`) in addition to Compose JSON, with the same
  enrichment rules.
- `portucca init --scan --compose` discovers services from Compose files; `--exclude-service`
  skips infrastructure services.

## [0.2.0] - 2026-11-06

### Added

- `portucca init --scan` detects services, ports, env targets and bootstrap files from
  `package.json`, `.env*` and `docker-compose.json`/`compose.json`, and fails with actionable
  gaps when the repository does not provide enough information.

## [0.1.1] - 2026-11-05

### Changed

- `portucca run` now requires `--` immediately before the command; a `--` inside the command
  arguments no longer satisfies the separator.
- `portucca run --worktree` selects the claim and env but does not change the command's
  working directory.

### Fixed

- `portucca run` validates the command before `setup`, so `command_not_found` leaves the
  registry and env files untouched.
- `env_conflict` points to `portucca.json` when the conflict is within one service instead of
  suggesting `--service`.
- `run`/`setup` load `portucca.json` once, and `ensure` renders env values once.

## [0.1.0] - 2026-11-04

### Added

- CLI commands: `init`, `ensure`, `setup`, `run`, `reallocate`, `prune`, `doctor`, `env`,
  `list`, `version`, and `help`.
- Per-command help: `portucca help <command>` and `portucca <command> --help` include the
  description, usage, flags, examples, and notes for each command.
- `next` preset: `portucca init --preset next --service web:3000:3001` writes `PORT` and
  `NEXT_PUBLIC_URL` to `.env` and copies `.env` from the main checkout.
- Shared local config: a worktree without `portucca.json` falls back to the main checkout's
  config, so an unversioned, gitignored config can be shared across worktrees. A
  worktree-local `portucca.json` still takes precedence.
- `portucca run -- <command>` injects the resolved ports and environment into the command
  process, so frameworks that read the port from the process environment need no wrapper.
- Sticky port allocation per worktree, backed by a global claims registry locked with `flock`.
- Declarative bootstrap from the main checkout with file copies and symlinks.
- `adoptFrom` support to register existing ports without binding or restarting the stack.
- Stable JSON envelope (`schemaVersion`, `command`, `ok`, `data`) and deterministic exit codes.
- `install.sh` with OS/architecture detection and SHA-256 checksum verification.
- Release artifacts for Linux and macOS on `amd64` and `arm64`.

[Unreleased]: https://github.com/gomezv/portucca/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/gomezv/portucca/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/gomezv/portucca/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/gomezv/portucca/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/gomezv/portucca/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/gomezv/portucca/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/gomezv/portucca/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/gomezv/portucca/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/gomezv/portucca/releases/tag/v0.1.0
