## Context

The CI workflow `.github/workflows/ci.yml` runs lint, unit tests (Linux and macOS), cross builds, `govulncheck`, `gitleaks` and the e2e scenarios on each pull request and each push to `main`. The workflow `pr-title.yml` makes sure that each PR title is a Conventional Commit. Merges are squash only, thus the PR title becomes the commit subject on `main`.

`cmd/i2a/main.go` has `var version = "dev"`. `task build` sets it with `-ldflags "-X main.version=..."`.

## Goals / Non-Goals

**Goals:**

- Each push to `main` that passes CI makes a new version, a Git tag and a GitHub release.
- The release has binaries for darwin and linux (amd64, arm64) and a checksum file.
- The binary shows its version.

**Non-Goals:**

- Homebrew formula or cask.
- macOS code signing and notarization.
- A `CHANGELOG.md` file in the repository. The release notes are on the GitHub release only.

## Decisions

### D1. semantic-release with pinned packages, run with `npx`

The job runs `npx -p semantic-release@25.0.9 -p @semantic-release/commit-analyzer@13.0.1 ...`. Exact versions make the run repeatable. There is no `package.json`, because the project is Go.

`conventional-changelog-conventionalcommits` stays on 9.x: version 10 needs `conventional-changelog-writer` 9, but `@semantic-release/release-notes-generator` 14.1.1 loads writer 8, and the `generateNotes` step then fails.

Alternatives: `go-semantic-release` (its action runs on `node20`, and its plugins download at run time); `release-please` (it opens a release PR, thus a merge does not make a version immediately). The maintainer uses semantic-release in other projects.

### D2. Release rules: each commit type makes a version

`.releaserc.json` uses the `conventionalcommits` preset, which knows `!` before the colon. The release rules are:

| Commit | Version |
|---|---|
| `!` or a `BREAKING CHANGE` footer | major |
| `feat` | minor |
| `fix`, `perf`, `refactor`, `docs`, `chore`, `test`, `ci`, `build`, `style`, `revert` | patch |

The commit analyzer uses the highest release of all custom rules that match. A breaking commit matches the `breaking` rule and its type rule, thus it gets major.

The release notes have a section for each type, so that a release with only `chore` commits does not have empty notes. These types are the same as the types that `pr-title.yml` accepts (plus `style` and `revert`).

### D3. The release is a job in `ci.yml`, not a separate workflow

The `release` job has `needs:` on all other jobs and runs only for `push` to `main`. Thus a release never contains code that failed CI. A separate workflow with `workflow_run` can do the same, but it is more difficult to read and to test.

The concurrency group of `ci.yml` cancels runs in progress only for pull requests. On `main`, a new push must not cancel a release that runs. The `release` job has its own group, `release`, which does not cancel.

### D4. Build before publish

1. `semantic-release --dry-run` prints the next version. If there is no next version, the job stops.
2. `scripts/dist.sh <version>` builds the archives into `dist/`.
3. `semantic-release` makes the tag, the release notes and the GitHub release, and uploads `dist/*.tar.gz` and `dist/checksums.txt`.

If the build fails, no tag and no release are made. This is the same order as in the maintainer's other project.

### D5. A shell script, not GoReleaser

`scripts/dist.sh` is 40 lines. It uses only `go`, `tar` and `shasum` or `sha256sum`, and it runs on a laptop the same as in CI. GoReleaser adds a tool and a configuration file for 4 targets. GoReleaser can replace the script when Homebrew is added.

Each archive name has no version, for example `i2a_darwin_arm64.tar.gz`. Thus the URL `releases/latest/download/i2a_darwin_arm64.tar.gz` is always the latest release. Each archive holds `i2a`, `LICENSE` and `README.md`.

### D6. Version for `go install`

When `main.version` is `dev`, `main.go` reads `debug.ReadBuildInfo()`. If the module version is not `(devel)`, it uses that version. Thus `go install github.com/sondt2709/inbox2agent/cmd/i2a@v1.2.0` shows `v1.2.0`.

## Risks / Trade-offs

- [The first release is `1.0.0`, because semantic-release starts at `1.0.0`] → Accept. `i2a` works and has users. Each breaking change then makes a major version.
- [A CI failure on `main` blocks all releases until a fix is merged] → This is the intent. The next push that passes releases all commits since the last tag.
- [macOS shows a warning for a binary that a browser downloads, because it is not signed] → The README uses `curl`, which does not set the quarantine attribute. Signing is a later change.
- [npm packages download at each run] → The versions are exact. `npx` uses the npm registry checksums.

## Migration Plan

1. Merge this change. The first push to `main` makes `v1.0.0`.
2. Rollback: delete the `release` job from `ci.yml`. To remove a bad release, delete the GitHub release and its tag (`gh release delete v<version> --cleanup-tag`).
