# Checking the catalog's Claude permission rules

Catalog entries generate permission rules for `.claude/settings.json` through `enforce.claude.deny` and `enforce.claude.ask`. Those rules are only worth shipping if they block, or ask about, what the snippet says, and Claude Code decides that at run time, inside the agent, so `go test` cannot check it. This page is the manual procedure for checking it, with the result of every run.

Run it again when an entry's permission rules change, and when a new Claude Code release changes how permissions are matched.

## Procedure

1. **Record the environment.** Note the version of the Claude Code that runs the session, which can differ from `claude --version` on the `PATH`. In the desktop app, `CLAUDE_CODE_EXECPATH` and `AI_AGENT` name the running version. Also note the operating system and the permission mode, since some modes skip prompts.
2. **Back up `.claude/settings.local.json`** outside the repository, so it can be restored byte for byte.
3. **Create dummy files only.** Use content that is obviously fake, such as `DUMMY_ROOT=start`. Before creating a file, check that no file with that name exists, so a real one is never overwritten or deleted. Cover both the project root and a subfolder.
4. **Control run.** With no rules added, run every case below and confirm it succeeds. A case that fails here tells nothing about the rules.
5. **Add the rules** under test to the `deny` list of `.claude/settings.local.json`.
6. **Run each case in its own tool call.** A denied command blocks the whole call, so two cases in one call hide each other's results. The Read tool skips a file that has not changed since the session last read it ("file unchanged"); change the file's content before reading it again, or the check never reaches the permission system.
7. **Restore and clean up.** Copy the backup back, confirm it is identical with `cmp`, delete the dummy files, and check that `git status` is clean.
8. **Record the results** in a new section below. Write down failures too, not only what passed.

### Cases for `secrets-in-env`

Rules: `Read(.env)`, `Read(.env.*)`, `Read(!.env.example)`, `Edit(.env)`, `Edit(.env.*)`, `Edit(!.env.example)`, in this order. A negation only carves paths out of the rules listed before it, so each negation must come after the rules it narrows. Expected: `.env` and `.env.*` blocked everywhere, for reads and writes; `.env.example` readable and writable.

- Read tool: `.env` and `.env.local` at the root, `.env` in a subfolder, `.env.example`.
- Edit and Write tools: edit `.env`, overwrite `.env` and `.env.local`, create `.env.production`, edit `.env.example`.
- Shell reads, in Bash: `cat`, `head`, `tail`, `sed -n 1p`, `grep PATTERN .env`, `awk`, `wc -l < .env`, `cat ./.env`, an absolute path, `base64`, `git diff --no-index /dev/null .env`, `cp .env other`. Also try ways of reaching the file without naming it: a glob such as `cat .en*`, a command substitution such as `cat "$(echo .env)"`, `source .env`, `grep -r PATTERN .` and a script, such as `python -c "print(open('.env').read())"`.
- Shell reads, in PowerShell: `Get-Content .env`, the alias `gc .env.local`, `Get-Content .env.example`, `Get-Content .en*`.
- Shell writes: `>> .env`, `> .env.production`, `tee -a .env`, `sed -i`, `>> .env.example`, a Python script appending to `.env`, PowerShell `Add-Content` and `Set-Content`.

### Cases for package managers

Choose commands that do nothing if a rule fails to block them: `--help`, `--version`, and bare commands only in a dummy folder with an empty `package.json`. A package manager that is not installed still tells the two outcomes apart: a blocked command is refused before it runs, while one that is not blocked runs and fails with "command not found".

- Each command the snippet forbids, with each subcommand that installs (`npm i`, `install`, `ci`, `add`, `run`), and bare.
- Commands that must stay allowed, such as `npx` next to `npm *` and `pip --version` next to `pip install *`.
- Ways around a pattern: a compound command (`cd sub && npm --version`), `command npm`, `npm.cmd`, an option before the subcommand (`pip -q install`), and in PowerShell the call operator (`& npm`).
- Each case in Bash and in PowerShell.

