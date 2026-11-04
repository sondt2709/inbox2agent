## Why

i2a keeps only the Orca terminal handle of the session of each space. One terminal can run Claude session A, then a shell, then Claude session B. Today i2a sends to the terminal in all three cases. When Claude exits, i2a types the batch, which holds untrusted message text, into zsh, and zsh runs it. A probe on Orca 1.4.240 proved this. The user also cannot see which Orca tab is the session of a space, and cannot choose an existing Claude session.

## What Changes

- The session of a space becomes a pair: the terminal handle (the address for each `orca` command) and the Claude session ID (the identity).
- i2a reads the Claude session of a terminal from the Claude process: `~/.claude/sessions/<pid>.json` gives the session ID, and the process environment gives `ORCA_TERMINAL_HANDLE`.
- Before each batch and each ask, the driver checks that the terminal runs the stored Claude session. No Claude process, or another session ID, starts a new session. When exactly one other terminal runs the stored session, the driver follows the session to that terminal.
- When the Claude session cannot be read (another CLI, or an unknown file format), the driver requires the Orca field `agentIdentity` in addition to the current live check.
- A send that Orca reports with `observation: "unsupported"` (no agent in the terminal) is an error. The driver does not press Enter again.
- After the launch of a new session, the driver renames the Orca tab to `<agent name> · <space name>`.
- `i2a card add` also records `CLAUDE_CODE_SESSION_ID`. An ask goes to the terminal of the card only when that terminal still runs the session of the card.
- Setup → Agents shows the session of each space of the agent: handle, Claude session ID, Claude session name and state. The user can link a handle or a Claude session ID to a space, unlink it, and open it in Orca.
- Store schema version 8: `sessions.agent_session` and `cards.agent_session`, with the backup `inbox.db.v7.bak`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-delivery`: the reuse session checks the Claude session; the follow rule; a linked session gets the primer; a send to a terminal with no agent is an error; the tab name of a new session.
- `agent-ask`: the ask target checks the Claude session of the card.
- `card-feed`: `i2a card add` records the Claude session ID.
- `message-store`: schema version 8.
- `configuration`: the Agents tab shows and links the sessions of the spaces.

## Impact

- `internal/agent`: `orca.go`, a new Claude session finder (`claude.go`), the `Sessions` interface.
- `internal/store`: schema version 8, `Session`, `SetSession`, `SaveCards`.
- `internal/card`, `internal/cli/card.go`: the new environment variable.
- `internal/ask`: the target uses the session of the card.
- `internal/tui/tab_agents.go` and `dialog.go`: the sessions dialog.
- `CLAUDE.md`: the Orca and Claude facts.
- No new dependency. `golang.org/x/sys` is already in `go.mod`. No change to `config.toml` or the daemon API.
