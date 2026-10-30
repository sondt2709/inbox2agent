## 0. Start

- [x] 0.1 Wait until `add-whatsapp-source` is archived and merged; create the branch `feat/introduce-spaces` from `main`
- [x] 0.2 Check that the delta specs still apply: `openspec validate introduce-spaces --strict`; fix the REMOVED names if the WhatsApp change renamed a requirement
- [x] 0.3 Capture the "before" evidence (sample version 1 file, screens of the old tabs, `i2a status`; captured during explore on the WhatsApp branch, the same TUI and config code as main @ ec01841)

## 1. Configuration version 2

- [x] 1.1 `internal/config`: types `Account`, `Agent`, `Schedule`, `Space`, `Conversation`, `Unsorted`, `Settings`; `version = 2`; defaults (agent `triage` dry-run, schedule `always` every 15m, Unsorted with agent `triage` and read `mentions`)
- [x] 1.2 Validation: IDs, unique names, references, one space for each conversation, read levels, `days`, `hours`, durations, templates with `.Sources` or `.Profile` refused
- [x] 1.3 `Schedule.Next(after)` with aligned send times, days, hours across midnight; table tests with the examples of the spec
- [x] 1.4 `Config.Route(account, conversation)` and `Config.Level(account, conversation)`; helpers `SpacesUsing(agent|schedule)`, `MoveConversation`, `ReplaceAgent`, `ReplaceSchedule`
- [x] 1.5 Version 1 migration (`migrate_v1.go`): detection, conversion table of design D9, template rewrite, report, `config.toml.v1.bak` with mode 0600; tests with version 1 files from the README and from the e2e harness
- [x] 1.6 `Load` returns the report; `Save` writes version 2 only

## 2. Store schema version 6

- [x] 2.1 `migrateV6`: backup `inbox.db.v5.bak`, rename the `profile` columns to `space`, add `dispatches.agent`, set version 6, one transaction; new stores at version 6
- [x] 2.2 Rename the Go fields (`Message.Space`, `Dispatch.Space`, `Dispatch.Agent`, `Ask.Space`) and the session functions (`Session(space)`, `SetSession(space, ...)`)
- [x] 2.3 Store query for the space of a card (newest ref message with a space); waiting count for each space
- [x] 2.4 Tests: upgrade from a real version 5 store copy (counts and values as the spec), failure keeps version 5, new store at version 6

## 3. Sources read by level

- [x] 3.1 Mattermost `Settings`: `Levels map[string]string`, `Default string` instead of `Channels`, `AllDirect`, `Mentions`; `Collect` uses the level of each channel; `mentions` includes direct channels
- [x] 3.2 WhatsApp `Settings` and `wanted` with the same rule
- [x] 3.3 `source.Build` takes `config.Account` and the levels from `config.Level`; `ListChannels` and `ReadChats` stay for discovery
- [x] 3.4 Update the fixture-based tests; no hand-written fixture

## 4. Daemon, delivery and asks

- [x] 4.1 `Engine.Dispatch(ctx, spaces)`: route pending messages with `Config.Route`, one batch for each space with an agent, dry-run preview as before, record space and agent
- [x] 4.2 `daemon.Run`: `next[space]` from `Schedule.Next`, recompute on reload, send queued asks at least each minute; log lines with the space ID
- [x] 4.3 `i2a run --once`: all spaces with an agent, ignore schedules, skip Unsorted on hold
- [x] 4.4 `agent.Lock` on `<data>/space-<id>.lock`; drivers take the agent and the space; reuse session for each space
- [x] 4.5 `render`: template fields `.Space`, `.Agent`, `.Accounts`; default primer names the space; batch ID with the space ID
- [x] 4.6 `ask.Resolve` and `ask.Send`: space of the card, agent of the space, the text "The space <name> has no agent that reads asks."
- [x] 4.7 Notifications for failed dispatches name the space

## 5. CLI

