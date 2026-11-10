## ADDED Requirements

### Requirement: Agent of a space
Each space and Unsorted SHALL have its own agent in the file (`[spaces.agent]`, `[unsorted.agent]`) with `driver`, `cli`, `folder`, `session`, `payload`, `inline_max_bytes`, `message_max_chars`, `model` and `orca_bin`, and its own `instruction`. A change of the agent of one space SHALL NOT change another space. The agent SHALL NOT have an ID or a name in the file; the system SHALL describe it as `dry-run`, or `orca · <cli>`.

#### Scenario: Two spaces with the same settings
- **WHEN** the spaces `acme` and `borealis` both have an `orca` agent with `cli = "claude"` in `reuse` mode, and the user changes the folder of `acme`
- **THEN** the folder of `borealis` does not change, and each space has its own session

#### Scenario: Description
- **WHEN** a space has an `orca` agent with `cli = "claude"`
- **THEN** `i2a status` and the dashboard describe its agent as `orca · claude`

### Requirement: Paused space
A space and Unsorted SHALL have `paused` (default false). A paused space SHALL NOT send a batch and SHALL NOT read asks. Its messages SHALL stay `pending` and SHALL expire after `lookback`. When the user sets `paused = false` again, the next send time of the space SHALL send its pending messages.

#### Scenario: Pause a space
- **WHEN** the space `iso` is paused, a message arrives in its conversation, and its send time comes
- **THEN** no dispatch of `iso` exists, and the message is `pending`

#### Scenario: Resume a space
- **WHEN** the user sets `paused = false` for `iso` within `lookback`
- **THEN** the next send time of `iso` sends the waiting message

### Requirement: Migration from version 2
When the configuration file has `version = 2`, the system SHALL convert it to version 3, write the old file to `config.toml.v2.bak` with mode 0600, save the new file, and report each change in behavior. Each space SHALL get a copy of its agent, with `workspace` as `folder` and `primer` as `instruction`. Unsorted with `agent = ''` SHALL become paused, with a copy of the agent of the first space (or the default agent). An agent that two spaces used SHALL be copied into each, and the report SHALL name the spaces. An agent that no space used SHALL be removed, and the report SHALL say that the backup has it. For a space with a `dry-run` agent, the report SHALL say how to pause the space instead. The command that loaded the file SHALL print the report to stderr; the daemon SHALL log it.

#### Scenario: One agent for each space
- **WHEN** a version 2 file has the agents `triage` and `iso` with their primers, the spaces `triage` and `iso` that use them, and Unsorted with `agent = ''`
- **THEN** the version 3 file has `version = 3`, no `[[agents]]`, the space `triage` with the agent and the instruction of `triage`, the space `iso` with those of `iso`, Unsorted with `paused = true`, and `config.toml.v2.bak` has the old text

#### Scenario: Shared agent
- **WHEN** the spaces `acme` and `borealis` use the agent `claude-work`
- **THEN** both spaces have a copy of `claude-work`, and the report says `Agent "claude-work": copied into the spaces acme and borealis.`

#### Scenario: Unused agent
- **WHEN** no space uses the agent `old`
- **THEN** the version 3 file has no copy of `old`, and the report says that `config.toml.v2.bak` has its settings

## MODIFIED Requirements

### Requirement: Spaces
A space SHALL have a stable ID, a name, an agent, an instruction, a schedule, `paused`, and a list of conversations from any account. The ID SHALL be 1 to 32 lowercase letters, digits, `-` or `_`. The name SHALL be free text and SHALL be unique. A space SHALL be the only item that names a schedule.

#### Scenario: Space with conversations of two accounts
- **WHEN** the space `acme` lists `acme`/`<release channel>` and `phone`/`<group chat>`
- **THEN** messages from both conversations go to the agent of `acme` in one batch

#### Scenario: One account in many spaces
- **WHEN** the account `phone` has chats in the spaces `acme`, `atlas` and `personal`
- **THEN** each chat sends to the agent of its own space on the schedule of its own space

### Requirement: Unsorted space
The configuration SHALL have one built-in space `unsorted` with an agent, an instruction, `paused`, a schedule, a read level for the conversations that no space lists (`unsorted.read`), and an optional list of conversations with their own read levels. When Unsorted is paused (hold), its messages SHALL stay `pending` until they expire after `lookback`, or until the user moves their conversation to another space.

#### Scenario: Hold
- **WHEN** Unsorted is paused and a message arrives in an unsorted conversation
- **THEN** no dispatch sends it, and it stays `pending`

#### Scenario: Sort a conversation with waiting messages
- **WHEN** Unsorted holds 3 pending messages of a chat, and the user moves the chat to the space `acme`
- **THEN** the next dispatch of `acme` sends the 3 messages

### Requirement: References between items
Each space and Unsorted SHALL name a schedule that exists. Each conversation entry SHALL name an account that exists. The TUI SHALL NOT delete a schedule that a space uses until the user chooses a replacement; it SHALL then set the replacement in each of these spaces.

#### Scenario: Delete a used schedule
- **WHEN** the user deletes the schedule `work-hours` that `acme` and `borealis` use, and chooses `always` as the replacement
- **THEN** the file has no schedule `work-hours`, and `acme` and `borealis` use `always`

#### Scenario: Unknown schedule
- **WHEN** a space has `schedule = "missing"`
- **THEN** the configuration validation reports that the schedule `missing` does not exist

### Requirement: Migration from version 1
When the configuration file has no `version` or has `[[sources]]`, `[[profiles]]` or `[timing]`, the system SHALL convert it to version 2 as the design (D9 of introduce-spaces) says, then to version 3 as the requirement "Migration from version 2" says, write the old file to `config.toml.v1.bak` with mode 0600, save the new file, and report each change in behavior. The command that loaded the file SHALL print the report to stderr; the daemon SHALL log it.

#### Scenario: Profile and channel routes
- **WHEN** a version 1 file has the source `work` with `profile = "triage"`, `channels = ["C1","C2"]` and `channel_profiles = {C2 = "reviewer"}`
- **THEN** the version 3 file has the space `triage` with `work`/`C1` (read `all`) and the space `reviewer` with `work`/`C2` (read `all`), each with the agent of its profile

#### Scenario: Timing
- **WHEN** a version 1 file has `interval = "15m"` and `active_hours = "08:00-20:00"`
- **THEN** the version 3 file has the schedule `default` with `every = "15m"` and `hours = "08:00-20:00"`, and each migrated space uses it

#### Scenario: Backup and report
- **WHEN** `i2a status` loads a version 1 file
- **THEN** `config.toml.v1.bak` has the old text, `config.toml` has `version = 3`, and stderr shows the migration report

#### Scenario: Batch template
- **WHEN** a version 1 profile has a batch template
- **THEN** the migrated space has no batch template, and the report says that the batch template is not used

## REMOVED Requirements

### Requirement: Agents
**Reason**: Each space owns its agent. In real use, each agent served one space.
**Migration**: See the requirements "Agent of a space" and "Migration from version 2". A shared agent is copied into each space.
