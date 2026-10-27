@file:Suppress("DEPRECATION")

package com.dialex.presentation.legal

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.dialex.presentation.settings.LegalTexts
import com.dialex.theme.LocalCcColors
import com.dialex.ui.DialexLogoView
import kotlinx.coroutines.delay

private enum class LegalViewerMode {
    NONE,
    TERMS,
    PRIVACY,
    LICENSE
}

/**
 * Mandatory first-use legal consent dialog.
 * Requires explicit affirmative consent to the Terms of Service, AI Advisory Notice,
 * and Privacy Policy / Zero-Telemetry Charter before using Dialex AI.
 */
@Composable
fun TermsConsentDialog(
    onAccept: () -> Unit,
    onDecline: () -> Unit,
    modifier: Modifier = Modifier
) {
    val cc = LocalCcColors.current
    val clipboard = LocalClipboardManager.current
    val scrollState = rememberScrollState()

    var termsChecked by remember { mutableStateOf(false) }
    var privacyChecked by remember { mutableStateOf(false) }
    var activeViewer by remember { mutableStateOf(LegalViewerMode.NONE) }
    var copiedLabel by remember { mutableStateOf<String?>(null) }
    var showDeclineAlert by remember { mutableStateOf(false) }

    LaunchedEffect(copiedLabel) {
        if (copiedLabel != null) {
            delay(2000)
            copiedLabel = null
        }
    }

    val canProceed = termsChecked && privacyChecked

    Dialog(
        onDismissRequest = { /* Non-dismissible without explicit action */ },
        properties = DialogProperties(
            dismissOnBackPress = false,
            dismissOnClickOutside = false,
            usePlatformDefaultWidth = false
        )
    ) {
        Box(
            modifier = modifier
                .fillMaxSize()
                .background(Color.Black.copy(alpha = 0.75f))
                .padding(16.dp),
            contentAlignment = Alignment.Center
        ) {
            Surface(
                shape = RoundedCornerShape(16.dp),
                color = cc.panel,
                border = BorderStroke(1.dp, cc.border.copy(alpha = 0.65f)),
                shadowElevation = 16.dp,
                modifier = Modifier
                    .widthIn(min = 340.dp, max = 680.dp)
                    .fillMaxHeight(0.92f)
            ) {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(24.dp)
                ) {
                    // ── Header ──────────────────────────────────────────────
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(16.dp)
                    ) {
                        DialexLogoView(size = 48.dp, shape = RoundedCornerShape(12.dp))
                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                "Welcome to Dialex AI",
                                style = MaterialTheme.typography.titleLarge.copy(
                                    fontWeight = FontWeight.Bold,
                                    fontSize = 20.sp
                                ),
                                color = cc.textPrimary
                            )
                            Text(
                                "Terms of Service & Privacy Policy Consent",
                                style = MaterialTheme.typography.bodySmall.copy(fontSize = 12.sp),
                                color = cc.textMuted
                            )
                        }
                    }

                    Spacer(Modifier.height(16.dp))
                    HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)
                    Spacer(Modifier.height(14.dp))

                    // ── Scrollable Body ─────────────────────────────────────
                    Column(
                        modifier = Modifier
                            .weight(1f)
                            .verticalScroll(scrollState),
                        verticalArrangement = Arrangement.spacedBy(14.dp)
                    ) {
                        Text(
                            "Before exploring multi-agent deliberation and sovereign AI consensus, please review and accept our operating terms and privacy charter.",
                            style = MaterialTheme.typography.bodyMedium.copy(fontSize = 12.5.sp, lineHeight = 18.sp),
                            color = cc.textPrimary
                        )

                        // 4 Legal Pillar Highlights
                        ConsentPillarCard(
                            icon = Icons.Outlined.Gavel,
                            title = "Non-Deterministic AI Advisory Disclaimer",
                            description = "Dialex orchestrates simulated AI debates. Outputs are probabilistic statistical inferences and do NOT constitute certified engineering, legal, financial, architectural, or medical advice. You bear sole responsibility for verifying all outputs before production adoption.",
                            cc = cc
                        )

                        ConsentPillarCard(
                            icon = Icons.Outlined.Key,
                            title = "BYOK & Upstream Model Compliance",
                            description = "When using API keys or CLI runners, requests travel directly from your machine to official provider endpoints (Anthropic, OpenAI, Google, etc.). You are responsible for token costs and compliance with provider acceptable use policies.",
                            cc = cc
                        )

                        ConsentPillarCard(
                            icon = Icons.Outlined.Security,
                            title = "Zero-Telemetry & Local-First Sovereignty",
                            description = "Dialex collects zero analytics, telemetry pixels, crash beacons, or prompts. Workspace context, session transcripts, and custom personas reside exclusively on your local host disk with OS-level credential encryption.",
                            cc = cc
                        )

                        ConsentPillarCard(
                            icon = Icons.Outlined.VerifiedUser,
                            title = "PolyForm Noncommercial License 1.0.0",
                            description = "Free for individuals, personal study, research, education, and noncommercial pursuits. Commercial use to generate revenue or run a business requires a commercial license from Keltstone Systems.",
                            cc = cc
                        )

                        // Interactive Document Viewers Toggle Row
                        Text(
                            "Review Full Legal Documents:",
                            style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 11.5.sp),
                            color = cc.textMuted,
                            modifier = Modifier.padding(top = 4.dp)
                        )

                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.spacedBy(8.dp)
                        ) {
                            LegalDocButton(
                                label = "Terms of Service",
                                isSelected = activeViewer == LegalViewerMode.TERMS,
                                cc = cc,
                                modifier = Modifier.weight(1f)
                            ) {
                                activeViewer = if (activeViewer == LegalViewerMode.TERMS) LegalViewerMode.NONE else LegalViewerMode.TERMS
                            }

                            LegalDocButton(
                                label = "Privacy Policy",
                                isSelected = activeViewer == LegalViewerMode.PRIVACY,
                                cc = cc,
                                modifier = Modifier.weight(1f)
                            ) {
                                activeViewer = if (activeViewer == LegalViewerMode.PRIVACY) LegalViewerMode.NONE else LegalViewerMode.PRIVACY
                            }

                            LegalDocButton(
                                label = "PolyForm License",
                                isSelected = activeViewer == LegalViewerMode.LICENSE,
                                cc = cc,
                                modifier = Modifier.weight(1f)
                            ) {
                                activeViewer = if (activeViewer == LegalViewerMode.LICENSE) LegalViewerMode.NONE else LegalViewerMode.LICENSE
                            }
                        }

                        // Expandable Document Text Box
                        AnimatedVisibility(
                            visible = activeViewer != LegalViewerMode.NONE,
                            enter = expandVertically(),
                            exit = shrinkVertically()
                        ) {
                            val (docTitle, docContent) = when (activeViewer) {
                                LegalViewerMode.TERMS -> "Terms of Service & AI Advisory Notice" to LegalTexts.FullTermsOfService
                                LegalViewerMode.PRIVACY -> "Privacy Policy & Data Sovereignty Charter" to LegalTexts.FullPrivacyPolicy
                                LegalViewerMode.LICENSE -> "PolyForm Noncommercial License 1.0.0" to LegalTexts.PolyFormSummary
                                LegalViewerMode.NONE -> "" to ""
                            }

                            Surface(
                                shape = RoundedCornerShape(10.dp),
                                color = if (cc.isDark) Color(0xFF13141B) else Color(0xFFF3F4F6),
                                border = BorderStroke(1.dp, cc.border.copy(alpha = 0.5f)),
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .heightIn(max = 240.dp)
                            ) {
                                Column(modifier = Modifier.padding(12.dp)) {
                                    Row(
                                        modifier = Modifier.fillMaxWidth(),
                                        horizontalArrangement = Arrangement.SpaceBetween,
                                        verticalAlignment = Alignment.CenterVertically
                                    ) {
                                        Text(
                                            docTitle,
                                            style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Bold),
                                            color = cc.accent
                                        )

                                        OutlinedButton(
                                            onClick = {
                                                clipboard.setText(AnnotatedString(docContent))
                                                copiedLabel = docTitle
                                            },
                                            shape = RoundedCornerShape(6.dp),
                                            modifier = Modifier.height(24.dp),
                                            contentPadding = PaddingValues(horizontal = 8.dp)
                                        ) {
                                            Icon(Icons.Outlined.ContentCopy, contentDescription = null, modifier = Modifier.size(11.dp), tint = cc.textMuted)
                                            Spacer(Modifier.width(4.dp))
                                            Text(
                                                if (copiedLabel == docTitle) "Copied!" else "Copy",
                                                style = MaterialTheme.typography.labelSmall.copy(fontSize = 10.sp),
                                                color = cc.textMuted
                                            )
                                        }
                                    }

                                    Spacer(Modifier.height(8.dp))

                                    SelectionContainer {
                                        Box(modifier = Modifier.weight(1f).verticalScroll(rememberScrollState())) {
                                            Text(
                                                text = docContent,
                                                style = MaterialTheme.typography.bodySmall.copy(
                                                    fontFamily = FontFamily.Monospace,
                                                    fontSize = 10.5.sp,
                                                    lineHeight = 15.sp
                                                ),
                                                color = cc.textPrimary.copy(alpha = 0.9f)
                                            )
                                        }
                                    }
                                }
                            }
                        }

                        Spacer(Modifier.height(4.dp))
                        HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)

                        // ── Required Checkboxes ─────────────────────────────
                        ConsentCheckboxRow(
                            checked = termsChecked,
                            onCheckedChange = { termsChecked = it },
                            text = "I have read, understood, and agree to the Terms of Service, Non-Deterministic AI Advisory Disclaimer, and PolyForm Noncommercial License 1.0.0.",
                            cc = cc
                        )

                        ConsentCheckboxRow(
                            checked = privacyChecked,
                            onCheckedChange = { privacyChecked = it },
                            text = "I acknowledge the Privacy Policy & Zero-Telemetry Charter, and accept responsibility for compliance with all connected AI provider terms.",
                            cc = cc
                        )

                        if (showDeclineAlert) {
                            Surface(
                                shape = RoundedCornerShape(8.dp),
                                color = Color(0xFFEF4444).copy(alpha = 0.12f),
                                border = BorderStroke(1.dp, Color(0xFFEF4444).copy(alpha = 0.4f)),
                                modifier = Modifier.fillMaxWidth()
                            ) {
                                Row(
                                    modifier = Modifier.padding(12.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                                ) {
                                    Icon(Icons.Outlined.Warning, contentDescription = null, tint = Color(0xFFEF4444), modifier = Modifier.size(16.dp))
                                    Text(
                                        "Accepting the Terms of Service and Privacy Policy is required to use Dialex AI. If you decline, please close the application.",
                                        style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.sp),
                                        color = cc.textPrimary
                                    )
                                }
                            }
                        }
                    }

                    Spacer(Modifier.height(14.dp))
                    HorizontalDivider(color = cc.border.copy(alpha = 0.35f), thickness = 0.75.dp)
                    Spacer(Modifier.height(14.dp))

                    // ── Bottom Action Buttons ───────────────────────────────
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        OutlinedButton(
                            onClick = {
                                showDeclineAlert = true
                                onDecline()
                            },
                            shape = RoundedCornerShape(8.dp),
                            modifier = Modifier.height(40.dp)
                        ) {
                            Text(
                                "Decline & Exit",
                                style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.Medium),
                                color = cc.textMuted
                            )
                        }

                        Button(
                            onClick = { if (canProceed) onAccept() },
                            enabled = canProceed,
                            shape = RoundedCornerShape(8.dp),
                            colors = ButtonDefaults.buttonColors(
                                containerColor = cc.accent,
                                contentColor = Color.White,
                                disabledContainerColor = cc.accent.copy(alpha = 0.3f),
                                disabledContentColor = Color.White.copy(alpha = 0.45f)
                            ),
                            modifier = Modifier
                                .weight(1f)
                                .height(40.dp)
                        ) {
                            Icon(Icons.Outlined.CheckCircle, contentDescription = null, modifier = Modifier.size(16.dp))
                            Spacer(Modifier.width(8.dp))
                            Text(
                                "Accept & Continue",
                                style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 13.sp)
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ConsentPillarCard(
    icon: ImageVector,
    title: String,
    description: String,
    cc: com.dialex.theme.CcPalette
) {
    Surface(
        shape = RoundedCornerShape(8.dp),
        color = cc.panelAlt,
        border = BorderStroke(0.75.dp, cc.border.copy(alpha = 0.4f)),
        modifier = Modifier.fillMaxWidth()
    ) {
        Row(
            modifier = Modifier.padding(12.dp),
            verticalAlignment = Alignment.Top,
            horizontalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Icon(icon, contentDescription = null, tint = cc.accent, modifier = Modifier.size(18.dp).padding(top = 1.dp))
            Column(verticalArrangement = Arrangement.spacedBy(3.dp)) {
                Text(
                    title,
                    style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold, fontSize = 12.sp),
                    color = cc.textPrimary
                )
                Text(
                    description,
                    style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.sp, lineHeight = 16.sp),
                    color = cc.textMuted
                )
            }
        }
    }
}