- [x] 5.1 `i2a account add mattermost|whatsapp`, `account list`, `account remove` (move the code of `source.go` and `whatsapp.go`)
- [x] 5.2 `i2a conversations [--account] [--unsorted]` and `i2a conversation set <account> <id> [--space] [--read]`
- [x] 5.3 `i2a source` prints the rename text and exits with code 2
- [x] 5.4 `i2a status`: accounts, spaces (agent, schedule, next send, waiting), Unsorted, agents, schedules, counts, dispatches, service
- [x] 5.5 Print the migration report to stderr in each command that loads the configuration; log it in the daemon

## 6. TUI

- [x] 6.1 `model.go`: 8 tabs, groups and arrows in the tab bar, the definition line, `i2a config` opens Spaces, unsorted badge, `?` shows the model and the keys
- [x] 6.2 `tab_accounts.go`: list, detail with the bar of spaces, Advanced `read_every`, add forms (Mattermost check, WhatsApp pairing from the current Sources tab), remove
- [x] 6.3 `tab_conversations.go`: join of configured and listed conversations (background load), search, filters, multi-select, `m` move dialog with "new space", `l` read level
- [x] 6.4 `tab_spaces.go`: Unsorted first, sentence, name, agent and schedule choices, conversation table, `a` add dialog, delete to Unsorted, Unsorted hold choice and new read level
- [x] 6.5 `tab_agents.go`: the profile form with new names, used-by, delete with replacement, Orca warning
- [x] 6.6 `tab_schedules.go`: every, days, hours, today line with send marks and now, used-by, delete with replacement
- [x] 6.7 `tab_settings.go`: lookback, retention, notifications, service keys `i` and `u`
- [x] 6.8 Activity: message counts and the Space column
- [x] 6.9 Feed: space of each card in the list and the detail, `F` space filter
- [x] 6.10 Remove `tab_overview.go`, `tab_sources.go`, `tab_profiles.go`, `tab_schedule.go` and their tests; update `model_test.go`, `picker_test.go`, `feed_test.go`

## 7. Tests

- [x] 7.1 Unit tests for each spec scenario of `spaces`, `configuration`, `agent-delivery`, `message-store`, `daemon-service`, `agent-ask`, `card-feed`, `batch-rendering`; break each new test one time and restore the code
- [x] 7.2 e2e (CI, Docker Mattermost): update the harness configuration to version 2; new scenarios: migration of a version 1 file, two spaces from one account with two agents, move a conversation and send its waiting messages, `i2a conversation set`, the tab bar and the Spaces sentence, delete an agent with a replacement
- [ ] 7.3 e2e (local): `I2A_E2E_ORCA=1` two spaces with one `reuse` agent get two terminals; `I2A_E2E_WHATSAPP=1` read levels on chats (not run: needs the local Orca app with a real agent, and a phone; the unit test TestReuseKeepsOneSessionForEachSpace covers the session rule with real Orca outputs)
- [x] 7.4 `task test`, `task lint`, `golangci-lint run --build-tags e2e ./test/...`, `task e2e`, `task leakcheck`

## 8. Docs

- [x] 8.1 README: the model (diagram of account → conversation → space → agent + schedule), the version 2 configuration example, the TUI tabs, the CLI commands, the migration and rollback section
- [x] 8.2 CLAUDE.md: "What this is", the data flow (`Config.Route` to a space, dispatch for each space), the package notes (`config`, `agent` lock for each space, `tui` tabs), the store version 6, the new terms
- [x] 8.3 `test/e2e/SCENARIOS.md`: new and changed scenarios
- [x] 8.4 `docs/demo/record.sh`: new tape for the Spaces flow; record `docs/demo/i2a-config.gif` again
- [x] 8.5 Search the repo for the old words (`profile`, `source add`, `all_direct`, `channel_profiles`, `interval`, `active_hours`, Overview) in docs, help texts and errors; replace them

## 9. Verify and release

- [ ] 9.1 Run the test plan again on the last commit; save the results with the SHA in `tmp/evidence/clarify-sources-profiles-schedule/after/`
- [ ] 9.2 Record the screens of the real app and compare them with the playground; report the differences
- [ ] 9.3 PR title `feat!: replace sources and profiles with accounts, spaces, agents and schedules`, with a `BREAKING CHANGE` footer
