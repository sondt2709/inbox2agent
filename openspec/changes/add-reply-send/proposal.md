## Why

A card often has a good suggested reply, but i2a cannot send it. The user copies the reply with `c`, opens Mattermost with `o`, pastes, sends, goes back to i2a and presses `d`: five steps in two applications for each reply. The user wants to send the suggested reply, or an edited reply, from the Feed. The agent must also be able to send a reply when the profile primer allows it. Mattermost is the first platform; WhatsApp, GitHub and Linear come later, so the send path must not depend on one platform.

## What Changes

- New key `r` in the Feed: a reply form in the detail panel with the fields Send to, Reply to, Where and Text. `ctrl+s` sends, `esc` cancels. After a successful send, the card becomes `done` and shows "Replied <time> by you".
- New command `i2a reply --to <message id> | --card <id|key>`. It reads the text on stdin, or uses the suggested reply of the card. Flags `--thread` and `--channel`. Each send with this command shows a desktop notification.
- The target of a reply is always a message in the store. i2a does not take a channel, a user or a URL from card text or from the command line.
- A failed send keeps the text. A retry does not post the same reply two times.
- New `replies` table in `inbox.db` and a new `thread_id` column for messages. Schema version 4. The upgrade writes a backup `inbox.db.v3.bak` first.
- **BREAKING** i2a writes to Mattermost as the user. Polling stays read only. i2a sends no request that changes the read state: Mattermost keeps it for a reply in a thread and marks the channel read for a top-level post, as for any post of the user. The README sentence "`i2a` uses read-only API calls" changes.
- A source can send replies when it implements the new optional `Replier` interface. Mattermost implements it. For a source without it, `r` and `i2a reply` say that the source cannot send replies yet.
- The automatic card section of the primer does not mention `i2a reply`. The user allows the agent to send in the profile primer.

## Capabilities

### New Capabilities
- `reply-send`: the reply form in the Feed, the `i2a reply` command, the reply target and place, the text rules, the safe retry, the read state of a reply and the reply notification.

### Modified Capabilities
- `mattermost-source`: the source sends replies with `POST /posts`; the read-state rule names the requests of a send; each collected message keeps the ID of its thread root.
- `message-store`: the store keeps replies; schema version 4 with a backup; retention deletes old replies.
- `card-feed`: the Feed shows the key `r` and the "Replied" line of a card.

## Impact

- Code: `internal/inbox`, `internal/source` (new `Replier`), `internal/source/mattermost` (new requests and `Reply`), `internal/store` (migration, replies), new `internal/reply`, `internal/cli` (new `reply.go`), `internal/tui/tab_feed.go`.
- Data: `inbox.db` schema 3 to 4. An older `i2a` refuses a version 4 store.
- External systems: i2a creates posts on the Mattermost server as the user. The existing personal access token is enough; no new permission.
- Agents: to let the agent send, the user adds an instruction to the profile primer and an allow rule for `i2a reply` in the workspace `.claude/settings.json`.
- Tests: unit tests, new Mattermost fixtures from `capture.sh`, new e2e scenarios.
- No new dependency, service, network port, secret or configuration key.
