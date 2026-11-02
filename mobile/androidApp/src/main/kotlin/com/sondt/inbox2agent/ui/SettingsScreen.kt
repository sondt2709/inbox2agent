package com.sondt.inbox2agent.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.sondt.inbox2agent.BuildConfig
import com.sondt.inbox2agent.PairingInfo

@Composable
fun SettingsScreen(
    pairing: PairingInfo?,
    notificationsAllowed: Boolean,
    onAllowNotifications: () -> Unit,
    onPair: () -> Unit,
    onUnpair: () -> Unit,
    onBack: () -> Unit,
) {
    var confirm by remember { mutableStateOf(false) }
    Column(Modifier.fillMaxSize().background(I2aColors.base)) {
        TopBar("Settings", onBack = onBack)
        Column(
            Modifier.verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            LabeledBox("Pairing") {
                if (pairing == null) {
                    Text("This phone is not paired.", style = I2aType.body, color = I2aColors.text)
                } else {
                    Field("Mac", pairing.macName)
                    Field("Paired", fullTime(pairing.pairedAt / 1000))
                    Field("Firebase project", pairing.projectId)
                    Field("Topic", pairing.topic.take(12) + "…")
                    Field("Subscription", if (pairing.subscribed) "active" else "waiting for the network")
                }
            }
            LabeledBox("Notifications") {
                Text(
                    if (notificationsAllowed) "Allowed." else "Not allowed. The phone gets cards but shows no notification.",
                    style = I2aType.body,
                    color = if (notificationsAllowed) I2aColors.ok else I2aColors.warn,
                )
                if (!notificationsAllowed) {
                    FilledTonalButton(onClick = onAllowNotifications, modifier = Modifier.heightIn(min = 48.dp)) { Text("Allow notifications") }
                }
            }
            if (pairing == null) {
                Button(onClick = onPair, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) { Text("Pair with your Mac") }
            } else {
                Button(
                    onClick = { confirm = true },
                    modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
                    colors = ButtonDefaults.buttonColors(containerColor = I2aColors.dangerBg, contentColor = I2aColors.dangerFg),
                ) { Text("Unpair") }
            }
            Text("i2a for Android ${BuildConfig.VERSION_NAME}. Reply, Ask and Done from the phone come later.", style = I2aType.meta, color = I2aColors.muted)
        }
    }
    if (confirm) {
        AlertDialog(
            onDismissRequest = { confirm = false },
            title = { Text("Unpair this phone?") },
            text = { Text("The phone stops to get cards, and it deletes the cards and the key. To pair again, run i2a phone pair on the Mac.") },
            confirmButton = {
                Button(
                    onClick = { confirm = false; onUnpair() },
                    colors = ButtonDefaults.buttonColors(containerColor = I2aColors.dangerBg, contentColor = I2aColors.dangerFg),
                ) { Text("Unpair") }
            },
            dismissButton = { TextButton(onClick = { confirm = false }) { Text("Cancel") } },
            containerColor = I2aColors.raised,
        )
    }
}

@Composable
private fun Field(label: String, value: String) {
    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(label, style = I2aType.meta, color = I2aColors.mutedOnBox, modifier = Modifier.weight(0.4f))
        Text(value, style = I2aType.body, color = I2aColors.text, modifier = Modifier.weight(0.6f))
    }
}
