## 1. Store

- [ ] 1.1 Test first: a version 9 store with 2 sessions opens as version 10 with `inbox.db.v9.bak`, the handles and IDs kept, no name and no last send (fails on the base)
- [ ] 1.2 Schema version 10: `sessions.agent_name`, `sessions.last_sent_at`, the upgrade step with the backup
- [ ] 1.3 `inbox.Session` gets `Name` and `LastSent`; `Session`, `Sessions` and `SetSession` read and write them; `TouchSession(space, handle, name, at)` updates the name and the last send of the stored row when the handle is equal

## 2. Driver

- [ ] 2.1 Tests first (fake runner and fake Claude finder): resume after a closed terminal; resume after `/exit`; no resume after `/clear`; no resume when `S` runs in a process without a handle; resume fails, so a named new session; no resume for a non-Claude CLI and in the `new` mode; the name of a new session; `LastSent` and the name after a send
- [ ] 2.2 `check` returns the reason of a failed check (gone, no Claude, other session, runs elsewhere, unknown)
- [ ] 2.3 `resume`: launch, wait for the agent and `tui-idle`, type `/resume <S>` (UUID pattern), read the session ID each second for at most 10 seconds
- [ ] 2.4 `nameSession`: type `/rename i2a · <space name>` (sanitized, at most 80 characters) and wait for `tui-idle`, for each new Claude session in all modes; a failure is a note
- [ ] 2.5 `reuse` and `startSession` use the resume; the store gets the handle, the session ID and the name; the detail starts with "resumed Claude session" or "could not resume Claude session"
- [ ] 2.6 After each `sent` result in the stored session (batch and ask), call `TouchSession` with the name of the Claude process

## 3. Screens

- [ ] 3.1 `sessionLine`: handle · Claude <S8> · "<name>" · last send <time>; test
- [ ] 3.2 Sessions dialog: the stored name and the last send
- [ ] 3.3 `i2a status`: the `Sessions:` part; test

## 4. End-to-end and documents

- [ ] 4.1 Local Orca scenario: a batch, `orca terminal close`, a second batch: "resumed Claude session", the new terminal runs the stored session ID, and the session name is "i2a · Unsorted"
- [ ] 4.2 Update the e2e texts that the name step or the new status part changes
- [ ] 4.3 `CLAUDE.md`: the facts of `/resume` and `/rename` (Claude Code 2.1.322, Orca 1.4.253), the arguments of `orca agent launch`, schema version 10, the resume rule; `docs/daemon-api.yaml` if a field changes
- [ ] 4.4 `task lint`, `task test`, `task leakcheck`, the Orca e2e scenarios; evidence in `tmp/evidence/trace-agent-session/`
