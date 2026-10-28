## Context

The repository is empty except for the OpenSpec setup. This change creates the first version of `i2a`. Later changes add more sources (WhatsApp, GitHub, Linear), more drivers (direct agent CLIs) and the release pipeline (GoReleaser, Homebrew). The design must make those changes small: a new source or driver must not change the store, the renderer or the daemon.

Constraints:

- macOS first. Linux later. The binary must build without a C compiler.
- Message text comes from other people. The agent that receives it can run commands. In Orca, an agent can run with permission prompts turned off.
- The Mattermost user may keep messages unread on purpose. `i2a` must not change the read state.
- The repository is public. Committed files must not contain local paths, personal data or data from a real Mattermost server.

## Goals / Non-Goals

**Goals:**

- One static binary `i2a` with a CLI, a TUI and a daemon.
- A source interface, a driver interface and a store that later changes reuse without modification.
- The Mattermost source with channel selection, direct messages and mentions.
- The `orca` driver with three session modes, two payload modes and two templates.
- A sanitizer that keeps message text inside a data block.
- An end-to-end scenario list and a harness that drives the real binary through a terminal.
- CI that runs lint, unit tests, a vulnerability check, a secret scan and the end-to-end scenarios that need no local apps.

**Non-Goals:**

- WhatsApp, GitHub and Linear sources (later changes).
- OAuth for any source (later change).
- Mattermost followed-thread replies in channels that are not selected (later change).
- The release pipeline and the Homebrew formula (next change).
- Direct agent drivers (`claude -p`, `codex exec`, `opencode run`) (later change).
- Linux service files (systemd).

## Decisions

### D1. One Go module, monorepo layout

```
cmd/i2a/              main package
internal/app/         paths and wiring
internal/config/      TOML schema, defaults, validation
internal/secrets/     keychain and file backends
internal/store/       SQLite store and migrations
internal/source/      Source interface and registry
internal/source/mattermost/
internal/render/      sanitizer, digest, templates
internal/agent/       Driver interface, routing, orca, dryrun
internal/daemon/      collect and dispatch loop, timing
internal/service/     macOS LaunchAgent
internal/tui/         i2a config screens
internal/cli/         cobra commands
test/e2e/             scenario list, terminal harness, Mattermost fixture server
scripts/              repository checks
```

One module keeps the build and the dependency updates simple. A `go.work` file with more modules is possible later if a source pulls in many dependencies. Alternative: one module for each source. Rejected for now, because it adds release work and gives no benefit for one binary.

### D2. Pure-Go SQLite (`modernc.org/sqlite`)

The store uses `modernc.org/sqlite` with WAL mode and a busy timeout. This keeps `CGO_ENABLED=0`, so cross-compilation and the Homebrew build need no C compiler. The WhatsApp library used in a later change accepts this driver. Alternative: `mattn/go-sqlite3`. Rejected, because it needs CGO.

### D3. Configuration and secrets

- Paths follow XDG on all platforms: `$XDG_CONFIG_HOME/i2a/config.toml` (default `~/.config/i2a/config.toml`) and `$XDG_DATA_HOME/i2a/` (default `~/.local/share/i2a/`) for `inbox.db`, `digests/` and `i2a.log`.
- `I2A_HOME` overrides both paths with one directory. Tests use this for isolation.
- The configuration file holds no secrets. Each secret has the key `<source-id>/token`.
- Secret backends: `keychain` (default, through `zalando/go-keyring`) and `file` (`<data>/secrets.json`, mode 0600). `I2A_SECRET_BACKEND=file` selects the file backend. CI and headless Linux use it.
- The format is TOML, because people edit it by hand and it supports multi-line strings for the templates.

Configuration shape (example):

```toml
[timing]
interval = "15m"        # dispatch interval
lookback = "24h"        # first-run window and maximum message age
active_hours = ""       # "08:00-20:00" local time; empty means always

[[sources]]
id = "work"
type = "mattermost"
url = "https://chat.example.com"
poll_every = "1m"
all_direct = true       # all direct and group messages
mentions = true         # mentions of the user in any channel
channels = ["<channel-id>"]
profile = "triage"
[sources.channel_profiles]  # optional route for one channel
"<channel-id>" = "reviewer"

[[profiles]]
name = "triage"
driver = "dry-run"      # dry-run | orca
agent = "claude"
workspace = "~/notes"
session = "reuse"       # new | reuse | worktree
payload = "pointer"     # pointer | inline
inline_max_bytes = 8000
message_max_chars = 2000
model = ""
primer = """..."""
batch = """..."""
```

