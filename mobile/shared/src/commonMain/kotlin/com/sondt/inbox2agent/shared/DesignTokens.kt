package com.sondt.inbox2agent.shared

/**
 * The design tokens of i2a. The source is docs/design-tokens.json.
 * internal/tui/tokens_test.go reads each line of the form
 * `const val Name: Long = 0xFFRRGGBB` and compares it with the JSON file.
 * Change the JSON file first, then this file.
 */
object DesignTokens {
    const val BrandPrimary: Long = 0xFF7D56F4 // brand.primary
    const val BrandOnPrimary: Long = 0xFFFFFDF5 // brand.onPrimary
    const val BrandPrimarySoft: Long = 0xFFB9A8FF // brand.primarySoft
    const val BrandPrimaryDim: Long = 0xFF3B2F6B // brand.primaryDim
    const val SurfaceBase: Long = 0xFF15131F // surface.base
    const val SurfaceRaised: Long = 0xFF1E1B2E // surface.raised
    const val SurfaceInput: Long = 0xFF2A2740 // surface.input
    const val SurfaceInputFocus: Long = 0xFF3A3360 // surface.inputFocus
    const val SurfaceButton: Long = 0xFF3A3A4E // surface.button
    const val LineBorder: Long = 0xFF4A4A5E // line.border
    const val TextDefault: Long = 0xFFD0D0D0 // text.default
    const val TextMuted: Long = 0xFF808080 // text.muted
    const val TextMutedOnBox: Long = 0xFFA8A8A8 // text.mutedOnBox
    const val StateOk: Long = 0xFF3FB950 // state.ok
    const val StateWarn: Long = 0xFFF2C94C // state.warn
    const val StateError: Long = 0xFFF85149 // state.error
    const val StateInfo: Long = 0xFF58A6FF // state.info
    const val MarkUnread: Long = 0xFFF778BA // mark.unread
    const val DangerBg: Long = 0xFF5A1E26 // danger.bg
    const val DangerFg: Long = 0xFFFFD7DC // danger.fg
    const val CategoryFg: Long = 0xFF111111 // category.fg
}
