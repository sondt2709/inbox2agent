# daemon-service Specification

## Purpose
This capability runs i2a: one time (`i2a run --once`), as a loop (`i2a daemon`), or as a macOS LaunchAgent. It also covers the active hours, the configuration reload and the desktop notifications.
## Requirements
### Requirement: Run once
`i2a run --once` SHALL read all accounts one time, send the pending messages of each space that has an agent, print a summary and exit. It SHALL ignore the schedules. It SHALL NOT send the messages of Unsorted when Unsorted has no agent. When a daemon runs and answers on its socket, `i2a run --once` SHALL ask the daemon to do the run (`POST /v1/run`) and print the summary of the daemon; it SHALL NOT read an account in its own process. When no daemon answers, it SHALL do the run in its own process.

#### Scenario: Summary output
- **WHEN** the user runs `i2a run --once` and one account returns 3 new messages for the space `acme`
- **THEN** the output shows 3 collected messages and one batch of `acme` sent to its agent, with the digest path

#### Scenario: Nothing new
- **WHEN** the user runs `i2a run --once` two times and no new message arrives between the runs
- **THEN** the second run shows 0 collected messages and sends no batch

#### Scenario: Run once while the daemon runs
- **WHEN** the daemon runs and the user runs `i2a run --once`
- **THEN** the daemon reads the accounts and sends the batches, and the command prints the summary of the daemon

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
On macOS, the daemon and `i2a run --once` SHALL show a desktop notification after each failed dispatch with a driver other than `dry-run`, with the space, the agent, the message count and the error text. They SHALL NOT show a notification for a sent batch; the cards of the agent give the notifications (see the capability `card-feed`). The daemon SHALL show one notification when a WhatsApp account becomes not paired, and again only after a successful read of that account. The system SHALL show these notifications through the notifier chain (see the requirement "Notifier chain" of the capability `desktop-notifications`). `[notify] enabled = false` SHALL turn all notifications off. The default SHALL be `true`. `[notify] min_urgency` SHALL be `high`, `normal` or `low`; the default SHALL be `normal`. The configuration validation SHALL reject other values.

#### Scenario: Batch sent
- **WHEN** an `orca` batch of 12 messages becomes `sent`
- **THEN** no notification shows

#### Scenario: Dispatch failed
- **WHEN** an `orca` batch fails because Orca is not reachable
- **THEN** a notification shows with the title "i2a: dispatch failed", the space name and the error text

#### Scenario: Notifications off
- **WHEN** `[notify] enabled = false` and a dispatch fails
- **THEN** no notification shows

#### Scenario: WhatsApp not paired
- **WHEN** the phone unlinks i2a and the daemon polls the source 5 times
- **THEN** one notification shows with the title "i2a: WhatsApp not paired" and the source ID

#### Scenario: Daemon as a LaunchAgent with terminal-notifier
- **WHEN** the daemon runs as the LaunchAgent, terminal-notifier is in `/opt/homebrew/bin`, Orca does not run, and a dispatch fails
- **THEN** terminal-notifier shows the notification

### Requirement: Source connections
The daemon SHALL close the connection of each source when it loads a new configuration and when it stops. `i2a run --once` SHALL close the connections before it exits.

#### Scenario: Configuration reload
- **WHEN** the daemon holds the connection of a WhatsApp source and the user saves the configuration
- **THEN** the daemon closes the connection and opens it again on the next poll

#### Scenario: Daemon stops
- **WHEN** the daemon holds the connection of a WhatsApp source and stops
- **THEN** `<data>/whatsapp-<id>.lock` is not locked any more

### Requirement: Status command
`i2a status` SHALL print the configuration and data paths, the accounts with their state, each space with the description of its agent (or "paused"), its schedule, the next send time and the number of waiting messages, the number of unsorted conversations and the agent or "hold" of Unsorted, the schedules, the message counts, the last dispatches and the service state. It SHALL NOT print a list of agents.

#### Scenario: Space line
- **WHEN** the space `acme` has an `orca` agent with `cli = "claude"`, the schedule `work-hours`, 6 waiting messages, and the next send time is 14:45
- **THEN** `i2a status` prints a line with `acme`, `orca · claude`, `work-hours`, `next 14:45` and `6 waiting`

