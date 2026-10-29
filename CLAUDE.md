# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`i2a` (inbox2agent) is a Go CLI. It polls chat and work tools for new messages and sends them in batches to an AI agent. The agent sends its results back as cards with `i2a card add`; the user reads them in the Feed tab of `i2a`. Sources: Mattermost now; WhatsApp, GitHub and Linear later. Drivers: `orca` (the Orca CLI) and `dry-run`. One static binary (`CGO_ENABLED=0`), macOS first.

## Commands

```sh
task build                                   # bin/i2a
task test                                    # go test -race ./...
go test ./internal/render -run TestAttack    # one test
task lint                                    # golangci-lint v2 (.golangci.yml)
golangci-lint run --build-tags e2e ./test/...
task e2e                                     # starts Docker Mattermost, runs the CI scenarios
I2A_E2E_ORCA=1 I2A_E2E_SERVICE=1 task e2e    # also the Orca and LaunchAgent scenarios (local only)
task leakcheck                               # forbidden terms from .leakcheck.local
task dist -- 1.2.0                           # release archives and checksums.txt in dist/
internal/source/mattermost/testdata/capture.sh   # capture Mattermost fixtures again (needs a new server)
docs/demo/record.sh                          # record docs/demo/i2a-config.gif with VHS
```

The e2e Mattermost server is `test/e2e/mattermost/compose.yaml`. It runs as `linux/amd64` (no arm64 image). Its first user must be `e2e-admin`. If the harness reports that `e2e-admin` is not an admin, or user creation fails with `ERROR_SAFETY_LIMITS_EXCEEDED` (each run creates users), run `docker compose -f test/e2e/mattermost/compose.yaml down -v` and start it again.

## Architecture

Data flow: `source.Source.Collect` → `store.SavePoll` (messages and cursor in one transaction) → `daemon.Engine.Dispatch` → `agent.Route` → `render.Render` (sanitize, digest file, prompts, card section) → `agent.Driver.Deliver` → `store.FinishDispatch`.

Return path: the agent runs `i2a card add` (JSON on stdin) → `card.Parse` and `card.Prepare` (validate, sanitize, cut) → `store.SaveCards` (one transaction, upsert by key) → `card.Plan` → notifications. The Feed tab reads the cards every 2 seconds. `i2a card add` writes the store directly; it does not talk to the daemon.

- `internal/inbox`: shared types (`Message`, `Cursor`, `Card`, states). It prevents import cycles between the source, store, render and agent packages.
- `internal/source`: the `Source` interface and `Build`. A source returns messages and the next cursor. It never writes to the store.
- `internal/source/mattermost`: a small typed REST client, not the official module. Polling uses `GET /channels/{id}/posts?page&per_page` (newest first, stop at the cursor). Do not use `posts?since=`: it filters on `update_at` and has an unordered `LIMIT 1000`. Never call an endpoint that marks a channel as read.
- `internal/render`: the prompt-injection boundary. Each message is one `json.Marshal` line inside a fence labelled `i2a-data-<nonce>`. Hidden characters are removed before encoding. Keep every data line starting with `{`.
- `internal/card`: the input of `i2a card add` and the notification rule (`Plan`). Card text comes from an agent that read untrusted messages, so it gets the same sanitizing as message text.
- `internal/agent`: the drivers. In `reuse` mode the store keeps the primer hash with the handle; a different hash starts a new session. Delivery is at least once: messages become `sent` only after the driver returns `sent`; `held` and errors keep them `pending`. A `dry-run` batch makes its messages `previewed`; a real driver still sends `previewed` messages.
- `internal/daemon`: `Engine` (poll, dispatch, prune) and `Run` (loop, configuration reload, `flock` lock).
- `internal/tui`: the dashboard. `i2a` opens it on the Feed tab (`tab_feed.go`), `i2a config` on Overview. The Feed opens only the stored URL of a source message, never a link from card text. `model.go` has the layout (header, tab bar, content, footer), the focus (list or form), the global keys, the mouse zones and the dialog stack. Each tab is one file (`tab_*.go`). Add and edit are forms in the detail panel, made with the small form engine in `form.go` (text inputs, segmented choices and switches, text areas, the channel picker from `picker.go`, buttons, errors under the fields). `buildForm` makes the form of the selected item again when the selection changes or after a save. A form saves through `ui.commit`, which validates and writes the file and the token; there is no global working copy. Dialogs (`dialog.go`) are only for confirmations, the digest viewer and the key help.
- `internal/notify`: macOS notifications through `osascript`. `i2a card add` notifies for cards; the daemon notifies only for failed dispatches. The title and the text are script arguments, never script code. `I2A_NOTIFY=off` turns them off (the e2e harness sets it).
- `internal/service`: the macOS LaunchAgent `io.github.sondt2709.i2a`.

