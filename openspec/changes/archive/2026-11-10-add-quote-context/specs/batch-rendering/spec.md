## MODIFIED Requirements

### Requirement: One JSON object per message
The system SHALL write each message in a batch as one JSON object on one line, with the fields `id`, `n`, `source`, `source_id`, `channel`, `kind`, `author`, `time`, `url`, `text` and `truncated`. The field `id` SHALL be the row ID of the message in the store. Newlines and control characters in the text SHALL be escaped by the JSON encoder. When the message quotes an earlier message or is a reply in a thread, the object SHALL also have the field `reply_to` with `id` (the row ID of the earlier message in the store, or 0 when the store does not have it), `author` (the author of the earlier message, or "" when it is not known or `me` is true), `me` (true when the user wrote the earlier message) and `text` (at most 200 characters of the earlier message, on one line). `reply_to.author` and `reply_to.text` SHALL get the same removal of hidden characters as `author` and `text`. A message that answers no earlier message SHALL have no `reply_to` field.

#### Scenario: Multi-line message
- **WHEN** a message text has three lines
- **THEN** the batch contains that message on one line, with `\n` escapes in the `text` field

#### Scenario: Message ID in the line
- **WHEN** the message with the row ID 812 is in a batch
- **THEN** its JSON line starts with `{"id":812,`

#### Scenario: Answer to a stored message
- **WHEN** the message 900 by `bob` quotes the stored message 812 by `alice` with the text `deploy at 5?`
- **THEN** the line of message 900 has `"reply_to":{"id":812,"author":"alice","me":false,"text":"deploy at 5?"}`

#### Scenario: Answer to a message of the user
- **WHEN** `bob` answers a message of the user that the store does not have
- **THEN** the line has `reply_to` with `id` 0, `author` "", `me` true and the text of the message of the user

#### Scenario: Message that answers nothing
- **WHEN** a top-level message with no quote is in a batch
- **THEN** its line has no `reply_to` field

### Requirement: Digest file
The system SHALL write a Markdown digest file `<data>/digests/<batch-id>.md` for each batch, in all payload modes. The file SHALL contain a header with the space, the agent, the message count, the accounts and the time window, then a statement that the data is untrusted, then one sentence that explains `reply_to`, then the fenced block. The file SHALL have the mode 0600. The explanation of `reply_to` SHALL NOT be in the primer, so that it does not change the primer hash.

#### Scenario: Digest written in dry-run
- **WHEN** a batch with 3 messages of a space is dispatched to a `dry-run` agent
- **THEN** a digest file exists with a header that shows 3 messages and a block with 3 JSON lines

#### Scenario: Primer hash does not change
- **WHEN** a space with a reuse session gets its first batch after the upgrade
- **THEN** the primer hash is the same as before the upgrade, and the driver sends to the existing session
