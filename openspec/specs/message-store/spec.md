# message-store Specification

## Purpose
This capability keeps the collected messages and the cursors in a local SQLite file. It prevents duplicate messages, records the state of each message and deletes old rows.

## Requirements
### Requirement: Local message store
The system SHALL keep collected messages, source cursors, dispatch records and agent sessions in a SQLite file `<data>/inbox.db`. The file SHALL have the mode 0600. The store SHALL use WAL mode and a busy timeout.

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
Each message SHALL have one of these states: `pending`, `previewed`, `sent` or `expired`. A new message SHALL be `pending`. A message SHALL become `sent` only after a driver other than `dry-run` reports `sent` for its batch. A message of a `dry-run` batch SHALL become `previewed`. A `dry-run` profile SHALL take only `pending` messages. A profile with another driver SHALL take `pending` and `previewed` messages. A `pending` or `previewed` message older than `lookback` SHALL become `expired`.

#### Scenario: Driver failure keeps messages pending
- **WHEN** the driver returns an error for a batch
- **THEN** the messages of that batch stay `pending`, and the dispatch record has the status `failed` and the error text

#### Scenario: Previewed messages go to a real driver
- **WHEN** a message is `previewed` and its profile now uses the `orca` driver
- **THEN** the next dispatch sends the message, and it becomes `sent`

#### Scenario: Store from an earlier version
- **WHEN** `i2a` opens a store with schema version 1, where `dry-run` dispatches marked messages `sent`
- **THEN** the store has schema version 2, and those messages are `previewed`

#### Scenario: Old pending message expires
- **WHEN** a message is `pending` and its creation time is older than `lookback`
- **THEN** the message becomes `expired` and is not dispatched

### Requirement: Retention
The system SHALL delete `sent` and `expired` messages, and digest files, that are older than the `retention` value. The default value SHALL be 7 days.

#### Scenario: Old sent message deleted
- **WHEN** the retention task runs and a `sent` message is 8 days old
- **THEN** the store no longer contains that message

