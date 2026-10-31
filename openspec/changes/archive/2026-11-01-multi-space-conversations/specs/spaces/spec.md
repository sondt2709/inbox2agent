## MODIFIED Requirements

### Requirement: Conversations
A conversation SHALL be one direct conversation, group or channel of one account, identified by the account ID and the platform ID (Mattermost channel ID, WhatsApp chat JID). A conversation SHALL be in one space or more. Each space that lists a conversation SHALL have one link to it, with its own read level and an optional label. A conversation that no space lists SHALL be in the Unsorted space. A conversation SHALL NOT be two times in one space, and SHALL NOT be in Unsorted and in a space. A direct conversation, a group and a channel SHALL have the same settings: the spaces, the read level of each link and an optional label. The first label of the links SHALL replace the platform name in the TUI, `i2a status` and `i2a conversations`, and a refresh of the platform name SHALL NOT change it.

#### Scenario: Conversation in two spaces
- **WHEN** the spaces `review` and `team` both list the conversation `work`/`C1`
- **THEN** the configuration is valid, and the conversation has two links

#### Scenario: Two times in one space
- **WHEN** the space `review` lists the conversation `work`/`C1` two times
- **THEN** the configuration validation reports that the conversation is two times in the space `review`

#### Scenario: In Unsorted and in a space
- **WHEN** `[unsorted]` and the space `review` both list the conversation `work`/`C1`
- **THEN** the configuration validation reports that the conversation is in Unsorted and in the space `review`

#### Scenario: New conversation
- **WHEN** a contact starts a new direct conversation that no space lists
- **THEN** the conversation is in Unsorted, with the read level of `unsorted.read`

### Requirement: Read levels
Each link SHALL have the read level `all`, `mentions` or `off`. A message SHALL be for the user when it is in a direct conversation, or mentions the user, or replies to a message of the user. A link with `all` SHALL get each message of the conversation. A link with `mentions` SHALL get only the messages for the user. A link with `off` SHALL get no message. The source SHALL read each conversation at the widest level of its links (`all`, then `mentions`, then `off`), and SHALL mark each message that is for the user. A change of the read level SHALL apply to the messages that arrive after the change, and to the waiting messages, because each space filters at send time.

#### Scenario: Mentions in a direct conversation
- **WHEN** a direct conversation has the read level `mentions` and the other person writes "are you free at 3?"
- **THEN** the source collects the message

#### Scenario: Mentions in a channel
- **WHEN** a channel has the read level `mentions` and a member posts with no mention and no reply to the user
- **THEN** the source does not collect the post

#### Scenario: Off
- **WHEN** a group has the read level `off` and a member mentions the user
- **THEN** the source does not collect the message

#### Scenario: Two links with different levels
- **WHEN** the channel `release` has the level `all` in Review and `mentions` in Team, and a member posts "notes are ready" and later "@alice please check"
- **THEN** the source collects both posts, Review gets both, and Team gets only "@alice please check"
