## MODIFIED Requirements

### Requirement: Prompt templates
Each agent SHALL have a `primer` template and a `batch` template in Go `text/template` syntax. The templates SHALL be able to use `.BatchID`, `.Space` (the name of the space), `.Agent` (the ID of the agent), `.Count`, `.Accounts`, `.DigestPath`, `.Messages`, `.Nonce`, `.WindowStart` and `.WindowEnd`. When an agent has no template, the system SHALL use the default template for its payload mode. The default primer SHALL name the space. The configuration validation SHALL reject a template that does not parse, and a template that uses `.Sources` or `.Profile`, with the name of the new field.

#### Scenario: Custom batch template
- **WHEN** the batch template is `Process {{.Count}} items of {{.Space}} in {{.DigestPath}}`
- **THEN** the prompt for a batch of 2 messages of the space Acme is `Process 2 items of Acme in <path of the digest file>`

#### Scenario: Template with a syntax error
- **WHEN** the batch template is `{{.Count`
- **THEN** the configuration validation reports the template error and the agent ID

#### Scenario: Old field
- **WHEN** the batch template of a version 2 file uses `{{.Sources}}`
- **THEN** the configuration validation says to use `{{.Accounts}}`
