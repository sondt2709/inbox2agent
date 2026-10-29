# reply-send Specification

## Purpose
This capability sends a reply from i2a to the conversation of a stored message, as the user of the source. The user sends from the Feed with the reply form; the agent sends with `i2a reply`. The capability keeps the target to stored messages, records each reply, and does not post the same reply two times.
## Requirements
### Requirement: Reply form in the Feed
The key `r` on a selected card SHALL open a reply form in the detail panel. The form SHALL have these fields: Send to (the channel, the source type and the source ID; not editable), Reply to (a choice of the card refs that exist in the store; shown only when there is more than one), Where (thread or channel; "Chat" instead of "Channel" for a direct or group conversation), a note about the read state, and Text (the suggested reply of the card, or empty). `ctrl+s` SHALL send the reply. `esc` SHALL close the form and send nothing. When the card has no ref that exists in the store, `r` SHALL show "The card has no message to reply to" and open no form.

#### Scenario: Send the suggested reply as it is
- **WHEN** the user selects a card with the suggested reply "Thanks Minh, I will review PR 57 this afternoon." and the ref 812, and presses `r` and then `ctrl+s`
- **THEN** Mattermost has a new post of the user with that text in the thread of message 812, the card is `done`, and the status line says "Sent to pr-reviews (thread). The card is done."

#### Scenario: Edit before the send
- **WHEN** the user presses `r`, adds " Can you confirm the old API has no change?" to the text, and presses `ctrl+s`
- **THEN** the post in Mattermost has the edited text

#### Scenario: Cancel
- **WHEN** the user presses `r` and then `esc`
- **THEN** no post is created and the card keeps its state

#### Scenario: Card without a message
- **WHEN** retention deleted all messages of a card and the user presses `r`
- **THEN** no form opens and the status line says "The card has no message to reply to"

### Requirement: Reply target and place
The target of a reply SHALL be a message in the store. The system SHALL NOT take a channel, a user or a URL from card text, from the reply text or from a command-line value other than a message ID or a card ID or key. The default target SHALL be the newest card ref that exists in the store. The default place SHALL be the thread of the message when the message is in a thread or in a channel of kind `channel`, and the channel when the message is a top-level message of a direct or group conversation. A reply in a thread SHALL use the root of the thread, also when the target message is a reply in that thread.

#### Scenario: Direct message
- **WHEN** the target is a top-level message from `@lena.wong` in a direct conversation
- **THEN** the default place is the channel, and the form shows "Chat"

#### Scenario: Reply to a reply
- **WHEN** the target message is a reply in the thread of message A, and the place is thread
- **THEN** the new post is in the thread of message A

#### Scenario: URL in the card body
- **WHEN** the card body contains `https://chat.example.com/team/pl/abc123`
- **THEN** the reply goes to the card ref message, not to the post in the URL

### Requirement: Reply text
The system SHALL remove hidden characters from the reply text with the rule of message text, and remove spaces at the start and the end. Empty text SHALL be an error. Text with more than 4,000 characters SHALL be an error. The system SHALL NOT change mentions or Markdown in the text.

#### Scenario: Empty text
- **WHEN** the user deletes all text in the form and presses `ctrl+s`
- **THEN** no post is created and the form shows "The text is empty" under the Text field

#### Scenario: Too long
- **WHEN** an agent runs `i2a reply` with 4,500 characters on stdin
- **THEN** the command exits with code 1, says that the maximum is 4,000 characters, and creates no post

### Requirement: Read state of a reply
The system SHALL NOT send a request that marks a channel as read or as unread. The read state SHALL change only as the platform changes it for a post of the user. The form SHALL tell the user that a thread reply keeps the read state and that a post in the channel or chat marks it read.

#### Scenario: Reply in a thread
- **WHEN** the user has unread messages in `pr-reviews` and sends a reply in a thread there
- **THEN** the `last_viewed_at` of the user in `pr-reviews` is the same before and after the send

#### Scenario: Post in the chat
- **WHEN** the user sends a reply in the chat of `@lena.wong`
- **THEN** Mattermost marks the chat read, and i2a sent only `GET /posts/{id}` and `POST /posts`

