## Context

A data line has the fields `id`, `n`, `source`, `source_id`, `channel`, `kind`, `author`, `time`, `url`, `text` and `truncated` (`render.Item`). Facts from the local store and the WhatsApp file (read only, 2026-11-05):

- WhatsApp: 133 stored messages start with the `> ` quote line. Of 33 quotes by other people in `wa_messages`, the quoted message is in `wa_messages` for 29 (6 of them are messages of the user) and in `inbox.db` for 22.
- Mattermost: 91 stored messages have a `thread_id`. The root is in `inbox.db` for 37 of them. The root is not collected when it is older than the cursor, when the read level filters it, or when the user wrote it (the source skips posts of the user).
- The WhatsApp `ContextInfo` of a quote has `stanzaID` (the quoted message ID), `participant` (the author JID) and `quotedMessage`. The fixture `internal/source/whatsapp/testdata/messages.jsonl` has real rows with these fields.
- The Mattermost `PostList` of `GET /channels/{id}/posts` can hold thread roots that are not in `order`. `GET /posts/{id}` returns one post and does not mark a channel read; the reply flow uses it already (`post_root.json`, `post_reply.json`).
- `render.Digest` writes the text before the data block. That text is not in the primer hash (`primerHash` uses the template and the card section only).

## Goals / Non-Goals

**Goals:**

- The agent can join an answer to the earlier message: by row ID when the store has it, else by author and text.
- The agent knows when the earlier message is a message of the user.
- A Mattermost thread reply under a post of the user is for the user, as a WhatsApp quote of a message of the user is.

**Non-Goals:**

- Messages of the user stay out of the batches. A quote by the user in the app does not close a card (meaning B of the request).
- No new field in the Feed. The Feed keeps the WhatsApp `> ` line in the text.
- No backfill: the messages stored before the upgrade have no reply data.
- Mattermost quotes in Markdown (`> text`) and permalinks in the text are not references.
- No change to the reply flow. WhatsApp keeps an empty `ThreadID`, so the default reply place does not change.

## Decisions

### One field `reply_to`, the same for both platforms

`reply_to` is an object: `id` (row ID, 0 when not in the store), `author`, `me`, `text` (at most 200 characters, on one line). The field is absent for a message that answers nothing (`omitempty` on a pointer). Each data line still starts with `{"id":`.

Alternative: separate fields (`quote_id`, `thread_id`). Rejected: the agent needs one concept, "this message answers that message", and the platforms differ only in how they show it.

### The store keeps the platform ID; the row ID is found when i2a reads

Schema version 9 adds `messages.reply_ext` (the external ID of the earlier message), `reply_author`, `reply_me` and `reply_text`. `messageCols` gets the row ID with a subquery: `(SELECT r.id FROM messages r WHERE r.source_id = messages.source_id AND r.external_id = messages.reply_ext)`. The unique index `(source_id, external_id)` serves it. `Pending`, `DispatchMessages` and the card messages all use `messageCols`, so batches and asks get the same data.

Alternative: find the row ID in `SavePoll`. Rejected: the earlier message can arrive later (a requeue, an earlier lookback) and a prune can delete it; the read-time lookup is always current. A pruned message gives `id` 0, and `author` and `text` stay.

### WhatsApp: read the quote in the event handler

`rowFromEvent` already has the contact names (`names`) and the user (`Me`). It reads `ContextInfo`: `quote_id` = `stanzaID`, `quote_me` = `participant` is the user, `quote_author` = the contact name of `participant`, else its phone number as `+<number>`, else "". `quote_author` is "" when `quote_me` is true. The file `whatsapp-<id>.db` gets the three columns with `ALTER TABLE` (the same way as `media`). `collect` reads the quoted text from `raw` with `quoted()`, so the text is not stored twice. When `quote_author` is empty and `quote_me` is false, `collect` takes the sender name (and `from_me`) of the quoted row in `wa_messages`, when the file still has it.

Alternative: read all from `raw` in `collect`. Rejected: `collect` has no contact store and no `Me`.

### Mattermost: root from the page, else one GET, with a cache

For each post with `root_id`, the source takes the root from the `PostList` of the page, else from a cache in `Source`, else with `GET /posts/{root_id}`. The cache holds at most 1,000 roots (author ID and text) and drops all entries when it is full. HTTP 403 and 404 give an unknown root (no `reply_to` author or text, `me` false). Other errors fail the poll of the channel, as an error of `newPosts` does now, so the cursor does not move. The root author IDs go into the `usernames` call.

The `ForMe` rule becomes: a direct channel, a mention of the user, or a thread reply whose root author is the user. With the level `mentions`, the source needs the root of each thread reply before it filters. A busy channel at `mentions` therefore costs at most one GET for each new thread root in a poll.

Alternative: no GET, `id` and author only from the store. Rejected: 54 of 91 thread replies would have no author and no text, and the `ForMe` rule needs the root author.

### Explain `reply_to` before the data block, not in the primer

`Digest` adds one sentence: "`reply_to` names the earlier message that a message quotes or answers in a thread; `id` 0 means that the earlier message is not in i2a." A change to the primer would change the primer hash and restart each reuse session.

## Risks / Trade-offs

- [More Mattermost calls in channels with the level `mentions`] → The page roots and the cache come first. A GET is read only and does not mark a channel read.
- [`reply_to.text` is untrusted text from other people] → It gets `Sanitize` with 200 characters and goes inside the data fence, like `text`.
- [`reply_author` is untrusted] → It gets `field()`, like `author`.
- [The WhatsApp `> ` line and `reply_to.text` repeat the same text] → Accepted. The Feed needs the `> ` line, and the cost is at most 200 characters for each quote.
- [A thread reply under a post of the user makes more Mattermost messages `ForMe`] → This matches the WhatsApp rule and the Mattermost reply notification setting "threads that I start".
- [Schema upgrade] → The backup `inbox.db.v8.bak`, one transaction, the same pattern as versions 6 to 8.

## Migration Plan

1. On the first start, the store writes `inbox.db.v8.bak` and upgrades to version 9 in one transaction. The new columns are empty for existing rows.
2. The WhatsApp file gets the three columns at the next open.
3. Rollback: stop i2a, install the earlier binary, and copy `inbox.db.v8.bak` to `inbox.db`. The earlier binary ignores the extra columns of `wa_messages`.

## Open Questions

- Does a real Mattermost `PostList` hold thread roots that are not in `order`? The apply checks this on the Docker server. The GET fallback makes the result correct in both cases.
