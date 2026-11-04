## 1. Failing test and fixtures

- [x] 1.1 Capture real Orca fixtures with the paths replaced: `show_agent_exited.json` (`agentIdentity: null` after `/exit`), `show_claude_live.json`, `send_shell.json` (`observation: "unsupported"`), `rename_ok.json`
- [x] 1.2 Capture a real `~/.claude/sessions/<pid>.json` as `internal/agent/testdata/claude/sessions/*.json` with the IDs and paths replaced
- [x] 1.3 Add `TestReuseAgentExited` in `internal/agent/orca_test.go`: the stored terminal shows `show_agent_exited.json`, and the driver must not send to it. Run it on 937b7c2 and keep the failure output

## 2. Claude session finder

- [x] 2.1 Add `internal/agent/claude.go`: `ClaudeProc`, `ClaudeFinder` with `InTerminal(handle)` and `WithSession(id)`, `ErrUnknown`; reader of `<claude dir>/sessions/*.json`; pid check; environment from `kern.procargs2`
- [x] 2.2 Put the process reader behind a function field, so tests use the fixture directory and a fake environment
- [x] 2.3 Unit tests: one process, no process, two processes with one session, a file that does not decode, `CLAUDE_CONFIG_DIR`

## 3. Store schema version 8

- [x] 3.1 Add `migrateV8` (backup `inbox.db.v7.bak`, `sessions.agent_session`, `cards.agent_session`) and set `SchemaVersion = 8`
- [x] 3.2 Change `Session` and `SetSession` to `agent.Session{Handle, AgentSession, PrimerHash}`; add `ListSessions` for the dashboard
- [x] 3.3 `SaveCards` stores `Card.AgentSession`; the update by key replaces it when the call has one
- [x] 3.4 Tests: upgrade from version 7, failure keeps version 7, new store has version 8

## 4. Driver

- [x] 4.1 `live` requires `agentIdentity` not null when the finder is not used
- [x] 4.2 `reuse`: the check of D3 (valid, adopt, follow, new session); `PrimerPending` sends the primer to a linked terminal
- [x] 4.3 After launch, read the Claude session of the new terminal (retry up to 5 seconds) and store it with the handle
- [x] 4.4 `send`: `observation: "unsupported"` returns an error, sends no Enter and deletes the session of the space
- [x] 4.5 Rename the tab to `<agent name> · <space name>` after launch and worktree create; a failure is a note only
- [x] 4.6 `agent.New` takes `config.Space`; `Driver.Ask` takes `agent.Terminal{Handle, AgentSession}`; update the callers in `internal/ask` and `internal/daemon`
- [x] 4.7 Tests for each scenario of the `agent-delivery` and `agent-ask` delta specs

## 5. Cards

- [x] 5.1 `i2a card add` reads `CLAUDE_CODE_SESSION_ID` (UUID, else a warning) into `Card.AgentSession`
- [x] 5.2 Tests: good value, bad value, update by key

## 6. Agents tab

- [x] 6.1 The agent detail shows one line for each space with the stored handle and the short Claude session ID
- [x] 6.2 The key `s` opens the sessions dialog; it loads the state in a `tea.Cmd` (finder and `orca terminal show`)
- [x] 6.3 `l` link (input, resolve, confirm for another workspace or another space), `u` unlink, `o` `orca terminal switch`
- [x] 6.4 Tests for the dialog: rows, link by handle, link by session ID, no Claude process, session of another space

## 7. Docs and e2e

- [x] 7.1 `CLAUDE.md`: the Orca facts (shell after exit, `unsupported`, rename, Copy Terminal ID) and the Claude session facts
- [ ] 7.2 Local e2e scenario (`I2A_E2E_ORCA=1`): run once, `/exit` in the session, run again; expect a new handle and no prompt text in the old terminal
- [ ] 7.3 Write `docs/config.schema.json` again only if a configuration field changed (none expected)
- [ ] 7.4 Run `task test`, `task lint`, `golangci-lint run --build-tags e2e ./test/...`, `task leakcheck`
