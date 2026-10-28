# daemon-service Specification

## Purpose
This capability runs i2a: one time (`i2a run --once`), as a loop (`i2a daemon`), or as a macOS LaunchAgent. It also covers the active hours, the configuration reload and the desktop notifications.

## Requirements
### Requirement: Run once
`i2a run --once` SHALL poll all sources one time, dispatch all pending messages, print a summary and exit. It SHALL ignore `interval` and `active_hours`.

#### Scenario: Summary output
- **WHEN** the user runs `i2a run --once` and one source returns 3 new messages for the profile `triage`
- **THEN** the output shows 3 collected messages and one batch sent to `triage` with the digest path

#### Scenario: Nothing new
- **WHEN** the user runs `i2a run --once` two times and no new message arrives between the runs
- **THEN** the second run shows 0 collected messages and sends no batch

### Requirement: Daemon loop
`i2a daemon` SHALL poll each source on its `poll_every` value and dispatch pending messages on `timing.interval`. The daemon SHALL log one line for each poll and each dispatch. A source error SHALL NOT stop the other sources.

#### Scenario: Two ticks
- **WHEN** the daemon runs with `interval = "10s"` for 25 seconds
- **THEN** the log shows at least two dispatch ticks

#### Scenario: Source error
- **WHEN** one source returns an error
- **THEN** the log shows the error with the source ID, and the other sources continue to poll

### Requirement: Single daemon
The daemon SHALL hold an exclusive lock on `<data>/daemon.lock`. A second daemon SHALL exit with an error that says that a daemon already runs.

#### Scenario: Second daemon
- **WHEN** a daemon runs and the user starts `i2a daemon` again
- **THEN** the second process exits with a non-zero code and the message "another i2a daemon is running"

### Requirement: Active hours
When `active_hours` is set and the local time is outside the range, the daemon SHALL collect messages but SHALL NOT dispatch them. A range where the end is before the start SHALL cross midnight.

#### Scenario: Outside active hours
- **WHEN** `active_hours = "09:00-17:00"` and the local time is 20:00
- **THEN** the daemon collects new messages and logs "outside active hours" instead of a dispatch

#### Scenario: Range across midnight
- **WHEN** `active_hours = "22:00-02:00"` and the local time is 01:00
- **THEN** the daemon dispatches pending messages

### Requirement: Configuration reload
The daemon SHALL load the configuration again when the configuration file changes. When the new configuration is not valid, the daemon SHALL log the error and keep the previous configuration.

#### Scenario: Interval changed
- **WHEN** the user saves a new interval in the TUI while the daemon runs
- **THEN** the daemon logs "configuration reloaded" and uses the new interval

### Requirement: macOS LaunchAgent
On macOS, `i2a service install` SHALL write a LaunchAgent file for the label `io.github.sondt2709.i2a` that runs the absolute path of `i2a` with the argument `daemon`, and SHALL load it with `launchctl bootstrap`. `i2a service uninstall` SHALL unload it and delete the file. `i2a service status` SHALL show whether the agent is loaded and its process ID. On other platforms, these commands SHALL exit with the message "not supported on this platform".

#### Scenario: Install and uninstall
- **WHEN** the user runs `i2a service install`
- **THEN** `launchctl print gui/<uid>/io.github.sondt2709.i2a` shows the state `running`
- **WHEN** the user runs `i2a service uninstall`
- **THEN** `launchctl print` reports that the service is not found and the file is deleted

#### Scenario: Orca path in the service environment
- **WHEN** the user installs the service and a profile uses the `orca` driver
- **THEN** the LaunchAgent file has a `PATH` value that contains the directory of the `orca` executable

### Requirement: Desktop notification
On macOS, the system SHALL show a desktop notification after each dispatch with a driver other than `dry-run`: one for `sent` with the profile, the message count and the driver detail, and one for a failure with the error text. The notification SHALL NOT contain message text. The system SHALL pass the title and the text to `osascript` as arguments, not as script code. `[notify] enabled = false` SHALL turn notifications off. The default SHALL be `true`.

#### Scenario: Batch sent
- **WHEN** an `orca` batch of 12 messages becomes `sent`
- **THEN** macOS shows a notification with the title "i2a: 12 messages sent" and the profile name

#### Scenario: Notifications off
- **WHEN** `[notify] enabled = false` and a batch is sent
- **THEN** no notification shows

#### Scenario: Dry run
- **WHEN** a `dry-run` batch is written
- **THEN** no notification shows

