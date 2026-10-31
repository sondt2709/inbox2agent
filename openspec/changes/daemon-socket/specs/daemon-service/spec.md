## MODIFIED Requirements

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

## ADDED Requirements

### Requirement: Daemon API on a Unix socket
The daemon SHALL serve HTTP on the Unix socket `<data>/i2a.sock` with the mode 0600. When that path is longer than 103 bytes, the daemon SHALL use `$TMPDIR/i2a-<hash>.sock` and SHALL write its path to `<data>/i2a.sock.path`. The daemon SHALL remove an old socket file when it starts (it holds the daemon lock) and SHALL remove the socket file when it stops. The API SHALL have these endpoints:

- `GET /v1/status`: the process ID, the version, the start time, the configuration file, each space with its next send (no time for a space that does not send), and the last read of each account (time, new messages, error).
- `POST /v1/run` with `{"spaces": [...]}`: read all accounts now, send the given spaces (an empty list is all spaces), and return the summary text, the reads, the batches and `failed`.
- `GET /v1/events`: server-sent events with the kinds `read`, `send`, `ask`, `reload`, `run` and `error`.

The API SHALL NOT return message text or card text. The loop of the daemon SHALL run each request, so a request never runs at the same time as a scheduled send. A client SHALL treat only a failed connection to the socket as "no daemon"; an error after the connection SHALL be an error of the run, and the client SHALL NOT do the run itself. A process that does the run itself (`i2a run --once`, the dashboard) SHALL hold the daemon lock while it runs, and SHALL fail with a message to try again when a daemon holds the lock.

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
- **WHEN** a batch sends a message with the text "secret plan"
- **THEN** no response and no event of the API contains "secret plan"

### Requirement: Status shows the daemon API
`i2a status` SHALL show whether a daemon answers on the socket, with the path of the socket.

#### Scenario: Status with a daemon
- **WHEN** the daemon runs and the user runs `i2a status`
- **THEN** the output has a line `Daemon: running, API <socket path>`
