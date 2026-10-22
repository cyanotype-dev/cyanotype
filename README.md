# Cyanotype

Cyanotype generates instruction files for coding agents (CLAUDE.md, AGENTS.md, Cursor and Copilot rules) from a catalog of choices, and backs critical rules with real constraints such as permissions, hooks and config files.

Your choices live in a `cyanotype.yaml` spec in your repo; the agent files are regenerable output.

## Status

**Pre-alpha.** v0.3.0 is a pre-release: commands, flags and the `cyanotype.yaml` format may still change. `cyanotype init` creates a `cyanotype.yaml` from a few questions and a starter preset or, in an existing project, from the stack it detects, and `cyanotype generate` writes AGENTS.md, CLAUDE.md, the Claude Code permission rules and a `.gitignore` block from it, never replacing instruction files written by hand without your choice. `cyanotype.lock` records what was generated, so regeneration removes its own leftovers, and `cyanotype check` reports drift in a hook or in CI. Files for other agents and advanced mode are not there yet. See [docs/DESIGN.md](docs/DESIGN.md) for the design and roadmap.

## Installation

Cyanotype is a single binary with no runtime dependencies, for Linux, macOS and Windows on x86-64 (amd64) and ARM64. Every channel except `go install` and Scoop also installs `cyty`, a short alias for `cyanotype`.

