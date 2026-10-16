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

## Licensing of contributions

- Code is licensed under Apache-2.0.
- Catalog entries in `catalog/` are released under CC0-1.0. By contributing to the catalog you agree to that dedication.
