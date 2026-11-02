package com.sondt.inbox2agent.shared

import kotlin.test.Test
import kotlin.test.assertEquals

class PlainTextTest {
    @Test
    fun removesEmphasis() {
        assertEquals(
            "Lena asks if the portal release can go on Friday 18:00.",
            firstPlainLine("Lena asks if the **portal release** can go on Friday 18:00.\n\n- QA passed"),
        )
        assertEquals("a b code c", firstPlainLine("*a* __b__ `code` ~~c~~"))
    }

    @Test
    fun removesLineMarks() {
        assertEquals("Title", firstPlainLine("\n\n## Title\ntext"))
        assertEquals("First item", firstPlainLine("- First item"))
        assertEquals("Quote", firstPlainLine("> Quote"))
    }

    @Test
    fun keepsPlainText() {
        assertEquals("2 * 3 = 6", firstPlainLine("2 * 3 = 6"))
        assertEquals("", firstPlainLine(""))
    }
}
