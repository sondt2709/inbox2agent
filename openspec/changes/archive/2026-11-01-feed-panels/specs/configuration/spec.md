## MODIFIED Requirements

### Requirement: Dashboard tabs
The system SHALL provide a full-screen dashboard in the alternate screen with three areas: Feed, Spaces and Setup. Setup SHALL have the sections Accounts, Agents, Schedules, Settings and Send log, in this order. The header SHALL show the areas as `Feed`, `Spaces` and `Setup` with no numbers, with the number of new cards after Feed and the number of new conversations after Spaces. The line under the header SHALL show the sections of Setup when Setup is open, and else the result of the last run. `i2a` with no arguments SHALL open the Feed; `i2a config` SHALL open the Spaces area. A mouse click on an area or a section SHALL open it. The keys `1` to `3` SHALL NOT open an area. `tab` and `shift+tab` SHALL go to the next and the previous section, in the order Feed, Spaces, Accounts, Agents, Schedules, Settings, Send log. Forms SHALL stay in the detail panel, as before. `q` SHALL quit; `?` SHALL show the model of i2a and all keys.

#### Scenario: Header
- **WHEN** the user runs `i2a config` in a window of 120 × 40, with 3 new conversations
- **THEN** the header shows `Feed`, `Spaces 3 new`, `Setup` and `ctrl+r run now`, and the Spaces area is open

#### Scenario: Setup sections
- **WHEN** the user clicks `Setup`
- **THEN** the line under the header shows Accounts, Agents, Schedules, Settings and Send log, and a click on Agents opens Agents

#### Scenario: Feed first
- **WHEN** the user runs `i2a` with no arguments
- **THEN** the dashboard opens on the Feed

#### Scenario: Terminal restored
- **WHEN** the user exits the TUI
- **THEN** the terminal shows the shell screen from before the dashboard

### Requirement: Key hints and key help fit the window
The footer SHALL show the hints of the focused list or panel with its main key first, and `? keys · q quit` at the right. It SHALL show only the hints that fit the width of the window, in their order, and SHALL NOT cut a hint. At 80 columns the footer SHALL show `ctrl+r` in the Spaces area and `enter` and `a` in the panel of a space. The key help (`?`) SHALL be a dialog as wide as its longest line, or the window when the window is narrower. It SHALL NOT wrap a line that fits, and it SHALL scroll when the window is too short.

#### Scenario: Spaces footer at 80 columns
- **WHEN** the window is 80 columns wide and a space is selected in the Spaces area
- **THEN** the footer shows `ctrl+r run now`, and each hint is complete

#### Scenario: Key help at 140 columns
- **WHEN** the window is 140 × 34 and the user presses `?`
- **THEN** each line of the key help shows as one line, and the line of the Feed keys starts with "Feed"
