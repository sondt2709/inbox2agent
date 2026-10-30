## REMOVED Requirements

### Requirement: Collect new messages from selected channels
**Reason**: The read level of each conversation replaces `channels`, `all_direct` and `mentions`.
**Migration**: See "Collect new messages by read level" below. The version 1 migration turns `channels` into conversations with the read level `all`, and `all_direct` or `mentions` into `unsorted.read = "mentions"`.

## ADDED Requirements

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