### Cases for `ask` rules

An ask rule shows a permission prompt, but so does any command the session has not allowed, so a prompt alone proves nothing. Add an `allow` rule that covers each case first, such as `Bash(npm *)`, and check in the control run that the case runs without a prompt. Then add the ask rules, and have the user answer **No** to every prompt: a refusal shows that the prompt appeared, while an approval runs the command and looks the same as no prompt at all.

- An ask rule over an allow rule, in Bash and PowerShell.
- A command with no ask rule next to one that has it (`npm ci` next to `npm install *`).
- A bare command against a pattern ending in ` *`.
- The same rule in deny and ask.
- `Edit` ask rules with the Edit and Write tools, in accept-edits mode.

### Cases for `cyanotype select`

Rules: the `ask` rules `Bash(cyanotype select *)`, `Bash(cyty select *)`, `Bash(cyanotype -* select *)`, `Bash(cyty -* select *)` and the same four as `PowerShell(...)` rules, of `rule-maintenance-propose`. Expected: every way of running `cyanotype select` asks, and other cyanotype commands do not.

Put a build of the branch on the `PATH` under both names, `cyanotype` and `cyty`, and use a dummy project folder outside the repository holding a valid `cyanotype.yaml`, so a case that is not blocked changes only that file. The allow rules for the control run are `Bash(cyanotype *)`, `Bash(cyty *)`, `PowerShell(cyanotype *)` and `PowerShell(cyty *)`. Answer **No** to every prompt.

- `cyanotype select --dir <dummy> --add small-commits --dry-run`, and the same with `--yes`.
- `cyanotype select` with no arguments, against the pattern ending in ` *`.
- `cyty select --dir <dummy> --add small-commits --dry-run`.
- Ways around the pattern: a compound command (`cd <dummy> && cyanotype select --add small-commits --dry-run`), `cyanotype.exe select ...`, a flag before the subcommand (`cyanotype --dir <dummy> select ...` and `cyanotype --dir=<dummy> select ...`), and in PowerShell the call operator (`& cyanotype select ...`).
- Commands that must not ask: `cyanotype spec --dir <dummy>`, `cyanotype --dir <dummy> spec`, `cyanotype validate --dir <dummy>`, and `cyanotype catalog search select`, which names select only as a search term.
- Each case in Bash and in PowerShell.

### Cases for the `set_selections` MCP tool

Rule: the `ask` rule `mcp__cyanotype__set_selections` of `rule-maintenance-propose`. Expected: every call of `set_selections` asks, with or without `write`, and the server's other tools do not.

Build the branch, and add the binary to Claude Code as a stdio server named `cyanotype`, confined to a dummy project folder outside the repository that holds a valid `cyanotype.yaml`: `claude mcp add cyanotype -- <binary> mcp --allow <dummy>`. Start a new session, so it connects to the server, and check with `/mcp` that `cyanotype` is connected. The allow rule for the control run is `mcp__cyanotype__*`. Answer **No** to every prompt.

- `set_selections` with `add: ["small-commits"]` and no `write`, which writes nothing.
- `set_selections` with the same `add` and `write: true`.
- `set_selections` with no arguments, a usage error, against a rule with no pattern.
- Tools that must not ask: `search_catalog`, `get_spec`, `validate`, `preview`, and `generate` without `write`.
- The same server added a second time under another name, such as `cyanotype-other`: its `set_selections` is expected not to ask, since the rule names the server. README and DESIGN.md say to adjust the rule for another name; this case confirms why.

### Other entries

Use the same method for the other entries with permission rules. Choose cases that would do no harm if a rule failed: delete a dummy folder with `rm -rf`, delete a dummy branch with `git branch -D`, and add `--dry-run` to force pushes.

