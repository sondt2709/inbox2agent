## Why

The user sees i2a cards only at the Mac: in the Feed tab and in macOS notifications. Away from the Mac, an urgent card waits unseen. The user wants the cards on an Android phone now, with no server to run and no open port on the Mac. Actions from the phone (reply, ask, done) need a path from the phone to the Mac. That path is a later change.

## What Changes

- New design system for all surfaces: `docs/design.md` and the tokens in `docs/design-tokens.json`. A Go test checks that the TUI colors and the Android colors equal the tokens. The primary color is `#7D56F4` on all surfaces.
- New command group `i2a phone`:
  - `i2a phone setup --service-account <file> --app-config <google-services.json>` stores the Firebase service-account key and the Android client configuration of the same Firebase project in the secret store. Each user can use an own Firebase project.
  - `i2a phone pair` makes a pairing (a random FCM topic and a random 256-bit key) when none exists, and shows it as a QR code and as a link. The link also holds the Firebase client configuration, so the APK holds no Firebase project.
  - `i2a phone test` sends a test push.
  - `i2a phone status` shows the setup and the pairing.
  - `i2a phone reset` makes a new topic and key. Phones that used the old pairing stop to receive pushes.
- `i2a card add` sends each added or updated card to the paired phones as one FCM data message. The card is compressed and sealed with AES-256-GCM. Google sees only ciphertext. A card that gives a desktop notification also gives a phone notification. The others go to the phone with no notification.
- New folder `mobile/`: a Kotlin Multiplatform project (`shared`, `androidApp`) in the layout of the other KMP project of the user. The Android app scans the QR code, subscribes to the topic, opens the sealed messages, keeps the cards on the phone, shows notifications, and shows a read-only Feed with Copy reply, Open thread and Hide. `mobile/iosApp/README.md` is a placeholder for iOS. The app starts Firebase from the configuration in the pairing link; the build needs no `google-services.json`.
- New CI job `mobile`: Kotlin unit tests and a debug APK build, with the APK as a workflow artifact. The release job waits for it.
- No relay, no new server, no change to the daemon, the store schema or `config.toml`.

## Capabilities

### New Capabilities
- `phone-push`: pairing, the sealed card message, the push from `i2a card add`, and the `i2a phone` commands on the Mac.
- `android-app`: the Android app: pairing by QR code, receiving and opening pushes, notifications, the read-only Feed and card screens, and the design tokens.

### Modified Capabilities
- `card-feed`: the requirement "Card notifications" adds the phone push after the desktop notification.

## Impact

- Go: new package `internal/phone` (pairing, seal, FCM sender with the OAuth 2.0 JWT flow from the standard library), new file `internal/cli/phone.go`, a change in `internal/cli/card.go`. The QR code uses `rsc.io/qr`, which is in `go.mod` already. No new Go module. The binary stays static (`CGO_ENABLED=0`).
- External services: Firebase Cloud Messaging HTTP v1 (`fcm.googleapis.com`) and the Google OAuth token endpoint (`oauth2.googleapis.com`). They are called only when a pairing and a service account exist.
- Secrets: the service-account key, the topic and the pairing key go in the secret store (`internal/secrets`, Keychain by default).
- Mobile: Kotlin 2.3.21, AGP 8.13.2, Compose, Firebase Messaging (no Google Services Gradle plugin), Google code scanner (no camera permission). The APK is built on an Android build host for development and in CI.
- CI: `.github/workflows/ci.yml` gets the job `mobile` (JDK 17, Gradle, the Android SDK of the runner); `release` needs it.
- `I2A_NOTIFY=off` turns the phone push off too, so the e2e harness sends nothing.
