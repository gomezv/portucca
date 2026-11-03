# AGENTS.md — portucca

Rules for working in this repository.

## Code rules

- Code and identifiers in English; documentation and user-facing CLI output in English.
- Go 1.27 or later. The module lives at the repository root (`go.mod`) and produces a single
  binary.
- Standard library first; no third-party dependencies in v1 unless justified with evidence.
- `gofmt`, `go vet`, and `staticcheck` clean before committing.
- Tests: `go test -race ./...`.
- Linux and macOS in v1. Windows compiles with a stub that fails with `unsupported_platform`.
- Fail-fast: a port conflict fails with an actionable message; it is never reassigned silently.
- Conventional commits, with no AI attribution.
- Update `CHANGELOG.md` under `[Unreleased]` for user-visible changes.

## Scope

- Portucca does not create worktrees, install dependencies, run builds, or supervise processes.
- The claims registry is global to the machine; port ranges live in `portucca.json` inside the
  repository.
- The main checkout never receives claims or automatic writes.
