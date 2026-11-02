package com.sondt.inbox2agent.shared

import kotlin.io.encoding.Base64
import kotlin.io.encoding.ExperimentalEncodingApi

/** The size of the pairing key in bytes (AES-256). */
const val KEY_SIZE = 32

/**
 * The content of a pairing link:
 * `i2a://pair?v=1&p=<project_id>&s=<sender_id>&a=<app_id>&ak=<api_key>&t=<topic>&k=<key>&n=<Mac name>`.
 * `i2a phone pair` on the Mac makes it.
 */
class Pairing(
    val projectId: String,
    val senderId: String,
    val appId: String,
    val apiKey: String,
    val topic: String,
    val key: ByteArray,
    val macName: String,
)

/** The link is not a valid pairing link. The message names the field. */
class PairingLinkException(message: String) : IllegalArgumentException(message)

object PairingLink {
    private const val PREFIX = "i2a://pair?"
    private val required = listOf("v", "p", "s", "a", "ak", "t", "k")

    @OptIn(ExperimentalEncodingApi::class)
    private val keyCodec = Base64.UrlSafe.withPadding(Base64.PaddingOption.ABSENT_OPTIONAL)

    /** Parses a pairing link. It throws [PairingLinkException] when the link is not valid. */
    @OptIn(ExperimentalEncodingApi::class)
    fun parse(link: String): Pairing {
        val text = link.trim()
        if (!text.startsWith(PREFIX)) {
            throw PairingLinkException("The link does not start with $PREFIX.")
        }
        val q = query(text.substring(PREFIX.length))
        for (f in required) {
            if (q[f].isNullOrEmpty()) throw PairingLinkException("The link has no field \"$f\".")
        }
        if (q["v"] != "1") throw PairingLinkException("The field \"v\" is ${q["v"]}. This app reads version 1.")
        val topic = q.getValue("t")
        if (!topic.startsWith("i2a-")) throw PairingLinkException("The field \"t\" does not start with i2a-.")
        val key = try {
            keyCodec.decode(q.getValue("k"))
        } catch (e: IllegalArgumentException) {
            throw PairingLinkException("The field \"k\" is not base64url.")
        }
        if (key.size != KEY_SIZE) {
            throw PairingLinkException("The field \"k\" has ${key.size} bytes. It must have $KEY_SIZE.")
        }
        return Pairing(
            projectId = q.getValue("p"),
            senderId = q.getValue("s"),
            appId = q.getValue("a"),
            apiKey = q.getValue("ak"),
            topic = topic,
            key = key,
            macName = q["n"]?.takeIf { it.isNotBlank() } ?: "your Mac",
        )
    }

    private fun query(s: String): Map<String, String> {
        val out = mutableMapOf<String, String>()
        for (part in s.split('&')) {
            if (part.isEmpty()) continue
            val i = part.indexOf('=')
            val k = decode(if (i < 0) part else part.substring(0, i))
            val v = if (i < 0) "" else decode(part.substring(i + 1))
            out.putIfAbsent(k, v)
        }
        return out
    }

    /** Decodes %XX sequences as UTF-8, and '+' as a space. */
    private fun decode(s: String): String {
        if ('%' !in s && '+' !in s) return s
        val bytes = ArrayList<Byte>(s.length)
        var i = 0
        while (i < s.length) {
            val c = s[i]
            when {
                c == '+' -> { bytes.add(' '.code.toByte()); i++ }
                c == '%' -> {
                    val hex = s.substring(i + 1, minOf(i + 3, s.length))
                    val b = hex.toIntOrNull(16)
                    if (hex.length == 2 && b != null && hex.all { it.isLetterOrDigit() }) {
                        bytes.add(b.toByte()); i += 3
                    } else {
                        throw PairingLinkException("The link has a bad escape \"%$hex\".")
                    }
                }
                else -> {
                    for (b in c.toString().encodeToByteArray()) bytes.add(b)
                    i++
                }
            }
        }
        return bytes.toByteArray().decodeToString()
    }
}
