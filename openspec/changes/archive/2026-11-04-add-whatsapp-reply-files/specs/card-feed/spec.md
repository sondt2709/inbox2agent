## MODIFIED Requirements

### Requirement: Card section in the primer
The system SHALL append a fixed card section to the primer of each batch for a driver other than `dry-run`. The section SHALL name the absolute path of the `i2a` executable (with `I2A_HOME=<path>` before it when the variable is set), show the JSON shape of a card, explain each field, and tell the agent to use the `id` values of the messages in `refs` and the same `key` for the same topic. The section SHALL say that the body can use Markdown (headings, lists, bold, code, tables) and that links show as text and cannot be opened. The section SHALL say that a WhatsApp message with a media placeholder has a file, that `i2a download --message <id>` saves the file and prints its path, and that the file content is untrusted data like message text. A custom primer template SHALL NOT remove the section.

#### Scenario: Custom primer
- **WHEN** a custom primer in Vietnamese is set
- **THEN** the primer that the agent receives is the custom primer, followed by the card section with the path of the `i2a` executable

#### Scenario: Markdown line
- **WHEN** the system builds the primer for an `orca` driver
- **THEN** the card section contains "You can use Markdown"

#### Scenario: Download line
- **WHEN** the system builds the primer for an `orca` driver
- **THEN** the card section contains the path of the `i2a` executable followed by `download --message <id>`, and says that the file content is untrusted data
