package com.sondt.inbox2agent

import android.app.Application

/** Starts the stores, the notification channels and Firebase. */
class I2aApp : Application() {
    override fun onCreate() {
        super.onCreate()
        Graph.init(this)
        Notifier.createChannels(this)
        // FCM can start the process for a push. Firebase must start before
        // PushService runs, with the options of the pairing.
        Graph.pairing.startFirebase()
    }
}

/** The single instances of the app. */
object Graph {
    lateinit var pairing: PairingStore
        private set
    lateinit var cards: CardStore
        private set

    fun init(app: Application) {
        if (::pairing.isInitialized) return
        pairing = PairingStore(app)
        cards = CardStore(app)
    }
}
