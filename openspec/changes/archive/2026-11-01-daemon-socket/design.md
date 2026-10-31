## Context

`daemon.Run` is one loop: reload, read the accounts that are due, send the spaces that are due, asks each minute, prune each hour. `Engine` is not safe for concurrent use; only the loop calls it. `daemon.Lock` (`<data>/daemon.lock`, flock) makes sure that one daemon runs. `i2a run --once` builds its own `Engine`.

## Goals / Non-Goals

**Goals:**
- One process reads the accounts: the daemon when it runs.
- Run now from the dashboard and from scripts.
- A transport-independent API with a version prefix, which a relay can expose later.

**Non-Goals:**
- A TCP listener or authentication. The socket file mode is the access control.
- Message text or card text in the API.
- Change the configuration through the API: the file is the interface (config-as-api).

## Decisions

1. **HTTP over a Unix socket with `net/http`** (as `docker.sock`). `curl --unix-socket` works for a person, and the Go client is a `http.Client` with a `DialContext` to the socket. JSON lines need their own framing and give no tools.
2. **The loop runs each request.** The handler of `POST /v1/run` puts a request on a channel and waits for the reply. The loop selects on the ticker and the channel, so the engine stays in one goroutine. A run reads all accounts (forced) and sends the given spaces with `Engine.Dispatch`. A run while a batch waits for an agent waits too; the handler waits up to 10 minutes, and the client cancels when the caller cancels.
3. **The run reply has the summary text and a failed flag.** `daemon.Summary` already formats a run for `i2a run --once`; the reply carries the same text, the reads, the batches and `failed`. The CLI prints the text and exits 1 when `failed` is true, as before.
4. **Events are a small hub.** Each subscriber has a buffered channel (64). The loop publishes without a block: a slow subscriber loses events, not the loop. The kinds are `read` (only with new messages or an error), `send`, `ask`, `reload`, `run` and `error`. An event has a time, a kind, an account or a space, a count, a status and a short detail. The detail is the error text or the driver result (for example "turn started"), never message text.
5. **The socket path.** The daemon holds `daemon.lock` before it listens, so it can remove an old socket file. When `<data>/i2a.sock` is longer than 103 bytes, the daemon uses `$TMPDIR/i2a-<first 12 hex of the SHA-256 of the data path>.sock` and writes that path to `<data>/i2a.sock.path` (mode 0600). The client reads `i2a.sock.path` first. The daemon removes both files when it stops.
6. **The dashboard does not know HTTP.** `tui.Deps` gets `RunNow(ctx, spaces)` and `Events(ctx)`. The CLI gives functions that use the daemon client, or else an engine in the dashboard process under `daemon.lock`, so a daemon cannot start while the dashboard runs the engine. When the lock is held but the socket does not answer, `R` reports that the daemon is starting.
7. **`docs/daemon-api.yaml` is hand-written** OpenAPI 3.1. It is short (3 paths), and a test checks that each path of the server is in the file.

## Risks / Trade-offs

- [Another local user reads the socket] → Mode 0600 in the data directory (0700). The API has no message text.
- [A long batch blocks a run] → The loop runs one step at a time, as before. The dashboard shows "Running…" and the user can keep working; the reply comes when the loop is free.
- [A stale `i2a.sock.path` after a crash] → The client dials the path; when the dial fails, it treats the daemon as stopped. The next daemon writes the file again.
