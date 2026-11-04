## MODIFIED Requirements

### Requirement: Message file of a source
The system SHALL keep the device keys and the received messages of a WhatsApp source in `<data>/whatsapp-<id>.db` with the file mode 0600. The system SHALL write each received message to this file when it arrives, also messages from a history sync. The system SHALL NOT store messages older than `lookback`. The system SHALL delete stored messages without a file when they are older than `lookback` plus one hour, and stored messages with a file (image, video, document, audio, voice message or sticker) when they are older than `retention`, so that `i2a download` can read them later.

#### Scenario: File mode
- **WHEN** the user pairs the source `phone`
- **THEN** `<data>/whatsapp-phone.db` exists with the mode 0600

#### Scenario: Message received while no poll runs
- **WHEN** a message arrives and the daemon stops before the next poll
- **THEN** the next poll after the restart returns the message

#### Scenario: Document after the lookback
- **WHEN** `lookback` is 1 day, `retention` is 7 days, and a contact sent a document 2 days ago
- **THEN** the row of the document is still in `wa_messages`, and the text messages of the same day are deleted

### Requirement: Message text
The source SHALL use the text of the message. For media, the text SHALL start with a placeholder: `[image]`, `[video]`, `[document: <file name>]`, `[voice message]`, `[audio]`, `[sticker]`, `[contact: <name>]`, `[location: <name>]` or `[poll] <question>`, followed by the caption when it exists. A message that quotes another message SHALL start with one line `> ` and at most 200 characters of the quoted text. The source SHALL NOT download media when it collects; only `i2a download` downloads a file.

#### Scenario: Image with a caption
- **WHEN** a contact sends an image with the caption `plan for Friday`
- **THEN** the message text is `[image] plan for Friday`

#### Scenario: Reply to a message
- **WHEN** a contact replies `yes` to the message `can you join at 3?`
- **THEN** the message text is `> can you join at 3?` and `yes` on the next line

## ADDED Requirements

### Requirement: Send a reply
A WhatsApp source SHALL send a reply as a text message to the chat of the target message. A reply in the place `thread` SHALL quote the target message. The source SHALL use the WhatsApp message ID that the reply gives. When WhatsApp answers with an error, or the source is not paired, the error SHALL mean that no message exists. A time-out or a lost connection SHALL mean that the result is not known.

#### Scenario: Plain reply
- **WHEN** the source sends the reply "ok, 3pm" with the place `channel` to a direct chat
- **THEN** the contact gets "ok, 3pm" with no quote

#### Scenario: Quoted reply in a group
- **WHEN** the source sends the reply "yes" with the place `thread` to a group message of Lan
- **THEN** the group gets "yes" with a quote of the message of Lan

#### Scenario: Not paired
- **WHEN** the phone unlinked i2a and the user sends a reply
- **THEN** the reply is `failed` with the error "not paired: pair again in i2a config"

### Requirement: Download the file of a message
A WhatsApp source SHALL download and decrypt the file of a stored message when `i2a download` asks for it, with the media keys of the stored row. The source SHALL refuse a message without a file and a message whose row no longer exists.

#### Scenario: Row deleted
- **WHEN** retention deleted the row of a document and the agent runs `i2a download --message <id>`
- **THEN** the command exits with code 1 and says that the file is no longer available
