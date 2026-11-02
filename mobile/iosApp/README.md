# i2a for iOS (placeholder)

The iOS app does not exist yet. This file lists what it reuses and the steps to add it.

## What the iOS app reuses

The module `mobile/shared` keeps the parts that do not depend on a platform, in `commonMain`:

- `DesignTokens`: the colors of `docs/design-tokens.json`.
- `Payload`, `PushCard`, `Ref` and `PayloadParser`: the plaintext of a sealed message.
- `PairingLink`: the parser of the link `i2a://pair?...`.
- `CardRepository`: the rules of the cards on the phone (the highest `ts` wins, 500 cards, 30 days, hide).
- `SealedOpener`: the seam for the crypto. Android gives `AesGcmOpener` in `androidMain`.

## Steps

1. In `mobile/shared/build.gradle.kts`, remove the comment marks from `iosArm64()` and `iosSimulatorArm64()`. Add a framework binary.
2. Write `actual fun sealedOpener` in `iosMain`. Use CryptoKit `AES.GCM` (nonce 12 bytes, tag 16 bytes, additional data `i2a-push-v1`) and `Compression` with `COMPRESSION_ZLIB` (raw deflate). Keep the key in the Keychain with `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`.
3. Make an Xcode project in this folder. Use SwiftUI or Compose Multiplatform. Use the tokens and the rules of `docs/design.md`.
4. Push: FCM sends to APNs. Upload an APNs key to the Firebase project. Add a Notification Service Extension: it opens the sealed message and sets the title and the text before iOS shows the notification.
5. Run the vector test of `mobile/shared` for iOS too: it must open each message of `internal/phone/testdata/vectors.json`.
