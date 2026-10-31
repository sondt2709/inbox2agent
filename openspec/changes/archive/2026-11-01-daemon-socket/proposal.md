## Why

The user cannot ask i2a to read and send now from the dashboard: the daemon sends only at the times of the schedule of a space. `i2a run --once` does the work in its own process while the daemon runs, so two processes read the same accounts, and a WhatsApp account gets a second connection that replaces the connection of the daemon. Other programs (the dashboard, scripts, later a phone app through a relay) have no way to ask the running daemon what it does.

## What Changes

- The daemon serves a small HTTP API on a Unix socket: `<data>/i2a.sock`, mode 0600. When that path is longer than the limit of a socket path (104 bytes on macOS), the socket is `$TMPDIR/i2a-<hash>.sock` and `<data>/i2a.sock.path` holds its path.
- `GET /v1/status`: the daemon process, the next send of each space, the last read of each account. `POST /v1/run`: read all accounts now and send the given spaces (empty: all). `GET /v1/events`: server-sent events for reads, sends, asks, reloads, runs and errors.
- The API carries no message text and no card text.
- `i2a run --once` asks the running daemon through the socket. When no daemon runs, it runs the engine in its own process, as before.
- The dashboard key `R` reads and sends now: in the Feed for all spaces, in the Spaces tab for the selected space. It uses the daemon when it runs, else it runs the engine in the dashboard process. The status line shows the result and the events of the daemon.
- `docs/daemon-api.yaml` (OpenAPI 3.1) describes the API, so a relay can expose it later.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `daemon-service`: the socket API; `run --once` through the daemon; the status command shows the API.
- `card-feed`: the key `R` in the Feed; events of the daemon in the status line.

## Impact

- New `internal/daemon/api.go` (server, events) and `internal/daemon/client.go` (client, socket path).
- `internal/daemon/loop.go`: the loop serves run requests and publishes events.
- `internal/cli/run.go`, `internal/cli/status.go`, `internal/cli/configcmd.go` (the run function of the dashboard).
- `internal/tui`: key `R` in the Feed and the Spaces tab, the event stream.
- New file at run time: `<data>/i2a.sock` (and maybe `<data>/i2a.sock.path`).
- No change to the configuration file or the store.
