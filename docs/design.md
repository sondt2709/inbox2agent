# i2a design system

This file is the design system of i2a. It applies to these surfaces:

| Surface | Status | Code |
|---|---|---|
| TUI (`i2a`, `i2a config`) | live | `internal/tui/style.go` |
| Android app | first version: notifications and the Feed (read only) | `mobile/androidApp` |
| iOS app | placeholder | `mobile/iosApp/README.md` |
| Web dashboard | future | none |

The tokens are in `docs/design-tokens.json`. A Go test (`internal/tui/tokens_test.go`) checks that `internal/tui/style.go` and `mobile/shared/.../DesignTokens.kt` use the same values. Change a color in the JSON file first, then in the code.

## 1. Principles

1. **The terminal comes first.** The TUI defines the look. The other surfaces copy the TUI. They do not get their own brand.
2. **There is one brand color.** The primary color is `#7D56F4` on all surfaces. The Android app does not use dynamic color (Material You), because dynamic color replaces the primary color with the wallpaper colors.
3. **Color is never the only signal.** A category has a text label (`ACTION`). An urgency has bars (`▰▰▱`) and a text alternative ("urgency normal"). An unread card has a dot and a bold title.
4. **The screens are dense and quiet.** The user reads many short cards. Use small gaps, few borders and no decoration. Motion only shows a change of state.
5. **Card text is untrusted.** An agent writes the card text after it reads untrusted messages. No surface makes a link from card text. A surface opens only the stored URL of a source message (`http` or `https`).
6. **Use the same words on all surfaces.** Card, space, Feed, Done, suggested reply, source message. Write all text in ASD-STE100 style.

## 2. Color

The TUI is dark, so the design is dark first. The first Android version has a dark theme only. A light theme is future work, and it must keep the primary color.

### 2.1 Brand and surfaces

| Token | Value | TUI (`style.go`, `form.go`) | Android (Material 3 role) | Use |
|---|---|---|---|---|
| `brand.primary` | `#7D56F4` | `accent` | `primary` | Fills: active tab, brand mark, primary button, dialog title. Not for text on a dark surface (contrast 3.9:1). |
| `brand.onPrimary` | `#FFFDF5` | text on `accent` | `onPrimary` | Text and icons on `brand.primary` (contrast 4.6:1). |
| `brand.primarySoft` | `#B9A8FF` | `labelFocus` | `secondary`, link text | Text and icons in the brand color on a dark surface (contrast 8.8:1). |
| `brand.primaryDim` | `#3B2F6B` | `accentDim` | `primaryContainer` | The selected row. |
| `surface.base` | `#15131F` | the terminal background | `background`, `surface` | The page. |
| `surface.raised` | `#1E1B2E` | `headerStyle` | `surfaceContainer` | The header, the top bar, the bottom bar. |
| `surface.input` | `#2A2740` | `inputBg` | `surfaceContainerHigh` | A text input, a box (suggested reply, source message). |
| `surface.inputFocus` | `#3A3360` | `focusBg` | `surfaceContainerHighest` | A focused input. |
| `surface.button` | `#3A3A4E` | `btnNormal` | `secondaryContainer` | A normal button. |
| `line.border` | `#4A4A5E` | `borderFg` | `outline` | Panel borders, dividers. |
| `text.default` | `#D0D0D0` | `fgColor` (xterm 252) | `onSurface` | Body text. |
| `text.muted` | `#808080` | `mutedFg` (xterm 244) | `onSurfaceVariant` | Meta lines, hints, times, on `surface.base` and `surface.raised` only (4.6:1). |
| `text.mutedOnBox` | `#A8A8A8` | not used yet (xterm 248) | `outlineVariant` text | Muted text on `surface.input` and `surface.inputFocus` (6.0:1 and 4.9:1). `text.muted` is 3.6:1 there. |

### 2.2 Meaning