### D4. Store schema

```sql
messages(
  id INTEGER PRIMARY KEY,
  source_id TEXT, external_id TEXT,
  channel_id TEXT, channel_name TEXT, channel_kind TEXT,  -- channel | direct | group
  author TEXT, text TEXT, url TEXT,
  created_at INTEGER,            -- Unix ms, from the source
  collected_at INTEGER,
  profile TEXT,
  dispatch_id INTEGER NULL,      -- NULL: not sent yet
  state TEXT,                    -- pending | previewed | sent | expired
  UNIQUE(source_id, external_id))
cursors(source_id TEXT, key TEXT, value TEXT, PRIMARY KEY(source_id, key))
dispatches(id INTEGER PRIMARY KEY, profile TEXT, driver TEXT, created_at INTEGER,
  status TEXT, digest_path TEXT, message_count INTEGER, detail TEXT)  -- status: sent | failed | held
sessions(profile TEXT PRIMARY KEY, handle TEXT, created_at INTEGER)
```

The `UNIQUE(source_id, external_id)` constraint removes duplicates. A source can return the same message two times, for example after an edit.

### D5. Source interface

```go
type Source interface {
    ID() string
    Collect(ctx context.Context, since Cursor) ([]Message, Cursor, error)
}
```

Each source returns new messages and the next cursor. The daemon saves the messages and the cursor in one transaction. The source does not write to the store. This keeps the sources simple to test with recorded responses.

### D6. Mattermost collection

The source uses a small typed HTTP client with 5 endpoints, not the official Go module. The official module needs a newer Go patch version and adds many dependencies.

Algorithm for one poll:

1. `GET /api/v4/users/me` (cached): the user ID and the username.
2. `GET /api/v4/users/me/teams`, then `GET /api/v4/users/me/teams/{team}/channels` for each team. The channel list includes `last_post_at`.
3. Select candidate channels: the selected channels, plus the direct and group channels if `all_direct` is on, plus all member channels if `mentions` is on.
4. Skip a channel if `last_post_at` is not newer than its cursor.
5. `GET /api/v4/channels/{id}/posts?page=<n>&per_page=200`. This call returns the posts newest first. Stop at the first post whose `create_at` is not after the cursor. Read only the IDs in `order`, because `posts` also holds old thread roots. Keep a post only if it is not deleted, not a system post and not written by the user. Do not use `posts?since=`: it filters on `update_at` and has an unordered `LIMIT 1000` with no pagination, so it returns edited old posts and can lose new posts. In a channel that is a candidate only because of `mentions`, keep a post only if it contains `@<username>`.
6. Resolve author names with `POST /api/v4/users/ids` (cached).
7. The cursor of each channel is the largest `create_at` seen. On the first poll, the cursor is `now - lookback`.

The source never calls `POST /api/v4/channels/members/me/view`. That call marks a channel as read. A test compares the unread counts before and after a poll.

A permalink has the form `<url>/<team-name>/pl/<post-id>`.

### D7. Sanitizer and batch format

Each message becomes one JSON object on one line:

```json
{"n":1,"source":"mattermost","source_id":"work","channel":"town-square","kind":"channel","author":"alice","time":"2026-01-02T03:04:05Z","url":"https://...","text":"...","truncated":false}
```

The sanitizer does these steps on the text before encoding:

1. Remove Unicode bidi controls (U+202A–U+202E, U+2066–U+2069), zero-width characters (U+200B–U+200F, U+2060, U+FEFF) and C0/C1 control characters except newline and tab.
2. Truncate to `message_max_chars` runes and set `truncated` to true.
3. Encode with `json.Marshal`. Newlines, quotes and control characters become escapes, so the text cannot start a new line.

The block:

````
```i2a-data-<nonce>
{...}
{...}
```
````

The nonce is 6 random bytes in hexadecimal, new for each batch. Each data line starts with `{`, so a message cannot close the fence. The header text, from the template, says that the data is untrusted.

