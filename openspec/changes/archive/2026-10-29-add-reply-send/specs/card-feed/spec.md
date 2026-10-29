## MODIFIED Requirements

### Requirement: Act on a card
A card SHALL become `read` when the user moves the selection to it or presses `enter`, `c`, `o` or `r` on it. The key `c` SHALL copy the suggested reply to the clipboard (`pbcopy` on macOS, OSC 52 on other systems). The key `o` SHALL open the URL of the first reference message that still exists, and only when the URL starts with `http://` or `https://`. The system SHALL NOT open a URL from the card text. The key `r` SHALL open the reply form of the `reply-send` capability. The key `d` SHALL set the state `done` and remove the card from the list. The key `u` SHALL set the state `new`.

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
