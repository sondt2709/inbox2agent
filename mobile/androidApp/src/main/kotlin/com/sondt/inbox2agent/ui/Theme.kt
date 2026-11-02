package com.sondt.inbox2agent.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.em
import androidx.compose.ui.unit.sp
import com.sondt.inbox2agent.shared.DesignTokens as T

/** Colors of docs/design-tokens.json. Only this file maps them to Material 3 roles. */
object I2aColors {
    val primary = Color(T.BrandPrimary)
    val onPrimary = Color(T.BrandOnPrimary)
    val primarySoft = Color(T.BrandPrimarySoft)
    val primaryDim = Color(T.BrandPrimaryDim)
    val base = Color(T.SurfaceBase)
    val raised = Color(T.SurfaceRaised)
    val input = Color(T.SurfaceInput)
    val inputFocus = Color(T.SurfaceInputFocus)
    val button = Color(T.SurfaceButton)
    val border = Color(T.LineBorder)
    val text = Color(T.TextDefault)
    val muted = Color(T.TextMuted)
    val mutedOnBox = Color(T.TextMutedOnBox)
    val ok = Color(T.StateOk)
    val warn = Color(T.StateWarn)
    val error = Color(T.StateError)
    val info = Color(T.StateInfo)
    val unread = Color(T.MarkUnread)
    val dangerBg = Color(T.DangerBg)
    val dangerFg = Color(T.DangerFg)
    val categoryFg = Color(T.CategoryFg)

    fun category(cat: String): Color = when (cat) {
        "action" -> error
        "question" -> warn
        "info" -> info
        else -> button
    }

    fun urgency(urg: String): Color = when (urg) {
        "high" -> error
        "low" -> muted
        else -> text
    }
}

// Dark only, and no dynamic color: the primary color is the TUI accent on each phone.
private val scheme = darkColorScheme(
    primary = I2aColors.primary,
    onPrimary = I2aColors.onPrimary,
    primaryContainer = I2aColors.primaryDim,
    onPrimaryContainer = I2aColors.onPrimary,
    secondary = I2aColors.primarySoft,
    onSecondary = I2aColors.base,
    secondaryContainer = I2aColors.button,
    onSecondaryContainer = I2aColors.text,
    tertiary = I2aColors.primarySoft,
    background = I2aColors.base,
    onBackground = I2aColors.text,
    surface = I2aColors.base,
    onSurface = I2aColors.text,
    surfaceVariant = I2aColors.input,
    onSurfaceVariant = I2aColors.mutedOnBox,
    surfaceContainerLowest = I2aColors.base,
    surfaceContainerLow = I2aColors.raised,
    surfaceContainer = I2aColors.raised,
    surfaceContainerHigh = I2aColors.input,
    surfaceContainerHighest = I2aColors.inputFocus,
    inverseSurface = I2aColors.text,
    inverseOnSurface = I2aColors.base,
    inversePrimary = I2aColors.primaryDim,
    outline = I2aColors.border,
    outlineVariant = I2aColors.border,
    error = I2aColors.error,
    onError = I2aColors.categoryFg,
    errorContainer = I2aColors.dangerBg,
    onErrorContainer = I2aColors.dangerFg,
    scrim = Color.Black,
)

/** The type scale of docs/design.md section 3. */
object I2aType {
    val screenTitle = TextStyle(fontSize = 20.sp, lineHeight = 28.sp, fontWeight = FontWeight.Bold)
    val cardTitle = TextStyle(fontSize = 18.sp, lineHeight = 24.sp, fontWeight = FontWeight.Bold)
    val rowTitle = TextStyle(fontSize = 15.sp, lineHeight = 20.sp, fontWeight = FontWeight.SemiBold)
    val body = TextStyle(fontSize = 15.sp, lineHeight = 22.sp)
    val meta = TextStyle(fontSize = 12.sp, lineHeight = 16.sp, fontFamily = FontFamily.Monospace)
    val label = TextStyle(
        fontSize = 11.sp, lineHeight = 14.sp, fontFamily = FontFamily.Monospace,
        fontWeight = FontWeight.SemiBold, letterSpacing = 0.06.em,
    )
}

@Composable
fun I2aTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = scheme,
        typography = Typography(bodyLarge = I2aType.body, bodyMedium = I2aType.body, titleLarge = I2aType.screenTitle),
        content = content,
    )
}
