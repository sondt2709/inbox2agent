## Why

The configuration couples three concerns in one `[[sources]]` entry: the login (url, token), the selection (`all_direct`, `mentions`, `channels`) and the route (`profile`, `channel_profiles`). The user cannot split one account into work, project and personal groups, a DM and a channel have different settings, and the words Source, Profile and Schedule do not tell the user what each item does. A sample configuration also shows a defect: a profile that only `channel_profiles` uses shows "used by 0 sources", and its delete fails with a validation error.

## What Changes

- New model with five items, each defined one time:
  - **Account**: one login to a platform. It only connects. It has no selection and no route.
  - **Conversation**: one DM, group or channel of an account. All kinds have the same settings: one space and one read level (`all`, `mentions`, `off`).
  - **Space**: a group of conversations that the user names (a company, a project, personal). It can hold conversations from any account. It is the only item that links the others: its conversations go to one agent on one schedule.
  - **Agent**: how to send a batch (the old profile). One agent can serve many spaces. Each space has its own session.
  - **Schedule**: when a space sends (every, days, hours). Named and reusable.
- Built-in space **Unsorted**: it holds each conversation that no space lists. It has a read level for these conversations, and an agent or "hold".
- The `mentions` read level means "messages for you": a mention, a reply to you, or each message of a direct conversation.
- **BREAKING** Configuration file version 2: `[[accounts]]`, `[[agents]]`, `[[schedules]]`, `[[spaces]]`, `[unsorted]`, `[settings]`. The keys `[[sources]]`, `[[profiles]]` and `[timing]` go away. `i2a` migrates a version 1 file automatically, keeps a backup and prints a report.
- **BREAKING** The daemon sends each space on its own schedule. One dispatch is one space. The global `interval` and `active_hours` go away.
- **BREAKING** Store schema version 6: the `profile` columns become `space`; dispatches also record the `agent`. A backup is made before the migration.
- **BREAKING** CLI: `i2a source ...` becomes `i2a account ...`; `i2a source channels` becomes `i2a conversations`; new `i2a conversation set`. `i2a status` prints the spaces with their agent, schedule and next send.
- **BREAKING** Prompt templates: `{{.Sources}}` becomes `{{.Accounts}}`; new `{{.Space}}`. The migration rewrites custom templates.
- TUI tabs: `1 Feed  2 Activity │ 3 Accounts → 4 Conversations → 5 Spaces → 6 Agents + 7 Schedules │ 8 Settings`. The Overview tab goes away. Its service keys move to Settings, and its message counts move to Activity. The Feed shows and filters the space of each card.
- Delete of an agent or a schedule that a space uses asks for a replacement. This removes the defect above.
- README, CLAUDE.md, `test/e2e/SCENARIOS.md` and `docs/demo/record.sh` describe the new model.

## Capabilities

### New Capabilities
- `spaces`: accounts, conversations, read levels, spaces, the Unsorted space, agents and schedules as separate items; the route from a conversation to a space; the version 1 migration.

### Modified Capabilities
- `configuration`: the file schema, validation, the TUI tabs and forms, the conversation picker, and the non-interactive commands.
- `daemon-service`: the dispatch time comes from the schedule of each space; active hours move into schedules; `i2a status`.
- `agent-delivery`: routes go to spaces, one batch for each space, sessions and locks for each space.
- `message-store`: schema version 6 (`space` columns, `dispatches.agent`).
- `mattermost-source`: the read level of each channel replaces `all_direct`, `mentions` and `channels`.
- `whatsapp-source`: the read level of each chat replaces `all_direct`, `mentions` and `channels` (from `add-whatsapp-source`).
- `batch-rendering`: template fields `.Accounts` and `.Space`.
- `agent-ask`: the target of an ask is the agent of the space of the card.
- `card-feed`: the space of a card in the list, the detail and the filter.

## Impact

- Code: `internal/config` (new schema, migration, `Schedule.Next`, `Route`), `internal/daemon` (loop and dispatch for each space), `internal/agent` (route, lock), `internal/ask`, `internal/render`, `internal/store` (schema version 6), `internal/source/mattermost` and `internal/source/whatsapp` (read levels), `internal/cli` (account, conversations, status), `internal/tui` (all tabs except Feed are new or rewritten).
- Data: `config.toml` is rewritten once (backup `config.toml.v1.bak`). `inbox.db` migrates to version 6 (backup before the migration).
- Users: custom primer and batch templates are rewritten. Scripts that call `i2a source` must change.
- Order: this change starts after `add-whatsapp-source` merges to `main`.
- No new dependency. No CI change.
