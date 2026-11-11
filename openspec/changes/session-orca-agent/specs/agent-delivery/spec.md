## ADDED Requirements

### Requirement: Orca agent of a session
The driver SHALL record for the stored session of a space the Orca agent that runs it: `claude-agent-teams` when its Claude process has a non-empty `ORCA_AGENT_TEAMS_TEAM_ID` in its environment, else `claude`. It SHALL record it when it starts, adopts or follows a session, and the dashboard SHALL record it when the user links a session. A resume SHALL launch the recorded Orca agent; when none is recorded, it SHALL launch the `cli` of the agent of the space. A new session SHALL launch the `cli`. The driver SHALL NOT pass `--model` to `orca agent launch` or `orca worktree create` for `claude-agent-teams`.

#### Scenario: Resume an agent-teams session
- **WHEN** the stored session of Triage ran in a `claude-agent-teams` terminal that is gone, and the `cli` of Triage is `claude`
- **THEN** the driver runs `orca agent launch --agent claude-agent-teams` with no `--model`, and types `/resume <id>`

#### Scenario: Linked agent-teams session
- **WHEN** the user links a terminal whose Claude process has `ORCA_AGENT_TEAMS_TEAM_ID`
- **THEN** the store has the Orca agent `claude-agent-teams` for the space

### Requirement: Send to an agent that Orca cannot observe
When `orca terminal send` returns `observation: "unsupported"` and i2a reads exactly one interactive Claude process in the terminal, the driver SHALL treat the text as sent to the agent: the result SHALL be `sent` with the detail "Orca cannot observe this agent", and the driver SHALL NOT send Enter again. When no Claude process runs in the terminal, or i2a cannot read the Claude processes, `unsupported` SHALL stay the error "no agent runs in <handle>".

#### Scenario: Batch to an agent-teams session
- **WHEN** the stored terminal runs the stored Claude session under `claude-agent-teams`, and Orca returns `observation: "unsupported"` for the prompt
- **THEN** the dispatch status is `sent`, the detail has "Orca cannot observe this agent", and the store keeps the session

#### Scenario: Send into a shell with the Claude files
- **WHEN** Orca returns `observation: "unsupported"` and no Claude process runs in the terminal
- **THEN** the dispatch status is `failed` with "no agent runs in", and the store has no session for the space
