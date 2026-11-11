## 1. Footer

- [x] 1.1 `renderHints` writes `Label key`: the label with a capital first letter in the accent color, the key in grey
- [x] 1.2 The hints of a confirmation keep the case of the label
- [x] 1.3 Unit test `TestFooterHintIsLabelThenKey`; update the footer text in the unit and e2e tests

## 2. Add dialog

- [x] 2.1 `model.dialogBox` and `dialogPoint` give the position of a dialog box
- [x] 2.2 `searchDialog` keeps the changes in `pending` and saves them with OK in one `editLinks`
- [x] 2.3 OK and Cancel buttons: `tab`, `←`/`→`, `enter`, `ctrl+s`, `esc` and a mouse click
- [x] 2.4 The title "Add conversations to <space>", the mark `*` and "N changes not saved"
- [x] 2.5 `TestAddConversationsWithTheSearch`: no save before OK, esc, Cancel, OK and a click on OK

## 3. Docs and checks

- [x] 3.1 `README.md` and `CLAUDE.md`
- [x] 3.2 `task test`, `task lint`, `task leakcheck`, `task e2e`
