package com.sondt.inbox2agent.shared

import java.io.ByteArrayOutputStream
import java.security.Key
import java.util.Base64
import java.util.zip.Inflater
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

private const val NONCE_SIZE = 12
private const val TAG_BITS = 128
private const val MAX_PLAIN = 1 shl 20

/** Opens sealed messages with a JCA key: a raw key or a key of the Android Keystore. */
class AesGcmOpener(private val key: Key) : SealedOpener {
    override fun open(sealed: String): String {
        val raw = Base64.getDecoder().decode(sealed)
        require(raw.size > NONCE_SIZE + TAG_BITS / 8) { "the sealed text is too short" }
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(TAG_BITS, raw, 0, NONCE_SIZE))
        cipher.updateAAD(SEAL_AAD.encodeToByteArray())
        val z = cipher.doFinal(raw, NONCE_SIZE, raw.size - NONCE_SIZE)
        return inflateRaw(z).decodeToString()
    }
}

actual fun sealedOpener(rawKey: ByteArray): SealedOpener {
    require(rawKey.size == KEY_SIZE) { "the key has ${rawKey.size} bytes" }
    return AesGcmOpener(SecretKeySpec(rawKey, "AES"))
}

/** Inflates raw deflate data (no zlib header), as Go compress/flate writes it. */
fun inflateRaw(z: ByteArray): ByteArray {
    val inflater = Inflater(true)
    try {
        // With nowrap, the Inflater needs one extra dummy byte after the input.
        inflater.setInput(z + byteArrayOf(0))
        val out = ByteArrayOutputStream()
        val buf = ByteArray(4096)
        while (!inflater.finished()) {
            val n = inflater.inflate(buf)
            if (n == 0) {
                if (inflater.needsInput() || inflater.needsDictionary()) {
                    throw IllegalStateException("the compressed data is not complete")
                }
            }
            out.write(buf, 0, n)
            if (out.size() > MAX_PLAIN) throw IllegalStateException("the message is too large")
        }
        return out.toByteArray()
    } finally {
        inflater.end()
    }
}
