# file-download Specification

## Purpose
This capability saves the file of a stored message on request, with `i2a download`. The user or the agent runs it; i2a never downloads a file without it. The file goes to `~/Downloads` or to `--dir`, with a safe name and the quarantine attribute, and i2a does not record or delete it.
## Requirements
### Requirement: Download command
The system SHALL provide `i2a download --message <id>` with the optional flag `--dir <dir>`. The command SHALL download the file of the stored message with that ID and write it to `--dir`, or to `~/Downloads` when `--dir` is not set. On success, the command SHALL print the absolute path of the file on stdout and exit with code 0. On an error, it SHALL print the error on stderr, write no file and exit with code 1. The command SHALL download only when it is run; no other part of i2a SHALL download a file.

#### Scenario: Agent downloads a document
- **WHEN** message 1234 is a WhatsApp message with the text `[document: plan.pdf]`, and the agent runs `i2a download --message 1234`
- **THEN** `~/Downloads/plan.pdf` has the bytes of the document, and the output is the path of that file

#### Scenario: Message without a file
- **WHEN** message 1235 is a WhatsApp text message, and the agent runs `i2a download --message 1235`
- **THEN** the command exits with code 1, says that message 1235 has no file, and writes no file

#### Scenario: Unknown message
- **WHEN** the store has no message 99999, and the agent runs `i2a download --message 99999`
- **THEN** the command exits with code 1 and says that the store has no message 99999

#### Scenario: Account that cannot download
- **WHEN** message 812 is a Mattermost message, and the agent runs `i2a download --message 812`
- **THEN** the command exits with code 1 and says that the account of message 812 cannot download files

#### Scenario: No automatic download
- **WHEN** a contact sends an image to a WhatsApp account and the daemon collects and sends the message
- **THEN** no file of that image exists in `~/Downloads` or in the data directory

### Requirement: File name of a download
The file name SHALL be the file name of the message, with hidden characters, `/`, `\` and `:` removed, and without dots at the start. When the message has no file name, or the result is empty, the name SHALL be `whatsapp-<message id>` and the extension of the MIME type (for example `.jpg`). The system SHALL NOT overwrite a file: when the name exists, it SHALL add ` (1)`, ` (2)` and so on before the extension. The file SHALL have the mode 0600.

#### Scenario: Name with a path
- **WHEN** the document name is `../../.zshrc`
- **THEN** the file is `~/Downloads/zshrc` and no file outside `~/Downloads` changes

#### Scenario: Image without a name
- **WHEN** message 1240 is an image of type `image/jpeg` with no file name
- **THEN** the file is `~/Downloads/whatsapp-1240.jpg`

#### Scenario: Name exists
- **WHEN** `~/Downloads/plan.pdf` exists and the agent downloads a document with the name `plan.pdf`
- **THEN** the new file is `~/Downloads/plan (1).pdf` and `~/Downloads/plan.pdf` does not change

### Requirement: Size limit of a download
The system SHALL NOT download a file larger than 100 MB. The command SHALL say the size of the file and the limit.

#### Scenario: Large video
- **WHEN** message 1250 is a video of 300 MB
- **THEN** the command exits with code 1, says "300 MB" and "100 MB", and writes no file

### Requirement: Downloaded file is from the internet
The system SHALL set the extended attribute `com.apple.quarantine` on each downloaded file, so that macOS asks before it opens a program from the file. The system SHALL NOT record the downloaded file and SHALL NOT delete it.

#### Scenario: Quarantine attribute
- **WHEN** the agent downloads a document
- **THEN** `xattr -l <path>` shows `com.apple.quarantine`

#### Scenario: Retention
- **WHEN** retention deletes the message of a downloaded file
- **THEN** the file in `~/Downloads` still exists

