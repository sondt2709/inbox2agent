## MODIFIED Requirements

### Requirement: One JSON object per message
The system SHALL write each message in a batch as one JSON object on one line, with the fields `id`, `n`, `source`, `source_id`, `channel`, `kind`, `author`, `time`, `url`, `text` and `truncated`. The field `id` SHALL be the row ID of the message in the store. Newlines and control characters in the text SHALL be escaped by the JSON encoder.

#### Scenario: Multi-line message
- **WHEN** a message text has three lines
- **THEN** the batch contains that message on one line, with `\n` escapes in the `text` field

#### Scenario: Message ID in the line
- **WHEN** the message with the row ID 812 is in a batch
- **THEN** its JSON line starts with `{"id":812,`
