# Contributing to Cyanotype

Cyanotype is pre-alpha and not yet accepting outside contributions. This guide will grow once the catalog format is stable.

## Before you start

Read [docs/DESIGN.md](docs/DESIGN.md); it is the source of truth for how things should work.

## Workflow

- One branch per change, merged through a pull request. No direct commits to `main`.
- Use [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `chore:`, `docs:`, `test:`) and keep commits small.
- New Go dependencies need approval first: Cyanotype ships as a single binary and every dependency adds supply-chain risk.

## Checks

All three must pass before a pull request is merged:

```sh
go build ./...
go test ./...
golangci-lint run
```

CI pins golangci-lint v2.15.0; install the same version locally:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
```

CI also runs govulncheck v1.8.0, which fails on a known vulnerability that our code calls, in a dependency or in the Go standard library. Fix it by updating the module, or the Go version in `go.mod`. To run it locally:

```sh
go install golang.org/x/vuln/cmd/govulncheck@v1.8.0
govulncheck ./...
```

## Licensing of contributions

- Code is licensed under Apache-2.0.
- Catalog entries in `catalog/` are released under CC0-1.0. By contributing to the catalog you agree to that dedication.
