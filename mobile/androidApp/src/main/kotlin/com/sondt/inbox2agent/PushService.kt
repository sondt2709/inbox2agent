package com.sondt.inbox2agent

import android.util.Log
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import com.sondt.inbox2agent.shared.KIND_CARD
import com.sondt.inbox2agent.shared.KIND_TEST
import com.sondt.inbox2agent.shared.openPayload

/**
 * Gets the data messages of the topic. Each message has the fields "v" and "c".
 * A message that the key does not open, or that this app does not know, goes
 * away with no notification.
 */
class PushService : FirebaseMessagingService() {
    override fun onMessageReceived(message: RemoteMessage) {
        Graph.init(application)
        val data = message.data
        if (data["v"] != "1") return
        val sealed = data["c"] ?: return
        val opener = Graph.pairing.opener() ?: return
        val p = opener.openPayload(sealed)
        if (p == null) {
            Log.i("i2a", "dropped a message that this phone cannot open")
            return
        }
        when (p.kind) {
            KIND_TEST -> Notifier.showTest(this, p.ts)
            KIND_CARD -> {
                val changed = Graph.cards.upsert(p)
                if (changed && p.alert) Notifier.showCard(this, p.card!!)
            }
        }
    }

    override fun onNewToken(token: String) {
        Graph.init(application)
        Graph.pairing.tokenChanged()
    }
}
