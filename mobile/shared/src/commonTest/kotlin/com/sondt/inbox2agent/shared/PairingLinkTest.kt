package com.sondt.inbox2agent.shared

import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class PairingLinkTest {
    // 32 bytes 0..31 in base64url with no padding.
    private val key = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8" // gitleaks:allow (public test key)
    private fun link(drop: String? = null, set: Pair<String, String>? = null): String {
        val fields = linkedMapOf(
            "v" to "1", "p" to "demo-123", "s" to "123456", "a" to "1%3A123456%3Aandroid%3Aabc",
            "ak" to "AIzaDemo", "t" to "i2a-0123456789abcdef0123456789abcdef", "k" to key, "n" to "Lena%27s+Mac",
        )
        drop?.let { fields.remove(it) }
        set?.let { fields[it.first] = it.second }
        return "i2a://pair?" + fields.entries.joinToString("&") { "${it.key}=${it.value}" }
    }

    @Test
    fun validLink() {
        val p = PairingLink.parse(link())
        assertEquals("demo-123", p.projectId)
        assertEquals("123456", p.senderId)
        assertEquals("1:123456:android:abc", p.appId)
        assertEquals("AIzaDemo", p.apiKey)
        assertEquals("i2a-0123456789abcdef0123456789abcdef", p.topic)
        assertEquals(KEY_SIZE, p.key.size)
        assertEquals(31, p.key[31].toInt())
        assertEquals("Lena's Mac", p.macName)
    }

    @Test
    fun missingFieldNamesTheField() {
        for (f in listOf("v", "p", "s", "a", "ak", "t", "k")) {
            val e = assertFailsWith<PairingLinkException> { PairingLink.parse(link(drop = f)) }
            assertContains(e.message!!, "\"$f\"")
        }
    }

    @Test
    fun missingNameUsesDefault() {
        assertEquals("your Mac", PairingLink.parse(link(drop = "n")).macName)
    }

    @Test
    fun badValues() {
        assertFailsWith<PairingLinkException> { PairingLink.parse(link(set = "v" to "2")) }
        assertFailsWith<PairingLinkException> { PairingLink.parse(link(set = "t" to "news")) }
        assertFailsWith<PairingLinkException> { PairingLink.parse(link(set = "k" to "AAEC")) }
        assertFailsWith<PairingLinkException> { PairingLink.parse(link(set = "k" to "!!!")) }
        assertFailsWith<PairingLinkException> { PairingLink.parse("https://example.com/pair?v=1") }
        assertFailsWith<PairingLinkException> { PairingLink.parse(link(set = "n" to "%zz")) }
    }
}
