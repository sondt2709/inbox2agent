## Context

See the proposal for the spike facts. The driver reads the environment of each Claude process already (`readProc`, `kern.procargs2`, `ORCA_TERMINAL_HANDLE`).

## Goals / Non-Goals

**Goals:** a resumed session runs under the same Orca agent; agent-teams sessions work; the card shows its terminal.

**Non-Goals:** other agents than Claude Code; reading the Orca data files (`orca-data.json` has `launchAgent` for each tab, but it is an internal file).

## Decisions

### D1. The Orca agent comes from the environment

`ProcInfo.Teams` is true when the environment has a non-empty `ORCA_AGENT_TEAMS_TEAM_ID`. `ClaudeProc.OrcaAgent` is `claude-agent-teams` when `Teams`, else `claude` for a process with an Orca handle, else "". Alternatives: `agentIdentity` (not reliable, see the proposal); the `--teammate-mode` argument (the command of an agent can change in the settings of Orca; the variable names the team).

### D2. Store

`sessions.orca_agent TEXT NOT NULL DEFAULT ''` in `migrateV10` and in the schema. Version 10 is not released, so a store that a build of PR #24 upgraded gets the column at the next open: `migrateV10` adds only missing columns, and a version 10 store runs a check for the column (`ensureV10`).

### D3. Which agent to launch

`launchAgent(resume)`: for a resume, the stored `OrcaAgent` when it is set, else the `cli` of the space; for a new session, the `cli`. `launchArgs` and the worktree command add `--model` only when the agent is not `claude-agent-teams` (`modelAllowed`).

### D4. `unsupported` with a Claude process

In `send` and `command`: when the observation is `unsupported`, the driver reads the Claude processes of the terminal. Exactly one interactive process: the result is "sent, Orca cannot observe this agent" (no second Enter: the stage `turn_started` never comes, and `--enter` was part of the send). Else: `errNoAgent`, as before. Without the Claude files (another CLI, or files that cannot be read), `unsupported` stays `errNoAgent`.

### D5. Screens

- Card panel: a line `terminal <handle> · Claude <id8>` after the meta line when the card has a handle. The handle is not cut.
- Session row and `i2a status`: `agent teams` when the session is `claude-agent-teams`.

## Risks / Trade-offs

- [A Claude process starts in a shell terminal after a send that Orca reports `unsupported`] → the check runs right after the send; the process must be the only interactive Claude process of the terminal, and `sendIdle` checks the session just before it types.
- [Orca renames the variable] → the session gets `claude`, and a resume launches `claude`: the behavior of PR #24 before this change.
