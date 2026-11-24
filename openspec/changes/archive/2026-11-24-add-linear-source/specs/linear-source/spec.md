## ADDED Requirements

### Requirement: Personal API key authentication
The Linear source SHALL send the key from the secret store in the header `Authorization: <key>` to the GraphQL endpoint of the account (`url`, default `https://api.linear.app/graphql`). When the endpoint returns HTTP 401 or the GraphQL error code `AUTHENTICATION_ERROR`, the source SHALL report an authentication error that names the account ID. When it returns HTTP 429 or the code `RATELIMITED`, the source SHALL retry with exponential backoff, up to 3 times.

#### Scenario: Revoked key
- **WHEN** the endpoint returns HTTP 401 during a poll
- **THEN** the poll fails with an authentication error that names the account, the cursor does not move, and the other accounts continue

#### Scenario: Rate limit
- **WHEN** the endpoint returns the code `RATELIMITED` one time and then a result
- **THEN** the poll returns the result

### Requirement: Only queries
The Linear source SHALL send only GraphQL queries. It SHALL NOT send a mutation, so it never marks a notification as read and never writes to Linear.

#### Scenario: Inbox stays unread
- **WHEN** the user has unread Linear notifications and the source polls an `inbox` feed
- **THEN** each request body is a query, and the `readAt` of each notification is the same before and after the poll

### Requirement: Feeds of a Linear account
Each feed of a Linear account SHALL be one conversation, with the feed ID as the conversation ID and the feed name as the conversation name. A `comments` feed SHALL read the issue comments. An `inbox` feed SHALL read the notifications of the user. The source SHALL use the read level of the feed in the configuration, or `unsorted.read` when no space lists the feed. A feed with the level `off` SHALL not be read.

#### Scenario: Feed in a space
- **WHEN** the space `triage` lists the feed `agent` of the account `lin` with the read level `all`
- **THEN** the matching comments of `agent` go to the agent of `triage`

#### Scenario: Feed with the level off
- **WHEN** the feed `inbox` has the read level `off`
- **THEN** the poll request has no part for `inbox`

### Requirement: Match rule of a feed
An item SHALL match a feed when all these conditions are true:
- Its author (comment) or actor (notification) is in `from`. `me` is the user of the key; another value is a display name or an email, without case. An empty `from` SHALL be `["me"]` for a `comments` feed and anyone for an `inbox` feed.
- When `projects`, `labels` or `teams` is not empty, the issue is in one of the projects, has one of the labels and is in one of the teams (team key). Names SHALL compare without case.
- When `prefix` is not empty, the body without leading spaces and leading mentions (`@<name>` or a Linear profile URL) starts with `prefix`, with case. One Markdown backslash before the prefix is ignored. A notification with no comment SHALL NOT match a feed with a prefix.

A comment with no issue SHALL NOT match.

#### Scenario: Comment with a self-mention and the prefix
- **WHEN** the user writes the comment `@me.user /agent please check this` and the feed `agent` has `prefix = "/agent"` and `from = ["me"]`
- **THEN** the next poll returns the comment as a message for the user

#### Scenario: Mention as a profile URL
- **WHEN** the user writes `https://linear.app/acme/profiles/me.user /agent please check`
- **THEN** the feed `agent` returns the comment

#### Scenario: Comment with the prefix only
- **WHEN** the user writes the comment `/agent summarize this issue`
- **THEN** the feed `agent` returns the comment

#### Scenario: Prefix not at the start
- **WHEN** the user writes the comment `please ask /agent later`
- **THEN** the feed `agent` does not return the comment

#### Scenario: Comment of another person
- **WHEN** another person writes `/agent check this` and `from` is empty
- **THEN** the feed `agent` does not return the comment

#### Scenario: Project filter
- **WHEN** a feed has `projects = ["Bridge"]` and the user writes `/agent check` on an issue of another project
- **THEN** the feed does not return the comment

#### Scenario: Label filter
- **WHEN** a feed has `labels = ["infra"]` and the issue has the label `Infra`
- **THEN** the feed returns the matching comment

### Requirement: Messages of a Linear feed
Each message SHALL have the feed as the conversation, the issue identifier as the thread ID, the display name of the author or actor as the author, the comment URL (else the issue URL) as the URL, and a text that starts with the line `<identifier> <issue title>`. A message of a `comments` feed SHALL be for the user. A notification SHALL be for the user when its type is an assignment to the user, a mention, a comment mention or a review request. A comment with a parent comment SHALL have the reply reference: the parent ID, its author (or "" when the user wrote it), whether the user wrote it, and at most 200 characters of its text.

#### Scenario: Inbox feed with the level mentions
- **WHEN** the feed `inbox` has the level `mentions` and the inbox has one new comment on a followed issue and one mention
- **THEN** the poll returns only the mention

#### Scenario: Reply to a comment
- **WHEN** the user writes `/agent check` as a reply to a comment of `alice`
- **THEN** the message has the reply reference with the author `alice` and the parent text

### Requirement: Cursor of a Linear feed
The cursor SHALL hold one time for each feed. A `comments` feed SHALL read the comments with `updatedAt` after its time. An `inbox` feed SHALL read the notifications with `createdAt` after its time. On the first poll, the time SHALL be the current time minus `lookback`. One poll SHALL send one request for all the feeds of the account, and more requests only for a feed with more pages, at most 10 pages for a feed. When a poll fails, the cursor SHALL NOT move.

#### Scenario: Edit adds the prefix
- **WHEN** the user edits an old comment and adds `/agent` at the start
- **THEN** the next poll returns the comment

#### Scenario: Edit of a collected comment
- **WHEN** the user edits a comment that the source collected before
- **THEN** the store keeps one message for it and no new message goes to the agent

#### Scenario: Read in Linear
- **WHEN** the user reads a collected notification in Linear
- **THEN** the next poll does not return it again

#### Scenario: One request for two feeds
- **WHEN** an account has two feeds with one page of new items each
- **THEN** the poll sends one request

### Requirement: No reply to Linear
The system SHALL NOT send a reply to a Linear message. The Feed and `i2a reply` SHALL refuse it with the error for a platform that cannot send replies.

#### Scenario: Reply to a Linear card
- **WHEN** the user presses `r` on a card of a Linear message
- **THEN** the Feed shows that the platform cannot send replies, and no request goes to Linear
