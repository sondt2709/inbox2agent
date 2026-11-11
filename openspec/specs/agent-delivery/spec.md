# agent-delivery Specification

## Purpose
This capability sends a batch of messages to an AI agent through a driver. It routes each message to a space, sends the batch of the space to its agent, and makes sure that one batch goes into one agent session one time only.
## Requirements
### Requirement: Dry-run driver
The `dry-run` driver SHALL write the digest file and return `sent`. It SHALL NOT start any process. A new agent SHALL use the `dry-run` driver by default. The messages of a dry-run batch SHALL become `previewed`, not `sent`.

#### Scenario: Dry-run dispatch
- **WHEN** a space with a `dry-run` agent sends a batch
- **THEN** the digest file exists, the messages become `previewed`, and no agent starts

#### Scenario: Change from dry-run to a real driver
- **WHEN** a `dry-run` agent previewed 75 messages of a space, and the user changes the agent driver to `orca`, and 1 new message arrives
- **THEN** the next `orca` batch of the space contains 76 messages

### Requirement: Orca new-session mode
In `new` mode, the `orca` driver SHALL run `orca agent launch` with the `cli` of the agent, `--surface terminal` and the workspace selector, and without `--prompt`. Then the driver SHALL wait until the agent TUI is idle and submit the prompt of the batch, as the requirement "Prompt submission" says.

#### Scenario: New session for each batch
- **WHEN** two batches of a space go to an agent in `new` mode
- **THEN** Orca shows two agent terminals, and each agent started a turn with the prompt of one batch

### Requirement: Prompt submission
The `orca` driver SHALL submit a prompt with `orca terminal send --text <prompt> --enter --wait-submit 30`. Before the first prompt to a new terminal, the driver SHALL check each second, for up to 90 seconds, that the agent runs in the terminal: exactly one interactive Claude process when the `cli` is Claude Code and i2a can read the Claude processes, else Orca reports `agentIdentity`. When no agent runs after 90 seconds, the driver SHALL return the error "no agent started in the terminal <handle> after 1m30s; i2a sent nothing", SHALL NOT type into the terminal, and SHALL NOT store the session. After the agent runs, the driver SHALL wait up to 90 seconds for `tui-idle`. When the send result has `observation: "unsupported"`, no agent runs in the terminal: the driver SHALL return the error "no agent runs in <handle>", SHALL NOT send Enter again, and SHALL delete the stored session of the space. When the send result does not include the stage `turn_started` and the observation is not `unsupported`, the driver SHALL send one bare Enter to the terminal and record the detail "turn not proven, Enter sent again". The driver SHALL NOT send the prompt text again.

#### Scenario: Prompt left in the input box
- **WHEN** `orca terminal send` returns `accepted: true` and the stages are only `input_accepted`
- **THEN** the driver sends one `orca terminal send --enter` with no text, and the dispatch status is `sent`

#### Scenario: Long multi-line prompt
- **WHEN** a new session gets an inline prompt with an instruction of 15 lines with non-ASCII text
- **THEN** the send result includes `turn_started`, and the input box of the agent is empty

#### Scenario: Send into a shell
- **WHEN** `orca terminal send` returns `observation: "unsupported"`
- **THEN** the driver sends no Enter, the dispatch status is `failed` with "no agent runs in", the messages stay `pending`, and the store has no session for the space

#### Scenario: Agent starts after the shell
- **WHEN** a new terminal shows a shell for 2 seconds before Claude Code starts in it
- **THEN** the driver sends the prompt only after Claude Code runs, and the dispatch status is `sent`

#### Scenario: Agent does not start
- **WHEN** no agent starts in a new terminal within 90 seconds
- **THEN** the driver types nothing into the terminal, the dispatch status is `failed` with "no agent started in the terminal", and the messages stay `pending`

### Requirement: Orca reuse-session mode
In `reuse` mode, the `orca` driver SHALL keep for each space in the store one terminal handle and the Claude session ID of the agent in that terminal. Before a send, the driver SHALL check the session as the requirement "Claude session of a terminal" says. When the session is valid and the agent is idle, the driver SHALL send the prompt of the batch with `orca terminal send --enter`. When no valid session exists, the driver SHALL resume the stored Claude session as the requirement "Resume a Claude session" says, else start a new session with the prompt of the batch, and store the new handle and the Claude session ID for the space. The driver SHALL NOT start a new session because the instruction, the CLI instruction or the template changed. The driver SHALL NOT close the old terminal. When the agent is busy, the driver SHALL return `held`, and the messages SHALL stay `pending`.

#### Scenario: Second batch in the same session
- **WHEN** two batches of one space go to an agent in `reuse` mode, and the agent is idle before the second batch
- **THEN** both batches arrive in the same terminal, and each has the instruction, the content and the CLI instruction

