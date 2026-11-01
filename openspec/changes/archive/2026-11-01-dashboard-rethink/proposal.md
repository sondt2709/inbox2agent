## Why

The dashboard has 8 tabs that copy the data model: Feed, Activity, Accounts, Conversations, Spaces, Agents, Schedules and Settings. The user does two jobs in it: read the cards, and decide where each conversation goes. A test on a real configuration showed these problems:

- `m` (move) and `L` (add to a space) both set the spaces of a conversation, and the `SPACE` column reads as one space. A conversation in many spaces looks like an exception.
- The tab bar shows "hundreds of unsorted conversations" as a warning. These conversations are not lost: Unsorted reads them and sends them to its agent. The user cannot see which conversations are really new.
- Setup items that the user sets one time (accounts, agents, schedules, settings) have the same weight as the Feed.
- On 100 to 120 columns the footer drops `R` and `L`, and "run now" has no fixed place.
- The conversation list has hundreds of rows sorted by name, so busy and dead channels look the same.

## What Changes

- **BREAKING (dashboard keys):** three areas in place of eight tabs: `1 Feed`, `2 Spaces`, `3 Setup`. Setup has the sections Accounts, Agents, Schedules, Settings and Send log (the old Activity tab). The Conversations tab goes into the Spaces area. The keys `m`, `L` and `l` are removed.
- The header always shows the service, the next send and `R run now`. The result of the last run stays on the line under the header.
- The Feed gets a list of spaces with their card counts at the left. `←`/`→` change the space; `F` stays.
- The Spaces area shows each space with its agent, schedule, last send, conversations and recent sends. Unsorted is shown as "Everything else". Two more items show "New conversations" and "Ignored".
- One dialog sets the spaces of a conversation: a checklist of the spaces, a read level for each, and "Ignore". `/` opens it from any screen through a search of all conversations.
- `a` in a space opens a search of all conversations of all accounts, newest activity first, to add or remove them.
- A conversation is "new" when the account lists it, the file has no entry for it, and the dashboard did not show it before. The dashboard keeps the shown conversations in `<data>/conversations-seen.json`, not in `config.toml`.
- The read levels show as "All messages", "Only for me" and "Ignored". The file keeps `all`, `mentions` and `off`.
- Changes in the Spaces area save at once. `u` undoes the last saved change when the file did not change after it.
- When the user adds a space to a conversation, the dialog can also send the messages of the last `lookback` again, to the new spaces only (off by default). The store gets `Requeue`.

No change to the format of `config.toml`, the store schema, the daemon or the CLI commands.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `configuration`: the dashboard areas, the Spaces area, the spaces dialog, the conversation search, new conversations, undo, and the key hints.
- `card-feed`: the space list of the Feed, the Send log, and `R` in the header.
- `message-store`: `Requeue` of the messages of a conversation.

## Impact

- `internal/tui`: most files. `tab_conversations.go` is replaced by the Spaces area and two dialogs.
- `internal/config`: `SetSpaces`, one function that sets all links of a conversation.
- `internal/store`: `Requeue`.
- `test/e2e`: the scenarios that drive the tabs 4 and 5, and the keys `m`, `L` and `l`.
- `README.md` and `CLAUDE.md`: the key tables and the tab names.
