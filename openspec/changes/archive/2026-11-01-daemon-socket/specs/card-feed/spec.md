## ADDED Requirements

### Requirement: Read and send now from the dashboard
The key `R` in the Feed tab SHALL read all accounts and send the waiting messages of each space that has an agent. The key `R` in the Spaces tab SHALL read all accounts and send the selected space. When the daemon runs, the dashboard SHALL ask the daemon (`POST /v1/run`); when no daemon runs, the dashboard SHALL run the same work in its own process and SHALL hold the daemon lock while it runs. While a run is busy, a second `R` SHALL show that a run is busy and SHALL NOT start a run. The status line SHALL show the result: the number of new messages and each batch with its space and status. While the daemon runs, the status line SHALL show the `send`, `reload` and `error` events of the daemon.

#### Scenario: R in the Feed with the daemon
- **WHEN** the daemon runs, a new message waits, and the user presses `R` in the Feed
- **THEN** the daemon sends the batch, and the status line names the space and the status of the batch

#### Scenario: R in the Spaces tab without a daemon
- **WHEN** no daemon runs, the selected space is Acme with a waiting message, and the user presses `R` in the Spaces tab
- **THEN** the dashboard sends one batch of Acme, and the batches of other spaces stay pending

#### Scenario: Event of the daemon
- **WHEN** the dashboard is open and the daemon sends a batch at its schedule
- **THEN** the status line shows the space and the status of that batch within 3 seconds