#### Scenario: Two spaces with one agent
- **WHEN** the spaces `acme` and `borealis` use the same agent in `reuse` mode, and each sends a batch
- **THEN** the batches arrive in two different terminals, and the store has one handle and one Claude session ID for each space

#### Scenario: Terminal closed
- **WHEN** the stored terminal of a space was closed, the stored row has no Claude session ID, and a new batch of the space arrives
- **THEN** the driver starts a new session, submits the batch, and stores the new handle

#### Scenario: Claude exited in the terminal
- **WHEN** the user ran `/exit` in the stored terminal, so the terminal runs zsh, and a new batch of the space arrives
- **THEN** the driver sends nothing to that terminal, starts a new terminal, resumes the stored Claude session in it, and submits the batch there

#### Scenario: Another Claude session in the terminal
- **WHEN** the user ran `/clear`, or quit Claude and started `claude` again, in the stored terminal
- **THEN** the driver sends nothing to that terminal, and starts a new session with the batch

#### Scenario: Linked session
- **WHEN** the user linked the terminal `term_c` to the space, and a batch of the space arrives
- **THEN** the batch arrives in `term_c`

#### Scenario: Instruction changed
- **WHEN** the stored terminal is live and runs the stored Claude session, and the user changed the instruction of the agent
- **THEN** the batch arrives in the stored terminal with the new instruction, and the dispatch detail does not say "started session"

#### Scenario: Session stored by an older version
- **WHEN** the stored row has a primer hash from an older version of i2a, and the terminal runs the stored Claude session
- **THEN** the batch arrives in the stored terminal

#### Scenario: Agent busy
- **WHEN** the agent does not become idle within 5 seconds
- **THEN** the dispatch status is `held`, the messages stay `pending`, and the next send time of the space sends them

### Requirement: Orca worktree mode
In `worktree` mode, the `orca` driver SHALL create a new Orca worktree named `i2a-<batch-id>` in the workspace repository, with the `cli` of the agent and without `--prompt`. Then the driver SHALL submit the prompt of the batch, as the requirement "Prompt submission" says.

#### Scenario: Worktree for a batch
- **WHEN** a batch of a space goes to an agent in `worktree` mode
- **THEN** Orca lists a worktree named `i2a-<batch-id>` with an agent terminal that received the prompt

### Requirement: Orca failures
When the `orca` executable is missing, the Orca runtime is not reachable, or the JSON result reports `ok: false`, the driver SHALL return an error with the Orca error text. The messages SHALL stay `pending`.

#### Scenario: Orca not reachable
- **WHEN** the `orca` executable exits with a non-zero code
- **THEN** the dispatch status is `failed` with the Orca error text, and the messages stay `pending`

### Requirement: No duplicate send into one session
The `orca` driver SHALL NOT send the same batch text two times to one terminal.

#### Scenario: Unproven submission
- **WHEN** `orca terminal send` returns `accepted: true` and the stages do not include `turn_started`
- **THEN** the driver does not send the text again, and the dispatch status is `sent`

### Requirement: Queued asks in a dispatch
The daemon SHALL send the `queued` asks at least one time each minute, independent of the schedules, with the wait of 5 seconds of the reuse mode. A busy agent SHALL keep the ask `queued`. An Orca error SHALL make the ask `failed` with the error text.

#### Scenario: Queued ask sent
- **WHEN** an ask is `queued` because the agent was busy, the schedule of the space sends every 2 hours, and the agent is idle 1 minute later
- **THEN** the daemon sends the ask within 2 minutes, and the ask is `sent`

### Requirement: Spaces and batches
The system SHALL send each pending message to each space that has a link to its conversation and whose read level allows the message, or to Unsorted when no space lists the conversation. The routes SHALL be computed when a space sends, so a moved conversation takes its pending messages to its new space. The system SHALL make one batch for each space that is not paused and has pending messages at its send time, and SHALL send it to the agent of the space with the instruction of the space. A paused space SHALL NOT send. A space SHALL get a message one time: a message that a space has (sent, or previewed by a dry run of that space) SHALL NOT go to that space again, except a previewed message to a real driver. A failed batch of one space SHALL NOT change the messages of another space. The dispatch SHALL record the space and the description of its agent (`dry-run`, or `orca · <cli>`). The run summary SHALL be `batch of <space name> (<agent description>, <n> messages): <status>`.

#### Scenario: Two spaces from one account
- **WHEN** the account `phone` has a new message in a chat of `acme` and one in a chat of `personal`, and both spaces are due
- **THEN** there is one batch of `acme` and one batch of `personal`, each to the agent of its space

