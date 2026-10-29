## MODIFIED Requirements

### Requirement: Local message store
The system SHALL keep collected messages, source cursors, dispatch records, agent sessions and cards in a SQLite file `<data>/inbox.db`. The file SHALL have the mode 0600. The store SHALL use WAL mode and a busy timeout.

#### Scenario: Store created on first use
- **WHEN** the system runs for the first time
- **THEN** `<data>/inbox.db` exists with the mode 0600 and the current schema version

### Requirement: Retention
The system SHALL delete `sent` and `expired` messages, and digest files, that are older than the `retention` value. It SHALL delete `done` cards that became `done` before the `retention` value. It SHALL NOT delete `new` or `read` cards. The default value SHALL be 7 days.

#### Scenario: Old sent message deleted
- **WHEN** the retention task runs and a `sent` message is 8 days old
- **THEN** the store no longer contains that message

#### Scenario: Old done card deleted
- **WHEN** the retention task runs and a card became `done` 8 days ago
- **THEN** the store no longer contains that card

#### Scenario: Old open card kept
- **WHEN** the retention task runs and a `read` card is 30 days old
- **THEN** the store still contains that card

## ADDED Requirements

### Requirement: Card states
Each card SHALL have one of these states: `new`, `read` or `done`. A new card SHALL be `new`. The store SHALL keep at most one card for each key.

#### Scenario: Two cards with the same key
- **WHEN** two calls add a card with the key `mm:pr-57`
- **THEN** the store has one card with that key

### Requirement: Schema version 3 with a backup
When the system opens a store with schema version 2, it SHALL first write a consistent copy to `<data>/inbox.db.v2.bak`, then add the cards table and the primer hash of the sessions, and set the version to 3 in one transaction. A new store SHALL get version 3 and no backup.

#### Scenario: Upgrade of an existing store
- **WHEN** `i2a` opens a store with schema version 2 that has 120 messages
- **THEN** `inbox.db.v2.bak` exists with schema version 2, and the store has schema version 3 and the same 120 messages
