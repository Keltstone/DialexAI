package com.dialex.ui

import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Terminal
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.dialex.theme.LocalCcColors
import kotlin.io.encoding.Base64
import kotlin.io.encoding.ExperimentalEncodingApi

/**
 * Official Keltstone Systems logo component across Android and Desktop.
 * Renders the official Kolt arrow-K monogram mark.
 */
@OptIn(ExperimentalEncodingApi::class)
@Composable
fun KeltstoneLogoView(
    modifier: Modifier = Modifier,
    size: Dp = 28.dp,
    shape: Shape? = null,
    contentDescription: String? = "Keltstone Systems"
) {
    val cc = LocalCcColors.current
    val imageBitmap: ImageBitmap? = remember {
        runCatching {
            val bytes = Base64.Default.decode(KOLT_LOGO_BASE64)
            decodeImageBitmap(bytes)
        }.getOrNull()
    }

    if (imageBitmap != null) {
        val clipModifier = if (shape != null) modifier.clip(shape) else modifier
        Image(
            bitmap = imageBitmap,
            contentDescription = contentDescription,
            contentScale = ContentScale.Fit,
            modifier = clipModifier.size(size)
        )
    } else {
        Box(
            modifier = modifier
                .size(size)
                .clip(shape ?: RoundedCornerShape(percent = 20))
                .background(cc.accent.copy(alpha = 0.15f)),
            contentAlignment = Alignment.Center
        ) {
            Icon(
                Icons.Outlined.Terminal,
                contentDescription = contentDescription,
                tint = cc.accent,
                modifier = Modifier.size(size * 0.6f)
            )
        }
    }
}

/**
 * Backward-compatible alias for [KeltstoneLogoView].
 */
@Composable
fun KoltSystemsLogoView(
    modifier: Modifier = Modifier,
    size: Dp = 28.dp,
    shape: Shape? = null,
    contentDescription: String? = "Keltstone Systems"
) {
    KeltstoneLogoView(modifier = modifier, size = size, shape = shape, contentDescription = contentDescription)
}

/**
 * Full Brand Lockup with Logo and Typography for Keltstone Systems.
 */
@Composable
fun KeltstoneBrandLockup(
    modifier: Modifier = Modifier,
    logoSize: Dp = 28.dp,
    showSubtitle: Boolean = true
) {
    val cc = LocalCcColors.current
    Row(
        modifier = modifier,
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        KeltstoneLogoView(size = logoSize)
        Column(verticalArrangement = Arrangement.Center) {
            Text(
                "KELTSTONE SYSTEMS",
                style = MaterialTheme.typography.titleMedium.copy(
                    fontWeight = FontWeight.ExtraBold,
                    fontSize = 13.5.sp,
                    letterSpacing = 1.2.sp,
                    fontFamily = FontFamily.SansSerif
                ),
                color = cc.textPrimary
            )
            if (showSubtitle) {
                Text(
                    "Independent developer tools for private, local compute.",
                    style = MaterialTheme.typography.labelSmall.copy(
                        fontSize = 10.5.sp,
                        fontWeight = FontWeight.Medium
                    ),
                    color = cc.textMuted
                )
            }
        }
    }
}

/**
 * Backward-compatible alias for [KeltstoneBrandLockup].
 */
@Composable
fun KoltSystemsBrandLockup(
    modifier: Modifier = Modifier,
    logoSize: Dp = 28.dp,
    showSubtitle: Boolean = true
) {
    KeltstoneBrandLockup(modifier = modifier, logoSize = logoSize, showSubtitle = showSubtitle)
}
