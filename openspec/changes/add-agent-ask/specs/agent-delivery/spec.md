## ADDED Requirements

### Requirement: One delivery at a time for each profile
The system SHALL hold an exclusive lock on the file `<data>/agent-<profile>.lock` while a batch or an ask goes to the agent of the profile. A batch SHALL wait up to 3 minutes for the lock; after that, the dispatch status SHALL be `held` and the messages SHALL stay `pending`. An ask SHALL wait up to 3 minutes for the lock; after that, the ask SHALL stay `queued`.

#### Scenario: Ask and batch at the same time
- **WHEN** the user sends an ask while the daemon dispatches a batch to the same profile
- **THEN** the agent terminal gets the two prompts one after the other, and neither prompt contains text of the other

### Requirement: Queued asks in a dispatch
Each dispatch SHALL send the `queued` asks before the batches, with the wait of 5 seconds of the reuse mode. A busy agent SHALL keep the ask `queued`. An Orca error SHALL make the ask `failed` with the error text.

#### Scenario: Queued ask sent
- **WHEN** an ask is `queued` because the agent was busy, and the agent is idle at the next dispatch
- **THEN** the dispatch sends the ask first, and the ask is `sent`
