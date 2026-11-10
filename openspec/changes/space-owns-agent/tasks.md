## 1. First failing tests

- [ ] 1.1 Add `TestMigrateV2OneAgentForEachSpace` in `internal/config` with a fixture of the real shape (4 agents with primers, 4 spaces, Unsorted `agent = ''`, names and paths replaced). Run it on the base commit and keep the failure output in `tmp/evidence/space-owns-agent/`
- [ ] 1.2 Add `TestPausedSpaceDoesNotSend` in `internal/daemon`. Run it on the base commit and keep the failure output

## 2. Configuration version 3

- [ ] 2.1 New types (D1): `Space.Paused`, `Space.Instruction`, `Space.Agent`; the same for `Unsorted`; `Agent.Folder`; `Agent.ID` and `Agent.Name` not in the file; `Agent.Title()` is the description; remove `Config.Agents` and `Config.Agent`
- [ ] 2.2 `migrate_v2.go`: version 2 types, `migrateV2` (D2), the report lines, `config.toml.v2.bak`; `Load` and `convert` handle version 2 like version 1; `migrateV1` builds the version 2 types
- [ ] 2.3 `applyDefaults` sets the agent defaults, `Agent.ID` and `Agent.Name` of each space and of Unsorted; `UnsortedSpace` copies the agent, the instruction and `paused`
- [ ] 2.4 `Validate` (D8); remove `KindAgent`, `SpacesUsing("agent")` and `Replace("agent")`; `config.Item` for a space
- [ ] 2.5 `doc` tags of the new fields; write `docs/config.schema.json` again with `-update`
- [ ] 2.6 Tests: conversion (one agent for each space, shared agent, unused agent, hold, dry-run note, workspace → folder, primer → instruction), version 1 to version 3, validation errors, save and load of version 3

## 3. Daemon, ask, render and drivers

- [ ] 3.1 `Engine.Dispatch` skips a paused space and uses `s.Agent` and `s.Instruction`; the dispatch records `Agent.Title()`; the run summary `batch of <space> (<agent>, <n> messages)`
- [ ] 3.2 `daemon.Run`: next send and log for paused spaces; the API field `agent` is the description or `""`; `docs/daemon-api.yaml`
- [ ] 3.3 `render.Render` and `render.Ask` take the instruction of the space
- [ ] 3.4 `ask.TargetOf` uses the agent of the space; a paused space gives `NoAgentError`
- [ ] 3.5 `agent.TabTitle(space)` gives `i2a · <space>`; the orca driver uses `Folder`
- [ ] 3.6 Tests: paused space, run summary text, tab title, ask of a paused space

## 4. CLI

- [ ] 4.1 `i2a status`: the agent description or "paused" on each space line; no Agents part
- [ ] 4.2 `i2a config --help` and `i2a config check` text for version 3
- [ ] 4.3 `i2a service install` resolves the `orca` executable of each space
- [ ] 4.4 Tests of the status lines and the help

## 5. Dashboard

- [ ] 5.1 Remove the Agents section (`tabAgents`, its keys, list, clicks and help text)
- [ ] 5.2 Spaces panel rows Name, Status, Agent, Session, When (D5); Status toggles `paused`; the rail shows `○` for a paused space
- [ ] 5.3 The agent form of a space in the panel (`m.spaceForm`, `formTarget`), with Folder and Instruction, saved through `commitForm` with the space item
- [ ] 5.4 The Session row and the sessions dialog of one space
- [ ] 5.5 The When dialog: pick, "Rename <schedule>…", "Edit the times of <schedule> in Setup"
- [ ] 5.6 New spaces copy the agent of the selected space or of Unsorted (D6), also in `tab_conversations.go`
- [ ] 5.7 Tests: pause, agent form save and conflict, session row, rename a schedule from the When dialog, new space copies the agent

## 6. End-to-end and documents

- [ ] 6.1 Update the e2e scenarios that name agents or the run summary; add S45 "pause a space" (pause, `ctrl+r`, no dispatch; resume, the message goes) and S46 "convert a version 2 file"; update `SCENARIOS.md`
- [ ] 6.2 Update `CLAUDE.md` (the model paragraph, config, tui, agent) and `README.md` (model, configuration example, upgrade note with the rollback)
- [ ] 6.3 Run `task lint`, `task test`, the full e2e with `I2A_E2E_ORCA=1`, `task leakcheck`; convert a copy of the real configuration with the new binary and read the report
