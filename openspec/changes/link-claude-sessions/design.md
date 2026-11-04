## Context

The `orca` driver stores one terminal handle and one primer hash for each space (`sessions`). Before a batch, `Orca.live` runs `orca terminal show` and accepts a terminal that is `connected`, `writable` and has no `exitCause`. It does not check the program in the terminal.

Facts from probes on Orca 1.4.240 and Claude Code (files in the local evidence folder of the change):

- After `/exit`, the terminal stays open with zsh. `show` keeps `connected`, `writable` and `exitCause: null`. `agentIdentity` stays `"claude"` for some seconds, then becomes null.
- `terminal wait --for tui-idle` is satisfied on zsh. `terminal send` types into zsh and returns `provider: "unsupported"`, `observation: "unsupported"`.
- `~/.claude/sessions/<pid>.json` has `pid`, `sessionId`, `cwd`, `name`, `kind`, `startedAt` for each running Claude process.
- The Claude process inherits `ORCA_TERMINAL_HANDLE` from the terminal. `sysctl kern.procargs2` (the source of `ps eww`) returns it for a process of the same user.
- `/clear` keeps the pid and writes a new `sessionId` to the file. A command that the agent runs after `/clear` sees the new `CLAUDE_CODE_SESSION_ID`.
- `/exit` and a new `claude` in the same terminal give a new pid and a new `sessionId`.
- 54 live terminals had exactly one Claude process each. An Agent Teams terminal (`orca claude-teams`) runs a normal Claude process.
- The Orca CLI does not return the Claude session ID. `orca agent launch` cannot resume a session. Each terminal command needs a handle.
- `orca terminal rename --title` sets `customTitle`. The tab bar shows `customTitle ?? title`, so the Claude title does not replace it.
- The Orca pane menu "Copy Terminal ID" copies the handle. A `term_…` text in a terminal is a link: cmd+click focuses that terminal.

## Goals / Non-Goals

**Goals:**

- Never type a batch or an ask into a terminal that does not run the stored Claude session.
- Keep the conversation of a space when its session moves to another terminal.
- Show the handle and the Claude session ID of each space, and let the user link an existing session.

**Non-Goals:**

- Resume a closed Claude session (`claude --resume`). Orca cannot launch it.
- A session check for other CLIs (codex, opencode). They keep the Orca check with `agentIdentity`.
- A command line for the link. The dashboard only.
- A Linux process reader. i2a is macOS first; on another OS the finder reports "unknown" and the Orca check applies.

## Decisions

### D1. The session is (handle, Claude session ID)

The handle stays the address, because every `orca` command needs it. The Claude session ID is the identity, because a terminal can run several sessions over time. Alternative: the Claude session ID only. Rejected: i2a cannot send to a session ID.

### D2. Read the Claude session from the process (`internal/agent/claude.go`)

`ClaudeFinder` reads `<claude dir>/sessions/*.json` (`CLAUDE_CONFIG_DIR`, else `~/.claude`). For each file, it checks that the pid runs, reads the environment of the pid with `unix.SysctlRaw("kern.procargs2", pid)` and takes `ORCA_TERMINAL_HANDLE`. It returns `[]ClaudeProc{PID, SessionID, Name, Cwd, Handle}`. Two queries: `InTerminal(handle)` and `WithSession(id)`. A missing directory or a file that does not decode gives `ErrUnknown`, not an error of the batch.

Alternative: run `ps eww -p <pid>`. Rejected: one process for each file, and values with spaces break the output. `x/sys/unix` is already a dependency and works with `CGO_ENABLED=0`.

Alternative: ask the agent for its session ID after the primer. Rejected: it costs a turn and it is late.

The finder is used only when the `cli` of the agent is `claude` or starts with `claude`.

### D3. Check before each send

```
stored (H, S, hash)
  finder.InTerminal(H):
    one proc, SessionID == S      → Orca live check of H → send to H
    S == "" and one proc          → adopt its SessionID (rows of version 7) → send
    else finder.WithSession(S):
      exactly one proc, Handle H2 → Orca live check of H2 → store (H2, S) → send to H2
      else                        → new session
  finder unknown                  → Orca live check + agentIdentity != null
```

