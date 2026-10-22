# Cyanotype

Cyanotype generates instruction files for coding agents (CLAUDE.md, AGENTS.md, Cursor and Copilot rules) from a catalog of choices, and backs critical rules with real constraints such as permissions, hooks and config files.

Your choices live in a `cyanotype.yaml` spec in your repo; the agent files are regenerable output.

## Status

**Pre-alpha.** v0.2.0 is a pre-release: commands, flags and the `cyanotype.yaml` format may still change. `cyanotype init` creates a `cyanotype.yaml` from a few questions and a starter preset or, in an existing project, from the stack it detects, and `cyanotype generate` writes AGENTS.md, CLAUDE.md, the Claude Code deny rules and a `.gitignore` block from it, never replacing instruction files written by hand without your choice; files for other agents and advanced mode are not there yet. See [docs/DESIGN.md](docs/DESIGN.md) for the design and roadmap.

## Installation

Cyanotype is a single binary with no runtime dependencies, for Linux, macOS and Windows on x86-64 (amd64) and ARM64. Every channel except `go install` and Scoop also installs `cyty`, a short alias for `cyanotype`.

Release files are on the [v0.2.0 release page](https://github.com/cyanotype-dev/cyanotype/releases/tag/v0.2.0). Since v0.2.0 is a pre-release, GitHub does not list it as the latest release, so the links below name the version.

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
curl -LO https://github.com/cyanotype-dev/cyanotype/releases/download/v0.2.0/cyanotype_0.2.0_amd64.deb
sudo apt install ./cyanotype_0.2.0_amd64.deb
```

On ARM64, use `cyanotype_0.2.0_arm64.deb`.

### Fedora, RHEL and openSUSE

```sh
curl -LO https://github.com/cyanotype-dev/cyanotype/releases/download/v0.2.0/cyanotype-0.2.0-1.x86_64.rpm
sudo dnf install ./cyanotype-0.2.0-1.x86_64.rpm
```

On ARM64, use `cyanotype-0.2.0-1.aarch64.rpm`; on openSUSE, install it with `sudo zypper install`.

### Go

With the Go version in [go.mod](go.mod) or newer (Go downloads it for you if needed):

```sh
go install github.com/cyanotype-dev/cyanotype/cmd/cyanotype@v0.2.0
```

This installs `cyanotype` only, in `$(go env GOPATH)/bin`. For the alias:

```sh
ln -s cyanotype "$(go env GOPATH)/bin/cyty"
```

### Archives

Each archive holds the binary, the `cyty` alias and the licenses.

- Linux: `cyanotype_0.2.0_linux_amd64.tar.gz` or `cyanotype_0.2.0_linux_arm64.tar.gz`
- macOS: `cyanotype_0.2.0_darwin_arm64.tar.gz` (Apple Silicon) or `cyanotype_0.2.0_darwin_amd64.tar.gz` (Intel)
- Windows: `cyanotype_0.2.0_windows_amd64.zip` or `cyanotype_0.2.0_windows_arm64.zip`

On Linux and macOS, `cyty` is a symlink to `cyanotype`; move both into a folder on your `PATH`:

```sh
curl -LO https://github.com/cyanotype-dev/cyanotype/releases/download/v0.2.0/cyanotype_0.2.0_linux_amd64.tar.gz
tar -xzf cyanotype_0.2.0_linux_amd64.tar.gz
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
  --certificate-identity https://github.com/cyanotype-dev/cyanotype/.github/workflows/release.yml@refs/tags/v0.2.0 \
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
(Get-FileHash .\cyanotype_0.2.0_windows_amd64.zip -Algorithm SHA256).Hash.ToLower()
Select-String cyanotype_0.2.0_windows_amd64.zip checksums.txt
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

The import prompt goes to stdout. The agent asks you questions and writes `cyanotype.yaml`, so use it in a normal interactive session: save it to a file,

```sh
cyanotype init --existing import > import-prompt.md
```

then ask your agent to follow the instructions in `import-prompt.md`, and delete the file when it is done, so it does not end up in a commit. Review `cyanotype.yaml`, then run `cyanotype generate --existing replace`.

### Generating the files

To write the files after changing the spec, run:

```sh
cyanotype generate
```

It shows the changes as a diff and asks before writing. `--dry-run` only shows them, `--yes` writes without asking, and `--dir` points to another project folder. Without a terminal, as in CI, it writes nothing unless `--yes` is given, and exits with code 3 when files are out of date.

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
