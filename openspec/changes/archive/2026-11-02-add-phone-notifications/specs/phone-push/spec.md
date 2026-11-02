## ADDED Requirements

### Requirement: Service account setup
`i2a phone setup --service-account <file> --app-config <file>` SHALL read a Firebase service-account JSON file and check that it has `project_id`, `client_email`, `private_key` and `token_uri`, and that `private_key` is an RSA key in PEM. It SHALL read a `google-services.json` file, take the client with the package `com.sondt.inbox2agent`, and keep its project ID, project number, `mobilesdk_app_id` and API key. Both files SHALL have the same project ID. It SHALL store them in the secret store as `phone/service-account` and `phone/app-config` and print the project ID. It SHALL NOT print a key. Each flag alone SHALL replace only its own item.

#### Scenario: Valid file
- **WHEN** the user runs `i2a phone setup --service-account sa.json` with a valid file of the project `demo-123`
- **THEN** the secret store has `phone/service-account` and the output contains `demo-123`

#### Scenario: Wrong file
- **WHEN** the `--service-account` file is a `google-services.json` client file
- **THEN** the command fails with an error that names the missing field `private_key` and stores nothing

#### Scenario: Two projects
- **WHEN** the service account is of project `a-1` and the client file is of project `b-2`
- **THEN** the command fails with an error that names both project IDs

#### Scenario: No client for the package
- **WHEN** the client file has no client with the package `com.sondt.inbox2agent`
- **THEN** the command fails and tells the user to add an Android app with that package in the Firebase project

### Requirement: Pairing
`i2a phone pair` SHALL need a service account and a client configuration. When no pairing exists, it SHALL make a topic `i2a-` + 32 random hex characters and a random 256-bit key, and store them as `phone/topic` and `phone/key` with the time in `phone/paired-at`. When a pairing exists, it SHALL show the same pairing. It SHALL print the link `i2a://pair?v=1&p=<project_id>&s=<sender_id>&a=<app_id>&ak=<api_key>&t=<topic>&k=<base64url key>&n=<host name>` and the same link as a QR code. `--link` SHALL print the link only.

#### Scenario: First pairing
- **WHEN** the user runs `i2a phone pair` with a service account and no pairing
- **THEN** the secret store has a new topic and key, and the output has a QR code that decodes to the link

#### Scenario: Pair a second phone
- **WHEN** the user runs `i2a phone pair` again
- **THEN** the link has the same topic and key as before

#### Scenario: No setup
- **WHEN** no service account or no client configuration is stored
- **THEN** the command fails with an error that tells the user to run `i2a phone setup`

### Requirement: Reset the pairing
`i2a phone reset` SHALL replace the topic and the key with new random values, so phones with the old pairing get no more pushes. `i2a phone reset --remove` SHALL delete all `phone/*` secrets.

#### Scenario: Reset
- **WHEN** the user runs `i2a phone reset`
- **THEN** the topic and the key differ from the old values

### Requirement: Sealed card message
The Mac SHALL send each card as an FCM data message with the fields `v` = `1` and `c`. `c` SHALL be the base64 of a 12-byte random nonce and the AES-256-GCM ciphertext and tag of the raw-deflate compressed JSON payload, with the additional data `i2a-push-v1`. The payload SHALL hold `v`, `kind`, `ts`, `alert` and, for `kind` `card`, the card ID, key, category, urgency, title, group, body, suggested reply, creation time, state, a `cut` flag and the source messages (author, text, URL, channel name, time). `c` SHALL be 3,600 characters or less: the Mac SHALL cut ref texts to 140 characters, then keep the first 3 refs, then halve the body until the message fits, and set `cut` to true.

#### Scenario: Round trip
- **WHEN** the Mac seals a card and a reader opens `c` with the key
- **THEN** the reader gets the same payload

#### Scenario: Wrong key
- **WHEN** a reader opens `c` with another key
- **THEN** the open fails

#### Scenario: Large card
- **WHEN** a card has a body of 4,000 characters and 10 refs
- **THEN** `c` is 3,600 characters or less and the payload has `cut` true

### Requirement: Push from card add
When a service account and a pairing exist and `I2A_NOTIFY` is not `off`, `i2a card add` SHALL send one sealed message to the topic for each card with the result `added` or `updated`. `alert` SHALL be true for a card that the desktop notification rules count, for at most 5 cards in one call. A message with `alert` true SHALL use the Android priority `high`; others SHALL use `normal`. TTL SHALL be 7 days. The sender SHALL get an OAuth access token with a signed JWT (RS256, scope `https://www.googleapis.com/auth/firebase.messaging`) and post to `/v1/projects/<project_id>/messages:send`. A failure SHALL print `warning: phone push: <reason>` to stderr; the cards SHALL stay saved and the command SHALL exit 0. The push step SHALL stop after 15 seconds. When it sent one message or more, the output line SHALL add `Phone: <n>.` with the number of messages sent.

#### Scenario: Two new cards
- **WHEN** a call adds one `high` action and one `low` info card, with `min_urgency` `normal`
- **THEN** FCM gets 2 messages to the topic; the action has `alert` true and priority `high`; the info card has `alert` false and priority `normal`

#### Scenario: Unchanged card
- **WHEN** a call contains only an unchanged card
- **THEN** FCM gets no message

#### Scenario: FCM is down
- **WHEN** the FCM endpoint returns HTTP 503
- **THEN** the cards are saved, stderr has `warning: phone push:`, and the exit code is 0

#### Scenario: Not paired
- **WHEN** no pairing exists
- **THEN** `i2a card add` makes no network call

### Requirement: Test push and status
`i2a phone test` SHALL send one message with `kind` `test` and `alert` true. `i2a phone status` SHALL print the secret backend, the project ID, the pairing time and the first 8 characters of the topic, or the next step when an item is missing. It SHALL NOT print the key or the private key.

#### Scenario: Status when paired
- **WHEN** the user runs `i2a phone status` after setup and pair
- **THEN** the output has the project ID and the pairing time, and no key
