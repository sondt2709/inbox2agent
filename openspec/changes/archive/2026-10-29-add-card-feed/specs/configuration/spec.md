## MODIFIED Requirements

### Requirement: Configuration TUI
The system SHALL provide a full-screen dashboard in the alternate screen of the terminal, with the tabs Feed, Overview, Sources, Profiles, Schedule and Activity. `i2a` with no arguments SHALL open the dashboard on the Feed tab. `i2a config` SHALL open the dashboard on the Overview tab. The keys `1` to `6`, `tab` and `shift+tab`, and a mouse click on a tab SHALL change the tab. The Sources and Profiles tabs SHALL have a list at the left and a detail panel at the right. The detail panel SHALL be the form of the selected item, or the form of a new item when "+ Add" is selected; the TUI SHALL NOT use separate screens or dialogs to add or edit. A form SHALL show all its fields at one time, with the help of the focused field, an error under each field that is not valid, and buttons. Submitting a form (`ctrl+s`, the primary button, or `enter` in the last field) SHALL write the configuration file. `esc` SHALL give the keyboard back to the list and SHALL ask before it discards changes. The footer SHALL show the keys of the current focus. `q` SHALL quit; `?` SHALL show all keys.

#### Scenario: Dashboard
- **WHEN** the user runs `i2a config` in a window of 120 × 40 with one source
- **THEN** the TUI fills the window, the tab bar shows the 6 tabs, and the Overview tab shows the service state, the message counts and the source

#### Scenario: Feed first
- **WHEN** the user runs `i2a` with no arguments
- **THEN** the dashboard opens on the Feed tab, and the tab bar shows "1 Feed"

#### Scenario: Edit in the detail panel
- **WHEN** the user selects the source `work` in the Sources tab
- **THEN** the detail panel shows the form of `work` with its values, and `enter` gives the keyboard to the form

#### Scenario: Add in the detail panel
- **WHEN** the user selects "+ Add source"
- **THEN** the detail panel shows the form of a new source with the fields Source ID, Server URL and Token and the button "Connect"

#### Scenario: Error under the field
- **WHEN** the user types `abc` as the interval and presses `ctrl+s`
- **THEN** the text "is not a duration" shows under the interval field, and the file does not change

#### Scenario: Leave a form with Esc
- **WHEN** the user types `45m` as the interval, presses `esc`, and then presses `d` (discard)
- **THEN** the form shows the interval `15m` again, and the file does not change

#### Scenario: Terminal restored
- **WHEN** the user exits the TUI
- **THEN** the terminal shows the shell screen from before the dashboard, and the last line tells whether the file was saved

#### Scenario: Save timing
- **WHEN** the user sets the interval to `10m` and presses `ctrl+s`
- **THEN** the configuration file contains `interval = "10m"`, and the dashboard stays open