| Entry | Rules | Last checked |
| --- | --- | --- |
| `secrets-in-env` | `Read` and `Edit` rules on `.env` files | 2026-10-19, see below |
| `no-destructive-commands` | `Bash` and `PowerShell` command patterns | Not yet |
| `rule-maintenance-propose` | `Edit` deny rules on generated files; `ask` rules on `cyanotype.yaml`, `cyanotype select` and the `set_selections` MCP tool | `Edit(/cyanotype.lock)` on 2026-10-22, see below; the other deny rules not yet; the `ask` rule on `cyanotype.yaml` on 2026-10-20, those on `cyanotype select` on 2026-10-23; the one on `set_selections` not yet |
| `npm`, `pnpm`, `uv` | `Bash` and `PowerShell` rules on the other package managers | 2026-10-20, see below |
| `npm`, `pnpm`, `uv`, `go`, `expo` | `Bash` and `PowerShell` ask rules on their install commands, under `with.dependency-approval` | 2026-10-20 for `npm install` and `go get` in both shells and `npm i` in Bash; not yet for the others |

## Run of 2026-10-19: `secrets-in-env`

**Environment:**
- Claude Code 2.1.299, running in the Claude desktop app 2.42304.2 on Windows 11. The `claude` CLI on the `PATH` was 2.1.303 and was not the one tested.
- The session was in bypass permissions mode; the deny rules were enforced anyway.
- An earlier, smaller run in default permission mode (Read tool only, subfolder only, same Claude Code version) gave the same Read results.

In the control run, every case below succeeded.

Phase A uses the three `Read` rules the entry shipped with at the time. Phase B adds `Edit(.env)`, `Edit(.env.*)` and `Edit(!.env.example)`, to see whether they close the gaps found in phase A. They did, so the entry now ships the phase B rules.

| Case | A: Read rules | B: Read + Edit rules |
| --- | --- | --- |
| Read tool: `.env`, `.env.local` (root), `.env` (subfolder) | Blocked | |
| Read tool: `.env.example` | Allowed, as intended | |
| Edit and Write tools: `.env`, `.env.local`, new `.env.production` | Blocked | |
| Edit tool: `.env.example` | Allowed, as intended | Allowed, as intended |
| Bash: `cat`, `head`, `tail`, `sed -n`, `grep` naming the file, `awk`, `wc -l < .env`, `./.env`, absolute path, `base64`, `git diff --no-index`, `cp` | Blocked | |
| Bash: `cat .env.local`, `cat` on the subfolder `.env` | Blocked | |
| Bash: `cat .env.example` | Allowed, as intended | |
| Bash: glob `cat .en*` | **Not blocked**: printed `.env`, `.env.local` and `.env.example` | |
| Bash: command substitution `cat "$(echo .env)"` | **Not blocked** | |
| Bash: `source .env` | **Not blocked** | |
| Bash: `grep -r PATTERN .` | **Not blocked** | |
| Bash: Python script reading `.env` | **Not blocked** | |
| PowerShell: `Get-Content .env`, `gc .env.local` | Blocked, with a message about allowed working directories | |
| PowerShell: `Get-Content .env.example` | Allowed, as intended | |
| PowerShell: glob `Get-Content .en*` | **Not blocked** | |
| Bash: `>> .env`, `> .env.production` (new file), `tee -a .env` | **Not blocked** | Blocked |
| Bash: `sed -i ... .env` | Blocked | |
| Bash: `>> .env.example` | Allowed, as intended | Allowed, as intended |
| Bash: Python script appending to `.env` | **Not blocked** | **Not blocked** |
| PowerShell: `Add-Content .env` | **Not blocked** | Blocked |
| PowerShell: `Set-Content .env.local` | | Blocked |

An empty cell in column B means the case was not repeated, because Edit rules do not affect reads.

**Findings:**

