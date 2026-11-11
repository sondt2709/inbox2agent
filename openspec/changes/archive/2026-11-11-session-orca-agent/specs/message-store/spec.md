## MODIFIED Requirements

### Requirement: Schema version 10 with a backup
When the system opens a store with schema version 9, it SHALL first write a consistent copy to `<data>/inbox.db.v9.bak`, then, in one transaction, add the columns `sessions.agent_name` (empty for the existing rows), `sessions.last_sent_at` (0 for the existing rows) and `sessions.orca_agent` (empty for the existing rows), and set the version to 10. A store of an earlier version SHALL get the earlier upgrades first. A new store SHALL get version 10 and no backup. A version 10 store without the column `sessions.orca_agent` SHALL get it when the system opens it.

#### Scenario: Upgrade from version 9
- **WHEN** the system opens a version 9 store with 2 sessions
- **THEN** `inbox.db.v9.bak` exists with schema version 9, and the store has schema version 10 and the 2 sessions with their handles and Claude session IDs, no name, no last send and no Orca agent

#### Scenario: Version 10 store without the Orca agent
- **WHEN** the system opens a version 10 store that has no column `sessions.orca_agent`
- **THEN** the store has the column, and its sessions stay
