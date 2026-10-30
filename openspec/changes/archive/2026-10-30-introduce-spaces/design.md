## Context

Today one `[[sources]]` entry holds the login, the selection (`all_direct`, `mentions`, `channels`) and the route (`profile`, `channel_profiles`). `agent.Route` (internal/agent/agent.go:60) picks a profile at dispatch time: the channel entry, else the source profile, else the first profile. `daemon.Run` (internal/daemon/loop.go) dispatches all profiles on one global `timing.interval` inside one global `active_hours`. The store keeps the profile name in `messages.profile`, `dispatches.profile`, `sessions.profile` and `asks.profile`. The approved concept and screens are in the playground `tmp/evidence/clarify-sources-profiles-schedule/proposal/i2a-playground.html` (not committed).

The Mattermost source already reads each member channel to find mentions (internal/source/mattermost/source.go:265). The WhatsApp source reads all rows of its local `inbox` table (`wanted`, internal/source/whatsapp/source.go:123). So a read level for each conversation fits both sources with no new API call.

Constraints: one static binary (`CGO_ENABLED=0`); no change to the reply path; the render boundary stays the same; fixtures stay real captures.

## Goals / Non-Goals

**Goals:**
- Five separate items: account, conversation, space, agent, schedule. Only a space links them.
- The same settings for a DM, a group and a channel.
- One account can feed many spaces; one space can hold conversations of many accounts.
- A schedule for each space; reusable agents and schedules.
- Automatic, reported migration of version 1 files and stores, with backups.
- Docs (README, CLAUDE.md, e2e scenarios, demo) that use the new words only.

**Non-Goals:**
- A conversation in two spaces (fan-out). One conversation has one space.
- Rules by pattern (for example "all channels named proj-*").
- Markdown in the Feed. That is the separate change `render-card-markdown`.
- GitHub and Linear accounts.
- Renaming internal Go packages: `internal/source` stays the platform adapter that reads one account; the store column `messages.source_id` keeps the account ID.

## Decisions

### D1. Configuration version 2

```toml
version = 2

[settings]
lookback = "1d"
retention = "7d"

[notify]
enabled = true
min_urgency = "normal"

[[accounts]]
id = "phone"                 # stable: secrets, store, whatsapp-<id>.db
name = "Personal phone"      # free text, can change
platform = "whatsapp"        # mattermost | whatsapp
read_every = "10s"           # mattermost also has url = "https://..." and user = "<username>" (cache)

[[agents]]
id = "claude-work"
name = "Claude for work"
driver = "orca"              # dry-run | orca
cli = "claude"               # was profiles.agent
workspace = "~/work-notes"
session = "reuse"            # reuse | new | worktree
payload = "pointer"          # also inline_max_bytes, message_max_chars, model, orca_bin, primer, batch

[[schedules]]
id = "work-hours"
name = "Work hours"
every = "15m"
days = ["mon", "tue", "wed", "thu", "fri"]   # empty: every day
hours = "08:00-18:00"                        # empty: all day; may cross midnight

[unsorted]
agent = ""                   # empty: hold, do not send
schedule = "always"
read = "mentions"            # read level of a conversation that no space lists

[[unsorted.conversations]]   # explicit levels inside Unsorted, for example off
account = "phone"
id = "<chat id>"
name = "Football Sunday"
read = "off"

[[spaces]]
id = "acme"                  # stable: store, sessions, locks
name = "Acme"                # free text, can change
agent = "claude-work"
schedule = "work-hours"

[[spaces.conversations]]
account = "acme"
id = "<channel id>"
name = "#release"            # platform name, refreshed when the account lists its conversations
label = "Release channel"    # optional rename by the user; shown instead of name
read = "all"                 # all | mentions | off
```

Why IDs plus names: each item (account, agent, schedule, space) has a stable `id` and a free `name` that the user can change; a conversation has the platform `name` and an optional user `label`. the store, the sessions and the lock files need a stable key; the user needs to rename freely. Alternative "name only" breaks sessions on a rename.

Why `name` in a conversation entry: `i2a status` and the TUI must show a label without a network call. The label refreshes when the TUI lists the conversations of the account.

### D1b. Account information

The Accounts tab and `i2a status` show what identifies an account: for Mattermost the server URL and the username (`user`, cached in the account entry when the TUI or `i2a account add` checks the token, and refreshed when the TUI lists the conversations); for WhatsApp the phone number and the push name, read from `jid` and `push_name` of `whatsmeow_device` in `whatsapp-<id>.db` (local, no network, not copied into the configuration).