Release files are on the [v0.3.0 release page](https://github.com/cyanotype-dev/cyanotype/releases/tag/v0.3.0). Since v0.3.0 is a pre-release, GitHub does not list it as the latest release, so the links below name the version.

### Homebrew (macOS and Linux)

```sh
brew install cyanotype-dev/tap/cyanotype
```

### Scoop (Windows)

```powershell
scoop bucket add cyanotype https://github.com/cyanotype-dev/scoop-bucket
scoop install cyanotype/cyanotype
```

Scoop installs `cyanotype` only. For the `cyty` alias, add this line to your PowerShell profile (`notepad $PROFILE`):

```powershell
Set-Alias cyty cyanotype
```

### Debian and Ubuntu

Download the package for your architecture and install it with apt:

```sh
curl -LO https://github.com/cyanotype-dev/cyanotype/releases/download/v0.3.0/cyanotype_0.3.0_amd64.deb
sudo apt install ./cyanotype_0.3.0_amd64.deb
```

On ARM64, use `cyanotype_0.3.0_arm64.deb`.

### Fedora, RHEL and openSUSE

```sh
curl -LO https://github.com/cyanotype-dev/cyanotype/releases/download/v0.3.0/cyanotype-0.3.0-1.x86_64.rpm
sudo dnf install ./cyanotype-0.3.0-1.x86_64.rpm
```

On ARM64, use `cyanotype-0.3.0-1.aarch64.rpm`; on openSUSE, install it with `sudo zypper install`.

### Go

With the Go version in [go.mod](go.mod) or newer (Go downloads it for you if needed):

```sh
go install github.com/cyanotype-dev/cyanotype/cmd/cyanotype@v0.3.0
```

This installs `cyanotype` only, in `$(go env GOPATH)/bin`. For the alias:

```sh
ln -s cyanotype "$(go env GOPATH)/bin/cyty"
```

### Archives

Each archive holds the binary, the `cyty` alias and the licenses.

- Linux: `cyanotype_0.3.0_linux_amd64.tar.gz` or `cyanotype_0.3.0_linux_arm64.tar.gz`
- macOS: `cyanotype_0.3.0_darwin_arm64.tar.gz` (Apple Silicon) or `cyanotype_0.3.0_darwin_amd64.tar.gz` (Intel)
- Windows: `cyanotype_0.3.0_windows_amd64.zip` or `cyanotype_0.3.0_windows_arm64.zip`

On Linux and macOS, `cyty` is a symlink to `cyanotype`; move both into a folder on your `PATH`:

```sh
curl -LO https://github.com/cyanotype-dev/cyanotype/releases/download/v0.3.0/cyanotype_0.3.0_linux_amd64.tar.gz
tar -xzf cyanotype_0.3.0_linux_amd64.tar.gz
sudo mv cyanotype cyty /usr/local/bin/
```

On Windows, extract the zip into a folder on your `PATH`. `cyty.cmd` runs `cyanotype.exe` from the same folder, in both Command Prompt and PowerShell.

### macOS: unsigned binary

The macOS binaries are not yet signed or notarized by Apple. Homebrew, `go install` and archives downloaded with `curl` are not affected. An archive downloaded with a web browser is quarantined, and the first run of `cyanotype` stops with a dialog saying that Apple could not verify that "cyanotype" is free of malware, with the buttons **Done** and **Move to Trash**.

[Verify the download](#verifying-a-download) first, then either:

- remove the quarantine attribute in the folder where you extracted the archive:

  ```sh
  xattr -d com.apple.quarantine cyanotype
  ```

- or click **Done**, open **System Settings → Privacy & Security**, click **Open Anyway** next to the message about "cyanotype", and confirm with your password.

## Verifying a download

Every release lists the SHA-256 checksum of each archive and package in `checksums.txt`. The checksum file is signed with [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) keyless signing: the signature, in `checksums.txt.sigstore.json`, proves the file was made by this repository's release workflow for that tag, and is recorded in the public Sigstore transparency log.

Download both files next to your archive or package, then check the signature:

```sh
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity https://github.com/cyanotype-dev/cyanotype/.github/workflows/release.yml@refs/tags/v0.3.0 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

It prints `Verified OK`. Then check the files you downloaded against the checksums:

```sh
# Linux
sha256sum --ignore-missing -c checksums.txt
# macOS
shasum -a 256 --ignore-missing -c checksums.txt
```

On Windows, in PowerShell, compare the hash with the file's line in `checksums.txt`:

```powershell
(Get-FileHash .\cyanotype_0.3.0_windows_amd64.zip -Algorithm SHA256).Hash.ToLower()
Select-String cyanotype_0.3.0_windows_amd64.zip checksums.txt
```

## Quick start

In your project folder, run:

```sh
cyanotype init
```

It asks what you are building, which language you know, how much autonomy the agent gets, whether the app is multilingual, the language of the agent's replies, the goal and your coding agents, then writes `cyanotype.yaml` and offers to generate the files. Every question has a flag, so scripts and agents can run it without a terminal; `cyanotype init --help` lists them. It never overwrites an existing `cyanotype.yaml`, which you can also write by hand (the Project spec section of [docs/DESIGN.md](docs/DESIGN.md) has an example).

### In an existing project

When the folder already has code, init first detects the stack, shows each entry with the file that revealed it, and asks whether to start from it: yes, yes without some of it, or no to pick a starter instead. For a Next.js app with two lockfiles:

```text
Detected in this project:
  - Next.js (package.json: next)
  - Node.js (package.json)
  - npm (package-lock.json)
  - pnpm (pnpm-lock.yaml)
  - TypeScript (package.json: typescript)
  - Vitest (package.json: vitest)
Only one of npm and pnpm can be selected: the project uses one of them.
Scripts in package.json: build, dev, lint, start, test
```

The stack comes from detection, never from a preset, and init asks when detection finds a clash, such as the two lockfiles above. The `dev`, `build`, `test` and `lint` commands come from the `package.json` scripts, run with the package manager you keep: here `pnpm dev`, `pnpm build`, `pnpm test` and `pnpm lint`.

Without a terminal, `--detect yes` starts from the detected stack and `--detect no` ignores it; `--with` adds entries or picks one where detection found several, and `--without` drops detected entries the project does not use. init lists every missing flag with its allowed values, for example:

```sh
cyanotype init --detect yes --with pnpm --existing keep --autonomy balanced --i18n no --goal "An online shop for handmade prints." --agents claude
```

### Instruction files written by hand

A CLAUDE.md or AGENTS.md without the cyanotype header was written by hand, and cyanotype never replaces it silently, not even with `--yes`. init asks about it right after detection, before any other question, and generate before writing; without a terminal, `--existing` answers:

- `import` (recommended): prints a prompt for your coding agent, which turns the file's rules into `cyanotype.yaml`. Nothing is written.
- `keep`: the whole file goes in a `cyanotype:custom` block, which regeneration keeps.
- `move`: its content goes to `docs/agents/original-claude.md` (for CLAUDE.md), which the main file tells the agent to read.
- `replace`: the generated file overwrites it, only when git has it committed with no pending changes, so `git show HEAD:CLAUDE.md` gives it back.

The agent asks you about what the file does not settle and writes `cyanotype.yaml`, so it needs a normal interactive session. Ask your agent to run the command and follow what it prints; copying the output into the session also works. After the prompt, cyanotype says:

```text
Nothing was written. The prompt printed on stdout asks your coding agent to turn CLAUDE.md into cyanotype.yaml.
To use it, ask your agent in an interactive session to run "cyanotype init --existing import" and follow what it prints, or copy the prompt into the session. Do not pipe it into the agent: piped, the agent runs without a session, so it cannot ask you questions or for permissions.
Coding agent: ask the user about every choice the prompt leaves open; never make one yourself.
When the agent is done, review cyanotype.yaml and run "cyanotype generate --existing replace".
```

The prompt is never written to a file in the project, where it would end up in a commit.

### Generating the files

To write the files after changing the spec, run:

```sh
cyanotype generate
```

It shows the changes as a diff and asks before writing. `--dry-run` only shows them, `--yes` writes without asking, and `--dir` points to another project folder. Without a terminal, as in CI, it writes nothing unless `--yes` is given, and exits with code 3 when files are out of date.

### cyanotype.lock

Every generate also writes `cyanotype.lock`, a record of what cyanotype generated. Commit it, like a package lockfile, and do not edit it: with the default rule-maintenance policy, Claude Code is denied edits to it, as to the instruction files. For a Go command-line tool it starts like this:

```yaml
# Generated by cyanotype from cyanotype.yaml; regenerating replaces this file.
# It records what cyanotype generated, so it can tell that apart from what
# people wrote. Commit it, and do not edit it by hand.
version: 1
catalog_version: "2026.10.1"
entries:
  conventional-commits: sha256:01df90ce7de50a81ced52e8969174b30bf1fa5b051679ab8333aa0e39cc6c3d4
  definition-of-done: sha256:cd605e820f34374c6951c7a846c17f01aee6ded93f8b83d970e57f3ffbd2e27e
  dependency-approval: sha256:490db0cf26b79b7ac1d99d626574d6b5280e2f327a25aad1234e8b2611dfae78
```

It goes on with a hash of each generated file and the permission rules cyanotype added to `.claude/settings.json`. The hashes ignore line endings and the text between `cyanotype:custom` markers, so a CRLF checkout or a note in a protected block changes nothing. Regenerating an unchanged project leaves the lock byte for byte.

With the lock, regeneration cleans up after itself:

- Deselect an entry, and the permission rules cyanotype added for it leave `.claude/settings.json`. A rule you wrote yourself always stays, even one you had before cyanotype generated the same rule.
- Change the split, and the topic files cyanotype no longer generates are deleted, in the same diff and with the same confirmation:

  ```text
    unchanged  .gitignore
    modified   AGENTS.md
    modified   cyanotype.lock
    deleted    docs/agents/git.md
    deleted    docs/agents/style.md
    deleted    docs/agents/workflow.md
  5 files to write (0 created, 2 modified, 3 deleted), 1 unchanged.
  ```

  Only a file the lock records, unchanged since and with no `cyanotype:custom` block, is ever deleted. One you edited stays, with a warning that asks you to delete it or remove its header to keep it as your own.

On a merge conflict in `cyanotype.lock`, keep the lines of both sides, then run `cyanotype generate`.

### Upgrading from v0.2

A project generated by v0.1 or v0.2 has no lock. The first generate with v0.3 creates it, and cyanotype never guesses which existing rules or files are its own. The rules already in `.claude/settings.json` count as yours, and that first generate says so:

```text
.claude\settings.json: warning: cyanotype.lock does not exist yet, so cyanotype treats the rules already in this file as yours and never removes them: 31 match rules it generates. If an older cyanotype added them, delete them from the file and run cyanotype generate again; it adds back the ones still generated, and removes them once they are not
```

Topic files left by an earlier split get a warning on every run, and are never deleted, since no recorded hash shows they are unchanged.

The v0.3 catalog is `2026.10.1`. A spec pinned to `2026.10` gets a warning until you review the changes with `cyanotype generate --dry-run` and set `catalog_version` to `"2026.10.1"`. Commit `cyanotype.lock` after this first generate: until then, `cyanotype generate --dry-run` exits with code 3 and `cyanotype check` reports the lock as missing.

## Checking for drift

`cyanotype check` reports where the project and `cyanotype.yaml` drifted apart. It never writes and never reads stdin, so it fits a pre-commit hook and CI:

```sh
cyanotype check
```

After editing CLAUDE.md by hand, committing a `.env` and adding SQLite to `go.mod`, it prints:

```text
.env: error: is tracked by git, although secrets-in-env keeps it out of git (.gitignore: .env); git rm --cached .env stops tracking it, but does not remove it from the history, so any secret it held must also be rotated
CLAUDE.md: error: was edited by hand since cyanotype generated it, outside its cyanotype:custom blocks; cyanotype generate would undo the edits, so move them into a cyanotype:custom block, or into custom_rules in cyanotype.yaml
go.mod: warning: lists github.com/mattn/go-sqlite3, which suggests SQLite, and cyanotype.yaml does not select it; add sqlite to selections
2 errors and 1 warning.
```

Errors are generated files that are out of date or edited by hand, instruction files written by hand that wait for a choice, and files git tracks that the generated `.gitignore` lines keep out. Warnings are entries detected in the project that the spec does not select, an older `catalog_version`, a main file over `output.max_lines`, pointers to files that do not exist, and leftover files. The lock tells a hand edit from a change to the spec, so check says which one it is.

| Exit code | Meaning |
| --- | --- |
| 0 | No errors; warnings are allowed |
| 1 | The check could not run, such as a spec with errors |
| 2 | Wrong usage |
| 3 | Drift found: errors, or warnings with `--strict` |

`--strict` makes warnings fail too. It fits projects that can resolve every warning: a detected entry the project does not use cannot be silenced yet.

As a pre-commit hook, put this in `.git/hooks/pre-commit` and make it executable; check reads the working tree, so unstaged changes count too:

```sh
#!/bin/sh
exec cyanotype check
```

With the [pre-commit](https://pre-commit.com) framework, add a local hook to `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: local
    hooks:
      - id: cyanotype-check
        name: cyanotype check
        entry: cyanotype check
        language: system
        pass_filenames: false
```

In CI, install cyanotype and run check; its errors fail the job. In GitHub Actions:

```yaml
- uses: actions/setup-go@v7
  with:
    go-version: stable
- run: go install github.com/cyanotype-dev/cyanotype/cmd/cyanotype@v0.3.0
- run: cyanotype check
```

cyanotype never installs hooks or workflows itself.

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