| Token | Value | TUI | Use |
|---|---|---|---|
| `state.ok` | `#3FB950` | `green` | Success, "service running", sent. |
| `state.warn` | `#F2C94C` | `yellow` | Warnings, unsaved changes. |
| `state.error` | `#F85149` | `red` | Errors, failed, high urgency. |
| `state.info` | `#58A6FF` | `blue` | Information. |
| `mark.unread` | `#F778BA` | `unreadMark` | The unread dot. Only for this use. |
| `danger.bg` / `danger.fg` | `#5A1E26` / `#FFD7DC` | `btnDanger` | Destructive buttons (delete, unpair). |
| `category.fg` | `#111111` | category badge text | Text on a category badge. |

| Category | Badge background | Label |
|---|---|---|
| action | `state.error` | `ACTION` |
| question | `state.warn` | `QUESTION` |
| info | `state.info` | `INFO` |

| Urgency | Bars | Color | Text alternative |
|---|---|---|---|
| high | `▰▰▰` | `state.error`, bold | "urgency high" |
| normal | `▰▰▱` | `text.default` | "urgency normal" |
| low | `▰▱▱` | `text.muted` | "urgency low" |

## 3. Typography

| Surface | Face |
|---|---|
| TUI | The monospace font of the terminal. Use bold for weight. Do not use italics for meaning. |
| Android | The system sans (Roboto) for text. The system monospace for data: times, keys, IDs, urgency bars, labels. No bundled fonts. |
| iOS (future) | SF Pro and SF Mono. |
| Dashboard (future) | The system sans, and JetBrains Mono for data. |

Android type scale (sp, line height in brackets):

| Token | Size | Weight | Use |
|---|---|---|---|
| `type.screenTitle` | 20 (28) | bold | The title in the top bar. |
| `type.cardTitle` | 18 (24) | bold | The title on the card screen. |
| `type.rowTitle` | 15 (20) | semibold, bold when unread | The title in a Feed row. |
| `type.body` | 15 (22) | regular | The card body, the source messages. |
| `type.meta` | 12 (16) | regular, monospace | Group, space, time, key. |
| `type.label` | 11 (14) | semibold, monospace, all caps, 0.06 em | Box labels, badges. |

The layout must work at the system font scale of 200%.

## 4. Space, shape and size

| Token | TUI | Android | Use |
|---|---|---|---|
| `space.xs` | 0 | 4 dp | Inside a badge. |
| `space.s` | 1 column | 8 dp | Between a badge and its text. |
| `space.m` | 1 line | 12 dp | Between rows of a box. |
| `space.l` | 2 columns | 16 dp | The screen edge, between sections. |
| `space.xl` | — | 24 dp | Above a section title. |
| `radius.badge` | — | 4 dp | Category badge, brand mark. |
| `radius.chip` | — | 999 dp | Filter chips. |
| `radius.box` | rounded border | 12 dp | Boxes and dialogs. |
| `size.touch` | — | 48 dp minimum | Each tap target. |
| `size.icon` | — | 24 dp | Each icon. |

## 5. Icons

- TUI: text glyphs only: `◆` (brand), `●` (unread, status), `▰ ▱` (urgency), `▌` (selected row), `│` and `→` (tab bar).
- Android: Material Symbols, outlined, 24 dp, one style on one screen. The notification small icon is a white `◆` vector.
- Do not use emoji as icons.
- A decorative icon next to text has no content description. An icon-only button has one.

## 6. Motion

- TUI: no animation. A state change shows at the next frame.
- Android: 150 ms fades and the default Material 3 transitions. The app obeys the system setting "Remove animations".

## 7. Components

Each component has one name on all surfaces.

