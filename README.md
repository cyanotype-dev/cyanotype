# Cyanotype

Cyanotype generates instruction files for coding agents (CLAUDE.md, AGENTS.md, Cursor and Copilot rules) from a catalog of choices, and backs critical rules with real constraints such as permissions, hooks and config files.

Your choices live in a `cyanotype.yaml` spec in your repo; the agent files are regenerable output.

## Status

**Pre-alpha.** `cyanotype init` creates a `cyanotype.yaml` from a few questions and a starter preset, and `cyanotype generate` writes AGENTS.md, CLAUDE.md, the Claude Code deny rules and a `.gitignore` block from it; files for other agents and advanced mode are not there yet. See [docs/DESIGN.md](docs/DESIGN.md) for the design and roadmap.

## Usage

In your project folder, run:

```sh
cyanotype init
```

It asks what you are building, which language you know, how much autonomy the agent gets, whether the app is multilingual, the language of the agent's replies, the goal and your coding agents, then writes `cyanotype.yaml` and offers to generate the files. Every question has a flag, so scripts and agents can run it without a terminal; `cyanotype init --help` lists them. It never overwrites an existing `cyanotype.yaml`, which you can also write by hand (the Project spec section of [docs/DESIGN.md](docs/DESIGN.md) has an example).

To write the files after changing the spec, run:

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
