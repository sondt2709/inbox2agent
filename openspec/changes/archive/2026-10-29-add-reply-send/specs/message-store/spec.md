## MODIFIED Requirements

### Requirement: Local message store
The system SHALL keep collected messages, source cursors, dispatch records, agent sessions, cards and replies in a SQLite file `<data>/inbox.db`. The file SHALL have the mode 0600. The store SHALL use WAL mode and a busy timeout.

#### Scenario: Store created on first use
- **WHEN** the system runs for the first time
- **THEN** `<data>/inbox.db` exists with the mode 0600 and the current schema version

### Requirement: Retention
The system SHALL delete `sent` and `expired` messages, and digest files, that are older than the `retention` value. It SHALL delete `done` cards that became `done` before the `retention` value. It SHALL delete replies that are older than the `retention` value. It SHALL NOT delete `new` or `read` cards. The default value SHALL be 7 days.

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

## REMOVED Requirements

### Requirement: Schema version 3 with a backup
**Reason**: Replaced by "Schema version 4 with a backup". The upgrade from version 2 still runs before the upgrade to version 4.
**Migration**: None. `i2a` upgrades a version 2 or 3 store automatically.

## ADDED Requirements

### Requirement: Schema version 4 with a backup
When the system opens a store with schema version 2, it SHALL first upgrade it to version 3 with the backup `<data>/inbox.db.v2.bak`. When the system opens a store with schema version 3, it SHALL first write a consistent copy to `<data>/inbox.db.v3.bak`, then add the replies table and the thread ID of the messages, and set the version to 4 in one transaction. A new store SHALL get version 4 and no backup.

#### Scenario: Upgrade of an existing store
- **WHEN** `i2a` opens a store with schema version 3 that has 120 messages and 5 cards
- **THEN** `inbox.db.v3.bak` exists with schema version 3, and the store has schema version 4, the same 120 messages and the same 5 cards

### Requirement: Reply states
Each reply SHALL have one of these states: `sending`, `sent` or `failed`. A reply SHALL record the target message, the card when there is one, the source, the channel name, the place, the text, who sent it (`user` or `agent`), the pending ID, and, when it is `sent`, the platform ID of the post and the send time.

#### Scenario: Reply recorded
- **WHEN** the user sends a reply from the Feed and the platform accepts it
- **THEN** the store has one reply with the state `sent`, `by` = `user` and the platform ID of the post
