## Context

`i2a card add` saves cards and shows macOS notifications (`card.Plan`, `notify.Desktop`). Nothing leaves the Mac. The user made a Firebase project for an Android app with the package `com.sondt.inbox2agent`. The user wants notifications on the phone now. Actions from the phone come later, through a path that is not chosen yet (relay or other). The user has a second KMP project (`shared` + `androidApp`, Compose, Ktor) and builds Android apps on a Linux build host (Android SDK platform 36, JDK 17, an AVD with Google APIs).

Constraints: one static Go binary (`CGO_ENABLED=0`); no open port on the Mac; no server to run; card text is untrusted; no personal data in committed files.

## Goals / Non-Goals

**Goals:**
- A card that the agent adds shows on the phone in seconds, with the same notification rules as the Mac.
- Google and any other party see only ciphertext.
- Pairing needs no network path from the phone to the Mac.
- One design system for the TUI, the Android app, a future iOS app and a future dashboard.
- The Android app is ready for iOS: the payload, the crypto seam and the card model are in `commonMain`.

**Non-Goals:**
- Reply, Ask, Done or Unread from the phone. "Hide" on the phone changes only the phone.
- A relay, a phone-to-Mac channel, or device tokens on the Mac.
- An iOS build, a light theme, an APK in CI or in the release.
- Push for daemon events (a failed dispatch).

## Decisions

### D1. One-way pairing: an FCM topic and a shared key
`i2a phone pair` makes a random topic name (`i2a-` + 32 hex characters, 128 bits) and a random 256-bit key. The QR code holds both. The phone subscribes to the topic and keeps the key. The Mac sends to the topic.

- Alternative: the phone sends its FCM token and public key to the Mac. This needs a path from the phone to the Mac, which does not exist yet.
- Alternative: one key for each phone with HPKE. Same problem. HPKE comes with the action path later.
- Effect: all phones that scanned one QR code share the key. `i2a phone reset` revokes all of them. This is acceptable for one user.

### D2. The sealed message: deflate, then AES-256-GCM
The plaintext is a JSON object (D5). The Mac compresses it with raw deflate, then seals it with AES-256-GCM, a random 12-byte nonce, and the additional data `i2a-push-v1`. The FCM data message has two fields: `v` = `"1"` and `c` = base64(nonce ‖ ciphertext ‖ tag).

- Both sides use the standard library: Go `crypto/aes`, `crypto/cipher`, `compress/flate`; Android `javax.crypto` (`AES/GCM/NoPadding`) and `java.util.zip.Inflater(true)`. No crypto library to add.
- Random nonces are safe here: the key seals far fewer than 2^32 messages before a reset.
- Deflate before the seal: the text is the user's own cards, and no attacker can add chosen text next to a secret in one message, so the compression leak (CRIME) does not apply.

### D3. The Mac sends from `i2a card add`
After the save and the desktop notifications, `i2a card add` sends one message for each card with the result `added` or `updated`. Each send has a timeout; the whole push step stops after 15 seconds. A failure prints `warning: phone push: ...` on stderr. The cards stay saved and the command exits 0.

- Alternative: the daemon watches the store and sends. This adds a dependency on the daemon and a delay of the poll interval. `i2a card add` already owns the notification step.

### D4. FCM HTTP v1 with the standard library
The service-account JSON gives `project_id`, `client_email`, `private_key` and `token_uri`. The sender signs a JWT (RS256, scope `https://www.googleapis.com/auth/firebase.messaging`, 1 hour) with `crypto/rsa`, exchanges it at `token_uri` for an access token, then posts to `https://fcm.googleapis.com/v1/projects/<project_id>/messages:send`. One token serves all messages of one call.

- Alternative: `golang.org/x/oauth2/google` and the Firebase Admin SDK. They add many modules for two HTTP calls.
- The endpoints are fields of the sender, so tests use `httptest` servers.

### D5. The payload
```json
{"v":1,"kind":"card","ts":1793700000,
 "card":{"id":812,"key":"mm:lena","cat":"action","urg":"high","title":"…","group":"…",
         "body":"…","reply":"…","created":1793699000,"state":"new","cut":false,
         "refs":[{"author":"lena","text":"…","url":"https://…","channel":"town-square","at":1793698000}]},
 "alert":true}
```
- `kind` is `card` or `test`. The phone ignores an unknown `kind` and an unknown `v`.
- FCM limits a data message to 4096 bytes. The Mac keeps `c` at 3,600 characters or less. When the sealed message is too large, the Mac cuts in this order: the text of each ref to 140 characters, then the refs after the first 3, then the body (halve it until it fits), and sets `cut` to true. The phone then shows "The full text is on the Mac."
- The phone keeps the message with the highest `ts` for each card `id`.

### D6. Alert and priority
`alert` is true for a card that `card.Plan` counts (added, higher urgency, or back from done, at or above `[notify] min_urgency`, with `[notify] enabled`). At most 5 cards in one call have `alert` true; the others go silent, so a tricked agent cannot flood the phone. An alert message uses the FCM Android priority `high`; a silent one uses `normal`, because FCM lowers the priority of apps that send high-priority messages with no notification. TTL: 7 days.

