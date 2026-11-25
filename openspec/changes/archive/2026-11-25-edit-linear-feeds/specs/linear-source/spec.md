## ADDED Requirements

### Requirement: Names of the workspace
The Linear source SHALL read the names of the workspace for the feed form with one GraphQL query: the teams (key and name), the projects, the issue labels and the users (display name). It SHALL read 250 items on each page and follow `after` for a list with more pages, at most 10 pages for each list. It SHALL keep each label name one time, without case. The query SHALL be a query, not a mutation.

#### Scenario: Names of a workspace
- **WHEN** the workspace has teams, projects, labels and users
- **THEN** the result has each team key with its name, each project name, each label name one time and each display name

#### Scenario: More pages
- **WHEN** the labels have a second page
- **THEN** the next request reads only the labels, with `after` set to the end cursor of the first page
