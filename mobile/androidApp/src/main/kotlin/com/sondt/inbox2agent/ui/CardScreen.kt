package com.sondt.inbox2agent.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.automirrored.outlined.OpenInNew
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.sondt.inbox2agent.shared.PushCard

@Composable
fun CardScreen(
    card: PushCard,
    onBack: () -> Unit,
    onCopy: () -> Unit,
    onOpenThread: () -> Unit,
    onHide: () -> Unit,
) {
    Column(Modifier.fillMaxSize().background(I2aColors.base)) {
        TopBar("Card", onBack = onBack)
        Column(
            Modifier.weight(1f).verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                CategoryBadge(card.cat)
                UrgencyBars(card.urg)
                Text(
                    listOf(card.urg, card.group, whenText(card.created)).filter { it.isNotEmpty() }.joinToString(" · "),
                    style = I2aType.meta, color = I2aColors.muted,
                )
            }
            Text(card.title, style = I2aType.cardTitle, color = I2aColors.text)
            if (card.cut) {
                Text("The full text is on the Mac.", style = I2aType.meta, color = I2aColors.warn)
            }
            if (card.body.isNotEmpty()) {
                Text(markdownText(card.body), style = I2aType.body, color = I2aColors.text)
            }
            if (card.refs.isNotEmpty()) {
                LabeledBox("Source messages") {
                    for (r in card.refs) {
                        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                Text(r.author, style = I2aType.body.copy(fontWeight = FontWeight.Bold), color = I2aColors.text)
                                Text(
                                    listOf(r.channel, whenText(r.at)).filter { it.isNotEmpty() }.joinToString(" · "),
                                    style = I2aType.meta, color = I2aColors.mutedOnBox,
                                    modifier = Modifier.align(Alignment.CenterVertically),
                                )
                            }
                            Text(r.text, style = I2aType.body, color = I2aColors.text)
                        }
                    }
                }
            }
            if (card.reply.isNotEmpty()) {
                LabeledBox("Suggested reply") {
                    Row(Modifier.height(IntrinsicSize.Min)) {
                        Box(Modifier.width(3.dp).fillMaxHeight().clip(RoundedCornerShape(2.dp)).background(I2aColors.primary))
                        Text(card.reply, style = I2aType.body, color = I2aColors.text, modifier = Modifier.padding(start = 10.dp))
                    }
                }
            }
        }
        // Reply, Ask and Done from the phone come later. They need a path from the phone to the Mac.
        Row(Modifier.fillMaxWidth().background(I2aColors.raised).padding(vertical = 4.dp)) {
            BarAction(Icons.Outlined.ContentCopy, "Copy reply", enabled = card.reply.isNotEmpty(), onClick = onCopy)
            BarAction(Icons.AutoMirrored.Outlined.OpenInNew, "Open thread", enabled = card.threadUrl() != null, onClick = onOpenThread)
            BarAction(Icons.Outlined.VisibilityOff, "Hide", enabled = true, onClick = onHide)
        }
    }
}

@Composable
private fun androidx.compose.foundation.layout.RowScope.BarAction(
    icon: ImageVector, label: String, enabled: Boolean, onClick: () -> Unit,
) {
    TextButton(
        onClick = onClick,
        enabled = enabled,
        modifier = Modifier.weight(1f).heightIn(min = 56.dp),
    ) {
        Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(2.dp)) {
            Icon(icon, contentDescription = null, tint = if (enabled) I2aColors.primarySoft else I2aColors.muted)
            Text(label, style = I2aType.meta, color = if (enabled) I2aColors.text else I2aColors.muted)
        }
    }
}
