## MODIFIED Requirements

### Requirement: Card section in the primer
The system SHALL append a fixed card section to the primer of each batch for a driver other than `dry-run`. The section SHALL name the absolute path of the `i2a` executable (with `I2A_HOME=<path>` before it when the variable is set), show the JSON shape of a card, explain each field, and tell the agent to use the `id` values of the messages in `refs` and the same `key` for the same topic. The section SHALL say that the body can use Markdown (headings, lists, bold, code, tables) and that links show as text and cannot be opened. A custom primer template SHALL NOT remove the section.

#### Scenario: Custom primer
- **WHEN** a custom primer in Vietnamese is set
- **THEN** the primer that the agent receives is the custom primer, followed by the card section with the path of the `i2a` executable

#### Scenario: Markdown line
- **WHEN** the system builds the primer for an `orca` driver
- **THEN** the card section contains "You can use Markdown"

## ADDED Requirements

### Requirement: Card body in Markdown
The Feed SHALL render the body of the selected card as Markdown (CommonMark and GitHub tables), wrapped to the width of the detail panel. Links and images SHALL show as text; the output SHALL contain no terminal hyperlink sequence and no other OSC sequence from card text. Raw HTML SHALL NOT show. The title and the suggested reply SHALL stay plain text. When the render fails, the Feed SHALL show the plain body. The Feed SHALL NOT render a body again when the card, its update time and the panel width are the same.

#### Scenario: Formatted body
- **WHEN** a card body is `**Blocked**: QA needs sign-off\n\n- item one\n- item two`
- **THEN** the detail panel shows "Blocked" in bold, no `**` marks, and two list items with bullets

#### Scenario: Link in a body
- **WHEN** a card body contains `[click](https://evil.example/x)`
- **THEN** the detail panel shows the text and the URL, and the rendered text contains no `ESC ] 8` sequence

#### Scenario: Raw HTML
- **WHEN** a card body contains `<script>alert(1)</script>`
- **THEN** the detail panel does not show the tag or `alert(1)`

#### Scenario: Reply stays plain
- **WHEN** the suggested reply is `**yes**, ship it`
- **THEN** the reply panel shows `**yes**, ship it`, and `c` copies the same text
