## MODIFIED Requirements

### Requirement: Spaces and batches
The system SHALL send each pending message to each space that has a link to its conversation and whose read level allows the message, or to Unsorted when no space lists the conversation. The routes SHALL be computed when a space sends, so a moved conversation takes its pending messages to its new space. The system SHALL make one batch for each space that has pending messages at its send time, and SHALL send it to the agent of the space. A space SHALL get a message one time: a message that a space has (sent, or previewed by a dry run of that space) SHALL NOT go to that space again, except a previewed message to a real driver. A failed batch of one space SHALL NOT change the messages of another space. The dispatch SHALL record the space and the agent.

#### Scenario: Two spaces from one account
- **WHEN** the account `phone` has a new message in a chat of `acme` and one in a chat of `personal`, and both spaces are due
- **THEN** there is one batch of `acme` and one batch of `personal`, each to the agent of its space

#### Scenario: One conversation in two spaces
- **WHEN** the channel `release` is in Review and in Team, both with the level `all`, and a member posts one message
- **THEN** Review and Team each get one batch with the message, each to its own agent

#### Scenario: One space fails
- **WHEN** the channel `release` is in Review and in Team, and the batch of Team fails
- **THEN** the message has a delivery `sent` for Review, the next batch of Team contains it, and the next batch of Review does not