Alternative: escape only the backticks. Rejected, because a message can still add lines that look like instructions outside of the JSON.

### D8. Templates

Go `text/template` with these fields: `.BatchID`, `.Profile`, `.Count`, `.Sources`, `.DigestPath`, `.Messages` (the fenced block), `.Nonce`, `.WindowStart`, `.WindowEnd`.

- `primer`: standing instructions. In `reuse` mode, the driver sends it one time when it starts a session. In `new` and `worktree` mode, the driver puts it before the batch prompt.
- `batch`: the prompt for each batch. The default pointer template names the digest file. The default inline template includes `.Messages`.

The digest file is always written, in both payload modes. It is the record of what the agent received.

In inline mode, if the rendered prompt is larger than `inline_max_bytes`, the driver uses the pointer template. Orca passes prompts as command arguments, and its RPC message limit is 1 MiB. The configuration rejects `inline_max_bytes` above 65,536.

### D9. Drivers and routing

```go
type Driver interface {
    Deliver(ctx context.Context, b Batch) (Result, error) // Result.Status: sent | held
}
```

Routing: each message gets the profile from `channel_profiles`, or else from the `profile` of its source, or else from the first profile.

`dry-run` driver: writes the digest file and returns `sent`. The daemon marks the messages of a dry-run batch `previewed`, not `sent` (see D10).

`orca` driver: runs the `orca` executable with `--json` and parses the result. The driver resolves the executable path when the configuration is saved, because a LaunchAgent has a short `PATH`.

The driver starts an agent without a prompt, waits until its TUI is idle, and then submits the prompt with `orca terminal send`. It does not use `--prompt` on `agent launch` or `worktree create`. With `--prompt`, Orca 1.4.240 types the prompt while the agent starts. A real run showed two failures: the prompt stayed in the input box with no Enter (`prompt.outcome` was `handed-to-terminal`), and the prompt did not arrive (`not-delivered`). `orca terminal send --wait-submit` returns the stage `turn_started` when the agent starts the turn, so the driver can prove the submission.

| Step | Commands |
|---|---|
| start (`new`) | `orca agent launch --agent <a> --surface terminal --worktree path:<ws> --json` |
| start (`worktree`) | `orca worktree create --repo path:<ws> --name i2a-<batch-id> --no-parent --agent <a> --json` |
| submit | `orca terminal wait --terminal <h> --for tui-idle --timeout-ms 90000`, then `orca terminal send --terminal <h> --text <prompt> --enter --wait-submit 30` |
| `reuse` | If a stored handle exists and `orca terminal show` reports it as live: `orca terminal wait --for tui-idle --timeout-ms 5000`. If the agent is idle: submit the batch prompt. If the agent is busy: return `held`. If no live handle exists: start as in `new` mode, submit the primer and the batch prompt, and store the handle. |

`--surface terminal` is fixed, because the driver needs a terminal handle. A send succeeds when `accepted` is true. When the stages do not include `turn_started`, the driver sends one bare Enter (`orca terminal send --terminal <h> --enter`), because the text can stay in the input box. An Enter in an empty input box has no effect. The driver records the result in `dispatches.detail`: "turn started" or "turn not proven, Enter sent again".

The driver never sends the same prompt text two times to one session.

### D10. Delivery guarantee

The daemon sends messages at least once:

1. It creates a dispatch row and renders the digest.
2. It calls the driver.
3. When the result is `sent`, it marks the messages `sent` in one transaction.
4. When the result is `held` or an error, it keeps the messages `pending`, so the next tick sends them again.

Pending messages older than `lookback` change to `expired`, and the daemon does not send them.

A dry-run batch is a preview. Its messages become `previewed`. A `dry-run` profile takes only `pending` messages, so the daemon does not preview a message two times. A profile with a real driver takes `pending` and `previewed` messages. Thus, when you change a profile from `dry-run` to `orca`, the agent gets the messages that you previewed. `previewed` messages older than `lookback` also become `expired`. Schema version 2 adds this state: the migration marks the messages of earlier `dry-run` dispatches `previewed`.

### D11. Daemon and timing

