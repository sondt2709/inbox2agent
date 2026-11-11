## Context

- The store keeps one session for each space (`sessions`: `space`, `handle`, `agent_session`, `created_at`, `primer_hash` not used). Real store of the user (2026-11-11): 5 rows, each with a handle and a Claude session ID.
- `Orca.reuse` (`internal/agent/orca.go`) calls `check`. When the check fails, `startSession` runs `orca agent launch`, renames the tab, `waitAgent`, waits for `tui-idle`, sends the prompt, then stores the handle and the Claude session that `claudeSession` reads.
- `check` fails in these cases: the terminal is closed or gone; the terminal runs no Claude (after `/exit`, or a shell after an Orca restart); the terminal runs another Claude session (after `/clear` or a new `claude`); the stored session runs in two other terminals; with no Claude files, Orca reports no agent.
- `ClaudeFinder` reads `name` from `<claude dir>/sessions/<pid>.json` already (`ClaudeProc.Name`). The sessions dialog shows it live; the Session row shows the store only (no `orca` command and no file scan in the 2-second refresh).
- Spike (Orca 1.4.253, Claude Code 2.1.322, `tmp/evidence/trace-agent-session/spike.md`):
  1. `orca agent launch` starts `claude --plugin-dir <Orca agent view> --dangerously-skip-permissions [--model]`.
  2. `/resume <id>` typed into that Claude: the pid does not change, `sessionId` becomes `<id>`, the name of the old session comes back, the agent remembers the old turns, and Orca reports `turn_started`.
  3. `/resume <unknown id>`: `sessionId` does not change, and no picker opens.
  4. `/rename <name>`: `name` changes, `nameSource: user`.
  5. `orca terminal create --command "claude --resume <id>"` also resumes, but without the Orca arguments.

## Goals / Non-Goals

**Goals:**

- A space keeps its Claude conversation when its terminal closes, also after an Orca restart or a computer restart.
- The user can find the session of a space: by its handle, its ID, its name `i2a · <space>` (in the `/resume` picker of Claude and in the Orca tab), and its last send.

**Non-Goals:**

- Resuming for a CLI other than Claude Code, or in the `new` and `worktree` modes.
- Resuming the session of a card for an ask: the ask goes to the agent of the space, as today.
- Closing old terminals that hold a shell.
- A dashboard key that starts a new session. Unlink (`u`) followed by the next batch already gives a new session.

## Decisions

### D1. Resume with `/resume` in an Orca-launched Claude

The driver starts the terminal as before (`orca agent launch`), waits for the agent (`waitAgent`) and for `tui-idle`, then types `/resume <id>` with `orca terminal send --enter`. Then it reads the Claude process of the terminal each second, for at most 10 seconds, until `sessionId` is `<id>`.

Alternative: `orca terminal create --command "claude --resume <id>"`. Rejected: Orca does not add `--plugin-dir` and `--dangerously-skip-permissions`. The agent then stops at each permission prompt, and Orca shows no agent view. i2a cannot copy the arguments: the plugin folder path belongs to Orca and can change.

### D2. When to resume

The driver resumes only in `reuse` mode, when the CLI is Claude (`isClaude`) and the Claude files can be read, the stored row has a Claude session ID `S`, and no Claude process runs `S` (`WithSession(S)` is empty). These are the cases where `S` is "not open": the terminal is gone, or `/exit` left a shell, or Orca restarted. The driver does not resume when:

- the stored terminal runs another Claude session: the user ran `/clear` or started a new `claude`, so the user chose a new conversation;
- `S` runs in a terminal (two writers on one session): the check follows `S` when it runs in exactly one Orca terminal; else a new session starts, as today;
- the user unlinked the session (no stored row).

`check` returns the reason with the result. `reuse` uses it to choose a resume or a new session.

### D3. Result of a resume

- `sessionId` becomes `S` in time: the batch goes to the terminal. The store gets the new handle, `S`, and the name from the session file. The detail is `resumed Claude session <S8> in <handle>, <send detail>`.
- `sessionId` does not become `S` (no transcript in the folder, for example after a change of the folder): the driver keeps the new session that `orca agent launch` started, names it, and sends the batch. The detail is `could not resume Claude session <S8>; started session <handle>, …`.
- The Claude process exits, or Orca reports an error: the dispatch fails as a start failure does today, and the messages stay `pending`.

### D4. Name of a new session

After a new session starts (not resumed), the driver types `/rename i2a · <space name>` and waits for `tui-idle`. Then it sends the prompt. The name is the tab title (`TabTitle`). The driver removes control characters and line breaks from the name, and cuts it to 80 characters. A failed rename adds a note to the detail, as a failed tab rename does. A resumed session keeps its name, so a name that the user gave stays. The `new` and `worktree` modes also name their session: a name is useful in the `/resume` picker.

### D5. Store

Schema version 10: `sessions.agent_name TEXT NOT NULL DEFAULT ''` and `sessions.last_sent_at INTEGER NOT NULL DEFAULT 0` (Unix milliseconds, as `created_at`). Backup `inbox.db.v9.bak`, as for the other versions. `inbox.Session` gets `Name` and `LastSent`. `SetSession` writes all fields. The driver sets `LastSent` after each `sent` result in the session (a batch or an ask), with `TouchSession(space, handle, name, at)`. The name comes from `ClaudeProc.Name`, read during the check that the send does already. So a `/rename` of the user shows after the next send.

### D6. Screens

- Session row: `<handle> · Claude <S8> · "<name>" · last send 09:04` (only the known parts).
- Sessions dialog: the stored name and the last send under the live state.
- `i2a status`: a `Sessions:` part with one line for each stored session: space, handle, Claude session, name, last send.

### D7. Tests

- Unit tests with the fake runner and a fake `ClaudeFinder` of `orca_test.go`:
  - resume after a closed terminal and after `/exit`;
  - no resume after `/clear`;
  - no resume when `S` runs in another terminal;
  - resume fails, so a named new session;
  - a non-Claude CLI;
  - the name in a new session;
  - `LastSent` after a send.
- Store test: schema 9 to 10 with the backup.
- Local Orca e2e scenario (`I2A_E2E_ORCA=1`): send a batch, close the terminal with `orca terminal close`, send a second batch. The second detail says "resumed", the new terminal runs the stored session ID, and the session name is `i2a · Unsorted`.

## Risks / Trade-offs

- [The driver types Claude commands, not only prompts] → only two fixed commands; the UUID is checked with a pattern; the name is sanitized. The driver checks the result in the session file, not on the screen.
- [`/resume` and `/rename` are not documented as stable] → the check of `sessionId` detects a change of behavior, and the batch still goes to a new session. The facts go into `CLAUDE.md` with the version.
- [A resumed old conversation can be long] → Claude compacts it as usual; this is the same as a long reuse session today.
- [A shell tab stays after `/exit`] → as today; the user closes it.
- [Store migration] → the backup `inbox.db.v9.bak`; an older i2a refuses the version 10 store as it refuses all newer schemas, so the rollback is: copy the backup back.
