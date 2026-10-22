package com.dialex.ui

import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.dialex.theme.CcPalette
import com.dialex.theme.LocalCcColors
import kolt.composekmp.components.core.dropdowns.models.DropDownItem
import kolt.composekmp.wrappers.toUiText

data class ThemedDropdownOption(
    val id: String,
    val title: String,
    val subtitle: String? = null,
    val icon: ImageVector? = null,
    val isDividerBefore: Boolean = false,
    val isAccent: Boolean = false,
    val isHeader: Boolean = false,
)

/**
 * Kolt-powered themed dropdown adhering strictly to Dialex palette.
 * Provides smooth animated chevron, rounded container, and custom styled popup menu.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ThemedDropdown(
    selectedId: String?,
    options: List<ThemedDropdownOption>,
    onSelect: (ThemedDropdownOption) -> Unit,
    modifier: Modifier = Modifier,
    placeholder: String = "Select...",
    leadingIcon: ImageVector? = null,
    minHeight: Dp = 34.dp,
    fontSize: androidx.compose.ui.unit.TextUnit = 13.sp,
    isMinimal: Boolean = false,
    containerColor: Color? = null,
    borderColor: Color? = null,
    isCompact: Boolean = false,
    cc: CcPalette = LocalCcColors.current
) {
    var expanded by remember { mutableStateOf(false) }
    val selectedOption = options.firstOrNull { it.id == selectedId && !it.isHeader }

    val rotation by animateFloatAsState(
        targetValue = if (expanded) 180f else 0f,
        animationSpec = spring(stiffness = Spring.StiffnessLow),
        label = "DropdownChevronRotation"
    )

    Box(modifier = modifier) {
        // Trigger Box - subtle rounded container matching reference dropdowns
        val shape = RoundedCornerShape(8.dp)
        val resolvedBackground = containerColor ?: if (isMinimal) Color.Transparent else cc.panelAlt
        val resolvedBorder = borderColor ?: if (!isMinimal) cc.border.copy(alpha = 0.5f) else null

        Row(
            modifier = Modifier
                .clip(shape)
                .background(resolvedBackground)
                .then(
                    if (resolvedBorder != null) Modifier.border(BorderStroke(0.75.dp, resolvedBorder), shape)
                    else Modifier
                )
                .clickable { expanded = !expanded }
                .padding(horizontal = if (isMinimal) 8.dp else 12.dp, vertical = 2.dp)
                .heightIn(min = if (isCompact) 44.dp else minHeight),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.weight(1f, fill = false)
            ) {
                val iconToShow = selectedOption?.icon ?: leadingIcon
                if (iconToShow != null) {
                    Icon(
                        iconToShow,
                        contentDescription = null,
                        tint = cc.textMuted,
                        modifier = Modifier.size(if (isMinimal) 14.dp else 16.dp)
                    )
                    Spacer(Modifier.width(if (isMinimal) 5.dp else 8.dp))
                }

                Text(
                    text = selectedOption?.title ?: placeholder,
                    style = MaterialTheme.typography.bodySmall.copy(
                        fontWeight = if (selectedOption != null) FontWeight.Medium else FontWeight.Normal,
                        fontSize = fontSize
                    ),
                    color = if (selectedOption != null) cc.textPrimary else cc.textMuted.copy(alpha = 0.7f),
                    maxLines = 1
                )
            }

            Spacer(Modifier.width(if (isMinimal) 3.dp else 8.dp))

            Icon(
                Icons.Default.ArrowDropDown,
                contentDescription = "Open dropdown",
                tint = if (isMinimal) cc.textMuted.copy(alpha = 0.55f) else cc.textMuted,
                modifier = Modifier
                    .size(if (isMinimal) 16.dp else 20.dp)
                    .graphicsLayer { rotationZ = rotation }
            )
        }

        // On mobile / compact screens, use native ModalBottomSheet with ergonomic touch targets
        if (isCompact && expanded) {
            ModalBottomSheet(
                onDismissRequest = { expanded = false },
                containerColor = cc.panel,
                contentColor = cc.textPrimary,
                scrimColor = Color.Black.copy(alpha = 0.55f),
                shape = RoundedCornerShape(topStart = 16.dp, topEnd = 16.dp),
                dragHandle = {
                    Box(
                        modifier = Modifier
                            .padding(vertical = 10.dp)
                            .size(width = 36.dp, height = 4.dp)
                            .clip(RoundedCornerShape(2.dp))
                            .background(cc.border)
                    )
                }
            ) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp)
                        .padding(bottom = 32.dp)
                ) {
                    Text(
                        text = placeholder,
                        style = MaterialTheme.typography.titleSmall.copy(fontSize = 15.sp, fontWeight = FontWeight.SemiBold),
                        color = cc.textPrimary,
                        modifier = Modifier.padding(horizontal = 8.dp, vertical = 8.dp)
                    )
                    HorizontalDivider(color = cc.border.copy(alpha = 0.4f), thickness = 0.75.dp)
                    Spacer(Modifier.height(8.dp))

                    options.forEach { opt ->
                        if (opt.isDividerBefore) {
                            HorizontalDivider(color = cc.border.copy(alpha = 0.3f), thickness = 0.5.dp, modifier = Modifier.padding(vertical = 4.dp))
                        }
                        if (opt.isHeader) {
                            Box(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(horizontal = 14.dp, vertical = 6.dp)
                            ) {
                                Text(
                                    text = opt.title,
                                    style = MaterialTheme.typography.labelSmall.copy(
                                        fontSize = 11.sp,
                                        fontWeight = FontWeight.Bold,
                                        letterSpacing = 0.5.sp
                                    ),
                                    color = cc.textMuted
                                )
                            }
                        } else {
                            val isSelected = opt.id == selectedId
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .heightIn(min = 48.dp)
                                    .clip(RoundedCornerShape(8.dp))
                                    .background(if (isSelected) cc.panelAlt else Color.Transparent)
                                    .clickable {
                                        expanded = false
                                        onSelect(opt)
                                    }
                                    .padding(horizontal = 12.dp, vertical = 8.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.SpaceBetween
                            ) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    modifier = Modifier.weight(1f)
                                ) {
                                    if (opt.icon != null) {
                                        Icon(
                                            opt.icon,
                                            contentDescription = null,
                                            tint = cc.textMuted,
                                            modifier = Modifier.size(18.dp)
                                        )
                                        Spacer(Modifier.width(10.dp))
                                    }
                                    Column {
                                        Text(
                                            text = opt.title,
                                            style = MaterialTheme.typography.bodyMedium.copy(
                                                fontSize = 14.sp,
                                                fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal
                                            ),
                                            color = cc.textPrimary
                                        )
                                        if (opt.subtitle != null) {
                                            Text(
                                                text = opt.subtitle,
                                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                                color = cc.textMuted
                                            )
                                        }
                                    }
                                }
                                if (isSelected) {
                                    Icon(
                                        Icons.Default.Check,
                                        contentDescription = "Selected",
                                        tint = cc.textPrimary,
                                        modifier = Modifier.size(18.dp)
                                    )
                                }
                            }
                        }
                    }
                }
            }
        } else {
            // Desktop / Expanded screen popup menu
            DropdownMenu(
                expanded = expanded,
                onDismissRequest = { expanded = false },
                modifier = Modifier
                    .background(cc.panel)
                    .border(BorderStroke(1.dp, cc.border), RoundedCornerShape(10.dp))
                    .clip(RoundedCornerShape(10.dp))
            ) {
                options.forEach { opt ->
                    if (opt.isDividerBefore) {
                        HorizontalDivider(color = cc.border.copy(alpha = 0.5f), thickness = 0.75.dp)
                    }

                    if (opt.isHeader) {
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 12.dp, vertical = 6.dp)
                        ) {
                            Text(
                                text = opt.title,
                                style = MaterialTheme.typography.labelSmall.copy(
                                    fontSize = 10.5.sp,
                                    fontWeight = FontWeight.Bold,
                                    letterSpacing = 0.5.sp
                                ),
                                color = cc.textMuted
                            )
                        }
                    } else {
                        val isSelected = opt.id == selectedId
                        DropdownMenuItem(
                            text = {
                                Column {
                                    Text(
                                        opt.title,
                                        style = MaterialTheme.typography.bodySmall.copy(
                                            fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal,
                                            fontSize = 13.sp
                                        ),
                                        color = cc.textPrimary
                                    )
                                    if (opt.subtitle != null) {
                                        Text(
                                            opt.subtitle,
                                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.sp),
                                            color = cc.textMuted
                                        )
                                    }
                                }
                            },
                            leadingIcon = opt.icon?.let { icon ->
                                {
                                    Icon(
                                        icon,
                                        contentDescription = null,
                                        tint = cc.textMuted,
                                        modifier = Modifier.size(16.dp)
                                    )
                                }
                            },
                            trailingIcon = if (isSelected) {
                                {
                                    Icon(
                                        Icons.Default.Check,
                                        contentDescription = "Selected",
                                        tint = cc.textPrimary,
                                        modifier = Modifier.size(16.dp)
                                    )
                                }
                            } else null,
                            onClick = {
                                expanded = false
                                onSelect(opt)
                            },
                            modifier = Modifier
                                .fillMaxWidth()
                                .heightIn(min = 36.dp)
                                .background(if (isSelected) cc.panelAlt else Color.Transparent)
                                .padding(horizontal = 4.dp)
                        )
                    }
                }
            }
        }
    }
}