#### Scenario: One conversation in two spaces
- **WHEN** the channel `release` is in Review and in Team, both with the level `all`, and a member posts one message
- **THEN** Review and Team each get one batch with the message, each to its own agent

#### Scenario: One space fails
- **WHEN** the channel `release` is in Review and in Team, and the batch of Team fails
- **THEN** the message has a delivery `sent` for Review, the next batch of Team contains it, and the next batch of Review does not

#### Scenario: Run summary
- **WHEN** Unsorted has a `dry-run` agent and sends 3 messages
- **THEN** `i2a run --once` prints `batch of Unsorted (dry-run, 3 messages): sent`

### Requirement: One delivery at a time for each space
The system SHALL hold an exclusive lock on the file `<data>/space-<id>.lock` while a batch or an ask goes to the agent of the space. A batch SHALL wait up to 3 minutes for the lock; after that, the dispatch status SHALL be `held` and the messages SHALL stay `pending`. An ask SHALL wait up to 3 minutes for the lock; after that, the ask SHALL stay `queued`.

#### Scenario: Ask and batch at the same time
- **WHEN** the user sends an ask about a card of `acme` while the daemon sends a batch of `acme`
- **THEN** the agent terminal gets the two prompts one after the other, and neither prompt contains text of the other

### Requirement: Claude session of a terminal
When the `cli` of the agent is `claude` or starts with `claude`, the driver SHALL find the Claude processes of a terminal from the files `<claude dir>/sessions/<pid>.json` (`CLAUDE_CONFIG_DIR`, else `~/.claude`) and the environment variable `ORCA_TERMINAL_HANDLE` of each running pid. Only a process with the `kind` `interactive` (or no `kind`) SHALL count: a background process keeps the variable, but the terminal shows the shell. The stored session SHALL be valid when the stored terminal has exactly one Claude process with the stored Claude session ID and Orca reports the terminal live. When the stored row has no Claude session ID and the terminal has exactly one Claude process, the driver SHALL store that session ID. When the stored terminal does not run the stored session and exactly one other terminal runs it, the driver SHALL use that terminal, and store its handle. When i2a cannot read the Claude processes, or the `cli` is another agent, the session SHALL be valid when Orca reports the terminal live and `agentIdentity` is not null. After the agent is idle and just before the driver types, it SHALL check again that the terminal runs the session; when it does not, the driver SHALL send nothing to that terminal and SHALL start a new session (an ask goes to the agent of the space).

#### Scenario: Session moved to another tab
- **WHEN** the stored session `S` of the space no longer runs in `term_a`, and the user resumed `S` in `term_b`
- **THEN** the batch goes to `term_b`, and the store has `term_b` for the space

#### Scenario: Session in two tabs
- **WHEN** the stored session `S` runs in `term_b` and in `term_c`, and not in the stored terminal
- **THEN** the driver starts a new session

#### Scenario: Store upgraded from version 7
- **WHEN** the stored row has the handle `term_a`, no Claude session ID, and `term_a` runs Claude session `S`
- **THEN** the batch goes to `term_a`, and the store has `S` for the space

#### Scenario: Claude moved to the background
- **WHEN** the user chose "Move to background and exit" in the stored terminal, so a Claude process with the `kind` `bg` has the handle of the terminal
- **THEN** the driver sends nothing to that terminal and starts a new session

#### Scenario: Claude exits during the wait
- **WHEN** the check finds the stored session in the terminal, and the agent exits while the driver waits for `tui-idle`
- **THEN** the driver sends nothing to that terminal and starts a new session with the batch

#### Scenario: Unknown file format
- **WHEN** `~/.claude/sessions` holds files that do not decode
- **THEN** the driver uses the Orca check with `agentIdentity`, and the batch is not refused for this reason

### Requirement: Tab name of a new session
After the driver starts a terminal (new, reuse and worktree modes), it SHALL run `orca terminal rename --terminal <handle> --title "i2a · <space name>"`. A failed rename SHALL add a note to the detail of the dispatch and SHALL NOT change its status. The driver SHALL NOT rename a terminal that it did not start.

#### Scenario: New session of the space Unsorted
- **WHEN** the agent of Unsorted starts a new session
- **THEN** the Orca tab of the session shows "i2a · Unsorted"

