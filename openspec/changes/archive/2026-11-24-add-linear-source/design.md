## Context

i2a reads Mattermost with REST polls and WhatsApp with a linked device. Linear has webhooks, but a webhook needs a public HTTPS URL, and i2a runs on the computer of the user. The Linear GraphQL API (`POST https://api.linear.app/graphql`, header `Authorization: <personal API key>`) gives the comments and the notifications of the user. These facts come from real requests on 2026-11-25:

- `comments(filter: CommentFilter)` filters on `user` (`isMe`, `displayName`, `email`), `body` (`contains`, `startsWith`), `createdAt`, `updatedAt` and `issue`.
- `notifications(filter: NotificationFilter)` filters on `createdAt`, `updatedAt`, `type` only. `IssueNotification` has `issue`, `comment`, `team`, `actor`. `PullRequestNotification` has `pullRequestId` and no PR object.
- A read in Linear changes `updatedAt` of a notification. `createdAt` does not change.
- In 250 notifications, no notification has the user as the actor: Linear does not notify the user of the actions of the user.
- A comment body keeps a mention as `@<displayName>`.
- Pages of `comments` and `notifications` come newest first. `after` with the `endCursor` gives the next page.
- A new comment can have an `updatedAt` some milliseconds before its `createdAt`.
- A comment that the API creates keeps its body as sent: `@me.user /agent ...` and `>agent ...` stay plain text.
- A bad key gives HTTP 401 with the code `AUTHENTICATION_ERROR`.
- Limits: 2,500 requests per hour and 3,000,000 complexity points per key. One request with two feeds costs about 40 points.

## Goals / Non-Goals

**Goals:**

- Collect the comments of the user that start with a prefix, for example `/agent`, with or without a self-mention in front.
- Collect the Linear inbox of the user.
- Filter by account, project, people, label and prefix. Route each feed to spaces with the current link model.
- No public server, no write to Linear, no change of the read state.

**Non-Goals:**

- Reply to Linear (a comment from the Feed). It is a later change.
- Issue descriptions that start with the prefix.
- Comments of project updates, documents and initiatives.
- Webhooks and the Linear sync WebSocket (not a public API).
- Feed editing in the dashboard. The user or an agent edits `config.toml`.

## Decisions

### A feed is a conversation that the user defines

A Linear workspace has no small set of channels. A team is too wide, and one conversation for each issue gives a long list. So the user names feeds in the account entry:

```toml
[[accounts]]
id = "linear-work"
platform = "linear"
read_every = "30s"

[[accounts.feeds]]
id = "agent"          # the conversation ID
name = "Agent requests"
kind = "comments"     # comments | inbox
from = ["me"]         # "me", a display name or an email. Empty: ["me"] for comments, anyone for inbox
prefix = "/agent"     # empty: no prefix rule
projects = []         # project names, empty: all
labels = []           # issue label names, empty: all
teams = []            # team keys, for example "ENG"; empty: all
```

The feed ID is the conversation ID in `[[spaces.conversations]]`. `Levels`, `Links`, `SetSpaces` and the read levels work with no change. Alternatives: a conversation for each team (no prefix or people rule, and the routing does not follow the use case), filters on each link (a large change of the link model for one platform).

### Match rule

A comment or a notification matches a feed when all of these are true:

1. Its author (comments) or actor (inbox) is in `from`. For an `inbox` feed, an empty `from` means anyone, because the inbox holds the actions of other people.
2. Its issue is in `projects`, has one of `labels` and is in `teams`, when the list is not empty. Names compare without case.
3. When `prefix` is not empty: the body, after leading spaces and leading `@<name>` mentions, starts with `prefix` (case-sensitive). For an `inbox` feed, the body is the comment of the notification; a notification with no comment does not match a prefix.

The request filters on time, on the author and on `body contains prefix`, to make the response small. The source applies the full rule to each item. Thus one code path decides a match for both kinds.

Comments with no issue are skipped.

### Messages

| Field | `comments` | `inbox` |
|---|---|---|
| `ExternalID` | comment ID | notification ID |
| `ChannelID`, `ChannelName` | feed ID, feed name | feed ID, feed name |
| `ChannelKind` | `channel` | `channel` |
| `ThreadID` | issue identifier (`ENG-12`) | issue identifier, else "" |
| `Author` | display name of the author | display name of the actor |
| `Text` | `ENG-12 <issue title>` line, a blank line, the body | the same with the comment body, else a line from the type (`assigned you to ENG-12 <title>`) |
| `URL` | comment URL | comment URL, else issue URL, else "" |
| `ForMe` | true | true for an assignment, a mention, a comment mention and a review request |
| `ReplyTo` | parent comment: ID, author, `Me`, text (200 characters) | the same for a comment of the notification |

