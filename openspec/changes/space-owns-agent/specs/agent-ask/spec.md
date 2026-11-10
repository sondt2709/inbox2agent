## MODIFIED Requirements

### Requirement: Space with no reading agent
When the space of the ask is paused, or its agent uses the `dry-run` driver, the system SHALL NOT record or send the ask, and SHALL say "The space <name> has no agent that reads asks." In the Feed, `t` SHALL open no form and SHALL show this text in the status line. The same rule SHALL apply when the space of the card is not in the configuration, with the text of the requirement "Ask target".

#### Scenario: Dry run in the Feed
- **WHEN** the space Acme uses a `dry-run` agent and the user presses `t` on a card of Acme
- **THEN** no form opens, the status line says "Cannot ask: The space Acme has no agent that reads asks.", and the store has no ask

#### Scenario: Hold with the command
- **WHEN** Unsorted is paused and a user runs `i2a ask --card <key>` with text for a card of Unsorted
- **THEN** the command exits with code 1, says "The space Unsorted has no agent that reads asks.", and the store has no ask
