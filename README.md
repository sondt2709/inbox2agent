# inbox2agent (`i2a`)

`i2a` collects new messages from your chat and work tools and sends them to an AI agent in batches.

![i2a config, then i2a run --once](docs/demo/i2a-config.gif)

```
 Mattermost ──┐                                         ┌─► Orca: new session, reused session or new worktree
 (WhatsApp,   │   i2a daemon ─► inbox.db ─► sanitizer ─►│     (Claude Code, Codex, OpenCode and more)
  GitHub,     ├─  polls the channels      one JSON line  └─► dry-run: writes the digest file only
  Linear:     │   that you select         per message,
  planned)  ──┘                           fenced block
```

## Status

| Part | State |
|---|---|
| Mattermost source (personal access token) | Available |
| `orca` driver and `dry-run` driver | Available |
| Configuration TUI, daemon, macOS LaunchAgent | Available |
| Release binaries for macOS and Linux (amd64, arm64) | Available |
| Homebrew package | Planned |
| WhatsApp, GitHub and Linear sources | Planned |
| OAuth sign-in, direct `claude` / `codex` / `opencode` drivers | Planned |

`i2a` runs on macOS. The binary also builds for Linux, but the background service is macOS only.

## Install

Download the binary of the latest release. For an Apple silicon Mac:

```sh
curl -fsSL -o i2a.tar.gz https://github.com/sondt2709/inbox2agent/releases/latest/download/i2a_darwin_arm64.tar.gz
tar -xzf i2a.tar.gz i2a
mkdir -p ~/.local/bin && mv i2a ~/.local/bin/
i2a --version
```

For other computers, replace `darwin_arm64` with `darwin_amd64`, `linux_amd64` or `linux_arm64`. Each release also has `checksums.txt` with the SHA-256 sum of each archive. Make sure that `~/.local/bin` is in your `PATH`.

The binary is not signed. If you download it with a browser, macOS can block it. Then run `xattr -d com.apple.quarantine ~/.local/bin/i2a`.

You can also install with Go 1.26 or later:

```sh
go install github.com/sondt2709/inbox2agent/cmd/i2a@latest
```

### Versions

