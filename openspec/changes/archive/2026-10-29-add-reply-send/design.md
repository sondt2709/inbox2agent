## Context

The Feed shows cards with a suggested reply, but the user must copy the reply, open Mattermost, paste and send (`c`, `o`, paste, Send, `d`). i2a only reads from sources: `README.md` says "`i2a` uses read-only API calls", and the `mattermost-source` spec allows only read requests.

The stakeholder approved the workflow and the gate on 2026-10-29: sending is always on (no opt-in switch), the card becomes `done` after a send, and the agent can send with a CLI command. The stakeholder removed the broadcast-mention check and the rate limit. During apply, the stakeholder removed the "Mark as read" switch (see D7a).

Facts from the Docker Mattermost server (main @ 59ed9d5):
- `root_id` must be the thread root. The ID of a reply gives HTTP 400 "Invalid RootId parameter".
- `GET /posts/{id}` returns `root_id` and `channel_id`.
- A top-level `POST /posts` of the user sets `last_viewed_at` (the channel becomes read). A reply in a thread does not. `set_online=false` does not change this; it only keeps the status of the user. (The explore probe compared a thread reply with a top-level post, so it showed the wrong cause. The apply probe in `read-state-probe.txt` tested the 4 cases.)
- Two `POST /posts` calls with the same `pending_post_id` return the same post.

Constraints: one static binary, no new dependency, no network listener. The daemon, the dashboard and the CLI are separate processes on one SQLite file. Card text comes from an agent that reads untrusted messages.

## Goals / Non-Goals

**Goals:**
- Send the suggested reply, or an edited reply, from the Feed with two keys (`r`, `ctrl+s`).
- Let the agent send a reply with one command, when the user allows it.
- Never post the same reply two times because of a retry.
- Send no request that changes the read state.
- One send path for all platforms; Mattermost now.

**Non-Goals:**
- A new message that is not a reply to a stored message (no free channel or user target).
- Edit or delete a sent post.
- Files, reactions or formatting help.
- Sending the replies of the user back to the agent as context.
- A check of broadcast mentions (`@channel`, `@all`, `@here`) and a rate limit. The stakeholder removed them.

## Decisions

### D1. One send flow in a new package `internal/reply`

The Feed and `i2a reply` call `reply.Send`. It does these steps:
1. Validate the text and find the target message in the store.
2. Find an earlier reply with the same message, place and text (D5).
3. Record the reply as `sending` with a pending ID.
4. Call the `Replier` of the source of the message.
5. Record `sent` or `failed`. On `sent`, set the card `done` when a card is given.

Alternative: logic in the TUI and the CLI separately. Rejected: the retry and the duplicate rules must be the same in both.

### D2. `source.Replier` is an optional interface

`ReplyRequest` and `Posted` are in `internal/inbox`, because `source` imports `mattermost`, and `mattermost` must not import `source`. The source makes the platform duplicate guard from `PendingID`.

```go
type ReplyRequest struct {
    Target    inbox.Message // a stored message
    Text      string
    Thread    bool          // true: reply in the thread of Target; false: post in the channel
    PendingID string        // the same value for each try of one reply
    Since     time.Time     // the time of the first try
}
type Replier interface {
    Reply(ctx context.Context, r ReplyRequest) (Posted, error)
    // Find looks for a post of the user with the text of r, created after r.Since.
    Find(ctx context.Context, r ReplyRequest) (Posted, bool, error)
}
```

`source.BuildReplier(cfg, sourceID, secrets, http)` builds the source and returns `ErrCannotReply` when the source type does not implement `Replier`. The daemon does not use it.

An error from `Reply` is one of two kinds. An error with the method `Answered() bool` that returns true (`*mattermost.APIError`: the server answered) means that no post exists: the reply becomes `failed`. Any other error (time-out, connection reset) means that the result is not known: the reply stays `sending`.

Alternative: a method on all sources. Rejected: WhatsApp, GitHub and Linear come later, and a source without send support must still build.

### D3. Mattermost requests

- `GET /posts/{external_id}` before each send. It gives `channel_id` and `root_id`, and it fails when the post was deleted. Root = `root_id`, or the post ID when `root_id` is empty.
- `POST /posts?set_online=false` with `{channel_id, message, root_id, pending_post_id}`. `root_id` is empty when `Thread` is false. `set_online=false` keeps the status of the user: i2a posts, not the person at the keyboard.
- `pending_post_id` = `<user id>:<PendingID>`. `PendingID` is the creation time of the reply row in ms. This is the format of the Mattermost web app.
- `Find`: `GET /channels/{id}/posts?page=0&per_page=200` (newest first, like polling). Find a post with the same `user_id`, `message` and `root_id`, and `create_at` after `Since`.
- The 429 retry of `Client.do` applies. A 429 is an answer, so no post exists.

Polling does not change: it still uses only read requests.

### D4. Store: schema version 4

```sql
ALTER TABLE messages ADD COLUMN thread_id TEXT NOT NULL DEFAULT '';
CREATE TABLE replies (
  id           INTEGER PRIMARY KEY,
  message_id   INTEGER NOT NULL,      -- the target; the message can be pruned later
  card_id      INTEGER,               -- NULL for i2a reply --to without --card
  source_id    TEXT NOT NULL,
  channel_name TEXT NOT NULL,         -- for the "Replied" line after the message is pruned
  place        TEXT NOT NULL,         -- thread or channel
  text         TEXT NOT NULL,
  by           TEXT NOT NULL,         -- user (Feed) or agent (i2a reply)
  state        TEXT NOT NULL,         -- sending, sent, failed
  pending_id   TEXT NOT NULL,
  external_id  TEXT NOT NULL DEFAULT '',
  url          TEXT NOT NULL DEFAULT '',
  error        TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL,
  sent_at      INTEGER
);
CREATE INDEX replies_card ON replies(card_id);
CREATE INDEX replies_target ON replies(message_id, place);
```

