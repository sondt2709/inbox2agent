# whatsapp-source Specification

## Purpose
This capability collects the messages of a personal WhatsApp account. i2a connects as a linked device, stores each received message in the file of the source, and collects the selected chats, the direct chats and the group mentions. It only reads: it sends no messages, no read receipts and no presence.

## Requirements
### Requirement: Pair as a linked device
The system SHALL connect a WhatsApp source as a linked device of the personal account of the user. Pairing SHALL show a QR code that changes when WhatsApp sends a new code, or, when the user gives a phone number in international format, one 8-character pairing code. The device name on the phone SHALL be "i2a". Pairing SHALL stop when WhatsApp stops giving codes (after about 160 seconds) with no scan. When the window is too short for the QR code, the TUI SHALL say how many rows it needs.

#### Scenario: Pair with a QR code
- **WHEN** the user adds a WhatsApp source with no phone number and scans the QR code with the phone
- **THEN** the system shows "Paired" and the list of chats, and the phone lists "i2a" in Linked devices

#### Scenario: Pair with a phone code
- **WHEN** the user adds a WhatsApp source with the phone number `+15551234567`
- **THEN** the system shows an 8-character code and the steps to type it on the phone

#### Scenario: No scan
- **WHEN** the user does not scan the code for 160 seconds
- **THEN** the system stops the pairing, shows "Pairing timed out", and saves no source

### Requirement: Message file of a source
The system SHALL keep the device keys and the received messages of a WhatsApp source in `<data>/whatsapp-<id>.db` with the file mode 0600. The system SHALL write each received message to this file when it arrives, also messages from a history sync. The system SHALL NOT store messages older than `lookback`, and SHALL delete stored messages older than `lookback` plus one hour.

#### Scenario: File mode
- **WHEN** the user pairs the source `phone`
- **THEN** `<data>/whatsapp-phone.db` exists with the mode 0600

#### Scenario: Message received while no poll runs
- **WHEN** a message arrives and the daemon stops before the next poll
- **THEN** the next poll after the restart returns the message

### Requirement: Connection
The daemon SHALL open the connection of a WhatsApp source on the first poll and SHALL keep it open between polls. Only one process SHALL hold the connection of a source; the holder SHALL lock `<data>/whatsapp-<id>.lock`. On the first poll after a connect, the source SHALL wait for the messages that arrived while it was offline, for at most 30 seconds. `i2a run --once` SHALL connect, collect and disconnect.

#### Scenario: Messages while i2a was off
- **WHEN** the daemon is stopped, a contact sends 2 direct messages, and the user runs `i2a run --once`
- **THEN** the run collects the 2 messages

#### Scenario: Daemon holds the connection
- **WHEN** the daemon runs with a WhatsApp source
- **THEN** `<data>/whatsapp-<id>.lock` is locked by the daemon process

### Requirement: Collect messages from selected chats
A WhatsApp source SHALL collect the messages that arrived after the cursor in these chats:

- the selected chats in `channels`, which can be direct chats and groups;
- all direct chats, when `all_direct` is on;
- any group, but only messages that mention the user or quote a message of the user, when `mentions` is on.

The source SHALL skip messages of the user, status updates, broadcast lists, WhatsApp channels (newsletters), reactions, edits, deletes and poll votes.

#### Scenario: Direct message
- **WHEN** `all_direct` is on and a contact sends a direct message
- **THEN** the next poll returns the message with the kind `direct`, the contact name as the author and as the channel name

#### Scenario: Selected group
- **WHEN** a member posts in a group that is in `channels`
- **THEN** the next poll returns the message with the kind `group` and the group name

#### Scenario: Group that is not selected
- **WHEN** `mentions` is on and a member posts in a group that is not selected, with no mention and no quote of the user
- **THEN** the poll does not return the message

#### Scenario: Mention in a group that is not selected
- **WHEN** `mentions` is on and a member posts `@<user> please check` with a WhatsApp mention of the user in a group that is not selected
- **THEN** the poll returns the message

#### Scenario: Quote of a message of the user
- **WHEN** `mentions` is on and a member replies to a message of the user in a group that is not selected
- **THEN** the poll returns the reply

#### Scenario: Selected direct chat only
- **WHEN** `all_direct` and `mentions` are off and the direct chat with one contact is selected
- **THEN** the poll returns the messages of that contact only

#### Scenario: Own message
- **WHEN** the user sends a message from the phone in a selected chat
- **THEN** the poll does not return the message

#### Scenario: Status update
- **WHEN** a contact posts a status update
- **THEN** the poll does not return it

### Requirement: Message text
The source SHALL use the text of the message. For media, the text SHALL start with a placeholder: `[image]`, `[video]`, `[document: <file name>]`, `[voice message]`, `[audio]`, `[sticker]`, `[contact: <name>]`, `[location: <name>]` or `[poll] <question>`, followed by the caption when it exists. A message that quotes another message SHALL start with one line `> ` and at most 200 characters of the quoted text. The source SHALL NOT download media.

#### Scenario: Image with a caption
- **WHEN** a contact sends an image with the caption `plan for Friday`
- **THEN** the message text is `[image] plan for Friday`

#### Scenario: Reply to a message
- **WHEN** a contact replies `yes` to the message `can you join at 3?`
- **THEN** the message text is `> can you join at 3?` and `yes` on the next line

### Requirement: Message link
The URL of a message from a direct chat SHALL be `https://wa.me/<phone number>` when the phone number of the chat is known. The URL of a group message SHALL be empty.

#### Scenario: Open a direct chat
- **WHEN** the user presses `o` on a card that refers to a direct message from a contact with a known phone number
- **THEN** the system opens `https://wa.me/<phone number>`

#### Scenario: Open a group message
- **WHEN** the user presses `o` on a card that refers to a group message only
- **THEN** the status line says that the card has no message link

### Requirement: Read state does not change
The source SHALL NOT send read receipts, typing state or online presence to WhatsApp.

#### Scenario: Unread chat stays unread
- **WHEN** a contact sends a direct message and the daemon collects it
- **THEN** the chat is still unread on the phone, and the contact does not see blue ticks

### Requirement: Not paired
When the file of a source has no device keys, because the source was never paired or because the phone unlinked it, a poll SHALL fail with the error "not paired: pair again in i2a config". `i2a status` and the Sources tab SHALL show the state `paired` or `not paired` for each WhatsApp source.

#### Scenario: Phone unlinks i2a
- **WHEN** the user removes "i2a" from Linked devices on the phone while the daemon runs
- **THEN** the log shows "not paired" for the source, and `i2a status` shows `not paired`

