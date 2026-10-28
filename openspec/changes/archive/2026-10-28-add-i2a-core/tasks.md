## 1. End-to-end scenarios first

- [x] 1.1 Write `test/e2e/SCENARIOS.md`: one row for each main scenario, with an ID, the steps as a person does them, the expected visible result, and where it runs (CI or local only)
- [x] 1.2 Add the Mattermost fixture: `test/e2e/mattermost/compose.yaml` (`mattermost/mattermost-preview`, personal access tokens on) and a setup step that creates two users, a token, two channels and a direct channel through the real API
- [x] 1.3 Add the terminal harness in `test/e2e/harness`: build `i2a`, start a detached `tmux` session (120×40), send keys, wait for visible text with a timeout, save each screen capture to `test/e2e/out/`

## 2. Repository setup

- [x] 2.1 Add `go.mod` (`github.com/sondt2709/inbox2agent`, Go 1.26), `cmd/i2a/main.go` with a version stamp, `.gitignore` (`tmp/`, `dist/`, `test/e2e/out/`, `*.local*`, `.leakcheck.local`) and an MIT `LICENSE`
- [x] 2.2 Add `Taskfile.yml` with `build`, `test`, `lint`, `e2e`, `leakcheck` and `fmt`, and `.golangci.yml` (v2)
- [x] 2.3 Add `scripts/leakcheck.sh`: read terms from `.leakcheck.local`, search all tracked and staged files, exit 1 on a match, exit 0 with a notice when the term file is missing

## 3. Configuration and secrets

- [x] 3.1 Add `internal/app` paths: XDG defaults and the `I2A_HOME` override
- [x] 3.2 Add `internal/config`: the TOML schema, defaults, load, save and validation (durations, active hours, limits, unique names, profile references, template parsing)
- [x] 3.3 Add `internal/secrets`: the keychain backend and the file backend (mode 0600), selected by `I2A_SECRET_BACKEND`
- [x] 3.4 Add unit tests for the paths, the validation errors and the file backend permissions

## 4. Message store

- [x] 4.1 Add `internal/store`: open with WAL and a busy timeout, mode 0600, schema version 1 with the tables `messages`, `cursors`, `dispatches` and `sessions`
- [x] 4.2 Add these store operations: save a poll result and its cursor in one transaction, list pending messages by profile, mark messages sent, expire old pending messages, record dispatches, get and set session handles, and delete data older than the retention value
- [x] 4.3 Add unit tests for duplicate messages, the atomic cursor update, the state changes and the retention

## 5. Mattermost source

- [x] 5.1 Add `internal/source` with the `Source` interface, the `Message` type and the registry
- [x] 5.2 Add the Mattermost HTTP client: `users/me`, teams, channels for each team, channel posts by page (newest first, stop at the cursor), users by IDs, with the bearer token and backoff on HTTP 429
- [x] 5.3 Add the collection algorithm: candidate channels, `last_post_at` skip, post filters, mention filter, cursor for each channel, permalinks
- [x] 5.4 Capture real responses from the Docker Mattermost into `internal/source/mattermost/testdata/` and add unit tests for each spec scenario with these fixtures

## 6. Batch rendering

- [x] 6.1 Add `internal/render` sanitizer: hidden-character removal, truncation, one JSON line for each message
- [x] 6.2 Add the fenced block with a random nonce, the digest file (mode 0600) and the default `primer`, `pointer` and `inline` templates
- [x] 6.3 Add the payload selection with the inline-size fallback
- [x] 6.4 Add unit tests with attack inputs: a fence inside a message, a forged label, ANSI escapes, `\x03`, bidi controls, a 1 MB message; check that each test can fail

## 7. Agent delivery

- [x] 7.1 Add `internal/agent`: the `Driver` interface, the `Batch` and `Result` types, routing by channel, source and default profile
- [x] 7.2 Add the `dry-run` driver
- [x] 7.3 Add the `orca` driver: run the executable with `--json`, parse the results, `new` mode, `reuse` mode (show, wait for idle, send, held, relaunch) and `worktree` mode
- [x] 7.4 Resolve the Orca open questions from the design (folder that is not a git repository, `path:` selector for `--repo`) against the real Orca and update the driver
- [x] 7.5 Add unit tests for the `orca` driver with a fake executable that returns JSON captured from the real Orca

## 8. Daemon and service

- [x] 8.1 Add `internal/daemon`: poll scheduling for each source, dispatch on the interval, active hours (with ranges across midnight), expiry, retention once each hour, configuration reload, and the single-daemon lock
- [x] 8.2 Add `internal/service`: the LaunchAgent file, `install`, `uninstall` and `status` through `launchctl`, with `PATH` from the `orca` location; "not supported on this platform" on other systems
- [x] 8.3 Add unit tests for active hours, the dispatch flow with a test driver, and the LaunchAgent file content

