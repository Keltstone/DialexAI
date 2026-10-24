@file:Suppress("DEPRECATION")

package com.dialex.presentation.legal.licensing

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.DisableSelection
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.OpenInNew
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.dialex.theme.LocalCcColors

/**
 * Subtle, comprehensive statutory licensing dedication and legal information screen.
 * Engineered with understated executive typography to satisfy worldwide software
 * copyright, open-source attribution, and disclaimer requirements.
 */
@Composable
fun LicensingScreen(
    state: LicensingState,
    onIntent: (LicensingIntent) -> Unit,
    onBack: () -> Unit,
    modifier: Modifier = Modifier
) {
    val cc = LocalCcColors.current
    val clipboardManager = LocalClipboardManager.current
    val uriHandler = LocalUriHandler.current

    Surface(
        modifier = modifier.fillMaxSize(),
        color = cc.bg
    ) {
        Column(
            modifier = Modifier.fillMaxSize()
        ) {
            // ── Top Header Bar with Subtle Back Navigation ──────────────────
            Surface(
                color = cc.panel,
                border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.45f)),
                modifier = Modifier.fillMaxWidth()
            ) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp, vertical = 12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                        modifier = Modifier.weight(1f)
                    ) {
                        IconButton(
                            onClick = onBack,
                            modifier = Modifier.size(32.dp)
                        ) {
                            Icon(
                                Icons.AutoMirrored.Filled.ArrowBack,
                                contentDescription = "Return",
                                tint = cc.textPrimary,
                                modifier = Modifier.size(18.dp)
                            )
                        }

                        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                Text(
                                    "Licensing Dedication & Statutory Notices",
                                    style = MaterialTheme.typography.titleMedium.copy(
                                        fontWeight = FontWeight.SemiBold,
                                        fontSize = 15.sp
                                    ),
                                    color = cc.textPrimary
                                )

                                Surface(
                                    shape = RoundedCornerShape(4.dp),
                                    color = cc.accent.copy(alpha = 0.12f),
                                    border = BorderStroke(0.75.dp, cc.accent.copy(alpha = 0.35f))
                                ) {
                                    Text(
                                        "v${LicensingTexts.CURRENT_VERSION}",
                                        style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.sp, fontWeight = FontWeight.Medium),
                                        color = cc.accent,
                                        modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                                    )
                                }
                            }

                            Text(
                                "PolyForm Noncommercial 1.0.0 • Open Source Ledger • Statutory Disclaimers",
                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp),
                                color = cc.textMuted
                            )
                        }
                    }

                    // Action: Copy Full Legal Ledger
                    DisableSelection {
                        OutlinedButton(
                            onClick = {
                                val fullLedger = LicensingTexts.generateFullLegalLedger()
                                clipboardManager.setText(AnnotatedString(fullLedger))
                                onIntent(LicensingIntent.CopyFullLedger)
                            },
                            shape = RoundedCornerShape(6.dp),
                            modifier = Modifier.height(30.dp),
                            contentPadding = PaddingValues(horizontal = 10.dp)
                        ) {
                            Icon(
                                Icons.Outlined.ContentCopy,
                                contentDescription = null,
                                modifier = Modifier.size(13.dp),
                                tint = cc.textPrimary
                            )
                            Spacer(Modifier.width(6.dp))
                            Text(
                                "Copy Full Legal Ledger",
                                style = MaterialTheme.typography.labelSmall.copy(fontSize = 11.sp),
                                color = cc.textPrimary
                            )
                        }
                    }
                }
            }

            // ── Copied Feedback Toast Banner ─────────────────────────────────
            AnimatedVisibility(
                visible = state.copiedToast != null,
                enter = expandVertically() + fadeIn(),
                exit = shrinkVertically() + fadeOut()
            ) {
                Surface(
                    color = Color(0xFF10B981).copy(alpha = 0.15f),
                    border = BorderStroke(0.75.dp, Color(0xFF10B981).copy(alpha = 0.45f)),
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 16.dp, vertical = 6.dp),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.SpaceBetween
                    ) {
                        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                            Icon(Icons.Filled.Check, contentDescription = null, tint = Color(0xFF10B981), modifier = Modifier.size(14.dp))
                            Text(
                                state.copiedToast.orEmpty(),
                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp, fontWeight = FontWeight.Medium),
                                color = Color(0xFF10B981)
                            )
                        }
                        IconButton(
                            onClick = { onIntent(LicensingIntent.ClearCopiedToast) },
                            modifier = Modifier.size(20.dp)
                        ) {
                            Icon(Icons.Filled.Close, contentDescription = "Dismiss", tint = Color(0xFF10B981), modifier = Modifier.size(12.dp))
                        }
                    }
                }
            }

            // ── Section Switcher Tabs ────────────────────────────────────────
            DisableSelection {
                Surface(
                    color = cc.panelAlt,
                    border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.35f)),
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 12.dp, vertical = 6.dp),
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        LicensingSection.entries.forEach { section ->
                            val isSelected = state.selectedSection == section
                            val bg = if (isSelected) cc.panel else Color.Transparent
                            val border = if (isSelected) cc.border.copy(alpha = 0.6f) else Color.Transparent
                            val textColor = if (isSelected) cc.textPrimary else cc.textMuted

                            Surface(
                                shape = RoundedCornerShape(6.dp),
                                color = bg,
                                border = BorderStroke(0.75.dp, border),
                                modifier = Modifier
                                    .clip(RoundedCornerShape(6.dp))
                                    .clickable { onIntent(LicensingIntent.SelectSection(section)) }
                            ) {
                                Row(
                                    modifier = Modifier.padding(horizontal = 10.dp, vertical = 6.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                                ) {
                                    Text(
                                        section.title,
                                        style = MaterialTheme.typography.labelSmall.copy(
                                            fontSize = 11.5.sp,
                                            fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal
                                        ),
                                        color = textColor
                                    )

                                    if (section == LicensingSection.THIRD_PARTY) {
                                        Surface(
                                            shape = RoundedCornerShape(10.dp),
                                            color = cc.accent.copy(alpha = 0.15f),
                                            modifier = Modifier.padding(start = 2.dp)
                                        ) {
                                            Text(
                                                "${state.allNotices.size}",
                                                style = MaterialTheme.typography.labelSmall.copy(fontSize = 9.5.sp, fontWeight = FontWeight.Bold),
                                                color = cc.accent,
                                                modifier = Modifier.padding(horizontal = 5.dp, vertical = 1.dp)
                                            )
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }

            // ── Dynamic Section Viewport ─────────────────────────────────────
            AnimatedContent(
                targetState = state.selectedSection,
                transitionSpec = { fadeIn() togetherWith fadeOut() },
                modifier = Modifier
                    .weight(1f)
                    .fillMaxWidth()
            ) { section ->
                when (section) {
                    LicensingSection.PRIMARY -> PrimaryDedicationView(cc, clipboardManager, onIntent)
                    LicensingSection.THIRD_PARTY -> ThirdPartyComponentsView(state, cc, clipboardManager, uriHandler, onIntent)
                    LicensingSection.WARRANTY -> StatutoryWarrantyView(cc, clipboardManager, onIntent)
                    LicensingSection.AI_SAFE_HARBOR -> AiSafeHarborView(cc, clipboardManager, onIntent)
                    LicensingSection.SOVEREIGNTY -> DataSovereigntyView(cc, clipboardManager, onIntent)
                }
            }
        }
    }
}

// ── 1. Primary Dedication View ───────────────────────────────────────────────
@Composable
private fun PrimaryDedicationView(
    cc: com.dialex.theme.CcPalette,
    clipboardManager: androidx.compose.ui.platform.ClipboardManager,
    onIntent: (LicensingIntent) -> Unit
) {
    val scrollState = rememberScrollState()
    SelectionContainer {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(scrollState)
                .padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            Surface(
                shape = RoundedCornerShape(10.dp),
                color = cc.panel,
                border = BorderStroke(0.85.dp, cc.border.copy(alpha = 0.45f)),
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(
                    modifier = Modifier.padding(18.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp)
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                            Text(
                                "Dialex AI Core Software License",
                                style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 15.sp),
                                color = cc.textPrimary
                            )
                            Text(
                                "PolyForm Noncommercial License 1.0.0 • Kolta Labs & Contributors",
                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp),
                                color = cc.textMuted
                            )
                        }

                        DisableSelection {
                            OutlinedButton(
                                onClick = {
                                    clipboardManager.setText(AnnotatedString(LicensingTexts.PrimaryDedicationStatement))
                                    onIntent(LicensingIntent.CopyText("Primary Dedication Statement", LicensingTexts.PrimaryDedicationStatement))
                                },
                                shape = RoundedCornerShape(6.dp),
                                modifier = Modifier.height(28.dp),
                                contentPadding = PaddingValues(horizontal = 8.dp)
                            ) {
                                Icon(Icons.Outlined.ContentCopy, contentDescription = null, modifier = Modifier.size(12.dp), tint = cc.textMuted)
                                Spacer(Modifier.width(4.dp))
                                Text("Copy Statement", style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp), color = cc.textMuted)
                            }
                        }
                    }

                    HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)

                    // Rights pills
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        DedicationStatusPill("✔ Free for Individual & Academic Study", Color(0xFF10B981))
                        DedicationStatusPill("✔ Full Code Inspection & Local Execution", Color(0xFF10B981))
                        DedicationStatusPill("🔒 Commercial Use Requires Enterprise License", Color(0xFFF59E0B))
                    }

                    Text(
                        LicensingTexts.PrimaryDedicationStatement,
                        style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp, lineHeight = 18.5.sp),
                        color = cc.textPrimary.copy(alpha = 0.9f)
                    )
                }
            }

            // Monospace PolyForm Terms
            Surface(
                shape = RoundedCornerShape(8.dp),
                color = if (cc.isDark) Color(0xFF14151B) else Color(0xFFF3F4F6),
                border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.4f)),
                modifier = Modifier.fillMaxWidth()
            ) {
                Text(
                    text = com.dialex.presentation.settings.LegalTexts.PolyFormSummary,
                    style = MaterialTheme.typography.bodySmall.copy(
                        fontFamily = FontFamily.Monospace,
                        fontSize = 11.sp,
                        lineHeight = 16.5.sp
                    ),
                    color = cc.textMuted,
                    modifier = Modifier.padding(14.dp)
                )
            }
        }
    }
}

