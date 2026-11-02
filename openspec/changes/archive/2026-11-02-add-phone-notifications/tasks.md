## 1. Design system

- [x] 1.1 Write `docs/design.md` (done before the proposal) and `docs/design-tokens.json`
- [x] 1.2 Add `internal/tui/tokens_test.go`: check the TUI colors and `DesignTokens.kt` against the JSON file; break one value to see it fail

## 2. Seal and payload (Go)

- [x] 2.1 `internal/phone`: payload types, `Seal`, `Open`, the size budget and the cut order
- [x] 2.2 Tests: round trip, wrong key, large card, unknown version
- [x] 2.3 Test vectors `internal/phone/testdata/vectors.json` with `-update`

## 3. Pairing and secrets (Go)

- [x] 3.1 Service-account parse and check; `google-services.json` parse (`--app-config`); secret keys `phone/*`
- [x] 3.2 Pairing: new, load, reset, remove; the link format
- [x] 3.3 Move the QR text renderer from `internal/source/whatsapp` to a shared place, or call it

## 4. FCM sender (Go)

- [x] 4.1 JWT RS256 signer and token exchange with the standard library
- [x] 4.2 `Send` to `messages:send` with the topic, `data`, the Android priority and TTL
- [x] 4.3 Tests with `httptest`: the JWT claims, the request body, an HTTP 503

## 5. CLI (Go)

- [x] 5.1 `i2a phone setup`, `pair`, `reset`, `test`, `status`
- [x] 5.2 `i2a card add`: push after the desktop notifications, alert cap 5, 15-second limit, warnings, `Phone: <n>.`
- [x] 5.3 CLI tests with fake endpoints; a test fails when the push code is broken
- [x] 5.4 Update `CLAUDE.md` (architecture, commands) and `README.md` (phone setup)

## 6. Android app (KMP, on the Android build host)

- [x] 6.1 Project: `mobile/settings.gradle.kts`, `gradle/libs.versions.toml`, wrapper, `shared`, `androidApp`, `mobile/iosApp/README.md`; no Google Services plugin
- [x] 6.2 `shared/commonMain`: `DesignTokens.kt`, `Card`, `Payload`, link parser, `CardRepository` rules (upsert by `ts`, 500 cards, 30 days, hide)
- [x] 6.3 `shared/androidMain`: `PushCrypto` (AES-GCM, Inflater); Keystore key import
- [x] 6.4 Kotlin tests: open the Go vectors; link parser; repository rules
- [x] 6.5 `androidApp`: theme, Feed screen, card screen, pair screen, settings
- [x] 6.6 Firebase start from the stored options; `FirebaseMessagingService`, channels, notifications with actions, permission request
- [x] 6.7 Build the debug APK on the build host and copy it to `~/Downloads/i2a.apk`

## 7. Mobile CI

- [x] 7.1 Job `mobile` in `.github/workflows/ci.yml`; `release` needs it
- [x] 7.2 The job passes on the PR

## 8. Verify

- [x] 8.1 `task test`, `task lint`, `task leakcheck`, `./gradlew :shared:testDebugUnitTest :androidApp:assembleDebug` on the build host
- [x] 8.2 End to end on the emulator of the build host: setup, pair by link, `i2a card add`, screenshot of the notification and the Feed
- [x] 8.3 Screenshots of the TUI and the app side by side for the brand colors