## 9. CLI and TUI

- [x] 9.1 Add cobra and fang commands: `config`, `run --once`, `daemon`, `status`, `source add mattermost|list|channels|remove`, `service install|uninstall|status`, `version`
- [x] 9.2 Add the `i2a config` TUI with Huh v2: main menu, add Mattermost source with the connection check, filterable channel picker with the toggles and the profile select, profiles screen with the Orca warning, timing screen, save and exit
- [x] 9.3 Add the `status` output: paths, sources, profiles, pending counts, last dispatch, service state

## 10. End-to-end runs

- [x] 10.1 Implement the CI scenarios from `SCENARIOS.md` as Go tests with the `e2e` build tag, using the harness and the Docker Mattermost
- [x] 10.2 Implement the local-only Orca and LaunchAgent scenarios behind `I2A_E2E_ORCA=1` and `I2A_E2E_SERVICE=1`
- [x] 10.3 Run all scenarios on the local machine and keep the screen captures, the digest files, the store rows and the Orca terminal reads as evidence
- [x] 10.4 Record the TUI with VHS for the README

## 11. CI and documentation

- [x] 11.1 Add `.github/workflows/ci.yml`: lint, `go test -race` on macOS and Linux, `govulncheck`, `gitleaks`, cross-build, and the CI end-to-end scenarios on Linux
- [x] 11.2 Add `.github/workflows/pr-title.yml` (Conventional Commits)
- [x] 11.3 Write `README.md`: what `i2a` does, install, quick start, configuration, profiles and templates, security, sources, development
- [x] 11.4 Write `CLAUDE.md`: commands, architecture, the writing rules, the data rules, and the OpenSpec and commit conventions
- [x] 11.5 Run `task leakcheck` and `gitleaks` on the final tree

## 12. Fixes after the first real use

- [x] 12.1 Add a store test that fails: a dry-run batch, then an `orca` profile, must send the previewed messages
- [x] 12.2 Add the `previewed` state, schema version 2 and its migration; `dry-run` profiles take `pending` only, other profiles take `pending` and `previewed`; show `previewed` in `i2a status`
- [x] 12.3 Capture real Orca outputs for a launch with no prompt, a worktree create with no prompt, a wait after launch and a send with no `turn_started`
- [x] 12.4 Change the `orca` driver: start the agent without `--prompt`, wait for `tui-idle` (90 seconds), submit with `--wait-submit`, and send one bare Enter when `turn_started` is not proven
- [x] 12.5 Add `internal/notify` and the `[notify] enabled` setting; notify after real dispatches only; pass the text to `osascript` as arguments
- [x] 12.6 Make `i2a config` one full-screen Bubble Tea program: header, path line, body with the embedded Huh form, summary panel, status line, key help, `Esc` for back, spinners in the body
- [x] 12.7 Update the e2e harness and scenarios for the full-screen TUI, and add a scenario for `Esc` and the summary panel
- [x] 12.8 Run the unit tests, the CI e2e scenarios and the Orca e2e scenarios again; reproduce the user's prompt with the real Orca and confirm `turn_started`
- [x] 12.9 Replace the channel multi-select with a picker that has an always-focused search box; test it with unit tests and scenario S04
- [x] 12.10 Show direct and group conversations in the channel picker (most recent first, without the user in group labels); test with unit tests and scenario S22

## 13. Dashboard TUI

- [x] 13.1 Replace the menu stack with a dashboard model: header, tab bar, footer with the keys of the tab, dialogs as compositor layers, global keys, mouse for tabs and rows, a 3-second refresh of the store counts
- [x] 13.2 Add the tabs Overview, Sources, Profiles, Schedule and Activity with their keys
- [x] 13.3 Move the forms into dialogs: add source with the connection check, channel picker, profile, schedule value, confirmations, digest viewer, help, quit with unsaved changes
- [x] 13.4 Give the TUI the store (counts, dispatches) and the service install action
- [x] 13.5 Add unit tests for the model (tab keys, dialogs, save, quit with unsaved changes, mouse) and update the TUI scenarios S02, S03, S04, S09, S10, S20 and S22
- [x] 13.6 Move add and edit into the detail panel: a form engine with inputs, segmented choices, switches, text areas, the channel picker, buttons and errors under the fields; a form saves when the user submits it; tests for the form keys, save, errors, discard and clicks; update the scenarios
