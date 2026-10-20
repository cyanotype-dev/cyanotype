# Cyanotype — Design doc v0.1

Oct 16, 2026 · @Omar Della Mazza

## Vision and goals

Cyanotype generates instruction files for coding agents — CLAUDE.md, AGENTS.md, GEMINI.md, Cursor and Copilot rules — from choices made with checkboxes, radio buttons and a few open questions. The generated files follow one standard structure and are always written in English, as are the tool's interface, documentation and codebase.

It serves two audiences: developers who want a solid file in two minutes, and vibe coders who don't know the stacks and need sensible defaults chosen for them.

Guiding principles:

- **Short beats complete.** A short, concrete file gets followed; a long, generic one gets ignored.
- **Every rule has a reason.** Rules that explain why are followed more reliably than bare commands.
- **Critical rules become real constraints.** Where possible, the text is backed by permissions, hooks and config that enforce it.
- **One source of truth.** Choices live in a spec file in the repo; agent files are regenerable output.
- **The catalog belongs to the community.** Adding a framework or a rule takes a pull request to a YAML file, not a code change.
- **Usable by humans and by agents.** Every feature is reachable from the GUI, the terminal and the MCP server.

## Architecture

All logic lives in one shared core; CLI, TUI, desktop app and MCP server are thin layers on top, so every feature is written once and appears everywhere.

&#91;embedded content: architecture · 4 interfaces, 4 core modules\]

The core reads the catalog and the spec files, resolves compatibility and conflicts, and generates both instruction files and real constraints. It also writes `cyanotype.yaml` back when choices change, and compares generated files with the project and writes them safely, so every interface shows the same diff and writes the same way; only asking for confirmation belongs to the interfaces.

