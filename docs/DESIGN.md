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

The core reads the catalog and the spec files, resolves compatibility and conflicts, and generates both instruction files and real constraints. It also writes `cyanotype.yaml` back when choices change.

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
| `level` | no | Lets simple mode prefer beginner-friendly entries |
| `topic` | no | Which split file the snippet goes to when output is split |
| `snippet.rule`, `snippet.why` | yes | The rule and its reason |
| `enforce` | no | Real constraints to generate (permissions, hooks, config files) |
| `detect` | no | Files or dependencies that reveal the entry in an existing project |

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
    deny: ["Bash(npm install:*)", "Bash(yarn:*)"]
detect:
  files: [pnpm-lock.yaml]
```

References in `requires` and `conflicts` are plain entry ids, never qualified by category. Ids are unique across the whole catalog, so a qualifier adds nothing, and moving an entry to another category must not break the entries that point to it.

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

A GitHub Actions check validates every pull request to the catalog (schema, consistent `requires` and `conflicts`) and periodically verifies that every `url` still responds.

The catalog, including snippets and presets, is released under CC0-1.0, separately from the Apache-2.0 code. Snippets are copied into users' instruction files, and those files must carry no attribution or license notice.

### Distribution

The catalog ships inside every release and is updated only on explicit request, never automatically. Its `enforce` field generates hooks and config that run in users' projects, so a compromised catalog pushed automatically would reach every project within hours.

1. **Bundled snapshot.** Each release embeds a catalog copy, so the tool works immediately and offline.
2. **Versioned, signed releases.** The catalog has its own date-based versions, such as `2026.10`, published as signed GitHub releases with checksums.
3. **Explicit updates.** `cyanotype catalog update` downloads a newer version, verifies its signature and shows what changes before switching. The tool may notify about a new version at most once a day and never applies it on its own; notifications can be turned off for CI and offline use.
4. **Pinned per project.** `cyanotype.yaml` records the catalog version it was generated with, so regeneration is reproducible. `cyanotype update` moves a project to a newer version with a diff, and blueprints rely on the same pin.
5. **Additional catalogs.** Organizations can add their own catalog as a second source next to the official one, for internal rules and private presets.

Phase 1 ships the bundled snapshot only; signed updates and additional catalogs arrive in Phase 2.

### MVP entries

The first release ships 40 entries: one solid choice per simple-mode project type, plus every agent safety and workflow rule, which work with any stack and carry the most value. Simple mode covers website, mobile app, API and script; desktop app and game appear as coming later.

| Category | Entries |
| --- | --- |
| Languages | TypeScript, Python, Go |
| Runtimes and package managers | Node.js, pnpm, npm, uv |
| Frameworks | Next.js, Vite + React, Expo, FastAPI, Go standard library |
| UI | Tailwind CSS, shadcn/ui |
| Databases | PostgreSQL, SQLite, Supabase |
| Testing | Vitest, Playwright, pytest |
| Lint and format | ESLint with Prettier, Ruff, golangci-lint |
| Internationalization | Generic i18n rules, next-intl |
| Git | Conventional Commits, one branch per task |
| Agent safety | No destructive commands, no test tampering, secrets only in `.env`, no unrequested changes, new dependencies only with approval, no duplicate files |
| Agent workflow | Plan before code, verifiable definition of done, small commits, rule maintenance |
| Learning | Learning mode: explain, guided, hands-on |

Starter presets built from these entries: Next.js + Supabase, Vite + React, Expo app, FastAPI + PostgreSQL, Python script with uv, Go command-line tool.

Deferred, and good first contributions once the format is proven: Vue, Svelte, Angular, Django, Rails, Rust, Flutter, Java and Kotlin, MongoDB, desktop and game stacks.

The quality of each snippet and its reason matters more than the number of entries.

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

selections:
  - typescript
  - nextjs
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

From these answers the tool picks a preset, shows it with a short explanation of each component and offers to switch to advanced mode to inspect or change the details.

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
- **Project commands**: dev, build, test, lint, pre-filled when detected and editable.

## Output

Each agent has a renderer that turns the same choices into its own format. AGENTS.md is the main file when the user targets several agents; the others import or mirror it.

| Agent | Generated file | Note |
| --- | --- | --- |
| Claude Code | `CLAUDE.md` | With several targets it holds `@AGENTS.md` plus any Claude-only rules |
| Codex and compatible agents | `AGENTS.md` | The shared format, used as the main source |
| Gemini CLI | `GEMINI.md` | Same import logic as Claude |
| Cursor | `.cursor/rules/*.mdc` | One file per topic, with frontmatter for scope |
| GitHub Copilot | `.github/copilot-instructions.md` | Compact version |

Formats change often: every renderer is checked against the official docs and covered by snapshot tests.

### Splitting into multiple files

When the main file would grow past a threshold, the content is split by `topic` into a short main file plus topic files under `docs/agents/` (testing, style, git, architecture, i18n…). The behavior is set by `output.split`:

- `auto` (default): split only when the main file would exceed `max_lines` (default 100).
- `always`: the "Always split" checkbox in the UI; useful for teams that want one file per area from day one.
- `never`: everything in one file, with a warning when it gets long.

The topic folder defaults to `docs/agents/`, visible so that human contributors read the same conventions and agent search tools, which often skip hidden folders, find them. Each repo can change it with `output.topic_dir`. When the tool detects a documentation site generator such as MkDocs or Docusaurus, it warns that the folder could be published and offers to exclude it.

The main file always keeps the project goal, commands and safety rules, which the agent must never have to go looking for. Each topic file then reaches the agent in one of three ways, chosen per topic:

| Mode | How | Context cost | When to use |
| --- | --- | --- | --- |
| Imported | `@docs/agents/testing.md` in CLAUDE.md or GEMINI.md | Always loaded | Rules needed in almost every task; organizes, does not save context |
| On demand | *Before writing tests, read docs/agents/testing.md.* | Loaded when relevant | Long guides used in specific tasks |
| Path-scoped | `.claude/rules/` with `paths:`, Cursor globs, nested AGENTS.md | Loaded when matching files are touched | Rules for one part of the codebase (frontend, migrations) |

Only the last two actually reduce what the agent reads; importing is about tidiness. The preview shows the resulting layout and the line count of each file.

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
| Propose (default) | Never edits instruction files. When it learns a convention or is corrected twice on the same point, it proposes a rule and a reason; the user approves | Most projects |
| Self-update | May add `custom_rules` to `cyanotype.yaml` and regenerate, mentioning it in the commit message | Solo projects, high autonomy |
| Off | Rules change only by hand | Strictly controlled repos |

Generated files open with a header saying they come from `cyanotype.yaml` and where manual edits belong. With the Propose policy, a deny rule on generated files backs this up.

An optional decision log, `docs/agents/decisions.md`, lets the agent record architectural choices as date, decision and reason. The main file references it on demand, so it costs no context until needed.

### What the tool checks

`cyanotype check` detects drift and runs locally, as a pre-commit hook or in CI:

- dependencies or config files found in the repo but missing from the spec (via `detect`);
- generated files edited by hand or out of sync with the spec;
- a newer catalog with changes to selected entries;
- files over the line threshold, and references to files that no longer exist.

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

- Never hardcode user-facing text; every string goes through the i18n library with a key.
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

- **Stack detection.** On an existing project it reads `package.json`, `pyproject.toml`, `go.mod`, lockfiles and config files through the catalog's `detect` field, and pre-selects what it finds. The user confirms instead of typing.
- **Presets.** Ready combinations ("Next.js + Supabase", "FastAPI + Postgres", "Simple Python script") stored as ordinary `cyanotype.yaml` files. Community presets arrive by pull request.
- **Protected sections.** Hand-written text inside markers such as `<!-- cyanotype:custom -->` survives regeneration.
- **Live preview** in TUI and desktop, updated on every selection.
- **Sharing.** Export a configuration as a file or a compact string, import it in one step.
- **Multiple agents at once**, generating every target in a single run.
- **Link checking** of catalog URLs on a schedule in GitHub Actions.

## Roadmap

The MVP is the CLI with simple mode: it proves the value with the least work, and everything it builds in the core carries over to the TUI, MCP server and desktop app.

&#91;embedded content: roadmap · 4 phases, 3 releases\]

Phases have no dates yet. The catalog grows alongside every phase, including outside contributions after v0.1, and Linux, macOS and Windows builds ship from the first release.

## Decisions

- [x] **Language**: English everywhere — interface, generated files, documentation, code, comments and commits.
- [x] **Core language.** Go, with Bubble Tea and Huh for the TUI and Wails for the desktop. Chosen over Rust and TypeScript for single-binary distribution, the Charm terminal ecosystem and a low barrier for contributors.
- [x] **Name: Cyanotype, command cyanotype with the short alias cyty (checked free on Homebrew, Debian, Arch, AUR, PyPI, crates.io and npm). Available: GitHub organization cyanotype-dev and domain cyanotype.dev (cyanotype, cyanotype-ai and cyty are taken on GitHub; cyanotype.com, .io and .app are registered). Trademarks checked on TMview (6 results): none in classes 9 or 42 in the EU, Italy or the US; class 42 marks exist only in Japan (cyanotype) and the UK (Cyanotype Media). GitHub organization cyanotype-dev created; Go module path github.com/cyanotype-dev/cyanotype. Next step: register cyanotype.dev**.
- [x] **License**: Apache-2.0 for the code, for its explicit patent grant and contribution terms; CC0-1.0 for the catalog, so generated files carry no license obligations. User blueprints use the license in their own manifest.
- [x] **Catalog distribution**: bundled snapshot plus explicit, signed updates, pinned per project. See the catalog's Distribution section.
- [x] **MVP catalog entries**: 40 entries and 6 starter presets. See the catalog's MVP entries section.
- [x] **Topic folder**: `docs/agents/` by default, configurable per repo with `output.topic_dir`; personal topic files in `.agents.local/`, configurable per person. See Output and Team rules.
- [x] **Catalog references**: `requires` and `conflicts` hold plain entry ids; exclusivity inside a radio category is implicit and `conflicts` only links entries of different categories. See The catalog.
- [x] **Catalog layout**: one flat folder, `catalog/entries/<id>.yaml`, with the file name equal to the id. The category lives only in the entry.
- [x] **Catalog YAML parsing**: `go.yaml.in/yaml/v3` with unknown fields rejected. The JSON Schema in `schema/` serves editor autocompletion; runtime validation is Go code, which also checks references across entries.
- [x] **Catalog select per category**: all entries of a category share one `select` value, enforced by validation until category metadata exists.
- [x] **Spec format**: `cyanotype.yaml` fields, required fields and defaults as in the Project spec section; defaults live in code, never in the file. Locked selections go in a top-level `locked` list so `selections` stays a plain list of ids.
- [x] **Spec versioning**: an integer `version`, bumped only for breaking changes, with migrations on the YAML tree; newer versions are rejected.
- [x] **Spec write-back**: merge into the original YAML tree to keep comments and order, then write a canonical layout (blank line between top-level keys, flow sequences over 80 characters as block lists).
- [x] **Spec validation boundary**: `internal/spec` validates structure only; checks that need the catalog, and the merge with `cyanotype.local.yaml`, belong to `internal/resolve`.
- [ ] **Category metadata**: categories need their own metadata — display name, UI order, whether a choice is required, whether "Not decided yet" is offered — probably in a `catalog/categories.yaml`. Until then a category exists only as a string in its entries.
