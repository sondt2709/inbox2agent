## REMOVED Requirements

### Requirement: Collect messages from selected chats
**Reason**: The read level of each conversation replaces `channels`, `all_direct` and `mentions`.
**Migration**: See "Collect messages by read level" below. The version 1 migration turns `channels` into conversations with the read level `all`, and `all_direct` or `mentions` into `unsorted.read = "mentions"`.

## ADDED Requirements

### Requirement: Collect messages by read level
The source SHALL use, for each chat, the read level of the conversation in the configuration, or `unsorted.read` when no space lists the chat. It SHALL collect: each message for `all`; for `mentions`, each message of a direct chat and, in a group, only messages that mention the user or quote a message of the user; no message for `off`. The source SHALL skip messages of the user, status updates, broadcast lists, WhatsApp channels (newsletters), reactions, edits, deletes and poll votes.

#### Scenario: Direct chat with read level mentions
- **WHEN** a direct chat has the read level `mentions` and the contact sends a message
- **THEN** the source collects it with the kind `direct`

#### Scenario: Group with read level all
- **WHEN** a member posts in a group with the read level `all`
- **THEN** the source collects the message with the kind `group`

#### Scenario: Unsorted group without a mention
- **WHEN** `unsorted.read = "mentions"` and a member posts in a group that no space lists, with no mention and no quote of the user
- **THEN** the source does not collect the message

#### Scenario: Reply to the user in an unsorted group
- **WHEN** `unsorted.read = "mentions"` and a member replies to a message of the user in a group that no space lists
- **THEN** the source collects the message

#### Scenario: Group off
- **WHEN** a group has the read level `off` and a member mentions the user
- **THEN** the source does not collect the message
