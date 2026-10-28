# release Specification

## Purpose
This capability makes a new version, a Git tag and a GitHub release with binaries for each push to `main` that passes CI.

## Requirements
### Requirement: Version from the commit subjects

The release job SHALL make a new version for each push to `main` that has new commits since the last tag. The version SHALL follow these rules: a commit with `!` before the colon or a `BREAKING CHANGE` footer makes a major version; `feat` makes a minor version; `fix`, `perf`, `refactor`, `docs`, `chore`, `test`, `ci`, `build`, `style` and `revert` make a patch version. The highest version of all new commits SHALL apply.

#### Scenario: Chore commit

- **WHEN** the last tag is `v1.2.3` and one new commit on `main` has the subject `chore: update deps`
- **THEN** the next version is `1.2.4`

#### Scenario: Feature commit

- **WHEN** the last tag is `v1.2.3` and the new commits are `docs: fix typo` and `feat: add mentions filter`
- **THEN** the next version is `1.3.0`

#### Scenario: Breaking commit

- **WHEN** the last tag is `v1.2.3` and one new commit has the subject `refactor!: rename the config keys`
- **THEN** the next version is `2.0.0`

### Requirement: Release only after CI passes

The release job SHALL run only for a push to `main`, and only after all other CI jobs pass. A new push to `main` SHALL NOT cancel a release job that runs.

#### Scenario: Failed test

- **WHEN** a unit test fails on a push to `main`
- **THEN** the release job does not run and no tag is made

### Requirement: Release assets

Each release SHALL have one `.tar.gz` archive for each of darwin/amd64, darwin/arm64, linux/amd64 and linux/arm64, and a `checksums.txt` file with the SHA-256 sum of each archive. Each archive SHALL hold the `i2a` binary, `LICENSE` and `README.md`. The archive names SHALL NOT include the version. If the build fails, the job SHALL NOT make a tag or a release.

#### Scenario: Download the latest release

- **WHEN** a user downloads `releases/latest/download/i2a_darwin_arm64.tar.gz` and extracts it
- **THEN** `./i2a --version` shows the version of the latest release
- **AND** the SHA-256 sum of the archive is in `checksums.txt`

### Requirement: Version in the binary

The binary SHALL show its version with `i2a --version`. A release build SHALL show the release version. A build with `go install <module>/cmd/i2a@<tag>` SHALL show the module version. A local build without a version SHALL show `dev`.

#### Scenario: go install of a tag

- **WHEN** a user runs `go install github.com/sondt2709/inbox2agent/cmd/i2a@v1.0.0`
- **THEN** `i2a --version` shows `v1.0.0`

