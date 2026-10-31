## MODIFIED Requirements

### Requirement: Run once
`i2a run --once` SHALL read all accounts one time, send the pending messages of each space that has an agent, print a summary and exit. It SHALL ignore the schedules. It SHALL NOT send the messages of Unsorted when Unsorted has no agent.

#### Scenario: Summary output
- **WHEN** the user runs `i2a run --once` and one account returns 3 new messages for the space `acme`
- **THEN** the output shows 3 collected messages and one batch of `acme` sent to its agent, with the digest path

#### Scenario: Nothing new
- **WHEN** the user runs `i2a run --once` two times and no new message arrives between the runs
- **THEN** the second run shows 0 collected messages and sends no batch

### Requirement: Daemon loop
`i2a daemon` SHALL read each account on its `read_every` value. It SHALL send the pending messages of a space at each send time of the schedule of the space. A space with no agent SHALL never send. The daemon SHALL log one line for each read and each dispatch, with the space ID. An account error SHALL NOT stop the other accounts.

#### Scenario: Two spaces on two schedules
- **WHEN** the space `acme` uses a schedule with `every = "1m"` and the space `personal` uses a schedule with `every = "2m"`, and the daemon runs for 4 minutes with new messages in both
- **THEN** the log shows about 4 dispatches of `acme` and about 2 dispatches of `personal`

#### Scenario: Account error
- **WHEN** one account returns an error
- **THEN** the log shows the error with the account ID, and the other accounts continue to read

### Requirement: Configuration reload
The daemon SHALL load the configuration again when the configuration file changes, and SHALL compute the next send time of each space again. When the new configuration is not valid, the daemon SHALL log the error and keep the previous configuration.

#### Scenario: Schedule changed
- **WHEN** the user changes the schedule of a space in the TUI while the daemon runs
- **THEN** the daemon logs "configuration reloaded" and uses the new send times

## REMOVED Requirements

### Requirement: Active hours
**Reason**: Hours and days are now part of each schedule.
**Migration**: The version 1 `active_hours` becomes `hours` of the schedule `default` (requirement "Migration from version 1" of the `spaces` capability). Outside the send times the daemon still collects, as the requirement "Schedules" of the `spaces` capability says.

## ADDED Requirements

### Requirement: Status command
`i2a status` SHALL print the configuration and data paths, the accounts with their state, each space with its agent, schedule, next send time and number of waiting messages, the number of unsorted conversations and the agent or "hold" of Unsorted, the agents, the schedules, the message counts, the last dispatches and the service state.

#### Scenario: Space line
- **WHEN** the space `acme` uses `claude-work` and `work-hours`, has 6 waiting messages, and the next send time is 14:45
- **THEN** `i2a status` prints a line with `acme`, `claude-work`, `work-hours`, `next 14:45` and `6 waiting`