// ── 2. Third-Party Components & Notice Ledger ───────────────────────────────
@Composable
private fun ThirdPartyComponentsView(
    state: LicensingState,
    cc: com.dialex.theme.CcPalette,
    clipboardManager: androidx.compose.ui.platform.ClipboardManager,
    uriHandler: androidx.compose.ui.platform.UriHandler,
    onIntent: (LicensingIntent) -> Unit
) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        // Search & Filter Box
        OutlinedTextField(
            value = state.searchQuery,
            onValueChange = { onIntent(LicensingIntent.UpdateSearchQuery(it)) },
            placeholder = {
                Text(
                    "Filter dependencies by name, license type, or description...",
                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                    color = cc.textMuted
                )
            },
            leadingIcon = {
                Icon(Icons.Filled.Search, contentDescription = null, tint = cc.textMuted, modifier = Modifier.size(16.dp))
            },
            trailingIcon = {
                if (state.searchQuery.isNotBlank()) {
                    IconButton(
                        onClick = { onIntent(LicensingIntent.UpdateSearchQuery("")) },
                        modifier = Modifier.size(24.dp)
                    ) {
                        Icon(Icons.Filled.Close, contentDescription = "Clear", tint = cc.textMuted, modifier = Modifier.size(14.dp))
                    }
                }
            },
            singleLine = true,
            shape = RoundedCornerShape(8.dp),
            colors = OutlinedTextFieldDefaults.colors(
                focusedContainerColor = cc.panel,
                unfocusedContainerColor = cc.panelAlt,
                focusedBorderColor = cc.accent.copy(alpha = 0.6f),
                unfocusedBorderColor = cc.border.copy(alpha = 0.4f),
                cursorColor = cc.accent
            ),
            modifier = Modifier.fillMaxWidth()
        )

        // Notice Count & Attribution Disclaimer
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Text(
                "Displaying ${state.filteredNotices.size} of ${state.allNotices.size} open-source components",
                style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.sp),
                color = cc.textMuted
            )

            Text(
                "All dependencies are permissively licensed (Apache 2.0 / BSD / MIT)",
                style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.sp, fontWeight = FontWeight.Medium),
                color = Color(0xFF10B981)
            )
        }

        // List of Third-Party Components
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            items(state.filteredNotices, key = { it.id }) { item ->
                val isExpanded = state.expandedNoticeIds.contains(item.id)

                Surface(
                    shape = RoundedCornerShape(8.dp),
                    color = cc.panel,
                    border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.45f)),
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Column(modifier = Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.SpaceBetween
                        ) {
                            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                    Text(
                                        item.name,
                                        style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 13.5.sp),
                                        color = cc.textPrimary
                                    )

                                    Surface(
                                        shape = RoundedCornerShape(4.dp),
                                        color = cc.accent.copy(alpha = 0.12f),
                                        border = BorderStroke(0.5.dp, cc.accent.copy(alpha = 0.35f))
                                    ) {
                                        Text(
                                            item.licenseName,
                                            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.sp, fontWeight = FontWeight.Medium),
                                            color = cc.accent,
                                            modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp)
                                        )
                                    }
                                }

                                Text(
                                    item.copyright,
                                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.sp),
                                    color = cc.textMuted
                                )
                            }

                            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                DisableSelection {
                                    IconButton(
                                        onClick = {
                                            clipboardManager.setText(AnnotatedString("${item.name}\n${item.copyright}\nLicense: ${item.licenseName}\n\n${item.noticeText}"))
                                            onIntent(LicensingIntent.CopyText(item.name, item.noticeText))
                                        },
                                        modifier = Modifier.size(28.dp)
                                    ) {
                                        Icon(Icons.Outlined.ContentCopy, contentDescription = "Copy Notice", tint = cc.textMuted, modifier = Modifier.size(13.dp))
                                    }

                                    IconButton(
                                        onClick = { uriHandler.openUri(item.licenseUrl) },
                                        modifier = Modifier.size(28.dp)
                                    ) {
                                        Icon(Icons.AutoMirrored.Outlined.OpenInNew, contentDescription = "Open License", tint = cc.textMuted, modifier = Modifier.size(13.dp))
                                    }

                                    IconButton(
                                        onClick = { onIntent(LicensingIntent.ToggleNoticeExpansion(item.id)) },
                                        modifier = Modifier.size(28.dp)
                                    ) {
                                        Icon(
                                            if (isExpanded) Icons.Filled.KeyboardArrowUp else Icons.Filled.KeyboardArrowDown,
                                            contentDescription = "Expand",
                                            tint = cc.textPrimary,
                                            modifier = Modifier.size(16.dp)
                                        )
                                    }
                                }
                            }
                        }

                        Text(
                            item.description,
                            style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp, lineHeight = 17.5.sp),
                            color = cc.textPrimary.copy(alpha = 0.85f)
                        )

                        // Expandable Notice Body
                        AnimatedVisibility(
                            visible = isExpanded,
                            enter = expandVertically() + fadeIn(),
                            exit = shrinkVertically() + fadeOut()
                        ) {
                            Surface(
                                shape = RoundedCornerShape(6.dp),
                                color = if (cc.isDark) Color(0xFF131418) else Color(0xFFF3F4F6),
                                border = BorderStroke(0.5.dp, cc.border.copy(alpha = 0.35f)),
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(top = 6.dp)
                            ) {
                                SelectionContainer {
                                    Text(
                                        text = item.noticeText,
                                        style = MaterialTheme.typography.bodySmall.copy(
                                            fontFamily = FontFamily.Monospace,
                                            fontSize = 10.5.sp,
                                            lineHeight = 15.sp
                                        ),
                                        color = cc.textMuted,
                                        modifier = Modifier.padding(10.dp)
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

// ── 3. Statutory Warranty View ───────────────────────────────────────────────
@Composable
private fun StatutoryWarrantyView(
    cc: com.dialex.theme.CcPalette,
    clipboardManager: androidx.compose.ui.platform.ClipboardManager,
    onIntent: (LicensingIntent) -> Unit
) {
    val scrollState = rememberScrollState()
    SelectionContainer {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(scrollState)
                .padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            Surface(
                shape = RoundedCornerShape(10.dp),
                color = cc.panel,
                border = BorderStroke(0.85.dp, cc.border.copy(alpha = 0.45f)),
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(modifier = Modifier.padding(18.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            "Statutory Disclaimer of Warranties",
                            style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 15.sp),
                            color = cc.textPrimary
                        )

                        DisableSelection {
                            OutlinedButton(
                                onClick = {
                                    clipboardManager.setText(AnnotatedString(LicensingTexts.StatutoryWarrantyDisclaimer))
                                    onIntent(LicensingIntent.CopyText("Warranty Disclaimer", LicensingTexts.StatutoryWarrantyDisclaimer))
                                },
                                shape = RoundedCornerShape(6.dp),
                                modifier = Modifier.height(28.dp),
                                contentPadding = PaddingValues(horizontal = 8.dp)
                            ) {
                                Icon(Icons.Outlined.ContentCopy, contentDescription = null, modifier = Modifier.size(12.dp), tint = cc.textMuted)
                                Spacer(Modifier.width(4.dp))
                                Text("Copy Disclaimer", style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp), color = cc.textMuted)
                            }
                        }
                    }

                    HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)

                    Surface(
                        shape = RoundedCornerShape(8.dp),
                        color = if (cc.isDark) Color(0xFF14151B) else Color(0xFFF3F4F6),
                        border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.4f)),
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text(
                            text = LicensingTexts.StatutoryWarrantyDisclaimer,
                            style = MaterialTheme.typography.bodySmall.copy(
                                fontFamily = FontFamily.Monospace,
                                fontSize = 11.sp,
                                lineHeight = 16.5.sp
                            ),
                            color = cc.textMuted,
                            modifier = Modifier.padding(14.dp)
                        )
                    }
                }
            }

            Surface(
                shape = RoundedCornerShape(10.dp),
                color = cc.panel,
                border = BorderStroke(0.85.dp, cc.border.copy(alpha = 0.45f)),
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(modifier = Modifier.padding(18.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            "Comprehensive Limitation of Liability",
                            style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 15.sp),
                            color = cc.textPrimary
                        )

                        DisableSelection {
                            OutlinedButton(
                                onClick = {
                                    clipboardManager.setText(AnnotatedString(LicensingTexts.StatutoryLimitationOfLiability))
                                    onIntent(LicensingIntent.CopyText("Liability Limitation", LicensingTexts.StatutoryLimitationOfLiability))
                                },
                                shape = RoundedCornerShape(6.dp),
                                modifier = Modifier.height(28.dp),
                                contentPadding = PaddingValues(horizontal = 8.dp)
                            ) {
                                Icon(Icons.Outlined.ContentCopy, contentDescription = null, modifier = Modifier.size(12.dp), tint = cc.textMuted)
                                Spacer(Modifier.width(4.dp))
                                Text("Copy Limitation", style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp), color = cc.textMuted)
                            }
                        }
                    }

                    HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)

                    Surface(
                        shape = RoundedCornerShape(8.dp),
                        color = if (cc.isDark) Color(0xFF14151B) else Color(0xFFF3F4F6),
                        border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.4f)),
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text(
                            text = LicensingTexts.StatutoryLimitationOfLiability,
                            style = MaterialTheme.typography.bodySmall.copy(
                                fontFamily = FontFamily.Monospace,
                                fontSize = 11.sp,
                                lineHeight = 16.5.sp
                            ),
                            color = cc.textMuted,
                            modifier = Modifier.padding(14.dp)
                        )
                    }
                }
            }
        }
    }
}