- The negation works: `Read(!.env.example)` keeps `.env.example` readable and editable, while `.env` and every other `.env.*` file stay blocked, at the root and in subfolders.
- `Read` deny rules block the Read, Edit and Write tools and every shell command that names the file. They do not block commands that reach the file without naming it: globs, command substitution, `source`, recursive `grep` and scripts.
- `Read` deny rules alone do **not** stop shell writes: redirections, `tee` and PowerShell `Add-Content` wrote to `.env` and created `.env.production`. Adding matching `Edit` deny rules blocked all of these, so `secrets-in-env` now ships both kinds of rules.
- No permission rule stops a script that opens the file itself. Only the sandbox can, at the operating-system level, as the Claude Code documentation says.

## Run of 2026-10-20: package managers and `ask` rules

**Environment:**
- Claude Code 2.1.299 (`CLAUDE_CODE_EXECPATH`), running in the Claude desktop app on Windows 11. The `claude` CLI on the `PATH` was 2.1.304 and was not the one tested.
- The session was in accept-edits mode.
- npm 11.13.0, pip 26.2.1 (Python 3.14) and Go were installed; pnpm, yarn and uv were not, so for them "Not blocked" would have shown as "command not found".

In the control run, every installed command ran and pnpm, yarn and uv failed with "command not found". The rules came from the proposed entries, except `Bash(pnpm:*)`, written in the legacy colon syntax to compare it with the space syntax.

**Deny rules** (`Bash(npm *)`, `Bash(yarn *)`, `Bash(pnpm:*)`, `pip install` in six spellings, and `PowerShell(...)` versions):

| Case | Result |
| --- | --- |
| Bash: `npm --version`, `npm i`, `install`, `ci`, `add`, `run` (all with `--help`), bare `npm` | Blocked |
| Bash: `cd sub && npm --version`, `command npm --version` | Blocked |
| Bash: `npx --version` | Allowed, as intended |
| Bash: `npm.cmd --version` | **Not blocked** |
| Bash: `yarn --version`, `yarn add --help`, bare `yarn` (`Bash(yarn *)`) | Blocked |
| Bash: `pnpm --version`, `pnpm add --help`, bare `pnpm` (legacy `Bash(pnpm:*)`) | Blocked |
| Bash: `pip install`, `pip3 install`, `python -m pip install`, `python3 -m pip install`, `py -m pip install`, `uv pip install` (all with `--help`), bare `pip install` | Blocked |
| Bash: `pip --version` | Allowed, as intended |
| Bash: `pip -q install --help` | **Not blocked** |
| PowerShell: `npm --version`, `npm i --help`, `yarn --version`, `pnpm --version`, `npm.cmd --version`, `& npm --version` | Blocked |
| PowerShell: `npx --version` | Allowed, as intended |
| PowerShell: `pip install`, `python -m pip install`, `py -m pip install` (with `--help`) | Blocked |
| PowerShell: `pip3 install --help`, with no `PowerShell(pip3 install *)` rule | Not blocked, as expected: each spelling needs its own rule |

**Ask rules** (`Bash(npm install *)`, `Bash(npm i *)`, `Bash(npm ls *)`, `Bash(go get *)`, `Bash(yarn add *)`, `PowerShell(npm install *)`, `PowerShell(go get *)`, `Edit(/cyanotype.yaml)`, plus deny `Bash(yarn *)`; allow rules `Bash(npm *)`, `PowerShell(npm *)`, `Bash(go get *)` and `PowerShell(go *)` for the control):

| Case | Control: allow only | With ask rules |
| --- | --- | --- |
| Bash: `npm install --help`, `npm i --help` | Ran, no prompt | Prompt |
| Bash: `npm ci --help` (no ask rule) | Ran, no prompt | Ran, no prompt |
| Bash: bare `npm ls` against `Bash(npm ls *)` | Ran, no prompt | Prompt |
| Bash: `go get --help` | Ran, no prompt | Prompt |
| Bash: `yarn add --help`, in deny and ask | | Blocked, no prompt |
| PowerShell: `npm install --help` | Ran, no prompt | Prompt |
| PowerShell: `go get --help` | Not run; covered by the existing allow rule `PowerShell(go *)` | Prompt |
| Edit tool on `cyanotype.yaml` | Ran, no prompt | Prompt |
| Write tool on `cyanotype.yaml` | | Prompt |
| Bash: `printf ... >> cyanotype.yaml` | Prompt | Not checked |

