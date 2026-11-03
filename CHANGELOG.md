# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- CLI commands: `init`, `ensure`, `setup`, `run`, `reallocate`, `prune`, `doctor`, `env`,
  `list`, `version`, and `help`.
- Per-command help: `portucca help <command>` and `portucca <command> --help` include the
  description, usage, flags, examples, and notes for each command.
- `next` preset: `portucca init --preset next --service web:3000:3001` writes `PORT` and
  `NEXT_PUBLIC_URL` to `.env` and copies `.env` from the main checkout.
- `portucca run -- <command>` injects the resolved ports and environment into the command
  process, so frameworks that read the port from the process environment need no wrapper.
- Sticky port allocation per worktree, backed by a global claims registry locked with `flock`.
- Declarative bootstrap from the main checkout with file copies and symlinks.
- `adoptFrom` support to register existing ports without binding or restarting the stack.
- Stable JSON envelope (`schemaVersion`, `command`, `ok`, `data`) and deterministic exit codes.
- `install.sh` with OS/architecture detection and SHA-256 checksum verification.
- Release artifacts for Linux and macOS on `amd64` and `arm64`.

[Unreleased]: https://github.com/gomezv/portucca/compare/v0.1.0...HEAD
