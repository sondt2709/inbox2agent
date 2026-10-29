# agent-delivery Specification

## Purpose
This capability sends a batch of messages to an AI agent through a driver. It routes each message to a profile, and it makes sure that one batch goes into one agent session one time only.
## Requirements
### Requirement: Profiles and routes
The system SHALL send each message to one profile. The profile SHALL come from `channel_profiles` of its source when the channel has an entry, or else from the `profile` of its source, or else from the first profile in the configuration. The system SHALL make one batch for each profile that has pending messages.

#### Scenario: Channel route
- **WHEN** channel `C1` maps to profile `reviewer` and the source profile is `triage`
- **THEN** messages from `C1` go to the `reviewer` batch, and messages from other channels go to the `triage` batch

### Requirement: Dry-run driver
The `dry-run` driver SHALL write the digest file and return `sent`. It SHALL NOT start any process. A new profile SHALL use the `dry-run` driver by default. The messages of a dry-run batch SHALL become `previewed`, not `sent`.

#### Scenario: Dry-run dispatch
- **WHEN** a `dry-run` profile receives a batch
- **THEN** the digest file exists, the messages become `previewed`, and no agent starts

#### Scenario: Change from dry-run to a real driver
- **WHEN** a `dry-run` profile previewed 75 messages, and the user changes the profile driver to `orca`, and 1 new message arrives
- **THEN** the next `orca` batch contains 76 messages

### Requirement: Orca new-session mode
In `new` mode, the `orca` driver SHALL run `orca agent launch` with the profile agent, `--surface terminal` and the workspace selector, and without `--prompt`. Then the driver SHALL wait until the agent TUI is idle and submit a prompt made of the primer and the batch prompt, as the requirement "Prompt submission" says.

#### Scenario: New session for each batch
- **WHEN** two batches go to a profile in `new` mode
- **THEN** Orca shows two agent terminals, and each agent started a turn with the primer and one batch prompt

### Requirement: Prompt submission
The `orca` driver SHALL submit a prompt with `orca terminal send --text <prompt> --enter --wait-submit 30`. Before the first prompt to a new terminal, the driver SHALL wait up to 90 seconds for `tui-idle`. When the send result does not include the stage `turn_started`, the driver SHALL send one bare Enter to the terminal and record the detail "turn not proven, Enter sent again". The driver SHALL NOT send the prompt text again.

#### Scenario: Prompt left in the input box
- **WHEN** `orca terminal send` returns `accepted: true` and the stages are only `input_accepted`
- **THEN** the driver sends one `orca terminal send --enter` with no text, and the dispatch status is `sent`

#### Scenario: Long multi-line prompt
- **WHEN** a new session gets a primer of 15 lines with non-ASCII text and an inline batch
- **THEN** the send result includes `turn_started`, and the input box of the agent is empty

### Requirement: Orca reuse-session mode
In `reuse` mode, the `orca` driver SHALL keep the terminal handle of the profile and the hash of the primer in the store. When the handle is live, the stored hash is the hash of the current primer, and the agent is idle, the driver SHALL send only the batch prompt with `orca terminal send --enter`. When no live handle exists, or the stored hash is different, the driver SHALL start a new session with the primer and the batch prompt, and store the new handle and hash. The driver SHALL NOT close the old terminal. When the agent is busy, the driver SHALL return `held`, and the messages SHALL stay `pending`.

#### Scenario: Second batch in the same session
- **WHEN** two batches go to a profile in `reuse` mode, and the agent is idle before the second batch
- **THEN** both batches arrive in the same terminal, and the primer arrives one time only

#### Scenario: Terminal closed
- **WHEN** the stored terminal of a profile was closed, and a new batch arrives
- **THEN** the driver starts a new session, submits the primer and the batch, and stores the new handle

#### Scenario: Primer changed
- **WHEN** the stored terminal is live, but the user changed the primer template, or the store came from schema version 2 with no hash
- **THEN** the driver starts a new session with the new primer and the batch, and stores the new handle and hash

#### Scenario: Agent busy
- **WHEN** the agent does not become idle within 5 seconds
- **THEN** the dispatch status is `held`, the messages stay `pending`, and the next tick sends them

### Requirement: Orca worktree mode
In `worktree` mode, the `orca` driver SHALL create a new Orca worktree named `i2a-<batch-id>` in the workspace repository, with the profile agent and without `--prompt`. Then the driver SHALL submit a prompt made of the primer and the batch prompt, as the requirement "Prompt submission" says.

#### Scenario: Worktree for a batch
- **WHEN** a batch goes to a profile in `worktree` mode
- **THEN** Orca lists a worktree named `i2a-<batch-id>` with an agent terminal that received the prompt

### Requirement: Orca failures
When the `orca` executable is missing, the Orca runtime is not reachable, or the JSON result reports `ok: false`, the driver SHALL return an error with the Orca error text. The messages SHALL stay `pending`.

#### Scenario: Orca not reachable
- **WHEN** the `orca` executable exits with a non-zero code
- **THEN** the dispatch status is `failed` with the Orca error text, and the messages stay `pending`

### Requirement: No duplicate send into one session
The `orca` driver SHALL NOT send the same batch text two times to one terminal.

#### Scenario: Unproven submission
- **WHEN** `orca terminal send` returns `accepted: true` and the stages do not include `turn_started`
- **THEN** the driver does not send the text again, and the dispatch status is `sent`

### Requirement: One delivery at a time for each profile
The system SHALL hold an exclusive lock on the file `<data>/agent-<profile>.lock` while a batch or an ask goes to the agent of the profile. A batch SHALL wait up to 3 minutes for the lock; after that, the dispatch status SHALL be `held` and the messages SHALL stay `pending`. An ask SHALL wait up to 3 minutes for the lock; after that, the ask SHALL stay `queued`.

#### Scenario: Ask and batch at the same time
- **WHEN** the user sends an ask while the daemon dispatches a batch to the same profile
- **THEN** the agent terminal gets the two prompts one after the other, and neither prompt contains text of the other

### Requirement: Queued asks in a dispatch
Each dispatch SHALL send the `queued` asks before the batches, with the wait of 5 seconds of the reuse mode. A busy agent SHALL keep the ask `queued`. An Orca error SHALL make the ask `failed` with the error text.

#### Scenario: Queued ask sent
- **WHEN** an ask is `queued` because the agent was busy, and the agent is idle at the next dispatch
- **THEN** the dispatch sends the ask first, and the ask is `sent`