**Findings:**

- The space syntax `Bash(npm *)` and the legacy `Bash(pnpm:*)` behave the same: both match the bare command and every subcommand. The entries now use the space syntax, like the others.
- `X *` matches only the whole word `X`: `Bash(npm *)` does not match `npx`, and a pattern ending in ` *` also matches the bare command. So `Bash(npm install *)` also asks for a plain `npm install`, which only restores the lockfile.
- Claude Code checks each part of a compound command and strips `command`: neither gets around a rule.
- In Bash, `npm.cmd` gets around `Bash(npm *)`; in PowerShell, `PowerShell(npm *)` also blocks `npm.cmd` and `& npm`. Options placed before the subcommand, such as `pip -q install`, get around patterns that name the subcommand.
- Ask wins over allow, and deny wins over ask, so an entry's rule must not be in both of its lists, and a package manager's deny rules win over another tool's install ask rules.
- An `Edit` ask rule prompts for the Edit and Write tools even in accept-edits mode. Whether it also prompts for shell writes could not be told apart with this method, since the shell write prompted without it too.

## Change of 2026-10-20: install ask rules moved to each tool

The install ask rules moved from `dependency-approval` to the entry of each tool, under `enforce.claude.with.dependency-approval`, so a project only gets the rules of its own stack. The patterns did not change, and `yarn add` was dropped, since yarn is not in the catalog. No new run was made: which rules are generated is renderer logic that `go test` covers, while how Claude Code matches a pattern depends only on the pattern.

As ask rules, the run of 2026-10-20 checked `npm install *` and `go get *` in Bash and PowerShell, and `npm i *` in Bash. `npm i *` in PowerShell, `npm add *`, `pnpm add *`, `uv add *` and `npx expo install *` have the same form but have not been checked as ask rules; the table under Other entries says so.

## Run of 2026-10-20: tag rules in the repository settings

These rules are in this repository's own `.claude/settings.json`, not in the catalog. They back the rule that only the maintainer creates and pushes tags: a pushed `v*` tag starts the release workflow, which publishes to every channel. The `release` environment's required reviewer is the real guard, since it also stops a tag pushed by mistake from outside an agent; these rules keep agents from getting that far. The same procedure applies.

**Environment:**
- Claude Code 2.1.299 (`CLAUDE_CODE_EXECPATH`), running in the Claude desktop app on Windows 11. The `claude` CLI on the `PATH` was 2.1.304 and was not the one tested.
- The session was in auto mode.
- Git 2.45.1 for Windows and GitHub CLI 2.102.0. `.claude/settings.local.json` allows `PowerShell(git *)` and `Bash(git push *)`, so the run also shows deny winning over allow.

Every case is harmless if a rule fails: tags are only listed, created on a ref that does not exist, or deleted when they do not exist, and every push is a `--dry-run` of a tag that does not exist. In the control run every case ran: listing printed nothing, the tag commands failed on the missing ref or tag, the dry runs reported "Everything up-to-date" or "src refspec does not match any", and `gh release create --help` printed its help. A first choice for creating a tag, on the all-zero object id, exited 0 without creating anything and was replaced by a ref name that does not exist. No tag existed before or after the run.

**Rules** (`Bash(...)` and `PowerShell(...)` versions of each): `git tag *`, `git push *--tags*`, `git push *--follow-tags*`, `git push *--mirror*`, `git push *refs/tags/*`, `git push * v*`, `gh release *`. After the first pass, `git push *:v*` and `git push *+v*` were added for the gap found in it.

