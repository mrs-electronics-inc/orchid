# Contributing

Use `just check` before opening a pull request.

## Release Workflow

Orchid uses a single source of truth for release versioning:

- `VERSION` at the repo root stores the next release version, including the `v` prefix.
- `orchid --version` prints the build version and short commit hash when available.
- `just build` stamps dev builds with `dev` plus the current short commit.

### Draft Release

Use the `Draft Release` GitHub Actions workflow to prepare the next version.

1. Pick a `patch`, `minor`, or `major` bump.
2. The workflow updates `VERSION` and opens a pull request.
3. Merge the pull request to update `main`.

### Release

The `Release` workflow runs when `VERSION` changes on `main`.

1. It reads the version from `VERSION`.
2. It creates and pushes an annotated git tag if needed.
3. It runs GoReleaser to publish Linux release artifacts and checksums on GitHub Releases.

### Repository Setup

The draft release workflow expects a GitHub App with these repository secrets:

- `RELEASE_BOT_APP_ID`
- `RELEASE_BOT_PRIVATE_KEY`
