## 1. Data

- [x] 1.1 `config.SetSpaces` sets all links of a conversation (spaces, one Unsorted entry, or none), keeps the first label; unit tests
- [x] 1.2 `store.Requeue` with `forMeOnly`; unit test with a delivery that stays `sent`
- [x] 1.3 `conversations-seen.json` read, join and write (0600); first listing seeds an account

## 2. Navigation and header

- [x] 2.1 Three areas in the header, Setup sections on row 1, last run on row 1, content from row 2
- [x] 2.2 `1` `2` `3`, `tab`/`shift+tab` through all sections, mouse zones
- [x] 2.3 Header right: service, next send, `R run now`; drop parts on a narrow window
- [x] 2.4 `R` from each area (the selected space in the Spaces area); last run stays under the header
- [x] 2.5 Footer: main key first, `? keys · q quit` at the right

## 3. Spaces area

- [x] 3.1 Left list: spaces with `●`/`○`, counts; Everything else; New conversations; Ignored; + New space
- [x] 3.2 Panel of a space: Name, Sends to, When rows with dialogs that save at once; last send; conversation rows; recent sends
- [x] 3.3 Conversation row keys: `←`/`→` level, `x` remove, `enter` dialog, `space` select, `n` rename
- [x] 3.4 Everything else: read level of new conversations, agent with hold, schedule, rows newest first, `i` ignore
- [x] 3.5 New conversations: `enter`, `k`, `K`, `i`; Ignored: `x` un-ignore, `enter`
- [x] 3.6 Delete a space with `x` in the left list; new space with enter on "+ New space"
- [x] 3.7 Undo `u` of the last change

## 4. Dialogs

- [x] 4.1 Spaces dialog of one or many conversations, Ignore, + New space, the requeue option
- [x] 4.2 Search of conversations: `/` from each area, `a` for a space with check marks

## 5. Feed and Setup

- [x] 5.1 Space list at the left of the Feed (100 columns or wider), `←`/`→`
- [x] 5.2 Setup sections reuse the forms; Send log is the Activity table; texts name the new areas
- [x] 5.3 Remove `tab_conversations.go` keys `m`, `L`, `l`, `f`; key help

## 6. Tests and docs

- [x] 6.1 Unit tests of the TUI for each requirement; each new test fails once with the code broken
- [ ] 6.2 e2e scenarios that use tabs 4 and 5 and the keys `m`, `L`, `l`
- [x] 6.3 README and CLAUDE.md key tables and tab names
- [ ] 6.4 `task test`, both lints, `task leakcheck`, full `task e2e`
