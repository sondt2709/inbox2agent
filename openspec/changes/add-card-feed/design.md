## Context

Today the data flow ends at the agent: `source.Collect` → `store.SavePoll` → `daemon.Engine.Dispatch` → `render.Render` → `agent.Driver.Deliver` → `store.FinishDispatch`. The agent writes its results as terminal text in Orca. i2a never sees them. The daemon shows a notification "i2a: N messages sent" after each real dispatch (`internal/daemon/engine.go`, `notify`).

This change adds a return path: the agent runs `i2a card add`, and i2a stores, shows and notifies the results. The demo video of the approved workflow is outside the repo. The user approved the gate decisions on 2026-10-29.

Constraints:
- One static binary, no new dependency, no network listener.
- The store is SQLite in WAL mode with `busy_timeout(5000)` and one connection for each process. The daemon, the dashboard and `i2a card add` are separate processes that open the same file.
- Card text comes from an agent that reads untrusted messages. Card text is untrusted too.

## Goals / Non-Goals

**Goals:**
- An agent-friendly command that saves cards in one call, with one notification for each call.
- A Feed tab where the user reads, copies the reply, opens the source thread and closes each card.
- Notifications for cards by urgency, and no notification for a sent batch.
- A safe upgrade of existing stores and of running `reuse` sessions.

**Non-Goals:**
- Cards without an agent (for example one card for each message of a `dry-run` batch).
- Closing a card when the user replies in Mattermost (the source drops the messages of the user).
- An MCP server or an HTTP endpoint.
- A click action on a notification (needs `terminal-notifier`).
- Sending replies to Mattermost. i2a stays read-only for sources.

## Decisions

### D1. The agent calls a CLI command that writes the store directly

`i2a card add` opens `inbox.db` and writes in one transaction. It does not talk to the daemon.

Alternatives: an HTTP endpoint in the daemon (a new port, authentication, and the daemon must run); an MCP server (each agent needs its own setup); a file that the daemon reads (no validation at write time). The CLI works with each agent that can run a shell command. SQLite already handles more than one writer process.

### D2. JSON on stdin, all-or-nothing

The input is a JSON array of card objects, or one object. The decoder rejects unknown fields, so a misspelled field fails and the agent sees the error. The command validates all cards first. When one card is not valid, it saves nothing and exits with code 1. Text that is too long is cut, with a warning on stderr; the card is saved.

| Field | Rule |
|---|---|
| `category` | `action`, `question` or `info`; required |
| `urgency` | `high`, `normal` or `low`; default `normal` |
| `title` | required; 200 characters; newlines become spaces |
| `group` | 100 characters |
| `body` | 4,000 characters |
| `reply` | 2,000 characters |
| `refs` | message row IDs; 50 at most; IDs that are not in the store are dropped with a warning |
| `key` | 200 characters; optional |

Limits for each call: 50 cards, 1 MiB of input. All text goes through `render.Sanitize` (the same hidden-character rule as message text).

Flags were rejected: agents quote multi-line text in flags badly.

### D3. Upsert by key

`key` is unique when it is set (a partial unique index). A card with a known key updates the stored card: all fields change, `updated_at` changes, and the state becomes `new`. A card without a key is always a new card.

The command reports for each card: `added`, `updated`, or `unchanged` (an update with the same content). An `unchanged` card keeps its state. This stops an agent that posts the same card again from marking it unread.

### D4. Notification rule in `internal/card`

A pure function plans the notifications for one call. It gets the saved cards and `min_urgency`.

1. A card counts when it was `added`, or `updated` with a higher urgency, or `updated` from the `done` state.
2. Remove the cards with an urgency lower than `min_urgency`.
3. Each `high` card gets one notification, up to 3 for each call.
4. All other cards (and the `high` cards after the third) go into one grouped notification.
5. When the group has one card, that card gets its own notification instead.

The title of a single notification is `i2a · <CATEGORY> · <urgency>` (with ` (updated)` for an update). The text is `<group>: <title>`. A grouped notification has the title `i2a: <n> new cards` and the text with the counts by category, for example `1 action, 2 info`. The text is cut to 200 characters. `notify.enabled = false` turns all of them off.

The command shows the notifications itself, so they arrive at once and do not wait for the daemon loop. The limit of 3 bounds the effect of an agent that a message tricks into posting many urgent cards.

### D5. Store schema version 3

```
cards(id INTEGER PRIMARY KEY, key TEXT, category, urgency, grp, title, body, reply,
      refs TEXT  -- JSON array of message IDs
      state TEXT -- new | read | done
      created_at, updated_at, done_at INTEGER)
UNIQUE INDEX cards_key ON cards(key) WHERE key IS NOT NULL
sessions.primer_hash TEXT NOT NULL DEFAULT ''
```

`migrate` makes the backup with `VACUUM INTO '<db>.v2.bak'` before it changes a version 2 file, then adds the column and sets the version to 3 in one transaction. `VACUUM INTO` gives a consistent copy in WAL mode. A new store gets version 3 directly and no backup.