@Composable
private fun LegalDocButton(
    label: String,
    isSelected: Boolean,
    cc: com.dialex.theme.CcPalette,
    modifier: Modifier = Modifier,
    onClick: () -> Unit
) {
    Surface(
        shape = RoundedCornerShape(6.dp),
        color = if (isSelected) cc.accent.copy(alpha = 0.12f) else cc.panelAlt,
        border = BorderStroke(0.85.dp, if (isSelected) cc.accent else cc.border.copy(alpha = 0.4f)),
        modifier = modifier
            .clip(RoundedCornerShape(6.dp))
            .clickable(onClick = onClick)
    ) {
        Row(
            modifier = Modifier.padding(vertical = 6.dp, horizontal = 8.dp),
            horizontalArrangement = Arrangement.Center,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Icon(
                if (isSelected) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility,
                contentDescription = null,
                modifier = Modifier.size(12.dp),
                tint = if (isSelected) cc.accent else cc.textMuted
            )
            Spacer(Modifier.width(4.dp))
            Text(
                label,
                style = MaterialTheme.typography.labelSmall.copy(
                    fontSize = 10.5.sp,
                    fontWeight = if (isSelected) FontWeight.SemiBold else FontWeight.Normal
                ),
                color = if (isSelected) cc.accent else cc.textPrimary,
                maxLines = 1
            )
        }
    }
}

@Composable
private fun ConsentCheckboxRow(
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit,
    text: String,
    cc: com.dialex.theme.CcPalette
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(6.dp))
            .clickable { onCheckedChange(!checked) }
            .padding(vertical = 4.dp),
        verticalAlignment = Alignment.Top,
        horizontalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        Checkbox(
            checked = checked,
            onCheckedChange = onCheckedChange,
            colors = CheckboxDefaults.colors(
                checkedColor = cc.accent,
                checkmarkColor = Color.White,
                uncheckedColor = cc.border
            ),
            modifier = Modifier.size(20.dp).padding(top = 1.dp)
        )
        Text(
            text,
            style = MaterialTheme.typography.bodySmall.copy(fontSize = 11.5.sp, lineHeight = 16.5.sp),
            color = if (checked) cc.textPrimary else cc.textPrimary.copy(alpha = 0.85f)
        )
    }
}
