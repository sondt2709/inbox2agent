## Why

New messages arrive in many tools: chat, issue trackers and code hosts. A person must read each tool, then copy the important items to an AI agent. `i2a` (inbox2agent) collects new messages from these tools and sends them to an AI agent in batches. This change builds the core and the first source (Mattermost), so that each later source is a small change.

## What Changes

- Add a Go CLI `i2a` in a monorepo layout. The binary is static (`CGO_ENABLED=0`).
- Add a TOML configuration file and keep secrets in the operating-system keychain.
- Add a local SQLite store (`inbox.db`) for messages, source cursors and dispatch records.
- Add a source interface and the Mattermost source. It uses a personal access token and read-only API calls. It does not mark messages as read.
- Add a sanitizer that makes message text safe to give to an agent: one JSON object per line, a fence with a random label for each batch, removal of hidden Unicode characters, and truncation.
- Add agent profiles and routes. A profile selects a driver, a workspace, a session mode, a payload mode and two prompt templates (`primer` and `batch`).
- Add two drivers: `orca` (new session, reuse session, or new worktree) and `dry-run` (writes the digest file only).
- Add a daemon with an interval, a lookback window and optional active hours. Add `i2a service install|uninstall` for a macOS LaunchAgent. The daemon shows a macOS notification after each real dispatch.
- Add a full-screen TUI (`i2a config`) to connect sources, select channels for each source, and edit profiles and timing.
- Add an end-to-end scenario list and a harness that drives the real binary through a terminal, as a person does.
- Add repository quality gates: CI (lint, tests, vulnerability check, secret scan), a pull-request title check, and a local leak check.

## Capabilities

### New Capabilities
- `configuration`: the configuration file, the secret storage, and the `i2a config` TUI with the source and channel picker.
- `message-store`: the local store for messages, cursors, dispatch records and retention.
- `mattermost-source`: the connection to a Mattermost server and the collection of new messages from the selected channels.
- `batch-rendering`: the sanitizer, the digest file and the prompt templates.
- `agent-delivery`: the agent profiles, the routes, the `orca` driver and the `dry-run` driver.
- `daemon-service`: the collection and dispatch loop, the timing settings and the macOS LaunchAgent.

### Modified Capabilities

None.

## Impact

- New code: all of the repository. No current code changes.
- New dependencies: cobra and fang (CLI), Bubble Tea, Huh and Lip Gloss v2 (TUI), go-toml v2, modernc SQLite, go-keyring.
- New files on the user machine: the configuration file, `inbox.db`, the digest files, the log file and one LaunchAgent file.
- New outgoing connections: the Mattermost API (read only) and the local Orca runtime.
- Security: message text from other people goes to an agent that can run commands. The sanitizer reduces this risk. It does not remove it. The `dry-run` driver is the default.
- New CI workflows in `.github/workflows/`.