| Case | Bash | PowerShell |
| --- | --- | --- |
| `git tag -l`, bare `git tag` | Blocked | Blocked (`git tag -l`) |
| `git tag v0.0.0-dummy does-not-exist-dummy` | Blocked | Blocked |
| `git tag -d nonexistent-dummy` | Blocked | Blocked |
| `cd docs && git tag -l` | Blocked | |
| `& git tag -l` | | Blocked |
| `git.exe tag -l` | **Not blocked** | Blocked |
| `git -C . tag -l` | **Not blocked** | **Not blocked** |
| `git push --dry-run origin --tags` | Blocked | Blocked |
| `git push --dry-run --follow-tags origin chore/release-v0.1` | Blocked | Blocked |
| `git push --dry-run --mirror origin` | Blocked | Blocked |
| `git push --dry-run origin v0.0.0-dummy` | Blocked | Blocked |
| `git push --dry-run origin refs/tags/dummy` | Blocked | Blocked |
| `git push --dry-run origin --delete v0.0.0-dummy` | Blocked | |
| `git push --dry-run origin :v0.0.0-dummy` | **Not blocked** in the first pass; blocked with `git push *:v*` | Blocked |
| `git push --dry-run origin +v0.0.0-dummy`, `HEAD:v0.0.0-dummy` | Blocked | |
| `gh release create --help` | Blocked | Blocked |
| `git push --dry-run origin chore/release-v0.1` | Allowed, as intended | Allowed, as intended |
| `git describe --tags --always` | Allowed, as intended | |

**Findings:**

- `git tag *` blocks every use of `git tag`, listing included; `git describe --tags` still reads tags.
- A tag pushed by name, with `--tags`, `--follow-tags`, `--mirror` or a `refs/tags/` refspec is blocked. A deleting refspec, `:v0.1.0`, got around `git push * v*`, which needs a space before the `v`; `git push *:v*` and `git push *+v*` close it and the forced form.
- The rules are broader than tags in two places: `git push * v*` also blocks pushing a branch whose name starts with `v`, which no branch prefix of this repository does, and `gh release *` also blocks reading releases with `gh release view` or `list`.
- As with `npm.cmd`, `git.exe` gets around the `Bash` rules, while `PowerShell(git tag *)` also blocks `git.exe` and the call operator.
- `git -C <dir>` before the subcommand gets around the rules in both shells, like an option before the subcommand of a package manager. Tags without a `v` prefix, pushes through `gh api` and a `push.followTags` setting in git config are not covered either. The required reviewer of the `release` environment is what stops a release in those cases.

## Run of 2026-10-22: `Edit(/cyanotype.lock)`

`rule-maintenance-propose` gained the deny rule `Edit(/cyanotype.lock)` with v0.3, which generates `cyanotype.lock`. This run checks that rule alone; the entry's other `Edit` deny rules have the same form and are still unchecked.

**Environment:**
- Claude Code 2.1.308 (`CLAUDE_CODE_EXECPATH`), running in the Claude desktop app on Windows 11.
- The session was in auto mode.
- The rule was added to the `deny` list of this repository's `.claude/settings.local.json` during the session, which was started in the repository, as in the earlier runs.

No `cyanotype.lock` existed at the root or in `docs/` before the run. The dummy files held `DUMMY_LOCK=...` lines. In the control run, without the rule, every case below wrote to its file.

| Case | Control: no rule | With `Edit(/cyanotype.lock)` |
| --- | --- | --- |
| Edit tool on `cyanotype.lock` at the root | Written | Blocked ("denied by your permission settings") |
| Write tool on `cyanotype.lock` at the root | Written | Blocked ("denied by your permission settings") |
| Write tool on `docs/cyanotype.lock` | Written | Written, as intended: the leading `/` anchors the rule at the root |
| Bash: `echo ... >> cyanotype.lock` | Written | Blocked (permission denied) |
| PowerShell: `Add-Content cyanotype.lock` | Written | Blocked |

After the run the root file still held only the control run's lines.

**Findings:**