// ── 4. AI Advisory & Safe Harbor View ────────────────────────────────────────
@Composable
private fun AiSafeHarborView(
    cc: com.dialex.theme.CcPalette,
    clipboardManager: androidx.compose.ui.platform.ClipboardManager,
    onIntent: (LicensingIntent) -> Unit
) {
    val scrollState = rememberScrollState()
    SelectionContainer {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(scrollState)
                .padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            Surface(
                shape = RoundedCornerShape(10.dp),
                color = cc.panel,
                border = BorderStroke(0.85.dp, cc.border.copy(alpha = 0.45f)),
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(modifier = Modifier.padding(18.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            "AI Deliberation & Non-Deterministic Safe Harbor",
                            style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 15.sp),
                            color = cc.textPrimary
                        )

                        DisableSelection {
                            OutlinedButton(
                                onClick = {
                                    clipboardManager.setText(AnnotatedString(LicensingTexts.AiAdvisorySafeHarbor))
                                    onIntent(LicensingIntent.CopyText("AI Safe Harbor", LicensingTexts.AiAdvisorySafeHarbor))
                                },
                                shape = RoundedCornerShape(6.dp),
                                modifier = Modifier.height(28.dp),
                                contentPadding = PaddingValues(horizontal = 8.dp)
                            ) {
                                Icon(Icons.Outlined.ContentCopy, contentDescription = null, modifier = Modifier.size(12.dp), tint = cc.textMuted)
                                Spacer(Modifier.width(4.dp))
                                Text("Copy Notice", style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp), color = cc.textMuted)
                            }
                        }
                    }

                    HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)

                    Text(
                        LicensingTexts.AiAdvisorySafeHarbor,
                        style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp, lineHeight = 19.sp),
                        color = cc.textPrimary.copy(alpha = 0.9f)
                    )
                }
            }
        }
    }
}

