# Cyanotype catalog

This folder holds the catalog: one YAML file per selectable technology or rule, in `entries/`, and one per preset, a ready combination of entries, in `presets/`. Each entry carries the text it contributes to generated instruction files, the reason behind it, and the real constraints it can enforce.

## License

The catalog, including snippets and presets, is dedicated to the public domain under [CC0-1.0](LICENSE). The rest of the repository (the Go code) is licensed under [Apache-2.0](../LICENSE).

Why the split: snippets are copied verbatim into users' instruction files (CLAUDE.md, AGENTS.md and so on). Those files must carry no attribution or license notice, so the text they come from cannot have any license obligations.

By contributing to this folder you agree to release your contribution under CC0-1.0.

## Version

`catalog.yaml` holds the catalog's own date-based version, such as `"2026.10"` (quoted, since YAML reads `2026.10` as a number). Projects pin it in `cyanotype.yaml` so regeneration is reproducible.

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
- All entries in a category share the same `select` value: a category is either `radio` or `checkbox`.
- `requires` and `conflicts` hold plain entry ids that must exist.
- `requires_one_of` holds groups of alternatives, such as `[nextjs, vite-react]`: each group needs at least one of its entries selected. A group has at least two ids; a single id belongs in `requires`. No id may be the entry itself, appear twice in a group, or also be listed in `requires` or `conflicts`.
- Entries in a `radio` category already exclude each other: do not list them in `conflicts`. Use `conflicts` only for entries of different categories, and declare it on both sides.
- `enforce.gitignore` lines go into the `.gitignore` block cyanotype generates, in the order written, so a negation such as `!.env.example` comes after the lines it narrows. Each line is one pattern: not blank, without leading or trailing spaces, not starting with `#`, and not listed twice. Add lines only when the snippet promises that something stays out of git.
- `snippet.rule` and `snippet.why` may use the `{languages}` placeholder, filled with the selected languages ("Go and TypeScript"). Any other `{name}` is an error; wrap literal braces in backticks, as in `` `{children}` ``.

The fields and the reasoning behind them are in the catalog section of [docs/DESIGN.md](../docs/DESIGN.md).

## Adding a preset

A preset is a ready combination of entries that simple mode starts a project from, such as "Next.js + Supabase". It holds the stack part of a `cyanotype.yaml` (`selections`, `commands`, `output`) plus metadata; the fields about one project, and `targets`, are chosen when someone uses it.

1. Create `presets/<id>.yaml`. As with entries, the folder is flat and the file name must equal the `id`.
2. Start the file with this line, for editor autocompletion against [the preset schema](../schema/preset.schema.json):

   ```yaml
   # yaml-language-server: $schema=../../schema/preset.schema.json
   ```

3. Run `go test ./internal/preset/ ./catalog/`. Loading resolves the preset against the catalog, so the test reports every problem a spec built from it would have.

The rules the test enforces:

- `id`, `name`, `summary`, `project_type` and `selections` are required; `i18n_selections`, `commands` and `output` are optional. Any other key is an error.
- `project_type` is `website`, `mobile-app`, `api` or `script`.
- `selections` must resolve without errors: known entries, one entry per radio category, every `requires` and `requires_one_of` met.
- `i18n_selections` lists the entries a multilingual project adds after `i18n-rules`, such as the i18n library of the stack. The selections must still resolve with them added.
- Every bundled preset must also generate files without warnings.

The format and the reasoning behind it are in the Presets section of [docs/DESIGN.md](../docs/DESIGN.md).
