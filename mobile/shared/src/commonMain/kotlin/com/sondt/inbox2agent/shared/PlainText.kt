package com.sondt.inbox2agent.shared

private val emphasis = Regex("""(\*\*|__|\*|_|`|~~)(\S(?:.*?\S)?)\1""")
private val lineMark = Regex("""^\s*(#{1,6}\s+|[-*+]\s+|>\s*|\d+[.)]\s+)""")

/**
 * Returns the first line with text of a Markdown body, with no Markdown marks.
 * A notification shows plain text, so "**portal release**" becomes "portal release".
 */
fun firstPlainLine(markdown: String): String {
    val line = markdown.lineSequence().map { it.trim() }.firstOrNull { it.isNotEmpty() && !it.startsWith("```") }.orEmpty()
    var s = lineMark.replace(line, "")
    while (true) {
        val next = emphasis.replace(s) { it.groupValues[2] }
        if (next == s) return s
        s = next
    }
}
