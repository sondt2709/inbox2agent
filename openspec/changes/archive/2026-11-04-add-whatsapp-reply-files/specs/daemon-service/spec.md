## MODIFIED Requirements

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
