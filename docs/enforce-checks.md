# Checking the catalog's Claude deny rules

Catalog entries generate deny rules for `.claude/settings.json` through `enforce.claude.deny`. Those rules are only worth shipping if they block what the snippet says they block, and Claude Code decides that at run time, inside the agent, so `go test` cannot check it. This page is the manual procedure for checking it, with the result of every run.

Run it again when an entry's deny rules change, and when a new Claude Code release changes how permissions are matched.

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

Rules: `Read(.env)`, `Read(.env.*)`, `Read(!.env.example)`. Expected: `.env` and `.env.*` blocked everywhere; `.env.example` readable and writable.

- Read tool: `.env` and `.env.local` at the root, `.env` in a subfolder, `.env.example`.
- Edit and Write tools: edit `.env`, overwrite `.env` and `.env.local`, create `.env.production`, edit `.env.example`.
- Shell reads, in Bash: `cat`, `head`, `tail`, `sed -n 1p`, `grep PATTERN .env`, `awk`, `wc -l < .env`, `cat ./.env`, an absolute path, `base64`, `git diff --no-index /dev/null .env`, `cp .env other`. Also try ways of reaching the file without naming it: a glob such as `cat .en*`, a command substitution such as `cat "$(echo .env)"`, `source .env`, `grep -r PATTERN .` and a script, such as `python -c "print(open('.env').read())"`.
- Shell reads, in PowerShell: `Get-Content .env`, the alias `gc .env.local`, `Get-Content .env.example`, `Get-Content .en*`.
- Shell writes: `>> .env`, `> .env.production`, `tee -a .env`, `sed -i`, `>> .env.example`, a Python script appending to `.env`, PowerShell `Add-Content` and `Set-Content`.

### Other entries

Use the same method for the other entries with deny rules. Choose cases that would do no harm if a rule failed: delete a dummy folder with `rm -rf`, delete a dummy branch with `git branch -D`, and add `--dry-run` to force pushes.

| Entry | Rules | Last checked |
| --- | --- | --- |
| `secrets-in-env` | `Read` rules on `.env` files | 2026-10-19, see below |
| `no-destructive-commands` | `Bash` and `PowerShell` command patterns | Not yet |
| `rule-maintenance-propose` | `Edit` rules on generated files | Not yet |
| `npm`, `pnpm`, `uv` | `Bash` rules on the other package managers | Not yet |

## Run of 2026-10-19: `secrets-in-env`

**Environment:**
- Claude Code 2.1.299, running in the Claude desktop app 2.42304.2 on Windows 11. The `claude` CLI on the `PATH` was 2.1.303 and was not the one tested.
- The session was in bypass permissions mode; the deny rules were enforced anyway.
- An earlier, smaller run in default permission mode (Read tool only, subfolder only, same Claude Code version) gave the same Read results.

In the control run, every case below succeeded.

Phase A uses the entry's rules as shipped. Phase B adds `Edit(.env)`, `Edit(.env.*)` and `Edit(!.env.example)` to see whether they close the gaps found in phase A.

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
- `Read` deny rules alone do **not** stop shell writes: redirections, `tee` and PowerShell `Add-Content` wrote to `.env` and created `.env.production`. Adding matching `Edit` deny rules blocked all of these.
- No permission rule stops a script that opens the file itself. Only the sandbox can, at the operating-system level, as the Claude Code documentation says.

## Known limits

- Results hold for the Claude Code version recorded with each run. Permission matching changes between releases, so rerun the procedure after upgrading.
- Deny rules match what a command names. Globs, command substitution, `source`, recursive tools and scripts can reach a file without naming it, so deny rules are best effort, as Real constraints in [DESIGN.md](DESIGN.md) says. Repository, pre-commit and CI checks stay the baseline.
- Only `secrets-in-env` has been checked so far; see the table under Other entries.
