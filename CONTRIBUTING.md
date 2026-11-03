# Contributing

Thanks for your interest in portucca. Issues and pull requests are welcome.

## Process

1. Open an issue to report a bug or propose a change.
2. Fork the repository and create a branch from `main`.
3. Submit a pull request with tests.
4. Accepted changes ship in the next release.

## Project rules

- Code and identifiers in English; documentation in English.
- Go 1.27 or later; a single module at the repository root.
- Standard library first; no third-party dependencies in v1 unless justified with evidence.
- Linux and macOS in v1; Windows compiles but fails with `unsupported_platform`.
- Fail-fast: a port conflict fails with an actionable message; it is never reassigned silently.
- Conventional commits, with no AI attribution.
- Update `CHANGELOG.md` under `[Unreleased]` for user-visible changes.

## Verification

```bash
go test -race ./...
go vet ./...
staticcheck ./...      # when installed
```

Before opening a pull request, also make sure `gofmt -l .` returns no files.
