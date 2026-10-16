# Cyanotype catalog

This folder holds the catalog: one YAML file per selectable technology or rule, in `entries/`. Each entry carries the text it contributes to generated instruction files, the reason behind it, and the real constraints it can enforce.

## License

The catalog, including snippets and presets, is dedicated to the public domain under [CC0-1.0](LICENSE). The rest of the repository (the Go code) is licensed under [Apache-2.0](../LICENSE).

Why the split: snippets are copied verbatim into users' instruction files (CLAUDE.md, AGENTS.md and so on). Those files must carry no attribution or license notice, so the text they come from cannot have any license obligations.

By contributing to this folder you agree to release your contribution under CC0-1.0.

## Adding an entry

1. Create `entries/<id>.yaml`. The folder is flat: the file name must equal the `id`, and the category lives only inside the entry.
2. Start the file with this line, so editors that use yaml-language-server (VS Code, Neovim, JetBrains) autocomplete and check the fields against [the schema](../schema/catalog-entry.schema.json):

   ```yaml
   # yaml-language-server: $schema=../../schema/catalog-entry.schema.json
   ```

3. Run `go test ./internal/catalog/`. It loads every entry and reports each problem with its file and field.

The rules the test enforces:

- `id`, `name`, `category`, `select`, `summary`, `snippet.rule` and `snippet.why` are required. Unknown fields are errors.
- `id`, `category` and `topic` are kebab-case; `url` is an absolute `https` URL.
- `requires` and `conflicts` hold plain entry ids that must exist.
- Entries in a `radio` category already exclude each other: do not list them in `conflicts`. Use `conflicts` only for entries of different categories, and declare it on both sides.

The fields and the reasoning behind them are in the catalog section of [docs/DESIGN.md](../docs/DESIGN.md).
