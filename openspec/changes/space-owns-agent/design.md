## Context

- Version 2 (`internal/config/config.go`): `[[agents]]` with `id`, `name`, `driver`, `cli`, `workspace`, `session`, `payload`, limits, `model`, `orca_bin`, `primer` (the instruction since change A) and `batch` (not used). A space names one agent by ID; Unsorted names one agent or `''` (hold).
- Real configuration of the user (read-only check, 2026-11-11): 4 agents, 4 spaces, each agent used by one space; Unsorted has `agent = ''`; the space `iso` uses a `dry-run` agent to stop its sends.
- The agent ID appears in: `dispatches.agent`, the run summary (`batch of Unsorted to triage (dry-run, 3 messages)`), the Orca tab name (`<agent> · <space>`), the daemon log and the API field `agent`, `i2a status`, errors (`agent "triage": …`).
- The dashboard edits agents in Setup › Agents (master-detail form) and picks the agent of a space in a pick dialog. The Feed already shows forms (reply, ask) in its Card panel through `m.form` and `formTarget`.
- The store keys sessions by space (`sessions.space`), so sessions do not change.

## Goals / Non-Goals

**Goals:**

- One item, the space, holds its conversations, its agent, its instruction, its schedule and its pause.
- A safe automatic conversion of version 2 files, with a backup and a report.
- The dashboard edits the agent where the space is.

**Non-Goals:**

- Renaming "space" (decision 2 of the gate: keep the term).
- Changing schedules into a part of the space: schedules stay shared items (`default` serves 3 spaces in the real file).
- Changing the ID of a schedule (decision 10 of the gate: names only).
- Storing the Claude session name or resuming a session (change C).

## Decisions

### D1. Version 3 types

```go
type Space struct {
    ID, Name      string
    Paused        bool   `toml:"paused,omitempty"`
    Schedule      string
    Instruction   string `toml:"instruction,multiline,omitempty"`
    Agent         Agent  `toml:"agent"`
    Conversations []Conversation
}
type Unsorted struct {
    Paused bool; Schedule, Read, Instruction string; Agent Agent; Conversations []Conversation
}
type Agent struct {
    ID   string `toml:"-"` // the ID of the space; set at load
    Name string `toml:"-"` // the name of the space
    Driver, CLI, Folder, Session, Payload string
    InlineMaxBytes, MessageMaxChars int
    Model, OrcaBin string
}
```

- `paused` and not `enabled`: a missing key must mean "sends", and a Go `bool` is false when the key is missing.
- `Agent` stays a type, because the drivers, the renderer and the ask flow take `config.Agent`. `ID` and `Name` are not in the file: `applyDefaults` and `UnsortedSpace` set them from the space, so the session code (keyed by space) does not change.
- `folder` replaces `workspace`: in Orca, "workspace" is another item, and the user called the space a workspace.
- `Agent.Title()` is the description: `dry-run`, or `orca · <cli>`.

Alternative: keep `[[agents]]` as shared presets and add `instruction` and `folder` to the space. Rejected in the gate (decision 3): the presets are one-to-one in practice and the two places confuse.

### D2. Conversion of a version 2 file

`migrate_v2.go` holds the version 2 types (`configV2`, `agentV2`, `spaceV2`, `unsortedV2`) and `migrateV2(b) (*Config, error)`:

1. Each space gets a copy of the settings of its agent; `instruction` = the agent `primer`; `paused = false`.
2. Unsorted: `agent = ''` gives `paused = true` and the agent of the first space (else `NewAgent` defaults); an agent ID gives a copy of that agent.
3. An agent that two or more spaces use: each space gets a copy. Report: `Agent "x": copied into the spaces a and b. A change of one space no longer changes the other.`
4. An agent that no space uses: removed. Report: `Agent "x" was used by no space; i2a removed it. Its settings are in config.toml.v2.bak.`
5. A space with a `dry-run` agent: report `Space "x" uses the dry-run driver. To stop its sends and keep its agent, set paused = true.`
6. `Load` writes the old bytes to `config.toml.v2.bak` (mode 0600) under the write lock, saves version 3, and returns `Config.Migration`, as the version 1 conversion does.

