# Contributing

Thanks for your interest in portucca. Issues and pull requests are welcome.

## Process

1. Open an issue to report a bug or propose a change.
2. Fork the repository and create a branch from `main`.
3. Submit a pull request with tests.
4. Accepted changes ship in the next release.

## Project rules

- Code and identifiers in English; documentation and user-facing CLI output in English.
- Go 1.27 or later; a single module at the repository root.
- Standard library first; no third-party dependencies in v1 unless justified with evidence.
- Linux and macOS in v1; Windows compiles but fails with `unsupported_platform`.
- Fail-fast: a port conflict fails with an actionable message; it is never reassigned silently.
- Conventional commits, with no AI attribution.
- Update `CHANGELOG.md` under `[Unreleased]` for user-visible changes.

## Scope

- Portucca does not create worktrees, install dependencies, run builds, or supervise processes.
- `portucca run` only execs an explicit command with the resolved environment.

## Verification

CI runs `staticcheck` v0.8.2, pinned in `.github/workflows/ci.yml`.

```bash
go test -race ./...
go vet ./...
staticcheck ./...      # v0.8.2, pinned in CI
```

Before opening a pull request, also make sure `gofmt -l .` returns no files.
