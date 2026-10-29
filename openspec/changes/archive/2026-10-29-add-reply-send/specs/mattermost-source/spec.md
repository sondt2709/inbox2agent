## MODIFIED Requirements

### Requirement: Read state does not change
The source SHALL use only read requests to collect messages. The source SHALL NOT call any API that marks a channel or a post as read when it collects messages. To send a reply, the source SHALL use only `GET /posts/{id}`, `GET /channels/{id}/posts` and `POST /posts?set_online=false`. Mattermost marks the channel read for a top-level post of the user and keeps the read state for a reply in a thread; the source SHALL NOT add a request that changes this.

#### Scenario: Unread count stays the same
- **WHEN** the user has unread messages and the source polls
- **THEN** the unread message count and the mention count of the user are the same before and after the poll

#### Scenario: Reply in a thread
- **WHEN** the user has unread messages in a channel and the source sends a reply in a thread there
- **THEN** the `last_viewed_at` of the user in that channel is the same before and after the send

## ADDED Requirements

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