- `i2a daemon` runs in the foreground. It polls each source on its `poll_every` value and dispatches on `timing.interval`.
- Outside `active_hours`, the daemon collects messages but does not dispatch them.
- `i2a run --once` collects and dispatches one time. It ignores `interval` and `active_hours`.
- A file lock (`<data>/daemon.lock`, `flock`) allows one daemon only.
- The daemon reads the configuration again when the file changes.
- Logs use `log/slog` in text format to stderr. The LaunchAgent sends stderr to `<data>/i2a.log`.
- Retention: once each hour, the daemon deletes sent and expired messages and digest files that are older than `retention` (default `7d`).

### D12. macOS LaunchAgent

`i2a service install` writes `~/Library/LaunchAgents/io.github.sondt2709.i2a.plist` with these values:

- `ProgramArguments`: the absolute `i2a` path and `daemon`.
- `RunAtLoad` and `KeepAlive`.
- `EnvironmentVariables.PATH` (with the directory of the `orca` executable).
- The log file paths.

Then it runs `launchctl bootstrap gui/<uid> <plist>`. `uninstall` runs `launchctl bootout gui/<uid>/<label>` and deletes the file. `status` runs `launchctl print`. On other platforms, these commands return "not supported on this platform".

### D13. TUI

`i2a config` is a dashboard: one full-screen Bubble Tea v2 program (alternate screen, mouse on). It works like a web dashboard, not like a sequence of questions.

```
 ◆ i2a  inbox2agent                                      ● service running · pid 4211
  1 Overview   2 Sources   3 Profiles   4 Schedule   5 Activity   ● unsaved changes · ctrl+s save
 ╭ Sources ─────────╮╭ work · mattermost ─────────────────────────────────────────╮
 │ ▌work mattermost ││ ▌ Server URL     https://chat.example.com                  │
 │  + Add source    ││                  The address of the Mattermost server.     │
 │                  ││   All direct     [On] Off                                  │
 │                  ││   Channels       3 selected                                │
 │                  ││     Search: ▏    45 of 45 · 3 selected                     │
 │                  ││       [ Save changes ]  [ Cancel ]  [ Remove source ]      │
 ╰──────────────────╯╰────────────────────────────────────────────────────────────╯
 tab ↑↓ fields · ←/→ space choose · ctrl+s save · esc back to the list
```

Layout: a header bar (name and service state), a tab bar (the unsaved mark at the right), the tab content, and a footer with the keys of the current focus. A message (for example "Saved source work.") shows in the footer for a short time.

The Sources and Profiles tabs have a list at the left and a detail panel at the right. The detail panel is a form: the form of the selected item, or the form of a new item when "+ Add" is selected. Add and edit use the same place and the same controls, as in a web dashboard. Schedule is one form. Overview has cards with live counts and the last dispatches. Activity is a table of dispatches with a viewer for each digest file.

Form controls (`internal/tui/form.go`, a small form engine, not Huh): a label column; text inputs and a hidden token input with a filled background; segmented controls for choices (driver, session, payload, profile) and for On/Off switches; text areas for the prompt templates; the channel picker; a row of buttons (the first is the primary action). The help text of the focused field shows under it. A field error shows in red under the field, and the focus moves to the first field with an error.

Focus: the list has the keyboard first. `enter`, `→` or a click in the detail panel gives the keyboard to the form; the panel border changes to the accent color and the list turns grey. In a form: `tab` / `shift+tab` and `↑` / `↓` move between the fields; `←` / `→` and `space` change a choice or a switch (`y` / `n` set a switch); `enter` goes to the next field and, in the last field before the buttons, submits; `ctrl+s` submits from any field; `esc` goes back to the list and, when the form has changes, asks to discard them.

Save: a form writes the configuration file (and the token) when the user submits it, as a web form does. The dashboard has no second "save all" step. A save that fails validation changes nothing. Quick keys in the Sources list (`D` all direct, `m` mentions, `x` remove) also save at once. The running daemon reads the file again when it changes.

Add source: step 1 is the account (ID, URL, token) with the button "Connect". It checks the connection and reads the channels, with a progress line in place of the buttons. A failure shows "Connection failed. Nothing was saved." and the server error under the buttons. Step 2 is in the same panel: "✓ Connected as <user>", the channel picker (with the focus), the switches, the profile and the button "Add source".

Edit source: a new token or a new URL is checked with the server before the save.

