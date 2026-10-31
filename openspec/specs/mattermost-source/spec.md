# mattermost-source Specification

## Purpose
This capability collects new messages from a Mattermost server with a personal access token. It reads the selected channels, the direct messages and the mentions, and it does not change the read state.
## Requirements
### Requirement: Personal access token authentication
The Mattermost source SHALL send the token from the secret backend in the header `Authorization: Bearer <token>`. When the server returns HTTP 401, the source SHALL report an authentication error that names the source ID.

#### Scenario: Revoked token
- **WHEN** the server returns HTTP 401 during a poll
- **THEN** the log shows an authentication error for that source, and the other sources continue

### Requirement: Read state does not change
The source SHALL use only read requests to collect messages. The source SHALL NOT call any API that marks a channel or a post as read when it collects messages. To send a reply, the source SHALL use only `GET /posts/{id}`, `GET /channels/{id}/posts` and `POST /posts?set_online=false`. Mattermost marks the channel read for a top-level post of the user and keeps the read state for a reply in a thread; the source SHALL NOT add a request that changes this.

#### Scenario: Unread count stays the same
- **WHEN** the user has unread messages and the source polls
- **THEN** the unread message count and the mention count of the user are the same before and after the poll

#### Scenario: Reply in a thread
- **WHEN** the user has unread messages in a channel and the source sends a reply in a thread there
- **THEN** the `last_viewed_at` of the user in that channel is the same before and after the send

### Requirement: Efficient polling
The source SHALL skip a channel when its `last_post_at` is not newer than its cursor. The source SHALL follow pagination until it has all new posts. When the server returns HTTP 429, the source SHALL wait and retry with exponential backoff, up to 3 times.

#### Scenario: Quiet channel skipped
- **WHEN** a selected channel has no post after its cursor
- **THEN** the source does not request the posts of that channel

#### Scenario: Many new posts
- **WHEN** a channel has more new posts than one page holds
- **THEN** the poll returns all new posts

### Requirement: Send a reply to a post
The source SHALL send a reply as the user of the token with `POST /posts`. Before the send, it SHALL read the target post with `GET /posts/{id}` to get the channel and the thread root. A reply in a thread SHALL have `root_id` set to the thread root. A reply in the channel SHALL have no `root_id`. Each try of one reply SHALL send the same `pending_post_id` in the format `<user id>:<time in ms>`.

#### Scenario: Reply to a post in a thread
- **WHEN** the target post has the `root_id` R and the place is thread
- **THEN** the request `POST /posts` has `root_id` R

#### Scenario: Deleted target post
- **WHEN** the target post was deleted
- **THEN** the source returns the error of the server and does not call `POST /posts`

### Requirement: Thread of a collected message
For each collected post, the source SHALL keep the ID of the thread root (`root_id`) as the thread ID of the message. A top-level post SHALL have an empty thread ID.

#### Scenario: Reply in a thread collected
- **WHEN** the source collects a post with `root_id` R
- **THEN** the stored message has the thread ID R

### Requirement: Collect new messages by read level
The source SHALL read each member channel, direct channel and group channel of the user. For each channel it SHALL use the read level of the conversation in the configuration, or `unsorted.read` when no space lists the channel. It SHALL collect the posts that were created after the channel cursor: each post for `all`; for `mentions`, each post of a direct channel and, in other channels, only posts that contain `@<username>`; no post for `off`. The source SHALL skip deleted posts, system posts and posts by the user. On the first poll, the cursor SHALL be the current time minus `lookback`.

#### Scenario: Channel with read level all
- **WHEN** another user posts in a channel with the read level `all`
- **THEN** the next poll returns that post with the channel name, the author username, the text, the time and a permalink

#### Scenario: Channel with read level mentions
- **WHEN** another user posts in a channel with the read level `mentions`, with no mention of the user
- **THEN** the poll does not return that post

#### Scenario: Mention in an unsorted channel
- **WHEN** `unsorted.read = "mentions"` and another user posts `@<username> please check` in a channel that no space lists
- **THEN** the poll returns that post

#### Scenario: Direct message with read level mentions
- **WHEN** a direct channel has the read level `mentions` and the other user writes "lunch?"
- **THEN** the poll returns that post with the kind `direct`

#### Scenario: Direct channel off
- **WHEN** the direct channel with `bob` has the read level `off` and `bob` posts in it
- **THEN** the poll does not return that post

#### Scenario: Own post
- **WHEN** the user posts in a channel with the read level `all`
- **THEN** the poll does not return that post