#### Scenario: Paused space line
- **WHEN** the space `iso` is paused and has 2 waiting messages
- **THEN** `i2a status` prints a line with `iso`, `paused: does not send` and `2 waiting`

### Requirement: Daemon API on a Unix socket
The daemon SHALL serve HTTP on the Unix socket `<data>/i2a.sock` with the mode 0600. When that path is longer than 103 bytes, the daemon SHALL use `$TMPDIR/i2a-<hash>.sock` and SHALL write its path to `<data>/i2a.sock.path`. The daemon SHALL remove an old socket file when it starts (it holds the daemon lock) and SHALL remove the socket file when it stops. The API SHALL have these endpoints:

- `GET /v1/status`: the process ID, the version, the start time, the configuration file, each space with its next send (no time for a space that does not send), and the last read of each account (time, new messages, error).
- `POST /v1/run` with `{"spaces": [...]}`: read all accounts now, send the given spaces (an empty list is all spaces), and return the summary text, the reads, the batches and `failed`.
- `GET /v1/events`: server-sent events with the kinds `read`, `send`, `ask`, `reload`, `run` and `error`.
- `POST /v1/reply` with `{"reply_id": <n>}`: send the stored reply `<n>` (state `sending`) through the connection of its source, and return the message ID, or the error and whether the platform answered.
- `POST /v1/download` with `{"message_id": <n>}`: download the file of the stored message `<n>` to a new file in `<data>/tmp/` with the mode 0600, and return its path, the file name and the MIME type.

The API SHALL NOT return message text, card text, reply text or file content. The loop of the daemon SHALL run each request of `/v1/run`, so a run never runs at the same time as a scheduled send. `/v1/reply` and `/v1/download` SHALL NOT wait for the loop; they SHALL lock the source only. A client SHALL treat only a failed connection to the socket as "no daemon"; an error after the connection SHALL be an error of the request, and the client SHALL NOT do the request itself. A process that does the run itself (`i2a run --once`, the dashboard) SHALL hold the daemon lock while it runs, and SHALL fail with a message to try again when a daemon holds the lock.

#### Scenario: Status over the socket
- **WHEN** the daemon runs and a person runs `curl --unix-socket <data>/i2a.sock http://i2a/v1/status`
- **THEN** the response is JSON with the process ID and the next send of each space that has an agent

#### Scenario: Daemon that holds its lock and does not answer
- **WHEN** a daemon starts and holds the daemon lock, and the user runs `i2a run --once` before the daemon listens
- **THEN** the command fails with "a daemon holds its lock but does not answer yet: try again in a few seconds" and reads no account

#### Scenario: Run a space now
- **WHEN** a new message waits for the space `acme`, and a client posts `{"spaces": ["acme"]}` to `/v1/run`
- **THEN** the daemon sends one batch of `acme`, and the response has the batch with its status

#### Scenario: Events of a run
- **WHEN** a client reads `/v1/events` and another client posts to `/v1/run`
- **THEN** the first client gets a `run` event and a `send` event for each batch

#### Scenario: Long data path
- **WHEN** the data directory path makes `<data>/i2a.sock` longer than 103 bytes
- **THEN** the daemon listens on a socket in `$TMPDIR`, `<data>/i2a.sock.path` contains that path, and the client finds the daemon

#### Scenario: No message text
- **WHEN** a batch sends a message with the text "secret plan", and the user sends the reply "secret answer"
- **THEN** no response and no event of the API contains "secret plan" or "secret answer"

#### Scenario: Reply while a batch waits
- **WHEN** the daemon waits for a busy agent during a batch, and the user sends a WhatsApp reply
- **THEN** the reply is sent before the batch ends

#### Scenario: Download through the daemon
- **WHEN** the daemon runs and the agent runs `i2a download --message 1234`
- **THEN** the daemon writes the file to `<data>/tmp/`, the command moves it to `~/Downloads`, and `<data>/tmp/` has no file of it after the command

### Requirement: Status shows the daemon API
`i2a status` SHALL show whether a daemon answers on the socket, with the path of the socket.

#### Scenario: Status with a daemon
- **WHEN** the daemon runs and the user runs `i2a status`
- **THEN** the output has a line `Daemon: running, API <socket path>`

