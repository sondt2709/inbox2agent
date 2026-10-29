## MODIFIED Requirements

### Requirement: Orca reuse-session mode
In `reuse` mode, the `orca` driver SHALL keep the terminal handle of the profile and the hash of the primer in the store. When the handle is live, the stored hash is the hash of the current primer, and the agent is idle, the driver SHALL send only the batch prompt with `orca terminal send --enter`. When no live handle exists, or the stored hash is different, the driver SHALL start a new session with the primer and the batch prompt, and store the new handle and hash. The driver SHALL NOT close the old terminal. When the agent is busy, the driver SHALL return `held`, and the messages SHALL stay `pending`.

#### Scenario: Second batch in the same session
- **WHEN** two batches go to a profile in `reuse` mode, and the agent is idle before the second batch
- **THEN** both batches arrive in the same terminal, and the primer arrives one time only

#### Scenario: Terminal closed
- **WHEN** the stored terminal of a profile was closed, and a new batch arrives
- **THEN** the driver starts a new session, submits the primer and the batch, and stores the new handle

#### Scenario: Primer changed
- **WHEN** the stored terminal is live, but the user changed the primer template, or the store came from schema version 2 with no hash
- **THEN** the driver starts a new session with the new primer and the batch, and stores the new handle and hash

#### Scenario: Agent busy
- **WHEN** the agent does not become idle within 5 seconds
- **THEN** the dispatch status is `held`, the messages stay `pending`, and the next tick sends them
