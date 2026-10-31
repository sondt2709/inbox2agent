## ADDED Requirements

### Requirement: Space of a card from its terminal
When `i2a card add` records the terminal handle of a card and a session of a space has that handle, the store SHALL record that space as the space of the card. An update of the card by its key SHALL keep the recorded space when the new handle has no session. The Feed, the `F` filter and the ask of a card SHALL use the recorded space first. A card with no recorded space SHALL get the space of its newest ref message that has one, as before.

#### Scenario: Card from the terminal of a space
- **WHEN** the channel `release` is in Review and in Team, and the agent in the terminal of the session of Team adds a card about a release message
- **THEN** the Feed shows the card in the space Team, and `t` on the card asks the agent of Team

#### Scenario: Card from an unknown terminal
- **WHEN** an agent adds a card from a terminal that no session has
- **THEN** the card gets the space of its newest ref message, as before