### Requirement: Resume a Claude session
In `reuse` mode, when the `cli` of the agent is Claude Code, i2a can read the Claude processes, the stored row of the space has a Claude session ID `S`, the stored terminal does not run another Claude session, and no Claude process runs `S`, the driver SHALL resume `S`. It SHALL start a terminal with `orca agent launch`, wait for the agent and for `tui-idle`, and type `/resume S` with `orca terminal send --enter`. `S` SHALL match the UUID pattern; else the driver SHALL NOT type it. The driver SHALL then read the Claude process of the terminal each second for at most 10 seconds. When its session ID is `S`, the driver SHALL send the prompt of the batch to the terminal, store the handle, `S` and the session name for the space, and the dispatch detail SHALL start with `resumed Claude session`. When the session ID does not become `S`, the driver SHALL name and use the new session, and the detail SHALL start with `could not resume Claude session`. The driver SHALL NOT resume in the `new` and `worktree` modes, for another CLI, or when the user unlinked the session.

#### Scenario: Orca restarted
- **WHEN** the stored terminal of the space Triage is gone, the stored Claude session `S` runs nowhere, and a batch of Triage arrives
- **THEN** the batch arrives in a new terminal that runs `S`, the store has the new handle and `S`, and the detail starts with "resumed Claude session"

#### Scenario: Session cleared by the user
- **WHEN** the stored terminal runs another Claude session after `/clear`
- **THEN** the driver types no `/resume`, and starts a new session

#### Scenario: Session runs in a terminal of another app
- **WHEN** the stored terminal is gone and a Claude process without an Orca handle runs `S`
- **THEN** the driver types no `/resume`, and starts a new session

#### Scenario: Transcript not found
- **WHEN** Claude cannot open `S` in the folder of the agent, so the session ID of the terminal does not change
- **THEN** the batch arrives in the new session, the new session has the name `i2a · <space name>`, and the detail starts with "could not resume Claude session"

### Requirement: Name of a new Claude session
After the driver starts a new Claude session and before it sends the first prompt, it SHALL type `/rename i2a · <space name>` and wait for `tui-idle`. The name SHALL be one line with no control characters and at most 80 characters. A failed rename SHALL add a note to the detail of the dispatch and SHALL NOT change its status, except when Orca reports that the text went to a shell: then the send SHALL fail and the driver SHALL send no prompt. A resumed session SHALL keep its name.

#### Scenario: New session of Unsorted
- **WHEN** the agent of Unsorted starts a new Claude session
- **THEN** the Claude session file of the terminal has the name "i2a · Unsorted"

### Requirement: Last send of a session
After each `sent` result in the stored session of a space (a batch or an ask), the driver SHALL store the time of the send and the current Claude session name for the space.

#### Scenario: Name changed by the user
- **WHEN** the user ran `/rename review` in the session of the space, and the next batch is sent there
- **THEN** the store has the name "review" and the time of that send for the space

### Requirement: Orca agent of a session
The driver SHALL record for the stored session of a space the Orca agent that runs it: `claude-agent-teams` when its Claude process has a non-empty `ORCA_AGENT_TEAMS_TEAM_ID` in its environment, else `claude`. It SHALL record it when it starts, adopts or follows a session, and the dashboard SHALL record it when the user links a session. A resume SHALL launch the recorded Orca agent; when none is recorded, it SHALL launch the `cli` of the agent of the space. A new session SHALL launch the `cli`. The driver SHALL NOT pass `--model` to `orca agent launch` or `orca worktree create` for `claude-agent-teams`.

#### Scenario: Resume an agent-teams session
- **WHEN** the stored session of Triage ran in a `claude-agent-teams` terminal that is gone, and the `cli` of Triage is `claude`
- **THEN** the driver runs `orca agent launch --agent claude-agent-teams` with no `--model`, and types `/resume <id>`

#### Scenario: Linked agent-teams session
- **WHEN** the user links a terminal whose Claude process has `ORCA_AGENT_TEAMS_TEAM_ID`
- **THEN** the store has the Orca agent `claude-agent-teams` for the space

### Requirement: Send to an agent that Orca cannot observe
When `orca terminal send` returns `observation: "unsupported"` and i2a reads exactly one interactive Claude process in the terminal, the driver SHALL treat the text as sent to the agent: the result SHALL be `sent` with the detail "Orca cannot observe this agent", and the driver SHALL NOT send Enter again. When no Claude process runs in the terminal, or i2a cannot read the Claude processes, `unsupported` SHALL stay the error "no agent runs in <handle>".

#### Scenario: Batch to an agent-teams session
- **WHEN** the stored terminal runs the stored Claude session under `claude-agent-teams`, and Orca returns `observation: "unsupported"` for the prompt
- **THEN** the dispatch status is `sent`, the detail has "Orca cannot observe this agent", and the store keeps the session

#### Scenario: Send into a shell with the Claude files
- **WHEN** Orca returns `observation: "unsupported"` and no Claude process runs in the terminal
- **THEN** the dispatch status is `failed` with "no agent runs in", and the store has no session for the space

