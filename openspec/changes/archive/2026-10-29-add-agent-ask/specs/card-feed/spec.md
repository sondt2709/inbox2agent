## ADDED Requirements

### Requirement: Terminal of the card
`i2a card add` SHALL record the value of the environment variable `ORCA_TERMINAL_HANDLE` as the terminal of each card of the call, when the value matches `term_` followed by 1 to 64 letters, digits or hyphens. It SHALL ignore another value and print a warning on stderr. Card JSON SHALL NOT set the terminal. An update by key SHALL replace the terminal when the call has one. A call that changes only the terminal SHALL count the card as unchanged and keep its state.

#### Scenario: Agent in an Orca terminal
- **WHEN** an agent in the Orca terminal `term_b9f22da8-13d6-49bc-a1ab-53eba56c9578` runs `i2a card add`
- **THEN** each card of the call has that terminal

#### Scenario: Bad value
- **WHEN** `ORCA_TERMINAL_HANDLE` is `term_x; rm -rf /`
- **THEN** the cards have no terminal, and stderr has a warning

## MODIFIED Requirements

### Requirement: Act on a card
A card SHALL become `read` when the user moves the selection to it or presses `enter`, `c`, `o`, `r` or `t` on it. The key `c` SHALL copy the suggested reply to the clipboard (`pbcopy` on macOS, OSC 52 on other systems). The key `o` SHALL open the URL of the first reference message that still exists, and only when the URL starts with `http://` or `https://`. The system SHALL NOT open a URL from the card text. The key `r` SHALL open the reply form of the `reply-send` capability. The key `t` SHALL open the ask form of the `agent-ask` capability. The key `d` SHALL set the state `done` and remove the card from the list. The key `u` SHALL set the state `new`.

#### Scenario: Copy and open
- **WHEN** the user selects an action card that refers to message 812 and presses `c` and then `o`
- **THEN** the clipboard has the suggested reply, the browser opens the URL of message 812, and the card is `read`

#### Scenario: Link in the card body
- **WHEN** a card body contains `https://evil.example` and the card has no reference
- **THEN** `o` opens nothing and the status line says that the card has no message link

#### Scenario: Done
- **WHEN** the user presses `d` on a card
- **THEN** the card leaves the list, and `i2a card list --json` does not contain it

#### Scenario: Reply key
- **WHEN** the user presses `r` on a `new` card that refers to message 812
- **THEN** the card is `read` and the reply form shows the target message 812

#### Scenario: Ask key
- **WHEN** the user presses `t` on a `new` card
- **THEN** the card is `read` and the ask form shows the agent that gets the ask
