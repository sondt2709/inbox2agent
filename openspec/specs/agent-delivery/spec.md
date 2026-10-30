# agent-delivery Specification

## Purpose
This capability sends a batch of messages to an AI agent through a driver. It routes each message to a space, sends the batch of the space to its agent, and makes sure that one batch goes into one agent session one time only.
## Requirements
### Requirement: Dry-run driver
The `dry-run` driver SHALL write the digest file and return `sent`. It SHALL NOT start any process. A new agent SHALL use the `dry-run` driver by default. The messages of a dry-run batch SHALL become `previewed`, not `sent`.

#### Scenario: Dry-run dispatch
- **WHEN** a space with a `dry-run` agent sends a batch
- **THEN** the digest file exists, the messages become `previewed`, and no agent starts

#### Scenario: Change from dry-run to a real driver
- **WHEN** a `dry-run` agent previewed 75 messages of a space, and the user changes the agent driver to `orca`, and 1 new message arrives
- **THEN** the next `orca` batch of the space contains 76 messages

### Requirement: Orca new-session mode
In `new` mode, the `orca` driver SHALL run `orca agent launch` with the `cli` of the agent, `--surface terminal` and the workspace selector, and without `--prompt`. Then the driver SHALL wait until the agent TUI is idle and submit a prompt made of the primer and the batch prompt, as the requirement "Prompt submission" says.

#### Scenario: New session for each batch
- **WHEN** two batches of a space go to an agent in `new` mode
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
In `reuse` mode, the `orca` driver SHALL keep one terminal handle and the hash of the primer for each space in the store. When the handle of the space is live, the stored hash is the hash of the current primer, and the agent is idle, the driver SHALL send only the batch prompt with `orca terminal send --enter`. When no live handle exists, or the stored hash is different, the driver SHALL start a new session with the primer and the batch prompt, and store the new handle and hash for the space. The driver SHALL NOT close the old terminal. When the agent is busy, the driver SHALL return `held`, and the messages SHALL stay `pending`.

#### Scenario: Second batch in the same session
- **WHEN** two batches of one space go to an agent in `reuse` mode, and the agent is idle before the second batch
- **THEN** both batches arrive in the same terminal, and the primer arrives one time only

#### Scenario: Two spaces with one agent
- **WHEN** the spaces `acme` and `borealis` use the same agent in `reuse` mode, and each sends a batch
- **THEN** the batches arrive in two different terminals, and the store has one handle for each space

#### Scenario: Terminal closed
- **WHEN** the stored terminal of a space was closed, and a new batch of the space arrives
- **THEN** the driver starts a new session, submits the primer and the batch, and stores the new handle

#### Scenario: Primer changed
- **WHEN** the stored terminal is live, but the user changed the primer template, or the store came from schema version 2 with no hash
- **THEN** the driver starts a new session with the new primer and the batch, and stores the new handle and hash

#### Scenario: Agent busy
- **WHEN** the agent does not become idle within 5 seconds
- **THEN** the dispatch status is `held`, the messages stay `pending`, and the next send time of the space sends them

### Requirement: Orca worktree mode
In `worktree` mode, the `orca` driver SHALL create a new Orca worktree named `i2a-<batch-id>` in the workspace repository, with the `cli` of the agent and without `--prompt`. Then the driver SHALL submit a prompt made of the primer and the batch prompt, as the requirement "Prompt submission" says.

#### Scenario: Worktree for a batch
- **WHEN** a batch of a space goes to an agent in `worktree` mode
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

### Requirement: Queued asks in a dispatch
The daemon SHALL send the `queued` asks at least one time each minute, independent of the schedules, with the wait of 5 seconds of the reuse mode. A busy agent SHALL keep the ask `queued`. An Orca error SHALL make the ask `failed` with the error text.

#### Scenario: Queued ask sent
- **WHEN** an ask is `queued` because the agent was busy, the schedule of the space sends every 2 hours, and the agent is idle 1 minute later
- **THEN** the daemon sends the ask within 2 minutes, and the ask is `sent`

### Requirement: Spaces and batches
The system SHALL send each pending message to the space that lists its conversation, or to Unsorted. The route SHALL be computed when the space sends, so a moved conversation takes its pending messages to its new space. The system SHALL make one batch for each space that has pending messages at its send time, and SHALL send it to the agent of the space. The dispatch SHALL record the space and the agent. A message SHALL keep the space of the dispatch that sent it.

#### Scenario: Two spaces from one account
- **WHEN** the account `phone` has a new message in a chat of `acme` and one in a chat of `personal`, and both spaces are due
- **THEN** there is one batch of `acme` and one batch of `personal`, each to the agent of its space

### Requirement: One delivery at a time for each space
The system SHALL hold an exclusive lock on the file `<data>/space-<id>.lock` while a batch or an ask goes to the agent of the space. A batch SHALL wait up to 3 minutes for the lock; after that, the dispatch status SHALL be `held` and the messages SHALL stay `pending`. An ask SHALL wait up to 3 minutes for the lock; after that, the ask SHALL stay `queued`.

#### Scenario: Ask and batch at the same time
- **WHEN** the user sends an ask about a card of `acme` while the daemon sends a batch of `acme`
- **THEN** the agent terminal gets the two prompts one after the other, and neither prompt contains text of the other

