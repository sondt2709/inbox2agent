## Why

Feedback on PR #24 (change C of #20):

- The user wants the terminal ID in the dashboard, so a card leads back to its Orca terminal.
- When a closed session opens in a new terminal, it must use the same Orca agent as before. Orca has two Claude agents:
  - `claude` runs `claude --plugin-dir <Orca agent view> --dangerously-skip-permissions`.
  - `claude-agent-teams` runs `claude --teammate-mode auto --dangerously-skip-permissions`, with `ORCA_AGENT_TEAMS_TEAM_ID` and `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` in its environment.

  i2a always launched with the `cli` of the space.

A spike on Orca 1.4.253 also found these facts:

- `orca agent launch --agent claude-agent-teams` refuses `--model`.
- Orca reports `agentIdentity: claude-agent-teams` for a terminal that it launched with that agent, but `claude` for another agent-teams terminal. So `agentIdentity` cannot tell the two agents apart.
- Each `orca terminal send` to an agent-teams terminal returns `observation: "unsupported"` (provider `unsupported`, stage `input_accepted` only), and the agent still gets the text. The driver reads `unsupported` as a shell: it fails the send and forgets the session. So i2a cannot use an agent-teams session today.
- `tui-idle` works on an agent-teams terminal: satisfied when idle, `timeout` while busy.
- `/resume <id>` works in it: the agent had the old turns.

## What Changes

- The Card panel of the Feed shows the terminal of the card (`terminal term_<uuid>`, on one line, so cmd+click in Orca shows it) and its Claude session.
- The driver records the Orca agent of each stored session: `claude-agent-teams` when the Claude process has `ORCA_AGENT_TEAMS_TEAM_ID` in its environment, else `claude`. It records it when it starts, adopts or follows a session; the dashboard records it when the user links a session. The store keeps it in `sessions.orca_agent` (schema version 10; this PR is not released, so no new version).
- A resume launches the stored Orca agent of the session. A new session launches the `cli` of the space (`claude-agent-teams` is a valid value).
- The driver does not pass `--model` to `claude-agent-teams` (launch and worktree).
- When Orca returns `observation: "unsupported"` and the Claude files show exactly one interactive Claude process in the terminal, the text went to Claude: the send is `sent` with the detail "Orca cannot observe this agent". Otherwise `unsupported` stays an error, as before.
- The Session row, the sessions dialog and `i2a status` show the Orca agent of a session when it is `claude-agent-teams`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-delivery`: the Orca agent of a session; prompt submission to an agent that Orca cannot observe.
- `card-feed`: the terminal of a card in the Card panel.
- `message-store`: schema version 10 has `sessions.orca_agent`.

## Impact

- `internal/agent` (`claude.go`, `claude_darwin.go`, `orca.go`, `resume.go`, `inspect.go`).
- `internal/inbox/session.go`, `internal/store/store.go`.
- `internal/tui/tab_feed.go`, `agent_form.go`, `dialog_sessions.go`, `internal/cli/status.go`.
- No configuration change.
