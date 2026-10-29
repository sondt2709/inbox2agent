## Why

A card is often a first answer, not the final one. The user wants the agent to do more about a card: "research more", "check the auth code first", "add the effort in hours". Today the Feed can reply only to the platform (`r`). To instruct the agent, the user must find the Orca terminal of the agent and type there, and i2a does not know that the user asked. The user wants to send the instruction from the card, and to get the answer back in the same card.

## What Changes

- New key `t` in the Feed: an "Ask the agent" form in the detail panel with the fields Agent (read only) and Text. `ctrl+s` sends, `esc` cancels.
- New command `i2a ask --card <id|key>`. It reads the text on stdin.
- `i2a card add` records the Orca terminal of the agent that runs it (`ORCA_TERMINAL_HANDLE`) with each card.
- An ask goes to the terminal that wrote the card when it is live, else to the reuse session of the profile, else to a new session with the primer.
- The ask prompt has the text of the user as the instruction, the card and its messages in a fenced data block, and the command to answer: `i2a card add` with the key of the card. The agent updates the card, so the card becomes `new` and i2a notifies, as today.
- When the agent is busy, the ask stays `queued`. The Feed waits up to 2 minutes; then the daemon sends the ask on its next dispatch, before the batches.
- One lock file for each profile keeps a batch and an ask from typing into the same terminal at the same time.
- The list row of a card with an ask shows "↪ asked". The detail panel shows each ask with its time, text and state.
- A `dry-run` profile refuses an ask.
- New `asks` table and a new `handle` column for cards. Schema version 5. The upgrade writes a backup `inbox.db.v4.bak` first.
- The primer does not change, so no running session restarts.

## Capabilities

### New Capabilities
- `agent-ask`: the ask form in the Feed, the `i2a ask` command, the target terminal, the ask prompt, the ask states, the queue for a busy agent and the dry-run refusal.

### Modified Capabilities
- `card-feed`: `i2a card add` records the terminal of the agent; the Feed shows the key `t` and the asks of a card.
- `agent-delivery`: one delivery at a time for each profile (the profile lock); the daemon sends queued asks before the batches.
- `message-store`: the store keeps asks and the terminal of a card; schema version 5 with a backup; retention deletes old asks.

## Impact

- Code: `internal/inbox`, `internal/store` (migration, asks, card handle), `internal/card`, `internal/render` (ask prompt), `internal/agent` (`Ask`, profile lock), new `internal/ask`, `internal/daemon` (queued asks), `internal/cli` (new `ask.go`, `card.go`), `internal/tui/tab_feed.go`.
- Data: `inbox.db` schema 4 to 5. An older `i2a` refuses a version 5 store.
- External systems: i2a reads `ORCA_TERMINAL_HANDLE` (checked on orca 1.4.240). The TUI and `i2a ask` now run `orca` and can launch an agent session.
- New lock files `<data>/agent-<profile>.lock`.
- Tests: unit tests with the real Orca fixtures, new e2e scenarios.
- No new dependency, service, network port, secret or configuration key.
