## MODIFIED Requirements

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

## ADDED Requirements

### Requirement: Schema version 7 with a backup
When the system opens a store with schema version 6, it SHALL first write a consistent copy to `<data>/inbox.db.v6.bak`, then, in one transaction, add the column `messages.for_me` (1 for the existing messages), create the table `deliveries(message_id, space, state, dispatch_id)` with one row for each message that has a space and the state `sent` or `previewed`, add the column `cards.space` (empty for the existing cards), and set the version to 7. A store of an earlier version SHALL get the earlier upgrades first. A new store SHALL get version 7 and no backup. Retention SHALL delete the deliveries of the deleted messages.

#### Scenario: Upgrade of a version 6 store
- **WHEN** `i2a` opens a store with schema version 6 that has 3 messages sent to the space `triage` and 1 pending message
- **THEN** `inbox.db.v6.bak` exists with schema version 6, and the store has schema version 7, 3 `sent` deliveries for `triage`, and the pending message has no delivery

#### Scenario: Failure during the upgrade
- **WHEN** the upgrade transaction fails
- **THEN** the store stays at schema version 6, and the command reports the error and the backup path
