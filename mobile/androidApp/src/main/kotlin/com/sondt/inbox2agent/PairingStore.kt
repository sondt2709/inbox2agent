package com.sondt.inbox2agent

import android.content.Context
import android.content.Intent
import android.security.keystore.KeyProperties
import android.security.keystore.KeyProtection
import android.util.Log
import com.google.firebase.FirebaseApp
import com.google.firebase.FirebaseOptions
import com.google.firebase.messaging.FirebaseMessaging
import com.sondt.inbox2agent.shared.AesGcmOpener
import com.sondt.inbox2agent.shared.Pairing
import com.sondt.inbox2agent.shared.SealedOpener
import java.security.KeyStore
import javax.crypto.SecretKey
import javax.crypto.spec.SecretKeySpec
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/** What the app knows about the pairing. The key is in the Keystore, not here. */
data class PairingInfo(
    val projectId: String,
    val topic: String,
    val macName: String,
    val pairedAt: Long,
    val subscribed: Boolean,
)

/**
 * Keeps the pairing: the Firebase options and the topic in private preferences,
 * and the AES key in the Android Keystore. The app never reads the raw key again.
 */
class PairingStore(private val context: Context) {
    private val prefs = context.getSharedPreferences("pairing", Context.MODE_PRIVATE)
    private val _info = MutableStateFlow(load())
    val info: StateFlow<PairingInfo?> = _info

    private fun load(): PairingInfo? {
        val topic = prefs.getString(TOPIC, null) ?: return null
        return PairingInfo(
            projectId = prefs.getString(PROJECT, "") ?: "",
            topic = topic,
            macName = prefs.getString(NAME, "") ?: "",
            pairedAt = prefs.getLong(PAIRED_AT, 0),
            subscribed = prefs.getBoolean(SUBSCRIBED, false),
        )
    }

    private fun options(): FirebaseOptions? {
        val project = prefs.getString(PROJECT, null) ?: return null
        return FirebaseOptions.Builder()
            .setProjectId(project)
            .setGcmSenderId(prefs.getString(SENDER, "") ?: "")
            .setApplicationId(prefs.getString(APP_ID, "") ?: "")
            .setApiKey(prefs.getString(API_KEY, "") ?: "")
            .build()
    }

    /** Starts Firebase with the stored options. It does nothing when the phone is not paired. */
    fun startFirebase() {
        val opts = options() ?: return
        if (FirebaseApp.getApps(context).isEmpty()) {
            FirebaseApp.initializeApp(context, opts)
        }
    }

    /** The opener of the paired key, or null when the phone is not paired. */
    fun opener(): SealedOpener? {
        val ks = KeyStore.getInstance(KEYSTORE).apply { load(null) }
        val key = ks.getKey(KEY_ALIAS, null) as? SecretKey ?: return null
        return AesGcmOpener(key)
    }

    /**
     * Stores a pairing and subscribes to its topic. When Firebase runs with
     * another project, the app restarts so that Firebase starts with the new options.
     * It returns true when the app must restart.
     */
    fun pair(p: Pairing, now: Long): Boolean {
        val running = FirebaseApp.getApps(context).firstOrNull()
        val restart = running != null && running.options.projectId != p.projectId
        info.value?.let { old -> if (old.topic != p.topic && !restart) unsubscribe(old.topic) }

        val ks = KeyStore.getInstance(KEYSTORE).apply { load(null) }
        ks.setEntry(
            KEY_ALIAS,
            KeyStore.SecretKeyEntry(SecretKeySpec(p.key, "AES")),
            KeyProtection.Builder(KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .build(),
        )
        prefs.edit()
            .putString(PROJECT, p.projectId)
            .putString(SENDER, p.senderId)
            .putString(APP_ID, p.appId)
            .putString(API_KEY, p.apiKey)
            .putString(TOPIC, p.topic)
            .putString(NAME, p.macName)
            .putLong(PAIRED_AT, now)
            .putBoolean(SUBSCRIBED, false)
            .commit()
        _info.value = load()
        if (restart) return true
        startFirebase()
        subscribe()
        return false
    }

    /** Subscribes to the topic when the subscription is not confirmed. FCM retries offline. */
    fun subscribe() {
        val i = info.value ?: return
        if (i.subscribed || FirebaseApp.getApps(context).isEmpty()) return
        FirebaseMessaging.getInstance().subscribeToTopic(i.topic).addOnCompleteListener { task ->
            if (task.isSuccessful && prefs.getString(TOPIC, null) == i.topic) {
                prefs.edit().putBoolean(SUBSCRIBED, true).apply()
                _info.value = load()
            } else if (!task.isSuccessful) {
                Log.w(TAG, "subscribe failed", task.exception)
            }
        }
    }

    /** FCM gave a new token: subscribe again. */
    fun tokenChanged() {
        if (info.value == null) return
        prefs.edit().putBoolean(SUBSCRIBED, false).apply()
        _info.value = load()
        subscribe()
    }

    /** Removes the pairing: unsubscribe, delete the key, the options and the cards. */
    fun unpair() {
        info.value?.let { unsubscribe(it.topic) }
        runCatching {
            KeyStore.getInstance(KEYSTORE).apply { load(null) }.deleteEntry(KEY_ALIAS)
        }
        prefs.edit().clear().commit()
        _info.value = null
        Graph.cards.clear()
    }

    private fun unsubscribe(topic: String) {
        if (FirebaseApp.getApps(context).isEmpty()) return
        FirebaseMessaging.getInstance().unsubscribeFromTopic(topic)
    }

    /** Restarts the process. Firebase can start only one time in a process. */
    fun restartApp() {
        val intent = context.packageManager.getLaunchIntentForPackage(context.packageName)
            ?.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
        context.startActivity(intent)
        Runtime.getRuntime().exit(0)
    }

    private companion object {
        const val TAG = "i2a"
        const val KEYSTORE = "AndroidKeyStore"
        const val KEY_ALIAS = "i2a-push-key"
        const val PROJECT = "project_id"
        const val SENDER = "sender_id"
        const val APP_ID = "app_id"
        const val API_KEY = "api_key"
        const val TOPIC = "topic"
        const val NAME = "mac_name"
        const val PAIRED_AT = "paired_at"
        const val SUBSCRIBED = "subscribed"
    }
}
