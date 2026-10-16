# Cyanotype

Cyanotype generates instruction files for coding agents (CLAUDE.md, AGENTS.md, Cursor and Copilot rules) from a catalog of choices, and backs critical rules with real constraints such as permissions, hooks and config files.

Your choices live in a `cyanotype.yaml` spec in your repo; the agent files are regenerable output.

## Status

**Pre-alpha.** Nothing usable yet: the repository currently contains only the project skeleton. See [docs/DESIGN.md](docs/DESIGN.md) for the design and roadmap.

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
