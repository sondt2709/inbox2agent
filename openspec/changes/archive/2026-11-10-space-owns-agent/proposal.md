## Why

In the real configuration of the user, each of the 4 agents serves exactly one space. The agent holds the folder, the instruction and the session mode of that space, so the agent acts as the workspace, and the space only groups conversations (GitHub issue #20). The user also cannot pause a space: the space `iso` uses the `dry-run` driver as a workaround. The space panel cannot rename or edit its schedule.

## What Changes

- **BREAKING**: configuration version 3. The fields of `[[agents]]` move into each space as `[spaces.agent]`: `driver`, `cli`, `folder` (the old `workspace`), `session`, `payload`, `inline_max_bytes`, `message_max_chars`, `model`, `orca_bin`. The agent `primer` becomes `instruction` of the space. The list `[[agents]]` and the key `agent` of a space are removed. `[unsorted]` gets the same `agent`, `instruction` and `paused`.
- A space gets `paused` (default false). A paused space does not send; its messages wait and expire after `lookback`. This replaces the hold of Unsorted (`agent = ''`).
- `Load` converts a version 2 file to version 3, keeps `config.toml.v2.bak` and reports each change: an agent that two spaces used is copied into each; an agent that no space used is removed; Unsorted with no agent is paused. A version 1 file converts through version 2.
- The dashboard has no Agents section. The Spaces panel of a space has the rows Name, Status (sending or paused), Agent (a form in the panel: driver, CLI, folder, session, model, payload, limits, instruction), Session (the stored terminal and Claude session; `s` opens the sessions dialog of the space) and When.
- The When dialog can pick a schedule, rename the schedule, and open its times in Setup › Schedules.
- A new space copies the agent of the selected space (or of Unsorted), with no instruction.
- The dispatch records the agent as its description (`dry-run`, or `orca · claude`). The run summary says `batch of <space> (<agent>, <n> messages)`. A new Orca tab is `i2a · <space>`. `i2a status` shows the agent and the pause of each space and has no Agents part.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `spaces`: spaces own their agent and instruction; paused spaces; references without agents; migration from version 2; the version 1 migration ends at version 3.
- `configuration`: validation of version 3; no Agents section; Spaces area rows (Status, Agent form, Session, When with rename); Everything else rows; the sessions dialog of one space; `i2a config --help`.
- `agent-delivery`: the tab name of a new session; a paused space never sends; the dispatch records the agent description.
- `agent-ask`: a paused space or a `dry-run` agent reads no asks.
- `daemon-service`: `i2a status` shows the agent and the pause of each space; the API field `agent` is the agent description, empty for a paused space.

## Impact

- Code: `internal/config` (types, `migrate_v2.go`, `migrate_v1.go`, validation, schema), `internal/daemon` (engine, loop, API), `internal/ask`, `internal/agent` (tab title, workspace → folder), `internal/render` (instruction of the space), `internal/cli` (status, config help, service), `internal/tui` (Spaces panel, agent form, When dialog, sessions dialog, Agents section removed).
- Configuration file: version 3, `config.schema.json`, backup `config.toml.v2.bak`. An older i2a cannot read a version 3 file.
- Store: no schema change. `dispatches.agent` gets the agent description for new dispatches.
- Daemon API: the value of `agent` in `/v1/status` and the events changes (`docs/daemon-api.yaml`).
- Rollback: copy `config.toml.v2.bak` to `config.toml` and install the older binary.