Stack: Go for the core, [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the TUI, [Wails](https://wails.io) for the desktop app with an HTML and TypeScript interface over the Go core. The result is one executable per operating system with no runtime to install.

### Platforms

Linux, macOS and Windows are first-class targets, each built and tested in CI on every release.

| Platform | CLI, TUI and MCP server | Desktop app |
| --- | --- | --- |
| Linux (x86-64, ARM64) | Static binary, `.deb`, `.rpm`, Homebrew | AppImage, `.deb`, `.rpm`, Flathub |
| macOS (Intel, Apple Silicon) | Homebrew, static binary | Signed `.app` in a `.dmg` |
| Windows (x86-64, ARM64) | Scoop, winget, `.exe` | Installer |

On Linux the desktop app depends on WebKitGTK, which the packages declare as a dependency. The CLI, TUI and MCP server have no system dependencies, so they also run on servers and in containers.

## The catalog

The catalog is a folder of YAML files, one per entry: each selectable technology or rule carries its own text for the generated file, its reason, the link for the "i" icon and its real constraints. The tool's code holds no knowledge about stacks; it all lives here.

| Field | Required | Purpose |
| --- | --- | --- |
| `id`, `name` | yes | Stable identifier and display name |
| `category` | yes | Group it appears in (language, framework, database, i18n, agent workflow…) |
| `select` | yes | `radio` if exclusive within its group, `checkbox` if cumulative |
| `url` | for technologies | Target of the "i" icon |
| `summary` | yes | One-line tooltip |
| `requires`, `conflicts` | no | Compatibility: disables incompatible entries and says why. Plain entry ids |
| `requires_one_of` | no | Groups of alternatives: each group needs at least one of its entries selected |
| `level` | no | Lets simple mode prefer beginner-friendly entries |
| `topic` | no | Which split file the snippet goes to when output is split |
| `snippet.rule`, `snippet.why` | yes | The rule and its reason |
| `enforce` | no | Real constraints to generate: Claude Code permission rules in `enforce.claude.deny` and `enforce.claude.ask`, and in `enforce.claude.with` when they depend on another entry, `.gitignore` lines in `enforce.gitignore`; hooks and config files later |
| `detect` | no | Files or dependencies that reveal the entry in an existing project, and for package managers how they run a script; see Detection |

A technology entry:

```yaml
id: pnpm
name: pnpm
category: package-manager
select: radio
url: https://pnpm.io
summary: Fast, disk-efficient package manager for Node.js
requires: [node]
level: beginner
topic: tooling
snippet:
  rule: Use pnpm for every install and script. Never run npm or yarn.
  why: Mixing package managers creates conflicting lockfiles.
enforce:
  claude:
    deny: ["Bash(npm *)", "Bash(yarn *)", "PowerShell(npm *)", "PowerShell(yarn *)"]
detect:
  files: [pnpm-lock.yaml]
  run_script: pnpm {script}
```

`requires` means "all of these". When an entry works with any one of several others, `requires_one_of` lists groups of alternatives, and each group is satisfied when at least one of its entries is selected. shadcn/ui needs Tailwind CSS and either Next.js or Vite + React:

```yaml
id: shadcn-ui
requires: [tailwind]
requires_one_of:
  - [nextjs, vite-react]
```

A group has at least two alternatives, since a single id belongs in `requires`. No alternative may be the entry itself, appear twice, or also be listed in `requires` or `conflicts`. The field is separate from `requires` rather than a list nested inside it, so `requires` stays a plain list of ids and each field has one type.

References in `requires`, `requires_one_of` and `conflicts` are plain entry ids, never qualified by category. Ids are unique across the whole catalog, so a qualifier adds nothing, and moving an entry to another category must not break the entries that point to it.

All entries in a category share the same `select` value, because the interface draws a category either as radio buttons or as checkboxes; validation rejects a mix. Until category metadata exists, this is how a category's kind is defined.

Exclusivity inside a `radio` category is implicit: selecting pnpm deselects npm without either entry saying so. `conflicts` is only for entries in different categories, such as a framework that does not work with a given database, and it must be declared on both sides. Listing an entry of the same radio category in `conflicts` is a validation error. Languages are `checkbox`, because many projects use more than one.

An agent behavior entry:

```yaml
id: no-test-tampering
name: Never weaken tests to make them pass
category: agent-safety
select: checkbox
summary: The agent may not edit or delete tests to make them pass
topic: testing
snippet:
  rule: Never edit, skip or delete an existing test to make it pass. Fix the code, or stop and ask.
  why: A passing test suite is only meaningful if the tests stay honest.
```

Learning mode is for users who want to learn the project's languages while the agent works. It comes in three levels in the `learning` category, which is `radio`: the user picks at most one, and selecting none turns learning mode off.

| Entry | The agent | The user |
| --- | --- | --- |
| `learning-explain` | Writes all the code and explains each new construct the first time it uses it | Reads and asks |
| `learning-guided` | Explains, works in small steps, writes the complex parts | Writes the simple parts; the agent reviews |
| `learning-hands-on` | Describes what to write and gives hints instead of solutions | Writes most of the code; the agent reviews |

```yaml
id: learning-guided
name: "Learning mode: guided"
category: learning
select: radio
summary: The agent teaches as it works and lets the user write the simple parts
topic: workflow
snippet:
  rule: |
    The user is learning {languages}. The first time you use a construct or idiom, explain it in one or two sentences.
    Work in small steps instead of writing whole modules at once.
    For simple parts, describe what to write and let the user write it, then review their code.
  why: The user wants to understand the code, not just receive it.
```

`{languages}` is filled with the `name` of every selected entry in the `language` category, joined as an English list: "Go", "Go and TypeScript", "Go, Python and TypeScript", in the order they appear in the spec's `selections`. Learning mode is slower by design, so simple mode suggests it but never turns it on by default.

`{languages}` is the only placeholder. Any other `{name}` in `snippet.rule` or `snippet.why` is a validation error, so a typo never reaches a generated file; text inside backticks is code and stays literal, so `` `{children}` `` in a JSX example is allowed. Placeholders are filled by `internal/resolve`, and selecting an entry whose placeholder the selection cannot fill, such as a learning mode with no language selected, is an error.

A GitHub Actions check validates every pull request to the catalog (schema, consistent `requires` and `conflicts`) and periodically verifies that every `url` still responds.

The catalog, including snippets and presets, is released under CC0-1.0, separately from the Apache-2.0 code. Snippets are copied into users' instruction files, and those files must carry no attribution or license notice.

### Detection

`detect` tells `internal/detect` what reveals an entry in an existing project. Finding any one of its files or dependencies is enough:

```yaml
# catalog/entries/postgres.yaml
detect:
  dependencies:
    npm: [pg, postgres]
    pypi: [psycopg, psycopg2, psycopg2-binary, asyncpg]
    go: [github.com/jackc/pgx, github.com/lib/pq]
```

| Field | Read from | Matches |
| --- | --- | --- |
| `files` | The project folder | A file or folder at that path, relative to the root |
| `dependencies.npm` | `package.json`: `dependencies`, `devDependencies`, `optionalDependencies`, `peerDependencies` | The package name exactly |
| `dependencies.pypi` | `pyproject.toml` and `requirements.txt` | The project name normalized as PEP 503 says: lowercase, one hyphen for every run of `-`, `_` and `.`; the catalog writes names in that form, which validation checks |
| `dependencies.go` | The `require` directives of `go.mod`, except `// indirect` ones | The module path, or one of its major versions and subpaths: `github.com/jackc/pgx` matches `github.com/jackc/pgx/v5` |
| `run_script` | | For package managers: how the tool runs a `package.json` script, with `{script}` for its name, such as `npm run {script}` |

The names live in the catalog and the code only knows how to read each manifest, so detecting a new stack needs no code change. Ecosystems are fields rather than map keys, so a misspelled or unsupported one is an unknown field instead of a list nothing ever reads. `run_script` exists because commands differ between tools that run the same scripts: `npm run dev`, `pnpm dev`.

In `pyproject.toml` detection reads `[project]` `dependencies`, every list of `[project.optional-dependencies]` and of `[dependency-groups]`, `[tool.uv]` `dev-dependencies`, and the keys of Poetry's dependency tables except `python`. A small in-house reader handles the TOML these tables use (tables, dotted and quoted keys, strings, arrays, inline tables, comments) and reads past every other value; the standard library has no TOML parser, and a full one would be a dependency for a few lists of strings. Only the files at the project root are read, so a monorepo with a manifest per package is not covered yet. A manifest that cannot be read is a warning, and detection goes on with the rest.

In the MVP, frameworks, libraries, databases, test runners and linters list their packages; Tailwind CSS 4 has no config file, so its package is the only sign. `go-stdlib` has no `detect`, since the standard library is the absence of a framework, and rule entries are detected only where a file reveals them, such as a commitlint config for `conventional-commits`.

### Distribution

The catalog ships inside every release and is updated only on explicit request, never automatically. Its `enforce` field generates hooks and config that run in users' projects, so a compromised catalog pushed automatically would reach every project within hours.

1. **Bundled snapshot.** Each release embeds a catalog copy, so the tool works immediately and offline.
2. **Versioned, signed releases.** The catalog has its own date-based versions, such as `2026.10`, published as signed GitHub releases with checksums.
3. **Explicit updates.** `cyanotype catalog update` downloads a newer version, verifies its signature and shows what changes before switching. The tool may notify about a new version at most once a day and never applies it on its own; notifications can be turned off for CI and offline use.
4. **Pinned per project.** `cyanotype.yaml` records the catalog version it was generated with, so regeneration is reproducible. `cyanotype update` moves a project to a newer version with a diff, and blueprints rely on the same pin.
5. **Additional catalogs.** Organizations can add their own catalog as a second source next to the official one, for internal rules and private presets.

Phase 1 ships the bundled snapshot only; signed updates and additional catalogs arrive after v0.6, the last release of Phase 2 (see Roadmap).

The catalog's version lives in its own data, in `catalog/catalog.yaml`, rather than in the code, because a catalog downloaded on its own must carry its version with it:

```yaml
version: "2026.10"
```

The entries stay in `catalog/entries/`. When a spec's `catalog_version` differs from the loaded catalog:

| Pin | Result | Why |
| --- | --- | --- |
| Missing | The loaded catalog is used | The spec has not pinned one yet |
| Same as the catalog | Nothing to report | |
| Older than the catalog | Warning: entries may have changed since the files were generated | The bundled catalog is the only one available; refusing it would make every spec unusable after an upgrade |
| Newer than the catalog | Error: upgrade cyanotype | The spec may select entries this catalog does not have, as with a newer spec `version` |

An unknown selection under a different pin says so, since the entry may have been renamed or removed. From Phase 2 the tool loads the pinned version when it has it, and this table applies only when it does not.

### MVP entries

The first release ships 41 entries: one solid choice per simple-mode project type, plus every agent safety and workflow rule, which work with any stack and carry the most value. Simple mode covers website, mobile app, API and script; desktop app and game appear as coming later.

| Category | Entries |
| --- | --- |
| Languages | TypeScript, Python, Go |
| Runtimes and package managers | Node.js, pnpm, npm, uv |
| Frameworks | Next.js, Vite + React, Expo, FastAPI, Go standard library |
| UI | Tailwind CSS, shadcn/ui |
| Databases | PostgreSQL, SQLite, Supabase |
| Testing | Vitest, Playwright, pytest |
| Lint and format | ESLint with Prettier, Ruff, golangci-lint |
| Internationalization | Generic i18n rules, next-intl, react-i18next |
| Git | Conventional Commits, one branch per task |
| Agent safety | No destructive commands, no test tampering, secrets only in `.env`, no unrequested changes, new dependencies only with approval, no duplicate files |
| Agent workflow | Plan before code, verifiable definition of done, small commits, rule maintenance |
| Learning | Learning mode: explain, guided, hands-on |

The rows above are groups for reading; the entries use these categories, and the `select` value says whether a project can pick more than one entry of a category:

| Category | Select | Entries | Why |
| --- | --- | --- | --- |
| `language` | checkbox | `typescript`, `python`, `go` | Many projects use more than one language |
| `runtime` | radio | `node` | |
| `package-manager` | radio | `npm`, `pnpm` | One lockfile per Node.js project |
| `python-package-manager` | radio | `uv` | Separate from Node.js package managers, so pnpm and uv can coexist |
| `frontend-framework` | radio | `nextjs`, `vite-react` | |
| `mobile-framework` | radio | `expo` | Next.js for the web and Expo for mobile are often combined, even in one repo |
| `backend-framework` | radio | `fastapi`, `go-stdlib` | Separate from the frontend, so Next.js + FastAPI works |
| `styling` | radio | `tailwind` | Styling approaches exclude each other |
| `ui-components` | radio | `shadcn-ui` | Component libraries exclude each other |
| `database` | radio | `postgres`, `sqlite`, `supabase` | One primary database keeps the rules unambiguous, and Supabase is itself Postgres; a cache or queue will get its own category |
| `testing` | checkbox | `vitest`, `playwright`, `pytest` | Unit and end-to-end tests combine |
| `lint` | checkbox | `eslint-prettier`, `ruff`, `golangci-lint` | One linter per language |
| `i18n` | checkbox | `i18n-rules` | Generic rules combine with any library |
| `i18n-library` | radio | `i18n-next-intl`, `i18n-react-i18next` | Libraries exclude each other |
| `git` | checkbox | `conventional-commits`, `branch-per-task` | |
| `agent-safety` | checkbox | `no-destructive-commands`, `no-test-tampering`, `secrets-in-env`, `no-unrequested-changes`, `dependency-approval`, `no-duplicate-files` | |
| `agent-workflow` | checkbox | `plan-before-code`, `definition-of-done`, `small-commits` | |
| `rule-maintenance` | radio | `rule-maintenance-propose` | The policies in Keeping rules current; selecting none means Off |
| `learning` | radio | `learning-explain`, `learning-guided`, `learning-hands-on` | Selecting none turns learning mode off |

`node` requires one of `npm` or `pnpm`, so every JavaScript project declares its package manager and generated commands are unambiguous. `i18n-next-intl` does not require `i18n-rules`, which a preset can add. `i18n-react-i18next` requires one of `vite-react` or `expo`, since react-i18next serves React on the web and React Native alike, so both React presets share one library entry.

`i18n-rules` tells the agent to route strings through "the i18n library's translation function" without naming one, so it works with any library entry. When none is selected, as in a spec written by hand or a preset without `i18n_selections`, the same rule tells the agent to propose a library that fits the stack and ask before adding it, rather than picking one silently. This is the per-entry form of the "Not decided yet" choice in Open questions, which needs category metadata to be offered for every radio category.

Seven starter presets are built from these entries, at least one per simple-mode project type, so that every framework entry is used by some preset (see Presets):

| Preset | Project type | Selections |
| --- | --- | --- |
| `nextjs-supabase`: Next.js + Supabase | website | `typescript`, `node`, `pnpm`, `nextjs`, `tailwind`, `shadcn-ui`, `supabase`, `vitest`, `eslint-prettier`; `i18n-next-intl` when multilingual |
| `vite-react-app`: Vite + React | website | `typescript`, `node`, `pnpm`, `vite-react`, `tailwind`, `shadcn-ui`, `vitest`, `eslint-prettier`; `i18n-react-i18next` when multilingual |
| `expo-app`: Expo app | mobile-app | `typescript`, `node`, `npm`, `expo`, `eslint-prettier`; `i18n-react-i18next` when multilingual |
| `fastapi-postgres`: FastAPI + PostgreSQL | api | `python`, `uv`, `fastapi`, `postgres`, `pytest`, `ruff` |
| `go-api`: Go API + PostgreSQL | api | `go`, `go-stdlib`, `postgres`, `golangci-lint` |
| `python-script`: Python script with uv | script | `python`, `uv`, `pytest`, `ruff` |
| `go-cli`: Go command-line tool | script | `go`, `golangci-lint` |

`go-api` gives someone who knows Go an API stack in their language, and is the preset that uses `go-stdlib`. Expo uses npm, the package manager of the Expo documentation, and has no test runner yet: Vitest fits React Native poorly, and Jest is not in the catalog. Playwright is in no preset, since it is an `intermediate` entry and starter presets aim at beginners.

Deferred, and good first contributions once the format is proven: Vue, Svelte, Angular, Django, Rails, Rust, Flutter, Java and Kotlin, MongoDB, desktop and game stacks.

The quality of each snippet and its reason matters more than the number of entries.

### Presets

A preset is a ready combination of entries, such as "Next.js + Supabase", that simple mode starts a project from. It is the stack part of a `cyanotype.yaml`, with the same keys and meaning, plus metadata of its own. It cannot be a whole spec: `project.name` and `project.goal` are required, and a preset cannot know them.

```yaml
# yaml-language-server: $schema=../../schema/preset.schema.json
id: nextjs-supabase
name: Next.js + Supabase
summary: Full-stack web app with a Postgres database, auth and storage from Supabase
project_type: website
selections: [typescript, node, pnpm, nextjs, tailwind, shadcn-ui, supabase, vitest, eslint-prettier]
i18n_selections: [i18n-next-intl]
commands:
  dev: pnpm dev
  build: pnpm build
  test: pnpm test
  lint: pnpm lint
```

| Field | Required | Purpose |
| --- | --- | --- |
| `id`, `name` | yes | Stable identifier, equal to the file name, and display name |
| `summary` | yes | One line saying what the preset is good for |
| `project_type` | yes | `website`, `mobile-app`, `api` or `script`: simple mode offers the presets of the type the user picks |
| `selections` | yes | Entry ids, in the order they go into the spec |
| `i18n_selections` | no | Entries added after `i18n-rules` when the app is multilingual or will be, such as the i18n library that fits the stack |
| `commands`, `output` | no | As in the spec |

Presets live in `catalog/presets/<id>.yaml`, one flat folder next to `catalog/entries/`, and are embedded, released and licensed with the catalog, so a preset always ships with the entries it selects. Every other spec key is an unknown field. `targets` is left out on purpose: which agents someone uses is a personal choice rather than part of the stack (see Team rules), and simple mode always asks for it. Presets shared by organizations may bring it back as an optional field. `project_type` and `i18n_selections` keep stack knowledge in the catalog rather than in code, so a community preset needs no code change. The preset's languages, for `{languages}` and for offering learning mode, are its selections of category `language`.

Loading a preset resolves it against its catalog twice, as it is and with `i18n-rules` and its `i18n_selections` added: a preset that would give a spec with errors never loads, and its problems name the file and the field.

## Project spec

Each project has an `cyanotype.yaml` at its root holding the user's choices, never the generated text. CLI, TUI, desktop app and MCP server all read and write it the same way, so work can start in one interface and continue in another.

```yaml
version: 1

catalog_version: "2026.10"

targets: [claude, agents-md, cursor]

mode: advanced

project:
  name: recipe-box
  goal: Web app where families save, organize and share recipes.
  users: Non-technical family members, mostly on mobile.
  core_features:
    - Save recipes with photos and ingredients
    - Share a recipe with family or publicly
  out_of_scope:
    - Meal planning and shopping lists (later)
  brief: docs/PRODUCT.md

languages:
  code: english
  comments: english
  docs: english
  commits: english
  agent_replies: italian

commands:
  dev: pnpm dev
  build: pnpm build
  test: pnpm test
  lint: pnpm lint

selections:
  - typescript
  - nextjs
  - node
  - pnpm
  - postgres
  - vitest
  - i18n-next-intl
  - conventional-commits
  - no-test-tampering
  - no-destructive-commands

locked: [no-test-tampering, no-destructive-commands]

i18n:
  default_locale: en
  locales: [en, it, es]
  translations_by: agent-draft

answers:
  domain_notes: Private recipes never appear in search results.

custom_rules:
  - rule: Store all quantities in metric units; convert only for display.
    why: Keeps scaling and conversion logic in one place.

output:
  split: auto
  max_lines: 100
```

### Fields

| Field | Required | Default | Rules |
| --- | --- | --- | --- |
| `version` | yes | — | Spec format version, see Versioning |
| `catalog_version` | no | Bundled catalog | Date-based, `YYYY.MM` or `YYYY.MM.N`; quoted, since YAML reads `2026.10` as a number |
| `targets` | yes | — | At least one of `claude`, `agents-md`, `gemini`, `cursor`, `copilot` |
| `mode` | no | `advanced` | `simple` or `advanced`. A file without a mode was written by hand, and opening it in simple mode could replace its selections |
| `project.name`, `project.goal` | yes | — | Name on one line |
| `project.users`, `core_features`, `out_of_scope`, `brief` | no | — | `brief` is a relative path with forward slashes |
| `languages.*` | no | `english` | `code`, `comments`, `docs`, `commits`, `agent_replies`; lowercase language names |
| `commands.*` | no | none | `dev`, `build`, `test`, `lint`; each a shell command on one line, listed in the main file's Commands section |
| `selections` | no | none | Catalog entry ids, kebab-case, no duplicates. Order matters, as in `{languages}` |
| `locked` | no | none | Selected ids a local file may not weaken; see Team rules |
| `i18n.default_locale` | yes, if `i18n` is present | — | BCP 47 tag such as `en` or `pt-BR` |
| `i18n.locales` | no | The default locale | Must include the default locale |
| `i18n.translations_by` | no | `agent-draft` | `agent-draft`, `agent-default-only` or `human` |
| `answers` | no | none | Free text keyed by snake_case question id, such as `domain_notes` |
| `custom_rules[]` | no | none | `rule` and `why` required, `locked` optional |
| `output.split`, `max_lines`, `topic_dir` | no | `auto`, `100`, `docs/agents` | See Splitting into multiple files |

Defaults are applied in code and never written to the file, so the spec records only what the user chose. Unknown fields are errors. `schema/spec.schema.json` gives editors autocompletion; as with the catalog, runtime validation is Go code.

Validation is split in two. `internal/spec` checks the file on its own: types, required fields, enums, formats, duplicates, and consistency inside the file, such as `locked` ids being selected. Anything that needs the catalog — whether selections exist, radio exclusivity, `requires`, `conflicts`, known `answers` keys — belongs to `internal/resolve`. Errors name the field and its line.

### Versioning

`version` changes only for breaking changes: renaming, removing or changing the meaning of a field. New optional fields keep the version, so an older cyanotype rejects them as unknown fields and suggests upgrading. A file with a newer version than the tool supports is rejected. An older one is migrated on load, one version at a time, by steps that rewrite the YAML tree rather than Go types, so comments survive; the new version reaches the disk only when the spec is saved. Anchors, aliases and merge keys are not supported, because a shared value cannot be updated in one place only.

### Resolving against the catalog

`internal/resolve` combines a loaded catalog and a loaded spec into a resolved selection: the selected entries in the order of `selections`, with placeholders filled and locked entries marked, plus the spec with its defaults applied. Renderers read only this result.

Problems that would produce wrong or inconsistent files are errors and block generation; the others are warnings. Both name the field and its line, such as `line 9: selections[3]: unknown entry "nextjs"; did you mean "next-js"?`.

| Check | Kind | Example message |
| --- | --- | --- |
| Selection not in the catalog, with the closest id when one is close enough | Error | `unknown entry "pnpn"; did you mean "pnpm"?` |
| Two entries of the same radio category | Error | `"npm" and "pnpm" (selections[1]) are both in radio category "package-manager"; keep only one` |
| A requirement that is not selected | Error | `"vitest" requires "vite" and "node" (through "vite"), which are not selected` |
| A `requires_one_of` group with no selected alternative | Error | `"shadcn-ui" requires one of "nextjs" or "vite-react", but none is selected` |
| Two selected entries that conflict | Error | `"supabase" conflicts with "sqlite" (selections[0]); keep only one` |
| A placeholder the selection cannot fill | Error | `"learning-guided" uses {languages}, but no entry of category "language" is selected` |
| An `answers` key that matches no question | Error | `unknown question "domain_note"; did you mean "domain_notes"?` |
| `catalog_version` newer or older than the catalog | Error or warning | See Distribution |

`requires` is transitive: selecting an entry means needing everything its requirements need. Each selection reports all its missing requirements in one message, following only entries that are not selected, because a selected entry reports its own gaps. `requires_one_of` groups follow the same path, and each unmet group gets a message of its own, with `(through "x")` when it comes from an unselected requirement. The search never goes into the alternatives of an unmet group, because none has been chosen yet. A group counts as met when any alternative is selected, including the entry being checked. That keeps cycles such as Node.js requiring one of npm or pnpm, which both require Node.js, from reporting anything when one package manager and Node.js are selected. The resolver never adds entries by itself: the spec records the user's choices, and the interface offers the fix. An unknown selection is reported once and left out of the other checks, so a typo does not cause a cascade.

Questions are not in the catalog yet, so the known `answers` keys are a list in the code, currently `domain_notes`. They move into the catalog together with category metadata.

### Writing the file back

When the tool updates `cyanotype.yaml`, it merges the new choices into the file as it was read: comments, key order and quoting survive, and only changed values change. The output always has the same canonical layout, so saving never reformats a file at random:

- two-space indentation, with list items indented under their key;
- one blank line between top-level keys and none elsewhere; a key's comment stays attached above it;
- flow sequences such as `[claude, cursor]` stay on one line while the line fits in 80 characters, and become one item per line when it does not or when an item carries a comment;
- LF line endings and one space before trailing comments.

Comments attached to a removed key or list item go with it. A file already in canonical layout comes back byte for byte when nothing changes.

### Project goal and product brief

The goal lives in the same spec, in two layers. A short summary — goal, users, core features, out of scope — goes into the main instruction file, because the agent needs it in every session and "out of scope" alone prevents a lot of unrequested work. The full product description goes into a separate `docs/PRODUCT.md`, which the tool can scaffold from the same answers and which the main file references on demand: *Before adding a feature, read docs/PRODUCT.md.*

A separate spec file means the user can regenerate when the catalog changes, add an agent without repeating choices, review changes in a pull request and share a configuration as a preset.

## Interface

Choices are made with radio buttons and checkboxes grouped by category; open text fields appear only where a closed choice is not enough. There are two modes and one configuration: simple mode pre-fills advanced mode.

### Simple mode

1. What do you want to build: website, mobile app, API, script or automation, desktop app, game (the last two marked as coming later in the first release).
2. Do you already know a programming language? Yes (which one) or no. Either way, the user can turn on Learning mode to learn the chosen language while the agent works.
3. How much autonomy should the agent have: asks often, balanced, autonomous.
4. Should the app support more than one language? No, later, or yes (which ones).
5. Language for agent replies.
6. Describe the project in one or two sentences (open field).
7. Which coding agents do you work with: Claude Code, Codex, Gemini CLI, Cursor, GitHub Copilot, or another agent that reads AGENTS.md. Several can be chosen.

From these answers the tool picks a preset, shows it with a short explanation of each component and offers to switch to advanced mode to inspect or change the details.

The project type and the known language pick the preset: the presets of that type, narrowed to those in the known language when there are any. When several remain, such as the two website presets, a starter question lists them. The translation question is asked only for websites and mobile apps, which have text to translate. Learning mode is offered only when the user does not know a language of the chosen preset, and is never on by default. The project name is not asked: it defaults to the folder name.

Each answer maps to the spec as follows; questions and mappings live in `internal/simple`, so every interface asks the same questions:

| Answer | Spec |
| --- | --- |
| Preset | `selections` first, in the preset's order, then `commands` and `output` |
| Multilingual: later | `i18n-rules` and the preset's `i18n_selections`; `i18n.default_locale`, `en` unless another is given |
| Multilingual: yes | The same entries; `i18n.default_locale` and `i18n.locales`, the default first |
| Autonomy | The entries in the table below, after the i18n entries |
| Learning mode | `learning-explain`, `learning-guided` or `learning-hands-on`, last |
| Reply language | `languages.agent_replies`, left out when English, the default |
| Description | `project.goal` |
| Agents | `targets`, see below |
| Always | `version`, `catalog_version` pinned to the bundled catalog, `mode: simple` |

| Entry | Asks often | Balanced | Autonomous |
| --- | --- | --- | --- |
| `conventional-commits`, `small-commits` | ✓ | ✓ | ✓ |
| `definition-of-done`, `no-destructive-commands`, `no-test-tampering`, `secrets-in-env`, `no-unrequested-changes`, `no-duplicate-files`, `rule-maintenance-propose` | ✓ | ✓ | ✓ |
| `plan-before-code` | ✓ | | |
| `dependency-approval` | ✓ | ✓ | |
| `branch-per-task` | | | ✓ |

Safety rules and an honest definition of done hold at every level; autonomy only changes how often the agent asks before acting. An autonomous agent works on a branch per task, so unsupervised work stays apart until it is reviewed.

Claude Code maps to `claude`, Codex and other agents to `agents-md`, and Gemini CLI, Cursor and GitHub Copilot to their own targets. `agents-md` is added whenever an agent other than Claude Code is chosen: AGENTS.md is the shared main file, and Cursor and GitHub Copilot read it before cyanotype generates their own files. Targets without a renderer stay in the spec, so their files appear once cyanotype supports them.

### Advanced mode

- Every catalog category, with radio buttons for exclusive choices and checkboxes for cumulative ones.
- An "i" icon next to each entry: tooltip with the `summary` and a link to the official page.
- Incompatible entries disabled, with the reason shown on hover.
- Live preview of the generated files, with a line counter and the split layout.

### Open questions and manual additions

- **"Not decided yet"** in every radio group. It generates a rule such as: *No database chosen yet. Propose two options with trade-offs and ask before adding one.* Indecision becomes a useful instruction.
- **"Other…"** in every group, for a technology missing from the catalog: name, link and an optional rule.
- **Project goal fields**: goal, users, core features, out of scope.
- **Domain notes**: business rules no catalog can know.
- **Custom rules**: two fields, rule and reason, to nudge users into writing the why.
- **Project commands**: dev, build, test, lint, pre-filled when detected and editable, stored in the spec's `commands`.

## Output

Each agent has a renderer that turns the same choices into its own format. AGENTS.md is the main file when the user targets several agents; the others import or mirror it.

| Agent | Generated file | Note |
| --- | --- | --- |
| Claude Code | `CLAUDE.md` | When AGENTS.md is also a target it holds `@AGENTS.md` plus any Claude-only rules |
| Codex and compatible agents | `AGENTS.md` | The shared format, used as the main source |
| Gemini CLI | `GEMINI.md` | Same import logic as Claude |
| Cursor | `.cursor/rules/*.mdc` | One file per topic, with frontmatter for scope |
| GitHub Copilot | `.github/copilot-instructions.md` | Compact version |

Formats change often: every renderer is checked against the official docs and covered by snapshot tests.

Renderers live in `internal/render`. They take the resolved selection and return the files to write, path and content, in memory; comparing them with the disk and writing belong to `internal/generate`, and asking to the interfaces (see Generating files). Output is deterministic: the same input gives byte-identical files, with LF line endings. To keep what regeneration must not lose, a renderer reads the project as it is now through an `fs.FS` rooted at the project folder, never through the disk directly. An `fs.FS` rather than a list of files, because the renderer, not the caller, knows which files it generates once output is split. Targets without a renderer yet are skipped with a warning.

### The main file

The main file follows the standard structure in Generated file quality. Each section has a fixed source, and a section with nothing to say is left out:

| Section | Source |
| --- | --- |
| Title | `project.name` |
| Project overview | `project.goal`, `users`, `core_features`, `out_of_scope` |
| Language policy | `languages.*`; kinds of text in the same language share a line, and replies always get their own |
| Tech stack | A "Uses:" line naming the selected entries that have a `url`, except those of topics `git`, `workflow` and `safety`, which are practices; then entries of topics `architecture` and `tooling` |
| Commands | `commands.*`, in the order dev, build, test, lint |
| Project structure | No source yet, so never generated; a protected section can hold it |
| Code style | Topics `style` and `i18n`, then the `i18n` settings: locales and who translates |
| Testing, Git workflow, Agent workflow | Topics `testing`, `git` and `workflow` |
| Other topics | An unknown topic gets a section of its own, titled from the topic and sorted by it, after Agent workflow, so a new topic needs no code change; entries without a topic go to "Other rules" |
| Safety rules | Topic `safety` |
| Domain notes | `answers.domain_notes`, then `custom_rules`: project knowledge no catalog can have |
| Further reading | Documents read on demand: `project.brief` (*Before adding a feature, read docs/PRODUCT.md.*), then the topic files of a split output |

Inside a section, entries keep the order of `selections`. Each entry is one bullet, `- <rule> Why: <why>`, with the lines of a multi-line rule joined by spaces: the shortest form keeps the file within its line budget.

AGENTS.md and CLAUDE.md get the same content when only one of them is a target. With both, AGENTS.md holds everything and CLAUDE.md holds the header, `@AGENTS.md` and only Claude-specific content. CLAUDE.md imports AGENTS.md only when `agents-md` is a target, so its content depends on the spec alone.

Every generated Markdown file opens with this header:

```markdown
<!-- Generated by cyanotype from cyanotype.yaml; regenerating replaces this file.
Change rules in cyanotype.yaml (custom_rules for your own), or write between
cyanotype:custom markers, which regeneration keeps. -->
```

It names the markers without quoting them, because an HTML comment cannot contain another one.

### Protected sections

Hand-written text between a `<!-- cyanotype:custom -->` line and a `<!-- /cyanotype:custom -->` line survives regeneration, markers included and byte for byte apart from CRLF line endings, which become LF. Each marker must be alone on its line.

A block belongs to the nearest `## ` heading above it; headings inside a block are hand-written text and do not count. On regeneration the block goes back to the end of the section with the same heading, a block above every section goes right after the title, and a block whose section is no longer generated goes to the end of the file. Blocks that land in the same place keep their order, and regenerating an unchanged project leaves every block where it is.

A marker without its partner, or a block inside another, stops generation with the file and line, because guessing wrong would silently drop hand-written text.

### Splitting into multiple files

When the main file would grow past a threshold, the content is split by `topic` into a short main file plus topic files under `docs/agents/` (testing, style, git, architecture, i18n…). The behavior is set by `output.split`:

- `auto` (default): split only when the main file would exceed `max_lines` (default 100).
- `always`: the "Always split" checkbox in the UI; useful for teams that want one file per area from day one.
- `never`: everything in one file, with a warning when it gets long.

For `auto`, the renderer builds the main file unsplit, header and protected sections included, and splits when it has more than `max_lines` lines; a file of exactly `max_lines` stays whole. Splitting is all or nothing: every topic that can move out does, rather than moving topics one by one until the file fits, so the layout stays predictable and adding one entry never reshuffles the files. A main file still longer than `max_lines` after splitting gets a warning too.

The topic folder defaults to `docs/agents/`, visible so that human contributors read the same conventions and agent search tools, which often skip hidden folders, find them. Each repo can change it with `output.topic_dir`. When the tool detects a documentation site generator such as MkDocs or Docusaurus, it warns that the folder could be published and offers to exclude it.

The main file always keeps the project goal, commands and safety rules, which the agent must never have to go looking for. When split, it also keeps the language policy, the Tech stack section (its "Uses:" line, still naming every technology, and every entry of topic `tooling`), domain notes and custom rules, and entries without a topic, which have no file to go to. Tooling stays because rules like "use pnpm, never npm" apply to almost every task, and an agent may run `npm install` without ever opening a topic file.

Every other topic moves to `<topic_dir>/<topic>.md`, such as `docs/agents/testing.md`: the header, a `# ` title, and the topic's entries in selection order. Protected sections work in topic files as in the main file. Titles and the triggers that tell the agent when to read each file:

| Topic | Title | Trigger in the main file |
| --- | --- | --- |
| `architecture` | Architecture | Before adding a dependency or changing how the app is structured |
| `style` | Code style | Before writing code |
| `i18n` | Internationalization | Before adding or changing user-facing text |
| `testing` | Testing | Before writing or changing tests |
| `git` | Git workflow | Before committing or opening a pull request |
| `workflow` | Agent workflow | Before starting a task |
| Unknown, such as `code-review` | From the topic: Code review | For code review rules |

Each topic file then reaches the agent in one of three ways, chosen per topic:

| Mode | How | Context cost | When to use |
| --- | --- | --- | --- |
| Imported | `@docs/agents/testing.md` in CLAUDE.md or GEMINI.md | Always loaded | Rules needed in almost every task; organizes, does not save context |
| On demand | *Before writing tests, read docs/agents/testing.md.* | Loaded when relevant | Long guides used in specific tasks |
| Path-scoped | `.claude/rules/` with `paths:`, Cursor globs, nested AGENTS.md | Loaded when matching files are touched | Rules for one part of the codebase (frontend, migrations) |

Only the last two actually reduce what the agent reads; importing is about tidiness. The preview shows the resulting layout and the line count of each file.

In the MVP every topic file is read on demand, for every target: the main file's Further reading lists one line per topic file, *Before writing or changing tests, read docs/agents/testing.md.*, in section order. Not imported for Claude, because:
1. Splitting exists because the main file is too long, and an import is always loaded, so Claude would read as many lines as without splitting.
2. With both targets CLAUDE.md only imports AGENTS.md; imports only Claude can follow would make Claude and Codex read different rules from the same spec.
3. AGENTS.md has no import syntax, so on demand is the one mode that works for both, with one code path.

Path-scoped files need globs per entry, which the catalog does not have yet, so they wait.

The renderer returns only the files the current layout needs. A topic file left over from an earlier split, or from a topic no longer selected, is not removed: telling it apart from a hand-written file needs a record of what cyanotype generated, the `cyanotype.lock` open item in Decisions.

### Real constraints

Critical catalog rules carry an `enforce` field that generates, alongside the text, something that makes them hold.

| Level | Examples | Applies to |
| --- | --- | --- |
| Agent permissions | Deny rules in `.claude/settings.json` for destructive commands or the wrong package manager | That agent only |
| Agent hooks | A hook that blocks edits to test files or `.env` | That agent only |
| Repository | `.gitignore` with `.env`, `.env.example`, `.editorconfig` | Everyone, humans included |
| Pre-commit | Secret scanning, linter, formatter | Everyone, at commit time |
| CI | GitHub Actions workflow with required tests and lint | Everyone, before merge |

Repository, pre-commit and CI levels work with any agent, so they are the baseline. The tool never overwrites an existing file without showing a diff and asking.

Agent-level constraints only cover the tools they name. A hook or deny rule that blocks the agent's file-editing tools does not stop it from writing the same file through the shell, with a redirection, `sed` or a script it runs. Agent-level enforcement must therefore cover shell commands too: rules and hooks on every shell tool the agent has, such as Bash and PowerShell, plus the agent's sandbox where one exists, for what no command pattern can catch. Even then it is best effort, so the repository, pre-commit and CI levels stay the baseline.

A manual check of `secrets-in-env` with Claude Code 2.1.299 confirmed this; the procedure and results are in [enforce-checks.md](enforce-checks.md). `Read` deny rules on `.env` files blocked the Read, Edit and Write tools and every shell command that named the file. They did not block:
- shell writes such as `>> .env`, `tee -a` and PowerShell `Add-Content`, which matching `Edit` deny rules did block, so the entry ships both kinds of rules;
- reads that do not name the file, such as globs (`cat .en*`), command substitution, `source` and `grep -r`;
- scripts that open the file themselves.

The package manager entries and the `ask` rules went through the same procedure on 2026-10-20. The space syntax `Bash(npm *)` and the legacy `Bash(npm:*)` matched the same commands, bare ones included, and `npm *` did not match `npx`. Compound commands and `command npm` were blocked; `npm.cmd` in Bash and an option before the subcommand, such as `pip -q install`, were not. Every entry now uses the space syntax, covers every command its snippet forbids rather than one subcommand, and has a `PowerShell(...)` rule for each `Bash(...)` rule. enforce-checks.md tracks which entries have been checked.

#### Generating `.claude/settings.json`

When `claude` is a target, the selected entries' `enforce.claude.deny` rules go to the `permissions.deny` list of `.claude/settings.json`, and their `enforce.claude.ask` rules to `permissions.ask`, entries in selection order and each entry's rules in catalog order. `rule-maintenance-propose` also gets the deny rule `Edit(/<topic_dir>/**)`, added by the renderer because the topic folder is configurable. CLAUDE.md then gets a Claude Code section saying the permission rules back the safety rules, that deny rules block an action and ask rules make Claude Code ask the user first, and that a blocked action means stop and ask, not find another way; AGENTS.md stays agent-neutral.

Deny is for "never": the action stays blocked even when the user would approve it. Ask is for "ask first": Claude Code asks the user before the action, so an approved change can still happen. `rule-maintenance-propose` asks before edits to `cyanotype.yaml`, so the user can still have the agent apply a rule they approved, but denies edits to the generated instruction files, which regeneration would undo. In Claude Code deny wins over ask and ask wins over allow, so validation rejects an entry that lists the same rule in both, which would never ask. A pattern ending in ` *` also matches the bare command, so a plain `npm install` asks too; approving it is one click.

Some rules depend on two entries at once. `dependency-approval` asks before a dependency is added, but the command that adds one belongs to the stack: `pnpm add`, `uv add`, `go get`, `npx expo install`. Listing every tool's command in `dependency-approval` would fill a pnpm project's settings with rules for uv and Go, and give a stack-agnostic safety rule knowledge of every stack. So rules can be conditional, in `enforce.claude.with`, keyed by the entry that must also be selected:

```yaml
# catalog/entries/pnpm.yaml
enforce:
  claude:
    deny: ["Bash(npm *)", "Bash(yarn *)", "PowerShell(npm *)", "PowerShell(yarn *)"]
    with:
      dependency-approval:
        ask: ["Bash(pnpm add *)", "PowerShell(pnpm add *)"]
```

The rules of a key apply only when that entry is selected too, in either order in `selections`. They join the ask group of the entry that carries them, after its own `ask` rules, keys in alphabetical order since a map has none, each key's rules in catalog order. Each tool that installs dependencies carries its own command, `npm`, `pnpm`, `uv`, `go` and `expo` in the MVP, so a new package manager entry brings its rule in one file and `dependency-approval` itself has no `enforce`. The mechanism knows no entry by name: the link lives in the catalog data. Validation requires an existing key other than the entry itself, outside the entry's own radio category, where the two could never be selected together; at least one rule per key; and no rule also in the entry's `deny`. Only `ask` is supported inside `with`; any other key is an unknown field. Conditional deny rules, `with.<id>.deny`, are a possible extension that fits the same map.

Order matters: a negated rule such as `Read(!.env.example)` only narrows the rules listed before it. So the merge keeps every entry's rules as a subsequence of the final list, in their catalog order. It walks each entry's rules with a cursor and reuses a rule already in the file only when it comes after the entry's previous rule; otherwise it appends the rule again. A user's list with `Read(!.env.example)` before `Read(.env)` therefore gets a second negation after the generated rules, at the cost of a duplicate. Merging twice changes nothing. The deny and ask lists are merged the same way, each on its own.

An existing file is merged, never replaced. Its keys, their order and their exact values stay, unknown fields included, and the user's own rules stay first in each list, in their order; only `permissions.deny` and `permissions.ask` change, and a list with nothing to add is left as it is, or left out when absent. When the merge adds nothing, the file comes back byte for byte; otherwise it is written with two-space indentation and without escaping characters such as `>` and `&`. A file that cannot be read safely is an error rather than a rewrite: invalid JSON, duplicate keys, a `permissions` that is not an object, or a `deny` or `ask` that is not a list of strings. A new file starts with the schemastore `$schema`, then `deny` before `ask`; with no rules and no existing file, none is generated.

The merge is additive: deselecting an entry leaves its old rules in the file, because JSON has no comments to mark which rules cyanotype wrote. The same holds when a rule moves from deny to ask in a newer catalog: the old deny rule stays and, since deny wins, the new ask rule never fires until someone removes it by hand. And a project generated when `dependency-approval` still listed every package manager's install command keeps all those ask rules after regenerating. The `cyanotype.lock` open item in Decisions would solve this.

#### The `.gitignore` block

Personal files, `cyanotype.local.yaml` and `.agents.local/`, go into `.gitignore` inside a block cyanotype owns, whatever the targets. The `enforce.gitignore` lines of the selected entries follow them, entries in selection order and each entry's lines in catalog order. With `secrets-in-env` selected:

```gitignore
# cyanotype:begin (generated from cyanotype.yaml; regeneration replaces this block)
cyanotype.local.yaml
.agents.local/
.env
.env.*
!.env.example
# cyanotype:end
```

`.gitignore` is the repository level of Real constraints: it holds for every agent and for people, so a rule such as "`.env` stays in `.gitignore`" is true from the first `git add .`. Entry lines are generated whatever the targets, like the personal files. Order matters here too: `!.env.example` only re-includes what the lines before it ignore. So the block's lines are merged like deny rules: each entry's lines stay a subsequence in catalog order, and a line two entries share is written once unless reusing it would break an entry's order. In the MVP, `secrets-in-env` ignores `.env` files and `sqlite` its database files; an entry gets lines only when its snippet promises that something stays out of git.

Catalog validation keeps each line one pattern: not blank, on one line, without leading or trailing spaces, which git reads as part of the pattern, not starting with `#`, which would make a comment and could imitate a marker, and not listed twice in the entry.

`.gitignore` only stops untracked files from being added: a `.env` committed before the block existed stays tracked, and its secrets stay in the history. Warning about that is an open item for `cyanotype check` in Decisions.

Regeneration replaces the block where it is and leaves the rest of the file byte for byte; a line starting with `# cyanotype:begin` opens it, so the explanation can change. Without a block, it is appended after a blank line. The block follows the file's line endings, CRLF when the file has any. Entries the user already lists outside the block are not removed or skipped, so the block never depends on the rest of the file. A second block, or a marker without its partner, is an error.

### Generating files

`cyanotype generate` is the first command that uses the whole core. It reads `cyanotype.yaml` from the current folder, or the one `--dir` names, resolves it against the catalog embedded in the binary, renders the files with the project folder as the existing files, and compares each one with the disk. It prints a unified diff of what changes, colored by line kind, then one line per generated file with its status, `created`, `modified` or `unchanged`, and the counts. Then it asks `Write these files? [y/N]`; only `y` or `yes` writes. When nothing changes it says so and asks nothing.

| Flag | Effect |
| --- | --- |
| `--dir <folder>` | The project folder; default the current one. `cyanotype.yaml` is not searched for in parent folders |
| `--dry-run` | Show the diff and write nothing |
| `--yes`, `-y` | Write without asking. Cannot be combined with `--dry-run` |

When stdin is not a terminal and `--yes` is not given, the command never reads stdin: it shows the diff and exits, saying to run it again with `--yes`. A CI job or script waiting for an answer nobody gives would otherwise hang. Git Bash's mintty connects programs through pipes, so there it counts as non-interactive too.

| Exit code | Meaning |
| --- | --- |
| 0 | Files written, or already up to date |
| 1 | Error: a problem in the spec or in existing files, or an I/O error |
| 2 | Wrong usage: unknown commands or flags, arguments, `--yes` with `--dry-run` |
| 3 | Changes found but not written: `--dry-run`, a declined confirmation, or no terminal to ask on |

Usage errors name the real mistake: `unknown command "genrate" for "cyanotype"; did you mean "generate"?` for a mistyped command, even when flags follow it, and `"cyanotype generate" takes no arguments, got "extra"` for an extra argument. Cobra reports an unknown command as a plain error while it looks the command up, before any flag handling, so the root command validates its own arguments to turn it into a usage error, without matching cobra's message text.

Code 3 lets scripts check that generated files are current with `cyanotype generate --dry-run`, as `git diff --exit-code` does, until `cyanotype check` exists, and makes a script that forgets `--yes` fail instead of succeeding without writing.

Warnings and errors go to stderr, warnings first, in the `cyanotype.yaml:9: error: selections[3]: ...` form that editors and terminals link to the line. yaml.v3 reports the line of some syntax errors one off, so those say `invalid YAML near line 7` and show lines 6 to 8: the right line is in view either way, and no precise but wrong number misleads the user. The diff and the summary go to stdout, so they can be redirected on their own.

Colors appear only when the stream is a terminal, never when `NO_COLOR` is set to a non-empty value or `TERM` is `dumb`. Windows consoles show them once the command turns on their ANSI support, and stay plain if that fails.

Comparing and writing live in `internal/generate`, shared with the MCP server's `preview` and `generate` tools, which return the same diff:

- **Line endings.** A file that matches the generated content once CRLF is read as LF is `unchanged` and not rewritten: with `core.autocrlf`, the default of Git for Windows, every checkout has CRLF endings, and rewriting them all on each run would only churn files. A file that really changes is written with LF, as all generated output is, and the summary says its line endings change. The diff compares lines as LF, so it shows only the real changes.
- **Paths.** Every path is checked before anything is written. A path that leaves the project root, uses backslashes, or is a Windows reserved name such as `con.md` stops the whole run; writes go through `os.Root`, which also refuses symbolic links that lead out of the project.
- **Links and folders.** A target that is a symbolic link, such as a `CLAUDE.md` linked to `AGENTS.md`, or not a regular file, stops the run rather than being replaced by a file: the layout is the user's.
- **Atomic writes.** Each file is written to a temporary file in its folder, synced, then renamed over the target, so a crash never leaves a half-written file; missing folders are created. Existing files keep their permissions; new ones are readable by everyone, as checked-out files are.
- **No deletions.** Nothing is ever deleted; stale topic files wait for `cyanotype.lock`. If writing stops halfway, the command lists the files already written.

The diff is in-house, in `internal/diff`: Myers' algorithm with three lines of context, which is enough for files of a few hundred lines. Texts that need more than 1000 edits are shown as wholly replaced, which bounds memory. Go's standard library has no diff; of the libraries, `pmezard/go-difflib` and `hexops/gotextdiff` are archived, `sergi/go-diff` diffs characters rather than lines, and `aymanbagabas/go-udiff` is maintained but not worth a dependency here.

### Creating the spec

`cyanotype init` is simple mode in the terminal. In an existing project it starts from the detected stack once the user confirms it (see Detecting the stack). It asks the questions of Simple mode, shows the chosen preset with the name and `summary` of every entry it selects, then the project name and goal, writes `cyanotype.yaml` and, on a terminal, offers to run generate, which then shows its diff and asks before writing as usual. Advanced mode arrives with the TUI; until then the spec is edited by hand.

Every question has a flag, so agents, scripts and the MCP server can run it without a terminal:

| Flag | Answers |
| --- | --- |
| `--type` | What to build: `website`, `mobile-app`, `api` or `script`. `desktop-app` and `game` are refused as coming later |
| `--language` | A language the user knows: a catalog language, `other` or `none` |
| `--preset` | The starter preset; implies `--type`, and is needed only when several presets fit |
| `--learning` | `off` (default), `explain`, `guided` or `hands-on` |
| `--autonomy` | `asks-often`, `balanced` or `autonomous` |
| `--i18n`, `--locales` | `no`, `later` or `yes`; the locales, default first, such as `en,it`. Websites and mobile apps only |
| `--replies` | Language of the agent's replies; default English |
| `--goal` | The project in one or two sentences |
| `--agents` | `claude`, `codex`, `gemini`, `cursor`, `copilot`, `other`, separated by commas |
| `--name` | Project name; default the folder name. The name is not asked, so the summary shows it and, when it comes from the folder, says so and how to change it: `--name`, or `project.name` in `cyanotype.yaml` |
| `--dir` | The project folder, as for generate |
| `--detect` | In an existing project: `yes` to start from the detected stack, `no` to ignore it and pick a starter. See Detecting the stack |
| `--with`, `--without` | Entry ids to add to the detected stack, or to choose where detection found several, and detected entries the project does not use |

Flags answer their questions and the rest are asked. When stdin is not a terminal, init never reads it: the missing answers are one usage error that lists each missing flag with its allowed values, so an agent can fix its call in one step instead of hanging. Wrong values are usage errors before anything is asked.

init never overwrites a spec. An existing `cyanotype.yaml`, even a symbolic link, stops it before the first question, and the file is created exclusively, through `generate.CreateSpec`, so one that appears while the user answers is not replaced either. Before writing, the spec is resolved against the catalog like any other.

Exit codes are those of generate: 0 written, 1 an error such as an existing spec, 2 wrong usage or missing answers, and, when init runs generate, the code generate ends with, 3 for files not written.

Prompts use the standard library: numbered choices, answered by number or value, and lines of text, re-asked with a reason when wrong. Questions go to stderr and results to stdout, as generate's confirmation does. charmbracelet/huh would give arrow keys and inline validation, but it brings about twenty modules and is part of the TUI that Phase 2 builds with Bubble Tea. The questions are data in `internal/simple`, so a huh form will render the same ones when the TUI pays for that dependency.

#### Detecting the stack

When the folder has no `cyanotype.yaml`, init first runs detection on it (see Detection). If it finds nothing, init is simple mode as described above. If it finds something, init shows every entry with what revealed it, the entries that cannot all be selected, and the `package.json` scripts, then asks whether to start from that stack: yes, yes without some of it, or no, which ignores it and picks a starter. Detection never writes anything; the spec is written at the end, as always. Without a terminal `--detect` answers, and it is the only missing answer reported until given, because the questions that follow depend on it.

```text
Detected in this project:
  - Next.js (package.json: next)
  - Node.js (package.json)
  - npm (package-lock.json)
  - pnpm (pnpm-lock.yaml)
  - TypeScript (package.json: typescript)
Only one of npm and pnpm can be selected: the project uses one of them.
Scripts in package.json: build, dev, lint, start
```

Detection says what the project uses; a preset says what a new project would use. So on an existing project the stack comes from detection, and simple mode never adds a technology detection did not find, such as Supabase to a Next.js app that does not use it. Presets still answer what the files cannot:

| Question or field | With a detected stack |
| --- | --- |
| What to build | Skipped when the presets sharing the most entries with the stack are all of one project type; otherwise asked. The type is not stored in the spec: it only decides whether the translation question is asked and which i18n library is offered, so this best guess is enough |
| Starter | Never asked |
| Known language | Asked, for learning mode only, which is offered when the stack has a language the user does not know |
| Translations | Skipped when the stack has an entry some preset adds for translations, such as `i18n-next-intl`: only the locales are asked, and one locale means `later`, several `yes`. Otherwise asked; the closest preset of the project type gives its `i18n_selections`, each only when it fits the stack, so a Next.js library never joins a Vite app |
| Autonomy, reply language, goal, agents | Asked as for a new project; rules the stack already has are not added twice |
| `selections` | The detected entries in the order of the closest preset, then in catalog order, with `--without` entries removed and `--with` entries added |
| `commands` | The `dev`, `build`, `test` and `lint` scripts of `package.json`, run with the selected package manager's `run_script`. A preset's commands are never used, since they may name scripts the project does not have; without matching scripts the summary says there are none |

Partial matches and clashes are completed by the user, never guessed:

- An entry a detected one `requires` is added, and the summary says which entry required it.
- A `requires_one_of` group with no detected alternative, such as Node.js with neither lockfile, is a question, with the closest preset's alternative as the default.
- Detected entries of one radio category, such as npm and pnpm from `package-lock.json` and `pnpm-lock.yaml`, or entries that conflict, are a question: which one the project uses. Only the user knows which lockfile is current.
- A file that reveals something the project does not use, such as an `app.json` that is not Expo's, is removed with "yes, without some of it" or `--without`.

Without a terminal each open question is a missing `--with`, listed with the other missing answers. An id in `--with` replaces the entries of its radio category and those it conflicts with, so `--with pnpm` keeps pnpm and drops npm. `--preset` starts from that starter and skips detection; combining it with `--detect yes`, `--with` or `--without` is a usage error, as are `--with` and `--without` with `--detect no`, unknown ids, which get a "did you mean", and removing an entry detection did not find.

## Generated file quality

The main file aims to stay under 80–100 lines, because the most common complaint ("it ignores my CLAUDE.md") almost always comes from files that are too long or too vague.

- **Line counter** in the preview, yellow above 80 lines and red above 150, with a one-click suggestion to split.
- **Concrete rules only.** The catalog rejects generic snippets like "write clean code"; pull request review enforces this.
- **A reason next to each rule**, in short form, from the `why` field.
- **Contradictions blocked at the source** through `conflicts`; custom rules are compared with catalog rules and flagged when they overlap.
- **Long documents referenced, not embedded**, as described in the splitting section.
- **Known traps handled per agent** by the renderers. Example: in Claude Code, a CLAUDE.md or CLAUDE.local.md that is present stops AGENTS.md from being read, so every Claude file the tool writes imports it.

Standard structure of the main file, always in this order: Project overview, Language policy, Tech stack, Commands, Project structure, Code style, Testing, Git workflow, Agent workflow, Safety rules, Domain notes, Further reading.

## Keeping rules current

Instruction files drift as soon as the project starts: dependencies change, conventions emerge, the same mistake happens twice. Cyanotype handles this on two sides, and both go through the spec file so regeneration never wipes anything.

### What the agent may do

A catalog category, "Rule maintenance", sets the policy with a radio choice:

| Policy | Agent behavior | Fits |
| --- | --- | --- |
| Propose (default) | Never edits generated instruction files, and edits `cyanotype.yaml` only when the user explicitly asks. When it learns a convention or is corrected twice on the same point, it proposes a rule and a reason; the user approves, and may have the agent apply it | Most projects |
| Self-update | May add `custom_rules` to `cyanotype.yaml` and regenerate, mentioning it in the commit message | Solo projects, high autonomy |
| Off | Rules change only by hand | Strictly controlled repos |

Generated files open with a header saying they come from `cyanotype.yaml` and where manual edits belong. With the Propose policy, deny rules on the generated files back this up, and an ask rule on `cyanotype.yaml` makes the agent ask before changing it, so a rule the user approved can still be applied.

An optional decision log, `docs/agents/decisions.md`, lets the agent record architectural choices as date, decision and reason. The main file references it on demand, so it costs no context until needed.

### What the tool checks

`cyanotype check` detects drift and runs locally, as a pre-commit hook or in CI:

- dependencies or config files found in the repo but missing from the spec (via `detect`);
- generated files edited by hand or out of sync with the spec;
- a newer catalog with changes to selected entries;
- files over the line threshold, and references to files that no longer exist;
- files that match an `enforce.gitignore` line but are already tracked by git, such as a committed `.env` (open item in Decisions).

`cyanotype update` then applies catalog updates, showing a diff before writing anything.

## Team rules and personal rules

Several people can work in the same repo with different rules: the team spec is committed, each person's overrides are not.

| File | Committed | Holds |
| --- | --- | --- |
| `cyanotype.yaml` | yes | Stack, conventions, safety rules: everything the whole team shares |
| `cyanotype.local.yaml` | no, added to `.gitignore` | Personal preferences: agent reply language, autonomy level, extra personal rules |

The tool merges the two and writes personal output to each agent's local file where one exists, such as `CLAUDE.local.md` and `.claude/settings.local.json` for Claude Code. For agents without a local variant, personal rules go to the user-level file in the home directory, which then applies to all of that person's projects.

Collaborators often use different agents. The team spec lists the targets whose files are committed; each person can add more targets in `cyanotype.local.yaml`, and the files generated for them go into `.gitignore`, so one person's setup never changes what others see. Shared files, the main instruction file and the topic folder, stay in one place per repo as set in the team spec; moving them per person would scatter copies of the same rules across commits. Personal topic files go to a local folder, `.agents.local/` by default, which each person can change.

Team entries can be locked. A locked rule, typically a safety rule, cannot be weakened or removed by a local file; `cyanotype check` reports any attempt. Selections are plain ids, so locked selections are listed in a top-level `locked` list, and custom rules carry `locked: true`.

`cyanotype.local.yaml` will have its own type, built from the same parts as the team spec (languages, targets, custom rules), so strict decoding rejects team-only keys such as `project` by itself. Merging needs the catalog, since a personal radio choice replaces the team's unless it is locked, so it belongs to `internal/resolve`: lists add up, `agent_replies` from the local file wins, and changes to locked entries are reported instead of applied. In the local file, `output.topic_dir` is the personal topic folder.

One trap the renderer must handle: a `CLAUDE.local.md` also stops Claude Code from reading AGENTS.md, so the generated local file always imports it.

## App internationalization

Translations in the app the user is building get their own catalog category, because agents left alone hardcode strings and forget locales. Deciding up front costs one question; retrofitting costs a refactor of every screen.

| Choice | Options |
| --- | --- |
| Multilingual | No, later (prepare the structure), yes |
| Locales | Default locale plus supported locales |
| Library | Filtered by stack, e.g. next-intl, i18next, Flutter intl, gettext |
| Who translates | Agent drafts all locales, agent writes the default locale only and marks the rest, human translators via files or a platform |
| Extras | Right-to-left support, pluralization rules, locale-aware dates, numbers and currency |

Typical rules this generates:

- Never hardcode user-facing text; every string goes through the i18n library with a key. With no library chosen yet, the agent proposes one and asks before adding it.
- Key naming follows `area.screen.element`, for example `recipes.list.emptyState`.
- A new key is added to every locale file in the same change; untranslated values are marked so they are easy to find.
- Dates, numbers and currency are formatted with the locale APIs, never by hand.

Even "later" is useful: it tells the agent to route strings through a single module from day one, so adding languages becomes a configuration change.

Real constraints: a lint rule against hardcoded strings where the stack has one, and a CI check that every locale file has the same keys.

## MCP server

`cyanotype mcp` starts a local MCP server over stdio, a fourth interface on the same core, so other agents can configure projects without a human clicking through the UI. Typical uses: an orchestrator bootstrapping many repos with consistent rules, and coding agents proposing rule changes under the Propose policy.

| Tool | Does | Writes |
| --- | --- | --- |
| `search_catalog` | Finds entries by name, category or stack | no |
| `get_spec` | Returns the current spec, merged with local overrides | no |
| `detect_stack` | Suggests entries from the repo's files | no |
| `validate` | Reports conflicts, missing requirements, line counts | no |
| `preview` | Returns the files that would be generated, as a diff | no |
| `set_selections` | Adds or removes catalog entries in the spec | spec only |
| `propose_rule` | Queues a custom rule with its reason for human approval | queue only |
| `generate` | Writes instruction files and constraints | yes, see below |
| `check` | Runs drift detection | no |

Safety: `generate` is dry-run unless called with an explicit flag, refuses to overwrite files edited by hand, and never removes or weakens `locked` rules. Approval of queued rules happens in the CLI, TUI or desktop app, never through MCP.

The CLI mirrors every MCP tool with non-interactive flags and `--json` output, for scripts and agents without MCP support.

## Blueprints

A blueprint packages everything needed to rebuild a product from scratch: the same stack, rules, scope and acceptance criteria. It guarantees the same verifiable behavior, not the same code. Agents are non-deterministic, so two builds from one blueprint will differ in implementation; anyone who needs identical code should share the repository instead.

### Two kinds

| Kind | Describes | Typical use |
| --- | --- | --- |
| Template | A kind of product, e.g. "SaaS with Next.js and payments" | Starting many different products the same way; an evolved preset |
| Product | One specific app, e.g. recipe-box | Handing a project to another developer, rebuilding it from scratch, or having an agent build it through the MCP server |

A template carries stack, rules and structure. A product blueprint adds the brief, data model, plan and acceptance criteria.

### Package contents

| File | Holds | Template | Product |
| --- | --- | --- | --- |
| `blueprint.yaml` | Manifest: name, kind, version, author, license, catalog version | required | required |
| `cyanotype.yaml` | Team spec with pinned technology versions | required | required |
| `PRODUCT.md` | Goal, users, features, out of scope | optional | required |
| `acceptance.yaml` | Acceptance criteria per feature | no | required |
| `plan.md` | Milestones in order, each with its done criteria | no | required |
| `contracts/` | Data model and API contracts, e.g. an SQL schema and an OpenAPI file | no | recommended |
| `decisions.md` | Decisions taken so far and why | optional | recommended |
| `design/` | Palette, typography, reference screens | optional | optional |

Never included: `cyanotype.local.yaml`, secrets and environment files, and the generated instruction files, which are rebuilt on import for the receiver's agents.

A blueprint is either a `blueprint/` folder in the repo or a single `.blueprint` archive of the same files.

```yaml
# blueprint.yaml
name: recipe-box
kind: product
version: 1.2.0
author: Jane Doe
license: MIT
catalog_version: 2026.10
cyanotype_version: ">=0.5"
```

### Acceptance criteria

Acceptance criteria are what make independent builds converge: what matters is checked, not left to each agent's interpretation.

```yaml
- id: private-recipes-hidden
  feature: sharing
  milestone: 2
  given: a visitor who is not signed in
  when: they search for recipes
  then: private recipes never appear in the results
  test: e2e
```

The blueprint adds two rules to the generated files: every criterion is covered by a test named after its `id`, and a milestone is done only when all its criteria pass.

### Lifecycle

1. `cyanotype blueprint export` packages the current project, strips personal and secret files and flags missing parts, such as features without criteria.
2. `cyanotype blueprint import` unpacks it, asks for the receiver's local preferences and agents, generates the instruction files and scaffolds one pending test per criterion.
3. The agent builds milestone by milestone following `plan.md`.
4. `cyanotype blueprint verify` runs the acceptance tests and reports which criteria pass, fail or have no test.

Pinned versions age. On import the tool offers to keep them or update to current versions, showing what changes; updating may require adjusting the plan.

The MCP server exposes the same steps as `export_blueprint`, `import_blueprint` and `verify_blueprint`, so an orchestrator can rebuild a product end to end.

Open question: whether to host a public blueprint gallery, and how to review submissions for quality and safety.

## Additional features

These turn a one-shot generator into a tool that stays with the project.

- **Stack detection.** On an existing project it reads `package.json`, `pyproject.toml`, `requirements.txt`, `go.mod`, lockfiles and config files through the catalog's `detect` field, and pre-selects what it finds, with commands from `package.json` scripts. The user confirms instead of typing. See Detection and Detecting the stack.
- **Presets.** Ready combinations ("Next.js + Supabase", "FastAPI + Postgres", "Simple Python script") stored as partial `cyanotype.yaml` files: the stack part of a spec, with the same keys, plus metadata. See Presets. Community presets arrive by pull request.
- **Protected sections.** Hand-written text between `<!-- cyanotype:custom -->` markers survives regeneration; see Output.
- **Live preview** in TUI and desktop, updated on every selection.
- **Sharing.** Export a configuration as a file or a compact string, import it in one step.
- **Multiple agents at once**, generating every target in a single run.
- **Link checking** of catalog URLs on a schedule in GitHub Actions.

## Roadmap

The MVP is the CLI with simple mode: it proves the value with the least work, and everything it builds in the core carries over to the TUI, MCP server and desktop app.

&#91;embedded content: roadmap · 4 phases, 3 releases\]

Phases have no dates yet. The catalog grows alongside every phase, including outside contributions after v0.1, and Linux, macOS and Windows builds ship from the first release.

Phase 2 ships as one release per feature, so each one reaches users as soon as it is ready and a problem in one never holds back the others:

| Release | Ships |
| --- | --- |
| v0.2 | Stack detection, and handling of existing instruction files, so `init` works in existing projects |
| v0.3 | `cyanotype.lock` and `cyanotype check` |
| v0.4 | The shared JSON contract of the CLI and the MCP server, and the MCP server |
| v0.5 | Category metadata and the TUI |
| v0.6 | Gemini, Cursor and Copilot renderers, with path-scoped rules |

Signed catalog updates and additional catalogs come after v0.6.

## Decisions

- [x] **Language**: English everywhere — interface, generated files, documentation, code, comments and commits.
- [x] **Core language.** Go, with Bubble Tea and Huh for the TUI and Wails for the desktop. Chosen over Rust and TypeScript for single-binary distribution, the Charm terminal ecosystem and a low barrier for contributors.
- [x] **Name: Cyanotype, command cyanotype with the short alias cyty (checked free on Homebrew, Debian, Arch, AUR, PyPI, crates.io and npm). Available: GitHub organization cyanotype-dev and domain cyanotype.dev (cyanotype, cyanotype-ai and cyty are taken on GitHub; cyanotype.com, .io and .app are registered). Trademarks checked on TMview (6 results): none in classes 9 or 42 in the EU, Italy or the US; class 42 marks exist only in Japan (cyanotype) and the UK (Cyanotype Media). GitHub organization cyanotype-dev created; Go module path github.com/cyanotype-dev/cyanotype. Next step: register cyanotype.dev**.
- [x] **License**: Apache-2.0 for the code, for its explicit patent grant and contribution terms; CC0-1.0 for the catalog, so generated files carry no license obligations. User blueprints use the license in their own manifest.
- [x] **Catalog distribution**: bundled snapshot plus explicit, signed updates, pinned per project. See the catalog's Distribution section.
- [x] **MVP catalog entries**: 41 entries and 7 starter presets. See the catalog's MVP entries section.
- [x] **Preset format**: a preset is the stack part of a spec (`selections`, `commands`, `output`) plus `id`, `name`, `summary`, `project_type` and `i18n_selections`, in `catalog/presets/<id>.yaml`, embedded and versioned with the catalog. No `targets`, which are a personal choice. Loading resolves each preset against the catalog, with and without its i18n entries, so a preset that loads always generates. See Presets.
- [x] **Topic folder**: `docs/agents/` by default, configurable per repo with `output.topic_dir`; personal topic files in `.agents.local/`, configurable per person. See Output and Team rules.
- [x] **Catalog references**: `requires` and `conflicts` hold plain entry ids; exclusivity inside a radio category is implicit and `conflicts` only links entries of different categories. See The catalog.
- [x] **Catalog layout**: one flat folder, `catalog/entries/<id>.yaml`, with the file name equal to the id. The category lives only in the entry.
- [x] **Catalog YAML parsing**: `go.yaml.in/yaml/v3` with unknown fields rejected. The JSON Schema in `schema/` serves editor autocompletion; runtime validation is Go code, which also checks references across entries.
- [x] **Catalog select per category**: all entries of a category share one `select` value, enforced by validation until category metadata exists.
- [x] **Spec format**: `cyanotype.yaml` fields, required fields and defaults as in the Project spec section; defaults live in code, never in the file. Locked selections go in a top-level `locked` list so `selections` stays a plain list of ids.
- [x] **Spec versioning**: an integer `version`, bumped only for breaking changes, with migrations on the YAML tree; newer versions are rejected.
- [x] **Spec write-back**: merge into the original YAML tree to keep comments and order, then write a canonical layout (blank line between top-level keys, flow sequences over 80 characters as block lists).
- [x] **Spec validation boundary**: `internal/spec` validates structure only; checks that need the catalog, and the merge with `cyanotype.local.yaml`, belong to `internal/resolve`.
- [x] **Catalog version**: the bundled catalog's version lives in `catalog/catalog.yaml`, next to `catalog/entries/`. A spec pin older than the loaded catalog is a warning, a newer one an error. See Distribution.
- [x] **Resolution**: unknown selections, radio clashes, missing `requires` (transitive), conflicts, unfillable placeholders and unknown `answers` keys are errors with field and line; warnings never block generation. Resolved entries keep the order of `selections`. See Resolving against the catalog.
- [x] **Snippet placeholders**: `{languages}` is the only placeholder; other names are catalog validation errors, and text inside backticks is literal.
- [x] **Agent enforcement covers the shell**: agent-level constraints on file tools can be bypassed through shell commands, so they must also cover the agent's shell tools and sandbox; repository, pre-commit and CI levels stay the baseline. See Real constraints.
- [x] **Alternatives in `requires`**: a separate `requires_one_of` field holds groups of alternatives, each satisfied when at least one of its entries is selected, such as `[nextjs, vite-react]` for shadcn/ui. A separate field keeps `requires` a plain list of ids. Unmet groups are resolver errors, one message per group. See The catalog and Resolving against the catalog.
- [x] **MVP categories**: separate radio categories for web, mobile and backend frameworks, for Python and Node.js package managers, for styling and UI components, and for i18n libraries next to the checkbox category of generic i18n rules; databases are radio; rule maintenance is its own radio category. See the catalog's MVP entries section.
- [x] **i18n library per stack**: every website and mobile preset names its i18n library in `i18n_selections`, `i18n-next-intl` for Next.js and `i18n-react-i18next` for Vite + React and Expo, and `i18n-rules` names no library and asks the agent to propose one when none is selected. Stack knowledge stays in the catalog; no category name is added to the code. See the catalog's MVP entries section.
- [x] **Project commands in the spec**: an optional `commands` object with fixed keys `dev`, `build`, `test` and `lint`, each one line. Fixed keys rather than a free map, so a typo is an unknown field and the Commands section always lists them in the same order. A new optional field, so the spec `version` stays 1. See Project spec.
- [x] **Renderer input and output**: renderers take the resolved selection and an `fs.FS` of the existing project, and return files in memory, deterministic and sorted by path; they never write to disk. See Output.
- [x] **Main file layout**: each standard section has a fixed source, catalog topics map to sections, unknown topics get their own section, and each entry is one `rule Why: why` bullet. With both targets, CLAUDE.md imports AGENTS.md and adds only Claude-specific content. See The main file.
- [x] **Protected sections**: `<!-- cyanotype:custom -->` and `<!-- /cyanotype:custom -->` lines; a block returns to the end of the section it was in, and unmatched markers stop generation. See Protected sections.
- [x] **Splitting**: `auto` splits all or nothing when the unsplit main file has more than `max_lines` lines; the main file keeps the overview, language policy, Tech stack with `tooling` entries, commands, safety rules, domain notes and custom rules; every other topic goes to `<topic_dir>/<topic>.md`, read on demand for every target in the MVP. See Splitting into multiple files.
- [x] **Generated constraints**: `.claude/settings.json` deny and ask rules are merged into the existing file, each list on its own, keeping every user key and rule and each entry's rules as an ordered subsequence so negations keep working; the merge is additive. `.gitignore` gets a `# cyanotype:begin` / `# cyanotype:end` block replaced in place, holding the personal files and the selected entries' `enforce.gitignore` lines, merged with the same order guarantee. See Real constraints.
- [x] **`ask` permissions in `enforce`**: `enforce.claude.ask` holds rules that mean "ask first" rather than "never", written to `permissions.ask` with the same order guarantee as deny rules. `dependency-approval` asks before install commands, and `rule-maintenance-propose` asks before edits to `cyanotype.yaml` while still denying edits to generated instruction files. A rule may not be in both lists of an entry, since deny wins. Checked with Claude Code 2.1.299 in enforce-checks.md. See Generating `.claude/settings.json`.
- [x] **Conditional rules**: `enforce.claude.with.<entry id>.ask` holds ask rules that apply only when that entry is selected too. Each tool that installs dependencies carries the ask rule for its own install command under `with.dependency-approval`, so a project gets only the rules of its stack and `dependency-approval` stays stack-agnostic. A nested map rather than an `ask_with` field, so conditional deny rules can be added later without a new field; only `ask` is supported for now. See Generating `.claude/settings.json`.
- [x] **Permission rule syntax**: catalog rules use the space syntax, `Bash(npm *)`, which matched the same commands as the legacy `Bash(npm:*)` in the 2026-10-20 check, cover every command a snippet forbids, and pair each `Bash(...)` rule with a `PowerShell(...)` one. See Real constraints.
- [x] **Generate command**: `cyanotype generate` previews a colored unified diff and a summary, asks before writing, and has `--dir`, `--dry-run` and `--yes`. Without a terminal on stdin it never reads input. Exit codes are 0 done, 1 error, 2 usage and 3 changes not written. Comparing and atomic, root-confined writing live in `internal/generate` for every interface; files differing only in CRLF endings count as unchanged; nothing is deleted. The catalog is embedded with `go:embed` from `catalog/`. See Generating files.
- [x] **Init command**: `cyanotype init` asks the Simple mode questions plus the coding agents, which decide the targets, with a flag for every question. Without a terminal, missing answers are one usage error listing the flags. It shows the preset entry by entry, writes the spec exclusively and never overwrites one, and offers to run generate. Autonomy levels, i18n answers, learning mode and agents map to entries and fields as in Simple mode. See Simple mode and Creating the spec.
- [x] **Detection format**: `detect.files`, `detect.dependencies` with one field per ecosystem (`npm`, `pypi`, `go`), and `detect.run_script` for package managers. Any one match reveals the entry; Python names are written normalized and Go paths match their major versions. Only the project root is read. See Detection.
- [x] **Reading pyproject.toml**: an in-house reader of the tables that list dependencies, rather than a TOML library, which would be a dependency for a few lists of strings; a file it cannot read is a warning. go.mod is read in-house too, rather than with golang.org/x/mod. See Detection.
- [x] **Detection and presets**: on an existing project the stack comes from detection, adjusted by the user, and never gains a technology detection did not find; presets give only the project type, the order, open choices' defaults and the i18n library. Commands come from `package.json` scripts and the package manager's `run_script`. Clashes and unmet `requires_one_of` groups are questions, or missing `--with` flags. init asks before starting from a detected stack, or needs `--detect`. See Detecting the stack.
- [x] **Simple-mode prompts**: standard-library prompts now; charmbracelet/huh with the Phase 2 TUI, which pays for the dependency. Questions and options are data in `internal/simple`, shared by every interface. See Creating the spec.
- [x] **Line diff**: written in-house in `internal/diff` (Myers, unified output) rather than taken from a library. See Generating files.
- [x] **Terminal detection**: `golang.org/x/term` tells whether stdin and stdout are terminals, and `golang.org/x/sys/windows`, which x/term already depends on, turns on ANSI colors in Windows consoles. Both are maintained by the Go team and cover every platform; unlike a check for character devices, x/term reports `/dev/null` as not a terminal.
- [x] **Release tooling**: GoReleaser, configured in `.goreleaser.yaml` and run by a workflow when the maintainer pushes a `v*` tag, in a `release` environment that requires the maintainer's approval. GoReleaser, cosign and the workflow actions are pinned, so an upstream update cannot change a release unannounced. Pull requests that change the release configuration run `goreleaser check` and a snapshot build. Agents never create or push tags. See docs/RELEASING.md.
- [x] **Release channels for v0.1**: GitHub Releases (tar.gz for Linux and macOS, zip for Windows, amd64 and arm64), `go install`, a Homebrew tap (`cyanotype-dev/homebrew-tap`), a Scoop bucket (`cyanotype-dev/scoop-bucket`) and `.deb` and `.rpm` packages. winget, Flathub and the desktop app's packages come later. v0.x releases are GitHub pre-releases.
- [x] **The `cyty` alias**: never a second copy of the binary. A relative symlink in tar archives and in the `.deb` and `.rpm` packages, `bin.install_symlink` in the Homebrew formula, and `cyty.cmd` next to `cyanotype.exe` in Windows zip archives. Scoop and `go install` install `cyanotype` only: GoReleaser builds Scoop's `bin` list from the binaries alone, and a shim created in `post_install` would survive an uninstall; the README shows a PowerShell alias instead.
- [x] **Release integrity**: `checksums.txt` lists the SHA-256 of every archive and package and is signed with cosign keyless signing, through the release workflow's GitHub OIDC identity, into `checksums.txt.sigstore.json`. There is no signing key to store or leak, and a signature names the workflow and tag that made it. The README shows how to verify a download.
- [ ] **Homebrew cask and signed macOS binaries**: the tap uses a formula, which GoReleaser deprecated in v2.10 in favor of casks, because a formula also works on Linux and is not quarantined by Gatekeeper; the release check accepts only this deprecation. Moving to a cask belongs with signing and notarizing the macOS binaries, which removes the need to strip the quarantine attribute that a cask of unsigned binaries would need.
- [ ] **`--json` output for generate**: the MCP server section says the CLI mirrors every MCP tool with `--json`. The JSON of `generate` is a public contract shared with the MCP tools `preview` and `generate`, so it is designed together with them; `generate.Change` (path, status, old and new content, diff) is the shape it will encode.
- [ ] **Pinning `catalog_version` on generate**: a spec without `catalog_version` could get the loaded catalog's version written back on generation, so regeneration is reproducible. `generate` never changes the spec today: writing it is a choice of its own, and would need the spec in the diff too.
- [ ] **Tracked files that should be ignored**: `cyanotype check` should warn when a file matched by an `enforce.gitignore` line of a selected entry is already tracked by git, such as a committed `.env`. `.gitignore` does not untrack a file, so the generated block alone gives a false sense of safety in a repository where the secret was committed before. The warning should name the file and say that untracking it (`git rm --cached`) does not remove it from the history, so a committed secret must also be rotated.
- [ ] **Generated-state file**: a file such as `cyanotype.lock` recording the files and the permission rules cyanotype produced. Without it, generated output cannot be told apart from what people wrote, which leaves three known limits: permission rules of deselected entries, or of rules a newer catalog moved from deny to ask, stay in `.claude/settings.json`, topic files from an earlier split stay on disk, and `cyanotype check` cannot detect generated files edited by hand. One record solves all three. Not implemented yet.
- [ ] **Category metadata**: categories need their own metadata — display name, UI order, whether a choice is required, whether "Not decided yet" is offered — probably in a `catalog/categories.yaml`. Until then a category exists only as a string in its entries.
