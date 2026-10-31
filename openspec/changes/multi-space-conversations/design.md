## Context

`config.Route` gives one space for each conversation, and `Validate` rejects a conversation in two spaces. The sources read each conversation at its level and drop the other messages. The store has one state and one space for each message (`messages.state`, `messages.space`). `cardSpace` in the dashboard and `ask.Resolve` find the space of a card from its messages.

## Goals / Non-Goals

**Goals:**
- One conversation in many spaces, each with its own read level, agent and schedule.
- Each space gets each message once; a failure in one space does not resend the message to another.
- Keep the configuration format (version 2) and the behavior of a conversation in one space.

**Non-Goals:**
- A link in Unsorted and in a space at the same time.
- Send old messages to a space that a conversation joins later (only the waiting messages go).
- A different label for each link on the screens (the first label shows).

## Decisions

1. **A link is an entry.** `[[spaces.conversations]]` already has `account`, `id`, `read` and `label`. Two spaces can list the same conversation now; `Validate` rejects only a conversation two times in one space, or in Unsorted and in a space. `Links(account, id)` returns the links in the order of the file, or the Unsorted link.
2. **Read at the widest level, filter at send time.** `Level` and `Levels` return the widest level of the links, so the source reads each message that one link wants. The source sets `Message.ForMe`: a direct conversation, a mention of the user, a quote (WhatsApp) or a mention (Mattermost). The engine gives a message to a link with `all`, or with `mentions` when `ForMe` is true.
3. **Deliveries in the store, and an aggregate state.** The table `deliveries(message_id, space, state, dispatch_id)` records the state for each space (`sent`, `previewed`). `messages.state` stays the aggregate that the screens count: `pending` while a space that wants it does not have it, `previewed` when each wanting space has it and one has only a preview, `sent` when each has it, `expired` as before. For each batch, the engine computes the aggregate of each of its messages with the links of now and the deliveries of the round, and `Store.FinishDispatchStates` writes the delivery and the aggregate in one transaction, so a stop between two spaces cannot make a message `sent` before each space has it. At the end of the round, `settle` sets the aggregate of the other waiting messages that a space has (`Store.SetStates`), for example when a link was removed. `messages.space` and `messages.dispatch_id` keep the last delivery, for the screens.
4. **A message that no link wants.** A message that no space has and that no link wants now (the level changed, or a `mentions` link and no mark) stays `pending` until `lookback` expires it, as a message of an Unsorted on hold does now. A message that a space has and that no other link wants is `sent`.
9. **Delete a space.** Its conversations go to Unsorted only when no other space lists them.
5. **The space of a card from its terminal.** `i2a card add` records the terminal handle of the card (since version 5). `SaveCards` looks up the session with that handle (`sessions.handle`) and stores its space in `cards.space`. An update by key keeps the space when the new terminal is unknown. `cardSpace` and `ask.Resolve` use `cards.space` first, then the old rule.
6. **Schema version 7 with a backup.** The upgrade writes `inbox.db.v6.bak`, then in one transaction adds `messages.for_me` (default 1, so an old message passes each filter), creates `deliveries` from the messages with a space and the state `sent` or `previewed`, and adds `cards.space`.
7. **The dashboard.** The Conversations tab shows the spaces of a conversation (`Review, Team`). `L` opens the pick dialog of the spaces that do not list the conversation, then asks for the level of the new link (`all` or `mentions`). `m` moves the conversation to one space (it removes the other links). `l` sets the level of each link of the conversation. The Spaces tab shows each conversation that the space lists.

## Risks / Trade-offs

- [Store upgrade] → High (approved). Backup `inbox.db.v6.bak`; one transaction; a test upgrades a real version 6 store.
- [More agent cost] → High (approved). Only a conversation with two links or more makes two batches.
- [The source reads more messages] → Medium (approved). Only when one link is wider; retention prunes them.
- [A card from a dry-run batch has no terminal] → It keeps the old rule.
