## 1. Store

- [x] 1.1 Test first: a version 10 store without `orca_agent` gets the column; a version 9 store gets all three columns
- [x] 1.2 `sessions.orca_agent` in the schema, `migrateV10` and the version 10 check; `inbox.Session.OrcaAgent` read and written

## 2. Driver

- [x] 2.1 Tests first: the Orca agent from the environment; resume launches the stored agent with no `--model`; a new session launches the `cli`; `unsupported` with one Claude process is `sent`; `unsupported` with none is an error; adopt and follow record the agent
- [x] 2.2 `ProcInfo.Teams` from `ORCA_AGENT_TEAMS_TEAM_ID`; `ClaudeProc.OrcaAgent`
- [x] 2.3 `launchAgent`, `modelAllowed` in `launchArgs` and the worktree command
- [x] 2.4 `send` and `command`: `unsupported` with one Claude process
- [x] 2.5 `check` and `startSession` record the Orca agent; `Inspector.Resolve` returns it

## 3. Screens

- [x] 3.1 Card panel: the terminal line; test
- [x] 3.2 Link stores the Orca agent; Session row, dialog and `i2a status` show `agent teams`; tests

## 4. Checks and documents

- [x] 4.1 Orca e2e scenario: a batch to a `claude-agent-teams` session (link its handle), close it, a second batch resumes it under `claude-agent-teams`
- [x] 4.2 `CLAUDE.md` facts (agent teams, `--model`, `unsupported`, `agentIdentity`), README `cli` values
- [ ] 4.3 lint, unit tests, leakcheck, the Orca e2e scenarios
