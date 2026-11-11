## MODIFIED Requirements

### Requirement: Read and send now from the dashboard
The header SHALL always show the button `Run now ctrl+r` and the next send of the spaces. A click on the button SHALL do the same as the key `ctrl+r`; while a run is busy, the header SHALL show the spinner and no button. The key `ctrl+r` SHALL read all accounts and send the waiting messages of each space that has an agent, except in the Spaces area, where it SHALL send the selected space. `R` SHALL NOT start a run. When the daemon runs, the dashboard SHALL ask the daemon (`POST /v1/run`); when no daemon runs, the dashboard SHALL run the same work in its own process and SHALL hold the daemon lock while it runs. While a run is busy, a second `ctrl+r` SHALL show that a run is busy and SHALL NOT start a run. The line under the header SHALL show the result of the last run until the next run: the time, the number of new messages and each batch with its space and status, or "nothing waits". While the daemon runs, the status line SHALL show the `send`, `reload` and `error` events of the daemon.

#### Scenario: Run in the Feed with the daemon
- **WHEN** the daemon runs, a new message waits, and the user presses `ctrl+r` in the Feed
- **THEN** the daemon sends the batch, and the line under the header names the space and the status of the batch

#### Scenario: Run in the Spaces area without a daemon
- **WHEN** no daemon runs, the selected space is Acme with a waiting message, and the user presses `ctrl+r` in the Spaces area
- **THEN** the dashboard sends one batch of Acme, and the batches of other spaces stay pending

#### Scenario: Click Run now
- **WHEN** no daemon runs, a message of a space with an agent waits, and the user clicks `Run now ctrl+r` in the header of the Feed
- **THEN** the dashboard sends the batch, and the line under the header shows the result of the run

#### Scenario: Event of the daemon
- **WHEN** the dashboard is open and the daemon sends a batch at its schedule
- **THEN** the status line shows the space and the status of that batch within 3 seconds
