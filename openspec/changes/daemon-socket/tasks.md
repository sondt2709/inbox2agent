## 1. Tests that fail on the base

- [x] 1.1 Test: the server answers `/v1/status`, `/v1/run` and `/v1/events` on a socket with mode 0600
- [x] 1.2 Test: a long data path uses `$TMPDIR` and `i2a.sock.path`; the client finds it
- [x] 1.3 Test: no response and no event contains the text of a message
- [x] 1.4 Test: `R` in the Feed and in the Spaces tab calls the run with all spaces and with the selected space; a second `R` while busy does not start a run
- [x] 1.5 Test: an event of the daemon shows in the status line
- [x] 1.6 Test: each path of the server is in `docs/daemon-api.yaml`

## 2. Daemon

- [x] 2.1 `internal/daemon/api.go`: the server, the event hub, the socket path
- [x] 2.2 `internal/daemon/client.go`: `Dial`, `Status`, `Run`, `Events`
- [x] 2.3 `loop.go`: run requests, events, status
- [x] 2.4 `i2a daemon` listens on the socket and removes it when it stops

## 3. CLI and dashboard

- [x] 3.1 `i2a run --once` through the daemon when it answers
- [x] 3.2 `i2a status`: the daemon line
- [x] 3.3 `tui.Deps.RunNow` and `tui.Deps.Events`; the CLI gives them (daemon client, or a local engine under the daemon lock)
- [x] 3.4 Key `R` in the Feed and the Spaces tab, the hints and the key help; events in the status line

## 4. Docs and e2e

- [x] 4.1 `docs/daemon-api.yaml`, README, CLAUDE.md
- [x] 4.2 E2E: `run --once` while the daemon runs; `curl` status and run on the socket; `R` in the Feed with and without the daemon

## 5. Check

- [x] 5.1 Break each change one time and see the new test fail
- [ ] 5.2 `task test`, `task lint`, e2e lint, `task e2e`, `task leakcheck`, `openspec validate --strict`
- [ ] 5.3 Archive the change and open the PR