`Prune` also deletes `done` cards whose `done_at` is older than `retention`. It never deletes `new` or `read` cards. Messages that a card refers to can be deleted by retention; the Feed then shows "message deleted" for that reference.

### D6. The card section of the primer

`render.Render` gets the card command (for example `/Users/x/.local/bin/i2a`, or `I2A_HOME=/x /Users/x/.local/bin/i2a` when `I2A_HOME` is set). It appends a fixed section to the rendered primer: the command, the JSON shape, the meaning of each field, and the rules "use the `id` values in `refs`" and "use the same key for the same topic". The engine resolves the command one time with `os.Executable`. The section is not a template field, so a custom primer cannot lose it.

`Rendered.PrimerHash` is the first 16 hexadecimal characters of the SHA-256 of the primer template text and the card section. It does not use the rendered primer, because the rendered primer can contain the batch ID.

The digest file does not change, apart from the `id` field in each line.

### D7. Restart of a `reuse` session when the primer changes

`agent.Sessions` returns and stores the primer hash with the handle. In `reuse` mode, when the stored hash is not `Rendered.PrimerHash`, the driver acts as if no live session exists: it launches a new agent, submits the primer and the batch, and stores the new handle and hash. The old terminal stays open; the user can close it. Stores from version 2 have an empty hash, so the first dispatch after the upgrade starts a new session one time.

Alternative: send the new section as a normal prompt into the running session. Rejected: the agent can be busy, and a mid-session instruction is less reliable than a primer.

### D8. The Feed tab

- `internal/tui/tab_feed.go` follows the pattern of the other tabs: a list at the left and a detail panel at the right. The Feed is `tabFeed`, the first tab. `i2a` with no arguments runs the dashboard on the Feed; `i2a config` runs it on Overview.
- The tab reads the cards in the existing refresh of the dashboard (`load`, a `tea.Tick`), now every 2 seconds instead of 3. A key that changes a card writes the store and reads the cards again outside the UI loop. When no store file exists, the tab shows "No cards yet" and tries to open the store on each tick.
- The bare `i2a` command opens (and creates) the store, because the Feed is its main screen. `i2a config` keeps its rule: it reads the store only when it exists.
- Sort: `urgency` (high first, then the newest `updated_at`) or `newest`. Filter: all, action, question, info. The header shows the counts of new cards, actions and questions.
- A card becomes `read` when the user moves the selection to it, presses `enter`, `c` or `o`. The card that is selected when the tab opens stays `new` until one of these actions.
- `c`: `pbcopy` on macOS, OSC 52 (`tea.SetClipboard`) on other systems. `o`: opens the URL of the first reference message that still exists, with `open` on macOS and `xdg-open` on other systems, only when the URL starts with `http://` or `https://`. i2a never opens a URL from the card text.
- `d` marks the card `done` (and it leaves the list); `u` marks it `new`. `a` shows the done cards too.
- `Deps` gets `Copy func(string) error` and `Open func(string) error`, so tests and e2e can replace them.

### D9. No notification for a sent batch

`Engine.notify` keeps the failure notification and drops the `sent` notification. The `enabled` comment in `config.toml` changes to "Show macOS notifications for new cards and failed dispatches."

## Risks / Trade-offs

- [A message tricks the agent into fake urgent cards] → at most 3 urgent notifications for each call; card text is sanitized; `o` opens only stored message URLs; the primer says that message text is untrusted.
- [Notification text can quote message content on the lock screen] → the user approved card titles in notifications; macOS settings control the preview; `min_urgency = "high"` or `enabled = false` reduces it.
- [An older daemon binary runs while a newer `i2a` migrates the store] → the old process keeps its connection and its SQL still works with version 3 (the new column has a default). The next start of the old binary fails with "Update i2a". The upgrade note says to run `i2a service install` after an update, which restarts the service with the new binary.
- [The agent does not call `i2a card add`] → the terminal output still exists as before. The primer section is fixed text; the e2e scenario with a real agent proves the call one time.
- [Claude Code asks permission for each call] → the README tells the user to add an allow rule for the `i2a card` command in the workspace `.claude/settings.json`.
- [Tab numbers change] → the README, the help dialog and the e2e scenarios change together.
- [The agent sees the wrong store when `XDG_DATA_HOME` differs between the service and the agent shell] → the command carries `I2A_HOME` when it is set; the README names the case for XDG variables.

## Migration Plan

1. Install the new binary and run `i2a service install` (it restarts the service).
2. The first open of `inbox.db` writes `inbox.db.v2.bak` and upgrades the schema to version 3.
3. The first dispatch of each `reuse` profile starts a new agent session with the new primer section.
4. Add the allow rule for `i2a card` in the agent workspace.

Rollback: install the earlier binary, stop the service, replace `inbox.db` with `inbox.db.v2.bak` (cards are lost; messages after the backup are collected again within `lookback`), and run `i2a service install`.

## Open Questions

None. The gate decisions of 2026-10-29 answered the open items.
