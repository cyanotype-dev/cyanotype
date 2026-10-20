# Releasing Cyanotype

Releases are built by [GoReleaser](https://goreleaser.com) from `.goreleaser.yaml`, in the `Release` workflow (`.github/workflows/release.yml`). The workflow runs when the maintainer pushes a `v*` tag and waits for the maintainer's approval before it starts. Only the maintainer creates and pushes tags; agents never do.

A release publishes:

- a GitHub release, marked as a pre-release, with a changelog grouped by Conventional Commit type;
- tar.gz archives for Linux and macOS and zip archives for Windows, on amd64 and arm64;
- `.deb` and `.rpm` packages for Linux;
- `checksums.txt`, signed with cosign keyless signing into `checksums.txt.sigstore.json`;
- a Homebrew formula in `cyanotype-dev/homebrew-tap` and a Scoop manifest in `cyanotype-dev/scoop-bucket`.

`go install` needs nothing from the workflow: the Go module proxy serves the tag once it is pushed.

## One-time setup

No token or key is ever written in this repository. The workflow uses the automatic `GITHUB_TOKEN`, with the permissions it declares (`contents: write` to create the release, `id-token: write` for cosign), and two deploy keys kept as environment secrets.

1. **Package repositories.** Create two public repositories in the `cyanotype-dev` organization, each initialized with a README so that the `main` branch exists:
   - `cyanotype-dev/homebrew-tap`, for the formula in `Formula/`;
   - `cyanotype-dev/scoop-bucket`, for the manifest in `bucket/`.
2. **Deploy keys.** For each of the two repositories, generate an SSH key without a passphrase, which GoReleaser cannot use:

   ```sh
   ssh-keygen -t ed25519 -N "" -C "cyanotype release: homebrew-tap" -f homebrew-tap-deploy-key
   ssh-keygen -t ed25519 -N "" -C "cyanotype release: scoop-bucket" -f scoop-bucket-deploy-key
   ```

   Add each public key (`.pub`) under the repository's **Settings → Deploy keys**, with **Allow write access** checked. A deploy key can push to that one repository only and is not tied to a person's account, which makes it narrower than a personal access token.
3. **Release environment.** In `cyanotype-dev/cyanotype`, under **Settings → Environments**, create an environment named `release`:
   - **Required reviewers:** add the maintainer. The release job then waits for approval even when a tag is pushed by mistake, or with the maintainer's own credentials, which a tag ruleset does not stop. Leave **Prevent self-review** off, since the maintainer approves releases of the tags they push.
   - **Deployment branches and tags:** choose **Selected branches and tags** and add the tag rule `v*`, so no branch or pull request can run a job in this environment.
   - **Environment secrets:** add `HOMEBREW_TAP_DEPLOY_KEY` and `SCOOP_BUCKET_DEPLOY_KEY`, each holding the full content of its private key file. Environment secrets reach only jobs in that environment, unlike repository secrets.

   Delete the local private key files afterwards; a lost key is replaced by generating a new one.
4. **Tag ruleset (optional).** Under **Settings → Rules → Rulesets**, add a tag ruleset for `v*` that restricts creation, updates and deletion, so a release tag cannot be moved or removed once it is published.
5. **Maintainer address.** The `.deb` and `.rpm` packages and the tap and bucket commits name `Cyanotype maintainers <maintainers@cyanotype.dev>`. Before the first release, check that the address receives mail.

## Making a release

1. Check that `main` is green and that the `Release check` workflow passed on the last pull request that changed the release configuration.
2. Tag the release commit on `main` and push the tag:

   ```sh
   git switch main
   git pull --ff-only
   git tag -a v0.1.0 -m "v0.1.0"
   git push origin v0.1.0
   ```

3. Open the `Release` run under **Actions** and approve the `release` environment. The job runs the tests, then GoReleaser.
4. Check the result:
   - the release page lists the archives, packages, `checksums.txt` and `checksums.txt.sigstore.json`, and is marked as a pre-release;
   - the signature verifies, as in [Verifying a download](../README.md#verifying-a-download);
   - `homebrew-tap` and `scoop-bucket` each got a commit for the new version;
   - `brew install cyanotype-dev/tap/cyanotype`, `scoop install cyanotype/cyanotype` and `go install github.com/cyanotype-dev/cyanotype/cmd/cyanotype@v0.1.0` install a binary whose `cyanotype version` prints the tag.

If the job fails before publishing, fix the cause on `main` through a pull request, then delete the tag and push it again on the fixed commit. Once a release is public, do not move its tag: publish a new patch version instead.

## Pre-releases

`.goreleaser.yaml` sets `prerelease: "true"`, so every release is a pre-release. GoReleaser's `auto` setting only marks tags with a pre-release suffix, such as `v0.2.0-rc.1`, and v0.1.0 has none. With the first stable release, switch it to `auto`. Pre-releases are not GitHub's "latest" release, so the README links name the version; update them with each release.

## Changing the release configuration

- GoReleaser is pinned in both `release.yml` and `release-check.yml`, and cosign in `release.yml`. Upgrade them in one pull request, keeping the two GoReleaser versions equal.
- The actions in both workflows are pinned by commit SHA, with the version in a comment.
- Pull requests that change `.goreleaser.yaml`, the two workflows, `packaging/` or `cmd/cyanotype/main.go` run the `Release check` workflow. It runs `goreleaser check`, which may report only the deliberate deprecation of the Homebrew formula (`brews`), then builds a snapshot without signing or publishing, checks the `cyty` alias in the archives and the `.deb` package, and lists the produced files in the job summary.
- To build a snapshot locally, on Linux or macOS:

  ```sh
  goreleaser check
  goreleaser release --snapshot --clean --skip=sign
  ```

  The build creates the `cyty` symlink with `ln -s`, so it does not run on Windows; use the `Release check` workflow there, which also runs on demand (`workflow_dispatch`).