- The anchored rule blocks the Edit and Write tools and shell appends in Bash and PowerShell on the root `cyanotype.lock`, and leaves a file of the same name in a subfolder alone.
- In auto mode the Bash refusal does not name the rule that caused it; the same command ran in the control run, so the rule is the only difference between the two.
- As with `.env`, a script that opens the file itself would not be stopped by a permission rule; this run did not try one.

## Run of 2026-10-23: `cyanotype select` ask rules

`rule-maintenance-propose` gained four ask rules with v0.4, which adds `cyanotype select`: `Bash(cyanotype select *)`, `Bash(cyty select *)`, `PowerShell(cyanotype select *)` and `PowerShell(cyty select *)`. This run checks them with the cases under Cases for `cyanotype select`.

**Environment:**
- Claude Code 2.1.308 (`CLAUDE_CODE_EXECPATH`), running in the Claude desktop app on Windows 11.
- The session was in auto mode.
- A build of the `feat/mirror-commands` branch (commit `f1e0b54`) was installed in the Go bin folder on the `PATH` as `cyanotype.exe` and, copied, as `cyty.exe`; neither existed there before. The dummy project was a folder outside the repository holding a valid `cyanotype.yaml` with dummy content.
- The allow rules `Bash(cyanotype *)`, `Bash(cyty *)`, `PowerShell(cyanotype *)` and `PowerShell(cyty *)` and then the ask rules were added to this repository's `.claude/settings.local.json` during the session, which was started in the repository, as in the earlier runs.

The user answered **No** to every prompt but the first, which they approved, so that case ran after its prompt.

| Case | Control: allow only | With ask rules |
| --- | --- | --- |
| Bash: `cyanotype select --dir <dummy> --add <id> --dry-run` | Ran, no prompt | Prompt (approved, then ran) |
| Bash: the same with `--yes` | Ran, no prompt, wrote the dummy spec | Prompt |
| Bash: bare `cyanotype select` | Ran, no prompt (usage error, exit code 2) | Prompt |
| Bash: `cyty select ... --dry-run` | Ran, no prompt | Prompt |
| Bash: `cd <dummy> && cyanotype select ... --dry-run` | Ran, no prompt | Prompt |
| Bash: `cyanotype.exe select ... --dry-run` | Ran, no prompt | **Ran, no prompt** |
| Bash: `cyanotype --dir <dummy> select ... --dry-run` | Ran, no prompt | **Ran, no prompt** |
| Bash: `cyanotype spec`, `cyanotype validate`, `cyanotype catalog search select` | Ran, no prompt | Ran, no prompt, as intended |
| PowerShell: `cyanotype select ... --dry-run`, with `--yes`, bare | Ran, no prompt | Prompt |
| PowerShell: `cyty select ... --dry-run` | Ran, no prompt | Prompt |
| PowerShell: `Set-Location <dummy>; cyanotype select ... --dry-run` | Ran, no prompt | Prompt |
| PowerShell: `cyanotype.exe select ... --dry-run` | Ran, no prompt | Prompt |
| PowerShell: `& cyanotype select ... --dry-run` | Ran, no prompt | Prompt |
| PowerShell: `cyanotype --dir <dummy> select ... --dry-run` | Ran, no prompt | **Ran, no prompt** |
| PowerShell: `cyanotype spec`, `cyanotype validate`, `cyanotype catalog search select` | Ran, no prompt | Ran, no prompt, as intended |

After the run `.claude/settings.local.json` was copied back and `cmp` found it identical to the backup; the dummy folder and both binaries were deleted, and `git status` was clean.

**Findings:**

- The rules ask before `cyanotype select` and `cyty select` in both shells, bare, with any flags after the subcommand, and after `cd` or `Set-Location` in a compound command. In auto mode, too, the ask rules showed a prompt.
- `cyanotype catalog search select`, which names select only as a search term, does not ask, nor do the other commands.
- As with `npm.cmd`, `cyanotype.exe` gets around the `Bash` rules, while `PowerShell(cyanotype select *)` also covers `cyanotype.exe` and the call operator.
- A flag before the subcommand, `cyanotype --dir <dummy> select`, gets around the rules in both shells, like an option before the subcommand of a package manager: cobra accepts the subcommand's flags before its name. The next run checks the rules added for it.

