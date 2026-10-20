# Convert AGENTS.md and CLAUDE.md into cyanotype.yaml

This project has instruction files for coding agents written by hand: AGENTS.md and CLAUDE.md. Cyanotype generates these files from cyanotype.yaml, which records choices from its catalog and the project's own rules. Your task: update cyanotype.yaml, which already exists, so that the files cyanotype generates from it carry the same rules as AGENTS.md and CLAUDE.md.

## Steps

1. Read AGENTS.md and CLAUDE.md completely.
2. For each rule, look for an equivalent entry in the catalog list below. When there is one, add its id to `selections` and do not repeat the rule anywhere else. An entry is equivalent when its summary says the same thing; a rule that is stricter or narrower than the entry is not, and becomes a custom rule.
3. Turn every rule with no equivalent entry into an item of `custom_rules`, with `rule` and `why`. Take the reason from the file. When the file gives none, ask me for it; never invent a reason.
4. Move the commands the file gives for running, building, testing and linting the project to `commands`, which has only the keys `dev`, `build`, `test` and `lint`.
5. Move the description of the project to `project`: `goal` in one or two sentences, and `users`, `core_features` and `out_of_scope` where the file says them. Business rules no entry covers go in `answers.domain_notes`.
6. Invent nothing: no entry, command, reason, feature or language the files or the detected stack below do not support. When the files and the detected stack disagree, ask me.
7. Do not edit, move or delete AGENTS.md and CLAUDE.md. Write only cyanotype.yaml.
8. Check the result with `cyanotype generate --dry-run --existing keep`, which reports any error in cyanotype.yaml with its line and writes nothing; exit code 3 means the spec is valid. Fix every error it reports.
9. End your reply with a list of everything in AGENTS.md and CLAUDE.md you could not convert, each with the reason, then tell me to review cyanotype.yaml and run `cyanotype generate --existing replace` to replace AGENTS.md and CLAUDE.md with the generated files.

## The current cyanotype.yaml

Keep its choices unless the instruction files contradict them, and keep its comments.

```yaml
version: 1
targets: [claude, agents-md]
project:
  name: shop
  goal: An online shop.
```

## The cyanotype.yaml format

An example with every field the conversion needs:

```yaml
version: 1
catalog_version: "2026.10"
targets: [claude, agents-md]   # claude writes CLAUDE.md, agents-md AGENTS.md
project:
  name: shop                   # one line
  goal: Online shop for handmade furniture.   # required, one or two sentences
  users: Furniture buyers, mostly on mobile.  # optional
  core_features:               # optional
    - Browse and buy products
  out_of_scope:                # optional
    - A marketplace for other sellers
languages:                     # optional; each is english by default
  agent_replies: italian
commands:                      # optional; only these four keys, each one line
  dev: pnpm dev
  build: pnpm build
  test: pnpm test
  lint: pnpm lint
selections:                    # catalog entry ids, in the order their rules should appear
  - typescript
  - node
  - pnpm
  - no-test-tampering
answers:
  domain_notes: Prices include VAT.   # optional: business rules no entry covers
custom_rules:                  # rules with no equivalent entry
  - rule: Store prices in cents, as integers.
    why: Floating-point sums lose cents.
```

Rules:

- `version`, `targets`, `project.name` and `project.goal` are required. Keep `version: 1` and `catalog_version` as they are.
- `targets` lists the agents the files are for: `claude` for CLAUDE.md, `agents-md` for AGENTS.md, which Codex and most other agents read.
- Unknown fields are errors. Leave out fields with nothing to say instead of writing them empty.
- An entry's requirements must be selected too: the catalog list says what each entry requires.
- Every custom rule needs both `rule` and `why`. Write the rule as an instruction to the agent, and the reason as one short sentence.

## Catalog entries

Each line is `id`: name, then what the entry makes the agent do. Only these ids can go in `selections`. Entries marked radio are exclusive within their category: select at most one of them.

### agent-safety (checkbox: any number)

- `no-test-tampering`: Never weaken tests to make them pass. The agent may not edit or delete tests to make them pass.

### backend-framework (radio: at most one)

- `fastapi`: FastAPI. Python web framework for APIs. Requires `python`.

### frontend-framework (radio: at most one)

- `nextjs`: Next.js. React framework for full-stack web apps. Requires `node`.

### language (checkbox: any number)

- `python`: Python. General-purpose language.
- `typescript`: TypeScript. JavaScript with static types.

### package-manager (radio: at most one)

- `npm`: npm. The package manager bundled with Node.js. Requires `node`.
- `pnpm`: pnpm. Fast, disk-efficient package manager. Requires `node`.

### runtime (radio: at most one)

- `node`: Node.js. JavaScript runtime. Requires one of `npm` or `pnpm`.
