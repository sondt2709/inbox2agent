# daemon-service Specification

## Purpose
This capability runs i2a: one time (`i2a run --once`), as a loop (`i2a daemon`), or as a macOS LaunchAgent. It also covers the active hours, the configuration reload and the desktop notifications.
## Requirements
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

### Requirement: Single daemon
The daemon SHALL hold an exclusive lock on `<data>/daemon.lock`. A second daemon SHALL exit with an error that says that a daemon already runs.

#### Scenario: Second daemon
- **WHEN** a daemon runs and the user starts `i2a daemon` again
- **THEN** the second process exits with a non-zero code and the message "another i2a daemon is running"

### Requirement: Configuration reload
The daemon SHALL load the configuration again when the configuration file changes, and SHALL compute the next send time of each space again. When the new configuration is not valid, the daemon SHALL log the error and keep the previous configuration.

#### Scenario: Schedule changed
- **WHEN** the user changes the schedule of a space in the TUI while the daemon runs
- **THEN** the daemon logs "configuration reloaded" and uses the new send times

### Requirement: macOS LaunchAgent
On macOS, `i2a service install` SHALL write a LaunchAgent file for the label `io.github.sondt2709.i2a` that runs the absolute path of `i2a` with the argument `daemon`, and SHALL load it with `launchctl bootstrap`. `i2a service uninstall` SHALL unload it and delete the file. `i2a service status` SHALL show whether the agent is loaded and its process ID. On other platforms, these commands SHALL exit with the message "not supported on this platform".

#### Scenario: Install and uninstall
- **WHEN** the user runs `i2a service install`
- **THEN** `launchctl print gui/<uid>/io.github.sondt2709.i2a` shows the state `running`
- **WHEN** the user runs `i2a service uninstall`
- **THEN** `launchctl print` reports that the service is not found and the file is deleted

#### Scenario: Orca path in the service environment
- **WHEN** the user installs the service and an agent uses the `orca` driver
- **THEN** the LaunchAgent file has a `PATH` value that contains the directory of the `orca` executable

### Requirement: Desktop notification
On macOS, the daemon and `i2a run --once` SHALL show a desktop notification after each failed dispatch with a driver other than `dry-run`, with the space, the agent, the message count and the error text. They SHALL NOT show a notification for a sent batch; the cards of the agent give the notifications (see the capability `card-feed`). The daemon SHALL show one notification when a WhatsApp account becomes not paired, and again only after a successful read of that account. The system SHALL pass the title and the text to `osascript` as arguments, not as script code. `[notify] enabled = false` SHALL turn all notifications off. The default SHALL be `true`. `[notify] min_urgency` SHALL be `high`, `normal` or `low`; the default SHALL be `normal`. The configuration validation SHALL reject other values.

#### Scenario: Batch sent
- **WHEN** an `orca` batch of 12 messages becomes `sent`
- **THEN** no notification shows

#### Scenario: Dispatch failed
- **WHEN** an `orca` batch fails because Orca is not reachable
- **THEN** macOS shows a notification with the title "i2a: dispatch failed", the profile name and the error text

#### Scenario: Notifications off
- **WHEN** `[notify] enabled = false` and a dispatch fails
- **THEN** no notification shows

#### Scenario: WhatsApp not paired
- **WHEN** the phone unlinks i2a and the daemon polls the source 5 times
- **THEN** macOS shows one notification with the title "i2a: WhatsApp not paired" and the source ID

### Requirement: Source connections
The daemon SHALL close the connection of each source when it loads a new configuration and when it stops. `i2a run --once` SHALL close the connections before it exits.

#### Scenario: Configuration reload
- **WHEN** the daemon holds the connection of a WhatsApp source and the user saves the configuration
- **THEN** the daemon closes the connection and opens it again on the next poll

#### Scenario: Daemon stops
- **WHEN** the daemon holds the connection of a WhatsApp source and stops
- **THEN** `<data>/whatsapp-<id>.lock` is not locked any more

### Requirement: Status command
`i2a status` SHALL print the configuration and data paths, the accounts with their state, each space with its agent, schedule, next send time and number of waiting messages, the number of unsorted conversations and the agent or "hold" of Unsorted, the agents, the schedules, the message counts, the last dispatches and the service state.

#### Scenario: Space line
- **WHEN** the space `acme` uses `claude-work` and `work-hours`, has 6 waiting messages, and the next send time is 14:45
- **THEN** `i2a status` prints a line with `acme`, `claude-work`, `work-hours`, `next 14:45` and `6 waiting`

