## ADDED Requirements

### Requirement: Reference to a quoted message
When the source stores a message that quotes another message, it SHALL keep the ID of the quoted message (`stanzaID`), whether the user wrote the quoted message (`participant` is the user), the author of the quoted message (the contact name of `participant`, else `+<phone number>`, else "") and at most 200 characters of the quoted text. The author SHALL be "" when the user wrote the quoted message. The source SHALL give these values to the store as the reply reference of the message. A message without a quote SHALL have no reply reference.

#### Scenario: Quote of a message of a contact
- **WHEN** `bob` quotes the message `deploy at 5?` of `alice` in a group and writes `yes`
- **THEN** the collected message has the reply reference with the ID of the message of `alice`, the author `alice`, `me` false and the text `deploy at 5?`

#### Scenario: Quote of a message of the user
- **WHEN** a contact quotes a message of the user in a direct chat
- **THEN** the collected message has the reply reference with `me` true and the author ""

#### Scenario: Message stored before the upgrade
- **WHEN** the source collects a row that was stored before the file had the quote columns
- **THEN** the message has no reply reference, and the text keeps its `> ` line
