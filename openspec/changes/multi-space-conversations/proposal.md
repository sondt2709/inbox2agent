## Why

One conversation often matters to two groups of work. Example: the channel `release` of the account `work` is in the space Review (read level `all`, agent `reviewer`, which checks each release note), and the person also wants the agent `triage` of the space Team to see only the release posts that mention them. Now a conversation is in exactly one space, so the user must choose one agent and lose the other view. The validation rejects the second entry: "conversation … is in two spaces".

## What Changes

- A conversation can be in many spaces. Each entry in `[[spaces.conversations]]` is a link with its own read level and label. A conversation that no space lists is in Unsorted, as before. A conversation cannot be two times in one space, and it cannot be in Unsorted and in a space.
- Fan-out: each space that lists a conversation gets its own copy of each message that its read level allows, in its own batch, for its own agent, at its own schedule.
- The source reads each conversation at the widest level of its links (`all` > `mentions` > `off`) and stores a "for the user" mark with each message (a direct conversation, a mention, or a reply to the user). Each space filters the messages at send time with the level of its link.
- The store records a delivery for each message and space. A message is `sent` when each space that wants it has it.
- A card gets its space from the terminal of the agent that adds it (`ORCA_TERMINAL_HANDLE` → the session of a space). A card from a terminal that i2a does not know keeps the old rule (the space of its messages). `ask` sends to that space.
- The dashboard shows all spaces of a conversation, and the key `L` links a conversation to one more space. `m` still moves a conversation to one space.
- `i2a conversations` shows all spaces; `--json` adds `spaces` (each with `space`, `read`, `label`).
- The configuration stays version 2.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `spaces`: a conversation in many spaces; the read level of each link.
- `agent-delivery`: one batch for each space that lists the conversation.
- `message-store`: deliveries for each message and space; the "for the user" mark; schema version 7 with a backup.
- `card-feed`: the space of a card from its terminal.

## Impact

- `internal/config`: `Links`, `Routes`, the widest level in `Level` and `Levels`, `AddLink`, `SetLevel`; `Validate`; the schema text.
- `internal/inbox`: `Message.ForMe`, `Message.Delivered`, `Card.Space`.
- `internal/source/mattermost`, `internal/source/whatsapp`: set `ForMe`.
- `internal/store`: schema version 7 (`messages.for_me`, `deliveries`, `cards.space`), backup `inbox.db.v6.bak`.
- `internal/daemon/engine.go`: fan-out in `Dispatch`; the state of each message after the round.
- `internal/ask`: the space of the card first.
- `internal/tui`: the Conversations tab (spaces column, key `L`), the Spaces tab, `cardSpace`.
- `internal/cli`: `status` and `conversations`.
- An older i2a cannot open a version 7 store. Rollback: `inbox.db.v6.bak`.
- More batches when a conversation is in two spaces with agents: more agent cost, only for those conversations.
