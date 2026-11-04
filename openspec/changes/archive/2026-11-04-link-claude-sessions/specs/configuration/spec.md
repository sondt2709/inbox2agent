## MODIFIED Requirements

### Requirement: Agents tab
The Agents tab SHALL list the agents with their driver. The detail panel SHALL show the spaces that use the agent, the sentence "Each space has its own session", one line for each of these spaces with its stored terminal handle and short Claude session ID, and the form: name, driver, CLI, workspace, session, payload, limits and prompts. Delete SHALL follow the requirement "References between items" of the `spaces` capability. For an `orca` agent, the key `s` SHALL open the sessions dialog: one row for each space that uses the agent, with the terminal handle, the Claude session ID, the Claude session name and the state (`live`, `no Claude`, `other session`, `closed`, `no agent` for an agent that is not Claude Code, `unknown`). In the dialog, `l` SHALL link a session to the selected space, `u` SHALL unlink it, and `o` SHALL show its terminal in Orca with `orca terminal switch`.

#### Scenario: Used by
- **WHEN** the user selects the agent `claude-work` that `acme` and `borealis` use
- **THEN** the detail panel shows Acme and Project Borealis as users of the agent, each with its terminal handle

#### Scenario: Sessions dialog
- **WHEN** the user presses `s` on the agent `triage`, and the space Triage has the live terminal `term_a` with Claude session `S`
- **THEN** the dialog shows Triage with `term_a`, the first 8 characters of `S`, the Claude session name and `live`

## ADDED Requirements

### Requirement: Link a session to a space
In the sessions dialog, `l` SHALL open an input for a terminal handle (`term_…`) or a Claude session ID (a UUID). The system SHALL find the other ID from the running Claude processes and SHALL require exactly one Claude process. It SHALL ask for a confirmation when the `cwd` of the process is outside the workspace of the agent, or when another space has the same Claude session. The link SHALL store the handle, the Claude session ID and the primer hash `pending` for the space. Unlink SHALL delete the stored session of the space; the old terminal stays open.

#### Scenario: Link by handle
- **WHEN** the user links `term_c` to the space Triage, and `term_c` runs one Claude process with session `S` in the workspace of the agent
- **THEN** the store has `term_c` and `S` for Triage, and the next batch of Triage sends the primer and the batch to `term_c`

#### Scenario: Link by Claude session ID
- **WHEN** the user links the Claude session `S`, and `S` runs in `term_c`
- **THEN** the store has `term_c` and `S` for the space

#### Scenario: No Claude process
- **WHEN** the user links `term_d`, and `term_d` runs zsh only
- **THEN** the dialog shows "no running Claude session in term_d", and the store does not change

#### Scenario: Session of another space
- **WHEN** the user links to Unsorted the Claude session that Triage has
- **THEN** a confirmation says that the data of the two spaces goes to one session, and the store changes only after `y`
