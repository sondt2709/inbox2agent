package com.sondt.inbox2agent

import android.content.BroadcastReceiver
import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.widget.Toast
import com.sondt.inbox2agent.shared.CardRepository

/** The action "Copy reply" of a card notification. It copies the suggested reply. */
class NotificationActionReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != ACTION_COPY) return
        Graph.init(context.applicationContext as android.app.Application)
        val id = intent.getLongExtra(Notifier.EXTRA_CARD_ID, 0)
        val card = CardRepository.find(Graph.cards.state.value, id)?.card ?: return
        if (card.reply.isEmpty()) return
        copyText(context, card.reply)
        Toast.makeText(context, "Copied the suggested reply.", Toast.LENGTH_SHORT).show()
    }

    companion object {
        const val ACTION_COPY = "com.sondt.inbox2agent.COPY_REPLY"

        fun copyText(context: Context, text: String) {
            val cm = context.getSystemService(ClipboardManager::class.java)
            cm.setPrimaryClip(ClipData.newPlainText("Suggested reply", text))
        }
    }
}
