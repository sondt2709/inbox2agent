package com.sondt.inbox2agent.shared

import java.io.File
import java.util.Base64
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * Opens the messages that Go sealed in internal/phone/testdata/vectors.json.
 * This proves that the Mac and the phone agree on the format.
 */
class VectorsTest {
    private val file = File("../../internal/phone/testdata/vectors.json")
    private val root: JsonObject by lazy { Json.parseToJsonElement(file.readText()).jsonObject }
    private val key: ByteArray by lazy { Base64.getDecoder().decode(root.getValue("key").jsonPrimitive.content) }

    @Test
    fun opensEachGoVector() {
        assertTrue(file.exists(), "missing ${file.absolutePath}")
        assertEquals(SEAL_AAD, root.getValue("aad").jsonPrimitive.content)
        val vectors = root.getValue("vectors").jsonArray
        assertTrue(vectors.isNotEmpty())
        val opener = sealedOpener(key)
        for (v in vectors) {
            val o = v.jsonObject
            val name = o.getValue("name").jsonPrimitive.content
            val plain = opener.open(o.getValue("c").jsonPrimitive.content)
            assertEquals(o.getValue("payload"), Json.parseToJsonElement(plain), name)
            val parsed = assertNotNull(opener.openPayload(o.getValue("c").jsonPrimitive.content), name)
            val want = PayloadParser.json.decodeFromString(Payload.serializer(), o.getValue("payload").toString())
            assertEquals(want, parsed, name)
        }
    }

    @Test
    fun wrongKeyFails() {
        val other = key.copyOf().also { it[0] = (it[0].toInt() xor 1).toByte() }
        val c = root.getValue("vectors").jsonArray[0].jsonObject.getValue("c").jsonPrimitive.content
        assertFailsWith<Exception> { sealedOpener(other).open(c) }
        assertEquals(null, sealedOpener(other).openPayload(c))
    }

    @Test
    fun vietnameseTextSurvives() {
        val opener = sealedOpener(key)
        val titles = root.getValue("vectors").jsonArray.mapNotNull {
            opener.openPayload(it.jsonObject.getValue("c").jsonPrimitive.content)?.card?.title
        }
        assertTrue(titles.any { it.contains("Bữa tối") }, titles.toString())
    }
}
