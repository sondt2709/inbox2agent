package com.sondt.inbox2agent.shared

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class CardRepositoryTest {
    private val now = 1_800_000_000L
    private fun payload(id: Long, ts: Long, title: String = "t$id", alert: Boolean = false) =
        Payload(v = 1, kind = KIND_CARD, ts = ts, alert = alert, card = PushCard(id = id, title = title, created = ts))

    @Test
    fun highestTsWins() {
        var s = CardRepository.upsert(CardsState(), payload(1, now - 10, "new"), now).state
        val old = CardRepository.upsert(s, payload(1, now - 20, "old"), now)
        assertFalse(old.applied)
        assertEquals("new", CardRepository.find(old.state, 1)!!.card.title)
        s = CardRepository.upsert(s, payload(1, now - 5, "newer"), now).state
        assertEquals("newer", CardRepository.find(s, 1)!!.card.title)
        assertEquals(1, s.cards.size)
    }

    @Test
    fun keepsAtMost500() {
        var s = CardsState()
        for (i in 1..520L) s = CardRepository.upsert(s, payload(i, now - 1000 + i), now).state
        assertEquals(MAX_CARDS, s.cards.size)
        assertNull(CardRepository.find(s, 1))
        assertTrue(CardRepository.find(s, 520) != null)
    }

    @Test
    fun drops30DaysOld() {
        var s = CardRepository.upsert(CardsState(), payload(1, now - MAX_AGE_SECONDS - 1), now - MAX_AGE_SECONDS).state
        assertEquals(1, s.cards.size)
        s = CardRepository.prune(s, now)
        assertEquals(0, s.cards.size)
    }

    @Test
    fun hideIsPhoneOnlyAndAlertShowsAgain() {
        var s = CardRepository.upsert(CardsState(), payload(1, now - 10), now).state
        s = CardRepository.upsert(s, payload(2, now - 9), now).state
        s = CardRepository.hide(s, 1)
        assertEquals(listOf(2L), CardRepository.visible(s).map { it.card.id })
        s = CardRepository.upsert(s, payload(1, now - 8), now).state
        assertEquals(listOf(2L), CardRepository.visible(s).map { it.card.id }, "a silent update keeps the card hidden")
        s = CardRepository.upsert(s, payload(1, now - 7, alert = true), now).state
        assertEquals(setOf(1L, 2L), CardRepository.visible(s).map { it.card.id }.toSet())
    }

    @Test
    fun readAndAlert() {
        var s = CardRepository.upsert(CardsState(), payload(1, now - 10), now).state
        s = CardRepository.markRead(s, 1)
        assertTrue(CardRepository.find(s, 1)!!.read)
        s = CardRepository.upsert(s, payload(1, now - 9), now).state
        assertTrue(CardRepository.find(s, 1)!!.read, "a silent update keeps the read state")
        s = CardRepository.upsert(s, payload(1, now - 8, alert = true), now).state
        assertFalse(CardRepository.find(s, 1)!!.read, "an alert makes the card unread")
    }

    @Test
    fun parserDropsUnknown() {
        assertNull(PayloadParser.parse("""{"v":2,"kind":"card","card":{"id":1}}"""))
        assertNull(PayloadParser.parse("""{"v":1,"kind":"state"}"""))
        assertNull(PayloadParser.parse("""{"v":1,"kind":"card"}"""))
        assertNull(PayloadParser.parse("not json"))
        val p = PayloadParser.parse("""{"v":1,"kind":"card","ts":5,"extra":true,"card":{"id":3,"title":"x","new_field":1}}""")
        assertEquals(3L, p!!.card!!.id)
        assertEquals(KIND_TEST, PayloadParser.parse("""{"v":1,"kind":"test","ts":1}""")!!.kind)
    }

    @Test
    fun threadUrlOnlyWeb() {
        val c = PushCard(id = 1, refs = listOf(Ref(url = "javascript:alert(1)"), Ref(url = "mattermost://x"), Ref(url = "https://chat.example.com/p/1")))
        assertEquals("https://chat.example.com/p/1", c.threadUrl())
        assertNull(PushCard(id = 2, refs = listOf(Ref(url = "file:///etc/passwd"))).threadUrl())
    }
}
