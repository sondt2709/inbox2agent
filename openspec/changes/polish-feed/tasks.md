## 1. Tests that fail on the base

- [x] 1.1 Test: the Feed rows show the urgency bars, and the detail shows `▰▰▰ high`
- [x] 1.2 Test: `F` opens the dialog with the card counts; enter on a space filters; a removed space resets the filter
- [x] 1.3 Test: `F` in Activity filters the sends; one filter for both tabs
- [x] 1.4 Test: the Feed footer at 140 columns has no cut hint
- [x] 1.5 Test: the key help at 140 × 34 shows the line "Feed" at its start
- [x] 1.6 Test: `i2a run --help` and `i2a daemon --help` do not contain "profile" or "source"

## 2. Feed and Activity

- [x] 2.1 Urgency bars in the list and the detail panel (`urgLabel`, `urgBars`)
- [x] 2.2 `model.spaceFilter` (space ID) replaces `model.feedSpace`; reset when the space does not exist
- [x] 2.3 `F` opens `pickDialog` with counts in the Feed and in Activity
- [x] 2.4 Activity filters the dispatches and shows the space in its title; `F` in the Activity hints

## 3. Footer, help and CLI text

- [x] 3.1 `fitHints` in `style.go`; `renderFooter` uses it
- [x] 3.2 The key help is a `viewerDialog` with a width; shorter help lines
- [x] 3.3 Help and errors of `i2a run` and `i2a daemon`

## 4. Check

- [x] 4.1 Break each change one time and see the new test fail
- [ ] 4.2 `task test`, `task lint`, e2e lint, `task e2e`, `task leakcheck`, `openspec validate --strict`
- [ ] 4.3 Real binary at 140 × 34 in tmux: Feed, `F` dialog, Activity, `?`; save the screens in `tmp/evidence/polish-feed/`
- [ ] 4.4 Archive the change and open the PR