### D2. Read levels and "mentions"

`all`: each message. `mentions`: messages for the user: a mention, a reply to a message of the user (WhatsApp quote), or each message of a direct conversation. `off`: nothing. A source gets `Levels map[conversationID]level` and `Default level` (`unsorted.read`) in its settings, instead of `Channels`, `AllDirect` and `Mentions`.

Why "each message of a direct conversation" counts as "for you": a DM rarely contains `@name`, so a literal mention rule would make `mentions` mean `off` for a DM. With this rule the user sees one choice with one meaning for all kinds. Alternative: a separate default for DMs. Rejected: it brings back the DM and channel split.

### D3. Route at dispatch time

`config.Route(accountID, conversationID) spaceID` returns the space that lists the conversation, or `unsorted`. The engine routes `pending` messages when a space is due, as `agent.Route` does today. A move of a conversation therefore takes its waiting messages to the new space. `sent` messages keep the space of their dispatch (`messages.space`).

### D4. Schedule times

`Schedule.Next(after time.Time) time.Time` returns the next send time: the start of the hours (or 00:00) plus a multiple of `every`, inside the hours, on an allowed day, in local time. The daemon keeps `next[space]`. On each tick, a space with an agent and `next <= now` dispatches, then gets `Next(now)`. On start and on reload, `next = Next(now)`.

Why aligned times: the TUI and `i2a status` can show "next send 14:45", and the same function serves the daemon and the screens. Alternative "every N after the last send" drifts and cannot show the time before the first send.

`i2a run --once` sends each space with an agent now, as today it ignores `interval` and `active_hours`.

### D5. One dispatch, one space

`Engine.Dispatch(ctx, spaces)` makes one batch for each given space that has pending messages. The dispatch row records the space and the agent. A dry-run agent previews (as today). A space with no agent (Unsorted on hold) never dispatches; its messages expire after `lookback`.

### D6. Sessions and locks for each space

`sessions` has one row for each space. Two spaces with the same agent get two terminals, so the data of Acme never enters the session of Borealis. The lock file becomes `<data>/space-<id>.lock`, because a lock protects one terminal and a terminal now belongs to one space. The reuse primer hash stays per session; the primer can use `{{.Space}}`.

### D7. Asks

`ask.Resolve` takes the space of the newest ref message that has one, else `config.Route` of the newest ref message. The agent is the agent of that space. A space with no agent or with a dry-run agent refuses the ask with "The space <name> has no agent that reads asks." The daemon sends queued asks on each loop tick with a minimum gap of 1 minute, not only in a dispatch round, because a space can have a long schedule.

### D8. Store schema version 6

Write `<data>/inbox.db.v5.bak` (VACUUM INTO, as for version 5). In one transaction: `ALTER TABLE ... RENAME COLUMN profile TO space` for `messages`, `dispatches`, `sessions` and `asks`; `ALTER TABLE dispatches ADD COLUMN agent TEXT NOT NULL DEFAULT ''`; `UPDATE dispatches SET agent = space`; set the version to 6. SQLite 3.25 and later support RENAME COLUMN; `modernc.org/sqlite` v1.61.0 embeds 3.49.

Why the values stay valid: the configuration migration (D9) makes one space and one agent for each version 1 profile, with the profile name as the ID.

### D9. Configuration migration from version 1

`config.Load` detects version 1 (no `version`, or any of `[[sources]]`, `[[profiles]]`, `[timing]`). It converts in memory, writes `config.toml.v1.bak` (mode 0600), saves version 2 and returns a `Migration` report. The CLI prints the report to stderr one time; the daemon logs it.

| Version 1 | Version 2 |
|---|---|
| `sources[i]`: id, type, url, poll_every | `accounts[i]`: id, platform, url, read_every |
| `profiles[i]` | `agents[i]`: id = name, cli = agent, other keys the same |
| `timing.interval`, `timing.active_hours` | `schedules = [{id = "default", every, hours}]` (all days) |
| `timing.lookback`, `timing.retention` | `settings.lookback`, `settings.retention` |
| each profile that a source or a channel uses | `spaces[j]`: id = name = profile name, agent = profile name, schedule = `default` |
| `channels` of a source | conversations of the space of `channel_profiles[ch]`, else of `profile`, else of the first profile; read `all` |
| a `channel_profiles` key that is not in `channels` | the same space; read `mentions` |
| fallback for other conversations | `unsorted.agent` = the profile of the first source (else the first profile); `unsorted.read` = `mentions` when any source had `all_direct` or `mentions`, else `off` |
| `{{.Sources}}` and `{{.Profile}}` in a template | `{{.Accounts}}` and `{{.Agent}}` |

