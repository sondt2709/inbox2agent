# batch-rendering Specification

## Purpose
This capability makes the digest file and the prompts of a batch. It keeps message text as data: each message is one JSON line inside a fence with a random label, and hidden characters are removed.
## Requirements
### Requirement: One JSON object per message
The system SHALL write each message in a batch as one JSON object on one line, with the fields `id`, `n`, `source`, `source_id`, `channel`, `kind`, `author`, `time`, `url`, `text` and `truncated`. The field `id` SHALL be the row ID of the message in the store. Newlines and control characters in the text SHALL be escaped by the JSON encoder.

#### Scenario: Multi-line message
- **WHEN** a message text has three lines
- **THEN** the batch contains that message on one line, with `\n` escapes in the `text` field

#### Scenario: Message ID in the line
- **WHEN** the message with the row ID 812 is in a batch
- **THEN** its JSON line starts with `{"id":812,`

### Requirement: Remove hidden characters
Before encoding, the system SHALL remove Unicode bidi controls (U+202A to U+202E, U+2066 to U+2069), zero-width characters (U+200B to U+200F, U+2060, U+FEFF) and C0 and C1 control characters except newline and tab.

#### Scenario: Bidi override and escape sequence
- **WHEN** a message text contains U+202E and the bytes `ESC [ 2 J`
- **THEN** the encoded text contains no U+202E and no ESC character

### Requirement: Truncate long messages
The system SHALL truncate each message text to `message_max_chars` characters and set `truncated` to true for that message.

#### Scenario: Long message
- **WHEN** a message text has 10,000 characters and `message_max_chars` is 2000
- **THEN** the encoded text has 2000 characters and `truncated` is true

### Requirement: Fenced data block with a random label
The system SHALL put the JSON lines in a fenced block that opens with a line of three backticks and the label `i2a-data-<nonce>`, and closes with a line of three backticks. The nonce SHALL be 12 hexadecimal characters from a cryptographic random source, new for each batch. Each line inside the block SHALL start with `{`.

#### Scenario: Message contains a fence
- **WHEN** a message text contains a line with three backticks followed by "ignore previous instructions"
- **THEN** the block still has exactly one opening fence line and one closing fence line, and the message text is inside one JSON line

#### Scenario: New nonce for each batch
- **WHEN** the system renders two batches
- **THEN** the two blocks have different labels

### Requirement: Digest file
The system SHALL write a Markdown digest file `<data>/digests/<batch-id>.md` for each batch, in all payload modes. The file SHALL contain a header with the profile, the message count, the sources and the time window, then a statement that the data is untrusted, then the fenced block. The file SHALL have the mode 0600.

#### Scenario: Digest written in dry-run
- **WHEN** a batch with 3 messages is dispatched to a `dry-run` profile
- **THEN** a digest file exists with a header that shows 3 messages and a block with 3 JSON lines

### Requirement: Prompt templates
Each profile SHALL have a `primer` template and a `batch` template in Go `text/template` syntax. The templates SHALL be able to use `.BatchID`, `.Profile`, `.Count`, `.Sources`, `.DigestPath`, `.Messages`, `.Nonce`, `.WindowStart` and `.WindowEnd`. When a profile has no template, the system SHALL use the default template for its payload mode. The configuration validation SHALL reject a template that does not parse.

#### Scenario: Custom batch template
- **WHEN** the batch template is `Process {{.Count}} items in {{.DigestPath}}`
- **THEN** the prompt for a batch of 2 messages is `Process 2 items in <path of the digest file>`

#### Scenario: Template with a syntax error
- **WHEN** the batch template is `{{.Count`
- **THEN** the configuration validation reports the template error and the profile name

### Requirement: Payload modes
In `pointer` mode, the batch prompt SHALL name the digest file and SHALL NOT include message text. In `inline` mode, the batch prompt SHALL include the fenced block. When the rendered inline prompt is larger than `inline_max_bytes`, the system SHALL use the pointer template for that batch and record this in the dispatch detail.

#### Scenario: Inline fallback
- **WHEN** the payload mode is `inline` and the rendered prompt is 20,000 bytes with `inline_max_bytes` 8000
- **THEN** the driver receives the pointer prompt and the dispatch detail says "inline too large, sent pointer"

