## MODIFIED Requirements

### Requirement: Terminal of the card
`i2a card add` SHALL record the value of the environment variable `ORCA_TERMINAL_HANDLE` as the terminal of each card of the call, when the value matches `term_` followed by 1 to 64 letters, digits or hyphens. It SHALL record the value of `CLAUDE_CODE_SESSION_ID` as the Claude session of each card of the call, when the value is a UUID. It SHALL ignore another value of either variable and print a warning on stderr. Card JSON SHALL NOT set the terminal or the Claude session. An update by key SHALL replace the terminal and the Claude session when the call has them. A call that changes only the terminal or the Claude session SHALL count the card as unchanged and keep its state.

#### Scenario: Agent in an Orca terminal
- **WHEN** Claude session `04ebe19c-543e-42ff-8f84-ea13cea7a3ec` in the Orca terminal `term_b9f22da8-13d6-49bc-a1ab-53eba56c9578` runs `i2a card add`
- **THEN** each card of the call has that terminal and that Claude session

#### Scenario: Bad value
- **WHEN** `ORCA_TERMINAL_HANDLE` is `term_x; rm -rf /`
- **THEN** the cards have no terminal, and stderr has a warning
