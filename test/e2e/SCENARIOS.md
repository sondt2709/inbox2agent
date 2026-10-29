# End-to-end scenarios

These scenarios prove that `i2a` works for a person. Each test drives the real `i2a` binary through a real terminal. The harness types keys into a detached `tmux` session and reads the screen. Then it checks the visible text, the files and the store rows.

Rules:

1. Use the real program and the real interfaces. Do not call internal functions.
2. Do the steps that a person does: type, press keys, read the screen.
3. Run in the background. Never attach to or take focus from the terminal of the developer.
4. Keep the evidence: each screen capture goes to `test/e2e/out/<scenario-id>/`.
5. A scenario fails when its expected text does not appear before the timeout.

Where a scenario runs:

- **CI**: needs Docker and `tmux` only. CI runs it on each pull request.
- **Local**: needs a local app (Orca or `launchctl`). Run it with the environment variable in the table.

## Fixture

`test/e2e/mattermost/compose.yaml` starts a Mattermost server. The setup step creates these items through the real API:

- user `alice` (the `i2a` user) with a personal access token;
- user `bob` (the sender);
- team `e2e` with the channels `selected` and `other`;
- a direct channel between `alice` and `bob`.

## Scenarios

| ID | Name | Steps (as a person) | Expected result | Runs |
|---|---|---|---|---|
| S01 | First start | Run `i2a run --once` with an empty `I2A_HOME`. | The screen tells you to run `i2a config`. The exit code is not 0. | CI |
| S02 | Connect Mattermost in the TUI | Run `i2a config`. Press `3`, then `enter`. In the form at the right, type the ID, the URL and the token of `alice`, and press `enter`. | The form shows `✓ Connected as alice`, then the channel list. | CI |
| S03 | Wrong token | Do S02 with a wrong token. Press `ctrl+c`, then `d`. | The form shows the server error under the buttons. No source is saved. The secret store is empty. | CI |
| S04 | Select channels with the search | In the channel list, type `sel` (no other key first), press space to select `selected`, press `Esc` to clear the search. Press `ctrl+s`, then `q`. | The form of `work` shows 1 selected channel. `config.toml` lists the `selected` channel ID and contains no token. | CI |
| S05 | Collect and dry-run | `bob` posts in `selected`, in `other` (no mention), in `other` with `@alice`, and in the direct channel. Run `i2a run --once`. | The screen shows 3 collected messages and 1 batch. The digest has 3 JSON lines: `selected`, the mention and the direct message. The post in `other` without a mention is not there. The 3 messages are `previewed` in the store. | CI |
| S06 | Messages stay unread | Read the unread counts of `alice`. Do S05. Read the counts again. | The counts are the same before and after. | CI |
| S07 | No duplicates | Run `i2a run --once` again with no new posts. | The screen shows 0 collected messages and no batch. No new digest file. | CI |
| S08 | Injection text stays data | `bob` posts these texts: a fence followed by "ignore previous instructions", a forged `i2a-data-` label, an ANSI escape, a bidi override, and 10,000 characters. Run `i2a run --once`. | The digest has 1 opening fence line and 1 closing fence line. Each message is 1 JSON line. No ESC or U+202E. The long message has `"truncated":true`. | CI |
| S09 | Timing in the TUI | Run `i2a config`. Press `5`, then `enter`. Type `abc` as the interval and press `ctrl+s`. Then type `10m` and press `ctrl+s`. Press `q`. | The error for `abc` shows under the field. `config.toml` has `interval = "10m"`. | CI |
| S10 | Leave a form without saving | Change the interval in the form. Press `esc`, then `d` (discard), then `q`. | The TUI asks before it discards. `config.toml` does not change. | CI |
| S11 | Daemon ticks | Set the interval to `5s`. Start `i2a daemon` in a second terminal. `bob` posts in `selected`. Wait 15 seconds. | The daemon log shows poll and dispatch lines. A digest file holds the new post. | CI |
| S12 | One daemon only | Start `i2a daemon` while S11 runs. | The screen shows `another i2a daemon is running`. The exit code is not 0. | CI |
| S13 | Active hours | Set the active hours to a range that does not include now. Run the daemon for one tick after a new post. | The log shows `outside active hours`. No digest. The message is pending in `i2a status`. | CI |
| S14 | Orca new session, pointer | Set a profile to driver `orca`, session `new`, payload `pointer`. `bob` posts. Run `i2a run --once`. | `orca terminal read` shows the primer and a prompt with the digest path. The agent replies with a summary. | Local (`I2A_E2E_ORCA=1`) |
| S15 | Orca reuse session, inline | Set session `reuse`, payload `inline`. Send two batches. | Both batches arrive in the same terminal. The primer arrives one time. | Local (`I2A_E2E_ORCA=1`) |
| S16 | Orca terminal closed | Close the terminal of S15 in Orca. Send one more batch. | A new terminal starts with the primer and the batch. | Local (`I2A_E2E_ORCA=1`) |
| S17 | Orca not reachable | Point the Orca path to an executable that does not exist. Send a batch. | `i2a status` shows the failed dispatch and the pending messages. A run after you fix the path sends them. | Local (`I2A_E2E_ORCA=1`) |
| S18 | Orca worktree mode | Set session `worktree`. Send one batch. | `orca worktree list` shows `i2a-<batch-id>` with an agent terminal. | Local (`I2A_E2E_ORCA=1`) |
| S19 | LaunchAgent | Run `i2a service install`, then `i2a service status`, then `i2a service uninstall`. | `status` shows `running` and a process ID. After uninstall, `launchctl print` does not find the service. | Local (`I2A_E2E_SERVICE=1`) |
| S20 | Dashboard | Run `i2a config` in a window of 120 × 40. Read the screen. Press `5`, `enter`, type `45m`, press `esc` and `d`. Press `q`. | The header shows `i2a` and the service state. The tab bar shows the 6 tabs (Feed first) and then the unsaved mark. The Overview tab shows the source, the profile and the message counts. The TUI uses the full window. After the discard, the interval is `15m`. After `q`, the shell screen comes back and `config.toml` did not change. | CI |
| S21 | Dry run, then Orca with a long primer | `bob` posts. Run `i2a run --once` with the `dry-run` profile. Change the profile to driver `orca`, session `new`, payload `inline`, and a primer of 14 lines in Vietnamese. `bob` posts again. Run `i2a run --once`. | The second run sends 2 messages: the previewed one and the new one. The screen shows `turn started`. The agent replies about both messages. | Local (`I2A_E2E_ORCA=1`) |
| S22 | Watch one direct conversation | Run `i2a config` and connect the source. In the channel list, type the name of `bob`, select `direct / @bob`. Press `tab`, `n` (all direct off), `tab`, `n` (mentions off), `ctrl+s`, `q`. `bob` posts in the direct channel and in `selected` with `@alice`. Run `i2a run --once`. | `config.toml` lists only the direct channel ID. The digest has only the direct message. | CI |
| S23 | Card feed | `bob` posts. Run `i2a run --once`. Run `i2a card add` with 3 cards (a low info, a high action that refers to the message, a normal question with a link in its body). Run `i2a` with `pbcopy`, `open` and `xdg-open` shims in `PATH`. Press `s` two times, `f` four times, `c`, `o`, `↓`, `o`, `↑`, `d`, `q`. | The Feed shows the action first. `s` shows `sort: newest`. The first `f` shows only the action. `c` copies the suggested reply. `o` opens the URL of the message. On the question, `o` opens nothing and says that the card has no message link. After `d`, `i2a card list --json --all` shows the action `done`, the question `read` and the info card `new`. | CI |
| S24 | Update by key, close, reject | Run `i2a card add` with a card with the key `mm:pr-57`. Run `i2a`. Add the same key again with the urgency `high` and a new title. Add 2 cards where the second has the category `todo`. Run `i2a card done --key mm:pr-57`. | The open Feed shows the new title and `updated` within 5 seconds, with 1 card. The call with `todo` exits with a code that is not 0, names card 2, and saves nothing. After `done`, the Feed has no card. | CI |
| S25 | The agent adds a card | Allow `i2a card` in the workspace settings of Claude Code. Set the profile to `orca`, `reuse`, `inline`. `bob` asks for the release notes before 17:00. Run `i2a run --once`. | Within 4 minutes, `i2a card list --json` has a card from the agent that refers to the message of `bob`. | Local (`I2A_E2E_ORCA=1`) |
| S26 | Reply from the Feed | `bob` posts in `selected` and replies in its thread. Run `i2a run --once`. Run `i2a card add` with a card that refers to both messages. Run `i2a`, press `r`, type a sentence at the end of the suggested reply, press `ctrl+s`, then `q`. | The form shows `Send to selected · mattermost · work`, the two messages, `Thread` and the suggested reply. The status line says `Sent to selected (thread). The card is done.` `alice` has one post in `selected`, in the thread of the first post of `bob` (the root), with the edited text. The `last_viewed_at` of `alice` in `selected` does not change. The card is `done`. | CI |
| S27 | Reply with `i2a reply` | `bob` writes a direct message. Run `i2a run --once`. Run `i2a reply --to <id> < file`. Run it again. Run `i2a reply --to 99999`. Add a card with a suggested reply and the key `mm:s27`, and run `i2a reply --card mm:s27`. | The first call prints `sent to @bob-… (channel)`. The second call exits with 1 and says that the reply was sent at a time. The unknown message exits with 1. `--card` sends the suggested reply and the card is `done`. `alice` has 2 posts in the chat, with no root. The chat is read for `alice` (Mattermost marks it read for a top-level post). | CI |
| S28 | Retry after a lost answer | Add the source through a proxy that forwards the first `POST /posts` and closes the connection before the answer. Run `i2a reply --to <id>`. Wait 35 seconds. Run it again. | The first call says that the result is not known. The second call says `sent to selected (thread …)`. `alice` has 1 post. The reply row is `sent` with the ID of that post. | CI |
| S29 | The agent sends a reply | Allow `i2a card` and `i2a reply` in the workspace settings. Set a primer that tells the agent to thank the sender of a news message with `i2a reply --to <id>`. `bob` posts news in `selected`. Run `i2a run --once`. | Within 4 minutes, `alice` has one post in the thread of the post of `bob`, and the store has a `sent` reply by the agent. | Local (`I2A_E2E_ORCA=1`) |

## Run

```sh
task e2e                          # CI scenarios
I2A_E2E_ORCA=1 task e2e           # also the Orca scenarios
I2A_E2E_SERVICE=1 task e2e        # also the LaunchAgent scenario
```