### Requirement: Failed send and safe retry
The system SHALL record each reply in the store with the state `sending` before it calls the platform, and `sent` or `failed` after. When the platform answers with an error, the reply SHALL be `failed`, the form SHALL show the error under the Text field and keep the text, and the card SHALL keep its state. When the result is not known (a time-out or a lost connection), the reply SHALL stay `sending`. A new send with the same target, place and text as an earlier reply SHALL:
- be refused with the time of the earlier send, when that reply is `sent`;
- first look on the platform for a post of the user with that text after the first try, when that reply is `sending`, and record `sent` without a new post when it finds one;
- use the same pending ID as the earlier try, when that reply is `sending` or `failed`.

#### Scenario: Time-out, then retry
- **WHEN** the first send times out after the server created the post, and the user presses `ctrl+s` again with the same text
- **THEN** Mattermost has one post with that text, and the reply is `sent`

#### Scenario: Server error
- **WHEN** the target post was deleted in Mattermost and the user presses `ctrl+s`
- **THEN** the form stays open with the text and shows the error of the server, and the card is not `done`

#### Scenario: Same reply again
- **WHEN** an agent runs `i2a reply --to 812` two times with the same text
- **THEN** the second call exits with code 1 and says that this reply was sent at the time of the first call, and Mattermost has one post

### Requirement: Send command
The system SHALL provide `i2a reply --to <message id>` and `i2a reply --card <id|key>`, with the optional flags `--thread` and `--channel`. The command SHALL read the text on stdin when stdin is not a terminal. When stdin gives no text and `--card` is set, the text SHALL be the suggested reply of the card. With `--card` and without `--to`, the target SHALL be the default target of the card. With `--card`, the card SHALL become `done` after a successful send. On success, the command SHALL print one line with the channel, the place and the reply number, and exit with code 0. On an error, it SHALL print the error on stderr and exit with code 1. A reply from this command SHALL be recorded as sent by the agent.

#### Scenario: Agent replies to a message
- **WHEN** an agent runs `i2a reply --to 7` with "Thanks Minh, noted." on stdin, and message 7 is in the thread of Minh in `pr-reviews`
- **THEN** Mattermost has the post in that thread, and the output is "sent to pr-reviews (thread of minh.tran). Reply <n>."

#### Scenario: Suggested reply of a card
- **WHEN** a user runs `i2a reply --card mm:pr-57` with no stdin text
- **THEN** Mattermost has a post with the suggested reply of that card, and the card is `done`

#### Scenario: Unknown message
- **WHEN** an agent runs `i2a reply --to 99999` and the store has no message 99999
- **THEN** the command exits with code 1, says that the store has no message 99999, and creates no post

### Requirement: Reply notification
On macOS, each successful send with `i2a reply` SHALL show one desktop notification with the title "i2a · the agent sent a reply" and the text `<channel>: <text>`, cut to 200 characters. `[notify] enabled = false` SHALL turn it off. A send from the Feed SHALL NOT show a notification.

#### Scenario: Agent send
- **WHEN** an agent sends "Thanks Minh, noted." with `i2a reply` to a message in `pr-reviews`
- **THEN** macOS shows "i2a · the agent sent a reply" with "pr-reviews: Thanks Minh, noted."

### Requirement: Replied line
The detail panel of a card SHALL show one line for each `sent` reply of the card, with the time, "you" or "the agent", the channel and the place, for example "✓ Replied 14:52 by you → pr-reviews (thread)". The list row of a card with a sent reply SHALL show "✓ replied".

#### Scenario: Done card with a reply
- **WHEN** the user shows the done cards with `a` and selects a card that the user replied to at 14:52 in the thread of `pr-reviews`
- **THEN** the detail panel shows "✓ Replied 14:52 by you → pr-reviews (thread)"

### Requirement: Sources that cannot send
When the source of the target message does not support replies, or is no longer in the configuration, the system SHALL NOT open the form or send, and SHALL say which source cannot send. The key `c` SHALL still copy the suggested reply.

#### Scenario: Removed source
- **WHEN** the user removed the source `work` from the configuration and presses `r` on a card with a message from `work`
- **THEN** no form opens, and the status line says that the source `work` is not in the configuration