"Adopt" keeps the 4 live sessions of an upgraded store. The cost: a session that changed before the upgrade is adopted once. The Orca live check still runs in every case: the finder proves the Claude process, and Orca proves that the terminal is writable.

### D4. Follow the session to another terminal

If the user resumes session S in another tab, or Orca restarts and gives new handles, the space keeps its conversation. Two or more processes with S (resumed two times) give a new session, because the target is not clear.

### D5. A linked session gets the primer

The link stores `primer_hash = "pending"` (`agent.PrimerPending`). The driver sends the primer and the batch to that terminal, then stores the real hash. Other hash values keep today's rule: a different hash starts a new session.

### D6. Post-send guard

`sendResult.Prompt.Observation == "unsupported"` returns an error "no agent runs in <handle>". The driver does not press Enter again and deletes the stored session of the space, so the next batch starts a new one. The text is already in the terminal; this only limits the damage when D3 did not see the exit in time.

### D7. Tab name

After `launch` (new and reuse modes) and `worktree create`, the driver runs `orca terminal rename --terminal H --title "<agent name> · <space name>"`. A failure adds a note to the detail and does not fail the delivery. The driver renames only a terminal that it started. `agent.New` takes `config.Space` instead of the space ID, so the driver knows the name.

### D8. Cards record the Claude session

`i2a card add` reads `CLAUDE_CODE_SESSION_ID` (a UUID, else ignored with a warning) into `cards.agent_session`. `Driver.Ask` takes `agent.Terminal{Handle, AgentSession}`. The ask goes to the terminal of the card only when the finder sees `AgentSession` in that terminal, or the finder is unknown and the Orca check passes. Otherwise it goes to the space as a batch does.

### D9. Store schema version 8

`ALTER TABLE sessions ADD COLUMN agent_session TEXT NOT NULL DEFAULT ''` and the same for `cards`, after the backup `inbox.db.v7.bak`, in one transaction. `Sessions.Session` returns `agent.Session{Handle, AgentSession, PrimerHash}`; `SetSession` takes it.

### D10. Agents tab

The agent detail gets a note "Sessions" with one line for each space that uses the agent: space, handle, short Claude session ID, from the store only (no Orca call in the 2 second refresh). The key `s` opens the sessions dialog. `enter` and `x` keep their meanings (edit, delete). The dialog loads the state with the finder and `orca terminal show` in a `tea.Cmd`: handle, Claude session ID, Claude session name, state (`live`, `no Claude`, `other session`, `closed`, `unknown`). Actions on the selected space: `l` link, `u` unlink, `o` open in Orca (`orca terminal switch`).

Link: an input dialog accepts `term_…` or a UUID. The finder resolves the other ID and requires exactly one Claude process. A `cwd` outside the agent `workspace`, or a session that another space has, opens a confirm dialog. The link writes `(H, S, "pending")` to the store. Unlink deletes the row.

The approved sketch used `enter`, `x` and `o` on rows of the detail panel. The form engine has no row lists, and `enter` and `x` are taken in the Agents tab, so the actions move to a dialog. The content is the same.

## Risks / Trade-offs

- [Claude Code changes `~/.claude/sessions/*.json`] → The finder returns `ErrUnknown`, the driver falls back to the Orca check with `agentIdentity`, and the daemon logs one warning.
- [`agentIdentity` stays `"claude"` for some seconds after an exit] → Only the fallback depends on it. D6 limits the damage.
- [The daemon runs as a LaunchAgent] → Same user, so `kern.procargs2` returns the environment. The local e2e scenario runs the real binary.
- [Adopt on upgrade] → One time only, for rows with no Claude session ID.
- [Text of a probe or test reaches a shell] → Tests use fixtures; the e2e scenario sends to a temporary workspace only.

## Migration Plan

1. The first open of the store makes `inbox.db.v7.bak` and upgrades to version 8.
2. The first batch of each space adopts the Claude session of its live terminal.
3. Rollback: stop the daemon, put back `inbox.db.v7.bak` and the earlier binary.

## Open Questions

None. The human approved the decisions on 2026-11-04.
