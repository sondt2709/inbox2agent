# spaces Specification

## Purpose
TBD - created by archiving change introduce-spaces. Update Purpose after archive.
## Requirements
### Requirement: Accounts
An account SHALL be one login to one platform (`mattermost` or `whatsapp`), with a stable ID, a name that the user can change, the platform, the server URL for Mattermost, and `read_every` (at least 5 seconds). An account SHALL NOT name a space, an agent, a schedule or a conversation.

#### Scenario: Rename an account
- **WHEN** the user changes the name of the account `phone` to "Personal phone"
- **THEN** the TUI and `i2a status` show "Personal phone", and the store, the secrets and `whatsapp-phone.db` still use the ID `phone`

#### Scenario: Rename a conversation
- **WHEN** the user gives the Mattermost channel `town-square` the label "Company news", and the TUI lists the channels of the account again
- **THEN** the Conversations tab shows "Company news"

#### Scenario: Account with a route key
- **WHEN** an account entry has the key `profile` or `space`
- **THEN** the configuration validation reports an unknown field

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

### Requirement: Spaces
A space SHALL have a stable ID, a name, one agent, one schedule and a list of conversations from any account. The ID SHALL be 1 to 32 lowercase letters, digits, `-` or `_`. The name SHALL be free text and SHALL be unique. A space SHALL be the only item that names an agent or a schedule.

#### Scenario: Space with conversations of two accounts
- **WHEN** the space `acme` lists `acme`/`<release channel>` and `phone`/`<group chat>`
- **THEN** messages from both conversations go to the agent of `acme` in one batch

#### Scenario: One account in many spaces
- **WHEN** the account `phone` has chats in the spaces `acme`, `atlas` and `personal`
- **THEN** each chat sends to the agent of its own space on the schedule of its own space

### Requirement: Unsorted space
The configuration SHALL have one built-in space `unsorted` with an agent or no agent (hold), a schedule, a read level for the conversations that no space lists (`unsorted.read`), and an optional list of conversations with their own read levels. When Unsorted has no agent, its messages SHALL stay `pending` until they expire after `lookback`, or until the user moves their conversation to another space.

#### Scenario: Hold
- **WHEN** Unsorted has no agent and a message arrives in an unsorted conversation
- **THEN** no dispatch sends it, and it stays `pending`

#### Scenario: Sort a conversation with waiting messages
- **WHEN** Unsorted holds 3 pending messages of a chat, and the user moves the chat to the space `acme`
- **THEN** the next dispatch of `acme` sends the 3 messages

### Requirement: Agents
An agent SHALL have a stable ID and a name that the user can change. It SHALL describe how to send a batch: `driver`, `cli`, `workspace`, `session`, `payload`, `inline_max_bytes`, `message_max_chars`, `model`, `orca_bin`, `primer` and `batch`. An agent SHALL NOT name a space or an account. Many spaces SHALL be able to use one agent.

#### Scenario: Two spaces with one agent
- **WHEN** the spaces `acme` and `borealis` use the agent `claude-work` in `reuse` mode
- **THEN** each space has its own session, and no prompt of `acme` goes to the session of `borealis`

### Requirement: Schedules
A schedule SHALL have a stable ID, a name that the user can change, `every` (at least 1 minute), `days` (empty means every day) and `hours` (`HH:MM-HH:MM` in local time, empty means all day, a range can cross midnight). The send times SHALL be the start of the hours (or 00:00) plus a multiple of `every`, inside the hours, on an allowed day. Outside the send times, the sources SHALL still collect, and the messages SHALL wait.

#### Scenario: Next send time
- **WHEN** a schedule has `every = "15m"`, `days = ["mon","tue","wed","thu","fri"]`, `hours = "08:00-18:00"`, and the local time is Thursday 14:37
- **THEN** the next send time is Thursday 14:45

#### Scenario: After the hours
- **WHEN** the same schedule is checked on Friday at 18:05
- **THEN** the next send time is Monday 08:00

### Requirement: References between items
Each space SHALL name an agent and a schedule that exist. Unsorted SHALL name a schedule that exists and an agent that exists or no agent. Each conversation entry SHALL name an account that exists. The TUI SHALL NOT delete an agent or a schedule that a space uses until the user chooses a replacement; it SHALL then set the replacement in each of these spaces.

#### Scenario: Delete a used agent
- **WHEN** the user deletes the agent `claude-work` that `acme` and `borealis` use, and chooses `claude-atlas` as the replacement
- **THEN** the file has no agent `claude-work`, and `acme` and `borealis` use `claude-atlas`

#### Scenario: Unknown agent
- **WHEN** a space has `agent = "missing"`
- **THEN** the configuration validation reports that the agent `missing` does not exist

### Requirement: Migration from version 1
When the configuration file has no `version` or has `[[sources]]`, `[[profiles]]` or `[timing]`, the system SHALL convert it to version 2 as the design (D9) says, write the old file to `config.toml.v1.bak` with mode 0600, save the new file, and report each change in behavior. The command that loaded the file SHALL print the report to stderr; the daemon SHALL log it.

#### Scenario: Profile and channel routes
- **WHEN** a version 1 file has the source `work` with `profile = "triage"`, `channels = ["C1","C2"]` and `channel_profiles = {C2 = "reviewer"}`
- **THEN** the version 2 file has the agents `triage` and `reviewer`, the space `triage` with `work`/`C1` (read `all`), and the space `reviewer` with `work`/`C2` (read `all`)

#### Scenario: Timing
- **WHEN** a version 1 file has `interval = "15m"` and `active_hours = "08:00-20:00"`
- **THEN** the version 2 file has the schedule `default` with `every = "15m"` and `hours = "08:00-20:00"`, and each migrated space uses it

#### Scenario: Backup and report
- **WHEN** `i2a status` loads a version 1 file
- **THEN** `config.toml.v1.bak` has the old text, `config.toml` has `version = 2`, and stderr shows the migration report

#### Scenario: Template field
- **WHEN** a version 1 profile has the batch template `{{.Count}} from {{.Sources}}`
- **THEN** the migrated agent has the batch template `{{.Count}} from {{.Accounts}}`

