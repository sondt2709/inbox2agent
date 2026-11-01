## ADDED Requirements

### Requirement: Dashboard reloads a changed configuration file
While the dashboard is open, it SHALL check `config.toml` on each refresh (every 2 seconds). When the bytes of the file differ from the last bytes that the dashboard read or wrote, the dashboard SHALL load the file again and SHALL show the loaded items in all tabs. When the loaded file is invalid, the dashboard SHALL keep its current configuration and SHALL show the first error in the status line. The dashboard SHALL NOT discard the unsaved edits of an open form because of a reload.

#### Scenario: A space added by another process shows
- **WHEN** the dashboard is open on the Spaces tab
- **AND** another process adds a `[[spaces]]` item with the name `Side project` to `config.toml`
- **THEN** within 3 seconds the Spaces list shows `Side project`

#### Scenario: Invalid file from another process
- **WHEN** another process writes `config.toml` with `every = "ten minutes"` in a schedule
- **THEN** the dashboard keeps its current spaces and schedules
- **AND** the status line shows `config.toml` and the error of the `every` field

#### Scenario: Unsaved form edits stay after a reload
- **WHEN** the user types a new name in the form of the space `Company A` and does not save
- **AND** another process adds a schedule to `config.toml`
- **THEN** the form still shows the typed name and the unsaved mark
- **AND** the Schedules tab shows the new schedule

### Requirement: A save keeps the changes of other processes
Each save of the dashboard SHALL read `config.toml` again, apply only the change of the form or the action to that content, validate the result and write it. A change of another process to a different item SHALL stay in the file. When the item of the change does not exist in the file any more, the save SHALL fail with an error that names the item, and SHALL NOT write the file. When the file does not exist, the save SHALL apply the change to the default configuration.

#### Scenario: A settings save keeps a space of another process
- **WHEN** the dashboard is open
- **AND** another process adds the space `side-project` to `config.toml`
- **AND** the user changes the lookback to `2d` in the Settings tab and saves
- **THEN** `config.toml` contains `lookback = '2d'` and the space `side-project`

#### Scenario: The item was deleted by another process
- **WHEN** the user edits the agent `preview` in the dashboard
- **AND** another process removes the agent `preview` from `config.toml`
- **AND** the user saves the form
- **THEN** the form shows an error that the agent `preview` does not exist any more
- **AND** `config.toml` does not contain the agent `preview`

### Requirement: Conflict on the item of a form
When the user saves a form, and another process changed the same item in `config.toml` after the form was made, the dashboard SHALL open a dialog before it writes. The choice "Reload" SHALL be the default; it SHALL discard the edits of the form and SHALL make the form again from the file. The choice "Overwrite" SHALL save the values of the form over the item. The settings and notification fields are one item; the Unsorted space is one item; each account, agent, schedule, space and conversation is one item.

#### Scenario: Reload after a conflict
- **WHEN** the user changes the `every` field of the schedule `work-hours` to `30m` in the form
- **AND** another process changes `every` of `work-hours` to `5m` in `config.toml`
- **AND** the user saves and chooses "Reload"
- **THEN** `config.toml` contains `every = '5m'` for `work-hours`
- **AND** the form shows `5m`

#### Scenario: Overwrite after a conflict
- **WHEN** the same conflict happens and the user chooses "Overwrite"
- **THEN** `config.toml` contains `every = '30m'` for `work-hours`

### Requirement: Write lock of the configuration file
Each writer of `config.toml` SHALL hold an exclusive `flock` on `config.toml.lock` in the same directory while it reads, changes and writes the file. The writers are the dashboard, `i2a account add`, `i2a account remove`, `i2a conversation set` and the conversion of a version 1 file. The lock file SHALL have the mode 0600. When another process holds the lock for more than 5 seconds, the writer SHALL fail with the error `config.toml is locked by another process` and SHALL NOT write. A writer SHALL write the file to a temporary file and rename it, so a reader never reads a partly written file.

#### Scenario: Two writers at the same time
- **WHEN** two processes each add a different space with the locked update at the same time
- **THEN** `config.toml` contains both spaces

#### Scenario: Lock held too long
- **WHEN** a process holds `config.toml.lock` for 10 seconds
- **AND** the user saves a form in the dashboard
- **THEN** the form shows `config.toml is locked by another process`
- **AND** the file does not change

### Requirement: Header of the configuration file
Each save SHALL write a header comment that says that i2a writes the file again on each save and removes other comments.

#### Scenario: Header after a save
- **WHEN** the dashboard saves `config.toml`
- **THEN** the first lines of the file say that a save by i2a removes comments
