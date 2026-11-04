## MODIFIED Requirements

### Requirement: Ask target
The space of an ask SHALL be the space of the newest ref message of the card that has a space, or else the space that lists the conversation of the newest ref message, or else Unsorted. The agent of the ask SHALL be the agent of that space. The ask SHALL go to the terminal that wrote the card when the card has a terminal, Orca reports it live, and the terminal still runs the Claude session of the card as the requirement "Claude session of a terminal" of `agent-delivery` says. Otherwise the ask SHALL go to the agent of the space as a batch does: the reuse session of the space when it is valid and has the current primer, else a new session (or a new worktree in `worktree` mode) with the primer and the ask. The system SHALL record the space and the terminal that got the ask.

#### Scenario: Terminal of the card is live
- **WHEN** card 4 was written in terminal `term_a` by Claude session `S`, and `term_a` is live and runs `S`
- **THEN** the ask goes to `term_a`, without the primer

#### Scenario: Terminal of the card runs another session
- **WHEN** card 4 was written in `term_a` by Claude session `S`, and `term_a` now runs Claude session `T`
- **THEN** the ask does not go to `term_a`; it goes to the session of the space

#### Scenario: Terminal of the card is closed
- **WHEN** the terminal of the card is closed, and the space `acme` uses an agent in `reuse` mode with the live session `term_b`
- **THEN** the ask goes to `term_b`

#### Scenario: No session
- **WHEN** the card has no terminal, and the space has no live session
- **THEN** i2a starts a new session, submits the primer and the ask, and stores the new session of the space

#### Scenario: Space not in the configuration
- **WHEN** the ref messages of the card have the space `old`, and the configuration has no space `old`
- **THEN** nothing goes to an agent, and the error says that the space `old` is not in the configuration
