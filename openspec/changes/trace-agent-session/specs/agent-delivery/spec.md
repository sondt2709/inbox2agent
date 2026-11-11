## MODIFIED Requirements

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

## ADDED Requirements

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