The Mattermost source fills `thread_id` with `root_id` when it collects a post. Messages from before version 4 have `''`. Only the default place uses `thread_id`; the send reads the root from the server (D3).

The migration writes the backup `inbox.db.v3.bak`, then adds the column and sets the version in one transaction. The table comes from the schema text, as for cards. Retention deletes replies that are older than `retention`.

### D5. Retry without a duplicate

Before step 3 of D1, `Send` looks for a reply row with the same `message_id`, `place` and `text`:

| Earlier row | Action |
|---|---|
| `sent` | Refuse: "this reply was sent at 14:52". Exit 1 for the CLI. |
| `sending` (time-out, or the process stopped) | Call `Find` first. When it finds the post, record `sent`. If not, call `Reply` with the same pending ID. |
| `failed` | Use the same row and pending ID, and call `Reply` again. |
| none | New row and new pending ID. |

The `pending_post_id` duplicate guard of Mattermost uses a cache with a limited lifetime. `Find` covers a retry after that time. A changed text is a new reply, on purpose.

### D6. Default target and place

- Reply to: the newest of the card refs that still exist in the store. The form can select another ref. `i2a reply --card` uses the newest ref; `--to` selects a message.
- Where: `thread` when the message has a `thread_id` or is in a channel of kind `channel`; `channel` for a top-level message in a direct or group conversation. The form shows `channel` as "Chat" for a direct or group conversation. `--thread` and `--channel` change the default.

### D7. Text rules

The text goes through `render.Sanitize` (hidden characters removed) and `strings.TrimSpace`. Empty text is an error. More than 4,000 characters is an error; the form text area has a limit of 4,000. i2a does not change mentions or Markdown.

### D7a. No Mark as read switch

The gate approved a "Mark as read" switch, off by default. The apply probe showed that Mattermost marks the channel read for each top-level post of the user, whatever the request parameters, and never for a thread reply. "Off" was not possible for a post in a chat, which is the default place for a direct message. The stakeholder chose to remove the switch. i2a sends no request that changes the read state, and the form says: "A thread reply keeps the read state. A chat post marks it read."

Alternatives: restore the unread state with `POST /users/{id}/posts/{id}/set_unread` after a top-level post (works on the Docker server, but adds 2 requests and counts the reply as unread); a switch for thread replies only.

### D8. Feed form

`r` builds a form with the engine of `form.go`: a note (Send to), a choice (Reply to, only when the card has more than one ref that exists), a choice (Where), a note about the read state, a text area (Text), and the buttons Send and Cancel. `ctrl+s` runs Send. Send runs in a `tea.Cmd`, so the UI does not stop. A failure shows the error under the text area and keeps the form. Success closes the form, sets the card `done` and shows "Sent to <channel> (<place>). The card is done." in the status line.

The detail panel shows one line for each sent reply of the card: "✓ Replied 14:52 by you → pr-reviews (thread)". The TUI builds the replier from `Deps.Config` and `Deps.Secrets`.

### D9. `i2a reply`

```
i2a reply --to <message id> [--card <id|key>] [--thread | --channel] < text
i2a reply --card <id|key> [...]          # no stdin text: the suggested reply of the card
```

- stdin is read when it is not a terminal. When stdin is empty and `--card` has a suggested reply, that reply is the text.
- With `--card`, the card becomes `done` after the send.
- Output on success: `sent to pr-reviews (thread of minh.tran). Reply 3.` Errors go to stderr with exit code 1.
- `by` = `agent`. A desktop notification "i2a · the agent sent a reply" with "<channel>: <text>", cut to 200 characters. `[notify] enabled = false` and `I2A_NOTIFY=off` turn it off.
- The card section of the primer does not change.

## Risks / Trade-offs

- [The agent posts as the user after a message tricks it] → The target is only a stored message, so the agent cannot reach a new channel or person. A notification shows each send. The primer does not mention the command, and the workspace needs an allow rule. The stakeholder accepted the remaining risk.
- [A sent post cannot be undone by i2a] → The form shows the target before the send. The user deletes a post in Mattermost.
- [A time-out after the server created the post] → D5: same pending ID, and `Find` before a second post.
- [The pending ID cache has a short lifetime] → Measured on the Docker server: the same `pending_post_id` gives the same post after 25 s and a new post after 35 s. The HTTP time-out of the client is 30 s, so a retry after a time-out is outside the cache. `Find` does not depend on the cache.
- [Polling skips the posts of the user] → The reply does not start a new batch. The agent does not see the reply; this is a non-goal.
- [A server with a lower post size limit] → 4,000 characters is below the minimum Mattermost limit of 16,383.

## Migration Plan

1. The first `i2a` command after the upgrade writes `inbox.db.v3.bak` and migrates the store to version 4.
2. No configuration change. Sending works with the existing token.
3. Rollback: install the earlier `i2a` and copy `inbox.db.v3.bak` to `inbox.db`. Replies that were sent stay in Mattermost.

## Open Questions

- None.
