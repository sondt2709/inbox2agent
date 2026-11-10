## MODIFIED Requirements

### Requirement: Thread of a collected message
For each collected post, the source SHALL keep the ID of the thread root (`root_id`) as the thread ID of the message. A top-level post SHALL have an empty thread ID. For a post with `root_id`, the source SHALL also keep the reply reference: the root ID, whether the user wrote the root, the username of the root author (or "" when the user wrote it) and at most 200 characters of the root text. The source SHALL take the root from the page of posts, else from its cache of roots, else with `GET /posts/{root_id}`. When the GET returns HTTP 403 or 404, the reply reference SHALL have the root ID only. Another error SHALL fail the poll of the channel, and the channel cursor SHALL NOT move.

#### Scenario: Reply in a thread collected
- **WHEN** the source collects a post with `root_id` R
- **THEN** the stored message has the thread ID R

#### Scenario: Root not in the page
- **WHEN** the source collects a reply whose root `alice` posted two days before the cursor
- **THEN** the source reads the root with `GET /posts/{root_id}`, and the message has the reply reference with the author `alice` and the root text

#### Scenario: Deleted root
- **WHEN** `GET /posts/{root_id}` returns HTTP 404
- **THEN** the source collects the reply with a reply reference that has the root ID only

### Requirement: Collect new messages by read level
The source SHALL read each member channel, direct channel and group channel of the user. For each channel it SHALL use the read level of the conversation in the configuration, or `unsorted.read` when no space lists the channel. It SHALL collect the posts that were created after the channel cursor: each post for `all`; for `mentions`, each post of a direct channel and, in other channels, only posts that contain `@<username>` or that reply in a thread whose root the user wrote; no post for `off`. A post of a direct channel, a post with `@<username>` and a reply in a thread whose root the user wrote SHALL be for the user. The source SHALL skip deleted posts, system posts and posts by the user. On the first poll, the cursor SHALL be the current time minus `lookback`.

#### Scenario: Channel with read level all
- **WHEN** another user posts in a channel with the read level `all`
- **THEN** the next poll returns that post with the channel name, the author username, the text, the time and a permalink

#### Scenario: Channel with read level mentions
- **WHEN** another user posts in a channel with the read level `mentions`, with no mention of the user
- **THEN** the poll does not return that post

#### Scenario: Mention in an unsorted channel
- **WHEN** `unsorted.read = "mentions"` and another user posts `@<username> please check` in a channel that no space lists
- **THEN** the poll returns that post

#### Scenario: Reply to the user in a thread
- **WHEN** a channel has the read level `mentions` and another user replies, with no mention, in a thread whose root the user wrote
- **THEN** the poll returns that post as a message for the user

#### Scenario: Direct message with read level mentions
- **WHEN** a direct channel has the read level `mentions` and the other user writes "lunch?"
- **THEN** the poll returns that post with the kind `direct`

#### Scenario: Direct channel off
- **WHEN** the direct channel with `bob` has the read level `off` and `bob` posts in it
- **THEN** the poll does not return that post
