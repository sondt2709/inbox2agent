# mattermost-source Specification

## Purpose
This capability collects new messages from a Mattermost server with a personal access token. It reads the selected channels, the direct messages and the mentions, and it does not change the read state.

## Requirements
### Requirement: Personal access token authentication
The Mattermost source SHALL send the token from the secret backend in the header `Authorization: Bearer <token>`. When the server returns HTTP 401, the source SHALL report an authentication error that names the source ID.

#### Scenario: Revoked token
- **WHEN** the server returns HTTP 401 during a poll
- **THEN** the log shows an authentication error for that source, and the other sources continue

### Requirement: Collect new messages from selected channels
The source SHALL collect posts that were created after the channel cursor in these channels:

- the selected channels, which can also be direct and group channels;
- all direct and group channels, when `all_direct` is on;
- any member channel, but only posts that contain `@<username>`, when `mentions` is on.

The source SHALL skip deleted posts, system posts and posts by the user. On the first poll, the cursor SHALL be the current time minus `lookback`.

#### Scenario: Post in a selected direct channel
- **WHEN** `all_direct` and `mentions` are off, the direct channel with `bob` is selected, and `bob` posts in it and in another channel
- **THEN** the next poll returns only the post in the direct channel

#### Scenario: Post in a selected channel
- **WHEN** another user posts in a selected channel
- **THEN** the next poll returns that post with the channel name, the author username, the text, the time and a permalink

#### Scenario: Post in a channel that is not selected
- **WHEN** another user posts in a channel that is not selected, with no mention of the user, and `mentions` is on
- **THEN** the poll does not return that post

#### Scenario: Mention in a channel that is not selected
- **WHEN** another user posts `@<username> please check` in a channel that is not selected, and `mentions` is on
- **THEN** the poll returns that post

#### Scenario: Direct message
- **WHEN** another user sends a direct message to the user and `all_direct` is on
- **THEN** the poll returns that post with the kind `direct`

#### Scenario: Own post
- **WHEN** the user posts in a selected channel
- **THEN** the poll does not return that post

### Requirement: Read state does not change
The source SHALL use only read requests. The source SHALL NOT call any API that marks a channel or a post as read.

#### Scenario: Unread count stays the same
- **WHEN** the user has unread messages and the source polls
- **THEN** the unread message count and the mention count of the user are the same before and after the poll

### Requirement: Efficient polling
The source SHALL skip a channel when its `last_post_at` is not newer than its cursor. The source SHALL follow pagination until it has all new posts. When the server returns HTTP 429, the source SHALL wait and retry with exponential backoff, up to 3 times.

#### Scenario: Quiet channel skipped
- **WHEN** a selected channel has no post after its cursor
- **THEN** the source does not request the posts of that channel

#### Scenario: Many new posts
- **WHEN** a channel has more new posts than one page holds
- **THEN** the poll returns all new posts

