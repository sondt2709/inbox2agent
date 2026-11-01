## MODIFIED Requirements

### Requirement: Feed sort and filter
The key `s` SHALL switch the sort between `urgency` (high first, then the newest update) and `newest` (the newest update first). The key `f` SHALL change the category filter in the order all, action, question, info. When the window is 100 columns or wider, the Feed SHALL show at the left a list with "All" and each space, Everything else last, each with the number of its cards that the other filters show. `←` and `→` SHALL select the previous and the next row of that list, and the Feed SHALL show the cards of the selected space. The key `F` SHALL open a dialog with the same rows. The filter SHALL keep the ID of the space; when that space does not exist any more, the filter SHALL be all spaces. The filter bar SHALL show the name of the space of the filter. The header SHALL show the number of `new` cards, of actions and of questions. The key `a` SHALL show or hide the `done` cards.

#### Scenario: Sort by urgency
- **WHEN** the Feed has a `low` info card from 17:57 and a `high` action card from 17:55, and the sort is `urgency`
- **THEN** the action card is first

#### Scenario: Filter questions
- **WHEN** the user presses `f` two times
- **THEN** the list shows only `question` cards and the filter bar marks "Question"

#### Scenario: Filter a space with the list
- **WHEN** the spaces are Acme and Personal, the window is 120 columns, and the user presses `→` two times
- **THEN** the list shows only cards of Personal, and the space list marks Personal

#### Scenario: Card counts in the space list
- **WHEN** Everything else has 2 cards and Acme has 1 card
- **THEN** the space list shows "All" with 3, "Acme" with 1 and "Everything else" with 2

#### Scenario: The space of the filter is removed
- **WHEN** the filter is Acme and another process removes the space Acme from `config.toml`
- **THEN** after the reload the Feed shows the cards of all spaces

### Requirement: Space filter in Activity
The Send log section of Setup SHALL show the sends of the spaces, and SHALL use the space filter of the Feed. The key `F` in the Send log SHALL open the dialog of the Feed, with the number of sends of each space. When the filter is a space, the Send log SHALL show only the sends of that space and SHALL show the name of the space in its title. The panel of a space in the Spaces area SHALL show its recent sends.

#### Scenario: Filter the sends of a space
- **WHEN** the Send log has sends of Acme and of Everything else, and the user selects Acme in the `F` dialog
- **THEN** the table shows only the sends of Acme and the title shows "space Acme"

#### Scenario: One filter for the Feed and the Send log
- **WHEN** the user selects Acme in the Feed and then opens the Send log
- **THEN** the Send log shows only the sends of Acme

### Requirement: Read and send now from the dashboard
The header SHALL always show `R run now` and the next send of the spaces. The key `R` SHALL read all accounts and send the waiting messages of each space that has an agent, except in the Spaces area, where it SHALL send the selected space. When the daemon runs, the dashboard SHALL ask the daemon (`POST /v1/run`); when no daemon runs, the dashboard SHALL run the same work in its own process and SHALL hold the daemon lock while it runs. While a run is busy, a second `R` SHALL show that a run is busy and SHALL NOT start a run. The line under the header SHALL show the result of the last run until the next run: the time, the number of new messages and each batch with its space and status, or "nothing waits". While the daemon runs, the status line SHALL show the `send`, `reload` and `error` events of the daemon.

#### Scenario: R in the Feed with the daemon
- **WHEN** the daemon runs, a new message waits, and the user presses `R` in the Feed
- **THEN** the daemon sends the batch, and the line under the header names the space and the status of the batch

#### Scenario: R in the Spaces area without a daemon
- **WHEN** no daemon runs, the selected space is Acme with a waiting message, and the user presses `R` in the Spaces area
- **THEN** the dashboard sends one batch of Acme, and the batches of other spaces stay pending

#### Scenario: Event of the daemon
- **WHEN** the dashboard is open and the daemon sends a batch at its schedule
- **THEN** the status line shows the space and the status of that batch within 3 seconds
