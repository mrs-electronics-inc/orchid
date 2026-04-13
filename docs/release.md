# Release Workflow

Orchid uses a single source of truth for release versioning:

- `VERSION` at the repo root stores the next release version, including the `v` prefix.
- `orchid --version` prints the build version and short commit hash when available.
- `just build` stamps dev builds with `dev` plus the current short commit.

## Draft Release

Use the `Draft Release` GitHub Actions workflow to prepare the next version.

1. Pick a `patch`, `minor`, or `major` bump.
2. The workflow updates `VERSION` and opens a pull request.
3. Merge the pull request to update `main`.

## Release

The `Release` workflow runs when `VERSION` changes on `main`.

1. It reads the version from `VERSION`.
2. It creates and pushes an annotated git tag if needed.
3. It runs GoReleaser to publish Linux release artifacts and checksums on GitHub Releases.

## Repository Setup

The draft release workflow expects a GitHub App with permission to create pull requests and repository secrets for authentication.

1. Register a GitHub App in the repository owner account or organization.
2. Set the app name to something like `Orchid Release Bot`.
3. Set the homepage URL to the Orchid repository URL.
4. Disable webhooks unless you need them for other automation.
5. Grant the app `Contents: Read and write` and `Pull requests: Read and write`.
6. Generate a private key and download the `.pem` file.
7. Install the app on this repository.
8. Save the app's client ID as the repository or organization secret `RELEASE_BOT_APP_ID`.
9. Save the downloaded private key as the repository or organization secret `RELEASE_BOT_PRIVATE_KEY`.

The workflow mints a short-lived installation token on each run, so there is no long-lived PAT to rotate.