### D7. Secrets
Keys in the secret store: `phone/service-account` (the JSON), `phone/topic`, `phone/key` (base64), `phone/paired-at`. No change to `config.toml`. `i2a phone status` shows the backend, the project ID, the pairing date and the first 8 characters of the topic. It never shows the key.

### D8. The Android app
- Layout: `mobile/settings.gradle.kts`, `mobile/shared` (KMP: `commonMain` model, payload parser, `CardRepository` logic, `expect` `PushCrypto`; `androidMain` `actual`), `mobile/androidApp` (Compose UI, `FirebaseMessagingService`, notifications). Versions follow the other KMP project: Kotlin 2.3.21, AGP 8.13.2, compileSdk 36, minSdk 26.
- The key goes into the Android Keystore as an AES key (`KeyProtection`, purpose decrypt, GCM). The app never reads the raw key again.
- The cards go in a JSON file in the private app folder: at most 500 cards, 30 days. Hidden cards stay hidden by `id`.
- QR scan: Google code scanner (`play-services-code-scanner`). It needs no camera permission. A text field accepts the pairing link as a second way.
- The link: `i2a://pair?v=1&p=<project_id>&s=<sender_id>&a=<app_id>&ak=<api_key>&t=<topic>&k=<base64url key>&n=<Mac name>`. The app refuses a link that misses a field. A new pairing with another project replaces the old one and restarts the app process.
- Colors: `DesignTokens.kt` in `shared` holds the tokens of `docs/design-tokens.json`. The theme is dark only and does not use dynamic color.
- No `google-services.json` in the build and no Google Services Gradle plugin. The pairing link holds the Firebase client options (project ID, sender ID, app ID, API key). The app stores them and calls `FirebaseApp.initializeApp(context, options)` in `Application.onCreate`, before FCM starts a service for a push. One APK works with each Firebase project that registered the package `com.sondt.inbox2agent`.
- Alternative: a `google-services.json` at build time. Each user would need to build an own APK, and CI would need a secret file.
- A card that comes again with `alert` true shows again, unread, also when the user hid it. A silent update keeps the hidden and read states.
- After a pairing that restarts the process (another Firebase project), the app does not ask for the notification permission again; Settings shows the state.
- iOS: `mobile/iosApp/README.md` lists the steps. The `shared` build file has the iOS targets as a comment.

### D9. Design tokens
`docs/design-tokens.json` is the source. `internal/tui/tokens_test.go` reads it and checks the values of `internal/tui/style.go` and `form.go` (through the Go variables) and of `mobile/shared/src/commonMain/kotlin/.../DesignTokens.kt` (through a regular expression on `0xFFRRGGBB` values). The TUI does not change its colors in this change.

### D9b. The client configuration on the Mac
`i2a phone setup --app-config <file>` reads a `google-services.json` file, takes the client with the package `com.sondt.inbox2agent`, and stores `project_id`, `project_number` (sender ID), `mobilesdk_app_id` and the first `api_key` as `phone/app-config` (JSON). Its `project_id` must equal the `project_id` of the service account. The API key of a Firebase Android client is not a secret, but it goes only into the QR code.

### D10. Test vectors between Go and Kotlin
A Go test writes `internal/phone/testdata/vectors.json` with `-update`: a fixed key, fixed nonces, the plaintexts and the sealed strings. The Go test opens them. A Kotlin JVM test in `mobile/shared` reads the same file and opens each one. This proves that both sides agree. The vectors use invented card text.

### D11. Mobile CI
A new job `mobile` in `.github/workflows/ci.yml` runs on `ubuntu-latest`: `actions/setup-java` (Temurin 17), `gradle/actions/setup-gradle`, then in `mobile/`: `./gradlew :shared:testDebugUnitTest :androidApp:testDebugUnitTest :androidApp:assembleDebug`. It uploads `i2a-debug.apk` as a workflow artifact. The runner has the Android SDK; AGP downloads a missing platform. `release` adds `mobile` to `needs`, because the release spec says that a release waits for all CI jobs. The job needs no secret, because the APK has no Firebase configuration.

## Risks / Trade-offs

- [The QR code or the link leaks] → Any person with it can read new pushes. Mitigation: `i2a phone pair` shows the QR code only on request; `i2a phone reset` makes a new topic and key.
- [The service-account key is powerful] → The docs tell the user to make a service account with only the role "Firebase Cloud Messaging API Admin". It is stored in the Keychain.
- [FCM topic delivery is slower than delivery to a token] → Usually seconds. Acceptable for notifications.
- [A large card loses text] → `cut` is shown; the full card is on the Mac.
- [The phone and the Mac disagree on Done] → The phone has Hide only. Sync of the state comes with the action path.
- [The default service account `firebase-adminsdk` has wide roles] → It works, but the README tells the user to make an account with only "Firebase Cloud Messaging API Admin".
- [The mobile job makes CI slower and can block a Go release] → It runs in parallel with the other jobs; Gradle caches cut later runs.
- [`i2a card add` becomes slower] → Two HTTPS calls, about 0.5 seconds, only when a pairing exists. A 15-second limit protects the agent.

## Migration Plan

No migration. Nothing changes until the user runs `i2a phone setup` and `i2a phone pair`. Rollback: revert the commit; remove the `phone/*` secrets with `i2a phone reset --remove`.

## Open Questions

- Later: the action path (relay or other), state sync, and iOS.