`ForMe` makes the read levels useful: `mentions` gives each matching comment and the inbox items for the user; `all` gives the whole inbox.

### Cursor and polls

The cursor of an account holds one RFC 3339 time for each feed: `{"agent": "2026-11-25T10:00:00.123Z"}`. A `comments` feed uses `updatedAt`, so a comment that the user edits to add the prefix is found. The store refuses a second copy of the same `(source_id, external_id)`, so an edit never sends a comment two times. An `inbox` feed uses `createdAt`, because a read changes `updatedAt`. On the first poll, the cursor is the current time minus `lookback`. Each request reads from 2 minutes before the cursor, because Linear can give an item a time before a time that a poll saw already; the store drops the items that it has.

One poll sends one GraphQL document with one alias for each feed (`f0`, `f1`, ...), `first: 100`. A feed with `hasNextPage` gets more requests with `after`, at most 10 pages. When the 10 pages are full, the source logs a warning and moves the cursor to the newest item, the same as the page limit of Mattermost. The poll frequency is `read_every` of the account (at least 5 s, default 1 m).

Errors: HTTP 401 or the GraphQL code `AUTHENTICATION_ERROR` gives an authentication error that names the account. HTTP 429 or the code `RATELIMITED` gives a retry with exponential backoff, up to 3 times. Another error fails the poll, and the cursor does not move.

### Only queries

The client builds each document from a fixed template that starts with `query`. A test reads each request body of a poll and fails on `mutation`. The source never calls `notificationUpdate`, `notificationMarkReadAll` or `commentCreate`.

### Account entry

- `platform = "linear"`. `url` is optional: the GraphQL endpoint, default `https://api.linear.app/graphql`. The tests and the e2e harness set it to a fake server.
- The secret store keeps the key at `secrets.TokenKey(id)`, the same as Mattermost.
- `user` gets `<display name>@<workspace key>` after the connection check, so two accounts are easy to tell apart (`viewer { displayName organization { name urlKey } }`).
- `i2a account add linear` and the dashboard add two feeds: `agent` (`comments`, `from = ["me"]`, `prefix = "/agent"`) and `inbox` (`inbox`). Both start in Unsorted.

### Dashboard

The add form gets the choice Linear: account ID, name, API key, "Connect". The key help names Linear › Settings › Security & access › Personal API keys. The detail of a Linear account shows the user, the workspace, and one line for each feed (`agent · comments · from me · prefix /agent`), with the note "Edit [[accounts.feeds]] in config.toml". `loadConversations` lists the feeds from the configuration after the connection check.

### No reply

`canReply` stays false for `linear`. The Feed shows the current message for a platform with no reply.

## Risks / Trade-offs

- [The Linear editor changes a prefix such as `>agent` into a quote block] → capture real comments with `/agent` and `>agent`; the match rule follows the stored body. The help of `prefix` names the characters that Markdown changes.
- [A comment edited many times, or reactions, change `updatedAt`] → the store skips a known comment ID, so the cost is one more item in a response.
- [A feed with no `prefix` and `from` of many people reads many comments] → the 10 page limit and the warning; the help of `from` says this.
- [The same comment is in an `inbox` feed and a `comments` feed] → two messages with different IDs. A `comments` feed reads the user by default and the inbox has no item from the user, so this needs a custom `from`.
- [Private data in fixtures: the key belongs to a company workspace] → the capture tool replaces IDs, names, texts, URLs and the workspace key. The author reads its output before each commit. `task leakcheck` runs before each push.
- [Pull request notifications have little data] → the text names the type and the actor only.

## Migration Plan

The change is additive. A configuration file with no Linear account does not change. The `config.toml` version stays 3. The store schema does not change. Rollback: an older build refuses `platform = "linear"` in `config check`; remove the account to go back.

## Open Questions

- The stored form of a mention and of `>agent` typed in the Linear app (not the API). The match rule also accepts a Markdown backslash before the prefix and a profile URL as a mention. The manual verify checks a comment typed in the app.