`migrateV1` builds `configV2` (it used the v2 types before) and `Parse` runs `migrateV2` on it, so a version 1 file ends at version 3 with both reports. Only `config.toml.v1.bak` is written for a version 1 file.

### D3. Removed items

`Config.Agents`, `Config.Agent(id)`, `KindAgent`, `SpacesUsing("agent")`, `Replace("agent")`, the Agents section (`tab_agents.go` is removed; `agent_form.go` has the agent form of a space, the schedule delete with a replacement, and `usedBy`), the agent pick dialog, the "agents" count of the daemon log, and the Agents part of `i2a status`.

### D4. Paused spaces

- The engine skips a paused space in `Dispatch`, as it skipped a space with no agent. `daemon.Run` logs `space does not send` and the API gives no next send for it.
- Its messages stay `pending` and expire after `lookback` (`ExpirePending`, unchanged).
- An ask about a card of a paused space is refused with `NoAgentError` ("The space <name> has no agent that reads asks."), as for `dry-run`.
- The rail shows `○` for a paused space and `●` for a space that sends.

### D5. The Spaces panel of a space

Rows, in order: Name, Status, Agent, Session, When, the last send, then the conversations.

- **Status**: "Sending" or "Paused: messages wait until they expire after <lookback>". `enter` toggles and saves at once.
- **Agent**: `orca · claude · ~/notes · reuse` (or `dry-run`). `enter` opens the agent form in the panel, as the Feed shows the reply form: `m.spaceForm = <space ID>` and `formTarget` returns `space:<id>` with `agentForm(space)`. Fields: Driver, the Orca warning, CLI, Folder, Session, Model, Payload, Inline limit, Message limit, Instruction. "Save changes" commits through `commitForm` with the item `KindSpace`/`<id>` (the conflict check of other processes). `esc` leaves the form and shows the rows again.
- **Session**: the stored handle and the first 8 characters of the Claude session (or "none"). `enter` or `s` opens the sessions dialog of this one space (`sessionsDialog` with one space). The row reads the store only; the dialog runs `orca`.
- **When**: the pick dialog lists the schedules, then "Rename <schedule>…" (the input dialog; it changes `name` only) and "Edit the times of <schedule> in Setup" (opens Setup › Schedules with that schedule and its form).
- Everything else gets the same rows (Status is "Hold" or "Sending") and keeps its "New and other conversations read" row.

### D6. New spaces

`newSpace` copies the `Agent` of the selected space (else of Unsorted) and the schedule of the first schedule; no instruction; not paused. The text of the new space dialog says this.

### D7. Names in outputs

- `dispatches.agent` = `Agent.Title()`.
- Run summary: `batch of <space> (<agent title>, <n> messages): <status>`.
- Tab title: `i2a · <space name>` (`TabTitle(space)`).
- Daemon log and API `agent`: the agent title; `""` for a paused space.
- Errors name the space: `space "triage".agent.folder: the orca driver needs a folder`.

### D8. Validation

- Each space and Unsorted: `schedule` exists; agent fields as before (`driver`, `session`, `payload`, limits; `folder` and `cli` for `orca`); `instruction` without three backticks.
- `spaces[i].id` rules unchanged; the reference to an agent is gone.

## Risks / Trade-offs

- [The conversion changes the file of the user automatically] → backup `config.toml.v2.bak`, the report on stderr and in the daemon log, and a migration test with a fixture of the real shape (4 agents, 4 spaces, hold).
- [An older i2a cannot read version 3] → the README says to copy the backup back before a downgrade.
- [A shared agent becomes two copies] → the report names both spaces.
- [The dispatch `agent` column mixes old IDs and new descriptions] → the Send log shows the column as text only; no code joins it to a configuration item.
- [Tests and e2e scenarios that name agents change in many files] → the CLAUDE.md model paragraph and the e2e scenario text change in the same commit.

## Migration Plan

1. Release as `feat!` (major). The first load converts the file and prints the report.
2. Rollback: install the older binary, copy `config.toml.v2.bak` to `config.toml`, run `i2a service install`.

## Open Questions

None.