## Run of 2026-10-23: a flag before `select`

The run above found that a flag before the subcommand gets around the ask rules on `cyanotype select`. `rule-maintenance-propose` gained four more ask rules for it: `Bash(cyanotype -* select *)`, `Bash(cyty -* select *)`, `PowerShell(cyanotype -* select *)` and `PowerShell(cyty -* select *)`. A pattern that starts with a dash leaves out `cyanotype catalog search select`, which names select only as a search term.

**Environment:** as in the run above: Claude Code 2.1.308, the Claude desktop app on Windows 11, auto mode, the same allow rules for the control. The build was commit `37f89af` of `feat/mirror-commands`, installed again under both names in the Go bin folder, and a new dummy folder outside the repository held a valid `cyanotype.yaml`. The eight ask rules of the entry were added to `.claude/settings.local.json` together. The user answered **No** to every prompt.

| Case | Control: allow only | With ask rules |
| --- | --- | --- |
| Bash: `cyanotype --dir <dummy> select --add <id> --dry-run` | Ran, no prompt | Ran, no prompt, on the first command after the rules were added; Prompt when run again |
| Bash: `cyty --dir <dummy> select ... --dry-run` | Ran, no prompt | Prompt |
| Bash: `cyanotype --dir=<dummy> select ... --dry-run` | Ran, no prompt | Prompt |
| Bash: `cyanotype select --dir <dummy> ... --dry-run` | Ran, no prompt | Prompt |
| Bash: `cyanotype -y select ...` | Usage error from cyanotype: `unknown shorthand flag: 'y' in -y` | Not repeated |
| Bash: `cyanotype --yes select ...` | Usage error from cyanotype: `unknown flag: --yes` | Not repeated |
| Bash: `cyanotype --dir <dummy> spec`, `cyanotype catalog search select` | Ran, no prompt | Ran, no prompt, as intended |
| PowerShell: `cyanotype --dir <dummy> select ...`, `cyty --dir <dummy> select ...`, `cyanotype --dir=<dummy> select ...`, `cyanotype select ...` (all with `--dry-run`) | Ran, no prompt | Prompt |
| PowerShell: `cyanotype --dir <dummy> spec`, `cyanotype catalog search select` | Ran, no prompt | Ran, no prompt, as intended |

After the run `.claude/settings.local.json` was copied back and `cmp` found it identical to the backup; the dummy spec still held its original content, and the dummy folder and both binaries were deleted. `git status` was clean.

**Findings:**

- The new rules ask before `select` with a flag before it, separated by a space or by `=`, under both names and in both shells, and still leave `cyanotype --dir <dummy> spec` and `cyanotype catalog search select` alone. A `*` in the middle of a pattern matches across spaces, as one at the end does.
- Only flags that take a value work before the subcommand: cyanotype refuses `-y select` and `--yes select` with a usage error, so those need no rule.
- The first command after the rules were added to `.claude/settings.local.json` ran without a prompt, and the same command asked when run again a few commands later, as did every other case. The settings file seems to be read again shortly after it changes, not at once; a run should leave a command or two between adding rules and the first case, or repeat the first case.

## Known limits

- Results hold for the Claude Code version recorded with each run. Permission matching changes between releases, so rerun the procedure after upgrading.
- Deny rules match what a command names. Globs, command substitution, `source`, recursive tools and scripts can reach a file without naming it, and `npm.cmd` or `cyanotype.exe` in Bash or an option before the subcommand of a package manager gets around a command pattern, so deny and ask rules are best effort, as Real constraints in [DESIGN.md](DESIGN.md) says. Repository, pre-commit and CI checks stay the baseline.
- `no-destructive-commands` and the `Edit` deny rules of `rule-maintenance-propose` other than `Edit(/cyanotype.lock)` have not been checked yet; see the table under Other entries.
