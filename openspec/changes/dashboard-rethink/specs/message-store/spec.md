## ADDED Requirements

### Requirement: Requeue the messages of a conversation
`Store.Requeue(account, conversation, since, forMeOnly, skip)` SHALL set the state of each `sent` or `previewed` message of the conversation that was created at `since` or later back to `pending`, in one transaction, and SHALL return the number of messages. With `forMeOnly`, it SHALL change only the messages for the user. It SHALL keep the `sent` deliveries, so a space that has a `sent` delivery of a message SHALL NOT get it again. It SHALL write a `skipped` delivery of each message for each space in `skip` that has no delivery, and SHALL remove the older `skipped` deliveries of the message. A dispatch SHALL NOT send a message to a space whose delivery is `skipped`. An expiry SHALL keep a message `sent` when a space has a `sent` delivery of it.

#### Scenario: Only the new space
- **WHEN** a message of `work`/`C1` was sent to triage, `C1` moved to Review with no requeue, and the dashboard later adds code-review with the requeue and `skip = [review]`
- **THEN** the next dispatch sends the message to code-review only

#### Scenario: Requeue for a new space
- **WHEN** a message of `work`/`C1` has a `sent` delivery for triage, and the dashboard requeues `work`/`C1` after `C1` is added to code-review
- **THEN** the message is `pending`, and the next dispatch sends it to code-review and not to triage
