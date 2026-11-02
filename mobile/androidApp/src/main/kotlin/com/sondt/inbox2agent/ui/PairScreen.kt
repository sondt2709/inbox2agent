package com.sondt.inbox2agent.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material3.Button
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp

@Composable
fun PairScreen(
    link: String,
    error: String?,
    busy: Boolean,
    onLinkChange: (String) -> Unit,
    onScan: () -> Unit,
    onPair: () -> Unit,
    onBack: () -> Unit,
) {
    Column(Modifier.fillMaxSize().background(I2aColors.base)) {
        TopBar("Pair", onBack = onBack)
        Column(
            Modifier.verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text("Pair this phone with i2a on your Mac.", style = I2aType.cardTitle, color = I2aColors.text)
            LabeledBox("Steps") {
                Text("1. On the Mac, run i2a phone pair.", style = I2aType.body, color = I2aColors.text)
                Text("2. Tap Scan QR code and point the camera at the code.", style = I2aType.body, color = I2aColors.text)
                Text("3. Or paste the pairing link below, then tap Pair.", style = I2aType.body, color = I2aColors.text)
            }
            FilledTonalButton(onClick = onScan, modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp)) {
                Icon(Icons.Outlined.QrCodeScanner, contentDescription = null)
                Text("  Scan QR code")
            }
            OutlinedTextField(
                value = link,
                onValueChange = onLinkChange,
                label = { Text("Pairing link") },
                placeholder = { Text("i2a://pair?v=1&…") },
                isError = error != null,
                supportingText = {
                    Text(error ?: "The link holds a secret key. Do not share it.", color = if (error != null) I2aColors.error else I2aColors.muted)
                },
                textStyle = I2aType.meta.copy(fontFamily = FontFamily.Monospace, color = I2aColors.text),
                minLines = 3,
                modifier = Modifier.fillMaxWidth(),
                colors = OutlinedTextFieldDefaults.colors(
                    focusedContainerColor = I2aColors.inputFocus,
                    unfocusedContainerColor = I2aColors.input,
                    focusedBorderColor = I2aColors.primarySoft,
                    unfocusedBorderColor = I2aColors.border,
                    focusedLabelColor = I2aColors.primarySoft,
                    unfocusedLabelColor = I2aColors.mutedOnBox,
                ),
            )
            Button(
                onClick = onPair,
                enabled = link.isNotBlank() && !busy,
                modifier = Modifier.fillMaxWidth().heightIn(min = 48.dp),
            ) { Text(if (busy) "Pairing…" else "Pair") }
        }
    }
}
