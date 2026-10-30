## MODIFIED Requirements

### Requirement: Ask target
The space of an ask SHALL be the space of the newest ref message of the card that has a space, or else the space that lists the conversation of the newest ref message, or else Unsorted. The agent of the ask SHALL be the agent of that space. The ask SHALL go to the terminal that wrote the card when the card has a terminal and Orca reports it live. Otherwise the ask SHALL go to the agent of the space as a batch does: the reuse session of the space when it is live and has the current primer, else a new session (or a new worktree in `worktree` mode) with the primer and the ask. The system SHALL record the space and the terminal that got the ask.

#### Scenario: Terminal of the card is live
- **WHEN** card 4 was written in terminal `term_a` and `term_a` is live
- **THEN** the ask goes to `term_a`, without the primer

#### Scenario: Terminal of the card is closed
- **WHEN** the terminal of the card is closed, and the space `acme` uses an agent in `reuse` mode with the live session `term_b`
- **THEN** the ask goes to `term_b`

#### Scenario: No session
- **WHEN** the card has no terminal, and the space has no live session
- **THEN** i2a starts a new session, submits the primer and the ask, and stores the new session of the space

#### Scenario: Space not in the configuration
- **WHEN** the ref messages of the card have the space `old`, and the configuration has no space `old`
- **THEN** nothing goes to an agent, and the error says that the space `old` is not in the configuration

## REMOVED Requirements

### Requirement: Dry-run profile
**Reason**: An ask now goes to the agent of a space; a space can also have no agent (Unsorted on hold).
**Migration**: See "Space with no reading agent" below.

## ADDED Requirements

### Requirement: Space with no reading agent
When the space of the ask has no agent, or its agent uses the `dry-run` driver, the system SHALL NOT record or send the ask, and SHALL say "The space <name> has no agent that reads asks." In the Feed, `t` SHALL open no form and SHALL show this text in the status line. The same rule SHALL apply when the space of the card is not in the configuration, with the text of the requirement "Ask target".

#### Scenario: Dry run in the Feed
- **WHEN** the space Acme uses a `dry-run` agent and the user presses `t` on a card of Acme
- **THEN** no form opens, the status line says "Cannot ask: The space Acme has no agent that reads asks.", and the store has no ask

#### Scenario: Hold with the command
- **WHEN** Unsorted has no agent and a user runs `i2a ask --card <key>` with text for a card of Unsorted
- **THEN** the command exits with code 1, says "The space Unsorted has no agent that reads asks.", and the store has no ask
