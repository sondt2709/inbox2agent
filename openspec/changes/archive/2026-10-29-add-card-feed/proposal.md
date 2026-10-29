## Why

The agent writes good results (a summary, evidence and a suggested reply for each topic), but the results exist only as terminal text in Orca. The text scrolls away, and the user cannot mark a topic read or done. The only notification says "i2a: N messages sent", which tells the user nothing to do. i2a must collect the results of the agent and become the place where the user reads and closes them.

## What Changes

- New command `i2a card add`: the agent sends cards as a JSON array on stdin. A card has a category (`action`, `question`, `info`), an urgency (`high`, `normal`, `low`), a group, a title, a body, a suggested reply, references to message IDs and an optional key. A card with a known key updates the existing card.
- New commands `i2a card done --key|--id` and `i2a card list [--json] [--all]`.
- New `cards` table in `inbox.db`. Schema version 3. The upgrade writes a backup `inbox.db.v2.bak` first.
- New Feed tab in the dashboard: a list of cards with sort (urgency or newest), a category filter and the keys `c` (copy the reply), `o` (open the thread of the source message), `d` (done) and `u` (unread). The tab reads the store again every 2 seconds.
- **BREAKING** The Feed is tab 1. The other tabs move one number up (Overview is 2, Activity is 6). `i2a` with no arguments opens the Feed instead of the help text. `i2a config` opens the Overview tab.
- New notification rule: `i2a card add` shows notifications for new and raised cards by urgency. New setting `[notify] min_urgency` (default `normal`).
- **BREAKING** The daemon no longer shows a notification for a sent batch. It still shows one for a failed dispatch.
- Each JSON line in a batch gets the field `id` (the message row ID), so a card can refer to its messages.
- i2a adds a fixed section to each primer that tells the agent how to run `i2a card add`, with the absolute path of the `i2a` executable.
- **BREAKING** The `reuse` session mode stores a hash of the primer. When the hash changes, the driver starts a new session one time.

## Capabilities

### New Capabilities
- `card-feed`: the card commands, the card data, the card notification rule and the Feed tab.

### Modified Capabilities
- `message-store`: the store keeps cards; schema version 3 with a backup; retention deletes old done cards.
- `batch-rendering`: the `id` field in each JSON line; the card section in the primer.
- `agent-delivery`: the `reuse` mode starts a new session when the primer hash changes.
- `configuration`: the dashboard has 6 tabs with Feed first; `i2a` with no arguments opens the Feed.
- `daemon-service`: the desktop notification rule (no notification for a sent batch; `min_urgency`).

## Impact

- Code: `internal/inbox`, `internal/store`, new `internal/card`, `internal/cli` (new `card.go`, root command), `internal/render`, `internal/agent/orca.go`, `internal/daemon/engine.go`, `internal/config`, `internal/notify`, `internal/tui` (new `tab_feed.go`).
- Data: `inbox.db` schema 2 to 3. An older `i2a` refuses a version 3 store.
- Agents: the primer and the data lines change. The agent needs permission to run `i2a card` (for Claude Code, an allow rule in the workspace `.claude/settings.json`).
- Tests: unit tests in each package; new e2e scenarios; the e2e scenarios that use tab numbers change.
- No new dependency, service, network port or secret.
