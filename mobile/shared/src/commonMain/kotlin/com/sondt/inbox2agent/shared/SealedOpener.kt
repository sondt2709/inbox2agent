package com.sondt.inbox2agent.shared

/** The additional data of each seal. internal/phone/seal.go uses the same value. */
const val SEAL_AAD = "i2a-push-v1"

/**
 * Opens the field "c" of a message: base64 of nonce (12 bytes), AES-256-GCM
 * ciphertext and tag, of raw deflate data. Each platform gives the crypto.
 */
interface SealedOpener {
    /** Returns the plaintext. It throws when the key does not open the message. */
    fun open(sealed: String): String
}

/** An opener for a raw key. The tests use it; the Android app uses the Keystore. */
expect fun sealedOpener(rawKey: ByteArray): SealedOpener

/** Opens a sealed message and parses it. It returns null when either step fails. */
fun SealedOpener.openPayload(sealed: String): Payload? {
    val text = try {
        open(sealed)
    } catch (e: Exception) {
        return null
    }
    return PayloadParser.parse(text)
}