Global keys (when the list has the keyboard): `1` to `5` and `tab` / `shift+tab` change the tab, `q` quits, `?` shows all keys. `ctrl+c` quits from anywhere; with unsaved form changes, it asks first. Dialogs are only for confirmations, the digest viewer and the key help.

Non-interactive commands (`i2a source add mattermost --token-stdin`, `i2a source list`, `i2a source channels <id>`) do the same actions for scripts.

History: the first version asked one Huh form at a time, inline. The second version was full-screen but a stack of menus. The third version had tabs, with forms in dialogs. The first user found these versions "like a normal CLI, not a TUI" and asked for add and edit in the detail panel, like a web dashboard.

### D16. Desktop notifications

After each dispatch with a real driver, the daemon shows a macOS notification: "N messages sent to <agent>" for `sent`, and "Dispatch failed" with the error for a failure. `held` and `dry-run` dispatches do not notify. The notifier runs `osascript` and gives the title and the text as arguments (`on run argv`), so no text becomes AppleScript code. The text contains the profile, the count and the driver detail, not message text. `[notify] enabled = false` turns notifications off. The default is `true`. On other platforms, the notifier does nothing. The environment variable `I2A_NOTIFY=off` turns them off for tests.

### D14. End-to-end tests drive the real binary like a person

The lesson from Orca's test suite: drive the real program through its real interface, and keep automated runs in the background.

- The harness builds `i2a`. It starts it in a detached `tmux` session with a fixed size (120×40). It sends key presses with `tmux send-keys` and reads the screen with `tmux capture-pane`. It waits for visible text with a timeout. It never attaches to the terminal of the developer.
- The harness saves each screen capture to `test/e2e/out/` (not committed). These captures are the evidence.
- Mattermost runs in Docker (`mattermost/mattermost-preview`) with personal access tokens on. A setup step creates two users, a token and channels through the real API.
- Scenarios that need Orca run only when `I2A_E2E_ORCA=1`. CI does not run them.
- The scenario list is in `test/e2e/SCENARIOS.md`. Each scenario has an ID, steps as a person does them, and the expected visible result.

### D15. Repository quality gates

- `ci.yml` on each pull request and push to `main`:
  - `golangci-lint` v2.
  - `go test -race ./...` on macOS and Linux.
  - `govulncheck`.
  - `gitleaks`.
  - A cross-build for `darwin/arm64`, `darwin/amd64` and `linux/amd64`.
  - The end-to-end scenarios that need only Docker and tmux, on Linux.
- `pr-title.yml` checks Conventional Commits. The squash title becomes the release version input in the next change.
- `scripts/leakcheck.sh` reads forbidden terms from `.leakcheck.local` (not committed) and searches all tracked and staged files. `task leakcheck` runs it.

## Risks / Trade-offs

- **[Risk]** An agent obeys instructions in message text. → **Mitigation:**
  - The sanitizer and the fence keep the text as data.
  - The templates say that the data is untrusted.
  - The default driver is `dry-run`.
  - The TUI warns about Orca permission bypass.

  The risk stays. An LLM can still obey text in a data block.
- **[Risk]** The `orca` CLI output changes in a new Orca version. → **Mitigation:** the driver parses only the fields that it needs. Unit tests use JSON captured from Orca 1.4.240. The driver logs the raw output when parsing fails.
- **[Risk]** Polling many Mattermost channels sends many requests. → **Mitigation:** the source skips channels whose `last_post_at` did not change, and it handles HTTP 429 with backoff.
- **[Risk]** A `reuse` session gets long and slow. → **Mitigation:** the user can change to `new` mode. Session rotation is a later change.
- **[Trade-off]** At-least-once delivery can send a batch two times if the driver fails after the agent got the prompt. The driver records the submission stages to make this visible.
- **[Trade-off]** The dashboard has its own small form engine. It uses the Bubbles text input and text area, so the text editing is not written again.

## Migration Plan

This is the first version, so no data migration is necessary. To roll back, run `i2a service uninstall`, then delete the binary, the configuration directory and the data directory. The store has a `schema_version` table for later migrations.

## Open Questions

- Does `orca repo add` accept a folder that is not a git repository? If not, the `orca` driver tells the user to run `git init` in the workspace, or to select a git folder.
- Is the `path:` selector accepted by `orca worktree create --repo`? If not, the driver resolves the repo ID with `orca repo list --json`.
