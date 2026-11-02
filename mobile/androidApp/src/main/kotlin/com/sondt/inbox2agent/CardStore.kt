package com.sondt.inbox2agent

import android.content.Context
import android.util.Log
import com.sondt.inbox2agent.shared.CardRepository
import com.sondt.inbox2agent.shared.CardsState
import com.sondt.inbox2agent.shared.Payload
import com.sondt.inbox2agent.shared.PayloadParser
import java.io.File
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/**
 * Keeps the cards in one JSON file in the private app folder. The rules are in
 * [CardRepository]. The service and the UI run in one process and share this object.
 */
class CardStore(context: Context) {
    private val file = File(context.filesDir, "cards.json")
    private val lock = Any()
    private val _state = MutableStateFlow(read())
    val state: StateFlow<CardsState> = _state

    private fun read(): CardsState = try {
        if (file.exists()) PayloadParser.json.decodeFromString(CardsState.serializer(), file.readText()) else CardsState()
    } catch (e: Exception) {
        Log.w("i2a", "cannot read the cards; the phone starts with no cards", e)
        CardsState()
    }

    private fun write(s: CardsState) {
        val tmp = File(file.parentFile, "cards.json.tmp")
        tmp.writeText(PayloadParser.json.encodeToString(CardsState.serializer(), s))
        tmp.renameTo(file)
        _state.value = s
    }

    private fun now() = System.currentTimeMillis() / 1000

    /** Adds the card of a payload. It returns true when the card changed. */
    fun upsert(p: Payload): Boolean = synchronized(lock) {
        val r = CardRepository.upsert(_state.value, p, now())
        if (r.applied) write(r.state)
        r.applied
    }

    fun hide(id: Long) = synchronized(lock) { write(CardRepository.hide(_state.value, id)) }

    fun markRead(id: Long, read: Boolean = true) = synchronized(lock) {
        val s = _state.value
        if (CardRepository.find(s, id)?.read != read) write(CardRepository.markRead(s, id, read))
    }

    fun prune() = synchronized(lock) { write(CardRepository.prune(_state.value, now())) }

    fun clear() = synchronized(lock) { write(CardsState()) }
}
