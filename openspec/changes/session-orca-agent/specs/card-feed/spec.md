## ADDED Requirements

### Requirement: Terminal of a card
When a card has a terminal handle, the Card panel SHALL show a line with `terminal`, the full handle and the first part of the Claude session ID of the card, after the line of the category and the urgency. The line SHALL NOT cut or wrap the handle when the panel is at least 60 columns wide, so cmd+click in an Orca terminal opens it.

#### Scenario: Card from an agent terminal
- **WHEN** the agent in `term_a1b2…` with the Claude session `04ebe19c-…` added the selected card
- **THEN** the Card panel shows `terminal term_a1b2… · Claude 04ebe19c`
