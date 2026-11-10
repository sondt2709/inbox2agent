## Why

A person often answers an earlier message: a quote in WhatsApp, a thread reply in Mattermost. Today the agent cannot join the answer to the earlier message. A WhatsApp line has only one `> ` line of quoted text, with no author and no ID. A Mattermost line has no thread data: i2a stores `thread_id` but does not send it. On the local store, 133 WhatsApp messages have a quote and 91 Mattermost messages are thread replies. Also, a Mattermost thread reply under a post of the user is not for the user, but a WhatsApp quote of a message of the user is.

## What Changes

- Each data line of a batch and of an ask gets the optional field `reply_to` when the message quotes an earlier message (WhatsApp) or is a thread reply (Mattermost): `{"id": <row ID or 0>, "author": "<name>", "me": <bool>, "text": "<at most 200 characters>"}`. `id` is the row ID of the earlier message in the store, or 0 when the store does not have it. `me` is true when the user wrote the earlier message.
- The WhatsApp source reads the quote of each message when it stores the message: the quoted message ID, the author and the text.
- The Mattermost source finds the thread root of each reply: in the page of posts, else with `GET /posts/{root_id}`.
- In Mattermost, a thread reply under a post of the user is for the user (`ForMe`). A `mentions` link reads it.
- The text before the data block explains `reply_to`. The primer does not change, so a reuse session does not restart.
- Store schema version 9: the columns `messages.reply_ext`, `messages.reply_author`, `messages.reply_me` and `messages.reply_text`, with the backup `inbox.db.v8.bak`.
- The WhatsApp message file gets the columns `wa_messages.quote_id`, `quote_author` and `quote_me`.
- A WhatsApp message text keeps its `> ` line.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `batch-rendering`: the optional field `reply_to` in each data line, and the sentence before the data block.
- `whatsapp-source`: the source keeps the reference to the quoted message.
- `mattermost-source`: the source keeps the thread root data, and a thread reply under a post of the user is for the user.
- `message-store`: schema version 9.

## Impact

- `internal/inbox/message.go`: the reply reference of a message.
- `internal/source/whatsapp`: `extract.go`, `conn.go` (`rowFromEvent`), `db.go` (columns), `source.go` (`message`).
- `internal/source/mattermost/source.go`: the root lookup and the `ForMe` rule.
- `internal/store/store.go`: schema version 9; `messageCols` finds the row ID of the earlier message.
- `internal/render/sanitize.go`, `batch.go`: `Item.ReplyTo` and the text before the data block.
- `CLAUDE.md`, `docs/`.
- No new dependency. No change to `config.toml`, the daemon API or the primer.
