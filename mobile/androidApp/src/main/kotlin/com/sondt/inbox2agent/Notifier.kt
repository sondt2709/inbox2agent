package com.sondt.inbox2agent

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import com.sondt.inbox2agent.shared.DesignTokens
import com.sondt.inbox2agent.shared.PushCard
import com.sondt.inbox2agent.shared.firstPlainLine
import com.sondt.inbox2agent.shared.isWebUrl
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/** The notifications of the app. docs/design.md section 8 gives the rules. */
object Notifier {
    const val CHANNEL_HIGH = "cards_high"
    const val CHANNEL_NORMAL = "cards_normal"
    const val CHANNEL_LOW = "cards_low"
    private const val GROUP = "i2a.cards"
    private const val SUMMARY_ID = 0
    private const val TEST_ID = 1
    const val EXTRA_CARD_ID = "card_id"

    fun createChannels(context: Context) {
        val nm = context.getSystemService(NotificationManager::class.java)
        nm.createNotificationChannels(
            listOf(
                NotificationChannel(CHANNEL_HIGH, "Cards: high urgency", NotificationManager.IMPORTANCE_HIGH)
                    .apply { description = "Cards with the urgency high. They show on the screen." },
                NotificationChannel(CHANNEL_NORMAL, "Cards: normal urgency", NotificationManager.IMPORTANCE_DEFAULT)
                    .apply { description = "Cards with the urgency normal, and test pushes." },
                NotificationChannel(CHANNEL_LOW, "Cards: low urgency", NotificationManager.IMPORTANCE_LOW)
                    .apply { description = "Cards with the urgency low. They make no sound." },
            ),
        )
    }

    fun canNotify(context: Context): Boolean {
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            return false
        }
        return NotificationManagerCompat.from(context).areNotificationsEnabled()
    }

    private fun channelOf(urg: String) = when (urg) {
        "high" -> CHANNEL_HIGH
        "low" -> CHANNEL_LOW
        else -> CHANNEL_NORMAL
    }

    private fun notificationId(cardId: Long): Int = (cardId xor (cardId ushr 32)).toInt().let { if (it in 0..1) it + 2 else it }

    fun openCardIntent(context: Context, cardId: Long): PendingIntent {
        val intent = Intent(context, MainActivity::class.java)
            .putExtra(EXTRA_CARD_ID, cardId)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP)
        return PendingIntent.getActivity(
            context, notificationId(cardId), intent,
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
    }

    private fun base(context: Context, channel: String) = NotificationCompat.Builder(context, channel)
        .setSmallIcon(R.drawable.ic_stat_i2a)
        .setColor(DesignTokens.BrandPrimary.toInt())
        .setAutoCancel(true)
        .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
        .setPublicVersion(
            NotificationCompat.Builder(context, channel)
                .setSmallIcon(R.drawable.ic_stat_i2a)
                .setColor(DesignTokens.BrandPrimary.toInt())
                .setContentTitle("i2a · new card")
                .build(),
        )

    @Suppress("MissingPermission")
    fun showCard(context: Context, card: PushCard) {
        if (!canNotify(context)) return
        val id = notificationId(card.id)
        val firstLine = firstPlainLine(card.body)
        val text = listOf(card.group, firstLine).filter { it.isNotEmpty() }.joinToString(": ")
        val b = base(context, channelOf(card.urg))
            .setContentTitle(card.title)
            .setContentText(text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(text))
            .setSubText("${card.cat.uppercase()} · ${card.urg}")
            .setWhen(card.created * 1000)
            .setShowWhen(true)
            .setGroup(GROUP)
            .setContentIntent(openCardIntent(context, card.id))
            .setCategory(NotificationCompat.CATEGORY_MESSAGE)
        if (card.reply.isNotEmpty()) {
            val copy = Intent(context, NotificationActionReceiver::class.java)
                .setAction(NotificationActionReceiver.ACTION_COPY)
                .putExtra(EXTRA_CARD_ID, card.id)
            b.addAction(
                0, "Copy reply",
                PendingIntent.getBroadcast(context, id, copy, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT),
            )
        }
        card.threadUrl()?.takeIf { isWebUrl(it) }?.let { url ->
            val view = Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            b.addAction(
                0, "Open thread",
                PendingIntent.getActivity(context, id, view, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT),
            )
        }
        val nm = NotificationManagerCompat.from(context)
        nm.notify(id, b.build())
        showSummary(context)
    }

    @Suppress("MissingPermission")
    private fun showSummary(context: Context) {
        val nm = context.getSystemService(NotificationManager::class.java)
        val n = nm.activeNotifications.count { it.notification.group == GROUP && it.id != SUMMARY_ID }
        val summary = base(context, CHANNEL_NORMAL)
            .setContentTitle(if (n == 1) "1 new card" else "$n new cards")
            .setGroup(GROUP)
            .setGroupSummary(true)
            .setGroupAlertBehavior(NotificationCompat.GROUP_ALERT_CHILDREN)
            .setContentIntent(
                PendingIntent.getActivity(
                    context, SUMMARY_ID, Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                    PendingIntent.FLAG_IMMUTABLE,
                ),
            )
            .build()
        NotificationManagerCompat.from(context).notify(SUMMARY_ID, summary)
    }

    @Suppress("MissingPermission")
    fun showTest(context: Context, ts: Long) {
        if (!canNotify(context)) return
        val time = DateTimeFormatter.ofPattern("HH:mm:ss").withZone(ZoneId.systemDefault())
            .format(Instant.ofEpochSecond(if (ts > 0) ts else System.currentTimeMillis() / 1000))
        val n = base(context, CHANNEL_NORMAL)
            .setContentTitle("i2a test")
            .setContentText("The Mac sent a test push at $time.")
            .setContentIntent(
                PendingIntent.getActivity(
                    context, TEST_ID, Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                    PendingIntent.FLAG_IMMUTABLE,
                ),
            )
            .build()
        NotificationManagerCompat.from(context).notify(TEST_ID, n)
    }

    fun cancelCard(context: Context, cardId: Long) {
        NotificationManagerCompat.from(context).cancel(notificationId(cardId))
    }
}
