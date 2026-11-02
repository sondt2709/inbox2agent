package com.sondt.inbox2agent.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.sondt.inbox2agent.shared.Urgency

/** The brand mark: "i2a" on the primary color, as the TUI header shows it. */
@Composable
fun BrandMark() {
    Text(
        "i2a",
        style = I2aType.label.copy(fontWeight = FontWeight.Bold),
        color = I2aColors.onPrimary,
        modifier = Modifier
            .clip(RoundedCornerShape(4.dp))
            .background(I2aColors.primary)
            .padding(horizontal = 6.dp, vertical = 2.dp),
    )
}

/** The top bar on surface.raised. With [onBack], it shows a back button and no brand mark. */
@Composable
fun TopBar(title: String, onBack: (() -> Unit)? = null, actions: @Composable () -> Unit = {}) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(I2aColors.raised)
            .padding(horizontal = 4.dp)
            .height(56.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (onBack != null) {
            IconButton(onClick = onBack) {
                Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = "Back", tint = I2aColors.text)
            }
        } else {
            Spacer(Modifier.width(12.dp))
            BrandMark()
            Spacer(Modifier.width(8.dp))
        }
        Text(title, style = I2aType.screenTitle, color = I2aColors.text, modifier = Modifier.weight(1f))
        actions()
    }
}

/** The category badge: the label in category.fg on the category color. */
@Composable
fun CategoryBadge(cat: String) {
    Text(
        cat.uppercase(),
        style = I2aType.label.copy(fontWeight = FontWeight.Bold),
        color = I2aColors.categoryFg,
        modifier = Modifier
            .clip(RoundedCornerShape(4.dp))
            .background(I2aColors.category(cat))
            .padding(horizontal = 4.dp, vertical = 1.dp),
    )
}

/** The urgency bars, with a text alternative for TalkBack. */
@Composable
fun UrgencyBars(urg: String) {
    Text(
        Urgency.bars(urg),
        style = I2aType.meta.copy(fontWeight = if (urg == "high") FontWeight.Bold else FontWeight.Normal),
        color = I2aColors.urgency(urg),
        modifier = Modifier.semantics { contentDescription = Urgency.description(urg) },
    )
}

@Composable
fun UnreadDot(unread: Boolean) {
    Box(
        Modifier
            .size(8.dp)
            .clip(CircleShape)
            .background(if (unread) I2aColors.unread else I2aColors.base)
            .semantics { if (unread) contentDescription = "unread" },
    )
}

/** A box on surface.input with a small label, as in docs/design.md section 7. */
@Composable
fun LabeledBox(label: String, modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    Column(
        modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(12.dp))
            .background(I2aColors.input)
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(label.uppercase(), style = I2aType.label, color = I2aColors.mutedOnBox)
        content()
    }
}
