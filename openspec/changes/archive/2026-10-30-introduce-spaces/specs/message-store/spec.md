## REMOVED Requirements

### Requirement: Schema version 5 with a backup
**Reason**: Schema version 6 continues the same upgrade chain.
**Migration**: See "Schema version 6 with a backup" below. A store with version 2 to 4 still upgrades to version 5 first.

## ADDED Requirements

### Requirement: Schema version 6 with a backup
When the system opens a store with schema version 2, 3 or 4, it SHALL first upgrade it to version 5 as before, with the backup `inbox.db.v4.bak`. When the system opens a store with schema version 5, it SHALL first write a consistent copy to `<data>/inbox.db.v5.bak`, then, in one transaction, rename the column `profile` to `space` in the tables `messages`, `dispatches`, `sessions` and `asks`, add the column `dispatches.agent` with the old profile name as its value, and set the version to 6. A new store SHALL get version 6 and no backup.

#### Scenario: Upgrade of an existing store
- **WHEN** `i2a` opens a store with schema version 5 that has 120 messages sent by the profile `triage`, 4 dispatches, 1 session and 2 asks
- **THEN** `inbox.db.v5.bak` exists with schema version 5, and the store has schema version 6, the same 120 messages with `space = "triage"`, 4 dispatches with `space` and `agent` both `triage`, 1 session for the space `triage`, and 2 asks

#### Scenario: Failure during the upgrade
- **WHEN** the upgrade transaction fails
- **THEN** the store stays at schema version 5, and the command reports the error and the backup path
