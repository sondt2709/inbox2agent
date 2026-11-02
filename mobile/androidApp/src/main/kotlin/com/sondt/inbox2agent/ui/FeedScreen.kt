package com.sondt.inbox2agent.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.NotificationsNone
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.sondt.inbox2agent.PairingInfo
import com.sondt.inbox2agent.shared.Categories
import com.sondt.inbox2agent.shared.StoredCard

@Composable
fun FeedScreen(
    cards: List<StoredCard>,
    pairing: PairingInfo?,
    lastPush: Long,
    onOpen: (Long) -> Unit,
    onPair: () -> Unit,
    onSettings: () -> Unit,
) {
    var filter by rememberSaveable { mutableStateOf("") }
    val shown = if (filter.isEmpty()) cards else cards.filter { it.card.cat == filter }
    Column(Modifier.fillMaxSize().background(I2aColors.base)) {
        TopBar("Feed") {
            if (lastPush > 0) {
                Text("last push ${whenText(lastPush)}", style = I2aType.meta, color = I2aColors.muted)
            }
            IconButton(onClick = onSettings) {
                Icon(Icons.Outlined.Settings, contentDescription = "Settings", tint = I2aColors.text)
            }
        }
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Chip("All ${cards.size}", filter.isEmpty()) { filter = "" }
            for (c in Categories.all) {
                val n = cards.count { it.card.cat == c }
                Chip("${c.replaceFirstChar { it.uppercase() }} $n", filter == c) { filter = if (filter == c) "" else c }
            }
        }
        if (shown.isEmpty()) {
            EmptyFeed(paired = pairing != null, filtered = cards.isNotEmpty(), onPair = onPair)
        } else {
            LazyColumn(Modifier.fillMaxSize()) {
                items(shown, key = { it.card.id }) { c ->
                    CardRow(c, onClick = { onOpen(c.card.id) })
                    HorizontalDivider(color = I2aColors.raised)
                }
            }
        }
    }
}

@Composable
private fun Chip(label: String, selected: Boolean, onClick: () -> Unit) {
    FilterChip(
        selected = selected,
        onClick = onClick,
        label = { Text(label, style = I2aType.meta.copy(fontWeight = FontWeight.SemiBold)) },
        colors = FilterChipDefaults.filterChipColors(
            containerColor = I2aColors.raised,
            labelColor = I2aColors.mutedOnBox,
            selectedContainerColor = I2aColors.primary,
            selectedLabelColor = I2aColors.onPrimary,
        ),
        border = FilterChipDefaults.filterChipBorder(
            enabled = true, selected = selected,
            borderColor = I2aColors.border, selectedBorderColor = I2aColors.primary,
        ),
    )
}

/** One card row: the unread dot, the badge, the bars and the time; the title; the group. */
@Composable
private fun CardRow(c: StoredCard, onClick: () -> Unit) {
    Row(
        Modifier
            .fillMaxWidth()
            .heightIn(min = 72.dp)
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Box(Modifier.padding(top = 6.dp)) { UnreadDot(!c.read) }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                CategoryBadge(c.card.cat)
                UrgencyBars(c.card.urg)
                Spacer(Modifier.weight(1f))
                Text(whenText(c.card.created), style = I2aType.meta, color = I2aColors.muted)
            }
            Text(
                c.card.title,
                style = I2aType.rowTitle.copy(fontWeight = if (c.read) FontWeight.Medium else FontWeight.Bold),
                color = I2aColors.text,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
            if (c.card.group.isNotEmpty()) {
                Text(c.card.group, style = I2aType.meta, color = I2aColors.muted, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

@Composable
private fun EmptyFeed(paired: Boolean, filtered: Boolean, onPair: () -> Unit) {
    Column(
        Modifier.fillMaxSize().padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Icon(Icons.Outlined.NotificationsNone, contentDescription = null, tint = I2aColors.primarySoft, modifier = Modifier.size(48.dp))
        Spacer(Modifier.width(16.dp).padding(8.dp))
        val text = when {
            filtered -> "No cards in this category."
            paired -> "No cards yet. New cards from your Mac show here."
            else -> "No cards yet. Pair this phone with i2a on your Mac."
        }
        Text(text, style = I2aType.body, color = I2aColors.text, textAlign = TextAlign.Center)
        if (!paired) {
            Spacer(Modifier.padding(12.dp))
            Button(onClick = onPair, modifier = Modifier.heightIn(min = 48.dp)) { Text("Pair with your Mac") }
        }
    }
}