Facts about the `orca` CLI (1.4.240) that the driver depends on:

- `orca terminal show` returns `ok: true` for a closed terminal. A terminal is live when `connected` and `writable` are true and `exitCause` is null.
- `orphaned` is true for a running agent terminal that has no visible pane. Do not use it as a liveness signal.
- `--worktree path:<dir>` fails with `selector_not_found` when Orca does not know the folder. `orca repo add` accepts git repositories only.
- `orca terminal wait --for tui-idle` returns the error code `timeout` when the agent is busy.
- Do not pass `--prompt` to `orca agent launch` or `orca worktree create`. Orca types it while the agent starts, and it can stay in the input box with no Enter (`prompt.outcome` is still `handed-to-terminal`), or not arrive (`not-delivered`). The driver launches with no prompt, waits for `tui-idle`, then runs `orca terminal send --enter --wait-submit 30`. The stage `turn_started` proves the submission.
- `orca terminal send --enter` with no `--text` sends a bare Enter.
- Prompts are command arguments (the RPC limit is 1 MiB). Keep `inline_max_bytes` at 65,536 or less.

The fixtures in `internal/agent/testdata` are real Orca outputs with local paths replaced.

## Rules

- **Writing:** use ASD-STE100 style for all text (docs, comments, errors, OpenSpec artifacts, PRs). Use short sentences, the active voice and one term for one concept. Name the specific file, command or value.
- **No personal data in committed files:** no local paths, real server names, real usernames or other project names. `task leakcheck` checks the terms in `.leakcheck.local` (not committed). `tmp/` holds local secrets and is git-ignored.
- **Fixtures must copy real responses.** Mattermost fixtures come from `capture.sh` against the Docker server. Orca fixtures come from a real `orca` run with the paths replaced. Do not write the shape of external data by hand.
- **A new test must be able to fail.** Break the code one time and confirm that the test fails, then restore the code.
- **E2E tests drive the real binary.** They use `test/e2e/harness` (a detached tmux session on a private socket). Wait for the screen text before you send keys. Use `Term.Press`, which sends a key again when it changed nothing. Huh can drop the first key while a form starts. Wait for text that shows only on the target screen: the dashboard draws dialogs over the tab, so the tab text stays visible under a dialog. The footer keys of a tab (for example `enter edit`) show only when no dialog is open.
- In some agent sessions, the `tmux` in `PATH` is an Orca shim. The harness skips paths that contain `/.orca/`. Set `I2A_E2E_TMUX` to choose a binary.
- **OpenSpec:** significant changes go through `openspec/changes/<name>/` (proposal, design, specs, tasks) before code.
- **PR titles** use Conventional Commits (`.github/workflows/pr-title.yml`). Merges are squash-only. The title becomes the commit subject on `main`.
- **Releases** are automatic. After all CI jobs pass on a push to `main`, the `release` job in `ci.yml` runs semantic-release (`.releaserc.json`). Each push makes a new version: `!` or a `BREAKING CHANGE` footer is major, `feat` is minor, all other types are patch. `scripts/dist.sh` builds the archives. The npm packages have exact versions in `ci.yml`; change them together. Keep `conventional-changelog-conventionalcommits` on 9.x while `@semantic-release/release-notes-generator` uses `conventional-changelog-writer` 8. Before you change a version, do a full `semantic-release --dry-run` against a local Git repository and check that `generateNotes` passes.