// ── 5. Data Sovereignty View ─────────────────────────────────────────────────
@Composable
private fun DataSovereigntyView(
    cc: com.dialex.theme.CcPalette,
    clipboardManager: androidx.compose.ui.platform.ClipboardManager,
    onIntent: (LicensingIntent) -> Unit
) {
    val scrollState = rememberScrollState()
    SelectionContainer {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(scrollState)
                .padding(20.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp)
        ) {
            Surface(
                shape = RoundedCornerShape(10.dp),
                color = cc.panel,
                border = BorderStroke(0.85.dp, cc.border.copy(alpha = 0.45f)),
                modifier = Modifier.fillMaxWidth()
            ) {
                Column(modifier = Modifier.padding(18.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text(
                            "Data Sovereignty & Zero-Telemetry Charter",
                            style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 15.sp),
                            color = cc.textPrimary
                        )

                        DisableSelection {
                            OutlinedButton(
                                onClick = {
                                    clipboardManager.setText(AnnotatedString(LicensingTexts.DataSovereigntyDeclaration))
                                    onIntent(LicensingIntent.CopyText("Data Sovereignty Charter", LicensingTexts.DataSovereigntyDeclaration))
                                },
                                shape = RoundedCornerShape(6.dp),
                                modifier = Modifier.height(28.dp),
                                contentPadding = PaddingValues(horizontal = 8.dp)
                            ) {
                                Icon(Icons.Outlined.ContentCopy, contentDescription = null, modifier = Modifier.size(12.dp), tint = cc.textMuted)
                                Spacer(Modifier.width(4.dp))
                                Text("Copy Charter", style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp), color = cc.textMuted)
                            }
                        }
                    }

                    HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)

                    Text(
                        LicensingTexts.DataSovereigntyDeclaration,
                        style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp, lineHeight = 19.sp),
                        color = cc.textPrimary.copy(alpha = 0.9f)
                    )
                }
            }
        }
    }
}

@Composable
private fun DedicationStatusPill(label: String, color: Color) {
    Surface(
        shape = RoundedCornerShape(5.dp),
        color = color.copy(alpha = 0.12f),
        border = BorderStroke(0.75.dp, color.copy(alpha = 0.35f))
    ) {
        Text(
            label,
            style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.5.sp, fontWeight = FontWeight.Medium),
            color = color,
            modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp)
        )
    }
}
