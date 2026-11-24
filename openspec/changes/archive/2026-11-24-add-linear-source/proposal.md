## Why

The user wants an AI agent to act on a Linear comment. The main case: the user writes a comment such as `@me.user /agent please check this` and the agent of a space gets it. Linear webhooks need a public HTTPS server, and Linear sends no notification for an action of the user. The Linear GraphQL API gives the comments and the inbox to a personal API key, so i2a can poll Linear in the same way as Mattermost, with no public server.

## What Changes

- A new platform `linear`. An account is one personal API key. The secret store keeps the key. More than one Linear account is possible.
- A Linear account has a list of feeds (`[[accounts.feeds]]`). Each feed is one conversation of the account. Spaces list feeds in `[[spaces.conversations]]` in the same way as channels.
- A feed has the kind `comments` (the comments that match the filter) or `inbox` (the Linear notifications of the user).
- The filter of a feed: `from` (people, default `["me"]`), `prefix` (the text after the leading mentions starts with it), `projects`, `labels` and `teams`.
- One poll of an account sends one GraphQL request for all its feeds. `read_every` of the account sets the time between two polls.
- The source uses only queries. It never marks a notification as read and never writes to Linear.
- `i2a account add linear --id <id> --token-stdin [--name <name>]` checks the key and adds the account with two feeds: `agent` (comments of the user with the prefix `/agent`) and `inbox`.
- The Accounts section of Setup can add a Linear account and shows its user, workspace and feeds.
- Reply to Linear is not in this change. The Feed refuses a reply to a Linear message.

## Capabilities

### New Capabilities

- `linear-source`: collect Linear comments and notifications with a personal API key, by feed.

### Modified Capabilities

- `configuration`: the platform `linear`, `[[accounts.feeds]]` and its validation, `i2a account add linear`, and the Linear form of the Accounts section.

## Impact

- New package `internal/source/linear` (GraphQL client, `Collect`, `Check`, capture tool in `testdata/capture`).
- `internal/config`: `PlatformLinear`, `Feed`, validation, schema, `docs/config.schema.json`.
- `internal/source/source.go` (`Build`), `internal/source/reply.go` (no reply for `linear`).
- `internal/cli/account.go`, `internal/tui/tab_accounts.go`, `internal/tui/model.go`.
- New connection to `https://api.linear.app/graphql`. No new Go dependency. No store schema change.
