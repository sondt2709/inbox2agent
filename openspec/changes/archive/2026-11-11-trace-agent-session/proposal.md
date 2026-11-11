## Why

When the Orca terminal of a space closes (Orca restarts, the computer restarts, or the user closes the tab), the driver starts a new Claude session. The agent then loses all that it knew about the space. The stored Claude session ID is not used to get the conversation back. The user also cannot easily find the session of a space: the store keeps the handle and the session ID only. The Claude session has the name that Orca gives it (`<folder>-<suffix>`), and i2a records no last send of the session.

Change C of issue #20. A spike on Orca 1.4.253 and Claude Code 2.1.322 (`tmp/evidence/trace-agent-session/spike.md`, not committed) found that:

- `/resume <session ID>`, typed into a Claude that `orca agent launch` started, opens the old conversation in the same process and keeps the arguments of Orca.
- `claude --resume` in `orca terminal create --command` loses these arguments: `--plugin-dir` of the Orca agent view and `--dangerously-skip-permissions`.
- `/rename <name>` sets the session name.
- `/resume` with an unknown ID changes nothing.

## What Changes

- In `reuse` mode, when the stored terminal of a space has no live agent, and no Claude process runs the stored Claude session, the driver starts a new terminal with `orca agent launch` and types `/resume <session ID>`. When the terminal then runs the stored session, the batch goes there, and the dispatch detail says "resumed Claude session". Otherwise the driver uses the new session and the detail says why.
- The driver does not resume when the stored terminal runs another Claude session (after `/clear` or a new `claude`), when the user unlinked the session, when the CLI is not Claude, or in the `new` and `worktree` modes.
- A new Claude session that i2a starts gets the name `i2a · <space name>` with `/rename`. A resumed session keeps its name.
- The store keeps the Claude session name and the time of the last prompt sent to the session. **Schema version 10** adds `sessions.agent_name` and `sessions.last_sent_at`, with the backup `inbox.db.v9.bak`.
- The Session row of a space and the sessions dialog show the session name and the last send. `i2a status` shows the session of each space.
- An ask whose card terminal is gone goes to the agent of the space as before, so it also resumes the session.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-delivery`: the reuse mode resumes the stored Claude session; a new Claude session gets a name; the last send is stored.
- `message-store`: schema version 10 with a backup.
- `configuration`: the Session row and the sessions dialog show the name and the last send.
- `daemon-service`: `i2a status` shows the session of each space.

## Impact

- `internal/agent/orca.go` (start, resume, rename, last send), `internal/agent/claude.go` (session name).
- `internal/inbox/session.go`, `internal/store/store.go` (schema version 10, `SetSession`, `TouchSession`).
- `internal/tui/agent_form.go` (`sessionLine`), `internal/tui/dialog_sessions.go`, `internal/cli/status.go`.
- The driver types two Claude commands into a terminal that it started: `/resume <UUID>` and `/rename i2a · <space name>`. The UUID is checked, and the name is one line with no control characters.
- No configuration change. No new dependency.
