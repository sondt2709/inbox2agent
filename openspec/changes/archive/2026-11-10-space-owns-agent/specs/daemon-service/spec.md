## MODIFIED Requirements

### Requirement: Status command
`i2a status` SHALL print the configuration and data paths, the accounts with their state, each space with the description of its agent (or "paused"), its schedule, the next send time and the number of waiting messages, the number of unsorted conversations and the agent or "hold" of Unsorted, the schedules, the message counts, the last dispatches and the service state. It SHALL NOT print a list of agents.

#### Scenario: Space line
- **WHEN** the space `acme` has an `orca` agent with `cli = "claude"`, the schedule `work-hours`, 6 waiting messages, and the next send time is 14:45
- **THEN** `i2a status` prints a line with `acme`, `orca · claude`, `work-hours`, `next 14:45` and `6 waiting`

#### Scenario: Paused space line
- **WHEN** the space `iso` is paused and has 2 waiting messages
- **THEN** `i2a status` prints a line with `iso`, `paused: does not send` and `2 waiting`
