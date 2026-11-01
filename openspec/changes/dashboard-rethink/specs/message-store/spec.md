## ADDED Requirements

### Requirement: Requeue the messages of a conversation
`Store.Requeue(account, conversation, since, forMeOnly)` SHALL set the state of each `sent` or `previewed` message of the conversation that was created at `since` or later back to `pending`, in one transaction, and SHALL return the number of messages. With `forMeOnly`, it SHALL change only the messages for the user. It SHALL NOT change the deliveries, so a space that has a `sent` delivery of a message SHALL NOT get it again.

#### Scenario: Requeue for a new space
- **WHEN** a message of `work`/`C1` has a `sent` delivery for triage, and the dashboard requeues `work`/`C1` after `C1` is added to code-review
- **THEN** the message is `pending`, and the next dispatch sends it to code-review and not to triage