The report names each change in behavior: an account whose other conversations now go to a different agent, and DMs that now arrive because `mentions` includes DMs. Alternative "refuse version 1 and ask the user to edit by hand": rejected, the daemon runs as a LaunchAgent and would stop with no visible error.

### D10. TUI

Tabs: `1 Feed  2 Activity │ 3 Accounts → 4 Conversations → 5 Spaces → 6 Agents + 7 Schedules │ 8 Settings`, with the arrows drawn in the tab bar. A line under the tab bar defines the item of the tab. `i2a config` opens on Spaces. Each tab is one file (`tab_accounts.go`, `tab_conversations.go`, `tab_spaces.go`, `tab_agents.go`, `tab_schedules.go`, `tab_settings.go`); `tab_overview.go`, `tab_sources.go`, `tab_profiles.go` and `tab_schedule.go` go away. The form engine (`form.go`) and the dialogs stay.

The Conversations tab joins the configured conversations with the conversations that the account lists: `ListChannels` for Mattermost (network, in the background, as the picker does now) and `ReadChats` for WhatsApp (local file). A conversation that no space lists shows in Unsorted with the `new` mark. The tab has a search, filters (Unsorted, account, kind), multi-select (`space`), move (`m`) and read level (`l`). Spaces: the sentence, the agent and schedule choices, and the conversation table. Agents and Schedules list the spaces that use the item; delete asks for a replacement. Settings: lookback, retention, notifications, and the service keys `i` and `u` from the old Overview. Activity: the message counts from the old Overview, and the dispatch table with a Space column.

### D11. CLI

`i2a account add mattermost|whatsapp`, `i2a account list`, `i2a account remove <id>` (the old `source` subcommands with new names). `i2a conversations [--account <id>] [--unsorted]` prints account, ID, kind, name, space and read level. `i2a conversation set <account> <conversation-id> [--space <id>] [--read all|mentions|off]` writes the configuration. `i2a source` prints "renamed: use i2a account" and exits with code 2. `i2a status` prints accounts, spaces (agent, schedule, next send, waiting messages), the Unsorted count, agents and schedules.

### D12. Order with add-whatsapp-source

This change starts after `add-whatsapp-source` is archived and merged. Its delta specs then remove and replace the WhatsApp requirement "Collect messages from selected chats". The delta specs use REMOVED plus ADDED for requirements that `add-whatsapp-source` also changes, so the two changes do not need the same base text.

## Risks / Trade-offs

- [The migration routes some messages to a different agent than before] → The report lists each account whose unlisted conversations change agent; Unsorted keeps the old fallback agent; the backup allows a manual return.
- [Store migration fails half way] → One transaction; the backup `inbox.db.v5.bak` is written first; a failure leaves version 5.
- [`mentions` now includes DMs, so a user who had `all_direct = false` gets DMs] → The report says so; the user sets the DM to `off` in Unsorted.
- [Many WhatsApp chats (more than 100) to sort] → Unsorted keeps working on its own read level; multi-select move; filters by account and kind.
- [A Mattermost account lists channels over the network in the Conversations tab] → Background load with a status line, as the picker does now; configured conversations show at once from the `name` cache.
- [One session for each space opens more Orca terminals] → Expected and wanted for isolation; the Agents tab says so.
- [Scripts that call `i2a source`] → Clear error with the new command; README migration section.
- [Large TUI rewrite] → Keep `form.go`, `picker.go`, `dialog.go` and the model layout; rewrite tab files one at a time with tests.

## Migration Plan

1. Release as a major version (`feat!:`), because the configuration, the CLI and the templates change.
2. On first start, `config.Load` migrates the file and `store.Open` migrates the database; both keep backups.
3. Rollback: stop the service, install the previous version, copy `config.toml.v1.bak` to `config.toml` and `inbox.db.v5.bak` to `inbox.db`.

## Open Questions

- None. The stakeholder approved the concept and the playground on 2026-10-31.