| Component | TUI | Android | Rules |
|---|---|---|---|
| Brand mark | `◆ i2a` on `brand.primary` | `i2a` badge on `brand.primary`, top bar | One brand mark on each screen. |
| Filter chips | `All 3  Action 1 …` line | Chip row, the selected chip filled with `brand.primary` | Show the count in each chip. |
| Category badge | Bold `#111111` on the category color | Same | Always with the text label. |
| Urgency bars | `urgBar` | Same glyphs, monospace | Always with a text alternative for TalkBack. |
| Card row | Two lines: dot, badge, bars, title; group and time | Three lines: badge, bars and time; title; group. 72 dp minimum height. A phone is too narrow for the title next to the badge. | Unread: dot `mark.unread` and bold title. |
| Card screen | Detail panel | Full screen | Order: meta line, title, body, source messages, suggested reply, actions. |
| Suggested reply | `┃` quote block | Box with a 3 dp `brand.primary` left bar | Copy is always possible. |
| Status line | One line in the footer, `okStyle` or `errStyle` | Snackbar, 4 seconds | One message at a time. |
| Button | `btnPrimary`, `btnNormal`, `btnDanger` | Filled (primary), tonal (normal), filled `danger.bg` (danger) | One primary button on a screen. |
| Empty state | One faint line | Icon, one sentence, one primary button | Tell the user the next step. |

## 8. Android notifications

A notification is the main surface of the first Android version.

| Item | Value |
|---|---|
| Channels | `cards_high` (importance high, heads-up), `cards_normal` (default), `cards_low` (low, no sound). The urgency of the card selects the channel. |
| Small icon | The white `◆` vector. |
| Color | `brand.primary` (`setColor`). Android uses it for the app name and the icon circle. |
| Title | The card title. |
| Text | The group, then the first line of the body. |
| Sub text | `ACTION · high`. |
| Group | `i2a.cards`, with a summary notification "N new cards". |
| Actions | **Copy reply** (when the card has a suggested reply). **Open thread** (when a source message has a stored URL). |
| Tap | Opens the card screen. The card becomes read. |
| Privacy | `VISIBILITY_PRIVATE`. The lock screen shows "i2a · new card" until the phone is unlocked. |

The Mac decides which card gives a notification. It uses the rules of the desktop notification (`card.Plan`): a new card, a higher urgency, or a card back from done, at the minimum urgency of `[notify]`. Other cards go to the phone with no notification, so the Feed on the phone stays complete.

## 9. Android screens (first version)

1. **Feed.** Top bar: brand mark, "Feed", the time of the last push. Filter chips: All, Action, Question, Info. A list of card rows, newest first; high urgency first is a later option. An empty Feed shows "No cards yet" and the button **Pair with your Mac** when the phone is not paired.
2. **Card.** The meta line (badge, bars, group, time), the title, the body, the source messages, the suggested reply. Bottom bar: **Copy reply**, **Open thread**, **Hide**. Hide removes the card from the phone only; the Mac does not change.
3. **Pair.** Steps: run `i2a phone pair` on the Mac, tap **Scan QR code**. A text field accepts the pairing link as a second way (for a test device with no camera).
4. **Settings.** The paired Mac, the date of the pairing, the notification permission, **Unpair** (danger button).

Reply, Ask and Done from the phone are future work. They need a path from the phone to the Mac. The screens keep space for them in the bottom bar of the card screen.

## 10. Future surfaces

- **iOS.** The shared Kotlin module keeps the payload, the decryption and the card model in `commonMain`. The iOS app uses them through Kotlin/Native. The UI is SwiftUI or Compose Multiplatform, to decide later. It uses the tokens of this file. Push goes through APNs, which FCM can send to.
- **Web dashboard.** CSS custom properties with the token names (`--brand-primary`, `--surface-raised`, …), generated from `docs/design-tokens.json`. The dashboard follows the TUI layout: tabs, a list on the left, the detail on the right.

## 11. Checklist for a new screen

- [ ] Each color comes from a token. No hex value in a component.
- [ ] Text on a dark surface uses `brand.primarySoft`, not `brand.primary`.
- [ ] Each state has a text or a shape, not only a color.
- [ ] Each tap target is 48 dp or more, and each icon-only button has a label.
- [ ] No link from card text. Only stored source URLs open.
- [ ] The screen works at the font scale of 200% and with "Remove animations" on.
- [ ] All text uses the words of section 1.6.
