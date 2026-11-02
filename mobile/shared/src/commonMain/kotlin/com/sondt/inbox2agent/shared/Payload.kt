package com.sondt.inbox2agent.shared

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/** The version of the payload that this app reads. */
const val PAYLOAD_VERSION = 1

const val KIND_CARD = "card"
const val KIND_TEST = "test"

/** The plaintext of one sealed message. internal/phone/payload.go writes it. */
@Serializable
data class Payload(
    val v: Int = 0,
    val kind: String = "",
    val ts: Long = 0,
    val alert: Boolean = false,
    val card: PushCard? = null,
)

/** A card as the Mac sends it. */
@Serializable
data class PushCard(
    val id: Long = 0,
    val key: String = "",
    val cat: String = "",
    val urg: String = "",
    val title: String = "",
    val group: String = "",
    val body: String = "",
    val reply: String = "",
    val created: Long = 0,
    val state: String = "",
    val cut: Boolean = false,
    val refs: List<Ref> = emptyList(),
) {
    /** The URL of the first source message with an http or https URL, or null. */
    fun threadUrl(): String? = refs.firstOrNull { isWebUrl(it.url) }?.url
}

/** A source message of a card. */
@Serializable
data class Ref(
    val author: String = "",
    val text: String = "",
    val url: String = "",
    val channel: String = "",
    val at: Long = 0,
)

/** True for an http or https URL. The app opens no other URL. */
fun isWebUrl(url: String): Boolean = url.startsWith("https://") || url.startsWith("http://")

object PayloadParser {
    val json = Json {
        ignoreUnknownKeys = true
        coerceInputValues = true
    }

    /**
     * Parses an opened message. It returns null for a payload that this app
     * does not know: another version, another kind, or a card kind with no card.
     */
    fun parse(text: String): Payload? {
        val p = try {
            json.decodeFromString(Payload.serializer(), text)
        } catch (e: Exception) {
            return null
        }
        return when {
            p.v != PAYLOAD_VERSION -> null
            p.kind == KIND_TEST -> p
            p.kind == KIND_CARD && p.card != null && p.card.id != 0L -> p
            else -> null
        }
    }
}
