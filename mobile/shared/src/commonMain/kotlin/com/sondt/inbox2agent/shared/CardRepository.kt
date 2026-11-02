package com.sondt.inbox2agent.shared

import kotlinx.serialization.Serializable

/** The phone keeps at most this number of cards. */
const val MAX_CARDS = 500

/** The phone drops a card when its last message is older than this (30 days). */
const val MAX_AGE_SECONDS = 30L * 24 * 60 * 60

/** One card on the phone, with the time of its last message and the phone-only read state. */
@Serializable
data class StoredCard(
    val card: PushCard,
    val ts: Long,
    val read: Boolean = false,
)

/** All cards on the phone. The app writes it as one JSON file. */
@Serializable
data class CardsState(
    val cards: List<StoredCard> = emptyList(),
    val hidden: Set<Long> = emptySet(),
    val lastPush: Long = 0,
)

/** The result of [CardRepository.upsert]. */
data class Upsert(val state: CardsState, val applied: Boolean)

/**
 * The rules of the cards on the phone. The functions are pure: they return a
 * new state and do not write a file.
 */
object CardRepository {
    /**
     * Adds or replaces the card of a payload. The phone keeps the version with
     * the highest ts for each card ID. An alert makes the card unread, and it
     * shows a hidden card again.
     */
    fun upsert(state: CardsState, p: Payload, now: Long): Upsert {
        val card = p.card ?: return Upsert(state, false)
        val old = state.cards.firstOrNull { it.card.id == card.id }
        if (old != null && old.ts >= p.ts) return Upsert(state, false)
        val read = (old?.read ?: false) && !p.alert
        val next = StoredCard(card, p.ts, read)
        val cards = listOf(next) + state.cards.filter { it.card.id != card.id }
        val hidden = if (p.alert) state.hidden - card.id else state.hidden
        return Upsert(prune(CardsState(cards, hidden, maxOf(state.lastPush, p.ts)), now), true)
    }

    /** Drops cards older than [MAX_AGE_SECONDS] and keeps the newest [MAX_CARDS]. */
    fun prune(state: CardsState, now: Long): CardsState {
        val kept = state.cards
            .filter { now - it.ts <= MAX_AGE_SECONDS }
            .sortedByDescending { it.ts }
            .take(MAX_CARDS)
        val ids = kept.map { it.card.id }.toSet()
        return state.copy(cards = kept, hidden = state.hidden.filter { it in ids }.toSet())
    }

    /** Hides a card on the phone only. The Mac does not change. */
    fun hide(state: CardsState, id: Long): CardsState = state.copy(hidden = state.hidden + id)

    fun markRead(state: CardsState, id: Long, read: Boolean = true): CardsState =
        state.copy(cards = state.cards.map { if (it.card.id == id) it.copy(read = read) else it })

    /** The cards that the Feed shows: not hidden, newest first. */
    fun visible(state: CardsState): List<StoredCard> =
        state.cards
            .filter { it.card.id !in state.hidden }
            .sortedWith(compareByDescending<StoredCard> { it.card.created }.thenByDescending { it.ts })

    fun find(state: CardsState, id: Long): StoredCard? = state.cards.firstOrNull { it.card.id == id }
}

/** The category labels, as the TUI writes them. */
object Categories {
    const val ACTION = "action"
    const val QUESTION = "question"
    const val INFO = "info"
    val all = listOf(ACTION, QUESTION, INFO)
}

/** The urgency bars of the TUI (`urgBar`) and their text alternative. */
object Urgency {
    fun bars(urg: String): String = when (urg) {
        "high" -> "▰▰▰"
        "normal" -> "▰▰▱"
        "low" -> "▰▱▱"
        else -> "▱▱▱"
    }

    fun description(urg: String): String = "urgency " + urg.ifEmpty { "unknown" }
}
