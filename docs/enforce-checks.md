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

### Other entries

Use the same method for the other entries with permission rules. Choose cases that would do no harm if a rule failed: delete a dummy folder with `rm -rf`, delete a dummy branch with `git branch -D`, and add `--dry-run` to force pushes.

| Entry | Rules | Last checked |
| --- | --- | --- |
| `secrets-in-env` | `Read` and `Edit` rules on `.env` files | 2026-10-19, see below |
| `no-destructive-commands` | `Bash` and `PowerShell` command patterns | Not yet |
| `rule-maintenance-propose` | `Edit` deny rules on generated files | Not yet; the `ask` rule on `cyanotype.yaml` on 2026-10-20 |
| `npm`, `pnpm`, `uv` | `Bash` and `PowerShell` rules on the other package managers | 2026-10-20, see below |
| `dependency-approval` | `Bash` and `PowerShell` ask rules on install commands | 2026-10-20, see below |

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
- Ask wins over allow, and deny wins over ask, so an entry's rule must not be in both of its lists, and a package manager's deny rules win over `dependency-approval`'s ask rules.
- An `Edit` ask rule prompts for the Edit and Write tools even in accept-edits mode. Whether it also prompts for shell writes could not be told apart with this method, since the shell write prompted without it too.

## Known limits

- Results hold for the Claude Code version recorded with each run. Permission matching changes between releases, so rerun the procedure after upgrading.
- Deny rules match what a command names. Globs, command substitution, `source`, recursive tools and scripts can reach a file without naming it, and `npm.cmd` in Bash or an option before the subcommand gets around a command pattern, so deny rules are best effort, as Real constraints in [DESIGN.md](DESIGN.md) says. Repository, pre-commit and CI checks stay the baseline.
- `no-destructive-commands` and the `Edit` deny rules of `rule-maintenance-propose` have not been checked yet; see the table under Other entries.
