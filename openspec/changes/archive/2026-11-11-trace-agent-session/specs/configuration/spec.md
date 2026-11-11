## MODIFIED Requirements

### Requirement: Link a session to a space
The Session row of a space SHALL show the stored terminal handle, the Claude session ID, the stored session name and the time of the last send, from the store only. The sessions dialog SHALL show the space, its terminal handle, the Claude session ID, the Claude session name, the stored last send and the state (`live`, `no Claude`, `other session`, `closed`, `no agent` for an agent that is not Claude Code, `unknown`). `o` SHALL show the terminal in Orca with `orca terminal switch`. `l` SHALL open an input for a terminal handle (`term_…`) or a Claude session ID (a UUID). The system SHALL find the other ID from the running Claude processes and SHALL require exactly one Claude process. It SHALL ask for a confirmation when the `cwd` of the process is outside the folder of the agent of the space, or when another space has the same Claude session. The link SHALL store the handle and the Claude session ID for the space. Unlink (`u`) SHALL delete the stored session of the space; the old terminal stays open.

#### Scenario: Link by handle
- **WHEN** the user links `term_c` to the space Triage, and `term_c` runs one Claude process with session `S` in the folder of the agent
- **THEN** the store has `term_c` and `S` for Triage, and the next batch of Triage goes to `term_c`

#### Scenario: Link by Claude session ID
- **WHEN** the user links the Claude session `S`, and `S` runs in `term_c`
- **THEN** the store has `term_c` and `S` for the space

#### Scenario: No Claude process
- **WHEN** the user links `term_d`, and `term_d` runs zsh only
- **THEN** the dialog shows "no running Claude session in term_d", and the store does not change

#### Scenario: Session of another space
- **WHEN** the user links to Unsorted the Claude session that Triage has
- **THEN** a confirmation says that the data of the two spaces goes to one session, and the store changes only after `y`

#### Scenario: Session row with a name
- **WHEN** the store has `term_a`, the Claude session `04ebe19c-…`, the name "i2a · Review" and the last send 09:04 for the space Review
- **THEN** the row Session shows `term_a`, `04ebe19c`, "i2a · Review" and `09:04`
