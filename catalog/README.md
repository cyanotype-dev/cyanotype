# Cyanotype catalog

This folder holds the catalog: one YAML file per selectable technology or rule, in `entries/`. Each entry carries the text it contributes to generated instruction files, the reason behind it, and the real constraints it can enforce.

## License

The catalog, including snippets and presets, is dedicated to the public domain under [CC0-1.0](LICENSE). The rest of the repository (the Go code) is licensed under [Apache-2.0](../LICENSE).

Why the split: snippets are copied verbatim into users' instruction files (CLAUDE.md, AGENTS.md and so on). Those files must carry no attribution or license notice, so the text they come from cannot have any license obligations.

By contributing to this folder you agree to release your contribution under CC0-1.0.

## Status

Empty for now. The entry format and its JSON Schema arrive in a later phase; see the catalog section of [docs/DESIGN.md](../docs/DESIGN.md).
