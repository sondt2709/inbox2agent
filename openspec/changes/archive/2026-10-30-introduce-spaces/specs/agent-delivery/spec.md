## MODIFIED Requirements

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

### Requirement: Queued asks in a dispatch
The daemon SHALL send the `queued` asks at least one time each minute, independent of the schedules, with the wait of 5 seconds of the reuse mode. A busy agent SHALL keep the ask `queued`. An Orca error SHALL make the ask `failed` with the error text.

#### Scenario: Queued ask sent
- **WHEN** an ask is `queued` because the agent was busy, the schedule of the space sends every 2 hours, and the agent is idle 1 minute later
- **THEN** the daemon sends the ask within 2 minutes, and the ask is `sent`

## REMOVED Requirements

### Requirement: Profiles and routes
**Reason**: Profiles and per-source routes are replaced by spaces.
**Migration**: See "Spaces and batches" below. The version 1 migration makes one space for each used profile.

### Requirement: One delivery at a time for each profile
**Reason**: A terminal now belongs to one space, so the lock is for each space.
**Migration**: See "One delivery at a time for each space" below.

## ADDED Requirements

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
