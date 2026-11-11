## MODIFIED Requirements

### Requirement: Status command
`i2a status` SHALL print the configuration and data paths, the accounts with their state, each space with the description of its agent (or "paused"), its schedule, the next send time and the number of waiting messages, the number of unsorted conversations and the agent or "hold" of Unsorted, the schedules, the message counts, the last dispatches, the stored session of each space (the handle, the Claude session ID, the name and the last send) and the service state. It SHALL NOT print a list of agents.

#### Scenario: Space line
- **WHEN** the space `acme` has an `orca` agent with `cli = "claude"`, the schedule `work-hours`, 6 waiting messages, and the next send time is 14:45
- **THEN** `i2a status` prints a line with `acme`, `orca · claude`, `work-hours`, `next 14:45` and `6 waiting`

#### Scenario: Paused space line
- **WHEN** the space `iso` is paused and has 2 waiting messages
- **THEN** `i2a status` prints a line with `iso`, `paused: does not send` and `2 waiting`

#### Scenario: Session line
- **WHEN** the store has `term_a`, the Claude session `04ebe19c-…` and the name "i2a · Triage" for the space `triage`
- **THEN** `i2a status` prints a line under `Sessions:` with `triage`, `term_a`, `04ebe19c` and "i2a · Triage"
