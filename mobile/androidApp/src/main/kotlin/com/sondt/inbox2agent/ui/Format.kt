package com.sondt.inbox2agent.ui

import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.format.DateTimeFormatter

private val timeFmt = DateTimeFormatter.ofPattern("HH:mm")
private val dayFmt = DateTimeFormatter.ofPattern("d MMM")
private val fullFmt = DateTimeFormatter.ofPattern("d MMM yyyy, HH:mm")

/** The time of today, or the date of an earlier day, as the Feed tab shows it. */
fun whenText(sec: Long, zone: ZoneId = ZoneId.systemDefault()): String {
    if (sec <= 0) return ""
    val t = Instant.ofEpochSecond(sec).atZone(zone)
    return if (t.toLocalDate() == LocalDate.now(zone)) timeFmt.format(t) else dayFmt.format(t)
}

fun fullTime(sec: Long): String =
    if (sec <= 0) "" else fullFmt.format(Instant.ofEpochSecond(sec).atZone(ZoneId.systemDefault()))

private val linkRe = Regex("""\[([^\]]*)]\([^)]*\)""")

/**
 * Shows card Markdown as styled text: headings, bold, inline code and lists.
 * Card text is untrusted, so a Markdown link shows only its text, and no text
 * becomes a link.
 */
fun markdownText(md: String): AnnotatedString = buildAnnotatedString {
    val lines = md.replace("\r\n", "\n").split('\n')
    lines.forEachIndexed { i, raw ->
        var line = raw.replace(linkRe) { it.groupValues[1] }
        val heading = Regex("^#{1,6}\\s+").find(line)
        when {
            heading != null -> {
                withStyle(SpanStyle(fontWeight = FontWeight.Bold)) { inline(line.substring(heading.range.last + 1)) }
            }
            Regex("^\\s*[-*+]\\s+").containsMatchIn(line) -> {
                line = line.replaceFirst(Regex("^(\\s*)[-*+]\\s+"), "$1• ")
                inline(line)
            }
            else -> inline(line)
        }
        if (i < lines.size - 1) append('\n')
    }
}

private fun AnnotatedString.Builder.inline(s: String) {
    var i = 0
    while (i < s.length) {
        when {
            s.startsWith("**", i) && s.indexOf("**", i + 2) > i + 2 -> {
                val end = s.indexOf("**", i + 2)
                withStyle(SpanStyle(fontWeight = FontWeight.Bold)) { append(s.substring(i + 2, end)) }
                i = end + 2
            }
            s[i] == '`' && s.indexOf('`', i + 1) > i + 1 -> {
                val end = s.indexOf('`', i + 1)
                withStyle(SpanStyle(fontFamily = FontFamily.Monospace)) { append(s.substring(i + 1, end)) }
                i = end + 1
            }
            else -> {
                append(s[i]); i++
            }
        }
    }
}
