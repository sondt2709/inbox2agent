## 1. Build

- [x] 1.1 Add `scripts/dist.sh <version>`: cross builds with `CGO_ENABLED=0`, `-trimpath` and the version in `-ldflags`, one archive for each platform, `checksums.txt`
- [x] 1.2 Add `task dist` to `Taskfile.yml`
- [x] 1.3 Read the module version from `debug.ReadBuildInfo()` in `cmd/i2a/main.go` when the version is `dev`, with a test

## 2. Release

- [x] 2.1 Add `.releaserc.json` with the `conventionalcommits` preset and the release rules
- [x] 2.2 Add the `release` job to `.github/workflows/ci.yml` (needs all jobs, push to `main` only, own concurrency group, dry run, build, publish), and cancel runs in progress only for pull requests
- [x] 2.3 Prove the release rules with a local dry run against a local Git repository: chore gives patch, feat gives minor, `!` gives major

## 3. Docs and repository

- [x] 3.1 Update `README.md` (install from a release), `CLAUDE.md` (release commands and rules) and the comments in `pr-title.yml`
- [x] 3.2 Set the repository to squash merge only, with the PR title as the squash commit subject
- [ ] 3.3 Push to `main`, then confirm that CI passes, the release `v1.0.0` exists with 5 assets, and the downloaded darwin/arm64 binary shows `1.0.0`
