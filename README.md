# Cyanotype

Cyanotype generates instruction files for coding agents (CLAUDE.md, AGENTS.md, Cursor and Copilot rules) from a catalog of choices, and backs critical rules with real constraints such as permissions, hooks and config files.

Your choices live in a `cyanotype.yaml` spec in your repo; the agent files are regenerable output.

## Status

**Pre-alpha.** `cyanotype generate` writes AGENTS.md, CLAUDE.md, the Claude Code deny rules and a `.gitignore` block from a hand-written `cyanotype.yaml`; other agents, `init` and simple mode are not there yet. See [docs/DESIGN.md](docs/DESIGN.md) for the design and roadmap.

## Usage

Write a `cyanotype.yaml` at the root of your project (the Project spec section of [docs/DESIGN.md](docs/DESIGN.md) has an example), then run:

```sh
cyanotype generate
```

It shows the changes as a diff and asks before writing. `--dry-run` only shows them, `--yes` writes without asking, and `--dir` points to another project folder. Without a terminal, as in CI, it writes nothing unless `--yes` is given, and exits with code 3 when files are out of date.

## Building

Requires the Go version pinned in [go.mod](go.mod).

```sh
go build ./...
go test ./...
go run ./cmd/cyanotype version
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

- Code: [Apache-2.0](LICENSE).
- Catalog (`catalog/`): [CC0-1.0](catalog/LICENSE), so generated instruction files carry no license obligations. See [catalog/README.md](catalog/README.md).