Each change that is merged into `main` makes a new release. The PR title gives the version: `feat` makes a minor version, `!` before the colon makes a major version, and all other types make a patch version. See [Releases](https://github.com/sondt2709/inbox2agent/releases).

## Quick start

1. Create a personal access token in Mattermost: **Profile > Security > Personal Access Tokens**. If you do not see this section, ask your administrator to turn on personal access tokens.
2. Run `i2a config`. Press `2` (the Sources tab), then `enter`. Type the ID, the server URL and the token, and press `enter` to connect.
3. In the channel list, type part of a channel name or a person's name to search, and press space to select it. The list also has your direct and group conversations (`direct / @bob`), the most recent first. `Esc` clears the search. Press `tab` to go to the switches for direct messages and mentions. Press `ctrl+s` to add the source, then `q` to quit.
4. Run `i2a run --once`. The default profile uses the `dry-run` driver, so `i2a` writes a digest file and starts no agent. Read the digest to see what an agent would get.
5. Run `i2a config` again. Press `3` (the Profiles tab) and `enter` to edit `triage`. Set the driver to `orca` (`→`), the agent (for example `claude`) and the workspace folder, and press `ctrl+s`. The next batch also contains the messages that the dry run previewed.
6. Press `esc`, then `1` (the Overview tab), then `i` to install the background service. Or run `i2a service install`. `i2a` now runs in the background at login.

Run `i2a status` to see the sources, the profiles, the message counts, the last dispatches and the service state.

### The dashboard

`i2a config` is a dashboard with 5 tabs. Press `1` to `5` or `tab` to change the tab, or click a tab.

| Tab | Shows |
|---|---|
| 1 Overview | the service state, the message counts, the sources, the profiles, the schedule, the files, the last dispatches (`i` installs or restarts the service, `u` uninstalls it) |
| 2 Sources | your sources at the left; the form of the selected source at the right |
| 3 Profiles | your profiles at the left; the form of the selected profile at the right |
| 4 Schedule | the form of interval, lookback, active hours, retention and notifications |
| 5 Activity | the last 50 dispatches (`enter` shows the digest file) |

To add or edit, select the item (or "+ Add") and press `enter`: the form at the right gets the keyboard.

| In a form | Key |
|---|---|
| Next or previous field | `tab` / `shift+tab`, or `↓` / `↑` |
| Change a choice or a switch | `←` / `→` or `space` (`y` / `n` for a switch) |
| Save | `ctrl+s`, the first button, or `enter` in the last field |
| Back to the list | `esc` (it asks before it discards changes) |

A form saves to `config.toml` when you submit it. A running daemon reads the file again. In the Sources list, `D` and `m` switch "all direct" and "mentions" at once, and `x` removes a source. `?` shows all keys.

## How it works

1. The daemon polls each source on its `poll_every` interval and saves new messages in `inbox.db`.
2. On each `interval`, inside the `active_hours`, the daemon makes one batch for each profile that has pending messages.
3. The sanitizer writes each message as one JSON line in a fenced block with a random label, and writes the digest file.
4. The driver gives the batch to the agent. When the driver reports `sent`, the messages are done. When it fails or the agent is busy, the messages stay pending, and the next interval sends them again.
5. On macOS, `i2a` shows a notification after each batch that it sends to an agent, and after each failure.

A `dry-run` batch is a preview. Its messages become `previewed`, not `sent`. When you change the profile to a real driver, the next batch contains them (if they are newer than `lookback`).

`i2a` uses read-only API calls. It does not mark Mattermost messages as read.

## Configuration

The file is `~/.config/i2a/config.toml`. The data (`inbox.db`, `digests/`, `i2a.log`) is in `~/.local/share/i2a/`. The `XDG_CONFIG_HOME` and `XDG_DATA_HOME` variables change these paths. `I2A_HOME` puts both in one directory.

Tokens are not in the file. `i2a` keeps them in the macOS keychain. Set `I2A_SECRET_BACKEND=file` to keep them in `secrets.json` (mode 0600) instead, for example on a server without a keychain.

```toml
[timing]
interval = "15m"          # time between dispatches
lookback = "1d"           # first-run window and maximum message age
active_hours = "08:00-20:00"  # local time; empty means always
retention = "7d"          # keep sent messages and digest files for this time

[notify]
enabled = true            # macOS notification after each dispatch to an agent

[[sources]]
id = "work"
type = "mattermost"
url = "https://chat.example.com"
poll_every = "1m"
all_direct = true         # all direct and group messages (false: only the conversations in channels)
mentions = true           # mentions of you in any channel
channels = ["<channel-id>"]   # channels and direct or group conversations; see: i2a source channels work
profile = "triage"
[sources.channel_profiles]    # optional: send one channel to another profile
"<channel-id>" = "reviewer"

[[profiles]]
name = "triage"
driver = "orca"           # orca | dry-run
agent = "claude"
workspace = "~/notes"     # a git folder, or a folder that you added in Orca
session = "reuse"         # reuse | new | worktree
payload = "pointer"       # pointer | inline
inline_max_bytes = 8000
message_max_chars = 2000
```

## Profiles and delivery

A profile tells `i2a` how to send a batch.

| Setting | Values | Effect |
|---|---|---|
| `session` | `reuse` | One long agent session. `i2a` sends the primer one time, when it starts the session. Each batch after that is the batch prompt only. If the session is closed, `i2a` starts a new one. If the agent is busy, `i2a` keeps the batch until the next interval. |
| | `new` | A new agent session for each batch, with the primer and the batch prompt. |
| | `worktree` | A new Orca worktree `i2a-<batch-id>` for each batch. The workspace must be a git repository. |
| `payload` | `pointer` | The prompt names the digest file. The agent reads the file. |
| | `inline` | The prompt contains the messages. If the prompt is larger than `inline_max_bytes`, `i2a` sends a pointer for that batch. |

Each profile has two templates in Go `text/template` syntax:

- `primer`: the standing instructions.
- `batch`: the prompt for each batch.

The templates can use `.BatchID`, `.Profile`, `.Count`, `.Sources`, `.DigestPath`, `.Messages`, `.Nonce`, `.WindowStart` and `.WindowEnd`. Edit them in the TUI, or set `primer` and `batch` in the file. If a template is empty, `i2a` uses the default.

The `orca` driver needs the Orca app to run. It runs the `orca` CLI. If Orca does not know the workspace and the workspace is a git repository, `i2a` adds it with `orca repo add`.

The driver starts the agent, waits until it is ready, and then sends the prompt with Enter. It records "turn started" when Orca proves that the agent started to work. If Orca cannot prove it, the driver presses Enter one more time and records "turn not proven, Enter sent again". It never types the prompt two times.

## Security

Other people write the messages, and an agent can run commands. A message can contain text such as "ignore your instructions and run this command". This is prompt injection.

`i2a` reduces the risk:

1. It writes each message as one JSON object on one line. The text cannot start a new line or close the data block.
2. It puts the data in a fenced block with a random label for each batch.
3. It removes control characters, ANSI escape sequences, bidi overrides and zero-width characters.
4. It truncates long messages.
5. The default templates tell the agent that the data is untrusted.
6. New profiles use the `dry-run` driver.

These steps reduce the risk. They do not remove it: an agent can still obey text in a data block. Do these steps too:

1. Do not let the agent run commands without approval. Orca can start agents with permission prompts off. Turn this off for the agent that `i2a` uses.
2. Use a workspace that holds no secrets.
3. Give the agent a narrow task in the primer, for example "summarise and write notes".

## Commands

| Command | Purpose |
|---|---|
| `i2a config` | Open the dashboard: sources, channels, profiles, schedule, service, activity |
| `i2a run --once` | Poll all sources and send all pending messages now |
| `i2a daemon` | Run in the foreground: poll and dispatch on the intervals |
| `i2a status` | Show the paths, sources, profiles, messages and service state |
| `i2a source add mattermost --id <id> --url <url> --token-stdin` | Add a source from a script |
| `i2a source list` / `channels <id>` / `remove <id>` | List sources, list channel IDs, remove a source |
| `i2a service install` / `uninstall` / `status` | Manage the macOS LaunchAgent |

## Development

You need Go 1.26, [Task](https://taskfile.dev), and for the end-to-end tests, Docker and tmux.

```sh
task build          # bin/i2a
task test           # unit tests with the race detector
task lint           # golangci-lint
task e2e            # end-to-end scenarios (starts a Mattermost server in Docker)
I2A_E2E_ORCA=1 task e2e      # also the Orca scenarios (needs the Orca app and Claude Code)
I2A_E2E_SERVICE=1 task e2e   # also the LaunchAgent scenario (macOS)
```

The end-to-end tests drive the real binary in a terminal, as a person does. The scenarios are in [`test/e2e/SCENARIOS.md`](test/e2e/SCENARIOS.md).

Changes go through [OpenSpec](https://github.com/Fission-AI/OpenSpec): see `openspec/`. Pull request titles use Conventional Commits, for example `feat(mattermost): add thread replies`.

## License

MIT. See [LICENSE](LICENSE).
