## ADDED Requirements

### Requirement: Schema version 8 with a backup
When the system opens a store with schema version 7, it SHALL first write a consistent copy to `<data>/inbox.db.v7.bak`, then, in one transaction, add the columns `sessions.agent_session` and `cards.agent_session` (empty for the existing rows), and set the version to 8. A store of an earlier version SHALL get the earlier upgrades first. A new store SHALL get version 8 and no backup.

#### Scenario: Upgrade of a version 7 store
- **WHEN** `i2a` opens a store with schema version 7 that has the session `term_a` for the space `triage`
- **THEN** `inbox.db.v7.bak` exists with schema version 7, and the store has schema version 8 and the session `term_a` for `triage` with no Claude session ID

#### Scenario: Failure during the upgrade
- **WHEN** the upgrade transaction fails
- **THEN** the store stays at schema version 7, and the command reports the error and the backup path
