## ADDED Requirements

### Requirement: Key hints and key help fit the window
The footer SHALL show only the key hints that fit the width of the window, in their order, and SHALL NOT cut a hint. The key help (`?`) SHALL be a dialog as wide as its longest line, or the window when the window is narrower. It SHALL NOT wrap a line that fits, and it SHALL scroll when the window is too short.

#### Scenario: Feed footer at 140 columns
- **WHEN** the window is 140 columns wide and the Feed tab is open
- **THEN** each hint in the footer is complete, and no hint ends in the middle of its text

#### Scenario: Key help at 140 columns
- **WHEN** the window is 140 × 34 and the user presses `?`
- **THEN** each line of the key help shows as one line, and the line of the Feed keys starts with "Feed"

### Requirement: Command help uses the words of the model
The help and the errors of `i2a run` and `i2a daemon` SHALL use the words account, conversation, space, agent and schedule. They SHALL NOT use the words source, profile, interval or active hours.

#### Scenario: Help of run
- **WHEN** the user runs `i2a run --help`
- **THEN** the output says that the command reads each account one time and sends the waiting messages of each space to its agent, and the output does not contain "profile" or "source"
