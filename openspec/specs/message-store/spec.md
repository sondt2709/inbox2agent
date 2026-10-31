# message-store Specification

## Purpose
This capability keeps the collected messages and the cursors in a local SQLite file. It prevents duplicate messages, records the state of each message and deletes old rows.
## Requirements
### Requirement: Local message store
The system SHALL keep collected messages, source cursors, dispatch records, agent sessions, cards, replies and asks in a SQLite file `<data>/inbox.db`. The file SHALL have the mode 0600. The store SHALL use WAL mode and a busy timeout.

#### Scenario: Store created on first use
- **WHEN** the system runs for the first time
- **THEN** `<data>/inbox.db` exists with the mode 0600 and the current schema version

### Requirement: No duplicate messages
The system SHALL keep one row for each pair of source ID and external message ID.

#### Scenario: Same message collected two times
- **WHEN** a source returns a message that the store already has
- **THEN** the store keeps one row for that message and does not reset its state

### Requirement: Atomic cursor update
The system SHALL save the messages from one poll and the new cursor of that source in one transaction.

#### Scenario: Save fails
- **WHEN** the store cannot save a message from a poll
- **THEN** the cursor of that source does not change, and the next poll collects the same messages again

### Requirement: Message states
Each message SHALL have one of these states: `pending`, `previewed`, `sent` or `expired`. A new message SHALL be `pending`. The store SHALL record a delivery for each message and space that got it, with the state `sent` (a driver other than `dry-run` reported `sent`) or `previewed` (a `dry-run` batch). The state of a message SHALL be `sent` when each space that wants it now has a `sent` delivery, `previewed` when each such space has a delivery and one of them is `previewed`, and `pending` in all other cases. A space with a `dry-run` agent SHALL take only the messages that it has no delivery for. A space with an agent of another driver SHALL also take the messages that it has a `previewed` delivery for. A `pending` or `previewed` message older than `lookback` SHALL become `expired`.

#### Scenario: Driver failure keeps messages pending
- **WHEN** the driver returns an error for a batch
- **THEN** the messages of that batch stay `pending`, and the dispatch record has the status `failed` and the error text

#### Scenario: Previewed messages go to a real driver
- **WHEN** a message is `previewed` and the agent of its space now uses the `orca` driver
- **THEN** the next dispatch sends the message, and it becomes `sent`

#### Scenario: Store from an earlier version
- **WHEN** `i2a` opens a store with schema version 1, where `dry-run` dispatches marked messages `sent`
- **THEN** the store has schema version 2, and those messages are `previewed`

#### Scenario: Old pending message expires
- **WHEN** a message is `pending` and its creation time is older than `lookback`
- **THEN** the message becomes `expired` and is not dispatched

#### Scenario: Sent to one of two spaces
- **WHEN** a message is wanted by Review and Team and only the batch of Review is sent
- **THEN** the message has a `sent` delivery for Review and stays `pending`

### Requirement: Retention
The system SHALL delete `sent` and `expired` messages, and digest files, that are older than the `retention` value. It SHALL delete `done` cards that became `done` before the `retention` value. It SHALL delete replies and asks that are older than the `retention` value. It SHALL NOT delete `new` or `read` cards. The default value SHALL be 7 days.

#### Scenario: Old sent message deleted
- **WHEN** the retention task runs and a `sent` message is 8 days old
- **THEN** the store no longer contains that message

#### Scenario: Old done card deleted
- **WHEN** the retention task runs and a card became `done` 8 days ago
- **THEN** the store no longer contains that card

#### Scenario: Old open card kept
- **WHEN** the retention task runs and a `read` card is 30 days old
- **THEN** the store still contains that card

#### Scenario: Old reply deleted
- **WHEN** the retention task runs and a reply is 8 days old
- **THEN** the store no longer contains that reply

#### Scenario: Old ask deleted
- **WHEN** the retention task runs and an ask is 8 days old
- **THEN** the store no longer contains that ask

### Requirement: Card states
Each card SHALL have one of these states: `new`, `read` or `done`. A new card SHALL be `new`. The store SHALL keep at most one card for each key.

#### Scenario: Two cards with the same key
- **WHEN** two calls add a card with the key `mm:pr-57`
- **THEN** the store has one card with that key

### Requirement: Reply states
Each reply SHALL have one of these states: `sending`, `sent` or `failed`. A reply SHALL record the target message, the card when there is one, the source, the channel name, the place, the text, who sent it (`user` or `agent`), the pending ID, and, when it is `sent`, the platform ID of the post and the send time.

#### Scenario: Reply recorded
- **WHEN** the user sends a reply from the Feed and the platform accepts it
- **THEN** the store has one reply with the state `sent`, `by` = `user` and the platform ID of the post

### Requirement: Ask states
Each ask SHALL have one of these states: `queued`, `sent` or `failed`. An ask SHALL record the card, the space, the text, the detail of the last try, the creation time, and, when it is `sent`, the terminal that got it and the send time.

#### Scenario: Ask recorded
- **WHEN** the user sends an ask from the Feed and the agent is idle
- **THEN** the store has one ask with the state `sent`, the terminal of the agent and the send time

### Requirement: Schema version 6 with a backup
When the system opens a store with schema version 2, 3 or 4, it SHALL first upgrade it to version 5 as before, with the backup `inbox.db.v4.bak`. When the system opens a store with schema version 5, it SHALL first write a consistent copy to `<data>/inbox.db.v5.bak`, then, in one transaction, rename the column `profile` to `space` in the tables `messages`, `dispatches`, `sessions` and `asks`, add the column `dispatches.agent` with the old profile name as its value, and set the version to 6. A new store SHALL get version 6 and no backup.

#### Scenario: Upgrade of an existing store
- **WHEN** `i2a` opens a store with schema version 5 that has 120 messages sent by the profile `triage`, 4 dispatches, 1 session and 2 asks
- **THEN** `inbox.db.v5.bak` exists with schema version 5, and the store has schema version 6, the same 120 messages with `space = "triage"`, 4 dispatches with `space` and `agent` both `triage`, 1 session for the space `triage`, and 2 asks

#### Scenario: Failure during the upgrade
- **WHEN** the upgrade transaction fails
- **THEN** the store stays at schema version 5, and the command reports the error and the backup path

### Requirement: Schema version 7 with a backup
When the system opens a store with schema version 6, it SHALL first write a consistent copy to `<data>/inbox.db.v6.bak`, then, in one transaction, add the column `messages.for_me` (1 for the existing messages), create the table `deliveries(message_id, space, state, dispatch_id)` with one row for each message that has a space and the state `sent` or `previewed`, add the column `cards.space` (empty for the existing cards), and set the version to 7. A store of an earlier version SHALL get the earlier upgrades first. A new store SHALL get version 7 and no backup. Retention SHALL delete the deliveries of the deleted messages.

#### Scenario: Upgrade of a version 6 store
- **WHEN** `i2a` opens a store with schema version 6 that has 3 messages sent to the space `triage` and 1 pending message
- **THEN** `inbox.db.v6.bak` exists with schema version 6, and the store has schema version 7, 3 `sent` deliveries for `triage`, and the pending message has no delivery

#### Scenario: Failure during the upgrade
- **WHEN** the upgrade transaction fails
- **THEN** the store stays at schema version 6, and the command reports the error and the backup path
