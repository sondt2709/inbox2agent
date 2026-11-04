## MODIFIED Requirements

### Requirement: Reply form in the Feed
The key `r` on a selected card SHALL open a reply form in the detail panel. The form SHALL have these fields: Send to (the channel, the source type and the source ID; not editable), Reply to (a choice of the card refs that exist in the store; shown only when there is more than one), Where (thread or channel; "Chat" instead of "Channel" for a direct or group conversation, and "Quote" instead of "Thread" for a WhatsApp target), a note about the read state, and Text (the suggested reply of the card, or empty). `ctrl+s` SHALL send the reply. `esc` SHALL close the form and send nothing. When the card has no ref that exists in the store, `r` SHALL show "The card has no message to reply to" and open no form.

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

#### Scenario: WhatsApp card
- **WHEN** the user presses `r` on a card with a ref from a WhatsApp group
- **THEN** the form opens, and Where shows "Quote" and "Chat" with "Quote" selected

### Requirement: Reply target and place
The target of a reply SHALL be a message in the store. The system SHALL NOT take a channel, a user or a URL from card text, from the reply text or from a command-line value other than a message ID or a card ID or key. The default target SHALL be the newest card ref that exists in the store. The default place SHALL be the thread of the message when the message is in a thread or in a channel of kind `channel`, and the channel when the message is a top-level message of a direct or group conversation. For a WhatsApp target, the default place SHALL be the channel (a plain message) in a direct chat and the thread (a quote of the target) in a group. A reply in a thread SHALL use the root of the thread, also when the target message is a reply in that thread.

#### Scenario: Direct message
- **WHEN** the target is a top-level message from `@lena.wong` in a direct conversation
- **THEN** the default place is the channel, and the form shows "Chat"

#### Scenario: Reply to a reply
- **WHEN** the target message is a reply in the thread of message A, and the place is thread
- **THEN** the new post is in the thread of message A

#### Scenario: URL in the card body
- **WHEN** the card body contains `https://chat.example.com/team/pl/abc123`
- **THEN** the reply goes to the card ref message, not to the post in the URL

#### Scenario: WhatsApp group
- **WHEN** the target is a message in a WhatsApp group and the agent runs `i2a reply --to <id>` without `--thread` or `--channel`
- **THEN** the reply is a quote of the target message in that group

### Requirement: Read state of a reply
The system SHALL NOT send a request that marks a channel as read or as unread. For WhatsApp, the system SHALL NOT send read receipts, typing state or presence. The read state SHALL change only as the platform changes it for a post of the user. For a Mattermost target, the form SHALL tell the user that a thread reply keeps the read state and that a post in the channel or chat marks it read. For a WhatsApp target, the form SHALL tell the user that i2a sends no read receipt.

#### Scenario: Reply in a thread
- **WHEN** the user has unread messages in `pr-reviews` and sends a reply in a thread there
- **THEN** the `last_viewed_at` of the user in `pr-reviews` is the same before and after the send

#### Scenario: Post in the chat
- **WHEN** the user sends a reply in the chat of `@lena.wong`
- **THEN** Mattermost marks the chat read, and i2a sent only `GET /posts/{id}` and `POST /posts`

#### Scenario: WhatsApp reply
- **WHEN** the user sends a reply to a WhatsApp direct message of a contact
- **THEN** the contact gets the message, and the earlier messages of the contact do not get blue ticks

### Requirement: Sources that cannot send
When the source of the target message does not support replies, or is no longer in the configuration, the system SHALL NOT open the form or send, and SHALL say which source cannot send. Mattermost and WhatsApp sources SHALL support replies. The key `c` SHALL still copy the suggested reply.

#### Scenario: Removed source
- **WHEN** the user removed the source `work` from the configuration and presses `r` on a card with a message from `work`
- **THEN** no form opens, and the status line says that the source `work` is not in the configuration

#### Scenario: WhatsApp source
- **WHEN** the agent runs `i2a reply --to <id>` for a message of a paired WhatsApp source
- **THEN** the command does not say "cannot send replies yet"

## ADDED Requirements

### Requirement: WhatsApp reply delivery
A WhatsApp reply SHALL use the connection of the daemon when the daemon answers on its socket. When no daemon answers, the process SHALL open the connection of the source, send, and close it. A quote SHALL contain the ID, the sender and the stored content of the target message. When the stored row of the target message no longer exists, the system SHALL refuse a quote, say to use the chat place, and send nothing. The WhatsApp message ID of a reply SHALL come from its pending ID, so each try of one reply has the same message ID. A new send of a reply that is `sending` SHALL send again with the same message ID.

#### Scenario: Daemon runs
- **WHEN** the daemon holds the WhatsApp connection and the user sends a reply from the Feed
- **THEN** the daemon sends the message, the reply is `sent`, and the Feed process does not open the connection

#### Scenario: No daemon
- **WHEN** no daemon runs and the agent runs `i2a reply --to <id>` for a WhatsApp message
- **THEN** the command connects, sends, disconnects, and releases `<data>/whatsapp-<id>.lock`

#### Scenario: Quote of an old message
- **WHEN** the stored row of the target message was deleted and the user sends with Where = Quote
- **THEN** nothing is sent, and the form says that the message is too old to quote and to use Chat

#### Scenario: Retry after a time-out
- **WHEN** the first send of a reply times out, and the user sends the same reply again
- **THEN** both tries use the same WhatsApp message ID, and the contact sees one message
