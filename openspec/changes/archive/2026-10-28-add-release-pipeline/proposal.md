## Why

`i2a` has no releases. Users must install it with `go install`, and the binary then shows the version `dev`. Each change that goes into `main` must make a new version automatically, so that users can download a known build and report a version.

## What Changes

- A `release` job in `.github/workflows/ci.yml`. It runs on each push to `main`, after all CI jobs pass.
- The job reads the Conventional Commit subjects since the last tag and makes the next version with semantic-release. A breaking change makes a major version, `feat` makes a minor version, and all other types make a patch version. Thus each squash-merged PR makes a new version.
- `scripts/dist.sh <version>` builds `i2a` for darwin and linux, amd64 and arm64, with the version in the binary. It writes one `.tar.gz` file for each platform and a `checksums.txt` file.
- semantic-release makes the Git tag `v<version>` and the GitHub release with the release notes and the archives.
- `i2a --version` shows the module version when a user installs with `go install ...@<tag>`.
- The README tells how to install from a release.
- Repository settings: squash merge only, and the squash commit subject is the PR title.

## Capabilities

### New Capabilities

- `release`: how a push to `main` becomes a versioned GitHub release with binaries.

### Modified Capabilities

None.

## Impact

- New files: `.releaserc.json`, `scripts/dist.sh`.
- Changed files: `.github/workflows/ci.yml`, `.github/workflows/pr-title.yml` (comment), `cmd/i2a/main.go`, `README.md`, `CLAUDE.md`.
- The release job needs `contents: write` permission for the `GITHUB_TOKEN`. It installs pinned npm packages at run time. There is no `package.json`.
- Out of scope: Homebrew, code signing and notarization of the macOS binary.
