## ADDED Requirements

### Requirement: Schema version 9 with a backup
When the system opens a store with schema version 8, it SHALL first write a consistent copy to `<data>/inbox.db.v8.bak`, then, in one transaction, add the columns `messages.reply_ext`, `messages.reply_author`, `messages.reply_me` and `messages.reply_text` (empty or false for the existing rows), and set the version to 9. A store of an earlier version SHALL get the earlier upgrades first. A new store SHALL get version 9 and no backup.

#### Scenario: Upgrade of a version 8 store
- **WHEN** `i2a` opens a store with schema version 8 that has 10 messages
- **THEN** `inbox.db.v8.bak` exists with schema version 8, and the store has schema version 9 and the 10 messages with no reply reference

#### Scenario: Failure during the upgrade
- **WHEN** the upgrade transaction fails
- **THEN** the store stays at schema version 8, and the command reports the error and the backup path

### Requirement: Reply reference of a message
The store SHALL keep the reply reference of each message that a source gives: the external ID of the earlier message, its author, whether the user wrote it, and its text. When the store reads a message, it SHALL give the row ID of the earlier message when a message of the same account has that external ID, else 0.

#### Scenario: Earlier message in the store
- **WHEN** the store has message 812 with the external ID `abc`, and message 900 of the same account has the reply reference `abc`
- **THEN** `Pending` returns message 900 with the reply row ID 812

#### Scenario: Earlier message deleted by the retention
- **WHEN** the retention deletes message 812, and message 900 is still pending
- **THEN** `Pending` returns message 900 with the reply row ID 0 and keeps the author and the text of the reference
