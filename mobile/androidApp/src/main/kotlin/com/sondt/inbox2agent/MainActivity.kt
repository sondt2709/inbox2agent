package com.sondt.inbox2agent

import android.Manifest
import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning
import com.sondt.inbox2agent.shared.CardRepository
import com.sondt.inbox2agent.shared.DesignTokens
import com.sondt.inbox2agent.shared.PairingLink
import com.sondt.inbox2agent.shared.PairingLinkException
import com.sondt.inbox2agent.shared.isWebUrl
import com.sondt.inbox2agent.ui.CardScreen
import com.sondt.inbox2agent.ui.FeedScreen
import com.sondt.inbox2agent.ui.I2aColors
import com.sondt.inbox2agent.ui.I2aTheme
import com.sondt.inbox2agent.ui.PairScreen
import com.sondt.inbox2agent.ui.SettingsScreen
import kotlinx.coroutines.launch

/** The screens of the app. A simple stack; the app has four screens. */
sealed interface Screen {
    data object Feed : Screen
    data class Card(val id: Long) : Screen
    data class Pair(val link: String = "") : Screen
    data object Settings : Screen
}

class MainActivity : ComponentActivity() {
    private val requested = mutableStateOf<Screen?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val bars = SystemBarStyle.dark(DesignTokens.SurfaceRaised.toInt())
        enableEdgeToEdge(statusBarStyle = bars, navigationBarStyle = bars)
        requested.value = screenOf(intent)
        Graph.pairing.subscribe()
        Graph.cards.prune()
        setContent { I2aTheme { App() } }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        requested.value = screenOf(intent)
    }

    private fun screenOf(intent: Intent?): Screen? {
        if (intent == null) return null
        val data = intent.data
        if (intent.action == Intent.ACTION_VIEW && data?.scheme == "i2a" && data.host == "pair") {
            return Screen.Pair(data.toString())
        }
        val id = intent.getLongExtra(Notifier.EXTRA_CARD_ID, 0)
        return if (id != 0L) Screen.Card(id) else null
    }

    private fun notificationsAllowed() = Notifier.canNotify(this)

    @Composable
    private fun App() {
        val cards by Graph.cards.state.collectAsStateWithLifecycle()
        val pairing by Graph.pairing.info.collectAsStateWithLifecycle()
        var stack by remember { mutableStateOf(listOf<Screen>(Screen.Feed)) }
        var pairLink by remember { mutableStateOf("") }
        var pairError by remember { mutableStateOf<String?>(null) }
        var busy by remember { mutableStateOf(false) }
        var allowed by remember { mutableStateOf(notificationsAllowed()) }
        val snackbar = remember { SnackbarHostState() }
        val scope = rememberCoroutineScope()
        fun say(text: String) = scope.launch { snackbar.currentSnackbarData?.dismiss(); snackbar.showSnackbar(text) }
        fun push(s: Screen) {
            if (s is Screen.Pair) {
                pairLink = s.link; pairError = null
            }
            stack = stack.filter { it != s && !(it is Screen.Card && s is Screen.Card) } + s
        }
        fun pop() {
            if (stack.size > 1) stack = stack.dropLast(1)
        }

        val permission = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { ok ->
            allowed = notificationsAllowed()
            if (!ok) say("Notifications are off. The Feed still gets the cards.")
        }
        fun askNotifications() {
            if (Build.VERSION.SDK_INT >= 33 && !notificationsAllowed()) {
                permission.launch(Manifest.permission.POST_NOTIFICATIONS)
            } else if (!notificationsAllowed()) {
                startActivity(Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, packageName))
            }
        }

        val req = requested.value
        LaunchedEffect(req) {
            if (req != null) {
                push(req)
                requested.value = null
            }
        }

        BackHandler(enabled = stack.size > 1) { pop() }

        Box(Modifier.fillMaxSize().background(I2aColors.base).safeDrawingPadding()) {
            when (val s = stack.last()) {
                Screen.Feed -> FeedScreen(
                    cards = CardRepository.visible(cards),
                    pairing = pairing,
                    lastPush = cards.lastPush,
                    onOpen = { push(Screen.Card(it)) },
                    onPair = { push(Screen.Pair()) },
                    onSettings = { allowed = notificationsAllowed(); push(Screen.Settings) },
                )
                is Screen.Card -> {
                    val stored = CardRepository.find(cards, s.id)
                    if (stored == null) {
                        LaunchedEffect(s.id) { pop(); say("The card is not on this phone.") }
                    } else {
                        LaunchedEffect(s.id) {
                            Graph.cards.markRead(s.id)
                            Notifier.cancelCard(this@MainActivity, s.id)
                        }
                        val card = stored.card
                        CardScreen(
                            card = card,
                            onBack = { pop() },
                            onCopy = {
                                NotificationActionReceiver.copyText(this@MainActivity, card.reply)
                                say("Copied the suggested reply.")
                            },
                            onOpenThread = {
                                val url = card.threadUrl()
                                if (url != null && isWebUrl(url)) {
                                    try {
                                        startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
                                    } catch (e: ActivityNotFoundException) {
                                        say("No app can open the link.")
                                    }
                                }
                            },
                            onHide = {
                                Graph.cards.hide(card.id)
                                pop()
                                say("The card is hidden on this phone. The Mac does not change.")
                            },
                        )
                    }
                }
                is Screen.Pair -> PairScreen(
                    link = pairLink,
                    error = pairError,
                    busy = busy,
                    onLinkChange = { pairLink = it; pairError = null },
                    onScan = {
                        val options = GmsBarcodeScannerOptions.Builder().setBarcodeFormats(Barcode.FORMAT_QR_CODE).build()
                        GmsBarcodeScanning.getClient(this@MainActivity, options).startScan()
                            .addOnSuccessListener { b -> pairLink = b.rawValue.orEmpty(); pairError = null }
                            .addOnFailureListener { e -> pairError = "The scanner does not work here: ${e.message}. Paste the link." }
                    },
                    onPair = {
                        try {
                            val p = PairingLink.parse(pairLink)
                            busy = true
                            val restart = Graph.pairing.pair(p, System.currentTimeMillis())
                            busy = false
                            if (restart) {
                                Graph.cards.clear()
                                Graph.pairing.restartApp()
                            } else {
                                stack = listOf(Screen.Feed)
                                say("Paired with ${p.macName}.")
                                askNotifications()
                            }
                        } catch (e: PairingLinkException) {
                            busy = false
                            pairError = e.message
                        } catch (e: Exception) {
                            busy = false
                            pairError = "The pairing failed: ${e.message}"
                        }
                    },
                    onBack = { pop() },
                )
                Screen.Settings -> SettingsScreen(
                    pairing = pairing,
                    notificationsAllowed = allowed,
                    onAllowNotifications = { askNotifications() },
                    onPair = { push(Screen.Pair()) },
                    onUnpair = {
                        Graph.pairing.unpair()
                        stack = listOf(Screen.Feed)
                        say("This phone is not paired now.")
                    },
                    onBack = { pop() },
                )
            }
            SnackbarHost(snackbar, Modifier.align(Alignment.BottomCenter).padding(16.dp))
        }
    }
}
